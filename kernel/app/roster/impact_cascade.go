// SPDX-License-Identifier: MIT

package roster

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/modules/board/board"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/kernel/modules/workflows/workflow"
	core "github.com/agezt/agezt/kernel/roster"
)

// ImpactPorts are the host's capabilities the teardown impact analysis reads.
// The application owns the DERIVATION (which labels belong to the agent, how
// a mailbox message reads, whether a workflow node references the agent);
// the host owns the DATA (the stores).
type ImpactPorts struct {
	// Schedules lists the cadence entries.
	Schedules func() []cadence.Entry
	// MemoryRecords lists the active memory records.
	MemoryRecords func() ([]memory.Record, error)
	// Skills lists the agent's skills.
	Skills func() ([]skill.Skill, error)
	// ConfigEntries lists the config-center entries.
	ConfigEntries func() []*configcenter.ConfigEntry
	// WorkspaceInfo describes the agent's workspace, when it has one.
	WorkspaceInfo func(p core.Profile) (string, bool)
	// Workflows lists the workflows.
	Workflows func() []workflow.Workflow
	// Mailbox reads the board's messages.
	Mailbox func() []board.Message
}

// ScheduleImpact lists the schedules that wake the agent.
func ScheduleImpact(ports ImpactPorts, p core.Profile) []string {
	var out []string
	for _, e := range ports.Schedules() {
		if strings.EqualFold(strings.TrimSpace(e.Agent), p.Slug) {
			label := e.Intent
			if strings.TrimSpace(label) == "" {
				label = e.ID
			}
			out = append(out, label+" ("+e.ID+")")
		}
	}
	sort.Strings(out)
	return out
}

// MemoryImpact lists the agent's own memory records.
func MemoryImpact(ports ImpactPorts, p core.Profile) []string {
	scope := strings.TrimSpace(p.MemoryScope)
	if scope == "" {
		scope = p.Slug
	}
	records, err := ports.MemoryRecords()
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range records {
		if MemoryRecordBelongsToAgent(r, scope) {
			out = append(out, MemoryRecordLabel(r))
		}
	}
	sort.Strings(out)
	return out
}

// AuthoredSharedMemoryImpact lists the shared memory records the agent
// authored or last updated.
func AuthoredSharedMemoryImpact(ports ImpactPorts, p core.Profile) []string {
	records, err := ports.MemoryRecords()
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range records {
		if MemoryRecordAuthoredSharedByAgent(r, p.Slug) {
			out = append(out, MemoryRecordLabel(r))
		}
	}
	sort.Strings(out)
	return out
}

// MemoryRecordLabel names a record by its subject, falling back to its id
// when a record was stored without one.
func MemoryRecordLabel(r memory.Record) string {
	subj := strings.TrimSpace(r.Subject)
	if subj == "" {
		subj = r.ID
	}
	return subj + " (" + r.ID + ")"
}

// SkillImpact lists the agent's skills that are not archived.
func SkillImpact(ports ImpactPorts, p core.Profile) []string {
	all, err := ports.Skills()
	if err != nil {
		return nil
	}
	var out []string
	for _, sk := range all {
		if strings.EqualFold(strings.TrimSpace(sk.Agent), p.Slug) && sk.Status != skill.StatusArchived {
			name := strings.TrimSpace(sk.Name)
			if name == "" {
				name = sk.ID
			}
			out = append(out, name+" ("+sk.ID+")")
		}
	}
	sort.Strings(out)
	return out
}

