// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { AgentAvatar } from "@/components/AgentAvatar";

afterEach(cleanup);

describe("AgentAvatar", () => {
  it("renders the agent monogram", () => {
    render(<AgentAvatar slug="researcher" name="The Researcher" />);
    expect(screen.getByText("TR")).toBeTruthy();
  });

  it("falls back to the slug for the monogram", () => {
    render(<AgentAvatar slug="ops" />);
    expect(screen.getByText("OP")).toBeTruthy();
  });

  it("shows a status dot only when running", () => {
    const { container, rerender } = render(<AgentAvatar slug="a" status="running" />);
    expect(container.querySelector(".work-pulse")).toBeTruthy();
    rerender(<AgentAvatar slug="a" />);
    expect(container.querySelector(".work-pulse")).toBeNull();
  });

  it("shows quieter markers for sleeping and paused agents", () => {
    const { container, rerender } = render(<AgentAvatar slug="a" status="sleeping" />);
    expect(container.querySelector(".bg-muted")).toBeTruthy();
    rerender(<AgentAvatar slug="a" status="paused" />);
    expect(container.querySelector(".bg-warn")).toBeTruthy();
  });

  it("desaturates a retired agent", () => {
    const { container } = render(<AgentAvatar slug="a" status="retired" />);
    expect(container.querySelector(".grayscale")).toBeTruthy();
  });
});
