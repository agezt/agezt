#!/usr/bin/env bash
# coverage-rank — run `go test -cover` on every package and print a sorted
# table of coverage percentages, lowest first. Useful for CI and for spotting
# which packages need the most attention.
#
# Usage:
#   ./scripts/coverage-rank.sh              # all packages
#   ./scripts/coverage-rank.sh ./kernel/...  # only kernel packages
#   go test -cover ./... | ./scripts/coverage-rank.sh  # from a pipe
#
# Exit code: 0 (always — the table is informational; use `go test` separately
# for a hard pass/fail gate).
set -euo pipefail

cd "$(git rev-parse --show-toplevel 2>/dev/null || echo "${0%/*}/..")"

if [ -t 0 ]; then
  # No pipe — run coverage ourselves.
  targets="${*:-./...}"
  go test -cover "$targets" 2>/dev/null || true
else
  cat
fi \
| grep -E '^ok\s+' \
| awk '{
    # "ok   github.com/agezt/agezt/pkg/path  0.123s  coverage: 75.3% of statements"
    pkg = $2; sub("github.com/agezt/agezt/", "", pkg);
    cov = $5; sub("%$", "", cov);
    cov_pct = cov + 0;
    if (cov_pct > 0) printf "%5.1f%%  %s\n", cov_pct, pkg;
  }' \
| sort -t% -k1 -n \
| awk 'BEGIN {print "COV    PACKAGE\n---    -------"} {print}'
