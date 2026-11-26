// The browser's payload views are pinned to the daemon's catalog.
//
// The daemon's rows live in one place — the generated catalog
// (`types.generated.ts`, rendered from the op registry). A browser view that
// reads a daemon row NAMING its fields by hand is a second contract: the
// daemon renames `history` to `events` and the browser keeps reading
// `history`, silently, until a panel goes blank.
//
// So every daemon field a browser view names is pinned here, at compile
// time: the assertion below PICKs the field out of the daemon's row, and a
// `Pick` on a field the daemon no longer has is a compile error. The browser
//'s own vocabulary — the badge summaries it builds, the tabs it draws, the
// fleet it models — is listed in the ledger below, and a ledger entry that
// stops being the browser's own is an error too.
//
// The runtime test below is the other tooth: a view that hand-names a daemon
// field without a pin fails here, so new debt cannot land; a pin whose
// fields are no longer hand-named fails too, because a stale pin is a lie
// about the code.
//
// Regenerate the daemon's rows with `go run ./tools/openapi`.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import type {
  AgentCapabilitiesResponsePermissionsItem,
  AgentImpactResponse,
  AgentListResponseProfilesItem,
  AgentRepairStatusResponse,
  ArtifactCollectResponseCandidatesItem,
  ArtifactListResponseEntriesItem,
  AttentionResponseItemsItem,
  AutonomyFeedResponseItemsItem,
  CatalogListResponseProvidersItem,
  CatalogListResponseProvidersItemModelsItem,
  ChannelListResponseChannelsItem,
  ConfigSchemaResponseSectionsItem,
  CouncilAskResponse,
  CouncilAskResponseOpinionsItem,
  DataCollectionsResponseCollectionsItem,
  ExecutionProfileCheckResponse,
  ExecutionProfilesResponse,
  InboxResponseThreadsItem,
  JournalGrepResponseEventsItem,
  MemoryListResponseRecordsItem,
  NodeRegistryResponseNodesItem,
  ReaperScanResponse,
  RunResponse,
  RunsListResponseRunsItem,
  ToolLogResponseInvocationsItem,
  WorkflowDraftResponseWorkflow,
  WorkflowRunsResponseRunsItem,
  WorkflowTemplatesResponseTemplatesItem,
} from "@/app/api/types.generated";
import type { DelegationData } from "@/components/DelegationGraph";
import type {
  LiveRun,
  PayloadIntent,
} from "@/components/FleetNowBar";
import type { LLMCall } from "@/components/Inspector";
import type {
  RemoteArtifact,
  RollbackCheckpoint,
  RunPhaseStep,
} from "@/components/RunDetail";
import type { Section } from "@/components/configcenter/page";
import type {
  FleetLiveEvent,
  RootSummary,
} from "@/features/agents/components/Agents";
import type { EffectiveToolPermission } from "@/features/agents/components/agentdetail/capability";
import type { AgentRemovalPlanInput } from "@/features/agents/components/roster/removal";
import type {
  AgentLivePatch,
  AgentLiveTarget,
} from "@/features/agents/lib/agentlive";
import type { ChannelRow } from "@/features/channels/components/types";
import type { CouncilRun } from "@/features/council/lib/council";
import type { Collection } from "@/features/data/components/page";
import type {
  ExecutionProfileHealthReport,
  ExecutionProfileInventory,
} from "@/features/execution-profiles/components/types";
import type {
  JournalEvent as AnalystJournalEvent,
} from "@/features/knowledge/components/Analyst";
import type {
  JournalEvent as ReflectJournalEvent,
  MemoryEvent,
} from "@/features/knowledge/components/Reflect";
import type { Provider } from "@/features/models/components/page";
import type {
  Wf,
  WfRun,
  WfTemplate,
} from "@/features/workflows/components/types";
import type { ActiveRun } from "@/lib/activity";
import type {
  AgentLastAutonomyRunbook,
  AgentRepairStatus,
  ReaperReport,
} from "@/lib/agentdetail";
import type { RepairProfile } from "@/lib/agentrepair";
import type { AgentProfile } from "@/lib/agentshared";
import type { InboxThread } from "@/lib/channelSessions";
import type {
  ChatFrame,
  ChatTool,
  ChatTurn,
} from "@/lib/chat";
import type { ConductorStep } from "@/lib/conductor";
import type { RunNode } from "@/lib/delegation";
import type { ApiProfile } from "@/lib/fleet";
import type {
  IncidentDelegateCandidate,
  IncidentProfileLite,
  IncidentResolutionRow,
} from "@/lib/incidents";
import type { LiveRunContext } from "@/lib/liveruncontext";
import type { ModelOption } from "@/lib/models";
import type { ToolCall } from "@/lib/rundetail";

