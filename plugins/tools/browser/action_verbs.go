// SPDX-License-Identifier: MIT

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

func actionVerbToActionInput(name string, in actionVerbInput) (actionInput, error) {
	out := actionInput{
		URL:           in.URL,
		Actions:       append([]browserStep(nil), in.Actions...),
		Screenshot:    in.Screenshot,
		FullPage:      in.FullPage,
		Snapshot:      in.Snapshot,
		SnapshotLimit: in.SnapshotLimit,
		Events:        in.Events,
		EventLimit:    in.EventLimit,
		Downloads:     in.Downloads,
		Cookies:       in.Cookies,
		Profile:       in.Profile,
		SessionID:     in.SessionID,
		TabID:         in.TabID,
		Viewport:      in.Viewport,
		Extract:       in.Extract,
		TimeoutMS:     in.TimeoutMS,
		MaxChars:      in.MaxChars,
	}
	switch name {
	case ActionVerbOpen:
		out.Snapshot = defaultBool(out.Snapshot, true)
	case ActionVerbSnapshot:
		out.Snapshot = defaultBool(out.Snapshot, true)
		out.Screenshot = defaultBool(out.Screenshot, false)
	case ActionVerbClick:
		if strings.TrimSpace(in.Selector) == "" {
			return actionInput{}, fmt.Errorf("%s: selector or ref required", name)
		}
		out.Actions = append(out.Actions, browserStep{Type: "click", Selector: in.Selector})
	case ActionVerbType:
		if strings.TrimSpace(in.Selector) == "" {
			return actionInput{}, fmt.Errorf("%s: selector or ref required", name)
		}
		if strings.TrimSpace(in.Value) == "" {
			return actionInput{}, fmt.Errorf("%s: value required", name)
		}
		out.Actions = append(out.Actions, browserStep{Type: "type", Selector: in.Selector, Value: in.Value, DelayMS: in.DelayMS})
		if in.Submit {
			key := strings.TrimSpace(in.Key)
			if key == "" {
				key = "Enter"
			}
			out.Actions = append(out.Actions, browserStep{Type: "press", Selector: in.Selector, Key: key})
		}
	case ActionVerbWait:
		out.Actions = append(out.Actions, browserStep{Type: "wait", Selector: in.WaitSelector, MS: in.WaitMS})
	case ActionVerbScreenshot:
		out.Screenshot = defaultBool(out.Screenshot, true)
		out.Snapshot = defaultBool(out.Snapshot, false)
	case ActionVerbDownloads:
		out.Downloads = defaultBool(out.Downloads, true)
		if strings.TrimSpace(in.Selector) != "" {
			out.Actions = append(out.Actions, browserStep{Type: "click", Selector: in.Selector})
		}
	case ActionVerbCookies:
		out.Cookies = true
		out.Screenshot = defaultBool(out.Screenshot, false)
		out.Snapshot = defaultBool(out.Snapshot, false)
	case ActionVerbTabs:
		return actionInput{}, fmt.Errorf("%s is handled without browser.action conversion", name)
	case ActionVerbClose:
		return actionInput{}, fmt.Errorf("%s is handled without browser.action conversion", name)
	default:
		return actionInput{}, fmt.Errorf("unknown browser verb %q", name)
	}
	if name != ActionVerbWait {
		out.Actions = appendOptionalWait(out.Actions, in)
	}
	return out, nil
}

func (t *ActionVerbTool) resolveRef(in *actionVerbInput) error {
	ref := strings.TrimSpace(in.Ref)
	if ref == "" {
		return nil
	}
	selector, err := t.Base.ResolveTabRef(in.SessionID, in.TabID, ref)
	if err != nil {
		return err
	}
	switch t.Name {
	case ActionVerbWait:
		if strings.TrimSpace(in.WaitSelector) == "" {
			in.WaitSelector = selector
		}
	case ActionVerbClick, ActionVerbType, ActionVerbDownloads:
		if strings.TrimSpace(in.Selector) == "" {
			in.Selector = selector
		}
	default:
		return fmt.Errorf("%s: ref is only supported for browser.click, browser.type, browser.wait, and browser.downloads", t.Name)
	}
	return nil
}

