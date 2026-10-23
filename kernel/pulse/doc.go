// SPDX-License-Identifier: MIT

// Package pulse implements the proactive heart (SPEC-03): a second heartbeat
// that triggers itself and, on every beat, asks "what changed? · is it
// important? · should I act or tell the user?". This is what makes Agezt a
// Jarvis rather than a tool.
//
// Pulse v1 (ROADMAP §2.1 item 6 / SPEC-03 §9 MVP cut) ships the spine:
//
//	tick → ① observers → ② salience → ③ initiative → ④ briefing
//
// each stage emitting its own journaled event so `agt why` reconstructs the
// whole proactive chain. Deliberately scoped to the Phase 3 demo gate
// (unprompted CI-broken detection → brief to CLI/log → explainable →
// haltable). Wired beyond that gate since: channel brief delivery (the engine's
// BriefSink, fanned out to the Telegram/Slack/Discord/webhook/email sinks the
// daemon wires) and world-model relevance in salience (Config.Relevance, the
// SPEC-05 §3.4 boost). Still deferred: Chronos, standing orders, adaptive
// cadence, autonomous `act`, reflection.
//
// The engine owns no permissions of its own — it borrows the same bus,
// Warden, state store, and provider every other path uses (SPEC-03 §5.1).
//
// The BRIEFING rows (the brief, its dispositions and the sinks that deliver
// it) live in the contract layer (kernel/contract/pulseapi) — W3.34 — because
// the surfaces that deliver a brief are plugins, and a plugin may reach the
// contract and nothing else. The module keeps every name as an alias, so
// there is one row and one spelling of it; what stays here is the
// composition — the engine's loop, the observers, the salience rules and the
// quiet-hours window.
package pulse
