// SPDX-License-Identifier: MIT

package roster

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/memory"
	core "github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/kernel/workflow"
)

// impactFake is a host stand-in for the impact ports.
type impactFake struct {
	schedules []cadence.Entry
	records   []memory.Record
	skills    []skill.Skill
	entries   []*configcenter.ConfigEntry
	workspace string
	workflows []workflow.Workflow
	messages  []board.Message
}

func (f *impactFake) ports() ImpactPorts {
	return ImpactPorts{
		Schedules:     func() []cadence.Entry { return f.schedules },
		MemoryRecords: func() ([]memory.Record, error) { return f.records, nil },
		Skills:        func() ([]skill.Skill, error) { return f.skills, nil },
		ConfigEntries: func() []*configcenter.ConfigEntry { return f.entries },
		WorkspaceInfo: func(p core.Profile) (string, bool) {
			if f.workspace == "" {
				return "", false
			}
			return f.workspace, true
		},
		Workflows: func() []workflow.Workflow { return f.workflows },
		Mailbox:   func() []board.Message { return f.messages },
	}
}

func TestScheduleImpact(t *testing.T) {
	fake := &impactFake{schedules: []cadence.Entry{
		{ID: "s1", Agent: "ops", Intent: "check"},
		{ID: "s2", Agent: "OPS"},
		{ID: "s3", Agent: "other", Intent: "not mine"},
	}}
	got := ScheduleImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 2 {
		t.Fatalf("schedules = %v, want the agent's two", got)
	}
	// The intent is the label; the id follows.
	if got[0] != "check (s1)" {
		t.Errorf("label = %q, want the intent and the id", got[0])
	}
	// A schedule without an intent falls back to its id.
	fake = &impactFake{schedules: []cadence.Entry{{ID: "s2", Agent: "ops"}}}
	got = ScheduleImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 1 || got[0] != "s2 (s2)" {
		t.Errorf("label = %v, want the id fallback", got)
	}
}

func TestMemoryImpact(t *testing.T) {
	fake := &impactFake{records: []memory.Record{
		{ID: "m1", Subject: "note", Tags: map[string]string{"scope": "ops"}},
		{ID: "m2", Tags: map[string]string{"scope": "other"}},
	}}
	got := MemoryImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 1 || got[0] != "note (m1)" {
		t.Errorf("memories = %v, want the agent's scoped record", got)
	}
	// A record without a subject falls back to its id.
	fake = &impactFake{records: []memory.Record{{ID: "m1", Tags: map[string]string{"scope": "ops"}}}}
	got = MemoryImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 1 || got[0] != "m1 (m1)" {
		t.Errorf("memories = %v, want the id fallback", got)
	}
}

func TestAuthoredSharedMemoryImpact(t *testing.T) {
	fake := &impactFake{records: []memory.Record{
		{ID: "m1", Subject: "shared", AddedBy: "ops"},
		{ID: "m2", Subject: "other", AddedBy: "someone"},
		{ID: "m3", Subject: "scoped", AddedBy: "ops", Tags: map[string]string{"scope": "ops"}},
	}}
	got := AuthoredSharedMemoryImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 1 || got[0] != "shared (m1)" {
		t.Errorf("shared memories = %v, want the unscoped authored record", got)
	}
}

func TestSkillImpact(t *testing.T) {
	fake := &impactFake{skills: []skill.Skill{
		{ID: "k1", Agent: "ops", Name: "deploy", Status: skill.StatusActive},
		{ID: "k2", Agent: "ops", Status: skill.StatusArchived},
		{ID: "k3", Agent: "other", Name: "theirs", Status: skill.StatusActive},
	}}
	got := SkillImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 1 || got[0] != "deploy (k1)" {
		t.Errorf("skills = %v, want the agent's active skill", got)
	}
	// A skill without a name falls back to its id.
	fake = &impactFake{skills: []skill.Skill{{ID: "k1", Agent: "ops", Status: skill.StatusActive}}}
	got = SkillImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 1 || got[0] != "k1 (k1)" {
		t.Errorf("skills = %v, want the id fallback", got)
	}
}

func TestConfigImpact(t *testing.T) {
	fake := &impactFake{entries: []*configcenter.ConfigEntry{
		{Key: "agent/ops/one", Rating: "secret"},
		{Key: "other"},
	}}
	got := ConfigImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 1 || got[0] != "agent/ops/one [secret]" {
		t.Errorf("configs = %v, want the agent's entry with its rating", got)
	}
}

func TestWorkspaceImpact(t *testing.T) {
	fake := &impactFake{workspace: "/root/ops"}
	got := WorkspaceImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 1 || got[0] != "/root/ops" {
		t.Errorf("workspaces = %v, want the workspace", got)
	}
	fake = &impactFake{}
	if got := WorkspaceImpact(fake.ports(), core.Profile{Slug: "ops"}); got != nil {
		t.Errorf("workspaces = %v, want none", got)
	}
}

func TestWorkflowImpact(t *testing.T) {
	cfg, _ := json.Marshal(map[string]any{"agent": "ops"})
	other, _ := json.Marshal(map[string]any{"agent": "someone"})
	fake := &impactFake{workflows: []workflow.Workflow{{
		Name: "flow",
		Nodes: []workflow.Node{
			{ID: "n1", Type: "task", Label: "do it", Config: cfg},
			{ID: "n2", Type: "task", Config: other},
		},
	}}}
	got := WorkflowImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 1 {
		t.Fatalf("workflows = %v, want the referencing node", got)
	}
	if got[0] != "flow/n1 do it [task]" {
		t.Errorf("label = %q, want the workflow, node, label and type", got[0])
	}
}

