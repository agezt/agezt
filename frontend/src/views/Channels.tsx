import { useEffect, useMemo, useState } from "react";
import { Radio, RefreshCw, Check, Settings2, ArrowLeftRight, ArrowRight, ExternalLink, Save, SendHorizontal, QrCode, Image as ImageIcon, Mic, ArrowDown, ArrowUp } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/components/ui/page-header";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { useUI } from "@/components/ui/feedback";

// One account field of a channel (mirrors kernel/settings.Field + set-state).
interface ChannelField {
  env: string;
  label: string;
  secret?: boolean;
  required?: boolean;
  help?: string;
  set?: boolean;
  value?: string;
  env_pinned?: boolean;
}

// A channel manifest joined with its account fields + configured state.
interface MediaCaps {
  image_in?: boolean;
  voice_in?: boolean;
  image_out?: boolean;
  voice_out?: boolean;
}

interface ChannelRow {
  kind: string;
  display: string;
  description?: string;
  transport?: string;
  duplex?: boolean;
  media?: MediaCaps;
  config_section?: string;
  docs_url?: string;
  configured?: boolean;
  live?: boolean;
  fields: ChannelField[];
}

// DirArrows renders ↓ (inbound), ↑ (outbound) or ↕ (both) for one modality.
function DirArrows({ inbound, outbound }: { inbound?: boolean; outbound?: boolean }) {
  if (inbound && outbound) return <ArrowLeftRight className="size-2.5" />;
  if (outbound) return <ArrowUp className="size-2.5" />;
  return <ArrowDown className="size-2.5" />;
}

// MediaChips shows which non-text modalities a channel carries (image / voice)
// and in which direction. Text is implicit on every channel, so it's not shown.
function MediaChips({ media }: { media?: MediaCaps }) {
  if (!media) return null;
  const img = media.image_in || media.image_out;
  const voice = media.voice_in || media.voice_out;
  if (!img && !voice) return null;
  return (
    <>
      {img && (
        <span
          className="inline-flex items-center gap-0.5 rounded bg-accent/10 px-1.5 py-0.5 text-accent"
          title={`Image ${media.image_in ? "receive" : ""}${media.image_in && media.image_out ? " + " : ""}${media.image_out ? "send" : ""}`}
        >
          <ImageIcon className="size-2.5" />
          <DirArrows inbound={media.image_in} outbound={media.image_out} />
        </span>
      )}
      {voice && (
        <span
          className="inline-flex items-center gap-0.5 rounded bg-accent2/10 px-1.5 py-0.5 text-accent2"
          title={`Voice ${media.voice_in ? "receive" : ""}${media.voice_in && media.voice_out ? " + " : ""}${media.voice_out ? "send" : ""}`}
        >
          <Mic className="size-2.5" />
          <DirArrows inbound={media.voice_in} outbound={media.voice_out} />
        </span>
      )}
    </>
  );
}

