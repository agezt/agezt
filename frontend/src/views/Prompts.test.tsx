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

import { Prompts, parsePromptsJSON } from "@/views/Prompts";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  getJSON.mockResolvedValue({ prompts: [] });
});

describe("Prompts view", () => {
  it("shows an empty hint when no prompts exist", async () => {
    render(withUI(<Prompts />));
    await waitFor(() => expect(screen.getByText(/No prompts yet/)).toBeTruthy());
  });

  it("loads existing prompts into editable rows", async () => {
    getJSON.mockResolvedValue({ prompts: [{ title: "Standup", text: "Draft my standup." }] });
    render(withUI(<Prompts />));
    await waitFor(() => expect((screen.getByLabelText("Prompt 1 title") as HTMLInputElement).value).toBe("Standup"));
    expect((screen.getByLabelText("Prompt 1 text") as HTMLTextAreaElement).value).toBe("Draft my standup.");
  });

  it("adds a prompt and saves the list (blank rows dropped, fields trimmed)", async () => {
    postJSON.mockResolvedValueOnce({ count: 1 });
    render(withUI(<Prompts />));
    await waitFor(() => expect(screen.getByText(/No prompts yet/)).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: /Add prompt/ }));
    fireEvent.change(screen.getByLabelText("Prompt 1 title"), { target: { value: "  Review  " } });
    fireEvent.change(screen.getByLabelText("Prompt 1 text"), { target: { value: "  Review the diff.  " } });

    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/prompts/set", { prompts: [{ title: "Review", text: "Review the diff." }] }),
    );
  });

  it("deletes a prompt row", async () => {
    getJSON.mockResolvedValue({ prompts: [{ title: "A", text: "a" }, { title: "B", text: "b" }] });
    render(withUI(<Prompts />));
    await waitFor(() => expect(screen.getByLabelText("Prompt 2 title")).toBeTruthy());
    fireEvent.click(screen.getAllByTitle("Delete prompt")[0]);
    await waitFor(() => expect(screen.queryByLabelText("Prompt 2 title")).toBeNull());
    // The remaining row is the former B.
    expect((screen.getByLabelText("Prompt 1 title") as HTMLInputElement).value).toBe("B");
  });
});

describe("parsePromptsJSON", () => {
  it("parses a bare array of {title,text}, trimming and dropping invalid rows", () => {
    const out = parsePromptsJSON(
      JSON.stringify([
        { title: "  A  ", text: "  body a  " },
        { title: "", text: "no title" },
        { title: "B", text: "" },
        { title: "C", text: "body c" },
      ]),
    );
    expect(out).toEqual([
      { title: "A", text: "body a" },
      { title: "C", text: "body c" },
    ]);
  });

  it("accepts a {prompts:[…]} wrapper", () => {
    const out = parsePromptsJSON(JSON.stringify({ prompts: [{ title: "X", text: "y" }] }));
    expect(out).toEqual([{ title: "X", text: "y" }]);
  });

  it("throws on bad JSON", () => {
    expect(() => parsePromptsJSON("{not json")).toThrow();
  });

  it("throws when no valid prompts are present", () => {
    expect(() => parsePromptsJSON(JSON.stringify([{ title: "", text: "" }]))).toThrow(/no valid prompts/);
    expect(() => parsePromptsJSON(JSON.stringify({ nope: 1 }))).toThrow(/expected an array/);
  });
});
