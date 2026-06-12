// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import {
  World,
  WorldAddForm,
  WorldRelateForm,
  WorldEditForm,
  parseWorldJSON,
  kindBreakdown,
  filterEntities,
} from "@/views/World";
import { UIProvider } from "@/components/ui/feedback";

describe("kindBreakdown (M918)", () => {
  it("counts entities per kind, sorted by count then name, defaulting blank to 'entity'", () => {
    const ents = [{ kind: "person" }, { kind: "person" }, { kind: "project" }, {}];
    expect(kindBreakdown(ents)).toEqual([
      { label: "person", count: 2 },
      { label: "entity", count: 1 },
      { label: "project", count: 1 },
    ]);
  });
});

describe("filterEntities (M918)", () => {
  const ents = [
    { name: "Ada", kind: "person", aliases: ["Countess"] },
    { name: "Babbage", kind: "person" },
    { name: "AnalyticalEngine", kind: "project" },
  ];
  it("narrows by exact kind", () => {
    expect(filterEntities(ents, "", "project").map((e) => e.name)).toEqual(["AnalyticalEngine"]);
  });
  it("composes the kind filter with the text query (name/kind/alias)", () => {
    expect(filterEntities(ents, "countess", "").map((e) => e.name)).toEqual(["Ada"]);
    expect(filterEntities(ents, "ada", "project")).toHaveLength(0);
    expect(filterEntities(ents, "", "").map((e) => e.name)).toEqual(["Ada", "Babbage", "AnalyticalEngine"]);
  });
});

// WorldGraph (React Flow / @xyflow) needs ResizeObserver, which jsdom lacks.
globalThis.ResizeObserver ||= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
  postJSON.mockResolvedValue({ id: "ent-1" });
  postAction.mockReset();
  postAction.mockResolvedValue({ id: "rel-1" });
});

