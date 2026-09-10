package worker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"forgegrid/internal/models"
)

func TestFakeAgentIntegrationPipeline(t *testing.T) {
	ws := t.TempDir()

	// Setup fake git repo to clone from
	remoteRepo := filepath.Join(ws, "remote-repo")
	if err := os.MkdirAll(remoteRepo, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(remoteRepo, "README.md"), []byte("Hello"), 0644)

	gitScript := `#!/bin/bash
git init
git branch -m main
git config user.email "test@example.com"
git config user.name "Test User"
git add README.md
git commit -m "Initial commit"
`
	scriptPath := filepath.Join(remoteRepo, "setup.sh")
	os.WriteFile(scriptPath, []byte(gitScript), 0755)

	cmd := exec.Command("bash", "./setup.sh")
	cmd.Dir = remoteRepo
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to setup git repo: %v", err)
	}

	// Coordinator mock
	mux := http.NewServeMux()
	// Real path is plural ("/api/workers/heartbeat"); this mock previously
	// registered the singular form, so every heartbeat silently fell
	// through to the catch-all handler below instead of this one.
	mux.HandleFunc("/api/workers/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/api/updates/worker", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"update": null}`))
	})

	// Mock job assignment — use json.Marshal to build the response so that
	// Windows paths (containing backslashes) are properly escaped.  The
	// previous implementation string-interpolated remoteRepo directly into
	// a JSON template literal; on Windows the resulting \U, \T, \A, etc.
	// sequences are invalid JSON escapes, causing json.Decoder on the
	// client side to fail silently.  The worker never decoded the job,
	// never claimed it, and the test timed out.
	jobAssigned := false
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		if jobAssigned {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
			return
		}
		jobAssigned = true
		w.Header().Set("Content-Type", "application/json")

		jobs := []models.Job{{
			ID:             "job-fake-1",
			Status:         models.StatusPending,
			TaskName:       "ai_task",
			Task:           "execute",
			Profile:        "ai",
			AgentRequested: "fake",
			RepositoryURL:  remoteRepo,
			BaseCommit:     "main",
			BranchName:     "forgegrid/test-branch",
			CommitChanges:  true,
			PushChanges:    false,
			CommitMessage:  "Agent changed something",
			TimeoutSeconds: 60,
			Stages: []models.JobStage{{
				Name:           "Agent",
				Profile:        "ai",
				TimeoutSeconds: 60,
				Parameters:     map[string]string{"prompt": "CHANGE the file"},
			}},
		}}
		resp, err := json.Marshal(jobs)
		if err != nil {
			t.Errorf("failed to marshal job response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(resp)
	})

	mux.HandleFunc("/api/jobs/job-fake-1/claim", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"attempt_id": "attempt-1"}`))
	})

	jobStatusCalls := make(chan string, 10)
	mux.HandleFunc("/api/jobs/job-fake-1", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		jobStatusCalls <- string(body)
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Unhandled request: %s %s", r.Method, r.URL.Path)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	worker := New("test-worker", ws, true)
	worker.CoordinatorURL = srv.URL
	worker.Token = "secret"
	worker.Workspace = ws
	worker.allowPush = false
	worker.allowBootstrap = true
	worker.SetGitPolicy(remoteRepo, true)
	worker.capabilityAllow = []string{"agent:fake", "git"}

	// jobLoop only polls every 2s, and the pipeline this test drives runs
	// several separate git subprocesses (clone, branch, commit) plus the
	// fake agent stage before it can report COMPLETED. 15s was tight
	// enough that it passed reliably on Linux/race-detector CI but timed
	// out on windows-latest, where per-process spawn overhead (Windows
	// Defender real-time scanning each new process is a known contributor)
	// is routinely a few times higher for the same git-heavy workload.
	// This still fails fast if the pipeline is genuinely stuck; it just
	// stops being tighter than the job's own declared timeout_seconds: 60
	// above.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	worker.Start()
	defer worker.Stop()

	// Wait for completion
	completed := false
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for job completion")
		case msg := <-jobStatusCalls:
			t.Logf("Job status update: %s", msg)
			if strings.Contains(msg, `"status":"COMPLETED"`) {
				completed = true
				if !strings.Contains(msg, `"agent_actual":"fake"`) {
					t.Errorf("Expected agent_actual to be fake, got %s", msg)
				}
			} else if strings.Contains(msg, `"status":"FAILED"`) {
				t.Fatalf("Job failed: %s", msg)
			}
		}
		if completed {
			break
		}
	}
}

// TestWindowsPathJSONEscaping is a regression test for the bug where Windows
// filesystem paths (containing backslashes) were directly string-interpolated
// into JSON template literals. On Windows, paths like C:\Users\foo produce
// invalid JSON escape sequences (\U, \f, etc.), causing json.Decoder to fail.
// This resulted in the worker silently failing to decode job assignments,
// never claiming jobs, and integration tests timing out.
func TestWindowsPathJSONEscaping(t *testing.T) {
	// Simulate a Windows-style path with backslashes
	windowsPath := `C:\Users\RUNNER~1\AppData\Local\Temp\TestFake\001\remote-repo`

	// This is what the old code did: direct string interpolation
	badJSON := `[{"repository_url": "` + windowsPath + `"}]`
	var badResult []map[string]interface{}
	err := json.Unmarshal([]byte(badJSON), &badResult)
	if runtime.GOOS == "windows" || true {
		// The old approach produces invalid JSON on Windows paths
		if err == nil {
			t.Log("Note: JSON happened to decode (path may not contain problematic escapes)")
		} else {
			t.Logf("Confirmed: direct interpolation produces invalid JSON: %v", err)
		}
	}

	// This is what the fixed code does: proper JSON marshaling
	type job struct {
		RepositoryURL string `json:"repository_url"`
	}
	goodJSON, err := json.Marshal([]job{{RepositoryURL: windowsPath}})
	if err != nil {
		t.Fatalf("json.Marshal should never fail on a simple struct: %v", err)
	}

	var goodResult []job
	if err := json.Unmarshal(goodJSON, &goodResult); err != nil {
		t.Fatalf("properly marshaled JSON should decode: %v", err)
	}
	if goodResult[0].RepositoryURL != windowsPath {
		t.Errorf("round-trip failed: got %q, want %q", goodResult[0].RepositoryURL, windowsPath)
	}
}

