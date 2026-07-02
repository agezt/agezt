// SPDX-License-Identifier: MIT

package executionprofile

import (
	"context"
	"os"
	"strconv"
	"strings"
)

type DaytonaConfig struct {
	Enabled bool
	Sandbox string
	WorkDir string
}

func DaytonaConfigFromEnv() DaytonaConfig {
	on := strings.ToLower(strings.TrimSpace(os.Getenv("AGEZT_EXEC_DAYTONA")))
	return DaytonaConfig{
		Enabled: on == "1" || on == "true" || on == "yes" || on == "on",
		Sandbox: strings.TrimSpace(os.Getenv("AGEZT_EXEC_DAYTONA_SANDBOX")),
		WorkDir: strings.TrimSpace(os.Getenv("AGEZT_EXEC_DAYTONA_WORKDIR")),
	}
}

func (c DaytonaConfig) Active() bool {
	return c.Enabled && strings.TrimSpace(c.Sandbox) != ""
}

func (c DaytonaConfig) ShellCommandArgv(command string, timeoutSeconds int) []string {
	return c.CommandArgv(command, c.WorkDir, timeoutSeconds)
}

func (c DaytonaConfig) CommandArgv(command, workDir string, timeoutSeconds int) []string {
	args := []string{"daytona", "exec", c.Sandbox}
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		workDir = strings.TrimSpace(c.WorkDir)
	}
	if workDir != "" {
		args = append(args, "--cwd", workDir)
	}
	if timeoutSeconds > 0 {
		args = append(args, "--timeout", strconv.Itoa(timeoutSeconds))
	}
	args = append(args, "--", "sh", "-lc", strings.TrimSpace(command))
	return args
}

const ctxKeyDaytonaOverride ctxKey = iota + 300

func WithDaytonaOverride(ctx context.Context, cfg DaytonaConfig) context.Context {
	if !cfg.Active() {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyDaytonaOverride, cfg)
}

func DaytonaOverrideFrom(ctx context.Context) (DaytonaConfig, bool) {
	cfg, ok := ctx.Value(ctxKeyDaytonaOverride).(DaytonaConfig)
	if !ok || !cfg.Active() {
		return DaytonaConfig{}, false
	}
	return cfg, true
}
