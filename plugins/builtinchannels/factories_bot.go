// SPDX-License-Identifier: MIT

// builtinchannels: bot/webhook channel adapters (OneBot factory + Zalo +
// Nostr + parseNamedWebhooks helper + chatWebhookFactory + DingTalk +
// Feishu + WeCom + Mastodon). Split from factories.go during Day 211
// god-file refactor (#44). Public API unchanged.
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
	"github.com/agezt/agezt/plugins/channels/feishu"
	"github.com/agezt/agezt/plugins/channels/mastodon"
	"github.com/agezt/agezt/plugins/channels/nostr"
	"github.com/agezt/agezt/plugins/channels/onebot"
	"github.com/agezt/agezt/plugins/channels/push"
	"github.com/agezt/agezt/plugins/channels/wecom"
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
// Two-way wins the kind's name: when no inbound addr is set but the
// incoming-webhook URL is, the factory falls back to the outbound-only push
// entry instead — the suppression rule cmd/agezt's twoWayChatConfigured
// predicate used to express, now owned by the kind's own factory.
func chatWebhookFactory(kind, prefix string) channelwire.Factory {
	return func(d channelwire.Deps) channelwire.Built {
		get := func(s string) string { return strings.TrimSpace(d.Get(brand.EnvPrefix + prefix + s)) }
		if get("_ADDR") == "" {
			if u := get("_WEBHOOK"); u != "" {
				return pushBuilt(d, push.Config{Kind: kind, URL: u})
			}
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
		// Two-way unconfigured — fall back to the outbound-only push entry via
		// the custom-robot webhook when set (two-way wins the "dingtalk" name).
		if u := strings.TrimSpace(d.Get(brand.EnvPrefix + "DINGTALK_WEBHOOK")); u != "" {
			return pushBuilt(d, push.Config{Kind: push.KindDingTalk, URL: u})
		}
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
		// Two-way unconfigured — fall back to the outbound-only push entry via
		// the custom-bot webhook when set (two-way wins the "feishu" name).
		if u := strings.TrimSpace(d.Get(brand.EnvPrefix + "FEISHU_WEBHOOK")); u != "" {
			return pushBuilt(d, push.Config{Kind: push.KindFeishu, URL: u})
		}
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
		// Two-way unconfigured — fall back to the outbound-only push entry via
		// the group-robot webhook when set (two-way wins the "wecom" name).
		if u := strings.TrimSpace(d.Get(brand.EnvPrefix + "WECOM_WEBHOOK")); u != "" {
			return pushBuilt(d, push.Config{Kind: push.KindWeCom, URL: u})
		}
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
// Two-way wins the "mastodon" name: when no acct allowlist is set but a token
// is, the factory falls back to the outbound-only push entry (post statuses,
// don't poll mentions) — the suppression rule cmd/agezt's
// twoWayMastodonConfigured predicate used to express, now owned here.
func buildMastodon(d channelwire.Deps) channelwire.Built {
	if strings.TrimSpace(d.Get(brand.EnvPrefix+"MASTODON_USERS")) == "" {
		if token := strings.TrimSpace(d.Get(brand.EnvPrefix + "MASTODON_TOKEN")); token != "" {
			return pushBuilt(d, push.Config{
				Kind:   push.KindMastodon,
				Server: strings.TrimSpace(d.Get(brand.EnvPrefix + "MASTODON_SERVER")),
				Token:  token,
			})
		}
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

// pushBuilt wraps one outbound push.Channel (plugins/channels/push) as a
// channelwire.Built: the channel itself, a brief sink that delivers via its
// Send, and a "push → <kind>" desc. Returns NotConfigured when push.New
// rejects the config — mirroring buildPushChannels' silent skip on a
// construction error (e.g. LINE token set but no recipient).
