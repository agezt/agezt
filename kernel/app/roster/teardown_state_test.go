// SPDX-License-Identifier: MIT

package roster

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/configcenter"
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
)

// stateFake is a host stand-in for the teardown state ports.
type stateFake struct {
	records   []memory.Record
	forgotten []string
	skills    []skill.Skill
	archived  []string
	entries   []*configcenter.ConfigEntry
	deleted   []string
	written   []*configcenter.ConfigEntry
	root      string
	exists    bool
	isDir     bool
	removed   []string
	missing   map[string]bool
	failOn    string
	failWith  error
}

// gone reports whether the fake's store no longer has the record.
func (f *stateFake) gone(id string) bool { return f.missing[id] }

func (f *stateFake) ports() TeardownStatePorts {
	return TeardownStatePorts{
		MemoryRecords: func() ([]memory.Record, error) { return f.records, nil },
		ForgetMemory: func(id string) (bool, error) {
			if f.failOn == id {
				return false, f.failWith
			}
			if f.gone(id) {
				return false, nil
			}
			f.forgotten = append(f.forgotten, id)
			return true, nil
		},
		Skills: func() ([]skill.Skill, error) { return f.skills, nil },
		ArchiveSkill: func(id, reason string) error {
			if f.failOn == id {
				return f.failWith
			}
			f.archived = append(f.archived, id)
			return nil
		},
		ConfigEntries: func() []*configcenter.ConfigEntry { return f.entries },
		DeleteConfig: func(key string) error {
			if f.failOn == key {
				return f.failWith
			}
			f.deleted = append(f.deleted, key)
			return nil
		},
		SetConfig: func(e *configcenter.ConfigEntry) error {
			if f.failOn == e.Key {
				return f.failWith
			}
			f.written = append(f.written, e)
			return nil
		},
		WorkspaceRoot: func() string { return f.root },
		WorkspaceDir: func(dir string) (bool, bool, error) {
			if f.failOn == "stat" {
				return false, false, f.failWith
			}
			return f.exists, f.isDir, nil
		},
		DeleteDir: func(dir string) error {
			if f.failOn == "remove" {
				return f.failWith
			}
			f.removed = append(f.removed, dir)
			return nil
		},
	}
}

func TestForgetAgentMemory(t *testing.T) {
	fake := &stateFake{records: []memory.Record{
		{ID: "m1", Tags: map[string]string{"scope": "ops"}},
		{ID: "m2", Tags: map[string]string{"scope": "OPS"}},
		{ID: "m3", Tags: map[string]string{"scope": "other"}},
		{ID: "m4"},
	}}
	n, err := ForgetAgentMemory(fake.ports(), core.Profile{Slug: "ops"}, true)
	if err != nil {
		t.Fatalf("forget: %v", err)
	}
	if n != 2 {
		t.Errorf("forgot = %d, want the two scoped records", n)
	}
	if len(fake.forgotten) != 2 || fake.forgotten[0] != "m1" || fake.forgotten[1] != "m2" {
		t.Errorf("forgotten = %v, want m1 and m2", fake.forgotten)
	}
}

func TestForgetAgentMemory_UsesTheMemoryScope(t *testing.T) {
	fake := &stateFake{records: []memory.Record{
		{ID: "m1", Tags: map[string]string{"scope": "shared-space"}},
		{ID: "m2", Tags: map[string]string{"scope": "ops"}},
	}}
	n, err := ForgetAgentMemory(fake.ports(), core.Profile{Slug: "ops", MemoryScope: "shared-space"}, true)
	if err != nil {
		t.Fatalf("forget: %v", err)
	}
	if n != 1 || len(fake.forgotten) != 1 || fake.forgotten[0] != "m1" {
		t.Errorf("forgot = %d/%v, want only the scoped record", n, fake.forgotten)
	}
}

// A record the store no longer has is not counted as forgotten.
func TestForgetAgentMemory_CountsOnlyWhatExisted(t *testing.T) {
	fake := &stateFake{
		records: []memory.Record{
			{ID: "m1", Tags: map[string]string{"scope": "ops"}},
			{ID: "m2", Tags: map[string]string{"scope": "ops"}},
		},
		missing: map[string]bool{"m2": true},
	}
	n, err := ForgetAgentMemory(fake.ports(), core.Profile{Slug: "ops"}, true)
	if err != nil {
		t.Fatalf("forget: %v", err)
	}
	if n != 1 {
		t.Errorf("forgot = %d, want only the record that existed", n)
	}
}