// ── the vocabulary of a pin ───────────────────────────────────────────────
/** Normalize drops the null and undefined arms: the daemon may send null and
 * the browser may read the field as absent — that is the browser's leniency,
 * not a different field. */
type Normalize<T> = Exclude<T, null | undefined>;
/** Comparable says the two types can stand for each other: one may narrow
 * the other (the browser may promise a subset), but they may not be
 * different shapes. */
type Comparable<A, B> = [Normalize<A>] extends [Normalize<B>]
  ? true
  : ([Normalize<B>] extends [Normalize<A>] ? true : false);
/** FieldsComparable compares the daemon's fields with the browser's, one by
 * one. */
type FieldsComparable<A, B> = {
  [K in keyof A]-?: K extends keyof B ? Comparable<A[K], B[K]> : false;
}[keyof A];
/** AllTrue says every field of a comparison held. */
type AllTrue<T> = [T] extends [true] ? true : false;
/** Expect fails the typecheck when its argument is not true. */
type Expect<T extends true> = T;

// ── the pins ──────────────────────────────────────────────────────────────
// @pin components/DelegationGraph.tsx:DelegationData | RunsListResponseRunsItem | pinned=status,model,iters | own=
type _pin_DelegationData = Expect<
  AllTrue<
    FieldsComparable<
      Pick<RunsListResponseRunsItem, "status" | "model" | "iters">,
      Pick<DelegationData, "status" | "model" | "iters">
    >
  >
>;

// @pin components/FleetNowBar.tsx:LiveRun | RunsListResponseRunsItem | pinned=agent,intent,phase,tool,model | own=
type _pin_LiveRun = Expect<
  AllTrue<
    FieldsComparable<
      Pick<RunsListResponseRunsItem, "agent" | "intent" | "phase" | "tool" | "model">,
      Pick<LiveRun, "agent" | "intent" | "phase" | "tool" | "model">
    >
  >
>;

// @pin components/FleetNowBar.tsx:PayloadIntent | RunsListResponseRunsItem | pinned=intent,agent,tool,model,phase,reason | own=
type _pin_PayloadIntent = Expect<
  AllTrue<
    FieldsComparable<
      Pick<RunsListResponseRunsItem, "intent" | "agent" | "tool" | "model" | "phase" | "reason">,
      Pick<PayloadIntent, "intent" | "agent" | "tool" | "model" | "phase" | "reason">
    >
  >
>;

// @pin components/Inspector.tsx:LLMCall | NodeRegistryResponseNodesItem | pinned=id,model,status,error | own=
type _pin_LLMCall = Expect<
  AllTrue<
    FieldsComparable<
      Pick<NodeRegistryResponseNodesItem, "id" | "model" | "status" | "error">,
      Pick<LLMCall, "id" | "model" | "status" | "error">
    >
  >
>;

// @pin components/RunDetail.tsx:RemoteArtifact | ArtifactListResponseEntriesItem | pinned=id,ref,name,mime,kind,source,size | own=
type _pin_RemoteArtifact = Expect<
  AllTrue<
    FieldsComparable<
      Pick<ArtifactListResponseEntriesItem, "id" | "ref" | "name" | "mime" | "kind" | "source" | "size">,
      Pick<RemoteArtifact, "id" | "ref" | "name" | "mime" | "kind" | "source" | "size">
    >
  >
>;

// @pin components/RunDetail.tsx:RollbackCheckpoint | ArtifactCollectResponseCandidatesItem | pinned=id,kind,created_ms | own=
type _pin_RollbackCheckpoint = Expect<
  AllTrue<
    FieldsComparable<
      Pick<ArtifactCollectResponseCandidatesItem, "id" | "kind" | "created_ms">,
      Pick<RollbackCheckpoint, "id" | "kind" | "created_ms">
    >
  >
>;

// @pin components/RunDetail.tsx:RunPhaseStep | AutonomyFeedResponseItemsItem | pinned=kind,phase,detail | own=
type _pin_RunPhaseStep = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AutonomyFeedResponseItemsItem, "kind" | "phase" | "detail">,
      Pick<RunPhaseStep, "kind" | "phase" | "detail">
    >
  >
>;

// @pin components/configcenter/page.tsx:Section | ConfigSchemaResponseSectionsItem | pinned=fields | own=
type _pin_Section = Expect<
  AllTrue<
    FieldsComparable<
      Pick<ConfigSchemaResponseSectionsItem, "fields">,
      Pick<Section, "fields">
    >
  >