// ConfigImpact lists the config-center entries that belong to the agent.
func ConfigImpact(ports ImpactPorts, p core.Profile) []string {
	if ports.ConfigEntries == nil {
		return nil
	}
	var out []string
	for _, e := range ports.ConfigEntries() {
		if ConfigEntryBelongsToAgent(e, p.Slug) {
			label := strings.TrimSpace(e.Key)
			if e.Rating != "" {
				label += " [" + string(e.Rating) + "]"
			}
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

// WorkspaceImpact lists the agent's workspace, when it has one.
func WorkspaceImpact(ports ImpactPorts, p core.Profile) []string {
	info, ok := ports.WorkspaceInfo(p)
	if !ok {
		return nil
	}
	return []string{info}
}

// WorkflowImpact lists the workflow nodes that reference the agent.
func WorkflowImpact(ports ImpactPorts, p core.Profile) []string {
	slug := strings.TrimSpace(p.Slug)
	if slug == "" || ports.Workflows == nil {
		return nil
	}
	var out []string
	for _, w := range ports.Workflows() {
		for _, n := range w.Nodes {
			if !WorkflowNodeConfigReferencesAgent(n.Config, slug) {
				continue
			}
			label := w.Name + "/" + n.ID
			if strings.TrimSpace(n.Label) != "" {
				label += " " + strings.TrimSpace(n.Label)
			}
			label += " [" + n.Type + "]"
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

// MailboxImpact lists the board messages the agent sent, received or
// acknowledged.
func MailboxImpact(ports ImpactPorts, p core.Profile) []string {
	if ports.Mailbox == nil {
		return nil
	}
	slug := strings.ToLower(strings.TrimSpace(p.Slug))
	if slug == "" {
		return nil
	}
	var out []string
	for _, msg := range ports.Mailbox() {
		if label, ok := MailboxImpactLabel(msg, slug); ok {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

// RemovalMailboxImpact is the removal payload's retained-message list: the
// agent's own threads plus, when sub-agents are cascaded, theirs — DEDUPED,
// because a message between a parent and its child appears in both.
func RemovalMailboxImpact(ports ImpactPorts, slug string, subagents []core.Profile, includeSubagents bool) []string {
	seen := map[string]bool{}
	add := func(labels []string) {
		for _, label := range labels {
			if strings.TrimSpace(label) != "" {
				seen[label] = true
			}
		}
	}
	add(MailboxImpact(ports, core.Profile{Slug: slug}))
	if includeSubagents {
		for _, child := range subagents {
			add(MailboxImpact(ports, child))
		}
	}
	out := make([]string, 0, len(seen))
	for label := range seen {
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

// MailboxImpactLabel names a board message from the agent's point of view:
// the topic and the direction, with the message id.
func MailboxImpactLabel(msg board.Message, slug string) (string, bool) {
	from := strings.ToLower(strings.TrimSpace(msg.From))
	to := strings.ToLower(strings.TrimSpace(msg.To))
	acked := BoardMessageAckedBy(msg, slug)
	var direction string
	switch {
	case from == slug:
		direction = "sent"
	case to == slug:
		direction = "received"
	case msg.To == board.Everyone && from != slug:
		direction = "broadcast"
	case acked:
		direction = "acked"
	default:
		return "", false
	}
	topic := strings.TrimSpace(msg.Topic)
	if topic == "" {
		topic = "board"
	}
	id := strings.TrimSpace(msg.ID)
	if id == "" {
		id = strconv.FormatInt(msg.TSMS, 10)
	}
	return topic + " " + direction + " (" + id + ")", true
}

// WorkflowNodeConfigReferencesAgent reports whether a workflow node's config
// names the agent.
func WorkflowNodeConfigReferencesAgent(raw json.RawMessage, slug string) bool {
	if len(raw) == 0 {
		return false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	return jsonValueReferencesAgent(v, strings.ToLower(strings.TrimSpace(slug)), "")
}

// jsonValueReferencesAgent walks a decoded config value looking for the
// agent under an agent-reference key.
func jsonValueReferencesAgent(v any, slug, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if jsonValueReferencesAgent(value, slug, strings.ToLower(strings.TrimSpace(k))) {
				return true
			}
		}
	case []any:
		for _, value := range x {
			if jsonValueReferencesAgent(value, slug, key) {
				return true
			}
		}
	case string:
		if !agentReferenceConfigKey(key) {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(x), slug)
	}
	return false
}

// agentReferenceConfigKey reports whether a config key can name an agent.
func agentReferenceConfigKey(key string) bool {
	switch key {
	case "agent", "agent_slug", "target_agent", "owner_agent", "parent_agent", "delegate_to", "source_agent", "root_agent":
		return true
	default:
		return false
	}
}

// Subagents returns the agent's sub-agent tree — the agents whose owner or
// parent is the agent, and their own sub-agents — sorted by slug.
func Subagents(profiles []core.Profile, slug string) []core.Profile {
	root := strings.TrimSpace(slug)
	if root == "" {
		return nil
	}
	byManager := map[string][]core.Profile{}
	for _, p := range profiles {
		childSlug := strings.TrimSpace(p.Slug)
		if childSlug == "" || strings.EqualFold(childSlug, root) {
			continue
		}
		seenManager := map[string]bool{}
		for _, manager := range []string{strings.TrimSpace(p.OwnerAgent), strings.TrimSpace(p.ParentAgent)} {
			if manager == "" || strings.EqualFold(manager, childSlug) || seenManager[strings.ToLower(manager)] {
				continue
			}
			seenManager[strings.ToLower(manager)] = true
			byManager[strings.ToLower(manager)] = append(byManager[strings.ToLower(manager)], p)
		}
	}
	var out []core.Profile
	seen := map[string]bool{strings.ToLower(root): true}
	var walk func(string)
	walk = func(parent string) {
		for _, child := range byManager[strings.ToLower(strings.TrimSpace(parent))] {
			key := strings.ToLower(strings.TrimSpace(child.Slug))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, child)
			walk(child.Slug)
		}
	}
	walk(root)
	sort.Slice(out, func(i, j int) bool {
		return strings.Compare(out[i].Slug, out[j].Slug) < 0
	})
	return out
}
