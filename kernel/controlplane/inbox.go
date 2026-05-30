// SPDX-License-Identifier: MIT

package controlplane

// Unified Inbox (SPEC-07 §4). Journal-backed: walks channel.inbound /
// channel.outbound events once and groups them into conversation threads by
// correlation_id, newest activity first. Every channel normalizes to
// UnifiedMessage (SPEC-04 §1.3), so this one view shows any channel's
// conversation regardless of origin platform. Read-only — the journal is the
// source of truth, no separate store.

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
)

const (
	defaultInboxLimit = 20
	maxInboxLimit     = 1_000
)

type inboxMessage struct {
	Direction string `json:"direction"` // "in" | "out"
	Sender    string `json:"sender,omitempty"`
	Text      string `json:"text"`
	TSUnixMS  int64  `json:"ts_unix_ms"`
	EventID   string `json:"event_id"`
}

type inboxThread struct {
	CorrelationID string         `json:"correlation_id"`
	ChannelKind   string         `json:"channel_kind"`
	ChannelID     string         `json:"channel_id"`
	Messages      []inboxMessage `json:"messages"`
	LastTSUnixMS  int64          `json:"last_ts_unix_ms"`
}

func (s *Server) handleInbox(conn net.Conn, req Request) {
	limit := defaultInboxLimit
	if raw, ok := req.Args["limit"]; ok {
		if v, ok := raw.(float64); ok {
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxInboxLimit {
		limit = maxInboxLimit
	}

	threads := map[string]*inboxThread{}
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
			th = &inboxThread{CorrelationID: e.CorrelationID, ChannelKind: p.ChannelKind, ChannelID: p.ChannelID}
			threads[key] = th
			order = append(order, key)
		}
		if th.ChannelID == "" {
			th.ChannelID = p.ChannelID
		}
		if th.ChannelKind == "" {
			th.ChannelKind = p.ChannelKind
		}
		th.Messages = append(th.Messages, inboxMessage{
			Direction: dir, Sender: p.Sender, Text: p.Text, TSUnixMS: e.TSUnixMS, EventID: e.ID,
		})
		if e.TSUnixMS > th.LastTSUnixMS {
			th.LastTSUnixMS = e.TSUnixMS
		}
	}

	err := s.k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindChannelInbound:
			add(e, "in")
		case event.KindChannelOutbound:
			add(e, "out")
		}
		return nil
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	all := make([]*inboxThread, 0, len(order))
	for _, k := range order {
		all = append(all, threads[k])
	}
	// Newest activity first; stable tie-break by correlation for determinism.
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].LastTSUnixMS != all[j].LastTSUnixMS {
			return all[i].LastTSUnixMS > all[j].LastTSUnixMS
		}
		return all[i].CorrelationID > all[j].CorrelationID
	})
	if len(all) > limit {
		all = all[:limit]
	}

	out := make([]any, 0, len(all))
	for _, th := range all {
		out = append(out, th)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"threads": out, "count": len(out)},
	})
}
