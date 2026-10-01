// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { SectionNav, ViewTabs } from "@/components/AppNav";
import { NAV_GROUPS } from "@/nav";

afterEach(cleanup);

const section = (id: string) => NAV_GROUPS.find((g) => g.id === id)!;

function renderNav(active: string, onSelect = vi.fn()) {
  const groupId = rowForViewGroup(active);
  render(
    <SectionNav
      navSection={groupId}
      setNavSection={() => {}}
      activeGroupId={groupId}
      shownGroup={section(groupId)}
      active={active}
      onSelect={onSelect}
      unseenAlerts={0}
      activeRunCount={0}
      build={null}
    />,
  );
  return onSelect;
}

function rowForViewGroup(view: string): string {
  return NAV_GROUPS.find((g) => g.rows.some((r) => r.views.some((v) => v.id === view)))!.id;
}

describe("ViewTabs", () => {
  it("renders nothing for a single-view destination", () => {
    const { container } = render(<ViewTabs active="voice" onSelect={() => {}} />);
    expect(container.querySelector('[role="tablist"]')).toBeNull();
  });

  // These three used to assert the Runs row had three tabs — Activity and
  // Replay alongside Runs — which is exactly what made them wrong: all three
  // rendered the same component, so the strip promised facets that did not
  // exist. They are written against Monitor instead, which genuinely has two,
  // so the strip's behaviour stays covered rather than being deleted along with
  // the row that broke it. Monitor was not chosen to be the fixture; it is
  // simply a row whose views are different components.
  it("renders one tab per facet of a folded destination", () => {
    render(<ViewTabs active="mission" onSelect={() => {}} />);
    const tabs = screen.getAllByRole("tab").map((el) => el.textContent);
    expect(tabs).toEqual(["Mission Control", "Live Stream"]);
  });

  it("marks the active facet, not the first one", () => {
    render(<ViewTabs active="feed" onSelect={() => {}} />);
    expect(screen.getByRole("tab", { selected: true }).textContent).toBe("Live Stream");
  });

  it("navigates by the facet's own view id, so deep links keep working", () => {
    const onSelect = vi.fn();
    render(<ViewTabs active="mission" onSelect={onSelect} />);
    fireEvent.click(screen.getByRole("tab", { name: "Live Stream" }));
    expect(onSelect).toHaveBeenCalledWith("feed");
  });

  it("renders no tab strip for Runs now that it is a single destination", () => {
    // The same rule, asserted at the strip: a destination with one view has
    // nothing to choose between.
    const { container } = render(<ViewTabs active="runs" onSelect={() => {}} />);
    expect(container.querySelector('[role="tablist"]')).toBeNull();
  });
});

describe("SectionNav rows", () => {
  it("lists destinations, not every view", () => {
    // Mission Control is a tab of the Monitor row; the sidebar must show the
    // row, not the tab label, so the operator sees "Monitor" as the
    // destination and not "Mission Control" / "Live Stream" competing for
    // scanner attention.
    renderNav("mission");
    expect(screen.getByText("Monitor")).toBeTruthy();
    expect(screen.queryByText("Mission Control")).toBeNull();
  });

  it("clicking a destination lands on its first facet", () => {
    // After Day 28 the Observe › Overview row was renamed to "Monitor" and
    // its (misleading, aliased-to-Standing) first tab was dropped; the first
    // surviving facet is Mission Control.
    const onSelect = renderNav("mission");
    fireEvent.click(screen.getByText("Monitor"));
    expect(onSelect).toHaveBeenCalledWith("mission");
  });

  it("highlights the destination of a deep-linked facet", () => {
    // `#models` is a tab of "Providers & Models" — the row must light up.
    renderNav("models");
    const row = screen.getByText("Providers & Models").closest("button")!;
    expect(row.className).toContain("font-semibold");
  });
});
