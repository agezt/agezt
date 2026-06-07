// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { JsonView, KeyValue, Muted, ErrorText } from "@/components/JsonView";

afterEach(cleanup);

describe("JsonView", () => {
  it("pretty-prints the value into a <pre>", () => {
    const { container } = render(<JsonView value={{ a: 1, b: "x" }} />);
    const pre = container.querySelector("pre");
    expect(pre).not.toBeNull();
    expect(pre!.textContent).toContain('"a": 1');
    expect(pre!.textContent).toContain('"b": "x"');
  });
});

describe("KeyValue", () => {
  it("renders each pair as a term + definition", () => {
    const { container } = render(
      <KeyValue
        pairs={[
          ["model", "mock"],
          ["status", <span key="s">ok</span>],
        ]}
      />,
    );
    expect(screen.getByText("model")).toBeTruthy();
    expect(screen.getByText("mock")).toBeTruthy();
    expect(container.querySelectorAll("dt")).toHaveLength(2);
    expect(container.querySelectorAll("dd")).toHaveLength(2);
  });
});

describe("Muted / ErrorText", () => {
  it("render their children with the right tone class", () => {
    render(
      <>
        <Muted>quiet</Muted>
        <ErrorText>boom</ErrorText>
      </>,
    );
    expect(screen.getByText("quiet").className).toContain("text-muted");
    expect(screen.getByText("boom").className).toContain("text-bad");
  });
});
