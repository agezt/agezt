// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { RunDetailCards, ToolCallRow, runPhaseSteps } from "@/components/RunDetail";
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

describe("runPhaseSteps", () => {
  it("shows wake provenance on the started phase", () => {
    const steps = runPhaseSteps([
      {
        seq: 1,
        kind: "task.received",
        payload: {
          intent: "check disks",
          wake_source: "schedule",
          wake_reason: "intent",
          schedule_id: "sched-ops",
        },
      },
    ]);
    expect(steps[0].phase).toBe("started");
    expect(steps[0].detail).toContain("check disks");
    expect(steps[0].detail).toContain("source: schedule");
    expect(steps[0].detail).toContain("schedule: sched-ops");
  });

  it("shows retry attempt and backoff details", () => {
    const steps = runPhaseSteps([
      {
        seq: 1,
        kind: "agent.retry",
        payload: {
          next_attempt: 2,
          max_attempts: 4,
          delay_ms: 125_000,
          backoff: "exponential",
          retry_on: ["error", "timeout"],
          reason: "timeout",
        },
      },
    ]);

    expect(steps[0].phase).toBe("retrying");
    expect(steps[0].detail).toBe("attempt 2/4 · wait 2m 5s · backoff exponential · retry_on error,timeout · timeout");
  });

  it("shows sub-agent delegation phases", () => {
    const steps = runPhaseSteps([
      {
        seq: 1,
        kind: "subagent.spawned",
        payload: { task: "fetch docs", child_correlation: "sub", depth: 1, agent: "researcher", async: true },
      },
      {
        seq: 2,
        kind: "subagent.completed",
        payload: { child_correlation: "sub", ok: false, async: true, error: "cancelled" },
      },
    ]);
    expect(steps[0].phase).toBe("delegating");
    expect(steps[0].detail).toContain("fetch docs");
    expect(steps[0].detail).toContain("child: sub");
    expect(steps[0].detail).toContain("async");
    expect(steps[1].phase).toBe("delegation failed");
    expect(steps[1].detail).toContain("cancelled");
  });
});

describe("RunDetailCards", () => {
  it("summarizes delegated sub-agent counts", () => {
    render(
      <RunDetailCards
        arc={[
          { seq: 1, kind: "subagent.spawned", payload: { child_correlation: "sub", task: "fetch docs" } },
          { seq: 2, kind: "subagent.completed", payload: { child_correlation: "sub", ok: true, async: true, chars: 128 } },
          { seq: 3, kind: "subagent.completed", payload: { child_correlation: "sub2", ok: false, async: true, error: "cancelled" } },
        ]}
      />,
    );
    expect(screen.getByText("delegations")).toBeTruthy();
    expect(screen.getByText("1 spawned / 1 completed / 1 failed")).toBeTruthy();
  });
});
