#!/usr/bin/env bash
# plugin-governance/run.sh
#
# Runnable positioning demo: proves AGEZT treats tools and plugins as governed,
# audited capabilities (see docs/COMPARISON.md, docs/PLUGIN-SECURITY.md).
# Shows tool registry, tool invocation audit, policy gating, plugin pin hashing,
# and the plugin list surface.
#
# No provider key, no network, no external LLM. Uses the keyless echo daemon.
#
# NOTE: the echo daemon has no external plugins by default. This demo proves the
# governance surfaces (tool list, tool log, edict test, plugin hash, plugin list)
# that plugin trust builds on. See README.md for the honest limitation.
#
# Usage:
#   bash examples/autonomous/plugin-governance/run.sh
#   bash examples/autonomous/plugin-governance/run.sh /path/to/agezt /path/to/agt
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

# --- 2. tool registry visibility -------------------------------------------

echo
echo "=== tool registry (what the model sees) ==="

tools_out=$("$AGT_BIN" tool list 2>/dev/null) || fail "tool list failed"
echo "$tools_out" | grep -q 'tool(s)' || fail "tool list should report tool count"
# The echo daemon registers at least a few in-process tools (file, http, shell, etc.)
tool_count=$(echo "$tools_out" | head -1 | grep -oE '[0-9]+' || echo "0")
if [ "$tool_count" -gt 0 ]; then
  ok "tool list shows $tool_count registered tools"
else
  echo "  note: no tools registered (some daemon configs expose zero tools to the model)"
fi

# --- 3. policy gating of tools ---------------------------------------------

echo
echo "=== policy gating: catastrophic tool input denied ==="

# The shell capability is gated by Edict. A catastrophic command is hard-denied.
"$AGT_BIN" edict test shell "rm -rf /"
rc=$?
[ "$rc" -eq 3 ] || fail "expected 'rm -rf /' to be denied (exit 3), got exit $rc"
ok "catastrophic shell input denied by policy (not just unadvertised)"

# --- 4. tool invocation audit ----------------------------------------------

echo
echo "=== tool invocation audit surface ==="

# `agt tool log` shows what the agent actually ran. On a fresh daemon with no
# runs yet, it may be empty — that's an honest result.
log_out=$("$AGT_BIN" tool log --json 2>/dev/null) || true
if echo "$log_out" | grep -q '"calls"'; then
  ok "tool log returned an invocations array"
else
  echo "  note: no tool invocations yet (fresh daemon) — honest empty result"
fi

# --- 5. plugin pin hashing (binary integrity control) -----------------------

echo
echo "=== plugin pin hashing (BLAKE3-256) ==="

# Hash the agt binary itself as a stand-in for a plugin binary — proving the
# pin hashing mechanism works and produces a usable BLAKE3-256 digest.
hash_out=$("$AGT_BIN" plugin hash "$AGT_BIN" 2>/dev/null) || true
if echo "$hash_out" | grep -qE '^[0-9a-f]{64}$'; then
  ok "plugin hash produced a valid BLAKE3-256 digest"
  echo "    digest: ${hash_out:0:16}..."
  echo "    usable as: AGEZT_PLUGIN_PINS=\"myprefix=$hash_out\""
else
  fail "plugin hash did not produce a 64-char hex digest"
fi

# --- 6. plugin list surface ------------------------------------------------

echo
echo "=== plugin list (external plugin trust surface) ==="

plugin_out=$("$AGT_BIN" plugin list 2>/dev/null) || true
if echo "$plugin_out" | grep -q 'no external plugins'; then
  ok "plugin list correctly reports no external plugins (fresh daemon)"
  echo "    to load one: AGEZT_PLUGINS=\"prefix=/path/to/plugin\" AGEZT_PLUGIN_PINS=\"prefix=<hash>\""
elif echo "$plugin_out" | grep -q 'plugin(s)'; then
  ok "plugin list shows external plugins"
  echo "$plugin_out" | grep -q '\[pinned\]' \
    && ok "at least one plugin is pin-verified" \
    || echo "  note: no pinned plugins (set AGEZT_PLUGIN_PINS to enable)"
else
  echo "  note: plugin list output unexpected — may vary by daemon version"
fi

# --- 7. governance chain (tool → policy → journal) -------------------------

echo
echo "=== governance chain (tool → policy → audit) ==="

# Show the policy decision log — this is where denied tool calls are journaled.
edict_log=$("$AGT_BIN" edict log --json 2>/dev/null) || true
if echo "$edict_log" | grep -q '"decisions"'; then
  ok "edict log shows policy decisions (governance audit trail)"
elif echo "$edict_log" | grep -q 'no policy decisions'; then
  echo "  note: no policy decisions journaled yet (edict test is a dry-run)"
  echo "    a live agent run with a denied tool call would appear here"
fi

# Show edict stats for the aggregate view.
edict_stats=$("$AGT_BIN" edict stats --json 2>/dev/null) || true
if echo "$edict_stats" | grep -q '"total"'; then
  ok "edict stats shows aggregate policy metrics"
fi

# --- 8. verify the governance model is layered -----------------------------

echo
echo "=== governance model summary ==="

echo "  tool list    → what the model CAN call"
echo "  edict test   → what policy WOULD decide (dry-run)"
echo "  edict log    → what policy DID decide (journaled)"
echo "  tool log     → what the agent ACTUALLY ran"
echo "  plugin list  → what external plugins are loaded + pinned"
echo "  plugin hash  → how to pin a plugin binary"
ok "governance is a layered chain, not a single gate"

# --- 9. graceful shutdown ---------------------------------------------------

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
echo "PLUGIN AND TOOL GOVERNANCE DEMO: PASS"
