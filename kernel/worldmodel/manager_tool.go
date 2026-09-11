// SPDX-License-Identifier: MIT

// World model tool + correlation: WithCorrelation + CorrelationFrom + Tool/Definition/Invoke + renderResolve + renderNeighbors.
// Code extracted from manager.go during the Day-65 god-file split. Public API unchanged.
package worldmodel


import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"strings"
)


func WithCorrelation(ctx context.Context, corr string) context.Context {
	return context.WithValue(ctx, ctxKeyCorrelation, corr)
}

// CorrelationFrom extracts the correlation id set by WithCorrelation.
func CorrelationFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}

// --- agent tool -----------------------------------------------------------

const toolInputSchema = `{
  "type": "object",
  "properties": {
    "action":  {"type": "string", "enum": ["add", "relate", "resolve", "neighbors"], "description": "what to do"},
    "kind":    {"type": "string", "description": "entity kind for add (project|repo|person|org|account|device|channel|topic|task; default topic)"},
    "name":    {"type": "string", "description": "entity name (add)"},
    "aliases": {"type": "array", "items": {"type": "string"}, "description": "alternative phrases that resolve to this entity (add)"},
    "from":    {"type": "string", "description": "source entity name (relate)"},
    "verb":    {"type": "string", "description": "relation verb (relate; owns|depends_on|member_of|prefers|relates_to|assigned_to|derived_from)"},
    "to":      {"type": "string", "description": "target entity name (relate)"},
    "query":   {"type": "string", "description": "phrase to resolve to entities (resolve), or an entity name (neighbors)"},
    "limit":   {"type": "integer", "description": "max results (resolve; default 5)"}
  },
  "required": ["action"]
}`

type toolInput struct {
	Action  string            `json:"action"`
	Kind    Kind              `json:"kind"`
	Name    string            `json:"name"`
	Aliases []string          `json:"aliases"`
	Attrs   map[string]string `json:"attrs"`
	From    string            `json:"from"`
	Verb    Verb              `json:"verb"`
	To      string            `json:"to"`
	Query   string            `json:"query"`
	Limit   int               `json:"limit"`
}

type worldTool struct{ g *Graph }

// Tool returns the agent-facing world-model tool. Register it under the name
// "world" in the agent loop's tool map.
func (g *Graph) Tool() agent.Tool { return worldTool{g: g} }

func (t worldTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:       "world",
		Capability: agent.ToolCapability{Name: string(edict.CapWorld)},
		Description: "Read and grow the world model — the graph of the operator's projects, repos, " +
			"people and topics and how they relate. action=add records an entity (kind, name, aliases); " +
			"action=relate links two entities (from, verb, to); action=resolve looks up what a phrase " +
			"refers to (query); action=neighbors lists what an entity connects to (query=name).",
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"read world-model entities and relationships for resolve/neighbors",
				"upsert entities or relationships for add/relate",
			},
			AffectedResources: []string{"world-model graph"},
			RollbackNotes:     "Read operations need no rollback. Added entities/relations are journaled and can be superseded or removed by a later world-model correction.",
			Confidence:        0.85,
		},
		InputSchema: json.RawMessage(toolInputSchema),
	}
}

func (t worldTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid world input: " + err.Error(), IsError: true}, nil
	}
	corr := CorrelationFrom(ctx)
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "add":
		e, created, err := t.g.Upsert(corr, UpsertSpec{Kind: in.Kind, Name: in.Name, Aliases: in.Aliases, Attrs: in.Attrs})
		if err != nil {
			return agent.Result{Output: "add failed: " + err.Error(), IsError: true}, nil
		}
		verb := "reinforced"
		if created {
			verb = "added"
		}
		return agent.Result{Output: fmt.Sprintf("%s entity %s (%s: %s)", verb, e.ID[:12], e.Kind, e.Name)}, nil
	case "relate":
		r, err := t.g.Relate(corr, in.From, in.Verb, in.To)
		if err != nil {
			return agent.Result{Output: "relate failed: " + err.Error(), IsError: true}, nil
		}
		return agent.Result{Output: fmt.Sprintf("related %s %s %s", in.From, r.Verb, in.To)}, nil
	case "resolve":
		limit := in.Limit
		if limit <= 0 {
			limit = 5
		}
		hits, err := t.g.Resolve(corr, in.Query, limit)
		if err != nil {
			return agent.Result{Output: "resolve failed: " + err.Error(), IsError: true}, nil
		}
		return agent.Result{Output: renderResolve(in.Query, hits)}, nil
	case "neighbors":
		hits, err := t.g.ResolveQuiet(in.Query, 1)
		if err != nil || len(hits) == 0 {
			return agent.Result{Output: "no entity matches " + in.Query, IsError: true}, nil
		}
		ns, err := t.g.Neighbors(hits[0].Entity.ID)
		if err != nil {
			return agent.Result{Output: "neighbors failed: " + err.Error(), IsError: true}, nil
		}
		return agent.Result{Output: renderNeighbors(hits[0].Entity, ns)}, nil
	default:
		return agent.Result{Output: "unknown action " + in.Action + " (add|relate|resolve|neighbors)", IsError: true}, nil
	}
}

func renderResolve(phrase string, hits []ScoredEntity) string {
	if len(hits) == 0 {
		return fmt.Sprintf("%q resolves to nothing known", phrase)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q resolves to:\n", phrase)
	for _, h := range hits {
		fmt.Fprintf(&b, "- [%s] %s\n", h.Entity.Kind, h.Entity.Name)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderNeighbors(e Entity, ns []Neighbor) string {
	if len(ns) == 0 {
		return e.Name + " has no known relations"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s connects to:\n", e.Name)
	for _, n := range ns {
		other := n.Other.Name
		if other == "" {
			other = "(forgotten)"
		}
		if n.Outgoing {
			fmt.Fprintf(&b, "- %s %s\n", n.Relation.Verb, other)
		} else {
			fmt.Fprintf(&b, "- %s %s (incoming)\n", n.Relation.Verb, other)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
