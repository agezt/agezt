// SPDX-License-Identifier: MIT
//
// SMS-style channel factories (buildSMS, buildSignal). Extracted from
// factories_chat.go during Day 211 god-file refactor (#61).
// Public API unchanged.
package builtinchannels

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/pulse"
	signalchan "github.com/agezt/agezt/plugins/channels/signal"
	"github.com/agezt/agezt/plugins/channels/sms"
)

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
