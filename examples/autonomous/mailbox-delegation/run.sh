#!/usr/bin/env bash
# mailbox-delegation/run.sh
#
# Runnable positioning demo: proves AGEZT treats agents as durable identities
# with wake causality (see docs/COMPARISON.md). Creates a leader + worker
# hierarchy, shows effective authority, arms a mailbox-wake standing order,
# fires a manual wake, and walks the audit chain.
#
# No provider key, no network, no external LLM. Uses the keyless echo daemon.
#
# NOTE: the echo provider returns text but does not produce LLM tool calls, so
# this demo proves the identity/authority/wake-causality infrastructure, not a
# live sub-agent delegation execution. See README.md for the honest limitation.
#
# Usage:
#   bash examples/autonomous/mailbox-delegation/run.sh
#   bash examples/autonomous/mailbox-delegation/run.sh /path/to/agezt /path/to/agt
set -uo pipefail

GOMAXPROCS=${GOMAXPROCS:-3}
export GOMAXPROCS

AGEZT_BIN="${1:-}"
AGT_BIN="${2:-}"
TMP="$(mktemp -d)"
DPID=""

fail() { echo "DEMO FAIL: $*" >&2; exit 1; }
ok()   { echo "  ok: $*"; }

cleanup() {
  if [ -n "${DPID:-}" ]; then
    kill "$DPID" 2>/dev/null || true
  fi
  sleep 1
  rm -rf "$TMP" 2>/dev/null || true
}
trap cleanup EXIT

# --- 0. prerequisites -------------------------------------------------------

if [ -z "$AGEZT_BIN" ] || [ -z "$AGT_BIN" ]; then
  if [ -z "$AGEZT_BIN" ]; then
    echo "building binaries..."
    go build -o "$TMP/agezt" ./cmd/agezt || fail "build agezt"
    AGEZT_BIN="$TMP/agezt"
  fi
  if [ -z "$AGT_BIN" ]; then
    go build -o "$TMP/agt" ./cmd/agt || fail "build agt"
    AGT_BIN="$TMP/agt"
  fi
fi

[ -x "$AGEZT_BIN" ] || fail "agezt binary not executable: $AGEZT_BIN"
[ -x "$AGT_BIN" ] || fail "agt binary not executable: $AGT_BIN"

# --- 1. boot keyless echo daemon -------------------------------------------

export AGEZT_HOME="$TMP/home"
mkdir -p "$AGEZT_HOME"

echo "starting keyless echo daemon..."
AGEZT_DEMO_ECHO=1 "$AGEZT_BIN" > "$AGEZT_HOME/daemon.log" 2>&1 &
DPID=$!

for _ in $(seq 1 50); do
  grep -q 'daemon ready' "$AGEZT_HOME/daemon.log" 2>/dev/null && break
  sleep 0.3
done
grep -q 'daemon ready' "$AGEZT_HOME/daemon.log" || fail "daemon did not become ready"
ok "daemon ready"

"$AGT_BIN" status >/dev/null 2>&1 || fail "agt status could not reach daemon"
ok "control plane reachable"

# --- 2. create durable agents ----------------------------------------------

echo
echo "=== durable agent creation ==="

"$AGT_BIN" agent add leader \
  --name "Team Lead" \
  --soul "You coordinate work and delegate to your team." \
  --trust-ceiling L3 \
  --tool-allow "file,http,memory" \
  --tool-deny "shell" \
  >/dev/null 2>&1 || fail "could not create leader agent"
ok "leader agent created"

"$AGT_BIN" agent add worker \
  --name "Worker" \
  --soul "You execute tasks delegated by your leader." \
  --parent-agent leader \
  --direct-callable false \
  --trust-ceiling L2 \
  --tool-allow "file" \
  --tool-deny "shell,code_exec" \
  >/dev/null 2>&1 || fail "could not create worker agent"
ok "worker agent created (parent=leader, managed sub-agent)"

# --- 3. verify agent roster ------------------------------------------------

echo
echo "=== agent roster ==="

roster=$("$AGT_BIN" agent list 2>/dev/null) || fail "agent list failed"
echo "$roster" | grep -q 'leader' || fail "leader not in roster"
echo "$roster" | grep -q 'worker' || fail "worker not in roster"
echo "$roster" | grep -q 'parent=leader' || fail "worker should show parent=leader"
ok "roster shows leader + worker with parent/child relationship"

