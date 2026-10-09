// SPDX-License-Identifier: MIT

package journal

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

const (
	defaultTailN = 20
	maxTailN     = 10_000
)

// Journal is the routed kernel's hash-chained event log.
type Journal interface {
	Head() (seq int64, hash string)
	Tail(n int) ([]*event.Event, error)
	Range(func(*event.Event) error) error
}

// Disk measures the journal's on-disk footprint, best-effort.
type Disk func() (segments int, bytes int64)

// ReadInput carries no arguments; unknown ones, such as the routing tenant,
// are accepted and ignored.
type ReadInput struct{}

type TailRequest struct {
	N json.RawMessage `json:"n,omitempty"`
}

type HeadOutput struct {
	Head int64  `json:"head"`
	Hash string `json:"hash"`
}

// wireEvent mirrors the event.Event JSON shape for the output schema only: the
// raw payload is any JSON value. Tail outputs carry the journal's own events.
type wireEvent struct {
	ID            string            `json:"id"`
	Seq           int64             `json:"seq"`
	TSUnixMS      int64             `json:"ts_unix_ms"`
	PrevHash      string            `json:"prev_hash"`
	Hash          string            `json:"hash,omitempty"`
	Subject       string            `json:"subject"`
	Actor         string            `json:"actor"`
	Kind          string            `json:"kind"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	CausationID   string            `json:"causation_id,omitempty"`
	Payload       any               `json:"payload,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
}

type wireTail struct {
	Events []wireEvent `json:"events"`
	Count  int         `json:"count"`
	Head   int64       `json:"head"`
}

type TailOutput struct {
	Events []*event.Event `json:"events"`
	Count  int            `json:"count"`
	Head   int64          `json:"head"`
}

type StatsOutput struct {
	Events       int64            `json:"events"`
	Segments     int              `json:"segments"`
	Bytes        int64            `json:"bytes"`
	ByKind       map[string]int64 `json:"by_kind"`
	OldestUnixMS int64            `json:"oldest_unix_ms"`
	NewestUnixMS int64            `json:"newest_unix_ms"`
}

// Service reads one kernel's journal.
type Service struct {
	journal Journal
	disk    Disk
}

func New(journal Journal, disk Disk) *Service { return &Service{journal: journal, disk: disk} }

// head clamps the empty journal's -1 to 0.
func (s *Service) head() (int64, string) {
	seq, hash := s.journal.Head()
	return max(seq, 0), hash
}

func (s *Service) Head(context.Context, ReadInput) (HeadOutput, error) {
	seq, hash := s.head()
	return HeadOutput{Head: seq, Hash: hash}, nil
}

// Tail returns the last n events (a JSON number truncated toward zero, else
// 20, clamped to 1..10,000) read from the newest segments backwards, with the
// head checkpoint taken first.
func (s *Service) Tail(_ context.Context, in TailRequest) (TailOutput, error) {
	n := defaultTailN
	if len(in.N) != 0 {
		var v any
		_ = json.Unmarshal(in.N, &v)
		if f, ok := v.(float64); ok {
			n = int(f)
		}
	}
	n = min(max(n, 1), maxTailN)
	head, _ := s.head()
	events, err := s.journal.Tail(n)
	if err != nil {
		return TailOutput{}, err
	}
	if events == nil {
		events = []*event.Event{}
	}
	return TailOutput{Events: events, Count: len(events), Head: head}, nil
}

// Stats folds the journal once into an event count, a per-kind breakdown and
// the stamped time span, then measures the files on disk.
func (s *Service) Stats(context.Context, ReadInput) (StatsOutput, error) {
	out := StatsOutput{ByKind: map[string]int64{}}
	if err := s.journal.Range(func(e *event.Event) error {
		out.Events++
		out.ByKind[string(e.Kind)]++
		if e.TSUnixMS > 0 {
			if out.OldestUnixMS == 0 || e.TSUnixMS < out.OldestUnixMS {
				out.OldestUnixMS = e.TSUnixMS
			}
			out.NewestUnixMS = max(out.NewestUnixMS, e.TSUnixMS)
		}
		return nil
	}); err != nil {
		return StatsOutput{}, err
	}
	out.Segments, out.Bytes = s.disk()
	return out, nil
}

func bind[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	shape := reflect.TypeFor[O]()
	if shape == reflect.TypeFor[TailOutput]() {
		shape = reflect.TypeFor[wireTail]()
	}
	output, err := schema.FromType(shape, false)
	if err != nil {
		return err
	}
	spec.OutputSchema, spec.ReadOnly, spec.Authz, spec.AllowUnknownInput = output, true, opapi.PrimaryOnly, true
	op, err := app.NewOperation(spec, handler)
	if err != nil {
		return err
	}
	*ops = append(*ops, op)
	return nil
}

// Operations declares the three unaudited primary journal reads. Head and tail
// read the primary journal; stats follows an operator-named tenant.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("journal provider required")
	}
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: "journal_head", Tenancy: opapi.Primary}, func(ctx context.Context, in ReadInput) (HeadOutput, error) {
				return provider(ctx).Head(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "journal_tail", Tenancy: opapi.Primary, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"n":{}}}`)}, func(ctx context.Context, in TailRequest) (TailOutput, error) {
				return provider(ctx).Tail(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "journal_stats", Tenancy: opapi.CallerTenant}, func(ctx context.Context, in ReadInput) (StatsOutput, error) {
				return provider(ctx).Stats(ctx, in)
			})
		},
	} {
		if err := b(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
