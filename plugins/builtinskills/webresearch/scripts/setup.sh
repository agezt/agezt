#!/bin/sh
# web-research setup: install HTTP + HTML-extraction deps.
# trafilatura gives much cleaner main-text extraction; it's optional — the
# helper falls back to BeautifulSoup if it isn't present. Idempotent.
set -e

echo "installing requests + beautifulsoup4 (+ trafilatura) ..."
python -m pip install --quiet requests beautifulsoup4 2>/dev/null \
  || pip install --quiet requests beautifulsoup4 2>/dev/null \
  || pip3 install requests beautifulsoup4

# Best-effort: better extraction when available, never fatal if it won't build.
python -m pip install --quiet trafilatura 2>/dev/null \
  || pip install --quiet trafilatura 2>/dev/null \
  || echo "(trafilatura not installed — falling back to BeautifulSoup extraction)"

echo "web-research ready: run scripts/extract.py '<json-spec>'"
