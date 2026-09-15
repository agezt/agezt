// SPDX-License-Identifier: MIT
//
// Chat-style channel factories (buildIRC, buildTwitch). Extracted from
// factories_chat.go during Day 211 god-file refactor (#44, #61).
// Public API unchanged.
package builtinchannels

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/plugins/channels/irc"
)

func buildIRC(d channelwire.Deps) channelwire.Built {
	server := strings.TrimSpace(d.Get(brand.EnvPrefix + "IRC_SERVER"))
	nick := strings.TrimSpace(d.Get(brand.EnvPrefix + "IRC_NICK"))
	if server == "" || nick == "" {
		return channelwire.NotConfigured
	}
	chans := splitNonEmpty(d.Get(brand.EnvPrefix + "IRC_CHANNELS"))
	// The joined channels are allowed by default; extra nicks/channels widen it.
	allowed := append([]string(nil), chans...)
	allowed = append(allowed, splitNonEmpty(d.Get(brand.EnvPrefix+"IRC_ALLOWLIST"))...)
	useTLS := strings.EqualFold(strings.TrimSpace(d.Get(brand.EnvPrefix+"IRC_TLS")), "true") || strings.HasSuffix(server, ":6697")

	ch := irc.New(irc.Config{
		Server:    server,
		TLS:       useTLS,
		Nick:      nick,
		Password:  strings.TrimSpace(d.Get(brand.EnvPrefix + "IRC_PASSWORD")),
		Channels:  chans,
		Allowlist: channel.NewAllowlist(allowed),
		Bus:       d.Bus,
		Handler:   d.Handler,
	})

	var sink pulse.BriefSink
	if len(chans) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, c := range chans {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: c, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("%s as %s, %d channel(s)", server, nick, len(chans))
	if len(chans) == 0 {
		desc = fmt.Sprintf("%s as %s, NO channels (set AGEZT_IRC_CHANNELS)", server, nick)
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}
func buildTwitch(d channelwire.Deps) channelwire.Built {
	user := strings.TrimSpace(d.Get(brand.EnvPrefix + "TWITCH_USERNAME"))
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "TWITCH_TOKEN"))
	if user == "" || token == "" {
		return channelwire.NotConfigured
	}
	chans := splitNonEmpty(d.Get(brand.EnvPrefix + "TWITCH_CHANNELS"))
	allowed := append([]string(nil), chans...)
	allowed = append(allowed, splitNonEmpty(d.Get(brand.EnvPrefix+"TWITCH_ALLOWLIST"))...)
	pass := token
	if !strings.HasPrefix(pass, "oauth:") {
		pass = "oauth:" + pass
	}

	ch := irc.New(irc.Config{
		Kind:      "twitch",
		Server:    "irc.chat.twitch.tv:6697",
		TLS:       true,
		Nick:      strings.ToLower(user),
		Password:  pass,
		Channels:  chans,
		Allowlist: channel.NewAllowlist(allowed),
		Bus:       d.Bus,
		Handler:   d.Handler,
	})

	var sink pulse.BriefSink
	if len(chans) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, c := range chans {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: c, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("as %s, %d channel(s)", user, len(chans))
	if len(chans) == 0 {
		desc = fmt.Sprintf("as %s, NO channels (set AGEZT_TWITCH_CHANNELS)", user)
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}