describe("World relations list (M766)", () => {
  it("lists relations with id→name-resolved endpoints and forgets one via /api/world/forget", async () => {
    getJSON.mockResolvedValue({
      entities: [
        { id: "e1", name: "Alice", kind: "person" },
        { id: "e2", name: "AGEZT", kind: "project" },
      ],
      edges: [{ id: "r1", from: "e1", verb: "owns", to: "e2" }],
      relation_count: 1,
    });
    postAction.mockResolvedValue({ forgotten: true });
    render(withUI(<World />));

    await waitFor(() => expect(screen.getByText("Relations (1)")).toBeTruthy());
    // The edge's entity ids resolved to names (the relation row shows the verb between
    // them); "owns" also appears in the relate-form dropdown, so assert via the list label.
    expect(screen.getAllByText("AGEZT").length).toBeGreaterThan(0);

    // The relations list renders before the entity rows, so the first "forget" is the
    // relation's. Clicking it opens the confirm modal; confirming posts world/forget.
    fireEvent.click(screen.getAllByRole("button", { name: "forget" })[0]);
    await waitFor(() => expect(screen.getByRole("button", { name: "Forget" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Forget" }));
    await waitFor(() => expect(postAction).toHaveBeenCalledWith("/api/world/forget", { id: "r1" }));
  });
});

describe("parseWorldJSON (M751)", () => {
  const exported = {
    version: 1,
    world: {
      entities: [
        { id: "e-alice", kind: "person", name: "Alice", weight: 3, created_ms: 1, aliases: ["al"], attrs: { role: "owner" } },
        { id: "e-proj", kind: "project", name: "AGEZT", last_seen_ms: 9 },
        { id: "e-empty", name: "  " }, // nameless → dropped
      ],
      edges: [
        { id: "r-1", from: "e-alice", verb: "owns", to: "e-proj", weight: 2 },
        { id: "r-2", from: "e-alice", verb: "missing_target", to: "e-ghost" }, // unresolved target → kept as raw id
      ],
    },
  };

  it("reads the {world:{…}} wrapper and the bare {entities,edges} shape", () => {
    expect(parseWorldJSON(JSON.stringify(exported)).entities).toHaveLength(2);
    expect(parseWorldJSON(JSON.stringify(exported.world)).entities).toHaveLength(2);
  });

  it("keeps name/kind/aliases/attrs and drops kernel id/weight/timestamps", () => {
    const { entities } = parseWorldJSON(JSON.stringify(exported));
    expect(entities[0]).toEqual({ name: "Alice", kind: "person", aliases: ["al"], attrs: { role: "owner" } });
    expect(entities[0]).not.toHaveProperty("id");
    expect(entities[0]).not.toHaveProperty("weight");
    expect(entities[1]).toEqual({ name: "AGEZT", kind: "project" });
  });

  it("resolves relation endpoints from entity ids back to names", () => {
    const { relations } = parseWorldJSON(JSON.stringify(exported));
    // First edge resolves both ids → names; second keeps the raw id for the unknown target.
    expect(relations[0]).toEqual({ from: "Alice", verb: "owns", to: "AGEZT" });
    expect(relations[1]).toEqual({ from: "Alice", verb: "missing_target", to: "e-ghost" });
  });

  it("treats from/to as names when no matching id exists (hand-written file)", () => {
    const { relations } = parseWorldJSON(
      JSON.stringify({ entities: [{ name: "Bob" }, { name: "Carol" }], edges: [{ from: "Bob", verb: "knows", to: "Carol" }] }),
    );
    expect(relations[0]).toEqual({ from: "Bob", verb: "knows", to: "Carol" });
  });

  it("throws on invalid JSON, a shape with no entities array, or no valid entities", () => {
    expect(() => parseWorldJSON("nope")).toThrow();
    expect(() => parseWorldJSON('{"foo":1}')).toThrow(/entities array/);
    expect(() => parseWorldJSON('{"entities":[{}]}')).toThrow(/no valid entities/);
  });
});

describe("WorldAddForm", () => {
  it("disables Add until a name is entered", () => {
    render(withUI(<WorldAddForm onAdded={() => {}} />));
    expect((screen.getByRole("button", { name: /Add entity/ }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Entity name"), { target: { value: "Acme" } });
    expect((screen.getByRole("button", { name: /Add entity/ }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("posts name + default kind (person), trimmed, and calls onAdded", async () => {
    const onAdded = vi.fn();
    render(withUI(<WorldAddForm onAdded={onAdded} />));
    fireEvent.change(screen.getByLabelText("Entity name"), { target: { value: "  Ada Lovelace  " } });
    fireEvent.click(screen.getByRole("button", { name: /Add entity/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/world/add", { name: "Ada Lovelace", kind: "person" }));
    await waitFor(() => expect(onAdded).toHaveBeenCalled());
  });

  it("honours a chosen kind", async () => {
    render(withUI(<WorldAddForm onAdded={() => {}} />));
    fireEvent.change(screen.getByLabelText("Entity name"), { target: { value: "agezt" } });
    fireEvent.change(screen.getByLabelText("Entity kind"), { target: { value: "repo" } });
    fireEvent.click(screen.getByRole("button", { name: /Add entity/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/world/add", { name: "agezt", kind: "repo" }));
  });
});

describe("WorldRelateForm", () => {
  it("defaults to the first two entities and a relates_to verb, posting them", async () => {
    const onRelated = vi.fn();
    render(withUI(<WorldRelateForm names={["Ada", "agezt"]} onRelated={onRelated} />));
    fireEvent.click(screen.getByRole("button", { name: /Relate/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/world/relate", { from: "Ada", verb: "relates_to", to: "agezt" }),
    );
    await waitFor(() => expect(onRelated).toHaveBeenCalled());
  });

  it("honours chosen from/verb/to", async () => {
    render(withUI(<WorldRelateForm names={["Ada", "agezt", "Acme"]} onRelated={() => {}} />));
    fireEvent.change(screen.getByLabelText("Relation from"), { target: { value: "Ada" } });
    fireEvent.change(screen.getByLabelText("Relation verb"), { target: { value: "owns" } });
    fireEvent.change(screen.getByLabelText("Relation to"), { target: { value: "agezt" } });
    fireEvent.click(screen.getByRole("button", { name: /Relate/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/world/relate", { from: "Ada", verb: "owns", to: "agezt" }),
    );
  });

  it("disables Relate when from and to are the same", () => {
    render(withUI(<WorldRelateForm names={["Solo"]} onRelated={() => {}} />));
    // Only one name → from and to both "Solo" → invalid.
    expect((screen.getByRole("button", { name: /Relate/ }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("WorldEditForm (M730)", () => {
  const entity = {
    id: "ent-42",
    kind: "person",
    name: "Ada",
    aliases: ["the boss"],
    attrs: { brief: "morning", tz: "UTC" },
  };

  it("prefills aliases and attribute rows from the entity", () => {
    render(withUI(<WorldEditForm entity={entity} onSaved={() => {}} />));
    expect((screen.getByLabelText("Edit entity aliases") as HTMLInputElement).value).toBe("the boss");
    expect((screen.getByLabelText("Attribute key 1") as HTMLInputElement).value).toBe("brief");
    expect((screen.getByLabelText("Attribute value 1") as HTMLInputElement).value).toBe("morning");
    expect((screen.getByLabelText("Attribute key 2") as HTMLInputElement).value).toBe("tz");
  });

  it("posts the replaced aliases + attrs (split, trimmed, blanks dropped) to world/edit", async () => {
    const onSaved = vi.fn();
    render(withUI(<WorldEditForm entity={entity} onSaved={onSaved} />));
    fireEvent.change(screen.getByLabelText("Edit entity aliases"), { target: { value: " ada k ,  , the lead " } });
    // Change brief, blank out tz's value (should be dropped).
    fireEvent.change(screen.getByLabelText("Attribute value 1"), { target: { value: "evening, terse" } });
    fireEvent.change(screen.getByLabelText("Attribute value 2"), { target: { value: "  " } });
    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/world/edit", {
        id: "ent-42",
        aliases: ["ada k", "the lead"],
        attrs: { brief: "evening, terse" },
      }),
    );
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
  });

  it("adds and removes attribute rows", async () => {
    render(withUI(<WorldEditForm entity={{ id: "e1", name: "X", attrs: {} }} onSaved={() => {}} />));
    // No rows yet → the empty hint shows.
    expect(screen.getByText(/add a preference or constraint/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /add attribute/ }));
    fireEvent.change(screen.getByLabelText("Attribute key 1"), { target: { value: "role" } });
    fireEvent.change(screen.getByLabelText("Attribute value 1"), { target: { value: "owner" } });
    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/world/edit", { id: "e1", aliases: [], attrs: { role: "owner" } }),
    );
  });
});

describe("World entity search (M774)", () => {
  it("entityMatches matches on name, kind, or alias (case-insensitive)", async () => {
    const { entityMatches } = await import("@/views/World");
    const e = { name: "AGEZT", kind: "project", aliases: ["the daemon"] };
    expect(entityMatches(e, "agezt")).toBe(true);
    expect(entityMatches(e, "project")).toBe(true);
    expect(entityMatches(e, "daemon")).toBe(true);
    expect(entityMatches(e, "nope")).toBe(false);
    expect(entityMatches(e, "")).toBe(true); // empty query matches all
  });

  it("filters the entity list and shows a match count (only when >4 entities)", async () => {
    getJSON.mockResolvedValue({
      entities: [
        { id: "e1", name: "Alice", kind: "person" },
        { id: "e2", name: "Bob", kind: "person" },
        { id: "e3", name: "AGEZT", kind: "project" },
        { id: "e4", name: "kernel", kind: "repo" },
        { id: "e5", name: "webui", kind: "repo" },
      ],
      edges: [],
      relation_count: 0,
    });
    render(withUI(<World />));
    const input = await screen.findByLabelText("Filter entities");
    // No count chip until a query is entered.
    expect(screen.queryByText("2/5")).toBeNull();
    // Filter to the repos → the match count reflects 2 of 5 (entity names also appear as
    // <option>s in the relate-form dropdown, so the count chip is the unambiguous signal).
    fireEvent.change(input, { target: { value: "repo" } });
    await waitFor(() => expect(screen.getByText("2/5")).toBeTruthy());
    // A non-matching query shows the empty hint and a 0/5 count.
    fireEvent.change(input, { target: { value: "zzz" } });
    await waitFor(() => expect(screen.getByText(/no entities match/)).toBeTruthy());
    expect(screen.getByText("0/5")).toBeTruthy();
  });
});
