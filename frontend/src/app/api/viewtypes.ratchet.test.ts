// The browser's VIEW of the daemon's contract must not narrow it.
//
// Every browser view type names the fields it reads; the daemon says what
// those fields are. A browser that promises `string` where the daemon may
// send `null` breaks at RUNTIME, not at compile time — so the promise is
// pinned here, at compile time: the daemon's field type must be assignable to
// the browser's.
//
// The assertions below do not run; they COMPILE. `Expect<...>` fails the
// typecheck gate the moment a view type narrows the daemon's contract, and
// the runtime test below keeps the file a suite.
//
// Regenerate the daemon's rows with `go run ./tools/openapi`; the browser's
// rows live in frontend/src/app/api/types.generated.ts.
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

import type {
  AcpAgentsResponse,
  AcpAgentsResponseAgentsItem,
  AgentCapabilitiesResponsePermissionsItem,
  AgentCapabilitiesResponseWakeAccess,
  AgentPermissionsResponse,
  AgentRepairStatusResponse,
  AgentRepairStatusResponseHistoryItem,
  AutonomyFeedResponse,
  AutonomyFeedResponseItemsItem,
  CatalogListResponse,
  CatalogListResponseProvidersItem,
  CatalogListResponseProvidersItemModelsItem,
  ChannelListResponseChannelsItem,
  ChainsGetResponse,
  ExecutionProfileCheckResponse,
  ExecutionProfilesResponse,
  InboxResponseThreadsItem,
  InboxResponseThreadsItemMessagesItem,
  ReaperScanResponse,
  ReaperScanResponseDeadAgentsItem,
  RoutingGetResponse,
  StandingListResponseOrdersItem,
  WorkflowDraftResponseWorkflow,
  WorkflowRunsResponseRunsItem,
  WorkflowRunsResponseRunsItemNodeEventsItem,
  WorkflowTemplatesResponseTemplatesItem,
} from "@/app/api/types.generated";
import type { ACPInventory, ACPAgentStatus } from "@/lib/acp";
import type { AgentRepairEvent, AgentRepairStatus, ReaperReport } from "@/lib/agentdetail";
import type { AutonomyItem } from "@/lib/autonomy";
import type { InboxThread } from "@/lib/channelSessions";
import type { ApiOrder } from "@/lib/fleet";
import type { ChainMap, ChainsState } from "@/lib/chains";
import type { ModelCatalog } from "@/lib/models";
import type { SetupCatalog, SetupModel, SetupProvider } from "@/features/setup/lib/setup";
import type { ChannelRow } from "@/features/channels/components/types";
import type {
  ExecutionProfileHealthReport,
  ExecutionProfileInventory,
} from "@/features/execution-profiles/components/types";
import type {
  AgentPermissionRow,
  AgentPermissionsSnapshot,
  AgentWakeAccess,
  RoutingInfo,
} from "@/features/agents/components/agentdetail/shared";
import type {
  Wf,
  WfRun,
  WfRunNodeEvent,
  WfTemplate,
} from "@/features/workflows/components/types";

/** Assignable says the daemon's type fits the browser's promise. */
type Assignable<From, To> = [From] extends [To] ? true : false;
/** Expect fails the typecheck when its argument is not true. */
type Expect<T extends true> = T;

// ── setup ───────────────────────────────────────────────────────────────────
type _setupProviderEnv = Expect<
  Assignable<CatalogListResponseProvidersItem["env"], SetupProvider["env"]>
>;
type _setupProviderModels = Expect<
  Assignable<CatalogListResponseProvidersItem["models"], SetupProvider["models"]>
>;
type _setupProviderCredentialed = Expect<
  Assignable<CatalogListResponseProvidersItem["credentialed"], SetupProvider["credentialed"]>
>;
type _setupModelId = Expect<
  Assignable<CatalogListResponseProvidersItemModelsItem["id"], SetupModel["id"]>
>;
type _setupCatalogProviders = Expect<Assignable<CatalogListResponse["providers"], SetupCatalog["providers"]>>;

// ── chains ──────────────────────────────────────────────────────────────────
type _chainMap = Expect<Assignable<RoutingGetResponse["chains"], ChainMap>>;
type _chainsState = Expect<Assignable<ChainsGetResponse["chains"], ChainsState["chains"]>>;

// ── catalog ─────────────────────────────────────────────────────────────────
type _modelCatalog = Expect<Assignable<CatalogListResponse["providers"], ModelCatalog["providers"]>>;

// ── acp ─────────────────────────────────────────────────────────────────────
// The ACP view normalises the daemon's nullable agent list at the boundary
// (the page reads a list, never a null), so the daemon's LIST is what must
// fit — not the null arm the normaliser already handled.
type _acpAgents = Expect<
  Assignable<NonNullable<AcpAgentsResponse["agents"]>, ACPInventory["agents"]>