// Channels — a wizard-style hub to connect communication channels (Telegram,
// WhatsApp, Slack, …). Each channel is configured separately from its own card;
// values persist to the Config Center (secrets to the vault) and apply on the
// next daemon restart. New channels appear here automatically once registered.
export function Channels() {
  const [rows, setRows] = useState<ChannelRow[] | null>(null);
  const [err, setErr] = useState("");
  const [openKind, setOpenKind] = useState<string>("");
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [testTo, setTestTo] = useState("");
  const [testing, setTesting] = useState(false);
  const [gwStatus, setGwStatus] = useState<{ ok: boolean; connected?: boolean; status?: string; error?: string } | null>(null);
  const [gwChecking, setGwChecking] = useState(false);
  const [gwQR, setGwQR] = useState<string>("");
  const [gwQRErr, setGwQRErr] = useState<string>("");
  const [gwQRLoading, setGwQRLoading] = useState(false);
  const ui = useUI();

  function gwArgs() {
    return {
      url: draft["AGEZT_WHATSAPPGW_URL"] || "",
      backend: draft["AGEZT_WHATSAPPGW_BACKEND"] || "",
      session: draft["AGEZT_WHATSAPPGW_SESSION"] || "",
      key: draft["AGEZT_WHATSAPPGW_KEY"] || "",
    };
  }

  // showQR fetches the gateway's login QR (proxied + SSRF-guarded by the daemon)
  // and renders it inline, so you scan to log in without opening the gateway UI.
  async function showQR() {
    setGwQRLoading(true);
    setGwQR("");
    setGwQRErr("");
    try {
      const r = await postJSON<{ ok: boolean; qr?: string; error?: string }>("/api/whatsappgw/qr", gwArgs());
      if (r.ok && r.qr) setGwQR(r.qr);
      else setGwQRErr(r.error || "no QR available");
    } catch (e) {
      setGwQRErr((e as Error).message);
    } finally {
      setGwQRLoading(false);
    }
  }

  // checkGateway probes a WhatsApp gateway (WAHA/Evolution) for whether its
  // session is logged in, using the current form values. The key (if typed this
  // session) rides in the body; otherwise the probe runs keyless.
  async function checkGateway() {
    setGwChecking(true);
    setGwStatus(null);
    try {
      const r = await postJSON<{ ok: boolean; connected?: boolean; status?: string; error?: string }>("/api/whatsappgw/status", gwArgs());
      setGwStatus(r);
    } catch (e) {
      setGwStatus({ ok: false, error: (e as Error).message });
    } finally {
      setGwChecking(false);
    }
  }

  async function sendTest(r: ChannelRow) {
    setTesting(true);
    try {
      await postJSON("/api/send", {
        channel: r.kind,
        to: testTo.trim(),
        text: "✅ AGEZT test message",
      });
      ui.toast(`Test sent via ${r.display}`, "success");
    } catch (e) {
      ui.toast(`${r.display}: ${(e as Error).message}`, "error");
    } finally {
      setTesting(false);
    }
  }

  async function load() {
    try {
      const res = await getJSON<{ channels: ChannelRow[] }>("/api/channels");
      setRows(res.channels || []);
      setErr("");
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  useEffect(() => {
    load();
  }, []);

  const liveCount = useMemo(() => (rows || []).filter((r) => r.live).length, [rows]);
  const configuredCount = useMemo(() => (rows || []).filter((r) => r.configured).length, [rows]);

  function openConfig(r: ChannelRow) {
    if (openKind === r.kind) {
      setOpenKind("");
      return;
    }
    // Seed the draft from current non-secret values; secrets start blank.
    const seed: Record<string, string> = {};
    for (const f of r.fields) if (!f.secret && f.value) seed[f.env] = f.value;
    setDraft(seed);
    setTestTo("");
    setGwStatus(null);
    setGwQR("");
    setGwQRErr("");
    setOpenKind(r.kind);
  }

  async function save(r: ChannelRow) {
    setSaving(true);
    try {
      let wrote = 0;
      for (const f of r.fields) {
        if (f.env_pinned) continue; // set in the environment — read-only here
        const v = draft[f.env];
        if (v === undefined) continue; // untouched
        if (f.secret && v.trim() === "") continue; // don't clobber a stored secret with blank
        await postJSON("/api/config/set", { name: f.env, value: v });
        wrote++;
      }
      ui.toast(
        wrote > 0 ? `Saved ${r.display} — restart the daemon to apply` : "No changes to save",
        wrote > 0 ? "success" : "info",
      );
      setOpenKind("");
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="flex min-h-0 flex-col gap-3">
      <PageHeader
        icon={Radio}
        title="Channels"
        description={
          rows
            ? `${rows.length} channels · ${liveCount} live · ${configuredCount} configured`
            : "Connect Telegram, WhatsApp, Slack, and more"
        }
        actions={
          <Button variant="ghost" size="sm" onClick={load} disabled={rows === null}>
            <RefreshCw className={cn("size-3.5", rows === null && "animate-spin")} /> Refresh
          </Button>
        }
      />

      {err ? (
        <div className="text-xs text-bad">{err}</div>
      ) : rows === null ? (
        <SkeletonList count={6} lines={2} />
      ) : (
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
          {rows.map((r) => (
            <Card key={r.kind} glass className="p-3">
              <div className="flex items-start gap-2">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="font-medium text-foreground">{r.display}</span>
                    {r.live ? (
                      <Badge variant="good">
                        <span className="size-1.5 animate-pulse rounded-full bg-good" /> live
                      </Badge>
                    ) : r.configured ? (
                      <Badge variant="warn">configured · restart to start</Badge>
                    ) : (
                      <Badge variant="warn">needs setup</Badge>
                    )}
                  </div>
                  {r.description && <p className="mt-0.5 line-clamp-2 text-[11px] text-muted">{r.description}</p>}
                  <div className="mt-1 flex flex-wrap items-center gap-2 text-[10px] text-muted">
                    {r.transport && <span className="rounded bg-panel px-1.5 py-0.5">{r.transport}</span>}
                    <span className="inline-flex items-center gap-1">
                      {r.duplex ? <ArrowLeftRight className="size-3" /> : <ArrowRight className="size-3" />}
                      {r.duplex ? "two-way" : "outbound only"}
                    </span>
                    <MediaChips media={r.media} />
                    {r.docs_url && (
                      <a
                        href={r.docs_url}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex items-center gap-0.5 text-accent hover:text-accent2"
                      >
                        docs <ExternalLink className="size-2.5" />
                      </a>
                    )}
                  </div>
                </div>
                <Button variant={openKind === r.kind ? "default" : "ghost"} size="sm" onClick={() => openConfig(r)}>
                  <Settings2 className="size-3.5" /> {r.configured ? "Edit" : "Connect"}
                </Button>
              </div>

              {openKind === r.kind && (
                <div className="mt-2 space-y-2 border-t border-border/50 pt-2">
                  {r.fields.length === 0 && <p className="text-[11px] text-muted">No configurable fields.</p>}
                  {r.fields.map((f) => (
                    <label key={f.env} className="block">
                      <span className="flex items-center gap-1 text-[11px] text-muted">
                        {f.label}
                        {f.required && <span className="text-bad">*</span>}
                        {f.env_pinned && <span className="text-[10px]">(set in environment)</span>}
                      </span>
                      <Input
                        type={f.secret ? "password" : "text"}
                        disabled={f.env_pinned}
                        defaultValue={f.secret ? "" : f.value || ""}
                        placeholder={f.secret ? (f.set ? "•••••••• (stored)" : "") : ""}
                        onChange={(e) => setDraft((d) => ({ ...d, [f.env]: e.target.value }))}
                        className="mt-0.5 h-8 w-full font-mono text-xs"
                        aria-label={f.label}
                      />
                      {f.help && <span className="mt-0.5 block text-[10px] text-muted">{f.help}</span>}
                    </label>
                  ))}
                  <div className="flex items-center gap-2">
                    <Button variant="default" size="sm" disabled={saving} onClick={() => save(r)}>
                      {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
                      Save
                    </Button>
                    <span className="text-[10px] text-muted">Secrets are stored in the vault. Restart to apply.</span>
                  </div>

                  {r.kind === "whatsappgw" && (
                    <div className="mt-1 flex flex-wrap items-center gap-2 border-t border-border/40 pt-2">
                      <Button variant="ghost" size="sm" disabled={gwChecking || !draft["AGEZT_WHATSAPPGW_URL"]} onClick={checkGateway}>
                        {gwChecking ? <RefreshCw className="size-3.5 animate-spin" /> : <Check className="size-3.5" />}
                        Check connection
                      </Button>
                      {gwStatus && (
                        gwStatus.ok ? (
                          <span className={cn("text-[11px]", gwStatus.connected ? "text-good" : "text-warn")}>
                            {gwStatus.connected ? "✓ logged in & ready" : `not logged in (${gwStatus.status || "scan the QR"})`}
                          </span>
                        ) : (
                          <span className="text-[11px] text-bad">{gwStatus.error || "probe failed"}</span>
                        )
                      )}
                      {draft["AGEZT_WHATSAPPGW_URL"] && !gwStatus?.connected && (
                        <Button variant="ghost" size="sm" disabled={gwQRLoading} onClick={showQR}>
                          {gwQRLoading ? <RefreshCw className="size-3.5 animate-spin" /> : <QrCode className="size-3.5" />}
                          Show QR
                        </Button>
                      )}
                      {draft["AGEZT_WHATSAPPGW_URL"] && !gwStatus?.connected && (
                        <a
                          href={draft["AGEZT_WHATSAPPGW_URL"]}
                          target="_blank"
                          rel="noreferrer noopener"
                          className="inline-flex items-center gap-0.5 text-[11px] text-accent hover:underline"
                        >
                          Open gateway <ExternalLink className="size-3" />
                        </a>
                      )}
                      {(gwQR || gwQRErr) && (
                        <div className="w-full pt-1">
                          {gwQR ? (
                            <div className="flex flex-col items-start gap-1">
                              <img src={gwQR} alt="WhatsApp login QR" className="size-44 rounded-md border border-border bg-white p-1.5" />
                              <span className="text-[10px] text-muted">Scan with WhatsApp → Linked devices. Then re-check the connection.</span>
                            </div>
                          ) : (
                            <span className="text-[11px] text-warn">{gwQRErr}</span>
                          )}
                        </div>
                      )}
                    </div>
                  )}

                  <div className="mt-1 flex flex-wrap items-center gap-1.5 border-t border-border/40 pt-2">
                    <span className="text-[11px] text-muted">Test:</span>
                    {r.duplex && (
                      <Input
                        value={testTo}
                        onChange={(e) => setTestTo(e.target.value)}
                        placeholder="recipient / chat id"
                        aria-label="Test recipient"
                        className="h-7 w-44 text-xs"
                      />
                    )}
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={testing || !r.live}
                      onClick={() => sendTest(r)}
                      title={r.live ? "Send a test message now" : "Configure and restart first — the channel must be live to test"}
                    >
                      {testing ? <RefreshCw className="size-3.5 animate-spin" /> : <SendHorizontal className="size-3.5" />}
                      Send test
                    </Button>
                    {!r.live && <span className="text-[10px] text-muted">(live channels only)</span>}
                  </div>
                </div>
              )}
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
