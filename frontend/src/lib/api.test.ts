// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { readAndScrubToken } from "@/lib/api";

beforeEach(() => {
  history.replaceState(null, "", "/");
});

describe("console token in the address bar", () => {
  it("returns the token and takes it back out of the URL", () => {
    history.replaceState(null, "", "/?token=SECRET-CONSOLE-TOKEN");

    expect(readAndScrubToken()).toBe("SECRET-CONSOLE-TOKEN");
    expect(location.search).toBe("");
    expect(location.href).not.toContain("SECRET-CONSOLE-TOKEN");
  });

  it("keeps every other query param, the path and the hash", () => {
    history.replaceState(null, "", "/console?view=runs&token=SECRET&sort=age#agent/scribe");

    expect(readAndScrubToken()).toBe("SECRET");
    expect(location.pathname).toBe("/console");
    expect(location.hash).toBe("#agent/scribe");
    // Order is preserved and only `token` is gone — scrubbing must not cost the
    // operator the deep link they followed.
    expect(location.search).toBe("?view=runs&sort=age");
  });

  it("is a no-op when there is no token", () => {
    history.replaceState(null, "", "/console?view=runs");

    expect(readAndScrubToken()).toBe("");
    expect(location.search).toBe("?view=runs");
  });

  it("replaces the history entry rather than pushing one, so Back still works", () => {
    const before = history.length;
    history.replaceState(null, "", "/?token=SECRET");

    readAndScrubToken();

    expect(history.length).toBe(before);
  });
});
