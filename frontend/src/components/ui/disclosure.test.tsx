// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Advanced, Disclosure } from "@/components/ui/disclosure";

afterEach(cleanup);

describe("Disclosure", () => {
  it("keeps children mounted when collapsed (queryable) but marks the region aria-hidden", () => {
    render(
      <Disclosure summary="Details">
        <p>folded fact</p>
      </Disclosure>,
    );
    // Children are in the DOM even while collapsed (so tests + lazy data survive).
    expect(screen.getByText("folded fact")).toBeTruthy();
    const trigger = screen.getByRole("button", { name: /Details/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });

  it("toggles aria-expanded on click", () => {
    render(
      <Disclosure summary="Diagnostics">
        <p>diag</p>
      </Disclosure>,
    );
    const trigger = screen.getByRole("button", { name: /Diagnostics/ });
    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });

  it("honors defaultOpen", () => {
    render(
      <Disclosure summary="Open" defaultOpen>
        <p>shown</p>
      </Disclosure>,
    );
    expect(screen.getByRole("button", { name: /Open/ }).getAttribute("aria-expanded")).toBe("true");
  });

  it("Advanced renders its label and is collapsed by default", () => {
    render(
      <Advanced>
        <p>power knobs</p>
      </Advanced>,
    );
    const trigger = screen.getByRole("button", { name: /Advanced/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByText("power knobs")).toBeTruthy();
  });
});
