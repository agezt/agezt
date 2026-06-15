import { useMemo, useState } from "react";
import { HardDrive, RefreshCw, Trash2, Brain, Combine, Skull, Loader2, FolderTree } from "lucide-react";
import { usePanel } from "@/lib/usePanel";
import { getJSON, postAction } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { SkeletonGrid } from "@/components/ui/skeleton";
import { ErrorText } from "@/components/JsonView";
import { useUI } from "@/components/ui/feedback";
import { PageHeader } from "@/components/ui/page-header";

// Storage view (M927): what under ~/.agezt is taking the space, and the
// collectors that reclaim it. The breakdown comes from /api/storage
// (storage_stats); each collector is the existing dry-run-first command
// (artifact collect M845, memory prune M857, brain consolidation M851,
// reaper scan M903) surfaced as a card — see the candidates, then confirm.

interface StorageDir {
  name: string;
  bytes: number;
  files: number;
  label?: string;
}

interface StorageStats {
  base_dir: string;
  total_bytes: number;
  total_files: number;
  dirs: StorageDir[];
  disk_available: boolean;
  disk_free_bytes?: number;
  disk_total_bytes?: number;
  disk_free_pct?: number;
}

// fmtBytes renders a byte count at a human scale (B → GB).
export function fmtBytes(n?: number): string {
  if (!n || n <= 0) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

// pctOf returns dir share of the total as a 0–100 number (0 when total is 0).
export function pctOf(bytes: number, total: number): number {
  if (!total || total <= 0) return 0;
  return (bytes / total) * 100;
}

export function Storage() {
  const ui = useUI();
  const { data, error, loading, reload } = usePanel<StorageStats>("/api/storage");
  const dirs = useMemo(() => data?.dirs ?? [], [data]);
  const total = data?.total_bytes ?? 0;
  const biggest = dirs[0];

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <PageHeader
        icon={HardDrive}
        title="Storage"
        description={
          data ? (
            <span className="truncate" title={data.base_dir}>
              {data.base_dir}
            </span>
          ) : undefined
        }
        actions={
          <Button variant="ghost" size="sm" onClick={reload} disabled={loading} title="Reload">
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          </Button>
        }
      />

      {error && <ErrorText>{error}</ErrorText>}
      {loading && !data && <SkeletonGrid count={6} />}

      {data && (
        <div className="min-h-0 flex-1 space-y-4 overflow-auto pr-1">
          {/* Summary band */}
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <SummaryCard label="Total used" value={fmtBytes(total)} sub={`${data.total_files} files`} />
            <SummaryCard
              label="Disk free"
              value={data.disk_available ? `${(data.disk_free_pct ?? 0).toFixed(0)}%` : "—"}
              sub={data.disk_available ? `${fmtBytes(data.disk_free_bytes)} of ${fmtBytes(data.disk_total_bytes)}` : "probe unavailable"}
              warn={data.disk_available && (data.disk_free_pct ?? 100) < 10}
            />
            <SummaryCard label="Subsystems" value={String(dirs.length)} sub="top-level directories" />
            <SummaryCard
              label="Largest"
              value={biggest ? biggest.name : "—"}
              sub={biggest ? `${fmtBytes(biggest.bytes)} · ${pctOf(biggest.bytes, total).toFixed(0)}%` : ""}
            />
          </div>

          {/* Per-directory breakdown */}
          <div>
            <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-muted">
              <FolderTree className="size-3" /> Breakdown
            </div>
            <ul className="space-y-1">
              {dirs.map((d) => {
                const pct = pctOf(d.bytes, total);
                return (
                  <li key={d.name} className="glass rounded-xl px-3 py-2">
                    <div className="flex items-baseline gap-2 text-sm">
                      <span className="font-mono font-medium">{d.name}</span>
                      <span className="min-w-0 flex-1 truncate text-[11px] text-muted" title={d.label}>
                        {d.label || ""}
                      </span>
                      <span className="text-[11px] text-muted">{d.files} files</span>
                      <span className="w-20 text-right text-xs font-semibold tabular-nums">{fmtBytes(d.bytes)}</span>
                    </div>
                    <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-panel">
                      <div
                        className="h-full rounded-full bg-accent/70"
                        style={{ width: `${Math.max(pct, d.bytes > 0 ? 1 : 0)}%` }}
                        role="progressbar"
                        aria-valuenow={Math.round(pct)}
                      />
                    </div>
                  </li>
                );
              })}
              {dirs.length === 0 && <li className="py-8 text-center text-sm text-muted">Home directory is empty.</li>}
            </ul>
          </div>

          {/* Collectors */}
          <div>
            <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-muted">
              <Trash2 className="size-3" /> Collectors
            </div>
            <div className="grid gap-2 md:grid-cols-2">
              <ArtifactCollector onDone={reload} />
              <MemoryPruner onDone={reload} />
              <BrainConsolidator onDone={reload} />
              <ReaperCard />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function SummaryCard({ label, value, sub, warn }: { label: string; value: string; sub?: string; warn?: boolean }) {
  return (
    <div className={cn("glass rounded-xl px-3 py-2", warn && "border-bad/50")}>
      <div className="text-[11px] uppercase tracking-wide text-muted">{label}</div>
      <div className={cn("truncate text-lg font-semibold", warn && "text-bad")}>{value}</div>
      {sub && <div className="truncate text-[11px] text-muted">{sub}</div>}
    </div>
  );
}

function CollectorCard({
  icon: Icon,
  title,
  desc,
  children,
}: {
  icon: typeof Trash2;
  title: string;
  desc: string;
  children: React.ReactNode;
}) {
  return (
    <div className="glass flex flex-col gap-2 rounded-xl p-3">
      <div className="flex items-center gap-2 text-sm font-semibold">
        <Icon className="size-4 text-accent" /> {title}
      </div>
      <p className="text-xs text-muted">{desc}</p>
      <div className="mt-auto flex flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}

function DaysInput({ value, onChange }: { value: number; onChange: (n: number) => void }) {
  return (
    <label className="flex items-center gap-1 text-xs text-muted">
      older than
      <input
        type="number"
        min={1}
        value={value}
        onChange={(e) => onChange(Math.max(1, Number(e.target.value) || 1))}
        className="w-14 rounded border border-border bg-panel px-1.5 py-0.5 text-xs text-foreground"
      />
      days
    </label>
  );
}

// ArtifactCollector — dry-run first (how many, how big), then a confirmed
// destructive pass. Same flow the Files view exposes (M845).
function ArtifactCollector({ onDone }: { onDone: () => void }) {
  const ui = useUI();
  const [days, setDays] = useState(30);
  const [busy, setBusy] = useState(false);

  async function run() {
    setBusy(true);
    try {
      const dry = await postAction<{ count: number; bytes: number }>("/api/artifact/collect", {
        older_than_days: String(days),
        dry_run: "true",
      });
      if (!dry.count) {
        ui.toast(`Nothing to collect — no artifacts older than ${days} days.`, "success");
        return;
      }
      const ok = await ui.confirm({
        title: `Collect ${dry.count} stale artifact${dry.count === 1 ? "" : "s"}?`,
        message: `Permanently delete artifacts older than ${days} days (~${fmtBytes(dry.bytes)}). Recent files are kept.`,
        confirmLabel: "Collect",
        danger: true,
      });
      if (!ok) return;
      const res = await postAction<{ count: number; bytes: number }>("/api/artifact/collect", {
        older_than_days: String(days),
        dry_run: "false",
      });
      ui.toast(`Collected ${res.count} artifact${res.count === 1 ? "" : "s"} (~${fmtBytes(res.bytes)}).`, "success");
      onDone();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <CollectorCard icon={Trash2} title="Artifact collect" desc="Reap stale stored files (inbound images, tool outputs). Dry-run shows the candidates before anything is deleted; blobs are kept while any entry still references them.">
      <DaysInput value={days} onChange={setDays} />
      <Button variant="default" size="sm" onClick={run} disabled={busy}>
        {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Trash2 className="size-3.5" />} Collect
      </Button>
    </CollectorCard>
  );
}

// MemoryPruner — hard-removes soft-deleted (tombstoned/superseded) memory
// records past the age threshold (M857). Dry-run reports prunable count.
function MemoryPruner({ onDone }: { onDone: () => void }) {
  const ui = useUI();
  const [days, setDays] = useState(30);
  const [busy, setBusy] = useState(false);

  async function run() {
    setBusy(true);
    try {
      const dry = await postAction<{ prunable: number }>("/api/memory/prune", {
        older_than_days: String(days),
        dry_run: "true",
      });
      if (!dry.prunable) {
        ui.toast(`Memory is clean — no soft-deleted records older than ${days} days.`, "success");
        return;
      }
      const ok = await ui.confirm({
        title: `Prune ${dry.prunable} dead memory record${dry.prunable === 1 ? "" : "s"}?`,
        message: `Permanently remove tombstoned/superseded records older than ${days} days. Active memories are never touched.`,
        confirmLabel: "Prune",
        danger: true,
      });
      if (!ok) return;
      const res = await postAction<{ pruned: number }>("/api/memory/prune", {
        older_than_days: String(days),
        dry_run: "false",
      });
      ui.toast(`Pruned ${res.pruned} record${res.pruned === 1 ? "" : "s"}.`, "success");
      onDone();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <CollectorCard icon={Brain} title="Memory prune" desc="Reclaim the dead weight forgetting and consolidation leave behind: soft-deleted memory records past their recovery window. Active memories are never touched.">
      <DaysInput value={days} onChange={setDays} />
      <Button variant="default" size="sm" onClick={run} disabled={busy}>
        {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Brain className="size-3.5" />} Prune
      </Button>
    </CollectorCard>
  );
}

// BrainConsolidator — runs the distillation pass (M851): clusters near-duplicate
// memories per scope and merges each cluster into one richer record (compaction).
function BrainConsolidator({ onDone }: { onDone: () => void }) {
  const ui = useUI();
  const [busy, setBusy] = useState(false);

  async function run() {
    const ok = await ui.confirm({
      title: "Consolidate memory?",
      message:
        "Clusters near-duplicate memories and merges each cluster into one richer record (originals are superseded, recoverable until pruned). Uses the model — this can take a minute.",
      confirmLabel: "Consolidate",
    });
    if (!ok) return;
    setBusy(true);
    try {
      const res = await postAction<{ clusters_merged: number; records_superseded: number }>("/api/memory/consolidate");
      ui.toast(`Merged ${res.clusters_merged ?? 0} cluster${res.clusters_merged === 1 ? "" : "s"}, superseded ${res.records_superseded ?? 0} records.`, "success");
      onDone();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <CollectorCard icon={Combine} title="Memory consolidate" desc="Compact the brain: merge near-duplicate memories into single richer records. Superseded originals stay recoverable until the next prune.">
      <Button variant="default" size="sm" onClick={run} disabled={busy}>
        {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Combine className="size-3.5" />} Consolidate
      </Button>
    </CollectorCard>
  );
}

interface ReaperReport {
  dead_count: number;
  dead_agents: { slug: string; name?: string; last_active_ms?: number }[];
  stale_artifacts: number;
  stale_bytes: number;
}

// ReaperCard — read-only scan (M903): surfaces dead agents and the stale
// artifact pile. Retiring agents stays in the Roster; collecting stays above.
function ReaperCard() {
  const ui = useUI();
  const [busy, setBusy] = useState(false);
  const [report, setReport] = useState<ReaperReport | null>(null);

  async function scan() {
    setBusy(true);
    try {
      setReport(await getJSON<ReaperReport>("/api/reaper/scan"));
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <CollectorCard icon={Skull} title="Reaper scan" desc="Read-only detection: roster agents idle for 30+ days and the stale artifact pile. Nothing is deleted — retire agents from the Roster, collect artifacts above.">
      <Button variant="default" size="sm" onClick={scan} disabled={busy}>
        {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Skull className="size-3.5" />} Scan
      </Button>
      {report && (
        <span className="text-xs text-muted">
          {report.dead_count} idle agent{report.dead_count === 1 ? "" : "s"} · {report.stale_artifacts} stale artifacts ({fmtBytes(report.stale_bytes)})
        </span>
      )}
    </CollectorCard>
  );
}
