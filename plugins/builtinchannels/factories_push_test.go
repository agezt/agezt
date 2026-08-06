// SPDX-License-Identifier: MIT

package builtinchannels

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/plugins/channels/push"
)

// mapGet adapts a fixed env map to a channelwire Deps.Get.
func mapGet(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

// TestDualKindFactories_TwoWayWinsOverPushFallback locks the ownership rule
// that moved out of cmd/agezt's twoWay* predicates and into each dual kind's
// own factory: two-way config wins the kind's name; with only the push env
// set, the factory returns the outbound-only push.Channel instead of
// NotConfigured.
func TestDualKindFactories_TwoWayWinsOverPushFallback(t *testing.T) {
	RegisterAll()
	ctx := context.Background()
	p := brand.EnvPrefix

	cases := []struct {
		kind    string
		pushEnv map[string]string // push fallback only
		twoWay  map[string]string // two-way config (wins, even with push env set)
	}{
		{
			kind:    "googlechat",
			pushEnv: map[string]string{p + "GOOGLECHAT_WEBHOOK": "https://chat.example/hook"},
			twoWay: map[string]string{
				p + "GOOGLECHAT_WEBHOOK": "https://chat.example/hook",
				p + "GOOGLECHAT_ADDR":    "127.0.0.1:0",
			},
		},
		{
			kind:    "mattermost",
			pushEnv: map[string]string{p + "MATTERMOST_WEBHOOK": "https://mm.example/hook"},
			twoWay: map[string]string{
				p + "MATTERMOST_WEBHOOK": "https://mm.example/hook",
				p + "MATTERMOST_ADDR":    "127.0.0.1:0",
			},
		},
		{
			kind: "mastodon",
			pushEnv: map[string]string{
				p + "MASTODON_SERVER": "https://masto.example",
				p + "MASTODON_TOKEN":  "tok",
			},
			twoWay: map[string]string{
				p + "MASTODON_SERVER": "https://masto.example",
				p + "MASTODON_TOKEN":  "tok",
				p + "MASTODON_USERS":  "@al@masto.example",
			},
		},
		{
			kind: "line",
			pushEnv: map[string]string{
				p + "LINE_TOKEN": "tok",
				p + "LINE_TO":    "U123",
			},
			twoWay: map[string]string{
				p + "LINE_TOKEN":  "tok",
				p + "LINE_TO":     "U123",
				p + "LINE_SECRET": "sec",
			},
		},
		{
			kind:    "feishu",
			pushEnv: map[string]string{p + "FEISHU_WEBHOOK": "https://open.feishu.example/hook"},
			twoWay: map[string]string{
				p + "FEISHU_WEBHOOK": "https://open.feishu.example/hook",
				p + "FEISHU_ADDR":    "127.0.0.1:0",
				p + "FEISHU_APP_ID":  "cli_x",
			},
		},
		{
			kind:    "dingtalk",
			pushEnv: map[string]string{p + "DINGTALK_WEBHOOK": "https://oapi.dingtalk.example/hook"},
			twoWay: map[string]string{
				p + "DINGTALK_WEBHOOK": "https://oapi.dingtalk.example/hook",
				p + "DINGTALK_ADDR":    "127.0.0.1:0",
			},
		},
		{
			kind:    "wecom",
			pushEnv: map[string]string{p + "WECOM_WEBHOOK": "https://qyapi.weixin.example/hook"},
			twoWay: map[string]string{
				p + "WECOM_WEBHOOK": "https://qyapi.weixin.example/hook",
				p + "WECOM_ADDR":    "127.0.0.1:0",
				p + "WECOM_CORP_ID": "corp1",
			},
		},
	}

	for _, tc := range cases {
		f, ok := channelwire.Lookup(tc.kind)
		if !ok {
			t.Errorf("%s: no channelwire factory registered", tc.kind)
			continue
		}

		// Only the push env set → the outbound-only push.Channel owns the name.
		built := f(channelwire.Deps{Ctx: ctx, Get: mapGet(tc.pushEnv)})
		if len(built.Channels) != 1 || built.Desc == "" {
			t.Errorf("%s push fallback: got %d channel(s), desc %q; want 1 push channel", tc.kind, len(built.Channels), built.Desc)
			continue
		}
		if _, isPush := built.Channels[0].(*push.Channel); !isPush {
			t.Errorf("%s push fallback: channel is %T, want *push.Channel", tc.kind, built.Channels[0])
		}
		if got := built.Channels[0].Name(); got != tc.kind {
			t.Errorf("%s push fallback: channel Name() = %q, want the kind", tc.kind, got)
		}
		if built.Sink == nil {
			t.Errorf("%s push fallback: nil brief sink, want one delivering via Send", tc.kind)
		}
		if !strings.HasPrefix(built.Desc, "push → ") {
			t.Errorf("%s push fallback: desc = %q, want a push desc", tc.kind, built.Desc)
		}

		// Two-way config set (push env still present) → two-way wins the name.
		built = f(channelwire.Deps{Ctx: ctx, Get: mapGet(tc.twoWay)})
		if len(built.Channels) != 1 || built.Desc == "" {
			t.Errorf("%s two-way: got %d channel(s), desc %q; want the two-way channel", tc.kind, len(built.Channels), built.Desc)
			continue
		}
		if _, isPush := built.Channels[0].(*push.Channel); isPush {
			t.Errorf("%s two-way: got the push fallback; two-way config must win the name", tc.kind)
		}

		// Nothing set → NotConfigured.
		built = f(channelwire.Deps{Ctx: ctx, Get: mapGet(nil)})
		if built.Desc != "" || len(built.Channels) != 0 {
			t.Errorf("%s unconfigured: desc %q, %d channel(s); want NotConfigured", tc.kind, built.Desc, len(built.Channels))
		}
	}
}

// TestPurePushFactories_Configured locks the seven pure push kinds: with the
// enabling env set each factory returns one push.Channel named after the kind,
// with a brief sink.
func TestPurePushFactories_Configured(t *testing.T) {
	RegisterAll()
	ctx := context.Background()
	p := brand.EnvPrefix

	cases := map[string]map[string]string{
		"ntfy":       {p + "NTFY_TOPIC": "alerts"},
		"pushover":   {p + "PUSHOVER_TOKEN": "app", p + "PUSHOVER_USER": "usr"},
		"gotify":     {p + "GOTIFY_TOKEN": "app", p + "GOTIFY_SERVER": "https://gotify.example"},
		"pushbullet": {p + "PUSHBULLET_TOKEN": "tok"},
		"rocketchat": {p + "ROCKETCHAT_WEBHOOK": "https://rocket.example/hook"},
		"zulip": {
			p + "ZULIP_APIKEY": "key", p + "ZULIP_SERVER": "https://zulip.example",
			p + "ZULIP_EMAIL": "bot@zulip.example", p + "ZULIP_STREAM": "general",
		},
		"synology": {p + "SYNOLOGY_WEBHOOK": "https://nas.example/hook"},
	}

	for kind, env := range cases {
		f, ok := channelwire.Lookup(kind)
		if !ok {
			t.Errorf("%s: no channelwire factory registered", kind)
			continue
		}
		built := f(channelwire.Deps{Ctx: ctx, Get: mapGet(env)})
		if len(built.Channels) != 1 || built.Desc == "" {
			t.Errorf("%s: got %d channel(s), desc %q; want 1 push channel", kind, len(built.Channels), built.Desc)
			continue
		}
		if _, isPush := built.Channels[0].(*push.Channel); !isPush {
			t.Errorf("%s: channel is %T, want *push.Channel", kind, built.Channels[0])
		}
		if got := built.Channels[0].Name(); got != kind {
			t.Errorf("%s: channel Name() = %q, want the kind", kind, got)
		}
		if built.Sink == nil {
			t.Errorf("%s: nil brief sink, want one delivering via Send", kind)
		}
	}
}
