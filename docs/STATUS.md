# ForgeGrid — Status

**Where we are:** The live coordinator has all 12 DadLAN workers online and ready. All workers now report ForgeGrid `0.8.3 / 77a07222a435`; all 12 completed a harmless built-in smoke job on 2026-09-17. Laptop10 remains the Windows/386 specialist.
**Next action:** Complete the capability/toolchain audit and continue Qwen research UI/mobile/E2E work.

This is the single source of truth for the 2026-09-17 "bring everything forward" run
(`~/forgegrid-handoff/2026-09-17/PROMPT.md`). Other status docs should link here
instead of restating this. Live git/API state always outranks anything below if they
disagree — recheck before trusting a stale entry.

## Branch map

| Branch | Head SHA | Pushed? | Role this run |
|---|---|---|---|
| `feature/qwen-assistant-v2` | `bfa7942` | yes | Qwen assistant workstream (main worktree) |
| `feature/qwen-durable-history-sqlite` → `origin/backup/973c048-persistent-history` | `ae0359f` | yes | `973c048` rebased onto `f6c9914` (adds only the two roadmap docs already on `f6c9914`, nothing else changes). Branch name says "sqlite" — stale, D1 says JSON; content is what matters |
| `temp-claude-work` → `origin/backup/973c048-persistent-history-original` | `973c048` | yes | Literal, unmodified historical commit. Preserved under this second name (not the first) because the first name was already repointed to `ae0359f` and this run never force-pushes — both objects exist on the remote, unambiguously named, neither lost |
| `fix/self-update-reliability` | `9ff9662` | yes | Updater workstream; concurrency fix verified (own worktree `ForgeGrid-self-update-reliability`) |
| `fix/structured-execution-security-gates-v2` | `be08bcb` | yes | Unrelated workstream (own worktree `ForgeGrid-security-gates`), parked — see Decisions |
| `chore/project-hygiene` | `633d7f5` | yes | This file, `.gitignore`, `tools/`, `CLAUDE.md` |
| `integration/next` | `07bff11` | yes | Merged pre-fleet integration build |
| `main` (local) | `505716f` | **no — 2 commits ahead of `origin/main`, unpushed** | Pre-existing anomaly, not created by this run — see Open risks |
| `origin/main` | `03aa1ac` | — | Integration target |

Full graph: `git log --graph --oneline --decorate --all`.

## Workstreams (Parts A–H roadmap)

| Part | Item | Status |
|---|---|---|
| A | Persistent, searchable chat history | Integrated and tested — JSON store is mutexed, atomic/backed-up, searchable, hard-delete, and bounded; server-side prompt assembly is present. |
| B | Opt-in web research | Fixed local SearXNG provider and safe fetch path implemented/tested; explicit UI toggle, source display and SearXNG deployment remain |
| C | Remote phone access | Design only, not built (D4, Phase 10) |
| D | Mobile-friendly UI | Not started (Phase 7) |
| E | Security hardening / chat-only login | Chat-only login implemented and focused auth tests pass; broader request/logging/XSS coverage remains |
| — | Self-update reliability (findings A–E) | Fixed and tested on `9ff9662`, pushed, **not deployed to any laptop**; two-hop canary on Laptop02 planned for Phase 12, gated on Josh's "GO fleet" |

## Fleet

