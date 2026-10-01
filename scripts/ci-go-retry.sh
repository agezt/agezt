#!/usr/bin/env bash
# Retry a Go command that intermittently fails. The original reason was the
# self-hosted WSL runners' flaky tmpfs `compile` binary; since 2026-10-01 all
# jobs run on GitHub-hosted runners, which do not stage to tmpfs at all, so the
# tmpfs cleanup below is normally a no-op and this is a plain retry wrapper for
# transient failures. It stays because every Go step in ci.yml calls it. Even
# when staging did apply, a full parallel `go build`/`go test` occasionally
# tripped "fork/exec .../compile: invalid argument" /
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
  echo "attempt $n/$max failed (rc=$rc); retrying..." >&2
  # Clear tmpfs Go cache + temp dirs so the corruption doesn't poison the retry.
  rm -rf /dev/shm/gocache-* /dev/shm/gotmp-* 2>/dev/null || true
  # Re-stage GOROOT from the original ext4 source to get a fresh tmpfs copy.
  # The setup-go-safe action stores the pre-tmpfs path in GOROOT_SRC; if set,
  # the tmpfs GOROOT (pointed to by $GOROOT) may have corrupt compiler binaries.
  if [ -n "${GOROOT_SRC:-}" ] && [ -n "${GOROOT:-}" ] && [ -d "$GOROOT_SRC" ]; then
    rm -rf "$GOROOT"
    cp -a "$GOROOT_SRC" "$GOROOT"
    echo "re-staged GOROOT from $GOROOT_SRC to $GOROOT" >&2
  fi
  # The rm -rf above also deleted the staged GOCACHE/GOTMPDIR that the setup
  # step created; recreate them or every retry dies with "creating work dir: ...
  # no such file or directory" before the command even runs. Observed on the
  # GitHub-hosted runners, 2026-09-07, while every job was still staging to
  # /dev/shm (setup-go-safe's hosted check tested the wrong variable and never
  # fired; it now tests RUNNER_ENVIRONMENT and does not stage here at all).
  [ -n "${GOCACHE:-}" ] && mkdir -p "$GOCACHE"
  [ -n "${GOTMPDIR:-}" ] && mkdir -p "$GOTMPDIR"
  sleep 3
done
