// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { exportAppearance, parseAppearanceJSON, applyAppearanceBundle } from "@/lib/appearance";
import { getAccentHue } from "@/lib/accent";
import { getConsoleName } from "@/lib/brand";
import { getTheme } from "@/lib/theme";

beforeEach(() => {
  localStorage.clear();
});

describe("parseAppearanceJSON", () => {
  it("reads a bare object, keeping only valid fields", () => {
    expect(parseAppearanceJSON('{"theme":"light","accentHue":300,"consoleName":"jarvis"}')).toEqual({
      theme: "light",
      accentHue: 300,
      consoleName: "jarvis",
    });
  });

  it("unwraps the {appearance:{…}} export wrapper", () => {
    expect(parseAppearanceJSON('{"version":1,"appearance":{"theme":"dark"}}')).toEqual({ theme: "dark" });
  });

  it("drops unknown/invalid fields and trims the name", () => {
    expect(parseAppearanceJSON('{"theme":"neon","accentHue":"x","consoleName":"  hal  ","junk":1}')).toEqual({
      consoleName: "hal",
    });
  });

  it("normalises an out-of-range hue onto [0,360)", () => {
    expect(parseAppearanceJSON('{"accentHue":420}')).toEqual({ accentHue: 60 });
    expect(parseAppearanceJSON('{"accentHue":-30}')).toEqual({ accentHue: 330 });
  });

  it("throws on invalid JSON, non-object, or no usable fields", () => {
    expect(() => parseAppearanceJSON("nope")).toThrow();
    expect(() => parseAppearanceJSON("[1,2]")).toThrow(/expected an appearance object/);
    expect(() => parseAppearanceJSON('{"foo":1}')).toThrow(/no valid appearance fields/);
  });
});

describe("export + apply round-trip", () => {
  it("exportAppearance wraps the current look; applying a bundle persists each field", () => {
    const snap = exportAppearance();
    expect(snap.version).toBe(1);
    expect(snap.appearance).toHaveProperty("theme");
    expect(snap.appearance).toHaveProperty("accentHue");
    expect(snap.appearance).toHaveProperty("consoleName");

    // Force a known baseline so the light transition below actually writes (setTheme
    // no-ops when unchanged, and the module singleton carries across tests).
    applyAppearanceBundle({ theme: "dark" });
    applyAppearanceBundle({ theme: "light", accentHue: 150, consoleName: "jarvis" });
    expect(getTheme()).toBe("light");
    expect(getAccentHue()).toBe(150);
    expect(getConsoleName()).toBe("jarvis");
    // Persisted to storage so a reload restores it.
    expect(localStorage.getItem("agezt-theme")).toBe("light");
    expect(localStorage.getItem("agezt-accent-hue")).toBe("150");
    expect(localStorage.getItem("agezt-console-name")).toBe("jarvis");

    // A partial bundle only touches the fields it carries.
    applyAppearanceBundle({ accentHue: 10 });
    expect(getAccentHue()).toBe(10);
    expect(getConsoleName()).toBe("jarvis"); // unchanged
  });
});
