// SPDX-License-Identifier: MIT

// Package prompt assembles a run's system prompt.
//
// It is the runs module's prompt assembly: the layer that decides WHAT a run
// knows before it starts — the live daemon identity, the learned operator
// profile, the curated taste exemplars, the recalled memory, the resolved
// world entities and the activated skills — plus the host-environment
// preamble that tells the model which machine it is acting on.
//
// The target architecture puts the run engine in the runs module (F1: the
// runtime is a god object to be dissolved). Prompt assembly is the run
// engine's first mile: it belongs to the module that owns the run, not to
// the daemon that happens to hold the stores.
//
// WHAT MOVED HERE (W3.76e): the daemon's prompt_run.go, prompt_context.go,
// prompt_environment.go, prompt_env.go, the context-selection publisher and
// the sub-agent injection half of subagent_inject.go.
//
// WHAT STAYED IN THE DAEMON, AND WHY:
//   - the prompt-injection MODE (kernel/runtime/promptinjection.go): it is
//     the platform's rule (W3.74), and the daemon keeps the name;
//   - the failure ANALYSIS (publishContextFailureAnalysis): it reads the
//     run's journal, which is the daemon's, so the daemon keeps it and asks
//     this module to publish the manifest it derives;
//   - the sub-agent SPEND walk (subAgentSpendMicrocents): it belongs to the
//     delegation family's budget rule, not to the prompt.
//
// THE PORT SET is four verbs wide, and each one is a thing this module
// cannot do for itself: the live IDENTITY (who the daemon is right now), the
// BUS (the manifests are journaled), the CLOCK (the candidates are scored
// against "now" and the preamble states today's date) — and the four STORES,
// which are the contract rows the daemon already holds (memory, taste, world
// and skills). The stores are dependencies, not host verbs: the module names
// the CONTRACT, never an implementation.
package prompt
