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
		DocsURL: "https://core.telegram.org/bots",
	},
	{
		Kind: "whatsapp", Display: "WhatsApp", Transport: "webhook", Duplex: true,
		Description:   "WhatsApp Cloud API (Meta) — two-way messaging with media.",
		ConfigSection: "whatsapp", RequiredEnv: []string{"AGEZT_WHATSAPP_ACCESS_TOKEN", "AGEZT_WHATSAPP_PHONE_NUMBER_ID"},
		DocsURL: "https://developers.facebook.com/docs/whatsapp/cloud-api",
	},
	{
		Kind: "slack", Display: "Slack", Transport: "webhook", Duplex: true,
		Description:   "Slack bot — slash/events with signed inbound verification.",
		ConfigSection: "slack", RequiredEnv: []string{"AGEZT_SLACK_TOKEN"},
		DocsURL: "https://api.slack.com/apps",
	},
	{
		Kind: "discord", Display: "Discord", Transport: "webhook", Duplex: true,
		Description:   "Discord bot — interactions via Ed25519-verified webhook.",
		ConfigSection: "discord", RequiredEnv: []string{"AGEZT_DISCORD_TOKEN"},
		DocsURL: "https://discord.com/developers/docs",
	},
	{
		Kind: "matrix", Display: "Matrix", Transport: "long-poll", Duplex: true,
		Description:   "Matrix — two-way via /sync long-poll on any homeserver.",
		ConfigSection: "matrix", RequiredEnv: []string{"AGEZT_MATRIX_HOMESERVER", "AGEZT_MATRIX_TOKEN"},
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
		DocsURL: "https://github.com/bbernhard/signal-cli-rest-api",
	},
	{
		Kind: "teams", Display: "Microsoft Teams", Transport: "webhook", Duplex: false,
		Description:   "Outbound notifications via Teams Incoming Webhooks.",
		ConfigSection: "teams", RequiredEnv: []string{"AGEZT_TEAMS_WEBHOOKS"},
		DocsURL: "https://learn.microsoft.com/microsoftteams/platform/webhooks-and-connectors/how-to/add-incoming-webhook",
	},
	{
		Kind: "email", Display: "Email / SMTP", Transport: "smtp", Duplex: false,
		Description:   "Outbound email over SMTP.",
		ConfigSection: "email", RequiredEnv: []string{"AGEZT_EMAIL_SMTP_ADDR", "AGEZT_EMAIL_FROM"},
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
		Kind: "googlechat", Display: "Google Chat", Transport: "webhook", Duplex: false,
		Description:   "Outbound messages via a Google Chat Incoming Webhook.",
		ConfigSection: "googlechat", RequiredEnv: []string{"AGEZT_GOOGLECHAT_WEBHOOK"},
		DocsURL: "https://developers.google.com/chat/how-tos/webhooks",
	},
	{
		Kind: "mattermost", Display: "Mattermost", Transport: "webhook", Duplex: false,
		Description:   "Outbound messages via a Mattermost Incoming Webhook.",
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
		Kind: "mastodon", Display: "Mastodon", Transport: "rest", Duplex: false,
		Description:   "Outbound posts to a Mastodon account.",
		ConfigSection: "mastodon", RequiredEnv: []string{"AGEZT_MASTODON_SERVER", "AGEZT_MASTODON_TOKEN"},
		DocsURL: "https://docs.joinmastodon.org/methods/statuses/",
	},
	{
		Kind: "line", Display: "LINE", Transport: "rest", Duplex: false,
		Description:   "Outbound push via the LINE Messaging API.",
		ConfigSection: "line", RequiredEnv: []string{"AGEZT_LINE_TOKEN", "AGEZT_LINE_TO"},
		DocsURL: "https://developers.line.biz/en/docs/messaging-api/",
	},
	{
		Kind: "zulip", Display: "Zulip", Transport: "rest", Duplex: false,
		Description:   "Outbound messages to a Zulip stream via a bot.",
		ConfigSection: "zulip", RequiredEnv: []string{"AGEZT_ZULIP_SERVER", "AGEZT_ZULIP_EMAIL", "AGEZT_ZULIP_APIKEY", "AGEZT_ZULIP_STREAM"},
		DocsURL: "https://zulip.com/api/send-message",
	},
	{
		Kind: "feishu", Display: "Feishu / Lark", Transport: "webhook", Duplex: false,
		Description:   "Outbound messages via a Feishu/Lark custom bot.",
		ConfigSection: "feishu", RequiredEnv: []string{"AGEZT_FEISHU_WEBHOOK"},
		DocsURL: "https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot",
	},
	{
		Kind: "dingtalk", Display: "DingTalk", Transport: "webhook", Duplex: false,
		Description:   "Outbound messages via a DingTalk custom robot.",
		ConfigSection: "dingtalk", RequiredEnv: []string{"AGEZT_DINGTALK_WEBHOOK"},
		DocsURL: "https://open.dingtalk.com/document/robots/custom-robot-access",
	},
	{
		Kind: "wecom", Display: "WeChat Work (WeCom)", Transport: "webhook", Duplex: false,
		Description:   "Outbound messages via a WeCom group robot.",
		ConfigSection: "wecom", RequiredEnv: []string{"AGEZT_WECOM_WEBHOOK"},
		DocsURL: "https://developer.work.weixin.qq.com/document/path/91770",
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
		DocsURL: "https://waha.devlike.pro/",
	},
	{
		Kind: "imessage", Display: "iMessage", Transport: "rest", Duplex: true,
		Description:   "iMessage via a self-hosted BlueBubbles server (a Mac bridge) — REST send + inbound webhook.",
		ConfigSection: "imessage", RequiredEnv: []string{"AGEZT_IMESSAGE_URL"},
		DocsURL: "https://bluebubbles.app/",
	},
}