func TestForgetAgentMemory_OffDoesNothing(t *testing.T) {
	fake := &stateFake{records: []memory.Record{{ID: "m1", Tags: map[string]string{"scope": "ops"}}}}
	n, err := ForgetAgentMemory(fake.ports(), core.Profile{Slug: "ops"}, false)
	if err != nil || n != 0 || len(fake.forgotten) != 0 {
		t.Errorf("forget off = %d/%v/%v, want nothing", n, fake.forgotten, err)
	}
}

func TestForgetAgentAuthoredSharedMemory(t *testing.T) {
	fake := &stateFake{records: []memory.Record{
		{ID: "m1", AddedBy: "ops"},
		{ID: "m2", UpdatedBy: "OPS"},
		{ID: "m3", AddedBy: "other"},
		{ID: "m4", AddedBy: "ops", Tags: map[string]string{"scope": "ops"}},
	}}
	n, err := ForgetAgentAuthoredSharedMemory(fake.ports(), "ops", true)
	if err != nil {
		t.Fatalf("forget: %v", err)
	}
	if n != 2 {
		t.Errorf("forgot = %d, want the two shared records", n)
	}
	if len(fake.forgotten) != 2 {
		t.Errorf("forgotten = %v, want m1 and m2", fake.forgotten)
	}
}

func TestForgetAgentAuthoredSharedMemory_OffDoesNothing(t *testing.T) {
	fake := &stateFake{records: []memory.Record{{ID: "m1", AddedBy: "ops"}}}
	n, err := ForgetAgentAuthoredSharedMemory(fake.ports(), "ops", false)
	if err != nil || n != 0 || len(fake.forgotten) != 0 {
		t.Errorf("forget off = %d/%v/%v, want nothing", n, fake.forgotten, err)
	}
}

func TestArchiveAgentSkills(t *testing.T) {
	fake := &stateFake{skills: []skill.Skill{
		{ID: "s1", Agent: "ops", Status: skill.StatusActive},
		{ID: "s2", Agent: "OPS", Status: skill.StatusDraft},
		{ID: "s3", Agent: "ops", Status: skill.StatusArchived},
		{ID: "s4", Agent: "other", Status: skill.StatusActive},
	}}
	n, err := ArchiveAgentSkills(fake.ports(), "ops", true)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n != 2 {
		t.Errorf("archived = %d, want the two agent's skills", n)
	}
	if len(fake.archived) != 2 {
		t.Errorf("archived = %v, want s1 and s2", fake.archived)
	}
}

func TestArchiveAgentSkills_OffDoesNothing(t *testing.T) {
	fake := &stateFake{skills: []skill.Skill{{ID: "s1", Agent: "ops", Status: skill.StatusActive}}}
	n, err := ArchiveAgentSkills(fake.ports(), "ops", false)
	if err != nil || n != 0 || len(fake.archived) != 0 {
		t.Errorf("archive off = %d/%v/%v, want nothing", n, fake.archived, err)
	}
}

// A skill that refuses the transition is left alone, not an error.
func TestArchiveAgentSkills_SkipsIllegalTransitions(t *testing.T) {
	fake := &stateFake{
		skills:   []skill.Skill{{ID: "s1", Agent: "ops", Status: skill.StatusActive}},
		failOn:   "s1",
		failWith: skill.ErrIllegalTransition,
	}
	n, err := ArchiveAgentSkills(fake.ports(), "ops", true)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n != 0 {
		t.Errorf("archived = %d, want none", n)
	}
}

func TestArchiveAgentSkills_ReportsTheFailure(t *testing.T) {
	fake := &stateFake{
		skills:   []skill.Skill{{ID: "s1", Agent: "ops", Status: skill.StatusActive}},
		failOn:   "s1",
		failWith: errors.New("forge down"),
	}
	if _, err := ArchiveAgentSkills(fake.ports(), "ops", true); err == nil {
		t.Error("err = nil, want the forge's error")
	}
}

