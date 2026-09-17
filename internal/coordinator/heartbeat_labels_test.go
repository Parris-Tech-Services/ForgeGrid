package coordinator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleHeartbeatDoesNotOverwriteAdminSetLabels is a regression test for a
// real bug found live: a worker's heartbeat payload sending a non-nil (often
// empty) "labels" field silently wiped out labels an admin had just set via
// handleWorkerPolicy, because handleHeartbeat unconditionally copied
// req.Labels over worker.Labels whenever the field was present at all. Labels
// are admin-assigned policy tags, not something a worker should self-report.
func TestHandleHeartbeatDoesNotOverwriteAdminSetLabels(t *testing.T) {
	c := testCoordinator(t)

	policyBody, _ := json.Marshal(map[string]any{
		"worker_id": "worker-1",
		"labels":    []string{"worker-01", "trusted"},
	})
	policyReq := httptest.NewRequest(http.MethodPost, "/api/workers/policy", bytes.NewReader(policyBody))
	policyW := httptest.NewRecorder()
	c.handleWorkerPolicy(policyW, policyReq)
	if policyW.Code != http.StatusOK {
		t.Fatalf("policy setup: status = %d, body=%s", policyW.Code, policyW.Body.String())
	}

	// Simulate a real worker heartbeat that sends its own (empty) idea of
	// "labels", exactly as observed live from an actual worker binary.
	heartbeatBody, _ := json.Marshal(map[string]any{
		"worker_id":    "worker-1",
		"cpu_percent":  12.5,
		"labels":       []string{},
		"capabilities": []string{"git", "node"},
	})
	hbReq := httptest.NewRequest(http.MethodPost, "/api/workers/heartbeat", bytes.NewReader(heartbeatBody))
	hbReq.Header.Set("Authorization", "Bearer worker-token")
	hbW := httptest.NewRecorder()
	c.handleHeartbeat(hbW, hbReq)
	if hbW.Code != http.StatusOK {
		t.Fatalf("heartbeat: status = %d, body=%s", hbW.Code, hbW.Body.String())
	}

	got := c.Store.Workers["worker-1"].Labels
	if len(got) != 2 || got[0] != "worker-01" || got[1] != "trusted" {
		t.Fatalf("Labels = %v after heartbeat, want [worker-01 trusted] unchanged -- heartbeat must never touch admin-assigned labels", got)
	}
}
