import { useRef, useState, type ReactNode } from "react";
import { Globe, Plus, RefreshCw, Pencil, Save, X, Download, Upload, Search, Tags, SlidersHorizontal, type LucideIcon } from "lucide-react";
import { Panel, Row, Count } from "@/components/Panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Muted } from "@/components/JsonView";
import { EmptyState } from "@/components/ui/empty";
import { ActionButton } from "@/components/ActionButton";
import { WorldGraph } from "@/components/WorldGraph";
import { BreakdownBar } from "@/components/Widgets";
import { postJSON, postAction } from "@/lib/api";
import { downloadText } from "@/lib/export";
import { cn } from "@/lib/utils";
import { useUI } from "@/components/ui/feedback";
import { Disclosure } from "@/components/ui/disclosure";

// The entity kinds the world model recognises — offered when teaching it one.
const WORLD_KINDS = ["person", "project", "repo", "org", "account", "device", "channel", "topic", "task"];
// The relation verbs the world model recognises.
const WORLD_VERBS = ["relates_to", "owns", "depends_on", "member_of", "prefers", "assigned_to", "derived_from"];

// WorldEntity is the client-side view of one entity node in the world knowledge graph.
export interface WorldEntity {
  id?: string;
  name: string;
  kind?: string;
  aliases?: string[];
  attrs?: Record<string, string>;
  weight?: number;
}

// kindBreakdown counts entities by kind for the breakdown bar + filter chips,
// sorted by count then name. Pure + unit-tested.
export function kindBreakdown(ents: WorldEntity[]): { label: string; count: number }[] {
  const c: Record<string, number> = {};
  for (const e of ents) c[e.kind || "entity"] = (c[e.kind || "entity"] || 0) + 1;
  return Object.entries(c)
    .map(([label, count]) => ({ label, count }))
    .sort((a, b) => (b.count !== a.count ? b.count - a.count : a.label.localeCompare(b.label)));
}

// filterEntities narrows the entity list by a free-text query (name/kind/alias,
// via entityMatches) and an optional exact kind (M918). Pure + unit-tested.
export function filterEntities(ents: WorldEntity[], query: string, kind: string): WorldEntity[] {
  const q = query.trim().toLowerCase();
  return ents.filter((e) => {
    if (kind && (e.kind || "entity") !== kind) return false;
    return entityMatches(e, q);
  });
}

// parseWorldJSON normalises an exported world file into re-addable entity
// (`world_add`) and relation (`world_relate`) arg lists. Accepts a {world:{entities,
// edges}} wrapper or the bare list shape ({entities, edges}). Entities keep
// name(+kind/aliases/attrs), dropping kernel id/weight/timestamps/lineage. Relations
// in the export reference entities by ID, so they're resolved back to names via the
// file's own entity list (falling back to the raw value, so a hand-written file using
// names also works). Throws on bad JSON / no valid entities.
export function parseWorldJSON(text: string): {
  entities: Record<string, unknown>[];
  relations: Record<string, unknown>[];
} {
  const data = JSON.parse(text);
  const obj = data && typeof data === "object" && !Array.isArray(data) ? (data as Record<string, unknown>) : null;
  const world =
    obj && obj.world && typeof obj.world === "object" && !Array.isArray(obj.world)
      ? (obj.world as Record<string, unknown>)
      : obj;
  const rawEnts = Array.isArray(world?.entities) ? (world!.entities as unknown[]) : null;
  if (!rawEnts) throw new Error("expected a world with an entities array ({entities:[…], edges:[…]})");

  const idToName = new Map<string, string>();
  const entities: Record<string, unknown>[] = [];
  for (const raw of rawEnts) {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) continue;
    const e = raw as Record<string, unknown>;
    const name = typeof e.name === "string" ? e.name.trim() : "";
    if (!name) continue;
    if (typeof e.id === "string" && e.id) idToName.set(e.id, name);
    const ent: Record<string, unknown> = { name };
    if (typeof e.kind === "string" && e.kind.trim()) ent.kind = e.kind.trim();
    if (Array.isArray(e.aliases)) {
      const al = (e.aliases as unknown[]).filter((a): a is string => typeof a === "string" && a.trim() !== "");
      if (al.length) ent.aliases = al;
    }
    if (e.attrs && typeof e.attrs === "object" && !Array.isArray(e.attrs)) {
      const attrs: Record<string, string> = {};
      for (const [k, v] of Object.entries(e.attrs as Record<string, unknown>)) if (typeof v === "string") attrs[k] = v;
      if (Object.keys(attrs).length) ent.attrs = attrs;
    }
    entities.push(ent);
  }
  if (entities.length === 0) throw new Error("no valid entities (each needs a name) found");

  const rawEdges = Array.isArray(world?.edges)
    ? (world!.edges as unknown[])
    : Array.isArray(world?.relations)
      ? (world!.relations as unknown[])
      : [];
  const relations: Record<string, unknown>[] = [];
  for (const raw of rawEdges) {
    if (!raw || typeof raw !== "object" || Array.isArray(raw)) continue;
    const r = raw as Record<string, unknown>;
    const fromRaw = typeof r.from === "string" ? r.from : "";
    const toRaw = typeof r.to === "string" ? r.to : "";
    const verb = typeof r.verb === "string" ? r.verb.trim() : "";
    const from = idToName.get(fromRaw) ?? fromRaw; // export stores ids → resolve to names
    const to = idToName.get(toRaw) ?? toRaw;
    if (!from || !to || !verb) continue;
    relations.push({ from, verb, to });
  }
  return { entities, relations };
}

