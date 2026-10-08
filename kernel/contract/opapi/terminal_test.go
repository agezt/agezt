// SPDX-License-Identifier: MIT
package opapi

import (
	"context"
	"testing"
)

type terminalProbe struct {
	accepted bool
	cleanup  func()
}

func (p *terminalProbe) Defer(cleanup func()) bool { p.cleanup = cleanup; return p.accepted }

func TestTerminalCleanupOwnershipAndContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "owned")
	if DeferTerminalCleanup(ctx, func() {}) || DeferTerminalCleanup(ctx, nil) {
		t.Fatal("missing scope accepted")
	}
	p := &terminalProbe{accepted: true}
	ctx = WithTerminalCleanup(ctx, p)
	called := 0
	if ctx.Value(key{}) != "owned" || !DeferTerminalCleanup(ctx, func() { called++ }) || called != 0 || p.cleanup == nil {
		t.Fatal("ownership/context changed")
	}
	p.cleanup()
	if called != 1 {
		t.Fatal(called)
	}
	p.accepted = false
	if DeferTerminalCleanup(ctx, func() {}) {
		t.Fatal("rejected ownership accepted")
	}
	ctx = WithTerminalCleanup(ctx, nil)
	if DeferTerminalCleanup(ctx, func() {}) {
		t.Fatal("nil scope accepted")
	}
}
