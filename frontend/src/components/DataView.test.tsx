// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { DataView } from "@/components/DataView";

afterEach(cleanup);

describe("DataView", () => {
  it("renders an array of objects as a table with the union of keys as columns", () => {
    render(
      <DataView
        data={[
          { name: "Gin", speed: "fast" },
          { name: "Echo", notes: "lean" },
        ]}
      />,
    );
    expect(document.querySelector("table")).toBeTruthy();
    // union columns: name, speed, notes
    for (const h of ["name", "speed", "notes"]) expect(screen.getByText(h)).toBeTruthy();
    expect(screen.getByText("Gin")).toBeTruthy();
    expect(screen.getByText("Echo")).toBeTruthy();
  });

  it("renders a plain object as a key/value card (not a table)", () => {
    render(<DataView data={{ count: 4, capped: false }} />);
    expect(document.querySelector("table")).toBeNull();
    expect(screen.getByText("count")).toBeTruthy();
    expect(screen.getByText("4")).toBeTruthy();
    expect(screen.getByText("false")).toBeTruthy();
  });

  it("renders an array of scalars as a list", () => {
    render(<DataView data={["alpha", "beta", "gamma"]} />);
    expect(document.querySelectorAll("li")).toHaveLength(3);
    expect(screen.getByText("beta")).toBeTruthy();
  });

  it("renders a scalar directly", () => {
    render(<DataView data={"just text"} />);
    expect(screen.getByText("just text")).toBeTruthy();
  });
});
