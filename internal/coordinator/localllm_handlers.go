package coordinator

import (
	"encoding/json"
	"io"
	"net/http"

	"forgegrid/internal/localllm"
)

// handleLLMGenerate handles requests to the local_llm.generate capability.
func (c *Coordinator) handleLLMGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req localllm.Request
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	if req.SystemPrompt == "" {
		req.SystemPrompt = "You are qwen3.5:4b running via Ollama on JParrisDesktop. JParrisDesktop is a Windows machine on Josh's DadLAN. AVANCE-WS7 is the Fedora machine running the ForgeGrid coordinator. Do not invent meanings for machine names. If a fact is not supplied or observable, say you do not know."
	}
	
	if c.LocalLLM == nil {
		_ = json.NewEncoder(w).Encode(localllm.Result{
			Status: localllm.StatusCapabilityDisabled,
			Err:    "local_llm.generate is disabled by configuration",
		})
		return
	}
	res := c.LocalLLM.Generate(r.Context(), req)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// handleLLMStatus returns the telemetry for the local LLM.
func (c *Coordinator) handleLLMStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c.LocalLLM == nil {
		http.Error(w, "Local LLM capability is not initialized", http.StatusServiceUnavailable)
		return
	}
	stats := c.LocalLLM.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
