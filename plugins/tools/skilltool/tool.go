// SPDX-License-Identifier: MIT

package skilltool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/skill"
)

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "skill",
		Description: "Teach yourself a reusable procedure and manage your own skills. " +
			"op=learn distills a repeatable how-to into a named, versioned skill (a draft); " +
			"op=list shows your skills with their status and usage; op=show returns one skill's " +
			"full body; op=promote advances a skill toward your active retrieval pool " +
			"(draft→shadow→active); op=retire pulls a skill that's gone wrong (quarantine). " +
			"A skill can also carry a BUNDLE of files (agentskills.io shape): reference docs and " +
			"scripts. op=files lists a skill's bundled resources and the directory they live in; " +
			"op=read returns one resource's contents — read a reference doc when you need the " +
			"detail, then run a bundled script (e.g. scripts/setup.sh to install a CLI) with your " +
			"shell or code_exec tool from the reported dir. " +
			"Use this to get better over time: when you work out how to do something, capture it " +
			"as a skill so future runs can reuse it. Every change is journaled and reversible.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":          {"type":"string", "enum":["learn","list","show","promote","retire","files","read"]},
    "name":        {"type":"string", "description":"For op=learn: a short kebab-case name, e.g. \"diagnose-failing-ci\"."},
    "description": {"type":"string", "description":"For op=learn: one line on when this skill applies (the retrieval-matching key)."},
    "body":        {"type":"string", "description":"For op=learn: the procedure itself — the steps/instructions to follow."},
    "triggers":    {"type":"array", "items":{"type":"string"}, "description":"For op=learn (optional): tags/conditions hinting when the skill is relevant."},
    "tools":       {"type":"array", "items":{"type":"string"}, "description":"For op=learn (optional): tools this skill expects to be available."},
    "id":          {"type":"string", "description":"For op=show/promote/retire/files/read: the skill id (a prefix is accepted)."},
    "path":        {"type":"string", "description":"For op=read: the bundle-relative path of the resource to read (from op=files), e.g. \"reference/api.md\" or \"scripts/setup.sh\"."},
    "reason":      {"type":"string", "description":"For op=retire (optional): why you're pulling the skill."}
  }
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"Read skill records and bundled resources.",
				"Learn, promote, or quarantine reusable skills that affect future retrieval and behavior.",
			},
			AffectedResources: []string{"skill store", "skill retrieval pool", "skill bundle files"},
			RollbackNotes:     "Retire/quarantine an unwanted skill or promote a prior version; file reads need no rollback.",
			Confidence:        0.85,
		},
	}
}

