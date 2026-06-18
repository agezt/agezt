// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import {
  notifyEventClassify,
  notifyEnabled,
  setNotifyEnabled,
} from "@/lib/notify";
import type { AgentEvent } from "@/lib/events";

const ev = (
  kind: string,
  payload: any = {},
  extra: Partial<AgentEvent> = {},
): AgentEvent => ({ kind, payload, ...extra }) as AgentEvent;

describe("notifyEventClassify (M919)", () => {
  it("notifies for the high-signal set, with the right view to open", () => {
    expect(
      notifyEventClassify(
        ev("approval.requested", { capability: "shell.exec", id: "ap1" }),
      ),
    ).toMatchObject({
      title: "Approval needed",
      body: "shell.exec",
      tag: "approval-ap1",
      hash: "approvals",
    });
    expect(
      notifyEventClassify(
        ev("task.failed", { reason: "boom" }, { correlation_id: "c1" }),
      ),
    ).toMatchObject({
      title: "Run failed",
      body: "boom",
      tag: "fail-c1",
      hash: "runs",
    });
    expect(notifyEventClassify(ev("halt", { reason: "manual" }))).toMatchObject(
      { title: "Daemon halted", hash: "alerts" },
    );
    expect(notifyEventClassify(ev("budget.exceeded"))).toMatchObject({
      title: "Budget ceiling exceeded",
      hash: "budget",
    });
    expect(
      notifyEventClassify(
        ev(
          "info",
          {
            agent: "builder",
            mode: "degraded",
            phase: "failed",
            error: "provider timeout",
          },
          { subject: "doctor.auto_repair" },
        ),
      ),
    ).toMatchObject({
      title: "Doctor run failed",
      body: "builder: provider timeout",
      hash: "alerts",
    });
    expect(
      notifyEventClassify(
        ev(
          "info",
          {
            agent: "builder",
            phase: "delegation_failed",
            reason: "target agent infra-lead is paused",
            root_agent: "builder",
            chain_depth: 1,
            delegate_to: "infra-lead",
          },
          { subject: "doctor.auto_repair" },
        ),
      ),
    ).toMatchObject({
      title: "Delegated wake failed",
      body:
        "builder: target agent infra-lead is paused · root builder · hop 1 · next infra-lead",
      hash: "alerts",
    });
    expect(
      notifyEventClassify(
        ev(
          "info",
          {
            agent: "builder",
            mode: "routing_forced_exhausted",
            phase: "routing_force_exhausted_detected",
            reason:
              "owner-forced chain stayed under pressure after repeated generations",
            root_agent: "builder",
            chain_depth: 2,
            target_agent: "lead",
          },
          { subject: "doctor.auto_repair" },
        ),
      ),
    ).toMatchObject({
      title: "Forced chain exhausted",
      body:
        "builder: owner-forced chain stayed under pressure after repeated generations · root builder · hop 2 · next lead",
      hash: "alerts",
    });
  });

  it("ignores routine events (no interruption for the firehose)", () => {
    for (const k of [
      "llm.token",
      "tool.result",
      "task.completed",
      "approval.granted",
      "capability.rejected",
    ]) {
      expect(notifyEventClassify(ev(k))).toBeNull();
    }
  });

  it("falls back to a sensible body when the payload is sparse", () => {
    expect(
      notifyEventClassify(ev("approval.requested", { tool_name: "browser" }))
        ?.body,
    ).toBe("browser");
    expect(notifyEventClassify(ev("task.failed"))?.body).toBe("A run failed.");
  });
});

describe("notify prefs (M919)", () => {
  beforeEach(() => localStorage.clear());
  it("defaults off and round-trips the opt-in", () => {
    expect(notifyEnabled()).toBe(false);
    setNotifyEnabled(true);
    expect(notifyEnabled()).toBe(true);
    setNotifyEnabled(false);
    expect(notifyEnabled()).toBe(false);
  });
});
