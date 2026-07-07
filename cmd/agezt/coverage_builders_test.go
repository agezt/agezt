// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	kernelruntime "github.com/agezt/agezt/kernel/runtime"
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

	// Builders that take a config lookup func — feed emptyGet so every key
	// resolves to "".
	getBuilders := map[string]func() (interface{}, interface{}, string){
		"telegram": func() (interface{}, interface{}, string) {
			c, s, d := buildTelegramInstance(ctx, k, "telegram", emptyGet)
			return c, s, d
		},
		"slack": func() (interface{}, interface{}, string) {
			c, s, d := buildSlackInstance(ctx, k, "slack", emptyGet)
			return c, s, d
		},
		"email": func() (interface{}, interface{}, string) {
			c, s, d := buildEmailInstance(ctx, k, "email", emptyGet)
			return c, s, d
		},
		"discord": func() (interface{}, interface{}, string) {
			c, s, d := buildDiscordInstance(ctx, k, "discord", emptyGet)
			return c, s, d
		},
		"matrix": func() (interface{}, interface{}, string) {
			c, s, d := buildMatrixInstance(ctx, k, "matrix", emptyGet)
			return c, s, d
		},
		"whatsapp": func() (interface{}, interface{}, string) {
			c, s, d := buildWhatsAppInstance(ctx, k, "whatsapp", emptyGet)
			return c, s, d
		},
	}
	for name, fn := range getBuilders {
		if _, _, desc := fn(); desc != "" {
			t.Errorf("%s builder returned a non-empty desc %q when unconfigured", name, desc)
		}
	}

	// Builders that read os.Getenv directly. None of their env vars are set in
	// the test process, so each must early-return an empty descriptor.
	envBuilders := map[string]func() string{
		"webhook":       func() string { _, _, d := buildWebhook(ctx, k); return d },
		"irc":           func() string { _, _, d := buildIRC(ctx, k); return d },
		"twitch":        func() string { _, _, d := buildTwitch(ctx, k); return d },
		"whatsappgw":    func() string { _, _, d := buildWhatsAppGateway(ctx, k); return d },
		"imessage":      func() string { _, _, d := buildIMessage(ctx, k); return d },
		"line":          func() string { _, _, d := buildLine(ctx, k); return d },
		"dingtalk":      func() string { _, _, d := buildDingTalk(ctx, k); return d },
		"feishu":        func() string { _, _, d := buildFeishu(ctx, k); return d },
		"wecom":         func() string { _, _, d := buildWeCom(ctx, k); return d },
		"zalo":          func() string { _, _, d := buildZalo(ctx, k); return d },
		"nextcloudtalk": func() string { _, _, d := buildNextcloudTalk(ctx, k); return d },
		"mastodon":      func() string { _, _, d := buildMastodon(ctx, k); return d },
		"nostr":         func() string { _, _, d := buildNostr(ctx, k); return d },
		"sms":           func() string { _, _, d := buildSMS(ctx, k); return d },
		"homeassistant": func() string { _, _, d := buildHomeAssistant(ctx, k); return d },
		"teams":         func() string { _, _, d := buildTeams(ctx, k); return d },
		"signal":        func() string { _, _, d := buildSignal(ctx, k); return d },
	}
	for name, fn := range envBuilders {
		if desc := fn(); desc != "" {
			t.Errorf("%s builder returned a non-empty desc %q when unconfigured", name, desc)
		}
	}

	// kind/prefix builders — a chat webhook + OneBot with an unset prefix.
	if _, _, d := buildChatWebhook(ctx, k, "custom", "CUSTOM"); d != "" {
		t.Errorf("chat webhook returned %q when unconfigured", d)
	}
	if _, _, d := buildOneBot(ctx, k, "qq", "QQ"); d != "" {
		t.Errorf("onebot returned %q when unconfigured", d)
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
