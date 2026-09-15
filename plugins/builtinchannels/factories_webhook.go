// SPDX-License-Identifier: MIT
//
// builtinchannels: generic Webhook channel factory.
// Extracted from factories.go during Day 211 god-file refactor (#68).
// Public API unchanged.
package builtinchannels

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	webhookchan "github.com/agezt/agezt/plugins/channels/webhook"
)

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
