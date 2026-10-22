// SPDX-License-Identifier: MIT

package reasoning

import (
	"context"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/reasoning/api"
)

// Conductor role labels (also used as event/result discriminators).
const (
	conductorRoleThinker  = "thinker"
	conductorRoleWorker   = "worker"
	conductorRoleVerifier = "verifier"
)

const (
	// conductorDefaultRounds is the default worker↔verifier retry cap: one
	// initial attempt plus one retry. Enough to recover from a single bad draft
	// without burning many model calls.
	conductorDefaultRounds = 2
	// Per-role token bounds — Worker is the largest (it writes the solution).
	conductorThinkerMaxTokens  = 900
	conductorWorkerMaxTokens   = 1600
	conductorVerifierMaxTokens = 600
	conductorPlanMaxTokens     = 700
	// conductorEventTextMax clips text carried in bus events (mirrors the
	// council's councilEventTextMax) so the live Web UI can render a run without
	// re-fetching while keeping the hash-chained journal from bloating.
	conductorEventTextMax = 4000
)

// conductorPublish emits one conductor event on the host's bus. The subject and
// the actor are the conductor's own, so the live Web UI can route the run
// without the host having to know the module's event kinds.
func (s *Service) conductorPublish(corr string, kind event.Kind, payload map[string]any) {
	s.publish(event.Spec{
		Subject:       "conductor." + corr,
		Kind:          kind,
		Actor:         "conductor",
		CorrelationID: corr,
		Payload:       payload,
	})
}

// Conduct runs the Thinker→Worker→Verifier loop on a task. Empty role models are
// filled from the Council's default membership (one model per keyed provider) so
// the roles run on DIFFERENT models out of the box. rounds<=0 uses
// conductorDefaultRounds. A role completion failing is recorded on its step
// (Error) rather than aborting the run; only an empty task or no available
// models errors.
func (s *Service) Conduct(ctx context.Context, corr string, cfg ConductorConfig) (ConductorResult, error) {
	cfg.Task = strings.TrimSpace(cfg.Task)
	if cfg.Task == "" {
		return ConductorResult{}, fmt.Errorf("conductor: task required")
	}
	thinker, worker, verifier, err := s.conductorRoleModels(cfg)
	if err != nil {
		return ConductorResult{}, err
	}
	rounds := cfg.MaxRounds
	if rounds <= 0 {
		rounds = conductorDefaultRounds
	}

	result := ConductorResult{
		Task:  cfg.Task,
		Roles: map[string]string{conductorRoleThinker: thinker, conductorRoleWorker: worker, conductorRoleVerifier: verifier},
	}

	s.conductorPublish(corr, event.KindConductorStarted, map[string]any{
		"task":     api.Clip(cfg.Task, 500),
		"thinker":  thinker,
		"worker":   worker,
		"verifier": verifier,
		"rounds":   rounds,
		"plan":     cfg.Plan,
	})

	// Optional Plan: tailor per-role instructions. Stored on the result so the
	// "what coordination did the conductor choose" decision stays auditable.
	briefs := map[string]string{}
	if cfg.Plan {
		result.Plan = s.conductorPlan(ctx, corr, cfg.Task, thinker, worker, verifier)
		briefs = parseRoleBriefs(result.Plan)
	}

	// Thinker: one decomposition pass.
	thinkStep := s.conductorStep(ctx, corr, 0, conductorRoleThinker, thinker,
		conductorRoleSystem(conductorRoleThinker, briefs[conductorRoleThinker]),
		conductorThinkerPrompt(cfg.Task), conductorThinkerMaxTokens)
	result.Steps = append(result.Steps, thinkStep)
	plan := thinkStep.Text

	// Worker↔Verifier loop.
	var feedback string
	for attempt := 1; attempt <= rounds; attempt++ {
		result.Rounds = attempt
		workStep := s.conductorStep(ctx, corr, attempt, conductorRoleWorker, worker,
			conductorRoleSystem(conductorRoleWorker, briefs[conductorRoleWorker]),
			conductorWorkerPrompt(cfg.Task, plan, feedback), conductorWorkerMaxTokens)
		result.Steps = append(result.Steps, workStep)
		result.Answer = workStep.Text

		verStep := s.conductorVerify(ctx, corr, attempt, verifier, briefs[conductorRoleVerifier], cfg.Task, workStep.Text)
		result.Steps = append(result.Steps, verStep)
		if verStep.Verdict == "pass" {
			result.Passed = true
			break
		}
		feedback = verStep.Reason
		if verStep.Exec != nil && !verStep.Exec.OK && verStep.Exec.Output != "" {
			feedback = strings.TrimSpace(feedback + "\n\nExecution output:\n" + verStep.Exec.Output)
		}
	}

	s.conductorPublish(corr, event.KindConductorDone, map[string]any{
		"passed": result.Passed,
		"rounds": result.Rounds,
		"answer": api.Clip(result.Answer, conductorEventTextMax),
	})
	return result, nil
}
