import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Settings2,
  Ear,
  Volume2,
  RefreshCw,
  Check,
  KeyRound,
  Laptop,
  Cloud,
  ChevronDown,
  Sliders,
  ExternalLink,
} from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { CollapsibleSection } from "@/components/ui/collapsible-section";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { useUI } from "@/components/ui/feedback";
import { cn } from "@/lib/utils";
import { FieldRow, type Field, type ValueEntry } from "@/views/ConfigCenter";
import {
  STT_PROVIDERS,
  TTS_PROVIDERS,
  providerFor,
  voicesFor,
  type SpeechProvider,
} from "@/lib/voiceCatalog";

// VoiceSetup is the friendly "set it up right here" panel inside the Voice
// cockpit — pick who listens and who talks, no trip to the Config Center and no
// wall of environment variables. It turns the daemon's voice config (the 7
// AGEZT_STT_*/AGEZT_TTS_* fields) into two warm cards: choose a provider, choose
// a model (and a voice), drop in a key if it's a hosted one. The raw fields are
// still there for custom endpoints, tucked under "Advanced".

interface SchemaSection {
  id: string;
  fields?: Field[];
}
interface SchemaResponse {
  sections?: SchemaSection[];
}

// FALLBACK mirrors kernel/settings/schema.go's "voice" section so the Advanced
// editor works even when the schema can't be fetched (older daemon / test).
const FALLBACK: Field[] = [
  { env: "AGEZT_STT_URL", label: "Transcription API URL", type: "text", secret: false, required: false, apply: "restart", help: "OpenAI-compatible API root for /audio/transcriptions." },
  { env: "AGEZT_STT_MODEL", label: "Transcription model", type: "text", secret: false, required: false, apply: "restart", help: "e.g. whisper-1 or Systran/faster-whisper-base." },
  { env: "AGEZT_STT_KEY", label: "Transcription API key", type: "password", secret: true, required: false, apply: "restart", help: "Bearer token for hosted APIs; empty for a local server." },
  { env: "AGEZT_TTS_URL", label: "Synthesis API URL", type: "text", secret: false, required: false, apply: "restart", help: "OpenAI-compatible API root for /audio/speech." },
  { env: "AGEZT_TTS_MODEL", label: "Synthesis model", type: "text", secret: false, required: false, apply: "restart", help: "e.g. tts-1 or kokoro." },
  { env: "AGEZT_TTS_VOICE", label: "Voice", type: "text", secret: false, required: false, apply: "restart", help: "voice name, e.g. alloy." },
  { env: "AGEZT_TTS_KEY", label: "Synthesis API key", type: "password", secret: true, required: false, apply: "restart", help: "Bearer token for hosted APIs; empty for a local server." },
];

type Half = "stt" | "tts";
const ENVS: Record<Half, { url: string; model: string; voice?: string; key: string }> = {
  stt: { url: "AGEZT_STT_URL", model: "AGEZT_STT_MODEL", key: "AGEZT_STT_KEY" },
  tts: { url: "AGEZT_TTS_URL", model: "AGEZT_TTS_MODEL", voice: "AGEZT_TTS_VOICE", key: "AGEZT_TTS_KEY" },
};

