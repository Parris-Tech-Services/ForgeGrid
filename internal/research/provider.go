package research

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"content"`
}

type Source struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Text    string `json:"text,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

// Provider is the only coordinator-facing research capability. The model
// receives results, never provider credentials or arbitrary URLs.
type Provider interface {
	Research(context.Context, string) ([]Source, error)
}

type SearXNG struct {
	BaseURL string
	Client  *http.Client
}

func (s *SearXNG) Research(ctx context.Context, query string) ([]Source, error) {
	base, err := url.Parse(s.BaseURL)
	if err != nil || base.Scheme != "http" || base.Hostname() != "127.0.0.1" && base.Hostname() != "localhost" || base.User != nil {
		return nil, errors.New("SearXNG must use a configured localhost HTTP endpoint")
	}
	endpoint := *base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/search"
	q := endpoint.Query()
	q.Set("q", query)
	q.Set("format", "json")
	endpoint.RawQuery = q.Encode()
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("SearXNG returned an error")
	}
	var payload struct {
		Results []SearchResult `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Results) > 3 {
		payload.Results = payload.Results[:3]
	}
	out := make([]Source, 0, len(payload.Results))
	for _, result := range payload.Results {
		source := Source{Title: result.Title, URL: result.URL, Snippet: result.Snippet}
		if fetched, err := Fetch(ctx, result.URL); err == nil {
			source.Text = fetched.Text
		}
		if source.Text != "" || source.Snippet != "" {
			out = append(out, source)
		}
	}
	return out, nil
}
