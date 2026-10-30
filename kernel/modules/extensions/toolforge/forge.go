// SPDX-License-Identifier: MIT

package toolforge

// Provenance: the SCRIPT-TOOL FORGE SERVICE (W3.72). The lifecycle over the
//             agent-authored scripts — draft, edit, test, promote, ask the
//             operator, quarantine, remove — moved out of kernel/runtime
//             (the daemon used to hold the lifecycle, the sandbox test and
//             the journaling) into the extensions module, which owns the
//             forge.
//
// The daemon keeps its own names as forwarders, so every caller outside the
// runtime is untouched. What moved is WHERE the answer comes from: the
// forge's OPERATIONS live with the forge's CATALOGUE now.
//
// The rule of the lifecycle is unchanged by the move: only TESTED code goes
// live, any edit demotes the tool back to draft with its test record
// cleared, and the OPERATOR — not the agent — grants a promotion.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/event"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// Service is the extensions module's script-tool forge: the catalogue, the
// lifecycle over it and the bridge that hands the promoted scripts to a run.
//
// It holds the host's PORTS (the catalogue's store, the sandbox runner, the
// operator's approval registry, the publisher) and nothing else of the
// host's. A host that handed over no store answers "no forge" rather than
// calling into nothing.
type Service struct {
	store       toolforgeapi.HostStore
	runner      toolregapi.ScriptRunner
	autoPromote bool
	approve     func(ctx context.Context, spec approvalapi.SubmitSpec) approvalapi.Outcome
	publish     func(spec event.Spec)
}

// NewService builds the forge service over the host's dependencies.
//
// The service satisfies the module's api (extapi.ForgeService); the
// constructor returns the concrete type so the host can hold either.
func NewService(deps extapi.ForgeDependencies) *Service {
	return &Service{
		store:       deps.Store,
		runner:      deps.ScriptRunner,
		autoPromote: deps.AutoPromote,
		approve:     deps.Approve,
		publish:     deps.Publish,
	}
}

// emit publishes one event on the host's bus, when the host has one.
func (s *Service) emit(spec event.Spec) {
	if s.publish == nil {
		return
	}
	s.publish(spec)
}

// Draft validates and persists a new DRAFT script tool, journaling
// scripttool.created so the tool's birth is auditable.
func (s *Service) Draft(corr string, st toolforgeapi.ScriptTool) (toolforgeapi.ScriptTool, error) {
	if s.store == nil {
		return toolforgeapi.ScriptTool{}, toolforgeapi.ErrNoStore
	}
	saved, err := s.store.Add(st)
	if err != nil {
		return toolforgeapi.ScriptTool{}, err
	}
	s.emit(event.Spec{
		Subject: "toolforge." + saved.Name, Kind: event.KindScriptToolCreated, Actor: "toolforge",
		CorrelationID: corr,
		Payload:       map[string]any{"id": saved.ID, "name": saved.Name, "language": saved.Language, "code_bytes": len(saved.Code)},
	})
	return saved, nil
}

// Update edits a script tool's mutable fields via mutate, journaling
// scripttool.updated. The store demotes the tool to draft and clears its test
// record when the code/language changed — the payload says so, so `agt why`
// can explain why a live tool went dark. Returns false + nil error for an
// unknown ref (standing pattern).
func (s *Service) Update(corr, ref string, mutate func(*toolforgeapi.ScriptTool)) (toolforgeapi.ScriptTool, bool, error) {
	if s.store == nil {
		return toolforgeapi.ScriptTool{}, false, toolforgeapi.ErrNoStore
	}
	st, err := s.store.Update(ref, mutate)
	if errors.Is(err, toolforgeapi.ErrNotFound) {
		return toolforgeapi.ScriptTool{}, false, nil
	}
	if err != nil {
		return toolforgeapi.ScriptTool{}, false, err
	}
	s.emit(event.Spec{
		Subject: "toolforge." + st.Name, Kind: event.KindScriptToolUpdated, Actor: "toolforge",
		CorrelationID: corr,
		Payload:       map[string]any{"id": st.ID, "name": st.Name, "status": string(st.Status), "tested_ok": st.TestedOK},
	})
	return st, true, nil
}

// Test runs a script tool's CURRENT code once in the code-exec sandbox with
// input as the call payload (surfaced to the script as ./stdin.txt), records
// the verdict on the tool (promotion requires a pass), and journals
// scripttool.tested. The sandbox output comes back verbatim so the author can
// iterate.
func (s *Service) Test(ctx context.Context, corr, ref, input string) (toolforgeapi.ScriptTool, string, error) {
	if s.runner == nil {
		return toolforgeapi.ScriptTool{}, "", errors.New("toolforge: script tools are not available on this daemon (no sandbox runner)")
	}
	if s.store == nil {
		return toolforgeapi.ScriptTool{}, "", toolforgeapi.ErrNoStore
	}
	st, found := s.store.Get(ref)
	if !found {
		return toolforgeapi.ScriptTool{}, "", toolforgeapi.ErrNotFound
	}
	if strings.TrimSpace(input) == "" {
		input = "{}"
	}
	out, isErr, err := s.runner.RunScript(ctx, st.Language, st.Code, input)
	if err != nil {
		return toolforgeapi.ScriptTool{}, "", err
	}
	ok := !isErr
	rec, rerr := s.store.RecordTest(st.ID, ok)
	if rerr != nil {
		return toolforgeapi.ScriptTool{}, "", rerr
	}
	s.emit(event.Spec{
		Subject: "toolforge." + st.Name, Kind: event.KindScriptToolTested, Actor: "toolforge",
		CorrelationID: corr,
		Payload:       map[string]any{"id": st.ID, "name": st.Name, "ok": ok, "output_bytes": len(out)},
	})
	return rec, out, nil
}

