package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	fgupdate "forgegrid/internal/update"
)

func setWorkerTestDataHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("APPDATA", dir)
}

func TestReportUpdateRetriesAfterTransientFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	w := &Worker{
		CoordinatorURL: server.URL,
		Token:          "worker-token",
		WorkerID:       "worker-1",
		Client:         server.Client(),
	}
	w.reportUpdate("update-1", "completed", "done", true)

	if requests.Load() < 2 {
		t.Fatalf("terminal update report made %d request(s), want a retry after transient failure", requests.Load())
	}
}

func TestDownloadUpdateArtifactEnforcesDeclaredSize(t *testing.T) {
	const expectedBody = "0123456789"
	sum := sha256.Sum256([]byte(expectedBody))
	expectedSHA := hex.EncodeToString(sum[:])

	tests := []struct {
		name      string
		size      int64
		body      string
		wantError bool
	}{
		{name: "short", size: int64(len(expectedBody)), body: expectedBody[:len(expectedBody)-1], wantError: true},
		{name: "exact", size: int64(len(expectedBody)), body: expectedBody, wantError: false},
		{name: "large", size: int64(len(expectedBody)), body: expectedBody + "x", wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			updateDir := t.TempDir()
			w := &Worker{
				WorkerID:       "worker-1",
				Token:          "worker-token",
				CoordinatorURL: server.URL,
				Client:         server.Client(),
				DownloadClient: server.Client(),
			}
			req := fgupdate.Request{
				ID: "update-1",
				Artifact: fgupdate.Artifact{
					Path:   "Windows/ForgeGrid.exe",
					Size:   tc.size,
					SHA256: expectedSHA,
				},
			}

			source, err := w.downloadUpdateArtifact(req, updateDir)
			if tc.wantError {
				if err == nil {
					t.Fatalf("download succeeded for body length %d with declared size %d", len(tc.body), tc.size)
				}
				if source != "" {
					t.Fatalf("failed download returned source %q", source)
				}
				matches, globErr := filepath.Glob(filepath.Join(updateDir, "downloaded-*"))
				if globErr != nil || len(matches) != 0 {
					t.Fatalf("failed download left partial artifacts: matches=%v err=%v", matches, globErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("exact-size download failed: %v", err)
			}
			if source == "" {
				t.Fatal("exact-size download returned an empty source path")
			}
		})
	}
}

func TestWriteTxReportsPersistenceFailures(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		dataHome := filepath.Join(t.TempDir(), "missing")
		setWorkerTestDataHome(t, dataHome)
		err := writeTx(&UpdateTransaction{ID: "update-1", CurrentState: "STAGED"})
		if err == nil {
			t.Fatal("writeTx succeeded with a missing worker data directory")
		}
	})

	t.Run("destination is directory", func(t *testing.T) {
		dataHome := t.TempDir()
		setWorkerTestDataHome(t, dataHome)
		if err := os.MkdirAll(getWorkerDataDir(), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(getTxPath(), 0700); err != nil {
			t.Fatal(err)
		}
		err := writeTx(&UpdateTransaction{ID: "update-1", CurrentState: "APPLYING"})
		if err == nil {
			t.Fatal("writeTx succeeded when the transaction destination was a directory")
		}
	})
}

func TestVerifyUpdateTransactionRecoversBadCandidateAfterRestart(t *testing.T) {
	if os.Getenv("FORGEGRID_TEST_VERIFY_CHILD") == "1" {
		(&Worker{}).verifyUpdateTransaction()
		return
	}

	dataHome := t.TempDir()
	setWorkerTestDataHome(t, dataHome)
	if err := os.MkdirAll(getWorkerDataDir(), 0700); err != nil {
		t.Fatal(err)
	}
	tx := &UpdateTransaction{
		ID:              "update-1",
		CurrentState:    "VERIFYING_NEW_WORKER",
		ExpectedSHA256:  "not-the-running-binary",
		RestartDeadline: time.Now().Add(time.Minute),
	}
	if err := writeTx(tx); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestVerifyUpdateTransactionRecoversBadCandidateAfterRestart")
	cmd.Env = append(os.Environ(), "FORGEGRID_TEST_VERIFY_CHILD=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("child unexpectedly exited successfully without recovering the bad candidate")
	}

	current, err := readTx()
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentState != "ROLLED_BACK" && current.CurrentState != "ROLLBACK_FAILED" {
		t.Fatalf("startup recovery left transaction in %q, want a durable rollback state", current.CurrentState)
	}
}

func TestApplyingTransactionHasRestartRecoveryPath(t *testing.T) {
	if os.Getenv("FORGEGRID_TEST_APPLYING_CHILD") == "1" {
		(&Worker{}).verifyUpdateTransaction()
		return
	}

	dataHome := t.TempDir()
	setWorkerTestDataHome(t, dataHome)
	if err := os.MkdirAll(getWorkerDataDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeTx(&UpdateTransaction{ID: "update-1", CurrentState: "APPLYING"}); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestApplyingTransactionHasRestartRecoveryPath")
	cmd.Env = append(os.Environ(), "FORGEGRID_TEST_APPLYING_CHILD=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("startup recovery child failed: %v", err)
	}
	current, err := readTx()
	if err != nil {
		t.Fatal(err)
	}
	if current.CurrentState == "APPLYING" {
		t.Fatalf("APPLYING transaction has no startup recovery transition")
	}
}

func TestDuplicateUpdateRemainsRejectedAfterWorkerRestart(t *testing.T) {
	dataHome := t.TempDir()
	setWorkerTestDataHome(t, dataHome)
	if err := os.MkdirAll(getWorkerDataDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeTx(&UpdateTransaction{ID: "update-1", CurrentState: "STAGED"}); err != nil {
		t.Fatal(err)
	}

	workerAfterRestart := &Worker{}
	if workerAfterRestart.tryBeginUpdate("update-1") {
		t.Fatal("worker accepted an update ID already represented by a persisted transaction")
	}
}
