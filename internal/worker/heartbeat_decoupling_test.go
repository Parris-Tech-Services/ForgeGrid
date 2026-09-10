package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHeartbeatDecoupling(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)

	var heartbeatCount int32
	var reportAttemptCount int32

	// Coordinator that hangs on report endpoint
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/workers/heartbeat" {
			atomic.AddInt32(&heartbeatCount, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/updates/report" {
			atomic.AddInt32(&reportAttemptCount, 1)
			// Hang the report handler longer to give slow runners time to heartbeat
			time.Sleep(3 * time.Second)
			w.WriteHeader(http.StatusOK)
			return
		}
	}))
	defer ts.Close()

	w := &Worker{
		WorkerID:       "worker-1",
		Token:          "tok-1",
		CoordinatorURL: ts.URL,
		Client:         &http.Client{Timeout: 5 * time.Second}, // Don't let client timeout prematurely
		activeJobs:     make(map[string]context.CancelFunc),
		stopCh:         make(chan struct{}),
	}

	// Create a pending report that will trigger the drain
	rep := pendingUpdateReport{
		WorkerID: "worker-1",
		UpdateID: "update-hung",
		Status:   "failed",
	}
	writePendingUpdateReport(rep)

	// Shorten heartbeat interval for test
	originalInterval := heartbeatInterval
	heartbeatInterval = 50 * time.Millisecond
	defer func() { heartbeatInterval = originalInterval }()

	// Start worker loops
	w.loopsDone.Add(1)
	go w.heartbeatLoop()

	// Wait enough time for multiple heartbeats to occur, but
	// less than the 3s report hang.
	time.Sleep(1500 * time.Millisecond)

	hbCount := atomic.LoadInt32(&heartbeatCount)
	if hbCount < 4 {
		t.Errorf("expected multiple heartbeats during report hang, got %d", hbCount)
	}

	w.Stop()

	// Ensure the report drain was single-flighted:
	// Since the first request takes 3s, we should only see 1 report attempt in 1.5s.
	repCount := atomic.LoadInt32(&reportAttemptCount)
	if repCount > 1 {
		t.Errorf("expected at most 1 report attempt due to single-flighting, got %d", repCount)
	}
}