>;

// @pin features/agents/components/Agents.tsx:FleetLiveEvent | AttentionResponseItemsItem | pinned=id,kind,ts | own=
type _pin_FleetLiveEvent = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AttentionResponseItemsItem, "id" | "kind" | "ts">,
      Pick<FleetLiveEvent, "id" | "kind" | "ts">
    >
  >
>;

// @pin features/agents/components/Agents.tsx:RootSummary | RunsListResponseRunsItem | pinned=intent,status,model,iters | own=
type _pin_RootSummary = Expect<
  AllTrue<
    FieldsComparable<
      Pick<RunsListResponseRunsItem, "intent" | "status" | "model" | "iters">,
      Pick<RootSummary, "intent" | "status" | "model" | "iters">
    >
  >
>;

// @pin features/agents/components/agentdetail/capability.tsx:EffectiveToolPermission | AgentCapabilitiesResponsePermissionsItem | pinned= | own=
type _pin_EffectiveToolPermission = Expect<true>;

// @pin features/agents/lib/agentlive.ts:AgentLivePatch | AgentListResponseProfilesItem | pinned=enabled,retired,status | own=
type _pin_AgentLivePatch = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AgentListResponseProfilesItem, "enabled" | "retired" | "status">,
      Pick<AgentLivePatch, "enabled" | "retired" | "status">
    >
  >
>;

// @pin features/agents/lib/agentlive.ts:AgentLiveTarget | AgentListResponseProfilesItem | pinned=status | own=
type _pin_AgentLiveTarget = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AgentListResponseProfilesItem, "status">,
      Pick<AgentLiveTarget, "status">
    >
  >
>;

// @pin features/agents/components/roster/removal.ts:AgentRemovalPlanInput | AgentImpactResponse | pinned=schedules,memories,skills,configs,workspaces,subagents | own=
type _pin_AgentRemovalPlanInput = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AgentImpactResponse, "schedules" | "memories" | "skills" | "configs" | "workspaces" | "subagents">,
      Pick<AgentRemovalPlanInput, "schedules" | "memories" | "skills" | "configs" | "workspaces" | "subagents">
    >
  >
>;

// @pin features/channels/components/types.ts:ChannelRow | ChannelListResponseChannelsItem | pinned=media,probe,fields,accounts | own=
type _pin_ChannelRow = Expect<
  AllTrue<
    FieldsComparable<
      Pick<ChannelListResponseChannelsItem, "media" | "probe" | "fields" | "accounts">,
      Pick<ChannelRow, "media" | "probe" | "fields" | "accounts">
    >
  >
>;

// @pin features/council/lib/council.ts:CouncilRun | CouncilAskResponse | pinned=question,rounds,opinions,consensus,dissent,brief | own=
type _pin_CouncilRun = Expect<
  AllTrue<
    FieldsComparable<
      Pick<CouncilAskResponse, "question" | "rounds" | "opinions" | "consensus" | "dissent" | "brief">,
      Pick<CouncilRun, "question" | "rounds" | "opinions" | "consensus" | "dissent" | "brief">
    >
  >
>;

// @pin features/data/components/page.tsx:Collection | DataCollectionsResponseCollectionsItem | pinned=fields | own=
type _pin_Collection = Expect<
  AllTrue<
    FieldsComparable<
      Pick<DataCollectionsResponseCollectionsItem, "fields">,
      Pick<Collection, "fields">
    >
  >
>;

// @pin features/execution-profiles/components/types.ts:ExecutionProfileHealthReport | ExecutionProfileCheckResponse | pinned=checks | own=
type _pin_ExecutionProfileHealthReport = Expect<
  AllTrue<
    FieldsComparable<
      Pick<ExecutionProfileCheckResponse, "checks">,
      Pick<ExecutionProfileHealthReport, "checks">
    >
  >
>;

// @pin features/execution-profiles/components/types.ts:ExecutionProfileInventory | ExecutionProfilesResponse | pinned=profiles | own=
type _pin_ExecutionProfileInventory = Expect<
  AllTrue<
    FieldsComparable<
      Pick<ExecutionProfilesResponse, "profiles">,
      Pick<ExecutionProfileInventory, "profiles">
    >
  >
>;

// @pin features/knowledge/components/Analyst.tsx:JournalEvent | JournalGrepResponseEventsItem | pinned=kind,actor,subject,correlation_id | own=
type _pin_AnalystJournalEvent = Expect<
  AllTrue<
    FieldsComparable<
      Pick<JournalGrepResponseEventsItem, "kind" | "actor" | "subject" | "correlation_id">,
      Pick<AnalystJournalEvent, "kind" | "actor" | "subject" | "correlation_id">
    >
  >
