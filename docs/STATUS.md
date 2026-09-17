# ForgeGrid — Status

**Where we are:** Phase 1 (preserve and organise) in progress. Inventory done, `973c048` preserved (both the literal commit and Codex's rebase, under two distinct branch names — see below), hygiene branch created, untracked files archived, `.gitignore` hardened, `tools/action1_llm_host.py` consolidated, this file created. Mid-run correction applied: fleet is Laptop01–11 + JParrisDesktop (not 01–10), and the Codex review prompts were tightened to be genuinely read-only.
**Next action:** Finish Phase 1 (CLAUDE.md, point NEXT_STEPS.md/Qwen status doc here, push this branch), then Phase 2 baseline validation (gofmt/vet/build/test/-race/cross-compile) on the three workstream branches.

This is the single source of truth for the 2026-09-17 "bring everything forward" run
(`~/forgegrid-handoff/2026-09-17/PROMPT.md`). Other status docs should link here
instead of restating this. Live git/API state always outranks anything below if they
disagree — recheck before trusting a stale entry.

## Branch map

| Branch | Head SHA | Pushed? | Role this run |
|---|---|---|---|
| `feature/qwen-assistant-v2` | `f6c9914` | yes | Qwen assistant workstream (main worktree) |
| `feature/qwen-durable-history-sqlite` → `origin/backup/973c048-persistent-history` | `ae0359f` | yes | `973c048` rebased onto `f6c9914` (adds only the two roadmap docs already on `f6c9914`, nothing else changes). Branch name says "sqlite" — stale, D1 says JSON; content is what matters |
| `temp-claude-work` → `origin/backup/973c048-persistent-history-original` | `973c048` | yes | Literal, unmodified historical commit. Preserved under this second name (not the first) because the first name was already repointed to `ae0359f` and this run never force-pushes — both objects exist on the remote, unambiguously named, neither lost |
| `fix/self-update-reliability` | `33c767e` | yes | Updater workstream (own worktree `ForgeGrid-self-update-reliability`) |
| `fix/structured-execution-security-gates-v2` | `be08bcb` | yes | Unrelated workstream (own worktree `ForgeGrid-security-gates`), parked — see Decisions |
| `chore/project-hygiene` | (this commit) | no yet | This file, `.gitignore`, `tools/`, `CLAUDE.md` |
| `main` (local) | `505716f` | **no — 2 commits ahead of `origin/main`, unpushed** | Pre-existing anomaly, not created by this run — see Open risks |
| `origin/main` | `03aa1ac` | — | Integration target |

Full graph: `git log --graph --oneline --decorate --all`.

## Workstreams (Parts A–H roadmap)

| Part | Item | Status |
|---|---|---|
| A | Persistent, searchable chat history | In progress — bringing `ae0359f` forward (D1: JSON store, not SQLite), search still to add (Phase 5) |
| B | Opt-in web research | Not started (Phase 9) |
| C | Remote phone access | Design only, not built (D4, Phase 10) |
| D | Mobile-friendly UI | Not started (Phase 7) |
| E | Security hardening / chat-only login | Not started (Phase 6) |
| — | Self-update reliability (findings A–E) | Fixed and tested on `33c767e`, pushed, **not deployed to any laptop**; two-hop canary on Laptop02 planned for Phase 12, gated on Josh's "GO fleet" |

## Fleet

DadLAN is **Laptop01–11 plus JParrisDesktop** (corrected 2026-09-17; the source reports and the original bundle said Laptop01–10). Laptop11 is the HP ProBook 11 EE G2, 8GB RAM. All 11 laptops plus JParrisDesktop are completely hands-off — no Action1, MeshCentral, SMB or update-queue calls — until Josh sends "GO fleet".

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
- No `go test ./...` / `-race` run has completed yet on any of the three workstream branches (Phase 2, next).
- **Resolved:** an external review caught that `backup/973c048-persistent-history` no longer pointed at the literal `973c048` commit the plan's checkpoint-review checklist expects — Codex had repointed that name to `ae0359f` (the rebase). Fixed without a force-push: the literal `973c048` snapshot is now separately preserved at `backup/973c048-persistent-history-original`. Both objects exist on the remote; see the branch map above for which name is which.

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
