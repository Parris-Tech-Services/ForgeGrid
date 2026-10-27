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

- [ ] **FG-GR-001** [TRIAGE / REVIEW RUNNING] Complete the existing structured-execution security review and preserve line-level findings.
  - **Start/triage:** Poll Greptile run `35971251` to `COMPLETED` or `FAILED`; do not trigger a second review. Read each original inline comment and compare it with the exact PR head `be08bcb1b6873d6cfa3e09d80b77c1ad4f8e4a1a` and current `main`.
  - **Evidence:** [PR #13](https://github.com/Parris-Tech-Services/ForgeGrid/pull/13); branch `fix/structured-execution-security-gates-v2`; review `35971251`, last observed `REVIEWING_FILES`. Review findings and their severity are **not yet known**.
  - **Dependencies:** Check whether another agent has claimed the PR/files; retain existing task identifiers. Respect `SECURITY.md` and `ACCEPTANCE_TESTS.md`; never include worker credentials or real network details in public task comments.
  - **Proposed minimal fix:** None before a source-validated finding; allocate distinct `FG-GR-002+` tasks only for genuinely actionable root causes, and avoid duplicating existing acceptance requirements.
  - **Acceptance:** Terminal Greptile status, exact reviewed SHA, every actionable finding mapped to a stable open task with source-comment URL, reproduction, smallest fix, negative security and regression tests, and re-review gate. Do not check this box until all findings are handed off and the queue is reconciled.
  - **Closure:** Link source commit, PR, Greptile addressed state and passing test evidence when available. This is a documentation checkpoint, not a completed code review.
