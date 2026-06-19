// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { Advanced, Calm } from "@/components/ui/advanced";
import { AdvancedToggle } from "@/components/AdvancedToggle";
import { setAdvanced } from "@/lib/advanced";

beforeEach(() => {
  localStorage.clear();
  setAdvanced(false);
});
afterEach(cleanup);

describe("Advanced / Calm gating", () => {
  it("shows Calm and hides Advanced by default, and the toggle flips both", () => {
    render(
      <>
        <AdvancedToggle />
        <Calm>
          <span>calm-summary</span>
        </Calm>
        <Advanced>
          <span>diagnostic-detail</span>
        </Advanced>
      </>,
    );
    expect(screen.queryByText("calm-summary")).toBeTruthy();
    expect(screen.queryByText("diagnostic-detail")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /toggle advanced mode/i }));
    expect(screen.queryByText("calm-summary")).toBeNull();
    expect(screen.queryByText("diagnostic-detail")).toBeTruthy();
  });
});
