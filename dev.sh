#!/usr/bin/env bash
# Thin wrapper — see scripts/dev.sh for the real thing and its flags.
set -uo pipefail
exec "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/scripts/dev.sh" "$@"
