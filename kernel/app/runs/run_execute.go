// SPDX-License-Identifier: MIT

package runs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

// execute runs the resolved run and streams its events to the caller until it
// finishes, then reports the terminal answer.
func (s *RunService) execute(ctx context.Context, resolved *resolvedRun, emit func(*event.Event) error) (RunOutput, error) {
	k := s.ports.kernel
	corr := resolved.corr
	// The subscription is taken before the run starts, so no event is missed.
	sub, err := k.Bus().Subscribe(k.SubjectForRun(corr), 1024)
	if err != nil {
		return RunOutput{}, err
	}
	defer sub.Cancel()

	// A per-run cost cap only trips if the run accrues priced spend. Journal
	// an advisory tied to this run's correlation so `agt why <run>` shows the
	// guardrail was inert. Best-effort.
	if resolved.maxCost > 0 && !modelPriced(resolved.model) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "governor.budget",
			Kind:          event.KindBudgetCapInert,
			Actor:         "controlplane",
			CorrelationID: corr,
			Payload:       map[string]any{"model": resolved.model, "cap_microcents": resolved.maxCost},
		})
	}

	type runResult struct {
		answer string
		err    error
	}
	resultCh := make(chan runResult, 1)
	go func() {
		answer, err := s.run(ctx, resolved)
		resultCh <- runResult{answer, err}
	}()

	// Cancel-on-disconnect: when the operator's client goes away, cancel the
	// run instead of letting it spend into a closed connection.
	if s.ports.CancelOnDisconnect && s.ports.Disconnect != nil {
		go func(signal <-chan struct{}) {
			<-signal
			k.CancelRun(corr)
		}(s.ports.Disconnect())
	}

	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				return RunOutput{}, errors.New("event subscription closed")
			}
			if err := emit(ev); err != nil {
				return RunOutput{}, err
			}
		case r := <-resultCh:
			// Drain the in-flight events before the terminal answer.
			for {
				select {
				case ev := <-sub.C:
					if ev == nil {
						goto drained
					}
					if err := emit(ev); err != nil {
						return RunOutput{}, err
					}
				default:
					goto drained
				}
			}
		drained:
			if r.err != nil {
				return RunOutput{}, r.err
			}
			return s.answer(resolved, r.answer), nil
		case <-ctx.Done():
			return RunOutput{}, ctx.Err()
		}
	}
}

// run performs the run itself: the remote-agezt delegation, the assured loop,
// the agent's retry policy, or the plain governed run.
func (s *RunService) run(ctx context.Context, resolved *resolvedRun) (string, error) {
	k := s.ports.kernel
	corr := resolved.corr
	if resolved.remote {
		return s.runRemote(ctx, resolved)
	}
	if resolved.assure > 0 {
		ans, _, err := k.RunAssured(ctx, corr, resolved.intent, int(resolved.assure))
		return ans, err
	}
	if resolved.agent != nil && resolved.agent.RetryPolicy != nil && resolved.agent.RetryPolicy.MaxAttempts > 1 {
		ans, err := k.RunWithRetry(ctx, corr, resolved.intent, *resolved.agent.RetryPolicy)
		return ans, err
	}
	return k.RunWith(ctx, corr, resolved.intent)
}

