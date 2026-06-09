// @vitest-environment jsdom
import { useState } from "react";
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { UIProvider, useUI } from "@/components/ui/feedback";

afterEach(cleanup);

// A tiny harness that exercises the public useUI() surface: fire a toast, and
// run a confirm() whose boolean result is reflected into the DOM so tests can
// assert how the modal resolved.
function Harness() {
  const ui = useUI();
  const [res, setRes] = useState<string>("");
  return (
    <div>
      <button onClick={() => ui.toast("saved it", "success")}>fire-toast</button>
      <button onClick={async () => setRes(String(await ui.confirm({ title: "Delete it?", danger: true })))}>
        ask
      </button>
      <span data-testid="res">{res}</span>
    </div>
  );
}

function renderHarness() {
  return render(
    <UIProvider>
      <Harness />
    </UIProvider>,
  );
}

describe("useUI", () => {
  it("throws when used outside <UIProvider>", () => {
    // Render the hook bare (no provider) and expect the guard to fire.
    expect(() => render(<Harness />)).toThrow(/UIProvider/);
  });
});

describe("toast", () => {
  it("shows the message text in a status region", () => {
    renderHarness();
    fireEvent.click(screen.getByText("fire-toast"));
    const toast = screen.getByRole("status");
    expect(toast.textContent).toContain("saved it");
  });

  it("dismisses when the close button is clicked", async () => {
    renderHarness();
    fireEvent.click(screen.getByText("fire-toast"));
    expect(screen.getByRole("status")).toBeTruthy();
    fireEvent.click(screen.getByTitle("Dismiss"));
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
  });
});

describe("confirm", () => {
  it("renders a modal dialog with the title", async () => {
    renderHarness();
    fireEvent.click(screen.getByText("ask"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeTruthy());
    expect(screen.getByText("Delete it?")).toBeTruthy();
  });

  it("resolves true when the confirm button is clicked", async () => {
    renderHarness();
    fireEvent.click(screen.getByText("ask"));
    await waitFor(() => screen.getByRole("dialog"));
    fireEvent.click(screen.getByText("Confirm"));
    await waitFor(() => expect(screen.getByTestId("res").textContent).toBe("true"));
    // Modal is gone after resolving.
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("resolves false when cancelled", async () => {
    renderHarness();
    fireEvent.click(screen.getByText("ask"));
    await waitFor(() => screen.getByRole("dialog"));
    fireEvent.click(screen.getByText("Cancel"));
    await waitFor(() => expect(screen.getByTestId("res").textContent).toBe("false"));
  });

  it("resolves false when Escape is pressed", async () => {
    renderHarness();
    fireEvent.click(screen.getByText("ask"));
    await waitFor(() => screen.getByRole("dialog"));
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() => expect(screen.getByTestId("res").textContent).toBe("false"));
  });
});
