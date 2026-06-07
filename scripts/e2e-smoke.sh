#!/usr/bin/env bash
# e2e-smoke.sh — boot the real agezt daemon and exercise every core product
# surface end-to-end with the keyless echo mock. Asserts 0 panics, an intact
# journal hash-chain, and graceful shutdown. Exits non-zero on any failure.
#
# This is the runnable form of the ACCEPTANCE.md §7 (runtime/E2E) criterion:
# "çalıştırılabilir kanıt" — proof you can re-run, not a one-time manual check.
#
#   make e2e            # build binaries, then run this
#   scripts/e2e-smoke.sh /path/to/agezt /path/to/agt   # against prebuilt binaries
#
# CPU-capped (GOMAXPROCS=3) per project convention. Linux/macOS/git-bash.
set -uo pipefail

fail() { echo "E2E FAIL: $*" >&2; exit 1; }
ok()   { echo "  ok: $*"; }

AGEZT_BIN="${1:-}"; AGT_BIN="${2:-}"
TMP="$(mktemp -d)"; export GOMAXPROCS=3
# Kill the daemon (if still up) and remove the temp dir. The sleep lets the OS
# release the daemon's open journal handles before rm (Windows can't delete files
# held open); rm errors on the throwaway temp dir are non-fatal.
cleanup() { [ -n "${DPID:-}" ] && kill "$DPID" 2>/dev/null; sleep 1; rm -rf "$TMP" 2>/dev/null || true; }
trap cleanup EXIT

if [ -z "$AGEZT_BIN" ]; then
  echo "building binaries…"
  go build -o "$TMP/agezt" ./cmd/agezt || fail "build agezt"
  go build -o "$TMP/agt"   ./cmd/agt   || fail "build agt"
  AGEZT_BIN="$TMP/agezt"; AGT_BIN="$TMP/agt"
fi

export AGEZT_HOME="$TMP/home"; mkdir -p "$AGEZT_HOME"
"$AGT_BIN" catalog sync --local >/dev/null 2>&1 || fail "catalog sync"

PORT_API=18799; PORT_REST=18800
echo "starting daemon (demo echo, OpenAI + REST APIs)…"
AGEZT_DEMO_ECHO=1 AGEZT_API_ADDR=127.0.0.1:$PORT_API AGEZT_REST_ADDR=127.0.0.1:$PORT_REST \
  "$AGEZT_BIN" > "$AGEZT_HOME/daemon.log" 2>&1 &
DPID=$!
for _ in $(seq 1 50); do grep -q 'daemon ready' "$AGEZT_HOME/daemon.log" 2>/dev/null && break; sleep 0.3; done
grep -q 'daemon ready' "$AGEZT_HOME/daemon.log" || fail "daemon did not become ready"
sleep 1
ok "daemon ready"

OAI=$(grep -oE 'openai api.*Bearer [a-f0-9]+' "$AGEZT_HOME/daemon.log" | grep -oE '[a-f0-9]{64}')
REST=$(grep -oE 'rest api.*Bearer [a-f0-9]+' "$AGEZT_HOME/daemon.log" | grep -oE '[a-f0-9]{64}')
CHAT="http://127.0.0.1:$PORT_API/v1/chat/completions"
RUNS="http://127.0.0.1:$PORT_REST/api/v1/runs"

# 1. control-plane run loop
out=$("$AGT_BIN" run "smoke" -q 2>&1) || fail "agt run"
echo "$out" | grep -q '\[echo\]' || fail "agt run did not echo: $out"
ok "agt run (control-plane loop)"

# 2. doctor + journal chain
"$AGT_BIN" doctor 2>&1 | grep -q 'hash chain verified' || fail "doctor journal chain"
"$AGT_BIN" journal verify 2>&1 | grep -q '"ok": true' || fail "journal verify"
ok "doctor + journal chain verified"

# 3. OpenAI chat (non-stream)
curl -fs -H "Authorization: Bearer $OAI" -H 'Content-Type: application/json' \
  -d '{"model":"mock","messages":[{"role":"user","content":"hi"}]}' "$CHAT" \
  | grep -q '"content":"\[echo\]' || fail "openai chat non-stream"
