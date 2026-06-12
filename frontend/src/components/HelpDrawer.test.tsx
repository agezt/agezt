// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { MessageSquare } from "lucide-react";
import { HelpDrawer } from "@/components/HelpDrawer";
import { HELP } from "@/lib/help";

afterEach(cleanup);

describe("HelpDrawer", () => {
  it("renders nothing while closed", () => {
    const { container } = render(
      <HelpDrawer open={false} viewId="chat" onClose={() => {}} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders the active view's topic with group chip, sections, tips and related chips", () => {
    render(
      <HelpDrawer
        open
        viewId="chat"
        group="Converse"
        icon={MessageSquare}
        onClose={() => {}}
        onNavigate={() => {}}
      />,
    );
    const dialog = screen.getByRole("dialog");
    expect(dialog.getAttribute("aria-label")).toBe("Help: Chat");
    expect(screen.getByText("Converse")).toBeTruthy();
    expect(dialog.textContent).toContain(HELP.chat.intro);
    // Every section heading is present.
    for (const s of HELP.chat.sections) expect(screen.getByText(s.heading)).toBeTruthy();
    // Related chips render as buttons.
    for (const r of HELP.chat.related || []) expect(screen.getByText(r.label)).toBeTruthy();
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
    render(<HelpDrawer open viewId="chat" onClose={onClose} onNavigate={onNavigate} />);
    const first = (HELP.chat.related || [])[0];
    fireEvent.click(screen.getByText(first.label));
    expect(onNavigate).toHaveBeenCalledWith(first.id);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows the fallback topic for an unknown view id", () => {
    render(<HelpDrawer open viewId="mystery" onClose={() => {}} />);
    expect(screen.getByRole("dialog").textContent).toContain("No detailed guide");
  });
});
