import { useCallback, useEffect, useMemo, useState } from "react";
import {
  SlidersHorizontal,
  RefreshCw,
  Lock,
  Save,
  Trash2,
  Zap,
  RotateCw,
  Check,
  Search,
  Puzzle,
  Cpu,
  Send,
  Mail,
  MessageSquare,
  Network,
  Gauge,
  Shield,
  X,
  KeyRound,
  Route,
  Bot,
  ArrowRight,
  type LucideIcon,
} from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader } from "@/components/ui/page-header";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";

// The Config Center is the editable companion to the read-only Config view:
// schema-driven forms (one section per channel/area) backed by the daemon's
// config store + vault. The schema is dynamic — built-in sections plus any a
// skill/plugin has registered (M695) — so this view groups sections into Core /
// Channels / Skills & Plugins, offers a sticky section nav and a search filter,
// and badges registered sections by their provenance. Secrets stay write-only
// (presence only, never the value); externally-pinned fields are read-only because
// the real process environment wins. Each save reports live (provider/model) vs restart.

type FieldType = "text" | "password" | "number" | "bool" | "csv" | "select";
type ApplyMode = "live" | "restart";

export interface Field {
  env: string;
  label: string;
  type: FieldType;
  secret: boolean;
  required: boolean;
  help?: string;
  apply: ApplyMode;
  options?: string[];
  read_only?: boolean; // system-managed: shown but not editable here
  locked?: boolean; // value may change but never be cleared
}
interface Section {
  id: string;
  name: string;
  help?: string;
  source?: string;
  fields: Field[];
}
interface ReloadBoundary {
  apply: ApplyMode;
  envs: string[];
}
interface ConfigSchemaResponse {
  sections?: Section[];
  reload_boundaries?: ReloadBoundary[];
}
export interface ValueEntry {
  env: string;
  secret: boolean;
  env_pinned: boolean;
  set: boolean;
  value?: string;
}
interface SetResult {
  env: string;
  saved: boolean;
  applied: "live" | "restart";
  env_pinned?: boolean;
  reload_error?: string;
}
interface AgentConfigEntry {
  key: string;
  value?: string;
  rating?: "public" | "internal" | "restricted" | "secret";
  description?: string;
  allowed_agents?: string[];
  excluded_agents?: string[];
  updated_at?: number;
}

type Category = "Core" | "Channels" | "Skills & Plugins";
const CATEGORY_ORDER: Category[] = ["Core", "Channels", "Skills & Plugins"];
const CHANNEL_IDS = new Set(["telegram", "email", "slack", "discord"]);

function isRegistered(sec: Section): boolean {
  return !!sec.source && sec.source !== "builtin";
}
function categoryOf(sec: Section): Category {
  if (isRegistered(sec)) return "Skills & Plugins";
  if (CHANNEL_IDS.has(sec.id)) return "Channels";
  return "Core";
}

const SECTION_ICONS: Record<string, LucideIcon> = {
  provider: Cpu,
  telegram: Send,
  email: Mail,
  slack: MessageSquare,
  discord: MessageSquare,
  interfaces: Network,
  limits: Gauge,
  security: Shield,
};
function iconFor(sec: Section): LucideIcon {
  if (isRegistered(sec)) return Puzzle;
  return SECTION_ICONS[sec.id] ?? SlidersHorizontal;
}

function sectionDomID(id: string): string {
  return `cfg-section-${id}`;
}

export function reloadBoundariesFromSections(sections: Array<{ fields: Array<{ env: string; apply?: string }> }>): ReloadBoundary[] {
  const grouped: Record<ApplyMode, Set<string>> = { live: new Set(), restart: new Set() };
  for (const sec of sections) {
    for (const field of sec.fields) {
      const apply: ApplyMode = field.apply === "live" ? "live" : "restart";
      grouped[apply].add(field.env);
    }
  }
  return (["live", "restart"] as ApplyMode[])
    .map((apply) => ({ apply, envs: Array.from(grouped[apply]).sort() }))
    .filter((boundary) => boundary.envs.length > 0);
}

export function summarizeReloadBoundaries(boundaries: ReloadBoundary[]): Record<ApplyMode, number> {
  return boundaries.reduce<Record<ApplyMode, number>>(
    (acc, boundary) => {
      acc[boundary.apply] += boundary.envs.length;
      return acc;
    },
    { live: 0, restart: 0 },
  );
}

