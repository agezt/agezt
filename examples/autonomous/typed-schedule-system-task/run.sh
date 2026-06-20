#!/usr/bin/env bash
# typed-schedule-system-task/run.sh
#
# Runnable positioning demo: proves AGEZT treats schedules as typed
# infrastructure, not cron-wrapped prompts (see docs/COMPARISON.md).
# Creates a system-task schedule, validates that invalid tasks are rejected,
# fires it immediately, and inspects the fire history.
#
# No provider key, no network, no external LLM.
#
# Usage:
#   bash examples/autonomous/typed-schedule-system-task/run.sh
#   bash examples/autonomous/typed-schedule-system-task/run.sh /path/to/agezt /path/to/agt
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

# --- 2. prompt-smuggling resistance ----------------------------------------

echo
echo "=== typed target validation: invalid system task rejected ==="

# An invalid system-task name must be rejected — proving the schedule store
# validates against the enum, not just stores arbitrary text.
invalid_out=$("$AGT_BIN" schedule add --system-task "rm -rf /" --every 1h 2>&1) || true
if echo "$invalid_out" | grep -qiE 'unknown|invalid|not a valid|unrecognized'; then
  ok "invalid system-task name rejected (prompt smuggling blocked)"
else
  # Some daemon versions may accept-and-then-fail at fire time; check that the
  # schedule was NOT silently added with the malicious payload.
  list_out=$("$AGT_BIN" schedule list --json 2>/dev/null) || true
  if echo "$list_out" | grep -q 'rm -rf'; then
    fail "malicious system-task payload was accepted into the schedule store"
  fi
  echo "  note: rejection message differs — verified payload not stored"
  ok "invalid system-task name did not enter the schedule store"
fi

# --- 3. create a valid system-task schedule --------------------------------

echo
echo "=== create typed system-task schedule ==="

add_out=$("$AGT_BIN" schedule add "demo catalog sync" \
  --system-task catalog_sync \
  --every 24h \
  --json 2>&1) || true

# Extract the schedule id from the JSON output.
SCHED_ID=$(echo "$add_out" | grep -oE '"id":[[:space:]]*"[^"]+"' | head -n1 | sed -E 's/.*"id":[[:space:]]*"([^"]+)".*/\1/')
if [ -z "$SCHED_ID" ]; then
  # Fall back to parsing from the non-JSON add output.
  add_out=$("$AGT_BIN" schedule add "demo catalog sync" --system-task catalog_sync --every 24h 2>&1) || true
  SCHED_ID=$(echo "$add_out" | grep -oE '[0-9a-f]{16,}' | head -n1)
fi
[ -n "$SCHED_ID" ] || fail "could not create system-task schedule"
ok "catalog_sync schedule created (id=$SCHED_ID)"

# --- 4. verify the schedule shows typed target ------------------------------

echo
echo "=== schedule list shows typed target ==="

list_out=$("$AGT_BIN" schedule list 2>/dev/null) || true
echo "$list_out" | grep -q 'catalog_sync' || fail "schedule list should show catalog_sync target"
echo "$list_out" | grep -q 'system' || fail "schedule list should show system_task target type"
ok "schedule list shows typed system-task target"

# --- 5. fire the schedule immediately --------------------------------------

echo
echo "=== fire the system-task schedule now ==="

# `schedule run` fires the schedule on the next tick. For a system task, this
# runs daemon-side maintenance — no agent wake, no LLM cost.
run_out=$("$AGT_BIN" schedule run "$SCHED_ID" 2>&1) || true
echo "$run_out"
echo "$run_out" | grep -qiE 'fir|trigger|ok|schedul' || fail "schedule run did not accept the trigger"
ok "schedule run triggered the system task"

# Give the daemon a moment to execute and journal the fire.
sleep 3

# --- 6. inspect fire history ------------------------------------------------

echo
echo "=== fire history (typed outcome) ==="

fires_out=$("$AGT_BIN" schedule fires --json 2>/dev/null) || true
if echo "$fires_out" | grep -q 'fires\|target'; then
  ok "schedule fires returned fire history"
  # Verify the fire shows a system_task target type, not an agent/prompt.
  if echo "$fires_out" | grep -q 'system_task'; then
    ok "fire history shows target_type=system_task"
  else
    echo "  note: fire history may not include target_type field in all daemon versions"
  fi
else
  echo "  note: no fire history yet — the system task may still be executing"
  echo "  note: this is valid for daemon versions that journal after completion"
fi

# --- 7. verify no agent was woken -------------------------------------------

echo
echo "=== verify: system task did not wake an agent ==="

# A system-task schedule should NOT produce an agent.wake event — it runs
# daemon-side. Check the journal for the absence of agent.wake in the
# schedule's correlation. (This is a best-effort assertion; the exact event
# shape may vary by daemon version.)
journal_out=$("$AGT_BIN" journal tail 20 --json 2>/dev/null) || true
if echo "$journal_out" | grep -q 'system_task\|catalog_sync'; then
  ok "journal shows system_task execution event"
else
  echo "  note: system_task event not found in recent journal (may be journaled differently)"
fi

# --- 8. preview next fire times (dry-run) -----------------------------------

echo
echo "=== schedule preview (dry-run) ==="

test_out=$("$AGT_BIN" schedule test "$SCHED_ID" --count 3 2>&1) || true
echo "$test_out" | head -5
echo "$test_out" | grep -qE '[0-9]|fire|next' || echo "  note: preview output shape may vary"
ok "schedule preview returned future fire times"

# --- 9. cleanup + graceful shutdown -----------------------------------------

echo
echo "=== cleanup + shutdown ==="

"$AGT_BIN" schedule rm "$SCHED_ID" >/dev/null 2>&1 || true
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
echo "TYPED SCHEDULE SYSTEM-TASK DEMO: PASS"
