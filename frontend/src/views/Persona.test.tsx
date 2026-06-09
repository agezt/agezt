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
  it("loads the current persona into the editor and reports default state", async () => {
    getJSON.mockResolvedValue({ system: "You are Jarvis.", set: true });
    render(withUI(<Persona />));
    await waitFor(() => expect((screen.getByLabelText("Persona system prompt") as HTMLTextAreaElement).value).toBe("You are Jarvis."));
    expect(screen.getByText(/custom persona active/)).toBeTruthy();
  });

  it("disables Save until the text changes, then posts the new persona", async () => {
    render(withUI(<Persona />));
    await waitFor(() => expect(screen.getByLabelText("Persona system prompt")).toBeTruthy());
    const save = screen.getByRole("button", { name: /Save/ }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("Persona system prompt"), { target: { value: "Be terse." } });
    await waitFor(() => expect((screen.getByRole("button", { name: /Save/ }) as HTMLButtonElement).disabled).toBe(false));
    expect(screen.getByText("unsaved changes")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/persona/set", { system: "Be terse." }));
  });

  it("inserts a preset template into the editor", async () => {
    render(withUI(<Persona />));
    await waitFor(() => expect(screen.getByLabelText("Persona system prompt")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Terse & proactive/ }));
    expect((screen.getByLabelText("Persona system prompt") as HTMLTextAreaElement).value).toMatch(/terse and direct/);
  });

  it("clears the persona via Clear (posts an empty system)", async () => {
    getJSON.mockResolvedValue({ system: "You are Jarvis.", set: true });
    render(withUI(<Persona />));
    await waitFor(() => expect(screen.getByText(/custom persona active/)).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Clear/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/persona/set", { system: "" }));
  });
});
