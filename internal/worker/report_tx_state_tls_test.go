package worker

import (
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"forgegrid/internal/network"
)

func startSelfSignedServer(t *testing.T, handler http.Handler) (url, fingerprint string) {
	t.Helper()
	certPEM, keyPEM, fp, err := network.GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("generate self-signed cert: %v", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("load cert: %v", err)
	}

	srv := httptest.NewUnstartedServer(handler)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.Listener = l
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv.URL, fp
}

// TestReportTxStateUsesPinnedTLSNotSystemCAs reproduces the coordinator's
// real TLS setup exactly (a self-signed certificate, the same as
// network.GenerateSelfSignedCert produces for a real ForgeGrid
// coordinator) and confirms reportTxState (invoked by every writeTx call,
// i.e. every STAGED/APPLYING/RESTARTING/VERIFYING_NEW_WORKER/ROLLING_BACK/
// ROLLED_BACK/ROLLBACK_FAILED transition) actually reaches it. Before the
// fix, reportTxState used a bare *http.Client with no custom Transport
// whenever Insecure was false - Go's default Transport verifies against
// the system CA pool, which a self-signed certificate never satisfies, so
// every one of these reports silently failed the TLS handshake in the
// normal (non-insecure, fingerprint-pinned) configuration every real
// DadLAN worker actually runs in.
func TestReportTxStateUsesPinnedTLSNotSystemCAs(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)

	var received int32
	url, fingerprint := startSelfSignedServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/updates/report" {
			return
		}
		atomic.AddInt32(&received, 1)
		w.WriteHeader(http.StatusOK)
	}))

	creds := WorkerCredentials{
		WorkerID:       "worker-tls-test",
		Token:          "secret",
		CoordinatorURL: url,
		Fingerprint:    fingerprint,
		Insecure:       false,
	}
	b, _ := json.Marshal(creds)
	if err := os.WriteFile(getWorkerCredsPath(), b, 0600); err != nil {
		t.Fatalf("write creds: %v", err)
	}

	tx := &UpdateTransaction{
		ID:            "tx-tls",
		WorkerID:      "worker-tls-test",
		CurrentState:  "APPLYING",
		LifecycleMode: "portable",
	}
	if err := writeTx(tx); err != nil {
		t.Fatalf("writeTx failed: %v", err)
	}

	// reportTxState fires a goroutine-free, synchronous, best-effort POST
	// inside writeTx via client.Do(req) - give it a brief moment since the
	// real handshake/round trip is real network I/O even over loopback.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&received) == 0 {
		time.Sleep(20 * time.Millisecond)
	}

	if atomic.LoadInt32(&received) != 1 {
		t.Fatalf("expected exactly one report to reach the self-signed coordinator over pinned TLS, got %d", received)
	}
}

// TestReportTxStateRejectsWrongFingerprint confirms the pinning is
// actually enforced, not merely present: a self-signed server whose real
// fingerprint does NOT match the one saved in worker credentials must be
// rejected by the TLS handshake, the same protection PinTLSConfig gives
// every other client in this package.
func TestReportTxStateRejectsWrongFingerprint(t *testing.T) {
	tmp := t.TempDir()
	setSandboxedDataDir(t, tmp)
	os.MkdirAll(filepath.Join(getWorkerDataDir(), "updates"), 0755)

	var received int32
	url, _ := startSelfSignedServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&received, 1)
		w.WriteHeader(http.StatusOK)
	}))

	creds := WorkerCredentials{
		WorkerID:       "worker-tls-test",
		Token:          "secret",
		CoordinatorURL: url,
		Fingerprint:    "0000000000000000000000000000000000000000000000000000000000000",
		Insecure:       false,
	}
	b, _ := json.Marshal(creds)
	os.WriteFile(getWorkerCredsPath(), b, 0600)

	tx := &UpdateTransaction{
		ID:            "tx-tls-wrong-fp",
		WorkerID:      "worker-tls-test",
		CurrentState:  "APPLYING",
		LifecycleMode: "portable",
	}
	writeTx(tx)

	time.Sleep(300 * time.Millisecond)
	if atomic.LoadInt32(&received) != 0 {
		t.Fatalf("expected the mismatched-fingerprint report to be rejected by the TLS handshake, but the server received %d requests", received)
	}
}
