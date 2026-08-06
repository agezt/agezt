// SPDX-License-Identifier: MIT

// Channel factories (Phase 2.1 of docs/REFACTORING-SCAN-2026-08.md): the
// modern per-account builders, moved verbatim out of cmd/agezt/main.go into
// channelwire.Factory form. Each factory reads config ONLY through d.Get,
// uses d.Bus / d.Handler for the kernel surface, and returns
// channelwire.NotConfigured when its enabling env is unset.
package builtinchannels

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/plugins/channels/chatwebhook"
	"github.com/agezt/agezt/plugins/channels/dingtalk"
	"github.com/agezt/agezt/plugins/channels/discord"
	"github.com/agezt/agezt/plugins/channels/email"
	"github.com/agezt/agezt/plugins/channels/feishu"
	"github.com/agezt/agezt/plugins/channels/homeassistant"
	"github.com/agezt/agezt/plugins/channels/imessage"
	"github.com/agezt/agezt/plugins/channels/irc"
	linechan "github.com/agezt/agezt/plugins/channels/line"
	"github.com/agezt/agezt/plugins/channels/mastodon"
	"github.com/agezt/agezt/plugins/channels/matrix"
	"github.com/agezt/agezt/plugins/channels/nextcloudtalk"
	"github.com/agezt/agezt/plugins/channels/nostr"
	"github.com/agezt/agezt/plugins/channels/onebot"
	signalchan "github.com/agezt/agezt/plugins/channels/signal"
	"github.com/agezt/agezt/plugins/channels/slack"
	"github.com/agezt/agezt/plugins/channels/sms"
	"github.com/agezt/agezt/plugins/channels/teams"
	"github.com/agezt/agezt/plugins/channels/telegram"
	webhookchan "github.com/agezt/agezt/plugins/channels/webhook"
	"github.com/agezt/agezt/plugins/channels/wecom"
	"github.com/agezt/agezt/plugins/channels/whatsapp"
	"github.com/agezt/agezt/plugins/channels/whatsappgw"
	"github.com/agezt/agezt/plugins/channels/zalo"
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

// buildWebhook constructs the vendor-neutral webhook channel. Enabled when an
// inbound secret OR an outbound URL is configured. Returns NotConfigured when
// neither is set.
//
//	AGEZT_WEBHOOK_SECRET        HMAC-SHA256 signing key (enables signed inbound)
//	AGEZT_WEBHOOK_ADDR          local addr to serve the inbound route (fronted by
//	                            a tunnel/reverse proxy); empty → no inbound listener
//	AGEZT_WEBHOOK_PATH          inbound route (default /webhook)
//	AGEZT_WEBHOOK_CHANNELS      comma-separated allowlist of channel ids that may
//	                            drive the agent AND receive Pulse briefs
//	AGEZT_WEBHOOK_OUTBOUND_URL  where Send / briefs POST (signed); empty → inbound-only
//
// The inbound handler runs the normal agent loop under the channel's correlation,
// so `agt why`/`agt inbox` link the webhook command to the task.
func buildWebhook(d channelwire.Deps) channelwire.Built {
	secret := strings.TrimSpace(d.Get(brand.EnvPrefix + "WEBHOOK_SECRET"))
	outboundURL := strings.TrimSpace(d.Get(brand.EnvPrefix + "WEBHOOK_OUTBOUND_URL"))
	if secret == "" && outboundURL == "" {
		return channelwire.NotConfigured
	}
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "WEBHOOK_ADDR"))
	path := strings.TrimSpace(d.Get(brand.EnvPrefix + "WEBHOOK_PATH"))
	channelIDs := splitNonEmpty(d.Get(brand.EnvPrefix + "WEBHOOK_CHANNELS"))

	ch := webhookchan.New(webhookchan.Config{
		Addr:        addr,
		Path:        path,
		Secret:      secret,
		Allowlist:   channel.NewAllowlist(channelIDs),
		OutboundURL: outboundURL,
		Bus:         d.Bus,
		Handler:     d.Handler,
	})

	var sink pulse.BriefSink
	if outboundURL != "" && len(channelIDs) > 0 {
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
	case secret == "":
		desc = fmt.Sprintf("outbound-only → %s, allowlist=%d (set AGEZT_WEBHOOK_SECRET + AGEZT_WEBHOOK_ADDR for inbound)", outboundURL, len(channelIDs))
	case addr == "":
		desc = fmt.Sprintf("inbound configured but not listening (set AGEZT_WEBHOOK_ADDR), allowlist=%d", len(channelIDs))
	default:
		p := path
		if p == "" {
			p = webhookchan.DefaultPath
		}
		desc = fmt.Sprintf("inbound at %s%s, allowlist=%d channel(s)", addr, p, len(channelIDs))
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}

