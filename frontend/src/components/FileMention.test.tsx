// @vitest-environment jsdom
import { describe, expect, it, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { FileMention } from "./FileMention";

// FileMention renders a clickable chip for paths the markdown parser flagged
// as `t: "file"`. Unsafe paths (parser defense-in-depth) render inert. The
// modal that the click opens pulls bytes from /api/files/raw, which 404s
// pre-Slice 5; that's the expected fallback path.

describe("FileMention", () => {
  afterEach(cleanup);

  it("renders a safe path as a clickable chip showing the leaf name", () => {
    render(<FileMention path="notes/README.md" />);
    const btn = screen.getByRole("button") as HTMLButtonElement;
    expect(btn.textContent).toContain("README.md");
    expect(btn.getAttribute("title")).toBe("Open notes/README.md in the file viewer");
  });

  it("renders an unsafe path as inert, struck-through text", () => {
    render(<FileMention path="/etc/passwd" />);
    expect(screen.queryByRole("button")).toBeNull();
    const span = screen.getByText("/etc/passwd") as HTMLSpanElement;
    expect(span.className).toMatch(/line-through/);
  });

  it("clicking the chip opens a modal with the file header and 'file manager' jump", () => {
    render(<FileMention path="kernel/agent/agent.go" />);
    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByText("kernel/agent/agent.go")).toBeTruthy();
    expect(screen.getByTitle(/file manager/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: /close detail/i })).toBeTruthy();
  });
});
