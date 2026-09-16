# Qwen Assistant Evolution

qwen3.5:4b model weights are static at runtime, but the assistant system can improve through:

### Persistent memory
* conversation history
* durable user-approved facts/preferences
* retrieval of relevant prior context

### Current knowledge
* controlled web search
* safe page retrieval
* citations
* time-sensitive fact verification

### DadLAN knowledge / RAG
* indexed ForgeGrid documentation
* machine inventory
* troubleshooting notes
* approved local documents
* retrieval instead of stuffing everything into prompts

### Feedback
* thumbs up/down or useful/not useful
* optional user correction
* record what answer/prompt/model/tool path produced the result

### Evaluation
Maintain a small regression/evaluation set such as:
* identify AVANCE correctly
* identify JParrisDesktop correctly
* current Matildas score requires web lookup
* unknown facts are not invented
* malicious web prompt injection ignored
* no tool call creates a ForgeGrid job
* source citations correspond to fetched material

Use these tests when changing:
* system prompts
* model
* generation settings
* web tooling
* retrieval logic

### Prompt/version improvement
Version the assistant system prompt/config.
Allow a new prompt/config to be evaluated against the regression suite before becoming active.
Do not allow Qwen to silently rewrite its own production system prompt.

### Model upgrades
Provide an operator-controlled way to:
* detect available Ollama model versions
* benchmark candidate local models
* compare latency, RAM/VRAM, quality/eval score
* propose an upgrade

Never automatically replace the production model without approval and validation.

### Self-diagnosis, not autonomous self-modification
Qwen may:
* identify weaknesses
* suggest code/config changes
* produce candidate patches as inert data

But it must NOT:
* merge its own code
* execute its own patches
* change security policy
* grant itself tools
* modify ForgeGrid workers
* replace its own model

Human/agent review remains the authority.
