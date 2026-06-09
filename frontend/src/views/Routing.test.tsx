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

import { Routing } from "@/views/Routing";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

const ROUTING = {
  task_types: ["chat", "plan", "code", "verify"],
  chains: {
    chat: ["claude-opus", "gpt-5", "deepseek-chat"],
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
