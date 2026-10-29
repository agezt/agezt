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
	ports        Ports
	tenants      TenantCounter
	httpBindings []HTTPBinding
	channels     []ChannelInfo
	credChain    string
}

// New binds current daemon metadata without depending on a transport server.
//
// The overview takes PORTS, not the handle the ports were filled from: the
// daemon's report about itself is read through the questions the overview
// asks, and the host does not hand the handle to a service that only reads.
func New(ports Ports, tenants TenantCounter, bindings []HTTPBinding, channels []ChannelInfo, credChain string) *Service {
	return &Service{ports: ports, tenants: tenants, httpBindings: bindings, channels: channels, credChain: credChain}
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
	events := s.ports.Events()
	if events == nil {
		return fc
	}
	_ = events.Range(func(e *event.Event) error {
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
	// A host with no daemon has nothing to report on. The overview says so
	// instead of answering with a status full of zeros, which would read as
	// a claim about a daemon that does not exist.
	if !s.ports.Ready() {
		return StatusOutput{}, ErrNoDaemon
	}

	headSeq, _ := s.ports.Events().Head() // (seq, hash); hash unused here
	// Journal.Head returns -1 on an empty journal (nextSeq-1), which
	// is a leaky implementation detail for an operator dashboard.
	// Clamp to 0 so `agt status` renders "head seq=0" on a fresh
	// install rather than the confusing "-1".
	if headSeq < 0 {
		headSeq = 0
	}

	uptime := time.Since(s.ports.Started())
	// Integer seconds — sub-second precision adds noise without
	// helping any operator workflow. Floor (not round) so a 0.9s
	// uptime renders as 0, matching "just started".
	uptimeSecs := int64(uptime / time.Second)

	// Delegation governance (M49): surface the active depth / fan-out / spend
	// ceilings (M46–M48) so an operator can see what's in effect — they were
	// silent until a delegation tripped one. 0 fan-out / spend = unbounded.
	dl := s.ports.Delegation()

	// Autonomy + actionable signals (M130): how many typed schedules are armed
	// (and how many enabled), and how many HITL approvals are waiting on the
	// operator right now. Both are cheap in-memory reads. Scheduled autonomy and
	// a blocking approval queue were invisible in the at-a-glance status until now.
	schedTotal, schedEnabled := 0, 0
	for _, e := range s.ports.Entries() {
		schedTotal++
		if e.Enabled {
			schedEnabled++
		}
	}
	schedRunning, schedResident := s.ports.Running()
	pendingApprovals := s.ports.Pending()

	// Fallbacks (M280 provider + M706 model-chain): make silent primary→backup
	// fallbacks visible so a provider that errors on every request (and gets masked
	// by the always-on mock fallback) is caught at a glance instead of via a journal
	// dig — and surface per-task model-chain fallbacks as their own dimension rather
	// than conflating them into the provider count.
	fb := s.fallbackCounts()

	// A daemon with NO world model or NO skill library holds a nil port
	// (W3.54, W3.55): the status reports zero rather than calling into
	// nothing.
	worldEntities, activeSkills := s.ports.Entities(), s.ports.Skills()

	result := StatusOutput{
		Daemon: brand.Version, Protocol: brand.ProtocolVersion, Model: s.ports.DefaultModel(),
		UptimeSeconds: uptimeSecs, Halted: s.ports.Stopped(), ActiveRuns: s.ports.ActiveRunCount(),
		Tools: s.ports.ToolCount(), MemoryRecords: s.ports.MemoryRecordCount(),
		WorldEntities: worldEntities, ActiveSkills: activeSkills, JournalHead: headSeq,
		Schedules:         ScheduleStatus{Total: schedTotal, Enabled: schedEnabled, Running: schedRunning, Resident: schedResident},
		PendingApprovals:  pendingApprovals,
		ProviderFallbacks: FallbackStatus{Count: fb.providerCount, LastReason: fb.providerLast, LastMS: fb.providerLastMS},
		ModelFallbacks:    FallbackStatus{Count: fb.modelCount, LastReason: fb.modelLast, LastMS: fb.modelLastMS},
		Delegation:        dl,
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
