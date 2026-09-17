# ForgeGrid bring-forward run — current handoff

Updated 2026-09-17 after normal compute activation. Claude has stopped; Codex is now the primary implementer. Ten ForgeGrid workers are online after an idempotent existing-service start; Laptop08 failed the Action1 action and Laptop11 has no ForgeGrid registration. No updater, credential, destructive, or fault-injection action was performed.

## Current verified state

| Worktree | Branch | HEAD | State |
|---|---|---|---|
| `/home/josh/dev/6 Laptops/ForgeGrid` | `feature/qwen-assistant-v2` | `1e84e77` | clean, pushed |
| `/home/josh/dev/6 Laptops/ForgeGrid-hygiene` | `chore/project-hygiene` | `051ed35` | census update pending |
| `/home/josh/dev/6 Laptops/ForgeGrid-integration` | `integration/next` | `4b7bed8` | clean, pushed |
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

The Qwen branch brings forward the persistent-history lineage through merge `338eacd`, adds hardened JSON persistence and search in `6d6e278`, fixes failed-append in-memory rollback in `1038713`, adds chat-only auth in `326af5c`, adds the tested SSRF-safe fetch boundary in `bfa7942`, and adds the bounded fixed-local SearXNG provider in `fc0fcdc`. The integrated branch contains these at `a5685b2`. Server-side prompt/history, chat-only auth, and provider wiring are present; explicit UI toggle/source display, SearXNG deployment, and mobile polish are not yet complete.

## Next action

Diagnose Laptop08 and Laptop11 worker connectivity, then add immutable-SHA distributed validation jobs; continue Phase 9 provider/UI wiring and Phase 7 mobile work on the Qwen branch, merge each completed branch head into `integration/next`, rerun the full suite and cross-compiles, and stop at Checkpoint A before updater/fault actions with exact immutable SHAs for independent review.

## Hard boundaries

Normal compute is authorized by Josh's broadened `GO FLEET` instruction. Updater installation/canary, fault injection, credential changes, destructive recovery and irreversible fleet changes remain separately gated. Do not push to `main`, force-push, delete unrelated refs, or expose services publicly. Qwen remains advisory and policy-controlled.
