// Package localllm calls the DadLAN production local-LLM gateway
// (D:\DadLAN\local-llm-service on JParrisDesktop, verified source read
// 2026-09-16) to provide a read-only reasoning capability
// ("local_llm.generate") for ForgeGrid jobs.
//
// Topology (verified against the gateway's actual source, not assumed):
//
//	AVANCE-WS7 ForgeGrid coordinator
//	    -- authenticated HTTP, X-API-Key --> JParrisDesktop:11435 (this gateway)
//	                                              -- loopback only --> 127.0.0.1:11434 Ollama
//
// Workers do NOT need a local Ollama instance. JParrisDesktop is the one
// dedicated inference node; the gateway is the only thing this package ever
// talks to, and its address must come from trusted configuration (Config.
// GatewayURL / an environment variable the operator sets), never from a
// prompt, a job manifest parameter, or model output.
//
// This package deliberately never imports os/exec, never touches the
// filesystem, and never invokes anything on this machine or any other. It
// performs a single outbound HTTP request per Generate call and returns the
// response as inert data. The gateway already performs its own schema
// validation, one constrained repair, and one constrained retry
// server-side and fails closed (HTTP 422, output:null) when it cannot
// produce a valid result; this package additionally re-validates whatever
// the gateway claims is valid before ever reporting a passing status, since
// a remote service's self-attestation is not something a safety-relevant
// caller should trust blindly. Any decision to act on the returned data
// belongs entirely to the caller; nothing in this file is capable of doing
// so itself.
package localllm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Config controls how the client talks to the gateway and what it is
// allowed to do. Enabled is the feature flag: when false, Generate returns
// a CapabilityDisabled result immediately and makes no network call.
//
// GatewayURL and APIKey must be sourced from trusted ForgeGrid worker
// configuration (flags/environment set by the operator), never from
// anything the coordinator forwards as job/task data.
type Config struct {
	Enabled        bool
	GatewayURL     string        // e.g. "http://10.245.173.58:11435" -- the DadLAN production LLM gateway
	APIKey         string        // sent as X-API-Key; never logged, never embedded in source
	RequestTimeout time.Duration // client-side HTTP timeout; also sent to the gateway as timeout_seconds
	MaxOutputBytes int           // hard cap on response body size read from the gateway
	MaxTokens      int           // sent as max_tokens; bounds a single generation
	MaxConcurrent  int
	AdvisoryOnly   bool
}

// DefaultConfig returns configuration matching the gateway's own
// config.json defaults as read from JParrisDesktop on 2026-09-16
// (num_ctx=4096, think=false, keep_alive=60m are gateway-side settings this
// client does not and cannot override -- see package docs).
func DefaultConfig() Config {
	return Config{
		Enabled:        false, // caller must opt in explicitly
		GatewayURL:     "http://10.245.173.58:11435",
		RequestTimeout: 60 * time.Second,
		MaxOutputBytes: 1 << 20, // 1 MiB; matches the gateway's own max_request_bytes order of magnitude
		MaxTokens:      1024,
		MaxConcurrent:  1,
		AdvisoryOnly:   true,
	}
}

// Status is the outcome of one Generate call. Callers that need to gate an
// action on a "clean pass" must treat only StatusGenerated,
// StatusStrictValid, StatusRepairedValid, StatusRetriedValid, and
// StatusRetriedRepairedValid as usable. Everything else is a failure and
// must never be treated as authoritative model output.
type Status string

const (
	StatusCapabilityDisabled   Status = "capability_disabled"    // feature flag off; no network call made
	StatusUnavailable          Status = "unavailable"            // could not reach the gateway, or the gateway could not reach Ollama
	StatusTimeout              Status = "timeout"                // request exceeded the configured timeout
	StatusOversizedOutput      Status = "oversized_output"       // response exceeded MaxOutputBytes
	StatusAuthError            Status = "auth_error"             // our own API key/IP was rejected by the gateway -- a config problem, not a transient one
	StatusProtocolError        Status = "protocol_error"         // gateway responded but not with a shape/claim we can trust
	StatusGenerated            Status = "generated"              // text mode: no schema was requested, content returned as-is
	StatusStrictValid          Status = "strict_valid"           // structured mode: gateway validated on the first attempt, and we independently agree
	StatusRepairedValid        Status = "repaired_valid"         // gateway's own narrow repair fixed it on the first attempt, and we independently agree
	StatusRetriedValid         Status = "retried_valid"          // gateway's one retry passed directly, and we independently agree
	StatusRetriedRepairedValid Status = "retried_repaired_valid" // gateway's retry needed its own repair too, and we independently agree
	StatusSchemaInvalid        Status = "schema_invalid"         // gateway itself failed closed (HTTP 422) after repair+retry
	StatusConfigurationError   Status = "configuration_error"
	StatusBusy                 Status = "busy"
	StatusUnexpectedModel      Status = "unexpected_model"
)

