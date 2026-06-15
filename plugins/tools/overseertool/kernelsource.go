// SPDX-License-Identifier: MIT

package overseertool

import (
	"fmt"
	"path/filepath"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// kernelSource adapts the live *runtime.Kernel to the overseer's Source. Reads
// and interventions go straight to the kernel's own methods, so each is
// journaled and reversible exactly like its operator-driven equivalent. The
// board is opened fresh per OpenHelp read (mirroring the control plane's board
// view) so the overseer always sees the latest committed help requests without
// sharing the board tool's in-process instance.
type kernelSource struct {
	k        *kernelruntime.Kernel
	boardDir string
}

// NewKernelSource builds the kernel-backed Source. baseDir is the daemon's base
// directory; the board lives at <baseDir>/board.
func NewKernelSource(k *kernelruntime.Kernel, baseDir string) Source {
	return &kernelSource{k: k, boardDir: filepath.Join(baseDir, "board")}
}

func (s *kernelSource) IsHalted() bool         { return s.k.IsHalted() }
func (s *kernelSource) ActiveRunIDs() []string { return s.k.ActiveRunIDs() }
func (s *kernelSource) Agents() []roster.Profile {
	return s.k.Roster().List()
}
func (s *kernelSource) AgentImpact(slug string) []string { return s.k.AgentImpact(slug) }
func (s *kernelSource) CancelRun(corr string) bool       { return s.k.CancelRun(corr) }
func (s *kernelSource) Halt(reason string)               { s.k.HaltWith(reason) }
func (s *kernelSource) ResumeAll(reason string)          { s.k.ResumeWith(reason) }

func (s *kernelSource) SetAgentEnabled(ref string, enabled bool) (roster.Profile, error) {
	return s.k.SetProfileEnabled(ref, enabled)
}
func (s *kernelSource) SetAgentRetired(ref string, retired bool) (roster.Profile, error) {
	return s.k.SetProfileRetired(ref, retired)
}

// EditAgent applies `in`'s mutable fields wholesale to the agent named by ref,
// mirroring the control plane's handleAgentEdit (the same whitelist the webui
// uses). Identity/lifecycle fields (id/slug/enabled/retired) and the System flag
// are NOT touched — a guardian can retune another agent but can't resurrect,
// rename, or promote it to a protected guardian.
func (s *kernelSource) EditAgent(ref string, in roster.Profile) (roster.Profile, error) {
	p, found, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		dst.Name = in.Name
		dst.Soul = in.Soul
		dst.Model = in.Model
		dst.Fallbacks = in.Fallbacks
		dst.TaskType = in.TaskType
		dst.MaxCostMc = in.MaxCostMc
		dst.MaxDailyMc = in.MaxDailyMc
		dst.MemoryScope = in.MemoryScope
		dst.Workdir = in.Workdir
		dst.Description = in.Description
	})
	if err != nil {
		return roster.Profile{}, err
	}
	if !found {
		return roster.Profile{}, fmt.Errorf("unknown agent: %s", ref)
	}
	return p, nil
}

// CreateAgent adds a brand-new agent. System is forced off — only boot-time
// guardian seeding may mint a protected agent.
func (s *kernelSource) CreateAgent(in roster.Profile) (roster.Profile, error) {
	in.System = false
	return s.k.AddProfile(in)
}

// OpenHelp opens the board fresh and returns its open help requests. A failure
// to open (no board yet) yields an empty list rather than an error — the
// overseer should still report on everything else.
func (s *kernelSource) OpenHelp(limit int) []board.Message {
	st, err := board.Open(s.boardDir)
	if err != nil {
		return nil
	}
	return st.OpenHelp(limit)
}