>;
type _acpSlug = Expect<Assignable<AcpAgentsResponseAgentsItem["slug"], ACPAgentStatus["slug"]>>;

// ── reaper + repair ─────────────────────────────────────────────────────────
type _reaperDead = Expect<Assignable<ReaperScanResponse["dead_agents"], ReaperReport["dead_agents"]>>;
type _reaperDeadRow = Expect<
  Assignable<ReaperScanResponseDeadAgentsItem, NonNullable<ReaperReport["dead_agents"]>[number]>
>;
type _repairHistory = Expect<
  Assignable<AgentRepairStatusResponse["history"], AgentRepairStatus["history"]>
>;
type _repairIssues = Expect<
  Assignable<AgentRepairStatusResponseHistoryItem["issues"], AgentRepairEvent["issues"]>
>;

// ── permissions ─────────────────────────────────────────────────────────────
type _permissions = Expect<
  Assignable<AgentPermissionsResponse["permissions"], AgentPermissionsSnapshot["permissions"]>
>;
type _configEntries = Expect<
  Assignable<AgentPermissionsResponse["config_entries"], AgentPermissionsSnapshot["config_entries"]>
>;
type _permissionAllowed = Expect<
  Assignable<AgentCapabilitiesResponsePermissionsItem["allowed"], AgentPermissionRow["allowed"]>
>;
type _wakeDelegationSources = Expect<
  Assignable<AgentCapabilitiesResponseWakeAccess["delegation_sources"], AgentWakeAccess["delegation_sources"]>
>;
type _routingChains = Expect<Assignable<RoutingGetResponse["chains"], RoutingInfo["chains"]>>;

// ── fleet ───────────────────────────────────────────────────────────────────
type _orderTriggers = Expect<
  Assignable<StandingListResponseOrdersItem["triggers"], ApiOrder["triggers"]>
>;
type _orderInitiative = Expect<
  Assignable<StandingListResponseOrdersItem["initiative"], ApiOrder["initiative"]>
>;

// ── autonomy ────────────────────────────────────────────────────────────────
type _autonomyTitle = Expect<
  Assignable<AutonomyFeedResponseItemsItem["title"], AutonomyItem["title"]>
>;
type _autonomyItems = Expect<
  Assignable<AutonomyFeedResponse["items"], { items?: AutonomyFeedResponse["items"] }["items"]>
>;

// ── channel sessions ────────────────────────────────────────────────────────
type _inboxMessages = Expect<
  Assignable<InboxResponseThreadsItem["messages"], InboxThread["messages"]>
>;
type _inboxMessageRow = Expect<
  Assignable<InboxResponseThreadsItemMessagesItem, NonNullable<InboxThread["messages"]>[number]>
>;

// ── channels ────────────────────────────────────────────────────────────────
type _channelKind = Expect<Assignable<ChannelListResponseChannelsItem["kind"], ChannelRow["kind"]>>;
type _channelFields = Expect<
  Assignable<ChannelListResponseChannelsItem["fields"], ChannelRow["fields"]>
>;

// ── execution profiles ──────────────────────────────────────────────────────
type _profiles = Expect<
  Assignable<ExecutionProfilesResponse["profiles"], ExecutionProfileInventory["profiles"]>
>;
type _checks = Expect<
  Assignable<ExecutionProfileCheckResponse["checks"], ExecutionProfileHealthReport["checks"]>
>;

// ── workflows ───────────────────────────────────────────────────────────────
type _workflowNodes = Expect<Assignable<WorkflowDraftResponseWorkflow["nodes"], Wf["nodes"]>>;
type _workflowTemplate = Expect<
  Assignable<WorkflowTemplatesResponseTemplatesItem["workflow"], WfTemplate["workflow"]>
>;
type _runNodeEvents = Expect<
  Assignable<WorkflowRunsResponseRunsItem["node_events"], WfRun["node_events"]>
>;
type _runNodeEvent = Expect<
  Assignable<
    WorkflowRunsResponseRunsItemNodeEventsItem,
    NonNullable<WfRunNodeEvent> extends never ? never : NonNullable<WfRun["node_events"]>[number]
  >
>;

describe("the browser's view types", () => {
  it("keep the daemon's null arms in the generated catalog", () => {
    // The assertions above are compile-time; this is the runtime echo. The
    // daemon's catalog is the source of every browser view type, so a catalog
    // that lost its null arm would silently re-promis(e) a value the daemon
    // may send as null.
    const generated = readFileSync(new URL("./types.generated.ts", import.meta.url), "utf8");
    expect(generated).toContain("env: string[] | null;");
    expect(generated).toContain("providers: CatalogListResponseProvidersItem[] | null;");
    expect(generated).not.toMatch(/\w \| null\[\]/);
  });
});
