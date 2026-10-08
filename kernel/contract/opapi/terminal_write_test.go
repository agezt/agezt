// SPDX-License-Identifier: MIT
package opapi

import (
	"context"
	"testing"
)

type writeProbe struct {
	accepted bool
	callback func()
}

func (p *writeProbe) After(callback func()) bool {
	if !p.accepted {
		return false
	}
	p.callback = callback
	return true
}
func TestTerminalWriteOptionalOwnershipAndContext(t *testing.T) {
	type key struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "owned"))
	cancel()
	if AfterTerminalWrite(parent, func() { t.Error("absent callback") }) || AfterTerminalWrite(parent, nil) {
		t.Fatal("absent ownership")
	}
	p := &writeProbe{accepted: true}
	ctx := WithTerminalWrite(parent, p)
	called := 0
	if ctx.Value(key{}) != "owned" || ctx.Err() != context.Canceled || !AfterTerminalWrite(ctx, func() { called++ }) || called != 0 || p.callback == nil {
		t.Fatal("ownership/context changed")
	}
	p.callback()
	if called != 1 {
		t.Fatal(called)
	}
	p.accepted = false
	if AfterTerminalWrite(ctx, func() {}) {
		t.Fatal("rejection lost")
	}
	if AfterTerminalWrite(WithTerminalWrite(parent, nil), func() {}) {
		t.Fatal("nil owner")
	}
}
