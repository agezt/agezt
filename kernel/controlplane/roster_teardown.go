// SPDX-License-Identifier: MIT

// Agent teardown mutation: what actually happens to each subsystem when an
// agent is retired or removed. The read-only preview of the same set lives in
// roster_cascade.go; these are the functions that delete.
// Split from roster.go (refactor Phase 3.5).

package controlplane

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/skill"
)

func (s *Server) retireAgentSubagents(parent string, children []roster.Profile, on bool) (int, []string, error) {
	if !on {
		return 0, nil, nil
	}
	retired := 0
	slugs := []string{}
	reason := "parent/owner " + parent + " removed"
	for _, child := range children {
		if child.Retired {
			continue
		}
		if _, err := s.k.SetProfileRetired(child.Slug, true, reason); err != nil {
			return retired, slugs, err
		}
		if _, err := s.pauseAgentStanding(child.Slug); err != nil {
			return retired, slugs, err
		}
		if _, err := s.pauseAgentSchedules(child.Slug); err != nil {
			return retired, slugs, err
		}
		slugs = append(slugs, child.Slug)
		retired++
	}
	sort.Strings(slugs)
	return retired, slugs, nil
}

func (s *Server) removeAgentStanding(slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	removed := 0
	for _, o := range s.k.Standing().List() {
		if strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			ok, err := s.k.RemoveStanding(o.ID)
			if err != nil {
				return removed, err
			}
			if ok {
				removed++
			}
		}
	}
	return removed, nil
}

func (s *Server) pauseAgentStanding(slug string) (int, error) {
	paused := 0
	for _, o := range s.k.Standing().List() {
		if !o.Enabled || !strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			continue
		}
		if _, err := s.k.SetStandingEnabled(o.ID, false); err != nil {
			return paused, err
		}
		paused++
	}
	return paused, nil
}

func (s *Server) countAgentPausedStanding(slug string) int {
	n := 0
	for _, o := range s.k.Standing().List() {
		if !o.Enabled && strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			n++
		}
	}
	return n
}

func (s *Server) removeAgentSchedules(slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	removed := 0
	for _, e := range s.k.Schedules().List() {
		if strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			ok, err := s.k.Schedules().Remove(e.ID)
			if err != nil {
				return removed, err
			}
			if ok {
				removed++
			}
		}
	}
	return removed, nil
}

func (s *Server) pauseAgentSchedules(slug string) (int, error) {
	paused := 0
	for _, e := range s.k.Schedules().List() {
		if !e.Enabled || !strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			continue
		}
		ok, err := s.k.Schedules().SetEnabled(e.ID, false)
		if err != nil {
			return paused, err
		}
		if ok {
			paused++
		}
	}
	return paused, nil
}

func (s *Server) countAgentPausedSchedules(slug string) int {
	n := 0
	for _, e := range s.k.Schedules().List() {
		if e.Enabled || !strings.EqualFold(strings.TrimSpace(e.Agent), slug) {
			continue
		}
		n++
	}
	return n
}

func (s *Server) forgetAgentMemory(p roster.Profile, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	scope := strings.TrimSpace(p.MemoryScope)
	if scope == "" {
		scope = p.Slug
	}
	records, err := s.k.Memory().Active()
	if err != nil {
		return 0, err
	}
	forgot := 0
	for _, r := range records {
		if !memoryRecordBelongsToAgent(r, scope) {
			continue
		}
		ok, err := s.k.Memory().Forget("", r.ID)
		if err != nil {
			return forgot, err
		}
		if ok {
			forgot++
		}
	}
	return forgot, nil
}

func memoryRecordBelongsToAgent(r memory.Record, scope string) bool {
	if r.Tags != nil && strings.EqualFold(strings.TrimSpace(r.Tags["scope"]), scope) {
		return true
	}
	return false
}

