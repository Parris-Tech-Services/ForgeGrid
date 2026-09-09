package coordinator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"forgegrid/internal/models"
)

// TestUpdateArtifactDownloadServesBytesForAuthenticatedWorker guards the
// fix for the bug found running the real Laptop03 canary: a remote worker
// has no shared filesystem with the coordinator, so the manifest's
// bundle-relative ArtifactPath is meaningless there. Downloading the bytes
// over this endpoint, authenticated the same way polling/reporting already
// are, is what makes updates actually reach a genuinely remote worker.
func TestUpdateArtifactDownloadServesBytesForAuthenticatedWorker(t *testing.T) {
	c := testCoordinator(t)
	writeUpdateManifest(t, c.Store.Dir(), "0.9.0")
	c.Store.Workers["worker-1"].UpdateRequest = &models.WorkerUpdateRequest{
		ID:           "update-1",
		ArtifactPath: "Windows/ForgeGrid.exe",
		Status:       "queued",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/updates/artifact?worker_id=worker-1&update_id=update-1", nil)
	req.Header.Set("Authorization", "Bearer worker-token")
	w := httptest.NewRecorder()
	c.handleUpdateArtifactDownload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != "windows amd64 exe" {
		t.Fatalf("unexpected body: %q", w.Body.String())
	}
}

func TestUpdateArtifactDownloadRejectsWrongToken(t *testing.T) {
	c := testCoordinator(t)
	writeUpdateManifest(t, c.Store.Dir(), "0.9.0")
	c.Store.Workers["worker-1"].UpdateRequest = &models.WorkerUpdateRequest{
		ID:           "update-1",
		ArtifactPath: "Windows/ForgeGrid.exe",
		Status:       "queued",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/updates/artifact?worker_id=worker-1&update_id=update-1", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	c.handleUpdateArtifactDownload(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateArtifactDownloadRejectsMismatchedUpdateID(t *testing.T) {
	c := testCoordinator(t)
	writeUpdateManifest(t, c.Store.Dir(), "0.9.0")
	c.Store.Workers["worker-1"].UpdateRequest = &models.WorkerUpdateRequest{
		ID:           "update-1",
		ArtifactPath: "Windows/ForgeGrid.exe",
		Status:       "queued",
	}

	// A stale/forged update_id (e.g. from a previous, already-superseded
	// update) must not be served the current artifact.
	req := httptest.NewRequest(http.MethodGet, "/api/updates/artifact?worker_id=worker-1&update_id=some-other-update", nil)
	req.Header.Set("Authorization", "Bearer worker-token")
	w := httptest.NewRecorder()
	c.handleUpdateArtifactDownload(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", w.Code, w.Body.String())
	}
}

// TestUpdateArtifactDownloadCannotEscapeBundleDir proves a crafted
// ArtifactPath (which would only ever get into the store via a coordinator
// bug or a compromised admin session) can't be used to read arbitrary files
// off the coordinator's disk.
func TestUpdateArtifactDownloadCannotEscapeBundleDir(t *testing.T) {
	c := testCoordinator(t)
	writeUpdateManifest(t, c.Store.Dir(), "0.9.0")
	c.Store.Workers["worker-1"].UpdateRequest = &models.WorkerUpdateRequest{
		ID:           "update-1",
		ArtifactPath: "../../../../../../etc/passwd",
		Status:       "queued",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/updates/artifact?worker_id=worker-1&update_id=update-1", nil)
	req.Header.Set("Authorization", "Bearer worker-token")
	w := httptest.NewRecorder()
	c.handleUpdateArtifactDownload(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (path must stay confined to the bundle dir), body=%s", w.Code, w.Body.String())
	}
}

// TestUpdateArtifactDownloadServesLargeRealBinary reproduces the exact
// Laptop03 canary failure (staged artifact hashed to the empty-file SHA-256)
// using the real ~13MB built binary instead of a short synthetic string, to
// isolate whether file size is the variable.
func TestUpdateArtifactDownloadServesLargeRealBinary(t *testing.T) {
	c := testCoordinator(t)
	realExe := "../../dist/ForgeGrid-USB/Windows/ForgeGrid.exe"
	data, err := os.ReadFile(realExe)
	if err != nil {
		t.Skipf("real built binary not present at %s, skipping: %v", realExe, err)
	}
	dir := filepath.Join(c.Store.Dir(), "Windows")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ForgeGrid.exe"), data, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	manifest := map[string]interface{}{
		"schema_version": "1", "product": "ForgeGrid", "version": "0.9.0", "commit": "test",
		"artifacts": []map[string]interface{}{{
			"role": "worker", "platform": "windows", "architecture": "amd64",
			"sha256": hex.EncodeToString(hash[:]), "path": "Windows/ForgeGrid.exe", "size": len(data),
		}},
	}
	b, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(c.Store.Dir(), "update-manifest.json"), b, 0600); err != nil {
		t.Fatal(err)
	}

	c.Store.Workers["worker-1"].UpdateRequest = &models.WorkerUpdateRequest{
		ID: "update-1", ArtifactPath: "Windows/ForgeGrid.exe", Status: "queued",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/updates/artifact?worker_id=worker-1&update_id=update-1", nil)
	req.Header.Set("Authorization", "Bearer worker-token")
	w := httptest.NewRecorder()
	c.handleUpdateArtifactDownload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := len(w.Body.Bytes()); got != len(data) {
		t.Fatalf("got %d bytes, want %d bytes", got, len(data))
	}
}

// TestUpdateArtifactDownloadOverRealServerLargeFile goes through a real
// httptest.Server (real net.Listener, real http.Server, real TCP), unlike
// httptest.NewRecorder() which bypasses http.Server entirely and so can
// never reproduce a transport/timeout-level bug.
func TestUpdateArtifactDownloadOverRealServerLargeFile(t *testing.T) {
	c := testCoordinator(t)
	realExe := "../../dist/ForgeGrid-USB/Windows/ForgeGrid.exe"
	data, err := os.ReadFile(realExe)
	if err != nil {
		t.Skipf("real built binary not present at %s, skipping: %v", realExe, err)
	}
	dir := filepath.Join(c.Store.Dir(), "Windows")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ForgeGrid.exe"), data, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	manifest := map[string]interface{}{
		"schema_version": "1", "product": "ForgeGrid", "version": "0.9.0", "commit": "test",
		"artifacts": []map[string]interface{}{{
			"role": "worker", "platform": "windows", "architecture": "amd64",
			"sha256": hex.EncodeToString(hash[:]), "path": "Windows/ForgeGrid.exe", "size": len(data),
		}},
	}
	b, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(c.Store.Dir(), "update-manifest.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	c.Store.Workers["worker-1"].UpdateRequest = &models.WorkerUpdateRequest{
		ID: "update-1", ArtifactPath: "Windows/ForgeGrid.exe", Status: "queued",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/updates/artifact", c.handleUpdateArtifactDownload)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/updates/artifact?worker_id=worker-1&update_id=update-1", nil)
	req.Header.Set("Authorization", "Bearer worker-token")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	t.Logf("status=%d content-length header=%q bytes read=%d", resp.StatusCode, resp.Header.Get("Content-Length"), len(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
	}
	if len(body) != len(data) {
		t.Fatalf("got %d bytes, want %d bytes", len(body), len(data))
	}
}