// entityMatches tests an entity against a lowercased query over its name, kind, and
// aliases — so you can find a node in a large graph by any of the ways you might know it.
export function entityMatches(e: WorldEntity, q: string): boolean {
  if (!q) return true;
  const hay = [e.name, e.kind, ...(Array.isArray(e.aliases) ? e.aliases : [])]
    .filter((s) => typeof s === "string")
    .join(" ")
    .toLowerCase();
  return hay.includes(q);
}

export function World() {
  const ui = useUI();
  const fileRef = useRef<HTMLInputElement>(null);
  const [q, setQ] = useState("");
  const [kindFilter, setKindFilter] = useState("");
  const [addOpen, setAddOpen] = useState(false);
  const [relateOpen, setRelateOpen] = useState(false);
  return (
    <Panel<Record<string, any>> title="World" icon={Globe} description="The agents' shared model of entities and how they relate" path="/api/world">
      {(d, reload) => {
        const ents = d.entities || [];
        const edges = d.edges || [];
        const query = q.trim().toLowerCase();
        const breakdown = kindBreakdown(ents);
        const shown = filterEntities(ents, query, kindFilter);
        const rels = d.relations ?? d.relation_count ?? edges.length;
        // Relations store their endpoints by entity id; resolve to names for display.
        const nameById: Record<string, string> = {};
        for (const e of ents) if (e.id) nameById[e.id] = e.name;

        function exportWorld() {
          downloadText(
            "agezt-world.json",
            JSON.stringify({ version: 1, world: { entities: ents, edges } }, null, 2),
            "application/json",
          );
        }

        // Restore a world: re-add entities (world_add), then relations (world_relate).
        // Both are content-addressed (entity by kind+name, relation by from/verb/to), so
        // re-importing a world the agent already knows dedupes — import is idempotent.
        async function importWorld(file: File) {
          try {
            const { entities, relations } = parseWorldJSON(await file.text());
            let ne = 0;
            for (const e of entities) {
              try {
                await postJSON("/api/world/add", e);
                ne++;
              } catch {
                /* skip one the daemon rejects; keep going */
              }
            }
            let nr = 0;
            for (const r of relations) {
              try {
                await postAction("/api/world/relate", r as Record<string, string>);
                nr++;
              } catch {
                /* a relation whose endpoints didn't resolve is skipped */
              }
            }
            const relPart = relations.length ? ` + ${nr}/${relations.length} relation${relations.length === 1 ? "" : "s"}` : "";
            ui.toast(
              `Imported ${ne}/${entities.length} entit${entities.length === 1 ? "y" : "ies"}${relPart}`,
              ne || nr ? "success" : "error",
            );
            void reload();
          } catch (e) {
            ui.toast(`Import failed: ${(e as Error).message}`, "error");
          }
        }

        return (
          <>
            <div className="flex items-center gap-2">
              <Count>
                {ents.length} entities · {rels} relations
              </Count>
              <input
                ref={fileRef}
                type="file"
                accept="application/json,.json"
                className="hidden"
                aria-hidden="true"
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) void importWorld(f);
                  e.target.value = "";
                }}
              />
              <Button variant="ghost" size="sm" className="ml-auto" onClick={() => fileRef.current?.click()} title="Import a world from a file">
                <Upload className="size-3.5" /> Import
              </Button>
              <Button variant="ghost" size="sm" onClick={exportWorld} disabled={ents.length === 0} title="Export the world to a file">
                <Download className="size-3.5" /> Export
              </Button>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button size="sm" onClick={() => setAddOpen(true)} title="Teach the world model a new entity">
                <Plus className="size-3.5" /> Add entity
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setRelateOpen(true)} disabled={ents.length < 2} title="Connect two known entities">
                <Globe className="size-3.5" /> Relate
              </Button>
            </div>
            {addOpen && (
              <WorldModal title="Add entity" onClose={() => setAddOpen(false)}>
                <WorldAddForm
                  onAdded={() => {
                    setAddOpen(false);
                    reload();
                  }}
                />
              </WorldModal>
            )}
            {relateOpen && (
              <WorldModal title="Relate entities" onClose={() => setRelateOpen(false)}>
                <WorldRelateForm
                  names={ents.map((e: any) => e.name).filter(Boolean)}
                  onRelated={() => {
                    setRelateOpen(false);
                    reload();
                  }}
                />
              </WorldModal>
            )}

            {ents.length > 0 && <BreakdownBar segments={breakdown} />}
            {/* Kind filter chips (M918): click a kind to narrow the entity list —
                complements the free-text search below. */}
            {breakdown.length > 1 && (
              <div className="flex flex-wrap gap-1.5">
                <KindChip label="all" n={ents.length} active={kindFilter === ""} onClick={() => setKindFilter("")} />
                {breakdown.map((b) => (
                  <KindChip
                    key={b.label}
                    label={b.label}
                    n={b.count}
                    active={kindFilter === b.label}
                    onClick={() => setKindFilter(kindFilter === b.label ? "" : b.label)}
                  />
                ))}
              </div>
            )}
            {ents.length >= 2 && (
              <div className="h-72 overflow-hidden rounded-md border border-border bg-panel">
                <WorldGraph entities={ents} edges={edges} />
              </div>
            )}
            {edges.length > 0 && (
              <div className="space-y-1">
                <div className="text-[11px] font-semibold uppercase tracking-normal text-muted">Relations ({edges.length})</div>
                {edges.map((r: any, i: number) => {
                  const from = nameById[r.from] || r.from;
                  const to = nameById[r.to] || r.to;
                  return (
                    <div key={r.id || i} className="flex items-center gap-2 rounded-md border border-border/60 bg-panel/40 px-2.5 py-1 text-xs">
                      <span className="font-medium text-foreground/80">{from}</span>
                      <span className="font-mono text-xs text-accent">{r.verb}</span>
                      <span className="font-medium text-foreground/80">{to}</span>
                      <div className="ml-auto">
                        <ActionButton
                          label="forget"
                          variant="danger"
                          path="/api/world/forget"
                          params={{ id: r.id }}
                          onDone={reload}
                          confirm={{
                            title: "Forget this relation?",
                            message: `“${from} ${r.verb} ${to}” will be removed from the world model.`,
                            confirmLabel: "Forget",
                            danger: true,
                          }}
                          success="Relation forgotten"
                        />
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
            {/* Find an entity in a large graph by name, kind, or alias (M774). */}
            {ents.length > 4 && (
              <div className="relative">
                <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted" />
                <input
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                  placeholder="filter entities..."
                  aria-label="Filter entities"
                  className="h-8 w-full rounded-md border border-border bg-panel pl-7 pr-2 text-xs text-foreground outline-none focus-visible:border-accent"
                />
                {query && (
                  <span className="absolute right-2 top-1/2 -translate-y-1/2 text-xs text-muted">
                    {shown.length}/{ents.length}
                  </span>
                )}
              </div>
            )}
            {ents.length === 0 ? (
              <EmptyState
                icon={Globe}
                title="No entities yet"
                hint="The agent builds its world model as it learns about people, projects and systems — it'll fill in here."
              />
            ) : shown.length === 0 ? (
              <Muted>
                no entities match{q.trim() ? ` “${q.trim()}”` : ""}
                {kindFilter ? ` in ${kindFilter}` : ""}
              </Muted>
            ) : (
              shown.map((e: any, i: number) => <EntityRow key={e.id || i} entity={e} onChanged={reload} />)
            )}
          </>
        );
      }}
    </Panel>
  );
}

// EntityRow renders one world entity with its forget control and an Edit (pencil)
// that reveals an inline aliases/attrs editor (M730). Each row owns its own edit
// state so opening one doesn't disturb the others.
function KindChip({ label, n, active, onClick }: { label: string; n: number; active: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] transition-colors",
        active ? "border-accent bg-accent/10 text-accent" : "border-border text-muted hover:border-accent",
      )}
    >
      <span>{label}</span>
      <span className="rounded-full bg-card px-1 text-xs tabular-nums">{n}</span>
    </button>
  );
}

