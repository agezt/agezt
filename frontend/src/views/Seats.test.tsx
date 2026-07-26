// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const confirm = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
}));
vi.mock("@/components/ui/feedback", () => ({
  useUI: () => ({ confirm: (...a: unknown[]) => confirm(...a) }),
}));

import { Seats, seatIsoLabel } from "@/views/Seats";

const seats = [
  { id: "default", name: "Default", description: "inherit", builtin: true },
  { id: "isolated", name: "Isolated", description: "sandboxed", execution_profile: "warden", builtin: true },
  { id: "gpu-box", name: "GPU Box", description: "gpu work", execution_profile: "container", builtin: false },
];

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postJSON.mockResolvedValue({});
  confirm.mockReset();
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/seats") return Promise.resolve({ seats, count: seats.length });
    return Promise.reject(new Error(`unexpected ${path}`));
  });
});

describe("Seats helpers", () => {
  it("labels isolation, defaulting empty to tool defaults", () => {
    expect(seatIsoLabel({ id: "a" })).toBe("tool defaults");
    expect(seatIsoLabel({ id: "b", execution_profile: "warden" })).toBe("warden");
  });
});

describe("Seats view", () => {
  it("groups built-in vs custom and shows isolation", async () => {
    render(<Seats />);
    await waitFor(() => expect(screen.getByText("GPU Box")).toBeTruthy());
    expect(screen.getByText("Built-in")).toBeTruthy();
    expect(screen.getByText("Custom")).toBeTruthy();
    // "warden"/"container" appear both as a badge and a select option — just
    // confirm they render at all.
    expect(screen.getAllByText("warden").length).toBeGreaterThan(0);
    expect(screen.getAllByText("container").length).toBeGreaterThan(0);
  });

  it("creates a custom seat and guards deletion with impact context", async () => {
    render(<Seats />);
    await waitFor(() => expect(screen.getByText("GPU Box")).toBeTruthy());

    fireEvent.change(screen.getByLabelText("Seat id"), { target: { value: "fast-box" } });
    fireEvent.change(screen.getByLabelText("Seat isolation"), { target: { value: "local" } });
    fireEvent.click(screen.getByText("Add seat"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/seats/create", expect.objectContaining({ id: "fast-box", execution_profile: "local" })),
    );

    // Only the custom seat exposes a remove control.
    confirm.mockResolvedValue(false);
    fireEvent.click(screen.getByTitle("Remove seat"));
    await waitFor(() => expect(confirm).toHaveBeenCalledWith(expect.objectContaining({
      title: "Remove seat GPU Box?",
      target: "Seat gpu-box",
      impact: expect.stringContaining("Existing tasks"),
      recovery: expect.stringContaining("no automatic undo"),
      confirmLabel: "Remove seat",
      danger: true,
    })));
    expect(postJSON).not.toHaveBeenCalledWith("/api/seats/delete", expect.anything());

    confirm.mockResolvedValue(true);
    fireEvent.click(screen.getByTitle("Remove seat"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/seats/delete", expect.objectContaining({ id: "gpu-box" })),
    );
  });
});
