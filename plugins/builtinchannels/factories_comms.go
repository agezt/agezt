// SPDX-License-Identifier: MIT
//
// builtinchannels: Email + Discord + Matrix + WhatsApp channel factories.
// Extracted from factories.go during Day 211 god-file refactor (#68).
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
	"github.com/agezt/agezt/plugins/channels/discord"
	"github.com/agezt/agezt/plugins/channels/email"
	"github.com/agezt/agezt/plugins/channels/matrix"
	"github.com/agezt/agezt/plugins/channels/whatsapp"
)

func buildEmail(d channelwire.Deps) channelwire.Built {
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "EMAIL_SMTP_ADDR"))
	if addr == "" {
		return channelwire.NotConfigured
	}
	from := strings.TrimSpace(d.Get(brand.EnvPrefix + "EMAIL_FROM"))
	recipients := splitNonEmpty(d.Get(brand.EnvPrefix + "EMAIL_RECIPIENTS"))
	inboxAddr := strings.TrimSpace(d.Get(brand.EnvPrefix + "EMAIL_INBOX_ADDR"))
	pollSecs := 0
	if v := strings.TrimSpace(d.Get(brand.EnvPrefix + "EMAIL_INBOX_POLL")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			pollSecs = n
		}
	}

	var handler channel.InboundHandler
	if inboxAddr != "" {
		handler = d.Handler
	}
	ch := email.New(email.Config{
		Addr:          addr,
		From:          from,
		Username:      strings.TrimSpace(d.Get(brand.EnvPrefix + "EMAIL_USERNAME")),
		Password:      d.Get(brand.EnvPrefix + "EMAIL_PASSWORD"),
		Allowlist:     channel.NewAllowlist(recipients),
		Bus:           d.Bus,
		InboxAddr:     inboxAddr,
		InboxProtocol: strings.TrimSpace(d.Get(brand.EnvPrefix + "EMAIL_INBOX_PROTOCOL")),
		InboxUsername: strings.TrimSpace(d.Get(brand.EnvPrefix + "EMAIL_INBOX_USERNAME")),
		InboxPassword: d.Get(brand.EnvPrefix + "EMAIL_INBOX_PASSWORD"),
		InboxTLS:      strings.TrimSpace(d.Get(brand.EnvPrefix + "EMAIL_INBOX_TLS")),
		PollSecs:      pollSecs,
		Handler:       handler,
	})

	var sink pulse.BriefSink
	if len(recipients) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range recipients {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	dir := "outbound"
	if inboxAddr != "" {
		dir = "two-way (inbox " + inboxAddr + ")"
	}
	var desc string
	switch {
	case from == "":
		desc = "configured but NO from address (set AGEZT_EMAIL_FROM)"
	case len(recipients) == 0:
		desc = fmt.Sprintf("%s via %s, NO recipients (set AGEZT_EMAIL_RECIPIENTS)", dir, addr)
	default:
		desc = fmt.Sprintf("%s via %s, %d recipient(s)", dir, addr, len(recipients))
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}
func buildDiscord(d channelwire.Deps) channelwire.Built {
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "DISCORD_TOKEN"))
	if token == "" {
		return channelwire.NotConfigured
	}
	pubKey := strings.TrimSpace(d.Get(brand.EnvPrefix + "DISCORD_PUBLIC_KEY"))
	appID := strings.TrimSpace(d.Get(brand.EnvPrefix + "DISCORD_APP_ID"))
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "DISCORD_ADDR"))
	channelIDs := splitNonEmpty(d.Get(brand.EnvPrefix + "DISCORD_CHANNELS"))

	ch := discord.New(discord.Config{
		Token:         token,
		PublicKey:     pubKey,
		ApplicationID: appID,
		Addr:          addr,
		BaseURL:       strings.TrimSpace(d.Get(brand.EnvPrefix + "DISCORD_API_BASE")), // empty → public API
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
		desc = "outbound-only, NO allowlist (set AGEZT_DISCORD_ADDR + AGEZT_DISCORD_CHANNELS to receive commands)"
	case addr == "":
		desc = fmt.Sprintf("outbound-only, allowlist=%d channel(s) (set AGEZT_DISCORD_ADDR to receive commands)", len(channelIDs))
	case pubKey == "":
		desc = "inbound DISABLED (set AGEZT_DISCORD_PUBLIC_KEY); outbound only"
	default:
		desc = fmt.Sprintf("interactions at %s%s, allowlist=%d channel(s)", addr, discord.InteractionsPath, len(channelIDs))
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}
func buildMatrix(d channelwire.Deps) channelwire.Built {
	homeserver := strings.TrimSpace(d.Get(brand.EnvPrefix + "MATRIX_HOMESERVER"))
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "MATRIX_TOKEN"))
	if homeserver == "" || token == "" {
		return channelwire.NotConfigured
	}
	roomIDs := splitNonEmpty(d.Get(brand.EnvPrefix + "MATRIX_ROOMS"))

	ch := matrix.New(matrix.Config{
		Homeserver: homeserver,
		Token:      token,
		Allowlist:  channel.NewAllowlist(roomIDs),
		Bus:        d.Bus,
		Handler:    d.Handler,
	})

	// Pulse briefs → the allowlisted rooms. Nil sink when no room configured (the
	// bot can still receive commands once a room is allowlisted).
	var sink pulse.BriefSink
	if len(roomIDs) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range roomIDs {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("listening, allowlist=%d room(s)", len(roomIDs))
	if len(roomIDs) == 0 {
		desc = "listening, NO allowlist (outbound-only; set AGEZT_MATRIX_ROOMS to allow commands)"
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}
func buildWhatsApp(d channelwire.Deps) channelwire.Built {
	appSecret := strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPP_APP_SECRET"))
	accessToken := strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPP_ACCESS_TOKEN"))
	if appSecret == "" || accessToken == "" {
		return channelwire.NotConfigured
	}
	phoneID := strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPP_PHONE_NUMBER_ID"))
	verifyToken := strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPP_VERIFY_TOKEN"))
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPP_ADDR"))
	path := strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPP_PATH"))
	numbers := splitNonEmpty(d.Get(brand.EnvPrefix + "WHATSAPP_NUMBERS"))

	ch := whatsapp.New(whatsapp.Config{
		Addr:          addr,
		Path:          path,
		VerifyToken:   verifyToken,
		AppSecret:     appSecret,
		AccessToken:   accessToken,
		PhoneNumberID: phoneID,
		Allowlist:     channel.NewAllowlist(numbers),
		Bus:           d.Bus,
		Handler:       d.Handler,
	})

	var sink pulse.BriefSink
	if phoneID != "" && len(numbers) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, id := range numbers {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: id, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	var desc string
	switch {
	case addr == "":
		desc = fmt.Sprintf("outbound-only (set AGEZT_WHATSAPP_ADDR for inbound), allowlist=%d number(s)", len(numbers))
	default:
		p := path
		if p == "" {
			p = whatsapp.DefaultPath
		}
		desc = fmt.Sprintf("inbound at %s%s, allowlist=%d number(s)", addr, p, len(numbers))
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}
