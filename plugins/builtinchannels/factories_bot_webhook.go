// SPDX-License-Identifier: MIT
//
// Chat-webhook factory helpers (parseNamedWebhooks, chatWebhookFactory).
// Extracted from factories_bot.go during Day 211 god-file refactor (#62).
// Public API unchanged.
package builtinchannels

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/plugins/channels/chatwebhook"
	"github.com/agezt/agezt/plugins/channels/push"
)

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
