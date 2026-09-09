package worker

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	fgupdate "forgegrid/internal/update"
)

func TestLocalCandidatePathRejectsMissingAndDirPaths(t *testing.T) {
	if got := localCandidatePath(""); got != "" {
		t.Fatalf("empty path: got %q, want empty", got)
	}
	if got := localCandidatePath(filepath.Join(t.TempDir(), "does-not-exist.exe")); got != "" {
		t.Fatalf("missing file: got %q, want empty", got)
	}
	dir := t.TempDir()
	if got := localCandidatePath(dir); got != "" {
		t.Fatalf("directory: got %q, want empty (must be a regular file)", got)
	}
	file := filepath.Join(dir, "candidate.exe")
	if err := os.WriteFile(file, []byte("x"), 0700); err != nil {
		t.Fatal(err)
	}
	if got := localCandidatePath(file); got != file {
		t.Fatalf("existing file: got %q, want %q", got, file)
	}
}

// TestResolveUpdateSourceFallsBackToDownload guards the fix for the bug
// found running the real Laptop03 canary: req.Artifact.Path is a
// coordinator-local bundle path ("Windows/ForgeGrid.exe") that resolves to
// nothing on a genuinely remote worker (it was being joined against the
// service's cwd, e.g. C:\Windows\system32, and failing checksum
// verification because the file simply wasn't there). resolveUpdateSource
// must recognize the local path doesn't exist and download the artifact
// from the coordinator instead.
func TestResolveUpdateSourceFallsBackToDownload(t *testing.T) {
	const artifactBody = "pretend worker binary bytes"
	var gotWorkerID, gotUpdateID, gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWorkerID = r.URL.Query().Get("worker_id")
		gotUpdateID = r.URL.Query().Get("update_id")
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(artifactBody))
	}))
	defer ts.Close()

	w := &Worker{
		WorkerID:       "worker-1",
		Token:          "tok-1",
		CoordinatorURL: ts.URL,
		Client:         &http.Client{},
	}

	req := fgupdate.Request{
		ID: "update-1",
		Artifact: fgupdate.Artifact{
			// Never valid on this machine: no such path exists here, so
			// resolveUpdateSource must fall through to the download path
			// rather than failing checksum verification against it.
			Path: filepath.Join(t.TempDir(), "nonexistent", "Windows", "ForgeGrid.exe"),
		},
	}

	source, err := w.resolveUpdateSource(req, t.TempDir())
	if err != nil {
		t.Fatalf("resolveUpdateSource: %v", err)
	}
	got, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("reading resolved source: %v", err)
	}
	if string(got) != artifactBody {
		t.Fatalf("downloaded content = %q, want %q", got, artifactBody)
	}
	if gotWorkerID != "worker-1" || gotUpdateID != "update-1" {
		t.Fatalf("download request had worker_id=%q update_id=%q, want worker-1/update-1", gotWorkerID, gotUpdateID)
	}
	if gotAuth != "Bearer tok-1" {
		t.Fatalf("download request Authorization = %q, want %q", gotAuth, "Bearer tok-1")
	}
}

// TestSetupClientGivesDownloadClientALongerTimeout guards the fix for the
// real Laptop03 canary failure: the artifact download silently returned
// zero bytes (not an error) because it shared Client's 10s overall
// http.Client.Timeout, sized for small poll/report JSON calls, not a
// multi-MB binary transfer to a physical remote machine. Verified against
// the real coordinator+curl (which is fast/local and succeeds) versus the
// real Windows worker (slower real network, empty body) before concluding
// the timeout, not the coordinator, was the cause.
func TestSetupClientGivesDownloadClientALongerTimeout(t *testing.T) {
	w := &Worker{}
	w.SetupClient("")
	if w.DownloadClient == nil {
		t.Fatal("SetupClient did not set DownloadClient")
	}
	if w.DownloadClient.Timeout <= w.Client.Timeout {
		t.Fatalf("DownloadClient.Timeout = %v, want it longer than Client.Timeout = %v", w.DownloadClient.Timeout, w.Client.Timeout)
	}
}

// TestDownloadUpdateArtifactUsesDownloadClientNotClient proves the artifact
// download actually goes through DownloadClient rather than the short-lived
// Client used for polling/reporting: Client here has a near-zero timeout
// that would fail against any real server, while DownloadClient points at
// the real working test server.
func TestDownloadUpdateArtifactUsesDownloadClientNotClient(t *testing.T) {
	const artifactBody = "pretend worker binary bytes"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(artifactBody))
	}))
	defer ts.Close()

	w := &Worker{
		WorkerID:       "worker-1",
		Token:          "tok-1",
		CoordinatorURL: ts.URL,
		Client:         &http.Client{Timeout: 1 * time.Nanosecond},
		DownloadClient: &http.Client{},
	}

	req := fgupdate.Request{ID: "update-1", Artifact: fgupdate.Artifact{Path: filepath.Join(t.TempDir(), "nonexistent.exe")}}
	source, err := w.resolveUpdateSource(req, t.TempDir())
	if err != nil {
		t.Fatalf("resolveUpdateSource: %v (Client's near-zero timeout would fail; DownloadClient should have been used instead)", err)
	}
	got, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("reading resolved source: %v", err)
	}
	if string(got) != artifactBody {
		t.Fatalf("downloaded content = %q, want %q", got, artifactBody)
	}
}

func TestResolveUpdateSourcePrefersExistingLocalPath(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "ForgeGrid.exe")
	if err := os.WriteFile(local, []byte("local bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	downloadCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadCalled = true
		w.Write([]byte("should not be used"))
	}))
	defer ts.Close()

	w := &Worker{CoordinatorURL: ts.URL, Client: &http.Client{}}
	req := fgupdate.Request{ID: "update-1", Artifact: fgupdate.Artifact{Path: local}}

	source, err := w.resolveUpdateSource(req, t.TempDir())
	if err != nil {
		t.Fatalf("resolveUpdateSource: %v", err)
	}
	if source != local {
		t.Fatalf("source = %q, want %q (should use the existing local file, not download)", source, local)
	}
	if downloadCalled {
		t.Fatalf("download endpoint was hit even though a valid local path existed")
	}
}
