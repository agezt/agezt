#!/usr/bin/env bash
# Install the WSL keepalive service on the self-hosted runner VM.
# Run inside WSL Ubuntu:
#   bash ops/wsl-runners/install-keepalive.sh
set -euo pipefail

UNIT="wsl-keepalive.service"
SRC_DIR="$(cd "$(dirname "$0")" && pwd)"
DST="/etc/systemd/system/${UNIT}"

echo "Installing ${UNIT}..."

sudo cp "${SRC_DIR}/${UNIT}" "${DST}"
sudo chown root:root "${DST}"
sudo chmod 644 "${DST}"

sudo systemctl daemon-reload
sudo systemctl enable --now "${UNIT}"

echo
echo "Status:"
systemctl status "${UNIT}" --no-pager -l || true
echo
echo "Done. The WSL VM will now stay alive even between CI jobs."