function EntityRow({ entity, onChanged }: { entity: any; onChanged: () => void }) {
  const [editing, setEditing] = useState(false);
  return (
    <>
      <Row>
        <Badge>{entity.kind || ""}</Badge>
        <span>{entity.name || ""}</span>
        {entity.weight != null ? <Muted>w={entity.weight}</Muted> : null}
        {entity.id ? (
          <span className="ml-auto flex items-center gap-1.5">
            <button
              onClick={() => setEditing((v) => !v)}
              title={editing ? "Close editor" : "Edit aliases & attributes"}
              className={editing ? "text-accent" : "text-muted transition-colors hover:text-accent"}
            >
              <Pencil className="size-3.5" />
            </button>
            <ActionButton
              label="forget"
              variant="danger"
              path="/api/world/forget"
              params={{ id: entity.id }}
              onDone={onChanged}
              confirm={{
                title: "Forget this entity?",
                message: entity.name
                  ? `“${entity.name}” and its relations will be permanently removed from the world model.`
                  : "This entity and its relations will be permanently removed from the world model.",
                confirmLabel: "Forget",
                danger: true,
              }}
              success="Entity forgotten"
            />
          </span>
        ) : null}
      </Row>
      {editing && entity.id && (
        <WorldModal title={`Edit ${entity.name || "entity"}`} onClose={() => setEditing(false)}>
          <WorldEditForm
            entity={entity}
            onSaved={() => {
              setEditing(false);
              onChanged();
            }}
          />
        </WorldModal>
      )}
    </>
  );
}

