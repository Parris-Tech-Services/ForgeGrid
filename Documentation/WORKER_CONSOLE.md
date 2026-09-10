# ForgeGrid Worker Console

The Worker Console is the per-computer monitor for ForgeGrid workers. It should
answer two questions quickly:

1. Is this computer healthy and available for work?
2. What is this computer doing right now, and why?

The console is intentionally ForgeGrid-focused. It records worker heartbeats,
job execution, toolchain health, service events, updates, connectivity changes,
and explicit diagnostics. It does not capture every user process by default.

## Planned capabilities

### 1. Live activity timeline

Show heartbeats, job transitions, tool executions, service restarts, updates,
errors, and connectivity changes as structured events with timestamps, severity,
source, worker ID, and job ID.

### 2. Resource dashboard

Report CPU load, memory, workspace disk, temperature when available, network
throughput, battery state, and uptime. Thresholds should mark a worker degraded
before scheduling work to it.

### 3. Process and job correlation

For each running job, show the process tree, elapsed time, CPU and memory use,
child-process failures, and exit code. Process details are collected only for
ForgeGrid-owned jobs.

### 4. Toolchain health

Display tool name, version, executable path, account visibility, and last
successful probe for Git, Python, Node/npm, Go, GitHub CLI, Rust, Java, .NET,
Godot, and configured agent tools.

### 5. Capability drift alerts

Compare configured capabilities with detected tools. Report missing tools,
changed paths, version changes, and tools unavailable to the ForgeGrid service
account.

### 6. Worker self-test

Provide a safe test that validates coordinator connectivity, repository access,
file creation, compilation or test execution, artifact collection, and result
reporting. Record each stage and its duration.

### 7. Live log viewer

Reuse the coordinator's job-log stream for a filtered worker timeline. Support
severity, job, subsystem, and time filters, pause/resume, search, download, and
error context. Logs must redact credentials and boundedly retain data.

### 8. Queue and workload controls

Show queued, running, completed, failed, and cancelled jobs. Support drain mode
so a worker finishes its current job without accepting another one.

### 9. Maintenance actions

Offer audited actions for capability refresh, worker restart, PATH repair,
ForgeGrid update, approved tool installation, diagnostics, and connectivity
checks. Every action requires confirmation and produces an event.

### 10. Readiness certificate

Compute `Ready`, `Limited`, or `Offline` from service health, coordinator
reachability, toolchain probes, resources, repository access, and a successful
test job. Export the result as JSON for scheduling and fleet audits.

## Telemetry contract

Heartbeat telemetry must remain backward-compatible. New fields are optional and
must be safe to omit when an operating system does not expose a measurement.
The initial fields are:

```json
{
  "cpu_percent": 12.4,
  "uptime_seconds": 86400,
  "battery_percent": 78,
  "battery_plugged": true,
  "active_job_count": 1,
  "worker_health": "ready"
}
```

Values are observations, not security claims. The coordinator should treat
stale heartbeats as unknown and should never schedule solely from a client-
reported readiness value.

## Privacy and retention

- Capture ForgeGrid activity completely enough to diagnose jobs.
- Do not record arbitrary user processes by default.
- Process telemetry is limited to ForgeGrid-owned job trees or an explicit
  diagnostic window.
- Redact tokens, secrets, environment values, and credential-helper output.
- Bound local and coordinator event retention, with export available for audits.

## Rollout order

1. Add optional heartbeat telemetry and coordinator persistence.
2. Add the worker health and toolchain panels to the existing dashboard.
3. Add the worker self-test and readiness certificate.
4. Add audited maintenance actions and drain controls.
5. Package the verified binary and roll it out through the existing update
   policy, beginning with a canary worker.

## Deployment gate

A worker is not considered ready merely because its Windows service is running.
The deployment gate requires:

- current heartbeat within the freshness window;
- coordinator connectivity;
- ForgeGrid service running;
- required capabilities detected from the service account;
- sufficient RAM and workspace disk;
- successful built-in test job;
- no unresolved capability drift.