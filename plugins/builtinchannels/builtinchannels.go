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
}
