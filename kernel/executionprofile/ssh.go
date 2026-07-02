// SPDX-License-Identifier: MIT

package executionprofile

import (
	"context"
	"os"
	"strings"
)

type SSHConfig struct {
	Enabled               bool
	Target                string
	WorkDir               string
	IdentityFile          string
	Port                  string
	StrictHostKeyChecking string
}

func SSHConfigFromEnv() SSHConfig {
	on := strings.ToLower(strings.TrimSpace(os.Getenv("AGEZT_EXEC_SSH")))
	cfg := SSHConfig{
		Enabled:               on == "1" || on == "true" || on == "yes" || on == "on",
		Target:                strings.TrimSpace(os.Getenv("AGEZT_EXEC_SSH_TARGET")),
		WorkDir:               strings.TrimSpace(os.Getenv("AGEZT_EXEC_SSH_WORKDIR")),
		IdentityFile:          strings.TrimSpace(os.Getenv("AGEZT_EXEC_SSH_IDENTITY")),
		Port:                  strings.TrimSpace(os.Getenv("AGEZT_EXEC_SSH_PORT")),
		StrictHostKeyChecking: strings.TrimSpace(os.Getenv("AGEZT_EXEC_SSH_STRICT_HOST_KEY")),
	}
	return cfg
}

func (c SSHConfig) Active() bool {
	return c.Enabled && strings.TrimSpace(c.Target) != ""
}

func (c SSHConfig) Args() []string {
	args := c.clientArgs(false)
	args = append(args, c.Target)
	return args
}

func (c SSHConfig) SCPToArgv(localPath, remotePath string) []string {
	args := []string{"scp"}
	args = append(args, c.clientArgs(true)...)
	args = append(args, "-r", localPath, c.Target+":"+ShellQuote(remotePath))
	return args
}

func (c SSHConfig) SCPFromArgv(remotePath, localPath string) []string {
	args := []string{"scp"}
	args = append(args, c.clientArgs(true)...)
	args = append(args, "-r", c.Target+":"+ShellQuote(remotePath), localPath)
	return args
}

func (c SSHConfig) CommandArgv(command string) []string {
	argv := []string{"ssh"}
	argv = append(argv, c.Args()...)
	argv = append(argv, "sh -lc "+ShellQuote(command))
	return argv
}

func (c SSHConfig) ShellCommandArgv(command string) []string {
	command = strings.TrimSpace(command)
	if strings.TrimSpace(c.WorkDir) != "" {
		command = "cd " + ShellQuote(c.WorkDir) + " && " + command
	}
	return c.CommandArgv(command)
}

func (c SSHConfig) clientArgs(scp bool) []string {
	args := []string{"-o", "BatchMode=yes"}
	if c.StrictHostKeyChecking != "" {
		args = append(args, "-o", "StrictHostKeyChecking="+c.StrictHostKeyChecking)
	}
	if c.IdentityFile != "" {
		args = append(args, "-i", c.IdentityFile)
	}
	if c.Port != "" {
		if scp {
			args = append(args, "-P", c.Port)
		} else {
			args = append(args, "-p", c.Port)
		}
	}
	return args
}

func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

type ctxKey int

const ctxKeySSHOverride ctxKey = iota

func WithSSHOverride(ctx context.Context, cfg SSHConfig) context.Context {
	if !cfg.Active() {
		return ctx
	}
	return context.WithValue(ctx, ctxKeySSHOverride, cfg)
}

func SSHOverrideFrom(ctx context.Context) (SSHConfig, bool) {
	cfg, ok := ctx.Value(ctxKeySSHOverride).(SSHConfig)
	if !ok || !cfg.Active() {
		return SSHConfig{}, false
	}
	return cfg, true
}
