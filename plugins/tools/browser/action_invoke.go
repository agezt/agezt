// SPDX-License-Identifier: MIT
//
// browser.action Invoke implementation: actionInput + browserStep +
// browserViewport request types + Invoke + textLimit + validateActions +
// validateActionOptions + prepareProfile + sessionDir. Split from action.go
// during Day 211 god-file refactor (#38). Public API unchanged.
package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

type actionInput struct {
	URL           string           `json:"url"`
	Actions       []browserStep    `json:"actions,omitempty"`
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
	UserDataDir   string           `json:"user_data_dir,omitempty"`
	RemoteCDPURL  string           `json:"cdp_url,omitempty"`
	Viewport      *browserViewport `json:"viewport,omitempty"`
	Extract       string           `json:"extract,omitempty"`
	TimeoutMS     int64            `json:"timeout_ms,omitempty"`
	MaxChars      int              `json:"max_chars,omitempty"`
}

type browserStep struct {
	Type     string `json:"type"`
	URL      string `json:"url,omitempty"`
	Selector string `json:"selector,omitempty"`
	Value    string `json:"value,omitempty"`
	Key      string `json:"key,omitempty"`
	MS       int64  `json:"ms,omitempty"`
	X        int64  `json:"x,omitempty"`
	Y        int64  `json:"y,omitempty"`
	DelayMS  int64  `json:"delay_ms,omitempty"`
}

type browserViewport struct {
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// Invoke implements agent.Tool.
func (t *ActionTool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in actionInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("browser.action: parse input: %w", err)
	}
	if strings.TrimSpace(t.DriverPath) == "" {
		return errResult("browser action driver not configured (set AGEZT_BROWSER_ACTION_DRIVER or disable AGEZT_BROWSER_ACTIONS)"), nil
	}
	if err := validateActionOptions(in); err != nil {
		return errResult(err.Error()), nil
	}
	if err := t.prepareProfile(ctx, &in); err != nil {
		return errResult(err.Error()), nil
	}
	if err := t.resolveTabURL(&in); err != nil {
		return errResult(err.Error()), nil
	}
	if strings.TrimSpace(in.URL) == "" {
		return errResult("url required unless tab_id resolves to a saved session tab"), nil
	}
	if err := t.validateURL(ctx, in.URL); err != nil {
		return errResult(err.Error()), nil
	}
	if err := t.validateActions(ctx, in.Actions); err != nil {
		return errResult(err.Error()), nil
	}
	timeout := DefaultActionTimeout
	if t.Timeout > 0 {
		timeout = t.Timeout
	}
	if in.TimeoutMS > 0 {
		timeout = time.Duration(in.TimeoutMS) * time.Millisecond
	}
	if timeout > MaxActionTimeout {
		timeout = MaxActionTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	spec, err := json.Marshal(in)
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser.action: marshal driver spec: %w", err)
	}
	dir := strings.TrimSpace(t.DriverDir)
	driver := strings.TrimSpace(t.DriverPath)
	if dir == "" {
		dir = filepath.Dir(driver)
	}
	run := t.run
	if run == nil {
		run = runActionDriver
	}
	node := strings.TrimSpace(t.NodePath)
	if node == "" {
		node = "node"
	}
	out, err := run(cctx, actionRunSpec{Dir: dir, NodePath: node, DriverPath: driver, Spec: spec})
	if err != nil {
		msg := strings.TrimSpace(out.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(out.Stdout)
		}
		if msg == "" {
			msg = err.Error()
		}
		return errResult("browser driver failed: " + truncateActionOutput(msg)), nil
	}
	normalized, source, err := normalizeActionOutput(out.Stdout, t.textLimit(in.MaxChars))
	if err != nil {
		return errResult("browser driver returned invalid JSON: " + err.Error()), nil
	}
	normalized = t.finalizeTabOutput(normalized, in, source)
	normalized = t.attachArtifacts(normalized)
	return agent.Result{
		Output:            normalized,
		ObservationTrust:  agent.ObservationUntrusted,
		ObservationSource: source,
	}, nil
}

func (t *ActionTool) textLimit(requested int) int {
	limit := t.MaxTextChars
	if limit <= 0 {
		limit = DefaultActionMaxTextChars
	}
	if requested > 0 && requested < limit {
		limit = requested
	}
	return limit
}

func (t *ActionTool) validateActions(ctx context.Context, steps []browserStep) error {
	for i, step := range steps {
		typ := strings.ToLower(strings.TrimSpace(step.Type))
		switch typ {
		case "goto":
			if strings.TrimSpace(step.URL) == "" {
				return fmt.Errorf("action %d: goto requires url", i)
			}
			if err := t.validateURL(ctx, step.URL); err != nil {
				return fmt.Errorf("action %d: %w", i, err)
			}
		case "click", "type", "press", "check", "uncheck", "hover":
			if strings.TrimSpace(step.Selector) == "" {
				return fmt.Errorf("action %d: %s requires selector", i, typ)
			}
		case "fill":
			if strings.TrimSpace(step.Selector) == "" {
				return fmt.Errorf("action %d: fill requires selector", i)
			}
		case "select":
			if strings.TrimSpace(step.Selector) == "" {
				return fmt.Errorf("action %d: select requires selector", i)
			}
			if strings.TrimSpace(step.Value) == "" {
				return fmt.Errorf("action %d: select requires value", i)
			}
		case "scroll":
			if strings.TrimSpace(step.Selector) == "" && step.X == 0 && step.Y == 0 {
				return fmt.Errorf("action %d: scroll requires selector, x, or y", i)
			}
		case "wait":
			if strings.TrimSpace(step.Selector) == "" && step.MS <= 0 {
				return fmt.Errorf("action %d: wait requires selector or positive ms", i)
			}
		default:
			return fmt.Errorf("action %d: unknown action type %q", i, step.Type)
		}
	}
	return nil
}

