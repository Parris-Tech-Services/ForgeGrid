# ForgeGrid — agent guardrails

These are standing rules for any agent (Claude, Codex, Antigravity, or other) working in
this repo, distilled from the 2026-09-17 "bring everything forward" run. See
`docs/STATUS.md` for current state — it is the single source of truth; this file is
rules, not status.

## Fleet

DadLAN is **Laptop01–11 plus JParrisDesktop**. (Corrected 2026-09-17 — older docs in
this repo may still say Laptop01–10; that range is stale. Laptop11 is the HP ProBook 11
EE G2, 8GB RAM.) JParrisDesktop is the LLM host, running Ollama.

## Secrets — never print, cat, echo, log or paste

- `dashboard-login.txt`, any chat-only login file
- `llm-api-key.txt`, `llm-token.txt`
- `github-token.txt`
- `coordinator.json` (holds the AdminToken)
- the SearXNG secret
- any Action1 credential (`~/.config/action1.json`)

Only touch these inside commands that don't echo them. If a command might print a
secret, don't run it. Load credentials at runtime from a `0600` file or an environment
variable — never hardcode a value in source. New secret/runtime files get `0600`
permissions; never loosen existing permissions. Before every push, secret-scan the
commits being pushed (gitleaks if available, otherwise a targeted grep for key/token/
password patterns, `X-API-Key`, `Authorization`, private keys) and report the result.
Never commit a runtime or secret file — see the "Runtime data and secrets" block in
`.gitignore`.

## Live systems — hands off without explicit sign-off

- **Fleet machines** (Laptop01–11, JParrisDesktop) are hands-off until a human explicitly
  says go: no Action1 scripts, no MeshCentral commands, no update queueing, no SMB
  writes, no restarts. Read-only coordinator API queries about the fleet are fine.
  Updates are queued one worker ID at a time, never "all".
- **JParrisDesktop:** don't touch its gateway, its Ollama install, or the private
  `127.0.0.1:8080` playground. Prompt assembly happens server-side, in the coordinator —
  never bypass it with a direct-to-Ollama path (a past prototype, `action1_proxy.py`,
  did exactly this via Action1 script execution; it's archived, not reused).
- **Network exposure:** don't set up Tailscale or a VPN, don't change port forwarding or
  the firewall, don't open any listener reachable from off this machine, without asking
  first.
- **sudo:** if something needs it, show the exact command and why, and ask.
- **Git:** no force-pushes, ever — if a branch name needs to point somewhere else,
  preserve what it currently points to under a new name first, then repoint by asking,
  or leave the mismatch documented rather than force-pushing. Nothing gets pushed to
  `main` except by a human's own merge.
- **Deleting:** don't delete branches, commits or files you didn't create. Archive them
  instead (see `~/forgegrid-scratch-archive/` for precedent).

## Environment

- The repo path contains a space — quote it everywhere: `"/home/josh/dev/6 Laptops/ForgeGrid"`.
  This has broken a systemd unit on this machine before when left unquoted.
- Before starting multi-phase or infra-touching work, check whether another agent
  (codex, antigravity, another claude process) looks active on this repo — check process
  cwd, recent file mtimes, and uncommitted changes in every worktree. If one looks
  active, stop and ask, unless it's a reviewer explicitly invoked read-only by the human
  against pinned SHAs (that's expected, not a conflict).
- Tests run on real hardware here. If something in the sandbox blocks a test (e.g.
  binding a localhost listener), say so and ask what needs allowing — don't skip it.

## Evidence

"Done" means demonstrated — command output, test results, hashes, HTTP status codes or
screenshots. Label anything verified only by reading code as code-reading; it's not a
substitute for a live check. Reviews should be adversarial: the reviewer's job is to find
problems, not confirm the implementer's claims.
