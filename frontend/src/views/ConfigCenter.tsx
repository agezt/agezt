import { useCallback, useEffect, useMemo, useState } from "react";
import { SlidersHorizontal, RefreshCw, Lock, Save, Trash2, Zap, RotateCw, Check } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";

// The Config Center is the editable companion to the read-only Config view:
// schema-driven forms (one section per channel/area) backed by the daemon's
// config store + vault. Secrets are write-only — the value is never returned,
// only "set / not set"; env-pinned fields (set in the real .env/shell) are shown
// read-only because the process env always wins over the store. Each save reports
// whether it applied live (provider/model) or needs a restart.

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

export function ConfigCenter() {
  const { toast } = useUI();
  const [sections, setSections] = useState<Section[] | null>(null);
  const [values, setValues] = useState<Record<string, ValueEntry>>({});
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

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

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <SlidersHorizontal className="size-4 text-accent" /> Config Center
        </h2>
        {sections && (
          <span className="text-xs text-muted">
            {setCount} of {Object.keys(values).length} configured
          </span>
        )}
        <Button variant="ghost" size="sm" className="ml-auto" onClick={reload} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} /> Refresh
        </Button>
      </div>

      <p className="text-xs text-muted">
        Edit settings without touching <code className="rounded bg-panel px-1">.env</code>. Secrets are stored encrypted
        and never shown back — only “set / not set”. Fields pinned by the environment are read-only (the real{" "}
        <code className="rounded bg-panel px-1">.env</code> wins). Provider &amp; model apply live; everything else needs a
        restart.
      </p>

      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !sections ? (
        <SkeletonList count={4} lines={2} />
      ) : (
        <div className="space-y-3">
          {sections.map((sec) => (
            <SectionCard key={sec.id} section={sec} values={values} onSaved={loadValues} toast={toast} />
          ))}
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
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="mb-2">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-accent">{section.name}</h3>
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