>;

// @pin features/knowledge/components/Reflect.tsx:JournalEvent | JournalGrepResponseEventsItem | pinned=kind,actor,subject | own=
type _pin_ReflectJournalEvent = Expect<
  AllTrue<
    FieldsComparable<
      Pick<JournalGrepResponseEventsItem, "kind" | "actor" | "subject">,
      Pick<ReflectJournalEvent, "kind" | "actor" | "subject">
    >
  >
>;

// @pin features/knowledge/components/Reflect.tsx:MemoryEvent | MemoryListResponseRecordsItem | pinned=id,subject,type,content,confidence,evidence,superseded_by | own=
type _pin_MemoryEvent = Expect<
  AllTrue<
    FieldsComparable<
      Pick<MemoryListResponseRecordsItem, "id" | "subject" | "type" | "content" | "confidence" | "evidence" | "superseded_by">,
      Pick<MemoryEvent, "id" | "subject" | "type" | "content" | "confidence" | "evidence" | "superseded_by">
    >
  >
>;

// @pin features/models/components/page.tsx:Provider | CatalogListResponseProvidersItem | pinned=models | own=
type _pin_Provider = Expect<
  AllTrue<
    FieldsComparable<
      Pick<CatalogListResponseProvidersItem, "models">,
      Pick<Provider, "models">
    >
  >
>;

// @pin features/workflows/components/types.ts:Wf | WorkflowDraftResponseWorkflow | pinned=nodes,edges | own=
type _pin_Wf = Expect<
  AllTrue<
    FieldsComparable<
      Pick<WorkflowDraftResponseWorkflow, "nodes" | "edges">,
      Pick<Wf, "nodes" | "edges">
    >
  >
>;

// @pin features/workflows/components/types.ts:WfRun | WorkflowRunsResponseRunsItem | pinned=node_events | own=
type _pin_WfRun = Expect<
  AllTrue<
    FieldsComparable<
      Pick<WorkflowRunsResponseRunsItem, "node_events">,
      Pick<WfRun, "node_events">
    >
  >
>;

// @pin features/workflows/components/types.ts:WfTemplate | WorkflowTemplatesResponseTemplatesItem | pinned=workflow | own=
type _pin_WfTemplate = Expect<
  AllTrue<
    FieldsComparable<
      Pick<WorkflowTemplatesResponseTemplatesItem, "workflow">,
      Pick<WfTemplate, "workflow">
    >
  >
>;

// @pin lib/activity.ts:ActiveRun | RunsListResponseRunsItem | pinned=intent,status,iters | own=
type _pin_ActiveRun = Expect<
  AllTrue<
    FieldsComparable<
      Pick<RunsListResponseRunsItem, "intent" | "status" | "iters">,
      Pick<ActiveRun, "intent" | "status" | "iters">
    >
  >
>;

// @pin lib/agentdetail.ts:AgentLastAutonomyRunbook | WorkflowRunsResponseRunsItem | pinned=source,schedule_id,standing_id,trigger_subject,parent_correlation_id,correlation_id | own=
type _pin_AgentLastAutonomyRunbook = Expect<
  AllTrue<
    FieldsComparable<
      Pick<WorkflowRunsResponseRunsItem, "source" | "schedule_id" | "standing_id" | "trigger_subject" | "parent_correlation_id" | "correlation_id">,
      Pick<AgentLastAutonomyRunbook, "source" | "schedule_id" | "standing_id" | "trigger_subject" | "parent_correlation_id" | "correlation_id">
    >
  >
>;

// @pin lib/agentdetail.ts:AgentRepairStatus | AgentRepairStatusResponse | pinned=history,inflight,latest,contract,next_action | own=
type _pin_AgentRepairStatus = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AgentRepairStatusResponse, "history" | "inflight" | "latest" | "contract" | "next_action">,
      Pick<AgentRepairStatus, "history" | "inflight" | "latest" | "contract" | "next_action">
    >
  >
>;