// Passing reports whether a result represents a usable, validated response
// that a caller may treat as authoritative.
func (s Status) Passing() bool {
	switch s {
	case StatusGenerated, StatusStrictValid, StatusRepairedValid, StatusRetriedValid, StatusRetriedRepairedValid:
		return true
	default:
		return false
	}
}

// Request is one generation request sent to the gateway.
type Request struct {
	SystemPrompt string
	UserPrompt   string
	Schema       map[string]interface{} // nil => gateway "text" mode; non-nil => "structured" mode
	TaskID       string                 // optional correlation id; the gateway assigns one if empty
}

// Metrics mirrors the gateway's own "timings" block.
type Metrics struct {
	TotalDurationMs      float64 `json:"total_duration_ms"`
	LoadDurationMs       float64 `json:"load_duration_ms"`
	PromptEvalDurationMs float64 `json:"prompt_eval_duration_ms"`
	EvalDurationMs       float64 `json:"eval_duration_ms"`
	PromptEvalCount      int     `json:"prompt_eval_count"`
	EvalCount            int     `json:"eval_count"`
	TokensPerSecond      float64 `json:"tokens_per_second"`
}

// RepairNote records one repair applied, either by the gateway (Source
// "gateway") or, only as a last-resort defense-in-depth measure, by this
// client itself when the gateway's own validated:true claim did not
// actually hold up under independent re-validation (Source "client").
type RepairNote struct {
	Source string `json:"source"`
	Detail string `json:"detail"`
}

// Result is the outcome of Generate. Content is the raw text the gateway
// returned in text mode; Parsed is the decoded JSON in structured mode once
// this client has independently confirmed it is schema-valid. Result
// carries no action and this package never acts on it.
type Result struct {
	TaskID                   string
	Status                   Status
	Content                  string
	Parsed                   interface{}
	Repaired                 bool
	Retried                  bool
	RepairLog                []RepairNote
	GatewayErrors            []string
	Metrics                  Metrics
	ElapsedSeconds           float64
	Err                      string // human-readable detail for non-passing statuses; never a stack trace or secret
	DuplicateRecommendations int    `json:"duplicate_recommendations,omitempty"`
}

// Client talks to one DadLAN local-LLM gateway and tracks telemetry across
// calls.
type Client struct {
	cfg  Config
	http *http.Client

	statsMu   sync.Mutex
	stats     Stats
	sem       chan struct{}
	configErr error
}

// Stats is a point-in-time snapshot of telemetry counters, cumulative since
// the Client was created.
type Stats struct {
	Calls               int64
	StatusCounts        map[Status]int64
	TotalElapsedSeconds float64
	TotalGenTokens      int64
	TotalGenSeconds     float64
}

// NewClient constructs a Client. httpClient may be nil, in which case a
// client with cfg.RequestTimeout is used; tests supply one pointed at an
// httptest.Server standing in for the gateway.
func NewClient(cfg Config, httpClient *http.Client) *Client {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 1
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.RequestTimeout}
	}
	clone := *httpClient
	clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	u, err := url.Parse(cfg.GatewayURL)
	var configErr error
	if err != nil || u.Scheme != "http" || u.Host != "10.245.173.58:11435" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		configErr = errors.New("gateway URL must be the trusted JParrisDesktop HTTP origin")
	}
	return &Client{cfg: cfg, http: &clone, sem: make(chan struct{}, cfg.MaxConcurrent), configErr: configErr, stats: Stats{StatusCounts: make(map[Status]int64)}}
}

