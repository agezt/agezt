import { useCallback, useEffect, useMemo, useState } from "react";
import { Settings2, Ear, Volume2, RefreshCw, Wand2 } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { CollapsibleSection } from "@/components/ui/collapsible-section";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useUI } from "@/components/ui/feedback";
import { FieldRow, type Field, type ValueEntry } from "@/views/ConfigCenter";

// VoiceSetup is the inline "set it up here" panel that lives inside the Voice
// cockpit so the operator never has to wander into the Config Center to wire up
// speech. It edits the daemon's "voice" config section (the 7 AGEZT_STT_*/
// AGEZT_TTS_* fields) using the very same field editor + /api/config/set path as
// the Config Center, so secrets, env-pinning and validation behave identically.

interface SchemaSection {
  id: string;
  fields?: Field[];
}
interface SchemaResponse {
  sections?: SchemaSection[];
}

// FALLBACK mirrors kernel/settings/schema.go's "voice" section verbatim so the
// panel stays usable even when the schema can't be fetched (older daemon / test).
const FALLBACK: Field[] = [
  { env: "AGEZT_STT_URL", label: "Transcription API URL", type: "text", secret: false, required: false, apply: "restart", help: "OpenAI-compatible API root for /audio/transcriptions, e.g. http://localhost:8000 or https://api.openai.com/v1." },
  { env: "AGEZT_STT_MODEL", label: "Transcription model", type: "text", secret: false, required: false, apply: "restart", help: "e.g. whisper-1 (OpenAI) or Systran/faster-whisper-base." },
  { env: "AGEZT_STT_KEY", label: "Transcription API key", type: "password", secret: true, required: false, apply: "restart", help: "Bearer token for hosted APIs; leave empty for a local server." },
  { env: "AGEZT_TTS_URL", label: "Synthesis API URL", type: "text", secret: false, required: false, apply: "restart", help: "OpenAI-compatible API root for /audio/speech." },
  { env: "AGEZT_TTS_MODEL", label: "Synthesis model", type: "text", secret: false, required: false, apply: "restart", help: "e.g. tts-1 (OpenAI) or kokoro." },
  { env: "AGEZT_TTS_VOICE", label: "Voice", type: "text", secret: false, required: false, apply: "restart", help: "voice name, e.g. alloy (default)." },
  { env: "AGEZT_TTS_KEY", label: "Synthesis API key", type: "password", secret: true, required: false, apply: "restart", help: "Bearer token for hosted APIs; leave empty for a local server." },
];

const STT_ENVS = ["AGEZT_STT_URL", "AGEZT_STT_MODEL", "AGEZT_STT_KEY"];
const TTS_ENVS = ["AGEZT_TTS_URL", "AGEZT_TTS_MODEL", "AGEZT_TTS_VOICE", "AGEZT_TTS_KEY"];

// Presets prefill the non-secret endpoints from the project's own documented
// values (the schema help text) — keys are always left for the operator.
type Preset = { id: string; label: string; note: string; values: Record<string, string> };
const PRESETS: Preset[] = [
  {
    id: "openai",
    label: "OpenAI",
    note: "OpenAI endpoints filled — add your API key to both key fields, then restart.",
    values: {
      AGEZT_STT_URL: "https://api.openai.com/v1",
      AGEZT_STT_MODEL: "whisper-1",
      AGEZT_TTS_URL: "https://api.openai.com/v1",
      AGEZT_TTS_MODEL: "tts-1",
      AGEZT_TTS_VOICE: "alloy",
    },
  },
  {
    id: "local",
    label: "Local server",
    note: "Local faster-whisper + Kokoro endpoints filled — no key needed. Start those servers, then restart.",
    values: {
      AGEZT_STT_URL: "http://localhost:8000/v1",
      AGEZT_STT_MODEL: "Systran/faster-whisper-base",
      AGEZT_TTS_URL: "http://localhost:8880/v1",
      AGEZT_TTS_MODEL: "kokoro",
    },
  },
];

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
      /* degrade: leave the last-known values in place */
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

  const byEnv = useMemo(() => {
    const m: Record<string, Field> = {};
    for (const f of fields) m[f.env] = f;
    return m;
  }, [fields]);

  const isSet = useCallback((env: string) => !!values[env]?.set, [values]);
  const sttReady = isSet("AGEZT_STT_URL") && isSet("AGEZT_STT_MODEL");
  const ttsReady = isSet("AGEZT_TTS_URL") && isSet("AGEZT_TTS_MODEL");
  const ready = sttReady && ttsReady;
  const setCount = useMemo(() => [...STT_ENVS, ...TTS_ENVS].filter((e) => isSet(e)).length, [isSet]);

  async function applyPreset(p: Preset) {
    setBusy(true);
    try {
      for (const [env, val] of Object.entries(p.values)) {
        if (values[env]?.env_pinned) continue; // the real .env owns it — don't fight it
        await postJSON("/api/config/set", { name: env, value: val });
      }
      await loadValues();
      toast(`${p.label} preset applied — ${p.note}`, "success");
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  const renderGroup = (envs: string[]) =>
    envs
      .map((env) => byEnv[env])
      .filter((f): f is Field => !!f)
      .map((f) => <FieldRow key={f.env} field={f} entry={values[f.env]} onSaved={loadValues} toast={toast} />);

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
      <p className="mb-3 text-xs leading-relaxed text-muted">
        Point AGEZT at OpenAI-compatible speech servers so it can hear you (transcription) and talk back (synthesis).
        Each half works on its own. Saved here straight to the daemon — <strong className="text-foreground">restart to apply</strong>.
      </p>

      {/* Readiness + quick-fill presets */}
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Badge variant={sttReady ? "good" : "warn"}>
          <Ear className="size-2.5" /> Hearing {sttReady ? "configured" : "not set"}
        </Badge>
        <Badge variant={ttsReady ? "good" : "warn"}>
          <Volume2 className="size-2.5" /> Voice {ttsReady ? "configured" : "not set"}
        </Badge>
        <span className="ml-1 text-xs text-muted">Quick fill:</span>
        {PRESETS.map((p) => (
          <Button key={p.id} size="sm" variant="ghost" disabled={busy} onClick={() => applyPreset(p)} title={p.note}>
            <Wand2 className="size-3.5" /> {p.label}
          </Button>
        ))}
      </div>

      <div className="grid gap-x-6 gap-y-4 md:grid-cols-2">
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wide text-muted">
            <Ear className="size-3.5" /> Hearing · Speech to text
          </div>
          {renderGroup(STT_ENVS)}
        </div>
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wide text-muted">
            <Volume2 className="size-3.5" /> Voice · Text to speech
          </div>
          {renderGroup(TTS_ENVS)}
        </div>
      </div>
    </CollapsibleSection>
  );
}
