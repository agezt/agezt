#!/bin/sh
# office-docs setup: install python-docx + openpyxl. Idempotent. Run via code_exec or shell.
set -e

echo "installing python-docx + openpyxl ..."
python -m pip install --quiet python-docx openpyxl 2>/dev/null \
  || pip install --quiet python-docx openpyxl 2>/dev/null \
  || pip3 install python-docx openpyxl

echo "office-docs ready: run scripts/office.py '<json-spec>'"
