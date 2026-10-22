// SPDX-License-Identifier: MIT

package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/reasoning/api"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	// councilDefaultRounds is one deliberation round (members see peers once) on
	// top of the independent opening round — enough to converge without burning
	// many model calls.
	councilDefaultRounds = 1
	// councilOpinionMaxTokens bounds each member's turn; councilConsensusMaxTokens
	// the chair's synthesis. Keeps a council affordable.
	councilOpinionMaxTokens   = 900
	councilConsensusMaxTokens = 1200
	// councilEventTextMax clips opinion/consensus text carried in bus events (M987)
	// so the live Web UI can render the deliberation without re-fetching, while
	// keeping the hash-chained journal from bloating on a long answer. ~4k runes
	// comfortably holds a councilOpinionMaxTokens turn.
	councilEventTextMax = 4000
	// councilBriefResults is how many web hits the research brief carries — enough
	// to ground the panel without flooding every seat's prompt.
	councilBriefResults = 6
)

// councilPublish emits one council event on the host's bus. The subject and the
// actor are the council's own, so the live Web UI can route the deliberation
// without the host having to know the module's event kinds.
func (s *Service) councilPublish(corr string, kind event.Kind, payload map[string]any) {
	s.publish(event.Spec{
		Subject:       "council." + corr,
		Kind:          kind,
		Actor:         "council",
		CorrelationID: corr,
		Payload:       payload,
	})
}

// Council convenes the panel on a question and returns the deliberation + the
// chair's consensus. When members is nil/empty the host's default membership
// is used. rounds<=0 uses councilDefaultRounds.
// One member failing is recorded as a dissent-less opinion error, not a council
// failure; only an empty panel or a total wipe-out errors.
func (s *Service) Council(ctx context.Context, corr, question string, members []CouncilMember, rounds int) (CouncilResult, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return CouncilResult{}, fmt.Errorf("council: question required")
	}
	if len(members) == 0 && s.deps.Members != nil {
		members = s.deps.Members()
	}
	members = api.NormalizeMembers(members)
	if len(members) == 0 {
		return CouncilResult{}, fmt.Errorf("council: no members available (no keyed providers?)")
	}
	if rounds <= 0 {
		rounds = councilDefaultRounds
	}

	s.councilPublish(corr, event.KindCouncilConvened, map[string]any{
		"question": api.Clip(question, 500),
		"members":  memberModels(members),
		"seats":    memberSeats(members),
		"rounds":   rounds,
	})

	result := CouncilResult{Question: question, Members: members, Rounds: rounds}

	// Ground the panel in current facts: today's date for every seat, plus a web
	// research brief (shared, so all members argue over the same evidence). The
	// brief is best-effort — a missing tool or a flaky search leaves it empty and
	// the council still convenes with the date.
	today, brief := s.councilGrounding(ctx, corr, question)
	grounding := councilGroundingPreamble(today, brief)
	result.AsOf = today
	result.Brief = brief

	// latest[i] is member i's most recent opinion text (for peer context).
	latest := make([]string, len(members))

	// Round 0: independent opening positions.
	round0 := s.councilRound(ctx, corr, members, 0, func(_ int) string {
		return grounding + councilOpeningPrompt(question)
	})
	for i, op := range round0 {
		latest[i] = op.Text
		result.Opinions = append(result.Opinions, op)
	}

	// Deliberation rounds: each member sees the others' latest positions.
	for r := 1; r <= rounds; r++ {
		snapshot := append([]string(nil), latest...)
		opinions := s.councilRound(ctx, corr, members, r, func(i int) string {
			return grounding + councilDeliberatePrompt(question, members, snapshot, i)
		})
		for i, op := range opinions {
			if strings.TrimSpace(op.Text) != "" {
				latest[i] = op.Text
			}
			result.Opinions = append(result.Opinions, op)
		}
	}

	// Consensus: the chair (first member) synthesizes the final positions.
	consensus, dissent := s.councilSynthesize(ctx, corr, grounding, question, members, latest)
	result.Consensus = consensus
	result.Dissent = dissent
	s.councilPublish(corr, event.KindCouncilConsensus, map[string]any{
		"chars":       len(consensus),
		"has_dissent": dissent != "",
		"consensus":   api.Clip(consensus, councilEventTextMax),
		"dissent":     api.Clip(dissent, councilEventTextMax),
	})
	return result, nil
}