// buildIRC constructs the two-way IRC channel when AGEZT_IRC_SERVER +
// AGEZT_IRC_NICK are set. It joins AGEZT_IRC_CHANNELS and acts on inbound from
// allowlisted sources (the joined channels, plus any AGEZT_IRC_ALLOWLIST nicks/
// channels); Pulse briefs tee to the joined channels.
//
//	AGEZT_IRC_SERVER     host:port (e.g. irc.libera.chat:6697)   (required)
//	AGEZT_IRC_NICK       the bot's nick                          (required)
//	AGEZT_IRC_CHANNELS   comma-separated channels to join (#foo) — allowed by default
//	AGEZT_IRC_PASSWORD   optional server password (PASS)
//	AGEZT_IRC_TLS        "true" to force TLS (auto for :6697)
//	AGEZT_IRC_ALLOWLIST  extra allowed sources (nicks for DMs / channels)
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

// buildTwitch constructs a Twitch chat channel when AGEZT_TWITCH_USERNAME +
// AGEZT_TWITCH_TOKEN are set. Twitch chat is IRC, so this reuses the IRC channel
// pinned to Twitch's server with an "oauth:" PASS; it joins AGEZT_TWITCH_CHANNELS
// (lowercase #channel) and acts on inbound from those channels by default.
//
//	AGEZT_TWITCH_USERNAME  the bot account's login name        (required)
//	AGEZT_TWITCH_TOKEN     OAuth token ("oauth:" prefix added) (required)
//	AGEZT_TWITCH_CHANNELS  comma-separated #channels to join — allowed by default
//	AGEZT_TWITCH_ALLOWLIST extra allowed sources (nicks / channels)
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

// buildSMS constructs the Twilio SMS channel when AGEZT_SMS_ACCOUNT_SID +
// AGEZT_SMS_AUTH_TOKEN are set. Inbound (signed Twilio webhook) is served when
// AGEZT_SMS_ADDR is also set; outbound texts + Pulse briefs to the allowlisted
// numbers (AGEZT_SMS_NUMBERS) need AGEZT_SMS_FROM.
//
//	AGEZT_SMS_ACCOUNT_SID  Twilio Account SID  (required)
//	AGEZT_SMS_AUTH_TOKEN   Twilio auth token   (required; signs inbound + REST)
//	AGEZT_SMS_FROM         Twilio number to send from, E.164 (outbound)
//	AGEZT_SMS_ADDR         host:port for the inbound webhook (inbound)
//	AGEZT_SMS_PATH         inbound route (default /sms)
//	AGEZT_SMS_PUBLIC_URL   exact public URL Twilio POSTs to (signature check behind a tunnel)
//	AGEZT_SMS_NUMBERS      comma-separated allowlist of sender numbers
func buildSMS(d channelwire.Deps) channelwire.Built {
	sid := strings.TrimSpace(d.Get(brand.EnvPrefix + "SMS_ACCOUNT_SID"))
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "SMS_AUTH_TOKEN"))
	if sid == "" || token == "" {
		return channelwire.NotConfigured
	}
	from := strings.TrimSpace(d.Get(brand.EnvPrefix + "SMS_FROM"))
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "SMS_ADDR"))
	path := strings.TrimSpace(d.Get(brand.EnvPrefix + "SMS_PATH"))
	publicURL := strings.TrimSpace(d.Get(brand.EnvPrefix + "SMS_PUBLIC_URL"))
	numbers := splitNonEmpty(d.Get(brand.EnvPrefix + "SMS_NUMBERS"))

	ch := sms.New(sms.Config{
		Addr:       addr,
		Path:       path,
		AccountSID: sid,
		AuthToken:  token,
		From:       from,
		PublicURL:  publicURL,
		Allowlist:  channel.NewAllowlist(numbers),
		Bus:        d.Bus,
		Handler:    d.Handler,
	})

	// Pulse briefs → the allowlisted numbers (needs an outbound From).
	var sink pulse.BriefSink
	if from != "" && len(numbers) > 0 {
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
		desc = fmt.Sprintf("outbound-only (set AGEZT_SMS_ADDR for inbound), allowlist=%d number(s)", len(numbers))
	default:
		p := path
		if p == "" {
			p = sms.DefaultPath
		}
		desc = fmt.Sprintf("inbound at %s%s, allowlist=%d number(s)", addr, p, len(numbers))
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}

