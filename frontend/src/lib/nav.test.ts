// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { goToHash, goToView, normaliseHash } from "@/lib/nav";

describe("normaliseHash", () => {
  it("adds a leading # when missing", () => {
    expect(normaliseHash("agents")).toBe("#agents");
  });

  it("accepts a leading # and leaves it alone", () => {
    expect(normaliseHash("#agents")).toBe("#agents");
  });

  it("preserves query strings", () => {
    expect(normaliseHash("files?path=notes/README.md")).toBe("#files?path=notes/README.md");
  });

  it("returns empty for empty or whitespace-only input", () => {
    expect(normaliseHash("")).toBe("");
    expect(normaliseHash("   ")).toBe("");
  });

  it("treats lone '#' as empty", () => {
    expect(normaliseHash("#")).toBe("");
  });
});

describe("goToHash", () => {
  beforeEach(() => {
    // jsdom resets location.hash to "" between tests
    location.hash = "";
  });

  it("sets location.hash when it differs from the current value", () => {
    location.hash = "#chat";
    goToHash("agents");
    expect(location.hash).toBe("#agents");
  });

  it("is a no-op when the destination matches the current hash", () => {
    location.hash = "#agents";
    // Browsers fire hashchange only on real changes; emulate by stubbing
    let fired = false;
    const onHash = () => {
      fired = true;
    };
    window.addEventListener("hashchange", onHash);
    goToHash("agents");
    window.removeEventListener("hashchange", onHash);
    expect(fired).toBe(false);
    expect(location.hash).toBe("#agents");
  });

  it("normalises a bare id without '#'", () => {
    location.hash = "";
    goToHash("runs");
    expect(location.hash).toBe("#runs");
  });

  it("ignores empty targets", () => {
    location.hash = "#chat";
    goToHash("");
    expect(location.hash).toBe("#chat");
  });
});

describe("goToView", () => {
  beforeEach(() => {
    location.hash = "";
  });

  it("navigates to a view id alone", () => {
    goToView("files");
    expect(location.hash).toBe("#files");
  });

  it("appends a query string verbatim", () => {
    goToView("files", "path=notes/README.md");
    expect(location.hash).toBe("#files?path=notes/README.md");
  });

  it("does not double-prefix a leading '?'", () => {
    goToView("files", "?path=notes/x.md");
    expect(location.hash).toBe("#files?path=notes/x.md");
  });

  it("refuses empty view ids", () => {
    location.hash = "#chat";
    goToView("");
    expect(location.hash).toBe("#chat");
  });
});
