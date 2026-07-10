// SPDX-License-Identifier: MIT

package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	stdhttp "net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/envscrub"
	"github.com/agezt/agezt/kernel/netguard"
)

const (
	// DefaultActionTimeout caps one browser action run.
	DefaultActionTimeout = 30 * time.Second
	// MaxActionTimeout is the hard ceiling for a model-requested run.
	MaxActionTimeout = 120 * time.Second
	// DefaultActionMaxTextChars caps extracted text returned to the model.
	DefaultActionMaxTextChars = 64 * 1024
	// MaxActionDriverOutputBytes caps stdout/stderr captured from the driver.
	MaxActionDriverOutputBytes = 512 * 1024

	actionProfileIsolated     = "isolated"
	actionProfileSession      = "session"
	actionProfileUserAttached = "user-attached"
	actionProfileRemoteCDP    = "remote-cdp"
)

// ActionTool is the governed, first-party browser action wrapper. It runs a
// Playwright driver: each call opens a browser, performs the requested action
// list, extracts text/html/selector text, optionally writes a screenshot, and
// exits. By default every call is isolated; profile=session can carry browser
// state through an AGEZT-managed persistent context directory.
type ActionTool struct {
	// NodePath is the node executable. Empty defaults to "node".
	NodePath string
	// DriverPath is the path to browse.mjs (usually the materialized
	// browser-use bundle's scripts/browse.mjs).
	DriverPath string
	// DriverDir is the working directory for node. Empty uses DriverPath's dir.
	DriverDir string

	AllowedHosts  []string
	AllowAll      bool
	AllowLoopback bool
	AllowPrivate  bool

	AllowUserProfile bool
	UserDataDir      string
	AllowRemoteCDP   bool
	RemoteCDPURL     string
	SessionRoot      string

	Timeout      time.Duration
	MaxTextChars int
	Now          func() int64

	lookupIP func(context.Context, string, string) ([]net.IP, error)
	run      func(context.Context, actionRunSpec) (actionRunOutput, error)
	index    actionArtifactIndexer
}

type actionArtifactIndexer interface {
	PutEntry(meta artifact.Entry, data []byte, createdMs int64) (artifact.Entry, error)
}

type actionRunSpec struct {
	Dir        string
	NodePath   string
	DriverPath string
	Spec       []byte
}

type actionRunOutput struct {
	Stdout string
	Stderr string
}

// NewAction returns a governed browser action tool. A blank driver path keeps
// the tool unavailable so daemon registration can stay opt-in.
func NewAction(nodePath, driverPath string) *ActionTool {
	if strings.TrimSpace(driverPath) == "" {
		return nil
	}
	if strings.TrimSpace(nodePath) == "" {
		nodePath = "node"
	}
	return &ActionTool{
		NodePath:     nodePath,
		DriverPath:   driverPath,
		MaxTextChars: DefaultActionMaxTextChars,
		Now:          func() int64 { return time.Now().UnixMilli() },
		lookupIP:     net.DefaultResolver.LookupIP,
		run:          runActionDriver,
	}
}

// SetIndex injects the artifact index after the kernel opens. When set,
// screenshot/download files produced by the Playwright driver are copied into
// the Files view as durable artifacts.
func (t *ActionTool) SetIndex(idx actionArtifactIndexer) { t.index = idx }

