import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  Store,
  RefreshCw,
  Search,
  Download,
  Trash2,
  ShieldCheck,
  ShieldAlert,
  ShieldQuestion,
  Check,
  Globe,
  Plus,
  RotateCw,
  ChevronDown,
  X,
  Sparkles,
  TrendingUp,
  ArrowDownUp,
  Package,
  Server,
  Terminal,
  type LucideIcon,
} from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { streamMarket, stepFromFrame, fetchPackDetails, type MarketStep, type PackDetails, type VetReport } from "@/lib/market";
import { cn } from "@/lib/utils";
import { Page } from "@/components/ui/page";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";
import { useUI } from "@/components/ui/feedback";
import { LoadMoreFooter } from "@/components/ui/load-more-footer";
import { Markdown } from "@/components/Markdown";

// PACK_WINDOW is how many pack cards render at once. /api/market has no
// cursor, so the whole catalogue arrives in one fetch — the window keeps a
// big catalogue from ballooning the DOM; a Load-more footer grows it
// client-side and the window resets on every search/category/filter change.
// Category chips and the header counts stay computed over the FULL list.
const PACK_WINDOW = 60;

// A marketplace catalogue row joined with install state (see kernel/market.Listing).
interface Pack {
  name: string;
  version: string;
  description?: string;
  category?: string;
  tags?: string[];
  signed?: boolean;
  featured?: boolean;
  downloads?: number;
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

type SortKey = "featured" | "popular" | "name";

// compactCount renders 1_200 → "1.2k" for the download signal.
function compactCount(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}k`;
  return `${(n / 1_000_000).toFixed(1)}m`;
}

// TrustChip renders the pack's signing posture — the primary trust signal
// (Ed25519 signature over canonical bytes). Signed = a publisher vouched for
// these exact bytes; unsigned = installable but unattributed.
function TrustChip({ signed }: { signed?: boolean }) {
  return signed ? (
    <span className="inline-flex items-center gap-1 text-good" title="Signed — publisher-attested bytes">
      <ShieldCheck className="size-3" aria-label="signed" />
    </span>
  ) : (
    <span className="inline-flex items-center gap-1 text-muted" title="Unsigned — installable, but no publisher signature">
      <ShieldQuestion className="size-3" aria-label="unsigned" />
    </span>
  );
}

// VET_TONE maps a security-review verdict to a Badge tone + label + icon.
const VET_TONE: Record<string, { variant: "good" | "warn" | "bad"; label: string; icon: LucideIcon }> = {
  clean: { variant: "good", label: "Security: clean", icon: ShieldCheck },
  caution: { variant: "warn", label: "Security: review", icon: ShieldAlert },
  danger: { variant: "bad", label: "Security: risky", icon: ShieldAlert },
};

// VetBadge surfaces the pre-install static review verdict — informational, never
// a blocker (default-allow posture), in the spirit of ClawHub/Hermes install
// scanning after the ClawHavoc supply-chain campaign.
function VetBadge({ vet }: { vet?: VetReport }) {
  if (!vet) return null;
  const tone = VET_TONE[vet.verdict] ?? VET_TONE.caution;
  const Icon = tone.icon;
  const n = vet.findings?.length ?? 0;
  return (
    <Badge variant={tone.variant} title={n > 0 ? `${n} finding(s) — expand for detail` : "No risky patterns found"}>
      <Icon className="size-2.5" /> {tone.label}
      {n > 0 ? ` (${n})` : ""}
    </Badge>
  );
}

function MarketModal({
  title,
  icon: Icon,
  onClose,
  wide,
  children,
}: {
  title: ReactNode;
  icon: LucideIcon;
  onClose: () => void;
  wide?: boolean;
  children: ReactNode;
}) {
  return (
    <div
      className="modal-overlay fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4 backdrop-blur-[2px]"
      onClick={onClose}
    >
      <div
        className={cn("modal-in glass flex max-h-[86vh] w-full flex-col rounded-xl shadow-e3", wide ? "max-w-3xl" : "max-w-2xl")}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        <div className="flex items-center gap-2 border-b border-border px-4 py-3">
          <Icon className="size-4 text-accent" />
          <div className="min-w-0 flex-1 text-sm font-semibold text-foreground">{title}</div>
          <Button size="icon" variant="ghost" onClick={onClose} aria-label="Close">
            <X className="size-4" />
          </Button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-4">{children}</div>
      </div>
    </div>
  );
}

// Marketplace — browse + install capability packs (skills + MCP servers + CLI
// tools). Leads with featured picks and a security posture on every card; pack
// internals stay one line (counts) until you open a pack, per the humane
// progressive-disclosure direction.
export function Market() {
  const [packs, setPacks] = useState<Pack[] | null>(null);
  const [err, setErr] = useState("");
  const [query, setQuery] = useState("");
  const [cat, setCat] = useState<string>("all");
  const [sort, setSort] = useState<SortKey>("featured");
  const [installedOnly, setInstalledOnly] = useState(false);
  const [busy, setBusy] = useState<string>("");
  const [progress, setProgress] = useState<Record<string, MarketStep[]>>({});
  // Lazily-loaded "What's inside" details, keyed by pack name ("loading" while fetching).
  const [details, setDetails] = useState<Record<string, PackDetails | "loading">>({});
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [detailPack, setDetailPack] = useState<Pack | null>(null);
  const [sources, setSources] = useState<MarketSource[]>([]);
  const [showSources, setShowSources] = useState(false);
  const [newURL, setNewURL] = useState("");
  const [newName, setNewName] = useState("");
  const [syncing, setSyncing] = useState(false);
  const [win, setWin] = useState(PACK_WINDOW);
  const ui = useUI();

  // Reset the render window whenever a filter changes so a new query always
  // starts from the top of its result set.
  useEffect(() => {
    setWin(PACK_WINDOW);
  }, [query, cat, installedOnly, sort]);

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

  const filterActive = query.trim() !== "" || cat !== "all" || installedOnly;

  // Featured strip: editor's picks, shown only on the unfiltered gallery so it
  // reads as a curated front page rather than duplicating filtered results.
  const featured = useMemo(() => (packs || []).filter((p) => p.featured).slice(0, 6), [packs]);

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = (packs || []).filter((p) => {
      if (installedOnly && !p.installed) return false;
      if (cat !== "all" && p.category !== cat) return false;
      if (!q) return true;
      const hay = `${p.name} ${p.description ?? ""} ${(p.tags || []).join(" ")}`.toLowerCase();
      return hay.includes(q);
    });
    const cmp: Record<SortKey, (a: Pack, b: Pack) => number> = {
      name: (a, b) => a.name.localeCompare(b.name),
      popular: (a, b) => (b.downloads ?? 0) - (a.downloads ?? 0) || a.name.localeCompare(b.name),
      featured: (a, b) =>
        Number(!!b.featured) - Number(!!a.featured) ||
        (b.downloads ?? 0) - (a.downloads ?? 0) ||
        a.name.localeCompare(b.name),
    };
    return [...list].sort(cmp[sort]);
  }, [packs, query, cat, installedOnly, sort]);

  // toggleDetails expands/collapses a pack's "What's inside" panel, lazily
  // fetching its contents the first time it's opened.
  async function toggleDetails(p: Pack) {
    setExpanded((cur) => {
      const next = new Set(cur);
      next.has(p.name) ? next.delete(p.name) : next.add(p.name);
      return next;
    });
    void ensureDetails(p);
  }

  async function ensureDetails(p: Pack) {
    if (details[p.name] !== undefined) return;
    setDetails((m) => ({ ...m, [p.name]: "loading" }));
    try {
      const d = await fetchPackDetails(p.name, p.marketplace);
      setDetails((m) => ({ ...m, [p.name]: d }));
    } catch {
      setDetails((m) => ({ ...m, [p.name]: { skills: [], mcp_servers: [], tools: [] } }));
    }
  }

  function openDetail(p: Pack) {
    setDetailPack(p);
    void ensureDetails(p);
  }

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
      if (result.vet_verdict === "danger")
        ui.toast(`${p.name} tripped the security review — open its details to see why`, "error");
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
    <Page
      icon={Store}
      title="Marketplace"
      description={packs ? `${packs.length} packs · ${installedCount} installed` : "Install capability packs — skills, MCP servers, and tools"}
      width="wide"
      mode="scroll"
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
          <div className="relative">
            <ArrowDownUp className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
            <select
              value={sort}
              onChange={(e) => setSort(e.target.value as SortKey)}
              aria-label="Sort packs"
              className="h-8 rounded-md border border-border bg-panel pl-7 pr-2 text-xs outline-none transition-[border-color,box-shadow] hover:border-accent/40 focus-glow"
            >
              <option value="featured">Featured</option>
              <option value="popular">Most installed</option>
              <option value="name">Name</option>
            </select>
          </div>
          <Button variant="ghost" size="sm" onClick={() => setShowSources(true)}>
            <Globe className="size-3.5" /> Sources{sources.length > 0 ? ` (${sources.length})` : ""}
          </Button>
          <Button variant="ghost" size="sm" onClick={load} disabled={packs === null}>
            <RefreshCw className={cn("size-3.5", packs === null && "animate-spin")} /> Refresh
          </Button>
        </>
      }
    >
      {showSources && (
        <MarketModal title="Remote marketplaces" icon={Globe} onClose={() => setShowSources(false)}>
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted">{sources.length} configured source{sources.length === 1 ? "" : "s"}</span>
              <Button variant="ghost" size="sm" disabled={syncing || sources.length === 0} onClick={() => sync()}>
                <RotateCw className={cn("size-3.5", syncing && "animate-spin")} /> Sync all
              </Button>
            </div>
            <div className="rounded-lg border border-accent/20 bg-accent/5 p-2.5 text-[11px] leading-relaxed text-muted">
              Remote packs are verified on sync: content is SHA-256 pinned, signatures are checked against a
              source's pinned key, and every pack runs the same pre-install security review as built-ins. Unsigned
              packs still install (default-allow), just flagged.
            </div>
            {sources.length === 0 ? (
              <div className="rounded-lg border border-border bg-card p-3 text-[11px] text-muted">
                No remote sources yet. Add a marketplace.json URL below; its packs appear in the gallery after sync.
              </div>
            ) : (
              <ul className="space-y-1">
                {sources.map((s) => (
                  <li key={s.name} className="flex items-center gap-2 rounded-md border border-border/60 px-2 py-1 text-xs">
                    <span className="font-medium">{s.name}</span>
                    <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted">{s.url}</span>
                    {s.pubkey ? (
                      <ShieldCheck className="size-3 text-good" aria-label="signer key pinned" />
                    ) : (
                      <ShieldQuestion className="size-3 text-muted" aria-label="no signer key pinned" />
                    )}
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
            <div className="grid gap-2 border-t border-border pt-3 sm:grid-cols-[1fr_160px_auto]">
              <Input
                value={newURL}
                onChange={(e) => setNewURL(e.target.value)}
                placeholder="https://.../marketplace.json"
                className="h-8 min-w-0"
                aria-label="Marketplace URL"
              />
              <Input
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                placeholder="name"
                className="h-8"
                aria-label="Source name"
              />
              <Button variant="default" size="sm" disabled={!newURL.trim()} onClick={addSource}>
                <Plus className="size-3.5" /> Add
              </Button>
            </div>
          </div>
        </MarketModal>
      )}

      {detailPack && (
        <PackDetailModal
          pack={detailPack}
          detail={details[detailPack.name]}
          busy={busy === detailPack.name}
          progress={progress[detailPack.name]}
          onClose={() => setDetailPack(null)}
          onInstall={() => install(detailPack)}
          onUninstall={() => uninstall(detailPack)}
        />
      )}

      {/* Featured front page — curated picks, only on the unfiltered gallery. */}
      {!filterActive && featured.length > 0 && (
        <section className="space-y-2">
          <div className="flex items-center gap-1.5 text-label text-accent">
            <Sparkles className="size-3.5" /> Featured
          </div>
          <div className="grid gap-2 stagger-in sm:grid-cols-2 xl:grid-cols-3">
            {featured.map((p) => (
              <Card
                key={p.name}
                glass
                interactive
                onClick={() => openDetail(p)}
                className="relative overflow-hidden p-3"
              >
                <div
                  className="pointer-events-none absolute -right-8 -top-8 size-24 rounded-full opacity-40 blur-2xl"
                  style={{ background: "radial-gradient(circle, var(--accent), transparent 70%)" }}
                />
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-center gap-1.5">
                    <span className="font-display text-sm font-semibold text-foreground">{p.name}</span>
                    <TrustChip signed={p.signed} />
                  </div>
                  {p.installed ? (
                    <Badge variant="good">
                      <Check className="size-2.5" /> installed
                    </Badge>
                  ) : (
                    <Badge variant="accent">
                      <Sparkles className="size-2.5" /> pick
                    </Badge>
                  )}
                </div>
                {p.description && <p className="mt-1 line-clamp-2 text-[11px] text-muted">{p.description}</p>}
                <div className="mt-2 flex items-center gap-2 text-[11px] text-muted">
                  <ContentCounts p={p} />
                  {(p.downloads ?? 0) > 0 && (
                    <span className="inline-flex items-center gap-0.5">
                      <TrendingUp className="size-3" /> {compactCount(p.downloads!)}
                    </span>
                  )}
                </div>
              </Card>
            ))}
          </div>
        </section>
      )}

      {(categories.length > 2 || installedCount > 0) && (
        <div className="flex flex-wrap items-center gap-1">
          {categories.length > 2 &&
            categories.map((c) => (
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
          {installedCount > 0 && (
            <button
              onClick={() => setInstalledOnly((v) => !v)}
              role="switch"
              aria-checked={installedOnly}
              className={cn(
                "ml-auto inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[11px] transition-colors",
                installedOnly ? "border-good/40 bg-good/10 text-good" : "border-border text-muted hover:text-foreground",
              )}
            >
              <Check className="size-3" /> Installed only
            </button>
          )}
        </div>
      )}

      {err ? (
        <div className="text-xs text-bad">{err}</div>
      ) : packs === null ? (
        <SkeletonList count={6} lines={2} />
      ) : shown.length === 0 ? (
        <EmptyState icon={Store} title="No packs match" hint="Try a different search or category." />
      ) : (
        <>
          <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
            {shown.slice(0, win).map((p) => (
              <Card key={p.name} glass className="p-3">
                <div className="flex items-start gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <button
                        onClick={() => openDetail(p)}
                        className="font-medium text-foreground transition-colors hover:text-accent"
                        title="Open pack details"
                      >
                        {p.name}
                      </button>
                      <span className="font-mono text-xs text-muted">{p.version}</span>
                      <TrustChip signed={p.signed} />
                      {p.featured && (
                        <span className="inline-flex items-center gap-0.5 text-accent" title="Editor's pick">
                          <Sparkles className="size-3" />
                        </span>
                      )}
                      {p.installed && (
                        <Badge variant="good">
                          <Check className="size-2.5" /> installed
                        </Badge>
                      )}
                      {p.update_available && <Badge variant="warn">update</Badge>}
                    </div>
                    {p.description && <p className="mt-0.5 line-clamp-2 text-[11px] text-muted">{p.description}</p>}
                    <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted">
                      {p.category && <span className="rounded bg-panel px-1.5 py-0.5 capitalize">{p.category}</span>}
                      <ContentCounts p={p} />
                      {(p.downloads ?? 0) > 0 && (
                        <span className="inline-flex items-center gap-0.5" title="installs">
                          <TrendingUp className="size-3" /> {compactCount(p.downloads!)}
                        </span>
                      )}
                      <button
                        onClick={() => toggleDetails(p)}
                        aria-expanded={expanded.has(p.name)}
                        className="inline-flex items-center gap-0.5 text-accent transition-colors hover:text-accent2"
                      >
                        <ChevronDown className={cn("size-3 transition-transform", expanded.has(p.name) && "rotate-180")} />
                        What's inside
                      </button>
                    </div>
                    {expanded.has(p.name) && (
                      <div className="mt-1.5 space-y-1 border-t border-border/50 pt-1.5 text-xs">
                        {details[p.name] === "loading" || details[p.name] === undefined ? (
                          <span className="text-muted">loading…</span>
                        ) : (
                          <>
                            {(details[p.name] as PackDetails).vet && (
                              <div className="pb-1">
                                <VetBadge vet={(details[p.name] as PackDetails).vet} />
                              </div>
                            )}
                            {(details[p.name] as PackDetails).skills.map((s, i) => (
                              <div key={`s${i}`}>
                                <span className="text-accent">skill</span>{" "}
                                <span className="text-foreground">{s.name}</span>
                                {s.description && <span className="text-muted"> — {s.description}</span>}
                              </div>
                            ))}
                            {(details[p.name] as PackDetails).mcp_servers.map((m, i) => (
                              <div key={`m${i}`}>
                                <span className="text-accent2">mcp</span> <span className="text-foreground">{m}</span>
                              </div>
                            ))}
                            {(details[p.name] as PackDetails).tools.length > 0 && (
                              <div>
                                <span className="text-muted">CLI tools needed:</span>{" "}
                                {(details[p.name] as PackDetails).tools.join(", ")}
                              </div>
                            )}
                            <button
                              onClick={() => openDetail(p)}
                              className="pt-0.5 text-accent transition-colors hover:text-accent2"
                            >
                              Full details →
                            </button>
                          </>
                        )}
                      </div>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    {p.installed ? (
                      <>
                        {p.update_available && (
                          <Button
                            variant="default"
                            size="sm"
                            disabled={busy === p.name}
                            onClick={() => install(p)}
                            title="Update to the catalogued version"
                          >
                            {busy === p.name ? <RefreshCw className="size-3.5 animate-spin" /> : <RotateCw className="size-3.5" />}
                            Update
                          </Button>
                        )}
                        <Button variant="ghost" size="sm" disabled={busy === p.name} onClick={() => uninstall(p)} title="Uninstall">
                          <Trash2 className="size-3.5 text-bad" />
                        </Button>
                      </>
                    ) : (
                      <Button variant="accent" size="sm" disabled={busy === p.name} onClick={() => install(p)}>
                        {busy === p.name ? <RefreshCw className="size-3.5 animate-spin" /> : <Download className="size-3.5" />}
                        Install
                      </Button>
                    )}
                  </div>
                </div>
                {progress[p.name] && progress[p.name].length > 0 && <ProgressList steps={progress[p.name]} />}
              </Card>
            ))}
          </div>
          {shown.length > PACK_WINDOW && (
            <LoadMoreFooter
              hasMore={win < shown.length}
              loadingMore={false}
              onLoadMore={() => setWin((w) => w + PACK_WINDOW)}
              pageSize={Math.min(PACK_WINDOW, Math.max(1, shown.length - win))}
              label="packs"
            />
          )}
        </>
      )}
    </Page>
  );
}

// ContentCounts is the "N skills · N MCP · N tools" line, shared by cards.
function ContentCounts({ p }: { p: Pack }) {
  return (
    <span className="inline-flex items-center gap-2">
      <span className="inline-flex items-center gap-0.5">
        <Package className="size-3" /> {p.skill_count ?? 0}
      </span>
      <span className="inline-flex items-center gap-0.5">
        <Server className="size-3" /> {p.mcp_count ?? 0}
      </span>
      <span className="inline-flex items-center gap-0.5">
        <Terminal className="size-3" /> {p.tool_count ?? 0}
      </span>
    </span>
  );
}

// ProgressList renders the streamed install/uninstall steps (vet/skill/mcp/tool/done).
function ProgressList({ steps }: { steps: MarketStep[] }) {
  return (
    <ul className="mt-2 space-y-0.5 border-t border-border/50 pt-1.5 text-xs">
      {steps.map((s, i) => (
        <li key={i} className="flex items-center gap-1.5 font-mono">
          {s.ok ? <Check className="size-2.5 text-good" /> : <ShieldAlert className="size-2.5 text-bad" />}
          <span className="text-muted">{s.stage}</span>
          {s.name && <span className="text-foreground">{s.name}</span>}
          {s.detail && <span className="truncate text-muted">— {s.detail}</span>}
        </li>
      ))}
    </ul>
  );
}

// PackDetailModal is the full-page pack view: security review, contents, and the
// pack's README (the first skill's SKILL.md) rendered as markdown — so the
// operator can read exactly what the agent will be instructed with before
// installing, the way a serious registry shows a package page.
function PackDetailModal({
  pack,
  detail,
  busy,
  progress,
  onClose,
  onInstall,
  onUninstall,
}: {
  pack: Pack;
  detail: PackDetails | "loading" | undefined;
  busy: boolean;
  progress?: MarketStep[];
  onClose: () => void;
  onInstall: () => void;
  onUninstall: () => void;
}) {
  const d = detail && detail !== "loading" ? detail : null;
  const readme = d?.skills.map((s) => s.skill_md).find((md) => md && md.trim().length > 0);
  return (
    <MarketModal
      wide
      icon={Store}
      onClose={onClose}
      title={
        <span className="flex items-center gap-2">
          <span className="font-display">{pack.name}</span>
          <span className="font-mono text-xs font-normal text-muted">{pack.version}</span>
          <TrustChip signed={pack.signed} />
          {pack.featured && <Sparkles className="size-3.5 text-accent" aria-label="editor's pick" />}
        </span>
      }
    >
      <div className="space-y-4">
        {pack.description && <p className="text-sm text-muted">{pack.description}</p>}

        <div className="flex flex-wrap items-center gap-2">
          {pack.category && <Badge variant="default" className="capitalize">{pack.category}</Badge>}
          {d?.vet && <VetBadge vet={d.vet} />}
          {(pack.downloads ?? 0) > 0 && (
            <Badge variant="default">
              <TrendingUp className="size-2.5" /> {compactCount(pack.downloads!)} installs
            </Badge>
          )}
          <div className="ml-auto flex items-center gap-1.5">
            {pack.installed ? (
              <>
                {pack.update_available && (
                  <Button variant="accent" size="sm" disabled={busy} onClick={onInstall}>
                    {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <RotateCw className="size-3.5" />} Update
                  </Button>
                )}
                <Button variant="danger" size="sm" disabled={busy} onClick={onUninstall}>
                  <Trash2 className="size-3.5" /> Uninstall
                </Button>
              </>
            ) : (
              <Button variant="accent" size="sm" disabled={busy} onClick={onInstall}>
                {busy ? <RefreshCw className="size-3.5 animate-spin" /> : <Download className="size-3.5" />} Install
              </Button>
            )}
          </div>
        </div>

        {progress && progress.length > 0 && <ProgressList steps={progress} />}

        {/* Security review — the findings in full, so risky patterns are legible
            before an install rather than after. */}
        {d?.vet && d.vet.findings && d.vet.findings.length > 0 && (
          <section className="space-y-1.5">
            <div className="text-label">Security review</div>
            <ul className="space-y-1">
              {d.vet.findings.map((f, i) => (
                <li
                  key={i}
                  className={cn(
                    "flex items-start gap-2 rounded-md border px-2 py-1.5 text-xs",
                    f.severity === "danger"
                      ? "border-bad/30 bg-bad/5"
                      : f.severity === "warn"
                        ? "border-warn/30 bg-warn/5"
                        : "border-border bg-panel/40",
                  )}
                >
                  <Badge variant={f.severity === "danger" ? "bad" : f.severity === "warn" ? "warn" : "default"}>
                    {f.severity}
                  </Badge>
                  <span className="min-w-0">
                    <span className="font-mono text-muted">{f.where}</span> — {f.detail}
                  </span>
                </li>
              ))}
            </ul>
          </section>
        )}

        {/* Contents */}
        <section className="grid gap-3 sm:grid-cols-3">
          <ContentColumn icon={Package} label="Skills" tone="text-accent">
            {detail === "loading" || !d ? (
              <span className="text-muted">loading…</span>
            ) : d.skills.length === 0 ? (
              <span className="text-muted">none</span>
            ) : (
              d.skills.map((s, i) => (
                <div key={i} className="text-foreground">
                  {s.name}
                  {s.description && <span className="block text-[11px] text-muted">{s.description}</span>}
                </div>
              ))
            )}
          </ContentColumn>
          <ContentColumn icon={Server} label="MCP servers" tone="text-accent2">
            {d && d.mcp_servers.length > 0 ? d.mcp_servers.map((m, i) => <div key={i} className="text-foreground">{m}</div>) : <span className="text-muted">none</span>}
          </ContentColumn>
          <ContentColumn icon={Terminal} label="CLI tools" tone="text-warn">
            {d && d.tools.length > 0 ? (
              d.tools.map((t, i) => (
                <div key={i} className="font-mono text-foreground">
                  {t}
                </div>
              ))
            ) : (
              <span className="text-muted">none</span>
            )}
          </ContentColumn>
        </section>

        {/* README — the pack's first SKILL.md, rendered. */}
        {readme && (
          <section className="space-y-1.5">
            <div className="text-label">Readme</div>
            <div className="rounded-lg border border-border bg-panel/40 p-3">
              <Markdown source={stripFrontmatter(readme)} />
            </div>
          </section>
        )}
      </div>
    </MarketModal>
  );
}

function ContentColumn({
  icon: Icon,
  label,
  tone,
  children,
}: {
  icon: LucideIcon;
  label: string;
  tone: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-1">
      <div className={cn("flex items-center gap-1 text-label", tone)}>
        <Icon className="size-3" /> {label}
      </div>
      <div className="space-y-1 text-xs">{children}</div>
    </div>
  );
}

// stripFrontmatter drops a leading YAML block so the rendered README shows the
// instructional body, not the raw metadata header.
function stripFrontmatter(md: string): string {
  const m = md.match(/^---\n[\s\S]*?\n---\n?/);
  return m ? md.slice(m[0].length) : md;
}
