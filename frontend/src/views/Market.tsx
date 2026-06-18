import { useEffect, useMemo, useState } from "react";
import { Store, RefreshCw, Search, Download, Trash2, ShieldCheck, ShieldAlert, Check, Globe, Plus, RotateCw } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { streamMarket, stepFromFrame, type MarketStep } from "@/lib/market";
import { cn } from "@/lib/utils";
import { PageHeader } from "@/components/ui/page-header";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { useUI } from "@/components/ui/feedback";

// A marketplace catalogue row joined with install state (see kernel/market.Listing).
interface Pack {
  name: string;
  version: string;
  description?: string;
  category?: string;
  tags?: string[];
  signed?: boolean;
  skill_count?: number;
  mcp_count?: number;
  tool_count?: number;
  marketplace?: string;
  builtin?: boolean;
  installed?: boolean;
  update_available?: boolean;
}

// A configured remote marketplace source (see kernel/market.Source).
interface MarketSource {
  name: string;
  url: string;
  pubkey?: string;
}

// Marketplace — browse + install capability packs (skills + MCP servers + CLI
// tools). The catalogue leads; pack internals stay one line (counts) until you
// open a pack, per the humane progressive-disclosure direction.
export function Market() {
  const [packs, setPacks] = useState<Pack[] | null>(null);
  const [err, setErr] = useState("");
  const [query, setQuery] = useState("");
  const [cat, setCat] = useState<string>("all");
  const [busy, setBusy] = useState<string>("");
  const [progress, setProgress] = useState<Record<string, MarketStep[]>>({});
  const [sources, setSources] = useState<MarketSource[]>([]);
  const [showSources, setShowSources] = useState(false);
  const [newURL, setNewURL] = useState("");
  const [newName, setNewName] = useState("");
  const [syncing, setSyncing] = useState(false);
  const ui = useUI();

  async function load() {
    try {
      const res = await getJSON<{ packs: Pack[] }>("/api/market");
      setPacks(res.packs || []);
      setErr("");
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  async function loadSources() {
    try {
      const res = await getJSON<{ sources: MarketSource[] }>("/api/market/sources");
      setSources(res.sources || []);
    } catch {
      /* sources are optional; ignore */
    }
  }
  useEffect(() => {
    load();
    loadSources();
  }, []);

  async function addSource() {
    const url = newURL.trim();
    if (!url) return;
    try {
      await postJSON("/api/market/source/add", { url, name: newName.trim() });
      ui.toast("Source added — syncing…", "info");
      setNewURL("");
      setNewName("");
      await loadSources();
      await sync();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  async function removeSource(name: string) {
    const ok = await ui.confirm({
      title: `Remove source ${name}?`,
      message: "Its cached catalogue is dropped. Already-installed packs stay installed.",
      confirmLabel: "Remove",
      danger: true,
    });
    if (!ok) return;
    try {
      await postJSON("/api/market/source/remove", { name });
      ui.toast(`Removed ${name}`, "success");
      await loadSources();
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    }
  }

  async function sync(name?: string) {
    setSyncing(true);
    try {
      const res = await postJSON<{ packs?: number; partial_error?: string }>("/api/market/sync", { name: name || "" });
      ui.toast(`Synced ${res.packs ?? 0} pack(s)`, "success");
      if (res.partial_error) ui.toast(`Some sources failed: ${res.partial_error}`, "error");
      await load();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setSyncing(false);
    }
  }

  const categories = useMemo(() => {
    const s = new Set<string>();
    for (const p of packs || []) if (p.category) s.add(p.category);
    return ["all", ...Array.from(s).sort()];
  }, [packs]);

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (packs || []).filter((p) => {
      if (cat !== "all" && p.category !== cat) return false;
      if (!q) return true;
      const hay = `${p.name} ${p.description ?? ""} ${(p.tags || []).join(" ")}`.toLowerCase();
      return hay.includes(q);
    });
  }, [packs, query, cat]);

  async function install(p: Pack) {
    setBusy(p.name);
    setProgress((m) => ({ ...m, [p.name]: [] }));
    let result: Record<string, unknown> = {};
    let streamErr = "";
    try {
      await streamMarket("/api/market/install", { name: p.name, marketplace: p.marketplace || "" }, (f) => {
        const step = stepFromFrame(f);
        if (step) setProgress((m) => ({ ...m, [p.name]: [...(m[p.name] || []), step] }));
        else if (f.kind === "done") result = f.result || {};
        else if (f.kind === "error") streamErr = f.error || "install failed";
      });
    } catch (e) {
      streamErr = (e as Error).message;
    }
    if (streamErr) {
      ui.toast(streamErr, "error");
    } else {
      let msg = `Installed ${p.name}`;
      if (result.unsigned) msg += " (unsigned)";
      ui.toast(msg, "success");
      const reqs = (result.tool_reqs as string[]) || [];
      if (reqs.length > 0) ui.toast(`${p.name} needs CLI tools: ${reqs.join(", ")} — install them in Toolbox`, "info");
      await load();
    }
    setBusy("");
    setProgress((m) => {
      const { [p.name]: _drop, ...rest } = m;
      return rest;
    });
  }

  async function uninstall(p: Pack) {
    const ok = await ui.confirm({
      title: `Uninstall ${p.name}?`,
      message: "Its skills are quarantined and its MCP servers removed. Shared resources are left alone.",
      confirmLabel: "Uninstall",
      danger: true,
    });
    if (!ok) return;
    setBusy(p.name);
    setProgress((m) => ({ ...m, [p.name]: [] }));
    let streamErr = "";
    try {
      await streamMarket("/api/market/uninstall", { name: p.name }, (f) => {
        const step = stepFromFrame(f);
        if (step) setProgress((m) => ({ ...m, [p.name]: [...(m[p.name] || []), step] }));
        else if (f.kind === "error") streamErr = f.error || "uninstall failed";
      });
    } catch (e) {
      streamErr = (e as Error).message;
    }
    if (streamErr) ui.toast(streamErr, "error");
    else {
      ui.toast(`Uninstalled ${p.name}`, "success");
      await load();
    }
    setBusy("");
    setProgress((m) => {
      const { [p.name]: _drop, ...rest } = m;
      return rest;
    });
  }

  const installedCount = (packs || []).filter((p) => p.installed).length;

  return (
    <div className="flex min-h-0 flex-col gap-3">
      <PageHeader
        icon={Store}
        title="Marketplace"
        description={packs ? `${packs.length} packs · ${installedCount} installed` : "Install capability packs — skills, MCP servers, and tools"}
        actions={
          <>
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search packs…"
                className="h-8 w-44 pl-7 sm:w-56"
                aria-label="Search packs"
              />
            </div>
            <Button variant="ghost" size="sm" onClick={() => setShowSources((v) => !v)}>
              <Globe className="size-3.5" /> Sources{sources.length > 0 ? ` (${sources.length})` : ""}
            </Button>
            <Button variant="ghost" size="sm" onClick={load} disabled={packs === null}>
              <RefreshCw className={cn("size-3.5", packs === null && "animate-spin")} /> Refresh
            </Button>
          </>
        }
      />

      {showSources && (
        <Card glass className="space-y-2 p-3">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Remote marketplaces</span>
            <Button variant="ghost" size="sm" disabled={syncing || sources.length === 0} onClick={() => sync()}>
              <RotateCw className={cn("size-3.5", syncing && "animate-spin")} /> Sync all
            </Button>
          </div>
          {sources.length === 0 ? (
            <p className="text-[11px] text-muted">
              No remote sources yet. Add a marketplace.json URL below — its packs appear in the gallery after a sync. The
              built-in Official catalogue is always available offline.
            </p>
          ) : (
            <ul className="space-y-1">
              {sources.map((s) => (
                <li key={s.name} className="flex items-center gap-2 rounded-md border border-border/60 px-2 py-1 text-xs">
                  <span className="font-medium">{s.name}</span>
                  <span className="min-w-0 flex-1 truncate font-mono text-[10px] text-muted">{s.url}</span>
                  {s.pubkey && <ShieldCheck className="size-3 text-good" aria-label="signer key pinned" />}
                  <Button variant="ghost" size="sm" disabled={syncing} onClick={() => sync(s.name)} title="Sync">
                    <RotateCw className="size-3" />
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => removeSource(s.name)} title="Remove">
                    <Trash2 className="size-3 text-bad" />
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <div className="flex flex-wrap items-center gap-1.5">
            <Input
              value={newURL}
              onChange={(e) => setNewURL(e.target.value)}
              placeholder="https://…/marketplace.json"
              className="h-8 min-w-0 flex-1"
              aria-label="Marketplace URL"
            />
            <Input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder="name (optional)"
              className="h-8 w-32"
              aria-label="Source name"
            />
            <Button variant="default" size="sm" disabled={!newURL.trim()} onClick={addSource}>
              <Plus className="size-3.5" /> Add
            </Button>
          </div>
        </Card>
      )}

      {categories.length > 2 && (
        <div className="flex flex-wrap gap-1">
          {categories.map((c) => (
            <button
              key={c}
              onClick={() => setCat(c)}
              className={cn(
                "rounded-full border px-2.5 py-0.5 text-[11px] capitalize transition-colors",
                cat === c ? "border-accent bg-accent/15 text-accent" : "border-border text-muted hover:text-foreground",
              )}
            >
              {c}
            </button>
          ))}
        </div>
      )}

      {err ? (
        <div className="text-xs text-bad">{err}</div>
      ) : packs === null ? (
        <SkeletonList count={6} lines={2} />
      ) : shown.length === 0 ? (
        <EmptyState icon={Store} title="No packs match" hint="Try a different search or category." />
      ) : (
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
          {shown.map((p) => (
            <Card key={p.name} glass className="p-3">
              <div className="flex items-start gap-2">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="font-medium text-foreground">{p.name}</span>
                    <span className="font-mono text-[10px] text-muted">{p.version}</span>
                    {p.signed ? (
                      <ShieldCheck className="size-3 text-good" aria-label="signed" />
                    ) : (
                      <ShieldAlert className="size-3 text-muted" aria-label="unsigned" />
                    )}
                    {p.installed && (
                      <Badge variant="good">
                        <Check className="size-2.5" /> installed
                      </Badge>
                    )}
                    {p.update_available && <Badge variant="warn">update</Badge>}
                  </div>
                  {p.description && <p className="mt-0.5 line-clamp-2 text-[11px] text-muted">{p.description}</p>}
                  <div className="mt-1 flex flex-wrap items-center gap-2 text-[10px] text-muted">
                    {p.category && <span className="rounded bg-panel px-1.5 py-0.5 capitalize">{p.category}</span>}
                    <span>
                      {p.skill_count ?? 0} skill{(p.skill_count ?? 0) === 1 ? "" : "s"} · {p.mcp_count ?? 0} MCP ·{" "}
                      {p.tool_count ?? 0} tool{(p.tool_count ?? 0) === 1 ? "" : "s"}
                    </span>
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  {p.installed ? (
                    <>
                      {p.update_available && (
                        <Button variant="default" size="sm" disabled={busy === p.name} onClick={() => install(p)} title="Update to the catalogued version">
                          {busy === p.name ? <RefreshCw className="size-3.5 animate-spin" /> : <RotateCw className="size-3.5" />}
                          Update
                        </Button>
                      )}
                      <Button variant="ghost" size="sm" disabled={busy === p.name} onClick={() => uninstall(p)} title="Uninstall">
                        <Trash2 className="size-3.5 text-bad" />
                      </Button>
                    </>
                  ) : (
                    <Button variant="default" size="sm" disabled={busy === p.name} onClick={() => install(p)}>
                      {busy === p.name ? <RefreshCw className="size-3.5 animate-spin" /> : <Download className="size-3.5" />}
                      Install
                    </Button>
                  )}
                </div>
              </div>
              {progress[p.name] && progress[p.name].length > 0 && (
                <ul className="mt-2 space-y-0.5 border-t border-border/50 pt-1.5 text-[10px]">
                  {progress[p.name].map((s, i) => (
                    <li key={i} className="flex items-center gap-1.5 font-mono">
                      {s.ok ? <Check className="size-2.5 text-good" /> : <ShieldAlert className="size-2.5 text-bad" />}
                      <span className="text-muted">{s.stage}</span>
                      {s.name && <span className="text-foreground">{s.name}</span>}
                      {s.detail && <span className="truncate text-muted">— {s.detail}</span>}
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
