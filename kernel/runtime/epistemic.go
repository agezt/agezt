// SPDX-License-Identifier: MIT

package runtime

import (
	"encoding/json"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/policy/epistemic"
)

// The epistemic guard is the PLATFORM's rule (W3.73): whether a tool call
// needs a human because the model cannot see what the journal remembers.
//
// The runtime keeps only the HOST's half of it — the three ports it owns: the
// journal it scans, the clock it measures age against, and the tool map it
// looks definitions up in. The decision belongs to
// kernel/platform/policy/epistemic, so every host asks the same rule instead
// of re-implementing it.
//
// Provenance: kernel/runtime/epistemic.go held the guard and its twelve
//             helpers until W3.73. The rule moved to the platform package;
//             this file became the host's port wiring.
func (k *Kernel) epistemicGate(toolName string, cap edict.Capability, input json.RawMessage, def toolapi.ToolDef, bundle approvalBundle) epistemic.Decision {
	return k.epistemicGateFor().Decide(toolName, string(cap), input, def, bundle.EffectClass, bundle.Confidence)
}

// epistemicGateFor builds the platform's guard over this kernel's ports.
//
// The guard is a struct of three function values, so it is built per call
// rather than held on the kernel: a held gate would need a lock for a value
// that never changes.
func (k *Kernel) epistemicGateFor() *epistemic.Gate {
	return epistemic.New(epistemic.Ports{
		Scan:   k.epistemicScan,
		Now:    time.Now,
		Lookup: k.epistemicLookup,
	})
}

// epistemicScan walks the journal's policy decisions and tool results — the
// history the guard reasons over.
//
// A kernel without a journal has no memory: the walk finds nothing and the
// guard treats every tool as novel, which is the SAFE direction (unknown
// means ask).
func (k *Kernel) epistemicScan(visit func(*event.Event) error) error {
	if k == nil || k.journal == nil {
		return nil
	}
	return k.journal.Range(visit)
}

// epistemicLookup finds a tool's definition by name — the host's tool map.
func (k *Kernel) epistemicLookup(name string) (toolapi.ToolDef, bool) {
	if k == nil {
		return toolapi.ToolDef{}, false
	}
	tool, ok := k.tools[name]
	if !ok {
		return toolapi.ToolDef{}, false
	}
	return tool.Definition(), true
}
