import { describe, it, expect } from "vitest";
import { addQueued, removeQueued, moveQueued, dequeueFront, type QueuedMsg } from "@/lib/queue";

const q = (...texts: string[]): QueuedMsg[] => texts.map((t, i) => ({ id: `id${i}`, text: t }));

describe("queue ops", () => {
  it("addQueued appends non-blank, trims, skips blank", () => {
    expect(addQueued([], "  hi  ", "a")).toEqual([{ id: "a", text: "hi" }]);
    expect(addQueued(q("one"), "two", "b")).toEqual([{ id: "id0", text: "one" }, { id: "b", text: "two" }]);
    expect(addQueued(q("one"), "   ", "b")).toEqual(q("one"));
  });

  it("removeQueued drops by id", () => {
    expect(removeQueued(q("a", "b", "c"), "id1")).toEqual([{ id: "id0", text: "a" }, { id: "id2", text: "c" }]);
    expect(removeQueued(q("a"), "nope")).toEqual(q("a"));
  });

  it("moveQueued reorders within bounds and no-ops at edges", () => {
    const base = q("a", "b", "c");
    expect(moveQueued(base, "id1", -1).map((m) => m.text)).toEqual(["b", "a", "c"]);
    expect(moveQueued(base, "id1", 1).map((m) => m.text)).toEqual(["a", "c", "b"]);
    expect(moveQueued(base, "id0", -1)).toEqual(base); // first up = no-op
    expect(moveQueued(base, "id2", 1)).toEqual(base); // last down = no-op
    expect(moveQueued(base, "ghost", 1)).toEqual(base);
  });

  it("dequeueFront splits head from rest", () => {
    expect(dequeueFront(q("a", "b"))).toEqual({ front: { id: "id0", text: "a" }, rest: [{ id: "id1", text: "b" }] });
    expect(dequeueFront([])).toEqual({ front: null, rest: [] });
  });
});
