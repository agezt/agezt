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

import { WorldAddForm } from "@/views/World";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
  postJSON.mockResolvedValue({ id: "ent-1" });
});

describe("WorldAddForm", () => {
  it("disables Add until a name is entered", () => {
    render(withUI(<WorldAddForm onAdded={() => {}} />));
    expect((screen.getByRole("button", { name: /Add entity/ }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Entity name"), { target: { value: "Acme" } });
    expect((screen.getByRole("button", { name: /Add entity/ }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("posts name + default kind (person), trimmed, and calls onAdded", async () => {
    const onAdded = vi.fn();
    render(withUI(<WorldAddForm onAdded={onAdded} />));
    fireEvent.change(screen.getByLabelText("Entity name"), { target: { value: "  Ada Lovelace  " } });
    fireEvent.click(screen.getByRole("button", { name: /Add entity/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/world/add", { name: "Ada Lovelace", kind: "person" }));
    await waitFor(() => expect(onAdded).toHaveBeenCalled());
  });

  it("honours a chosen kind", async () => {
    render(withUI(<WorldAddForm onAdded={() => {}} />));
    fireEvent.change(screen.getByLabelText("Entity name"), { target: { value: "agezt" } });
    fireEvent.change(screen.getByLabelText("Entity kind"), { target: { value: "repo" } });
    fireEvent.click(screen.getByRole("button", { name: /Add entity/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/world/add", { name: "agezt", kind: "repo" }));
  });
});
