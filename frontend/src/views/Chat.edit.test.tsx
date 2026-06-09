// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { UserBubble } from "@/views/Chat";

afterEach(cleanup);

describe("UserBubble edit", () => {
  it("renders the message text and an edit affordance when editable", () => {
    render(<UserBubble text="turn on the lamp" onEdit={() => {}} />);
    expect(screen.getByText("turn on the lamp")).toBeTruthy();
    expect(screen.getByTitle("Edit & re-run")).toBeTruthy();
  });

  it("hides the edit affordance when no handler (e.g. a run is in flight)", () => {
    render(<UserBubble text="busy now" />);
    expect(screen.queryByTitle("Edit & re-run")).toBeNull();
  });

  it("edits and submits the new text via Save & run", () => {
    const onEdit = vi.fn();
    render(<UserBubble text="old ask" onEdit={onEdit} />);
    fireEvent.click(screen.getByTitle("Edit & re-run"));
    const ta = screen.getByLabelText("Edit message") as HTMLTextAreaElement;
    fireEvent.change(ta, { target: { value: "new ask" } });
    fireEvent.click(screen.getByRole("button", { name: /Save & run/ }));
    expect(onEdit).toHaveBeenCalledWith("new ask");
  });

  it("submits on Enter and ignores an unchanged edit", () => {
    const onEdit = vi.fn();
    render(<UserBubble text="same" onEdit={onEdit} />);
    fireEvent.click(screen.getByTitle("Edit & re-run"));
    const ta = screen.getByLabelText("Edit message");
    // Unchanged text → no re-run.
    fireEvent.keyDown(ta, { key: "Enter" });
    expect(onEdit).not.toHaveBeenCalled();
    // Back to read view (textarea gone).
    expect(screen.queryByLabelText("Edit message")).toBeNull();
    expect(screen.getByText("same")).toBeTruthy();
  });

  it("cancels with Escape without calling onEdit", () => {
    const onEdit = vi.fn();
    render(<UserBubble text="keep me" onEdit={onEdit} />);
    fireEvent.click(screen.getByTitle("Edit & re-run"));
    const ta = screen.getByLabelText("Edit message");
    fireEvent.change(ta, { target: { value: "discard this" } });
    fireEvent.keyDown(ta, { key: "Escape" });
    expect(onEdit).not.toHaveBeenCalled();
    expect(screen.getByText("keep me")).toBeTruthy();
    expect(screen.queryByText("discard this")).toBeNull();
  });
});
