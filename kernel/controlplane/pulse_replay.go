// SPDX-License-Identifier: MIT

// Control-plane pulse: replayHistorical (connect-time historical replay).
// Code extracted from pulse.go during the Day-141 god-file split.
// Public API unchanged.
package controlplane


import (
	"context"
	"net"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) replayHistorical(ctx context.Context, conn net.Conn, reqID, pattern string, kindFilter map[event.Kind]struct{}, correlationFilter string, since, sinceTSMs, until, untilTSMs int64, replayRateEPS float64) (int64, error) {
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
				// Ctx-aware sleep: a `time.Sleep` here would block
				// the replay until minInterval elapses even if the
				// request context was cancelled (client disconnect /
				// daemon shutdown). For a low replayRateEPS that's
				// a multi-second stop-the-world delay (BUG:
				// ctx-unaware-sleep).
				wait := time.NewTimer(minInterval - elapsed)
				select {
				case <-ctx.Done():
					wait.Stop()
					return ctx.Err()
				case <-wait.C:
				}
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
