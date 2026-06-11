#!/bin/sh
# data-analysis setup: install pandas + matplotlib (+ Excel/parquet support).
# Idempotent. Run via code_exec or shell.
set -e

echo "installing pandas + matplotlib + openpyxl ..."
python -m pip install --quiet pandas matplotlib openpyxl pyarrow 2>/dev/null \
  || pip install --quiet pandas matplotlib openpyxl pyarrow 2>/dev/null \
  || pip3 install pandas matplotlib openpyxl pyarrow

echo "data-analysis ready: run scripts/analyze.py '<json-spec>'"
