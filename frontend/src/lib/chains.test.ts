import { describe, it, expect } from "vitest";
import {
  isChainRef,
  chainName,
  chainRef,
  chainLabel,
  validateChainName,
  moveItem,
  removeAt,
  renameChain,
  deleteChain,
} from "@/lib/chains";

describe("chain references", () => {
  it("isChainRef / chainName / chainRef round-trip", () => {
    expect(isChainRef("@fast")).toBe(true);
    expect(isChainRef("gpt-4o")).toBe(false);
    expect(isChainRef("@")).toBe(false); // bare prefix is not a reference
    expect(chainName("@fast")).toBe("fast");
    expect(chainName("gpt-4o")).toBe("");
    expect(chainRef("fast")).toBe("@fast");
  });

  it("chainLabel shows count for refs, passes plain ids through", () => {
    const chains = { fast: ["a", "b"] };
    expect(chainLabel("@fast", chains)).toBe("⛓ fast (2)");
    expect(chainLabel("@gone", chains)).toBe("⛓ gone"); // unknown chain: no count
    expect(chainLabel("gpt-4o", chains)).toBe("gpt-4o");
  });
});

describe("validateChainName", () => {
  it("accepts slugs, rejects bad names", () => {
    expect(validateChainName("fast-cheap")).toBeNull();
    expect(validateChainName("f1")).toBeNull();
    expect(validateChainName("")).toMatch(/required/);
    expect(validateChainName("  ")).toMatch(/required/);
    expect(validateChainName("Fast")).toBeTruthy(); // uppercase
    expect(validateChainName("-bad")).toBeTruthy(); // leading dash
    expect(validateChainName("a b")).toBeTruthy(); // space
    expect(validateChainName("@x")).toBeTruthy(); // @ not allowed
  });
});

describe("list ops", () => {
  it("moveItem swaps within bounds, no-op at edges", () => {
    expect(moveItem(["a", "b", "c"], 0, 1)).toEqual(["b", "a", "c"]);
    expect(moveItem(["a", "b", "c"], 2, 1)).toEqual(["a", "b", "c"]); // last down: no-op
    expect(moveItem(["a", "b", "c"], 0, -1)).toEqual(["a", "b", "c"]); // first up: no-op
  });
  it("removeAt drops by index", () => {
    expect(removeAt(["a", "b", "c"], 1)).toEqual(["a", "c"]);
  });
});

describe("map ops", () => {
  it("renameChain moves models under a new key", () => {
    const r = renameChain({ fast: ["a"], big: ["b"] }, "fast", "quick");
    expect(r).toEqual({ quick: ["a"], big: ["b"] });
  });
  it("renameChain is a no-op for same/missing key", () => {
    const c = { fast: ["a"] };
    expect(renameChain(c, "fast", "fast")).toBe(c);
    expect(renameChain(c, "nope", "x")).toBe(c);
  });
  it("deleteChain removes the key", () => {
    expect(deleteChain({ fast: ["a"], big: ["b"] }, "fast")).toEqual({ big: ["b"] });
  });
});
