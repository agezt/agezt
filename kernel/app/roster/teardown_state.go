// SPDX-License-Identifier: MIT

package roster

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/configcenter"
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/kernel/platform/fileworkspace"
)

// TeardownStatePorts are the host's capabilities the agent teardown's state
// cleanup needs. The application owns the DECISIONS (which memory records
// belong to the agent, which skills are the agent's, which config entries
// belong to it, which access references name it, whether a workspace path is
// safe to delete); the host owns the DATA (the stores and the filesystem).
type TeardownStatePorts struct {
	// MemoryRecords lists the active memory records.
	MemoryRecords func() ([]memory.Record, error)
	// ForgetMemory forgets a memory record; it reports whether it existed.
	ForgetMemory func(id string) (bool, error)
	// Skills lists the agent's skills.
	Skills func() ([]skill.Skill, error)
	// ArchiveSkill archives a skill with the given reason.
	ArchiveSkill func(id, reason string) error
	// ConfigEntries lists the config-center entries.
	ConfigEntries func() []*configcenter.ConfigEntry
	// DeleteConfig deletes a config-center entry by key.
	DeleteConfig func(key string) error
	// SetConfig writes a config-center entry back.
	SetConfig func(e *configcenter.ConfigEntry) error
	// WorkspaceRoot is the root every agent workspace lives under.
	WorkspaceRoot func() string
	// WorkspaceDir reports whether the directory exists and whether it is a
	// directory.
	WorkspaceDir func(dir string) (exists bool, isDir bool, err error)
	// DeleteDir removes a directory tree.
	DeleteDir func(dir string) error
}

// ForgetAgentMemory forgets the agent's own memory records — the ones whose
// scope tag is the agent's memory scope (the slug when it has none).
func ForgetAgentMemory(ports TeardownStatePorts, p core.Profile, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	scope := strings.TrimSpace(p.MemoryScope)
	if scope == "" {
		scope = p.Slug
	}
	records, err := ports.MemoryRecords()
	if err != nil {
		return 0, err
	}
	forgot := 0
	for _, r := range records {
		if !MemoryRecordBelongsToAgent(r, scope) {
			continue
		}
		ok, err := ports.ForgetMemory(r.ID)
		if err != nil {
			return forgot, err
		}
		if ok {
			forgot++
		}
	}
	return forgot, nil
}

// MemoryRecordBelongsToAgent reports whether the record carries the agent's
// scope tag.
func MemoryRecordBelongsToAgent(r memory.Record, scope string) bool {
	if r.Tags != nil && strings.EqualFold(strings.TrimSpace(r.Tags["scope"]), scope) {
		return true
	}
	return false
}

// ForgetAgentAuthoredSharedMemory forgets the unscoped memory records the
// agent authored or last updated.
func ForgetAgentAuthoredSharedMemory(ports TeardownStatePorts, slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	records, err := ports.MemoryRecords()
	if err != nil {
		return 0, err
	}
	forgot := 0
	for _, r := range records {
		if !MemoryRecordAuthoredSharedByAgent(r, slug) {
			continue
		}
		ok, err := ports.ForgetMemory(r.ID)
		if err != nil {
			return forgot, err
		}
		if ok {
			forgot++
		}
	}
	return forgot, nil
}

// MemoryRecordAuthoredSharedByAgent reports whether the record is a shared
// one the agent authored or last updated.
func MemoryRecordAuthoredSharedByAgent(r memory.Record, slug string) bool {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return false
	}
	if r.Tags != nil && strings.TrimSpace(r.Tags["scope"]) != "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.AddedBy), slug) || strings.EqualFold(strings.TrimSpace(r.UpdatedBy), slug)
}

// ArchiveAgentSkills archives the agent's skills. A skill that refuses the
// transition is left alone.
func ArchiveAgentSkills(ports TeardownStatePorts, slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	all, err := ports.Skills()
	if err != nil {
		return 0, err
	}
	archived := 0
	for _, sk := range all {
		if !strings.EqualFold(strings.TrimSpace(sk.Agent), slug) || sk.Status == skill.StatusArchived {
			continue
		}
		if err := ports.ArchiveSkill(sk.ID, "agent removed: "+slug); err != nil {
			if errors.Is(err, skill.ErrIllegalTransition) {
				continue
			}
			return archived, err
		}
		archived++
	}
	return archived, nil
}

// DeleteAgentConfigEntries deletes the config-center entries that belong to
// the agent and prunes the agent's access references from the entries that
// stay. It reports the deletions and the prunes.
func DeleteAgentConfigEntries(ports TeardownStatePorts, slug string, on bool) (int, int, error) {
	if !on || ports.ConfigEntries == nil {
		return 0, 0, nil
	}
	var keys []string
	deleting := map[string]bool{}
	for _, e := range ports.ConfigEntries() {
		if ConfigEntryBelongsToAgent(e, slug) {
			keys = append(keys, e.Key)
			deleting[e.Key] = true
		}
	}
	sort.Strings(keys)
	deleted := 0
	for _, key := range keys {
		if err := ports.DeleteConfig(key); err != nil {
			return deleted, 0, err
		}
		deleted++
	}
	pruned := 0
	for _, e := range ports.ConfigEntries() {
		if e == nil || deleting[e.Key] || !pruneConfigEntryAgentAccess(e, slug) {
			continue
		}
		if err := ports.SetConfig(e); err != nil {
			return deleted, pruned, err
		}
		pruned++
	}
	return deleted, pruned, nil
}

// DeleteAgentWorkspace deletes the agent's workspace directory. The path must
// live under the workspace root and must be a directory; anything else is an
// error, and a path that is already gone is nothing to do.
func DeleteAgentWorkspace(ports TeardownStatePorts, p core.Profile, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	workdir := strings.TrimSpace(p.Workdir)
	if workdir == "" {
		return 0, nil
	}
	root := ports.WorkspaceRoot()
	dir, ok := fileworkspace.ConfineUnder(root, workdir)
	if !ok || filepath.Clean(dir) == filepath.Clean(root) {
		return 0, fmt.Errorf("agent %s workspace path is unsafe: %s", p.Slug, workdir)
	}
	exists, isDir, err := ports.WorkspaceDir(dir)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	if !isDir {
		return 0, fmt.Errorf("agent %s workspace is not a directory: %s", p.Slug, workdir)
	}
	if err := ports.DeleteDir(dir); err != nil {
		return 0, err
	}
	return 1, nil
}

// pruneConfigEntryAgentAccess removes the agent's access reference from the
// entry's allowed and excluded lists and reports whether either changed.
func pruneConfigEntryAgentAccess(e *configcenter.ConfigEntry, slug string) bool {
	if e == nil {
		return false
	}
	nextAllowed, allowedChanged := removeAgentAccessRef(e.AllowedAgents, slug)
	nextExcluded, excludedChanged := removeAgentAccessRef(e.ExcludedAgents, slug)
	if allowedChanged {
		e.AllowedAgents = nextAllowed
	}
	if excludedChanged {
		e.ExcludedAgents = nextExcluded
	}
	return allowedChanged || excludedChanged
}

// removeAgentAccessRef removes the agent's reference from the list and
// reports whether it was there.
func removeAgentAccessRef(values []string, slug string) ([]string, bool) {
	slug = strings.TrimSpace(slug)
	if slug == "" || len(values) == 0 {
		return values, false
	}
	out := values[:0]
	changed := false
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), slug) {
			changed = true
			continue
		}
		out = append(out, value)
	}
	if !changed {
		return values, false
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}