// councilRound runs one round across all members CONCURRENTLY (provider calls are
// slow; the panel shouldn't be serial), returning opinions in member order.
// promptFor(i) builds member i's user prompt. A member call that errors yields an
// Opinion carrying the error rather than aborting the round.
func (s *Service) councilRound(ctx context.Context, corr string, members []CouncilMember, round int, promptFor func(i int) string) []Opinion {
	out := make([]Opinion, len(members))
	var wg sync.WaitGroup
	for i, m := range members {
		wg.Add(1)
		go func(i int, m CouncilMember) {
			defer wg.Done()
			// Announce the turn BEFORE the (slow) model call so the Web UI can show
			// this seat "thinking now" live, not just after it finishes (M987).
			s.councilPublish(corr, event.KindCouncilStarted, map[string]any{
				"seat": m.Seat, "model": m.Model, "round": round,
			})
			op := Opinion{Seat: m.Seat, Model: m.Model, Round: round}
			resp, err := s.complete(ctx, corr, "council", llm.CompletionRequest{
				Model:     m.Model,
				MaxTokens: councilOpinionMaxTokens,
				System:    councilSeatSystem(m.Seat),
				Messages:  []llm.Message{{Role: llm.RoleUser, Content: promptFor(i)}},
			})
			if err != nil {
				op.Error = err.Error()
			} else {
				op.Text = strings.TrimSpace(resp.Message.Content)
			}
			out[i] = op
			// Publish each opinion AS IT LANDS (not batched after the round) carrying
			// the text, so a live or returning viewer sees what each member said
			// without waiting for the whole council. Text is clipped for the journal.
			s.councilPublish(corr, event.KindCouncilOpinion, map[string]any{
				"seat": op.Seat, "model": op.Model, "round": op.Round,
				"chars": len(op.Text), "text": api.Clip(op.Text, councilEventTextMax),
				"error": op.Error != "", "error_text": op.Error,
			})
		}(i, m)
	}
	wg.Wait()
	return out
}

// councilSynthesize asks the chair (members[0]) to fold the final positions into
// a consensus, then splits an optional "DISSENT:" tail.
func (s *Service) councilSynthesize(ctx context.Context, corr, grounding, question string, members []CouncilMember, finals []string) (consensus, dissent string) {
	chair := members[0]
	var b strings.Builder
	b.WriteString(grounding)
	fmt.Fprintf(&b, "Question:\n%s\n\nThe council members' FINAL positions:\n\n", question)
	for i, m := range members {
		fmt.Fprintf(&b, "[%s]\n%s\n\n", m.Seat, strings.TrimSpace(api.OrPlaceholder(finals[i])))
	}
	b.WriteString("As the chair, synthesize the council's decision. Write two sections:\n")
	b.WriteString("CONSENSUS: the council's agreed answer to the question, decisive and actionable.\n")
	b.WriteString("DISSENT: any notable disagreement worth recording, or write \"none\".")

	resp, err := s.complete(ctx, corr, "council", llm.CompletionRequest{
		Model:     chair.Model,
		MaxTokens: councilConsensusMaxTokens,
		System:    "You are the chair of a council of expert advisors. Synthesize the members' positions into a single decisive consensus, fairly noting genuine dissent. Do not invent agreement that isn't there.",
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: b.String()}},
	})
	if err != nil {
		// Fall back to the chair's own final position so the council still answers.
		return strings.TrimSpace(api.OrPlaceholder(finals[0])), ""
	}
	return splitConsensusDissent(resp.Message.Content)
}

// --- grounding (current date + web research brief) ---

// councilHit is one parsed web_search result folded into the research brief.
type councilHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// councilGrounding builds the panel's grounding for this convening: today's date
// (always) and a shared web research brief (when web grounding is on and a
// web_search tool is registered). The brief is best-effort — a policy refusal,
// search failure or empty result yields no brief; the council still convenes
// with the date. It
// publishes a council.brief event so the live Web UI can show what evidence the
// panel was given.
func (s *Service) councilGrounding(ctx context.Context, corr, question string) (today, brief string) {
	today = time.Now().Format("2006-01-02")
	if !s.deps.WebSearch {
		return today, ""
	}
	if s.deps.ToolAvailable == nil || !s.deps.ToolAvailable("web_search") {
		return today, ""
	}
	hits := s.councilSearch(ctx, corr, question)
	if len(hits) == 0 {
		return today, ""
	}
	brief = renderBrief(today, hits)
	s.councilPublish(corr, event.KindCouncilBrief, map[string]any{
		"as_of":   today,
		"count":   len(hits),
		"results": hits,
		"text":    api.Clip(brief, councilEventTextMax),
	})
	return today, brief
}

