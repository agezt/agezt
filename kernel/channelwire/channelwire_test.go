// SPDX-License-Identifier: MIT

package channelwire

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

type fakeChannel struct{ name string }

func (f fakeChannel) Name() string                                     { return f.name }
func (f fakeChannel) Start(ctx context.Context) error                  { <-ctx.Done(); return ctx.Err() }
func (f fakeChannel) Send(_ context.Context, _ channel.Outbound) error { return nil }

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
	for _, k := range Kinds() {
		seen[k] = true
	}
	if !seen["wiretest-a"] || !seen["wiretest-b"] {
		t.Fatalf("Kinds() missing test kinds: %v", Kinds())
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

func TestMissingFactories_ListsUnwiredManifests(t *testing.T) {
	channel.RegisterManifest(channel.Manifest{
		Kind: "wiretest-manifest-only", Display: "WireTest", Description: "t",
		Transport: "rest", RequiredEnv: []string{"AGEZT_WIRETEST_TOKEN"},
	})
	found := false
	for _, k := range MissingFactories() {
		if k == "wiretest-manifest-only" {
			found = true
		}
	}
	if !found {
		t.Fatal("manifest without factory should be reported missing")
	}
}
