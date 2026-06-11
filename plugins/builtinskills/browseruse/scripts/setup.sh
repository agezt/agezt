#!/bin/sh
# browser-use setup: install Playwright + the Chromium browser into the sandbox.
# Idempotent — re-running is cheap once installed. Run via code_exec or shell.
set -e

echo "installing playwright (node) ..."
# Local install keeps it in the sandbox project; -g also works if you prefer.
npm install playwright@latest >/dev/null 2>&1 || npm install playwright@latest

echo "downloading the chromium browser ..."
# --with-deps pulls OS libraries on Linux; harmless elsewhere. If it fails for a
# missing OS package, install that package with the shell tool and re-run.
npx playwright install chromium --with-deps 2>/dev/null || npx playwright install chromium

echo "browser-use ready: drive it with scripts/browse.mjs"
