import { useEffect, useState } from "react";
import { Armchair, Plus, RefreshCw, Trash2, Shield, Lock } from "lucide-react";
import { getJSON, postJSON } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Page } from "@/components/ui/page";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ErrorText } from "@/components/JsonView";
import { SkeletonList } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty";

export interface SeatDef {
  id: string;
  name?: string;
  description?: string;
  execution_profile?: string;
  tools?: string[];
  restrict_tools?: boolean;
  builtin?: boolean;
}

interface SeatsData {
  seats?: SeatDef[];
  count?: number;
}

const ISO_OPTIONS = ["", "local", "warden", "container"];

export function seatIsoLabel(s: SeatDef): string {
  return s.execution_profile?.trim() ? s.execution_profile : "tool defaults";
}

export function Seats() {
  const [seats, setSeats] = useState<SeatDef[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [id, setId] = useState("");
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [iso, setIso] = useState("");

  async function reload() {
    setLoading(true);
    try {
      const d = await getJSON<SeatsData>("/api/seats");
      setSeats(Array.isArray(d.seats) ? d.seats : []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function mutate(path: string, body: Record<string, unknown>, after?: () => void) {
    setBusy(true);
    try {
      await postJSON(path, body);
      after?.();
      await reload();
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    void reload();
  }, []);

  const builtins = seats.filter((s) => s.builtin);
  const custom = seats.filter((s) => !s.builtin);

  return (
    <Page
      icon={Armchair}
      title="Seats"
      description="Execution presets a workboard task runs under — model tier, tool tier, and isolation surface."
      actions={
        <Button variant="ghost" size="sm" onClick={() => reload()} disabled={loading} title="Reload">
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
        </Button>
      }
    >
      <form
        className="flex flex-wrap items-end gap-2 rounded-lg border border-border bg-card/70 p-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (id.trim()) {
            mutate(
              "/api/seats/create",
              { id: id.trim(), name: name.trim(), description: desc.trim(), execution_profile: iso },
              () => {
                setId("");
                setName("");
                setDesc("");
                setIso("");
              },
            );
          }
        }}
      >
        <label className="flex flex-col gap-1 text-[11px] uppercase tracking-normal text-muted">
          id
          <input value={id} onChange={(e) => setId(e.target.value)} aria-label="Seat id" placeholder="gpu-box" className="h-8 w-32 rounded-md border border-border bg-panel px-2 text-xs text-foreground outline-none focus-visible:border-accent" />
        </label>
        <label className="flex flex-col gap-1 text-[11px] uppercase tracking-normal text-muted">
          name
          <input value={name} onChange={(e) => setName(e.target.value)} aria-label="Seat name" placeholder="GPU Box" className="h-8 w-32 rounded-md border border-border bg-panel px-2 text-xs text-foreground outline-none focus-visible:border-accent" />
        </label>
        <label className="flex min-w-40 flex-1 flex-col gap-1 text-[11px] uppercase tracking-normal text-muted">
          description
          <input value={desc} onChange={(e) => setDesc(e.target.value)} aria-label="Seat description" placeholder="what this seat is for" className="h-8 w-full rounded-md border border-border bg-panel px-2 text-xs text-foreground outline-none focus-visible:border-accent" />
        </label>
        <label className="flex flex-col gap-1 text-[11px] uppercase tracking-normal text-muted">
          isolation
          <select value={iso} onChange={(e) => setIso(e.target.value)} aria-label="Seat isolation" className="h-8 rounded-md border border-border bg-panel px-2 text-xs text-foreground outline-none focus-visible:border-accent">
            {ISO_OPTIONS.map((o) => (
              <option key={o || "none"} value={o}>{o || "tool defaults"}</option>
            ))}
          </select>
        </label>
        <Button type="submit" size="sm" disabled={busy || !id.trim()}>
          <Plus className="size-3.5" /> Add seat
        </Button>
      </form>

      {err && <ErrorText>{err}</ErrorText>}

      {loading && seats.length === 0 ? (
        <SkeletonList count={4} lines={2} />
      ) : (
        <div className="space-y-4">
          <SeatGroup title="Built-in" icon={Lock} seats={builtins} busy={busy} mutate={mutate} />
          {custom.length > 0 ? (
            <SeatGroup title="Custom" icon={Shield} seats={custom} busy={busy} mutate={mutate} />
          ) : (
            <EmptyState icon={Armchair} title="No custom seats" hint="Add one above — give it an id and an isolation surface, then pin it on a task with the seat picker." />
          )}
        </div>
      )}
    </Page>
  );
}

function SeatGroup({
  title,
  icon: Icon,
  seats,
  busy,
  mutate,
}: {
  title: string;
  icon: typeof Lock;
  seats: SeatDef[];
  busy: boolean;
  mutate: (path: string, body: Record<string, unknown>, after?: () => void) => void;
}) {
  if (seats.length === 0) return null;
  return (
    <section>
      <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-normal text-muted">
        <Icon className="size-3.5" /> {title}
      </div>
      <div className="grid gap-2">
        {seats.map((s) => (
          <article key={s.id} className="flex items-center justify-between gap-3 rounded-lg border border-border bg-card/75 p-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h3 className="truncate text-sm font-semibold">{s.name || s.id}</h3>
                <Badge variant={s.execution_profile?.trim() ? "accent" : "default"}>{seatIsoLabel(s)}</Badge>
                {s.restrict_tools && <Badge variant="warn">restricted tools</Badge>}
              </div>
              {s.description && <p className="mt-0.5 truncate text-xs text-muted">{s.description}</p>}
              <span className="font-mono text-[10px] text-muted">{s.id}</span>
            </div>
            {!s.builtin && (
              <Button size="sm" variant="ghost" disabled={busy} title="Remove seat" onClick={() => mutate("/api/seats/delete", { id: s.id })}>
                <Trash2 className="size-3.5" />
              </Button>
            )}
          </article>
        ))}
      </div>
    </section>
  );
}
