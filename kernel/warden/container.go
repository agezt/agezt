// SPDX-License-Identifier: MIT

package warden

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

const (
	defaultContainerRuntime = "docker"
	defaultContainerImage   = "python:3.12-slim"
	defaultContainerNetwork = "none"
	containerWorkDir        = "/workspace"
)

// Options configures optional warden backends. The zero value preserves the
// historical behavior: ProfileContainer is requested/audited but downgraded to
// the strongest built-in host profile.
type Options struct {
	Container ContainerOptions
}

// ContainerOptions enables an OCI runtime backend for ProfileContainer. AGEZT
// launches the runtime CLI (`docker` or `podman`) with a one-shot `run --rm`,
// mounts the requested WorkDir at /workspace, forwards only Spec.Env, and keeps
// Warden's timeout/output/audit handling around the runtime process.
type ContainerOptions struct {
	Enabled bool
	Runtime string
	Image   string
	Network string
}

func normalizeContainerOptions(o ContainerOptions) ContainerOptions {
	o.Runtime = strings.TrimSpace(o.Runtime)
	if o.Runtime == "" {
		o.Runtime = defaultContainerRuntime
	}
	o.Image = strings.TrimSpace(o.Image)
	if o.Image == "" {
		o.Image = defaultContainerImage
	}
	o.Network = strings.TrimSpace(o.Network)
	if o.Network == "" {
		o.Network = defaultContainerNetwork
	}
	return o
}

func (o ContainerOptions) active() bool {
	return o.Enabled && strings.TrimSpace(o.Runtime) != "" && strings.TrimSpace(o.Image) != ""
}

func buildContainerArgv(spec Spec, opts ContainerOptions) ([]string, error) {
	opts = normalizeContainerOptions(opts)
	if !opts.active() {
		return nil, fmt.Errorf("container backend is not enabled")
	}
	inner, err := containerInnerArgv(spec)
	if err != nil {
		return nil, err
	}
	argv := []string{opts.Runtime, "run", "--rm"}
	if opts.Network != "" {
		argv = append(argv, "--network", opts.Network)
	}
	if spec.WorkDir != "" {
		abs, err := filepath.Abs(spec.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("resolve container workdir: %w", err)
		}
		argv = append(argv, "-v", abs+":"+containerWorkDir, "-w", containerWorkDir)
	}
	for _, env := range spec.Env {
		if containerEnvOK(env) {
			argv = append(argv, "-e", env)
		}
	}
	if spec.Limits.AddressSpaceBytes > 0 {
		argv = append(argv, "--memory", fmt.Sprintf("%d", spec.Limits.AddressSpaceBytes))
	}
	argv = append(argv, opts.Image)
	argv = append(argv, inner...)
	return argv, nil
}

func containerEnvOK(v string) bool {
	if v == "" || strings.HasPrefix(v, "=") {
		return false
	}
	name, _, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(name) == "" {
		return false
	}
	return !strings.ContainsAny(name, "\x00\r\n")
}

func containerInnerArgv(spec Spec) ([]string, error) {
	argv := spec.Argv
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil, fmt.Errorf("%w: empty Argv", ErrBadSpec)
	}
	if cmd, ok := shellCommandArg(argv); ok {
		return []string{"sh", "-lc", cmd}, nil
	}
	out := append([]string(nil), argv...)
	out[0] = containerProgramName(out[0])
	for i := 1; i < len(out); i++ {
		out[i] = containerPathArg(out[i], spec.WorkDir)
	}
	return out, nil
}

func containerPathArg(arg, workDir string) string {
	if workDir == "" || arg == "" {
		return arg
	}
	if key, val, ok := strings.Cut(arg, "="); ok {
		if mapped, changed := containerPathValue(val, workDir); changed {
			return key + "=" + mapped
		}
		return arg
	}
	if mapped, changed := containerPathValue(arg, workDir); changed {
		return mapped
	}
	return arg
}

func containerPathValue(v, workDir string) (string, bool) {
	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return v, false
	}
	absValue, err := filepath.Abs(v)
	if err != nil {
		return v, false
	}
	rel, err := filepath.Rel(absWork, absValue)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return v, false
	}
	if rel == "." {
		return containerWorkDir, true
	}
	return path.Join(containerWorkDir, filepath.ToSlash(rel)), true
}

func shellCommandArg(argv []string) (string, bool) {
	if len(argv) < 3 {
		return "", false
	}
	prog := strings.ToLower(containerProgramName(argv[0]))
	switch prog {
	case "sh", "bash", "dash", "zsh":
		if argv[1] == "-c" {
			return argv[2], true
		}
	case "cmd", "cmd.exe":
		for i := 1; i < len(argv)-1; i++ {
			a := strings.ToLower(strings.TrimLeft(argv[i], "/-"))
			if a == "c" {
				return argv[i+1], true
			}
		}
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		for i := 1; i < len(argv)-1; i++ {
			a := strings.ToLower(strings.TrimLeft(argv[i], "-/"))
			if a == "command" || a == "c" {
				return argv[i+1], true
			}
		}
	}
	return "", false
}

func containerProgramName(v string) string {
	base := path.Base(strings.ReplaceAll(strings.TrimSpace(v), "\\", "/"))
	lower := strings.ToLower(base)
	switch {
	case lower == "python.exe" || strings.HasPrefix(lower, "python3") || strings.HasPrefix(lower, "python2"):
		return "python"
	case lower == "node.exe":
		return "node"
	case lower == "deno.exe":
		return "deno"
	}
	return base
}
