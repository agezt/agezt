// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)

// cmdPulse runs `agt pulse` — a live tail of the daemon's bus.
//
//	agt pulse                       # all events (subject ">")
//	agt pulse --subject agent.>     # filter by subject pattern
//	agt pulse --kind llm.response   # filter by kind (repeatable)
//	agt pulse --kind llm.response --kind tool.result
//	agt pulse --json                # one JSON object per line (machine-readable)
//
// Pattern syntax is the same NATS-style wildcards the bus uses
// internally (see kernel/bus): `*` matches one segment, `>` matches
// one-or-more trailing segments. Kind filter is applied by the
// daemon, so kinds you exclude never cross the socket.
//
// The command runs until SIGINT/SIGTERM; Ctrl+C exits cleanly.
func cmdPulse(args []string, stdout, stderr io.Writer) int {
	// Subcommand routing: `agt pulse {status|pause|resume}` controls the
	// resident engine (SPEC-03 §2). Bare `agt pulse [flags]` keeps its
	// original meaning — a live tail of the event bus — so existing
	// scripts and the `--subject/--kind/--json` flags are unaffected.
	if len(args) > 0 {
		switch args[0] {
		case "status", "pause", "resume":
			return cmdPulseControl(args[0], args[1:], stdout, stderr)
		case "asks":
			return cmdPulseAsks(args[1:], stdout, stderr)
		}
	}

	pattern := ">"
	var kinds []string
	correlation := ""
	asJSON := false
	showText := false
	since := int64(-1)
	sinceTSMs := int64(-1)
	until := int64(-1)
	untilTSMs := int64(-1)
	replayRate := float64(0)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--subject", "-s":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s pulse: --subject needs a value\n", brand.CLI)
				return 2
			}
			pattern = strings.TrimSpace(args[i])
		case "--kind", "-k":
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s pulse: --kind needs a value\n", brand.CLI)
				return 2
			}
			kinds = append(kinds, strings.TrimSpace(args[i]))
		case "--correlation", "-c":
			// Filter to a single correlation chain (live tail
			// complement to `agt why`). AND-composed with every
			// other filter.
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s pulse: --correlation needs a value\n", brand.CLI)
				return 2
			}
			correlation = strings.TrimSpace(args[i])
		case "--since":
			// Historical-replay seq cutoff (M1.aa). 0 = replay
			// everything in the journal; N = replay events with
			// seq >= N. Useful for "show me everything since the
			// last task started" and audit reconstruction. After
			// the replay completes the stream continues live.
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s pulse: --since needs a value\n", brand.CLI)
				return 2
			}
			n, err := strconv.ParseInt(strings.TrimSpace(args[i]), 10, 64)
			if err != nil || n < 0 {
				fmt.Fprintf(stderr, "%s pulse: --since must be a non-negative integer (got %q)\n", brand.CLI, args[i])
				return 2
			}
			since = n
		case "--replay-rate":
			// Cap on events-per-second emitted during the historical
			// replay (M1.nn). 0 / unset = unlimited.
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s pulse: --replay-rate needs a value (events/sec)\n", brand.CLI)
				return 2
			}
			n, err := strconv.ParseFloat(strings.TrimSpace(args[i]), 64)
			if err != nil || n < 0 {
				fmt.Fprintf(stderr, "%s pulse: --replay-rate must be a non-negative number (got %q)\n", brand.CLI, args[i])
				return 2
			}
			replayRate = n
		case "--until":
			// Upper-bound seq cutoff (M1.ii). Exclusive: events with
			// seq < until are replayed. Triggers replay-only mode on
			// the server (no transition to live), so the command
			// exits naturally once the window drains.
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s pulse: --until needs a value\n", brand.CLI)
				return 2
			}
			n, err := strconv.ParseInt(strings.TrimSpace(args[i]), 10, 64)
			if err != nil || n < 0 {
				fmt.Fprintf(stderr, "%s pulse: --until must be a non-negative integer (got %q)\n", brand.CLI, args[i])
				return 2
			}
			until = n
		case "--until-last":
			// Upper-bound time cutoff (M1.ii). "Events older than N
			// ago" — useful for "give me the window between 1 hour
			// ago and 30 minutes ago" via --last 1h --until-last 30m.
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s pulse: --until-last needs a duration\n", brand.CLI)
				return 2
			}
			dur, err := time.ParseDuration(strings.TrimSpace(args[i]))
			if err != nil || dur <= 0 {
				fmt.Fprintf(stderr, "%s pulse: --until-last must be a positive Go duration (got %q)\n", brand.CLI, args[i])
				return 2
			}
			untilTSMs = time.Now().Add(-dur).UnixMilli()
		case "--last":
			// Historical-replay time cutoff (M1.gg). Pure client-side
			// sugar that converts "5m" / "1h30m" / "45s" to a Unix-ms
			// timestamp the server filters by. Composes with --since
			// as AND — operators who set both get the intersection,
			// which is conservative but predictable.
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "%s pulse: --last needs a duration (e.g. 5m, 1h, 30s)\n", brand.CLI)
				return 2
			}
			dur, err := time.ParseDuration(strings.TrimSpace(args[i]))
			if err != nil || dur <= 0 {
				fmt.Fprintf(stderr, "%s pulse: --last must be a positive Go duration (got %q)\n", brand.CLI, args[i])
				return 2
			}
			sinceTSMs = time.Now().Add(-dur).UnixMilli()
		case "--json":
			asJSON = true
		case "--text", "-t":
			// Append a one-line excerpt of each event's `text` payload (the
			// streamed answer tokens and a reasoning model's chain of thought),
			// so an operator can watch the live content — not just event kinds —
			// in the human tail. Off by default; the structured one-line format
			// is unchanged without it.
			showText = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s pulse [--subject PATTERN] [--kind KIND]... [--correlation ID] [--since N] [--last DUR] [--text] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "live tail of the daemon's event bus; Ctrl+C to exit\n")
			fmt.Fprintf(stdout, "  --text            show a one-line excerpt of each event's text (streamed answer + reasoning)\n")
			fmt.Fprintf(stdout, "  --since N         replay matching events with seq >= N before going live\n")
			fmt.Fprintf(stdout, "  --last DUR        replay matching events from the last DUR (e.g. 5m, 1h30m) before going live\n")
			fmt.Fprintf(stdout, "  --correlation ID  show only events on the given correlation chain (live counterpart to `agt why`)\n")
			fmt.Fprintf(stdout, "                    all filters compose as AND (intersection)\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s pulse: unknown flag %q\n", brand.CLI, args[i])
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}

	// Ctrl+C → cancel ctx → client closes conn → server's writeResp
	// returns an error → handler unwinds. Pulse is the first command
	// that genuinely cares about signals; halt/resume/etc finish in
	// milliseconds.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	reqArgs := map[string]any{"pattern": pattern}
	if len(kinds) > 0 {
		kindAny := make([]any, len(kinds))
		for i, k := range kinds {
			kindAny[i] = k
		}
		reqArgs["kinds"] = kindAny
	}
	if correlation != "" {
		reqArgs["correlation"] = correlation
	}
	if since >= 0 {
		reqArgs["since"] = since
	}
	if sinceTSMs >= 0 {
		reqArgs["since_ts_ms"] = sinceTSMs
	}
	if until >= 0 {
		reqArgs["until"] = until
	}
	if untilTSMs >= 0 {
		reqArgs["until_ts_ms"] = untilTSMs
	}
	if replayRate > 0 {
		reqArgs["replay_rate"] = replayRate
	}

	if !asJSON {
		fmt.Fprintf(stdout, "pulse: subscribed to %q (Ctrl+C to exit)\n", pattern)
		if len(kinds) > 0 {
			fmt.Fprintf(stdout, "       filtering kinds: %s\n", strings.Join(kinds, ", "))
		}
		if correlation != "" {
			fmt.Fprintf(stdout, "       filtering correlation: %s\n", correlation)
		}
		if since >= 0 {
			fmt.Fprintf(stdout, "       replaying from seq=%d\n", since)
		}
		if sinceTSMs >= 0 {
			fmt.Fprintf(stdout, "       replaying since %s\n", time.UnixMilli(sinceTSMs).Format("15:04:05"))
		}
	}

	err := c.StreamUntilCancel(ctx, controlplane.CmdPulseSubscribe, reqArgs, func(ev *event.Event) {
		if asJSON {
			renderEventJSON(stdout, ev)
		} else {
			renderEventHuman(stdout, ev, showText)
		}
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s pulse: %v\n", brand.CLI, err)
		return 1
	}
	return 0
}

