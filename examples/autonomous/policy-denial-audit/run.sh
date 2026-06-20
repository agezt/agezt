#!/usr/bin/env bash
# policy-denial-audit/run.sh
#
# Runnable positioning demo: proves AGEZT treats governance as a first-class
# runtime concern (see docs/COMPARISON.md). Boots the keyless echo daemon,
# exercises the Edict policy engine against clearly dangerous shell input,
# and shows the decision/log/stats/audit surfaces.
#
# No provider key, no network, no external LLM.
#
# Usage:
#   bash examples/autonomous/policy-denial-audit/run.sh
#   bash examples/autonomous/policy-denial-audit/run.sh /path/to/agezt /path/to/agt
#
# Exit codes:
#   0 — demo passed
#   1 — an assertion failed (or the daemon did not behave as expected)
#   2 — usage error / prerequisites missing
#
# CPU-capped per project convention.
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
# AGEZT_DEMO_ECHO=1 gives a no-key echo provider so the daemon boots offline.
# No API/REST/Web addr is set: this demo only needs the control plane.
AGEZT_DEMO_ECHO=1 "$AGEZT_BIN" > "$AGEZT_HOME/daemon.log" 2>&1 &
DPID=$!

for _ in $(seq 1 50); do
  grep -q 'daemon ready' "$AGEZT_HOME/daemon.log" 2>/dev/null && break
  sleep 0.3
done
grep -q 'daemon ready' "$AGEZT_HOME/daemon.log" || fail "daemon did not become ready"
ok "daemon ready"

# Sanity: the CLI can reach it.
"$AGT_BIN" status >/dev/null 2>&1 || fail "agt status could not reach daemon"
ok "control plane reachable"

# --- 2. hard-deny dry-run ---------------------------------------------------

echo
echo "=== policy dry-run: catastrophic shell input ==="

# `rm -rf /` is blocked by the built-in hard-deny floor for the shell capability.
# edict test returns exit 3 for a deny verdict (distinct from 1 = error).
"$AGT_BIN" edict test shell "rm -rf /"
rc=$?
[ "$rc" -eq 3 ] || fail "expected 'rm -rf /' to be denied (exit 3), got exit $rc"
ok "catastrophic shell input hard-denied"

# --- 3. trust-level dry-run -------------------------------------------------

echo
echo "=== policy dry-run: benign shell input ==="

# A benign command is not hard-denied. The reported level shows the governing
# trust level for the shell capability (the value depends on the loaded ladder;
# we only assert that a decision + level is reported). Ignore the exit code
# (0=allow, 3=deny) — both are valid here; we only check the output shape.
out=$("$AGT_BIN" edict test shell "echo hi" 2>/dev/null || true)
echo "$out" | grep -q 'decision' || fail "edict test produced no decision line"
echo "$out" | grep -q 'level'   || fail "edict test produced no level line"
ok "benign shell input reports decision + level"

# --- 4. decision log --------------------------------------------------------

echo
echo "=== policy decision audit surface ==="

# `edict test` is a dry-run and does not journal. The log/stats surfaces read
# already-emitted policy.decision events. On a fresh daemon with no runs yet,
# "no policy decisions" is a valid, honest result — we accept both.
log_out=$("$AGT_BIN" edict log --json 2>/dev/null) || true
if echo "$log_out" | grep -q '"decisions"'; then
  ok "edict log returned a decisions array"
else
  echo "  note: no policy decisions journaled yet (fresh daemon) — honest empty result"
fi

# --- 5. aggregate stats -----------------------------------------------------

echo
echo "=== policy decision aggregate ==="

# Stats report total/allowed/denied/denial_rate. On a fresh daemon with no
# runs, total may be 0 — also valid.
stats_out=$("$AGT_BIN" edict stats --json 2>/dev/null) || true
if echo "$stats_out" | grep -q '"total"'; then
  total=$(echo "$stats_out" | grep -oE '"total":[[:space:]]*[0-9]+' | grep -oE '[0-9]+' || echo 0)
  ok "edict stats returned (total=$total)"
else
  fail "edict stats did not return a total field"
fi

# --- 6. audit chain ---------------------------------------------------------

echo
echo "=== audit chain (agt why) ==="

# Demonstrate the audit surface. On a fresh daemon there may be no events yet,
# so we capture whichever event id is available from the journal tail. If there
# is genuinely nothing journaled, we skip the `why` step with a note — this is
# an honest "no events yet" state, not a failure.
tail_out=$("$AGT_BIN" journal tail 1 --json 2>/dev/null) || true
event_id=$(echo "$tail_out" | grep -oE '"id":[[:space:]]*"[^"]+"' | head -n1 | sed -E 's/.*"id":[[:space:]]*"([^"]+)".*/\1/')
if [ -n "$event_id" ]; then
  why_out=$("$AGT_BIN" why "$event_id" 2>/dev/null) || true
  echo "$why_out" | grep -q 'events in correlation' \
    && ok "agt why walked the chain for $event_id" \
    || fail "agt why did not return a correlation for $event_id"
else
  echo "  note: no journaled events yet — skipped agt why (fresh daemon)"
fi

# --- 7. graceful shutdown ---------------------------------------------------

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
echo "POLICY DENIAL & AUDIT DEMO: PASS"
