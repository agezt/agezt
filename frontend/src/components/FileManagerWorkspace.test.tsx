import { describe, it, expect } from "vitest";
import { tooLargeReason } from "@/components/FileManagerWorkspace";

// Two-mebibyte cap; tested at the boundary so the off-by-one regression from
// `>=` vs `>` is caught here, not in production.
const TWO_MIB = 2 * 1024 * 1024;

describe("tooLargeReason", () => {
  it("returns null when the Content-Length header is missing", () => {
    expect(tooLargeReason(null)).toBeNull();
    expect(tooLargeReason(null, 0)).toBeNull();
  });

  it("returns null when the file is comfortably below the cap", () => {
    expect(tooLargeReason(1024)).toBeNull();
    expect(tooLargeReason(null, 4096)).toBeNull();
    expect(tooLargeReason(TWO_MIB - 1)).toBeNull();
  });

  it("returns null at exactly the cap (>= vs > off-by-one guard)", () => {
    expect(tooLargeReason(TWO_MIB)).toBeNull();
    expect(tooLargeReason(null, TWO_MIB)).toBeNull();
  });

  it("returns a sentinel string for files past the cap", () => {
    expect(tooLargeReason(TWO_MIB + 1)).toBe(`too_large:${TWO_MIB + 1}`);
    expect(tooLargeReason(null, TWO_MIB * 4)).toBe(`too_large:${TWO_MIB * 4}`);
  });

  it("prefers the actual body byte count over the header when both are given", () => {
    // Header says 1 KB (under cap), body says 10 MB (over): trust the body.
    expect(tooLargeReason(1024, TWO_MIB * 5)).toBe(`too_large:${TWO_MIB * 5}`);
    // Header says 10 MB (over cap), body says 1 KB (under): trust the body.
    expect(tooLargeReason(TWO_MIB * 10, 1024)).toBeNull();
  });

  it("encodes the measured size in the sentinel so the UI can render the right humanSize", () => {
    // 5_000_000 bytes ≈ 4.77 MB; the operator sees "4.8 MB" so they know
    // how big the file is before they download it.
    const r = tooLargeReason(5_000_000);
    expect(r).toBe("too_large:5000000");
  });
});
