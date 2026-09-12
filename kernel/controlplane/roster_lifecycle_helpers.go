// SPDX-License-Identifier: MIT

// Control-plane roster lifecycle analysis helpers (impact + reference + subagent helpers).
// Code extracted from roster_lifecycle.go during the Day-93 god-file split.
// Public API unchanged.
package controlplane


import (
	"sort"
	"strconv"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
)

func boolish(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		x = strings.TrimSpace(strings.ToLower(x))
		return x == "1" || x == "true" || x == "yes" || x == "on"
	default:
		return false
	}
}

func agentMailboxImpactLabel(msg board.Message, slug string) (string, bool) {
	from := strings.ToLower(strings.TrimSpace(msg.From))
	to := strings.ToLower(strings.TrimSpace(msg.To))
	acked := boardMessageAckedBy(msg, slug)
	var direction string
	switch {
	case from == slug:
		direction = "sent"
	case to == slug:
		direction = "received"
	case msg.To == board.Everyone && from != slug:
		direction = "broadcast"
	case acked:
		direction = "acked"
	default:
		return "", false
	}
	topic := strings.TrimSpace(msg.Topic)
	if topic == "" {
		topic = "board"
	}
	id := strings.TrimSpace(msg.ID)
	if id == "" {
		id = strconv.FormatInt(msg.TSMS, 10)
	}
	return topic + " " + direction + " (" + id + ")", true
}

func agentSubagentImpact(slug string, children []roster.Profile) []string {
	out := make([]string, 0, len(children))
	for _, child := range children {
		roles := make([]string, 0, 2)
		if strings.EqualFold(strings.TrimSpace(child.OwnerAgent), slug) {
			roles = append(roles, "owner")
		}
		if strings.EqualFold(strings.TrimSpace(child.ParentAgent), slug) {
			roles = append(roles, "parent")
		}
		if len(roles) == 0 {
			roles = append(roles, "descendant")
		}
		label := child.Slug
		if strings.TrimSpace(child.Name) != "" && strings.TrimSpace(child.Name) != child.Slug {
			label = strings.TrimSpace(child.Name) + " (" + child.Slug + ")"
		}
		if len(roles) > 0 {
			label += " [" + strings.Join(roles, ", ") + "]"
		}
		if child.Retired {
			label += " [retired]"
		}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func workflowNodeConfigReferencesAgent(raw json.RawMessage, slug string) bool {
	if len(raw) == 0 {
		return false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	return jsonValueReferencesAgent(v, strings.ToLower(strings.TrimSpace(slug)), "")
}

func jsonValueReferencesAgent(v any, slug, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if jsonValueReferencesAgent(value, slug, strings.ToLower(strings.TrimSpace(k))) {
				return true
			}
		}
	case []any:
		for _, value := range x {
			if jsonValueReferencesAgent(value, slug, key) {
				return true
			}
		}
	case string:
		if !agentReferenceConfigKey(key) {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(x), slug)
	}
	return false
}

func agentReferenceConfigKey(key string) bool {
	switch key {
	case "agent", "agent_slug", "target_agent", "owner_agent", "parent_agent", "delegate_to", "source_agent", "root_agent":
		return true
	default:
		return false
	}
}

func (s *Server) agentSubagents(slug string) []roster.Profile {
	root := strings.TrimSpace(slug)
	if root == "" {
		return nil
	}
	byManager := map[string][]roster.Profile{}
	for _, p := range s.k.Roster().List() {
		childSlug := strings.TrimSpace(p.Slug)
		if childSlug == "" || strings.EqualFold(childSlug, root) {
			continue
		}
		seenManager := map[string]bool{}
		for _, manager := range []string{strings.TrimSpace(p.OwnerAgent), strings.TrimSpace(p.ParentAgent)} {
			if manager == "" || strings.EqualFold(manager, childSlug) || seenManager[strings.ToLower(manager)] {
				continue
			}
			seenManager[strings.ToLower(manager)] = true
			byManager[strings.ToLower(manager)] = append(byManager[strings.ToLower(manager)], p)
		}
	}
	var out []roster.Profile
	seen := map[string]bool{strings.ToLower(root): true}
	var walk func(string)
	walk = func(parent string) {
		for _, child := range byManager[strings.ToLower(strings.TrimSpace(parent))] {
			key := strings.ToLower(strings.TrimSpace(child.Slug))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, child)
			walk(child.Slug)
		}
	}
	walk(root)
	sort.Slice(out, func(i, j int) bool {
		return strings.Compare(out[i].Slug, out[j].Slug) < 0
	})
	return out
}