ok "openai /v1/chat/completions"

# 4. OpenAI chat STREAMING must carry the content delta (M550 regression: a
#    non-streaming provider's answer must not be dropped from the stream).
sresp=$(curl -fsN -H "Authorization: Bearer $OAI" -H 'Content-Type: application/json' \
  -d '{"model":"mock","stream":true,"messages":[{"role":"user","content":"streamcheck"}]}' "$CHAT")
echo "$sresp" | grep -q '"content":"\[echo\]\\nstreamcheck"' \
  || fail "streaming dropped the answer (M550 regression):\n$sresp"
echo "$sresp" | grep -q 'data: \[DONE\]' || fail "streaming missing [DONE]"
ok "openai streaming carries content (M550 guard)"

# 5. native REST run
curl -fs -H "Authorization: Bearer $REST" -H 'Content-Type: application/json' \
  -d '{"intent":"rest smoke"}' "$RUNS" | grep -q '"status":"completed"' || fail "rest run"
ok "native REST /api/v1/runs"

# 6. auth + input-validation error paths
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
[ "$(code -H 'Authorization: Bearer WRONG' -d '{}' "$CHAT")" = "401" ] || fail "bad auth not 401"
[ "$(code -H "Authorization: Bearer $OAI" -H 'Content-Type: application/json' -d '{bad' "$CHAT")" = "400" ] || fail "malformed not 400"
big="$TMP/big.json"; { printf '{"model":"mock","messages":[{"role":"user","content":"'; head -c 17000000 /dev/zero | tr '\0' 'a'; printf '"}]}'; } > "$big"
[ "$(code -H "Authorization: Bearer $OAI" -H 'Content-Type: application/json' --data-binary @"$big" "$CHAT")" = "413" ] || fail "oversized not 413"
ok "error paths: 401 / 400 / 413"

# 7. concurrency: 10 simultaneous runs, all 200. NB: wait only on the curl PIDs —
# a bare `wait` would also block on the backgrounded daemon (DPID) forever.
cpids=""
for i in $(seq 1 10); do
  code -H "Authorization: Bearer $OAI" -H 'Content-Type: application/json' \
    -d "{\"model\":\"mock\",\"messages\":[{\"role\":\"user\",\"content\":\"c$i\"}]}" "$CHAT" >> "$TMP/codes" &
  cpids="$cpids $!"
done
wait $cpids
[ "$(tr -d '\n' < "$TMP/codes" | sed 's/200//g')" = "" ] || fail "concurrent runs not all 200: $(cat "$TMP/codes")"
ok "10 concurrent runs all 200"

# 8. halt → refuse → resume. Capture output first: a refused run exits non-zero,
# which under `pipefail` would mask a successful grep match in a pipeline.
"$AGT_BIN" halt --reason e2e >/dev/null 2>&1
halted_out=$("$AGT_BIN" run "during halt" -q 2>&1)
echo "$halted_out" | grep -q 'halted' || fail "halt did not refuse a run: $halted_out"
"$AGT_BIN" resume --reason e2e >/dev/null 2>&1
resumed_out=$("$AGT_BIN" run "after resume" -q 2>&1)
echo "$resumed_out" | grep -q '\[echo\]' || fail "resume did not restore runs: $resumed_out"
ok "halt → refuse → resume"

# 9. graceful shutdown + panic scan
"$AGT_BIN" shutdown >/dev/null 2>&1; sleep 1
kill -0 "$DPID" 2>/dev/null && fail "daemon did not exit on shutdown"
DPID=""
if grep -qiE 'panic|runtime error|nil pointer dereference' "$AGEZT_HOME/daemon.log"; then
  grep -iE 'panic|runtime error|nil pointer' "$AGEZT_HOME/daemon.log" | head; fail "panic in daemon log"
fi
ok "graceful shutdown, 0 panics"

echo "E2E SMOKE: PASS"
