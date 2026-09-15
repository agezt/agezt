// SPDX-License-Identifier: MIT

// builtinchannels: push-style channel adapters (pushBuilt helper + Ntfy +
// Pushover + Gotify + Pushbullet + RocketChat + Zulip + Synology).
// Split from factories.go during Day 211 god-file refactor (#44).
// Public API unchanged.
package builtinchannels

import (
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/plugins/channels/push"
)

func pushBuilt(d channelwire.Deps, cfg push.Config) channelwire.Built {
	cfg.Bus = d.Bus
	ch, err := push.New(cfg)
	if err != nil {
		return channelwire.NotConfigured
	}
	sink := pulse.SinkFunc(func(b pulse.Brief) error {
		return ch.Send(d.Ctx, channel.Outbound{Text: formatBrief(b), Priority: channel.PriorityNotify})
	})
	return channelwire.Built{Channels: []channel.Channel{ch}, Sink: sink, Desc: "push → " + ch.Name()}
}

// buildNtfy constructs the outbound ntfy push channel when AGEZT_NTFY_TOPIC is
// set. Briefs/`agt send` POST to the topic. Outbound-only.
//
//	AGEZT_NTFY_TOPIC   topic to publish to (required)
//	AGEZT_NTFY_SERVER  base URL (default https://ntfy.sh)
//	AGEZT_NTFY_TOKEN   optional bearer token
func buildNtfy(d channelwire.Deps) channelwire.Built {
	topic := strings.TrimSpace(d.Get(brand.EnvPrefix + "NTFY_TOPIC"))
	if topic == "" {
		return channelwire.NotConfigured
	}
	return pushBuilt(d, push.Config{
		Kind:   push.KindNtfy,
		Server: strings.TrimSpace(d.Get(brand.EnvPrefix + "NTFY_SERVER")),
		Topic:  topic,
		Token:  strings.TrimSpace(d.Get(brand.EnvPrefix + "NTFY_TOKEN")),
	})
}

// buildPushover constructs the outbound Pushover push channel when
// AGEZT_PUSHOVER_TOKEN is set. Outbound-only.
//
//	AGEZT_PUSHOVER_TOKEN  app token (required)
//	AGEZT_PUSHOVER_USER   user/group key (required by the API)
func buildPushover(d channelwire.Deps) channelwire.Built {
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "PUSHOVER_TOKEN"))
	if token == "" {
		return channelwire.NotConfigured
	}
	return pushBuilt(d, push.Config{
		Kind:  push.KindPushover,
		Token: token,
		User:  strings.TrimSpace(d.Get(brand.EnvPrefix + "PUSHOVER_USER")),
	})
}

// buildGotify constructs the outbound Gotify push channel when
// AGEZT_GOTIFY_TOKEN is set. Outbound-only.
//
//	AGEZT_GOTIFY_TOKEN   app token (required)
//	AGEZT_GOTIFY_SERVER  self-hosted server base URL (required by the API)
func buildGotify(d channelwire.Deps) channelwire.Built {
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "GOTIFY_TOKEN"))
	if token == "" {
		return channelwire.NotConfigured
	}
	return pushBuilt(d, push.Config{
		Kind:   push.KindGotify,
		Server: strings.TrimSpace(d.Get(brand.EnvPrefix + "GOTIFY_SERVER")),
		Token:  token,
	})
}

// buildPushbullet constructs the outbound Pushbullet push channel when
// AGEZT_PUSHBULLET_TOKEN is set. Outbound-only.
//
//	AGEZT_PUSHBULLET_TOKEN  access token (required)
func buildPushbullet(d channelwire.Deps) channelwire.Built {
	token := strings.TrimSpace(d.Get(brand.EnvPrefix + "PUSHBULLET_TOKEN"))
	if token == "" {
		return channelwire.NotConfigured
	}
	return pushBuilt(d, push.Config{Kind: push.KindPushbullet, Token: token})
}

// buildRocketChat constructs the outbound Rocket.Chat push channel when
// AGEZT_ROCKETCHAT_WEBHOOK is set. Outbound-only.
//
//	AGEZT_ROCKETCHAT_WEBHOOK  incoming-webhook URL (required)
func buildRocketChat(d channelwire.Deps) channelwire.Built {
	u := strings.TrimSpace(d.Get(brand.EnvPrefix + "ROCKETCHAT_WEBHOOK"))
	if u == "" {
		return channelwire.NotConfigured
	}
	return pushBuilt(d, push.Config{Kind: push.KindRocketChat, URL: u})
}

// buildZulip constructs the outbound Zulip push channel when
// AGEZT_ZULIP_APIKEY is set. Outbound-only (bot posts to a stream).
//
//	AGEZT_ZULIP_APIKEY  bot API key (required)
//	AGEZT_ZULIP_SERVER  Zulip server base URL (required by the API)
//	AGEZT_ZULIP_EMAIL   bot email (required by the API)
//	AGEZT_ZULIP_STREAM  stream to post to (required by the API)
//	AGEZT_ZULIP_TOPIC   topic within the stream (default "agezt")
func buildZulip(d channelwire.Deps) channelwire.Built {
	key := strings.TrimSpace(d.Get(brand.EnvPrefix + "ZULIP_APIKEY"))
	if key == "" {
		return channelwire.NotConfigured
	}
	return pushBuilt(d, push.Config{
		Kind:   push.KindZulip,
		Server: strings.TrimSpace(d.Get(brand.EnvPrefix + "ZULIP_SERVER")),
		User:   strings.TrimSpace(d.Get(brand.EnvPrefix + "ZULIP_EMAIL")),
		Token:  key,
		Target: strings.TrimSpace(d.Get(brand.EnvPrefix + "ZULIP_STREAM")),
		Topic:  strings.TrimSpace(d.Get(brand.EnvPrefix + "ZULIP_TOPIC")),
	})
}

// buildSynology constructs the outbound Synology Chat push channel when
// AGEZT_SYNOLOGY_WEBHOOK is set. Outbound-only.
//
//	AGEZT_SYNOLOGY_WEBHOOK  incoming-webhook URL (required)
func buildSynology(d channelwire.Deps) channelwire.Built {
	u := strings.TrimSpace(d.Get(brand.EnvPrefix + "SYNOLOGY_WEBHOOK"))
	if u == "" {
		return channelwire.NotConfigured
	}
	return pushBuilt(d, push.Config{Kind: push.KindSynology, URL: u})
}
