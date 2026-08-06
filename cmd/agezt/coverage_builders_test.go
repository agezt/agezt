// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/channelwire"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/builtinchannels"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// newBuilderTestKernel spins up a throwaway kernel for the daemon channel
// builders. Each builder only needs k.Bus() / makeChannelHandler(k) once it's
// enabled; the "not configured" path we exercise here never reaches that.
func newBuilderTestKernel(t *testing.T) *kernelruntime.Kernel {
	t.Helper()
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// emptyGet is a config lookup that reports every key as unset, so a builder
// takes its "not configured → (nil, nil, \"\")" early-return path.
func emptyGet(string) string { return "" }

// TestChannelBuilders_NotConfigured exercises the disabled path of every
// daemon channel builder. Because none of the enabling env vars/config keys are
// set in the test process, each builder must return a nil channel + empty desc
// without attempting any network work.
func TestChannelBuilders_NotConfigured(t *testing.T) {
	k := newBuilderTestKernel(t)
	ctx := context.Background()

	// Factory-migrated builders (Phase 2.1) — resolved through the channelwire
	// registry after builtinchannels.RegisterAll seeds it. Deps.Get is emptyGet
	// so every config key resolves to "" and each factory must return
	// NotConfigured (empty Desc, no channels) without any network work.
	builtinchannels.RegisterAll()
	// googlechat/mattermost cover the parameterized chatWebhookFactory closure
	// (which replaced the old generic-prefix buildChatWebhook assertion — the
	// factory registry only exposes the two registered kind/prefix pairs).
	for _, kind := range []string{
		"telegram", "slack", "email", "discord", "matrix", "whatsapp",
		"webhook", "irc", "twitch", "sms", "signal",
		"whatsappgw", "imessage", "homeassistant", "teams", "nextcloudtalk",
		"nostr", "zalo", "qq", "wechat", "line",
		"googlechat", "mattermost", "dingtalk", "feishu", "wecom", "mastodon",
	} {
		f, ok := channelwire.Lookup(kind)
		if !ok {
			t.Errorf("%s: no channelwire factory registered", kind)
			continue
		}
		built := f(channelwire.Deps{Ctx: ctx, Bus: k.Bus(), Handler: makeChannelHandler(k), Get: emptyGet})
		if built.Desc != "" {
			t.Errorf("%s factory returned a non-empty desc %q when unconfigured", kind, built.Desc)
		}
		if len(built.Channels) != 0 {
			t.Errorf("%s factory returned %d channel(s) when unconfigured", kind, len(built.Channels))
		}
	}
}

// TestTwoWayConfigHelpers covers the pure config predicates that gate the
// two-way (inbound) channel paths. With no env set both must report false.
func TestTwoWayConfigHelpers(t *testing.T) {
	if twoWayLineConfigured() {
		t.Error("twoWayLineConfigured should be false with no LINE env set")
	}
	if twoWayChatConfigured("CUSTOM") {
		t.Error("twoWayChatConfigured should be false with no prefix env set")
	}
}
