# ForgeGrid bring-forward run — live handoff (in-progress investigation)

Written mid-Phase-2 because a real bug was found and context is running low. This
supplements `docs/STATUS.md` (still the source of truth for overall status/gates/
decisions) with exact live state for whoever continues — human, Claude, or Codex.
**Do not skip the investigation below or proceed to Phase 3 integration until it's
resolved and re-verified under `-race`.**

## Exact live git state (as of this writing)

| Worktree | Branch | HEAD | Pushed? | Working tree |
|---|---|---|---|---|
| `/home/josh/dev/6 Laptops/ForgeGrid` | `feature/qwen-assistant-v2` | `1c013ed` | yes, matches `origin` | clean |
| `/home/josh/dev/6 Laptops/ForgeGrid-hygiene` | `chore/project-hygiene` | `cd11050` (this handoff + STATUS.md edits are uncommitted on top — commit them next) | `cd11050` pushed; newer STATUS.md/HANDOFF edits not yet committed | `docs/STATUS.md` modified, `docs/HANDOFF-CURRENT.md` new, both about to be committed |
| `/home/josh/dev/6 Laptops/ForgeGrid-self-update-reliability` | `fix/self-update-reliability` | `33c767e` | yes, matches `origin` | **clean — the fix below has NOT been applied here yet** |
| `/home/josh/dev/6 Laptops/ForgeGrid-security-gates` | `fix/structured-execution-security-gates-v2` | `be08bcb` | yes | parked, untouched, out of scope this run (see `STATUS.md`) |

Temporary detached worktrees still on disk from Phase 2 validation (safe to remove once
this investigation is resolved and re-validated, or reuse them — they're at the same
pinned SHAs):
- `/tmp/fg-validate/self-update-reliability` @ `33c767e`
- `/tmp/fg-validate/qwen-assistant-v2` @ `f6c9914`
- `/tmp/fg-validate/qwen-history-973c048` @ `ae0359f`

Logs from the Phase 2 runs are at `/tmp/fg-validate/*.log` (test/race per branch).

Commits created this session (all pushed):
- `feature/qwen-assistant-v2`: `69cadf7` (doc pointers to STATUS.md, fleet-size
  correction), `1c013ed` (gofmt fix for `internal/coordinator/localllm_handlers.go`,
  `internal/localllm/localllm_extra_test.go` — whitespace only).
- `chore/project-hygiene` (new branch, created this session): `cd11050` (`docs/STATUS.md`,
  `CLAUDE.md`, hardened `.gitignore`, `tools/action1_llm_host.py`). A follow-up commit
  with the STATUS.md/HANDOFF updates below is about to be made.
- No commits yet on `fix/self-update-reliability` — the bug below is unfixed there.

## Phases complete / incomplete

- **Phase 0 (inventory):** done.
- **Phase 1 (preserve and organise):** done. `973c048` preserved two ways without a
  force-push (`origin/backup/973c048-persistent-history` = `ae0359f`, Codex's rebase;
  `origin/backup/973c048-persistent-history-original` = `973c048` literal). Verified: the
  only difference between them is the two roadmap docs already on `f6c9914`, nothing else
  — safe to bring `ae0359f` forward in Phase 3. Security-gates worktree assessed and
  parked (already fully pushed, unrelated workstream). 17 untracked files archived to
  `~/forgegrid-scratch-archive/2026-09-17/` (not deleted); one of them,
  `action1_proxy.py`, is a prototype that bypasses the reviewed `local_llm` backend via
  Action1 — deliberately not reused.
- **Phase 2 (baseline validation):** gofmt/vet/build/git-diff-check/cross-compile/
  govulncheck/`go test ./...` all green on all three branches (`fix/self-update-
  reliability` @ `33c767e`, `feature/qwen-assistant-v2` @ `f6c9914`, `973c048`-as-now @
  `ae0359f`). **`go test -race ./...` is where this stopped** — see below. Full detail
  and the pass/fail table is in `docs/STATUS.md`.
- **Phase 3 onward:** not started. Do not start integration (`integration/next`) until
  the race bug is fixed and `fix/self-update-reliability` is genuinely green under
  `-race`, confirmed with repeated runs (flaky bugs pass most runs — one green run proves
  nothing here; run at least 10x, see below).
- **History/search/security/mobile/remote-access work (Phases 5-10):** not started.

## The bug: correct the earlier (wrong) Phase 2 table entry

An earlier version of `docs/STATUS.md`'s Phase 2 table said `go test -race ./...` on
`fix/self-update-reliability` was "pass". **That was premature — it was recorded before
a full, honest re-run.** The correct state, found during that re-run:

```
cd /tmp/fg-validate/self-update-reliability && go test -race ./...
```

fails intermittently:

```
--- FAIL: TestRollbackConcurrencyRace (0.62s)
    updater_integration_test.go:264: Start() was executed 2 times, expected exactly 1
```

Reproduction: ran `go test -race -run TestRollbackConcurrencyRace -count=1
./internal/worker/...` five times in a row from `/tmp/fg-validate/self-update-reliability`
(pinned at `33c767e`): **failed twice, passed three times.** It's genuinely
timing-dependent, not a one-off fluke and not a sandbox limitation — rerun it yourself to
confirm before trusting any fix.

