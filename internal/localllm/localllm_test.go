package localllm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.APIKey = "secret"
	tr := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		r.URL.Host = ts.Listener.Addr().String()
		return ts.Client().Transport.RoundTrip(r)
	})
	return NewClient(cfg, &http.Client{Transport: tr})
}

func TestGenerateGatewayContract(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/generate" || r.Header.Get("X-API-Key") != "secret" {
			t.Fatalf("bad gateway request: %s key=%q", r.URL.Path, r.Header.Get("X-API-Key"))
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "model": "qwen3.5:4b", "output": "hello"})
	}))
	res := c.Generate(context.Background(), Request{UserPrompt: "x"})
	if res.Status != StatusGenerated || res.Content != "hello" {
		t.Fatalf("%+v", res)
	}
}

func TestConfigAndSchemaFailClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.APIKey = "x"
	cfg.GatewayURL = "http://evil:1"
	if NewClient(cfg, nil).Generate(context.Background(), Request{}).Status != StatusConfigurationError {
		t.Fatal("bad URL accepted")
	}
	if schemaValid(map[string]interface{}{"x": 1}, map[string]interface{}{"type": "object", "x-unsupported": true}) {
		t.Fatal("unsupported schema accepted")
	}
}

func TestAdvisoryOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.APIKey = "x"
	cfg.AdvisoryOnly = false
	if NewClient(cfg, nil).Generate(context.Background(), Request{}).Status != StatusConfigurationError {
		t.Fatal("advisory policy not enforced")
	}
}

func TestRecommendationDeduplication(t *testing.T) {
	v := map[string]interface{}{"recommendations": []interface{}{
		map[string]interface{}{"capability": "inventory", "target": "all", "arguments": map[string]interface{}{"mode": "read"}, "reason": "a"},
		map[string]interface{}{"capability": "inventory", "target": "all", "arguments": map[string]interface{}{"mode": "read"}, "reason": "b"},
	}}
	got, n := dedupeRecommendations(v)
	if n != 1 || len(got.(map[string]interface{})["recommendations"].([]interface{})) != 1 {
		t.Fatalf("dedupe=%d %#v", n, got)
	}
}
