import { describe, expect, it } from "vitest";
import { stepFromFrame } from "@/lib/market";

describe("market helpers", () => {
  it("extracts install progress steps", () => {
    expect(stepFromFrame({ kind: "market.install.progress", payload: { stage: "skill", name: "web", ok: true, detail: "done" } })).toEqual({
      stage: "skill",
      name: "web",
      ok: true,
      detail: "done",
    });
  });

  it("ignores non-progress frames", () => {
    expect(stepFromFrame({ kind: "done", payload: {} })).toBeNull();
    expect(stepFromFrame({ kind: "market.install.progress", payload: { ok: 0 } })).toEqual({ stage: "", ok: false, name: undefined, detail: undefined });
  });
});