function WorldModal({ title, children, onClose }: { title: string; children: ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-bg/75 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={title}>
      <div className="glass flex max-h-[86vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-accent/25 shadow-e3">
        <div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent/12 text-accent">
            <Pencil className="size-4" />
          </span>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-foreground">{title}</h2>
            <p className="text-xs text-muted">Tune aliases and attributes without cluttering the entity list.</p>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} className="ml-auto" aria-label="Close world modal">
            <X className="size-4" />
          </Button>
        </div>
        <div className="min-h-0 overflow-auto p-4">{children}</div>
      </div>
    </div>
  );
}

function WorldFormBlock({
  icon: Icon,
  title,
  meta,
  children,
  defaultOpen = false,
}: {
  icon: LucideIcon;
  title: string;
  meta: string;
  children: ReactNode;
  defaultOpen?: boolean;
}) {
  return (
    <Disclosure
      defaultOpen={defaultOpen}
      className="rounded-lg border border-border bg-panel/45"
      summaryClassName="px-2.5 py-2"
      contentClassName="px-2.5 pb-2"
      summary={
        <span className="flex min-w-0 items-center gap-2">
          <span className="grid size-7 shrink-0 place-items-center rounded-md border border-border bg-background/70 text-accent">
            <Icon className="size-3.5" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-semibold text-foreground">{title}</span>
            <span className="block truncate text-[11px] font-normal text-muted">{meta}</span>
          </span>
        </span>
      }
    >
      {children}
    </Disclosure>
  );
}

