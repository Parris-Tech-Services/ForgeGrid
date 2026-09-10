package worker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDurablePendingReports(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)

	var reportsReceived int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/updates/report" {
			reportsReceived++
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	w := &Worker{
		WorkerID:       "worker-1",
		Token:          "tok-1",
		CoordinatorURL: ts.URL,
		Client:         &http.Client{Timeout: 5 * time.Second},
	}

	// 1. Write multiple pending reports
	err1 := writePendingUpdateReport(pendingUpdateReport{
		WorkerID: "w-1",
		UpdateID: "up-1",
		Status:   "failed",
	})
	err2 := writePendingUpdateReport(pendingUpdateReport{
		WorkerID: "w-1",
		UpdateID: "up-2",
		Status:   "completed",
	})
	if err1 != nil || err2 != nil {
		t.Fatalf("Failed to write reports: %v, %v", err1, err2)
	}

	// Verify they coexist
	entries, _ := os.ReadDir(pendingReportDir())
	if len(entries) != 2 {
		t.Fatalf("Expected 2 pending reports, found %d", len(entries))
	}

	// 2. Retry should drain both
	w.retryPendingUpdateReport()

	if reportsReceived != 2 {
		t.Fatalf("Expected 2 reports received by coordinator, got %d", reportsReceived)
	}

	// Verify they were cleaned up
	entries, _ = os.ReadDir(pendingReportDir())
	if len(entries) != 0 {
		t.Fatalf("Expected 0 pending reports after drain, found %d", len(entries))
	}

	// 3. Duplicate delivery (clearPendingUpdateReportIfMatches)
	err3 := writePendingUpdateReport(pendingUpdateReport{
		WorkerID: "w-1",
		UpdateID: "up-3",
		Status:   "completed",
	})
	if err3 != nil {
		t.Fatalf("Failed to write report: %v", err3)
	}

	w.clearPendingUpdateReportIfMatches("up-3")

	entries, _ = os.ReadDir(pendingReportDir())
	if len(entries) != 0 {
		t.Fatalf("Expected 0 pending reports after clear, found %d", len(entries))
	}
}

func TestReportTxStateDurable(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)

	// Create creds for reportTxState
	credsPath := getWorkerCredsPath()
	os.MkdirAll(filepath.Dir(credsPath), 0755)
	credsData := `{"worker_id":"w-1","token":"tok-1","coordinator_url":"http://127.0.0.1:0"}` // Invalid port, will fail
	os.WriteFile(credsPath, []byte(credsData), 0600)

	tx := &UpdateTransaction{
		WorkerID:     "w-1",
		ID:           "up-123",
		CurrentState: "ROLLED_BACK",
	}

	// This should fail to send and queue durably
	reportTxState(tx)

	entries, _ := os.ReadDir(pendingReportDir())
	if len(entries) != 1 {
		t.Fatalf("Expected 1 pending report after failed reportTxState, found %d", len(entries))
	}

	// Validate content
	b, _ := os.ReadFile(filepath.Join(pendingReportDir(), entries[0].Name()))
	var rep pendingUpdateReport
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}
	if rep.UpdateID != "up-123" || rep.Status != "rolled_back" {
		t.Fatalf("Unexpected report content: %+v", rep)
	}
}