func (t *ActionTool) Definition() agent.ToolDef {
	hosts := strings.Join(t.AllowedHosts, ", ")
	if t.AllowAll {
		hosts = "all hosts allowed by tool config"
	} else if hosts == "" {
		hosts = "none configured"
	}
	return agent.ToolDef{
		Name: "browser.action",
		Description: "Open a page in a real headless browser, run an ordered action list " +
			"(goto, click, fill, type, press, select, check, uncheck, hover, scroll, wait), extract " +
			"text/html/selector text, capture a compact interactive snapshot, record browser events, " +
			"return cookies when explicitly requested, and optionally write screenshot/download files. Use when JavaScript rendering or page " +
			"interaction is needed; use browser.read for simple read-only text fetches. profile=session " +
			"can carry cookies/storage through an AGEZT-managed directory; tab_id can persist the final URL " +
			"for follow-up actions but is not a live browser tab. Hosts must be allowed by tool config.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "anyOf": [{"required":["url"]}, {"required":["tab_id"]}],
  "properties": {
    "url": {"type":"string", "description":"Initial absolute http/https URL. Required unless tab_id resolves to a saved AGEZT session tab URL."},
    "actions": {"type":"array", "items":{"type":"object", "properties":{
      "type": {"type":"string", "enum":["goto","click","fill","type","press","select","check","uncheck","hover","scroll","wait"]},
      "url": {"type":"string", "description":"For goto: absolute http/https URL."},
      "selector": {"type":"string", "description":"CSS or Playwright text selector for click/fill/type/press/select/check/uncheck/hover/wait/scroll."},
      "value": {"type":"string", "description":"For fill/type/select: value to enter or select."},
      "key": {"type":"string", "description":"For press: key name, defaults to Enter in the driver."},
      "ms": {"type":"integer", "description":"For wait: milliseconds when selector is omitted."},
      "x": {"type":"integer", "description":"For scroll: horizontal wheel delta."},
      "y": {"type":"integer", "description":"For scroll: vertical wheel delta."},
      "delay_ms": {"type":"integer", "description":"For type: per-character delay in ms."}
    }}},
    "screenshot": {"type":"boolean", "description":"Whether the driver should write a PNG screenshot. Default true."},
    "full_page": {"type":"boolean", "description":"Whether screenshot should capture the full page. Default false."},
    "snapshot": {"type":"boolean", "description":"Whether to return compact refs/selectors for visible interactive elements. Default true."},
    "snapshot_limit": {"type":"integer", "description":"Maximum snapshot elements returned; default 60, max 200."},
    "events": {"type":"boolean", "description":"Whether to return console/page/network event summaries. Default true."},
    "event_limit": {"type":"integer", "description":"Maximum events per event bucket; default 50, max 200."},
    "downloads": {"type":"boolean", "description":"Whether to accept and save page downloads. Default true."},
    "cookies": {"type":"boolean", "description":"Whether to return final-page browser cookies. Default false because cookie values are sensitive."},
    "profile": {"type":"string", "enum":["isolated","session","user-attached","remote-cdp"], "description":"Browser profile policy. Default isolated. session uses an AGEZT-managed persistent browser session directory; user-attached and remote-cdp require operator env opt-in."},
    "session_id": {"type":"string", "description":"For profile=session: AGEZT-managed browser session id for cookie/session carryover."},
    "tab_id": {"type":"string", "description":"For profile=session: persistent AGEZT tab ref. A call with url stores/updates it; later calls may omit url and reuse the saved final URL. This is not a live browser tab."},
    "viewport": {"type":"object", "properties":{
      "width": {"type":"integer", "description":"Viewport width; default 1280."},
      "height": {"type":"integer", "description":"Viewport height; default 720."}
    }},
    "extract": {"type":"string", "description":"text, html, or a selector whose matched visible text should be returned."},
    "timeout_ms": {"type":"integer", "description":"Per-call browser timeout in ms; default 30000, max 120000."},
    "max_chars": {"type":"integer", "description":"Maximum returned text chars; default 65536."}
  }
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectIrreversible,
			PredictedEffects: []string{
				"launch a headless browser process and navigate to an allowed HTTP(S) page",
				"perform user-like page actions that may trigger remote-side effects",
				"optionally write local screenshot and download files from the page state",
			},
			AffectedResources: []string{"allowed browser.action hosts: " + hosts, "driver: " + t.DriverPath, "local artifact store when available"},
			RollbackNotes:     "Local screenshots/download artifacts can be deleted. Remote browser actions cannot be generically rolled back; compensate in the target service when possible.",
			Confidence:        0.55,
		},
	}
}

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

type actionTabState struct {
	URL       string              `json:"url"`
	UpdatedMS int64               `json:"updated_ms"`
	Snapshot  []actionSnapshotRef `json:"snapshot,omitempty"`
}

type actionSnapshotRef struct {
	Ref      string `json:"ref"`
	Selector string `json:"selector"`
	Role     string `json:"role,omitempty"`
	Name     string `json:"name,omitempty"`
}

func (t *ActionTool) resolveTabURL(in *actionInput) error {
	if strings.TrimSpace(in.TabID) == "" {
		return nil
	}
	if in.Profile != actionProfileSession {
		return errors.New("tab_id requires profile=session and session_id")
	}
	if strings.TrimSpace(in.URL) != "" {
		return nil
	}
	st, err := t.readTabState(in.SessionID, in.TabID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(st.URL) == "" {
		return fmt.Errorf("tab_id %q has empty saved URL; call browser.open with url to refresh it", strings.TrimSpace(in.TabID))
	}
	in.URL = strings.TrimSpace(st.URL)
	return nil
}

func (t *ActionTool) ResolveTabRef(sessionID, tabID, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("ref required")
	}
	st, err := t.readTabState(sessionID, tabID)
	if err != nil {
		return "", err
	}
	for _, item := range st.Snapshot {
		if item.Ref == ref && strings.TrimSpace(item.Selector) != "" {
			return item.Selector, nil
		}
	}
	return "", fmt.Errorf("ref %q not found for tab_id %q in session_id %q; call browser.snapshot with the same session_id/tab_id and use a fresh ref", ref, strings.TrimSpace(tabID), strings.TrimSpace(sessionID))
}