type input struct {
	Op          string   `json:"op"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
	Triggers    []string `json:"triggers"`
	Tools       []string `json:"tools"`
	ID          string   `json:"id"`
	Path        string   `json:"path"`
	Reason      string   `json:"reason"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("skill: parse input: %w", err)
	}
	f := t.current()
	if f == nil {
		return errResult("skill learning is not available on this daemon"), nil
	}
	corr := agent.CorrelationFromContext(ctx)

	switch in.Op {
	case "learn":
		if strings.TrimSpace(in.Name) == "" {
			return errResult(`op=learn needs a "name"`), nil
		}
		if strings.TrimSpace(in.Body) == "" {
			return errResult(`op=learn needs a "body" (the procedure)`), nil
		}
		sk, created, err := f.Create(corr, skill.CreateSpec{
			Name:          in.Name,
			Description:   in.Description,
			Triggers:      in.Triggers,
			Body:          in.Body,
			ToolsRequired: in.Tools,
		})
		if err != nil {
			return errResult(err.Error()), nil
		}
		msg := "learned a new skill (draft)"
		if !created {
			msg = "you already knew this skill (refreshed)"
		} else if sk.Status != skill.StatusDraft {
			msg = "learned and auto-staged to " + string(sk.Status)
		}
		return okEntry(msg, sk), nil

	case "list":
		all, err := f.List()
		if err != nil {
			return errResult(err.Error()), nil
		}
		views := make([]map[string]any, 0, len(all))
		for _, sk := range all {
			views = append(views, skillView(sk))
		}
		return okJSON(map[string]any{"count": len(views), "skills": views}), nil

	case "show":
		sk, err := t.resolve(f, in.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		v := skillView(sk)
		v["body"] = sk.Body
		if len(sk.Lineage) > 0 {
			v["lineage"] = sk.Lineage
		}
		return okJSON(v), nil

	case "promote":
		sk, err := t.resolve(f, in.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		to, err := f.Promote(corr, sk.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"id": shortID(sk.ID), "name": sk.Name, "status": string(to), "message": "promoted to " + string(to)}), nil

	case "retire":
		sk, err := t.resolve(f, in.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		reason := strings.TrimSpace(in.Reason)
		if reason == "" {
			reason = "retired by the agent"
		}
		if err := f.Quarantine(corr, sk.ID, reason); err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{"id": shortID(sk.ID), "name": sk.Name, "status": string(skill.StatusQuarantined), "message": "retired (quarantined)"}), nil

	case "files":
		sk, err := t.resolve(f, in.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		bundles := f.Bundles()
		if bundles == nil {
			return errResult("skill bundles are not available on this daemon"), nil
		}
		files, err := bundles.List(sk.Name)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{
			"id": shortID(sk.ID), "name": sk.Name, "dir": bundles.Dir(sk.Name),
			"files": files, "count": len(files),
		}), nil

	case "read":
		sk, err := t.resolve(f, in.ID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if strings.TrimSpace(in.Path) == "" {
			return errResult(`op=read needs a "path" (a bundle-relative file from op=files)`), nil
		}
		bundles := f.Bundles()
		if bundles == nil {
			return errResult("skill bundles are not available on this daemon"), nil
		}
		data, err := bundles.Read(sk.Name, in.Path)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return okJSON(map[string]any{
			"id": shortID(sk.ID), "name": sk.Name, "path": in.Path,
			"content": string(data), "bytes": len(data),
		}), nil

	case "":
		return errResult("op required (learn|list|show|promote|retire|files|read)"), nil
	default:
		return errResult("unknown op " + in.Op + " (learn|list|show|promote|retire|files|read)"), nil
	}
}

// resolve looks up a skill by exact id or, failing that, a unique id prefix —
// agents see truncated ids in op=list, so a prefix is the natural handle.
func (t *Tool) resolve(f forge, id string) (skill.Skill, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return skill.Skill{}, fmt.Errorf("an %q id is required", "id")
	}
	if sk, ok, err := f.Get(id); err != nil {
		return skill.Skill{}, err
	} else if ok {
		return sk, nil
	}
	all, err := f.List()
	if err != nil {
		return skill.Skill{}, err
	}
	var match skill.Skill
	n := 0
	for _, sk := range all {
		if strings.HasPrefix(sk.ID, id) {
			match = sk
			n++
		}
	}
	switch n {
	case 1:
		return match, nil
	case 0:
		return skill.Skill{}, fmt.Errorf("no skill with id %s", id)
	default:
		return skill.Skill{}, fmt.Errorf("id %s is ambiguous (%d skills match — use more characters)", id, n)
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func skillView(s skill.Skill) map[string]any {
	v := map[string]any{
		"id":      shortID(s.ID),
		"name":    s.Name,
		"status":  string(s.Status),
		"version": s.Version,
		"uses":    s.Metrics.Uses,
	}
	if s.Description != "" {
		v["description"] = s.Description
	}
	if len(s.Triggers) > 0 {
		v["triggers"] = s.Triggers
	}
	return v
}

func okEntry(msg string, s skill.Skill) agent.Result {
	v := skillView(s)
	v["message"] = msg
	return okJSON(v)
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "skill: " + msg, IsError: true}
}
