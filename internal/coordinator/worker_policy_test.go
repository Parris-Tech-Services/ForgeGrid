package coordinator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"forgegrid/internal/models"
)

func TestHandleWorkerPolicySetsLabels(t *testing.T) {
	c := testCoordinator(t)
	c.Store.Workers["worker-1"].Labels = []string{"stale-label"}

	body, _ := json.Marshal(map[string]any{
		"worker_id": "worker-1",
		"labels":    []string{"worker-01", "trusted"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/workers/policy", bytes.NewReader(body))
	w := httptest.NewRecorder()

	c.handleWorkerPolicy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	got := c.Store.Workers["worker-1"].Labels
	want := []string{"worker-01", "trusted"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Labels = %v, want %v", got, want)
	}

	var dto models.WorkerDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(dto.Labels) != 2 || dto.Labels[0] != "worker-01" {
		t.Fatalf("response DTO Labels = %v, want %v", dto.Labels, want)
	}
}

func TestHandleWorkerPolicyOmittedLabelsLeavesExistingLabelsUnchanged(t *testing.T) {
	c := testCoordinator(t)
	c.Store.Workers["worker-1"].Labels = []string{"worker-01"}

	body, _ := json.Marshal(map[string]any{
		"worker_id": "worker-1",
		"drain":     true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/workers/policy", bytes.NewReader(body))
	w := httptest.NewRecorder()

	c.handleWorkerPolicy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := c.Store.Workers["worker-1"].Labels; len(got) != 1 || got[0] != "worker-01" {
		t.Fatalf("Labels changed to %v when the request did not include a labels field at all", got)
	}
	if !c.Store.Workers["worker-1"].Drain {
		t.Fatalf("Drain was not applied alongside the omitted Labels field")
	}
}

func TestHandleWorkerPolicyEmptyLabelsArrayClearsLabels(t *testing.T) {
	c := testCoordinator(t)
	c.Store.Workers["worker-1"].Labels = []string{"worker-01"}

	body, _ := json.Marshal(map[string]any{
		"worker_id": "worker-1",
		"labels":    []string{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/workers/policy", bytes.NewReader(body))
	w := httptest.NewRecorder()

	c.handleWorkerPolicy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := c.Store.Workers["worker-1"].Labels; len(got) != 0 {
		t.Fatalf("Labels = %v, want empty -- an explicit empty array must clear labels, distinct from omitting the field entirely", got)
	}
}