// WorldEditForm edits an entity's aliases and attributes in place (M730) — the
// "tune what the agent knows about X" surface. Identity (kind/name) is content-
// addressed and immutable, so it's read-only here; aliases (comma-separated) and
// attrs (key/value rows, add/remove) are the full editable state and post to
// world_edit, which REPLACES them (so removing one sticks, unlike a re-add).
export function WorldEditForm({ entity, onSaved }: { entity: any; onSaved: () => void }) {
  const { toast } = useUI();
  const [aliases, setAliases] = useState<string>(((entity.aliases as string[]) || []).join(", "));
  const [pairs, setPairs] = useState<{ k: string; v: string }[]>(
    Object.entries((entity.attrs as Record<string, string>) || {}).map(([k, v]) => ({ k, v: String(v) })),
  );
  const [submitting, setSubmitting] = useState(false);

  const setPair = (i: number, field: "k" | "v", val: string) =>
    setPairs((p) => p.map((pair, j) => (j === i ? { ...pair, [field]: val } : pair)));
  const addPair = () => setPairs((p) => [...p, { k: "", v: "" }]);
  const removePair = (i: number) => setPairs((p) => p.filter((_, j) => j !== i));

  async function save() {
    const attrs: Record<string, string> = {};
    for (const { k, v } of pairs) {
      const kk = k.trim();
      const vv = v.trim();
      if (kk && vv) attrs[kk] = vv;
    }
    const aliasList = aliases
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    setSubmitting(true);
    try {
      await postJSON("/api/world/edit", { id: entity.id, aliases: aliasList, attrs });
      toast(`Updated “${entity.name || entity.id}”`, "success");
      onSaved();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-md border border-border/70 bg-panel/70 p-2.5">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <div className="grid size-8 place-items-center rounded-lg border border-accent/25 bg-accent/10 text-accent">
            <Globe className="size-4" />
          </div>
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-foreground">{entity.name || entity.id || "Entity"}</div>
            <div className="text-[11px] text-muted">{pairs.length} attribute{pairs.length === 1 ? "" : "s"} · {aliases.trim() ? "aliases set" : "no aliases"}</div>
          </div>
        </div>
        <Badge variant={pairs.length > 0 || aliases.trim() ? "accent" : "default"}>{pairs.length + aliases.split(",").filter((x) => x.trim()).length} facts</Badge>
      </div>

      <div className="space-y-2">
        <WorldFormBlock icon={Tags} title="Aliases" meta={aliases.trim() || "no aliases"} defaultOpen>
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Aliases (comma-separated)
            <input
              value={aliases}
              onChange={(e) => setAliases(e.target.value)}
              placeholder="e.g. the portfolio, the repos"
              aria-label="Edit entity aliases"
              className="h-8 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
            />
          </label>
        </WorldFormBlock>

        <WorldFormBlock icon={SlidersHorizontal} title="Attributes" meta={pairs.length ? `${pairs.length} configured` : "none"} defaultOpen={pairs.length > 0}>
          <div className="flex flex-col gap-1 text-[11px] text-muted">
            {pairs.length === 0 && <span className="italic opacity-70">none - add a preference or constraint</span>}
            {pairs.map((p, i) => (
              <div key={i} className="flex items-center gap-1.5">
                <input
                  value={p.k}
                  onChange={(e) => setPair(i, "k", e.target.value)}
                  placeholder="key"
                  aria-label={`Attribute key ${i + 1}`}
                  className="h-8 w-32 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
                />
                <input
                  value={p.v}
                  onChange={(e) => setPair(i, "v", e.target.value)}
                  placeholder="value"
                  aria-label={`Attribute value ${i + 1}`}
                  className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
                />
                <button
                  onClick={() => removePair(i)}
                  title="Remove attribute"
                  className="text-muted transition-colors hover:text-bad"
                >
                  <X className="size-3.5" />
                </button>
              </div>
            ))}
            <button
              onClick={addPair}
              className="mt-0.5 inline-flex w-fit items-center gap-1 text-[11px] text-accent transition-opacity hover:opacity-80"
            >
              <Plus className="size-3" /> add attribute
            </button>
          </div>
        </WorldFormBlock>
      </div>

      <div className="mt-2 flex items-center justify-end">
        <Button size="sm" onClick={save} disabled={submitting} title="Save aliases & attributes">
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Save className="size-3.5" />} Save
        </Button>
      </div>
    </div>
  );
}