func (t *ActionTool) readTabState(sessionID, tabID string) (actionTabState, error) {
	p, err := t.tabStatePath(sessionID, tabID)
	if err != nil {
		return actionTabState{}, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return actionTabState{}, fmt.Errorf("tab_id %q has no saved URL in session_id %q; call browser.open with url, profile=session, session_id, and tab_id first", strings.TrimSpace(tabID), strings.TrimSpace(sessionID))
	}
	if err != nil {
		return actionTabState{}, fmt.Errorf("read browser tab state %q: %w", strings.TrimSpace(tabID), err)
	}
	var st actionTabState
	if err := json.Unmarshal(data, &st); err != nil {
		return actionTabState{}, fmt.Errorf("read browser tab state %q: %w", strings.TrimSpace(tabID), err)
	}
	return st, nil
}

func (t *ActionTool) finalizeTabOutput(normalized string, in actionInput, finalURL string) string {
	tabID := strings.TrimSpace(in.TabID)
	if tabID == "" {
		return normalized
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(normalized), &out); err != nil {
		return normalized
	}
	if strings.TrimSpace(finalURL) == "" {
		finalURL = strings.TrimSpace(in.URL)
	}
	out["session_id"] = strings.TrimSpace(in.SessionID)
	out["tab_id"] = tabID
	out["tab_ref"] = map[string]any{
		"session_id": strings.TrimSpace(in.SessionID),
		"tab_id":     tabID,
		"url":        finalURL,
		"live":       false,
		"refs":       len(extractSnapshotRefs(out)),
	}
	if err := t.saveTabState(in.SessionID, tabID, finalURL, extractSnapshotRefs(out)); err != nil {
		out["tab_state_error"] = err.Error()
	}
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return normalized
	}
	return string(enc)
}

func (t *ActionTool) saveTabState(sessionID, tabID, finalURL string, refs []actionSnapshotRef) error {
	finalURL = strings.TrimSpace(finalURL)
	if finalURL == "" {
		return errors.New("empty final URL")
	}
	u, err := url.Parse(finalURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid final URL for tab state: %q", finalURL)
	}
	p, err := t.tabStatePath(sessionID, tabID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("create browser tab state dir: %w", err)
	}
	now := time.Now().UnixMilli()
	if t.Now != nil {
		now = t.Now()
	}
	data, err := json.MarshalIndent(actionTabState{URL: finalURL, UpdatedMS: now, Snapshot: refs}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write browser tab state %q: %w", strings.TrimSpace(tabID), err)
	}
	return nil
}

func extractSnapshotRefs(out map[string]any) []actionSnapshotRef {
	rows, _ := out["snapshot"].([]any)
	if len(rows) == 0 {
		return nil
	}
	refs := make([]actionSnapshotRef, 0, len(rows))
	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ref, _ := row["ref"].(string)
		selector, _ := row["selector"].(string)
		ref = strings.TrimSpace(ref)
		selector = strings.TrimSpace(selector)
		if ref == "" || selector == "" {
			continue
		}
		role, _ := row["role"].(string)
		name, _ := row["name"].(string)
		refs = append(refs, actionSnapshotRef{Ref: ref, Selector: selector, Role: strings.TrimSpace(role), Name: strings.TrimSpace(name)})
	}
	return refs
}

func (t *ActionTool) tabStatePath(sessionID, tabID string) (string, error) {
	tabID = strings.TrimSpace(tabID)
	if !validBrowserSessionID(tabID) {
		return "", errors.New("tab_id must be 1-80 chars using letters, numbers, dot, underscore, or dash; it cannot start with dot")
	}
	dir, err := t.sessionDir(sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".agezt-tabs", tabID+".json"), nil
}

