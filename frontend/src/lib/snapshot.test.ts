// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { fetchFullSnapshot, snapshotCounts, parseSnapshotJSON, applyFullSnapshot } from "@/lib/snapshot";

beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postJSON.mockResolvedValue({});
  postAction.mockReset();
  postAction.mockResolvedValue({});
});

describe("snapshotCounts", () => {
  it("summarises every section", () => {
    const s = {
      version: 1,
      exported_note: "",
      config: { persona: "hi", prompts: [{}, {}], chains: { chat: ["a"] } },
      standing: [{}],
      schedules: [{}, {}, {}],
      memory: [{}, {}],
      world: { entities: [{}], relations: [] },
    };
    expect(snapshotCounts(s)).toBe("persona · 2 prompts · 1 chains · 1 standing · 3 schedules · 2 memories · 1 entities");
  });

  it("reports 'no persona' when blank", () => {
    const s = {
      version: 1,
      exported_note: "",
      config: { persona: "   ", prompts: [], chains: {} },
      standing: [],
      schedules: [],
      memory: [],
      world: { entities: [], relations: [] },
    };
    expect(snapshotCounts(s)).toContain("no persona");
  });
});

describe("fetchFullSnapshot", () => {
  it("gathers every read endpoint into one record", async () => {
    getJSON.mockImplementation((p: string) => {
      const m: Record<string, unknown> = {
        "/api/persona": { system: "be terse" },
        "/api/prompts": { prompts: [{ title: "t", text: "x" }] },
        "/api/routing": { chains: { chat: ["a"] } },
        "/api/standing": { orders: [{ id: "o1" }] },
        "/api/schedules": { schedules: [{ id: "s1" }] },
        "/api/memory": { records: [{ id: "m1" }] },
        "/api/world": { entities: [{ id: "e1" }], edges: [{ id: "r1" }] },
      };
      return Promise.resolve(m[p] ?? {});
    });
    const snap = await fetchFullSnapshot();
    expect(snap.config.persona).toBe("be terse");
    expect(snap.standing).toEqual([{ id: "o1" }]);
    expect(snap.schedules).toEqual([{ id: "s1" }]);
    expect(snap.memory).toEqual([{ id: "m1" }]);
    expect(snap.world.entities).toEqual([{ id: "e1" }]);
    expect(snap.world.relations).toEqual([{ id: "r1" }]); // falls back to edges
  });

  it("degrades a failing section to empty rather than failing the whole export", async () => {
    getJSON.mockImplementation((p: string) => {
      if (p === "/api/standing") return Promise.reject(new Error("boom"));
      if (p === "/api/persona") return Promise.resolve({ system: "ok" });
      return Promise.resolve({});
    });
    const snap = await fetchFullSnapshot();
    expect(snap.config.persona).toBe("ok");
    expect(snap.standing).toEqual([]); // failed → empty, not a throw
  });
});

describe("parseSnapshotJSON (M752)", () => {
  it("normalises a full snapshot, tolerating missing sections", () => {
    const snap = parseSnapshotJSON(
      JSON.stringify({
        version: 1,
        config: { persona: "be terse", prompts: [{ title: "t", text: "x" }], chains: { chat: ["a"] } },
        standing: [{ name: "o" }],
        // schedules omitted entirely → defaults to []
        memory: [{ content: "m" }],
        world: { entities: [{ name: "e" }], edges: [{ from: "e", verb: "v", to: "e" }] },
      }),
    );
    expect(snap.config.persona).toBe("be terse");
    expect(snap.schedules).toEqual([]);
    expect(snap.world.relations).toEqual([{ from: "e", verb: "v", to: "e" }]); // edges → relations
  });

  it("throws on bad JSON, a non-object, or an empty snapshot", () => {
    expect(() => parseSnapshotJSON("nope")).toThrow();
    expect(() => parseSnapshotJSON("[]")).toThrow(/expected a snapshot object/);
    expect(() =>
      parseSnapshotJSON(JSON.stringify({ config: { persona: "", prompts: [], chains: {} }, standing: [], schedules: [], memory: [], world: { entities: [] } })),
    ).toThrow(/no restorable content/);
  });
});

describe("applyFullSnapshot (M752)", () => {
  const snap = {
    version: 1,
    exported_note: "",
    config: { persona: "be terse", prompts: [{ title: "t", text: "x" }], chains: { chat: ["a"] } },
    standing: [{ id: "OLD", name: "watch", triggers: [{ type: "cron", schedule: "0 8 * * *" }] }],
    schedules: [{ id: "S", intent: "ping", mode: "", interval_sec: 900 }],
    memory: [{ id: "M", content: "the owner is in Istanbul", type: "FACT" }],
    world: {
      entities: [{ id: "e-a", kind: "person", name: "Alice" }, { id: "e-b", kind: "project", name: "AGEZT" }],
      relations: [{ id: "r", from: "e-a", verb: "owns", to: "e-b" }],
    },
  };

  it("replays each section through the per-domain endpoints and summarises", async () => {
    const applied = await applyFullSnapshot(snap);

    // config via the bundle's /set calls
    expect(postJSON).toHaveBeenCalledWith("/api/persona/set", { system: "be terse" });
    expect(postJSON).toHaveBeenCalledWith("/api/prompts/set", { prompts: [{ title: "t", text: "x" }] });
    expect(postJSON).toHaveBeenCalledWith("/api/routing/set", { chains: { chat: ["a"] } });
    // standing add (id stripped → re-add mints fresh)
    expect(postJSON).toHaveBeenCalledWith("/api/standing/add", {
      order: { name: "watch", triggers: [{ type: "cron", schedule: "0 8 * * *" }] },
    });
    // schedule add (interval rebuilt)
    expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", { intent: "ping", interval_sec: 900 });
    // memory add
    expect(postJSON).toHaveBeenCalledWith("/api/memory/add", { content: "the owner is in Istanbul", type: "FACT" });
    // world: entities then a relation resolved id→name
    expect(postJSON).toHaveBeenCalledWith("/api/world/add", { name: "Alice", kind: "person" });
    expect(postAction).toHaveBeenCalledWith("/api/world/relate", { from: "Alice", verb: "owns", to: "AGEZT" });

    expect(applied).toEqual([
      "config (persona+prompts+routing)",
      "1/1 standing",
      "1/1 schedules",
      "1/1 memories",
      "2/2 entities + 1 relations",
    ]);
  });

  it("skips empty autonomy/knowledge sections without throwing (config still replaces faithfully)", async () => {
    const applied = await applyFullSnapshot({
      version: 1,
      exported_note: "",
      config: { persona: "only persona", prompts: [], chains: {} },
      standing: [],
      schedules: [],
      memory: [],
      world: { entities: [], relations: [] },
    });
    // Config replaces all three sections (empty prompts/chains restore as empty — faithful);
    // the empty autonomy/knowledge sections produce no add calls and no summary entries.
    expect(applied).toEqual(["config (persona+prompts+routing)"]);
    expect(postJSON).not.toHaveBeenCalledWith("/api/standing/add", expect.anything());
    expect(postJSON).not.toHaveBeenCalledWith("/api/memory/add", expect.anything());
  });
});
