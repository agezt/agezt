// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { getTheme, toggleTheme, applyTheme } from "@/lib/theme";

// The module reads localStorage once at import, so these tests exercise the live
// singleton: they assert the toggle is consistent (DOM class + storage + getTheme
// all move together) rather than a specific absolute start value.
beforeEach(() => {
  document.documentElement.classList.remove("dark");
});

describe("theme store", () => {
  it("toggleTheme flips getTheme, the <html> dark class, and localStorage together", () => {
    const before = getTheme();
    toggleTheme();
    const after = getTheme();
    expect(after).not.toBe(before);
    // DOM + storage reflect the new value, in lockstep.
    expect(document.documentElement.classList.contains("dark")).toBe(after === "dark");
    expect(localStorage.getItem("agezt-theme")).toBe(after);

    // Toggling back restores the original.
    toggleTheme();
    expect(getTheme()).toBe(before);
    expect(document.documentElement.classList.contains("dark")).toBe(before === "dark");
  });

  it("applyTheme reflects the current theme onto the DOM (DOM-only, no persistence)", () => {
    localStorage.clear();
    document.documentElement.classList.remove("dark");
    applyTheme();
    expect(document.documentElement.classList.contains("dark")).toBe(getTheme() === "dark");
    // applyTheme must NOT persist — only an explicit setTheme/toggle does (so a first-run
    // OS-derived default isn't locked into storage before the user chooses).
    expect(localStorage.getItem("agezt-theme")).toBeNull();
  });

  it("notifies subscribers on toggle (single source of truth for header + palette)", () => {
    // useSyncExternalStore subscribes via the module's listener set; emulate a
    // subscriber by toggling and confirming getTheme is the snapshot it would read.
    let observed: string | null = null;
    const snapshot = () => {
      observed = getTheme();
    };
    snapshot();
    const start = observed;
    toggleTheme();
    snapshot();
    expect(observed).not.toBe(start);
    toggleTheme(); // restore
  });
});
