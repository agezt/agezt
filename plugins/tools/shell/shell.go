// SPDX-License-Identifier: MIT

// Package shell: Tool struct + constants + Definition + Name + NewWithWarden
// + shellInput. Invoke (the execution router) moved to shell_invoke.go;
// renderResult + ShellHint + resolveShell moved to shell_render.go.
// Day-211 god-file split. Public API unchanged.
package shell


import (
	"encoding/json"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/warden"
)

// DefaultTimeout caps a single command's wall time when the model
// omits timeout_ms and the Tool has no explicit Timeout set.
const DefaultTimeout = 30 * time.Second

// MaxOutputBytes truncates command output so a runaway command does not
// blow the journal/context budget. 64 KiB is the model-facing budget;
// Warden's own cap is 256 KiB (SPEC-02 §5) and we tighten it here. Warden
// caps each stream at this size; renderResult enforces it on the combined
// output.
const MaxOutputBytes = 64 * 1024

// Tool is the in-process shell tool implementation of agent.Tool.
type Tool struct {
	// Warden is the isolation engine commands run through. If nil, a
	// process-default engine (warden.New(nil)) is used — events go
	// nowhere in that case, suitable for unit tests.
	Warden warden.Engine
	// Shell overrides the default shell binary (mainly for tests). Empty
	// means use the platform default.
	Shell string
	// ShellArg is the flag passed before the command (default "/C" on
	// Windows, "-c" elsewhere).
	ShellArg string
	// Timeout overrides DefaultTimeout when > 0.
	Timeout time.Duration
	// Profile is the isolation profile the shell tool requests of
	// Warden. Defaults to ProfileNamespace (shell is the canonical
	// "needs isolation" tool per SPEC-06 §2). On non-Linux this
	// downgrades to ProfileNone with a journal event.
	Profile warden.Profile
	// WorkDir is the working directory commands run in. Empty inherits the
	// daemon's process CWD. The daemon sets this to the file tool's workspace
	// root so the shell and file tools agree on what "here" is — otherwise an
	// agent's `dir`/`ls` (shell, daemon CWD) and `file read x` (file tool,
	// workspace root) see different directories, which is deeply confusing
	// (M609).
	WorkDir string
	// BaseDir points at the AGEZT home/base dir so profile-specific vault-backed
	// secret file mounts can resolve creds.json. Empty disables that feature
	// unless no mounts are configured.
	BaseDir string
}

// NewWithWarden returns a Tool that routes through the supplied Warden
// engine — the path the daemon uses.
func NewWithWarden(w warden.Engine) *Tool {
	return &Tool{Warden: w, Profile: warden.ProfileNamespace}
}

// Name returns the tool's canonical name.
func (t *Tool) Name() string { return "shell" }

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	workDir := t.WorkDir
	if workDir == "" {
		workDir = "process working directory"
	}
	return agent.ToolDef{
		Name:       "shell",
		Capability: agent.ToolCapability{Name: string(edict.CapShell)},
		Description: "Run a command in the operating system's default shell. " +
			"Returns combined stdout+stderr. Output is truncated to 64 KiB.",
		Effect: agent.ToolEffect{
			Class: agent.EffectIrreversible,
			PredictedEffects: []string{
				"execute an operating-system command in the configured working directory",
				"may read, write, start processes, or contact the network depending on the command",
			},
			AffectedResources: []string{"host shell", "working directory: " + workDir},
			RollbackNotes:     "No reliable generic rollback exists for arbitrary shell commands; require command-specific rollback or restore from backups/version control.",
			Confidence:        0.45,
		},
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["command"],
  "properties": {
    "command": {"type": "string", "description": "The shell command to run (a single line; use && or ; to chain)."},
    "timeout_ms": {"type": "integer", "description": "Per-call timeout in milliseconds. Default 30000."}
  }
}`),
	}
}

type shellInput struct {
	Command   string `json:"command"`
	TimeoutMS int64  `json:"timeout_ms,omitempty"`
}