// buildSignal constructs the in-process Signal channel when AGEZT_SIGNAL_API_URL
// and AGEZT_SIGNAL_NUMBER are set, plus a Pulse brief sink to the allowlisted
// numbers. Returns NotConfigured when unconfigured. Talks to an operator-run
// signal-cli-rest-api: long-polls /v1/receive for inbound, POSTs /v2/send for
// outbound (mirrors buildMatrix).
//
//	AGEZT_SIGNAL_API_URL     signal-cli-rest-api base URL (required), e.g. http://127.0.0.1:8080
//	AGEZT_SIGNAL_NUMBER      the registered Signal number this bot is, E.164 (required)
//	AGEZT_SIGNAL_RECIPIENTS  comma-separated allowlist of sender numbers (+ brief recipients)
//	AGEZT_SIGNAL_TOKEN       optional bearer token (a reverse proxy fronting the API)
//	AGEZT_SIGNAL_POLL_SECS   /v1/receive long-poll seconds (default 10)
func buildSignal(d channelwire.Deps) channelwire.Built {
	apiURL := strings.TrimSpace(d.Get(brand.EnvPrefix + "SIGNAL_API_URL"))
	number := strings.TrimSpace(d.Get(brand.EnvPrefix + "SIGNAL_NUMBER"))
	if apiURL == "" || number == "" {
		return channelwire.NotConfigured
	}
	recipients := splitNonEmpty(d.Get(brand.EnvPrefix + "SIGNAL_RECIPIENTS"))
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "SIGNAL_TOKEN"))
	poll := 0
	if v := strings.TrimSpace(d.Get(brand.EnvPrefix + "SIGNAL_POLL_SECS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			poll = n
		}
	}

	ch := signalchan.New(signalchan.Config{
		APIURL:          apiURL,
		Number:          number,
		Token:           token,
		Allowlist:       channel.NewAllowlist(recipients),
		Bus:             d.Bus,
		Handler:         d.Handler,
		PollTimeoutSecs: poll,
	})

	// Pulse briefs → the allowlisted numbers. Nil sink when none configured (the
	// bot can still receive commands once a number is allowlisted, and operators
	// can still `agt send --channel signal --to <number>`).
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

	desc := fmt.Sprintf("listening, allowlist=%d number(s)", len(recipients))
	if len(recipients) == 0 {
		desc = "listening, NO allowlist (outbound-only; set AGEZT_SIGNAL_RECIPIENTS to allow commands)"
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}

