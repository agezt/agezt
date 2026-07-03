// @vitest-environment jsdom
//
// Markdown wires Monaco into CodeBlock and FileMention into the file token.
// We mock both heavy children to assert the contract: chat blocks render the
// editor with the right language from the fence label, file mentions render
// the chip, and code spans stay as plain <code>.

import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { Markdown } from "./Markdown";

const fakeMonacoView = vi.fn((props: { value?: string; path?: string; readOnly?: boolean }) => (
  <pre data-testid="monaco-stub" data-lang-path={props.path || ""}>
    {props.value}
  </pre>
));

const fakeFileMention = vi.fn((props: { path: string }) => (
  <button data-testid="file-mention-stub" data-path={props.path} type="button">
    {props.path}
  </button>
));

vi.mock("./MonacoView", () => ({
  MonacoView: (props: Parameters<typeof fakeMonacoView>[0]) => fakeMonacoView(props),
}));

vi.mock("./FileMention", () => ({
  FileMention: (props: { path: string }) => fakeFileMention(props),
}));

beforeEach(() => {
  fakeMonacoView.mockClear();
  fakeFileMention.mockClear();
});
afterEach(cleanup);

describe("Markdown", () => {
  it("renders a fenced code block via MonacoView and threads the language tag", () => {
    render(<Markdown source={"```go\npackage main\nfunc main(){}\n```"} />);
    const stub = screen.getByTestId("monaco-stub");
    expect(stub.textContent).toContain("package main");
    // The Monaco stub's data-lang-path is what we synthesised from the fence.
    expect(stub.getAttribute("data-lang-path")).toBe("snippet.go");
  });

  it("renders an inline file mention as a FileMention chip", () => {
    render(<Markdown source={"see notes/README.md for details"} />);
    const chip = screen.getByTestId("file-mention-stub");
    expect(chip.getAttribute("data-path")).toBe("notes/README.md");
  });

  it("leaves inline code spans as plain <code> (Monaco is only for fenced blocks)", () => {
    const { container } = render(<Markdown source={"run `npm test` now"} />);
    expect(container.querySelectorAll("code").length).toBeGreaterThan(0);
    // No Monaco stub for inline `npm test`.
    expect(screen.queryAllByTestId("monaco-stub").length).toBe(0);
  });

  it("renders prose without touching either heavy child", () => {
    render(<Markdown source={"no code here, just words"} />);
    expect(screen.queryAllByTestId("monaco-stub").length).toBe(0);
    expect(screen.queryAllByTestId("file-mention-stub").length).toBe(0);
  });
});
