import { useEffect, useMemo, useRef, useState } from "react";
import {
  Wrench, RefreshCw, Search, Download, PackageCheck, PackageX, ArrowUpCircle,
  TerminalSquare, CheckCircle2, XCircle, MinusCircle, Boxes, Cpu, Loader2, X,
} from "lucide-react";
import { getJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/empty";
import { SkeletonList } from "@/components/ui/skeleton";
import { ErrorText } from "@/components/JsonView";
import { useUI } from "@/components/ui/feedback";
import { PageHeader } from "@/components/ui/page-header";
import {
  filterTools, census, categoriesPresent, CATEGORY_LABELS, streamInstall,
  type Inventory, type ToolStatus, type ToolFilter, type ToolCategory, type InstallProgress,
} from "@/lib/toolbox";

// Toolbox (M956) — the host CLI-tool library. Shows what's installed / missing /
// outdated on the machine agezt runs on, and installs missing tools via the host
// package manager (winget/choco/brew/apt…) with live per-tool progress. The
// catalog + detection + install run in the Go backend; this view renders them.
export function Toolbox() {
  const ui = useUI();
  const [inv, setInv] = useState<Inventory | null>(null);
  const [outdated, setOutdated] = useState<Set<string>>(new Set());
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [checkingUpd, setCheckingUpd] = useState(false);
  const [filter, setFilter] = useState<ToolFilter>("all");
  const [query, setQuery] = useState("");
  const [running, setRunning] = useState(false);
  const [log, setLog] = useState<InstallProgress[]>([]);
  const [showLog, setShowLog] = useState(false);
  const abort = useRef<AbortController | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<Inventory>("/api/toolbox");
      setInv(d);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    reload();
  }, []);

  async function checkUpdates() {
    setCheckingUpd(true);
    try {
      const d = await getJSON<{ outdated?: string[] }>("/api/toolbox/updates");
      setOutdated(new Set(d.outdated || []));
      ui.toast(`${(d.outdated || []).length} update(s) available`, "success");
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setCheckingUpd(false);
    }
  }

  async function runInstall(names: string[], confirmMsg?: { title: string; message: string }) {
    if (names.length === 0) return;
    if (confirmMsg && !(await ui.confirm({ ...confirmMsg, confirmLabel: `Install ${names.length}` }))) return;
    setRunning(true);
    setShowLog(true);
    setLog([]);
    abort.current = new AbortController();
    try {
      await streamInstall(
        names,
        (f) => {
          if (f.kind === "toolbox.progress" && f.payload) {
            setLog((l) => [...l, f.payload as unknown as InstallProgress]);
          } else if (f.kind === "error") {
            ui.toast(f.error || "install failed", "error");
          }
        },
        abort.current.signal,
      );
      ui.toast("install run finished", "success");
      reload();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setRunning(false);
      abort.current = null;
    }
  }

  const tools = inv?.tools || [];
  const cen = useMemo(() => census(tools, outdated), [tools, outdated]);
  const shown = useMemo(() => filterTools(tools, filter, query), [tools, filter, query]);
  const cats = useMemo(() => categoriesPresent(tools), [tools]);

  // Bulk-install targets: every installable missing tool (Full), or the missing
  // installable ones in the active category.
  const missingInstallable = tools.filter((t) => !t.installed && t.installable).map((t) => t.name);
  const categoryMissing = (cat: ToolCategory) =>
    tools.filter((t) => t.category === cat && !t.installed && t.installable).map((t) => t.name);

  return (
    <div className="space-y-3">
      {/* Header */}
      <PageHeader
        icon={Wrench}
        title="Toolbox"
        description="Host CLI-tool library — install missing tools via the host package manager."
        actions={
          <>
            <Button variant="ghost" size="sm" onClick={checkUpdates} disabled={checkingUpd || !inv} title="Ask the package managers what's upgradable">
              <ArrowUpCircle className={cn("size-3.5", checkingUpd && "animate-pulse")} /> Check updates
            </Button>
            <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Re-scan host">
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
          </>
        }
      />

      {/* Host / package-manager chips */}
      {inv && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center gap-1 text-[11px] text-muted">
            <Cpu className="size-3" /> {inv.os}
          </span>
          {inv.managers.length > 0 && (
            <span className="flex flex-wrap items-center gap-1">
              {inv.managers.map((m) => (
                <Badge key={m} variant="default" className="font-mono text-xs">{m}</Badge>
              ))}
            </span>
          )}
        </div>
      )}

      {/* Census band */}
      {inv && (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-5">
          <BigStat icon={Boxes} label="catalog" value={cen.total} />
          <BigStat icon={PackageCheck} label="installed" value={cen.installed} accent={cen.installed > 0} />
          <BigStat icon={PackageX} label="missing" value={cen.missing} />
          <BigStat icon={ArrowUpCircle} label="outdated" value={cen.outdated} />
          <BigStat icon={Download} label="installable" value={cen.installableMissing} />
        </div>
      )}

      {/* Bulk install */}
      {inv && cen.installableMissing > 0 && (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-accent/30 bg-accent/5 p-2.5">
          <span className="text-[11px] text-muted">
            {cen.installableMissing} installable tool(s) missing — install in one go:
          </span>
          <Button
            size="sm"
            disabled={running}
            onClick={() =>
              runInstall(missingInstallable, {
                title: `Install all ${missingInstallable.length} missing tools?`,
                message: `Runs your host package manager for: ${missingInstallable.join(", ")}. This changes the machine agezt runs on.`,
              })
            }
          >
            <Download className="size-3.5" /> Install all missing
          </Button>
        </div>
      )}

      {/* Filters + search */}
      {inv && (
        <div className="flex flex-wrap items-center gap-1.5">
          {(["all", "installed", "missing"] as ToolFilter[]).map((f) => (
            <FilterChip key={f} active={filter === f} onClick={() => setFilter(f)} label={f}
              count={f === "all" ? cen.total : f === "installed" ? cen.installed : cen.missing} />
          ))}
          {cats.map((c) => (
            <FilterChip key={c} active={filter === c} onClick={() => setFilter(c)} label={CATEGORY_LABELS[c]}
              count={tools.filter((t) => t.category === c).length} />
          ))}
          <div className="relative ml-auto">
            <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted" />
            <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="search tools…"
              className="w-44 rounded-full border border-border bg-card py-1 pl-7 pr-2 text-xs outline-none focus:border-accent" />
          </div>
        </div>
      )}

      {/* Category bulk-install hint when a category is selected */}
      {inv && filter !== "all" && filter !== "installed" && filter !== "missing" && categoryMissing(filter).length > 0 && (
        <Button
          size="sm" variant="ghost" disabled={running}
          onClick={() =>
            runInstall(categoryMissing(filter as ToolCategory), {
              title: `Install ${categoryMissing(filter as ToolCategory).length} missing ${CATEGORY_LABELS[filter as ToolCategory]}?`,
              message: `Runs the host package manager for: ${categoryMissing(filter as ToolCategory).join(", ")}.`,
            })
          }
        >
          <Download className="size-3.5" /> Install missing in {CATEGORY_LABELS[filter as ToolCategory]}
        </Button>
      )}

      {/* Grid */}
      {err ? (
        <ErrorText>{err}</ErrorText>
      ) : !inv ? (
        <SkeletonList count={6} lines={2} />
      ) : shown.length === 0 ? (
        <EmptyState icon={Search} title="No matches" hint="Try a different filter or search term." />
      ) : (
        <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
          {shown.map((t) => (
            <ToolCard key={t.name} t={t} outdated={outdated.has(t.name)} running={running}
              onInstall={() => runInstall([t.name])} />
          ))}
        </div>
      )}

      {/* Live install log */}
      {showLog && (
        <div className="glass rounded-xl">
          <div className="flex items-center gap-2 border-b border-border px-3 py-1.5">
            <TerminalSquare className="size-3.5 text-accent" />
            <span className="text-[11px] font-semibold">Install output</span>
            {running ? (
              <span className="inline-flex items-center gap-1 text-xs text-accent"><Loader2 className="size-3 animate-spin" /> running…</span>
            ) : (
              <span className="text-xs text-muted">done</span>
            )}
            <span className="ml-auto flex items-center gap-1">
              {running && abort.current && (
                <Button size="sm" variant="ghost" onClick={() => abort.current?.abort()}>cancel</Button>
              )}
              {!running && (
                <button onClick={() => setShowLog(false)} className="rounded border border-border p-1 text-muted hover:text-foreground" title="Close">
                  <X className="size-3" />
                </button>
              )}
            </span>
          </div>
          {log.length === 0 ? (
            <div className="px-3 py-2 text-[11px] text-muted">starting…</div>
          ) : (
            <ul className="max-h-72 space-y-1 overflow-auto p-2">
              {log.map((p, i) => (
                <li key={`${p.tool}-${i}`} className="text-[11px]">
                  <div className="flex items-center gap-2">
                    {p.ok ? <CheckCircle2 className="size-3.5 text-good" /> : p.skipped ? <MinusCircle className="size-3.5 text-muted" /> : <XCircle className="size-3.5 text-bad" />}
                    <span className="font-mono font-medium">{p.tool}</span>
                    {p.version && <span className="text-muted">{p.version}</span>}
                    {p.manager && <span className="ml-auto font-mono text-xs text-muted">{p.manager}</span>}
                  </div>
                  {(p.error || p.command) && (
                    <div className="ml-5 font-mono text-xs text-muted">{p.error ? `✗ ${p.error}` : p.command}</div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}

function ToolCard({ t, outdated, running, onInstall }: { t: ToolStatus; outdated: boolean; running: boolean; onInstall: () => void }) {
  return (
    <div className={cn("glass flex flex-col gap-2 rounded-xl p-3", !t.installed && "border-border/60")}>
      <div className="flex items-center gap-2">
        <span className="font-mono text-sm font-semibold">{t.name}</span>
        {t.installed ? (
          outdated ? <Badge variant="accent">update</Badge> : <Badge variant="good">installed</Badge>
        ) : (
          <Badge variant="default" className="text-muted">missing</Badge>
        )}
        <span className="ml-auto text-xs text-muted">{CATEGORY_LABELS[t.category as ToolCategory] || t.category}</span>
      </div>
      {t.description && <div className="line-clamp-2 text-[11px] text-muted">{t.description}</div>}
      {t.installed && t.version && <div className="truncate font-mono text-xs text-foreground/70" title={t.path}>{t.version}</div>}
      {!t.installed && t.command && <div className="truncate font-mono text-xs text-muted" title={t.command}>$ {t.command}</div>}
      <div className="mt-auto flex items-center gap-2 pt-1">
        {!t.installed && t.installable && (
          <Button size="sm" disabled={running} onClick={onInstall}><Download className="size-3.5" /> Install</Button>
        )}
        {!t.installed && !t.installable && (
          <span className="text-xs text-muted">no install recipe for this host</span>
        )}
        {t.installed && outdated && (
          <Button size="sm" variant="ghost" disabled={running} onClick={onInstall}><ArrowUpCircle className="size-3.5" /> Update</Button>
        )}
      </div>
    </div>
  );
}

function FilterChip({ active, onClick, label, count }: { active: boolean; onClick: () => void; label: string; count: number }) {
  return (
    <button onClick={onClick}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs capitalize transition-colors",
        active ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:border-accent",
      )}>
      {label}
      <span className="rounded-full bg-card px-1.5 text-xs tabular-nums">{count}</span>
    </button>
  );
}

function BigStat({ icon: Icon, label, value, accent }: { icon: typeof Boxes; label: string; value: number | string; accent?: boolean }) {
  return (
    <div className={cn("rounded-xl p-2.5", accent ? "border border-accent/50 bg-card" : "glass")}>
      <div className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-muted">
        <Icon className={cn("size-3", accent && "text-accent")} /> {label}
      </div>
      <div className={cn("mt-0.5 text-lg font-semibold tabular-nums", accent && "text-accent")}>{value}</div>
    </div>
  );
}
