// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { getAdvanced, setAdvanced, toggleAdvanced, applyAdvanced } from "@/lib/advanced";

beforeEach(() => {
  localStorage.clear();
  setAdvanced(false);
});

describe("advanced mode store", () => {
  it("defaults to off (calm)", () => {
    expect(getAdvanced()).toBe(false);
  });

  it("toggles, persists, and reflects onto <html>.advanced", () => {
    toggleAdvanced();
    expect(getAdvanced()).toBe(true);
    expect(localStorage.getItem("agezt-advanced")).toBe("1");
    applyAdvanced();
    expect(document.documentElement.classList.contains("advanced")).toBe(true);

    toggleAdvanced();
    expect(getAdvanced()).toBe(false);
    expect(localStorage.getItem("agezt-advanced")).toBe("0");
    applyAdvanced();
    expect(document.documentElement.classList.contains("advanced")).toBe(false);
  });

  it("setAdvanced is idempotent", () => {
    setAdvanced(true);
    setAdvanced(true);
    expect(getAdvanced()).toBe(true);
  });
});
