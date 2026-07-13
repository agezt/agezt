#!/usr/bin/env bash
# Retry a Go command that intermittently fails because of the self-hosted WSL
# runners' flaky tmpfs `compile` binary. Even after staging GOROOT to a
# per-runner tmpfs path, a full parallel `go build`/`go test` occasionally
# trips "fork/exec .../compile: invalid argument" /
# "src/log/internal/: invalid argument" mid-build. The single-package probe in
# setup-go-safe passes, so the corruption only surfaces under the many
# concurrent compiler execs of a real build — and it is transient, so a
# re-run almost always succeeds.
#
# A genuine, deterministic failure (real test/build error) still fails every
# attempt and the job fails — this only papers over the transient toolchain
# corruption, exactly like the long-standing govulncheck retry.
#
# Before each retry we also nuke the tmpfs Go cache and temp dirs. The
# corruption leaves stale/corrupt compiled artifacts in GOCACHE that can
# poison the next attempt before it even starts; clearing them gives the
# retry a clean slate.
#
# Usage: scripts/ci-go-retry.sh go test ./...
set +e
max="${CI_RETRY_MAX:-5}"
n=0
while :; do
  "$@"
  rc=$?
  [ "$rc" -eq 0 ] && exit 0
  n=$((n + 1))
  if [ "$n" -ge "$max" ]; then
    echo "::error::command failed after $max attempts (rc=$rc): $*" >&2
    exit "$rc"
  fi
  echo "attempt $n/$max failed (rc=$rc; flaky WSL Go compiler on self-hosted runners); retrying..." >&2
  # Clear tmpfs Go cache + temp dirs so the corruption doesn't poison the retry.
  rm -rf /dev/shm/gocache-* /dev/shm/gotmp-* 2>/dev/null || true
  sleep 3
done
