#!/bin/sh
# ssh-remote setup: install paramiko. Idempotent. Run via code_exec or shell.
set -e

echo "installing paramiko ..."
python -m pip install --quiet paramiko 2>/dev/null \
  || pip install --quiet paramiko 2>/dev/null \
  || pip3 install paramiko

echo "ssh-remote ready: run scripts/ssh.py '<json-spec>'"
