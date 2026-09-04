import { describe, expect, it } from "vitest";
import { toneForStatus, toneForRate, toneText, toneBorder, tonePlate, toneChip, toneBg, toneSurface, toneBar } from "@/lib/tone";

describe("toneForStatus", () => {
  it("gives the same word the same meaning", () => {
    // These were coloured independently on the fleet, the agent detail tabs, the
    // inspector, the council and the insights chart — "done" was border-good in
    // one place and text-good in another, "failed" likewise.
    for (const ok of ["ok", "done", "completed", "passed", "armed", "installed"]) {
      expect(toneForStatus(ok), ok).toBe("good");
    }
    for (const live of ["running", "active", "streaming", "routed"]) {
      expect(toneForStatus(live), live).toBe("accent");
    }
    for (const attn of ["degraded", "pending", "blocked", "quarantined"]) {
      expect(toneForStatus(attn), attn).toBe("warn");
    }
    for (const broken of ["failed", "error", "cancelled", "denied"]) {
      expect(toneForStatus(broken), broken).toBe("bad");
    }
    for (const inert of ["retired", "disabled", "archived", "idle"]) {
      expect(toneForStatus(inert), inert).toBe("muted");
    }
  });

  it("is case- and whitespace-insensitive, and calm about the unknown", () => {
    expect(toneForStatus("FAILED")).toBe("bad");
    expect(toneForStatus("  Running ")).toBe("accent");
    // An unclassified status is context, not an alarm.
    expect(toneForStatus("frobnicating")).toBe("muted");
    expect(toneForStatus(undefined)).toBe("muted");
    expect(toneForStatus(null)).toBe("muted");
    expect(toneForStatus("")).toBe("muted");
  });
});

describe("toneForRate", () => {
  it("uses one threshold everywhere: 90 good, 70 warn", () => {
    expect(toneForRate(100)).toBe("good");
    expect(toneForRate(90)).toBe("good");
    expect(toneForRate(89)).toBe("warn");
    expect(toneForRate(70)).toBe("warn");
    expect(toneForRate(69)).toBe("bad");
  });

  it("inverts for rates where lower is better", () => {
    expect(toneForRate(0, true)).toBe("good");
    expect(toneForRate(9, true)).toBe("warn");
    expect(toneForRate(10, true)).toBe("bad");
  });
});

describe("the scales", () => {
  const TONES = ["accent", "good", "warn", "bad", "muted"] as const;

  it("covers every tone in every slot", () => {
    for (const map of [toneText, toneBorder, tonePlate, toneChip, toneBg, toneSurface, toneBar]) {
      for (const t of TONES) expect(map[t], t).toBeTruthy();
    }
  });

  it("keeps toneBg free of a text colour", () => {
    // toneChip carries both; toneBg must not, or a caller pairing it with
    // toneText ends up with two competing `text-*` classes whose winner is
    // decided by stylesheet order.
    for (const t of TONES) expect(toneBg[t]).not.toMatch(/\btext-/);
  });
});