// buildWhatsAppGateway constructs the easy-path WhatsApp channel — a self-hosted
// WAHA or Evolution API gateway (QR login, no Meta) — when AGEZT_WHATSAPPGW_URL
// is set. Outbound always; inbound (two-way) when AGEZT_WHATSAPPGW_ADDR points
// the gateway's webhook at this daemon. Briefs tee to the allowlisted numbers.
//
//	AGEZT_WHATSAPPGW_URL      gateway base URL, e.g. http://localhost:3000  (required)
//	AGEZT_WHATSAPPGW_BACKEND  "waha" (default) or "evolution"
//	AGEZT_WHATSAPPGW_SESSION  WAHA session / Evolution instance (default "default")
//	AGEZT_WHATSAPPGW_KEY      gateway API key
//	AGEZT_WHATSAPPGW_NUMBERS  comma-separated allowed sender numbers
//	AGEZT_WHATSAPPGW_ADDR     host:port to serve the inbound webhook (two-way)
//	AGEZT_WHATSAPPGW_PATH     inbound route (default /whatsappgw)
//	AGEZT_WHATSAPPGW_SECRET   optional shared secret the gateway must echo inbound
func buildWhatsAppGateway(d channelwire.Deps) channelwire.Built {
	base := strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPPGW_URL"))
	if base == "" {
		return channelwire.NotConfigured
	}
	numbers := splitNonEmpty(d.Get(brand.EnvPrefix + "WHATSAPPGW_NUMBERS"))
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPPGW_ADDR"))
	backend := strings.ToLower(strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPPGW_BACKEND")))

	ch := whatsappgw.New(whatsappgw.Config{
		Backend:   backend,
		BaseURL:   base,
		Session:   strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPPGW_SESSION")),
		APIKey:    strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPPGW_KEY")),
		Allowlist: channel.NewAllowlist(numbers),
		Bus:       d.Bus,
		Handler:   d.Handler,
		Addr:      addr,
		Path:      strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPPGW_PATH")),
		Secret:    strings.TrimSpace(d.Get(brand.EnvPrefix + "WHATSAPPGW_SECRET")),
	})

	var sink pulse.BriefSink
	if len(numbers) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, n := range numbers {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: n, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	be := backend
	if be == "" {
		be = "waha"
	}
	desc := fmt.Sprintf("%s via %s, allowlist=%d number(s)", base, be, len(numbers))
	if addr == "" {
		desc += " (outbound-only; set AGEZT_WHATSAPPGW_ADDR for two-way)"
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}

// buildIMessage constructs the iMessage channel — a self-hosted BlueBubbles
// server (a Mac bridge; https://bluebubbles.app) — when AGEZT_IMESSAGE_URL is
// set. Outbound always; inbound (two-way) when AGEZT_IMESSAGE_ADDR points the
// BlueBubbles webhook back at this channel.
//
//	AGEZT_IMESSAGE_URL        BlueBubbles server URL, e.g. http://localhost:1234  (required)
//	AGEZT_IMESSAGE_PASSWORD   BlueBubbles server password
//	AGEZT_IMESSAGE_METHOD     "private-api" (default) or "apple-script"
//	AGEZT_IMESSAGE_ADDRESSES  comma-separated allowed sender addresses (phone/email)
//	AGEZT_IMESSAGE_ADDR       host:port to serve the inbound webhook (two-way)
//	AGEZT_IMESSAGE_PATH       inbound route (default /imessage)
//	AGEZT_IMESSAGE_SECRET     optional shared secret the webhook must echo (X-Webhook-Secret)
func buildIMessage(d channelwire.Deps) channelwire.Built {
	base := strings.TrimSpace(d.Get(brand.EnvPrefix + "IMESSAGE_URL"))
	if base == "" {
		return channelwire.NotConfigured
	}
	addresses := splitNonEmpty(d.Get(brand.EnvPrefix + "IMESSAGE_ADDRESSES"))
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "IMESSAGE_ADDR"))

	ch := imessage.New(imessage.Config{
		BaseURL:   base,
		Password:  strings.TrimSpace(d.Get(brand.EnvPrefix + "IMESSAGE_PASSWORD")),
		Method:    strings.ToLower(strings.TrimSpace(d.Get(brand.EnvPrefix + "IMESSAGE_METHOD"))),
		Allowlist: channel.NewAllowlist(addresses),
		Bus:       d.Bus,
		Handler:   d.Handler,
		Addr:      addr,
		Path:      strings.TrimSpace(d.Get(brand.EnvPrefix + "IMESSAGE_PATH")),
		Secret:    strings.TrimSpace(d.Get(brand.EnvPrefix + "IMESSAGE_SECRET")),
	})

	var sink pulse.BriefSink
	if len(addresses) > 0 {
		sink = pulse.SinkFunc(func(b pulse.Brief) error {
			var firstErr error
			for _, a := range addresses {
				if err := ch.Send(d.Ctx, channel.Outbound{ChannelID: a, Text: formatBrief(b), Priority: channel.PriorityNotify}); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		})
	}

	desc := fmt.Sprintf("%s via BlueBubbles, allowlist=%d address(es)", base, len(addresses))
	if addr == "" {
		desc += " (outbound-only; set AGEZT_IMESSAGE_ADDR for two-way)"
	}
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
}

// buildNextcloudTalk constructs the Nextcloud Talk channel when AGEZT_NEXTCLOUDTALK_URL
// + AGEZT_NEXTCLOUDTALK_SECRET are set. Inbound (signed bot webhook) is served when
// AGEZT_NEXTCLOUDTALK_ADDR is also set; outbound replies + Pulse briefs go to the
// allowlisted conversation tokens (AGEZT_NEXTCLOUDTALK_TOKENS).
//
//	AGEZT_NEXTCLOUDTALK_URL     Nextcloud base URL, e.g. https://cloud.example.com (required)
//	AGEZT_NEXTCLOUDTALK_SECRET  shared bot secret; signs/verifies (required)
//	AGEZT_NEXTCLOUDTALK_ADDR    host:port for the inbound webhook (inbound)
//	AGEZT_NEXTCLOUDTALK_PATH    inbound route (default /nextcloudtalk)
//	AGEZT_NEXTCLOUDTALK_TOKENS  comma-separated allowlist of conversation tokens
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

// buildHomeAssistant constructs the outbound Home Assistant channel when
// AGEZT_HOMEASSISTANT_URL + AGEZT_HOMEASSISTANT_TOKEN are set. Pulse briefs +
// `agt send` go to the allowlisted notify services (AGEZT_HOMEASSISTANT_SERVICES).
//
//	AGEZT_HOMEASSISTANT_URL       HA base, e.g. http://homeassistant.local:8123
//	AGEZT_HOMEASSISTANT_TOKEN     long-lived access token
//	AGEZT_HOMEASSISTANT_SERVICES  comma-separated notify service allowlist
//	                              (e.g. mobile_app_phone,persistent_notification)
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

// buildTeams constructs the outbound Microsoft Teams channel when
// AGEZT_TEAMS_WEBHOOKS is set. Pulse briefs + `agt send` post a card to the named
// Teams Incoming Webhooks.
//
//	AGEZT_TEAMS_WEBHOOKS  name=url,name2=url2 — named Teams Incoming Webhook URLs;
//	                      `agt send --channel teams --to <name>` selects one.
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

// buildLine constructs the two-way LINE channel (official Messaging API) when a
// channel secret is set. Outbound push uses AGEZT_LINE_TOKEN; inbound (two-way)
// is served when AGEZT_LINE_ADDR is set and verified with AGEZT_LINE_SECRET.
//
//	AGEZT_LINE_TOKEN    channel access token (Bearer for reply/push)
//	AGEZT_LINE_SECRET   channel secret (verifies inbound signature; enables two-way)  (required)
//	AGEZT_LINE_USERS    comma-separated allowed sender userIds
//	AGEZT_LINE_TO       recipient id for Pulse briefs
//	AGEZT_LINE_ADDR     host:port to serve LINE's webhook (two-way)
//	AGEZT_LINE_PATH     inbound route (default /line)
//
// The outbound-only push LINE entry yields the "line" name to this channel via
// twoWayLineConfigured in cmd/agezt (which mirrors the secret check here for
// the DEFAULT instance; the push-suppression cluster migrates in batch D).
func buildLine(d channelwire.Deps) channelwire.Built {
	secret := strings.TrimSpace(d.Get(brand.EnvPrefix + "LINE_SECRET"))
	if secret == "" {
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

// oneBotFactory returns the factory for a QQ / WeChat channel over a OneBot v11
// gateway, enabled when its inbound addr is set. QQ and WeChat have no
// first-party bot API; a self-hosted gateway (go-cqhttp / NapCat / Lagrange for
// QQ; wcf / wechatbot for WeChat) speaks OneBot. kind is the channel name;
// prefix is the env namespace — the parameterization the whole channel-factory
// refactor aims for: one builder, two registered kinds.
//
//	AGEZT_<PREFIX>_GATEWAY  gateway HTTP API base, e.g. http://localhost:5700
//	AGEZT_<PREFIX>_TOKEN    gateway access token (bearer)
//	AGEZT_<PREFIX>_SECRET   HMAC-SHA1 secret verifying inbound X-Signature
//	AGEZT_<PREFIX>_USERS    comma-separated allowed user ids
//	AGEZT_<PREFIX>_ADDR     host:port to serve the inbound webhook (enables two-way)
//	AGEZT_<PREFIX>_PATH     inbound route (default /<kind>)
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

// buildZalo constructs the two-way Zalo channel (Official Account API) when
// AGEZT_ZALO_ADDR is set. Replies + briefs use the OA message API.
//
//	AGEZT_ZALO_APP_ID   OA app id (part of the inbound signature)
//	AGEZT_ZALO_TOKEN    OA access token (sends)
//	AGEZT_ZALO_SECRET   OA secret key (verifies the inbound signature)
//	AGEZT_ZALO_USERS    comma-separated allowed user ids
//	AGEZT_ZALO_ADDR     host:port to serve the inbound webhook (enables two-way)
//	AGEZT_ZALO_PATH     inbound route (default /zalo)
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

// buildNostr constructs the Nostr channel when AGEZT_NOSTR_PRIVKEY + AGEZT_NOSTR_RELAYS
// are set. It connects to the relays, answers kind-1 mentions of the agent's
// pubkey from allowlisted authors, and posts briefs as standalone notes.
//
//	AGEZT_NOSTR_PRIVKEY  64-char hex secret key (required)
//	AGEZT_NOSTR_RELAYS   comma-separated wss:// relay URLs (required)
//	AGEZT_NOSTR_AUTHORS  comma-separated author pubkeys (hex) allowed to drive the agent
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

// parseNamedWebhooks parses a "name=url,name2=url2" spec into a name→url map.
// Each entry splits on the FIRST '=' (URLs may contain '='); blank names/urls
// are dropped. (Moved from cmd/agezt/main.go with buildTeams — its only user.)
func parseNamedWebhooks(spec string) map[string]string {
	out := map[string]string{}
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		eq := strings.IndexByte(entry, '=')
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(entry[:eq])
		url := strings.TrimSpace(entry[eq+1:])
		if name != "" && url != "" {
			out[name] = url
		}
	}
	return out
}

// chatWebhookFactory returns the factory for a two-way Google Chat / Mattermost
// channel, enabled when its inbound addr is set. Outbound + replies use the
// incoming-webhook URL. kind is the channel name; prefix is the env namespace —
// one builder, two registered kinds (like oneBotFactory).
//
//	AGEZT_<PREFIX>_WEBHOOK  incoming-webhook URL (outbound + replies)
//	AGEZT_<PREFIX>_ADDR     host:port to serve the inbound webhook (enables two-way)
//	AGEZT_<PREFIX>_TOKEN    verification token (Mattermost outgoing-webhook token / Google Chat ?token=)
//	AGEZT_<PREFIX>_USERS    comma-separated allowed senders (usernames / emails)
//	AGEZT_<PREFIX>_PATH     inbound route (default /<kind>)
//
// The addr gate inlines cmd/agezt's twoWayChatConfigured predicate, which
// still lives there to yield the outbound-only push entry for the DEFAULT
// instance (the push-suppression cluster migrates with the push family).
func chatWebhookFactory(kind, prefix string) channelwire.Factory {
	return func(d channelwire.Deps) channelwire.Built {
		get := func(s string) string { return strings.TrimSpace(d.Get(brand.EnvPrefix + prefix + s)) }
		if get("_ADDR") == "" {
			return channelwire.NotConfigured
		}
		users := splitNonEmpty(d.Get(brand.EnvPrefix + prefix + "_USERS"))
		webhook := get("_WEBHOOK")

		ch := chatwebhook.New(chatwebhook.Config{
			Kind:       kind,
			WebhookURL: webhook,
			Token:      get("_TOKEN"),
			Allowlist:  channel.NewAllowlist(users),
			Bus:        d.Bus,
			Handler:    d.Handler,
			Addr:       get("_ADDR"),
			Path:       get("_PATH"),
		})

		var sink pulse.BriefSink
		if webhook != "" {
			sink = pulse.SinkFunc(func(b pulse.Brief) error {
				return ch.Send(d.Ctx, channel.Outbound{Text: formatBrief(b), Priority: channel.PriorityNotify})
			})
		}

		desc := fmt.Sprintf("%s two-way, allowlist=%d sender(s)", kind, len(users))
		if webhook == "" {
			desc += " (inbound-only; set AGEZT_" + prefix + "_WEBHOOK for replies/briefs)"
		}
		return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: desc}
	}
}

// buildDingTalk constructs the two-way DingTalk channel when AGEZT_DINGTALK_ADDR
// is set. Replies go to each message's sessionWebhook; briefs use the custom
// robot webhook (AGEZT_DINGTALK_WEBHOOK).
//
//	AGEZT_DINGTALK_WEBHOOK  custom-robot webhook (outbound briefs / agt send)
//	AGEZT_DINGTALK_SECRET   robot secret (verifies inbound timestamp+sign)
//	AGEZT_DINGTALK_USERS    comma-separated allowed senderStaffId / nick
//	AGEZT_DINGTALK_ADDR     host:port to serve the inbound webhook (enables two-way)
//	AGEZT_DINGTALK_PATH     inbound route (default /dingtalk)
func buildDingTalk(d channelwire.Deps) channelwire.Built {
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "DINGTALK_ADDR"))
	if addr == "" {
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

// buildFeishu constructs the two-way Feishu / Lark channel when AGEZT_FEISHU_ADDR
// is set. Replies + briefs use the IM API (tenant_access_token from app id/secret).
//
//	AGEZT_FEISHU_APP_ID        app id
//	AGEZT_FEISHU_APP_SECRET    app secret
//	AGEZT_FEISHU_VERIFY_TOKEN  event verification token
//	AGEZT_FEISHU_CHAT          chat_id for proactive briefs
//	AGEZT_FEISHU_USERS         comma-separated allowed sender open_ids
//	AGEZT_FEISHU_ADDR          host:port to serve the inbound webhook (enables two-way)
//	AGEZT_FEISHU_PATH          inbound route (default /feishu)
func buildFeishu(d channelwire.Deps) channelwire.Built {
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "FEISHU_ADDR"))
	appID := strings.TrimSpace(d.Get(brand.EnvPrefix + "FEISHU_APP_ID"))
	if addr == "" || appID == "" {
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

// buildWeCom constructs the two-way WeCom (WeChat Work) channel when
// AGEZT_WECOM_ADDR is set. Inbound is an AES-encrypted callback; replies use the
// app message-send API (access_token from corp id/secret).
//
//	AGEZT_WECOM_CORP_ID      corp id
//	AGEZT_WECOM_CORP_SECRET  app secret
//	AGEZT_WECOM_AGENT_ID     app agent id
//	AGEZT_WECOM_TOKEN        callback token
//	AGEZT_WECOM_AES_KEY      callback EncodingAESKey
//	AGEZT_WECOM_USERS        comma-separated allowed user ids
//	AGEZT_WECOM_ADDR         host:port to serve the inbound callback (enables two-way)
//	AGEZT_WECOM_PATH         inbound route (default /wecom)
func buildWeCom(d channelwire.Deps) channelwire.Built {
	addr := strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_ADDR"))
	corp := strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_CORP_ID"))
	if addr == "" || corp == "" {
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

// buildMastodon constructs the two-way Mastodon channel when AGEZT_MASTODON_USERS
// is set (alongside SERVER + TOKEN). It polls mention notifications and replies as
// threaded statuses; outbound briefs post standalone statuses.
//
//	AGEZT_MASTODON_SERVER  instance base URL (required)
//	AGEZT_MASTODON_TOKEN   access token, read:notifications + write:statuses (required)
//	AGEZT_MASTODON_USERS   comma-separated acct handles allowed to drive the agent (enables two-way)
//	AGEZT_MASTODON_POLL    poll interval seconds (default 60)
//
// The users gate inlines cmd/agezt's twoWayMastodonConfigured predicate, which
// still lives there to yield the outbound-only push entry for the DEFAULT
// instance (the push-suppression cluster migrates with the push family).
func buildMastodon(d channelwire.Deps) channelwire.Built {
	if strings.TrimSpace(d.Get(brand.EnvPrefix+"MASTODON_USERS")) == "" {
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