// WorldAddForm teaches the agent about an entity — a person, project, device, … —
// from the UI (M721), the write side of the world model. Name + kind post to
// world_add; the entity then participates in recall and relations like any other.
export function WorldAddForm({ onAdded }: { onAdded: () => void }) {
  const { toast } = useUI();
  const [name, setName] = useState("");
  const [kind, setKind] = useState("person");
  const [submitting, setSubmitting] = useState(false);
  const valid = name.trim() !== "";

  async function add() {
    if (!valid) return;
    setSubmitting(true);
    try {
      await postJSON("/api/world/add", { name: name.trim(), kind });
      toast(`Added ${kind} “${name.trim()}”`, "success");
      setName("");
      onAdded();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-md border border-border/70 bg-panel/70 p-2.5">
      <WorldChipPicker value={kind} onChange={setKind} options={WORLD_KINDS} label="Entity kind" />
      <input
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") add();
        }}
        placeholder="Name (e.g. Acme Corp, my-repo, Ada)"
        aria-label="Entity name"
        className="mt-2 h-8 w-full min-w-0 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
      />
      <div className="mt-2 flex justify-end">
        <Button size="sm" onClick={add} disabled={!valid || submitting} title="Add entity">
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Add entity
        </Button>
      </div>
    </div>
  );
}

// WorldRelateForm connects two entities with a verb (M722) — building the knowledge
// GRAPH from the UI, not just its nodes. from/verb/to post to world_relate.
export function WorldRelateForm({ names, onRelated }: { names: string[]; onRelated: () => void }) {
  const { toast } = useUI();
  const [from, setFrom] = useState(names[0] || "");
  const [verb, setVerb] = useState("relates_to");
  const [to, setTo] = useState(names[1] || "");
  const [submitting, setSubmitting] = useState(false);
  const valid = from !== "" && to !== "" && from !== to;

  async function relate() {
    if (!valid) return;
    setSubmitting(true);
    try {
      await postAction("/api/world/relate", { from, verb, to });
      toast(`${from} ${verb.replace(/_/g, " ")} ${to}`, "success");
      onRelated();
    } catch (e) {
      toast((e as Error).message, "error");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="space-y-2 rounded-md border border-border/70 bg-panel/70 p-2.5 text-xs">
      <WorldChipPicker value={from} onChange={setFrom} options={names} label="Relation from" />
      <WorldChipPicker value={verb} onChange={setVerb} options={WORLD_VERBS} label="Relation verb" format={(v) => v.replace(/_/g, " ")} />
      <WorldChipPicker value={to} onChange={setTo} options={names} label="Relation to" />
      <div className="flex justify-end">
        <Button size="sm" variant="ghost" onClick={relate} disabled={!valid || submitting} title="Relate entities">
          {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Relate
        </Button>
      </div>
    </div>
  );
}

function WorldChipPicker({
  value,
  onChange,
  options,
  label,
  format = (v) => v,
}: {
  value: string;
  onChange: (value: string) => void;
  options: string[];
  label: string;
  format?: (value: string) => string;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label={label}>
      {options.map((option) => {
        const selected = value === option;
        return (
          <button
            key={option}
            type="button"
            aria-pressed={selected}
            onClick={() => onChange(option)}
            className={cn(
              "inline-flex h-7 max-w-full items-center rounded-md border px-2 text-xs font-medium transition-colors",
              selected
                ? "border-accent bg-accent/15 text-accent"
                : "border-border bg-panel text-muted hover:border-accent/60 hover:text-foreground",
            )}
          >
            <span className="truncate">{format(option)}</span>
          </button>
        );
      })}
    </div>
  );
}
