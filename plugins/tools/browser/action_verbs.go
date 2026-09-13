// SPDX-License-Identifier: MIT

// Package browser: ActionVerbTool + ActionVerb* consts + NewActionVerbTools
// + Definition + Invoke + actionVerbInput struct (the contract surface for
// the small first-class browser.* verbs). The per-verb → action.Input
// conversion moved to action_verbs_convert.go; the per-verb agent.Tool
// metadata (description / effect / schema) moved to action_verbs_meta.go.
// Day-211 god-file split. Public API unchanged.
package browser


import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)
const (
	ActionVerbOpen       = "browser.open"
	ActionVerbSnapshot   = "browser.snapshot"
	ActionVerbClick      = "browser.click"
	ActionVerbType       = "browser.type"
	ActionVerbWait       = "browser.wait"
	ActionVerbScreenshot = "browser.screenshot"
	ActionVerbDownloads  = "browser.downloads"
	ActionVerbCookies    = "browser.cookies"
	ActionVerbTabs       = "browser.tabs"
	ActionVerbClose      = "browser.close"
)

// ActionVerbTool exposes small first-class browser.* verbs over the same
// Playwright engine as browser.action. By default each call is isolated; when
// profile=session is requested, calls share an AGEZT-managed persistent browser
// context directory until browser.close removes it.
type ActionVerbTool struct {
	Name string
	Base *ActionTool
}
// NewActionVerbTools returns the visible browser.* family backed by base.
func NewActionVerbTools(base *ActionTool) []agent.Tool {
	if base == nil {
		return nil
	}
	names := []string{
		ActionVerbOpen,
		ActionVerbSnapshot,
		ActionVerbClick,
		ActionVerbType,
		ActionVerbWait,
		ActionVerbScreenshot,
		ActionVerbDownloads,
		ActionVerbCookies,
		ActionVerbTabs,
		ActionVerbClose,
	}
	out := make([]agent.Tool, 0, len(names))
	for _, name := range names {
		out = append(out, &ActionVerbTool{Name: name, Base: base})
	}
	return out
}

func (t *ActionVerbTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        t.Name,
		Description: actionVerbDescription(t.Name),
		InputSchema: actionVerbSchema(t.Name),
		Effect:      actionVerbEffect(t.Name),
	}
}

func (t *ActionVerbTool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	if t == nil || t.Base == nil {
		return errResult("browser action driver not configured"), nil
	}
	var in actionVerbInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("%s: parse input: %w", t.Name, err)
	}
	if t.Name == ActionVerbClose {
		var out string
		var err error
		if strings.TrimSpace(in.TabID) != "" {
			out, err = t.Base.CloseTab(in.SessionID, in.TabID)
		} else {
			out, err = t.Base.CloseSession(in.SessionID)
		}
		if err != nil {
			return errResult(err.Error()), nil
		}
		return agent.Result{Output: out}, nil
	}
	if t.Name == ActionVerbTabs {
		out, err := t.Base.ListTabs(in.SessionID)
		if err != nil {
			return errResult(err.Error()), nil
		}
		return agent.Result{Output: out}, nil
	}
	if err := t.resolveRef(&in); err != nil {
		return errResult(err.Error()), nil
	}
	converted, err := actionVerbToActionInput(t.Name, in)
	if err != nil {
		return errResult(err.Error()), nil
	}
	spec, err := json.Marshal(converted)
	if err != nil {
		return agent.Result{}, fmt.Errorf("%s: marshal action input: %w", t.Name, err)
	}
	return t.Base.Invoke(ctx, spec)
}

type actionVerbInput struct {
	URL           string           `json:"url"`
	Actions       []browserStep    `json:"actions,omitempty"`
	Ref           string           `json:"ref,omitempty"`
	Selector      string           `json:"selector,omitempty"`
	Value         string           `json:"value,omitempty"`
	Key           string           `json:"key,omitempty"`
	Submit        bool             `json:"submit,omitempty"`
	DelayMS       int64            `json:"delay_ms,omitempty"`
	WaitSelector  string           `json:"wait_selector,omitempty"`
	WaitMS        int64            `json:"wait_ms,omitempty"`
	Screenshot    *bool            `json:"screenshot,omitempty"`
	FullPage      bool             `json:"full_page,omitempty"`
	Snapshot      *bool            `json:"snapshot,omitempty"`
	SnapshotLimit int              `json:"snapshot_limit,omitempty"`
	Events        *bool            `json:"events,omitempty"`
	EventLimit    int              `json:"event_limit,omitempty"`
	Downloads     *bool            `json:"downloads,omitempty"`
	Cookies       bool             `json:"cookies,omitempty"`
	Profile       string           `json:"profile,omitempty"`
	SessionID     string           `json:"session_id,omitempty"`
	TabID         string           `json:"tab_id,omitempty"`
	Viewport      *browserViewport `json:"viewport,omitempty"`
	Extract       string           `json:"extract,omitempty"`
	TimeoutMS     int64            `json:"timeout_ms,omitempty"`
	MaxChars      int              `json:"max_chars,omitempty"`
}