DadLAN is **Laptop01–11 plus JParrisDesktop**. Action1 most recently reported 11 connected endpoints (Laptop01's Action1 connection may drift independently); the ForgeGrid coordinator reports all 12 workers online and ready. All 12 are on ForgeGrid `0.8.3 / 77a07222a435`, with Laptop10 on the matching Windows/386 artifact. Existing worker service startup settings were preserved; no fleet-wide autostart redesign was performed.

The 12-worker smoke wave completed successfully: `job-5287aa067d9d3fbd9e1c7934eb954e4b`, `job-e6db274b1ad8091a3e66ace67f81556`, `job-64e924f811a7363a18777d89949c90d9`, `job-4ed6b7026e952f7ff3512716c2e1a5ba`, `job-b946431831e4949adf0337bb377725a3`, `job-c7d5f0834433600e1399569a51c1601e`, `job-a117345d9e54d2a9a35ab44c047f0ca5`, `job-9811dbee4ad71d434776fef40217b35f`, `job-11921afd33945843247931a2fd339bce`, `job-55e56509308b8befca6c0c1517e68207`, `job-38ba5b0934a291eadb58fb1eaeca5ca6`, and `job-8741a219af0e9e020d70d0d22817a0e4`. These prove liveness/execution, not full toolchain readiness.

Performance tuning completed 2026-09-17: High Performance power plan and AC no-sleep were applied successfully to Laptop01, Laptop03, Laptop07, Laptop08, Laptop09, Laptop10, and Laptop11. Laptop03/07/08/09/10 were labeled for legacy compatibility scheduling; Laptop10 additionally has `compat:win386` and remains a one-job-at-a-time specialist. Laptop08 was measured at 100% CPU during the audit and needs a follow-up process/thermal investigation. ForgeGrid does not currently expose a per-worker concurrency setting; scheduler specialization remains label-based.

Follow-up evidence: Laptop08's top process was Windows Defender `MsMpEng`; real-time protection is enabled, tamper protection is enabled, idle-only scanning is enabled, and no exclusions or threats were reported. Its built-in smoke job took 7.96 seconds versus under 1.4 seconds for the other measured workers, so Laptop08 was placed in coordinator drain mode pending investigation. No Defender protection was disabled.

## Gates

- **Checkpoint A** (after history + web search are live, before any laptop is touched): stop, list exact SHAs of every branch head / `integration/next` / deployed build, wait for Josh's Codex-reviewed go-ahead. Requires explicit **"GO fleet"** (and separately **"GO fault-test"**) before Phase 12/13.
- **Checkpoint B** (PRs open): stop, wait for Josh to merge. Don't merge anything.
- **Never**: fleet actions before GO fleet, Tailscale/VPN/port-forwarding setup, sudo without asking, force-push, push to `main`, deleting anything not created this run, printing/logging any secret file listed in `.gitignore`'s "Runtime data and secrets" block.

## Decisions this run builds to

See `~/forgegrid-handoff/2026-09-17/PROMPT.md` section 4 (D1–D10) for the full list, recorded by Josh via a separate design session. Summary: JSON history store not SQLite (D1), server-side-only prompt assembly (D2), opt-in web search via self-hosted SearXNG + SSRF-safe fetcher (D3), remote access designed but not built this run (D4), two-hop canary for the updater (D5), merge-not-rebase integration via `integration/next` with three separate PRs to `main` (D6), this file as the one status doc (D7), coordinator redeploys don't need per-instance approval but fleet/remote-access/`main` always do (D8), hard deletes for chat history (D9), fleet-wide rollout and several other items explicitly out of scope this run (D10).

**Security-gates worktree (`fix/structured-execution-security-gates-v2`, `be08bcb`) — parked.** Already fully pushed (no unpushed commits to preserve). Unrelated to the Qwen/updater/hygiene workstreams this run covers (D6 only names those three for `integration/next`). Also backs branches `pr-13` and two `rescue/reconcile-20260827-*` branches, suggesting an earlier, separate reconciliation effort. Left untouched; not part of this run's integration or PRs. Josh should decide separately whether/when to land it.

## Open risks

- **Local `main` is 2 commits ahead of `origin/main`, unpushed** (`505716f`, and `be08bcb` — the same commit as the parked security-gates branch). This predates this run; nobody has pushed local `main`. Guardrail says nothing gets pushed to `main` except via Josh's own merge, so this is left as-is and flagged here rather than pushed or altered.
- **AVANCE-WS7 now stores data, not just runs things.** Once persistent history is live, chat content is saved on this machine. Until it's confirmed whether AVANCE-WS7 sits on Josh's employer's network (the same open question blocking D4/remote access), avoid putting anything in the Qwen chat that shouldn't sit on a work machine.
- **17 untracked files found in the main worktree** — archived to `~/forgegrid-scratch-archive/2026-09-17/`, not deleted. No hardcoded secret values found in them (they load Action1 credentials from the existing 0600 `~/.config/action1.json` at runtime). One of them, `action1_proxy.py`, is a prototype that bypasses the reviewed `local_llm` backend entirely (calls Action1 to hit Ollama directly on JParrisDesktop per chat message) — archived for reference only, deliberately **not** rebuilt into `tools/`, since it conflicts with D2 and the "don't touch JParrisDesktop's gateway/Ollama" guardrail.
- **D2 gap confirmed live:** the currently deployed GUI (`ee801e0`/`f6c9914`) sends `SystemPrompt` from the browser directly to the admin-only `/api/capabilities/llm/generate` route (`internal/ui/dashboard/llm/index.html:28`, `internal/coordinator/coordinator.go:154`). This means today's chat UI requires the dashboard admin credential to use at all, and prompt assembly is not server-side. To fix in Phase 5 under a new `/api/llm/*` route family per D2.
- **`7efbbcf` is a dangling, unreferenced commit** (same title as `735fede`, an earlier/superseded attempt at the same LLM-gateway integration work). No branch contains it; `735fede` is the canonical, pushed version. No action needed, just resolves a contradiction from the source reports.
- **Two other active AI agent processes on this machine** (`codex --yolo`, another Claude Code session) were confirmed at the start of this run to be idle at `~`, not touching ForgeGrid. Codex has since acted deliberately as an early collaborator: pushed `973c048`'s rebase as `ae0359f`/`backup/973c048-persistent-history`, and reported a D2 finding matching the one above. Per the amended process (`AMENDMENT.txt`), Codex's role going forward is a **read-only** reviewer invoked by Josh at each checkpoint against pinned SHAs — not an ongoing active collaborator making its own commits.
- **Resolved:** an external review caught that `backup/973c048-persistent-history` no longer pointed at the literal `973c048` commit the plan's checkpoint-review checklist expects — Codex had repointed that name to `ae0359f` (the rebase). Fixed without a force-push: the literal `973c048` snapshot is now separately preserved at `backup/973c048-persistent-history-original`. Both objects exist on the remote; see the branch map above for which name is which.
- **A second amendment (received while Phase 2 was running) asked for the same fix via immutable tags instead of branch renaming, and asked that the earlier Codex session's changes be explicitly inventoried and verified.** Both are addressed:
  - The branch-based fix above already satisfies the underlying requirement (both commits preserved, unambiguously named, no force-push, no data loss) — kept as-is per the amendment's own fallback ("if you've already handled this differently, write down exactly what you did"), rather than adding redundant tags pointing at the same two commits.
  - Full inventory of what the earlier Codex session touched: exactly one push, to `backup/973c048-persistent-history` (later effectively split into the two branches above once the naming conflict was caught). Confirmed via `git log --all --since=2026-09-17` and a full remote branch re-listing that no other ref, commit, doc, or file was touched by it. Its tree-diff claim is verified, not just trusted: `ae0359f`'s only difference from `973c048` is the two roadmap docs that `f6c9914` (its actual parent) already had — no other content was added, changed or removed. Safe to bring forward in Phase 3 as-is.
- The updater branch was fully revalidated at `9ff9662`; the integrated tip `07bff11` passed normal and race tests after the provider merge. Earlier integrated vet/build/format/diff-check/cross-compile evidence remains valid for unchanged code; rerun the complete matrix before Checkpoint A. `govulncheck` is unavailable in this environment and remains unverified.

## Phase 2 — baseline validation results

The updater workstream was revalidated at `9ff9662`; Qwen remains at `1c013ed`; the preserved history lineage remains at `ae0359f` with literal `973c048` separately preserved.

| Check | `9ff9662` | `1c013ed` | `ae0359f` |
|---|---|---|---|
| `gofmt -l .` | clean | 2 files flagged, fixed on `feature/qwen-assistant-v2` in `1c013ed` (whitespace only) | same 2 files, inherited from `f6c9914`; fix carries forward once Phase 3 rebases this work onto the now-fixed branch |
| `go vet ./...` | clean | clean | clean |
| `go build ./...` | clean | clean | clean |
| `git diff --check` | clean | clean | clean |
| `go test ./...` | pass | pass, incl. `TestDownstream422` | pass, incl. `TestDownstream422` |
| `go test -race ./...` | pass | pass (prior validation) | pass (prior validation) |
| cross-compile (linux/amd64, windows/amd64, windows/386) | all 3 OK | all 3 OK | all 3 OK |
| `govulncheck ./...` | no vulnerabilities | no vulnerabilities | no vulnerabilities |

`TestDownstream422` (asked about explicitly in the source reports) exists in `internal/localllm/localllm_extra_test.go` and passes on both Qwen branches; it doesn't exist on the updater branch (different package).

## Backlog (explicitly out of scope this run — D10)

- Fleet-wide rollout beyond the Laptop02 canary (a wave plan can be drafted, not executed)
- A second confirmation canary run on Laptop03
- The 100-job acceptance test
- Laptop10 and the 386 update path
- The AgentBridge 401 issue
- MeshCentral
- Control Centre
- LANCommander
- Moving chat history from JSON to SQLite (triggers documented in D1: file > 25 MB, or save p95 > 100 ms)