// Stats returns a copy of the current telemetry snapshot.
func (c *Client) Stats() Stats {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	cp := c.stats
	cp.StatusCounts = make(map[Status]int64, len(c.stats.StatusCounts))
	for k, v := range c.stats.StatusCounts {
		cp.StatusCounts[k] = v
	}
	return cp
}

func (c *Client) recordTelemetry(res Result) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.stats.Calls++
	c.stats.StatusCounts[res.Status]++
	c.stats.TotalElapsedSeconds += res.ElapsedSeconds
	if res.Metrics.EvalDurationMs > 0 {
		c.stats.TotalGenTokens += int64(res.Metrics.EvalCount)
		c.stats.TotalGenSeconds += res.Metrics.EvalDurationMs / 1000.0
	}
}

// gatewayRequest is exactly the JSON body service.py's do_POST reads.
type gatewayRequest struct {
	TaskID         string                 `json:"task_id,omitempty"`
	Mode           string                 `json:"mode"`
	Prompt         string                 `json:"prompt"`
	SystemPrompt   string                 `json:"system_prompt,omitempty"`
	Schema         map[string]interface{} `json:"schema,omitempty"`
	Temperature    float64                `json:"temperature"`
	MaxTokens      int                    `json:"max_tokens"`
	TimeoutSeconds int                    `json:"timeout_seconds"`
}

// gatewayResponse is exactly the JSON shape service.py's send_json calls
// produce, across both the "ok" and "validation_failed"/error paths.
type gatewayResponse struct {
	TaskID     string          `json:"task_id"`
	Status     string          `json:"status"`
	Output     json.RawMessage `json:"output"`
	Model      string          `json:"model"`
	Error      string          `json:"error"`
	Timings    Metrics         `json:"timings"`
	Validation struct {
		Validated  bool     `json:"validated"`
		Repaired   bool     `json:"repaired"`
		Repairs    []string `json:"repairs"`
		Retried    bool     `json:"retried"`
		RetryCount int      `json:"retry_count"`
		Errors     []string `json:"errors"`
	} `json:"validation"`
}

// Generate performs one read-only generation call against the configured
// gateway. It never returns a Go error for a model-side, validation, or
// connectivity failure -- those are reported via Result.Status so callers
// cannot accidentally treat an ignored error as "safe to fall through to
// executing something else". Generate returns a non-nil error only for
// programmer errors (a nil ctx).
func (c *Client) Generate(ctx context.Context, req Request) Result {
	if ctx == nil {
		return Result{Status: StatusConfigurationError, Err: "nil context"}
	}
	if !c.cfg.Enabled {
		res := Result{Status: StatusCapabilityDisabled, Err: "local_llm.generate is disabled by configuration"}
		c.recordTelemetry(res)
		return res
	}
	if c.configErr != nil || c.cfg.APIKey == "" || !c.cfg.AdvisoryOnly {
		res := Result{Status: StatusConfigurationError, Err: "local LLM trusted configuration is incomplete"}
		c.recordTelemetry(res)
		return res
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		res := Result{Status: StatusTimeout, Err: "request cancelled while waiting for LLM capacity"}
		c.recordTelemetry(res)
		return res
	}

	mode := "text"
	if req.Schema != nil {
		mode = "structured"
	}

	timeoutSeconds := int(c.cfg.RequestTimeout.Seconds())
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}
	body := gatewayRequest{
		TaskID:         req.TaskID,
		Mode:           mode,
		Prompt:         req.UserPrompt,
		SystemPrompt:   req.SystemPrompt,
		Schema:         req.Schema,
		Temperature:    0,
		MaxTokens:      c.cfg.MaxTokens,
		TimeoutSeconds: timeoutSeconds,
	}

	start := time.Now()
	gwResp, err := c.call(ctx, body)
	elapsed := time.Since(start).Seconds()
	if err != nil {
		res := classifyErr(err, elapsed)
		c.recordTelemetry(res)
		return res
	}

	res := c.interpret(gwResp, mode, req.Schema, elapsed)
	c.recordTelemetry(res)
	return res
}

