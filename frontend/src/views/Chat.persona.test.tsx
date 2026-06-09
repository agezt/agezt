// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { ConversationPersona } from "@/views/Chat";

afterEach(cleanup);

describe("ConversationPersona", () => {
  it("shows the default label when no override is set", () => {
    render(<ConversationPersona value="" onChange={() => {}} />);
    expect(screen.getByText("persona: default")).toBeTruthy();
  });

  it("marks an active override (drops the ': default' suffix)", () => {
    render(<ConversationPersona value="be terse" onChange={() => {}} />);
    expect(screen.getByText("persona")).toBeTruthy();
    expect(screen.queryByText("persona: default")).toBeNull();
  });

  it("edits and saves a persona (trimmed)", () => {
    const onChange = vi.fn();
    render(<ConversationPersona value="" onChange={onChange} />);
    fireEvent.click(screen.getByText("persona: default"));
    const ta = screen.getByLabelText("Conversation persona");
    fireEvent.change(ta, { target: { value: "  act as a Go reviewer  " } });
    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    expect(onChange).toHaveBeenCalledWith("act as a Go reviewer");
  });

  it("clears the persona via Clear", () => {
    const onChange = vi.fn();
    render(<ConversationPersona value="something" onChange={onChange} />);
    fireEvent.click(screen.getByText("persona"));
    fireEvent.click(screen.getByRole("button", { name: /Clear/ }));
    expect(onChange).toHaveBeenCalledWith("");
  });
});