// runRemote delegates the run to a remote-agezt peer through the remote_run
// tool and mirrors the peer's events into this run's stream.
func (s *RunService) runRemote(ctx context.Context, resolved *resolvedRun) (string, error) {
	k := s.ports.kernel
	corr := resolved.corr
	receivedPayload := map[string]any{
		"intent":            resolved.intent,
		"execution_profile": "remote-agezt",
		"remote_tool":       "remote_run",
	}
	if resolved.peer != "" {
		receivedPayload["remote_peer"] = resolved.peer
	}
	if err := PublishRemoteRunEvent(k, corr, event.KindTaskReceived, "task", receivedPayload); err != nil {
		return "", err
	}
	delegatingInfo := map[string]any{
		"profile": "remote-agezt",
		"phase":   "delegating",
		"tool":    "remote_run",
	}
	if resolved.peer != "" {
		delegatingInfo["remote_peer"] = resolved.peer
	}
	if err := PublishRemoteRunEvent(k, corr, event.KindInfo, "execution_profile", delegatingInfo); err != nil {
		return "", err
	}
	payload := map[string]string{"task": resolved.intent}
	if resolved.peer != "" {
		payload["peer"] = resolved.peer
	}
	if resolved.overridden {
		payload["model"] = resolved.model
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		_ = PublishRemoteRunEvent(k, corr, event.KindTaskFailed, "task", map[string]any{
			"error":             err.Error(),
			"reason":            "remote-agezt",
			"execution_profile": "remote-agezt",
		})
		return "", err
	}
	res, err := k.RunTool(ctx, corr, "execution-profile-remote-agezt", "remote_run", raw)
	if err != nil {
		_ = PublishRemoteRunEvent(k, corr, event.KindTaskFailed, "task", map[string]any{
			"error":             err.Error(),
			"reason":            "remote-agezt",
			"execution_profile": "remote-agezt",
		})
		return "", err
	}
	if res.IsError {
		msg := strings.TrimSpace(res.Output)
		if msg == "" {
			msg = "remote-agezt execution profile failed"
		}
		_ = PublishRemoteRunEvent(k, corr, event.KindTaskFailed, "task", map[string]any{
			"error":             msg,
			"reason":            "remote-agezt",
			"execution_profile": "remote-agezt",
		})
		return "", errors.New(msg)
	}
	peerMeta := remotePeerMetadata(res.Output)
	if s.ports.MirrorRemote != nil {
		s.ports.MirrorRemote(ctx, corr, peerMeta)
	}
	completedInfo := map[string]any{
		"profile": "remote-agezt",
		"phase":   "completed",
		"tool":    "remote_run",
		"chars":   len(res.Output),
	}
	addPeerMetadata(completedInfo, peerMeta)
	if err := PublishRemoteRunEvent(k, corr, event.KindInfo, "execution_profile", completedInfo); err != nil {
		return "", err
	}
	completedTask := map[string]any{
		"iters":             0,
		"chars":             len(res.Output),
		"stopped":           "remote-agezt",
		"answer":            remoteAnswerPreview(res.Output),
		"execution_profile": "remote-agezt",
	}
	addPeerMetadata(completedTask, peerMeta)
	if err := PublishRemoteRunEvent(k, corr, event.KindTaskCompleted, "task", completedTask); err != nil {
		return "", err
	}
	return res.Output, nil
}

// answer builds the terminal answer of a plain run and enriches it with the
// run's own accounting.
func (s *RunService) answer(resolved *resolvedRun, answer string) RunOutput {
	out := RunOutput{Answer: &answer, CorrelationID: &resolved.corr}
	if resolved.agent != nil {
		slug := resolved.agent.Slug
		out.Agent = &slug
	}
	if s.ports.Run == nil {
		return out
	}
	entry, ok, err := s.ports.Run(resolved.corr)
	if err != nil || !ok {
		return out
	}
	iters := entry.Iters
	spent := entry.SpentMicrocents
	out.Iters = &iters
	out.SpentMC = &spent
	if entry.Model != "" {
		model := entry.Model
		out.Model = &model
	}
	return out
}

// PublishRemoteRunEvent publishes a remote-agezt lifecycle event under the
// run's own subject, so it reaches the run's event stream.
func PublishRemoteRunEvent(k *runtime.Kernel, corr string, kind event.Kind, suffix string, payload any) error {
	actor := "agent-" + corr
	_, err := k.Bus().Publish(event.Spec{
		Subject:       "agent." + actor + "." + suffix,
		Kind:          kind,
		Actor:         actor,
		CorrelationID: corr,
		Payload:       payload,
	})
	return err
}

// remoteAnswerPreview truncates a peer's answer to the run's event payload.
func remoteAnswerPreview(answer string) string {
	const max = 4096
	runes := []rune(answer)
	if len(runes) <= max {
		return answer
	}
	return string(runes[:max]) + "...[truncated]"
}

// remotePeerMetadata reads the peer's footer line, which names the peer, its
// model and its correlation.
func remotePeerMetadata(answer string) map[string]string {
	lines := strings.Split(strings.TrimSpace(answer), "\n")
	if len(lines) == 0 {
		return nil
	}
	footer := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(footer, "[") || !strings.HasSuffix(footer, "]") {
		return nil
	}
	fields := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(footer, "["), "]"))
	meta := map[string]string{}
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		switch key {
		case "peer":
			meta["remote_peer"] = value
		case "model":
			meta["remote_model"] = value
		case "correlation":
			meta["remote_correlation"] = value
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

// addPeerMetadata copies the peer's metadata into an event payload.
func addPeerMetadata(payload map[string]any, meta map[string]string) {
	for key, value := range meta {
		payload[key] = value
	}
}
