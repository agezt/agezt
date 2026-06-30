#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
#
# Production installer/runner/updater for AGEZT on Ubuntu.
#
# Usage:
#   sudo ./install.sh install              # install deps, build, install service
#   sudo ./install.sh start|stop|restart   # manage systemd service
#   ./install.sh status                    # show service + API health
#   sudo ./install.sh update               # fetch/checkout, rebuild, reinstall, restart
#   sudo ./install.sh run                  # run daemon in foreground as agezt user
#   sudo ./install.sh expose tailscale      # install/configure controlled access option
#   sudo ./install.sh expose cloudflare
#   sudo ./install.sh expose ngrok
#
# Useful overrides:
#   AGEZT_REPO=https://github.com/agezt/agezt.git
#   AGEZT_REF=v1.0.0
#   AGEZT_ALLOW_UNPINNED=1       # required for branch refs such as main
#   AGEZT_ALLOW_REMOTE_INSTALL=1 # required before adding external package repos or running remote installers
#   AGEZT_SRC=/opt/agezt/src
#   AGEZT_HOME=/var/lib/agezt
#   AGEZT_REST_ADDR=127.0.0.1:8787
#   GO_VERSION=1.26.4
#   NODE_MAJOR=22

set -Eeuo pipefail

SERVICE_NAME="agezt.service"
AGEZT_REPO="${AGEZT_REPO:-https://github.com/agezt/agezt.git}"
AGEZT_REF="${AGEZT_REF:-v1.0.0}"
AGEZT_ALLOW_UNPINNED="${AGEZT_ALLOW_UNPINNED:-0}"
AGEZT_ALLOW_REMOTE_INSTALL="${AGEZT_ALLOW_REMOTE_INSTALL:-0}"
AGEZT_SRC="${AGEZT_SRC:-/opt/agezt/src}"
AGEZT_PREFIX="${AGEZT_PREFIX:-/opt/agezt}"
AGEZT_BIN_DIR="${AGEZT_BIN_DIR:-/usr/local/bin}"
AGEZT_HOME="${AGEZT_HOME:-/var/lib/agezt}"
AGEZT_CONFIG_DIR="${AGEZT_CONFIG_DIR:-/etc/agezt}"
AGEZT_ENV_FILE="${AGEZT_ENV_FILE:-$AGEZT_CONFIG_DIR/agezt.env}"
AGEZT_REST_ADDR="${AGEZT_REST_ADDR:-127.0.0.1:8787}"
GO_VERSION="${GO_VERSION:-1.26.4}"
NODE_MAJOR="${NODE_MAJOR:-22}"
INSTALL_USER="${INSTALL_USER:-agezt}"
INSTALL_GROUP="${INSTALL_GROUP:-agezt}"

log() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2; }
fail() { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }
need_root() { [ "${EUID:-$(id -u)}" -eq 0 ] || fail "run this command with sudo/root"; }
have() { command -v "$1" >/dev/null 2>&1; }

apt_install() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y --no-install-recommends "$@"
}

ensure_ubuntu() {
  [ -r /etc/os-release ] || fail "cannot detect OS; this installer targets Ubuntu"
  local os_id os_pretty
  os_id="$(. /etc/os-release; printf '%s' "${ID:-}")"
  os_pretty="$(. /etc/os-release; printf '%s' "${PRETTY_NAME:-unknown}")"
  [ "$os_id" = "ubuntu" ] || warn "detected $os_pretty; script is tested for Ubuntu production hosts"
}

