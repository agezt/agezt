// SPDX-License-Identifier: MIT

package executionprofile

import (
	"context"
	"os"
	"strings"
)

type K8sConfig struct {
	Enabled   bool
	Context   string
	Namespace string
	Pod       string
	Container string
	WorkDir   string
}

func K8sConfigFromEnv() K8sConfig {
	on := strings.ToLower(strings.TrimSpace(os.Getenv("AGEZT_EXEC_K8S")))
	return K8sConfig{
		Enabled:   on == "1" || on == "true" || on == "yes" || on == "on",
		Context:   strings.TrimSpace(os.Getenv("AGEZT_EXEC_K8S_CONTEXT")),
		Namespace: strings.TrimSpace(os.Getenv("AGEZT_EXEC_K8S_NAMESPACE")),
		Pod:       strings.TrimSpace(os.Getenv("AGEZT_EXEC_K8S_POD")),
		Container: strings.TrimSpace(os.Getenv("AGEZT_EXEC_K8S_CONTAINER")),
		WorkDir:   strings.TrimSpace(os.Getenv("AGEZT_EXEC_K8S_WORKDIR")),
	}
}

func (c K8sConfig) Active() bool {
	return c.Enabled && strings.TrimSpace(c.Pod) != ""
}

func (c K8sConfig) Args() []string {
	args := []string{"kubectl"}
	if c.Context != "" {
		args = append(args, "--context", c.Context)
	}
	if c.Namespace != "" {
		args = append(args, "-n", c.Namespace)
	}
	return args
}

func (c K8sConfig) ShellCommandArgv(command string) []string {
	command = strings.TrimSpace(command)
	if c.WorkDir != "" {
		command = "cd " + ShellQuote(c.WorkDir) + " && " + command
	}
	return c.CommandArgv(command)
}

func (c K8sConfig) CommandArgv(command string) []string {
	args := c.Args()
	args = append(args, "exec", c.Pod)
	if c.Container != "" {
		args = append(args, "-c", c.Container)
	}
	args = append(args, "--", "sh", "-lc", command)
	return args
}

func (c K8sConfig) CopyToArgv(localPath, remotePath string) []string {
	args := c.Args()
	args = append(args, "cp", localPath, c.Pod+":"+remotePath)
	if c.Container != "" {
		args = append(args, "-c", c.Container)
	}
	return args
}

func (c K8sConfig) CopyFromArgv(remotePath, localPath string) []string {
	args := c.Args()
	args = append(args, "cp", c.Pod+":"+remotePath, localPath)
	if c.Container != "" {
		args = append(args, "-c", c.Container)
	}
	return args
}

const ctxKeyK8sOverride ctxKey = iota + 100

func WithK8sOverride(ctx context.Context, cfg K8sConfig) context.Context {
	if !cfg.Active() {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyK8sOverride, cfg)
}

func K8sOverrideFrom(ctx context.Context) (K8sConfig, bool) {
	cfg, ok := ctx.Value(ctxKeyK8sOverride).(K8sConfig)
	if !ok || !cfg.Active() {
		return K8sConfig{}, false
	}
	return cfg, true
}
