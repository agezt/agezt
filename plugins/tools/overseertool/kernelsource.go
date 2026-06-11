// SPDX-License-Identifier: MIT

package overseertool

import (
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
