// SPDX-License-Identifier: MIT
package pulse

import (
	"context"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"time"
)

type Journal interface {
	Range(func(*event.Event) error) error
}
type Replay struct{ journal Journal }

func NewReplay(journal Journal) *Replay { return &Replay{journal: journal} }

type ReplayInput struct {
	Pattern                            string
	Kinds                              map[event.Kind]struct{}
	Correlation                        string
	Since, SinceTSMS, Until, UntilTSMS int64
	RateEPS                            float64
}
type ReplayOutput struct{ LastWritten int64 }

// Replay retains journal order and AND-composed half-open cutoffs. LastWritten
// is the last successfully emitted seq, with -1 when no matching event was sent.
func (s *Replay) Replay(ctx context.Context, in ReplayInput, emit func(*event.Event) error) (ReplayOutput, error) {
	var minInterval time.Duration
	if in.RateEPS > 0 {
		minInterval = time.Duration(float64(time.Second) / in.RateEPS)
	}
	var lastWrite time.Time
	var lastWritten int64 = -1
	err := s.journal.Range(func(ev *event.Event) error {
		if in.Since >= 0 && ev.Seq < in.Since {
			return nil
		}
		if in.SinceTSMS >= 0 && ev.TSUnixMS < in.SinceTSMS {
			return nil
		}
		if in.Until >= 0 && ev.Seq >= in.Until {
			return nil
		}
		if in.UntilTSMS >= 0 && ev.TSUnixMS >= in.UntilTSMS {
			return nil
		}
		if !bus.MatchSubject(in.Pattern, ev.Subject) {
			return nil
		}
		if in.Kinds != nil {
			if _, want := in.Kinds[ev.Kind]; !want {
				return nil
			}
		}
		if in.Correlation != "" && ev.CorrelationID != in.Correlation {
			return nil
		}
		if minInterval > 0 && !lastWrite.IsZero() {
			elapsed := time.Since(lastWrite)
			if elapsed < minInterval {
				wait := time.NewTimer(minInterval - elapsed)
				select {
				case <-ctx.Done():
					wait.Stop()
					return ctx.Err()
				case <-wait.C:
				}
			}
		}
		if err := emit(ev); err != nil {
			return err
		}
		if minInterval > 0 {
			lastWrite = time.Now()
		}
		lastWritten = ev.Seq
		return nil
	})
	return ReplayOutput{LastWritten: lastWritten}, err
}
