// SPDX-License-Identifier: MIT

package runner

// Provenance: Runner retry + introspection + verification: RunWithRetry +
//             VerifyCompletion + PublishHeuristicBypass + DescribeImages. Code
//             extracted from runner.go during the Day-63 god-file split.
//             Public API unchanged.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/assureapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/resume"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

func (r *Runner) RunWithRetry(ctx context.Context, corr, intent string, pol agentapi.RetryPolicy) (string, error) {
	max := pol.MaxAttempts
	if max <= 1 {
		return r.RunWith(ctx, corr, intent)
	}
	if max > 10 {
		max = 10
	}
	// Resume ticket ownership (M1002): the retry wrapper owns
	// one ticket across all attempts; the inner RunWith calls
	// reuse the corr and skip creation. The deferred finalize
	// reads the final named err, so a shutdown-interrupted
	// retry keeps its ticket while a genuine give-up deletes it.
	var owns bool
	ctx, owns = r.tickets.Claim(ctx, corr, intent, resume.KindRetry, 0)
	var ans string
	var err error
	if owns {
		defer func() { r.tickets.Finalize(corr, err) }()
	}
	var lastErr error
	for attempt := 1; attempt <= max; attempt++ {
		ans, err = r.RunWith(ctx, corr, intent)
		if err == nil {
			return ans, nil
		}
		lastErr = err
		reason := r.k.RetryReason(err)
		if attempt >= max || !r.k.AgentRetryable(reason, pol.RetryOn) {
			return "", err
		}
		delay := r.k.RetryDelay(pol, attempt)
		agentSlug := runctx.AgentSlugFromCtx(ctx)
		subject := "agent.retry"
		if agentSlug != "" {
			subject = "agent." + agentSlug + ".retry"
		}
		_, _ = r.k.Bus().Publish(event.Spec{
			Subject:       subject,
			Kind:          event.KindAgentRetry,
			Actor:         "agent-retry",
			CorrelationID: corr,
			Payload: map[string]any{
				"agent":          agentSlug,
				"attempt":        attempt,
				"next_attempt":   attempt + 1,
				"max_attempts":   max,
				"reason":         reason,
				"error":          err.Error(),
				"delay_ms":       int64(delay / time.Millisecond),
				"backoff":        strings.TrimSpace(pol.Backoff),
				"base_delay_sec": pol.BaseDelaySec,
				"max_delay_sec":  pol.MaxDelaySec,
				"retry_on":       append([]string{}, pol.RetryOn...),
			},
		})
		if delay <= 0 {
			continue
		}
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			return "", ctx.Err()
		case <-t.C:
		}
	}
	return "", lastErr
}

// The JOURNAL's provenance walks are not the engine's (W3.76ad): `Why`,
// `Causes`, `ParentOf` and `Verify` answer over the event log, and the event
// log is the host's own file. The engine used to re-export all four — and the
// daemon copied them again, so one family had THREE spellings. The engine asks
// the host's journal for them now (`r.k.Journal().Why(…)`), and it holds no
// verb of its own for them.

// VerifyCompletion asks the model whether the given ANSWER
// fully accomplishes TASK. Returns an assureapi.Verdict with a
// complete/gap judgement (Day 33). The body was moved from
// kernel/runtime/runexec.go's private verifyCompletion. The
// bounded RunAssured loop and the workboard criteria check
// both consume this through *Kernel.VerifyCompletion.
func (r *Runner) VerifyCompletion(ctx context.Context, corr, task, answer string) (assureapi.Verdict, error) {
	prompt := "You are a strict completion checker. Given a TASK and the ANSWER an agent produced, decide whether the answer FULLY accomplishes the task with nothing important left undone. Be skeptical: a plan or a promise to do it is NOT completion.\n\n" +
		"Reply with ONLY a JSON object and no other text: {\"complete\": true|false, \"gap\": \"<concise description of what is still missing; empty string if complete>\"}.\n\n" +
		"TASK:\n" + task + "\n\nANSWER:\n" + answer
	resp, err := r.k.CompleteAux(ctx, corr, "verify", llm.CompletionRequest{
		Model:     r.k.Model(),
		MaxTokens: assureVerifyMaxTokens,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: prompt}},
	})
	if err != nil {
		return assureapi.Verdict{}, err
	}
	v, ok := assureapi.ParseVerdict(resp.Message.Content)
	if !ok {
		v = assureapi.Verdict{Complete: false, Gap: "verifier reply was not valid JSON"}
	}
	_, _ = r.k.Bus().Publish(event.Spec{
		Subject:       "agent.agent-" + corr + ".assure",
		Kind:          event.KindAssureVerdict,
		Actor:         "assure",
		CorrelationID: corr,
		Payload:       map[string]any{"complete": v.Complete, "gap": v.Gap},
	})
	return v, nil
}