func appendOptionalWait(steps []browserStep, in actionVerbInput) []browserStep {
	if strings.TrimSpace(in.WaitSelector) == "" && in.WaitMS <= 0 {
		return steps
	}
	return append(steps, browserStep{Type: "wait", Selector: in.WaitSelector, MS: in.WaitMS})
}

func defaultBool(v *bool, fallback bool) *bool {
	if v != nil {
		return v
	}
	return &fallback
}

func actionVerbDescription(name string) string {
	switch name {
	case ActionVerbOpen:
		return "Open an allowed URL in Playwright and return extracted text, snapshot, events, and optional screenshot artifacts. profile=session can carry cookies/storage and tab_id can save/reuse the final URL, but this does not keep a live tab."
	case ActionVerbSnapshot:
		return "Open an allowed URL or saved session tab_id and return compact refs/selectors for visible interactive elements. profile=session can carry cookies/storage, but this does not keep a live tab."
	case ActionVerbClick:
		return "Open an allowed URL or saved session tab_id, run optional setup actions, click a selector or saved snapshot ref, optionally wait, and return the resulting page state."
	case ActionVerbType:
		return "Open an allowed URL or saved session tab_id, run optional setup actions, type text into a selector or saved snapshot ref, optionally submit/wait, and return the resulting page state."
	case ActionVerbWait:
		return "Open an allowed URL or saved session tab_id, run optional setup actions, wait for a selector, saved snapshot ref, or duration, and return the resulting page state."
	case ActionVerbScreenshot:
		return "Open an allowed URL and save a screenshot artifact from the rendered page."
	case ActionVerbDownloads:
		return "Open an allowed URL, run actions or click a selector that may trigger downloads, and save resulting download artifacts."
	case ActionVerbCookies:
		return "Open an allowed URL or saved session tab_id and return final-page browser cookies. Cookie values are sensitive and only returned when this tool is called."
	case ActionVerbTabs:
		return "List persistent AGEZT tab refs saved under a browser session. These are URL/ref state records, not live browser tabs."
	case ActionVerbClose:
		return "Close an AGEZT-managed browser session, or close one persistent tab ref when tab_id is supplied."
	default:
		return "Browser verb backed by browser.action."
	}
}

func actionVerbEffect(name string) agent.ToolEffect {
	if name == ActionVerbClose {
		return agent.ToolEffect{
			Class: agent.EffectIrreversible,
			PredictedEffects: []string{
				"delete the local AGEZT-managed browser session directory for the requested session_id",
			},
			AffectedResources: []string{"AGEZT-managed browser action session store"},
			RollbackNotes:     "Deleted browser session cookies/storage cannot be generically restored.",
			Confidence:        0.8,
		}
	}
	return agent.ToolEffect{
		Class: agent.EffectIrreversible,
		PredictedEffects: []string{
			"launch a headless browser process and navigate to an allowed HTTP(S) page",
			"perform the named browser verb over a Playwright page",
			"optionally return cookies or write screenshot/download artifacts to the local artifact store",
		},
		AffectedResources: []string{"allowed browser.action hosts", "local artifact store when available"},
		RollbackNotes:     "Local screenshot/download/session artifacts can be deleted. Remote page actions cannot be generically rolled back.",
		Confidence:        0.55,
	}
}

