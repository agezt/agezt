#!/bin/sh
# http-api-client setup: install requests. Idempotent. Run via code_exec or shell.
set -e

echo "installing requests ..."
python -m pip install --quiet requests 2>/dev/null \
  || pip install --quiet requests 2>/dev/null \
  || pip3 install requests

echo "http-api-client ready: run scripts/api.py '<json-spec>'"
