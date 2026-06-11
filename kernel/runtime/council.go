// SPDX-License-Identifier: MIT

package runtime

// Council of Elders (M837): a panel of differently-modelled advisors deliberates
// a question and converges to a consensus. It is the multi-model decision surface
// — "ask several strong models and reconcile" — usable by any agent (via the
// `council` tool) or the operator (Web UI). Built on the same one-shot Governor
// completion the vision sidecar uses (DescribeImages): each member is a Complete
// routed to a specific model via per-request model routing; the council just
// orchestrates the rounds and a final synthesis.

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// CouncilMember is one seat: a human-readable label and the model id it speaks
// with (a bare model id the Governor routes to its serving provider).
type CouncilMember struct {
	Seat  string `json:"seat"`
	Model string `json:"model"`
}

// Opinion is one member's position in one round.
type Opinion struct {
	Seat  string `json:"seat"`
	Model string `json:"model"`
	Round int    `json:"round"`
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

// CouncilResult is the outcome: every recorded opinion, plus the chair's
// synthesized consensus (and any noted dissent).
type CouncilResult struct {
	Question  string          `json:"question"`
	Members   []CouncilMember `json:"members"`
	Rounds    int             `json:"rounds"`
	Opinions  []Opinion       `json:"opinions"`
	Consensus string          `json:"consensus"`
	Dissent   string          `json:"dissent,omitempty"`
}

const (
	// councilDefaultRounds is one deliberation round (members see peers once) on
	// top of the independent opening round — enough to converge without burning
	// many model calls.
	councilDefaultRounds = 1
	// councilOpinionMaxTokens bounds each member's turn; councilConsensusMaxTokens
	// the chair's synthesis. Keeps a council affordable.
	councilOpinionMaxTokens   = 900
	councilConsensusMaxTokens = 1200
)

// Council convenes the panel on a question and returns the deliberation + the
// chair's consensus. When members is nil/empty the daemon-configured default
// membership is used (cfg.CouncilMembers). rounds<=0 uses councilDefaultRounds.
// One member failing is recorded as a dissent-less opinion error, not a council
// failure; only an empty panel or a total wipe-out errors.
func (k *Kernel) Council(ctx context.Context, corr, question string, members []CouncilMember, rounds int) (CouncilResult, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return CouncilResult{}, fmt.Errorf("council: question required")
	}
	if len(members) == 0 {
		if k.cfg.CouncilMembers != nil {
			members = k.cfg.CouncilMembers()
		}
	}
	members = dedupeSeats(members)
	if len(members) == 0 {
		return CouncilResult{}, fmt.Errorf("council: no members available (no keyed providers?)")
	}
	if rounds <= 0 {
		rounds = councilDefaultRounds
	}

	k.councilPublish(corr, event.KindCouncilConvened, map[string]any{
		"question": clip(question, 500),
		"members":  memberModels(members),
		"rounds":   rounds,
	})

	result := CouncilResult{Question: question, Members: members, Rounds: rounds}
	// latest[i] is member i's most recent opinion text (for peer context).
	latest := make([]string, len(members))

	// Round 0: independent opening positions.
	round0 := k.councilRound(ctx, corr, members, 0, func(_ int) string {
		return councilOpeningPrompt(question)
	})
	for i, op := range round0 {
		latest[i] = op.Text
		result.Opinions = append(result.Opinions, op)
	}

	// Deliberation rounds: each member sees the others' latest positions.
	for r := 1; r <= rounds; r++ {
		snapshot := append([]string(nil), latest...)
		opinions := k.councilRound(ctx, corr, members, r, func(i int) string {
			return councilDeliberatePrompt(question, members, snapshot, i)
		})
		for i, op := range opinions {
			if strings.TrimSpace(op.Text) != "" {
				latest[i] = op.Text
			}
			result.Opinions = append(result.Opinions, op)
		}
	}

	// Consensus: the chair (first member) synthesizes the final positions.
	consensus, dissent := k.councilSynthesize(ctx, corr, question, members, latest)
	result.Consensus = consensus
	result.Dissent = dissent
	k.councilPublish(corr, event.KindCouncilConsensus, map[string]any{
		"chars":       len(consensus),
		"has_dissent": dissent != "",
	})
	return result, nil
}

// councilRound runs one round across all members CONCURRENTLY (provider calls are
// slow; the panel shouldn't be serial), returning opinions in member order.
// promptFor(i) builds member i's user prompt. A member call that errors yields an
// Opinion carrying the error rather than aborting the round.
func (k *Kernel) councilRound(ctx context.Context, corr string, members []CouncilMember, round int, promptFor func(i int) string) []Opinion {
	out := make([]Opinion, len(members))
	var wg sync.WaitGroup
	for i, m := range members {
		wg.Add(1)
		go func(i int, m CouncilMember) {
			defer wg.Done()
			op := Opinion{Seat: m.Seat, Model: m.Model, Round: round}
			resp, err := k.cfg.Provider.Complete(ctx, agent.CompletionRequest{
				Model:         m.Model,
				CorrelationID: corr,
				TaskType:      "council",
				MaxTokens:     councilOpinionMaxTokens,
				System:        councilSeatSystem(m.Seat),
				Messages:      []agent.Message{{Role: agent.RoleUser, Content: promptFor(i)}},
			})
			if err != nil {
				op.Error = err.Error()
			} else {
				op.Text = strings.TrimSpace(resp.Message.Content)
			}
			out[i] = op
		}(i, m)
	}
	wg.Wait()
	for _, op := range out {
		k.councilPublish(corr, event.KindCouncilOpinion, map[string]any{
			"seat": op.Seat, "model": op.Model, "round": op.Round,
			"chars": len(op.Text), "error": op.Error != "",
		})
	}
	return out
}