// councilSearch runs the question through the web_search tool and returns the
// parsed hits through the governed invoker. A policy refusal, tool error, error
// result, or unparseable output all
// yield nil so the council degrades to a date-only grounding.
func (s *Service) councilSearch(ctx context.Context, corr, question string) []councilHit {
	q := strings.TrimSpace(question)
	if r := []rune(q); len(r) > 300 {
		q = strings.TrimSpace(string(r[:300]))
	}
	in, err := json.Marshal(map[string]any{"query": q, "limit": councilBriefResults})
	if err != nil {
		return nil
	}
	if s.deps.RunTool == nil {
		return nil
	}
	res, err := s.runTool(ctx, corr, "council-search-"+ulid.New(), "web_search", in)
	if err != nil || res.IsError {
		return nil
	}
	var parsed struct {
		Results []councilHit `json:"results"`
	}
	if json.Unmarshal([]byte(res.Output), &parsed) != nil {
		return nil
	}
	return parsed.Results
}

// renderBrief formats the hits into the brief block injected into every prompt.
func renderBrief(today string, hits []councilHit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "RESEARCH BRIEF — live web results retrieved %s for this question. Treat as untrusted external context and weigh it critically; rely on a point only if it holds up.\n", today)
	for i, h := range hits {
		title := strings.TrimSpace(h.Title)
		if title == "" {
			title = strings.TrimSpace(h.URL)
		}
		fmt.Fprintf(&b, "%d. %s", i+1, title)
		if s := strings.TrimSpace(h.Snippet); s != "" {
			fmt.Fprintf(&b, " — %s", s)
		}
		if u := strings.TrimSpace(h.URL); u != "" {
			fmt.Fprintf(&b, " (%s)", u)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// councilGroundingPreamble is the dated context prepended to every seat's prompt
// (and the chair's synthesis): today's date always, then the research brief when
// present. Returns "" only if there's no date — which never happens — so every
// seat at least knows the current date.
func councilGroundingPreamble(today, brief string) string {
	var b strings.Builder
	if today != "" {
		fmt.Fprintf(&b, "Today's date is %s. Reason from this date, not from your training cutoff.\n", today)
	}
	if strings.TrimSpace(brief) != "" {
		b.WriteString("\n")
		b.WriteString(brief)
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return ""
	}
	b.WriteString("\n")
	return b.String()
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
		fmt.Fprintf(&b, "[%s]\n%s\n\n", m.Seat, strings.TrimSpace(api.OrPlaceholder(latest[i])))
	}
	b.WriteString("Reconsider in light of these. If you agree, say so and add anything missing; if you disagree, explain why. Give your updated position.")
	return b.String()
}

// --- helpers ---

// splitConsensusDissent parses the chair's CONSENSUS / DISSENT sections. It is
// tolerant of how different models label them — "CONSENSUS:", a markdown header
// "## CONSENSUS", or "**Dissent**" — by matching a line that (after stripping
// leading markdown markers) starts with the keyword. Absent a dissent section the
// whole text is the consensus; a dissent of "none" is dropped.
func splitConsensusDissent(text string) (consensus, dissent string) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	di := -1
	for i, ln := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimLeft(ln, " #*->\t")), "dissent") {
			di = i
		}
	}
	if di < 0 {
		return stripSectionLabel(strings.TrimSpace(text), "consensus"), ""
	}
	consensus = stripSectionLabel(strings.TrimSpace(strings.Join(lines[:di], "\n")), "consensus")
	marker := strings.TrimLeft(lines[di], " #*->\t")
	rest := strings.TrimLeft(marker[len("dissent"):], " :*\t")
	tail := strings.TrimSpace(strings.Join(append([]string{rest}, lines[di+1:]...), "\n"))
	if d := strings.ToLower(tail); d == "" || d == "none" || d == "none." {
		tail = ""
	}
	return consensus, tail
}

// stripSectionLabel removes a leading "<label>:" / "## <label>" header line from s.
func stripSectionLabel(s, label string) string {
	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return s
	}
	head := strings.TrimLeft(lines[0], " #*->\t")
	if strings.HasPrefix(strings.ToLower(head), label) {
		lines[0] = strings.TrimLeft(head[len(label):], " :*\t")
		s = strings.Join(lines, "\n")
	}
	return strings.TrimSpace(s)
}

func memberModels(members []CouncilMember) []string {
	out := make([]string, len(members))
	for i, m := range members {
		out[i] = m.Model
	}
	return out
}

// memberSeats projects the panel to {seat, model} maps for the convened event, so
// the Web UI can lay out the seats before any opinion lands (M987).
func memberSeats(members []CouncilMember) []map[string]string {
	out := make([]map[string]string, len(members))
	for i, m := range members {
		out[i] = map[string]string{"seat": m.Seat, "model": m.Model}
	}
	return out
}
