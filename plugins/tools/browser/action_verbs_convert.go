// SPDX-License-Identifier: MIT

// Package browser: per-verb → action.Input conversion helpers
// (actionVerbToActionInput + resolveRef + appendOptionalWait). Each verb has
// its own input shape (open vs snapshot vs click vs ...) and these helpers
// translate to the unified action.Input that ActionTool consumes. Extracted
// from action_verbs.go during the Day-211 god-file split. Public API unchanged.
package browser


import (
	"fmt"
	"strings"
)
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
