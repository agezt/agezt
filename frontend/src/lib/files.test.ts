// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, cleanup } from "@testing-library/react";

// We mock the api module so we can drive useFileTree through both the success
// branch and the HTTPError / 404 fallback path. The mock is hoisted before the
// import of `files.ts`, which vitest requires for the spies to be in place when
// the module evaluates.
vi.mock("@/lib/api", async (importOriginal) => {
  const actual = (await importOriginal()) as Record<string, unknown>;
  return {
    ...actual,
    HTTPError: class HTTPError extends Error {
      status: number;
      url: string;
      constructor(status: number, url: string, message: string) {
        super(message);
        this.name = "HTTPError";
        this.status = status;
        this.url = url;
      }
    },
    getJSON: vi.fn(),
  };
});

import { HTTPError, getJSON } from "@/lib/api";
import { useFileTree, basename, isPathSafe, joinPath, parentPath } from "./files";

const mockedGetJSON = vi.mocked(getJSON);

beforeEach(() => {
  mockedGetJSON.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("files path helpers", () => {
  it("isPathSafe rejects absolute, drive-prefixed, and NUL paths", () => {
    expect(isPathSafe("notes/x.md")).toBe(true);
    expect(isPathSafe("")).toBe(true);
    expect(isPathSafe("/etc/passwd")).toBe(false);
    expect(isPathSafe("C:\\Windows")).toBe(false);
    expect(isPathSafe("notes/x\0y")).toBe(false);
  });

  it("isPathSafe rejects '..' segments", () => {
    expect(isPathSafe("../foo")).toBe(false);
    expect(isPathSafe("a/../b")).toBe(false);
    expect(isPathSafe("a/..hidden")).toBe(true); // '..' as a prefix of a name is fine
  });

  it("joinPath concatenates and normalises", () => {
    expect(joinPath("a", "b/c")).toBe("a/b/c");
    expect(joinPath("a/", "/b")).toBe("a/b");
    expect(joinPath("", "b")).toBe("b");
    expect(joinPath("a", "")).toBe("a");
    expect(joinPath("a", "./b")).toBe("a/b");
  });

  it("parentPath returns '' for root and prefix for everything else", () => {
    expect(parentPath("")).toBe("");
    expect(parentPath("a")).toBe("");
    expect(parentPath("a/b")).toBe("a");
    expect(parentPath("a/b/c")).toBe("a/b");
  });

  it("basename returns the leaf only", () => {
    expect(basename("")).toBe("");
    expect(basename("a")).toBe("a");
    expect(basename("a/b/c.md")).toBe("c.md");
  });
});

describe("useFileTree hook", () => {
  it("returns the parsed tree when getJSON resolves", async () => {
    const fixture = { root: "", nodes: [{ name: "x", path: "x", type: "file" as const }] };
    mockedGetJSON.mockResolvedValueOnce(fixture);
    const { result } = renderHook(() => useFileTree(""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBeNull();
    expect(result.current.data).toEqual(fixture);
  });

  it("falls back to a stub tree on a typed HTTPError 404 (the pre-Slice-5 case)", async () => {
    // Real HTTPError shape (status field + name), exactly what lib/api.ts
    // throws now. The previous code matched this with a regex on the message,
    // which broke the moment lib/api changed its wording.
    const err = new HTTPError(404, "/api/files/tree?path=", "Not Found");
    mockedGetJSON.mockRejectedValueOnce(err);
    const { result } = renderHook(() => useFileTree(""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBeNull();
    // The stub tree is a deterministic placeholder — at minimum it should
    // expose the standard "notes" + "scratch.txt" pair so the page renders
    // something for the operator to see.
    expect(result.current.data?.nodes.some((n) => n.name === "scratch.txt")).toBe(true);
  });

  it("surfaces a non-404 HTTPError as a human error message", async () => {
    mockedGetJSON.mockRejectedValueOnce(new HTTPError(500, "/api/files/tree?path=", "boom"));
    const { result } = renderHook(() => useFileTree(""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBe("boom");
  });

  it("surfaces a plain Error (network failure, parse error) as-is", async () => {
    mockedGetJSON.mockRejectedValueOnce(new Error("network down"));
    const { result } = renderHook(() => useFileTree(""));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBe("network down");
  });
});

describe("HTTPError typed error", () => {
  it("preserves status, url, and message", () => {
    const e = new HTTPError(403, "/api/foo", "Forbidden");
    expect(e).toBeInstanceOf(Error);
    expect(e.status).toBe(403);
    expect(e.url).toBe("/api/foo");
    expect(e.message).toBe("Forbidden");
    expect(e.name).toBe("HTTPError");
  });
});
