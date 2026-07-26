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

import { Taste, tasteScopeLabel } from "@/views/Taste";

const exemplars = [
  { id: "ex1", title: "Good PR summary", body: "One line what, one line why.", scope: "" },
  { id: "ex2", title: "Builder house style", body: "Terse, concrete.", scope: "builder", tags: ["code"] },
];

afterEach(cleanup);
beforeEach(() => {
  getJSON.mockReset();
  postJSON.mockReset();
  postJSON.mockResolvedValue({});
  confirm.mockReset();
});

describe("Taste helpers", () => {
  it("labels scope, defaulting blank to every run", () => {
    expect(tasteScopeLabel({ id: "a", scope: "" })).toBe("every run");
    expect(tasteScopeLabel({ id: "b", scope: "builder" })).toBe("builder");
  });
});

describe("Taste view", () => {
  it("renders exemplars and guards deletion with scope impact", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/taste") return Promise.resolve({ exemplars, count: exemplars.length });
      return Promise.reject(new Error(`unexpected ${path}`));
    });

    render(<Taste />);

    await waitFor(() => expect(screen.getByText("Good PR summary")).toBeTruthy());
    expect(screen.getByText("Builder house style")).toBeTruthy();
    expect(screen.getByText("every run")).toBeTruthy();
    expect(screen.getByText("builder")).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Exemplar title"), { target: { value: "New anchor" } });
    fireEvent.change(screen.getByLabelText("Exemplar body"), { target: { value: "sample" } });
    fireEvent.click(screen.getByText("Add exemplar"));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/taste/create", expect.objectContaining({ title: "New anchor", body: "sample" })),
    );

    confirm.mockResolvedValue(false);
    fireEvent.click(screen.getAllByTitle("Remove")[0]);
    await waitFor(() => expect(confirm).toHaveBeenCalledWith(expect.objectContaining({
      title: "Remove exemplar Good PR summary?",
      target: "Exemplar ex1",
      impact: expect.stringContaining("Future runs"),
      recovery: expect.stringContaining("no automatic undo"),
      confirmLabel: "Remove exemplar",
      danger: true,
    })));
    expect(postJSON).not.toHaveBeenCalledWith("/api/taste/delete", expect.anything());

    confirm.mockResolvedValue(true);
    fireEvent.click(screen.getAllByTitle("Remove")[0]);
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/taste/delete", expect.objectContaining({ id: "ex1" })),
    );
  });
});
