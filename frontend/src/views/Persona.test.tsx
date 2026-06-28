// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { Persona } from "@/views/Persona";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  getJSON.mockResolvedValue({ system: "", set: false });
});

describe("Persona view", () => {
  it("loads the current default identity instructions into the editor and reports state", async () => {
    getJSON.mockResolvedValue({ system: "You are Jarvis.", set: true });
    render(withUI(<Persona />));
    await waitFor(() => expect(screen.getAllByText(/custom default identity active/).length).toBeGreaterThan(0));
    expect(screen.queryByLabelText("Default identity instructions")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /Edit/ }));
    await waitFor(() => expect((screen.getByLabelText("Default identity instructions") as HTMLTextAreaElement).value).toBe("You are Jarvis."));
  });

  it("disables Save until the text changes, then posts the new default identity", async () => {
    render(withUI(<Persona />));
    await waitFor(() => expect(screen.getByText(/Built-in identity is active/)).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Edit/ }));
    await waitFor(() => expect(screen.getByLabelText("Default identity instructions")).toBeTruthy());
    const save = screen.getByRole("button", { name: /Save/ }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("Default identity instructions"), { target: { value: "Be terse." } });
    await waitFor(() => expect((screen.getByRole("button", { name: /Save/ }) as HTMLButtonElement).disabled).toBe(false));
    expect(screen.getByText("unsaved changes")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/persona/set", { system: "Be terse." }));
  });

  it("inserts a preset template into the editor", async () => {
    render(withUI(<Persona />));
    await waitFor(() => expect(screen.getByText(/Built-in identity is active/)).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Terse & proactive/ }));
    expect(screen.getByRole("dialog", { name: "Edit default identity" })).toBeTruthy();
    expect((screen.getByLabelText("Default identity instructions") as HTMLTextAreaElement).value).toMatch(/terse and direct/);
  });

  it("clears the default identity via Clear (posts an empty system)", async () => {
    getJSON.mockResolvedValue({ system: "You are Jarvis.", set: true });
    render(withUI(<Persona />));
    await waitFor(() => expect(screen.getAllByText(/custom default identity active/).length).toBeGreaterThan(0));
    fireEvent.click(screen.getByRole("button", { name: /Clear/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/persona/set", { system: "" }));
  });
});
