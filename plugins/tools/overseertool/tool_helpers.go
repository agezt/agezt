// SPDX-License-Identifier: MIT

// Overseer tool: profile parse/validate helpers + view/ok/err helpers.
// Code extracted from tool.go during the Day-123 god-file split.
// Public API unchanged.
package overseertool


import (
	"fmt"
	"strings"
	"time"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
)

func parseProfile(profileRaw, fullRaw json.RawMessage) (roster.Profile, error) {
	if hasProfileObject(profileRaw) {
		var p roster.Profile
		if err := json.Unmarshal(profileRaw, &p); err != nil {
			return roster.Profile{}, fmt.Errorf("profile: %w", err)
		}
		return p, nil
	}
	if hasFlatProfileFields(fullRaw) {
		var p roster.Profile
		if err := json.Unmarshal(fullRaw, &p); err != nil {
			return roster.Profile{}, fmt.Errorf("profile: %w", err)
		}
		return p, nil
	}
	return roster.Profile{}, fmt.Errorf(`a "profile" object is required (nest the agent fields under "profile", or pass them as top-level keys like "slug"/"name"/"soul"/"model")`)
}

// hasProfileObject reports whether raw is a populated "profile" object (not
// absent, null, or empty).
func hasProfileObject(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null" && s != "{}"
}

// hasFlatProfileFields reports whether the top-level input carries any key that
// isn't a control key — i.e. the model flattened profile fields onto the input.
func hasFlatProfileFields(fullRaw json.RawMessage) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(fullRaw, &m) != nil {
		return false
	}
	for k := range m {
		if !overseerControlKeys[k] {
			return true
		}
	}
	return false
}

func cancelNote(ok bool) string {
	if ok {
		return "the run was in flight and has been cancelled"
	}
	return "no in-flight run matched that id (already finished or unknown)"
}

func agentView(p roster.Profile) map[string]any {
	state := "enabled"
	switch {
	case p.Retired:
		state = "retired"
	case !p.Enabled:
		state = "paused"
	}
	v := map[string]any{"slug": p.Slug, "state": state, "enabled": p.Enabled, "retired": p.Retired}
	if p.Name != "" {
		v["name"] = p.Name
	}
	if p.Model != "" {
		v["model"] = p.Model
	}
	return v
}

func helpView(m board.Message) map[string]any {
	v := map[string]any{"id": m.ID, "text": m.Text}
	if m.From != "" {
		v["from"] = m.From
	}
	if m.To != "" {
		v["to"] = m.To
	}
	if m.TSMS > 0 {
		v["at"] = time.UnixMilli(m.TSMS).Format(time.RFC3339)
	}
	return v
}

// cleanSlugs trims whitespace from each slug and removes empties.
func cleanSlugs(slugs []string) []string {
	if len(slugs) == 0 {
		return nil
	}
	out := make([]string, 0, len(slugs))
	for _, s := range slugs {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "overseer: " + msg, IsError: true}
}
