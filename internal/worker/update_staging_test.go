package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	fgupdate "forgegrid/internal/update"
)

func TestCopyFileRefusesSamePathInsteadOfSilentlyTruncating(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.exe")
	const content = "not empty"
	if err := os.WriteFile(path, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}

	err := copyFile(path, path, 0700)
	if err == nil {
		t.Fatal("copyFile(path, path, ...) returned no error; it must refuse rather than silently truncate the file")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading file after refused copy: %v", readErr)
	}
	if string(got) != content {
		t.Fatalf("file content changed to %q after a refused same-path copy; must be left untouched", got)
	}
}

func TestStagedArtifactPathNeverCollidesWithSource(t *testing.T) {
	updateDir := "/data/updates/update-1"
	cases := []string{
		filepath.Join(updateDir, "downloaded-ForgeGrid.exe"),                // real download-path shape
		"/dev/6 Laptops/ForgeGrid/dist/ForgeGrid-USB/Windows/ForgeGrid.exe", // local-candidate-path shape
		"ForgeGrid.exe",
	}
	for _, source := range cases {
		staged := stagedArtifactPath(updateDir, source)
		if staged == source {
			t.Fatalf("stagedArtifactPath(%q, %q) = %q, must never equal source", updateDir, source, staged)
		}
	}
}

// TestStagingPathNeverCollidesWithDownloadedSource is an end-to-end
// reproduction of the real Laptop03/Laptop04 canary bug: stagedPath was
// computed as filepath.Join(updateDir, filepath.Base(source)), which for a
// downloaded artifact (source == updateDir/downloaded-<name>) reproduces
// that exact same path. copyFile(source, stagedPath, ...) then opened that
// one path for reading and, simultaneously, for O_TRUNC writing, truncating
// it to zero bytes before ever reading it - silently staging an empty file
// that always failed the *second* checksum check (the first, right after
// download, always passed, which is why every real failure said "Staged
// update failed checksum verification", never "Update package failed").
//
// This exercises resolveUpdateSource's real download path (a real HTTP
// server, real bytes) and the same stagedPath derivation stageUpdate uses,
// without invoking stageUpdate itself - past this point it proceeds into
// real binary-replacement/restart logic that must not run inside a test
// process.
func TestStagingPathNeverCollidesWithDownloadedSource(t *testing.T) {
	const artifactBody = "pretend worker binary bytes, long enough to matter"
	sum := sha256.Sum256([]byte(artifactBody))
	expectedSHA := hex.EncodeToString(sum[:])

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(artifactBody))
	}))
	defer ts.Close()

	w := &Worker{
		WorkerID:       "worker-1",
		Token:          "tok-1",
		CoordinatorURL: ts.URL,
		Client:         &http.Client{},
		DownloadClient: &http.Client{},
	}
	updateDir := t.TempDir()
	req := fgupdate.Request{
		ID:       "update-1",
		Artifact: fgupdate.Artifact{Path: "Windows/ForgeGrid.exe", SHA256: expectedSHA},
	}

	source, err := w.resolveUpdateSource(req, updateDir)
	if err != nil {
		t.Fatalf("resolveUpdateSource: %v", err)
	}
	if err := fgupdate.VerifyFile(source, expectedSHA); err != nil {
		t.Fatalf("downloaded source failed checksum verification (unexpected): %v", err)
	}

	// The actual production helper stageUpdate calls - not a re-derivation -
	// so a regression here means stageUpdate itself would regress too.
	stagedPath := stagedArtifactPath(updateDir, source)
	if stagedPath == source {
		t.Fatalf("stagedPath (%s) must never equal source (%s)", stagedPath, source)
	}

	if err := copyFile(source, stagedPath, 0700); err != nil {
		t.Fatalf("copyFile(source, stagedPath): %v", err)
	}
	if err := fgupdate.VerifyFile(stagedPath, expectedSHA); err != nil {
		t.Fatalf("staged file failed checksum verification (this is the real bug's exact symptom): %v", err)
	}
}
