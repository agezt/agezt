// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { KeyListItem } from "./KeyListItem";

afterEach(cleanup);

describe("KeyListItem", () => {
  it("shows the active badge when key is active", () => {
    render(
      <ul>
        <KeyListItem keyInfo={{ label: "work", active: true, last4: "…1111" }} />
      </ul>,
    );
    expect(screen.getByText("active")).toBeTruthy();
    expect(screen.getByText("work")).toBeTruthy();
    expect(screen.getByText("…1111")).toBeTruthy();
    expect(screen.queryByText("activate")).toBeNull();
  });

  it("shows the activate button when key is inactive and fires onActivate", () => {
    const onActivate = vi.fn();
    render(
      <ul>
        <KeyListItem
          keyInfo={{ label: "personal", active: false, last4: "…2222" }}
          onActivate={onActivate}
        />
      </ul>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Activate personal" }));
    expect(onActivate).toHaveBeenCalledOnce();
  });

  it("shows the remove button when onRemove is provided and fires it", () => {
    const onRemove = vi.fn();
    render(
      <ul>
        <KeyListItem keyInfo={{ label: "old", active: false, last4: "…9999" }} onRemove={onRemove} />
      </ul>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Remove old" }));
    expect(onRemove).toHaveBeenCalledOnce();
  });

  it("disables buttons when busy", () => {
    const onActivate = vi.fn();
    const onRemove = vi.fn();
    render(
      <ul>
        <KeyListItem
          keyInfo={{ label: "x", active: false, last4: "…0000" }}
          busy
          onActivate={onActivate}
          onRemove={onRemove}
        />
      </ul>,
    );
    expect((screen.getByRole("button", { name: "Activate x" }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Remove x" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
