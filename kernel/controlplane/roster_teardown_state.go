// SPDX-License-Identifier: MIT

package controlplane

// The agent teardown's state cleanup: the host's bindings for the memory,
// skill, config-center and filesystem stores the application's cleanup
// decisions act on. The DECISIONS — which records belong to the agent, which
// skills are the agent's, which config entries belong to it, which access
// references name it, whether a workspace path is safe to delete — live in
// the application layer (`kernel/app/roster/teardown_state.go`); what stays
// here is the host's data access.

import (
	"os"

	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	core "github.com/agezt/agezt/kernel/roster"
)

// teardownStatePorts binds the host's stores to the application's state
// cleanup.
func (s *Server) teardownStatePorts() approster.TeardownStatePorts {
	return approster.TeardownStatePorts{
		MemoryRecords: func() ([]memory.Record, error) { return s.k.Memory().Active() },
		ForgetMemory:  func(id string) (bool, error) { return s.k.Memory().Forget("", id) },
		Skills:        func() ([]skill.Skill, error) { return s.k.Forge().List() },
		ArchiveSkill:  func(id, reason string) error { return s.k.Forge().Archive("", id, reason) },
		ConfigEntries: func() []*configcenter.ConfigEntry {
			if s.k.ConfigCenter() == nil {
				return nil
			}
			return s.k.ConfigCenter().ListEntries()
		},
		DeleteConfig: func(key string) error {
			if s.k.ConfigCenter() == nil {
				return nil
			}
			return s.k.ConfigCenter().Delete(key)
		},
		SetConfig: func(e *configcenter.ConfigEntry) error {
			if s.k.ConfigCenter() == nil {
				return nil
			}
			return s.k.ConfigCenter().Set(e)
		},
		WorkspaceRoot: s.agentWorkspaceRoot,
		WorkspaceDir: func(dir string) (bool, bool, error) {
			info, err := os.Stat(dir)
			if err != nil {
				if os.IsNotExist(err) {
					return false, false, nil
				}
				return false, false, err
			}
			return true, info.IsDir(), nil
		},
		DeleteDir: func(dir string) error { return os.RemoveAll(dir) },
	}
}

// forgetAgentMemory forgets the agent's own memory records.
func (s *Server) forgetAgentMemory(p core.Profile, on bool) (int, error) {
	return approster.ForgetAgentMemory(s.teardownStatePorts(), p, on)
}

// forgetAgentAuthoredSharedMemory forgets the shared memory records the agent
// authored or last updated.
func (s *Server) forgetAgentAuthoredSharedMemory(slug string, on bool) (int, error) {
	return approster.ForgetAgentAuthoredSharedMemory(s.teardownStatePorts(), slug, on)
}

// archiveAgentSkills archives the agent's skills.
func (s *Server) archiveAgentSkills(slug string, on bool) (int, error) {
	return approster.ArchiveAgentSkills(s.teardownStatePorts(), slug, on)
}

// deleteAgentConfigEntries deletes the agent's config-center entries and
// prunes its access references from the entries that stay.
func (s *Server) deleteAgentConfigEntries(slug string, on bool) (int, int, error) {
	return approster.DeleteAgentConfigEntries(s.teardownStatePorts(), slug, on)
}

// deleteAgentWorkspace deletes the agent's workspace directory.
func (s *Server) deleteAgentWorkspace(p core.Profile, on bool) (int, error) {
	return approster.DeleteAgentWorkspace(s.teardownStatePorts(), p, on)
}
