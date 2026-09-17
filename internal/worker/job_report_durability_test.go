package worker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"forgegrid/internal/models"
)

func countingHandler(status int) (http.HandlerFunc, *int32) {
	var calls int32
	return func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(status)
	}, &calls
}

func pendingJobReportFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(pendingJobReportDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	return out
}

// TestPostJobUpdateTerminalReportSurvivesTransportFailure is the direct
// Finding C regression test: a completely unreachable coordinator must not
// cause a terminal job report to be silently dropped - it must be
// persisted for later retry, exactly like reportUpdate already does for
// update-transaction reports.
func TestPostJobUpdateTerminalReportSurvivesTransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	badURL := srv.URL
	srv.Close() // genuinely unreachable

	w := newTestWorkerForReporting(t, badURL)
	w.jobUpdateBackoffForTest(t)

	w.postJobUpdate("job-1", map[string]interface{}{
		"attempt_id": "attempt-1",
		"status":     models.StatusCompleted,
		"result":     "success",
	})

	files := pendingJobReportFiles(t)
	if len(files) != 1 {
		t.Fatalf("expected exactly one persisted pending job report, got %d: %v", len(files), files)
	}
	b, err := os.ReadFile(filepath.Join(pendingJobReportDir(), files[0]))
	if err != nil {
		t.Fatalf("failed to read persisted report: %v", err)
	}
	var rep pendingJobReport
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatalf("persisted report was not valid JSON: %v", err)
	}
	if rep.JobID != "job-1" {
		t.Fatalf("expected persisted report for job-1, got %q", rep.JobID)
	}
}

// TestPostJobUpdateNonTerminalReportIsBestEffortOnly proves the
// intentional asymmetry: a lost mid-job progress update is not worth
// persisting (the next one supersedes it), unlike a terminal report.
func TestPostJobUpdateNonTerminalReportIsBestEffortOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	badURL := srv.URL
	srv.Close()

	w := newTestWorkerForReporting(t, badURL)
	w.jobUpdateBackoffForTest(t)

	w.postJobUpdate("job-progress", map[string]interface{}{
		"attempt_id": "attempt-1",
		"status":     models.StatusRunning,
		"result":     "",
	})

	if files := pendingJobReportFiles(t); len(files) != 0 {
		t.Fatalf("a non-terminal report must never be persisted, found %v", files)
	}
}

// TestPostJobUpdateAcceptsCoordinatorTerminalConflictAsResolved covers the
// coordinator's real handleJobAction behavior: it returns 409 Conflict for
// a job already in a terminal state. A worker retrying its own terminal
// report into that must treat 409 as "already resolved, nothing left to
// deliver" - not as a failure worth persisting and retrying forever.
func TestPostJobUpdateAcceptsCoordinatorTerminalConflictAsResolved(t *testing.T) {
	handler, calls := countingHandler(http.StatusConflict)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	w := newTestWorkerForReporting(t, srv.URL)
	w.jobUpdateBackoffForTest(t)

	w.postJobUpdate("job-already-terminal", map[string]interface{}{
		"attempt_id": "attempt-1",
		"status":     models.StatusCompleted,
	})

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("expected exactly one delivery attempt (409 stops retrying immediately), got %d", got)
	}
	if files := pendingJobReportFiles(t); len(files) != 0 {
		t.Fatalf("a 409 (already resolved) must not be persisted for further retry, found %v", files)
	}
}

// TestPostJobUpdateAcceptsNotFoundAsResolved mirrors the update-report
// path's existing 404-as-superseded convention for job reports.
func TestPostJobUpdateAcceptsNotFoundAsResolved(t *testing.T) {
	handler, _ := countingHandler(http.StatusNotFound)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	w := newTestWorkerForReporting(t, srv.URL)
	w.jobUpdateBackoffForTest(t)

	w.postJobUpdate("job-gone", map[string]interface{}{
		"attempt_id": "attempt-1",
		"status":     models.StatusFailed,
	})

	if files := pendingJobReportFiles(t); len(files) != 0 {
		t.Fatalf("a 404 (job/coordinator state reset) must not be persisted, found %v", files)
	}
}

// TestRetryPendingJobReportsDeliversAfterRestart is the reboot scenario:
// a terminal report was persisted before a crash (simulating the worker
// process dying right after postJobUpdate wrote it to disk, before this
// process's own retries ever succeeded), and a fresh process's ordinary
// heartbeat-loop drain must pick it up and deliver it once the coordinator
// becomes reachable again - without the caller ever calling postJobUpdate
// again itself.
func TestRetryPendingJobReportsDeliversAfterRestart(t *testing.T) {
	handler, calls := countingHandler(http.StatusOK)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	w := newTestWorkerForReporting(t, srv.URL)

	rep := pendingJobReport{
		JobID: "job-after-crash",
		Body: map[string]interface{}{
			"attempt_id": "attempt-1",
			"status":     string(models.StatusCompleted),
			"result":     "success",
		},
	}
	if err := writePendingJobReport(rep); err != nil {
		t.Fatalf("setup: failed to persist a pre-crash pending report: %v", err)
	}
	if files := pendingJobReportFiles(t); len(files) != 1 {
		t.Fatalf("setup: expected exactly one pre-existing pending report, got %v", files)
	}

	w.retryPendingJobReports()

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("expected the persisted report to be delivered exactly once, got %d calls", got)
	}
	if files := pendingJobReportFiles(t); len(files) != 0 {
		t.Fatalf("a successfully delivered pending report must be cleared, found %v", files)
	}
}

// TestPostJobUpdateSuccessClearsAnyStalePendingReportForSameJob ensures a
// later successful delivery (e.g. the eventual "completed" report, after
// an earlier "running" report for the same job had to be persisted)
// cleans up the stale file rather than leaving it to be redelivered later
// and regress state.
func TestPostJobUpdateSuccessClearsAnyStalePendingReportForSameJob(t *testing.T) {
	handler, _ := countingHandler(http.StatusOK)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	w := newTestWorkerForReporting(t, srv.URL)

	stale := pendingJobReport{JobID: "job-2", Body: map[string]interface{}{"status": string(models.StatusRunning)}}
	if err := writePendingJobReport(stale); err != nil {
		t.Fatalf("setup: %v", err)
	}

	w.postJobUpdate("job-2", map[string]interface{}{
		"attempt_id": "attempt-1",
		"status":     models.StatusCompleted,
	})

	if files := pendingJobReportFiles(t); len(files) != 0 {
		t.Fatalf("a successful terminal delivery must clear any stale pending report for the same job, found %v", files)
	}
}

// jobUpdateBackoffForTest shrinks the retry backoff to keep these tests
// fast without changing the retry *count* or *terminal-status* semantics
// under test.
func (w *Worker) jobUpdateBackoffForTest(t *testing.T) {
	t.Helper()
	original := jobUpdateBackoff
	jobUpdateBackoff = []time.Duration{0, 5 * time.Millisecond, 5 * time.Millisecond}
	t.Cleanup(func() { jobUpdateBackoff = original })
}
