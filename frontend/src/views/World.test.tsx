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

import { WorldAddForm, WorldRelateForm, WorldEditForm } from "@/views/World";
import { UIProvider } from "@/components/ui/feedback";

function withUI(node: ReactNode) {
  return <UIProvider>{node}</UIProvider>;
}

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
  postJSON.mockResolvedValue({ id: "ent-1" });
  postAction.mockReset();
  postAction.mockResolvedValue({ id: "rel-1" });
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

describe("WorldRelateForm", () => {
  it("defaults to the first two entities and a relates_to verb, posting them", async () => {
    const onRelated = vi.fn();
    render(withUI(<WorldRelateForm names={["Ada", "agezt"]} onRelated={onRelated} />));
    fireEvent.click(screen.getByRole("button", { name: /Relate/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/world/relate", { from: "Ada", verb: "relates_to", to: "agezt" }),
    );
    await waitFor(() => expect(onRelated).toHaveBeenCalled());
  });

  it("honours chosen from/verb/to", async () => {
    render(withUI(<WorldRelateForm names={["Ada", "agezt", "Acme"]} onRelated={() => {}} />));
    fireEvent.change(screen.getByLabelText("Relation from"), { target: { value: "Ada" } });
    fireEvent.change(screen.getByLabelText("Relation verb"), { target: { value: "owns" } });
    fireEvent.change(screen.getByLabelText("Relation to"), { target: { value: "agezt" } });
    fireEvent.click(screen.getByRole("button", { name: /Relate/ }));
    await waitFor(() =>
      expect(postAction).toHaveBeenCalledWith("/api/world/relate", { from: "Ada", verb: "owns", to: "agezt" }),
    );
  });

  it("disables Relate when from and to are the same", () => {
    render(withUI(<WorldRelateForm names={["Solo"]} onRelated={() => {}} />));
    // Only one name → from and to both "Solo" → invalid.
    expect((screen.getByRole("button", { name: /Relate/ }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("WorldEditForm (M730)", () => {
  const entity = {
    id: "ent-42",
    kind: "person",
    name: "Ada",
    aliases: ["the boss"],
    attrs: { brief: "morning", tz: "UTC" },
  };

  it("prefills aliases and attribute rows from the entity", () => {
    render(withUI(<WorldEditForm entity={entity} onSaved={() => {}} />));
    expect((screen.getByLabelText("Edit entity aliases") as HTMLInputElement).value).toBe("the boss");
    expect((screen.getByLabelText("Attribute key 1") as HTMLInputElement).value).toBe("brief");
    expect((screen.getByLabelText("Attribute value 1") as HTMLInputElement).value).toBe("morning");
    expect((screen.getByLabelText("Attribute key 2") as HTMLInputElement).value).toBe("tz");
  });

  it("posts the replaced aliases + attrs (split, trimmed, blanks dropped) to world/edit", async () => {
    const onSaved = vi.fn();
    render(withUI(<WorldEditForm entity={entity} onSaved={onSaved} />));
    fireEvent.change(screen.getByLabelText("Edit entity aliases"), { target: { value: " ada k ,  , the lead " } });
    // Change brief, blank out tz's value (should be dropped).
    fireEvent.change(screen.getByLabelText("Attribute value 1"), { target: { value: "evening, terse" } });
    fireEvent.change(screen.getByLabelText("Attribute value 2"), { target: { value: "  " } });
    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/world/edit", {
        id: "ent-42",
        aliases: ["ada k", "the lead"],
        attrs: { brief: "evening, terse" },
      }),
    );
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
  });

  it("adds and removes attribute rows", async () => {
    render(withUI(<WorldEditForm entity={{ id: "e1", name: "X", attrs: {} }} onSaved={() => {}} />));
    // No rows yet → the empty hint shows.
    expect(screen.getByText(/add a preference or constraint/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /add attribute/ }));
    fireEvent.change(screen.getByLabelText("Attribute key 1"), { target: { value: "role" } });
    fireEvent.change(screen.getByLabelText("Attribute value 1"), { target: { value: "owner" } });
    fireEvent.click(screen.getByRole("button", { name: /Save/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/world/edit", { id: "e1", aliases: [], attrs: { role: "owner" } }),
    );
  });
});
