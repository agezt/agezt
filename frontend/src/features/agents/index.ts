// features/agents — Day 10+11 fifth feature carve-out (split: Day 10
// covered the lib + components + 3 view pages; Day 11 moved Roster +
// roster/ subdir in to complete the feature surface).
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
//   - The Roster management surface (Roster.tsx + roster/ subdir: the
//     1700-line agent CRUD UI — sortAgentRoster, filterAgentRoster,
//     NewAgentForm, profileFields, guardian noise/safety helpers,
//     removal cascade presets, lifecycle summaries, mailbox counts,
//     wake/repair issue formatters, type AgentProfile, etc.)
//   - 4 page views (Agents, ACPAgents, AgentPage, Roster) + 3
//     sub-components (AgentDetail, AgentActivity, AgentRepair —
//     AgentRepair is also used by the incidents feature; AgentDetail
//     + AgentActivity are used by the agents detail + roster pages)
//   - The agentdetail/ subdir (16 files — Overview, DiagTab, MindTab,
//     MemoryTab, ModelTab, SkillsTab, FilesTab, TriggersTab,
//     CapabilityPanel, LifecyclePanel, capability/lifecycle/comms/
//     shared/tasks — the tabbed detail panel implementation)
//
// Public surface (re-exported here so the rest of the codebase reads
// @/features/agents and never needs to know about the sub-paths):
//   - components/Agents.tsx, ACPAgents.tsx, AgentPage.tsx, Roster.tsx
//     — the 4 page-level views
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
// See docs/FRONTEND-REFACTOR-PLAN.md for the carve-out rationale.
export { Agents } from "./components/Agents";
export { ACPAgents } from "./components/ACPAgents";
export { AgentPage } from "./components/AgentPage";
export { Roster } from "./components/Roster";
export { AgentDetail } from "./components/AgentDetail";
export { AgentActivity } from "./components/AgentActivity";
export { AgentRepair } from "./components/AgentRepair";
export * from "./types";
