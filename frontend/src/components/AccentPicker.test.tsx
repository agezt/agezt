// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { AccentPicker } from "@/components/AccentPicker";

beforeEach(() => {
  localStorage.clear();
  document.documentElement.style.removeProperty("--accent-hue");
});
afterEach(cleanup);

describe("AccentPicker", () => {
  it("opens the palette and recolours + persists on pick", () => {
    render(<AccentPicker />);
    fireEvent.click(screen.getByLabelText("Accent colour"));
    // Pick "Green" (hue 150).
    fireEvent.click(screen.getByLabelText("Green"));
    expect(document.documentElement.style.getPropertyValue("--accent-hue")).toBe("150");
    expect(localStorage.getItem("agezt-accent-hue")).toBe("150");
  });

  it("marks the active accent as pressed", () => {
    localStorage.setItem("agezt-accent-hue", "150");
    render(<AccentPicker />);
    fireEvent.click(screen.getByLabelText("Accent colour"));
    expect(screen.getByLabelText("Green").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByLabelText("Blue").getAttribute("aria-pressed")).toBe("false");
  });
});
