// SPDX-License-Identifier: MIT

// Channel factories (Phase 2.1 of docs/REFACTORING-SCAN-2026-08.md): the
// modern per-account builders, moved verbatim out of cmd/agezt/main.go into
// channelwire.Factory form. Each factory reads config ONLY through d.Get,
// uses d.Bus / d.Handler for the kernel surface, and returns
// channelwire.NotConfigured when its enabling env is unset.
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
	"github.com/agezt/agezt/plugins/channels/slack"
	"github.com/agezt/agezt/plugins/channels/telegram"
	"github.com/agezt/agezt/plugins/channels/whatsapp"
)

// formatBrief renders a Pulse brief as plain channel text. (Small copy of the
// cmd/agezt helper — the legacy builders still there need the original.)
func formatBrief(b pulse.Brief) string {
	if b.Body != "" {
		return "📣 " + b.Title + "\n" + b.Body
	}
	return "📣 " + b.Title
}

// splitNonEmpty splits a comma list, trimming and dropping blanks. (Small copy
// of the cmd/agezt helper — the legacy builders still there need the original.)
func splitNonEmpty(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// buildTelegram constructs the in-process Telegram channel when
// AGEZT_TELEGRAM_TOKEN is set, plus a Pulse brief sink that forwards briefs to
// the allowlisted chats. Returns NotConfigured when no token is configured.
//
//	AGEZT_TELEGRAM_TOKEN    bot token (required to enable)
//	AGEZT_TELEGRAM_CHAT_ID  comma-separated allowlist of chat ids that may
//	                        drive the agent AND receive Pulse briefs
//
// The inbound handler runs the normal agent loop under the channel's
// correlation, so `agt why`/`agt inbox` link the Telegram message to the task.
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

// buildSlack constructs the in-process Slack channel when AGEZT_SLACK_TOKEN is
// set, plus a Pulse brief sink to the allowlisted channels. Returns
// NotConfigured when no token is configured.
//
//	AGEZT_SLACK_TOKEN           bot token (xoxb-…), required to enable
//	AGEZT_SLACK_SIGNING_SECRET  app signing secret, required for inbound
//	AGEZT_SLACK_ADDR            local addr to serve /slack/events (fronted by a
//	                            tunnel/reverse proxy); empty → outbound-only
//	AGEZT_SLACK_CHANNELS        comma-separated allowlist of channel ids that may
//	                            drive the agent AND receive Pulse briefs
//
// The inbound handler runs the normal agent loop under the channel's correlation,
// so `agt why`/`agt inbox` link the Slack message to the task.
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

// buildEmail constructs an email channel account when AGEZT_EMAIL_SMTP_ADDR
// is set. Outbound over SMTP; two-way when an inbox (IMAP/POP3) is also configured.
//
//	AGEZT_EMAIL_SMTP_ADDR     SMTP server host:port (e.g. smtp.example.com:587), enables
//	AGEZT_EMAIL_FROM          sender address
//	AGEZT_EMAIL_USERNAME      SMTP AUTH username (with PASSWORD); empty → no auth
//	AGEZT_EMAIL_PASSWORD      SMTP AUTH password
//	AGEZT_EMAIL_RECIPIENTS    comma-separated allowlist of addresses (mail targets + inbound senders)
//	AGEZT_EMAIL_INBOX_ADDR    IMAP/POP3 server host:port — enables two-way (poll for new mail)
//	AGEZT_EMAIL_INBOX_PROTOCOL "imap" (default) or "pop3"
//	AGEZT_EMAIL_INBOX_USERNAME/PASSWORD  mailbox creds (default to the SMTP ones)
//	AGEZT_EMAIL_INBOX_TLS     "tls" (default) | "starttls" | "none"
//	AGEZT_EMAIL_INBOX_POLL    poll interval seconds (default 60)
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

// buildDiscord constructs the in-process Discord channel when AGEZT_DISCORD_TOKEN
// is set, plus a Pulse brief sink to the allowlisted channels. Returns
// NotConfigured when no token is configured.
//
//	AGEZT_DISCORD_TOKEN       bot token, required to enable
//	AGEZT_DISCORD_PUBLIC_KEY  app public key (hex), required for inbound verification
//	AGEZT_DISCORD_APP_ID      application id, required for follow-up replies
//	AGEZT_DISCORD_ADDR        local addr to serve /discord/interactions (fronted by
//	                          a tunnel/reverse proxy); empty → outbound-only
//	AGEZT_DISCORD_CHANNELS    comma-separated allowlist of channel ids that may
//	                          drive the agent AND receive Pulse briefs
//
// The inbound handler runs the normal agent loop under the channel's correlation,
// so `agt why`/`agt inbox` link the Discord command to the task.
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

// buildMatrix constructs the in-process Matrix channel when AGEZT_MATRIX_HOMESERVER
// and AGEZT_MATRIX_TOKEN are set, plus a Pulse brief sink to the allowlisted rooms.
// Returns NotConfigured when unconfigured. Mirrors buildTelegram: long-polls /sync
// for inbound, PUTs m.room.message for outbound.
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

// buildWhatsApp constructs the WhatsApp Cloud API channel when
// AGEZT_WHATSAPP_APP_SECRET + AGEZT_WHATSAPP_ACCESS_TOKEN are set. Inbound (signed
// Meta webhook) is served when AGEZT_WHATSAPP_ADDR is also set; outbound + Pulse
// briefs to the allowlisted numbers need AGEZT_WHATSAPP_PHONE_NUMBER_ID.
//
//	AGEZT_WHATSAPP_APP_SECRET       Meta app secret (required; signs inbound)
//	AGEZT_WHATSAPP_ACCESS_TOKEN     Graph API bearer token (required; outbound)
//	AGEZT_WHATSAPP_PHONE_NUMBER_ID  business phone-number id (outbound endpoint)
//	AGEZT_WHATSAPP_VERIFY_TOKEN     token echoed in Meta's GET verify handshake
//	AGEZT_WHATSAPP_ADDR             host:port for the inbound webhook (inbound)
//	AGEZT_WHATSAPP_PATH             inbound route (default /whatsapp)
//	AGEZT_WHATSAPP_NUMBERS          comma-separated allowlist of sender numbers
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
