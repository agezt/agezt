// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { AnimatedNumber } from "@/components/AnimatedNumber";

afterEach(cleanup);

describe("AnimatedNumber", () => {
  it("renders the target value immediately (count-up is test/SSR-safe)", () => {
    render(<AnimatedNumber value={42} />);
    expect(screen.getByText("42")).toBeTruthy();
  });

  it("groups thousands (locale separator), digits intact", () => {
    const { container } = render(<AnimatedNumber value={1234567} />);
    // Separator char is locale-dependent ("," or "."); assert grouping happened
    // and the digits are preserved, without pinning a locale.
    const text = container.textContent || "";
    expect(/[.,]/.test(text)).toBe(true);
    expect(text.replace(/[.,\s]/g, "")).toBe("1234567");
  });

  it("passes non-finite values through unchanged", () => {
    render(<AnimatedNumber value={NaN} />);
    expect(screen.getByText("NaN")).toBeTruthy();
  });
});
