#!/usr/bin/env bash
# dev.sh — one-shot dev loop: isolated home + seeded vault + build + run.
# POSIX counterpart of dev.ps1 (macOS / Linux).
#
#   ./dev.sh                        build ./agezt / ./agt, seed .dev-home, run the daemon
#   ./dev.sh --fresh                wipe .dev-home first (clean vault/journal/state)
#   ./dev.sh --skip-build           reuse ./agezt / ./agt from the last build
#   ./dev.sh --pull                 git pull --ff-only before building
#   ./dev.sh --web-addr 127.0.0.1:9000   serve the console elsewhere
#
# Frontend commands must be run from the repo's frontend directory:
#   cd ./frontend
#   npm run build     # runs: tsc --noEmit && vite build
#   npm test
# Running npm commands from the repo root will use the wrong working directory.
#
# The daemon runs against ./.dev-home (NEVER the real ~/.agezt — a dev run
# against the real home once rewrote live standing orders). Provider keys are
# seeded into the dev vault from .env (repo root, or the main repo root when
# running from a worktree): every KEY ending in _API_KEY, plus AGEZT_PROVIDER /
# AGEZT_MODEL / AGEZT_ALLOW_ALL / AGEZT_VAULT_PASSPHRASE pass through as env.
# The vault encrypts itself with the machine-bound key (M934) — no passphrase
# needed unless .env sets one.
set -uo pipefail

FRESH=0; SKIP_BUILD=0; PULL=0; WEB_ADDR="127.0.0.1:8899"
while [ $# -gt 0 ]; do
  case "$1" in
    -f|--fresh)      FRESH=1 ;;
    -s|--skip-build) SKIP_BUILD=1 ;;
    -p|--pull)       PULL=1 ;;
    -w|--web-addr)   WEB_ADDR="${2:-}"; shift ;;
    --web-addr=*)    WEB_ADDR="${1#*=}" ;;
    -h|--help)       sed -n '2,25p' "$0"; exit 0 ;;
    *) echo "unknown option: $1 (try --help)" >&2; exit 2 ;;
  esac
  shift
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT"
DEV_HOME="$ROOT/.dev-home"
AGEZT="$ROOT/agezt"
AGT="$ROOT/agt"

if [ -t 1 ]; then C_CYAN=$'\033[36m'; C_YEL=$'\033[33m'; C_OFF=$'\033[0m'; else C_CYAN=""; C_YEL=""; C_OFF=""; fi
step() { printf '%s==> %s%s\n' "$C_CYAN" "$*" "$C_OFF"; }
warn() { printf '%s    %s%s\n' "$C_YEL" "$*" "$C_OFF"; }

