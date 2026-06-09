import { describe, it, expect, beforeEach } from "vitest";
import { focusRun, clearRunFocus, getRunFocus } from "@/lib/runfocus";

beforeEach(() => clearRunFocus());

describe("runfocus store", () => {
  it("focusRun sets the pending run, clearRunFocus drops it", () => {
    expect(getRunFocus()).toBeNull();
    focusRun("corr-123");
    expect(getRunFocus()).toBe("corr-123");
    clearRunFocus();
    expect(getRunFocus()).toBeNull();
  });

  it("notifies subscribers on focus and on clear", () => {
    // emulate a useSyncExternalStore subscriber: read the snapshot on each notify.
    let snapshots: (string | null)[] = [];
    const observe = () => snapshots.push(getRunFocus());
    observe();
    focusRun("a");
    observe();
    clearRunFocus();
    observe();
    expect(snapshots).toEqual([null, "a", null]);
  });

  it("clearRunFocus is a no-op when nothing is focused", () => {
    // Should not throw and remains null.
    clearRunFocus();
    expect(getRunFocus()).toBeNull();
  });
});