func validBrowserSessionID(id string) bool {
	if id == "" || len(id) > 80 || strings.HasPrefix(id, ".") {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func (t *ActionTool) CloseSession(id string) (string, error) {
	id = strings.TrimSpace(id)
	dir, err := t.sessionDir(id)
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("close browser session %q: %w", id, err)
	}
	out, err := json.MarshalIndent(map[string]any{
		"closed":     true,
		"session_id": id,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *ActionTool) CloseTab(sessionID, tabID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	tabID = strings.TrimSpace(tabID)
	p, err := t.tabStatePath(sessionID, tabID)
	if err != nil {
		return "", err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("close browser tab %q in session %q: %w", tabID, sessionID, err)
	}
	out, err := json.MarshalIndent(map[string]any{
		"closed":     true,
		"session_id": sessionID,
		"tab_id":     tabID,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *ActionTool) ListTabs(sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	dir, err := t.sessionDir(sessionID)
	if err != nil {
		return "", err
	}
	tabDir := filepath.Join(dir, ".agezt-tabs")
	entries, err := os.ReadDir(tabDir)
	if errors.Is(err, os.ErrNotExist) {
		entries = nil
	} else if err != nil {
		return "", fmt.Errorf("list browser tabs for session %q: %w", sessionID, err)
	}
	tabs := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		tabID := strings.TrimSuffix(entry.Name(), ".json")
		if !validBrowserSessionID(tabID) {
			continue
		}
		st, err := t.readTabState(sessionID, tabID)
		if err != nil {
			tabs = append(tabs, map[string]any{
				"tab_id": tabID,
				"error":  err.Error(),
				"live":   false,
			})
			continue
		}
		tabs = append(tabs, map[string]any{
			"tab_id":     tabID,
			"url":        st.URL,
			"updated_ms": st.UpdatedMS,
			"refs":       len(st.Snapshot),
			"live":       false,
		})
	}
	out, err := json.MarshalIndent(map[string]any{
		"session_id": sessionID,
		"tabs":       tabs,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *ActionTool) validateCDPURL(ctx context.Context, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid remote-cdp url: %w", err)
	}
	switch u.Scheme {
	case "http", "https", "ws", "wss":
	default:
		return fmt.Errorf("remote-cdp url scheme %q not allowed (only http/https/ws/wss)", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("remote-cdp url missing host")
	}
	if err := t.validateHostEgress(ctx, u.Hostname()); err != nil {
		return fmt.Errorf("remote-cdp %w", err)
	}
	return nil
}

func (t *ActionTool) validateURL(ctx context.Context, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme %q not allowed (only http/https)", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("url missing host")
	}
	if !t.AllowAll && !hostAllowed(u.Host, t.AllowedHosts) {
		return fmt.Errorf("%w: %s", ErrHostDenied, u.Hostname())
	}
	return t.validateHostEgress(ctx, u.Hostname())
}

func (t *ActionTool) validateHostEgress(ctx context.Context, host string) error {
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		lookup := t.lookupIP
		if lookup == nil {
			lookup = net.DefaultResolver.LookupIP
		}
		var err error
		ips, err = lookup(ctx, "ip", host)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", host, err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("resolve %s: no addresses", host)
		}
	}
	var opts []netguard.Option
	if t.AllowLoopback {
		opts = append(opts, netguard.AllowLoopback())
	}
	if t.AllowPrivate {
		opts = append(opts, netguard.AllowPrivate())
	}
	g := netguard.New(opts...)
	for _, ip := range ips {
		if ok, reason := g.Allowed(ip); !ok {
			return fmt.Errorf("egress blocked: %s resolves to %s (%s)", host, ip, reason)
		}
	}
	return nil
}

func runActionDriver(ctx context.Context, spec actionRunSpec) (actionRunOutput, error) {
	cmd := exec.CommandContext(ctx, spec.NodePath, spec.DriverPath)
	cmd.Dir = spec.Dir
	cmd.Env = envscrub.Scrubbed()
	cmd.Stdin = bytes.NewReader(spec.Spec)
	var stdout, stderr limitedBuffer
	stdout.max = MaxActionDriverOutputBytes
	stderr.max = MaxActionDriverOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return actionRunOutput{Stdout: stdout.String(), Stderr: stderr.String()}, err
}

type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 || b.Len() >= b.max {
		return len(p), nil
	}
	keep := b.max - b.Len()
	if len(p) > keep {
		_, _ = b.Buffer.Write(p[:keep])
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

func normalizeActionOutput(stdout string, maxTextChars int) (string, string, error) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return "", "", errors.New("empty stdout")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		return "", "", err
	}
	if ok, _ := out["ok"].(bool); !ok {
		if msg, _ := out["error"].(string); msg != "" {
			return "", "", errors.New(msg)
		}
		return "", "", errors.New("driver reported ok=false")
	}
	if text, _ := out["text"].(string); text != "" && maxTextChars > 0 && len(text) > maxTextChars {
		out["text"] = truncateUTF8(text, maxTextChars) + "\n\n...[truncated]"
		out["truncated_text"] = true
	}
	source, _ := out["url"].(string)
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", "", err
	}
	return string(enc), source, nil
}

func (t *ActionTool) attachArtifacts(normalized string) string {
	if t == nil || t.index == nil {
		return normalized
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(normalized), &out); err != nil {
		return normalized
	}
	var artifacts []map[string]any
	if screenshot, _ := out["screenshot"].(string); strings.TrimSpace(screenshot) != "" {
		if e, err := t.saveActionArtifact(screenshot, "image", "browser-action-screenshot.png", "browser.action screenshot"); err == nil {
			rec := actionArtifactRecord(e)
			out["screenshot_artifact"] = rec
			artifacts = append(artifacts, rec)
		} else {
			out["screenshot_artifact_error"] = err.Error()
		}
	}
	if downloads, _ := out["downloads"].([]any); len(downloads) > 0 {
		for _, item := range downloads {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			p, _ := row["path"].(string)
			if strings.TrimSpace(p) == "" {
				continue
			}
			name, _ := row["suggested_filename"].(string)
			if e, err := t.saveActionArtifact(p, "", name, "browser.action download"); err == nil {
				rec := actionArtifactRecord(e)
				row["artifact"] = rec
				artifacts = append(artifacts, rec)
			} else {
				row["artifact_error"] = err.Error()
			}
		}
	}
	if len(artifacts) > 0 {
		out["artifacts"] = artifacts
	}
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return normalized
	}
	return string(enc)
}

