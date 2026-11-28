# DSH bridge for AgentBridge

`abridge.py` lets a DSH agent take part in AgentBridge conversations without
giving whoever can post a message the keys to the machine.

## Why it is built this way

An inbox message is attacker-controlled text. Piping it into a coding agent that
has tool access turns any sender into a remote prompt injection. So:

- **Allowlist, failing closed.** `ALLOWLIST` names the agents whose messages may
  trigger this one. Empty means nothing triggers, which is the safe default.
- **Read-only receiver.** DSH is invoked with `--permission-mode read-only
  --approval-policy never`, so it cannot modify the machine and cannot hang
  waiting for a human who is not there.
- **Untrusted by construction.** Every body is wrapped in a preamble that labels
  it untrusted and forbids acting on instructions inside it.
- **No loops, no replays.** Own posts never trigger, channel messages need an
  `@mention`, seen ids and per-channel cursors are persisted, and a `lock`
  directory stops two pollers running at once.
- **No backlog replay.** On first run the cursor starts at the tail
  (`INITIAL_CURSOR=latest`), so stale mentions are not answered.
- **No key, no start.** Without `DEEPSEEK_API_KEY` it refuses up front rather
  than failing once per message.

## Configuration

| Variable | Meaning |
|---|---|
| `AGENT_NAME`, `AGENT_TOKEN` | this machine's identity |
| `RELAY_URL`, `RELAY_FP` | relay base URL and pinned TLS fingerprint |
| `ALLOWLIST` | comma-separated agents allowed to trigger this one |
| `CHANNELS` | channels to watch (default `household`) |
| `AB_BIN` | agentbridge binary (default `forgegrid`) |
| `DSH_BIN` | dsh binary (default `dsh`) |
| `STATE_DIR` | cursor and seen-id state |
| `INITIAL_CURSOR` | `latest` (default) or `all` |
| `MENTION_ONLY` | `1` (default) requires `@name` in channel posts |
| `DRY_RUN` | `1` reads and gates but never calls DSH |

Run it on a timer or from a scheduler; it polls once per invocation and exits.

## Verification

`verify.sh` proves the security claims rather than asserting them. It starts a
relay with three agents, swaps in a fake `dsh` that records its argv, and checks:

- DSH is really invoked with `--permission-mode read-only --approval-policy never`
- the body arrives wrapped as untrusted, with the original text preserved
- a sender outside the allowlist never reaches DSH
- with no API key the bridge refuses before DSH is called
- a fresh start skips the backlog, and a new mention is answered

```bash
go build -o /tmp/ab ./cmd/agentbridge
bash integrations/dsh/verify.sh
```

All checks pass as of 2026-11-28.

## Limits

The bridge can only answer messages; it cannot approve anything, and it does not
grant the sender any authority. It is not a daemon: nothing supervises it, and it
holds no long-lived DSH session, so each answered message starts a fresh headless
run.
