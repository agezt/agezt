// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { loadAccentHue, applyAccentHue, saveAccentHue, DEFAULT_HUE, ACCENTS } from "@/lib/accent";

beforeEach(() => {
  localStorage.clear();
  document.documentElement.style.removeProperty("--accent-hue");
});

describe("accent hue", () => {
  it("defaults to the brand hue when nothing is stored", () => {
    expect(loadAccentHue()).toBe(DEFAULT_HUE);
  });

  it("saves a hue to storage and applies it to :root", () => {
    saveAccentHue(150);
    expect(localStorage.getItem("agezt-accent-hue")).toBe("150");
    expect(document.documentElement.style.getPropertyValue("--accent-hue")).toBe("150");
    expect(loadAccentHue()).toBe(150);
  });

  it("removes the override when set back to the default (lets the stylesheet govern)", () => {
    applyAccentHue(150);
    expect(document.documentElement.style.getPropertyValue("--accent-hue")).toBe("150");
    applyAccentHue(DEFAULT_HUE);
    expect(document.documentElement.style.getPropertyValue("--accent-hue")).toBe("");
  });

  it("falls back to the default for a corrupt stored value", () => {
    localStorage.setItem("agezt-accent-hue", "not-a-number");
    expect(loadAccentHue()).toBe(DEFAULT_HUE);
  });

  it("offers a spread of distinct preset hues", () => {
    const hues = ACCENTS.map((a) => a.hue);
    expect(new Set(hues).size).toBe(hues.length); // all distinct
    expect(hues).toContain(DEFAULT_HUE);
  });
});
