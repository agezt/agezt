// @vitest-environment jsdom
//
// Render-smoke guard for views migrated to the `Page` primitive that otherwise
// have no test. It locks in two things the big scroll/responsive sweep is easy
// to silently regress:
//   1. the view still MOUNTS without throwing, and
//   2. its root is the scroll-safe `Page` (min-h-full) — not a re-introduced
//      `h-full min-h-0` fixed shell that clips on short screens.
// If someone later reverts one of these back to a fixed-shell root, this fails.
import type { ReactElement } from "react";
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup } from "@testing-library/react";
import { UIProvider } from "@/components/ui/feedback";

// Data-less mounts: getJSON never resolves with real data, so each view renders
// its skeleton/empty branch — enough to exercise the Page root + header.
vi.mock("@/lib/api", () => ({
  getJSON: vi.fn(() => new Promise(() => {})),
  postAction: vi.fn(),
}));
// Config reads through usePanel; keep it in its loading state.
vi.mock("@/lib/usePanel", () => ({
  usePanel: () => ({ data: null, error: null, loading: true, reload: () => {} }),
}));
// FlowStudio subscribes to the event stream and renders a React-Flow DAG
// (@xyflow/react needs ResizeObserver, absent in jsdom) — stub both so the
// smoke exercises FlowStudio's own Page shell, not the graph internals.
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));
vi.mock("@/components/PlanDag", () => ({ PlanDag: () => null }));

import { Reflect } from "./Reflect";
import { Analyst } from "./Analyst";
import { Config } from "./Config";
import { FlowStudio } from "./FlowStudio";

afterEach(cleanup);

function root(container: HTMLElement): HTMLElement {
  return container.firstElementChild as HTMLElement;
}

describe("migrated scroll-mode views keep a scroll-safe Page root", () => {
  const cases: [string, () => ReactElement][] = [
    ["Reflection", () => <Reflect />],
    ["Analyst", () => <Analyst />],
    ["Config", () => <Config />],
  ];

  for (const [label, View] of cases) {
    it(`${label} mounts and keeps a min-h-full Page root`, () => {
      const { container } = render(
        <UIProvider>
          <View />
        </UIProvider>,
      );
      const el = root(container);
      expect(el).toBeTruthy();
      // Page scroll mode grows instead of pinning to the viewport.
      expect(el.className).toContain("min-h-full");
      // And it must NOT have re-introduced the fixed-shell clip.
      expect(el.className).not.toContain("h-full min-h-0");
    });
  }
});

describe("migrated fill-mode app views keep a bounded Page root", () => {
  it("FlowStudio mounts and keeps an h-full min-h-0 fill root", () => {
    const { container } = render(
      <UIProvider>
        <FlowStudio />
      </UIProvider>,
    );
    const el = root(container);
    expect(el).toBeTruthy();
    // fill mode is deliberately bounded so its inner panes scroll.
    expect(el.className).toContain("h-full");
    expect(el.className).toContain("min-h-0");
  });
});
