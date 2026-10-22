// SPDX-License-Identifier: MIT

// Package types is the contract-layer home for the value types that cross
// the boundary between the host (the runtime and its accessors) and the
// application layer.
//
// It was extracted from kernel/runtime/types so that the application layer
// can name the same type the host does without importing the runtime. The
// runtime sits in a module (runs) whose internals the application layer must
// not reach into, and the host must not import the application layer either
// — the edge points the wrong way (architecture/20-target-architecture.md,
// P1 and P5). A shared VALUE type carries no behaviour to move, so the
// contract layer — pure types and interfaces, no logic (P4) — is the right
// home for it.
//
// The three types here are:
//
//   - CouncilMember: one seat of the Council of Elders (the seat and the
//     model that speaks for it). The application's council row is an alias
//     of this type, so the host hands its membership over without copying.
//   - PluginInfo: the manifest entry of one external plugin. The
//     application's plugin registration is an alias of this type.
//   - SubAgentLimits: the delegation-governance ceilings (M46–M48).
//
// Rules of the road:
//
//   - Types only. No logic and no imports beyond the standard library: a
//     contract package that pulls in behaviour is no longer a contract.
//   - One type, one home. A value type that both the host and the
//     application name is declared here exactly once; the other side
//     aliases it, so the two can never drift apart.
//   - The runtime's own aliases (runtime.CouncilMember, runtime.PluginInfo,
//     runtime.SubAgentLimits) stay, so the host's public API is unchanged.
package types
