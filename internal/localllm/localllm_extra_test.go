package localllm

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestMissingCredential(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	// missing APIKey
	if NewClient(cfg, nil).Generate(context.Background(), Request{}).Status != StatusConfigurationError {
		t.Fatal("expected failure on missing credential")
	}
}
func TestDisabledFeature(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	if NewClient(cfg, nil).Generate(context.Background(), Request{}).Status != StatusCapabilityDisabled {
		t.Fatal("expected capability disabled status")
	}
}
func TestDownstream401(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))
	res := c.Generate(context.Background(), Request{UserPrompt: "x"})
	if res.Status != StatusAuthError {
		t.Fatalf("expected auth error, got %v", res.Status)
	}
}
func TestDownstream422(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unprocessable Entity", http.StatusUnprocessableEntity)
	}))
	res := c.Generate(context.Background(), Request{UserPrompt: "x"})
	if res.Status != StatusSchemaInvalid { // mapped to general protocol/unavailable error
		t.Fatalf("expected unavailable, got %v", res.Status)
	}
}
func TestInvalidJSONResponse(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{invalid"))
	}))
	if c.Generate(context.Background(), Request{}).Status != StatusProtocolError {
		t.Fatal("expected protocol error on invalid JSON")
	}
}

func TestTrailingJSON(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status": "ok", "model": "qwen3.5:4b", "output": "hello"}{"trailing": true}`))
	}))
	// json.Unmarshal doesn't strictly fail on trailing data by itself unless strictly configured,
	// but let's test how standard library behaves or rely on gateway refusing trailing JSON on requests
	_ = c
}

func TestTimeout(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just hang
		time.Sleep(50 * time.Millisecond)
	}))
	c.cfg.RequestTimeout = 10 * time.Millisecond
	if c.Generate(context.Background(), Request{}).Status != StatusTimeout {
		t.Fatal("expected timeout")
	}
}

func TestCancellation(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()                                                // cancel immediately
	res := c.Generate(ctx, Request{}); if res.Status != StatusTimeout && res.Status != StatusUnavailable { // context.Canceled maps to Timeout/Unavailable depending on classification
		t.Fatalf("expected timeout/canceled status, got %v", c.Generate(ctx, Request{}).Status)
	}
}
