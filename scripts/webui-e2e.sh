#!/usr/bin/env bash
# webui-e2e.sh — boot the real agezt daemon (keyless echo mock) with the Web UI
# enabled, submit one intent so the Runs view has a card, then drive the
# go:embed-ded production SPA in a headless browser with Playwright. Asserts the
# shell + live SSE indicator render, navigation works, live daemon status and the
# run-detail cards show real data, and there are ZERO console errors under the
# strict CSP. Exits non-zero on any failure.
#
#   make webui-e2e                                   # build, then run this
#   scripts/webui-e2e.sh /path/to/agezt /path/to/agt # against prebuilt binaries
#
# The frontend deps + Playwright browser must already be installed
# (cd frontend && npm ci && npx playwright install --with-deps chromium).
# CPU-capped (GOMAXPROCS=3) per project convention. Linux/macOS/git-bash.
set -uo pipefail

fail() { echo "WEBUI-E2E FAIL: $*" >&2; exit 1; }
ok()   { echo "  ok: $*"; }

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
AGEZT_BIN="${1:-}"; AGT_BIN="${2:-}"
TMP="$(mktemp -d)"; export GOMAXPROCS=3
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

PORT_WEB=18787
echo "starting daemon (demo echo, Web UI on :$PORT_WEB)…"
AGEZT_DEMO_ECHO=1 AGEZT_MODEL=mock AGEZT_WEB_ADDR=127.0.0.1:$PORT_WEB \
  "$AGEZT_BIN" > "$AGEZT_HOME/daemon.log" 2>&1 &
DPID=$!
for _ in $(seq 1 50); do grep -q 'daemon ready' "$AGEZT_HOME/daemon.log" 2>/dev/null && break; sleep 0.3; done
grep -q 'daemon ready' "$AGEZT_HOME/daemon.log" || fail "daemon did not become ready:\n$(cat "$AGEZT_HOME/daemon.log")"
sleep 1
ok "daemon ready"

# Submit an intent so the Runs view has a completed run to render + expand.
"$AGT_BIN" run "hello e2e" -q 2>&1 | grep -q '\[echo\]' || fail "agt run did not echo"
ok "seeded a run (hello e2e)"

# The daemon logs the Web UI URL with its one-time token in the query string.
URL=$(grep -oE "http://127\.0\.0\.1:$PORT_WEB/\?token=[a-f0-9]+" "$AGEZT_HOME/daemon.log" | head -1)
[ -n "$URL" ] || fail "could not find the Web UI URL in the daemon log"
ok "web ui url resolved"

echo "running Playwright against the embedded SPA…"
cd "$REPO_ROOT/frontend" || fail "cd frontend"
AGEZT_WEBUI_URL="$URL" npx playwright test || fail "playwright e2e"
ok "Playwright e2e passed"

echo "WEBUI-E2E PASS"
