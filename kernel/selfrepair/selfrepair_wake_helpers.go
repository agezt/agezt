// SPDX-License-Identifier: MIT

package selfrepair

// Wake helpers: autoRepairWakeSkipReason + autoRepairWakeIntent +
// autoRepairMessageID + autoRepairErrString + autoReplyEscalation
// + autoRepairWakeReplyText. Carved out of selfrepair_wake.go
// during the Day 191 god-file split so the main file can stay
// focused on autoWakeManager and the agent file can stay focused
// on autoRepairWakeAgent.
// Public API unchanged.

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
)

func autoRepairWakeSkipReason(p roster.Profile) string {
	if !p.Enabled {
		return "target agent " + p.Slug + " is paused"
	}
	if p.Retired {
		return "target agent " + p.Slug + " is retired"
	}
	if !p.AllowsDirectCall() {
		return "target agent " + p.Slug + " is a managed sub-agent"
	}
	return ""
}

func autoRepairWakeIntent(cand autoRepairCandidate, msg *board.Message) string {
	var b strings.Builder
	b.WriteString("Escalation wake-up.\n")
	b.WriteString("A doctor/self-repair attempt failed for agent ")
	b.WriteString(cand.Slug)
	b.WriteString(".\n")
	if mode := strings.TrimSpace(cand.Mode); mode != "" {
		b.WriteString("Failure mode: ")
		b.WriteString(mode)
		b.WriteString(".\n")
	}
	if reason := strings.TrimSpace(cand.Reason); reason != "" {
		b.WriteString("Reason: ")
		b.WriteString(reason)
		b.WriteString("\n")
	}
	if fp := strings.TrimSpace(cand.Fingerprint); fp != "" {
		b.WriteString("Fingerprint: ")
		b.WriteString(autoRepairClip(fp, 320))
		b.WriteString("\n")
	}
	if msg != nil {
		b.WriteString("Mailbox help request")
		if msg.ID != "" {
			b.WriteString(" id=")
			b.WriteString(msg.ID)
		}
		b.WriteString(" was posted")
		if to := strings.TrimSpace(msg.To); to != "" {
			b.WriteString(" to ")
			b.WriteString(to)
		}
		b.WriteString(".\n")
	}
	if root := autoRepairRootAgent(cand); root != "" {
		b.WriteString("Escalation root agent: ")
		b.WriteString(root)
		b.WriteString(".\n")
	}
	if incidentID := autoRepairIncidentIDValue(cand); incidentID != "" {
		b.WriteString("Incident id: ")
		b.WriteString(incidentID)
		b.WriteString(".\n")
	}
	if cand.ChainDepth > 0 {
		b.WriteString("Escalation chain depth: ")
		b.WriteString(fmt.Sprintf("%d", cand.ChainDepth))
		b.WriteString(".\n")
	}
	switch strings.TrimSpace(cand.Mode) {
	case "routing_forced_exhausted":
		b.WriteString("Routing state: an owner-forced chain has already been retried across multiple forced generations and still remains under fallback pressure.\n")
		if taskType := strings.TrimSpace(cand.RoutingRollbackTaskType); taskType != "" {
			b.WriteString("Forced task type: ")
			b.WriteString(taskType)
			b.WriteString(".\n")
		}
		if len(cand.RoutingRollbackToChain) > 0 {
			b.WriteString("Forced chain: ")
			b.WriteString(strings.Join(cand.RoutingRollbackToChain, " → "))
			b.WriteString(".\n")
		}
		b.WriteString("Treat this as an exhausted routing policy. Prefer retire, pause, or delegate unless you have a concrete new force_chain backed by stronger evidence.\n")
	case "routing_forced_failed":
		b.WriteString("Routing state: an owner-forced chain already served its probation window and the same task routing is still under fallback pressure.\n")
		if taskType := strings.TrimSpace(cand.RoutingRollbackTaskType); taskType != "" {
			b.WriteString("Forced task type: ")
			b.WriteString(taskType)
			b.WriteString(".\n")
		}
		if len(cand.RoutingRollbackToChain) > 0 {
			b.WriteString("Forced chain: ")
			b.WriteString(strings.Join(cand.RoutingRollbackToChain, " → "))
			b.WriteString(".\n")
		}
		b.WriteString("Choose deliberately between pause, retire, delegate, or a new force_chain decision. Do not answer with handled unless you actually stabilized ownership and routing policy.\n")
	case "routing_unstable":
		b.WriteString("Routing state: a previous routing rewrite rolled back and the task chain still destabilized again.\n")
		b.WriteString("Treat this as an ownership decision, not a routine retry. Prefer pause, retire, delegate, or force_chain when you have a concrete stable route.\n")
	}
	b.WriteString("Take ownership. Inspect the agent's health, mailbox, logs, and repair state; then recover it, pause/retire it, delegate follow-up, or force a stable routing chain when the task routing is clearly wrong.\n")
	b.WriteString("End your final answer with EXACTLY ONE fenced json block so the escalation can be classified deterministically:\n")
	b.WriteString("```json\n")
	b.WriteString("{\"resolution\":\"handled|paused|retired|delegated|blocked|force_chain\",\"summary\":\"short operator-facing closure note\",\"delegate_to\":\"optional target agent slug when delegated\",\"task_type\":\"required when resolution=force_chain\",\"task_model_chain\":[\"required\",\"when\",\"resolution=force_chain\"]}\n")
	b.WriteString("```\n")
	b.WriteString("Use only one resolution value. Omit delegate_to unless resolution is delegated. Use task_type and task_model_chain only when resolution is force_chain.")
	return b.String()
}

func autoRepairMessageID(msg *board.Message) string {
	if msg == nil {
		return ""
	}
	return strings.TrimSpace(msg.ID)
}

func autoRepairErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (c *autoRepairCoordinator) autoReplyEscalation(mailbox Mailbox, postNotify func(board.Message, string), wake autoRepairWakeResult, msg *board.Message) (*board.Message, error) {
	if mailbox == nil || msg == nil || strings.TrimSpace(msg.ID) == "" || strings.TrimSpace(wake.Target) == "" {
		return nil, nil
	}
	orig, ok := mailbox.Get(msg.ID)
	if !ok {
		return nil, nil
	}
	reply, err := mailbox.Send(board.Message{
		Topic:   orig.Topic,
		From:    wake.Target,
		To:      orig.From,
		ReplyTo: orig.ID,
		Text:    autoRepairWakeReplyText(wake),
	}, c.now().UnixMilli())
	if err != nil {
		return nil, err
	}
	if postNotify != nil {
		postNotify(reply, wake.Correlation)
	}
	return &reply, nil
}

func autoRepairWakeReplyText(wake autoRepairWakeResult) string {
	if wake.Resolution != nil {
		var parts []string
		parts = append(parts, "Resolution: "+wake.Resolution.Resolution+".")
		if wake.Resolution.Summary != "" {
			parts = append(parts, wake.Resolution.Summary)
		}
		if wake.Resolution.DelegateTo != "" {
			parts = append(parts, "Delegated to "+wake.Resolution.DelegateTo+".")
		}
		return autoRepairClip(strings.Join(parts, " "), 800)
	}
	answer := strings.TrimSpace(wake.Answer)
	if answer == "" {
		return "Escalation received and handled."
	}
	return autoRepairClip(answer, 800)
}


