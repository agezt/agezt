// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)

// fakeStreamer records the args of the last Stream call and relays a canned
// answer, standing in for *controlplane.Client without a daemon.
type fakeStreamer struct {
	lastCmd  string
	lastArgs map[string]any
	answer   string
}

func (f *fakeStreamer) Stream(_ context.Context, cmd string, args map[string]any, _ func(*event.Event)) (map[string]any, error) {
	f.lastCmd = cmd
	f.lastArgs = args
	return map[string]any{"answer": f.answer}, nil
}

// With a tenant set, the ACP runner must forward it as the `tenant` run arg so
// the daemon routes the prompt to that tenant's isolated kernel (M14 Phase 6).
func TestACPRunner_ForwardsTenant(t *testing.T) {
	fs := &fakeStreamer{answer: "ok"}
	r := controlPlaneRunner{c: fs, tenant: "alpha"}

	ans, err := r.Prompt(context.Background(), "/cwd", "do a thing", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if ans != "ok" {
		t.Errorf("answer = %q, want ok", ans)
	}
	if fs.lastCmd != controlplane.CmdRun {
		t.Errorf("cmd = %q, want %q", fs.lastCmd, controlplane.CmdRun)
	}
	if fs.lastArgs["intent"] != "do a thing" {
		t.Errorf("intent = %v", fs.lastArgs["intent"])
	}
	if fs.lastArgs["tenant"] != "alpha" {
		t.Errorf("tenant = %v, want alpha", fs.lastArgs["tenant"])
	}
}

// Without a tenant, the `tenant` key must be absent entirely (byte-for-byte the
// prior single-tenant request — not an empty string the daemon would have to
// special-case).
func TestACPRunner_OmitsTenantWhenUnset(t *testing.T) {
	fs := &fakeStreamer{answer: "ok"}
	r := controlPlaneRunner{c: fs} // no tenant

	if _, err := r.Prompt(context.Background(), "/cwd", "hi", func(string) {}); err != nil {
		t.Fatal(err)
	}
	if _, present := fs.lastArgs["tenant"]; present {
		t.Errorf("tenant key must be absent when unset, got args=%v", fs.lastArgs)
	}
}
