// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { loadConsoleName, saveConsoleName, applyConsoleTitle, DEFAULT_NAME } from "@/lib/brand";

beforeEach(() => {
  localStorage.clear();
  document.title = "";
});

describe("console name", () => {
  it("defaults to the brand name", () => {
    expect(loadConsoleName()).toBe(DEFAULT_NAME);
  });

  it("saves a trimmed name and updates the document title", () => {
    const saved = saveConsoleName("  Jarvis  ");
    expect(saved).toBe("Jarvis");
    expect(loadConsoleName()).toBe("Jarvis");
    expect(localStorage.getItem("agezt-console-name")).toBe("Jarvis");
    expect(document.title).toBe("Jarvis · console");
  });

  it("clears the override when set back to the default", () => {
    saveConsoleName("Jarvis");
    const back = saveConsoleName(DEFAULT_NAME);
    expect(back).toBe(DEFAULT_NAME);
    expect(localStorage.getItem("agezt-console-name")).toBeNull();
  });

  it("falls back to the default for a blank name", () => {
    expect(saveConsoleName("   ")).toBe(DEFAULT_NAME);
  });

  it("applyConsoleTitle sets the tab title", () => {
    applyConsoleTitle("Friday");
    expect(document.title).toBe("Friday · console");
  });
});
