// SPDX-License-Identifier: MIT

package dial

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/controlplane"
)

// New resolves the runtime base dir and dials the daemon. It is
// the workhorse for every agt command that talks to the control
// plane. On failure (daemon not started, or stale socket left
// by a crashed daemon), it prints the same actionable hint the
// pre-extraction code did and returns nil so the caller can
// `return 1` uniformly.
func New(stderr io.Writer) *controlplane.Client {
	base, err := paths.BaseDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		return nil
	}
	return NewAtBase(base, stderr)
}

// NewAtBase builds a control-plane client for base and verifies
// the daemon is actually reachable (M239). Two failure modes
// give the same actionable hint instead of two different
// errors: a missing runtime addr (the daemon was never
// started) and a recorded-but-unreachable one (a stale socket
// the daemon left when it crashed) — the latter previously
// surfaced only as a cryptic "connection refused" on each
// command's own call. A server-side rejection (e.g. a bad
// token) is NOT treated as unreachable: the daemon is alive,
// so the client is returned and the command surfaces the real
// error.
func NewAtBase(base string, stderr io.Writer) *controlplane.Client {
	c, err := controlplane.NewClient(base)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", brand.CLI, err)
		fmt.Fprintf(stderr, "Hint: start the daemon with `%s` in another terminal.\n", brand.Binary)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, controlplane.CmdStatus, nil); err != nil {
		var serr *controlplane.ErrServerError
		if !errors.As(err, &serr) {
			fmt.Fprintf(stderr, "%s: daemon recorded but not responding — a stale socket from a crash?\n", brand.CLI)
			fmt.Fprintf(stderr, "Hint: (re)start the daemon with `%s`.\n", brand.Binary)
			return nil
		}
	}
	return c
}
