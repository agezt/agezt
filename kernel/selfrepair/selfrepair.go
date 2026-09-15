// SPDX-License-Identifier: MIT

// Package selfrepair wires the deterministic doctor/auto-repair coordinator
// (Phase 2.6 extraction from cmd/agezt): it subscribes to the reaper pulse
// observer, claims broken/degraded/routing-unstable agents, drives the
// overseertool repair source, and escalates through the mailbox + wake chain
// when a repair fails. The daemon arms it once at boot via WireAutoRepair.
//
// Import posture: selfrepair imports kernel/runtime and (like
// kernel/controlplane) the overseertool plugin as its repair source; nothing in
// kernel/runtime may ever import selfrepair.
package selfrepair

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/board"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/tools/overseertool"
)

const (
	autoRepairPulseSubject          = "pulse.observer.system:reaper"
	autoRepairEventSubject          = "doctor.auto_repair"
	defaultAutoRepairCooldown       = 30 * time.Minute
	defaultRoutingRollbackProbation = 2 * time.Hour
	autoRepairReaperWindow          = 30 * 24 * time.Hour
)
type autoRepairSource interface {
	RepairAgent(ref, reason string) (overseertool.RepairResult, error)
}

type autoRepairRoutingRollbacker interface {
	RollbackRouting(ref, taskType string, targetChain []string, reason string) (overseertool.RepairResult, error)
}

type autoRepairRoutingChainApplier interface {
	ApplyRoutingChain(ref, taskType string, targetChain []string, reason string) (overseertool.RepairResult, error)
}
// Mailbox is the message-board surface auto-repair escalations post through.
// *board.Store satisfies it; a nil-tolerant caller may pass nil to disable
// mailbox escalation.
type Mailbox interface {
	HelpRequest(from, to, text string, nowMS int64) (board.Message, error)
	Get(id string) (board.Message, bool)
	Send(m board.Message, nowMS int64) (board.Message, error)
}
// WireAutoRepair subscribes the auto-repair coordinator to the reaper pulse
// subject and launches it on ctx. It returns the boot-banner status string
// ("armed (…)" or a "disabled (…)" reason) exactly as the daemon prints it.
func WireAutoRepair(ctx context.Context, k *kernelruntime.Kernel, baseDir string, mailbox Mailbox, postNotify func(board.Message, string)) string {
	if k == nil || k.Bus() == nil {
		return "disabled"
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(brand.EnvPrefix+"AUTO_REPAIR")), "off") {
		return "disabled (AGEZT_AUTO_REPAIR=off)"
	}
	sub, err := k.Bus().Subscribe(autoRepairPulseSubject, 64)
	if err != nil {
		return fmt.Sprintf("disabled (subscribe failed: %v)", err)
	}
	coord := newAutoRepairCoordinator(autoRepairCooldown())
	src := overseertool.NewKernelSource(k, baseDir)
	go coord.run(ctx, sub, k, src, mailbox, postNotify)
	return fmt.Sprintf("armed (%s; cooldown %s)", autoRepairPulseSubject, coord.cooldown)
}