// renderEventHuman prints a one-line summary suitable for an
// operator skimming the stream. Format:
//
//	14:32:07.412  seq=0042  agent.spawned     subject=agent.01H...  actor=kernel
//
// Kept tight so a 100-column terminal fits comfortably. The full
// event JSON is available via `agt why <event_id>` for any line.
func renderEventHuman(w io.Writer, ev *event.Event, showText bool) {
	ts := time.UnixMilli(ev.TSUnixMS)
	if ev.TSUnixMS == 0 {
		ts = time.Now()
	}
	seq := fmt.Sprintf("seq=%-5d", ev.Seq)
	if ev.IsEphemeral() {
		// Ephemeral events (streaming tokens) have Seq=0 and no Hash —
		// distinguish them so operators don't mistake the column for a
		// real journal sequence number.
		seq = "seq=eph  "
	}
	suffix := ""
	if showText {
		if excerpt := eventTextExcerpt(ev); excerpt != "" {
			suffix = "  ▸ " + excerpt
		}
	}
	fmt.Fprintf(w, "%s  %s  %-22s  subject=%s  actor=%s%s\n",
		ts.Format("15:04:05.000"),
		seq,
		ev.Kind,
		ev.Subject,
		ev.Actor,
		suffix,
	)
}

// eventTextExcerpt returns a single-line, length-bounded excerpt of an event's
// `text` payload field (the streamed answer tokens and a reasoning model's chain
// of thought both carry it), for the `--text` view. Newlines/tabs collapse to
// spaces so the tail stays one line per event; empty for events without text.
func eventTextExcerpt(ev *event.Event) string {
	if len(ev.Payload) == 0 {
		return ""
	}
	var p struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(ev.Payload, &p) != nil || p.Text == "" {
		return ""
	}
	s := strings.Join(strings.Fields(p.Text), " ") // collapse all whitespace runs
	return truncate(s, 160)
}

// renderEventJSON prints one JSON object per line. Useful for piping
// into jq or feeding a log aggregator.
func renderEventJSON(w io.Writer, ev *event.Event) {
	enc, err := json.Marshal(ev)
	if err != nil {
		// Shouldn't happen — event is a well-typed struct — but if it
		// does we'd rather lose one line than crash the stream.
		return
	}
	_, _ = w.Write(enc)
	_, _ = w.Write([]byte{'\n'})
}
