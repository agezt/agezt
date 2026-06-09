// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { FallbackNote } from "@/views/Chat";

afterEach(cleanup);

describe("FallbackNote", () => {
  it("shows a single hop as from → to", () => {
    render(<FallbackNote hops={[{ from: "deepseek-chat", to: "gpt-4o" }]} />);
    expect(screen.getByText("fell back")).toBeTruthy();
    expect(screen.getByText("deepseek-chat → gpt-4o")).toBeTruthy();
  });

  it("collapses multiple hops into one path and counts them", () => {
    render(
      <FallbackNote
        hops={[
          { from: "a", to: "b" },
          { from: "b", to: "c" },
        ]}
      />,
    );
    expect(screen.getByText("fell back 2×")).toBeTruthy();
    expect(screen.getByText("a → b → c")).toBeTruthy();
  });
});