// Promote moves a tested draft/quarantined tool to ACTIVE — from the next run
// on, every agent is offered it as forge_<name>. Journals
// scripttool.promoted.
func (s *Service) Promote(corr, ref string) (toolforgeapi.ScriptTool, error) {
	if s.store == nil {
		return toolforgeapi.ScriptTool{}, toolforgeapi.ErrNoStore
	}
	st, err := s.store.Promote(ref)
	if err != nil {
		return toolforgeapi.ScriptTool{}, err
	}
	s.emit(event.Spec{
		Subject: "toolforge." + st.Name, Kind: event.KindScriptToolPromoted, Actor: "toolforge",
		CorrelationID: corr,
		Payload:       map[string]any{"id": st.ID, "name": st.Name, "language": st.Language},
	})
	return st, nil
}

// RequestPromotion asks the OPERATOR to take the tool at ref live: the
// promotion queue. The request blocks on the HITL approval registry (it
// shows up in `agt approvals` and the console's Approvals view); a grant
// promotes through the exact path the operator CLI uses, a deny/timeout comes
// back as the decision, not an error. The "only tested code goes live"
// invariant is checked up front so an untested draft never even reaches the
// operator's queue.
func (s *Service) RequestPromotion(ctx context.Context, corr, ref string) (toolforgeapi.ScriptTool, approvalapi.Decision, string, error) {
	if s.store == nil {
		return toolforgeapi.ScriptTool{}, "", "", toolforgeapi.ErrNoStore
	}
	st, found := s.store.Get(strings.TrimSpace(ref))
	if !found {
		return toolforgeapi.ScriptTool{}, "", "", fmt.Errorf("toolforge: no script tool %q", ref)
	}
	if st.Status == toolforgeapi.StatusActive {
		return st, "", "", fmt.Errorf("toolforge: %s is already active", st.Name)
	}
	if !st.TestedOK {
		return toolforgeapi.ScriptTool{}, "", "", toolforgeapi.ErrUntested
	}
	if s.autoPromote {
		promoted, err := s.Promote(corr, st.Name)
		if err != nil {
			return st, approvalapi.DecisionGrant, "auto-promote enabled", err
		}
		return promoted, approvalapi.DecisionGrant, "auto-promote enabled", nil
	}
	if s.approve == nil {
		return st, "", "", errors.New("toolforge: promotions need an operator, and this daemon has no approval registry")
	}
	out := s.approve(ctx, approvalapi.SubmitSpec{
		Capability:    "toolforge.promote",
		ToolName:      "toolforge.promote",
		Input:         fmt.Sprintf("promote %s (%s): %s", st.Name, st.Language, st.Description),
		Reason:        "agent requested promotion of forge tool " + st.Name,
		Actor:         "toolforge",
		CorrelationID: corr,
	})
	if out.Decision != approvalapi.DecisionGrant {
		return st, out.Decision, out.Reason, nil // a verdict, not a failure
	}
	promoted, err := s.Promote(corr, st.Name)
	if err != nil {
		return st, out.Decision, out.Reason, err
	}
	return promoted, out.Decision, out.Reason, nil
}

// Quarantine pulls an active tool from production — the kill switch. Journals
// scripttool.quarantined with the operator's reason.
func (s *Service) Quarantine(corr, ref, reason string) (toolforgeapi.ScriptTool, error) {
	if s.store == nil {
		return toolforgeapi.ScriptTool{}, toolforgeapi.ErrNoStore
	}
	st, err := s.store.Quarantine(ref)
	if err != nil {
		return toolforgeapi.ScriptTool{}, err
	}
	payload := map[string]any{"id": st.ID, "name": st.Name}
	if reason = strings.TrimSpace(reason); reason != "" {
		payload["reason"] = reason
	}
	s.emit(event.Spec{
		Subject: "toolforge." + st.Name, Kind: event.KindScriptToolQuarantined, Actor: "toolforge",
		CorrelationID: corr,
		Payload:       payload,
	})
	return st, nil
}

// Remove deletes a script tool, journaling scripttool.removed when it
// existed. Returns whether it existed.
func (s *Service) Remove(corr, ref string) (bool, error) {
	if s.store == nil {
		return false, toolforgeapi.ErrNoStore
	}
	gone, ok, err := s.store.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		s.emit(event.Spec{
			Subject: "toolforge." + gone.Name, Kind: event.KindScriptToolRemoved, Actor: "toolforge",
			CorrelationID: corr,
			Payload:       map[string]any{"id": gone.ID, "name": gone.Name},
		})
	}
	return ok, nil
}