function envPreview(envs: string[]): string {
  if (envs.length === 0) return "No settings";
  const visible = envs.slice(0, 8).join(", ");
  return envs.length > 8 ? `${visible}, +${envs.length - 8} more` : visible;
}

function ReloadBoundarySummary({ boundaries, setCount }: { boundaries: ReloadBoundary[]; setCount: number }) {
  const summary = summarizeReloadBoundaries(boundaries);
  if (summary.live + summary.restart === 0) return null;
  const live = boundaries.find((b) => b.apply === "live")?.envs || [];
  const restart = boundaries.find((b) => b.apply === "restart")?.envs || [];
  return (
    <section className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-xs">
      <div className="mr-1 flex items-center gap-2 font-medium text-foreground">
        <RefreshCw className="size-3.5 text-accent" />
        <span>Reload boundaries</span>
      </div>
      <Badge variant="accent" title={envPreview(live)}>
        <Zap className="size-2.5" /> {summary.live} live
      </Badge>
      <Badge variant="default" title={envPreview(restart)}>
        <RotateCw className="size-2.5" /> {summary.restart} restart
      </Badge>
      {setCount > 0 && <span className="text-muted">{setCount} configured</span>}
    </section>
  );
}

export function ConfigCenter() {
  const { toast } = useUI();
  const [sections, setSections] = useState<Section[] | null>(null);
  const [reloadBoundaries, setReloadBoundaries] = useState<ReloadBoundary[]>([]);
  const [values, setValues] = useState<Record<string, ValueEntry>>({});
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [query, setQuery] = useState("");

  const loadValues = useCallback(async () => {
    const v = await getJSON<{ fields?: ValueEntry[] }>("/api/config/values");
    const map: Record<string, ValueEntry> = {};
    for (const f of v.fields || []) map[f.env] = f;
    setValues(map);
  }, []);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const [sch] = await Promise.all([getJSON<ConfigSchemaResponse>("/api/config/schema"), loadValues()]);
      const nextSections = sch.sections || [];
      setSections(nextSections);
      setReloadBoundaries(sch.reload_boundaries?.length ? sch.reload_boundaries : reloadBoundariesFromSections(nextSections));
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [loadValues]);

  useEffect(() => {
    reload();
  }, [reload]);

  const setCount = useMemo(() => Object.values(values).filter((v) => v.set).length, [values]);
  const boundarySummary = useMemo(
    () => (reloadBoundaries.length ? reloadBoundaries : sections ? reloadBoundariesFromSections(sections) : []),
    [reloadBoundaries, sections],
  );

  // Filter fields by the search query (env or label substring); drop emptied sections.
  const q = query.trim().toLowerCase();
  const filtered = useMemo(() => {
    if (!sections) return [];
    if (!q) return sections;
    return sections
      .map((sec) => ({
        ...sec,
        fields: sec.fields.filter((f) => f.env.toLowerCase().includes(q) || f.label.toLowerCase().includes(q)),
      }))
      .filter((sec) => sec.fields.length > 0);
  }, [sections, q]);

  // Group the (filtered) sections into ordered categories.
  const grouped = useMemo(() => {
    const g: Record<string, Section[]> = {};
    for (const sec of filtered) (g[categoryOf(sec)] ||= []).push(sec);
    return g;
  }, [filtered]);

  const quickActions = useMemo(() => buildQuickConfigActions(sections || [], values), [sections, values]);

  const jumpTo = (id: string) => {
    const el = typeof document !== "undefined" ? document.getElementById(sectionDomID(id)) : null;
    el?.scrollIntoView?.({ behavior: "smooth", block: "start" });
  };

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        icon={SlidersHorizontal}
        title="Config Center"
        actions={
          <>
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search settings…"
                className="h-8 w-44 pl-7 sm:w-56"
                aria-label="Search settings"
              />
            </div>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading}>
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
            </Button>
          </>
        }
      />

      {sections && <ReloadBoundarySummary boundaries={boundarySummary} setCount={setCount} />}

      {sections && <QuickConfigDeck actions={quickActions} values={values} onSaved={loadValues} toast={toast} />}

      <AgentRuntimeConfigPanel toast={toast} />

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !sections ? (
        <SkeletonList count={4} lines={2} />
      ) : filtered.length === 0 ? (
        <EmptyState icon={Search} title="No settings match" hint={`Nothing matches “${query}”. Clear the search to see all sections.`} />
      ) : (
        <div className="grid gap-4 lg:grid-cols-[12rem_1fr]">
          {/* Sticky section nav (desktop only). */}
          <nav className="hidden self-start lg:sticky lg:top-2 lg:block">
            <div className="space-y-3">
              {CATEGORY_ORDER.filter((c) => grouped[c]?.length).map((cat) => (
                <div key={cat}>
                  <div className="mb-1 px-2 text-xs font-semibold uppercase text-muted">{cat}</div>
                  <div className="space-y-0.5">
                    {grouped[cat].map((sec) => {
                      const Icon = iconFor(sec);
                      return (
                        <button
                          key={sec.id}
                          onClick={() => jumpTo(sec.id)}
                          className="flex w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs text-foreground/80 transition-colors hover:bg-card hover:text-foreground"
                        >
                          <Icon className="size-3.5 shrink-0 text-muted" />
                          <span className="truncate">{sec.name}</span>
                        </button>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          </nav>

          {/* Sections, grouped by category. */}
          <div className="min-w-0 space-y-5">
            {CATEGORY_ORDER.filter((c) => grouped[c]?.length).map((cat) => (
              <section key={cat} className="space-y-3">
                <h3 className="text-[11px] font-semibold uppercase text-muted">{cat}</h3>
                {grouped[cat].map((sec) => (
                  <SectionCard
                    key={sec.id + (query.trim() ? "·q" : "")}
                    section={sec}
                    values={values}
                    onSaved={loadValues}
                    toast={toast}
                  />
                ))}
              </section>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

interface QuickConfigAction {
  id: string;
  title: string;
  detail: string;
  icon: LucideIcon;
  fields: Field[];
  configured: number;
  total: number;
  tone: "accent" | "good" | "warn" | "bad" | "muted";
}

function buildQuickConfigActions(sections: Section[], values: Record<string, ValueEntry>): QuickConfigAction[] {
  const byEnv = new Map<string, Field>();
  const bySection = new Map<string, Section>();
  for (const sec of sections) {
    bySection.set(sec.id, sec);
    for (const field of sec.fields) byEnv.set(field.env, field);
  }
  const fields = (...envs: string[]) => envs.map((env) => byEnv.get(env)).filter(Boolean) as Field[];
  const sectionFields = (...ids: string[]) => ids.flatMap((id) => bySection.get(id)?.fields || []);
  const count = (items: Field[]) => items.filter((field) => values[field.env]?.set).length;
  const make = (action: Omit<QuickConfigAction, "configured" | "total" | "tone">): QuickConfigAction => {
    const configured = count(action.fields);
    const total = action.fields.length;
    return {
      ...action,
      configured,
      total,
      tone: total === 0 ? "muted" : configured === total ? "good" : configured > 0 ? "accent" : "warn",
    };
  };
  return [
    make({
      id: "provider",
      title: "Brain route",
      detail: "Provider, default model, fallback chain",
      icon: Cpu,
      fields: fields("AGEZT_PROVIDER", "AGEZT_MODEL", "AGEZT_MODEL_CHAIN", "AGEZT_ROUTING_TASKS", "AGEZT_PROVIDER_OAUTH"),
    }),
    make({
      id: "console",
      title: "Console access",
      detail: "Web UI password, bind address, browser open",
      icon: KeyRound,
      fields: fields("AGEZT_WEB_ADDR", "AGEZT_WEB_PASSWORD", "AGEZT_WEB_PASSWORD_DEFAULT", "AGEZT_WEB_OPEN"),
    }),
    make({
      id: "telegram",
      title: "Telegram ops",
      detail: "Bot token and allowed operator chat",
      icon: Send,
      fields: sectionFields("telegram"),
    }),
    make({
      id: "routing",
      title: "Fallbacks",
      detail: "Routing, retry, and live reload boundaries",
      icon: Route,
      fields: fields("AGEZT_MODEL_CHAIN", "AGEZT_ROUTER", "AGEZT_RETRY_MAX", "AGEZT_REPAIR_MAX", "AGEZT_REPAIR_ENABLED"),
    }),
    make({
      id: "agents",
      title: "Agent safety",
      detail: "Autonomy, guardrails, spend and tool policy",
      icon: Bot,
      fields: sectionFields("limits", "security", "autonomy"),
    }),
    make({
      id: "plugins",
      title: "Extensions",
      detail: "Plugin and skill-provided settings",
      icon: Puzzle,
      fields: sections.filter(isRegistered).flatMap((sec) => sec.fields),
    }),
  ].filter((action) => action.total > 0);
}

function QuickConfigDeck({
  actions,
  values,
  onSaved,
  toast,
}: {
  actions: QuickConfigAction[];
  values: Record<string, ValueEntry>;
  onSaved: () => Promise<void>;
  toast: (text: string, kind?: "success" | "error" | "info") => void;
}) {
  const [active, setActive] = useState<QuickConfigAction | null>(null);
  if (!actions.length) return null;
  return (
    <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {actions.map((action) => {
        const Icon = action.icon;
        return (
          <button
            key={action.id}
            onClick={() => setActive(action)}
            className={cn(
              "group flex min-h-28 flex-col rounded-lg border border-border bg-card p-3 text-left shadow-e1 transition-all hover:-translate-y-0.5 hover:border-accent/70 hover:shadow-lg",
              action.tone === "good" && "border-good/30",
              action.tone === "warn" && "border-warn/30",
            )}
          >
            <span className="flex items-start gap-2">
              <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-accent/12 text-accent ring-1 ring-inset ring-accent/25">
                <Icon className="size-4.5" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-semibold text-foreground">{action.title}</span>
                <span className="mt-0.5 block text-xs text-muted">{action.detail}</span>
              </span>
              <Badge variant={action.tone === "good" ? "good" : action.tone === "warn" ? "warn" : "accent"}>
                {action.configured}/{action.total}
              </Badge>
            </span>
            <span className="mt-auto inline-flex items-center gap-1 pt-3 text-xs font-medium text-accent opacity-80 transition-opacity group-hover:opacity-100">
              Configure <ArrowRight className="size-3" />
            </span>
          </button>
        );
      })}
      {active && (
        <ConfigModal title={active.title} icon={active.icon} onClose={() => setActive(null)}>
          <div className="space-y-2">
            {active.fields.map((field) => (
              <FieldRow key={field.env} field={field} entry={values[field.env]} onSaved={onSaved} toast={toast} />
            ))}
          </div>
        </ConfigModal>
      )}
    </section>
  );
}

function AgentRuntimeConfigPanel({ toast }: { toast: (text: string, kind?: "success" | "error" | "info") => void }) {
  const [entries, setEntries] = useState<AgentConfigEntry[] | null>(null);
  const [open, setOpen] = useState(false);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [rating, setRating] = useState<NonNullable<AgentConfigEntry["rating"]>>("internal");
  const [description, setDescription] = useState("");
  const [allow, setAllow] = useState("");
  const [deny, setDeny] = useState("");
  const [busy, setBusy] = useState(false);
  const summary = useMemo(() => summarizeAgentConfigEntries(entries || []), [entries]);

  const load = useCallback(async () => {
    const res = await getJSON<{ entries?: AgentConfigEntry[] }>("/api/configcenter/list");
    setEntries(res.entries || []);
  }, []);

  useEffect(() => {
    load().catch((e) => {
      toast(`Agent config load failed: ${(e as Error).message}`, "error");
      setEntries([]);
    });
  }, [load, toast]);

  async function save() {
    if (!key.trim() || !value.trim()) return;
    setBusy(true);
    try {
      await postJSON("/api/configcenter/set", {
        key: key.trim(),
        value,
        rating,
        description: description.trim(),
        allowed_agents: splitCSV(allow),
        excluded_agents: splitCSV(deny),
      });
      setKey("");
      setValue("");
      setDescription("");
      setAllow("");
      setDeny("");
      setOpen(false);
      await load();
      toast("Agent config saved", "success");
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  async function remove(entryKey: string) {
    setBusy(true);
    try {
      await postJSON("/api/configcenter/delete", { key: entryKey });
      await load();
      toast("Agent config deleted", "success");
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="rounded-lg border border-border bg-card p-3">
      <div className="flex flex-wrap items-start gap-3">
        <div className="flex items-center gap-2">
          <Shield className="size-4 text-accent" />
          <h3 className="text-xs font-semibold text-foreground">Agent Runtime Config</h3>
        </div>
        {entries && (
          <div className="flex flex-wrap gap-1.5">
            <Badge variant="default">{summary.total} total</Badge>
            <Badge variant="accent">{summary.identityBound} identity-bound</Badge>
            <Badge variant="default">{summary.shared} shared</Badge>
            <Badge variant="warn">{summary.restricted} restricted</Badge>
            <Badge variant="bad">{summary.secret} secret</Badge>
          </div>
        )}
        <Button size="sm" variant="ghost" onClick={() => load()} disabled={busy}>
          <RefreshCw className={cn("size-3.5", busy && "animate-spin")} /> Refresh
        </Button>
        <Button size="sm" onClick={() => setOpen(true)} disabled={busy}>
          <Save className="size-3.5" /> Add runtime key
        </Button>
      </div>

      <div className="mt-3 overflow-hidden rounded-lg border border-border">
        {!entries ? (
          <div className="p-3 text-xs text-muted">Loading agent config…</div>
        ) : entries.length === 0 ? (
          <div className="p-3 text-xs text-muted">No agent runtime config entries yet.</div>
        ) : (
          <div className="divide-y divide-border">
            {entries.map((entry) => (
              <div key={entry.key} className="grid gap-2 p-2 text-xs md:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_7rem_minmax(0,1fr)_auto] md:items-center">
                <div className="min-w-0">
                  <div className="truncate font-mono text-foreground" title={entry.key}>{entry.key}</div>
                  <div className="truncate text-[11px] text-muted">
                    {agentConfigScopeLabel(entry)}
                    {entry.description ? ` · ${entry.description}` : ""}
                  </div>
                </div>
                <div className="truncate font-mono text-muted" title={entry.rating === "secret" ? undefined : entry.value}>
                  {entry.rating === "secret" ? "********" : entry.value || "—"}
                </div>
                <Badge variant={entry.rating === "secret" ? "bad" : entry.rating === "restricted" ? "warn" : entry.rating === "public" ? "good" : "accent"}>{entry.rating || "internal"}</Badge>
                <div className="min-w-0 text-[11px] text-muted">
                  {entry.allowed_agents?.length ? <div className="truncate">allow: {entry.allowed_agents.join(", ")}</div> : null}
                  {entry.excluded_agents?.length ? <div className="truncate">deny: {entry.excluded_agents.join(", ")}</div> : null}
                  {!entry.allowed_agents?.length && !entry.excluded_agents?.length ? "all eligible agents" : null}
                </div>
                <Button size="sm" variant="ghost" onClick={() => remove(entry.key)} disabled={busy} title="Delete agent config">
                  <Trash2 className="size-3.5 text-bad" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>
      {open && (
        <ConfigModal title="Agent runtime key" icon={Shield} onClose={() => setOpen(false)}>
          <div className="grid gap-2">
            <Input value={key} onChange={(e) => setKey(e.target.value)} placeholder="agent/ops/runtime" aria-label="Agent config key" />
            <Input value={value} onChange={(e) => setValue(e.target.value)} placeholder="value" aria-label="Agent config value" />
            <AgentConfigRatingPicker value={rating} onChange={setRating} />
            <Input value={allow} onChange={(e) => setAllow(e.target.value)} placeholder="allow: ops,planner" aria-label="Allowed agents" />
            <Input value={deny} onChange={(e) => setDeny(e.target.value)} placeholder="deny: blocked" aria-label="Denied agents" />
            <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Optional description" aria-label="Agent config description" />
            <div className="flex justify-end gap-2 pt-2">
              <Button size="sm" variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
              <Button size="sm" onClick={save} disabled={busy || !key.trim() || !value.trim()} aria-label="Save agent config">
                {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
              </Button>
            </div>
          </div>
        </ConfigModal>
      )}
    </section>
  );
}

function splitCSV(value: string): string[] {
  return value.split(/[,\s]+/).map((v) => v.trim()).filter(Boolean);
}

export function agentConfigScopeLabel(entry: Pick<AgentConfigEntry, "key" | "allowed_agents" | "excluded_agents">): string {
  const bound = (entry.allowed_agents || []).length > 0;
  const excluded = (entry.excluded_agents || []).length > 0;
  if (bound) return "identity-bound";
  if (excluded) return "shared with denylist";
  if (String(entry.key || "").startsWith("agent/")) return "agent namespace";
  return "shared";
}

export function summarizeAgentConfigEntries(entries: AgentConfigEntry[]): {
  total: number;
  identityBound: number;
  shared: number;
  restricted: number;
  secret: number;
} {
  let identityBound = 0;
  let shared = 0;
  let restricted = 0;
  let secret = 0;
  for (const entry of entries) {
    const scope = agentConfigScopeLabel(entry);
    if (scope === "identity-bound") identityBound++;
    else shared++;
    if (entry.rating === "restricted") restricted++;
    if (entry.rating === "secret") secret++;
  }
  return { total: entries.length, identityBound, shared, restricted, secret };
}

function ratingClass(rating: string): string {
  if (rating === "secret") return "bg-bad/15 text-bad";
  if (rating === "restricted") return "bg-amber-500/15 text-amber-300";
  if (rating === "public") return "bg-good/15 text-good";
  return "bg-accent/15 text-accent";
}

function SectionCard({
  section,
  values,
  onSaved,
  toast,
}: {
  section: Section;
  values: Record<string, ValueEntry>;
  onSaved: () => Promise<void>;
  toast: (text: string, kind?: "success" | "error" | "info") => void;
}) {
  const Icon = iconFor(section);
  const registered = isRegistered(section);
  const setCount = section.fields.filter((f) => values[f.env]?.set).length;
  return (
    <div id={sectionDomID(section.id)} className="scroll-mt-2">
      <ConfigSectionPanel
        icon={Icon}
        title={section.name}
        description={registered ? `Registered by ${section.source}` : undefined}
        status={`${setCount}/${section.fields.length} set`}
      >
        {section.help && <p className="text-[11px] text-muted mb-2">{section.help}</p>}
        <div className="space-y-3">
          {section.fields.map((f) => (
            <FieldRow key={f.env} field={f} entry={values[f.env]} onSaved={onSaved} toast={toast} />
          ))}
        </div>
      </ConfigSectionPanel>
    </div>
  );
}

function ConfigSectionPanel({
  icon: Icon,
  title,
  description,
  status,
  children,
}: {
  icon: LucideIcon;
  title: string;
  description?: string;
  status: string;
  children: React.ReactNode;
}) {
  return (
    <section aria-label={`Config section: ${title}`} className="rounded-xl border border-border bg-card/70 p-3 shadow-e1">
      <div className="mb-2 flex items-center gap-2">
        <span className="grid size-8 shrink-0 place-items-center rounded-lg border border-accent/35 bg-accent/5 text-accent">
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h4 className="truncate text-sm font-semibold">{title}</h4>
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
            <span>{status}</span>
            {description && <span className="truncate">{description}</span>}
          </div>
        </div>
      </div>
      {children}
    </section>
  );
}

export function FieldRow({
  field,
  entry,
  onSaved,
  toast,
}: {
  field: Field;
  entry?: ValueEntry;
  onSaved: () => Promise<void>;
  toast: (text: string, kind?: "success" | "error" | "info") => void;
}) {
  const pinned = !!entry?.env_pinned;
  const readOnly = !!field.read_only;
  const managed = pinned || readOnly; // not editable in the Config Center
  const locked = !!field.locked; // editable, but cannot be cleared/removed
  const isSet = !!entry?.set;
  const original = field.secret ? "" : entry?.value ?? "";
  const [draft, setDraft] = useState(original);
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(false);

  // Re-sync the draft when the upstream value changes (after a save/refresh),
  // but only for non-secret fields (secrets always start blank).
  useEffect(() => {
    if (!field.secret) setDraft(entry?.value ?? "");
  }, [entry?.value, field.secret]);

  // Non-secret: dirty when changed. Secret: dirty only when something was typed
  // (a blank secret save would CLEAR it — that path is the explicit Clear button).
  const dirty = field.secret ? draft.trim() !== "" : draft !== original;

  async function save(value: string, opts?: { cleared?: boolean }): Promise<boolean> {
    setBusy(true);
    try {
      const r = await postJSON<SetResult>("/api/config/set", { name: field.env, value });
      await onSaved();
      if (opts?.cleared) {
        toast(`${field.label} cleared`, "success");
      } else if (r.env_pinned) {
        toast(`${field.label} saved, but an external startup value overrides it — restart without that override to apply`, "info");
      } else if (r.reload_error) {
        toast(`${field.label} saved; live reload failed: ${r.reload_error}`, "error");
      } else if (r.applied === "live") {
        toast(`${field.label} applied live`, "success");
      } else {
        toast(`${field.label} saved — restart to apply`, "success");
      }
      if (field.secret) setDraft("");
      return true;
    } catch (e) {
      toast((e as Error).message, "error");
      return false;
    } finally {
      setBusy(false);
    }
  }

  const statusText = managed
    ? pinned
      ? "external"
      : "read-only"
    : field.secret
      ? isSet
        ? "set"
        : "not set"
      : entry?.value || "not set";

  return (
    <div className="flex items-center gap-2 rounded-lg border border-border/70 bg-panel/35 px-2.5 py-2">
      <span className="grid size-7 shrink-0 place-items-center rounded-md bg-card text-muted ring-1 ring-inset ring-border">
        {field.secret ? <Lock className="size-3.5" /> : field.apply === "live" ? <Zap className="size-3.5" /> : <RotateCw className="size-3.5" />}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="truncate text-xs font-medium text-foreground" title={field.env}>{field.label}</span>
          {field.apply === "live" ? <Badge variant="accent">live</Badge> : <Badge variant="default">restart</Badge>}
          {locked && !managed && <Badge variant="warn">locked</Badge>}
          {managed && <Badge variant="default">{pinned ? "external" : "read-only"}</Badge>}
        </div>
        <div className="mt-0.5 flex min-w-0 items-center gap-2 text-[11px] text-muted">
          <code className="truncate">{field.env.replace(/^AGEZT_/, "")}</code>
          <span className={cn("truncate", isSet && !managed && "text-good")}>{statusText}</span>
        </div>
      </div>
      {managed ? (
        <ManagedValue field={field} entry={entry} chip={pinned ? "external" : "read-only"} compact />
      ) : (
        <>
          <Button size="sm" variant="ghost" onClick={() => setOpen(true)} disabled={busy} aria-label={`Edit ${field.label}`}>
            <SlidersHorizontal className="size-3.5" /> Edit
          </Button>
          {!locked && field.secret && isSet && (
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => save("", { cleared: true })} title="Clear (remove from vault)">
              <Trash2 className="size-3.5 text-bad" />
            </Button>
          )}
        </>
      )}
      {open && (
        <ConfigModal title={field.label} icon={field.secret ? Lock : SlidersHorizontal} onClose={() => setOpen(false)}>
          <div className="space-y-3">
            <div className="rounded-lg border border-border bg-panel/40 p-2 text-xs text-muted">
              <div className="font-mono text-foreground">{field.env}</div>
              {field.help && <div className="mt-1 leading-relaxed">{field.help}</div>}
              {field.secret && isSet && (
                <div className="mt-2 inline-flex items-center gap-1 text-good">
                  <Check className="size-3" /> set — type a new value to replace
                </div>
              )}
            </div>
            <FieldInput field={field} value={draft} setValue={setDraft} isSet={isSet} disabled={busy} onEnter={async () => {
              if (dirty && await save(draft)) setOpen(false);
            }} />
            <div className="flex justify-end gap-2">
              <Button size="sm" variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
              <Button
                size="sm"
                variant={dirty ? "default" : "ghost"}
                disabled={!dirty || busy}
                onClick={async () => {
                  if (await save(draft)) setOpen(false);
                }}
                title="Save"
              >
                {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
              </Button>
            </div>
          </div>
        </ConfigModal>
      )}
    </div>
  );
}

// ManagedValue renders a non-editable field read-only — either externally pinned
// (the real process environment owns it) or schema read-only (system-managed, chip
// "read-only"). Secrets show presence text instead of the value.
function ManagedValue({ field, entry, chip, compact = false }: { field: Field; entry?: ValueEntry; chip: string; compact?: boolean }) {
  return (
    <div className={cn("flex h-8 items-center gap-1.5 rounded-md border border-dashed border-border bg-panel/50 px-2.5 text-xs text-muted", compact ? "hidden max-w-52 md:flex" : "w-full")}>
      <Lock className="size-3 shrink-0" />
      {field.secret ? (
        <span>{entry?.set ? "set (managed)" : "not set"}</span>
      ) : (
        <span className="truncate font-mono" title={entry?.value}>
          {entry?.value || "—"}
        </span>
      )}
      <span className="ml-auto rounded bg-card px-1 text-[9px] uppercase">{chip}</span>
    </div>
  );
}

function ConfigModal({
  title,
  icon: Icon,
  onClose,
  children,
}: {
  title: string;
  icon: LucideIcon;
  onClose: () => void;
  children: React.ReactNode;
}) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="modal-overlay fixed inset-0 z-[160] flex items-start justify-center overflow-y-auto bg-black/55 p-4 backdrop-blur-sm" onClick={onClose}>
      <div
        className="modal-in mt-10 w-full max-w-xl rounded-lg border border-border bg-card p-4 shadow-xl shadow-black/30"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="mb-3 flex items-center gap-2">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent ring-1 ring-inset ring-accent/25">
            <Icon className="size-4" />
          </span>
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          <button className="ml-auto rounded-md p-1 text-muted transition-colors hover:bg-panel hover:text-foreground" onClick={onClose} aria-label="Close modal">
            <X className="size-4" />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

function FieldInput({
  field,
  value,
  setValue,
  isSet,
  disabled,
  onEnter,
}: {
  field: Field;
  value: string;
  setValue: (v: string) => void;
  isSet: boolean;
  disabled: boolean;
  onEnter: () => void;
}) {
  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      onEnter();
    }
  };

  if (field.type === "bool") {
    return (
      <NativeSelect value={value} disabled={disabled} onChange={setValue} options={[["", "—"], ["on", "On"], ["off", "Off"]]} />
    );
  }
  if (field.type === "select") {
    return (
      <NativeSelect
        value={value}
        disabled={disabled}
        onChange={setValue}
        options={(field.options || []).map((o) => [o, o === "" ? "—" : o])}
      />
    );
  }
  return (
    <Input
      type={field.type === "password" ? "password" : field.type === "number" ? "number" : "text"}
      value={value}
      disabled={disabled}
      onChange={(e) => setValue(e.target.value)}
      onKeyDown={onKey}
      autoComplete={field.secret ? "new-password" : "off"}
      placeholder={field.secret ? (isSet ? "•••••••• (set)" : "not set") : field.type === "csv" ? "comma,separated" : ""}
      className="font-mono"
    />
  );
}

function NativeSelect({
  value,
  onChange,
  options,
  disabled,
}: {
  value: string;
  onChange: (v: string) => void;
  options: [string, string][];
  disabled: boolean;
}) {
  return (
    <div className="flex flex-wrap gap-1.5" role="group" aria-label="Config option">
      {options.map(([v, label]) => (
        <button
          key={v}
          type="button"
          disabled={disabled}
          aria-pressed={value === v}
          onClick={() => onChange(v)}
          className={cn(
            "inline-flex min-h-8 items-center rounded-md border px-2 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50",
            value === v
              ? "border-accent bg-accent/15 text-accent"
              : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
          )}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

function AgentConfigRatingPicker({
  value,
  onChange,
}: {
  value: NonNullable<AgentConfigEntry["rating"]>;
  onChange: (value: NonNullable<AgentConfigEntry["rating"]>) => void;
}) {
  const ratings: NonNullable<AgentConfigEntry["rating"]>[] = ["public", "internal", "restricted", "secret"];
  return (
    <div className="flex flex-wrap gap-1.5" role="group" aria-label="Agent config rating">
      {ratings.map((rating) => {
        const selected = value === rating;
        return (
          <button
            key={rating}
            type="button"
            aria-pressed={selected}
            onClick={() => onChange(rating)}
            className={cn(
              "inline-flex h-8 items-center rounded-md border px-2 text-xs font-medium transition-colors",
              selected
                ? "border-accent bg-accent/15 text-accent"
                : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
            )}
          >
            {rating}
          </button>
        );
      })}
    </div>
  );
}
