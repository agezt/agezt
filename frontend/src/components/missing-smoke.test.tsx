// @vitest-environment jsdom
import { describe, expect, it, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { AdvancedToggle } from "@/components/AdvancedToggle";
import { ModelChip, HealthDot } from "@/components/ModelChip";
import { Panel } from "@/components/Panel";
import { Markdown } from "@/components/Markdown";
import { setAdvanced } from "@/lib/advanced";

beforeEach(() => {
  localStorage.clear();
  setAdvanced(false);
});
afterEach(cleanup);

describe("previously untested component smoke coverage", () => {
  it("AdvancedToggle flips global advanced mode", () => {
    render(<AdvancedToggle />);
    const button = screen.getByLabelText("Toggle advanced mode");
    expect(button.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(button);
    expect(button.getAttribute("aria-pressed")).toBe("true");
    expect(document.documentElement.classList.contains("advanced")).toBe(true);
  });

  it("ModelChip renders chain references and model health dots", () => {
    render(
      <div>
        <ModelChip id="@fast" chains={{ fast: ["m1", "m2"] }} />
        <ModelChip id="m1" cat={{ models: [{ id: "m1", provider: "p", keyed: true }] } as any} />
        <HealthDot status="unknown" />
      </div>,
    );
    expect(screen.getByText("fast")).toBeTruthy();
    expect(screen.getByText("m1")).toBeTruthy();
    expect(screen.getAllByLabelText(/model health:/).length).toBeGreaterThanOrEqual(2);
  });

  it("Panel shows skeleton before async data resolves", () => {
    render(<Panel title="Config" path="/api/config">{() => <span>loaded</span>}</Panel>);
    expect(screen.getByText("Config")).toBeTruthy();
    expect(screen.queryByText("loaded")).toBeNull();
  });

  it("Markdown escapes raw HTML and renders fenced code", () => {
    render(<Markdown source={'hello <img src=x onerror=alert(1)>\n\n```sh\necho ok\n```'} />);
    expect(screen.getByText(/hello <img/)).toBeTruthy();
    expect(screen.getByText("echo ok")).toBeTruthy();
    expect(document.querySelector("img")).toBeNull();
  });
});
