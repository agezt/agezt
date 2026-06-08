// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { DataView, ToolOutput } from "@/components/DataView";

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

describe("ToolOutput", () => {
  it("renders a JSON object tool result as a widget (key/value), not raw text", () => {
    render(<ToolOutput text={'{"count":4,"capped":false}'} />);
    expect(screen.getByText("count")).toBeTruthy();
    expect(screen.getByText("4")).toBeTruthy();
    expect(document.querySelector("pre")).toBeNull();
  });

  it("renders a JSON array tool result as a table", () => {
    render(<ToolOutput text={'[{"name":"a.txt"},{"name":"b.txt"}]'} />);
    expect(document.querySelector("table")).toBeTruthy();
    expect(screen.getByText("a.txt")).toBeTruthy();
  });

  it("keeps non-JSON output as raw text", () => {
    render(<ToolOutput text={"wrote 3 bytes to a.txt"} />);
    expect(document.querySelector("pre")).toBeTruthy();
    expect(screen.getByText("wrote 3 bytes to a.txt")).toBeTruthy();
  });
});
