import { describe, it, expect } from "vitest";
import { skillToRef, memoryToRef, runToRef, buildContext, withContext, type AttachRef } from "@/lib/attach";

describe("mappers", () => {
  it("maps a skill to a ref (body as content)", () => {
    expect(skillToRef({ id: "s1", name: "deploy", body: "step 1\nstep 2" })).toEqual({
      kind: "skill",
      id: "s1",
      label: "deploy",
      content: "deploy\nstep 1\nstep 2",
    });
  });

  it("maps a memory to a ref (subject + content)", () => {
    expect(memoryToRef({ id: "m1", subject: "fav color", content: "teal", type: "PREFERENCE" })).toEqual({
      kind: "memory",
      id: "m1",
      label: "fav color",
      content: "fav color: teal",
    });
  });

  it("maps a run to a ref keyed by correlation id", () => {
    const r = runToRef({ correlation_id: "run-9", intent: "do a thing", status: "completed", answer: "done" });
    expect(r?.kind).toBe("run");
    expect(r?.id).toBe("run-9");
    expect(r?.content).toContain("intent: do a thing");
    expect(r?.content).toContain("status: completed");
  });

  it("returns null when an entity lacks an id", () => {
    expect(skillToRef({ name: "x" })).toBeNull();
    expect(memoryToRef({ content: "x" })).toBeNull();
    expect(runToRef({ intent: "x" })).toBeNull();
  });
});

describe("buildContext / withContext", () => {
  const refs: AttachRef[] = [
    { kind: "skill", id: "s1", label: "deploy", content: "step 1" },
    { kind: "memory", id: "m1", label: "fav", content: "teal" },
  ];

  it("renders a labelled preamble per ref", () => {
    const ctx = buildContext(refs);
    expect(ctx).toContain("### Attached skill: deploy");
    expect(ctx).toContain("step 1");
    expect(ctx).toContain("### Attached memory: fav");
  });

  it("is empty (and leaves intent unchanged) with no attachments", () => {
    expect(buildContext([])).toBe("");
    expect(withContext("hello", [])).toBe("hello");
  });

  it("prepends the preamble before the intent when attachments exist", () => {
    const out = withContext("turn it off", refs);
    expect(out.endsWith("turn it off")).toBe(true);
    expect(out.indexOf("Attached skill")).toBeLessThan(out.indexOf("turn it off"));
  });
});
