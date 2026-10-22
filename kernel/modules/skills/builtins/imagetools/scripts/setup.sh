#!/bin/sh
# image-tools setup: install Pillow. Idempotent. Run via code_exec or shell.
set -e

echo "installing Pillow ..."
python -m pip install --quiet Pillow 2>/dev/null \
  || pip install --quiet Pillow 2>/dev/null \
  || pip3 install Pillow

echo "image-tools ready: run scripts/img.py '<json-spec>'"
