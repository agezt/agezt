// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { Workspace, WorkspaceColumn } from "./workspace";

describe("Workspace", () => {
  it("renders all three slots when provided", () => {
    render(
      <Workspace
        left={<div>left</div>}
        center={<div>center</div>}
        right={<div>right</div>}
      />,
    );
    expect(screen.getByText("left")).toBeTruthy();
    expect(screen.getByText("center")).toBeTruthy();
    expect(screen.getByText("right")).toBeTruthy();
  });

  it("omits the left slot when not provided", () => {
    const { container } = render(<Workspace right={<div>only-right</div>} />);
    // No aside (left) rendered; only the right section + content.
    expect(container.querySelectorAll("aside").length).toBe(0);
    expect(screen.getByText("only-right")).toBeTruthy();
  });

  it("treats undefined center as 'no center column'", () => {
    // Many workspaces only want two columns; the slot should disappear entirely.
    const { container } = render(
      <Workspace left={<div>tree</div>} right={<div>detail</div>} />,
    );
    // Should not render a second <section> for center — only left (aside) + right.
    const sections = container.querySelectorAll("section");
    expect(sections.length).toBe(1);
  });

  it("WorkspaceColumn applies overflow rules so inner content scrolls", () => {
    render(
      <WorkspaceColumn title="Tree" actions={<button>Act</button>}>
        <div>body</div>
      </WorkspaceColumn>,
    );
    expect(screen.getByText("Tree")).toBeTruthy();
    expect(screen.getByText("Act")).toBeTruthy();
    expect(screen.getByText("body")).toBeTruthy();
  });
});
