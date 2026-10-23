// SPDX-License-Identifier: MIT

package pulse

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/pulseapi"
)

// The ROWS now live in the contract layer (kernel/contract/pulseapi) — W3.34.
// The brief is delivered to surfaces that are PLUGINS (the built-in channels
// are), and a plugin may reach the contract and nothing else, so the row and
// the sinks that carry it travel together. The module keeps every name as an
// alias, so there is one row and one spelling of it.
//
// What stays here is the composition: composeBrief, composeDigest and the
// quiet-hours window — the engine's work, not the row's.

type (
	// Brief is one composed message ready for delivery (SPEC-03 §6).
	Brief = pulseapi.Brief
	// BriefSink delivers a composed brief to a surface.
	BriefSink = pulseapi.BriefSink
	// LogSink writes a one-line human brief to an io.Writer (the daemon's
	// stdout).
	LogSink = pulseapi.LogSink
	// SinkFunc adapts a plain function to a BriefSink.
	SinkFunc = pulseapi.SinkFunc
	// MultiSink fans a brief out to several sinks.
	MultiSink = pulseapi.MultiSink
)

// composeBrief turns one scored delta into a single-item brief (SPEC-03 §6.2:
// a one-liner for a single fact).
func composeBrief(d Delta, sc Score, corr string) Brief {
	return Brief{
		Title:         d.Summary,
		Disposition:   sc.Disposition,
		IssueKey:      d.IssueKey(),
		CorrelationID: corr,
		Items:         1,
	}
}

// composeDigest batches accumulated briefs into one coherent message
// (SPEC-03 §6.2: "3 things in your portfolio overnight: …"), grouped by
// source for readability rather than a flat list of pings.
func composeDigest(items []Brief) Brief {
	bySource := map[string][]string{}
	order := []string{}
	for _, it := range items {
		src := sourceOf(it.IssueKey)
		if _, ok := bySource[src]; !ok {
			order = append(order, src)
		}
		bySource[src] = append(bySource[src], it.Title)
	}
	var b strings.Builder
	for _, src := range order {
		fmt.Fprintf(&b, "%s:\n", src)
		for _, t := range bySource[src] {
			fmt.Fprintf(&b, "  - %s\n", t)
		}
	}
	return Brief{
		Title:       strconv.Itoa(len(items)) + " update(s) since the last digest",
		Body:        strings.TrimRight(b.String(), "\n"),
		Disposition: DispDigest,
		IssueKey:    "digest",
		Items:       len(items),
	}
}

func sourceOf(issueKey string) string {
	if i := strings.IndexByte(issueKey, '/'); i >= 0 {
		return issueKey[:i]
	}
	return issueKey
}

// QuietHours is a daily window during which only alert/act briefs break
// through (SPEC-03 §6.3). Handles windows that wrap midnight (e.g. 22→7).
type QuietHours struct {
	Enabled bool
	Start   int // hour [0,24)
	End     int // hour [0,24)
}

// Spec renders the window back into the "START-END" form ParseQuietHours accepts
// (M770), or "" when disabled — so a live change can be persisted and round-tripped.
func (q QuietHours) Spec() string {
	if !q.Enabled {
		return ""
	}
	return fmt.Sprintf("%d-%d", q.Start, q.End)
}

// Active reports whether t falls inside the quiet window.
func (q QuietHours) Active(t time.Time) bool {
	if !q.Enabled {
		return false
	}
	h := t.Hour()
	if q.Start == q.End {
		return false
	}
	if q.Start < q.End {
		return h >= q.Start && h < q.End
	}
	// Wraps midnight: e.g. 22..24 or 0..7.
	return h >= q.Start || h < q.End
}

// ParseQuietHours parses "START-END" (24h hours), e.g. "22-7". Returns a
// disabled window on any parse error so a bad config never breaks Pulse.
func ParseQuietHours(s string) QuietHours {
	s = strings.TrimSpace(s)
	if s == "" {
		return QuietHours{}
	}
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return QuietHours{}
	}
	start, err1 := strconv.Atoi(strings.TrimSpace(a))
	end, err2 := strconv.Atoi(strings.TrimSpace(b))
	if err1 != nil || err2 != nil || start < 0 || start > 23 || end < 0 || end > 23 {
		return QuietHours{}
	}
	return QuietHours{Enabled: true, Start: start, End: end}
}
