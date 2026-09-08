package coordinator

import (
	"net/http"
	"net/http/httptest"
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
