// SPDX-License-Identifier: MIT
//
// cmd/agezt channel instance lifecycle (collectChannels, combineSinks,
// chanInstance, wireInstances, startInstances, instanceSinks,
// registerInstances, instanceMatch, liveChannelKeys, briefSink).
// Extracted from main_channels.go during Day 211 god-file refactor (#66).
// Public API unchanged.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/pulse"
)

func collectChannels() []controlplane.ChannelInfo {
	env := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	var out []controlplane.ChannelInfo
	for _, m := range channel.Manifests() {
		configured := len(m.RequiredEnv) > 0
		for _, e := range m.RequiredEnv {
			if env(e) == "" {
				configured = false
				break
			}
		}
		// The generic webhook is usable outbound-only with just an outbound
		// URL — the one kind whose "configured" predicate is an OR the
		// manifest's all-required semantics can't express.
		if !configured && m.Kind == "webhook" && env("AGEZT_WEBHOOK_OUTBOUND_URL") != "" {
			configured = true
		}
		if !configured {
			continue
		}
		inbound := m.Duplex
		for _, e := range m.InboundEnv {
			if env(e) == "" {
				inbound = false
				break
			}
		}
		info := controlplane.ChannelInfo{Kind: m.Kind, Inbound: inbound}
		if m.AddrEnv != "" {
			info.Addr = env(m.AddrEnv)
		}
		if m.AllowlistEnv != "" {
			info.Allowlist = len(splitNonEmpty(env(m.AllowlistEnv)))
		}
		out = append(out, info)
	}
	return out
}
func combineSinks(sinks ...pulse.BriefSink) pulse.BriefSink {
	var live pulse.MultiSink
	for _, s := range sinks {
		if s != nil {
			live = append(live, s)
		}
	}
	switch len(live) {
	case 0:
		return nil
	case 1:
		return live[0]
	default:
		return live
	}
}
type chanInstance struct {
	key  string // channel.InstanceKey(kind, label): bare kind for the default, "kind#label" otherwise
	desc string
	ch   channel.Channel
	sink pulse.BriefSink
}
func wireInstances(insts []channelwire.Instance) []chanInstance {
	var out []chanInstance
	for _, in := range insts {
		out = append(out, chanInstance{key: in.Key, desc: in.Desc, ch: in.Channel, sink: in.Sink})
	}
	return out
}
func startInstances(ctx context.Context, stdout io.Writer, kind, label, disabledHint string, insts []chanInstance) {
	if len(insts) == 0 {
		if disabledHint != "" {
			fmt.Fprintf(stdout, "  %-16s : %s\n", label, disabledHint)
		}
		return
	}
	for _, in := range insts {
		go in.ch.Start(ctx)
		who := in.key
		if who == kind {
			who = "default"
		}
		fmt.Fprintf(stdout, "  %-16s : %s [%s]\n", label, in.desc, who)
	}
}
func instanceSinks(groups ...[]chanInstance) []pulse.BriefSink {
	var out []pulse.BriefSink
	for _, g := range groups {
		for _, in := range g {
			if in.sink != nil {
				out = append(out, in.sink)
			}
		}
	}
	return out
}
func registerInstances(live map[string]channel.Channel, groups ...[]chanInstance) {
	for _, g := range groups {
		for _, in := range g {
			live[in.key] = in.ch
		}
	}
}
func instanceMatch(keys []string, target string) []string {
	if strings.Contains(target, "#") {
		for _, k := range keys {
			if k == target {
				return []string{target}
			}
		}
		return nil
	}
	var out []string
	for _, k := range keys {
		if base, _, _ := strings.Cut(k, "#"); base == target {
			out = append(out, k)
		}
	}
	return out
}
func liveChannelKeys(live map[string]channel.Channel) []string {
	out := make([]string, 0, len(live))
	for k := range live {
		out = append(out, k)
	}
	return out
}
func briefSink(stdout io.Writer, extra pulse.BriefSink) pulse.BriefSink {
	log := pulse.LogSink{W: stdout}
	if extra == nil {
		return log
	}
	return pulse.MultiSink{log, extra}
}
