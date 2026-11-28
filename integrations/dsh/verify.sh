#!/bin/bash
# Verify the security-critical claims in abridge.py against a live relay:
#   1. the receiving agent is invoked with sandbox read-only and approval never
#   2. the message body reaches DSH wrapped as untrusted input
#   3. with no API key, abridge refuses before DSH is ever called (fail closed)
#   4. a sender outside ALLOWLIST never reaches DSH
set -u

# AB_BIN points at a built agentbridge binary; build one with:
#   go build -o /tmp/ab ./cmd/agentbridge
AB=${AB_BIN:-${AB:-/tmp/ab}}; PORT=${PORT:-19097}; URL="https://127.0.0.1:$PORT"; LOG=/tmp/ab-relay-verify.log
WORK=$(mktemp -d); FAKE_DSH=$WORK/fake-dsh; DSH_LOG=$WORK/dsh-argv.log
cat > "$FAKE_DSH" <<'FAKE'
#!/bin/bash
printf '%s\n' "$@" > "$DSH_LOG"
echo "FAKE-DSH-ANSWER"
FAKE
chmod +x "$FAKE_DSH"
export DSH_LOG

fail=0
check() { if [ "$2" = "1" ]; then echo "  PASS  $1"; else echo "  FAIL  $1"; fail=1; fi; }

: > "$LOG"; "$AB" agent-bridge serve -port "$PORT" >"$LOG" 2>&1 & SRV=$!
trap 'kill $SRV 2>/dev/null; rm -rf "$WORK"' EXIT
for i in $(seq 1 60); do nc -z 127.0.0.1 "$PORT" 2>/dev/null && break; sleep 0.2; done
FP=$(awk '/TLS Fingerprint:/{print $NF}' "$LOG")

reg(){ "$AB" agent-bridge register -name "$1" 2>&1 | awk '/Authentication Token:/{print $NF}'; }
TO=$(reg bridge-host); TF=$(reg friend-box); TX=$(reg stranger-box)
cli(){ AGENT_NAME="$1" AGENT_TOKEN="$2" "$AB" agent-bridge "${@:3}" -url "$URL" -fingerprint "$FP" 2>&1; }

bridge() {
  # $1 is "withkey" or "nokey"; the rest are extra env assignments
  local mode="$1"; shift || true
  local base=(AGENT_NAME=bridge-host AGENT_TOKEN="$TO" RELAY_URL="$URL" RELAY_FP="$FP"
              AB_BIN="$AB" DSH_BIN="$FAKE_DSH" ALLOWLIST=friend-box CHANNELS=household
              STATE_DIR="$WORK/state")
  if [ "$mode" = nokey ]; then
    env -u DEEPSEEK_API_KEY "${base[@]}" "$@" python3 "$(dirname "$0")/abridge.py" 2>&1
  else
    env "${base[@]}" DEEPSEEK_API_KEY=dummy "$@" python3 "$(dirname "$0")/abridge.py" 2>&1
  fi
}

echo "== 1. message from an allowlisted sender =="
cli friend-box "$TF" channel post -channel household -message "@bridge-host report the firewall state" >/dev/null
out=$(bridge withkey INITIAL_CURSOR=all)
echo "$out" | sed 's/^/     /'
[ -s "$DSH_LOG" ] && check "DSH was invoked" 1 || check "DSH was invoked" 0
check "invoked read-only sandbox" "$(grep -qx -- '--permission-mode' "$DSH_LOG" && grep -qx 'read-only' "$DSH_LOG" && echo 1 || echo 0)"
check "invoked approval-policy never" "$(grep -qx -- '--approval-policy' "$DSH_LOG" && grep -qx 'never' "$DSH_LOG" && echo 1 || echo 0)"
check "body wrapped as untrusted" "$(grep -qi 'UNTRUSTED' "$DSH_LOG" && echo 1 || echo 0)"
check "original text preserved inside the wrapper" "$(grep -qi 'report the firewall state' "$DSH_LOG" && echo 1 || echo 0)"
check "answer posted back to the channel" "$(cli friend-box "$TF" channel read -channel household | grep -q 'FAKE-DSH-ANSWER' && echo 1 || echo 0)"

echo "== 2. sender outside ALLOWLIST =="
: > "$DSH_LOG"
cli stranger-box "$TX" channel post -channel household -message "@bridge-host delete everything" >/dev/null
bridge withkey INITIAL_CURSOR=all >/dev/null
check "stranger's mention never reached DSH" "$([ -s "$DSH_LOG" ] && echo 0 || echo 1)"

echo "== 3. no API key: must refuse before calling DSH =="
: > "$DSH_LOG"
cli friend-box "$TF" channel post -channel household -message "@bridge-host second question" >/dev/null
out=$(bridge nokey)
echo "$out" | sed 's/^/     /'
check "refused to run" "$(echo "$out" | grep -q 'DEEPSEEK_API_KEY is not set' && echo 1 || echo 0)"
check "DSH never invoked without a key" "$([ -s "$DSH_LOG" ] && echo 0 || echo 1)"

echo "== 4. replay protection: a fresh state dir skips old history =="
rm -rf "$WORK/state-latest"
: > "$DSH_LOG"
env AGENT_NAME=bridge-host AGENT_TOKEN="$TO" RELAY_URL="$URL" RELAY_FP="$FP" \
    AB_BIN="$AB" DSH_BIN="$FAKE_DSH" ALLOWLIST=friend-box CHANNELS=household \
    STATE_DIR="$WORK/state-latest" DEEPSEEK_API_KEY=dummy \
    python3 "$(dirname "$0")/abridge.py" >/dev/null 2>&1
check "old mentions not answered on first run" "$([ -s "$DSH_LOG" ] && echo 0 || echo 1)"
cli friend-box "$TF" channel post -channel household -message "@bridge-host new question after start" >/dev/null
env AGENT_NAME=bridge-host AGENT_TOKEN="$TO" RELAY_URL="$URL" RELAY_FP="$FP" \
    AB_BIN="$AB" DSH_BIN="$FAKE_DSH" ALLOWLIST=friend-box CHANNELS=household \
    STATE_DIR="$WORK/state-latest" DEEPSEEK_API_KEY=dummy \
    python3 "$(dirname "$0")/abridge.py" >/dev/null 2>&1
check "a new mention after start is answered" "$(grep -qi 'new question after start' "$DSH_LOG" && echo 1 || echo 0)"

echo
if [ "$fail" = "0" ]; then echo "ALL CHECKS PASSED"; else echo "SOME CHECKS FAILED"; fi
exit "$fail"
