// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));
vi.mock("@/lib/events", () => ({
  useEvents: () => ({ events: [], connected: true, subscribe: () => () => {} }),
}));

import { Wizards } from "@/views/Wizards";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  getJSON.mockResolvedValue({});
  postJSON.mockResolvedValue({});
  postAction.mockResolvedValue({});
});

describe("Wizards", () => {
  it("lists the available guided flows", () => {
    render(withUI(<Wizards />));
    expect(screen.getByText("Connect a provider")).toBeTruthy();
    expect(screen.getByText("Create an agent")).toBeTruthy();
    expect(screen.getByText("Schedule a task")).toBeTruthy();
    expect(screen.getByText("Create a cron trigger for an agent wake, workflow, system task, or tool call.")).toBeTruthy();
  });

  it("opens a wizard overlay on click and closes it", () => {
    render(withUI(<Wizards />));
    // Click the "Create an agent" card (the embedded form fetches nothing).
    fireEvent.click(screen.getByText("Create an agent"));
    const close = screen.getByLabelText("Close wizard");
    expect(close).toBeTruthy();
    fireEvent.click(close);
    expect(screen.queryByLabelText("Close wizard")).toBeNull();
  });
});
