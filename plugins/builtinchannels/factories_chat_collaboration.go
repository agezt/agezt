// SPDX-License-Identifier: MIT
//
// Collaboration-channel factories (buildNextcloudTalk, buildHomeAssistant,
// buildTeams, buildLine). Extracted from factories_chat.go during Day 211
// god-file refactor (#61). Public API unchanged.
package builtinchannels

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/plugins/channels/homeassistant"
	linechan "github.com/agezt/agezt/plugins/channels/line"
	"github.com/agezt/agezt/plugins/channels/nextcloudtalk"
	"github.com/agezt/agezt/plugins/channels/push"
	"github.com/agezt/agezt/plugins/channels/teams"
)

func buildNextcloudTalk(d channelwire.Deps) channelwire.Built {
	server := strings.TrimSpace(d.Get(brand.EnvPrefix + "NEXTCLOUDTALK_URL"))
	secret := strings.TrimSpace(d.Get(brand.EnvPrefix + "NEXTCLOUDTALK_SECRET"))
	if server == "" || secret == "" {
		return channelwire.NotConfigured
	}
	tokens := splitNonEmpty(d.Get(brand.EnvPrefix + "NEXTCLOUDTALK_TOKENS"))
	ch := nextcloudtalk.New(nextcloudtalk.Config{
		ServerURL: server,
		Secret:    secret,
		Allowlist: channel.NewAllowlist(tokens),
		Bus:       d.Bus,
		Handler:   d.Handler,
		Addr:      strings.TrimSpace(d.Get(brand.EnvPrefix + "NEXTCLOUDTALK_ADDR")),
		Path:      strings.TrimSpace(d.Get(brand.EnvPrefix + "NEXTCLOUDTALK_PATH")),
	})
	var sink pulse.BriefSink
	if len(tokens) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, t := range tokens {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: t, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: fmt.Sprintf("Nextcloud Talk, allowlist=%d token(s)", len(tokens))}
}
func buildHomeAssistant(d channelwire.Deps) channelwire.Built {
	baseURL := strings.TrimSpace(d.Get(brand.EnvPrefix + "HOMEASSISTANT_URL"))
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "HOMEASSISTANT_TOKEN"))
	if baseURL == "" || token == "" {
		return channelwire.NotConfigured
	}
	services := splitNonEmpty(d.Get(brand.EnvPrefix + "HOMEASSISTANT_SERVICES"))

	ch := homeassistant.New(homeassistant.Config{
		BaseURL:   baseURL,
		Token:     token,
		Allowlist: channel.NewAllowlist(services),
		Bus:       d.Bus,
	})

	var sink pulse.BriefSink
	if len(services) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, svc := range services {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: svc, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("outbound → %s, allowlist=%d service(s)", baseURL, len(services))
	if len(services) == 0 {
		desc = fmt.Sprintf("outbound → %s, NO allowlist (set AGEZT_HOMEASSISTANT_SERVICES to notify)", baseURL)
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}
func buildTeams(d channelwire.Deps) channelwire.Built {
	hooks := parseNamedWebhooks(strings.TrimSpace(d.Get(brand.EnvPrefix + "TEAMS_WEBHOOKS")))
	if len(hooks) == 0 {
		return channelwire.NotConfigured
	}
	ch := teams.New(teams.Config{Webhooks: hooks, Bus: d.Bus})

	names := ch.Names()
	sink := pulse.SinkFunc(func(b pulse.Brief) error {
		var firstErr error
		for _, name := range names {
			if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: name, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	})

	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: fmt.Sprintf("outbound → %d Teams webhook(s)", len(names))}
}
func buildLine(d channelwire.Deps) channelwire.Built {
	secret := strings.TrimSpace(d.Get(brand.EnvPrefix + "LINE_SECRET"))
	if secret == "" {
		if token := strings.TrimSpace(d.Get(brand.EnvPrefix + "LINE_TOKEN")); token != "" {
			return pushBuilt(d, push.Config{
				Kind:   push.KindLine,
				Token:  token,
				Target: strings.TrimSpace(d.Get(brand.EnvPrefix + "LINE_TO")),
			})
		}
		return channelwire.NotConfigured
	}
	users := splitNonEmpty(d.Get(brand.EnvPrefix + "LINE_USERS"))
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "LINE_ADDR"))

	ch := linechan.New(linechan.Config{
		Secret:      secret,
		AccessToken: strings.TrimSpace(d.Get(brand.EnvPrefix + "LINE_TOKEN")),
		Allowlist:   channel.NewAllowlist(users),
		Bus:         d.Bus,
		Handler:     d.Handler,
		Addr:        addr,
		Path:        strings.TrimSpace(d.Get(brand.EnvPrefix + "LINE_PATH")),
	})

	var sink pulse.BriefSink
	if to := strings.TrimSpace(d.Get(brand.EnvPrefix + "LINE_TO")); to != "" {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			return ch.Send(d.Ctx, channel.Outbound{ChannelID: to, Text: formatBrief(b), Priority: channel.PriorityNotify})
		})
	}

	desc := fmt.Sprintf("LINE Messaging API, allowlist=%d user(s)", len(users))
	if addr == "" {
		desc += " (outbound-only; set AGEZT_LINE_ADDR for two-way)"
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}
