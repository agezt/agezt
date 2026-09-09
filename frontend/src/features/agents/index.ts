// features/agents — Day 10 fourth feature carve-out (split: Day 10 covers
// the lib + components + 3 view pages; Roster + roster/ subdir stay in
// views/ for now and migrate on Day 11).
//
// Owns:
//   - Agent detail logic (agentdetail.ts: ~50 types, AgentSummary /
//     ReaperReport / AgentEscalation / MemoryRecord / ProviderRoutingRow /
//     AgentRuntimeStatus / etc., plus the helpers that fold an agent's
//     journal + runs + memory into a render-ready shape)
//   - Agent activity (agentactivity.ts: AgentRunCorrelations,
//     AgentActivityPulse, AgentActivityOperationalState — the event
//     matching that powers the activity log)
//   - Agent live updates (agentlive.ts: AgentLivePatchMap — the
//     reducer that folds AgentEvents into the live agents view)
//   - Agent nav helpers (agentnav.ts: openAgent + agentSlugFromHash —
//     the deep-link helpers that App + nav + TriggersTab + FleetNowBar
//     call to scroll into a specific agent)
//   - Agent repair flows (agentrepair.ts: RepairProfile + the
//     parseRepairProposal / applyProposal helpers used by the repair
//     wizard and the incidents feature)
//   - Agent avatar hue/initials (agent.ts: agentHue + initials — the
//     shared visual hash that AgentAvatar, Chat avatars, Overseer, etc.
//     use; AgentAvatar itself stays in components/ because it's
//     cross-feature, but the underlying hash lives here)
//   - 3 page views (Agents, ACPAgents, AgentPage) + 3 sub-components
//     (AgentDetail, AgentActivity, AgentRepair — AgentRepair is also
//     used by the incidents feature; AgentDetail + AgentActivity are
//     used by the agents detail + roster pages)
//   - The agentdetail/ subdir (16 files — Overview, DiagTab, MindTab,
//     MemoryTab, ModelTab, SkillsTab, FilesTab, TriggersTab,
//     CapabilityPanel, LifecyclePanel, capability/lifecycle/comms/
//     shared/tasks — the tabbed detail panel implementation)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/agents and never needs to know about the sub-paths):
//   - components/Agents.tsx, ACPAgents.tsx, AgentPage.tsx — the 3
//     page-level views
//   - components/AgentDetail.tsx, AgentActivity.tsx, AgentRepair.tsx
//     — the 3 sub-components also used outside the feature
//     (AgentRepair is consumed by features/incidents/components/
//     IncidentPage.tsx)
//   - types.ts — the ~30 agent shapes re-exported from lib/agentdetail
//     + lib/agentlive + lib/agentrepair
//
// The 6 lib files (agent, agentactivity, agentdetail, agentlive,
// agentnav, agentrepair) stay package-internal — they're the feature's
// business logic; only the views + sub-components + types are
// surfaced. External code reaches them via @/features/agents/.../lib/
// (allowed but not advertised).
//
// Cross-feature deps:
//   - @/app/events (AgentEvent) — the global event hook from Day 4
//   - The features/incidents/ sub-component AgentRepair —
//     features/incidents/components/IncidentPage imports AgentRepair
//     from this feature's components/.
//
// Day 11 will move views/Roster.tsx + views/roster/* into this feature
// as components/Roster.tsx + components/roster/* — they're a single
// 1700-line Roster surface that's tightly coupled to lib/agentdetail
// (it imports summarizeConfigOverrides + summarizeAgentRuntimeStatus)
// and lib/agentlive (reduceAgentLivePatchMap + applyAgentLivePatches).
// Today we hold them in views/ to keep this commit's blast radius
// small.
//
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Agents } from "./components/Agents";
export { ACPAgents } from "./components/ACPAgents";
export { AgentPage } from "./components/AgentPage";
export { AgentDetail } from "./components/AgentDetail";
export { AgentActivity } from "./components/AgentActivity";
export { AgentRepair } from "./components/AgentRepair";
export * from "./types";
