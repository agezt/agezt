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

  it("renders one tab per facet of a folded destination", () => {
    // Runs is a multi-facet destination post-Day 25 (Runs + Activity + Replay).
    render(<ViewTabs active="runs" onSelect={() => {}} />);
    const tabs = screen.getAllByRole("tab").map((el) => el.textContent);
    expect(tabs).toEqual(["Runs", "Activity", "Replay"]);
  });

  it("marks the active facet, not the first one", () => {
    render(<ViewTabs active="replay" onSelect={() => {}} />);
    expect(screen.getByRole("tab", { selected: true }).textContent).toBe("Replay");
  });

  it("navigates by the facet's own view id, so deep links keep working", () => {
    const onSelect = vi.fn();
    render(<ViewTabs active="runs" onSelect={onSelect} />);
    fireEvent.click(screen.getByRole("tab", { name: "Replay" }));
    expect(onSelect).toHaveBeenCalledWith("replay");
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
