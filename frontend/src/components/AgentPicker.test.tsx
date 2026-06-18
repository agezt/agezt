// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";

const getJSON = vi.fn();
vi.mock("@/lib/api", () => ({ getJSON: (...a: unknown[]) => getJSON(...a) }));

import { AgentPicker, agentDirectCallable } from "@/components/AgentPicker";

afterEach(cleanup);
beforeEach(() => getJSON.mockReset());

describe("AgentPicker", () => {
  it("derives direct-callability from the agent identity kind", () => {
    expect(agentDirectCallable({ slug: "researcher", enabled: true })).toBe(true);
    expect(agentDirectCallable({ slug: "worker", enabled: true, kind: "subagent" })).toBe(false);
    expect(agentDirectCallable({ slug: "worker", enabled: true, direct_callable: false })).toBe(false);
  });

  it("lists directly callable agents and picks one; paused or managed agents are hidden", async () => {
    getJSON.mockResolvedValue({
      profiles: [
        { slug: "researcher", name: "The Researcher", model: "m-1", enabled: true },
        { slug: "ops", enabled: false },
        { slug: "worker", enabled: true, managed: true, direct_callable: false },
        { slug: "planner-child", enabled: true, kind: "subagent" },
      ],
    });
    const onChange = vi.fn();
    render(<AgentPicker value="" onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Pick conversation agent" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Use agent researcher" })).toBeTruthy());
    expect(screen.queryByRole("button", { name: "Use agent ops" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Use agent worker" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Use agent planner-child" })).toBeNull();
    expect(getJSON).toHaveBeenCalledWith("/api/agents");
    fireEvent.click(screen.getByRole("button", { name: "Use agent researcher" }));
    expect(onChange).toHaveBeenCalledWith("researcher");
  });

  it("clears back to the default identity and shows the active slug on the trigger", async () => {
    getJSON.mockResolvedValue({ profiles: [{ slug: "researcher", enabled: true }] });
    const onChange = vi.fn();
    render(<AgentPicker value="researcher" onChange={onChange} />);
    const trigger = screen.getByRole("button", { name: "Pick conversation agent" });
    expect(trigger.textContent).toContain("researcher");
    fireEvent.click(trigger);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Use agent default identity" })).toBeTruthy(),
    );
    expect(screen.getByText("the daemon's default identity and model")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Use agent default identity" }));
    expect(onChange).toHaveBeenCalledWith("");
  });
});