// @pin lib/agentdetail.ts:ReaperReport | ReaperScanResponse | pinned=dead_agents,degraded_agents,misconfigured_agents,routing_pressure_agents,routing_forced_probation_agents,routing_forced_failed_agents,routing_forced_exhausted_agents,routing_unstable_agents | own=
type _pin_ReaperReport = Expect<
  AllTrue<
    FieldsComparable<
      Pick<ReaperScanResponse, "dead_agents" | "degraded_agents" | "misconfigured_agents" | "routing_pressure_agents" | "routing_forced_probation_agents" | "routing_forced_failed_agents" | "routing_forced_exhausted_agents" | "routing_unstable_agents">,
      Pick<ReaperReport, "dead_agents" | "degraded_agents" | "misconfigured_agents" | "routing_pressure_agents" | "routing_forced_probation_agents" | "routing_forced_failed_agents" | "routing_forced_exhausted_agents" | "routing_unstable_agents">
    >
  >
>;

// @pin lib/agentrepair.ts:RepairProfile | AgentListResponseProfilesItem | pinned=slug,name,soul,model,fallbacks,task_type,workdir,memory_scope,owner_agent,parent_agent,direct_callable,retry_policy,health_policy,self_repair,noise_policy | own=
type _pin_RepairProfile = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AgentListResponseProfilesItem, "slug" | "name" | "soul" | "model" | "fallbacks" | "task_type" | "workdir" | "memory_scope" | "owner_agent" | "parent_agent" | "direct_callable" | "retry_policy" | "health_policy" | "self_repair" | "noise_policy">,
      Pick<RepairProfile, "slug" | "name" | "soul" | "model" | "fallbacks" | "task_type" | "workdir" | "memory_scope" | "owner_agent" | "parent_agent" | "direct_callable" | "retry_policy" | "health_policy" | "self_repair" | "noise_policy">
    >
  >
>;

// @pin lib/agentshared.ts:AgentProfile | AgentListResponseProfilesItem | pinned=status,lifecycle,tasklist | own=
type _pin_AgentProfile = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AgentListResponseProfilesItem, "status" | "lifecycle" | "tasklist">,
      Pick<AgentProfile, "status" | "lifecycle" | "tasklist">
    >
  >
>;

// @pin lib/channelSessions.ts:InboxThread | InboxResponseThreadsItem | pinned=messages | own=
type _pin_InboxThread = Expect<
  AllTrue<
    FieldsComparable<
      Pick<InboxResponseThreadsItem, "messages">,
      Pick<InboxThread, "messages">
    >
  >
>;

// @pin lib/chat.ts:ChatFrame | JournalGrepResponseEventsItem | pinned=kind,subject,payload,correlation_id | own=
type _pin_ChatFrame = Expect<
  AllTrue<
    FieldsComparable<
      Pick<JournalGrepResponseEventsItem, "kind" | "subject" | "payload" | "correlation_id">,
      Pick<ChatFrame, "kind" | "subject" | "payload" | "correlation_id">
    >
  >
>;

// @pin lib/chat.ts:ChatTool | ToolLogResponseInvocationsItem | pinned=tool,error,input,output | own=
type _pin_ChatTool = Expect<
  AllTrue<
    FieldsComparable<
      Pick<ToolLogResponseInvocationsItem, "tool" | "error" | "input" | "output">,
      Pick<ChatTool, "tool" | "error" | "input" | "output">
    >
  >
>;

// @pin lib/chat.ts:ChatTurn | RunResponse | pinned=answer,model,iters,agent | own=tools
type _pin_ChatTurn = Expect<
  AllTrue<
    FieldsComparable<
      Pick<RunResponse, "answer" | "model" | "iters" | "agent">,
      Pick<ChatTurn, "answer" | "model" | "iters" | "agent">
    >
  >
>;

// @pin lib/conductor.ts:ConductorStep | CouncilAskResponseOpinionsItem | pinned=round,model,text,error | own=
type _pin_ConductorStep = Expect<
  AllTrue<
    FieldsComparable<
      Pick<CouncilAskResponseOpinionsItem, "round" | "model" | "text" | "error">,
      Pick<ConductorStep, "round" | "model" | "text" | "error">
    >
  >
>;

// @pin lib/delegation.ts:RunNode | RunsListResponseRunsItem | pinned=status,model,iters,intent | own=
type _pin_RunNode = Expect<
  AllTrue<
    FieldsComparable<
      Pick<RunsListResponseRunsItem, "status" | "model" | "iters" | "intent">,
      Pick<RunNode, "status" | "model" | "iters" | "intent">
    >
  >
>;

// @pin lib/fleet.ts:ApiProfile | AgentListResponseProfilesItem | pinned=status | own=
type _pin_ApiProfile = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AgentListResponseProfilesItem, "status">,
      Pick<ApiProfile, "status">
    >
  >
>;

