# Acceptance Tests

## Core Requirements
1. **Single Executable**: Verify the Go binary can launch as Coordinator, Worker, or both.
2. **First-Run Wizard**: Verify successful prompting of node name, workspace, and mode selection.
3. **Hardware Detection**: Verify OS, CPU model, logical threads, RAM, and disk space are properly read and reported in both Windows and Linux.
4. **Auto-Discovery**: Verify a Worker automatically detects a Coordinator on the same LAN without manual IP entry.
5. **Pairing Security**: Verify a 6-digit code pairs a worker. Ensure mismatched codes reject pairing. Ensure the TLS fingerprint is visible.

## Project & Job Mechanics
6. **Manifest Parsing**: Verify `forgegrid.yaml` correctly triggers jobs, limits resources (e.g. `min_ram_gb`), and filters by OS.
7. **Mirror Sync**: Verify that modifying a file on Coordinator updates it on the Worker before the next job, without re-transmitting unchanged files.
8. **Godot Headless Build**: Verify a simple Godot headless export runs successfully and returns `build/**` artefacts.
9. **Log Streaming**: Ensure `stdout` and `stderr` from the job are visible in the Coordinator dashboard in near real-time.
10. **Cancellation & Retry**: Verify cancelling a running job terminates the child process cleanly. Verify retrying restarts it.
11. **Worker Constraints**: Ensure the "Very light worker" (Celeron N4500, 4GB RAM) does not get assigned heavy compilation jobs if `min_ram_gb: 8` is set.

## Resilience & Isolation
12. **Workspace Isolation**: Attempt directory traversal via job command (e.g., `cat ../../etc/passwd`) and ensure it is blocked/rejected.
13. **Worker Disconnect**: Disconnect a worker mid-job. Verify the Coordinator marks the job as failed or retries, and the dashboard reflects "Worker offline".
14. **Worker Reconnect**: Bring the worker back online. Ensure it reconnects seamlessly and is ready for new jobs.

## E2E Integration Suite
A Go test program must launch:
- 1 Coordinator (in-memory or tmp dir).
- 3 Simulated Workers.
- Inject 5 parallel jobs.
- Verify 1 fails as instructed.
- Verify 1 is cancelled successfully.
- Assert all artefacts are collected for the successful 3 jobs.

## Greptile security-gates review handoff — 28 October 2026