func (s *Server) forgetAgentAuthoredSharedMemory(slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	records, err := s.k.Memory().Active()
	if err != nil {
		return 0, err
	}
	forgot := 0
	for _, r := range records {
		if !memoryRecordAuthoredSharedByAgent(r, slug) {
			continue
		}
		ok, err := s.k.Memory().Forget("", r.ID)
		if err != nil {
			return forgot, err
		}
		if ok {
			forgot++
		}
	}
	return forgot, nil
}

func memoryRecordAuthoredSharedByAgent(r memory.Record, slug string) bool {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return false
	}
	if r.Tags != nil && strings.TrimSpace(r.Tags["scope"]) != "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.AddedBy), slug) || strings.EqualFold(strings.TrimSpace(r.UpdatedBy), slug)
}

func (s *Server) archiveAgentSkills(slug string, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	all, err := s.k.Forge().List()
	if err != nil {
		return 0, err
	}
	archived := 0
	for _, sk := range all {
		if !strings.EqualFold(strings.TrimSpace(sk.Agent), slug) || sk.Status == skill.StatusArchived {
			continue
		}
		if err := s.k.Forge().Archive("", sk.ID, "agent removed: "+slug); err != nil {
			if errors.Is(err, skill.ErrIllegalTransition) {
				continue
			}
			return archived, err
		}
		archived++
	}
	return archived, nil
}

func (s *Server) deleteAgentConfigEntries(slug string, on bool) (int, int, error) {
	if !on || s.k.ConfigCenter() == nil {
		return 0, 0, nil
	}
	var keys []string
	deleting := map[string]bool{}
	for _, e := range s.k.ConfigCenter().ListEntries() {
		if configEntryBelongsToAgent(e, slug) {
			keys = append(keys, e.Key)
			deleting[e.Key] = true
		}
	}
	sort.Strings(keys)
	deleted := 0
	for _, key := range keys {
		if err := s.k.ConfigCenter().Delete(key); err != nil {
			return deleted, 0, err
		}
		deleted++
	}
	pruned := 0
	for _, e := range s.k.ConfigCenter().ListEntries() {
		if e == nil || deleting[e.Key] || !pruneConfigEntryAgentAccess(e, slug) {
			continue
		}
		if err := s.k.ConfigCenter().Set(e); err != nil {
			return deleted, pruned, err
		}
		pruned++
	}
	return deleted, pruned, nil
}

func (s *Server) deleteAgentWorkspace(p roster.Profile, on bool) (int, error) {
	if !on {
		return 0, nil
	}
	workdir := strings.TrimSpace(p.Workdir)
	if workdir == "" {
		return 0, nil
	}
	root := s.agentWorkspaceRoot()
	dir, ok := confineUnder(root, workdir)
	if !ok || filepath.Clean(dir) == filepath.Clean(root) {
		return 0, fmt.Errorf("agent %s workspace path is unsafe: %s", p.Slug, workdir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("agent %s workspace is not a directory: %s", p.Slug, workdir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return 0, err
	}
	return 1, nil
}

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

func configEntryBelongsToAgent(e *configcenter.ConfigEntry, slug string) bool {
	if e == nil {
		return false
	}
	slug = strings.TrimSpace(strings.ToLower(slug))
	if slug == "" {
		return false
	}
	key := strings.TrimSpace(strings.ToLower(e.Key))
	for _, prefix := range []string{
		"agent/" + slug + "/",
		"agents/" + slug + "/",
		"agent." + slug + ".",
		"agents." + slug + ".",
	} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(e.CreatedBy), slug) {
		return true
	}
	for _, tag := range e.Tags {
		t := strings.TrimSpace(strings.ToLower(tag))
		if t == "agent:"+slug || t == "agent/"+slug || t == "owner:"+slug || t == "owner/"+slug {
			return true
		}
	}
	for _, k := range []string{"agent", "agent_slug", "owner_agent", "parent_agent"} {
		if strings.EqualFold(strings.TrimSpace(e.Metadata[k]), slug) {
			return true
		}
	}
	return false
}

// registerRosterCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