// @pin lib/incidents.ts:IncidentDelegateCandidate | AgentListResponseProfilesItem | pinned=slug,name,enabled,retired | own=
type _pin_IncidentDelegateCandidate = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AgentListResponseProfilesItem, "slug" | "name" | "enabled" | "retired">,
      Pick<IncidentDelegateCandidate, "slug" | "name" | "enabled" | "retired">
    >
  >
>;

// @pin lib/incidents.ts:IncidentProfileLite | AgentListResponseProfilesItem | pinned=status | own=
type _pin_IncidentProfileLite = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AgentListResponseProfilesItem, "status">,
      Pick<IncidentProfileLite, "status">
    >
  >
>;

// @pin lib/incidents.ts:IncidentResolutionRow | AutonomyFeedResponseItemsItem | pinned=subject,phase,mode,resolution | own=
type _pin_IncidentResolutionRow = Expect<
  AllTrue<
    FieldsComparable<
      Pick<AutonomyFeedResponseItemsItem, "subject" | "phase" | "mode" | "resolution">,
      Pick<IncidentResolutionRow, "subject" | "phase" | "mode" | "resolution">
    >
  >
>;

// @pin lib/liveruncontext.ts:LiveRunContext | RunsListResponseRunsItem | pinned=agent,phase,tool,model | own=
type _pin_LiveRunContext = Expect<
  AllTrue<
    FieldsComparable<
      Pick<RunsListResponseRunsItem, "agent" | "phase" | "tool" | "model">,
      Pick<LiveRunContext, "agent" | "phase" | "tool" | "model">
    >
  >
>;

// @pin lib/models.ts:ModelOption | CatalogListResponseProvidersItemModelsItem | pinned=id,name,reasoning,context | own=
type _pin_ModelOption = Expect<
  AllTrue<
    FieldsComparable<
      Pick<CatalogListResponseProvidersItemModelsItem, "id" | "name" | "reasoning" | "context">,
      Pick<ModelOption, "id" | "name" | "reasoning" | "context">
    >
  >
>;

// @pin lib/rundetail.ts:ToolCall | ToolLogResponseInvocationsItem | pinned=tool,error,input,output | own=
type _pin_ToolCall = Expect<
  AllTrue<
    FieldsComparable<
      Pick<ToolLogResponseInvocationsItem, "tool" | "error" | "input" | "output">,
      Pick<ToolCall, "tool" | "error" | "input" | "output">
    >
  >
>;

// ── the browser's own vocabulary ──────────────────────────────────────────
// @own components/configcenter/page.tsx:QuickConfigAction | the browser's quick-action card (icon, counters, tone), not a daemon row
// @own components/ui/tab-nav.tsx:TabDef | the browser's tab bar, not a daemon row
// @own features/admin/components/Backups.tsx:Checkpoint | the daemon's rollback op is not in the generated catalog yet
// @own features/agents/components/agentdetail/capability.tsx:WorkflowToolAccessSummary | the browser's badge summary, not the daemon's next_action row
// @own features/agents/components/agentdetail/comms.tsx:AgentInboxPrioritySummary | the browser's badge summary, not the daemon's next_action row
// @own features/agents/components/agentdetail/lifecycle.tsx:AgentLifecycleActionResultSummary | the browser's badge summary, not the daemon's next_action row
// @own features/agents/components/agentdetail/lifecycle.tsx:AgentLifecycleLedgerEntry | the browser's ledger entry, not the daemon's next_action row
// @own features/agents/components/agentdetail/lifecycle.tsx:AgentRemovalCascade | the browser's boolean rollup of the daemon's impact row
// @own features/agents/components/roster/removal.ts:AgentRemovalDecisionSummary | the browser's badge summary, not the daemon's next_action row
// @own features/mcp/components/types.ts:CatalogEntry | the browser's MCP preset catalogue, not the daemon's server registry
// @own features/schedules/lib/shared.ts:ScheduleTargetCounts | the browser's own counts, not a daemon row
// @own features/voice/components/VoiceSetup.test.tsx:TestProvider | a test fixture for the browser's voice catalogue
// @own features/voice/lib/voiceCatalog.ts:SpeechProvider | the browser's voice catalogue, not the daemon's model catalogue
// @own features/workflows/components/types.ts:WorkflowChainKind | the browser's chain vocabulary, not the daemon's next_action row
// @own lib/agentdetail.ts:AgentOperationalTask | the browser's escalation task, not the daemon's tasklist row
// @own lib/autonomy.ts:DoctorIncidentTreeOpsSummary | the browser's badge summary, not the daemon's next_action row
// @own lib/chatStore.tsx:LearnedMem | the browser's learned-memory record, not a daemon row
// @own lib/conversations.ts:Conversation | the browser's named thread, not a daemon row
// @own lib/fleet.ts:FleetAgentIdentityCardSummary | the browser's badge summary, not the daemon's next_action row
// @own lib/fleet.ts:FleetEntity | the browser's fleet model, not a daemon row
// @own nav.tsx:NavGroup | the browser's navigation, not a daemon row
// @own nav.tsx:NavItem | the browser's navigation, not a daemon row
// @own nav.tsx:NavRow | the browser's navigation, not a daemon row

