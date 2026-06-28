// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import { Data, dataLakeActorAgent, dataLakeAgents, dataRecordAttribution, dataRecordWriter, filterDataRecordsByAgent } from "@/views/Data";
import { UIProvider } from "@/components/ui/feedback";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postAction.mockReset();
  getJSON.mockImplementation((path: string) => {
    if (path === "/api/data/collections") {
      return Promise.resolve({
        collections: [{ name: "notes", title: "Notes", fields: [{ name: "title" }, { name: "body" }], count: 3 }],
      });
    }
    if (path === "/api/data/records") {
      return Promise.resolve({
        records: [
          { id: "r1", fields: { title: "Ops note", body: "disk" }, created_by: "ops:corr-1", created_ms: 1000 },
          { id: "r2", fields: { title: "Research note", body: "paper" }, created_by: "researcher:corr-2", updated_by: "ops:corr-3", updated_ms: 2000 },
          { id: "r3", fields: { title: "Planner note", body: "roadmap" }, created_by: "planner" },
        ],
      });
    }
    return Promise.resolve({});
  });
});

describe("data lake provenance helpers", () => {
  const records = [
    { id: "r1", fields: {}, created_by: "ops:corr-1" },
    { id: "r2", fields: {}, created_by: "researcher:corr-2", updated_by: "ops:corr-3" },
    { id: "r3", fields: {}, created_by: "planner" },
  ];

  it("lists and filters records by creating/updating agent", () => {
    expect(dataLakeAgents(records)).toEqual(["ops", "planner", "researcher"]);
    expect(filterDataRecordsByAgent(records, "ops").map((r) => r.id)).toEqual(["r1", "r2"]);
    expect(filterDataRecordsByAgent(records, "").map((r) => r.id)).toEqual(["r1", "r2", "r3"]);
  });

  it("formats the visible writer and detailed attribution", () => {
    expect(dataLakeActorAgent("researcher:corr-2")).toBe("researcher");
    expect(dataRecordWriter({ created_by: "researcher", updated_by: "ops" })).toBe("ops");
    expect(dataRecordWriter({})).toBe("unknown");
    expect(dataRecordAttribution({ created_by: "researcher", created_ms: 1000, updated_by: "ops", updated_ms: 2000 })).toContain("created by researcher");
    expect(dataRecordAttribution({ created_by: "researcher", created_ms: 1000, updated_by: "ops", updated_ms: 2000 })).toContain("updated by ops");
  });
});

describe("Data view writer filter", () => {
  it("filters visible records by agent provenance", async () => {
    render(withUI(<Data />));
    await waitFor(() => expect(screen.getByText("Ops note")).toBeTruthy());
    expect(screen.getByText("Research note")).toBeTruthy();
    expect(screen.getByText("Planner note")).toBeTruthy();

    fireEvent.click(within(screen.getByRole("group", { name: "Filter records by agent" })).getByRole("button", { name: "ops" }));
    expect(screen.getByText("Ops note")).toBeTruthy();
    expect(screen.getByText("Research note")).toBeTruthy();
    expect(screen.queryByText("Planner note")).toBeNull();
  });

  it("adds a record through the modal editor", async () => {
    render(withUI(<Data />));
    await waitFor(() => expect(screen.getByText("Ops note")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: /add/i }));
    const dialog = await screen.findByRole("dialog", { name: /add notes record/i });
    fireEvent.change(within(dialog).getByLabelText("title"), { target: { value: "New note" } });
    fireEvent.change(within(dialog).getByLabelText("body"), { target: { value: "ship it" } });
    fireEvent.click(within(dialog).getByRole("button", { name: /add/i }));

    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/data/insert", {
        collection: "notes",
        record: { title: "New note", body: "ship it" },
      }),
    );
  });
});
