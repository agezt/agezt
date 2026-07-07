// SPDX-License-Identifier: MIT

package webhook

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

func TestLoopbackHost_Localhost(t *testing.T) {
	if !loopbackHost("localhost") {
		t.Error("loopbackHost('localhost') should be true")
	}
	if !loopbackHost("LOCALHOST") {
		t.Error("loopbackHost('LOCALHOST') should be true (case-insensitive)")
	}
}

func TestLoopbackHost_IP(t *testing.T) {
	if !loopbackHost("127.0.0.1") {
		t.Error("loopbackHost('127.0.0.1') should be true")
	}
	if !loopbackHost("::1") {
		t.Error("loopbackHost('::1') should be true")
	}
	if !loopbackHost("127.0.0.2") {
		t.Error("loopbackHost('127.0.0.2') should be true (loopback subnet)")
	}
}

func TestLoopbackHost_NonLoopback(t *testing.T) {
	if loopbackHost("example.com") {
		t.Error("loopbackHost('example.com') should be false")
	}
	if loopbackHost("192.168.1.1") {
		t.Error("loopbackHost('192.168.1.1') should be false")
	}
}

func TestVerb_StripsPrefix(t *testing.T) {
	if got := verb(event.KindToolInvoked); got != "invoked" {
		t.Errorf("verb('tool.invoked') = %q, want 'invoked'", got)
	}
}

func TestVerb_NoDot(t *testing.T) {
	if got := verb(event.KindHalt); got != "halt" {
		t.Errorf("verb('halt') = %q, want 'halt'", got)
	}
}

func TestBackoff_Default(t *testing.T) {
	d := &Dispatcher{Backoff: nil}
	if d.backoff(1) != 250*time.Millisecond {
		t.Errorf("backoff(1) = %v, want 250ms", d.backoff(1))
	}
	if d.backoff(3) != 750*time.Millisecond {
		t.Errorf("backoff(3) = %v, want 750ms", d.backoff(3))
	}
}

func TestBackoff_Custom(t *testing.T) {
	d := &Dispatcher{Backoff: func(attempt int) time.Duration {
		return time.Duration(attempt) * time.Second
	}}
	if d.backoff(2) != 2*time.Second {
		t.Errorf("custom backoff(2) = %v, want 2s", d.backoff(2))
	}
}
