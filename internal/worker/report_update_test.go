package worker

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestWorkerForReporting(t *testing.T, coordinatorURL string) *Worker {
	t.Helper()
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	return &Worker{
		WorkerID:       "worker-report-test",
		Token:          "secret",
		CoordinatorURL: coordinatorURL,
		Client:         &http.Client{Timeout: 2 * time.Second},
	}
}

// TestReportUpdateSurvivesConnectionFailure: the coordinator is completely
// unreachable (closed server) - reportUpdate must not panic, must return
// false, and must persist the terminal report so it can be retried later.
func TestReportUpdateSurvivesConnectionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	badURL := srv.URL
	srv.Close() // now genuinely unreachable

	w := newTestWorkerForReporting(t, badURL)

	acked := w.reportUpdate("update-1", "completed", "done", true)
	if acked {
		t.Fatalf("expected reportUpdate to report failure when the coordinator is unreachable")
	}

	pending, err := readPendingUpdateReport()
	if err != nil {
		t.Fatalf("expected an unacknowledged terminal report to be persisted: %v", err)
	}
	if pending.UpdateID != "update-1" || pending.Status != "completed" {
		t.Fatalf("unexpected pending report: %+v", pending)
	}
}

// TestReportUpdateSurvivesTimeout: the coordinator hangs past the client
// timeout on every attempt.
func TestReportUpdateSurvivesTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	w := newTestWorkerForReporting(t, srv.URL)
	w.Client = &http.Client{Timeout: 50 * time.Millisecond}

	acked := w.reportUpdate("update-timeout", "failed", "boom", false)
	if acked {
		t.Fatalf("expected reportUpdate to report failure when every attempt times out")
	}
	if _, err := readPendingUpdateReport(); err != nil {
		t.Fatalf("expected the terminal report to be persisted after a timeout: %v", err)
	}
}

// TestReportUpdateSurvivesNon2xx: the coordinator responds but rejects the
// report (e.g. a transient 500).
func TestReportUpdateSurvivesNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	w := newTestWorkerForReporting(t, srv.URL)

	acked := w.reportUpdate("update-500", "rollback_failed", "still broken", true)
	if acked {
		t.Fatalf("expected reportUpdate to report failure on a persistent 500")
	}
	if _, err := readPendingUpdateReport(); err != nil {
		t.Fatalf("expected the terminal report to be persisted after repeated 500s: %v", err)
	}
}

// TestReportUpdateEventualAcknowledgementViaRetryLoop: the very first
// attempt fails, but the coordinator becomes reachable again before the
// next heartbeat-driven retry - retryPendingUpdateReport (the same
// function the heartbeat loop calls every 5s) must pick it up and clear
// the pending report once acknowledged.
func TestReportUpdateEventualAcknowledgementViaRetryLoop(t *testing.T) {
	var received int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&received, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	realURL := srv.URL

	// Point at a closed server first so the initial bounded retries in
	// reportUpdate all fail and a pending report gets persisted.
	closedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := closedSrv.URL
	closedSrv.Close()

	w := newTestWorkerForReporting(t, closedURL)
	if acked := w.reportUpdate("update-eventual", "completed", "done", true); acked {
		t.Fatalf("expected the first attempt to fail while the coordinator is unreachable")
	}
	if _, err := readPendingUpdateReport(); err != nil {
		t.Fatalf("expected a pending report after the initial failure: %v", err)
	}

	// Coordinator is reachable now - simulate what the heartbeat loop does.
	w.CoordinatorURL = realURL
	w.retryPendingUpdateReport()

	if atomic.LoadInt32(&received) != 1 {
		t.Fatalf("expected exactly one report to reach the coordinator once reachable, got %d", received)
	}
	if _, err := readPendingUpdateReport(); err == nil {
		t.Fatalf("expected the pending report to be cleared once acknowledged")
	}
}

// TestDuplicateTerminalReportIsSafe: reportUpdate succeeds, then
// retryPendingUpdateReport runs again with nothing pending (the ordinary
// steady-state case) - it must be a safe no-op, and a genuine resend of an
// already-acknowledged report must not error either.
func TestDuplicateTerminalReportIsSafe(t *testing.T) {
	var received int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&received, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := newTestWorkerForReporting(t, srv.URL)

	if acked := w.reportUpdate("update-dup", "completed", "done", true); !acked {
		t.Fatalf("expected the report to succeed against a healthy coordinator")
	}
	if _, err := readPendingUpdateReport(); err == nil {
		t.Fatalf("expected no pending report after a successful send")
	}

	// Nothing pending - must be a harmless no-op, not an error/panic.
	w.retryPendingUpdateReport()

	// A genuine resend of the same terminal report (e.g. the worker itself
	// retries independently of the pending-report mechanism) must also be
	// accepted without error - the coordinator's handler just overwrites
	// the same fields.
	if acked := w.reportUpdate("update-dup", "completed", "done", true); !acked {
		t.Fatalf("expected a duplicate terminal report to be accepted safely")
	}
	if atomic.LoadInt32(&received) != 2 {
		t.Fatalf("expected exactly 2 requests to have reached the coordinator, got %d", received)
	}
}
