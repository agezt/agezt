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

import { Roster, NewAgentForm, slugOk, usdToMc } from "@/views/Roster";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  postJSON.mockResolvedValue({});
  postAction.mockResolvedValue({});
});

describe("slugOk", () => {
  it("mirrors the kernel slug rule", () => {
    for (const s of ["researcher", "ops-watcher", "r2.d2", "x_1", "a"]) expect(slugOk(s)).toBe(true);
    for (const s of ["", "Researcher", "has space", "-lead", ".lead", "_lead", "a".repeat(65)])
      expect(slugOk(s)).toBe(false);
  });
});

describe("usdToMc", () => {
  it("converts dollars to USD-microcents ($1 = 1e9)", () => {
    expect(usdToMc("0.50")).toBe(500_000_000);
    expect(usdToMc("$1")).toBe(1_000_000_000);
    expect(usdToMc("")).toBe(0); // blank = no cap
    expect(usdToMc("abc")).toBeNull();
    expect(usdToMc("-1")).toBeNull();
  });
});

describe("NewAgentForm", () => {
  it("disables Create until the slug is valid, then posts the profile shape", async () => {
    const onCreated = vi.fn();
    render(<NewAgentForm onCreated={onCreated} onError={() => {}} />);
    const create = screen.getByRole("button", { name: /Create agent/ }) as HTMLButtonElement;
    expect(create.disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("Agent slug"), { target: { value: "BAD SLUG" } });
    expect((screen.getByRole("button", { name: /Create agent/ }) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("Agent slug"), { target: { value: "researcher" } });
    fireEvent.change(screen.getByLabelText("Agent soul"), { target: { value: "You dig deep." } });
    fireEvent.change(screen.getByLabelText("Agent model"), { target: { value: "m-1" } });
    fireEvent.change(screen.getByLabelText("Fallback models"), { target: { value: "m2, m3" } });
    fireEvent.change(screen.getByLabelText("Max cost per run"), { target: { value: "0.50" } });
    const btn = screen.getByRole("button", { name: /Create agent/ }) as HTMLButtonElement;
    expect(btn.disabled).toBe(false);
    fireEvent.click(btn);

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith(
        "/api/agents/add",
        expect.objectContaining({
          profile: expect.objectContaining({
            slug: "researcher",
            soul: "You dig deep.",
            model: "m-1",
            fallbacks: ["m2", "m3"],
            max_cost_mc: 500_000_000,
          }),
        }),
      ),
    );
    expect(onCreated).toHaveBeenCalledWith("researcher");
  });

  it("rejects a bad max-cost without posting", async () => {
    const onError = vi.fn();
    render(<NewAgentForm onCreated={() => {}} onError={onError} />);
    fireEvent.change(screen.getByLabelText("Agent slug"), { target: { value: "ops" } });
    fireEvent.change(screen.getByLabelText("Max cost per run"), { target: { value: "lots" } });
    fireEvent.click(screen.getByRole("button", { name: /Create agent/ }));
    await waitFor(() => expect(onError).toHaveBeenCalled());
    expect(postJSON).not.toHaveBeenCalled();
  });
});

describe("Roster", () => {
  it("renders profiles from /api/agents with state, model, and budget", async () => {
    getJSON.mockResolvedValue({
      profiles: [
        {
          id: "01A", slug: "researcher", name: "The Researcher", enabled: true,
          model: "m-1", task_type: "research", max_cost_mc: 500_000_000,
          soul: "You dig deep.", fallbacks: ["m2"],
        },
        { id: "01B", slug: "ops", enabled: false },
      ],
      count: 2,
      enabled_count: 1,
    });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("researcher")).toBeTruthy());
    expect(screen.getByText("ops")).toBeTruthy();
    expect(screen.getByText("paused")).toBeTruthy();
    expect(screen.getByText("model: m-1")).toBeTruthy();
    expect(screen.getByText("max/run: $0.5000")).toBeTruthy();
    expect(screen.getByText("You dig deep.")).toBeTruthy();
    expect(getJSON).toHaveBeenCalledWith("/api/agents");
  });

  it("pause posts /api/agents/enable with ref + enabled=false", async () => {
    getJSON.mockResolvedValue({
      profiles: [{ id: "01A", slug: "researcher", enabled: true }],
      count: 1, enabled_count: 1,
    });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("researcher")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Pause researcher" }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/agents/enable", { ref: "researcher", enabled: "false" }),
    );
  });

  it("shows the empty state when the roster is empty", async () => {
    getJSON.mockResolvedValue({ profiles: [], count: 0, enabled_count: 0 });
    render(withUI(<Roster />));
    await waitFor(() => expect(screen.getByText("No agents yet")).toBeTruthy());
  });
});
