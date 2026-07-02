// SPDX-License-Identifier: MIT

package executionprofile

import (
	"context"
	"os"
	"strings"
)

type ModalConfig struct {
	Enabled     bool
	Ref         string
	Image       string
	Environment string
	AddPython   string
	WorkDir     string
}

func ModalConfigFromEnv() ModalConfig {
	on := strings.ToLower(strings.TrimSpace(os.Getenv("AGEZT_EXEC_MODAL")))
	return ModalConfig{
		Enabled:     on == "1" || on == "true" || on == "yes" || on == "on",
		Ref:         strings.TrimSpace(os.Getenv("AGEZT_EXEC_MODAL_REF")),
		Image:       strings.TrimSpace(os.Getenv("AGEZT_EXEC_MODAL_IMAGE")),
		Environment: strings.TrimSpace(os.Getenv("AGEZT_EXEC_MODAL_ENVIRONMENT")),
		AddPython:   strings.TrimSpace(os.Getenv("AGEZT_EXEC_MODAL_ADD_PYTHON")),
		WorkDir:     strings.TrimSpace(os.Getenv("AGEZT_EXEC_MODAL_WORKDIR")),
	}
}

func (c ModalConfig) Active() bool {
	return c.Enabled
}

func (c ModalConfig) ShellCommandArgv(command string) []string {
	command = strings.TrimSpace(command)
	if c.WorkDir != "" {
		command = "cd " + ShellQuote(c.WorkDir) + " && " + command
	}
	args := []string{"modal", "shell"}
	if c.Environment != "" {
		args = append(args, "--env", c.Environment)
	}
	if c.Ref == "" {
		if c.Image != "" {
			args = append(args, "--image", c.Image)
		}
		if c.AddPython != "" {
			args = append(args, "--add-python", c.AddPython)
		}
	}
	if c.Ref != "" {
		args = append(args, c.Ref)
	}
	args = append(args, "--cmd", command, "--no-pty")
	return args
}

func (c ModalConfig) CodeExecArgv(localDir, command string) []string {
	args := []string{"modal", "shell"}
	if c.Environment != "" {
		args = append(args, "--env", c.Environment)
	}
	if c.Image != "" {
		args = append(args, "--image", c.Image)
	}
	if c.AddPython != "" {
		args = append(args, "--add-python", c.AddPython)
	}
	args = append(args, "--add-local", localDir, "--cmd", strings.TrimSpace(command), "--no-pty")
	return args
}

const ctxKeyModalOverride ctxKey = iota + 200

func WithModalOverride(ctx context.Context, cfg ModalConfig) context.Context {
	if !cfg.Active() {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyModalOverride, cfg)
}

func ModalOverrideFrom(ctx context.Context) (ModalConfig, bool) {
	cfg, ok := ctx.Value(ctxKeyModalOverride).(ModalConfig)
	if !ok || !cfg.Active() {
		return ModalConfig{}, false
	}
	return cfg, true
}