func (t *ActionTool) saveActionArtifact(path, forcedKind, name, caption string) (artifact.Entry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return artifact.Entry{}, errors.New("empty path")
	}
	if !isBrowserActionTempPath(path) {
		return artifact.Entry{}, fmt.Errorf("refusing to save browser output outside browseruse temp dir: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return artifact.Entry{}, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return artifact.Entry{}, fmt.Errorf("read %s: empty file", path)
	}
	cleanName := filepath.Base(strings.TrimSpace(name))
	if cleanName == "." || cleanName == string(filepath.Separator) || cleanName == "" {
		cleanName = filepath.Base(path)
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(cleanName)))
	if mimeType == "" {
		mimeType = stdhttp.DetectContentType(data)
	}
	kind := forcedKind
	if kind == "" {
		if strings.HasPrefix(mimeType, "image/") {
			kind = "image"
		} else {
			kind = "download"
		}
	}
	now := time.Now().UnixMilli()
	if t.Now != nil {
		now = t.Now()
	}
	return t.index.PutEntry(artifact.Entry{
		Kind:    kind,
		Source:  "browser.action",
		Name:    cleanName,
		Mime:    mimeType,
		Caption: caption,
	}, data, now)
}

func actionArtifactRecord(e artifact.Entry) map[string]any {
	return map[string]any{
		"id":   e.ID,
		"ref":  e.Ref,
		"name": e.Name,
		"mime": e.Mime,
		"kind": e.Kind,
		"size": e.Size,
	}
}

func isBrowserActionTempPath(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	temp, err := filepath.Abs(os.TempDir())
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(temp, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return len(parts) > 1 && strings.HasPrefix(parts[0], "browseruse-")
}

func truncateUTF8(s string, max int) string {
	return strutil.Ellipsis(s, max, "")
}

func truncateActionOutput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 4096 {
		return s
	}
	return truncateUTF8(s, 4096) + "\n...[truncated]"
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: msg, IsError: true}
}

// ResolveActionDriverPath finds the bundled browser-use Playwright driver when
// running from a source checkout. Packaged installs should pass an explicit
// AGEZT_BROWSER_ACTION_DRIVER path after materializing the browser-use bundle.
func ResolveActionDriverPath() string {
	candidates := []string{
		filepath.Join("plugins", "builtinskills", "browseruse", "scripts", "browse.mjs"),
		filepath.Join("..", "..", "plugins", "builtinskills", "browseruse", "scripts", "browse.mjs"),
	}
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, "plugins", "builtinskills", "browseruse", "scripts", "browse.mjs"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(base, "plugins", "builtinskills", "browseruse", "scripts", "browse.mjs"),
			filepath.Join(base, "..", "plugins", "builtinskills", "browseruse", "scripts", "browse.mjs"),
		)
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if st, err := os.Stat(abs); err == nil && !st.IsDir() {
			return abs
		}
	}
	return ""
}
