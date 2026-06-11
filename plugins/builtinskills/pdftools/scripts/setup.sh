#!/bin/sh
# pdf-tools setup: install pypdf (merge/split/metadata) + pdfplumber (text/tables).
# Idempotent. Run via code_exec or shell.
set -e

echo "installing pypdf + pdfplumber ..."
python -m pip install --quiet pypdf pdfplumber 2>/dev/null \
  || pip install --quiet pypdf pdfplumber 2>/dev/null \
  || pip3 install pypdf pdfplumber

echo "pdf-tools ready: run scripts/pdf.py '<json-spec>'"
