// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/event"
	"sort"
	"strconv"
	"strings"
)

type InboxJournal interface {
	Range(func(*event.Event) error) error
}
type Inbox struct{ journal InboxJournal }

func NewInbox(journal InboxJournal) *Inbox { return &Inbox{journal: journal} }

// CursorError preserves the native codec's delayed validation/error precedence.
type InboxInput struct {
	Limit           *int
	Channel, Cursor string
	CursorError     error
}
type InboxOutput = map[string]any

const (
	DefaultInboxLimit = 20
	MaxInboxLimit     = 1_000
)

type InboxMessage struct {
	Direction string `json:"direction"` // "in" | "out"
	Sender    string `json:"sender,omitempty"`
	Text      string `json:"text"`
	TSUnixMS  int64  `json:"ts_unix_ms"`
	EventID   string `json:"event_id"`
}

type InboxThread struct {
	CorrelationID string         `json:"correlation_id"`
	ChannelKind   string         `json:"channel_kind"`
	ChannelID     string         `json:"channel_id"`
	Messages      []InboxMessage `json:"messages"`
	LastTSUnixMS  int64          `json:"last_ts_unix_ms"`
}

func (s *Inbox) List(_ context.Context, in InboxInput) (InboxOutput, error) {
	limit := DefaultInboxLimit
	if in.Limit != nil {
		limit = *in.Limit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > MaxInboxLimit {
		limit = MaxInboxLimit
	}

	// Optional channel-kind filter (telegram | slack | discord | …). With several
	// channels live, an operator scoping to one platform shouldn't wade through the
	// rest. Normalized lowercase; empty means "all channels".
	channelFilter := strings.ToLower(strings.TrimSpace(in.Channel))

	threads := map[string]*InboxThread{}
	var order []string

	add := func(e *event.Event, dir string) {
		var p struct {
			ChannelKind string `json:"channel_kind"`
			ChannelID   string `json:"channel_id"`
			Sender      string `json:"sender"`
			Text        string `json:"text"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		// Group by correlation; events with no correlation each form their
		// own single-message thread keyed by event id so they aren't merged.
		key := e.CorrelationID
		if key == "" {
			key = e.ID
		}
		th, ok := threads[key]
		if !ok {
			th = &InboxThread{CorrelationID: e.CorrelationID, ChannelKind: p.ChannelKind, ChannelID: p.ChannelID}
			threads[key] = th
			order = append(order, key)
		}
		if th.ChannelID == "" {
			th.ChannelID = p.ChannelID
		}
		if th.ChannelKind == "" {
			th.ChannelKind = p.ChannelKind
		}
		th.Messages = append(th.Messages, InboxMessage{
			Direction: dir, Sender: p.Sender, Text: p.Text, TSUnixMS: e.TSUnixMS, EventID: e.ID,
		})
		if e.TSUnixMS > th.LastTSUnixMS {
			th.LastTSUnixMS = e.TSUnixMS
		}
	}

	err := s.journal.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindChannelInbound:
			add(e, "in")
		case event.KindChannelOutbound:
			add(e, "out")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	all := make([]*InboxThread, 0, len(order))
	for _, k := range order {
		th := threads[k]
		if channelFilter != "" && strings.ToLower(th.ChannelKind) != channelFilter {
			continue
		}
		all = append(all, th)
	}
	// Newest activity first; stable tie-break by correlation for determinism.
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].LastTSUnixMS != all[j].LastTSUnixMS {
			return all[i].LastTSUnixMS > all[j].LastTSUnixMS
		}
		return all[i].CorrelationID > all[j].CorrelationID
	})
	total := len(all)

	// Cursor pagination (M-pending follow-up): the SPA's Inbox view polls
	// this on every render, and active agents can generate thousands of
	// channel events. `cursor` encodes the (LastTSUnixMS, CorrelationID) of
	// the LAST entry on the previous page; server skips entries strictly
	// newer-or-equal. CorrelationID tie-breaks when LastTSUnixMS collides.
	var cursorTS int64
	var cursorCorr string
	cursorOK := false
	if raw, cerr := in.Cursor, in.CursorError; cerr != nil {
		return nil, cerr
	} else if raw != "" {
		tsStr, corr, _ := strings.Cut(raw, ":")
		if ts, perr := strconv.ParseInt(tsStr, 10, 64); perr == nil {
			cursorTS, cursorCorr, cursorOK = ts, corr, true
		}
	}
	if cursorOK {
		filtered := all[:0]
		for _, th := range all {
			if th.LastTSUnixMS > cursorTS {
				continue
			}
			if th.LastTSUnixMS == cursorTS && th.CorrelationID >= cursorCorr {
				continue
			}
			filtered = append(filtered, th)
		}
		all = filtered
	}
	var nextCursor string
	if limit > 0 && len(all) > limit {
		all = all[:limit]
		last := all[limit-1]
		nextCursor = strconv.FormatInt(last.LastTSUnixMS, 10) + ":" + last.CorrelationID
	}

	out := make([]any, 0, len(all))
	for _, th := range all {
		out = append(out, th)
	}
	result := map[string]any{"threads": out, "count": len(out), "total": total}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	if channelFilter != "" {
		result["channel"] = channelFilter
	}
	return result, nil
}
