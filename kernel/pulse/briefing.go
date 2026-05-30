// SPDX-License-Identifier: MIT

package pulse

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Brief is one composed message ready for delivery (SPEC-03 §6).
type Brief struct {
	Title         string      `json:"title"`
	Body          string      `json:"body,omitempty"`
	Disposition   Disposition `json:"disposition"`
	IssueKey      string      `json:"issue_key"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	Items         int         `json:"items"` // 1 for a single brief, N for a digest
}

// BriefSink delivers a composed brief to a surface. v1 ships LogSink (daemon
// log + journal via the engine); the Telegram sink slots in here in Phase 4
// without touching the engine.
type BriefSink interface {
	Deliver(b Brief) error
}

// LogSink writes a one-line human brief to an io.Writer (the daemon's
// stdout). This is the Phase 3 "brief to CLI/log" delivery.
type LogSink struct{ W io.Writer }

// Deliver implements BriefSink.
func (l LogSink) Deliver(b Brief) error {
	if l.W == nil {
		return nil
	}
	if b.Body != "" {
		_, err := fmt.Fprintf(l.W, "📣 BRIEF [%s] %s\n%s\n", b.Disposition, b.Title, indent(b.Body))
		return err
	}
	_, err := fmt.Fprintf(l.W, "📣 BRIEF [%s] %s\n", b.Disposition, b.Title)
	return err
}

// SinkFunc adapts a plain function to a BriefSink. The daemon uses it to wrap
// a channel's Send into a sink (e.g. brief → Telegram sendMessage) without the
// pulse package depending on any channel.
type SinkFunc func(Brief) error

// Deliver implements BriefSink.
func (f SinkFunc) Deliver(b Brief) error { return f(b) }

// MultiSink fans a brief out to several sinks (e.g. the daemon log AND
// Telegram). Delivery continues to every sink; the first error is returned but
// does not stop the others — a Telegram outage must not suppress the log line.
type MultiSink []BriefSink

// Deliver implements BriefSink.
func (m MultiSink) Deliver(b Brief) error {
	var firstErr error
	for _, s := range m {
		if s == nil {
			continue
		}
		if err := s.Deliver(b); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, ln := range lines {
		lines[i] = "    " + ln
	}
	return strings.Join(lines, "\n")
}

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
