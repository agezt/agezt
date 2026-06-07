// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { Badge, statusVariant } from "@/components/ui/badge";

afterEach(cleanup);

describe("Badge", () => {
  it("renders its children", () => {
    render(<Badge>hello</Badge>);
    expect(screen.getByText("hello")).toBeTruthy();
  });

  it("applies the variant's colour class", () => {
    render(<Badge variant="bad">nope</Badge>);
    const el = screen.getByText("nope");
    expect(el.className).toContain("text-bad");
  });
});

describe("statusVariant", () => {
  it("maps run/plan statuses to badge variants", () => {
    expect(statusVariant("completed")).toBe("good");
    expect(statusVariant("failed")).toBe("bad");
    expect(statusVariant("abandoned")).toBe("bad");
    expect(statusVariant("running")).toBe("accent");
    expect(statusVariant("queued")).toBe("default");
    expect(statusVariant(undefined)).toBe("default");
  });
});
