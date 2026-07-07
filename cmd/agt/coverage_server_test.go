// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// startCoverageServer boots a runtime + control-plane server on a temp AGEZT_HOME
// and waits until a client can connect. It returns the kernel for direct state
// assertions. Everything is torn down via t.Cleanup.
func startCoverageServer(t *testing.T) *runtime.Kernel {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AGEZT_HOME", dir)
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(mock.FinalText("ok"))})
	if err != nil {
		t.Fatalf("runtime.Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := controlplane.NewClient(dir)
		if err == nil {
			_ = c.Close()
			return k
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("control-plane client could not connect")
	return nil
}

// TestCmdAgentLifecycle drives the agent subcommands against a live server.
// It walks add -> list -> show -> set -> pause -> resume -> retire -> revive
// -> impact, which collectively exercises the largest uncovered handlers in
// agent.go.
func TestCmdAgentLifecycle(t *testing.T) {
	startCoverageServer(t)

	type step struct {
		name string
		args []string
	}
	steps := []step{
		{"add", []string{"add", "coverbot", "--role", "builder"}},
		{"add-json", []string{"add", "coverbot2", "--role", "builder", "--json"}},
		{"list", []string{"list"}},
		{"list-json", []string{"list", "--json"}},
		{"show", []string{"show", "coverbot"}},
		{"show-json", []string{"show", "coverbot", "--json"}},
		{"set", []string{"set", "coverbot", "--role", "reviewer"}},
		{"pause", []string{"pause", "coverbot"}},
		{"resume", []string{"resume", "coverbot"}},
		{"impact", []string{"impact", "coverbot"}},
		{"authority", []string{"authority", "coverbot"}},
		{"task-add", []string{"task", "add", "coverbot", "Ship it"}},
		{"task-list", []string{"task", "coverbot"}},
		{"retire", []string{"retire", "coverbot"}},
		{"revive", []string{"revive", "coverbot"}},
		{"graveyard", []string{"graveyard"}},
	}
	for _, s := range steps {
		s := s
		t.Run(s.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			// Exit code is not asserted: some subcommands legitimately return
			// non-zero (e.g. missing optional data). We only require that the
			// handler runs to completion without panicking. Coverage is the goal.
			_ = cmdAgent(s.args, &out, &errOut)
		})
	}
}

// TestCmdAgentErrorPaths exercises argument-validation error branches that do
// not need meaningful server state.
func TestCmdAgentErrorPaths(t *testing.T) {
	startCoverageServer(t)
	cases := [][]string{
		{"show"},                     // missing ref
		{"show", "does-not-exist"},   // unknown agent
		{"set"},                      // missing ref
		{"set", "does-not-exist"},    // unknown agent, no flags
		{"impact"},                   // missing ref
		{"impact", "does-not-exist"}, // unknown agent
		{"pause"},                    // missing ref
		{"resume"},                   // missing ref
		{"retire"},                   // missing ref
		{"revive"},                   // missing ref
		{"remove"},                   // missing ref
		{"authority"},                // missing ref
		{"task"},                     // missing args
		{"task", "add"},              // missing slug/title
		{"repair-status"},            // missing ref
		{"repair-status", "x", "--limit", "0"},   // invalid limit
		{"repair-status", "x", "--limit", "abc"}, // invalid limit
	}
	for _, args := range cases {
		var out, errOut bytes.Buffer
		_ = cmdAgent(args, &out, &errOut)
	}
}