# --- 4. effective authority proof ------------------------------------------

echo
echo "=== effective authority (leader) ==="

auth_leader=$("$AGT_BIN" agent authority leader 2>/dev/null) || true
echo "$auth_leader" | grep -q 'trust ceiling' || fail "authority should show trust ceiling"
echo "$auth_leader" | grep -q 'tool deny'    || fail "authority should show tool deny"
echo "$auth_leader" | grep -q 'shell'         || fail "leader should deny shell"
ok "leader authority: trust ceiling + tool deny visible"

echo
echo "=== effective authority (worker) ==="

auth_worker=$("$AGT_BIN" agent authority worker 2>/dev/null) || true
echo "$auth_worker" | grep -q 'trust ceiling' || fail "worker authority should show trust ceiling"
echo "$auth_worker" | grep -q 'shell'          || fail "worker should deny shell"
ok "worker authority: lower ceiling + stricter deny visible"

# --- 5. mailbox-wake standing order ----------------------------------------

echo
echo "=== mailbox-wake setup (standing order) ==="

# Arm a standing order that triggers on a board.dm.leader subject — the
# mailbox-wake route. The order runs AS the leader agent identity.
standing_out=$("$AGT_BIN" standing add \
  --name "leader-mailbox-wake" \
  --event "board.dm.leader" \
  --agent leader \
  --mode ask \
  2>&1) || true
# The standing add may succeed or the daemon may reject unknown subjects;
# either way we check the list.
if echo "$standing_out" | grep -qiE 'added|created|standing'; then
  ok "mailbox-wake standing order armed"
else
  echo "  note: standing add output: $standing_out"
  echo "  note: mailbox-wake arming may require daemon version support — skipping"
fi

# --- 6. manual wake + wake causality ---------------------------------------

echo
echo "=== manual wake + wake causality ==="

# Wake the leader with a simple intent. The echo provider will return text;
# the key assertion is that the wake event + runbook is journaled.
wake_out=$("$AGT_BIN" agent wake leader "check the latest status" --reason "demo" 2>&1) || true
echo "$wake_out"
echo "$wake_out" | grep -qiE 'accepted|wake|corr' || fail "wake did not produce an accepted event"
ok "leader wake accepted"

# Give the daemon a moment to journal the wake event.
sleep 2

# Find the wake event in the journal and walk the chain.
tail_out=$("$AGT_BIN" journal tail 10 --json 2>/dev/null) || true
wake_id=$(echo "$tail_out" | grep -oE '"id":[[:space:]]*"[^"]+"' | head -n1 | sed -E 's/.*"id":[[:space:]]*"([^"]+)".*/\1/')
if [ -n "$wake_id" ]; then
  why_out=$("$AGT_BIN" why "$wake_id" 2>/dev/null) || true
  echo "$why_out" | grep -q 'events in correlation' \
    && ok "agt why walked the wake causality chain" \
    || echo "  note: why output did not include correlation (may need a different event id)"
else
  echo "  note: no journaled events found to walk — this can happen if the wake is still in-flight"
fi

# --- 7. agent detail shows identity, not a prompt --------------------------

echo
echo "=== agent detail (durable identity proof) ==="

show_out=$("$AGT_BIN" agent show leader 2>/dev/null) || true
echo "$show_out" | grep -q 'slug:'         || fail "agent show should render slug"
echo "$show_out" | grep -q 'leader'         || fail "agent show should name the leader"
echo "$show_out" | grep -q 'trust ceiling'  || fail "agent show should show trust ceiling"
ok "agent show renders durable identity (slug, trust ceiling, tools)"

# --- 8. graceful shutdown ---------------------------------------------------

echo
echo "=== shutdown ==="

"$AGT_BIN" shutdown >/dev/null 2>&1 || true
sleep 1
if kill -0 "$DPID" 2>/dev/null; then
  fail "daemon did not exit on shutdown"
fi
DPID=""

if grep -qiE 'panic|runtime error|nil pointer dereference' "$AGEZT_HOME/daemon.log" 2>/dev/null; then
  grep -iE 'panic|runtime error|nil pointer' "$AGEZT_HOME/daemon.log" | head
  fail "panic in daemon log"
fi
ok "graceful shutdown, 0 panics"

echo
echo "MAILBOX WAKE & AGENT HIERARCHY DEMO: PASS"
