#!/bin/sh
# docker-services lifecycle helper. Every container it starts is named
# agezt-<name> and labelled agezt.service=1 so agezt's services are always
# discoverable and reapable without touching the user's own containers.
#
# Usage:
#   svc.sh up   <name> <image> [extra docker run args...]
#   svc.sh down <name>            # stop + remove container (named volumes kept)
#   svc.sh nuke <name>            # down AND remove its named volumes (destroys data)
#   svc.sh ls                     # list only agezt services
#   svc.sh logs <name> [lines]    # tail logs (default 50)
#   svc.sh ip   <name>            # container IP + published ports
set -e

LABEL="agezt.service=1"
PREFIX="agezt-"

die() { echo "svc: $*" >&2; exit 1; }
need_docker() { command -v docker >/dev/null 2>&1 || die "docker not found (install it first)"; }
cname() { echo "${PREFIX}$1"; }

cmd="${1:-}"
[ -n "$cmd" ] || die "usage: up|down|nuke|ls|logs|ip <name> ..."
need_docker

case "$cmd" in
  up)
    name="${2:-}"; image="${3:-}"
    [ -n "$name" ] && [ -n "$image" ] || die "up needs <name> <image>"
    shift 3
    c="$(cname "$name")"
    # Idempotent: already running -> leave it; exists stopped -> start it.
    state="$(docker inspect -f '{{.State.Running}}' "$c" 2>/dev/null || echo missing)"
    if [ "$state" = "true" ]; then
      echo "$c already running"; exit 0
    fi
    if [ "$state" = "false" ]; then
      echo "starting existing $c"; docker start "$c" >/dev/null; exit 0
    fi
    echo "creating $c from $image"
    # shellcheck disable=SC2068
    docker run -d --name "$c" --label "$LABEL" --restart unless-stopped $@ "$image" >/dev/null
    echo "$c up"
    ;;
  down)
    name="${2:-}"; [ -n "$name" ] || die "down needs <name>"
    c="$(cname "$name")"
    docker rm -f "$c" >/dev/null 2>&1 && echo "$c removed" || echo "$c not present"
    ;;
  nuke)
    name="${2:-}"; [ -n "$name" ] || die "nuke needs <name>"
    c="$(cname "$name")"
    vols="$(docker inspect -f '{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}} {{end}}{{end}}' "$c" 2>/dev/null || true)"
    docker rm -f "$c" >/dev/null 2>&1 && echo "$c removed" || echo "$c not present"
    for v in $vols; do docker volume rm "$v" >/dev/null 2>&1 && echo "volume $v removed" || true; done
    ;;
  ls)
    docker ps -a --filter "label=$LABEL" \
      --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}\t{{.Image}}'
    ;;
  logs)
    name="${2:-}"; [ -n "$name" ] || die "logs needs <name>"
    lines="${3:-50}"
    docker logs --tail "$lines" "$(cname "$name")"
    ;;
  ip)
    name="${2:-}"; [ -n "$name" ] || die "ip needs <name>"
    c="$(cname "$name")"
    echo "addr: $(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}' "$c")"
    echo "ports: $(docker port "$c" 2>/dev/null | tr '\n' ' ')"
    ;;
  *)
    die "unknown command: $cmd (up|down|nuke|ls|logs|ip)"
    ;;
esac
