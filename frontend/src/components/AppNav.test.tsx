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
    const { container } = render(<ViewTabs active="chat" onSelect={() => {}} />);
    expect(container.querySelector('[role="tablist"]')).toBeNull();
  });

  it("renders one tab per facet of a folded destination", () => {
    render(<ViewTabs active="health" onSelect={() => {}} />);
    const tabs = screen.getAllByRole("tab").map((el) => el.textContent);
    expect(tabs).toEqual(["Health", "Prompt cache", "Tool usage", "Routing log"]);
  });

  it("marks the active facet, not the first one", () => {
    render(<ViewTabs active="providers" onSelect={() => {}} />);
    expect(screen.getByRole("tab", { selected: true }).textContent).toBe("Routing log");
  });

  it("navigates by the facet's own view id, so deep links keep working", () => {
    const onSelect = vi.fn();
    render(<ViewTabs active="health" onSelect={onSelect} />);
    fireEvent.click(screen.getByRole("tab", { name: "Tool usage" }));
    expect(onSelect).toHaveBeenCalledWith("tools");
  });
});

describe("SectionNav rows", () => {
  it("lists destinations, not every view", () => {
    renderNav("health");
    // Observe has five rows; Health alone folds four views.
    expect(screen.getByText("Health")).toBeTruthy();
    expect(screen.queryByText("Routing log")).toBeNull();
  });

  it("clicking a destination lands on its first facet", () => {
    const onSelect = renderNav("alerts");
    fireEvent.click(screen.getByText("Overview"));
    expect(onSelect).toHaveBeenCalledWith("overview");
  });

  it("highlights the destination of a deep-linked facet", () => {
    // `#models` is a tab of "Providers & Models" — the row must light up.
    renderNav("models");
    const row = screen.getByText("Providers & Models").closest("button")!;
    expect(row.className).toContain("font-semibold");
  });
});
