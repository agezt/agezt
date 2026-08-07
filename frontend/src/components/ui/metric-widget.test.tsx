// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { MetricGrid, MetricWidget } from "@/components/ui/metric-widget";

// MetricGrid's `cols` prop accepts two forms; the Tailwind-class form used to
// be applied as an inline grid-template-columns value — invalid CSS that
// silently collapsed the grid to one full-width column per widget (the
// Activity/Insights stacked-cards bug).
describe("MetricGrid cols handling", () => {
  it("applies a CSS grid-template value as inline style", () => {
    const { container } = render(
      <MetricGrid cols="repeat(auto-fill, minmax(140px, 1fr))">
        <MetricWidget label="a" value={1} />
      </MetricGrid>,
    );
    const el = container.firstElementChild as HTMLElement;
    expect(el.style.gridTemplateColumns).toContain("auto-fill");
    expect(el.className).not.toContain("grid-cols");
  });

  it("routes Tailwind grid classes to className, never to inline style", () => {
    const { container } = render(
      <MetricGrid cols="grid-cols-2 sm:grid-cols-3 lg:grid-cols-5">
        <MetricWidget label="a" value={1} />
      </MetricGrid>,
    );
    const el = container.firstElementChild as HTMLElement;
    expect(el.className).toContain("grid-cols-2");
    expect(el.className).toContain("lg:grid-cols-5");
    expect(el.style.gridTemplateColumns).toBe("");
  });

  it("defaults to the responsive auto-fill template", () => {
    const { container } = render(
      <MetricGrid>
        <MetricWidget label="a" value={1} />
      </MetricGrid>,
    );
    const el = container.firstElementChild as HTMLElement;
    expect(el.style.gridTemplateColumns).toContain("minmax(160px, 1fr)");
  });
});
