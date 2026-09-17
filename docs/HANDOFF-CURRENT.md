# ForgeGrid bring-forward run — current handoff

Updated 2026-09-17 after the updater concurrency fix. Claude has stopped; Codex is now the primary implementer. No DadLAN fleet machine has been touched.

## Current verified state

| Worktree | Branch | HEAD | State |
|---|---|---|---|
| `/home/josh/dev/6 Laptops/ForgeGrid` | `feature/qwen-assistant-v2` | `1c013ed` | clean, pushed |
| `/home/josh/dev/6 Laptops/ForgeGrid-hygiene` | `chore/project-hygiene` | `d900e85` | this handoff update pending |
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

## Next action

Proceed with Phase 3 integration only after updating the canonical status file with this resolution. Then bring forward the JSON history implementation, implement server-side prompt assembly (D2), searchable history, web research and chat-only auth. Stop at Checkpoint A before any fleet action and provide exact immutable SHAs for independent review.

## Hard boundaries

Fleet machines are Laptop01–11, JParrisDesktop, and any other registered worker; all are hands-off until Josh explicitly says `GO fleet`. Do not use Action1, MeshCentral, SMB, remote updates, restarts, credentials, or fault injection before that gate. Do not push to `main`, force-push, delete unrelated refs, or expose services publicly. Qwen remains advisory and policy-controlled.
