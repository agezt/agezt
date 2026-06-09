import { useRef, useState } from "react";
import { Globe, Plus, RefreshCw, Pencil, Save, X, Download, Upload } from "lucide-react";
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
import { useUI } from "@/components/ui/feedback";

// The entity kinds the world model recognises — offered when teaching it one.
const WORLD_KINDS = ["person", "project", "repo", "org", "account", "device", "channel", "topic", "task"];
// The relation verbs the world model recognises.
const WORLD_VERBS = ["relates_to", "owns", "depends_on", "member_of", "prefers", "assigned_to", "derived_from"];

// kindBreakdown counts entities by kind for the breakdown bar.
function kindBreakdown(ents: any[]): { label: string; count: number }[] {
  const c: Record<string, number> = {};
  for (const e of ents) c[e.kind || "entity"] = (c[e.kind || "entity"] || 0) + 1;
  return Object.entries(c).map(([label, count]) => ({ label, count }));
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

export function World() {
  const ui = useUI();
  const fileRef = useRef<HTMLInputElement>(null);
  return (
    <Panel<Record<string, any>> title="World" path="/api/world">
      {(d, reload) => {
        const ents = d.entities || [];
        const edges = d.edges || [];
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
            <WorldAddForm onAdded={reload} />
            {ents.length >= 2 && <WorldRelateForm names={ents.map((e: any) => e.name).filter(Boolean)} onRelated={reload} />}

            {ents.length > 0 && <BreakdownBar segments={kindBreakdown(ents)} />}
            {ents.length >= 2 && (
              <div className="h-72 overflow-hidden rounded-md border border-border bg-panel">
                <WorldGraph entities={ents} edges={edges} />
              </div>
            )}
            {edges.length > 0 && (
              <div className="space-y-1">
                <div className="text-[11px] font-semibold uppercase tracking-wider text-muted">Relations ({edges.length})</div>
                {edges.map((r: any, i: number) => {
                  const from = nameById[r.from] || r.from;
                  const to = nameById[r.to] || r.to;
                  return (
                    <div key={r.id || i} className="flex items-center gap-2 rounded-md border border-border/60 bg-panel/40 px-2.5 py-1 text-xs">
                      <span className="font-medium text-foreground/80">{from}</span>
                      <span className="font-mono text-[10px] text-accent">{r.verb}</span>
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
            {ents.length ? (
              ents.map((e: any, i: number) => <EntityRow key={e.id || i} entity={e} onChanged={reload} />)
            ) : (
              <EmptyState
                icon={Globe}
                title="No entities yet"
                hint="The agent builds its world model as it learns about people, projects and systems — it'll fill in here."
              />
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
        <WorldEditForm
          entity={entity}
          onSaved={() => {
            setEditing(false);
            onChanged();
          }}
        />
      )}
    </>
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
    <div className="mb-2 ml-2 rounded-md border border-accent/30 bg-card p-2.5">
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

      <div className="mt-2 flex flex-col gap-1 text-[11px] text-muted">
        Attributes
        {pairs.length === 0 && <span className="italic opacity-70">none — add a preference or constraint</span>}
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
    <div className="mb-2 flex flex-wrap items-center gap-1.5">
      <select
        value={kind}
        onChange={(e) => setKind(e.target.value)}
        aria-label="Entity kind"
        className="h-8 rounded-md border border-border bg-panel px-1.5 text-sm outline-none focus-visible:border-accent"
      >
        {WORLD_KINDS.map((k) => (
          <option key={k} value={k}>
            {k}
          </option>
        ))}
      </select>
      <input
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") add();
        }}
        placeholder="Name (e.g. Acme Corp, my-repo, Ada)"
        aria-label="Entity name"
        className="h-8 min-w-0 flex-1 rounded-md border border-border bg-panel px-2 text-sm outline-none focus-visible:border-accent"
      />
      <Button size="sm" onClick={add} disabled={!valid || submitting} title="Add entity">
        {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Add entity
      </Button>
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
    <div className="mb-2 flex flex-wrap items-center gap-1.5 text-xs">
      <span className="text-muted">relate</span>
      <select value={from} onChange={(e) => setFrom(e.target.value)} aria-label="Relation from" className="h-8 min-w-0 max-w-[10rem] rounded-md border border-border bg-panel px-1.5 outline-none focus-visible:border-accent">
        {names.map((n) => (
          <option key={n} value={n}>{n}</option>
        ))}
      </select>
      <select value={verb} onChange={(e) => setVerb(e.target.value)} aria-label="Relation verb" className="h-8 rounded-md border border-border bg-panel px-1.5 outline-none focus-visible:border-accent">
        {WORLD_VERBS.map((v) => (
          <option key={v} value={v}>{v.replace(/_/g, " ")}</option>
        ))}
      </select>
      <select value={to} onChange={(e) => setTo(e.target.value)} aria-label="Relation to" className="h-8 min-w-0 max-w-[10rem] rounded-md border border-border bg-panel px-1.5 outline-none focus-visible:border-accent">
        {names.map((n) => (
          <option key={n} value={n}>{n}</option>
        ))}
      </select>
      <Button size="sm" variant="ghost" onClick={relate} disabled={!valid || submitting} title="Relate entities">
        {submitting ? <RefreshCw className="size-3.5 animate-spin" /> : <Plus className="size-3.5" />} Relate
      </Button>
    </div>
  );
}
