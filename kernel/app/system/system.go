// SPDX-License-Identifier: MIT

// Package system owns the transport-independent daemon status/version handlers.
// Its typed output models preserve the existing daemon wire contract.
package system

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

type StatusInput struct{}
type VersionInput struct{}

// TenantCounter reports the optional daemon-wide tenant count.
type TenantCounter interface{ Count() int }

type HTTPBinding struct {
	Name     string `json:"name"`
	Addr     string `json:"addr"`
	Loopback bool   `json:"loopback"`
}
type ChannelInfo struct {
	Kind      string `json:"kind"`
	Inbound   bool   `json:"inbound"`
	Addr      string `json:"addr"`
	Allowlist int    `json:"allowlist"`
}

type Service struct {
	k            *runtime.Kernel
	tenants      TenantCounter
	httpBindings []HTTPBinding
	channels     []ChannelInfo
	credChain    string
}

// New binds current daemon metadata without depending on a transport server.
func New(k *runtime.Kernel, tenants TenantCounter, bindings []HTTPBinding, channels []ChannelInfo, credChain string) *Service {
	return &Service{k: k, tenants: tenants, httpBindings: bindings, channels: channels, credChain: credChain}
}

// fallbackCounts folds the journal's governor.fallback events (KindProviderFallback)
// into the TWO distinct fallback dimensions they now carry:
//
//   - provider fallbacks (M280): the governor moved from a primary PROVIDER to a
//     backup because the primary errored (a misconfigured/incompatible provider
//     could otherwise silently serve every run from the mock fallback).
//   - model-chain fallbacks (M706): a per-task model chain moved from one MODEL to
//     the next (scope="model-chain"). Before this split these inflated the provider
//     count with no breakdown — configurable chains you couldn't see firing.
//
// Each dimension reports its count and most-recent reason. Model-chain events are
// identified by scope; everything else is a provider fallback.
type fallbackCounts struct {
	providerCount  int
	providerLast   string
	providerLastMS int64
	modelCount     int
	modelLast      string
	modelLastMS    int64
}

func (s *Service) fallbackCounts() fallbackCounts {
	var fc fallbackCounts
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindProviderFallback {
			return nil
		}
		var p struct {
			Reason string `json:"reason"`
			Scope  string `json:"scope"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		reason := ""
		if p.Reason != "" {
			reason = strutil.Ellipsis(p.Reason, 160, "…")
		}
		// last_ms rides along with the count so a dashboard can tell a failover
		// storm happening NOW from one that ended yesterday — the counts fold the
		// whole journal and read as current without it (the stale-alarm trap).
		if p.Scope == "model-chain" {
			fc.modelCount++
			fc.modelLastMS = e.TSUnixMS
			if reason != "" {
				fc.modelLast = reason
			}
		} else {
			fc.providerCount++
			fc.providerLastMS = e.TSUnixMS
			if reason != "" {
				fc.providerLast = reason
			}
		}
		return nil
	})
	return fc
}

func (s *Service) Status(_ context.Context, _ StatusInput) (StatusOutput, error) {
	headSeq, _ := s.k.Journal().Head() // (seq, hash); hash unused here
	// Journal.Head returns -1 on an empty journal (nextSeq-1), which
	// is a leaky implementation detail for an operator dashboard.
	// Clamp to 0 so `agt status` renders "head seq=0" on a fresh
	// install rather than the confusing "-1".
	if headSeq < 0 {
		headSeq = 0
	}

	uptime := time.Since(s.k.StartTime())
	// Integer seconds — sub-second precision adds noise without
	// helping any operator workflow. Floor (not round) so a 0.9s
	// uptime renders as 0, matching "just started".
	uptimeSecs := int64(uptime / time.Second)

	// Delegation governance (M49): surface the active depth / fan-out / spend
	// ceilings (M46–M48) so an operator can see what's in effect — they were
	// silent until a delegation tripped one. 0 fan-out / spend = unbounded.
	dl := s.k.SubAgentLimits()

	// Autonomy + actionable signals (M130): how many typed schedules are armed
	// (and how many enabled), and how many HITL approvals are waiting on the
	// operator right now. Both are cheap in-memory reads. Scheduled autonomy and
	// a blocking approval queue were invisible in the at-a-glance status until now.
	schedTotal, schedEnabled, schedRunning := 0, 0, 0
	schedResident := false
	if sched := s.k.Schedules(); sched != nil {
		for _, e := range sched.List() {
			schedTotal++
			if e.Enabled {
				schedEnabled++
			}
		}
	}
	if eng := s.k.ScheduleEngine(); eng != nil {
		schedResident = true
		schedRunning = eng.RunningCount()
	}
	pendingApprovals := 0
	if ap := s.k.Approvals(); ap != nil {
		pendingApprovals = ap.PendingCount()
	}

	// Fallbacks (M280 provider + M706 model-chain): make silent primary→backup
	// fallbacks visible so a provider that errors on every request (and gets masked
	// by the always-on mock fallback) is caught at a glance instead of via a journal
	// dig — and surface per-task model-chain fallbacks as their own dimension rather
	// than conflating them into the provider count.
	fb := s.fallbackCounts()

	result := StatusOutput{
		Daemon: brand.Version, Protocol: brand.ProtocolVersion, Model: s.k.Model(),
		UptimeSeconds: uptimeSecs, Halted: s.k.IsHalted(), ActiveRuns: s.k.ActiveRuns(),
		Tools: len(s.k.Tools()), MemoryRecords: s.k.Memory().Count(),
		WorldEntities: s.k.World().Count(), ActiveSkills: s.k.Forge().Count(), JournalHead: headSeq,
		Schedules:         ScheduleStatus{Total: schedTotal, Enabled: schedEnabled, Running: schedRunning, Resident: schedResident},
		PendingApprovals:  pendingApprovals,
		ProviderFallbacks: FallbackStatus{Count: fb.providerCount, LastReason: fb.providerLast, LastMS: fb.providerLastMS},
		ModelFallbacks:    FallbackStatus{Count: fb.modelCount, LastReason: fb.modelLast, LastMS: fb.modelLastMS},
		Delegation: DelegationStatus{
			Enabled: dl.Enabled, MaxDepth: dl.MaxDepth, MaxFanout: dl.MaxFanout,
			MaxSpendMicrocents: dl.MaxSpendMicrocents, MaxTotal: dl.MaxTotal,
		},
	}
	if s.tenants != nil {
		count := s.tenants.Count()
		result.Tenants = &count
	}
	if len(s.httpBindings) > 0 {
		result.HTTPServers = append([]HTTPBinding(nil), s.httpBindings...)
	}
	if len(s.channels) > 0 {
		result.Channels = append([]ChannelInfo(nil), s.channels...)
	}
	result.CredChain = s.credChain

	return result, nil
}
func (s *Service) Version(_ context.Context, _ VersionInput) (VersionOutput, error) {
	rev, committed, modified := brand.BuildInfo()
	return VersionOutput{Version: brand.Version, ProtocolVersion: brand.ProtocolVersion, Revision: rev, Built: committed, BuildModified: modified}, nil
}
