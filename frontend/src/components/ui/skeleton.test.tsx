// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { Skeleton, SkeletonCard, SkeletonList, SkeletonGrid } from "@/components/ui/skeleton";

afterEach(cleanup);

describe("Skeleton", () => {
  it("renders a shimmer block carrying the .skeleton class", () => {
    const { container } = render(<Skeleton className="h-4 w-10" />);
    const el = container.firstChild as HTMLElement;
    expect(el.className).toContain("skeleton");
    expect(el.className).toContain("h-4");
  });

  it("is hidden from the accessibility tree", () => {
    const { container } = render(<Skeleton />);
    expect((container.firstChild as HTMLElement).getAttribute("aria-hidden")).toBe("true");
  });
});

describe("SkeletonCard", () => {
  it("renders the requested number of body lines plus the header row", () => {
    const { container } = render(<SkeletonCard lines={3} />);
    // 3 header placeholders + 3 body lines = 6 shimmer blocks.
    expect(container.querySelectorAll(".skeleton").length).toBe(6);
  });
});

describe("SkeletonList / SkeletonGrid", () => {
  it("exposes a loading status region with N cards", () => {
    render(<SkeletonList count={5} />);
    const region = screen.getByRole("status", { name: "loading" });
    expect(region.children.length).toBe(5);
  });

  it("grid also exposes a loading status region with N cards", () => {
    render(<SkeletonGrid count={4} />);
    const region = screen.getByRole("status", { name: "loading" });
    expect(region.children.length).toBe(4);
  });
});