func (c *Client) call(ctx context.Context, body gatewayRequest) (gatewayResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return gatewayResponse{}, fmt.Errorf("encoding request: %w", err)
	}

	timeout := c.cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, c.cfg.GatewayURL+"/api/v1/generate", bytes.NewReader(payload))
	if err != nil {
		return gatewayResponse{}, fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.cfg.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return gatewayResponse{}, err // classified by classifyErr: timeout vs unavailable
	}
	defer resp.Body.Close()

	limit := int64(c.cfg.MaxOutputBytes)
	if limit <= 0 {
		limit = 1 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return gatewayResponse{}, fmt.Errorf("reading response: %w", err)
	}
	if int64(len(raw)) > limit {
		return gatewayResponse{}, errOversized
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return gatewayResponse{}, fmt.Errorf("%w: gateway returned HTTP %d", errAuth, resp.StatusCode)
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		return gatewayResponse{}, fmt.Errorf("%w: gateway returned HTTP 422", errValidation)
	}

	if resp.StatusCode != http.StatusOK {
		return gatewayResponse{}, fmt.Errorf("%w: gateway returned HTTP %d", errProtocol, resp.StatusCode)
	}

	var gwResp gatewayResponse
	if jsonErr := json.Unmarshal(raw, &gwResp); jsonErr != nil {
		return gatewayResponse{}, fmt.Errorf("%w: %v", errProtocol, jsonErr)
	}
	return gwResp, nil
}

// interpret maps a parsed gateway response to a Result, independently
// re-validating any claim of schema validity rather than trusting it.
func (c *Client) interpret(gw gatewayResponse, mode string, schema map[string]interface{}, elapsed float64) Result {
	base := Result{
		TaskID:         gw.TaskID,
		Metrics:        gw.Timings,
		ElapsedSeconds: elapsed,
		GatewayErrors:  gw.Validation.Errors,
	}

	switch gw.Status {
	case "timeout":
		base.Status = StatusTimeout
		base.Err = "gateway request timed out"
		return base
	case "model_error":
		base.Status = StatusUnavailable
		base.Err = "gateway model unavailable"
		return base
	case "auth_error":
		base.Status = StatusAuthError
		base.Err = "gateway authentication failed"
		return base
	case "error":
		base.Status = StatusProtocolError
		base.Err = "gateway returned an error"
		return base
	case "validation_failed":
		base.Status = StatusSchemaInvalid
		base.Err = "gateway failed closed after its own repair and retry"
		return base
	case "ok":
		// fall through
	default:
		base.Status = StatusProtocolError
		base.Err = fmt.Sprintf("unrecognized gateway status %q", gw.Status)
		return base
	}
	if gw.Model != "qwen3.5:4b" {
		base.Status = StatusUnexpectedModel
		base.Err = "gateway returned an unexpected model"
		return base
	}

	if mode == "text" {
		var content string
		if err := json.Unmarshal(gw.Output, &content); err != nil {
			base.Status = StatusProtocolError
			base.Err = "gateway reported ok in text mode but output was not a string"
			return base
		}
		base.Status = StatusGenerated
		base.Content = content
		return base
	}

	// Structured mode: never trust gw.Validation.Validated blindly.
	var parsed interface{}
	if err := json.Unmarshal(gw.Output, &parsed); err != nil {
		base.Status = StatusProtocolError
		base.Err = "gateway reported ok in structured mode but output was not valid JSON"
		return base
	}

	var repairLog []RepairNote
	for _, r := range gw.Validation.Repairs {
		repairLog = append(repairLog, RepairNote{Source: "gateway", Detail: r})
	}

	if schemaValid(parsed, schema) {
		base.Parsed = parsed
		base.Repaired = gw.Validation.Repaired
		base.Retried = gw.Validation.Retried
		base.RepairLog = repairLog
		base.Status = statusFor(gw.Validation.Repaired, gw.Validation.Retried)
		if deduped, count := dedupeRecommendations(parsed); count > 0 {
			base.Parsed = deduped
			base.DuplicateRecommendations = count
		}
		return base
	}

	// The gateway claimed validity but our own re-check disagrees: this is
	// exactly the failure mode a defense-in-depth caller exists to catch.
	// Try one narrow, logged local repair as a last resort; otherwise fail
	// closed regardless of what the gateway asserted.
	nullable := nullableFields(schema)
	repaired, changed := narrowRepair(parsed, nullable)
	for _, f := range changed {
		repairLog = append(repairLog, RepairNote{Source: "client", Detail: "coerced string \"null\" to JSON null on field " + f})
	}
	if len(changed) > 0 && schemaValid(repaired, schema) {
		base.Parsed = repaired
		base.Repaired = true
		base.Retried = gw.Validation.Retried
		base.RepairLog = repairLog
		base.Status = statusFor(true, gw.Validation.Retried)
		return base
	}

	base.Status = StatusProtocolError
	base.Err = "gateway reported validated:true but independent re-validation failed"
	base.RepairLog = repairLog
	return base
}

