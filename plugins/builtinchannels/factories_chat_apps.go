// SPDX-License-Identifier: MIT
//
// Mobile-app channel factories (buildWhatsAppGateway, buildIMessage).
// Extracted from factories_chat.go during Day 211 god-file refactor (#61).
// Public API unchanged.
package builtinchannels

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/plugins/channels/imessage"
	"github.com/agezt/agezt/plugins/channels/whatsappgw"
)

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
