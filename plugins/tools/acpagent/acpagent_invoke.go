// SPDX-License-Identifier: MIT
//
// acpagent Tool heavy lifting: Invoke (the JSON-RPC driver) + spawnAgent
// (the subprocess spawner) + DefaultTimeout + MaxOutputBytes consts.
// Extracted from acpagent.go during Day 211 god-file refactor (#69).
// Public API unchanged.
package acpagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/acp"
	"github.com/agezt/agezt/kernel/acpcatalog"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/envscrub"
)

// DefaultTimeout caps one delegated ACP session.
const DefaultTimeout = 5 * time.Minute

// MaxOutputBytes truncates the relayed answer so a runaway agent can't blow the
// context budget.
const MaxOutputBytes = 60 * 1024

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in struct {
		Task  string `json:"task"`
		Agent string `json:"agent"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	task := strings.TrimSpace(in.Task)
	if task == "" {
		return agent.Result{Output: "task is required", IsError: true}, nil
	}
	// Resolve which ACP agent to drive: an explicit `agent` slug (must be
	// installed) wins; otherwise the configured default command (t.Cmd).
	cmd, ok := acpcatalog.ResolveCommand(in.Agent, t.Cmd)
	if !ok {
		installed := acpcatalog.InstalledSlugs()
		hint := "set AGEZT_ACP_AGENT_CMD or install an ACP agent"
		if len(installed) > 0 {
			hint = "available installed agents: " + strings.Join(installed, ", ")
		} else if strings.TrimSpace(in.Agent) != "" {
			hint = "agent \"" + strings.TrimSpace(in.Agent) + "\" is not installed; " + hint
		}
		return agent.Result{Output: "no ACP agent to delegate to (" + hint + ")", IsError: true}, nil
	}

	to := t.Timeout
	if to <= 0 {
		to = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	tr, err := t.dial(ctx, cmd, t.Cwd)
	if err != nil {
		return agent.Result{Output: "spawn ACP agent failed: " + err.Error(), IsError: true}, nil
	}
	defer func() { _ = tr.close() }()

	// A blocking ACP read (Initialize/NewSession/Prompt) does not observe
	// ctx cancellation on its own — the agent is spawned with exec.Command,
	// not CommandContext. Tear the transport down when ctx fires (the 5-min
	// timeout above, or a caller cancel) so a silent or wedged external agent
	// can't hold this call open past the deadline. close() is idempotent, so
	// the deferred close is a no-op after this one; the watcher always wakes
	// (cancel runs on return) and exits, so it does not leak.
	go func() {
		<-ctx.Done()
		_ = tr.close()
	}()

	client := acp.NewClient(tr.out, tr.in)
	if err := client.Initialize(ctx); err != nil {
		return agent.Result{Output: "ACP initialize failed: " + err.Error(), IsError: true}, nil
	}
	cwd := t.Cwd
	if cwd == "" {
		cwd = "."
	}
	sid, err := client.NewSession(ctx, cwd)
	if err != nil {
		return agent.Result{Output: "ACP session/new failed: " + err.Error(), IsError: true}, nil
	}

	var answer strings.Builder
	stop, err := client.Prompt(ctx, sid, task, func(chunk string) {
		// Bound the in-memory accumulation: the result is truncated to
		// MaxOutputBytes anyway, so a runaway agent streaming without end can't
		// grow this without limit and OOM the daemon (M256). Overshoot is at most
		// one message; whole chunks are appended so no UTF-8 rune is split.
		if answer.Len() >= MaxOutputBytes {
			return
		}
		answer.WriteString(chunk)
	})
	if err != nil {
		return agent.Result{Output: "ACP session/prompt failed: " + err.Error() + render(answer.String(), ""), IsError: true}, nil
	}
	return agent.Result{Output: render(answer.String(), stop)}, nil
}
func spawnAgent(ctx context.Context, cmdStr, cwd string) (*transport, error) {
	shell, arg := platformShell()
	c := exec.Command(shell, arg, cmdStr) // not CommandContext: we manage teardown via close()
	c.Env = envscrub.Scrubbed()
	if cwd != "" {
		c.Dir = cwd
	}
	c.Stderr = os.Stderr
	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := c.Start(); err != nil {
		return nil, err
	}
	// close is idempotent (sync.Once): the Invoke deferred close and the
	// ctx-cancel watcher may both call it. The post-kill wait is bounded so
	// an un-reapable child (descendants holding the pipe, a stuck Wait) can't
	// pin the caller forever.
	var once sync.Once
	var closeErr error
	return &transport{
		out: stdout,
		in:  stdin,
		close: func() error {
			once.Do(func() {
				_ = stdin.Close() // EOF → graceful exit for a well-behaved agent
				done := make(chan error, 1)
				go func() { done <- c.Wait() }()
				select {
				case closeErr = <-done:
				case <-time.After(5 * time.Second):
					_ = c.Process.Kill()
					select {
					case closeErr = <-done:
					case <-time.After(5 * time.Second):
						closeErr = fmt.Errorf("acpagent: process did not exit after kill")
					}
				}
			})
			return closeErr
		},
	}, nil
}
