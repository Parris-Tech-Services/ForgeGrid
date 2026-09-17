# ForgeGrid bring-forward run — current handoff

Updated 2026-09-17 after auth hardening and the SSRF-safe research boundary. Claude has stopped; Codex is now the primary implementer. No DadLAN fleet machine has been touched.

## Current verified state

| Worktree | Branch | HEAD | State |
|---|---|---|---|
| `/home/josh/dev/6 Laptops/ForgeGrid` | `feature/qwen-assistant-v2` | `bfa7942` | clean, pushed |
| `/home/josh/dev/6 Laptops/ForgeGrid-hygiene` | `chore/project-hygiene` | `633d7f5` | clean, pushed |
| `/home/josh/dev/6 Laptops/ForgeGrid-integration` | `integration/next` | `04897b3` | clean, pushed |
| `/home/josh/dev/6 Laptops/ForgeGrid-self-update-reliability` | `fix/self-update-reliability` | `9ff9662` | clean, pushed |
| `/home/josh/dev/6 Laptops/ForgeGrid-security-gates` | `fix/structured-execution-security-gates-v2` | `be08bcb` | parked, pushed |

`origin/backup/973c048-persistent-history` points to `ae0359f`; the exact historical commit is preserved at `origin/backup/973c048-persistent-history-original` and at the immutable preservation ref created by the prior session. Neither branch was force-pushed or deleted.

## Updater race investigation and resolution

At `33c767e`, `TestRollbackConcurrencyRace` failed intermittently in the full package race suite with `Start() was executed 2 times`. Ten isolated repetitions passed, but an uncached full updater-package run reproduced the failure. Code inspection confirmed a check-then-act restart lease: `claimIsLive` was followed by non-exclusive lease creation, and the lease was released before verification completed.

Commit `9ff9662` fixes this with an atomic restart-lease directory claim, owner-file heartbeat, stale-owner fencing, process-local serialization, and a durable `.verified` completion fence. The lease remains owned through verification and is released only after the completion fence is written.

Evidence on `9ff9662`:

- `gofmt -l .`: pass
- `go vet ./...`: pass
- `go build ./...`: pass
- `go test ./...`: pass
- `go test -race ./...`: pass
- `go test -race -run '^TestRollbackConcurrencyRace$' -count=20 ./internal/worker/`: pass, exit 0
- `git diff --check`: pass
- cross-compiles: `linux/amd64`, `windows/amd64`, `windows/386` pass with `CGO_ENABLED=0`
- `govulncheck ./...`: unavailable in the environment, unverified

The updater fix is committed and pushed to `origin/fix/self-update-reliability`. It is not deployed and no canary has run.

## Integrated Qwen state

The Qwen branch brings forward the persistent-history lineage through merge `338eacd`, adds hardened JSON persistence and search in `6d6e278`, fixes failed-append in-memory rollback in `1038713`, adds chat-only auth in `326af5c`, and adds the tested SSRF-safe fetch boundary in `bfa7942`. The integrated branch contains these at `04897b3`. Server-side prompt/history assembly and chat-only auth are present; research provider/UI wiring and mobile polish are not yet complete.

## Next action

Continue Phase 9 provider/UI wiring and Phase 7 mobile work on the Qwen branch, merge each completed branch head into `integration/next`, rerun the full suite and cross-compiles, and stop at Checkpoint A before any fleet action with exact immutable SHAs for independent review.

## Hard boundaries

Fleet machines are Laptop01–11, JParrisDesktop, and any other registered worker; all are hands-off until Josh explicitly says `GO fleet`. Do not use Action1, MeshCentral, SMB, remote updates, restarts, credentials, or fault injection before that gate. Do not push to `main`, force-push, delete unrelated refs, or expose services publicly. Qwen remains advisory and policy-controlled.
