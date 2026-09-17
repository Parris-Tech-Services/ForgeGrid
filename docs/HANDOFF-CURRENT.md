# ForgeGrid bring-forward run — current handoff

Updated 2026-09-17 after the research-snippet fix and live browser verification. Claude has stopped; Codex is now the primary implementer. Twelve ForgeGrid workers are currently online; Laptop08 is drained for the Defender investigation. No updater canary, credential rotation, destructive recovery, or fault injection was performed.

## Current verified state

| Worktree | Branch | HEAD | State |
|---|---|---|---|
| `/home/josh/dev/6 Laptops/ForgeGrid` | `feature/qwen-assistant-v2` | `1e84e77` | clean, pushed |
| `/home/josh/dev/6 Laptops/ForgeGrid-hygiene` | `chore/project-hygiene` | `051ed35` | census update pending |
| `/home/josh/dev/6 Laptops/ForgeGrid-integration` | `integration/next` | `aa11e14` | clean, pushed |
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

The integrated restart-fence fix is committed and pushed to `origin/integration/next` as `5017f41`. The research-context fix is committed and pushed as `aa11e14`; it forwards untrusted SearXNG snippets when dynamic pages lack useful static text. The focused race passed 50 repetitions, the full normal/race/vet/build/cross-compile matrix passed, and the live browser verified casual no-search, automatic weather and forced research behavior. The live coordinator was rebuilt from `aa11e14` and restarted; the updater fix is not deployed to DadLAN and no updater canary has run.

## Integrated Qwen state

The integrated branch contains the persistent JSON history/search, chat-only auth, SSRF-safe fetch boundary, bounded local SearXNG provider, server-side prompt/history assembly, explicit UI toggle and source display. Live API and browser checks passed: casual chat returned zero sources with Web Search off, automatic weather returned three sources and a useful forecast, forced research returned source-backed Go release evidence, rename/read/delete worked, and a conversation survived coordinator restart. Mobile visual checks remain unverified.

## Next action

Complete the capability/toolchain audit and browser/E2E evidence, finish Laptop08 diagnosis, and independently review the failed-then-completed Eleven-Realms dispatch evidence. The successful dispatch used Laptop04 after adding the Eleven-Realms repository to its existing worker allowlist; the AI task completed with `no changes` because the worker lacked valid OpenAI authentication, so the end-to-end mission acceptance is not yet proven.

## Hard boundaries

Normal compute is authorized by Josh's broadened `GO FLEET` instruction. Updater installation/canary, fault injection, credential changes, destructive recovery and irreversible fleet changes remain separately gated. Do not push to `main`, force-push, delete unrelated refs, or expose services publicly. Qwen remains advisory and policy-controlled.