func statusFor(repaired, retried bool) Status {
	switch {
	case !repaired && !retried:
		return StatusStrictValid
	case repaired && !retried:
		return StatusRepairedValid
	case !repaired && retried:
		return StatusRetriedValid
	default:
		return StatusRetriedRepairedValid
	}
}

var (
	errOversized  = errors.New("oversized output")
	errProtocol   = errors.New("protocol error")
	errAuth       = errors.New("auth error")
	errValidation = errors.New("validation error")
)

func classifyErr(err error, elapsed float64) Result {
	switch {
	case errors.Is(err, errOversized):
		return Result{Status: StatusOversizedOutput, ElapsedSeconds: elapsed, Err: err.Error()}
	case errors.Is(err, errValidation):
		return Result{Status: StatusSchemaInvalid, ElapsedSeconds: elapsed, Err: err.Error()}
	case errors.Is(err, errAuth):
		return Result{Status: StatusAuthError, ElapsedSeconds: elapsed, Err: err.Error()}
	case errors.Is(err, errProtocol):
		return Result{Status: StatusProtocolError, ElapsedSeconds: elapsed, Err: err.Error()}
	case errors.Is(err, context.DeadlineExceeded):
		return Result{Status: StatusTimeout, ElapsedSeconds: elapsed, Err: "request exceeded configured timeout"}
	default:
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return Result{Status: StatusTimeout, ElapsedSeconds: elapsed, Err: "request exceeded configured timeout"}
		}
		return Result{Status: StatusUnavailable, ElapsedSeconds: elapsed, Err: err.Error()}
	}
}

// nullableFields returns the set of top-level schema property names whose
// declared type includes "null".
func nullableFields(schema map[string]interface{}) map[string]bool {
	out := make(map[string]bool)
	if schema == nil {
		return out
	}
	props, _ := schema["properties"].(map[string]interface{})
	for name, raw := range props {
		spec, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		switch t := spec["type"].(type) {
		case string:
			if t == "null" {
				out[name] = true
			}
		case []interface{}:
			for _, item := range t {
				if s, ok := item.(string); ok && s == "null" {
					out[name] = true
				}
			}
		}
	}
	return out
}

// narrowRepair coerces the exact string "null" to a JSON null, only on
// fields the schema explicitly declares nullable, only when the field is
// present and is exactly that string. Nothing else is ever touched.
func narrowRepair(parsed interface{}, nullable map[string]bool) (interface{}, []string) {
	obj, ok := parsed.(map[string]interface{})
	if !ok || len(nullable) == 0 {
		return parsed, nil
	}
	out := make(map[string]interface{}, len(obj))
	var changed []string
	for k, v := range obj {
		if nullable[k] {
			if s, ok := v.(string); ok && s == "null" {
				out[k] = nil
				changed = append(changed, k)
				continue
			}
		}
		out[k] = v
	}
	return out, changed
}

// schemaValid performs a small, deliberately non-exhaustive structural
// validation covering exactly the shapes this pilot's schemas use: object
// type, additionalProperties:false, required fields, per-property type
// checking (including nullable via a type array), and enum membership.
// This is the client's own independent check -- it does not delegate to
// the gateway's validated:true claim.
func schemaValid(value interface{}, schema map[string]interface{}) bool {
	if schema == nil {
		return true
	}
	if !validateSchema(schema) {
		return false
	}
	wantType, _ := schema["type"].(string)
	if wantType != "" && wantType != "object" {
		return matchesType(value, wantType)
	}

	obj, ok := value.(map[string]interface{})
	if !ok {
		return false
	}

	props, _ := schema["properties"].(map[string]interface{})
	if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
		for k := range obj {
			if _, known := props[k]; !known {
				return false
			}
		}
	}

	if required, ok := schema["required"].([]interface{}); ok {
		for _, r := range required {
			name, _ := r.(string)
			if _, present := obj[name]; !present {
				return false
			}
		}
	}
	if required, ok := schema["required"].([]string); ok {
		for _, name := range required {
			if _, present := obj[name]; !present {
				return false
			}
		}
	}

	for name, raw := range props {
		spec, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		v, present := obj[name]
		if !present {
			continue // required-ness already checked above
		}
		if !matchesSpec(v, spec) {
			return false
		}
	}
	return true
}

