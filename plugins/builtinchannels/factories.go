// SPDX-License-Identifier: MIT
//
// builtinchannels: formatBrief + splitNonEmpty helpers + Telegram + Slack
// channel factories. Extracted from factories.go during Day 211
// god-file refactor (#44, #68). Public API unchanged.
package builtinchannels

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/plugins/channels/slack"
	"github.com/agezt/agezt/plugins/channels/telegram"
)

func formatBrief(b pulse.Brief) string {
	if b.Body != "" {
		return "📣 " + b.Title + "\n" + b.Body
	}
	return "📣 " + b.Title
}
func splitNonEmpty(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
func buildTelegram(d channelwire.Deps) channelwire.Built {
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "TELEGRAM_TOKEN"))
	if token == "" {
		return channelwire.NotConfigured
	}
	chatIDs := splitNonEmpty(d.Get(brand.EnvPrefix + "TELEGRAM_CHAT_ID"))
	allow := channel.NewAllowlist(chatIDs)

	ch := telegram.New(telegram.Config{
		Token:     token,
		BaseURL:   strings.TrimSpace(d.Get(brand.EnvPrefix + "TELEGRAM_API_BASE")), // empty → public Bot API
		Allowlist: allow,
		Bus:       d.Bus,
		Handler:   d.Handler,
	})

	// Pulse briefs → the allowlisted chats. Nil sink when no chat configured
	// (the bot can still receive commands once a chat is allowlisted).
	var sink pulse.BriefSink
	if len(chatIDs) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range chatIDs {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("listening, allowlist=%d chat(s)", len(chatIDs))
	if len(chatIDs) == 0 {
		desc = "listening, NO allowlist (outbound-only; set AGEZT_TELEGRAM_CHAT_ID to allow commands)"
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}
func buildSlack(d channelwire.Deps) channelwire.Built {
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "SLACK_TOKEN"))
	if token == "" {
		return channelwire.NotConfigured
	}
	secret := strings.TrimSpace(d.Get(brand.EnvPrefix + "SLACK_SIGNING_SECRET"))
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "SLACK_ADDR"))
	channelIDs := splitNonEmpty(d.Get(brand.EnvPrefix + "SLACK_CHANNELS"))

	ch := slack.New(slack.Config{
		Token:         token,
		SigningSecret: secret,
		Addr:          addr,
		BaseURL:       strings.TrimSpace(d.Get(brand.EnvPrefix + "SLACK_API_BASE")), // empty → public Web API
		Allowlist:     channel.NewAllowlist(channelIDs),
		Bus:           d.Bus,
		Handler:       d.Handler,
	})

	var sink pulse.BriefSink
	if len(channelIDs) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range channelIDs {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	var desc string
	switch {
	case addr == "" && len(channelIDs) == 0:
		desc = "outbound-only, NO allowlist (set AGEZT_SLACK_ADDR + AGEZT_SLACK_CHANNELS to receive commands)"
	case addr == "":
		desc = fmt.Sprintf("outbound-only, allowlist=%d channel(s) (set AGEZT_SLACK_ADDR to receive commands)", len(channelIDs))
	case secret == "":
		desc = "inbound DISABLED (set AGEZT_SLACK_SIGNING_SECRET); outbound only"
	default:
		desc = fmt.Sprintf("events at %s%s, allowlist=%d channel(s)", addr, slack.EventsPath, len(channelIDs))
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}