func TestDeleteAgentConfigEntries(t *testing.T) {
	other := &configcenter.ConfigEntry{Key: "other", AllowedAgents: []string{"OPS"}}
	fake := &stateFake{entries: []*configcenter.ConfigEntry{
		{Key: "agent/ops/b.own", AllowedAgents: []string{"ops"}},
		{Key: "agent/ops/a.own"},
		other,
	}}
	deleted, pruned, err := DeleteAgentConfigEntries(fake.ports(), "ops", true)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want the agent's two entries", deleted)
	}
	if len(fake.deleted) != 2 || fake.deleted[0] != "agent/ops/a.own" {
		t.Errorf("deleted = %v, want them sorted", fake.deleted)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d, want the entry that stays", pruned)
	}
	if len(fake.written) != 1 || len(other.AllowedAgents) != 0 {
		t.Errorf("written = %v, allowed = %v, want the reference pruned", fake.written, other.AllowedAgents)
	}
}

func TestDeleteAgentConfigEntries_OffDoesNothing(t *testing.T) {
	fake := &stateFake{entries: []*configcenter.ConfigEntry{{Key: "agent/ops/a.own"}}}
	deleted, pruned, err := DeleteAgentConfigEntries(fake.ports(), "ops", false)
	if err != nil || deleted != 0 || pruned != 0 || len(fake.deleted) != 0 {
		t.Errorf("delete off = %d/%d/%v, want nothing", deleted, pruned, fake.deleted)
	}
}

func TestDeleteAgentWorkspace(t *testing.T) {
	root := t.TempDir()
	fake := &stateFake{root: root, exists: true, isDir: true}
	n, err := DeleteAgentWorkspace(fake.ports(), core.Profile{Slug: "ops", Workdir: "ops"}, true)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 1 || len(fake.removed) != 1 {
		t.Errorf("deleted = %d, removed = %v, want the workspace gone", n, fake.removed)
	}
}

// A path outside the workspace root is refused.
func TestDeleteAgentWorkspace_RefusesUnsafePaths(t *testing.T) {
	root := t.TempDir()
	fake := &stateFake{root: root, exists: true, isDir: true}
	outside := filepath.Join("..", "elsewhere")
	if _, err := DeleteAgentWorkspace(fake.ports(), core.Profile{Slug: "ops", Workdir: outside}, true); err == nil {
		t.Error("err = nil, want the unsafe path refused")
	}
	if len(fake.removed) != 0 {
		t.Errorf("removed = %v, want nothing removed", fake.removed)
	}
}

// The workspace root itself is never deleted.
func TestDeleteAgentWorkspace_RefusesTheRoot(t *testing.T) {
	root := t.TempDir()
	fake := &stateFake{root: root, exists: true, isDir: true}
	if _, err := DeleteAgentWorkspace(fake.ports(), core.Profile{Slug: "ops", Workdir: "."}, true); err == nil {
		t.Error("err = nil, want the root refused")
	}
	if len(fake.removed) != 0 {
		t.Errorf("removed = %v, want nothing removed", fake.removed)
	}
}

func TestDeleteAgentWorkspace_MissingWorkspaceIsNothingToDo(t *testing.T) {
	root := t.TempDir()
	fake := &stateFake{root: root, exists: false}
	n, err := DeleteAgentWorkspace(fake.ports(), core.Profile{Slug: "ops", Workdir: "ops"}, true)
	if err != nil || n != 0 {
		t.Errorf("delete = %d/%v, want nothing to do", n, err)
	}
}

func TestDeleteAgentWorkspace_RefusesANonDirectory(t *testing.T) {
	root := t.TempDir()
	fake := &stateFake{root: root, exists: true, isDir: false}
	if _, err := DeleteAgentWorkspace(fake.ports(), core.Profile{Slug: "ops", Workdir: "ops"}, true); err == nil {
		t.Error("err = nil, want the non-directory refused")
	}
}

func TestDeleteAgentWorkspace_NoWorkdirIsNothingToDo(t *testing.T) {
	root := t.TempDir()
	fake := &stateFake{root: root, exists: true, isDir: true}
	n, err := DeleteAgentWorkspace(fake.ports(), core.Profile{Slug: "ops"}, true)
	if err != nil || n != 0 || len(fake.removed) != 0 {
		t.Errorf("delete = %d/%v, want nothing to do", n, err)
	}
}

func TestDeleteAgentWorkspace_OffDoesNothing(t *testing.T) {
	root := t.TempDir()
	fake := &stateFake{root: root, exists: true, isDir: true}
	n, err := DeleteAgentWorkspace(fake.ports(), core.Profile{Slug: "ops", Workdir: "ops"}, false)
	if err != nil || n != 0 || len(fake.removed) != 0 {
		t.Errorf("delete off = %d/%v, want nothing", n, err)
	}
}
