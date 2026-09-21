// Backups.tsx — Day 26 bring-back.
//
// Admin › Backups surfaces the daemon's rollback checkpoints. The daemon
// keeps a checkpoint before any file-mutating operation (config writes,
// skill imports, channel updates, world edits) so the operator can roll
// back. The view lists checkpoints, lets the operator filter by run / kind,
// and shows a confirm dialog before applying one. Apply calls
// /api/rollback/apply and reloads the list on success.
import { useEffect, useMemo, useState } from "react";
import { Archive, ArrowDownToLine, Filter, RotateCcw, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { getJSON, postJSON } from "@/app/api";
import { useUI } from "@/components/ui/feedback";
import { cn, fmtAgo, fmtTime } from "@/app/utils/utils";

interface Checkpoint {
  id: string;
  created_at?: number;
  run_id?: string;
  kind?: string;
  description?: string;
  affected_paths?: string[];
  size_bytes?: number;
  reversible?: boolean;
}

export function Backups() {
  const ui = useUI();
  const [checkpoints, setCheckpoints] = useState<Checkpoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [runFilter, setRunFilter] = useState("");
  const [kindFilter, setKindFilter] = useState("all");
  const [pending, setPending] = useState<Checkpoint | null>(null);
  const [applying, setApplying] = useState(false);

  async function refresh() {
    setLoading(true);
    setError(null);
    try {
      const qs = runFilter.trim()
        ? `?run_id=${encodeURIComponent(runFilter.trim())}`
        : "";
      const r = await getJSON<{ checkpoints?: Checkpoint[] }>(
        `/api/rollback/checkpoints${qs}`,
      );
      setCheckpoints(r.checkpoints || []);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const kinds = useMemo(() => {
    const counts = new Map<string, number>();
    checkpoints.forEach((c) => {
      if (!c.kind) return;
      counts.set(c.kind, (counts.get(c.kind) || 0) + 1);
    });
    return Array.from(counts.entries()).sort((a, b) => b[1] - a[1]);
  }, [checkpoints]);

  const filtered = useMemo(() => {
    return checkpoints.filter((c) => {
      if (kindFilter !== "all" && c.kind !== kindFilter) return false;
      return true;
    });
  }, [checkpoints, kindFilter]);

  async function apply(cp: Checkpoint) {
    setApplying(true);
    try {
      await postJSON("/api/rollback/apply", { id: cp.id });
      ui.toast(`Restored checkpoint ${cp.id}`, "success");
      setPending(null);
      refresh();
    } catch (e) {
      ui.toast((e as Error).message, "error");
    } finally {
      setApplying(false);
    }
  }

  return (
    <div className="flex h-full flex-col gap-4 p-4">
      <header className="rounded-lg border border-border bg-card p-4">
        <div className="flex items-center gap-2">
          <Archive className="size-5 text-accent" />
          <h2 className="text-lg font-semibold text-fg">Backups</h2>
          <span className="rounded-full bg-accent/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-accent">
            Admin
          </span>
        </div>
        <p className="mt-1 text-sm text-muted">
          Rollback checkpoints the daemon keeps before mutating files. Pick a checkpoint and
          restore the workspace to that point in time.
        </p>

        <div className="mt-3 flex flex-wrap items-end gap-3">
          <label className="flex flex-1 min-w-[180px] flex-col gap-1 text-xs font-medium text-muted">
            <span className="inline-flex items-center gap-1">
              <Filter className="size-3" /> Run ID
            </span>
            <input
              type="text"
              value={runFilter}
              onChange={(e) => setRunFilter(e.target.value)}
              placeholder="filter by run correlation id"
              className="w-full rounded-md border border-border bg-bg px-2 py-1.5 text-sm text-fg focus:border-accent focus:outline-none"
            />
          </label>
          <label className="flex flex-col gap-1 text-xs font-medium text-muted">
            <span>Kind</span>
            <select
              value={kindFilter}
              onChange={(e) => setKindFilter(e.target.value)}
              className="rounded-md border border-border bg-bg px-2 py-1.5 text-sm text-fg focus:border-accent focus:outline-none"
            >
              <option value="all">All kinds</option>
              {kinds.map(([k]) => (
                <option key={k} value={k}>
                  {k}
                </option>
              ))}
            </select>
          </label>
          <Button onClick={refresh} disabled={loading}>
            {loading ? "Refreshing…" : "Refresh"}
          </Button>
        </div>
      </header>

      {error && (
        <div className="rounded-lg border border-danger/40 bg-danger/5 p-3 text-sm text-danger">
          {error}
        </div>
      )}

      <main className="flex-1 overflow-auto rounded-lg border border-border bg-card">
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-card text-left text-xs uppercase tracking-wide text-muted">
            <tr>
              <th className="px-3 py-2">When</th>
              <th className="px-3 py-2">ID</th>
              <th className="px-3 py-2">Run</th>
              <th className="px-3 py-2">Kind</th>
              <th className="px-3 py-2">Description</th>
              <th className="px-3 py-2">Paths</th>
              <th className="px-3 py-2">Size</th>
              <th className="px-3 py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {loading && checkpoints.length === 0 ? (
              <tr>
                <td colSpan={8} className="px-3 py-6 text-center text-sm text-muted">
                  Loading checkpoints…
                </td>
              </tr>
            ) : filtered.length === 0 ? (
              <tr>
                <td colSpan={8} className="px-3 py-6 text-center text-sm text-muted">
                  No checkpoints match the filter. Run a few file-mutating actions to populate
                  this list.
                </td>
              </tr>
            ) : (
              filtered.map((cp) => (
                <tr key={cp.id} className="border-t border-border align-top hover:bg-bg/40">
                  <td className="px-3 py-2 text-xs text-muted">
                    <div>{fmtAgo(cp.created_at)}</div>
                    <div className="text-[10px]">{fmtTime(cp.created_at)}</div>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-fg/80" title={cp.id}>
                    {cp.id.slice(0, 12)}
                  </td>
                  <td className="px-3 py-2 text-xs text-fg/90">{cp.run_id || "—"}</td>
                  <td className="px-3 py-2">
                    <span className="rounded-full bg-bg px-2 py-0.5 text-[10px] font-medium text-fg/80">
                      {cp.kind || "—"}
                    </span>
                  </td>
                  <td className="px-3 py-2 text-xs text-fg/90">{cp.description || "—"}</td>
                  <td className="px-3 py-2 text-xs text-fg/80">
                    {cp.affected_paths && cp.affected_paths.length > 0 ? (
                      <ul className="space-y-0.5">
                        {cp.affected_paths.slice(0, 3).map((p, i) => (
                          <li key={i} className="truncate" title={p}>
                            {p}
                          </li>
                        ))}
                        {cp.affected_paths.length > 3 && (
                          <li className="text-[10px] text-muted">
                            +{cp.affected_paths.length - 3} more
                          </li>
                        )}
                      </ul>
                    ) : (
                      "—"
                    )}
                  </td>
                  <td className="px-3 py-2 text-xs text-fg/80">
                    {typeof cp.size_bytes === "number"
                      ? `${(cp.size_bytes / 1024).toFixed(1)} kB`
                      : "—"}
                  </td>
                  <td className="px-3 py-2 text-right">
                    <Button
                      variant="ghost"
                      onClick={() => setPending(cp)}
                      disabled={!cp.reversible}
                    >
                      <ArrowDownToLine className="size-3.5" />
                      Restore
                    </Button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </main>

      {pending && (
        <Confirm
          cp={pending}
          busy={applying}
          onCancel={() => setPending(null)}
          onConfirm={() => apply(pending)}
        />
      )}
    </div>
  );
}

function Confirm({
  cp,
  busy,
  onCancel,
  onConfirm,
}: {
  cp: Checkpoint;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div
        className={cn(
          "w-full max-w-md rounded-xl border border-border bg-card p-5 shadow-e2",
        )}
      >
        <div className="flex items-center gap-2">
          <ShieldCheck className="size-5 text-warning" />
          <h3 className="text-base font-semibold text-fg">Restore checkpoint?</h3>
        </div>
        <p className="mt-2 text-sm text-muted">
          Files affected by{" "}
          <code className="rounded bg-bg px-1 py-0.5 text-xs">{cp.id}</code> will be reverted
          to their state at{" "}
          <span className="font-medium text-fg">{fmtTime(cp.created_at)}</span>. This action
          can't be undone from this screen.
        </p>
        {cp.affected_paths && cp.affected_paths.length > 0 && (
          <ul className="mt-3 max-h-32 overflow-auto rounded-md border border-border bg-bg/40 p-2 text-xs text-fg/80">
            {cp.affected_paths.map((p, i) => (
              <li key={i} className="truncate">
                {p}
              </li>
            ))}
          </ul>
        )}
        <div className="mt-4 flex items-center justify-end gap-2">
          <Button variant="ghost" onClick={onCancel} disabled={busy}>
            Cancel
          </Button>
          <Button onClick={onConfirm} disabled={busy}>
            <RotateCcw className="size-3.5" />
            {busy ? "Restoring…" : "Restore"}
          </Button>
        </div>
      </div>
    </div>
  );
}
