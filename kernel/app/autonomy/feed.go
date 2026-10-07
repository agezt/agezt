// SPDX-License-Identifier: MIT
package autonomy

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/event"
	"strings"
)

// Journal selects the durable recent history without exposing runtime or socket.
type Journal interface {
	Tail(int) ([]*event.Event, error)
}
type Feed struct{ journal Journal }

func NewFeed(journal Journal) *Feed { return &Feed{journal: journal} }

type FeedInput struct{ Limit int }

// FeedOutput retains required native items/count with typed row metadata.
type FeedOutput struct {
	Items []FeedItem `json:"items"`
	Count int        `json:"count"`
}

func (s *Feed) List(_ context.Context, in FeedInput) (FeedOutput, error) {
	limit := in.Limit
	if limit < 1 {
		limit = 1
	}
	if limit > FeedMaxLimit {
		limit = FeedMaxLimit
	}

	tail, err := s.journal.Tail(FeedScanN)
	if err != nil {
		return FeedOutput{}, err
	}

	// Walk newest-first, keeping only self-directed kinds up to the limit.
	items := make([]FeedItem, 0, limit)
	for i := len(tail) - 1; i >= 0 && len(items) < limit; i-- {
		e := tail[i]
		category, title, ok := autonomyMeta(e)
		if !ok {
			continue
		}
		item := FeedItem{Seq: e.Seq, TSUnixMS: e.TSUnixMS, Kind: string(e.Kind), Subject: e.Subject, Category: category, Title: title, CorrelationID: e.CorrelationID}
		if category == "doctor" {
			var p map[string]any
			if json.Unmarshal(e.Payload, &p) == nil {
				item.Agent = strPayload(p, "agent")
				item.TargetAgent = strPayload(p, "target_agent")
				item.DelegateTo = strPayload(p, "delegate_to")
				item.DelegatedBy = strPayload(p, "delegated_by")
				item.RootAgent = strPayload(p, "root_agent")
				item.IncidentID = strPayload(p, "incident_id")
				item.RootIncidentID = strPayload(p, "root_incident_id")
				item.ParentIncidentID = strPayload(p, "parent_incident_id")
				item.Phase = strPayload(p, "phase")
				item.Mode = strPayload(p, "mode")
				item.Resolution = strPayload(p, "resolution")
				item.RoutingTaskType = strPayload(p, "routing_task_type")
				item.RoutingTaskModelChain = strSlicePayload(p, "routing_task_model_chain")
				if n, ok := p["routing_force_generation"].(float64); ok {
					value := int(n)
					item.RoutingForceGeneration = &value
				}
				if n, ok := p["chain_depth"].(float64); ok {
					value := int(n)
					item.ChainDepth = &value
				}
			}
		}
		item.Detail = autonomyDetail(e)
		items = append(items, item)
	}

	return FeedOutput{Items: items, Count: len(items)}, nil
}

// autonomyDetail pulls a short, human detail string from an event's payload so a
// feed row says something concrete ("intent: digest the inbox", "skill:
// diagnose-ci", "complete: true"). Best-effort — a payload that doesn't carry the
// expected field just yields no detail.
func autonomyDetail(e *event.Event) string {
	if len(e.Payload) == 0 {
		return ""
	}
	var p map[string]any
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return ""
	}
	str := func(k string) string {
		return strPayload(p, k)
	}
	if e.Kind == event.KindInfo {
		if d := autonomyDoctorDetail(e.Subject, p); d != "" {
			return d
		}
	}
	switch e.Kind {
	case event.KindScheduleFired, event.KindStandingFired:
		if v := str("intent"); v != "" {
			return clipDetail(v)
		}
		return str("name")
	case event.KindSubAgentSpawned:
		var parts []string
		if agent := str("agent"); agent != "" {
			parts = append(parts, agent)
		}
		if by := str("delegated_by"); by != "" {
			parts = append(parts, "by "+by)
		}
		if task := clipDetail(str("task")); task != "" {
			parts = append(parts, task)
		}
		return strings.Join(parts, " · ")
	case event.KindStandingCreated, event.KindStandingError:
		return str("name")
	case event.KindSkillCreated, event.KindSkillPromoted, event.KindSkillQuarantined, event.KindSkillReverted:
		if v := str("name"); v != "" {
			return v
		}
		return str("id")
	case event.KindAssureVerdict:
		if c, ok := p["complete"].(bool); ok {
			if c {
				return "complete: true"
			}
			if g := str("gap"); g != "" {
				return "gap: " + clipDetail(g)
			}
			return "complete: false"
		}
	case event.KindBriefingSent:
		return str("subject")
	case event.KindBoardPosted:
		if topic := str("topic"); topic != "" {
			if from := str("from"); from != "" {
				return topic + " · from " + from
			}
			return topic
		}
	}
	return ""
}