This is the existing repository acceptance register; the original 14 requirements above remain unchanged. Review of the existing [draft PR #13](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13) is limited to its diff and **does not certify the full codebase**. No code fix, test execution, release or deployment is claimed by this section.

- [ ] **FG-GR-001** [TRIAGE / HANDOFF PENDING MERGE] Complete the existing structured-execution security review and preserve line-level findings.
  - **Start/triage:** Poll Greptile run `35971251` (now `COMPLETED`) and its 11 original inline findings; do not trigger a second review. Read each original inline comment and compare it with the exact PR head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a` and current `main`.
  - **Evidence:** [PR #13](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13); branch `fix/structured-execution-security-gates-v2`; review `35971251`, `COMPLETED` 28 October 2026 with 11 findings (10 provider P1, 1 provider P2); consolidated into ten engineering tasks below. Severity is triaged independently where platform-specific repro is still needed.
  - **Dependencies:** Check whether another agent has claimed the PR/files; retain existing task identifiers. Respect `SECURITY.md` and `ACCEPTANCE_TESTS.md`; never include worker credentials or real network details in public task comments.
  - **Proposed minimal fix:** None before a source-validated finding; allocate distinct `FG-GR-002+` tasks only for genuinely actionable root causes, and avoid duplicating existing acceptance requirements.
  - **Acceptance:** Terminal Greptile status, exact reviewed SHA, every actionable finding mapped to a stable open task with source-comment URL, reproduction, smallest fix, negative security and regression tests, and re-review gate. Do not check this box until all findings are handed off and the queue is reconciled.
  - **Closure:** Link source commit, PR, Greptile addressed state and passing test evidence when available. This is a documentation checkpoint, not a completed code review.

### Completed review findings — independently source-triaged, fixes OPEN

**Review:** Greptile `35971251` `COMPLETED`, PR #13 head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`; 11 inline comments, all unaddressed at review time. The ten tasks below combine the two output-capture comments into one root cause. Greptile labelled 10 P1 and 1 P2; environment-dependent launcher findings are triaged P2 pending reproduction. Static source inspection confirms the described control-flow patterns; **no runtime tests were executed**. Prioritise data migration, bounded process output and state-machine integrity before merge.

- [ ] **FG-GR-002** [P1] Migrate persisted job state before decoding new schema.
  - **Start/triage:** Use a sanitised legacy `jobs.json` fixture containing lowercase statuses and string-array logs; load on the reviewed head and verify error handling and queued-job continuity. Check whether another agent has already fixed or claimed the affected files.
  - **Evidence:** `internal/models/models.go:24-51`, `internal/store/store.go:35-45`; Greptile review `35971251`; [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794522); reviewed head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`. Source-level risk corroborated; reproduction still required.
  - **Dependencies:** First data-integrity gate before PR #13 merge; coordinate any storage changes with FG-GR-004 and FG-GR-007. Keep task ownership narrow; do not copy credentials or private machine details.
  - **Proposed minimal fix:** Introduce versioned migration/compatibility decoding before replacing stored data; preserve status, logs and execution settings, and propagate decode failures rather than silently ignoring them.
  - **Acceptance:** Golden-file migration tests for legacy, current, malformed and partial JSON; verify queued/running jobs and logs survive restart, with backup/rollback path and no real job data in fixtures. Run focused `go test ./...` and project CI on the repaired PR head, verify Greptile re-review and addressed status; do not mark complete without evidence.
  - **Closure:** Record fix commit/PR, commands and exit codes, platform evidence and reviewer resolution; leave unchecked until verified.

- [ ] **FG-GR-003** [P1] Drain stdout and stderr concurrently with bounded capture.
  - **Start/triage:** Run a synthetic child that fills stderr while keeping stdout open, then one that writes output above the cap; measure completion, memory and output truncation. Check whether another agent has already fixed or claimed the affected files.
  - **Evidence:** `internal/execution/executor.go:65-104`; Greptile review `35971251`; [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794497) and [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794502); reviewed head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`. Source-level risk corroborated; reproduction still required.
  - **Dependencies:** Fix together with FG-GR-005 process lifecycle and FG-GR-008 upload limits; two Greptile comments share this output-collection root cause. Keep task ownership narrow; do not copy credentials or private machine details.
  - **Proposed minimal fix:** Drain both pipes concurrently into a shared bounded collector that continues consuming after the limit, without unbounded `io.ReadAll`; coordinate shutdown with FG-GR-005.
  - **Acceptance:** Timeout-controlled dual-pipe stress, >1 MiB output and low-memory tests; confirm no hang, bounded resident memory, useful truncated logs and exit status. Run focused `go test ./...` and project CI on the repaired PR head, verify Greptile re-review and addressed status; do not mark complete without evidence.
  - **Closure:** Record fix commit/PR, commands and exit codes, platform evidence and reviewer resolution; leave unchecked until verified.

- [ ] **FG-GR-004** [P1] Make cancellation a complete state transition.
  - **Start/triage:** Cancel one pending, one claimed/running and one terminal job; inspect state after worker polls and restart. Check whether another agent has already fixed or claimed the affected files.
  - **Evidence:** `internal/coordinator/handlers.go:226-245,257-288`, `internal/worker/worker.go:315-351`; Greptile review `35971251`; [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794512); reviewed head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`. Source-level risk corroborated; reproduction still required.
  - **Dependencies:** Coordinate with FG-GR-005 and FG-GR-007; preserve backward compatibility under FG-GR-002. Keep task ownership narrow; do not copy credentials or private machine details.
  - **Proposed minimal fix:** Immediately terminalise unclaimed cancellations; deliver active cancel requests to the assigned worker and acknowledge terminal cancellation without overwriting prior completed jobs.
  - **Acceptance:** Coordinator/worker integration state matrix, duplicate cancels, worker disconnect, cancel-versus-complete race and restart; assert process termination and no stuck `CANCEL_REQUESTED`. Run focused `go test ./...` and project CI on the repaired PR head, verify Greptile re-review and addressed status; do not mark complete without evidence.
  - **Closure:** Record fix commit/PR, commands and exit codes, platform evidence and reviewer resolution; leave unchecked until verified.

- [ ] **FG-GR-005** [P1] Reap every started process after timeout or setup failure.
  - **Start/triage:** Repeat short-deadline jobs and inspect child process handles/zombies; simulate Windows job-object setup failure after `Start`. Check whether another agent has already fixed or claimed the affected files.
  - **Evidence:** `internal/execution/executor.go:90-107`, `internal/execution/executor_windows.go`; Greptile review `35971251`; [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794537); reviewed head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`. Source-level risk corroborated; reproduction still required.
  - **Dependencies:** Coordinate with FG-GR-003 bounded output and FG-GR-004 cancellation; do not merge before lifecycle tests. Keep task ownership narrow; do not copy credentials or private machine details.
  - **Proposed minimal fix:** Ensure each successful `Start` reaches exactly one `Wait` after kill/exit and pipe-drain completion; close Windows job handles on every path.
  - **Acceptance:** Linux child/grandchild cancellation, process-count and descriptor-leak tests; Windows setup-failure/handle tests; repeat hundreds of timeouts under resource limits. Run focused `go test ./...` and project CI on the repaired PR head, verify Greptile re-review and addressed status; do not mark complete without evidence.
  - **Closure:** Record fix commit/PR, commands and exit codes, platform evidence and reviewer resolution; leave unchecked until verified.

- [ ] **FG-GR-006** [P1] Reject invalid worker challenge as failed work.
  - **Start/triage:** Submit a completed test-job update with an incorrect challenge digest; inspect persisted job status and dashboard response. Check whether another agent has already fixed or claimed the affected files.
  - **Evidence:** `internal/coordinator/handlers.go:347-365`; Greptile review `35971251`; [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794546); reviewed head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`. Source-level risk corroborated; reproduction still required.
  - **Dependencies:** Coordinate state-transition logic with FG-GR-004; no test secrets in logs. Keep task ownership narrow; do not copy credentials or private machine details.
  - **Proposed minimal fix:** Set job status to `FAILED` atomically on challenge mismatch, never `COMPLETED`; ensure invalid result cannot be overwritten by a later success update.
  - **Acceptance:** Valid/invalid challenge, replay and duplicate terminal updates, restart persistence and dashboard status regression. Run focused `go test ./...` and project CI on the repaired PR head, verify Greptile re-review and addressed status; do not mark complete without evidence.
  - **Closure:** Record fix commit/PR, commands and exit codes, platform evidence and reviewer resolution; leave unchecked until verified.

- [ ] **FG-GR-007** [P1] Recover jobs whose claim acknowledgement is lost.
  - **Start/triage:** Drop the claim HTTP response after coordinator persistence, then retry the same worker and inspect job liveness. Check whether another agent has already fixed or claimed the affected files.
  - **Evidence:** `internal/coordinator/handlers.go:289-318`, `internal/worker/worker.go:353-384`; Greptile review `35971251`; [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794555); reviewed head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`. Source-level risk corroborated; reproduction still required.
  - **Dependencies:** Coordinate with FG-GR-002 state migration and FG-GR-004 cancellation. Keep task ownership narrow; do not copy credentials or private machine details.
  - **Proposed minimal fix:** Add idempotent same-worker claim recovery or an expiring lease with safe reassignment, preserving attempt identity and rejecting concurrent workers.
  - **Acceptance:** Lost response, malformed reply, worker crash/restart, lease expiry, concurrent claim, duplicate execution and terminal-state tests. Run focused `go test ./...` and project CI on the repaired PR head, verify Greptile re-review and addressed status; do not mark complete without evidence.
  - **Closure:** Record fix commit/PR, commands and exit codes, platform evidence and reviewer resolution; leave unchecked until verified.

- [ ] **FG-GR-008** [P1] Ensure final job status survives oversized output.
  - **Start/triage:** Generate 60 KiB output and verify base64 JSON exceeds the 65,536-byte request limit while the worker ignores non-2xx response. Check whether another agent has already fixed or claimed the affected files.
  - **Evidence:** `internal/worker/worker.go:386-402,478`, `internal/coordinator/handlers.go:257-258,367-379`; Greptile review `35971251`; [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794567); reviewed head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`. Source-level risk corroborated; reproduction still required.
  - **Dependencies:** Integrate with FG-GR-003 bounded capture and FG-GR-007 delivery/retry semantics. Keep task ownership narrow; do not copy credentials or private machine details.
  - **Proposed minimal fix:** Bound/chunk log uploads below request limit, check HTTP response codes, and send a separate small final status reliably; preserve log sequence semantics.
  - **Acceptance:** Boundary payloads 0/48/60/100 KiB, rejection/retry, deduplication, network drop and final terminal status tests. Run focused `go test ./...` and project CI on the repaired PR head, verify Greptile re-review and addressed status; do not mark complete without evidence.
  - **Closure:** Record fix commit/PR, commands and exit codes, platform evidence and reviewer resolution; leave unchecked until verified.

- [ ] **FG-GR-009** [P2 / ENVIRONMENT REPRO REQUIRED (Greptile P1)] Preserve trusted tool resolution for NodeBuild.
  - **Start/triage:** On an isolated worker with Node installed in a user toolchain, run the pinned npm launcher and confirm whether its shebang cannot resolve node without `PATH`. Check whether another agent has already fixed or claimed the affected files.
  - **Evidence:** `internal/execution/executor.go:29-45`, `internal/execution/profile.go:30-36`; Greptile review `35971251`; [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794528); reviewed head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`. Source-level risk corroborated; reproduction still required.
  - **Dependencies:** Coordinate launcher design with FG-GR-010; severity may increase if core NodeBuild is universally blocked. Keep task ownership narrow; do not copy credentials or private machine details.
  - **Proposed minimal fix:** Pass only an explicitly trusted host `PATH` or pin child runtimes; continue rejecting manifest-supplied environment overrides.
  - **Acceptance:** Linux nvm-style Node and system Node builds, absent runtime, PATH spoof attempt, and no coordinator-controlled env injection. Run focused `go test ./...` and project CI on the repaired PR head, verify Greptile re-review and addressed status; do not mark complete without evidence.
  - **Closure:** Record fix commit/PR, commands and exit codes, platform evidence and reviewer resolution; leave unchecked until verified.

- [ ] **FG-GR-010** [P2 / WINDOWS REPRO REQUIRED (Greptile P1)] Launch Windows NodeBuild without executing a batch file as an executable.
  - **Start/triage:** On a Windows CI worker where `LookPath("npm")` returns `npm.cmd`, run NodeBuild and capture the exact CreateProcess error. Check whether another agent has already fixed or claimed the affected files.
  - **Evidence:** `internal/execution/profile.go:30-34,46-66`, `internal/execution/executor_windows.go`; Greptile review `35971251`; [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794581); reviewed head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`. Source-level risk corroborated; reproduction still required.
  - **Dependencies:** Coordinate trusted executable and PATH policy with FG-GR-009. Keep task ownership narrow; do not copy credentials or private machine details.
  - **Proposed minimal fix:** Use pinned native `node.exe` with a known npm CLI entry script or another explicit safe Windows launcher; do not invoke an untrusted shell command.
  - **Acceptance:** Windows npm.cmd resolution, native node launcher, paths with spaces, malicious argument/flag and cancellation regression. Run focused `go test ./...` and project CI on the repaired PR head, verify Greptile re-review and addressed status; do not mark complete without evidence.
  - **Closure:** Record fix commit/PR, commands and exit codes, platform evidence and reviewer resolution; leave unchecked until verified.

- [ ] **FG-GR-011** [P2] Restore process-tree cancellation regression coverage.
  - **Start/triage:** Run the current sleeper test with termination disabled in a controlled test double and confirm it still passes. Check whether another agent has already fixed or claimed the affected files.
  - **Evidence:** `internal/execution/executor_test.go:10-59`; Greptile review `35971251`; [inline comment](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13#discussion_r4394794590); reviewed head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a`. Source-level risk corroborated; reproduction still required.
  - **Dependencies:** Implement alongside FG-GR-005, then enforce in CI. Keep task ownership narrow; do not copy credentials or private machine details.
  - **Proposed minimal fix:** Add observable child/grandchild liveness checks, elapsed-time assertions and cleanup verification, not just an error-substring assertion.
  - **Acceptance:** Linux and Windows process-tree termination, prompt cancellation, timeout and no leaked processes; fail the test when termination is deliberately disabled. Run focused `go test ./...` and project CI on the repaired PR head, verify Greptile re-review and addressed status; do not mark complete without evidence.
  - **Closure:** Record fix commit/PR, commands and exit codes, platform evidence and reviewer resolution; leave unchecked until verified.