// PublishHeuristicBypass journals a deterministic-look-up hit
// (what-time-is-it / today's-date) so the run's timeline still
// shows the task, the bypass, and the canned answer (Day 33).
// The body was moved from kernel/runtime/runexec.go's private
// publishHeuristicBypass.
func (r *Runner) PublishHeuristicBypass(ctx context.Context, corr, actor, intent, answer string) error {
	subject := func(suffix string) string {
		return "agent." + actor + "." + suffix
	}
	publish := func(kind event.Kind, suffix string, payload any) error {
		_, err := r.k.Bus().Publish(event.Spec{
			Subject:       subject(suffix),
			Kind:          kind,
			Actor:         actor,
			CorrelationID: corr,
			Payload:       payload,
		})
		return err
	}
	if err := publish(event.KindTaskReceived, "task", map[string]any{"intent": intent}); err != nil {
		return fmt.Errorf("runtime: publish heuristic task.received: %w", err)
	}
	if err := publish(event.KindInfo, "heuristic", map[string]any{
		"bypass": "deterministic",
		"reason": "known-safe fast path",
	}); err != nil {
		return fmt.Errorf("runtime: publish heuristic bypass: %w", err)
	}
	if err := publish(event.KindTaskCompleted, "task", map[string]any{
		"iters":   0,
		"chars":   len(answer),
		"stopped": "heuristic_bypass",
		"answer":  truncateHeuristicAnswer(answer),
	}); err != nil {
		return fmt.Errorf("runtime: publish heuristic task.completed: %w", err)
	}
	return nil
}

// ErrNoVisionModel is returned by DescribeImages when no vision-
// capable model is available. This is the one definition;
// runtime.ErrNoVisionModel re-exports this value.
var ErrNoVisionModel = errors.New("runtime: no vision-capable model available")

// DescribeImages runs the vision SIDECAR (M821): it sends the
// images to a keyed vision-capable model and returns a text
// description, so a run whose active model can't see images can
// still "read" them (Day 33). The body was moved from
// kernel/runtime/runexec.go.
func (r *Runner) DescribeImages(ctx context.Context, corr string, images []string, hint string) (string, error) {
	if len(images) == 0 {
		return "", nil
	}
	if r.k.VisionModel() == nil {
		return "", ErrNoVisionModel
	}
	model, ok := r.k.VisionModel()()
	if !ok || model == "" {
		return "", ErrNoVisionModel
	}
	prompt := hint
	if strings.TrimSpace(prompt) == "" {
		prompt = "Describe the attached image(s) in detail and transcribe any visible text. Be thorough and factual."
	}
	resp, err := r.k.CompleteAux(ctx, corr, "vision", llm.CompletionRequest{
		Model:     model,
		MaxTokens: visionDescribeMaxTokens,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: prompt, Images: images}},
	})
	if err != nil {
		return "", err
	}
	_, _ = r.k.Bus().Publish(event.Spec{
		Subject:       "agent.agent-" + corr + ".vision",
		Kind:          event.KindCapabilityRerouted,
		Actor:         "vision",
		CorrelationID: corr,
		Payload: map[string]any{
			"from_model": r.k.Model(),
			"to_model":   model,
			"capability": "vision",
			"images":     len(images),
		},
	})
	return resp.Message.Content, nil
}

// CompleteAgentLifecycle advances the durable lifecycle for a
// successful run (Day 33). Body moved from kernel/runtime/
// runexec.go's private completeAgentLifecycle. The Runner is
// the canonical caller; *Kernel.CompleteAgentLifecycle preserves
// the public surface for external callers (scheduled workflows,
// direct tool targets) that don't go through RunWith.