# --- locate .env: repo root, else the main repo root (worktree case)
ENV_FILE="$ROOT/.env"
if [ ! -f "$ENV_FILE" ]; then
  common="$(git -C "$ROOT" rev-parse --git-common-dir 2>/dev/null || true)"
  if [ -n "$common" ]; then
    case "$common" in /*|[A-Za-z]:*) ;; *) common="$ROOT/$common" ;; esac
    if [ -d "$common" ]; then
      main_root="$(cd "$common/.." && pwd)"
      [ -f "$main_root/.env" ] && ENV_FILE="$main_root/.env"
    fi
  fi
fi

# --- parse .env (KEY=VALUE, # comments) without echoing secrets to the console.
# Parallel indexed arrays, not an associative array: macOS still ships bash 3.2.
ENV_KEYS=(); ENV_VALS=(); ENV_N=0
dotenv_get() { # $1=key -> echoes value, returns 1 when absent
  local i=0
  while [ "$i" -lt "$ENV_N" ]; do
    if [ "${ENV_KEYS[$i]}" = "$1" ]; then printf '%s' "${ENV_VALS[$i]}"; return 0; fi
    i=$((i + 1))
  done
  return 1
}
if [ -f "$ENV_FILE" ]; then
  step "loading $ENV_FILE"
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line%$'\r'}"                       # tolerate CRLF .env files
    t="$(printf '%s' "$line" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
    case "$t" in ""|'#'*) continue ;; esac
    case "$t" in *=*) ;; *) continue ;; esac
    k="${t%%=*}"; v="${t#*=}"
    k="$(printf '%s' "$k" | sed -e 's/[[:space:]]*$//')"
    [ -z "$k" ] && continue
    v="$(printf '%s' "$v" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e 's/^"//' -e 's/"$//')"
    ENV_KEYS[$ENV_N]="$k"; ENV_VALS[$ENV_N]="$v"; ENV_N=$((ENV_N + 1))
  done < "$ENV_FILE"
else
  warn "(no .env found — vault will be seeded empty; add keys via ./agt provider creds set)"
fi

# --- isolated home
if [ "$FRESH" = 1 ] && [ -d "$DEV_HOME" ]; then step "wiping $DEV_HOME"; rm -rf "$DEV_HOME"; fi
mkdir -p "$DEV_HOME"
export AGEZT_HOME="$DEV_HOME"

# pass-through env the daemon reads; everything else stays out of the process env
for k in AGEZT_PROVIDER AGEZT_MODEL AGEZT_ALLOW_ALL AGEZT_VAULT_PASSPHRASE; do
  if v="$(dotenv_get "$k")"; then export "$k=$v"; fi
done

# --- build freshness (M972): the daemon embeds the web UI at build time, so a
# stale checkout silently ships an OLD console (e.g. no Fallback Chains page,
# old agent edit form). Print the revision we're building and shout when the
# branch is behind its upstream, so a redeploy that "changed nothing" is
# obvious. Pass --pull to fast-forward to the latest before building.
if [ "$SKIP_BUILD" = 0 ]; then
  branch="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
  if [ "$PULL" = 1 ]; then
    step "git pull --ff-only ($branch)"
    git -C "$ROOT" pull --ff-only || exit 1
  fi
  head="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || true)"
  [ -n "$head" ] && step "building $branch @ $head"
  git -C "$ROOT" fetch -q 2>/dev/null || true
  behind="$(git -C "$ROOT" rev-list --count 'HEAD..@{upstream}' 2>/dev/null || true)"
  if [ -n "$behind" ] && [ "$behind" -gt 0 ] 2>/dev/null; then
    warn "WARNING: this checkout is $behind commit(s) BEHIND its upstream — the build and its embedded console will be OLD. Run:  git pull   (or ./dev.sh --pull)"
  fi
fi

# --- build
if [ "$SKIP_BUILD" = 0 ]; then
  step "go build ./agezt + ./agt"
  go build -o "$AGEZT" ./cmd/agezt || exit 1
  go build -o "$AGT"   ./cmd/agt   || exit 1
fi

# --- seed catalog (once): copy the real synced catalog read-only, else sync offline
DEV_CATALOG="$DEV_HOME/catalog"
if [ ! -d "$DEV_CATALOG" ]; then
  REAL_CATALOG="$HOME/.agezt/catalog"
  if [ -d "$REAL_CATALOG" ]; then
    step "seeding catalog (copy of ~/.agezt/catalog, read-only source)"
    cp -R "$REAL_CATALOG" "$DEV_CATALOG"
  else
    step "seeding catalog (agt catalog sync --local)"
    if ! "$AGT" catalog sync --local >/dev/null 2>&1; then  # best effort; needs network
      warn "catalog sync failed — daemon boots on the mock; sync later from the console"
    fi
  fi
fi

# --- seed vault defaults: every *_API_KEY from .env goes into the DEV vault
keys=""; nkeys=0; i=0
while [ "$i" -lt "$ENV_N" ]; do
  case "${ENV_KEYS[$i]}" in *_API_KEY) keys="$keys ${ENV_KEYS[$i]}"; nkeys=$((nkeys + 1)) ;; esac
  i=$((i + 1))
done
if [ "$nkeys" -gt 0 ]; then
  step "seeding vault ($nkeys key(s): $(printf '%s' "$keys" | sed 's/^ //; s/ /, /g'))"
  for k in $keys; do "$AGT" provider creds set "$k" "$(dotenv_get "$k")" >/dev/null; done
fi

# --- run
export AGEZT_WEB_ADDR="$WEB_ADDR"
step "starting agezt  (home=$DEV_HOME, console=http://$WEB_ADDR, Ctrl+C to stop)"
exec "$AGEZT"
