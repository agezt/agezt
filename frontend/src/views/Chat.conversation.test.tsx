// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { ConversationItem } from "@/views/Chat";

afterEach(cleanup);

describe("ConversationItem", () => {
  it("selects on click and reveals rename + delete", () => {
    const onSelect = vi.fn();
    render(<ConversationItem title="Thread A" active={false} onSelect={onSelect} onRemove={() => {}} onRename={() => {}} />);
    fireEvent.click(screen.getByText("Thread A"));
    expect(onSelect).toHaveBeenCalled();
    expect(screen.getByLabelText("Rename conversation")).toBeTruthy();
    expect(screen.getByLabelText("Delete conversation")).toBeTruthy();
  });

  it("renames inline on Enter", () => {
    const onRename = vi.fn();
    render(<ConversationItem title="Old" active onSelect={() => {}} onRemove={() => {}} onRename={onRename} />);
    fireEvent.click(screen.getByLabelText("Rename conversation"));
    const input = screen.getByLabelText("Conversation title");
    fireEvent.change(input, { target: { value: "New name" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onRename).toHaveBeenCalledWith("New name");
  });

  it("renames via double-click then commits on blur", () => {
    const onRename = vi.fn();
    render(<ConversationItem title="Old" active onSelect={() => {}} onRemove={() => {}} onRename={onRename} />);
    fireEvent.doubleClick(screen.getByText("Old"));
    const input = screen.getByLabelText("Conversation title");
    fireEvent.change(input, { target: { value: "Renamed" } });
    fireEvent.blur(input);
    expect(onRename).toHaveBeenCalledWith("Renamed");
  });

  it("cancels on Escape without renaming", () => {
    const onRename = vi.fn();
    render(<ConversationItem title="Keep" active onSelect={() => {}} onRemove={() => {}} onRename={onRename} />);
    fireEvent.click(screen.getByLabelText("Rename conversation"));
    const input = screen.getByLabelText("Conversation title");
    fireEvent.change(input, { target: { value: "nope" } });
    fireEvent.keyDown(input, { key: "Escape" });
    expect(onRename).not.toHaveBeenCalled();
  });
});
