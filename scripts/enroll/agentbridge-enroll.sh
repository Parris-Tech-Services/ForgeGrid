#!/bin/bash
# Enrol this machine as an AgentBridge agent (macOS and Linux).
#
# Prerequisite: someone with access to the relay host has already run
#   agent-bridge register -name <this-machine>
# and given you the one-time token out of band.
#
# The token is read with hidden input and piped to the client, so it is never
# echoed, never stored in a shell history entry, and never written anywhere
# except the client config file (0600).
set -eu

AB_BIN="${AB_BIN:-forgegrid}"

printf 'Relay URL [https://relay.lan:9090]: '
read -r RELAY_URL
RELAY_URL="${RELAY_URL:-https://relay.lan:9090}"

printf 'Relay TLS fingerprint: '
read -r RELAY_FP

printf 'Agent name for this machine: '
read -r AGENT_NAME

printf 'Agent token (hidden): '
stty -echo 2>/dev/null || true
read -r AGENT_TOKEN
stty echo 2>/dev/null || true
printf '\n'

if [ -z "$RELAY_FP" ] || [ -z "$AGENT_NAME" ] || [ -z "$AGENT_TOKEN" ]; then
  echo "fingerprint, agent name and token are all required" >&2
  exit 1
fi

printf '%s' "$AGENT_TOKEN" | "$AB_BIN" agent-bridge configure-client \
  -name "$AGENT_NAME" -url "$RELAY_URL" -fingerprint "$RELAY_FP" -token-stdin

unset AGENT_TOKEN

echo
echo "Enrolled as $AGENT_NAME."
echo "Verify with:"
echo "  $AB_BIN agent-bridge channel read -channel household -limit 5"
