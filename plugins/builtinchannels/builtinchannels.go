// SPDX-License-Identifier: MIT

// Package builtinchannels registers the manifests for AGEZT's built-in
// communication channels (Telegram, WhatsApp, Slack, …) into the channel
// registry. It's the single place that describes every shipped channel for the
// Channels wizard — kept out of the kernel (which must not import plugins) and
// out of the per-channel packages (which stay transport-only). Adding a new
// built-in channel = one entry here plus its Config Center section; a future
// out-of-tree channel can call channel.RegisterManifest itself.
package builtinchannels

import "github.com/agezt/agezt/kernel/channel"

// RegisterAll seeds the channel registry with the built-in manifests. Called
// once at daemon start (idempotent).
func RegisterAll() {
	for _, m := range manifests {
		channel.RegisterManifest(m)
	}
}

// Manifests returns the built-in manifests (for tests / inspection).
func Manifests() []channel.Manifest { return append([]channel.Manifest(nil), manifests...) }

var manifests = []channel.Manifest{
	{
		Kind: "telegram", Display: "Telegram", Transport: "long-poll", Duplex: true,
		Description:   "Telegram bot — two-way chat and notifications via @BotFather.",
		ConfigSection: "telegram", RequiredEnv: []string{"AGEZT_TELEGRAM_TOKEN"},
		Media:         channel.MediaCaps{ImageIn: true, VoiceIn: true, ImageOut: true, VoiceOut: true},
		ConnectMethod: "token",
		SetupSteps: []string{
			"Open a chat with @BotFather in Telegram.",
			"Send /newbot and follow the prompts to name your bot.",
			"Copy the bot token it gives you into the Bot token field.",
			"Message your new bot, then add your chat id to Allowed chat IDs (get it from @userinfobot).",
		},
		DocsURL: "https://core.telegram.org/bots",
	},
	{
		Kind: "whatsapp", Display: "WhatsApp", Transport: "webhook", Duplex: true,
		Description:   "WhatsApp Cloud API (Meta) — two-way messaging with media.",
		ConfigSection: "whatsapp", RequiredEnv: []string{"AGEZT_WHATSAPP_ACCESS_TOKEN", "AGEZT_WHATSAPP_PHONE_NUMBER_ID"},
		Media:         channel.MediaCaps{ImageIn: true, VoiceIn: true, ImageOut: true, VoiceOut: true},
		ConnectMethod: "token",
		SetupSteps: []string{
			"In Meta for Developers, create an app and add the WhatsApp product.",
			"Copy the access token and phone number ID from the API setup page.",
			"Set the app secret and a webhook addr so inbound is signature-verified.",
			"Add the recipient numbers the agent is allowed to message. (Or use the easier WhatsApp Gateway option.)",
		},
		DocsURL: "https://developers.facebook.com/docs/whatsapp/cloud-api",
	},
	{
		Kind: "slack", Display: "Slack", Transport: "webhook", Duplex: true,
		Description:   "Slack bot — slash/events with signed inbound verification.",
		ConfigSection: "slack", RequiredEnv: []string{"AGEZT_SLACK_TOKEN"},
		Media:         channel.MediaCaps{ImageIn: true, VoiceIn: true, ImageOut: true, VoiceOut: true},
		ConnectMethod: "token",
		SetupSteps: []string{
			"Create a Slack app at api.slack.com/apps (From scratch).",
			"Add a Bot Token scope (chat:write) and install the app to your workspace.",
			"Copy the Bot User OAuth Token (xoxb-…) into the token field.",
			"Add the channel IDs the agent may post to in Allowed channels.",
		},
		DocsURL: "https://api.slack.com/apps",
	},
	{
		Kind: "discord", Display: "Discord", Transport: "webhook", Duplex: true,
		Description:   "Discord bot — interactions via Ed25519-verified webhook.",
		ConfigSection: "discord", RequiredEnv: []string{"AGEZT_DISCORD_TOKEN"},
		Media:         channel.MediaCaps{ImageIn: true, VoiceIn: true, ImageOut: true, VoiceOut: true},
		ConnectMethod: "token",
		SetupSteps: []string{
			"Create an application at discord.com/developers, then add a Bot.",
			"Copy the Bot token into the token field.",
			"Invite the bot to your server with the bot scope and message permissions.",
			"Add the channel IDs the agent may post to in Allowed channels.",
		},
		DocsURL: "https://discord.com/developers/docs",
	},
	{
		Kind: "matrix", Display: "Matrix", Transport: "long-poll", Duplex: true,
		Description:   "Matrix — two-way via /sync long-poll on any homeserver.",
		ConfigSection: "matrix", RequiredEnv: []string{"AGEZT_MATRIX_HOMESERVER", "AGEZT_MATRIX_TOKEN"},
		Media:   channel.MediaCaps{ImageIn: true, VoiceIn: true, ImageOut: true, VoiceOut: true},
		DocsURL: "https://matrix.org",
	},
	{
		Kind: "sms", Display: "SMS (Twilio)", Transport: "webhook", Duplex: true,
		Description:   "SMS via Twilio Programmable Messaging.",
		ConfigSection: "sms", RequiredEnv: []string{"AGEZT_SMS_ACCOUNT_SID", "AGEZT_SMS_AUTH_TOKEN"},
		DocsURL: "https://www.twilio.com/docs/messaging",
	},
	{
		Kind: "signal", Display: "Signal", Transport: "rest", Duplex: true,
		Description:   "Signal via a signal-cli REST gateway.",
		ConfigSection: "signal", RequiredEnv: []string{"AGEZT_SIGNAL_API_URL", "AGEZT_SIGNAL_NUMBER"},
		ConnectMethod: "gateway",
		SetupSteps: []string{
			"Run a signal-cli-rest-api gateway (Docker) and register/link your number there.",
			"Enter the gateway API URL and the Signal number (e.g. +1555…) below.",
			"Add the numbers the agent is allowed to talk to.",
		},
		DocsURL: "https://github.com/bbernhard/signal-cli-rest-api",
	},
	{
		Kind: "teams", Display: "Microsoft Teams", Transport: "webhook", Duplex: false,
		Description:   "Outbound notifications via Teams Incoming Webhooks.",
		ConfigSection: "teams", RequiredEnv: []string{"AGEZT_TEAMS_WEBHOOKS"},
		DocsURL: "https://learn.microsoft.com/microsoftteams/platform/webhooks-and-connectors/how-to/add-incoming-webhook",
	},
	{
		Kind: "email", Display: "Email / SMTP", Transport: "smtp", Duplex: true,
		Description:   "Email — outbound over SMTP, two-way with an IMAP/POP3 inbox. Add several accounts, each its own server.",
		ConfigSection: "email", RequiredEnv: []string{"AGEZT_EMAIL_SMTP_ADDR", "AGEZT_EMAIL_FROM"},
		ConnectMethod: "token",
		SetupSteps: []string{
			"Find your provider's SMTP host:port (e.g. smtp.gmail.com:587).",
			"For Gmail/Outlook, create an app password (not your login password).",
			"Enter the SMTP server, your from address, username and app password.",
			"Add the recipient addresses the agent is allowed to email.",
		},
	},
	{
		Kind: "homeassistant", Display: "Home Assistant", Transport: "rest", Duplex: false,
		Description:   "Outbound notifications via the Home Assistant notify API.",
		ConfigSection: "homeassistant", RequiredEnv: []string{"AGEZT_HOMEASSISTANT_URL", "AGEZT_HOMEASSISTANT_TOKEN"},
		DocsURL: "https://www.home-assistant.io/integrations/notify",
	},
	{
		Kind: "webhook", Display: "Generic Webhook", Transport: "webhook", Duplex: true,
		Description:   "Vendor-neutral signed-JSON channel — bridge anything.",
		ConfigSection: "webhook", RequiredEnv: []string{"AGEZT_WEBHOOK_SECRET"},
	},
	{
		Kind: "ntfy", Display: "ntfy", Transport: "rest", Duplex: false,
		Description:   "Outbound push notifications via ntfy.sh or a self-hosted server.",
		ConfigSection: "ntfy", RequiredEnv: []string{"AGEZT_NTFY_TOPIC"},
		DocsURL: "https://ntfy.sh",
	},
	{
		Kind: "pushover", Display: "Pushover", Transport: "rest", Duplex: false,
		Description:   "Outbound push notifications to phones via Pushover.",
		ConfigSection: "pushover", RequiredEnv: []string{"AGEZT_PUSHOVER_TOKEN", "AGEZT_PUSHOVER_USER"},
		DocsURL: "https://pushover.net",
	},
	{
		Kind: "gotify", Display: "Gotify", Transport: "rest", Duplex: false,
		Description:   "Outbound push via a self-hosted Gotify server.",
		ConfigSection: "gotify", RequiredEnv: []string{"AGEZT_GOTIFY_SERVER", "AGEZT_GOTIFY_TOKEN"},
		DocsURL: "https://gotify.net",
	},
	{
		Kind: "pushbullet", Display: "Pushbullet", Transport: "rest", Duplex: false,
		Description:   "Outbound push notifications via Pushbullet.",
		ConfigSection: "pushbullet", RequiredEnv: []string{"AGEZT_PUSHBULLET_TOKEN"},
		DocsURL: "https://www.pushbullet.com",
	},
	{
		Kind: "googlechat", Display: "Google Chat", Transport: "webhook", Duplex: true,
		Description:   "Google Chat — outbound via an Incoming Webhook, and two-way (app event webhook in) with an inbound addr.",
		ConfigSection: "googlechat", RequiredEnv: []string{"AGEZT_GOOGLECHAT_WEBHOOK"},
		DocsURL: "https://developers.google.com/chat/how-tos/webhooks",
	},
	{
		Kind: "mattermost", Display: "Mattermost", Transport: "webhook", Duplex: true,
		Description:   "Mattermost — outbound via an Incoming Webhook, and two-way (outgoing-webhook in) with an inbound addr.",
		ConfigSection: "mattermost", RequiredEnv: []string{"AGEZT_MATTERMOST_WEBHOOK"},
		DocsURL: "https://developers.mattermost.com/integrate/webhooks/incoming/",
	},
	{
		Kind: "rocketchat", Display: "Rocket.Chat", Transport: "webhook", Duplex: false,
		Description:   "Outbound messages via a Rocket.Chat Incoming Webhook.",
		ConfigSection: "rocketchat", RequiredEnv: []string{"AGEZT_ROCKETCHAT_WEBHOOK"},
		DocsURL: "https://docs.rocket.chat/docs/integrations",
	},
	{
		Kind: "mastodon", Display: "Mastodon", Transport: "rest", Duplex: true,
		Description:   "Mastodon — outbound posts, and two-way (polls mention notifications → threaded replies) with an acct allowlist.",
		ConfigSection: "mastodon", RequiredEnv: []string{"AGEZT_MASTODON_SERVER", "AGEZT_MASTODON_TOKEN"},
		DocsURL: "https://docs.joinmastodon.org/methods/statuses/",
	},
	{
		Kind: "zulip", Display: "Zulip", Transport: "rest", Duplex: false,
		Description:   "Outbound messages to a Zulip stream via a bot.",
		ConfigSection: "zulip", RequiredEnv: []string{"AGEZT_ZULIP_SERVER", "AGEZT_ZULIP_EMAIL", "AGEZT_ZULIP_APIKEY", "AGEZT_ZULIP_STREAM"},
		DocsURL: "https://zulip.com/api/send-message",
	},
	{
		Kind: "feishu", Display: "Feishu / Lark", Transport: "webhook", Duplex: true,
		Description:   "Feishu/Lark — outbound via a custom bot, and two-way (app event subscription + IM API) with app credentials + an inbound addr.",
		ConfigSection: "feishu", RequiredEnv: []string{"AGEZT_FEISHU_WEBHOOK"},
		Media:   channel.MediaCaps{ImageIn: true, VoiceIn: true},
		DocsURL: "https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot",
	},
	{
		Kind: "dingtalk", Display: "DingTalk", Transport: "webhook", Duplex: true,
		Description:   "DingTalk — outbound via a custom robot, and two-way (enterprise robot outgoing webhook → sessionWebhook reply) with an inbound addr.",
		ConfigSection: "dingtalk", RequiredEnv: []string{"AGEZT_DINGTALK_WEBHOOK"},
		DocsURL: "https://open.dingtalk.com/document/robots/custom-robot-access",
	},
	{
		Kind: "wecom", Display: "WeChat Work (WeCom)", Transport: "webhook", Duplex: true,
		Description:   "WeCom — outbound via a group robot, and two-way (AES-encrypted app callback + message-send API) with app credentials + an inbound addr.",
		ConfigSection: "wecom", RequiredEnv: []string{"AGEZT_WECOM_WEBHOOK"},
		Media:   channel.MediaCaps{ImageIn: true, VoiceIn: true},
		DocsURL: "https://developer.work.weixin.qq.com/document/path/91770",
	},
	{
		Kind: "qq", Display: "QQ", Transport: "webhook", Duplex: true,
		Description:   "Two-way QQ via a self-hosted OneBot v11 gateway (go-cqhttp / NapCat / Lagrange).",
		ConfigSection: "qq", RequiredEnv: []string{"AGEZT_QQ_GATEWAY", "AGEZT_QQ_ADDR"},
		Media:         channel.MediaCaps{ImageIn: true, VoiceIn: true},
		ConnectMethod: "gateway",
		DocsURL:       "https://github.com/botuniverse/onebot-11",
	},
	{
		Kind: "wechat", Display: "WeChat", Transport: "webhook", Duplex: true,
		Description:   "Two-way personal WeChat via a self-hosted OneBot-compatible gateway (wcf / wechatbot). No first-party API — gateway required.",
		ConfigSection: "wechat", RequiredEnv: []string{"AGEZT_WECHAT_GATEWAY", "AGEZT_WECHAT_ADDR"},
		Media:         channel.MediaCaps{ImageIn: true, VoiceIn: true},
		ConnectMethod: "gateway",
		DocsURL:       "https://github.com/botuniverse/onebot-11",
	},
	{
		Kind: "nextcloudtalk", Display: "Nextcloud Talk", Transport: "webhook", Duplex: true,
		Description:   "Two-way Nextcloud Talk via the Talk Bot API (signed webhook in + bot message API out).",
		ConfigSection: "nextcloudtalk", RequiredEnv: []string{"AGEZT_NEXTCLOUDTALK_URL", "AGEZT_NEXTCLOUDTALK_SECRET"},
		DocsURL: "https://nextcloud-talk.readthedocs.io/en/latest/bots/",
	},
	{
		Kind: "nostr", Display: "Nostr", Transport: "socket", Duplex: true,
		Description:   "Two-way Nostr over relays — answers signed kind-1 mentions and NIP-04 encrypted DMs of the agent's pubkey, with threaded/encrypted replies.",
		ConfigSection: "nostr", RequiredEnv: []string{"AGEZT_NOSTR_PRIVKEY", "AGEZT_NOSTR_RELAYS"},
		DocsURL: "https://github.com/nostr-protocol/nips/blob/master/01.md",
	},
	{
		Kind: "zalo", Display: "Zalo", Transport: "webhook", Duplex: true,
		Description:   "Two-way Zalo via the Official Account API (event webhook + OA message API).",
		ConfigSection: "zalo", RequiredEnv: []string{"AGEZT_ZALO_TOKEN"},
		DocsURL: "https://developers.zalo.me/docs/official-account",
	},
	{
		Kind: "synology", Display: "Synology Chat", Transport: "webhook", Duplex: false,
		Description:   "Outbound messages via a Synology Chat incoming webhook.",
		ConfigSection: "synology", RequiredEnv: []string{"AGEZT_SYNOLOGY_WEBHOOK"},
		DocsURL: "https://kb.synology.com/DSM/help/Chat/chat_integration",
	},
	{
		Kind: "irc", Display: "IRC", Transport: "socket", Duplex: true,
		Description:   "Two-way IRC — connect to any ircd, join channels, chat with the agent.",
		ConfigSection: "irc", RequiredEnv: []string{"AGEZT_IRC_SERVER", "AGEZT_IRC_NICK"},
		DocsURL: "https://modern.ircdocs.horse/",
	},
	{
		Kind: "twitch", Display: "Twitch", Transport: "socket", Duplex: true,
		Description:   "Two-way Twitch chat (IRC) — read and reply in your stream's channels.",
		ConfigSection: "twitch", RequiredEnv: []string{"AGEZT_TWITCH_USERNAME", "AGEZT_TWITCH_TOKEN"},
		DocsURL: "https://dev.twitch.tv/docs/irc/",
	},
	{
		Kind: "whatsappgw", Display: "WhatsApp (Gateway)", Transport: "rest", Duplex: true,
		Description:   "Easy WhatsApp via a self-hosted WAHA / Evolution gateway (QR login, no Meta).",
		ConfigSection: "whatsappgw", RequiredEnv: []string{"AGEZT_WHATSAPPGW_URL"},
		ConnectMethod: "qr",
		SetupSteps: []string{
			"Run a WAHA or Evolution gateway (Docker one-liner on their docs).",
			"Enter the gateway URL (and API key, if you set one) below.",
			"Save, then scan the QR code with WhatsApp on your phone to link the session.",
		},
		DocsURL: "https://waha.devlike.pro/",
	},
	{
		Kind: "imessage", Display: "iMessage", Transport: "rest", Duplex: true,
		Description:   "iMessage via a self-hosted BlueBubbles server (a Mac bridge) — REST send + inbound webhook.",
		ConfigSection: "imessage", RequiredEnv: []string{"AGEZT_IMESSAGE_URL"},
		Media:         channel.MediaCaps{ImageIn: true, VoiceIn: true, ImageOut: true, VoiceOut: true},
		ConnectMethod: "gateway",
		DocsURL:       "https://bluebubbles.app/",
	},
	{
		Kind: "line", Display: "LINE", Transport: "webhook", Duplex: true,
		Description:   "LINE Messaging API — outbound push, and two-way (signed webhook + reply token) with a channel secret + inbound addr.",
		ConfigSection: "line", RequiredEnv: []string{"AGEZT_LINE_TOKEN"},
		Media:   channel.MediaCaps{ImageIn: true, VoiceIn: true},
		DocsURL: "https://developers.line.biz/en/docs/messaging-api/",
	},
}
