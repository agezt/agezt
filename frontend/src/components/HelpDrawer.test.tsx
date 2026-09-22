// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { MessageSquare } from "lucide-react";
import { HelpDrawer } from "@/components/HelpDrawer";
import { HELP } from "@/app/help";

afterEach(cleanup);

describe("HelpDrawer", () => {
  it("renders nothing while closed", () => {
    const { container } = render(
      <HelpDrawer open={false} viewId="runs" onClose={() => {}} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders the active view's topic with group chip, sections, tips and related chips", () => {
    // `chat` was retired in Day 25; pick a still-live view that has a topic
    // with sections and related chips.
    render(
      <HelpDrawer
        open
        viewId="runs"
        group="Observe"
        icon={MessageSquare}
        onClose={() => {}}
        onNavigate={() => {}}
      />,
    );
    const dialog = screen.getByRole("dialog");
    expect(dialog.getAttribute("aria-label")).toBe("Help: Runs");
    expect(screen.getByText("Observe")).toBeTruthy();
    expect(dialog.textContent).toContain(HELP.runs.intro);
    // Every section heading is present.
    for (const s of HELP.runs.sections) expect(screen.getByText(s.heading)).toBeTruthy();
    // Related chips render as buttons.
    for (const r of HELP.runs.related || []) expect(screen.getByText(r.label)).toBeTruthy();
  });

  it("closes on Escape and on backdrop click, but not on a click inside", () => {
    const onClose = vi.fn();
    const { container } = render(<HelpDrawer open viewId="runs" onClose={onClose} />);
    fireEvent.click(screen.getByRole("dialog"));
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(container.firstChild as Element); // backdrop
    expect(onClose).toHaveBeenCalledTimes(1);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("navigates via related chips without closing", () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();
    render(<HelpDrawer open viewId="runs" onClose={onClose} onNavigate={onNavigate} />);
    const first = (HELP.runs.related || [])[0];
    fireEvent.click(screen.getByText(first.label));
    expect(onNavigate).toHaveBeenCalledWith(first.id);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows the fallback topic for an unknown view id", () => {
    render(<HelpDrawer open viewId="mystery" onClose={() => {}} />);
    expect(screen.getByRole("dialog").textContent).toContain("No detailed guide");
  });
});
