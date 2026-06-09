import { describe, it, expect, vi, beforeEach } from "vitest";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));

import { fetchFullSnapshot, snapshotCounts } from "@/lib/snapshot";

beforeEach(() => getJSON.mockReset());

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