### Root cause (found, not yet fixed)

File: `internal/worker/updater.go`, the Phase 2/3 rollback-restart logic (roughly lines
556-672 at `33c767e`).

Phase 1 and Phase 2 claim tokens are protected correctly, by an atomic `os.Rename` (only
one renamer of a given source path can ever succeed — that's the actual mutual-exclusion
primitive; the `.lease` file next to each is just a liveness signal for staleness
detection, refreshed by the plain `startLeaseKeeper()`).

**Phase 3 (the restart-lease that's supposed to guarantee only one actor ever calls
`Start()`) has no equivalent atomic claim step.** Both call sites —

- line ~598-599 (the "verification timed out, retry" path) and
- line ~667-668 (the "token just consumed, start for the first time" path)

— do this unprotected sequence:

```go
if claimIsLive(restartLeasePath) {   // just a staleness check, not a claim
    continue
}
stopRestartLease := startLeaseKeeper(restartLeasePath)  // starts refreshing, doesn't "claim" atomically
startErr := GetLifecycle(localTx.LifecycleMode).Start(localTx)
```

`claimIsLive` only checks whether the lease file exists and how recently it was
modified — it is a check, not a claim. `startLeaseKeeper`'s first write is a plain
`os.WriteFile` (create-or-truncate), not an exclusive create. So when neither actor has
called `Start()` yet (no lease file exists at all), **both** actors can observe
"not live" and **both** proceed to call `Start()` — a classic check-then-act race, not
protected by anything atomic. This is exactly what the flaky failure shows: it only
happens when both actors' `verifyWaitDuration` timeouts land before either has created
the restart lease.

### Fix direction (sketched, not yet implemented or tested)

Give the restart lease the same atomic-claim treatment Phase 1/2 already have, instead of
a bare "startLeaseKeeper() + hope":

1. Add a claim variant that does its *first* write with
   `os.OpenFile(leasePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)` instead of plain
   `os.WriteFile`. `O_EXCL` create is atomic — only one caller can ever win it.
2. On `EEXIST`: check `claimIsLive` as before. If live, back off (existing behaviour). If
   stale, `os.Remove` then retry the exclusive create once; if that second attempt also
   loses (a third actor won it first), back off too — don't call `Start()`.
3. Only the goroutine whose exclusive create wins goes on to start the periodic refresh
   goroutine (reuse the existing ticker logic, just not the initial write) and call
   `Start()`.
4. Apply this at **both** call sites (line ~598 and line ~667) — either one can race
   against the other, not just against itself.
5. After the change: rerun `go test -race -run TestRollbackConcurrencyRace -count=20
   ./internal/worker/...` (not just once — flaky bugs pass most runs) and the full
   `go test -race ./...` suite, then update `docs/STATUS.md`'s Phase 2 table with the
   real result and remove the "FLAKY" flag.

**This has not been coded yet.** Do not assume it's done — check
`git -C "/home/josh/dev/6 Laptops/ForgeGrid-self-update-reliability" log --oneline -3`;
if it still shows `33c767e` at HEAD with nothing on top, the fix is still pending.

## Exact next action

```
cd "/home/josh/dev/6 Laptops/ForgeGrid-self-update-reliability"
# apply the fix described above to internal/worker/updater.go
gofmt -l . && go vet ./... && go build ./...
go test -race -run TestRollbackConcurrencyRace -count=20 ./internal/worker/...
go test -race ./...   # full suite
```

Then: commit on `fix/self-update-reliability` (message referencing this finding),
secret-scan, push, and update `docs/STATUS.md`'s Phase 2 table and this file's status
before continuing to Phase 3.

## Things that must not be repeated

- Don't force-push anything, ever (guardrail, and see the `973c048`/`ae0359f` naming
  episode in `STATUS.md` for why this matters in practice).
- Don't touch any fleet machine (Laptop01-11, JParrisDesktop) — no Action1, MeshCentral,
  SMB, update-queue calls. Nothing in this run has required that yet and nothing should
  until Josh sends "GO fleet" at Checkpoint A.
- Don't set up Tailscale/VPN/port-forwarding, don't use sudo without asking first.
- Don't treat a single green `-race` run as proof for a timing-sensitive test like this
  one — it passed 3 of 5 runs while still buggy. Use `-count=20` or more before trusting
  it.
- Checkpoint A message (when reached) must list exact SHAs of every branch head,
  `integration/next`, and the deployed build (per `AMENDMENT.txt`), then wait — change
  nothing while Josh runs a read-only Codex review against those SHAs. Codex running
  read-only at a checkpoint is expected, not a conflict; anything else active on this
  repo (another full Codex/Claude/Antigravity session actually editing) is a stop-and-ask.
- All standing guardrails (secrets list, live-systems rules, fleet size Laptop01-11 +
  JParrisDesktop) are in `CLAUDE.md` at the repo root (on `chore/project-hygiene`,
  merges into `integration/next` in Phase 3) — read that before doing anything
  infrastructure-touching.