export function VoiceSetup() {
  const { toast } = useUI();
  const [fields, setFields] = useState<Field[]>(FALLBACK);
  const [values, setValues] = useState<Record<string, ValueEntry>>({});
  const [busy, setBusy] = useState(false);

  const loadValues = useCallback(async () => {
    try {
      const v = await getJSON<{ fields?: ValueEntry[] }>("/api/config/values");
      const map: Record<string, ValueEntry> = {};
      for (const f of v.fields || []) map[f.env] = f;
      setValues(map);
    } catch {
      /* degrade: keep last-known values */
    }
  }, []);

  const loadSchema = useCallback(async () => {
    try {
      const s = await getJSON<SchemaResponse>("/api/config/schema");
      const sec = s.sections?.find((x) => x.id === "voice");
      if (sec?.fields?.length) setFields(sec.fields);
    } catch {
      /* keep FALLBACK */
    }
  }, []);

  const refresh = useCallback(() => {
    loadSchema();
    loadValues();
  }, [loadSchema, loadValues]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // saveConfig writes one field and refreshes — shared by every picker so the
  // restart-apply + env-pinned semantics stay identical to the Config Center.
  const saveConfig = useCallback(
    async (env: string, value: string, quiet = false) => {
      setBusy(true);
      try {
        const r = await postJSON<{ env_pinned?: boolean }>("/api/config/set", { name: env, value });
        await loadValues();
        if (r?.env_pinned) toast(`${env} is pinned in the environment — unset it to change here`, "info");
        else if (!quiet) toast("Saved — restart to apply", "success");
        return true;
      } catch (e) {
        toast((e as Error).message, "error");
        return false;
      } finally {
        setBusy(false);
      }
    },
    [loadValues, toast],
  );

  const isSet = useCallback((env: string) => !!values[env]?.set, [values]);
  const sttReady = isSet("AGEZT_STT_URL") && isSet("AGEZT_STT_MODEL");
  const ttsReady = isSet("AGEZT_TTS_URL") && isSet("AGEZT_TTS_MODEL");
  const ready = sttReady && ttsReady;
  const setCount = useMemo(
    () => ["AGEZT_STT_URL", "AGEZT_STT_MODEL", "AGEZT_STT_KEY", "AGEZT_TTS_URL", "AGEZT_TTS_MODEL", "AGEZT_TTS_VOICE", "AGEZT_TTS_KEY"].filter((e) => isSet(e)).length,
    [isSet],
  );

  const fieldsByEnv = useMemo(() => {
    const m: Record<string, Field> = {};
    for (const f of fields) m[f.env] = f;
    return m;
  }, [fields]);

  return (
    <CollapsibleSection
      icon={Settings2}
      title="Voice setup"
      tone={ready ? "good" : "warn"}
      defaultOpen={!ready}
      count={`${setCount}/7`}
      actions={
        <Button size="sm" variant="ghost" onClick={refresh} disabled={busy} aria-label="Refresh voice setup">
          <RefreshCw className={busy ? "size-3.5 animate-spin" : "size-3.5"} /> Refresh
        </Button>
      }
    >
      <p className="mb-4 text-xs leading-relaxed text-muted">
        {ready
          ? "AGEZT can hear you and talk back. Pick a different provider any time — changes need a daemon restart to take effect."
          : "Choose who does the listening and who does the talking. Hosted options need an API key; local ones run on your machine, free. Changes apply after a restart."}
      </p>

      <div className="grid gap-4 lg:grid-cols-2">
        <SpeechHalf
          half="stt"
          icon={Ear}
          title="Hearing"
          subtitle="Speech → text, so AGEZT understands you"
          providers={STT_PROVIDERS}
          values={values}
          fieldsByEnv={fieldsByEnv}
          ready={sttReady}
          busy={busy}
          saveConfig={saveConfig}
          loadValues={loadValues}
          toast={toast}
        />
        <SpeechHalf
          half="tts"
          icon={Volume2}
          title="Voice"
          subtitle="Text → speech, so AGEZT talks back"
          providers={TTS_PROVIDERS}
          values={values}
          fieldsByEnv={fieldsByEnv}
          ready={ttsReady}
          busy={busy}
          saveConfig={saveConfig}
          loadValues={loadValues}
          toast={toast}
        />
      </div>
    </CollapsibleSection>
  );
}

function SpeechHalf({
  half,
  icon: Icon,
  title,
  subtitle,
  providers,
  values,
  fieldsByEnv,
  ready,
  busy,
  saveConfig,
  loadValues,
  toast,
}: {
  half: Half;
  icon: typeof Ear;
  title: string;
  subtitle: string;
  providers: SpeechProvider[];
  values: Record<string, ValueEntry>;
  fieldsByEnv: Record<string, Field>;
  ready: boolean;
  busy: boolean;
  saveConfig: (env: string, value: string, quiet?: boolean) => Promise<boolean>;
  loadValues: () => Promise<void>;
  toast: (text: string, kind?: "success" | "error" | "info") => void;
}) {
  const envs = ENVS[half];
  const urlVal = values[envs.url]?.value || "";
  const modelVal = values[envs.model]?.value || "";
  const voiceVal = envs.voice ? values[envs.voice]?.value || "" : "";
  const pinned = !!values[envs.url]?.env_pinned;

  const selected = providerFor(providers, urlVal);
  const urlSet = !!values[envs.url]?.set;
  // Custom is active when an endpoint is configured that we don't recognise, or
  // the operator explicitly opened it.
  const [customOpen, setCustomOpen] = useState(false);
  const customActive = customOpen || (urlSet && !selected);

  // Pick a provider: write its base URL + a sensible default model (and voice).
  async function pickProvider(p: SpeechProvider) {
    setCustomOpen(false);
    const ok = await saveConfig(envs.url, p.baseURL, true);
    if (!ok) return;
    const keepModel = p.models.some((m) => m.id === modelVal) ? modelVal : p.models[0]?.id || "";
    if (keepModel && keepModel !== modelVal) await saveConfig(envs.model, keepModel, true);
    if (envs.voice) {
      const vs = voicesFor(p, keepModel);
      if (vs.length && !vs.some((x) => x.id === voiceVal)) await saveConfig(envs.voice, vs[0].id, true);
    }
    toast(`${title}: ${p.name} selected — restart to apply`, "success");
  }

  async function pickModel(id: string) {
    await saveConfig(envs.model, id, true);
    if (envs.voice && selected) {
      const vs = voicesFor(selected, id);
      if (vs.length && !vs.some((x) => x.id === voiceVal)) await saveConfig(envs.voice, vs[0].id, true);
    }
    toast("Saved — restart to apply", "success");
  }

  const voiceList = voicesFor(selected, modelVal);
  const advancedFields = [envs.url, envs.model, envs.voice, envs.key]
    .filter((e): e is string => !!e)
    .map((e) => fieldsByEnv[e])
    .filter((f): f is Field => !!f);

  return (
    <div className="flex flex-col rounded-xl border border-border bg-panel/40 p-4">
      <div className="mb-3 flex items-start gap-2.5">
        <span className={cn("grid size-8 shrink-0 place-items-center rounded-lg ring-1 ring-inset", ready ? "bg-good/15 text-good ring-good/30" : "bg-accent/10 text-accent ring-accent/30")}>
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h4 className="text-sm font-semibold text-foreground">{title}</h4>
            <Badge variant={ready ? "good" : "warn"}>{ready ? "ready" : "not set up"}</Badge>
          </div>
          <p className="text-xs text-muted">{subtitle}</p>
        </div>
      </div>

      {pinned ? (
        <div className="rounded-lg border border-dashed border-border bg-card/50 px-3 py-2 text-xs text-muted">
          Set from the environment ({envs.url.replace("AGEZT_", "")}). Unset it to choose here.
        </div>
      ) : (
        <>
          {/* Provider chooser */}
          <div className="flex flex-wrap gap-1.5">
            {providers.map((p) => {
              const active = !customActive && selected?.id === p.id;
              return (
                <button
                  key={p.id}
                  type="button"
                  disabled={busy}
                  onClick={() => pickProvider(p)}
                  className={cn(
                    "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors disabled:opacity-50",
                    active ? "border-accent bg-accent/10 text-accent" : "border-border text-foreground/80 hover:bg-card",
                  )}
                >
                  {p.local ? <Laptop className="size-3" /> : <Cloud className="size-3" />}
                  {p.name}
                </button>
              );
            })}
            <button
              type="button"
              disabled={busy}
              onClick={() => setCustomOpen((o) => !o)}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors disabled:opacity-50",
                customActive ? "border-accent bg-accent/10 text-accent" : "border-border text-foreground/80 hover:bg-card",
              )}
            >
              <Sliders className="size-3" /> Custom…
            </button>
          </div>

          {/* Selected-provider detail: blurb, model + voice pickers, key */}
          {selected && !customActive && (
            <div className="mt-3 space-y-3">
              <p className="text-xs text-muted">{selected.blurb}</p>

              <PickRow label="Model">
                <StyledSelect
                  value={selected.models.some((m) => m.id === modelVal) ? modelVal : selected.models[0]?.id || ""}
                  disabled={busy}
                  onChange={pickModel}
                  options={selected.models.map((m) => [m.id, m.label || m.id, m.note] as [string, string, string?])}
                />
              </PickRow>

              {envs.voice &&
                (voiceList.length ? (
                  <PickRow label="Voice">
                    <StyledSelect
                      value={voiceList.some((x) => x.id === voiceVal) ? voiceVal : voiceList[0]?.id || ""}
                      disabled={busy}
                      onChange={(id) => saveConfig(envs.voice!, id)}
                      options={voiceList.map((x) => [x.id, x.label || x.id] as [string, string])}
                    />
                  </PickRow>
                ) : (
                  <PickRow label="Voice">
                    <Input
                      defaultValue={voiceVal}
                      disabled={busy}
                      placeholder="voice name (depends on your server)"
                      className="font-mono"
                      onBlur={(e) => e.target.value.trim() !== voiceVal && saveConfig(envs.voice!, e.target.value.trim())}
                    />
                  </PickRow>
                ))}

              {selected.needsKey ? (
                <KeyField env={envs.key} hint={selected.keyHint} link={selected.keyLink} entry={values[envs.key]} busy={busy} saveConfig={saveConfig} />
              ) : (
                <p className="inline-flex items-center gap-1.5 text-xs text-good">
                  <Check className="size-3" /> No API key needed — runs locally.
                </p>
              )}
            </div>
          )}

          {!selected && !customActive && <p className="mt-3 text-xs text-muted">Pick a provider above to get started.</p>}
        </>
      )}

      {/* Advanced / custom endpoint — the raw fields, same as Config Center. */}
      <CustomEndpoint open={customActive} onToggle={() => setCustomOpen((o) => !o)} fields={advancedFields} values={values} loadValues={loadValues} toast={toast} />
    </div>
  );
}

function PickRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="grid grid-cols-[4rem_1fr] items-center gap-2">
      <span className="text-xs font-medium text-muted">{label}</span>
      {children}
    </label>
  );
}

function StyledSelect({
  value,
  options,
  onChange,
  disabled,
}: {
  value: string;
  options: [string, string, string?][];
  onChange: (v: string) => void;
  disabled: boolean;
}) {
  return (
    <select
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      className="h-8 w-full rounded-md border border-border bg-surface px-2 text-sm outline-none focus-visible:border-accent disabled:opacity-50"
    >
      {options.map(([v, label, note]) => (
        <option key={v} value={v}>
          {label}
          {note ? ` · ${note}` : ""}
        </option>
      ))}
    </select>
  );
}

function KeyField({
  env,
  hint,
  link,
  entry,
  busy,
  saveConfig,
}: {
  env: string;
  hint?: string;
  link?: string;
  entry?: ValueEntry;
  busy: boolean;
  saveConfig: (env: string, value: string, quiet?: boolean) => Promise<boolean>;
}) {
  const [draft, setDraft] = useState("");
  const isSet = !!entry?.set;
  const pinned = !!entry?.env_pinned;
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-1.5">
        <KeyRound className="size-3.5 text-muted" />
        <span className="text-xs font-medium text-muted">API key</span>
        {isSet && (
          <span className="inline-flex items-center gap-0.5 text-xs text-good">
            <Check className="size-3" /> set
          </span>
        )}
        {link && (
          <a href={link} target="_blank" rel="noreferrer" className="ml-auto inline-flex items-center gap-0.5 text-xs text-accent hover:underline">
            Get one <ExternalLink className="size-3" />
          </a>
        )}
      </div>
      {pinned ? (
        <div className="rounded-md border border-dashed border-border bg-card/50 px-2.5 py-1.5 text-xs text-muted">Set from the environment.</div>
      ) : (
        <div className="flex items-center gap-1.5">
          <Input
            type="password"
            value={draft}
            disabled={busy}
            autoComplete="new-password"
            placeholder={isSet ? "•••••••• (set — type to replace)" : hint || "paste your key"}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && draft.trim()) {
                saveConfig(env, draft.trim()).then(() => setDraft(""));
              }
            }}
            className="font-mono"
          />
          <Button size="sm" disabled={busy || !draft.trim()} onClick={() => saveConfig(env, draft.trim()).then(() => setDraft(""))}>
            Save
          </Button>
        </div>
      )}
    </div>
  );
}

// CustomEndpoint reveals the raw AGEZT_*_URL/MODEL/VOICE/KEY fields for this half
// — power users and unlisted servers. Reuses Config Center's exact field editor.
function CustomEndpoint({
  open,
  onToggle,
  fields,
  values,
  loadValues,
  toast,
}: {
  open: boolean;
  onToggle: () => void;
  fields: Field[];
  values: Record<string, ValueEntry>;
  loadValues: () => Promise<void>;
  toast: (text: string, kind?: "success" | "error" | "info") => void;
}) {
  return (
    <div className="mt-3 border-t border-border/60 pt-2">
      <button type="button" onClick={onToggle} className="flex w-full items-center gap-1.5 text-xs text-muted hover:text-foreground">
        <ChevronDown className={cn("size-3.5 transition-transform", open ? "rotate-0" : "-rotate-90")} />
        Advanced · custom endpoint
      </button>
      {open && (
        <div className="mt-2 space-y-3">
          {fields.map((f) => (
            <FieldRow key={f.env} field={f} entry={values[f.env]} onSaved={loadValues} toast={toast} />
          ))}
        </div>
      )}
    </div>
  );
}
