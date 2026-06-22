import { useEffect, useMemo, useState } from "react";
import {
  Radio, RefreshCw, Check, ExternalLink, Save, SendHorizontal, QrCode,
  Image as ImageIcon, Mic, ArrowDown, ArrowUp, ArrowLeftRight, ArrowRight,
  Plus, Pencil, Trash2, X, ListChecks, KeyRound,
} from "lucide-react";
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

interface MediaCaps {
  image_in?: boolean;
  voice_in?: boolean;
  image_out?: boolean;
  voice_out?: boolean;
}

// One configured account-instance of a channel (default = empty label).
interface ChannelAccount {
  label: string;
  configured?: boolean;
  live?: boolean;
  fields: ChannelField[];
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
  connect_method?: string;
  setup_steps?: string[];
  configured?: boolean;
  live?: boolean;
  fields: ChannelField[];
  accounts?: ChannelAccount[];
}

// instanceKey addresses a channel account: bare kind for the default, "kind#label".
function instanceKey(kind: string, label: string) {
  return label ? `${kind}#${label}` : kind;
}

function DirArrows({ inbound, outbound }: { inbound?: boolean; outbound?: boolean }) {
  if (inbound && outbound) return <ArrowLeftRight className="size-2.5" />;
  if (outbound) return <ArrowUp className="size-2.5" />;
  return <ArrowDown className="size-2.5" />;
}

// MediaChips shows which non-text modalities a channel carries and in which direction.
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

function StatusBadge({ live, configured }: { live?: boolean; configured?: boolean }) {
  if (live)
    return (
      <Badge variant="good">
        <span className="size-1.5 animate-pulse rounded-full bg-good" /> live
      </Badge>
    );
  if (configured) return <Badge variant="warn">configured · restart</Badge>;
  return <Badge variant="warn">needs setup</Badge>;
}

// accountsOf returns a row's accounts, falling back to a synthetic default
// account built from the top-level fields (back-compat with an older daemon).
function accountsOf(r: ChannelRow): ChannelAccount[] {
  if (r.accounts && r.accounts.length) return r.accounts;
  return [{ label: "", configured: r.configured, live: r.live, fields: r.fields }];
}

