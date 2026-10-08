// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/roster"
)

// EscalationBoard is the read view of the shared board the escalation fold
// needs: the help topic and one message's replies, both under the host's cap.
type EscalationBoard interface {
	Help() []board.Message
	Replies(id string) []board.Message
}

type EscalationRequest = RefPageRequest

// escalationCursor is strict: a present non-string cursor is an error, and a
// string decodes as "<ts_unix_ms>:<message_id>" or is ignored.
func escalationCursor(raw json.RawMessage) (int64, string, error) {
	v, present := rawValue(raw)
	cursor, ok := v.(string)
	if present && !ok {
		return 0, "", fmt.Errorf("args.cursor must be a string")
	}
	if cursor == "" {
		return 0, "", nil
	}
	tsStr, id, _ := strings.Cut(cursor, ":")
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return 0, "", nil
	}
	return ts, id, nil
}

type EscalationOutputRow struct {
	MessageID         string `json:"message_id"`
	From              string `json:"from"`
	To                string `json:"to"`
	Text              string `json:"text"`
	TSUnixMS          int64  `json:"ts_unix_ms"`
	Status            string `json:"status"`
	ReplyCount        int    `json:"reply_count"`
	Acked             bool   `json:"acked"`
	SourceAgent       string `json:"source_agent"`
	Mode              string `json:"mode"`
	WakePhase         string `json:"wake_phase"`
	WakeReason        string `json:"wake_reason"`
	WakeError         string `json:"wake_error"`
	WakeCorrelationID string `json:"wake_correlation_id"`
	Fingerprint       string `json:"fingerprint"`
	Resolution        string `json:"resolution"`
	ResolutionSummary string `json:"resolution_summary"`
	DelegateTo        string `json:"delegate_to"`
	OriginKind        string `json:"origin_kind"`
	OriginAgent       string `json:"origin_agent"`
	RootAgent         string `json:"root_agent"`
	ChainDepth        int    `json:"chain_depth"`
	IncidentID        string `json:"incident_id"`
	RootIncidentID    string `json:"root_incident_id"`
	ParentIncidentID  string `json:"parent_incident_id"`
}

type EscalationsOutput struct {
	Slug        string                `json:"slug"`
	Escalations []EscalationOutputRow `json:"escalations"`
	Count       int                   `json:"count"`
	OpenCount   int                   `json:"open_count"`
	NextCursor  string                `json:"next_cursor,omitempty"`
}

// EscalationService reports the help requests addressed to one agent (or to
// everyone) with their doctor/delegation wake metadata from the journal.
type EscalationService struct {
	get     func(string) (core.Profile, bool)
	board   func() (EscalationBoard, error)
	journal func(func(*event.Event) error) error
}

func NewEscalations(get func(string) (core.Profile, bool), board func() (EscalationBoard, error), journal func(func(*event.Event) error) error) *EscalationService {
	return &EscalationService{get: get, board: board, journal: journal}
}

func (s *EscalationService) Escalations(_ context.Context, in EscalationRequest) (EscalationsOutput, error) {
	ref, err := in.ref()
	if err != nil {
		return EscalationsOutput{}, err
	}
	p, ok := s.get(ref)
	if !ok {
		return EscalationsOutput{}, errors.New("unknown agent: " + ref)
	}
	limit, err := in.limit(20, 100)
	if err != nil {
		return EscalationsOutput{}, err
	}
	st, err := s.board()
	if err != nil {
		return EscalationsOutput{}, err
	}
	cursorTS, cursorID, err := escalationCursor(in.Cursor)
	if err != nil {
		return EscalationsOutput{}, err
	}
	rows, next := s.rows(st, p.Slug, limit, cursorTS, cursorID)
	out := EscalationsOutput{Slug: p.Slug, Escalations: rows, Count: len(rows), NextCursor: next}
	for _, row := range rows {
		if row.Status == "open" {
			out.OpenCount++
		}
	}
	return out, nil
}

