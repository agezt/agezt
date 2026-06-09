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
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <SlidersHorizontal className="size-4 text-accent" /> Config Center
        </h2>
        {sections && (
          <span className="text-xs text-muted">
            {setCount} of {Object.keys(values).length} configured
          </span>
        )}
        <div className="relative ml-auto">
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
      </div>

      <p className="text-xs text-muted">
        Edit settings without touching <code className="rounded bg-panel px-1">.env</code>. Secrets are stored encrypted
        and never shown back — only “set / not set”. Fields pinned by the environment are read-only (the real{" "}
        <code className="rounded bg-panel px-1">.env</code> wins). Provider &amp; model apply live; everything else needs
        a restart. Skills &amp; plugins can register their own sections.
      </p>

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
                  <div className="mb-1 px-2 text-[10px] font-semibold uppercase tracking-wider text-muted">{cat}</div>
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
                  <SectionCard key={sec.id} section={sec} values={values} onSaved={loadValues} toast={toast} />
                ))}
              </section>
            ))}
          </div>
        </div>
      )}
    </div>
  );
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
  return (
    <div id={sectionDomID(section.id)} className="scroll-mt-2 rounded-lg border border-border bg-card p-3">
      <div className="mb-2">
        <div className="flex items-center gap-2">
          <Icon className={cn("size-4", registered ? "text-violet-400" : "text-accent")} />
          <h3 className="text-xs font-semibold uppercase tracking-wider text-foreground">{section.name}</h3>
          {registered && (
            <span
              className="inline-flex items-center gap-1 rounded bg-violet-500/15 px-1.5 py-0.5 text-[9px] font-medium uppercase tracking-wide text-violet-300"
              title={`Registered by ${section.source}`}
            >
              <Puzzle className="size-2.5" /> registered
            </span>
          )}
        </div>
        {section.help && <p className="mt-0.5 text-[11px] text-muted">{section.help}</p>}
      </div>
      <div className="space-y-3">
        {section.fields.map((f) => (
          <FieldRow key={f.env} field={f} entry={values[f.env]} onSaved={onSaved} toast={toast} />
        ))}
      </div>
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
        </div>
        <code className="text-[10px] text-muted">{field.env.replace(/^AGEZT_/, "")}</code>
        {field.help && <p className="text-[10px] leading-snug text-muted">{field.help}</p>}
      </div>

      <div className="flex flex-col gap-1">
        <div className="flex items-center gap-1.5">
          {pinned ? (
            <PinnedValue field={field} entry={entry} />
          ) : (
            <FieldInput field={field} value={draft} setValue={setDraft} isSet={isSet} disabled={busy} onEnter={() => dirty && save(draft)} />
          )}

          {!pinned && (
            <Button size="sm" variant={dirty ? "default" : "ghost"} disabled={!dirty || busy} onClick={() => save(draft)} title="Save">
              {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
            </Button>
          )}
          {!pinned && field.secret && isSet && (
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => save("", { cleared: true })} title="Clear (remove from vault)">
              <Trash2 className="size-3.5 text-bad" />
            </Button>
          )}
        </div>

        {field.secret && !pinned && (
          <span className={cn("inline-flex items-center gap-1 text-[10px]", isSet ? "text-good" : "text-muted")}>
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

// PinnedValue renders an env-pinned field read-only: the real environment owns it.
function PinnedValue({ field, entry }: { field: Field; entry?: ValueEntry }) {
  return (
    <div className="flex h-8 w-full items-center gap-1.5 rounded-md border border-dashed border-border bg-panel/50 px-2.5 text-xs text-muted">
      <Lock className="size-3 shrink-0" />
      {field.secret ? (
        <span>set in environment</span>
      ) : (
        <span className="truncate font-mono" title={entry?.value}>
          {entry?.value || "—"}
        </span>
      )}
      <span className="ml-auto rounded bg-card px-1 text-[9px] uppercase tracking-wide">env</span>
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
