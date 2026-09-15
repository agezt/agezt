// SPDX-License-Identifier: MIT
//
// Platform-bot factories (buildDingTalk, buildFeishu, buildWeCom, buildMastodon).
// Extracted from factories_bot.go during Day 211 god-file refactor (#62).
// Public API unchanged.
package builtinchannels

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/plugins/channels/dingtalk"
	"github.com/agezt/agezt/plugins/channels/feishu"
	"github.com/agezt/agezt/plugins/channels/mastodon"
	"github.com/agezt/agezt/plugins/channels/push"
	"github.com/agezt/agezt/plugins/channels/wecom"
)

func buildDingTalk(d channelwire.Deps) channelwire.Built {
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "DINGTALK_ADDR"))
	if addr == "" {
		// Two-way unconfigured — fall back to the outbound-only push entry via
		// the custom-robot webhook when set (two-way wins the "dingtalk" name).
		if u := strings.TrimSpace(d.Get(brand.EnvPrefix + "DINGTALK_WEBHOOK")); u != "" {
			return pushBuilt(d, push.Config{Kind: push.KindDingTalk, URL: u})
		}
		return channelwire.NotConfigured
	}
	users := splitNonEmpty(d.Get(brand.EnvPrefix + "DINGTALK_USERS"))
	webhook := strings.TrimSpace(d.Get(brand.EnvPrefix + "DINGTALK_WEBHOOK"))
	ch := dingtalk.New(dingtalk.Config{
		WebhookURL: webhook,
		Secret:     strings.TrimSpace(d.Get(brand.EnvPrefix + "DINGTALK_SECRET")),
		Allowlist:  channel.NewAllowlist(users),
		Bus:        d.Bus,
		Handler:    d.Handler,
		Addr:       addr,
		Path:       strings.TrimSpace(d.Get(brand.EnvPrefix + "DINGTALK_PATH")),
	})
	var sink pulse.BriefSink
	if webhook != "" {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			return ch.Send(d.Ctx, channel.Outbound{Text: formatBrief(b), Priority: channel.PriorityNotify})
		})
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: fmt.Sprintf("DingTalk robot, allowlist=%d sender(s)", len(users))}
}
func buildFeishu(d channelwire.Deps) channelwire.Built {
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "FEISHU_ADDR"))
	appID := strings.TrimSpace(d.Get(brand.EnvPrefix + "FEISHU_APP_ID"))
	if addr == "" || appID == "" {
		// Two-way unconfigured — fall back to the outbound-only push entry via
		// the custom-bot webhook when set (two-way wins the "feishu" name).
		if u := strings.TrimSpace(d.Get(brand.EnvPrefix + "FEISHU_WEBHOOK")); u != "" {
			return pushBuilt(d, push.Config{Kind: push.KindFeishu, URL: u})
		}
		return channelwire.NotConfigured
	}
	users := splitNonEmpty(d.Get(brand.EnvPrefix + "FEISHU_USERS"))
	chat := strings.TrimSpace(d.Get(brand.EnvPrefix + "FEISHU_CHAT"))
	ch := feishu.New(feishu.Config{
		AppID:       appID,
		AppSecret:   strings.TrimSpace(d.Get(brand.EnvPrefix + "FEISHU_APP_SECRET")),
		VerifyToken: strings.TrimSpace(d.Get(brand.EnvPrefix + "FEISHU_VERIFY_TOKEN")),
		DefaultChat: chat,
		Allowlist:   channel.NewAllowlist(users),
		Bus:         d.Bus,
		Handler:     d.Handler,
		Addr:        addr,
		Path:        strings.TrimSpace(d.Get(brand.EnvPrefix + "FEISHU_PATH")),
	})
	var sink pulse.BriefSink
	if chat != "" {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			return ch.Send(d.Ctx, channel.Outbound{ChannelID: chat, Text: formatBrief(b), Priority: channel.PriorityNotify})
		})
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: fmt.Sprintf("Feishu app, allowlist=%d user(s)", len(users))}
}
func buildWeCom(d channelwire.Deps) channelwire.Built {
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_ADDR"))
	corp := strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_CORP_ID"))
	if addr == "" || corp == "" {
		// Two-way unconfigured — fall back to the outbound-only push entry via
		// the group-robot webhook when set (two-way wins the "wecom" name).
		if u := strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_WEBHOOK")); u != "" {
			return pushBuilt(d, push.Config{Kind: push.KindWeCom, URL: u})
		}
		return channelwire.NotConfigured
	}
	users := splitNonEmpty(d.Get(brand.EnvPrefix + "WECOM_USERS"))
	ch := wecom.New(wecom.Config{
		CorpID:     corp,
		CorpSecret: strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_CORP_SECRET")),
		AgentID:    strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_AGENT_ID")),
		Token:      strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_TOKEN")),
		AESKey:     strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_AES_KEY")),
		Allowlist:  channel.NewAllowlist(users),
		Bus:        d.Bus,
		Handler:    d.Handler,
		Addr:       addr,
		Path:       strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_PATH")),
	})
	var sink pulse.BriefSink
	if len(users) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, u := range users {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: u, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: fmt.Sprintf("WeCom app, allowlist=%d user(s)", len(users))}
}
func buildMastodon(d channelwire.Deps) channelwire.Built {
	if strings.TrimSpace(d.Get(brand.EnvPrefix+"MASTODON_USERS")) == "" {
		if token := strings.TrimSpace(d.Get(brand.EnvPrefix + "MASTODON_TOKEN")); token != "" {
			return pushBuilt(d, push.Config{
				Kind:   push.KindMastodon,
				Server: strings.TrimSpace(d.Get(brand.EnvPrefix + "MASTODON_SERVER")),
				Token:  token,
			})
		}
		return channelwire.NotConfigured
	}
	server := strings.TrimSpace(d.Get(brand.EnvPrefix + "MASTODON_SERVER"))
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "MASTODON_TOKEN"))
	if server == "" || token == "" {
		return channelwire.NotConfigured
	}
	users := splitNonEmpty(d.Get(brand.EnvPrefix + "MASTODON_USERS"))
	poll := 0
	if v := strings.TrimSpace(d.Get(brand.EnvPrefix + "MASTODON_POLL")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			poll = n
		}
	}
	ch := mastodon.New(mastodon.Config{
		Server:    server,
		Token:     token,
		Allowlist: channel.NewAllowlist(users),
		Bus:       d.Bus,
		Handler:   d.Handler,
		PollSecs:  poll,
	})
	sink := pulse.SinkFunc(func(b pulse.Brief) error {
		return ch.Send(d.Ctx, channel.Outbound{Text: formatBrief(b), Priority: channel.PriorityNotify})
	})
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: fmt.Sprintf("Mastodon (two-way), allowlist=%d acct(s)", len(users))}
}
