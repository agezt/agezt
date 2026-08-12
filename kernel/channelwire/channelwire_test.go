// SPDX-License-Identifier: MIT

package channelwire

import (
	"context"
	"sort"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

type fakeChannel struct{ name string }

func (f fakeChannel) Name() string                                     { return f.name }
func (f fakeChannel) Start(ctx context.Context) error                  { <-ctx.Done(); return ctx.Err() }
func (f fakeChannel) Send(_ context.Context, _ channel.Outbound) error { return nil }

// registeredKinds lists the registered factory kinds, sorted. It lives in the
// test file on purpose: it was an exported Kinds() in channelwire.go until
// 2026-08-12, but no binary ever called it — only this test did, so
// deadcodecheck flagged it every run. Keeping a test-only accessor in the
// production file would have meant an allowlist entry to hide a true report;
// moving it here says what it actually is.
func registeredKinds() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(factories))
	for k := range factories {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestRegisterLookupKinds(t *testing.T) {
	Register("wiretest-a", func(Deps) Built { return NotConfigured })
	Register("wiretest-b", func(Deps) Built { return NotConfigured })
	if _, ok := Lookup("wiretest-a"); !ok {
		t.Fatal("registered factory not found")
	}
	if _, ok := Lookup("wiretest-nope"); ok {
		t.Fatal("unregistered factory found")
	}
	seen := map[string]bool{}
	for _, k := range registeredKinds() {
		seen[k] = true
	}
	if !seen["wiretest-a"] || !seen["wiretest-b"] {
		t.Fatalf("registeredKinds() missing test kinds: %v", registeredKinds())
	}
}

func TestBuildKind_NotConfiguredSkips(t *testing.T) {
	Register("wiretest-off", func(Deps) Built { return NotConfigured })
	insts := BuildKind(context.Background(), "wiretest-off", nil, nil)
	if len(insts) != 0 {
		t.Fatalf("unconfigured kind must yield no instances, got %d", len(insts))
	}
}

func TestBuildKind_SingleChannel(t *testing.T) {
	Register("wiretest-on", func(d Deps) Built {
		return Built{
			Channels: []channel.Channel{fakeChannel{name: "wiretest-on"}},
			Desc:     "1 chat allowed",
		}
	})
	insts := BuildKind(context.Background(), "wiretest-on", nil, nil)
	if len(insts) != 1 {
		t.Fatalf("want 1 instance, got %d", len(insts))
	}
	in := insts[0]
	if in.Key != channel.InstanceKey("wiretest-on", "") {
		t.Fatalf("wrong key: %s", in.Key)
	}
	if in.Desc != "1 chat allowed" || in.Kind != "wiretest-on" || in.Label != "" {
		t.Fatalf("instance fields wrong: %+v", in)
	}
}

func TestBuildKind_MultiChannelKeysByName(t *testing.T) {
	// The push-family shape: one factory, several channels, each addressable
	// by its own Name().
	Register("wiretest-push", func(d Deps) Built {
		return Built{
			Channels: []channel.Channel{
				fakeChannel{name: "wiretest-ntfy"},
				fakeChannel{name: "wiretest-gotify"},
			},
			Desc: "2 push targets",
		}
	})
	insts := BuildKind(context.Background(), "wiretest-push", nil, nil)
	if len(insts) != 2 {
		t.Fatalf("want 2 instances, got %d", len(insts))
	}
	if insts[0].Key != channel.InstanceKey("wiretest-ntfy", "") {
		t.Fatalf("multi-channel first key should use channel name: %s", insts[0].Key)
	}
	if insts[1].Key != channel.InstanceKey("wiretest-gotify", "") {
		t.Fatalf("multi-channel extra key should use channel name: %s", insts[1].Key)
	}
}

// The "manifest with no factory" case is deliberately NOT tested here any
// more. It used to be, against a synthetic wiretest manifest and a
// MissingFactories() helper that no binary called. The assertion that matters
// runs in plugins/builtinchannels: TestEveryManifestHasFactoryOrTODO checks
// every REAL manifest, and catches the inverse too (a factoryTODO entry whose
// manifest vanished). A fixture-only copy here added no coverage and kept a
// production function alive for a test's sake.
