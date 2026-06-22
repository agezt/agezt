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
  type LucideIcon,
} from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader } from "@/components/ui/page-header";
import { Disclosure } from "@/components/ui/disclosure";
import { EmptyState } from "@/components/ui/empty";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";

// The Config Center is the editable companion to the read-only Config view:
// schema-driven forms (one section per channel/area) backed by the daemon's
// config store + vault. The schema is dynamic — built-in sections plus any a
// skill/plugin has registered (M695) — so this view groups sections into Core /
// Channels / Skills & Plugins, offers a sticky section nav and a search filter,
// and badges registered sections by their provenance. Secrets stay write-only
// (presence only, never the value); env-pinned fields are read-only because the
// real .env wins. Each save reports live (provider/model) vs restart.

type FieldType = "text" | "password" | "number" | "bool" | "csv" | "select";

interface Field {
  env: string;
  label: string;
  type: FieldType;
  secret: boolean;
  required: boolean;
  help?: string;
  apply: "live" | "restart";
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
interface ValueEntry {
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

export function ConfigCenter() {
  const { toast } = useUI();
  const [sections, setSections] = useState<Section[] | null>(null);
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
      const [sch] = await Promise.all([getJSON<{ sections?: Section[] }>("/api/config/schema"), loadValues()]);
      setSections(sch.sections || []);
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

  const jumpTo = (id: string) => {
    const el = typeof document !== "undefined" ? document.getElementById(sectionDomID(id)) : null;
    el?.scrollIntoView?.({ behavior: "smooth", block: "start" });
  };

  return (
    <div className="space-y-4">
      <PageHeader
        icon={SlidersHorizontal}
        title="Config Center"
        description={sections ? `${setCount} of ${Object.keys(values).length} configured` : "Edit settings without touching .env"}
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

      <p className="text-xs text-muted">
        Edit settings without touching <code className="rounded bg-panel px-1">.env</code>. Open a section to see its
        fields; secrets stay encrypted (shown only as “set / not set”).
      </p>

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
                  <div className="mb-1 px-2 text-xs font-semibold uppercase tracking-wider text-muted">{cat}</div>
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
                <h3 className="text-[11px] font-semibold uppercase tracking-wider text-muted">{cat}</h3>
                {grouped[cat].map((sec) => (
                  <SectionCard
                    key={sec.id + (query.trim() ? "·q" : "")}
                    section={sec}
                    values={values}
                    onSaved={loadValues}
                    toast={toast}
                    defaultOpen={!!query.trim()}
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

function AgentRuntimeConfigPanel({ toast }: { toast: (text: string, kind?: "success" | "error" | "info") => void }) {
  const [entries, setEntries] = useState<AgentConfigEntry[] | null>(null);
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
      <div className="mb-3 flex flex-wrap items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <Shield className="size-4 text-accent" />
            <h3 className="text-xs font-semibold uppercase tracking-wider text-foreground">Agent Runtime Config</h3>
          </div>
          <p className="mt-0.5 text-[11px] text-muted">
            Rated key/value entries agents can read through Config Center policies. Use allow/deny lists to bind secrets or runtime knobs to identities.
          </p>
        </div>
        {entries && (
          <div className="flex flex-wrap gap-1.5 text-xs font-medium uppercase tracking-wider text-muted">
            <span className="rounded bg-panel px-1.5 py-0.5">{summary.total} total</span>
            <span className="rounded bg-panel px-1.5 py-0.5">{summary.identityBound} identity-bound</span>
            <span className="rounded bg-panel px-1.5 py-0.5">{summary.shared} shared</span>
            <span className="rounded bg-panel px-1.5 py-0.5">{summary.restricted} restricted</span>
            <span className="rounded bg-panel px-1.5 py-0.5">{summary.secret} secret</span>
          </div>
        )}
        <Button size="sm" variant="ghost" onClick={() => load()} disabled={busy}>
          <RefreshCw className={cn("size-3.5", busy && "animate-spin")} /> Refresh
        </Button>
      </div>

      <div className="grid gap-2 lg:grid-cols-[1.2fr_1.2fr_9rem_1fr_1fr_auto]">
        <Input value={key} onChange={(e) => setKey(e.target.value)} placeholder="agent/ops/runtime" aria-label="Agent config key" />
        <Input value={value} onChange={(e) => setValue(e.target.value)} placeholder="value" aria-label="Agent config value" />
        <select
          value={rating}
          onChange={(e) => setRating(e.target.value as NonNullable<AgentConfigEntry["rating"]>)}
          aria-label="Agent config rating"
          className="h-8 rounded-md border border-border bg-panel px-2 text-xs text-foreground outline-none focus-visible:border-accent"
        >
          <option value="public">public</option>
          <option value="internal">internal</option>
          <option value="restricted">restricted</option>
          <option value="secret">secret</option>
        </select>
        <Input value={allow} onChange={(e) => setAllow(e.target.value)} placeholder="allow: ops,planner" aria-label="Allowed agents" />
        <Input value={deny} onChange={(e) => setDeny(e.target.value)} placeholder="deny: blocked" aria-label="Denied agents" />
        <Button size="sm" onClick={save} disabled={busy || !key.trim() || !value.trim()} aria-label="Save agent config">
          <Save className="size-3.5" /> Save
        </Button>
      </div>
      <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Optional description" aria-label="Agent config description" className="mt-2" />

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
                <span className={cn("w-fit rounded px-1.5 py-0.5 text-xs uppercase", ratingClass(entry.rating || "internal"))}>{entry.rating || "internal"}</span>
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
  defaultOpen = false,
}: {
  section: Section;
  values: Record<string, ValueEntry>;
  onSaved: () => Promise<void>;
  toast: (text: string, kind?: "success" | "error" | "info") => void;
  defaultOpen?: boolean;
}) {
  const Icon = iconFor(section);
  const registered = isRegistered(section);
  // Collapsed by default — show the section + how many of its fields are set, and
  // reveal the inputs only when the operator opens it ("gözüme sokmamalısın").
  const setCount = section.fields.filter((f) => values[f.env]?.set).length;
  return (
    <div id={sectionDomID(section.id)} className="scroll-mt-2 rounded-lg border border-border bg-card">
      <Disclosure
        defaultOpen={defaultOpen}
        summaryClassName="px-3 py-2.5"
        summary={
          <span className="flex w-full items-center gap-2">
            <Icon className={cn("size-4 shrink-0", registered ? "text-violet-400" : "text-accent")} />
            {/* role=heading (not <h3>) keeps the heading semantics valid inside the
                summary button and still matches getByRole("heading"). */}
            <span role="heading" aria-level={3} className="text-xs font-semibold uppercase tracking-wider text-foreground">
              {section.name}
            </span>
            {registered && (
              <span
                className="inline-flex items-center gap-1 rounded bg-violet-500/15 px-1.5 py-0.5 text-[9px] font-medium uppercase tracking-wide text-violet-300"
                title={`Registered by ${section.source}`}
              >
                <Puzzle className="size-2.5" /> registered
              </span>
            )}
            <span className="ml-auto shrink-0 font-mono text-xs text-muted">
              {setCount}/{section.fields.length}
            </span>
          </span>
        }
      >
        <div className="space-y-3 px-3 pb-3">
          {section.help && <p className="text-[11px] text-muted">{section.help}</p>}
          {section.fields.map((f) => (
            <FieldRow key={f.env} field={f} entry={values[f.env]} onSaved={onSaved} toast={toast} />
          ))}
        </div>
      </Disclosure>
    </div>
  );
}

function FieldRow({
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

  // Re-sync the draft when the upstream value changes (after a save/refresh),
  // but only for non-secret fields (secrets always start blank).
  useEffect(() => {
    if (!field.secret) setDraft(entry?.value ?? "");
  }, [entry?.value, field.secret]);

  // Non-secret: dirty when changed. Secret: dirty only when something was typed
  // (a blank secret save would CLEAR it — that path is the explicit Clear button).
  const dirty = field.secret ? draft.trim() !== "" : draft !== original;

  async function save(value: string, opts?: { cleared?: boolean }) {
    setBusy(true);
    try {
      const r = await postJSON<SetResult>("/api/config/set", { name: field.env, value });
      await onSaved();
      if (opts?.cleared) {
        toast(`${field.label} cleared`, "success");
      } else if (r.env_pinned) {
        toast(`${field.label} saved, but ${field.env} is set in the environment — restart with it unset to apply`, "info");
      } else if (r.reload_error) {
        toast(`${field.label} saved; live reload failed: ${r.reload_error}`, "error");
      } else if (r.applied === "live") {
        toast(`${field.label} applied live`, "success");
      } else {
        toast(`${field.label} saved — restart to apply`, "success");
      }
      if (field.secret) setDraft("");
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid grid-cols-1 gap-1.5 sm:grid-cols-[minmax(10rem,14rem)_1fr] sm:items-start sm:gap-3">
      <div className="flex flex-col gap-0.5 pt-1.5">
        <div className="flex items-center gap-1.5">
          <label className="text-xs font-medium text-foreground" title={field.env}>
            {field.label}
          </label>
          {field.apply === "live" ? (
            <span className="inline-flex items-center gap-0.5 rounded bg-accent/15 px-1 text-[9px] font-medium uppercase text-accent" title="Applies immediately">
              <Zap className="size-2.5" /> live
            </span>
          ) : (
            <span className="inline-flex items-center gap-0.5 rounded bg-panel px-1 text-[9px] font-medium uppercase text-muted" title="Needs a restart">
              <RotateCw className="size-2.5" /> restart
            </span>
          )}
          {locked && !managed && (
            <span className="inline-flex items-center gap-0.5 rounded bg-amber-500/15 px-1 text-[9px] font-medium uppercase text-amber-300" title="Locked — can be changed but not cleared">
              <Lock className="size-2.5" /> locked
            </span>
          )}
        </div>
        <code className="text-xs text-muted">{field.env.replace(/^AGEZT_/, "")}</code>
        {field.help && <p className="text-xs leading-snug text-muted">{field.help}</p>}
      </div>

      <div className="flex flex-col gap-1">
        <div className="flex items-center gap-1.5">
          {managed ? (
            <ManagedValue field={field} entry={entry} chip={pinned ? "env" : "read-only"} />
          ) : (
            <FieldInput field={field} value={draft} setValue={setDraft} isSet={isSet} disabled={busy} onEnter={() => dirty && save(draft)} />
          )}

          {!managed && (
            <Button size="sm" variant={dirty ? "default" : "ghost"} disabled={!dirty || busy} onClick={() => save(draft)} title="Save">
              {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
            </Button>
          )}
          {!managed && !locked && field.secret && isSet && (
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => save("", { cleared: true })} title="Clear (remove from vault)">
              <Trash2 className="size-3.5 text-bad" />
            </Button>
          )}
        </div>

        {field.secret && !managed && (
          <span className={cn("inline-flex items-center gap-1 text-xs", isSet ? "text-good" : "text-muted")}>
            {isSet ? (
              <>
                <Check className="size-2.5" /> set — type a new value to replace
              </>
            ) : (
              "not set"
            )}
          </span>
        )}
      </div>
    </div>
  );
}

// ManagedValue renders a non-editable field read-only — either env-pinned (the
// real environment owns it, chip "env") or schema read-only (system-managed, chip
// "read-only"). Secrets show presence text instead of the value.
function ManagedValue({ field, entry, chip }: { field: Field; entry?: ValueEntry; chip: string }) {
  return (
    <div className="flex h-8 w-full items-center gap-1.5 rounded-md border border-dashed border-border bg-panel/50 px-2.5 text-xs text-muted">
      <Lock className="size-3 shrink-0" />
      {field.secret ? (
        <span>{entry?.set ? "set (managed)" : "not set"}</span>
      ) : (
        <span className="truncate font-mono" title={entry?.value}>
          {entry?.value || "—"}
        </span>
      )}
      <span className="ml-auto rounded bg-card px-1 text-[9px] uppercase tracking-wide">{chip}</span>
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
    <select
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      className="h-8 w-full rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
    >
      {options.map(([v, label]) => (
        <option key={v} value={v}>
          {label}
        </option>
      ))}
    </select>
  );
}