func TestMailboxImpact(t *testing.T) {
	fake := &impactFake{messages: []board.Message{
		{ID: "b1", From: "ops", To: "lead", Topic: "ops"},
		{ID: "b2", From: "lead", To: "ops", Topic: "asks"},
		{ID: "b3", From: "lead", To: board.Everyone, Topic: "all"},
		{ID: "b4", From: "lead", To: "other", Topic: "theirs"},
	}}
	got := MailboxImpact(fake.ports(), core.Profile{Slug: "ops"})
	if len(got) != 3 {
		t.Fatalf("messages = %v, want the three that involve the agent", got)
	}
	joined := ""
	for _, g := range got {
		joined += g + "|"
	}
	for _, want := range []string{"ops sent (b1)", "asks received (b2)", "all broadcast (b3)"} {
		if !contains(joined, want) {
			t.Errorf("messages = %v, want %q", got, want)
		}
	}
}

func TestMailboxImpactLabel(t *testing.T) {
	label, ok := MailboxImpactLabel(board.Message{ID: "b1", From: "ops", To: "lead", Topic: "t"}, "ops")
	if !ok || label != "t sent (b1)" {
		t.Errorf("label = %q/%v, want the sent direction", label, ok)
	}
	label, ok = MailboxImpactLabel(board.Message{ID: "b2", From: "lead", To: "ops"}, "ops")
	if !ok || label != "board received (b2)" {
		t.Errorf("label = %q/%v, want the received direction with the board topic", label, ok)
	}
	label, ok = MailboxImpactLabel(board.Message{ID: "b3", From: "lead", To: "other"}, "ops")
	if ok {
		t.Errorf("label = %q, want no label for another agent's message", label)
	}
	// An acknowledged message the agent only received a copy of still reads.
	label, ok = MailboxImpactLabel(board.Message{ID: "b4", From: "lead", To: "other", AckedBy: []string{"ops"}}, "ops")
	if !ok || label != "board acked (b4)" {
		t.Errorf("label = %q/%v, want the acked direction", label, ok)
	}
	// A message without an id falls back to its timestamp.
	label, ok = MailboxImpactLabel(board.Message{TSMS: 42, From: "ops", To: "lead"}, "ops")
	if !ok || label != "board sent (42)" {
		t.Errorf("label = %q/%v, want the timestamp fallback", label, ok)
	}
}

func TestRemovalMailboxImpact_Dedupes(t *testing.T) {
	fake := &impactFake{messages: []board.Message{
		{ID: "b1", From: "ops", To: "kid", Topic: "t"},
		{ID: "b2", From: "kid", To: "ops", Topic: "t"},
	}}
	// The list is deduplicated by LABEL, so a message the agent sent and a
	// message it received stay apart.
	got := RemovalMailboxImpact(fake.ports(), "ops", []core.Profile{{Slug: "kid"}}, true)
	if len(got) != 4 {
		t.Errorf("messages = %v, want both threads from both sides", got)
	}
	// Without the sub-agents, only the agent's own threads stay.
	got = RemovalMailboxImpact(fake.ports(), "ops", []core.Profile{{Slug: "kid"}}, false)
	if len(got) != 2 {
		t.Errorf("messages = %v, want only the agent's own", got)
	}
}

func TestWorkflowNodeConfigReferencesAgent(t *testing.T) {
	cases := []struct {
		raw  string
		slug string
		want bool
	}{
		{`{"agent":"ops"}`, "ops", true},
		{`{"agent":"OPS"}`, "ops", true},
		{`{"agent_slug":"ops"}`, "ops", true},
		{`{"target_agent":"ops"}`, "ops", true},
		{`{"nested":{"delegate_to":"ops"}}`, "ops", true},
		{`{"list":[{"owner_agent":"ops"}]}`, "ops", true},
		{`{"agent":"other"}`, "ops", false},
		{`{"name":"ops"}`, "ops", false},
		{`{}`, "ops", false},
		{`not json`, "ops", false},
		{``, "ops", false},
	}
	for _, tc := range cases {
		got := WorkflowNodeConfigReferencesAgent(json.RawMessage(tc.raw), tc.slug)
		if got != tc.want {
			t.Errorf("config %q for %q = %v, want %v", tc.raw, tc.slug, got, tc.want)
		}
	}
}

func TestSubagents(t *testing.T) {
	profiles := []core.Profile{
		{Slug: "root", OwnerAgent: ""},
		{Slug: "kid", OwnerAgent: "root"},
		{Slug: "grandkid", ParentAgent: "kid"},
		{Slug: "other", OwnerAgent: "someone"},
	}
	got := Subagents(profiles, "root")
	if len(got) != 2 {
		t.Fatalf("subagents = %v, want the tree", got)
	}
	// The tree is sorted by slug.
	if got[0].Slug != "grandkid" || got[1].Slug != "kid" {
		t.Errorf("subagents = %v, want them sorted", got)
	}
	if Subagents(profiles, "") != nil {
		t.Error("an empty root has no sub-agents")
	}
}

// contains is a tiny helper so the test does not need the strings package.
func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