func (s *EscalationService) rows(st EscalationBoard, slug string, limit int, cursorTS int64, cursorID string) ([]EscalationOutputRow, string) {
	msgs := st.Help()
	metaByMessage := map[string]RepairRow{}
	_ = s.journal(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if plString(pl, "target_agent") != slug {
			return nil
		}
		msgID := plString(pl, "mailbox_message_id")
		if strings.TrimSpace(msgID) == "" {
			return nil
		}
		row := RepairRow{
			Seq:               e.Seq,
			Agent:             plString(pl, "agent"),
			Mode:              plString(pl, "mode"),
			Phase:             plString(pl, "phase"),
			Reason:            plString(pl, "reason"),
			Error:             plString(pl, "error"),
			Fingerprint:       plString(pl, "fingerprint"),
			TargetCorr:        plString(pl, "target_correlation"),
			Resolution:        plString(pl, "resolution"),
			ResolutionSummary: plString(pl, "resolution_summary"),
			DelegateTo:        plString(pl, "delegate_to"),
			DelegatedBy:       plString(pl, "delegated_by"),
			RootAgent:         plString(pl, "root_agent"),
			ChainDepth:        intNumber(pl["chain_depth"]),
			IncidentID:        plString(pl, "incident_id"),
			RootIncidentID:    plString(pl, "root_incident_id"),
			ParentIncidentID:  plString(pl, "parent_incident_id"),
		}
		if cur, ok := metaByMessage[msgID]; !ok || row.Seq > cur.Seq {
			metaByMessage[msgID] = row
		}
		return nil
	})
	out := make([]EscalationOutputRow, 0, len(msgs))
	for _, msg := range msgs {
		if !msg.Help {
			continue
		}
		if msg.To != slug && msg.To != board.Everyone {
			continue
		}
		replies := st.Replies(msg.ID)
		acked := BoardMessageAckedBy(msg, slug)
		status := "open"
		if len(replies) > 0 {
			status = "answered"
		} else if acked {
			status = "acked"
		}
		row := EscalationOutputRow{MessageID: msg.ID, From: msg.From, To: msg.To, Text: msg.Text, TSUnixMS: msg.TSMS, Status: status, ReplyCount: len(replies), Acked: acked}
		if meta, ok := metaByMessage[msg.ID]; ok {
			row.SourceAgent = meta.Agent
			row.Mode = meta.Mode
			row.WakePhase = meta.Phase
			row.WakeReason = meta.Reason
			row.WakeError = meta.Error
			row.WakeCorrelationID = meta.TargetCorr
			row.Fingerprint = meta.Fingerprint
			row.Resolution = meta.Resolution
			row.ResolutionSummary = meta.ResolutionSummary
			row.DelegateTo = meta.DelegateTo
			row.RootAgent = meta.RootAgent
			row.ChainDepth = meta.ChainDepth
			row.IncidentID = meta.IncidentID
			row.RootIncidentID = meta.RootIncidentID
			row.ParentIncidentID = meta.ParentIncidentID
			if strings.HasPrefix(meta.Phase, "delegation_") {
				row.OriginKind = "delegated"
				row.OriginAgent = firstNonEmpty(meta.DelegatedBy, msg.From)
			} else {
				row.OriginKind = "doctor"
				row.OriginAgent = firstNonEmpty(msg.From, meta.DelegatedBy)
			}
		}
		// Parse the broken/source agent out of the message text only when the
		// event envelope didn't carry one (older history).
		if row.SourceAgent == "" {
			row.SourceAgent = escalationSourceFromText(msg.Text)
		}
		if row.RootAgent == "" {
			row.RootAgent = row.SourceAgent
		}
		if row.OriginKind == "" {
			row.OriginKind = "doctor"
			row.OriginAgent = msg.From
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSUnixMS > out[j].TSUnixMS })
	// The cursor is (TSUnixMS, MessageID) of the previous page's last entry;
	// skip entries strictly newer-or-equal.
	if cursorTS > 0 || cursorID != "" {
		filtered := out[:0]
		for _, r := range out {
			if r.TSUnixMS > cursorTS {
				continue
			}
			if r.TSUnixMS == cursorTS && r.MessageID >= cursorID {
				continue
			}
			filtered = append(filtered, r)
		}
		out = filtered
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		return out, strconv.FormatInt(last.TSUnixMS, 10) + ":" + last.MessageID
	}
	return out, ""
}

// BoardMessageAckedBy reports whether slug explicitly acknowledged m.
func BoardMessageAckedBy(m board.Message, slug string) bool {
	slug = strings.ToLower(strings.TrimSpace(slug))
	for _, by := range m.AckedBy {
		if strings.ToLower(strings.TrimSpace(by)) == slug {
			return true
		}
	}
	return false
}

func escalationSourceFromText(text string) string {
	text = strings.TrimSpace(text)
	const prefix = "Doctor "
	if !strings.HasPrefix(text, prefix) {
		return ""
	}
	if i := strings.Index(text, " for agent "); i > 0 {
		// The agent named after "for agent" is the broken agent, not the owner.
		start := i + len(" for agent ")
		if end := strings.Index(text[start:], "."); end > 0 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	return ""
}

func EscalationOperations(provider func(context.Context) *EscalationService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster escalations provider required")
	}
	op, err := app.NewOperation(opapi.Spec{Name: "agent_escalations", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"limit":{},"cursor":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/agents/escalations"}}, func(ctx context.Context, in EscalationRequest) (EscalationsOutput, error) {
		return provider(ctx).Escalations(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