// ConnectForm — the guided "what you'll need" + field entry for one account.
// New accounts take an optional label; editing keeps the account's label.
function ConnectForm({
  row,
  account,
  isNew,
  onClose,
  onSaved,
}: {
  row: ChannelRow;
  account: ChannelAccount;
  isNew: boolean;
  onClose: () => void;
  onSaved: () => void;
}) {
  const ui = useUI();
  const [label, setLabel] = useState(account.label);
  const [draft, setDraft] = useState<Record<string, string>>(() => {
    const seed: Record<string, string> = {};
    for (const f of account.fields) if (!f.secret && f.value) seed[f.env] = f.value;
    return seed;
  });
  const [saving, setSaving] = useState(false);
  const [showSteps, setShowSteps] = useState(isNew && !!row.setup_steps?.length);
  // whatsappgw QR / connection probe (connect_method "qr") — uses the live draft.
  const isQR = row.connect_method === "qr" || row.kind === "whatsappgw";
  const [gw, setGw] = useState<{ checking?: boolean; connected?: boolean; status?: string; error?: string }>({});
  const [qr, setQR] = useState<{ loading?: boolean; img?: string; error?: string }>({});
  // OAuth connect (connect_method "oauth") — "Connect with X" instead of pasting
  // a token. Client id/secret are entered here and used only for the exchange;
  // the resulting token is stored in the account's vault slot by the daemon.
  const isOAuth = row.connect_method === "oauth";
  const oauthInstanceField = row.kind === "mastodon" ? "AGEZT_MASTODON_SERVER" : "";
  const [oauth, setOAuth] = useState<{ clientId: string; clientSecret: string; instance: string; busy?: boolean; status?: string; error?: string }>(
    () => ({ clientId: "", clientSecret: "", instance: account.fields.find((f) => f.env === oauthInstanceField)?.value || "" }),
  );
  const redirectURI = typeof window !== "undefined" ? window.location.origin + "/oauth/callback" : "/oauth/callback";

  async function startOAuth() {
    const clientId = oauth.clientId.trim();
    const clientSecret = oauth.clientSecret.trim();
    if (!clientId || !clientSecret) {
      ui.toast("Enter the OAuth client id and client secret first", "error");
      return;
    }
    if (oauthInstanceField && !oauth.instance.trim()) {
      ui.toast("Enter your instance URL first", "error");
      return;
    }
    const lbl = label.trim();
    if (lbl && !/^[a-z0-9][a-z0-9_-]{0,31}$/.test(lbl)) {
      ui.toast("Account name must be a slug: lowercase letters/digits/-/_ (max 32)", "error");
      return;
    }
    setOAuth((o) => ({ ...o, busy: true, status: "Opening provider…", error: undefined }));
    try {
      const r = await postJSON<{ authorize_url?: string; state?: string; error?: string }>("/api/channel/oauth/start", {
        kind: row.kind, label: lbl, client_id: clientId, client_secret: clientSecret,
        redirect_uri: redirectURI, instance_url: oauth.instance.trim(),
      });
      if (!r.authorize_url || !r.state) throw new Error(r.error || "could not start OAuth");
      window.open(r.authorize_url, "_blank", "noopener,noreferrer");
      setOAuth((o) => ({ ...o, status: "Waiting for you to authorize in the new tab…" }));
      // Poll the flow status until it resolves (or ~2.5 min timeout).
      for (let i = 0; i < 75; i++) {
        await new Promise((res) => setTimeout(res, 2000));
        const st = await postJSON<{ status?: string; error?: string }>("/api/channel/oauth/status", { state: r.state });
        if (st.status === "done") {
          ui.toast(`Connected ${row.display}${lbl ? ` · ${lbl}` : ""} — restart to apply`, "success");
          setOAuth((o) => ({ ...o, busy: false, status: "Connected ✓" }));
          onSaved();
          onClose();
          return;
        }
        if (st.status === "error") throw new Error(st.error || "authorization failed");
      }
      throw new Error("timed out waiting for authorization");
    } catch (e) {
      setOAuth((o) => ({ ...o, busy: false, status: undefined, error: (e as Error).message }));
    }
  }
  function gwArgs() {
    return {
      url: draft["AGEZT_WHATSAPPGW_URL"] || "",
      backend: draft["AGEZT_WHATSAPPGW_BACKEND"] || "",
      session: draft["AGEZT_WHATSAPPGW_SESSION"] || "",
      key: draft["AGEZT_WHATSAPPGW_KEY"] || "",
    };
  }
  async function checkGateway() {
    setGw({ checking: true });
    try {
      const r = await postJSON<{ ok: boolean; connected?: boolean; status?: string; error?: string }>("/api/whatsappgw/status", gwArgs());
      setGw({ connected: r.connected, status: r.status, error: r.ok ? undefined : r.error || "probe failed" });
    } catch (e) {
      setGw({ error: (e as Error).message });
    }
  }
  async function showQR() {
    setQR({ loading: true });
    try {
      const r = await postJSON<{ ok: boolean; qr?: string; error?: string }>("/api/whatsappgw/qr", gwArgs());
      setQR(r.ok && r.qr ? { img: r.qr } : { error: r.error || "no QR available" });
    } catch (e) {
      setQR({ error: (e as Error).message });
    }
  }

  async function save() {
    const lbl = label.trim();
    if (lbl && !/^[a-z0-9][a-z0-9_-]{0,31}$/.test(lbl)) {
      ui.toast("Account name must be a slug: lowercase letters/digits/-/_ (max 32)", "error");
      return;
    }
    setSaving(true);
    try {
      let wrote = 0;
      for (const f of account.fields) {
        if (f.env_pinned) continue;
        const v = draft[f.env];
        if (v === undefined) continue; // untouched
        if (f.secret && v.trim() === "") continue; // don't clobber a stored secret
        await postJSON("/api/channel/account/set", { kind: row.kind, label: lbl, name: f.env, value: v });
        wrote++;
      }
      ui.toast(
        wrote > 0 ? `Saved ${row.display}${lbl ? ` · ${lbl}` : ""} — restart to apply` : "No changes to save",
        wrote > 0 ? "success" : "info",
      );
      onSaved();
      onClose();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-2 rounded-md border border-border/60 bg-panel/40 p-2">
      <div className="flex items-center gap-2">
        <span className="text-[11px] font-medium text-foreground">
          {isNew ? "Add account" : `Edit ${account.label || "default"}`}
        </span>
        <button className="ml-auto text-muted hover:text-foreground" onClick={onClose} aria-label="Close">
          <X className="size-3.5" />
        </button>
      </div>

      {/* Step 1 — what you'll need */}
      {!!row.setup_steps?.length && (
        <div className="rounded border border-border/50 bg-background/40">
          <button
            className="flex w-full items-center gap-1.5 px-2 py-1 text-[11px] text-muted hover:text-foreground"
            onClick={() => setShowSteps((v) => !v)}
          >
            <ListChecks className="size-3" /> What you'll need
            {row.docs_url && (
              <a
                href={row.docs_url}
                target="_blank"
                rel="noreferrer"
                className="ml-auto inline-flex items-center gap-0.5 text-accent hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                docs <ExternalLink className="size-2.5" />
              </a>
            )}
          </button>
          {showSteps && (
            <ol className="list-decimal space-y-0.5 px-6 pb-2 text-[11px] text-muted">
              {row.setup_steps.map((s, i) => (
                <li key={i}>{s}</li>
              ))}
            </ol>
          )}
        </div>
      )}

      {/* Account label — only when adding (lets you run several of this channel). */}
      {isNew && (
        <label className="block">
          <span className="text-[11px] text-muted">Account name (optional)</span>
          <Input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="e.g. work, personal — blank = default account"
            aria-label="Account name"
            className="mt-0.5 h-8 w-full text-xs"
          />
        </label>
      )}

      {/* OAuth connect — "Connect with X". Manual token entry stays below. */}
      {isOAuth && (
        <div className="space-y-1.5 rounded border border-accent/40 bg-accent/5 p-2">
          <div className="flex items-center gap-1.5 text-[11px] font-medium text-foreground">
            <KeyRound className="size-3" /> Connect with {row.display}
          </div>
          {oauthInstanceField && (
            <Input
              value={oauth.instance}
              onChange={(e) => setOAuth((o) => ({ ...o, instance: e.target.value }))}
              placeholder="Instance URL — e.g. https://mastodon.social"
              aria-label="Instance URL"
              className="h-8 w-full font-mono text-xs"
            />
          )}
          <Input
            value={oauth.clientId}
            onChange={(e) => setOAuth((o) => ({ ...o, clientId: e.target.value }))}
            placeholder="OAuth client id"
            aria-label="OAuth client id"
            className="h-8 w-full font-mono text-xs"
          />
          <Input
            type="password"
            value={oauth.clientSecret}
            onChange={(e) => setOAuth((o) => ({ ...o, clientSecret: e.target.value }))}
            placeholder="OAuth client secret"
            aria-label="OAuth client secret"
            className="h-8 w-full font-mono text-xs"
          />
          <p className="text-xs text-muted">
            Redirect URL (add this to your OAuth app): <code className="text-foreground">{redirectURI}</code>
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="default" size="sm" disabled={oauth.busy} onClick={startOAuth}>
              {oauth.busy ? <RefreshCw className="size-3.5 animate-spin" /> : <KeyRound className="size-3.5" />}
              Connect with {row.display}
            </Button>
            {oauth.status && <span className="text-[11px] text-muted">{oauth.status}</span>}
            {oauth.error && <span className="text-[11px] text-bad">{oauth.error}</span>}
          </div>
          <p className="text-xs text-muted">…or paste a token manually below.</p>
        </div>
      )}

      {/* Step 2 — fields */}
      {account.fields.length === 0 && <p className="text-[11px] text-muted">No configurable fields.</p>}
      {account.fields.map((f) => (
        <label key={f.env} className="block">
          <span className="flex items-center gap-1 text-[11px] text-muted">
            {f.label}
            {f.required && <span className="text-bad">*</span>}
            {f.env_pinned && <span className="text-xs">(set in environment)</span>}
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
          {f.help && <span className="mt-0.5 block text-xs text-muted">{f.help}</span>}
        </label>
      ))}

      {/* QR / connection probe for gateway channels (e.g. WhatsApp). */}
      {isQR && (
        <div className="flex flex-wrap items-center gap-2 border-t border-border/40 pt-2">
          <Button variant="ghost" size="sm" disabled={gw.checking || !draft["AGEZT_WHATSAPPGW_URL"]} onClick={checkGateway}>
            {gw.checking ? <RefreshCw className="size-3.5 animate-spin" /> : <Check className="size-3.5" />}
            Check connection
          </Button>
          {(gw.connected !== undefined || gw.status || gw.error) &&
            (gw.error ? (
              <span className="text-[11px] text-bad">{gw.error}</span>
            ) : (
              <span className={cn("text-[11px]", gw.connected ? "text-good" : "text-warn")}>
                {gw.connected ? "✓ logged in & ready" : `not logged in (${gw.status || "scan the QR"})`}
              </span>
            ))}
          {draft["AGEZT_WHATSAPPGW_URL"] && !gw.connected && (
            <Button variant="ghost" size="sm" disabled={qr.loading} onClick={showQR}>
              {qr.loading ? <RefreshCw className="size-3.5 animate-spin" /> : <QrCode className="size-3.5" />}
              Show QR
            </Button>
          )}
          {(qr.img || qr.error) && (
            <div className="w-full pt-1">
              {qr.img ? (
                <div className="flex flex-col items-start gap-1">
                  <img src={qr.img} alt="login QR" className="size-44 rounded-md border border-border bg-white p-1.5" />
                  <span className="text-xs text-muted">Scan with WhatsApp → Linked devices, then re-check.</span>
                </div>
              ) : (
                <span className="text-[11px] text-warn">{qr.error}</span>
              )}
            </div>
          )}
        </div>
      )}

      <div className="flex items-center gap-2">
        <Button variant="default" size="sm" disabled={saving} onClick={save}>
          {saving ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
          Save
        </Button>
        <span className="text-xs text-muted">Secrets are stored in the vault. Restart to apply.</span>
      </div>
    </div>
  );
}

// AccountManager — lists a channel's accounts (all run simultaneously, no single
// "active") with add / edit / remove / test, and the guided connect form.
function AccountManager({ row, onChanged }: { row: ChannelRow; onChanged: () => void }) {
  const ui = useUI();
  const accounts = accountsOf(row);
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<string | null>(null); // account label being edited
  const [testTo, setTestTo] = useState("");
  const [testingKey, setTestingKey] = useState<string>("");

  async function removeAccount(label: string) {
    const ok = await ui.confirm({
      title: `Remove ${row.display}${label ? ` · ${label}` : ""}?`,
      message: "This deletes the account's stored credentials. Restart to apply.",
      confirmLabel: "Remove",
      danger: true,
    });
    if (!ok) return;
    try {
      await postJSON("/api/channel/account/remove", { kind: row.kind, label });
      ui.toast("Account removed — restart to apply", "success");
      onChanged();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  async function testAccount(label: string, live?: boolean) {
    if (!live) {
      ui.toast("Configure and restart first — the account must be live to test", "info");
      return;
    }
    const key = instanceKey(row.kind, label);
    setTestingKey(key);
    try {
      await postJSON("/api/send", { channel: key, to: testTo.trim(), text: "✅ AGEZT test message" });
      ui.toast(`Test sent via ${row.display}${label ? ` · ${label}` : ""}`, "success");
    } catch (e) {
      ui.toast(`${row.display}: ${(e as Error).message}`, "error");
    } finally {
      setTestingKey("");
    }
  }

  return (
    <div className="mt-2 space-y-2 border-t border-border/50 pt-2">
      {accounts.map((a) => {
        const key = instanceKey(row.kind, a.label);
        const editingThis = editing === a.label;
        return (
          <div key={key} className="rounded-md border border-border/40 bg-background/30 p-2">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono text-xs text-foreground">{a.label || "default"}</span>
              <StatusBadge live={a.live} configured={a.configured} />
              <div className="ml-auto flex items-center gap-1">
                <Button variant="ghost" size="sm" onClick={() => setEditing(editingThis ? null : a.label)}>
                  <Pencil className="size-3.5" /> Edit
                </Button>
                {a.label !== "" && (
                  <Button variant="ghost" size="sm" onClick={() => removeAccount(a.label)} title="Remove account">
                    <Trash2 className="size-3.5 text-bad" />
                  </Button>
                )}
              </div>
            </div>

            {/* Per-account test row. */}
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
              <span className="text-xs text-muted">Test:</span>
              {row.duplex && (
                <Input
                  value={testTo}
                  onChange={(e) => setTestTo(e.target.value)}
                  placeholder="recipient / chat id"
                  aria-label="Test recipient"
                  className="h-7 w-40 text-xs"
                />
              )}
              <Button
                variant="ghost"
                size="sm"
                disabled={!!testingKey || !a.live}
                onClick={() => testAccount(a.label, a.live)}
                title={a.live ? "Send a test message now" : "Live accounts only — configure + restart first"}
              >
                {testingKey === key ? <RefreshCw className="size-3.5 animate-spin" /> : <SendHorizontal className="size-3.5" />}
                Send test
              </Button>
            </div>

            {editingThis && (
              <div className="mt-2">
                <ConnectForm
                  row={row}
                  account={a}
                  isNew={false}
                  onClose={() => setEditing(null)}
                  onSaved={onChanged}
                />
              </div>
            )}
          </div>
        );
      })}

      {adding ? (
        <ConnectForm
          row={row}
          account={{ label: "", fields: row.fields.map((f) => ({ ...f, value: "", set: false })) }}
          isNew
          onClose={() => setAdding(false)}
          onSaved={onChanged}
        />
      ) : (
        <Button variant="ghost" size="sm" onClick={() => setAdding(true)}>
          <Plus className="size-3.5" /> Add account
        </Button>
      )}
    </div>
  );
}

// Channels — a hub to connect communication channels. Each channel can hold
// SEVERAL accounts (e.g. multiple email mailboxes or Telegram bots), all running
// at once; values persist to the Config Center (secrets to the vault) and apply
// on the next restart.
export function Channels() {
  const [rows, setRows] = useState<ChannelRow[] | null>(null);
  const [err, setErr] = useState("");
  const [openKind, setOpenKind] = useState<string>("");

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
          {rows.map((r) => {
            const accts = accountsOf(r);
            const liveAccts = accts.filter((a) => a.live).length;
            const open = openKind === r.kind;
            return (
              <Card key={r.kind} glass className="p-3">
                <div className="flex items-start gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="font-medium text-foreground">{r.display}</span>
                      <StatusBadge live={r.live} configured={r.configured} />
                      {accts.length > 1 && (
                        <span className="text-xs text-muted">
                          {accts.length} accounts{liveAccts ? ` · ${liveAccts} live` : ""}
                        </span>
                      )}
                    </div>
                    {r.description && <p className="mt-0.5 line-clamp-2 text-[11px] text-muted">{r.description}</p>}
                    <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted">
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
                  <Button variant={open ? "default" : "ghost"} size="sm" onClick={() => setOpenKind(open ? "" : r.kind)}>
                    {open ? <X className="size-3.5" /> : <QrCode className="size-3.5" />}
                    {r.configured ? "Manage" : "Connect"}
                  </Button>
                </div>

                {open && <AccountManager row={r} onChanged={load} />}
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
