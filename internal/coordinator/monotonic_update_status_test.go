package coordinator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"forgegrid/internal/models"
)

func postUpdateReport(t *testing.T, c *Coordinator, workerID, updateID, status string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"worker_id": workerID,
		"update_id": updateID,
		"status":    status,
		"message":   "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/updates/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer worker-token")
	w := httptest.NewRecorder()
	c.handleWorkerUpdateReport(w, req)
	return w
}

// TestTerminalUpdateStatusCannotBeRegressedByStaleReport is the direct
// Finding D regression test: once an update reaches a terminal status
// (completed/failed/rolled_back/rollback_failed), a later report claiming
// a non-terminal status - exactly what a stale, delayed-delivery pending
// report redelivered from the worker's heartbeat-loop retry queue would
// look like - must never overwrite it.
func TestTerminalUpdateStatusCannotBeRegressedByStaleReport(t *testing.T) {
	c := testCoordinator(t)
	c.Store.Workers["worker-1"].UpdateRequest = &models.WorkerUpdateRequest{
		ID:     "update-1",
		Status: "completed",
	}

	w := postUpdateReport(t, c, "worker-1", "update-1", "running")

	if w.Code != http.StatusOK {
		t.Fatalf("expected the stale report to be acknowledged (200) rather than rejected, got %d body=%s", w.Code, w.Body.String())
	}
	if got := c.Store.Workers["worker-1"].UpdateRequest.Status; got != "completed" {
		t.Fatalf("terminal status must not regress: expected 'completed' to survive a stale 'running' report, got %q", got)
	}
}

// TestNonTerminalUpdateStatusCanStillProgressNormally proves the guard is
// scoped correctly: an ordinary, in-order progression between non-terminal
// statuses must continue to work exactly as before.
func TestNonTerminalUpdateStatusCanStillProgressNormally(t *testing.T) {
	c := testCoordinator(t)
	c.Store.Workers["worker-1"].UpdateRequest = &models.WorkerUpdateRequest{
		ID:     "update-2",
		Status: "queued",
	}

	w := postUpdateReport(t, c, "worker-1", "update-2", "running")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if got := c.Store.Workers["worker-1"].UpdateRequest.Status; got != "running" {
		t.Fatalf("expected a normal non-terminal transition to still apply, got %q", got)
	}
}

// TestTerminalUpdateStatusCanBeOverwrittenByAnotherTerminalStatus ensures
// the guard only blocks a regression to *non*-terminal, not a legitimate
// later terminal correction (e.g. a duplicate "completed" resend, which
// must remain a safe no-op idempotent write).
func TestTerminalUpdateStatusCanBeOverwrittenByAnotherTerminalStatus(t *testing.T) {
	c := testCoordinator(t)
	c.Store.Workers["worker-1"].UpdateRequest = &models.WorkerUpdateRequest{
		ID:     "update-3",
		Status: "completed",
	}

	w := postUpdateReport(t, c, "worker-1", "update-3", "completed")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if got := c.Store.Workers["worker-1"].UpdateRequest.Status; got != "completed" {
		t.Fatalf("expected status to remain 'completed', got %q", got)
	}
}
