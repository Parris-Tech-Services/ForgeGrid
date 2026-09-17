package worker

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	fgupdate "forgegrid/internal/update"
)

func TestArtifactDownloadLimit(t *testing.T) {
	tests := []struct {
		name        string
		size        int
		limit       int64
		infinite    bool
		wantErr     bool
		errContains string
	}{
		{
			name:    "exactly under limit",
			size:    9,
			limit:   10,
			wantErr: false,
		},
		{
			name:    "exactly at limit",
			size:    10,
			limit:   10,
			wantErr: false,
		},
		{
			name:        "one byte over",
			size:        11,
			limit:       10,
			wantErr:     true,
			errContains: "size exceeds",
		},
		{
			name:        "server streams indefinitely",
			infinite:    true,
			limit:       10,
			wantErr:     true,
			errContains: "size exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalLimit := artifactDownloadLimit
			artifactDownloadLimit = tt.limit
			defer func() { artifactDownloadLimit = originalLimit }()

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.infinite {
					// Stream indefinitely until client disconnects
					for {
						if _, err := w.Write([]byte("x")); err != nil {
							break
						}
					}
					return
				}
				body := make([]byte, tt.size)
				w.Write(body)
			}))
			defer ts.Close()

			w := &Worker{
				WorkerID:       "worker-1",
				Token:          "tok-1",
				CoordinatorURL: ts.URL,
				Client:         &http.Client{},
				DownloadClient: &http.Client{},
			}

			req := fgupdate.Request{
				ID: "update-1",
				Artifact: fgupdate.Artifact{
					Path: "candidate.exe",
				},
			}

			updateDir := t.TempDir()
			dest, err := w.downloadUpdateArtifact(req, updateDir)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (dest=%q)", dest)
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				// Verify partial file cleanup
				files, _ := os.ReadDir(updateDir)
				if len(files) > 0 {
					t.Fatalf("expected updateDir to be empty after failure, but found files: %v", files)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if dest == "" {
					t.Fatalf("expected valid dest path, got empty string")
				}
				info, err := os.Stat(dest)
				if err != nil {
					t.Fatalf("failed to stat dest: %v", err)
				}
				if info.Size() != int64(tt.size) {
					t.Fatalf("expected size %d, got %d", tt.size, info.Size())
				}
			}
		})
	}
}
