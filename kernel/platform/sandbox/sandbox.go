// SPDX-License-Identifier: MIT

// Package sandbox is the process platform (architecture/20-target-architecture.md
// §5, layer L2): the one place a child process is built outside kernel/warden's
// run-to-completion engine.
//
// Go's os/exec treats a nil Cmd.Env as "inherit the parent's environment", and
// the daemon's environment holds every provider API key, channel token and the
// vault passphrase. Children built here never inherit: Command and
// CommandContext preset Env to IsolatedEnv, so a caller that forgets the
// environment gets the safe one. Before this package git (driven by the coding
// tool, so a hostile repository's hooks), toolbox installers (npm/pip
// postinstall scripts), tunnel binaries and version probes all started with
// the full daemon environment.
//
// Two policies, chosen by who drives the child:
//
//   - IsolatedEnv: children an agent can steer (git under the coding tool, ACP
//     agents, the browser driver, MCP servers). An allowlist of launch
//     variables (kernel/envscrub) plus explicit grants.
//   - HelperEnv: operator-triggered helper CLIs (toolbox install/detect,
//     tunnels, version probes). Everything except secret-shaped names, so
//     toolchain configuration (GOPATH, CARGO_HOME, NVM_DIR, ...) still works.
package sandbox

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/agezt/agezt/kernel/envscrub"
)

// Command is exec.Command with Env preset to IsolatedEnv().
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = IsolatedEnv()
	return cmd
}

// CommandContext is exec.CommandContext with Env preset to IsolatedEnv().
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = IsolatedEnv()
	return cmd
}

// IsolatedEnv is the environment for a child an agent can steer: the launch
// allowlist (envscrub.Scrubbed — no AGEZT_*, no secret-shaped names) plus the
// named grants copied from the daemon's environment, secret-shaped or not.
func IsolatedEnv(grants ...string) []string {
	return withGrants(envscrub.Scrubbed(), grants)
}

// HelperEnv is the environment for an operator-triggered helper CLI: the
// daemon's environment minus AGEZT_* and secret-shaped names, plus the named
// grants (for a helper that needs its own token, e.g. NGROK_AUTHTOKEN).
func HelperEnv(grants ...string) []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if ok && !envscrub.IsSecretName(name) {
			out = append(out, kv)
		}
	}
	return withGrants(out, grants)
}

func withGrants(base, grants []string) []string {
	for _, g := range grants {
		if v, ok := os.LookupEnv(g); ok {
			base = append(base, g+"="+v)
		}
	}
	return base
}
