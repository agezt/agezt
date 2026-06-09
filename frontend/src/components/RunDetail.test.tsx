// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { ToolCallRow } from "@/components/RunDetail";
import type { ToolCall } from "@/lib/rundetail";

afterEach(cleanup);

describe("ToolCallRow", () => {
  it("expands to reveal the arguments and result of a tool call", () => {
    const c: ToolCall = {
      callId: "c1",
      tool: "code_exec",
      capability: "code.exec",
      allow: true,
      input: '{"language":"python","code":"print(42)"}',
      output: "[code_exec] language=python isolation=none\n42",
    };
    render(
      <ul>
        <ToolCallRow c={c} />
      </ul>,
    );
    // Collapsed: tool name + verdict show; the detail labels do not.
    expect(screen.getByText("code_exec")).toBeTruthy();
    expect(screen.getByText("code.exec")).toBeTruthy();
    expect(screen.queryByText("Arguments")).toBeNull();

    // Click expands → Arguments + Result, including the actual code and output.
    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByText("Arguments")).toBeTruthy();
    expect(screen.getByText("Result")).toBeTruthy();
    expect(screen.getByText(/print\(42\)/)).toBeTruthy();
  });

  it("labels the detail 'Error' when the tool call errored", () => {
    const c: ToolCall = { callId: "e1", tool: "shell", allow: true, error: true, output: "boom\n[exit code 1]" };
    render(
      <ul>
        <ToolCallRow c={c} />
      </ul>,
    );
    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByText("Error")).toBeTruthy();
    expect(screen.queryByText("Result")).toBeNull();
  });

  it("is not expandable when there is no args/output trace (e.g. a hard-denied call)", () => {
    const c: ToolCall = { callId: "d1", tool: "http", allow: false, hardDenied: true };
    render(
      <ul>
        <ToolCallRow c={c} />
      </ul>,
    );
    // No detail → clicking the row does nothing (labels never appear).
    fireEvent.click(screen.getByRole("button"));
    expect(screen.queryByText("Arguments")).toBeNull();
    expect(screen.getByText("hard-denied")).toBeTruthy();
  });
});
