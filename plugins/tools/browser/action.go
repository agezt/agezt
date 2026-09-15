// SPDX-License-Identifier: MIT
//
// browser.action tool: ActionTool struct + actionArtifactIndexer +
// actionRunSpec + actionRunOutput types + NewAction + setter methods +
// Definition (the JSON schema the model sees). The Default*/Max*/actionProfile*
// constants live here because every other file in the package references them.
// Split from action.go during Day 211 god-file refactor (#38).
// Public API unchanged.
package browser

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
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

	// OnBlock, if set, is called (resolved IP, reason) when the egress guard
	// refuses a dial, so the host can journal it as a netguard.blocked event.
	OnBlock func(ip, reason string)

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

// SetOnBlock installs the egress-guard audit callback (toolreg.NetguardAware).
func (t *ActionTool) SetOnBlock(fn func(ip, reason string)) { t.OnBlock = fn }

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
