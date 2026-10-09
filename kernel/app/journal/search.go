// SPDX-License-Identifier: MIT

package journal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

const (
	defaultGrepLimit = 100
	maxGrepLimit     = 10_000
)

// MaxExportN bounds how many events one export bundles. An export is meant to
// be complete, so it sits far above the tail and grep caps; it is only a memory
// backstop, and a hit is reported as truncated rather than cut silently.
const MaxExportN = 200_000

type GrepRequest struct {
	Pattern       json.RawMessage `json:"pattern,omitempty"`
	Kind          json.RawMessage `json:"kind,omitempty"`
	Subject       json.RawMessage `json:"subject,omitempty"`
	Actor         json.RawMessage `json:"actor,omitempty"`
	CorrelationID json.RawMessage `json:"correlation_id,omitempty"`
	Limit         json.RawMessage `json:"limit,omitempty"`
}

type ExportRequest struct {
	SinceMS     json.RawMessage `json:"since_ms,omitempty"`
	Correlation json.RawMessage `json:"correlation,omitempty"`
}

type ExportOutput struct {
	Events      []*event.Event `json:"events"`
	Count       int            `json:"count"`
	FirstSeq    int64          `json:"first_seq"`
	LastSeq     int64          `json:"last_seq"`
	HeadSeq     int64          `json:"head_seq"`
	HeadHash    string         `json:"head_hash"`
	Truncated   bool           `json:"truncated"`
	Correlation string         `json:"correlation"`
}

type wireExport struct {
	Events      []wireEvent `json:"events"`
	Count       int         `json:"count"`
	FirstSeq    int64       `json:"first_seq"`
	LastSeq     int64       `json:"last_seq"`
	HeadSeq     int64       `json:"head_seq"`
	HeadHash    string      `json:"head_hash"`
	Truncated   bool        `json:"truncated"`
	Correlation string      `json:"correlation"`
}

func rawValue(raw json.RawMessage) any {
	var v any
	if len(raw) != 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

// optionalString is a strict string: absent is empty, present must be a string.
func optionalString(raw json.RawMessage, key string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	s, ok := rawValue(raw).(string)
	if !ok {
		return "", fmt.Errorf("args.%s must be a string", key)
	}
	return s, nil
}

// errStopWalk short-circuits Range without being mistaken for an I/O error.
var errStopWalk = errors.New("stop walk")

// matchesPattern reports whether the lowercased pattern appears in the kind,
// subject, actor, correlation or raw payload of the event.
func matchesPattern(e *event.Event, pattern string) bool {
	for _, field := range []string{string(e.Kind), e.Subject, e.Actor, e.CorrelationID} {
		if strings.Contains(strings.ToLower(field), pattern) {
			return true
		}
	}
	return len(e.Payload) > 0 && strings.Contains(strings.ToLower(string(e.Payload)), pattern)
}

// Grep walks the journal from the oldest event and keeps those matching every
// given filter: exact kind, subject, actor and correlation, plus a
// case-insensitive pattern. The walk stops once limit matches accumulate.
func (s *Service) Grep(_ context.Context, in GrepRequest) (EventsOutput, error) {
	var f [5]string
	for i, field := range []struct {
		raw json.RawMessage
		key string
	}{{in.Pattern, "pattern"}, {in.Kind, "kind"}, {in.Subject, "subject"}, {in.Actor, "actor"}, {in.CorrelationID, "correlation_id"}} {
		var err error
		if f[i], err = optionalString(field.raw, field.key); err != nil {
			return EventsOutput{}, err
		}
	}
	pattern, kind, subject, actor, corr := strings.ToLower(f[0]), f[1], f[2], f[3], f[4]
	limit := defaultGrepLimit
	if v, ok := rawValue(in.Limit).(float64); ok {
		limit = int(v)
	}
	limit = min(max(limit, 1), maxGrepLimit)
	head, _ := s.head()
	matches := make([]*event.Event, 0, limit)
	err := s.journal.Range(func(e *event.Event) error {
		if kind != "" && string(e.Kind) != kind ||
			subject != "" && e.Subject != subject ||
			actor != "" && e.Actor != actor ||
			corr != "" && e.CorrelationID != corr ||
			pattern != "" && !matchesPattern(e, pattern) {
			return nil
		}
		matches = append(matches, e)
		if len(matches) >= limit {
			return errStopWalk
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return EventsOutput{}, err
	}
	return EventsOutput{Events: matches, Count: len(matches), Head: head}, nil
}

// Export bundles every event, optionally since now-since_ms and scoped to one
// correlation, with hashes intact and the chain head at export time, so the
// bundle can be verified offline. A correlation scope is deliberately
// non-contiguous.
func (s *Service) Export(_ context.Context, in ExportRequest) (ExportOutput, error) {
	var cutoff int64
	if v, ok := rawValue(in.SinceMS).(float64); ok && int64(v) > 0 {
		cutoff = s.now().UnixMilli() - int64(v)
	}
	corr, err := optionalString(in.Correlation, "correlation")
	if err != nil {
		return ExportOutput{}, err
	}
	out := ExportOutput{Events: make([]*event.Event, 0, 256), FirstSeq: -1, LastSeq: -1, Correlation: corr}
	out.HeadSeq, out.HeadHash = s.head()
	err = s.journal.Range(func(e *event.Event) error {
		if cutoff > 0 && e.TSUnixMS < cutoff || corr != "" && e.CorrelationID != corr {
			return nil
		}
		if len(out.Events) >= s.exportCap {
			out.Truncated = true
			return errStopWalk
		}
		if out.FirstSeq < 0 {
			out.FirstSeq = e.Seq
		}
		out.LastSeq = e.Seq
		out.Events = append(out.Events, e)
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return ExportOutput{}, err
	}
	out.Count = len(out.Events)
	return out, nil
}