is_pinned_ref() {
  case "$AGEZT_REF" in
    v[0-9]*|refs/tags/*) return 0 ;;
    *) return 1 ;;
  esac
}

require_pinned_ref() {
  if is_pinned_ref; then
    return
  fi
  [ "$AGEZT_ALLOW_UNPINNED" = "1" ] || fail "AGEZT_REF=$AGEZT_REF is not a pinned release tag; set AGEZT_REF=vX.Y.Z or AGEZT_ALLOW_UNPINNED=1 to opt into branch installs"
}

require_remote_install_opt_in() {
  [ "$AGEZT_ALLOW_REMOTE_INSTALL" = "1" ] || fail "external package repositories and remote installers require AGEZT_ALLOW_REMOTE_INSTALL=1; preinstall the dependency or opt in explicitly"
}

ensure_service_home_path() {
  case "$AGEZT_HOME" in
    /home/*|/root|/root/*)
      fail "AGEZT_HOME=$AGEZT_HOME is incompatible with the hardened systemd service; use /var/lib/agezt or another non-home path"
      ;;
  esac
}

ensure_user() {
  ensure_service_home_path
  if ! getent group "$INSTALL_GROUP" >/dev/null; then
    groupadd --system "$INSTALL_GROUP"
  fi
  if ! id -u "$INSTALL_USER" >/dev/null 2>&1; then
    useradd --system --gid "$INSTALL_GROUP" --home-dir "$AGEZT_HOME" --shell /usr/sbin/nologin "$INSTALL_USER"
  fi
  install -d -m 0750 -o "$INSTALL_USER" -g "$INSTALL_GROUP" "$AGEZT_HOME"
  install -d -m 0755 "$AGEZT_PREFIX" "$AGEZT_BIN_DIR" "$AGEZT_CONFIG_DIR"
}

ensure_go() {
  if have go; then
    log "Go found: $(go version)"
    return
  fi
  require_remote_install_opt_in
  log "Installing Go $GO_VERSION to /usr/local/go"
  local arch tarball url tmp
  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) fail "unsupported CPU architecture for automatic Go install: $(uname -m)" ;;
  esac
  tarball="go${GO_VERSION}.linux-${arch}.tar.gz"
  url="https://go.dev/dl/${tarball}"
  tmp="/tmp/${tarball}"
  curl -fsSL "$url" -o "$tmp"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "$tmp"
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
  ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
}

ensure_node() {
  if have node && node -v | grep -Eq "^v(${NODE_MAJOR}|2[3-9]|[3-9][0-9])\."; then
    log "Node found: $(node -v)"
    return
  fi
  require_remote_install_opt_in
  log "Installing Node.js ${NODE_MAJOR}.x from NodeSource"
  apt_install ca-certificates curl gnupg
  install -d -m 0755 /etc/apt/keyrings
  curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg
  printf 'deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_%s.x nodistro main\n' "$NODE_MAJOR" > /etc/apt/sources.list.d/nodesource.list
  apt-get update
  apt-get install -y nodejs
}

ensure_prereqs() {
  ensure_ubuntu
  apt_install ca-certificates curl git make tar gzip bash coreutils systemd procps
  ensure_go
  ensure_node
  have npm || fail "npm was not installed with Node.js"
}

sync_source() {
  require_pinned_ref
  if [ -d "$AGEZT_SRC/.git" ]; then
    log "Updating source in $AGEZT_SRC"
    git -C "$AGEZT_SRC" fetch --tags origin
    git -C "$AGEZT_SRC" checkout "$AGEZT_REF"
    if ! is_pinned_ref; then
      git -C "$AGEZT_SRC" pull --ff-only origin "$AGEZT_REF"
    fi
  elif [ -f "$(pwd)/go.mod" ] && grep -q '^module github.com/agezt/agezt' "$(pwd)/go.mod"; then
    log "Using current checkout: $(pwd)"
    install -d -m 0755 "$(dirname "$AGEZT_SRC")"
    if [ "$(pwd)" != "$AGEZT_SRC" ]; then
      rm -rf "$AGEZT_SRC"
      git clone "$(pwd)" "$AGEZT_SRC"
    fi
    git -C "$AGEZT_SRC" checkout "$AGEZT_REF" 2>/dev/null || true
  else
    log "Cloning $AGEZT_REPO#$AGEZT_REF to $AGEZT_SRC"
    install -d -m 0755 "$(dirname "$AGEZT_SRC")"
    git clone --branch "$AGEZT_REF" "$AGEZT_REPO" "$AGEZT_SRC"
  fi
}

build_agezt() {
  log "Building production frontend"
  cd "$AGEZT_SRC/frontend"
  if [ -f package-lock.json ]; then
    npm ci
  else
    npm install
  fi
  npm run build

  log "Building production Go binaries"
  cd "$AGEZT_SRC"
  export CGO_ENABLED=0
  local version commit build_time ldflags release_dir tmp_link
  version="${VERSION:-$(git describe --tags --always --dirty=-dev 2>/dev/null || echo dev)}"
  commit="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
  build_time="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
  release_dir="$AGEZT_PREFIX/releases/${version}-${commit}-${build_time//[:]/}"
  install -d -m 0755 "$release_dir"
  ldflags="-s -w -X github.com/agezt/agezt/internal/brand.Version=${version} -X github.com/agezt/agezt/internal/brand.BuildCommit=${commit} -X github.com/agezt/agezt/internal/brand.BuildTime=${build_time}"
  go mod download
  go build -trimpath -ldflags "$ldflags" -o "$release_dir/agezt" ./cmd/agezt
  go build -trimpath -ldflags "$ldflags" -o "$release_dir/agt" ./cmd/agt
  chmod 0755 "$release_dir/agezt" "$release_dir/agt"
  tmp_link="$AGEZT_PREFIX/current.tmp"
  ln -sfn "$release_dir" "$tmp_link"
  mv -Tf "$tmp_link" "$AGEZT_PREFIX/current"
  ln -sf "$AGEZT_PREFIX/current/agezt" "$AGEZT_BIN_DIR/agezt"
  ln -sf "$AGEZT_PREFIX/current/agt" "$AGEZT_BIN_DIR/agt"
}

write_env() {
  log "Writing $AGEZT_ENV_FILE"
  install -d -m 0755 "$AGEZT_CONFIG_DIR"
  if [ ! -f "$AGEZT_ENV_FILE" ]; then
    cat > "$AGEZT_ENV_FILE" <<EOF
# AGEZT production environment.
# Keep service bindings loopback by default. Use ./install.sh expose <provider>
# for controlled external access instead of binding directly to 0.0.0.0.
AGEZT_HOME=$AGEZT_HOME
AGEZT_REST_ADDR=$AGEZT_REST_ADDR
# Optional tunnel/external access defaults. Prefer configuring these in the
# Web UI Config Center after first boot; changes require restart.
# AGEZT_TUNNEL=cloudflare
# AGEZT_TUNNEL_TARGET=http://127.0.0.1:8787
# AGEZT_TUNNEL_CMD=
# AGEZT_TUNNEL_NOTES=

# Optional provider defaults. Prefer configuring credentials through the Web UI
# setup screen or: agt provider creds set <provider>
# AGEZT_PROVIDER=openai
# AGEZT_MODEL=gpt-4.1
# OPENAI_API_KEY=
EOF
    chmod 0640 "$AGEZT_ENV_FILE"
  else
    grep -q '^AGEZT_HOME=' "$AGEZT_ENV_FILE" || printf '\nAGEZT_HOME=%s\n' "$AGEZT_HOME" >> "$AGEZT_ENV_FILE"
    grep -q '^AGEZT_REST_ADDR=' "$AGEZT_ENV_FILE" || printf 'AGEZT_REST_ADDR=%s\n' "$AGEZT_REST_ADDR" >> "$AGEZT_ENV_FILE"
  fi
  chown root:"$INSTALL_GROUP" "$AGEZT_ENV_FILE"
}

write_service() {
  log "Writing /etc/systemd/system/$SERVICE_NAME"
  cat > "/etc/systemd/system/$SERVICE_NAME" <<EOF
[Unit]
Description=AGEZT daemon
Documentation=https://github.com/agezt/agezt
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$INSTALL_USER
Group=$INSTALL_GROUP
EnvironmentFile=$AGEZT_ENV_FILE
WorkingDirectory=$AGEZT_HOME
ExecStart=$AGEZT_PREFIX/current/agezt daemon
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=5s
TimeoutStopSec=30s
KillSignal=SIGTERM
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=$AGEZT_HOME
LockPersonality=true
MemoryDenyWriteExecute=false

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME"
}

install_all() {
  need_root
  ensure_prereqs
  ensure_user
  sync_source
  build_agezt
  write_env
  write_service
  systemctl restart "$SERVICE_NAME"
  log "AGEZT installed and started"
  status
  cat <<EOF

REST/Web binding: http://$AGEZT_REST_ADDR
Bearer token file: $AGEZT_HOME/rest.token (created after daemon starts)

Keep AGEZT_REST_ADDR on 127.0.0.1 for production. For external access, run one of:
  sudo ./install.sh expose tailscale
  sudo ./install.sh expose cloudflare
  sudo ./install.sh expose ngrok
EOF
}

status() {
  if have systemctl; then
    systemctl --no-pager --full status "$SERVICE_NAME" || true
  fi
  if [ -f "$AGEZT_HOME/rest.token" ]; then
    log "REST token exists: $AGEZT_HOME/rest.token"
  else
    warn "REST token not found yet. Check journalctl -u $SERVICE_NAME -f"
  fi
  if have curl; then
    curl -fsS "http://$AGEZT_REST_ADDR/healthz" >/dev/null && log "healthz OK at http://$AGEZT_REST_ADDR/healthz" || warn "healthz not reachable at http://$AGEZT_REST_ADDR/healthz"
  fi
}

update_all() {
  need_root
  ensure_prereqs
  sync_source
  build_agezt
  systemctl restart "$SERVICE_NAME"
  log "AGEZT updated and restarted"
  status
}

run_foreground() {
  need_root
  ensure_user
  write_env
  log "Running AGEZT in foreground as $INSTALL_USER. Press Ctrl+C to stop."
  exec runuser -u "$INSTALL_USER" -- env AGEZT_HOME="$AGEZT_HOME" AGEZT_REST_ADDR="$AGEZT_REST_ADDR" "$AGEZT_PREFIX/current/agezt" daemon
}

install_cloudflared() {
  need_root
  require_remote_install_opt_in
  log "Installing cloudflared"
  mkdir -p --mode=0755 /usr/share/keyrings
  curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null
  echo 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared any main' > /etc/apt/sources.list.d/cloudflared.list
  apt-get update
  apt-get install -y cloudflared

  log "Installing managed Cloudflare quick tunnel service"
  cat > /etc/systemd/system/agezt-cloudflared.service <<EOF
[Unit]
Description=AGEZT Cloudflare Quick Tunnel (trycloudflare.com)
After=network-online.target $SERVICE_NAME
Wants=network-online.target
Requires=$SERVICE_NAME

[Service]
Type=simple
User=$INSTALL_USER
Group=$INSTALL_GROUP
ExecStart=/usr/bin/cloudflared tunnel --no-autoupdate --url http://$AGEZT_REST_ADDR
Restart=always
RestartSec=10
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now agezt-cloudflared.service
  cat <<EOF

Cloudflare quick tunnel service installed and started.
Cloudflare will generate a random https://*.trycloudflare.com URL.

Show the generated URL:
  sudo journalctl -u agezt-cloudflared -n 100 --no-pager | grep -Eo 'https://[-a-zA-Z0-9.]+\\.trycloudflare\\.com'

Manage the tunnel service:
  sudo systemctl status agezt-cloudflared --no-pager
  sudo systemctl restart agezt-cloudflared
  sudo systemctl disable --now agezt-cloudflared

Production note: trycloudflare.com quick tunnels are convenient but not stable
hostnames. For a stable hostname and Cloudflare Access policy, create a named
Cloudflare Tunnel and route your own domain.
EOF
}

install_tailscale() {
  need_root
  require_remote_install_opt_in
  log "Installing Tailscale"
  curl -fsSL https://tailscale.com/install.sh | sh
  cat <<EOF

Tailscale installed.
Controlled private access flow:
  1. tailscale up --ssh
  2. tailscale serve --bg --http=8787 http://$AGEZT_REST_ADDR
  3. Open http://<this-hostname>:8787 from devices in your tailnet.

Optional public Funnel, only if your tailnet policy allows it:
  tailscale funnel --bg 8787
EOF
}

install_ngrok() {
  need_root
  require_remote_install_opt_in
  log "Installing ngrok"
  curl -fsSL https://ngrok-agent.s3.amazonaws.com/ngrok.asc | tee /etc/apt/trusted.gpg.d/ngrok.asc >/dev/null
  echo 'deb https://ngrok-agent.s3.amazonaws.com bookworm main' > /etc/apt/sources.list.d/ngrok.list
  apt-get update
  apt-get install -y ngrok
  cat <<EOF

ngrok installed.
Controlled access flow:
  1. ngrok config add-authtoken <token-from-ngrok-dashboard>
  2. ngrok http http://$AGEZT_REST_ADDR
  3. Restrict access in the ngrok dashboard with OAuth/IP policies when available.
EOF
}

expose() {
  case "${1:-}" in
    cloudflare|cloudflared) install_cloudflared ;;
    tailscale) install_tailscale ;;
    ngrok) install_ngrok ;;
    *) fail "usage: $0 expose cloudflare|tailscale|ngrok" ;;
  esac
}

case "${1:-install}" in
  install) install_all ;;
  update) update_all ;;
  run) run_foreground ;;
  start) need_root; systemctl start "$SERVICE_NAME" ;;
  stop) need_root; systemctl stop "$SERVICE_NAME" ;;
  restart) need_root; systemctl restart "$SERVICE_NAME" ;;
  status) status ;;
  logs) journalctl -u "$SERVICE_NAME" -f ;;
  expose) expose "${2:-}" ;;
  *)
    cat <<EOF
Usage: $0 install|update|run|start|stop|restart|status|logs|expose

Examples:
  sudo $0 install
  sudo $0 update
  sudo $0 expose tailscale
EOF
    exit 2
    ;;
esac
