// SPDX-License-Identifier: MIT

package selfrepair

// Package documentation lives in doc.go.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
)

const (
	autoRepairPulseSubject          = "pulse.observer.system:reaper"
	autoRepairEventSubject          = "doctor.auto_repair"
	defaultAutoRepairCooldown       = 30 * time.Minute
	defaultRoutingRollbackProbation = 2 * time.Hour
	autoRepairReaperWindow          = 30 * 24 * time.Hour
)

// RepairSource is the module's view of the agent-control surface the
// auto-repair coordinator drives. The daemon injects the implementation; the
// module never imports the tool that provides it — a module reaches another
// module's api, never a plugin (archcheck's principle P5).
type RepairSource = roster.RepairSource

type autoRepairSource interface {
	RepairAgent(ref, reason string) (roster.RepairResult, error)
}

type autoRepairRoutingRollbacker interface {
	RollbackRouting(ref, taskType string, targetChain []string, reason string) (roster.RepairResult, error)
}

type autoRepairRoutingChainApplier interface {
	ApplyRoutingChain(ref, taskType string, targetChain []string, reason string) (roster.RepairResult, error)
}

// Mailbox is the message-board surface auto-repair escalations post through.
// *boardapi.Store satisfies it; a nil-tolerant caller may pass nil to disable
// mailbox escalation.
type Mailbox interface {
	HelpRequest(from, to, text string, nowMS int64) (boardapi.Message, error)
	Get(id string) (boardapi.Message, bool)
	Send(m boardapi.Message, nowMS int64) (boardapi.Message, error)
}

// WireAutoRepair subscribes the auto-repair coordinator to the reaper pulse
// subject and launches it on ctx. It returns the boot-banner status string
// ("armed (…)" or a "disabled (…)" reason) exactly as the daemon prints it.
//
// src is the agent-control surface the coordinator repairs through. The
// daemon injects it: the module asks for the repair it needs, it does not
// reach into the tool that provides it.
//
// ports are the host's capabilities the coordinator needs: the bus it
// publishes on, the journal it reads the repair history from, the fleet
// it judges, the roster it pauses, the run engine it wakes through and
// the doors it binds a run with. The daemon fills them: a module may not
// hold the kernel (archcheck's principle P5), and until W5.4 this one
// did — which was the last cross-module edge in the tree.
func WireAutoRepair(ctx context.Context, ports Ports, src RepairSource, mailbox Mailbox, postNotify func(boardapi.Message, string)) string {
	if ports.Bus == nil {
		return "disabled"
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(brand.EnvPrefix+"AUTO_REPAIR")), "off") {
		return "disabled (AGEZT_AUTO_REPAIR=off)"
	}
	sub, err := ports.Bus.Subscribe(autoRepairPulseSubject, 64)
	if err != nil {
		return fmt.Sprintf("disabled (subscribe failed: %v)", err)
	}
	coord := newAutoRepairCoordinator(autoRepairCooldown())
	go coord.run(ctx, sub, ports, src, mailbox, postNotify)
	return fmt.Sprintf("armed (%s; cooldown %s)", autoRepairPulseSubject, coord.cooldown)
}
