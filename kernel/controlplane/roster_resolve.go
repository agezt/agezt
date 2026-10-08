// SPDX-License-Identifier: MIT

package controlplane

// Native effect ports for the operator incident-resolution operation
// (approster.ResolveService, M833/M846): the shared-board help request and the
// overseer routing-chain update.

import (
	"fmt"
	"time"

	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/plugins/tools/overseertool"
)

type routingChainApplier interface {
	ApplyRoutingChain(ref, taskType string, targetChain []string, reason string) (overseertool.RepairResult, error)
}

// postOperatorHelp posts an operator help request to target on the shared board
// and notifies board watchers.
func (s *Server) postOperatorHelp(target, text string) (string, error) {
	st, ok := s.boardWriter()
	if !ok {
		return "", fmt.Errorf("the board is not available on this daemon")
	}
	msg, err := st.HelpRequest("operator", target, text, time.Now().UnixMilli())
	if err != nil {
		return "", err
	}
	if s.boardNotify != nil {
		s.boardNotify(msg, "")
	}
	return msg.ID, nil
}

// applyRoutingChain forces a task type's model chain through the overseer repair
// source.
func (s *Server) applyRoutingChain(slug, taskType string, chain []string, reason string) (approster.RoutingChainResult, error) {
	src, ok := overseertool.NewKernelSource(s.k, s.baseDir).(routingChainApplier)
	if !ok {
		return approster.RoutingChainResult{}, fmt.Errorf("force_chain resolution is not supported by the active repair source")
	}
	res, err := src.ApplyRoutingChain(slug, taskType, chain, reason)
	if err != nil {
		return approster.RoutingChainResult{}, err
	}
	return approster.RoutingChainResult{TaskType: res.RoutingTaskType, Chain: res.RoutingTaskModelChain, Previous: res.PreviousRoutingTaskModelChain}, nil
}