func actionVerbSchema(name string) json.RawMessage {
	if name == ActionVerbClose {
		return json.RawMessage(`{"type":"object","required":["session_id"],"properties":{"session_id":{"type":"string","description":"AGEZT-managed browser session id to close."},"tab_id":{"type":"string","description":"Optional tab ref to close. When omitted, the entire session directory is deleted."}}}`)
	}
	if name == ActionVerbTabs {
		return json.RawMessage(`{"type":"object","required":["session_id"],"properties":{"session_id":{"type":"string","description":"AGEZT-managed browser session id whose saved tab refs should be listed."}}}`)
	}
	common := `"url":{"type":"string","description":"Initial absolute http/https URL. Required unless tab_id resolves to a saved AGEZT session tab URL."},
    "actions":{"type":"array","description":"Optional setup actions before the named verb.","items":{"type":"object"}},
    "ref":{"type":"string","description":"For profile=session with tab_id: snapshot ref such as e1. The selector is resolved from the saved tab snapshot; stale/missing refs ask for a fresh browser.snapshot."},
    "wait_selector":{"type":"string","description":"Optional selector to wait for after the named verb."},
    "wait_ms":{"type":"integer","description":"Optional milliseconds to wait after the named verb."},
    "screenshot":{"type":"boolean","description":"Whether to save a screenshot artifact."},
    "full_page":{"type":"boolean","description":"Whether screenshot should capture the full page."},
    "snapshot":{"type":"boolean","description":"Whether to return compact visible interactive element refs."},
    "snapshot_limit":{"type":"integer","description":"Maximum snapshot elements returned; default 60, max 200."},
    "events":{"type":"boolean","description":"Whether to return console/page/network event summaries."},
    "event_limit":{"type":"integer","description":"Maximum events per event bucket; default 50, max 200."},
    "downloads":{"type":"boolean","description":"Whether to accept and save page downloads."},
    "cookies":{"type":"boolean","description":"Whether to return final-page browser cookies. Default false; browser.cookies sets it true."},
    "profile":{"type":"string","enum":["isolated","session","user-attached","remote-cdp"],"description":"Browser profile policy. Default isolated. session uses an AGEZT-managed persistent browser session directory; user-attached and remote-cdp require operator env opt-in."},
    "session_id":{"type":"string","description":"For profile=session: AGEZT-managed browser session id for cookie/session carryover."},
    "tab_id":{"type":"string","description":"For profile=session: persistent AGEZT tab ref. A call with url stores/updates it; later calls may omit url and reuse the saved final URL. This is not a live browser tab."},
    "extract":{"type":"string","description":"text, html, or a selector whose matched visible text should be returned."},
    "timeout_ms":{"type":"integer","description":"Per-call browser timeout in ms; default 30000, max 120000."},
    "max_chars":{"type":"integer","description":"Maximum returned text chars; default 65536."},
    "viewport":{"type":"object","properties":{"width":{"type":"integer"},"height":{"type":"integer"}}}`
	extra := ""
	required := ``
	switch name {
	case ActionVerbClick:
		extra = `,"selector":{"type":"string","description":"CSS or Playwright text selector to click. Optional when ref is supplied."}`
	case ActionVerbType:
		extra = `,"selector":{"type":"string","description":"CSS or Playwright text selector to type into. Optional when ref is supplied."},
    "value":{"type":"string","description":"Text to type."},
    "submit":{"type":"boolean","description":"Whether to press Enter or key after typing."},
    "key":{"type":"string","description":"Key to press when submit=true; default Enter."},
    "delay_ms":{"type":"integer","description":"Per-character typing delay in ms."}`
		required = `"value"`
	case ActionVerbDownloads:
		extra = `,"selector":{"type":"string","description":"Optional selector to click to trigger a download."}`
	case ActionVerbWait:
		// wait_selector or wait_ms is validated by browser.action.
	case ActionVerbOpen, ActionVerbSnapshot, ActionVerbScreenshot, ActionVerbCookies:
	default:
	}
	return json.RawMessage(`{"type":"object","anyOf":[{"required":["url"]},{"required":["tab_id"]}],"required":[` + required + `],"properties":{` + common + extra + `}}`)
}
