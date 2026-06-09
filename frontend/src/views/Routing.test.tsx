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

import { Routing, parseChainsJSON } from "@/views/Routing";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

const ROUTING = {
  task_types: ["chat", "plan", "code", "verify"],
  chains: {
    chat: ["claude-opus", "gpt-5", "deepseek-chat"],
  },
  activity: {
    chat: { fallbacks: 2, last_failed: "claude-opus", last_next: "gpt-5", last_reason: "anthropic: 529 overloaded" },
  },
};

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  getJSON.mockResolvedValue(ROUTING);
});

describe("Routing view", () => {
  it("renders a row per task type with the known types first", async () => {
    render(withUI(<Routing />));
    await waitFor(() => expect(screen.getByText("chat")).toBeTruthy());
    expect(screen.getByText("plan")).toBeTruthy();
    expect(screen.getByText("code")).toBeTruthy();
    expect(screen.getByText("verify")).toBeTruthy();
  });

  it("shows a configured chain as primary + numbered fallbacks", async () => {
    render(withUI(<Routing />));
    await waitFor(() => expect(screen.getByText("claude-opus")).toBeTruthy());
    // "primary" also appears in the explainer paragraph; the badge makes it ≥2.
    expect(screen.getAllByText("primary").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("fallback 1")).toBeTruthy();
    expect(screen.getByText("fallback 2")).toBeTruthy();
    expect(screen.getByText("gpt-5")).toBeTruthy();
    expect(screen.getByText("deepseek-chat")).toBeTruthy();
  });

  it("surfaces model-chain fallback activity for a task", async () => {
    render(withUI(<Routing />));
    await waitFor(() => expect(screen.getByText("claude-opus")).toBeTruthy());
    expect(screen.getByText("2 fallbacks")).toBeTruthy();
    expect(screen.getByText(/claude-opus → gpt-5/)).toBeTruthy();
    expect(screen.getByText(/529 overloaded/)).toBeTruthy();
  });

  it("marks empty task types as daemon default", async () => {
    render(withUI(<Routing />));
    await waitFor(() => expect(screen.getByText("chat")).toBeTruthy());
    // plan/code/verify have no chain → at least one "daemon default" marker.
    expect(screen.getAllByText("daemon default").length).toBeGreaterThanOrEqual(3);
  });

  it("removes a model from a chain and Save posts the updated chains", async () => {
    postJSON.mockResolvedValueOnce({ saved: true, applied: "live", task_count: 1 });
    render(withUI(<Routing />));
    await waitFor(() => expect(screen.getByText("deepseek-chat")).toBeTruthy());

    // Remove the last fallback (deepseek-chat).
    const removeBtns = screen.getAllByTitle("Remove");
    fireEvent.click(removeBtns[removeBtns.length - 1]);
    await waitFor(() => expect(screen.queryByText("deepseek-chat")).toBeNull());

    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/routing/set", {
        chains: { chat: ["claude-opus", "gpt-5"] },
      }),
    );
  });

  it("reorders a fallback above the primary with the up control", async () => {
    postJSON.mockResolvedValueOnce({ saved: true, task_count: 1 });
    render(withUI(<Routing />));
    await waitFor(() => expect(screen.getByText("gpt-5")).toBeTruthy());

    // Move gpt-5 (index 1) up to become primary.
    const upBtns = screen.getAllByTitle("Move up");
    fireEvent.click(upBtns[1]);

    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/routing/set", {
        chains: { chat: ["gpt-5", "claude-opus", "deepseek-chat"] },
      }),
    );
  });

  it("disables Save until a change is made", async () => {
    render(withUI(<Routing />));
    await waitFor(() => expect(screen.getByText("chat")).toBeTruthy());
    const save = screen.getByRole("button", { name: /Save/ }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    fireEvent.click(screen.getAllByTitle("Remove")[0]);
    await waitFor(() => expect((screen.getByRole("button", { name: /Save/ }) as HTMLButtonElement).disabled).toBe(false));
  });
});

describe("parseChainsJSON", () => {
  it("accepts a bare {task: [models]} map", () => {
    expect(parseChainsJSON('{"chat":["a","b"],"code":["c"]}')).toEqual({ chat: ["a", "b"], code: ["c"] });
  });

  it("unwraps a {chains:{…}} export wrapper", () => {
    expect(parseChainsJSON('{"chains":{"chat":["a"]}}')).toEqual({ chat: ["a"] });
  });

  it("trims keys/models and drops blanks and non-strings", () => {
    expect(parseChainsJSON('{" chat ":["  a  ","",null,3,"b"]}')).toEqual({ chat: ["a", "b"] });
  });

  it("drops tasks whose chain ends up empty", () => {
    expect(parseChainsJSON('{"chat":["a"],"plan":[],"code":["","  "]}')).toEqual({ chat: ["a"] });
  });

  it("throws on invalid JSON", () => {
    expect(() => parseChainsJSON("not json")).toThrow();
  });

  it("throws on a non-object / array shape", () => {
    expect(() => parseChainsJSON('["a","b"]')).toThrow(/expected an object/);
    expect(() => parseChainsJSON("42")).toThrow(/expected an object/);
  });

  it("throws when nothing valid remains", () => {
    expect(() => parseChainsJSON('{"chat":[],"plan":["",null]}')).toThrow(/no valid/);
  });
});

describe("Routing import/export", () => {
  it("imports a chains file, merging over existing chains for review", async () => {
    render(withUI(<Routing />));
    await waitFor(() => expect(screen.getByText("chat")).toBeTruthy());

    // A file overriding chat and adding a new code chain.
    const file = new File([JSON.stringify({ chains: { chat: ["x", "y"], code: ["z"] } })], "agezt-routing.json", {
      type: "application/json",
    });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(input, { target: { files: [file] } });

    // Merged chains render: the imported chat models and the new code model.
    await waitFor(() => expect(screen.getByText("x")).toBeTruthy());
    expect(screen.getByText("y")).toBeTruthy();
    expect(screen.getByText("z")).toBeTruthy();
    // The old chat models are gone (chat was overridden, not appended).
    expect(screen.queryByText("claude-opus")).toBeNull();

    // Save posts the merged result.
    postJSON.mockResolvedValueOnce({ saved: true, task_count: 2 });
    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/routing/set", {
        chains: { chat: ["x", "y"], code: ["z"] },
      }),
    );
  });

  it("reports a bad import file without changing the chains", async () => {
    render(withUI(<Routing />));
    await waitFor(() => expect(screen.getByText("claude-opus")).toBeTruthy());

    const file = new File(["not json"], "bad.json", { type: "application/json" });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(input, { target: { files: [file] } });

    await waitFor(() => expect(screen.getByText(/Import failed/)).toBeTruthy());
    // Original chain intact.
    expect(screen.getByText("claude-opus")).toBeTruthy();
  });
});
