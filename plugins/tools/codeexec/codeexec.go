// SPDX-License-Identifier: MIT

// Package codeexec: Tool struct + constants (DefaultTimeout + MaxTimeout +
// MaxOutputBytes + best-effort resource caps) + artifactIndexer interface +
// NewWithWarden + Bind + SetIndex + Languages + Definition + input struct
// (the contract surface). Invoke (the execution router) moved to
// codeexec_invoke.go. Day-211 god-file split. Public API unchanged.
package codeexec


import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/warden"
)
const (
	// DefaultTimeout caps one run when the model omits timeout_ms.
	DefaultTimeout = 120 * time.Second
	// MaxTimeout is the hard ceiling — a model can ask for less, never more.
	MaxTimeout = 600 * time.Second
	// MaxOutputBytes caps captured stdout+stderr (matches warden's own default).
	MaxOutputBytes = 256 * 1024

	// Best-effort resource caps (real teeth only on Linux+ProfileNamespace; a
	// harmless no-op elsewhere). They guard against an accidental runaway pegging
	// the host, not against a determined escape.
	limitCPUSeconds           = 120
	limitAddressSpaceByte     = 2 << 30   // 2 GiB — Go-runtime children need ≥1 GiB; Python/Node fit
	limitMaxOpenFiles         = 512       //
	limitMaxFileSizeBytes     = 256 << 20 // 256 MiB per file
	modalArtifactArchiveBytes = 8 << 20
)

// Tool implements agent.Tool. Construct with New (tests) or NewWithWarden
// (production); Bind wires the bus so each run journals a code.executed event.
type Tool struct {
	// Warden is the isolation engine code runs through. Nil → a no-bus engine.
	Warden warden.Engine
	// SandboxRoot is <baseDir>/sandbox: ephemeral runs land in run-* tempdirs
	// here, named projects in <SandboxRoot>/projects/<slug>.
	SandboxRoot string
	// BaseDir points at the AGEZT home/base dir so profile-specific vault-backed
	// secret file mounts can resolve creds.json.
	BaseDir string
	// Runtimes maps a language id to its resolved interpreter absolute path. Only
	// languages present here can run.
	Runtimes map[string]string
	// NetEnabled is the master network switch (AGEZT_SANDBOX_NO_NET=1 → false);
	// when false, no run gets network regardless of allow_net.
	NetEnabled bool
	// Profile is the isolation profile requested of the warden (default
	// ProfileNamespace, downgraded + journaled where unavailable).
	Profile warden.Profile
	// Now returns wall-clock millis for exported artifact metadata.
	Now func() int64

	bus   *bus.Bus
	index artifactIndexer
}

// artifactIndexer is the slice of *artifact.Index code_exec needs for files a
// script intentionally exports under .agezt-artifacts/.
type artifactIndexer interface {
	PutEntry(meta artifact.Entry, data []byte, createdMs int64) (artifact.Entry, error)
}

// NewWithWarden returns a Tool routed through the supplied warden engine — the
// path the daemon uses so audit events land on the kernel bus.
func NewWithWarden(w warden.Engine, sandboxRoot string, runtimes map[string]string, netEnabled bool) *Tool {
	return &Tool{
		Warden:      w,
		SandboxRoot: sandboxRoot,
		BaseDir:     filepath.Dir(sandboxRoot),
		Runtimes:    runtimes,
		NetEnabled:  netEnabled,
		Profile:     warden.ProfileNamespace,
	}
}

// Bind wires the live bus so each run publishes a code.executed event. Called
// once after the kernel opens.
func (t *Tool) Bind(b *bus.Bus) { t.bus = b }

// SetIndex injects the artifact index so code can export files by writing them
// under .agezt-artifacts/ in the workspace.
func (t *Tool) SetIndex(idx artifactIndexer) { t.index = idx }

// Languages returns the available language ids (sorted) — for the daemon banner.
func (t *Tool) Languages() []string { return sortedLangs(t.Runtimes) }

// Definition implements agent.Tool. The language enum and description reflect
// exactly the runtimes detected on this host.
func (t *Tool) Definition() agent.ToolDef {
	langs := sortedLangs(t.Runtimes)
	enum, _ := json.Marshal(langs)
	netLine := "Network is ON by default"
	if !t.NetEnabled {
		netLine = "Network is DISABLED on this daemon"
	}
	desc := "Write and run code, then read its output. Languages: " + strings.Join(langs, ", ") +
		". Each call runs in its own scratch directory; pass a `project` name to keep a " +
		"persistent directory you can revisit and extend across calls (write more files, re-run). " +
		"Use `files` to drop extra source files alongside the entrypoint, and `packages` to pip-install " +
		"Python dependencies (they persist in a project). Save files you want to keep under `.agezt-artifacts/`; " +
		"they are copied into the artifact store when artifact storage is available. " + netLine +
		". The daemon's secrets are never visible to your code. Good for computation, scraping, " +
		"data processing, and building small programs. Returns combined stdout+stderr (truncated to 256 KiB)."

	schema := `{
  "type": "object",
  "required": ["language", "code"],
  "properties": {
    "language": {"type":"string", "enum": ` + string(enum) + `, "description":"Which runtime to use."},
    "code": {"type":"string", "description":"The program source to run (the entrypoint)."},
    "stdin": {"type":"string", "description":"Optional input; written to stdin.txt in the working dir for your code to read."},
    "project": {"type":"string", "description":"Optional persistent project name. Reuse the same name across calls to keep and extend a working directory."},
    "files": {"type":"object", "description":"Optional extra files to write before running, as {\"relative/name\": \"content\"}."},
    "packages": {"type":"array", "items":{"type":"string"}, "description":"Python only: pip packages to install before running, e.g. [\"requests\",\"beautifulsoup4\"]. In a project they persist across calls. For Deno/JS import npm packages inline instead: import x from \"npm:cheerio\"."},
    "timeout_ms": {"type":"integer", "description":"Per-call timeout in ms (default 120000, max 600000)."},
    "allow_net": {"type":"boolean", "description":"Deno only: grant network (default true). Ignored if the daemon has network disabled."}
  }
}`

	return agent.ToolDef{
		Name:        "code_exec",
		Capability:  agent.ToolCapability{Name: string(edict.CapCodeExec)},
		Description: desc,
		Effect: agent.ToolEffect{
			Class: agent.EffectIrreversible,
			PredictedEffects: []string{
				"write and execute model-provided code in the sandbox workspace",
				"may create project files, install packages, consume compute, and contact the network when enabled",
			},
			AffectedResources: []string{"sandbox root: " + t.SandboxRoot, "available runtimes: " + strings.Join(langs, ", ")},
			RollbackNotes:     "Ephemeral scratch runs are discarded after use. Persistent project files/packages must be deleted from the sandbox project or restored from a clean workspace.",
			Confidence:        0.55,
		},
		InputSchema: json.RawMessage(schema),
	}
}

type input struct {
	Language  string            `json:"language"`
	Code      string            `json:"code"`
	Stdin     string            `json:"stdin,omitempty"`
	Project   string            `json:"project,omitempty"`
	Files     map[string]string `json:"files,omitempty"`
	Packages  []string          `json:"packages,omitempty"`
	TimeoutMS int64             `json:"timeout_ms,omitempty"`
	AllowNet  *bool             `json:"allow_net,omitempty"`
}
