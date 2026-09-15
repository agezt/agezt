// SPDX-License-Identifier: MIT
//
// Bot-style channel factories (oneBotFactory, buildZalo, buildNostr).
// Extracted from factories_bot.go during Day 211 god-file refactor (#44, #62).
// Public API unchanged.
package builtinchannels

import (
	"fmt"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/plugins/channels/nostr"
	"github.com/agezt/agezt/plugins/channels/onebot"
	"github.com/agezt/agezt/plugins/channels/zalo"
)

func oneBotFactory(kind, prefix string) channelwire.Factory {
	return func(d channelwire.Deps) channelwire.Built {
		get := func(s string) string { return strings.TrimSpace(d.Get(brand.EnvPrefix + prefix + s)) }
		addr := get("_ADDR")
		if addr == "" {
			return channelwire.NotConfigured
		}
		users := splitNonEmpty(d.Get(brand.EnvPrefix + prefix + "_USERS"))
		ch := onebot.New(onebot.Config{
			Kind:        kind,
			APIBase:     get("_GATEWAY"),
			AccessToken: get("_TOKEN"),
			Secret:      get("_SECRET"),
			Allowlist:   channel.NewAllowlist(users),
			Bus:         d.Bus,
			Handler:     d.Handler,
			Addr:        addr,
			Path:        get("_PATH"),
		})
		var sink pulse.BriefSink
		if len(users) > 0 && get("_GATEWAY") != "" {
			sink = pulse.SinkFunc(func(b pulse.Brief) error {
				var firstErr error
				for _, u := range users {
					if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: "private:" + u, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
						firstErr = err
					}
				}
				return firstErr
			})
		}
		return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: fmt.Sprintf("%s via OneBot gateway, allowlist=%d user(s)", kind, len(users))}
	}
}
func buildZalo(d channelwire.Deps) channelwire.Built {
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "ZALO_ADDR"))
	if addr == "" {
		return channelwire.NotConfigured
	}
	users := splitNonEmpty(d.Get(brand.EnvPrefix + "ZALO_USERS"))
	ch := zalo.New(zalo.Config{
		AppID:       strings.TrimSpace(d.Get(brand.EnvPrefix + "ZALO_APP_ID")),
		AccessToken: strings.TrimSpace(d.Get(brand.EnvPrefix + "ZALO_TOKEN")),
		Secret:      strings.TrimSpace(d.Get(brand.EnvPrefix + "ZALO_SECRET")),
		Allowlist:   channel.NewAllowlist(users),
		Bus:         d.Bus,
		Handler:     d.Handler,
		Addr:        addr,
		Path:        strings.TrimSpace(d.Get(brand.EnvPrefix + "ZALO_PATH")),
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
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: fmt.Sprintf("Zalo OA, allowlist=%d user(s)", len(users))}
}
func buildNostr(d channelwire.Deps) channelwire.Built {
	priv := strings.TrimSpace(d.Get(brand.EnvPrefix + "NOSTR_PRIVKEY"))
	relays := splitNonEmpty(d.Get(brand.EnvPrefix + "NOSTR_RELAYS"))
	if priv == "" || len(relays) == 0 {
		return channelwire.NotConfigured
	}
	authors := splitNonEmpty(d.Get(brand.EnvPrefix + "NOSTR_AUTHORS"))
	// Authors may be hex or npub… — normalize to hex so the allowlist matches the
	// hex pubkey on inbound events.
	norm := make([]string, 0, len(authors))
	for _, a := range authors {
		if h, derr := nostr.DecodePubkey(a); derr == nil {
			norm = append(norm, h)
		}
	}
	ch, err := nostr.New(nostr.Config{
		PrivKeyHex: priv,
		Relays:     relays,
		Allowlist:  channel.NewAllowlist(norm),
		Bus:        d.Bus,
		Handler:    d.Handler,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: nostr channel disabled: %v\n", brand.Binary, err)
		return channelwire.NotConfigured
	}
	sink := pulse.SinkFunc(func(b pulse.Brief) error {
		return ch.Send(d.Ctx, channel.Outbound{Text: formatBrief(b), Priority: channel.PriorityNotify})
	})
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: fmt.Sprintf("Nostr, %d relay(s), allowlist=%d author(s)", len(relays), len(authors))}
}
