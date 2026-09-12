// SPDX-License-Identifier: MIT

// Alerter: brief builder + dedupe keys + small payload helpers.
// Code extracted from alerter.go during the Day-133 god-file split.
// Public API unchanged.
package alerter


import (
	"context"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/pulse"
)

func brief(a Alert, ev *event.Event) pulse.Brief {
	title := "⚠ " + a.Title
	if a.Level == LevelCritical {
		title = "🚨 " + a.Title
	}
	body := a.Detail
	if a.Source != "" {
		body = joinNonEmpty("\n", body, "source: "+a.Source)
	}
	return pulse.Brief{
		Title:         title,
		Body:          body,
		Disposition:   pulse.DispAlert,
		IssueKey:      alertIssueKey(a, ev),
		CorrelationID: ev.CorrelationID,
		Items:         1,
	}
}

func dedupeKey(a Alert, ev *event.Event, p map[string]any) string {
	key := string(a.Kind) + "/" + ev.CorrelationID
	if ev.CorrelationID != "" {
		return key
	}
	if strings.EqualFold(strings.TrimSpace(ev.Subject), "doctor.auto_repair") {
		parts := []string{ev.Subject, str(p, "agent"), str(p, "phase"), str(p, "fingerprint")}
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		if len(out) > 0 {
			return strings.Join(out, "/")
		}
	}
	return key
}

func alertIssueKey(a Alert, ev *event.Event) string {
	if strings.EqualFold(strings.TrimSpace(ev.Subject), "doctor.auto_repair") {
		return "alert/" + ev.Subject
	}
	return "alert/" + string(a.Kind)
}

// Start wires the notifier onto the bus: subscribe to everything, classify,
// gate, deliver. Returns false (nothing started) when bus or sink is missing.
// The goroutine stops on ctx cancellation or bus close; a panic in the loop is
// recovered so a notifier bug can never crash the daemon (anomaly pattern).
func Start(ctx context.Context, b *bus.Bus, sink pulse.BriefSink, cfg Config) bool {
	if b == nil || sink == nil {
		return false
	}
	sub, err := b.Subscribe(">", 256)
	if err != nil {
		return false
	}
	n := New(sink, cfg)
	go func() {
		defer func() {
			sub.Cancel()
			_ = recover() // a watcher panic must never take down the daemon
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.C:
				if !ok {
					return
				}
				n.Handle(ev)
			}
		}
	}()
	return true
}

// payloadMap decodes an event payload object, tolerating nil/non-object
// payloads (→ nil map, so every field lookup just comes back empty).
func payloadMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

func str(p map[string]any, key string) string {
	if p == nil {
		return ""
	}
	if v, ok := p[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func firstStr(p map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := str(p, k); s != "" {
			return s
		}
	}
	return ""
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
