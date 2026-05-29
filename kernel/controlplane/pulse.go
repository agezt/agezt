// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"

	"github.com/ersinkoc/agezt/kernel/bus"
	"github.com/ersinkoc/agezt/kernel/event"
)

// handlePulseSubscribe is the server side of `agt pulse`. It opens a
// long-lived bus subscription matching `args.pattern` (default ">"),
// optionally filters by `args.kinds` (a []string of event.Kind names),
// and streams matching events to the client until either:
//
//   - the server context is cancelled (daemon shutting down),
//   - the client closes the connection (write returns an error), or
//   - the subscription is dropped (channel closed unexpectedly).
//
// **Wire convention.** Pulse never sends RespResult. The protocol
// expects exactly one terminal response per request, so the client
// reads `RespEvent` lines in a loop until its own context is
// cancelled and it closes the conn — at which point the server's
// next writeResp fails and the handler returns. This is the simplest
// "server-push, client-terminates" shape on top of the existing
// single-line-JSON transport, without changing the framing.
//
// **Why not just reuse CmdRun's pattern.** CmdRun subscribes to a
// single run's correlation-scoped subject (subject_for_run), then
// returns a result when the run completes. Pulse subscribes to an
// operator-supplied pattern across the whole bus, and there's no
// natural "done" — it runs until the operator hits Ctrl+C. Folding
// these into one handler would either make CmdRun's result delivery
// awkward, or hide pulse's open-ended behaviour behind a fake
// timeout.
//
// **Backpressure.** Subscribe buffer is 4096 (4× the per-run buffer
// CmdRun uses). The bus drops events to slow subscribers — counted in
// Subscription.Dropped — rather than blocking publishers. If the
// client can't keep up, drops are silent; future iterations could
// emit a synthetic event noting "agezt: N events dropped" so the
// operator knows their view is incomplete.
func (s *Server) handlePulseSubscribe(ctx context.Context, conn net.Conn, req Request) {
	pattern := ">"
	if p, ok := req.Args["pattern"].(string); ok && strings.TrimSpace(p) != "" {
		pattern = strings.TrimSpace(p)
	}

	// Optional kinds filter. Wire shape: []string, decoded from JSON
	// as []any → coerce per-element. Empty / missing = no filter.
	var kindFilter map[event.Kind]struct{}
	if raw, ok := req.Args["kinds"].([]any); ok && len(raw) > 0 {
		kindFilter = make(map[event.Kind]struct{}, len(raw))
		for _, r := range raw {
			if k, ok := r.(string); ok && strings.TrimSpace(k) != "" {
				kindFilter[event.Kind(strings.TrimSpace(k))] = struct{}{}
			}
		}
	}

	// Optional `since` arg: replay every journaled event with
	// seq >= since that matches pattern+kinds *before* attaching
	// the live subscription. Lets operators reconstruct "what
	// just happened" without missing the next thing. -1 / missing
	// means "no replay; start live." (M1.aa — Pulse v2.)
	// JSON numbers decode as float64; coerce.
	since := int64(-1)
	if v, ok := req.Args["since"].(float64); ok {
		since = int64(v)
	}

	// Optional `since_ts_ms` arg (M1.gg — Pulse v3 partial): same
	// historical-replay semantics but cut by Unix-ms timestamp
	// rather than seq. Used by `agt pulse --last 5m` which is pure
	// client-side sugar that resolves "5 minutes ago" to a wall-
	// clock cutoff. `since` and `since_ts_ms` compose: if both are
	// set, an event must pass BOTH cutoffs to be replayed. The
	// common case is one or the other; AND semantics are the safer
	// default (no surprising inclusion).
	sinceTSMs := int64(-1)
	if v, ok := req.Args["since_ts_ms"].(float64); ok {
		sinceTSMs = int64(v)
	}

	// Optional `until` arg (M1.ii — Pulse v3 bounded-replay).
	// When set, terminates the call after the historical replay
	// finishes — never transitions to live. Useful for "extract
	// every event between A and B, pipe to support" without a
	// hanging stream. Pair with `since`/`since_ts_ms` for a
	// half-open window [since, until).
	//
	// `until` is a seq cutoff (exclusive); `until_ts_ms` is the
	// timestamp variant. Both can be set; the live loop is skipped
	// when EITHER is set, so a single bound triggers replay-only
	// mode.
	until := int64(-1)
	if v, ok := req.Args["until"].(float64); ok {
		until = int64(v)
	}
	untilTSMs := int64(-1)
	if v, ok := req.Args["until_ts_ms"].(float64); ok {
		untilTSMs = int64(v)
	}
	replayOnly := until >= 0 || untilTSMs >= 0

	// Optional `correlation` filter: only deliver events whose
	// CorrelationID matches exactly. Pairs the historical-walk
	// counterpart (`agt why <id>`) with a live-tail mode for
	// debugging an in-progress run. AND-composed with every
	// other filter — `--correlation X --kind tool.invoked`
	// means "tool invocations on X's chain, nothing else."
	correlationFilter := ""
	if v, ok := req.Args["correlation"].(string); ok {
		correlationFilter = strings.TrimSpace(v)
	}

	// Replay rate limit (M1.nn): cap events/second during the
	// historical replay so a multi-million-event window doesn't
	// saturate the operator's terminal or wedge a downstream
	// consumer's buffer. 0 (default) = unlimited; positive value
	// sleeps proportionally between writes. Live stream is
	// uncapped — back-pressure there is the bus's dropped-events
	// notice (M1.aa).
	replayRate := float64(0)
	if v, ok := req.Args["replay_rate"].(float64); ok && v > 0 {
		replayRate = v
	}

	// handleConn pins a 10-minute read deadline that's fine for every
	// short-lived command but would terminate a quiet pulse stream
	// prematurely. Clear it — the clientGone watcher below detects
	// disconnects without relying on the read timing out.
	_ = conn.SetReadDeadline(time.Time{})

	// Subscribe BEFORE replay so any event published mid-walk is
	// buffered (bounded 4096; bus drops to slow subscribers, which
	// pulse v2 will eventually surface as a synthetic notice).
	sub, err := s.k.Bus().Subscribe(pattern, 4096)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	defer sub.Cancel()

	// Historical replay. Walk the journal once, filter, write
	// matching events to the client. Errors during replay are
	// surfaced as an error response (the operator's audit walk
	// would otherwise silently complete missing data).
	//
	// `lastReplayed` lets the live loop below skip any event whose
	// seq was already delivered during replay — necessary because
	// the subscription was opened BEFORE the replay started (to
	// avoid losing events published mid-walk), so the sub channel
	// may contain duplicates of the highest-seq journaled events.
	var lastReplayed int64 = -1
	if since >= 0 || sinceTSMs >= 0 || replayOnly {
		var err error
		lastReplayed, err = s.replayHistorical(conn, req.ID, pattern, kindFilter, correlationFilter, since, sinceTSMs, until, untilTSMs, replayRate)
		if err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse replay: " + err.Error()})
			return
		}
	}
	if replayOnly {
		// Bounded-replay mode (M1.ii): the operator wants to extract
		// a half-open window of journal events, not a live tail.
		// Closing the conn (via writeResp returning a noop EOF on the
		// next call, or natural client read-loop exit) is the wire
		// signal; we just return here. The client's StreamUntilCancel
		// observes the connection close as the natural end of stream.
		return
	}

	// Watch the conn for client-initiated close. If we relied only on
	// "writeResp fails on broken pipe" we'd never notice a quiet
	// stream that has no events to write — the handler would hang
	// blocked on <-sub.C until the server itself shuts down. Spawn a
	// read goroutine that signals via clientGone when the conn drops.
	// Pulse never reads anything after the initial request, so any
	// Read returning err (typically EOF or "use of closed network
	// connection") means the client went away.
	clientGone := make(chan struct{})
	go func() {
		var buf [1]byte
		for {
			if _, err := conn.Read(buf[:]); err != nil {
				close(clientGone)
				return
			}
		}
	}()

	// Drop monitor: every tick, check the subscription's Dropped
	// counter and emit a synthetic ephemeral event whenever it
	// grew. Operators see a clear "you missed N events" notice
	// in the pulse stream rather than silently incomplete data.
	// (M1.aa — Pulse v2 dropped-events synthetic.)
	//
	// 1 second is fast enough that operators see drops near-
	// realtime, slow enough not to add measurable overhead to
	// the bus. We don't use bus.PublishStreaming because that
	// would fan the notice out to *every* subscriber; this
	// notice is per-pulse-stream.
	dropTicker := time.NewTicker(1 * time.Second)
	defer dropTicker.Stop()
	var lastDroppedCount uint64
	dropNoticeSubject := "agezt.pulse.dropped"

	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse: subscription closed"})
				return
			}
			// Skip durable events already delivered during replay.
			// Ephemeral events (Seq=0, no Hash) never overlap because
			// they aren't journaled, so always pass them through.
			if lastReplayed >= 0 && !ev.IsEphemeral() && ev.Seq <= lastReplayed {
				continue
			}
			if kindFilter != nil {
				if _, want := kindFilter[ev.Kind]; !want {
					continue
				}
			}
			if correlationFilter != "" && ev.CorrelationID != correlationFilter {
				continue
			}
			if err := writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: ev}); err != nil {
				return
			}
		case <-dropTicker.C:
			now := sub.Dropped.Load()
			if now > lastDroppedCount {
				delta := now - lastDroppedCount
				lastDroppedCount = now
				// Synthesize a notice event. Marked ephemeral via
				// empty Hash + Seq=0; carries the drop count in
				// Payload so JSON consumers can pick it up.
				payload, _ := json.Marshal(map[string]any{
					"dropped_since_last_notice": delta,
					"dropped_total":             now,
				})
				notice := &event.Event{
					Subject: dropNoticeSubject,
					Kind:    event.Kind("agezt.pulse.dropped"),
					Actor:   "agezt",
					Payload: payload,
					// Seq=0, Hash="" — IsEphemeral() returns true.
				}
				if err := writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: notice}); err != nil {
					return
				}
			}
		case <-clientGone:
			return
		case <-ctx.Done():
			return
		}
	}
}