func validateActionOptions(in actionInput) error {
	if in.TimeoutMS < 0 {
		return errors.New("timeout_ms must be positive")
	}
	if in.MaxChars < 0 {
		return errors.New("max_chars must be positive")
	}
	if in.SnapshotLimit < 0 {
		return errors.New("snapshot_limit must be positive")
	}
	if in.EventLimit < 0 {
		return errors.New("event_limit must be positive")
	}
	if in.Viewport != nil {
		if in.Viewport.Width < 0 || in.Viewport.Height < 0 {
			return errors.New("viewport width/height must be positive")
		}
	}
	for i, step := range in.Actions {
		if step.MS < 0 {
			return fmt.Errorf("action %d: ms must be positive", i)
		}
		if step.DelayMS < 0 {
			return fmt.Errorf("action %d: delay_ms must be positive", i)
		}
	}
	return nil
}

func (t *ActionTool) prepareProfile(ctx context.Context, in *actionInput) error {
	mode := strings.ToLower(strings.TrimSpace(in.Profile))
	if mode == "" && strings.TrimSpace(in.TabID) != "" && strings.TrimSpace(in.SessionID) != "" {
		mode = actionProfileSession
	}
	if mode == "" {
		mode = actionProfileIsolated
	}
	switch mode {
	case actionProfileIsolated:
		if strings.TrimSpace(in.TabID) != "" {
			return errors.New("tab_id requires profile=session and session_id")
		}
		in.Profile = actionProfileIsolated
		in.SessionID = ""
		in.TabID = ""
		in.UserDataDir = ""
		in.RemoteCDPURL = ""
		return nil
	case actionProfileSession:
		dir, err := t.sessionDir(in.SessionID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(in.TabID) != "" && !validBrowserSessionID(strings.TrimSpace(in.TabID)) {
			return errors.New("tab_id must be 1-80 chars using letters, numbers, dot, underscore, or dash; it cannot start with dot")
		}
		in.Profile = actionProfileSession
		in.UserDataDir = dir
		in.RemoteCDPURL = ""
		return nil
	case actionProfileUserAttached:
		if strings.TrimSpace(in.TabID) != "" {
			return errors.New("tab_id requires profile=session and session_id")
		}
		if !t.AllowUserProfile {
			return errors.New("profile user-attached is disabled (set AGEZT_BROWSER_ACTION_ALLOW_USER_PROFILE=1 and AGEZT_BROWSER_ACTION_USER_DATA_DIR)")
		}
		dir := strings.TrimSpace(t.UserDataDir)
		if dir == "" {
			return errors.New("profile user-attached requires AGEZT_BROWSER_ACTION_USER_DATA_DIR")
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("profile user-attached data dir: %w", err)
		}
		in.Profile = actionProfileUserAttached
		in.SessionID = ""
		in.TabID = ""
		in.UserDataDir = abs
		in.RemoteCDPURL = ""
		return nil
	case actionProfileRemoteCDP:
		if strings.TrimSpace(in.TabID) != "" {
			return errors.New("tab_id requires profile=session and session_id")
		}
		if !t.AllowRemoteCDP {
			return errors.New("profile remote-cdp is disabled (set AGEZT_BROWSER_ACTION_ALLOW_REMOTE_CDP=1 and AGEZT_BROWSER_ACTION_REMOTE_CDP_URL)")
		}
		cdpURL := strings.TrimSpace(t.RemoteCDPURL)
		if cdpURL == "" {
			return errors.New("profile remote-cdp requires AGEZT_BROWSER_ACTION_REMOTE_CDP_URL")
		}
		if err := t.validateCDPURL(ctx, cdpURL); err != nil {
			return err
		}
		in.Profile = actionProfileRemoteCDP
		in.SessionID = ""
		in.TabID = ""
		in.UserDataDir = ""
		in.RemoteCDPURL = cdpURL
		return nil
	default:
		return fmt.Errorf("unknown profile %q (use isolated, session, user-attached, or remote-cdp)", in.Profile)
	}
}

func (t *ActionTool) sessionDir(id string) (string, error) {
	id = strings.TrimSpace(id)
	if !validBrowserSessionID(id) {
		return "", errors.New("session_id must be 1-80 chars using letters, numbers, dot, underscore, or dash; it cannot start with dot")
	}
	root := strings.TrimSpace(t.SessionRoot)
	if root == "" {
		return "", errors.New("profile session requires browser action session root")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("browser action session root: %w", err)
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, id))
	if err != nil {
		return "", fmt.Errorf("browser action session dir: %w", err)
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", errors.New("session_id escapes browser action session root")
	}
	return targetAbs, nil
}