// ── the scanner ─────────────────────────────────────────────────────────────
// The daemon's rows, straight out of the generated catalog.
const SRC = new URL("../../", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");
const GENERATED = join(SRC, "app", "api", "types.generated.ts");
const SELF = new URL("./payloadtypes.ratchet.test.ts", import.meta.url)
  .pathname.replace(/^\/([A-Za-z]:)/, "$1");

/** Every source file under src/: the module graph is the graph. */
function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) out.push(...walk(p));
    else if (/\.tsx?$/.test(entry) && !entry.endsWith(".d.ts")) out.push(p);
  }
  return out;
}

/** The text of a `{ … }` block, comments and strings respected. */
function blockOf(src: string, open: number): string {
  let depth = 0;
  for (let i = open; i < src.length; i++) {
    const c = src[i];
    if (c === "/" && src[i + 1] === "/") {
      i = src.indexOf("\n", i);
      if (i === -1) return "";
      continue;
    }
    if (c === "/" && src[i + 1] === "*") {
      i = src.indexOf("*/", i);
      if (i === -1) return "";
      i++;
      continue;
    }
    if (c === '"' || c === "'" || c === "`") {
      const quote = c;
      i++;
      while (i < src.length && src[i] !== quote) {
        if (src[i] === "\\") i++;
        i++;
      }
      continue;
    }
    if (c === "{") depth++;
    else if (c === "}") {
      depth--;
      if (depth === 0) return src.slice(open + 1, i);
    }
  }
  return "";
}