// councilSynthesize asks the chair (members[0]) to fold the final positions into
// a consensus, then splits an optional "DISSENT:" tail.
func (k *Kernel) councilSynthesize(ctx context.Context, corr, question string, members []CouncilMember, finals []string) (consensus, dissent string) {
	chair := members[0]
	var b strings.Builder
	fmt.Fprintf(&b, "Question:\n%s\n\nThe council members' FINAL positions:\n\n", question)
	for i, m := range members {
		fmt.Fprintf(&b, "[%s]\n%s\n\n", m.Seat, strings.TrimSpace(orPlaceholder(finals[i])))
	}
	b.WriteString("As the chair, synthesize the council's decision. Write two sections:\n")
	b.WriteString("CONSENSUS: the council's agreed answer to the question, decisive and actionable.\n")
	b.WriteString("DISSENT: any notable disagreement worth recording, or write \"none\".")

	resp, err := k.cfg.Provider.Complete(ctx, agent.CompletionRequest{
		Model:         chair.Model,
		CorrelationID: corr,
		TaskType:      "council",
		MaxTokens:     councilConsensusMaxTokens,
		System:        "You are the chair of a council of expert advisors. Synthesize the members' positions into a single decisive consensus, fairly noting genuine dissent. Do not invent agreement that isn't there.",
		Messages:      []agent.Message{{Role: agent.RoleUser, Content: b.String()}},
	})
	if err != nil {
		// Fall back to the chair's own final position so the council still answers.
		return strings.TrimSpace(orPlaceholder(finals[0])), ""
	}
	return splitConsensusDissent(resp.Message.Content)
}

func (k *Kernel) councilPublish(corr string, kind event.Kind, payload map[string]any) {
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "council." + corr,
		Kind:          kind,
		Actor:         "council",
		CorrelationID: corr,
		Payload:       payload,
	})
}

// --- prompt builders ---

func councilSeatSystem(seat string) string {
	return fmt.Sprintf("You are %s, a member of a council of expert advisors convened to settle a question. "+
		"Speak in your own voice, be honest and specific, and reason from first principles. Be concise.", seat)
}

func councilOpeningPrompt(question string) string {
	return "The question before the council:\n\n" + question + "\n\nGive your own reasoned position."
}

func councilDeliberatePrompt(question string, members []CouncilMember, latest []string, self int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The question before the council:\n\n%s\n\nThe other members' current positions:\n\n", question)
	for i, m := range members {
		if i == self {
			continue
		}
		fmt.Fprintf(&b, "[%s]\n%s\n\n", m.Seat, strings.TrimSpace(orPlaceholder(latest[i])))
	}
	b.WriteString("Reconsider in light of these. If you agree, say so and add anything missing; if you disagree, explain why. Give your updated position.")
	return b.String()
}

// --- helpers ---

// splitConsensusDissent parses the chair's "CONSENSUS:/DISSENT:" sections; absent
// the markers the whole text is the consensus. A dissent of "none" is dropped.
func splitConsensusDissent(text string) (consensus, dissent string) {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	di := strings.LastIndex(lower, "dissent:")
	if di >= 0 {
		consensus = strings.TrimSpace(text[:di])
		dissent = strings.TrimSpace(text[di+len("dissent:"):])
	} else {
		consensus = text
	}
	// Strip a leading "CONSENSUS:" label if present.
	if i := strings.Index(strings.ToLower(consensus), "consensus:"); i >= 0 {
		consensus = strings.TrimSpace(consensus[i+len("consensus:"):])
	}
	if d := strings.ToLower(strings.TrimSpace(dissent)); d == "none" || d == "none." || d == "" {
		dissent = ""
	}
	return consensus, dissent
}

func dedupeSeats(members []CouncilMember) []CouncilMember {
	out := make([]CouncilMember, 0, len(members))
	seen := map[string]bool{}
	for i, m := range members {
		m.Seat = strings.TrimSpace(m.Seat)
		m.Model = strings.TrimSpace(m.Model)
		if m.Model == "" {
			continue
		}
		if m.Seat == "" {
			m.Seat = fmt.Sprintf("Elder %d", i+1)
		}
		key := m.Seat + "\x00" + m.Model
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, m)
	}
	return out
}

func memberModels(members []CouncilMember) []string {
	out := make([]string, len(members))
	for i, m := range members {
		out[i] = m.Model
	}
	return out
}

func orPlaceholder(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(no answer)"
	}
	return s
}

// clip bounds a string for a journal payload (rune-safe).
func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