// replayHistorical walks the journal once, writing each event that
// passes every active filter to the client. Returns the highest seq
// written (or -1 if none matched) so the live-stream loop can
// deduplicate.
//
// Filters (M1.aa shipped seq + pattern + kinds; M1.gg adds ts;
// M1.ii adds the upper bounds):
//   - pattern    : NATS-style wildcard match on subject
//   - kindFilter : if non-nil, ev.Kind must be in the set
//   - since      : if >= 0, ev.Seq must be >= since
//   - sinceTSMs  : if >= 0, ev.TSUnixMS must be >= sinceTSMs
//   - until      : if >= 0, ev.Seq must be <  until (exclusive)
//   - untilTSMs  : if >= 0, ev.TSUnixMS must be <  untilTSMs
//
// All filters compose as AND — every active cutoff must pass.
// This is the safer default for compositional CLI flags (an
// operator who sets both `--since` and `--last` gets the
// intersection, not the union).
//
// A write error (client disconnected mid-replay) terminates early
// and returns the last successfully-written seq + a wrapped error
// — the caller treats this as a fatal pulse exit, same as any
// other write failure.
func (s *Server) replayHistorical(conn net.Conn, reqID, pattern string, kindFilter map[event.Kind]struct{}, correlationFilter string, since, sinceTSMs, until, untilTSMs int64, replayRateEPS float64) (int64, error) {
	// Rate-limit step: when positive, sleep `1/rate` seconds between
	// each successfully-written event. We use a simple time-anchored
	// gate rather than a token bucket because the workload is
	// strictly sequential (one writer, one consumer) — no need for
	// bursty allowances.
	var minInterval time.Duration
	if replayRateEPS > 0 {
		minInterval = time.Duration(float64(time.Second) / replayRateEPS)
	}
	var lastWrite time.Time
	// -1 sentinel means "nothing replayed" — caller uses this to
	// skip the dedup check in the live loop. Otherwise a since
	// value past the journal head would set lastReplayed=since-1
	// and erroneously suppress all live events (their seqs are
	// much smaller).
	var lastWritten int64 = -1
	err := s.k.Journal().Range(func(ev *event.Event) error {
		if since >= 0 && ev.Seq < since {
			return nil
		}
		if sinceTSMs >= 0 && ev.TSUnixMS < sinceTSMs {
			return nil
		}
		if until >= 0 && ev.Seq >= until {
			return nil
		}
		if untilTSMs >= 0 && ev.TSUnixMS >= untilTSMs {
			return nil
		}
		if !bus.MatchSubject(pattern, ev.Subject) {
			return nil
		}
		if kindFilter != nil {
			if _, want := kindFilter[ev.Kind]; !want {
				return nil
			}
		}
		if correlationFilter != "" && ev.CorrelationID != correlationFilter {
			return nil
		}
		if minInterval > 0 && !lastWrite.IsZero() {
			elapsed := time.Since(lastWrite)
			if elapsed < minInterval {
				time.Sleep(minInterval - elapsed)
			}
		}
		if err := writeResp(conn, Response{ID: reqID, Type: RespEvent, Event: ev}); err != nil {
			return err
		}
		if minInterval > 0 {
			lastWrite = time.Now()
		}
		lastWritten = ev.Seq
		return nil
	})
	return lastWritten, err
}