func matchesSpec(v interface{}, spec map[string]interface{}) bool {
	switch t := spec["type"].(type) {
	case string:
		if !matchesType(v, t) {
			return false
		}
	case []interface{}:
		ok := false
		for _, item := range t {
			if s, isStr := item.(string); isStr && matchesType(v, s) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if enum, ok := spec["enum"].([]interface{}); ok {
		match := false
		for _, e := range enum {
			if jsonEqual(e, v) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

func matchesType(v interface{}, t string) bool {
	switch t {
	case "null":
		return v == nil
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "integer":
		f, ok := v.(float64) // encoding/json decodes all JSON numbers as float64
		return ok && f == float64(int64(f))
	case "number":
		_, ok := v.(float64)
		return ok
	case "object":
		_, ok := v.(map[string]interface{})
		return ok
	case "array":
		_, ok := v.([]interface{})
		return ok
	default:
		return false
	}
}

func validateSchema(s map[string]interface{}) bool {
	allowed := map[string]bool{"type": true, "properties": true, "required": true, "additionalProperties": true, "enum": true, "items": true}
	for k := range s {
		if !allowed[k] {
			return false
		}
	}
	if typ, ok := s["type"]; ok {
		valid := func(x interface{}) bool {
			t, ok := x.(string)
			if !ok {
				return false
			}
			return map[string]bool{"object": true, "array": true, "string": true, "number": true, "integer": true, "boolean": true, "null": true}[t]
		}
		switch t := typ.(type) {
		case string:
			if !valid(t) {
				return false
			}
		case []interface{}:
			if len(t) == 0 {
				return false
			}
			for _, x := range t {
				if !valid(x) {
					return false
				}
			}
		default:
			return false
		}
	}
	if p, ok := s["properties"]; ok {
		m, ok := p.(map[string]interface{})
		if !ok {
			return false
		}
		for _, v := range m {
			sm, ok := v.(map[string]interface{})
			if !ok || !validateSchema(sm) {
				return false
			}
		}
	}
	if r, ok := s["required"]; ok {
		switch x := r.(type) {
		case []interface{}:
			for _, v := range x {
				if _, ok := v.(string); !ok {
					return false
				}
			}
		case []string:
		default:
			return false
		}
	}
	if a, ok := s["additionalProperties"]; ok {
		if _, ok := a.(bool); !ok {
			return false
		}
	}
	if e, ok := s["enum"]; ok {
		if _, ok := e.([]interface{}); !ok {
			if _, ok := e.([]string); !ok {
				return false
			}
		}
	}
	if i, ok := s["items"]; ok {
		m, ok := i.(map[string]interface{})
		if !ok || !validateSchema(m) {
			return false
		}
	}
	return true
}

func jsonEqual(a, b interface{}) bool {
	return fmt.Sprintf("%T:%v", a, a) == fmt.Sprintf("%T:%v", b, b)
}

func dedupeRecommendations(v interface{}) (interface{}, int) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return v, 0
	}
	raw, ok := m["recommendations"].([]interface{})
	if !ok {
		return v, 0
	}
	seen := map[string]bool{}
	out := make([]interface{}, 0, len(raw))
	dup := 0
	for _, item := range raw {
		im, ok := item.(map[string]interface{})
		if !ok {
			out = append(out, item)
			continue
		}
		cap, _ := im["capability"].(string)
		target, _ := im["target"].(string)
		if cap == "" || target == "" {
			out = append(out, item)
			continue
		}
		b, _ := json.Marshal(map[string]interface{}{"capability": cap, "target": target, "arguments": im["arguments"]})
		k := string(b)
		if seen[k] {
			dup++
			continue
		}
		seen[k] = true
		out = append(out, item)
	}
	if dup == 0 {
		return v, 0
	}
	cp := map[string]interface{}{}
	for k, x := range m {
		cp[k] = x
	}
	cp["recommendations"] = out
	return cp, dup
}