/** The members of an object body: name, optional, type text. */
function membersOf(body: string): { name: string; optional: boolean; type: string }[] {
  const out: { name: string; optional: boolean; type: string }[] = [];
  const clean = body
    .replace(/\/\*[\s\S]*?\*\//g, " ")
    .replace(/\/\/[^\n]*/g, " ");
  let depth = 0;
  let start = 0;
  const parts: string[] = [];
  for (let i = 0; i < clean.length; i++) {
    const c = clean[i];
    if (c === '"' || c === "'" || c === "`") {
      const quote = c;
      i++;
      while (i < clean.length && clean[i] !== quote) {
        if (clean[i] === "\\") i++;
        i++;
      }
      continue;
    }
    if (c === "{" || c === "(" || c === "[" || c === "<") depth++;
    else if (c === "}" || c === ")" || c === "]" || c === ">") depth--;
    else if (c === ";" && depth === 0) {
      parts.push(clean.slice(start, i));
      start = i + 1;
    }
  }
  parts.push(clean.slice(start));
  for (const part of parts) {
    const m = /^\s*([A-Za-z_$][\w$]*)(\?)?\s*:\s*([\s\S]+)$/.exec(part);
    if (m) out.push({ name: m[1], optional: m[2] === "?", type: m[3].trim() });
  }
  return out;
}

const generated = readFileSync(GENERATED, "utf8");

/** The daemon's rows: name → the fields it carries. */
const ROWS = new Map<string, Set<string>>();
for (const m of generated.matchAll(/export interface ([A-Za-z_$][\w$]*)\s*\{/g)) {
  const body = blockOf(generated, m.index + m[0].length - 1);
  const fields = new Set<string>();
  for (const member of membersOf(body)) fields.add(member.name);
  ROWS.set(m[1], fields);
}

/** Every hand-written type/interface with an object body. */
interface Decl {
  file: string;
  name: string;
  members: { name: string; optional: boolean; type: string }[];
}
const DECLS: Decl[] = [];
for (const path of walk(SRC)) {
  if (path === GENERATED || path === SELF) continue;
  const src = readFileSync(path, "utf8");
  const file = path.slice(SRC.length).replace(/\\/g, "/");
  for (const m of src.matchAll(/(?:export\s+)?(?:type|interface)\s+([A-Za-z_$][\w$]*)\s*(?:=\s*)?\{/g)) {
    DECLS.push({
      file,
      name: m[1],
      members: membersOf(blockOf(src, m.index + m[0].length - 1)),
    });
  }
}

/** A member's type comes from the daemon when it indexes a row or Picks one. */
function daemonTyped(type: string): boolean {
  if (type.includes('["')) return true;
  if (/\bPick\s*</.test(type)) return true;
  for (const row of ROWS.keys()) {
    if (new RegExp(`\\b${row}\\b`).test(type)) return true;
  }
  return false;
}

/** A browser view: the rows it shares fields with, and the ones it names by hand. */
interface View {
  rows: Map<string, Set<string>>;
  hand: Map<string, Set<string>>;
}
const VIEWS = new Map<string, View>();
for (const decl of DECLS) {
  if (!decl.members.length) continue;
  const rows = new Map<string, Set<string>>();
  const hand = new Map<string, Set<string>>();
  for (const [row, fields] of ROWS) {
    const shared = decl.members.filter((m) => fields.has(m.name));
    if (shared.length < 3) continue;
    rows.set(row, new Set(shared.map((m) => m.name)));
    const named = shared.filter((m) => !daemonTyped(m.type)).map((m) => m.name);
    if (named.length) hand.set(row, new Set(named));
  }
  if (!hand.size) continue;
  VIEWS.set(`${decl.file}:${decl.name}`, { rows, hand });
}

// ── the pins, read back from this file ──────────────────────────────────────
// The marker IS the pin: the compile-time assertion below it says which
// daemon row the browser's fields are held to, and the runtime reads the
// marker so the two cannot drift apart.
interface Pin {
  row: string;
  fields: string[];
  own: string[];
  chunk: string;
}
const PINS = new Map<string, Pin>();
const OWN = new Map<string, string>();
const self = readFileSync(SELF, "utf8");
const lines = self.split("\n");
for (let i = 0; i < lines.length; i++) {
  const line = lines[i];
  const pin = /^\/\/ @pin (\S+) \| (\S+) \| pinned=(\S*) \| own=(\S*)$/.exec(line);
  if (pin) {
    const chunk: string[] = [];
    for (let j = i + 1; j < lines.length && !lines[j].startsWith("// @"); j++) {
      chunk.push(lines[j]);
    }
    PINS.set(pin[1], {
      row: pin[2],
      fields: pin[3] ? pin[3].split(",") : [],
      own: pin[4] ? pin[4].split(",") : [],
      chunk: chunk.join("\n"),
    });
    continue;
  }
  const own = /^\/\/ @own (\S+) \| (.*)$/.exec(line);
  if (own) OWN.set(own[1], own[2]);
}

describe("the browser's payload views", () => {
  it("pin every daemon field they name by hand", () => {
    const violations: string[] = [];
    for (const [key, view] of VIEWS) {
      if (PINS.has(key) || OWN.has(key)) continue;
      violations.push(
        `${key} names a daemon field by hand — pin it, or say it is the browser's own`,
      );
    }
    expect(violations).toEqual([]);
  });

  it("keep every pin and every ledger entry honest", () => {
    const violations: string[] = [];
    for (const [key, pin] of PINS) {
      const view = VIEWS.get(key);
      if (!view) {
        violations.push(`${key} is pinned but names no daemon field by hand any more`);
        continue;
      }
      if (!view.rows.has(pin.row)) {
        violations.push(`${key} is pinned to ${pin.row}, a row it does not share three fields with`);
        continue;
      }
      const named = view.hand.get(pin.row) ?? new Set<string>();
      const covered = new Set([...pin.fields, ...pin.own]);
      for (const field of named) {
        if (!covered.has(field)) {
          violations.push(`${key} names the daemon's ${field} by hand and does not pin it`);
        }
      }
      for (const field of covered) {
        if (!named.has(field)) {
          violations.push(`${key} pins ${field}, which it no longer names by hand`);
        }
      }
      // The compile-time assertion is the pin's teeth; without it the pin is
      // a comment.
      const union = pin.fields.map((f) => `"${f}"`).join(" | ");
      if (pin.fields.length) {
        if (!pin.chunk.includes(`Pick<${pin.row}, ${union}>`)) {
          violations.push(`${key} has no compile-time assertion on ${pin.row}`);
        }
        if (!new RegExp(`Pick<[A-Za-z_$][\\w$]*, ${union.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}>`).test(pin.chunk)) {
          violations.push(`${key} has no compile-time assertion on the browser's own fields`);
        }
      }
    }
    for (const key of OWN.keys()) {
      if (!VIEWS.has(key)) {
        violations.push(`${key} is on the ledger but names no daemon field by hand any more`);
      }
    }
    expect(violations).toEqual([]);
  });

  it("keep the daemon's rows in the generated catalog", () => {
    expect(ROWS.size).toBeGreaterThan(300);
  });
});
