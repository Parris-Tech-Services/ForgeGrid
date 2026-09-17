package research

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSearXNGUsesFixedLocalEndpointAndCapsResults(t *testing.T) {
	var got *http.Request
	provider := &SearXNG{BaseURL: "http://127.0.0.1:8081", Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r
		body := `{"results":[{"title":"one","url":"file:///tmp/a","content":"a"},{"title":"two","url":"file:///tmp/b","content":"b"},{"title":"three","url":"file:///tmp/c","content":"c"},{"title":"four","url":"file:///tmp/d","content":"d"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	sources, err := provider.Research(context.Background(), "safe query")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.URL.Query().Get("q") != "safe query" || got.URL.Query().Get("format") != "json" {
		t.Fatalf("unexpected provider request: %v", got)
	}
	if len(sources) != 3 {
		t.Fatalf("got %d sources, want 3", len(sources))
	}
}

func TestSearXNGRejectsNonLocalEndpoint(t *testing.T) {
	if _, err := (&SearXNG{BaseURL: "http://10.245.173.1:8081"}).Research(context.Background(), "x"); err == nil {
		t.Fatal("non-local SearXNG endpoint accepted")
	}
}
