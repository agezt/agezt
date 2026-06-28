// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";

const getJSON = vi.fn();
const postJSON = vi.fn();
const postAction = vi.fn();
vi.mock("@/lib/api", () => ({
  getJSON: (...a: unknown[]) => getJSON(...a),
  postJSON: (...a: unknown[]) => postJSON(...a),
  postAction: (...a: unknown[]) => postAction(...a),
}));

import {
  NewScheduleForm,
  Schedules,
  parseSchedulesJSON,
  scheduleActionTitle,
  untilLabel,
  scheduleCounts,
  scheduleTargetCounts,
  scheduleTargetMixLabel,
  scheduleTargetLabel,
  filterScheduleItems,
  scheduleAttentionReasons,
  scheduleAttentionCount,
  scheduleNeedsAttention,
  scheduleFireMeta,
  scheduleAgentManaged,
  scheduleSelectedAgentIssue,
  scheduleToolAgentIssue,
  systemTaskExecutionLabel,
  systemTaskDisplayName,
  scheduleExecutionContract,
  scheduleRowExecutionContract,
  scheduleRowIntentContract,
  scheduleRowIntentLabel,
  scheduleRowPayloadContract,
  scheduleRuntimePassport,
  scheduleExecutorPassport,
  scheduleCronJobPassport,
  scheduleCronjobLedger,
  scheduleCronPassport,
  scheduleCommandStrip,
  scheduleTargetHealthPassport,
  scheduleFrequencyIssue,
  scheduleIntentFieldHint,
  schedulePayloadContract,
  scheduleIdentityBoundary,
  scheduleFormCadenceLabel,
  scheduleContractSummary,
  scheduleExecutionManifest,
  scheduleSystemTaskPresetLabel,
  DUE_SOON_MS,
} from "@/views/Schedules";
import { UIProvider } from "@/components/ui/feedback";
import type { ReactNode } from "react";

const withUI = (node: ReactNode) => <UIProvider>{node}</UIProvider>;

function chooseScheduleOption(group: string, name: RegExp | string) {
  fireEvent.click(within(screen.getByRole("group", { name: group })).getByRole("button", { name }));
}

function expectScheduleOptionSelected(group: string, name: RegExp | string) {
  expect(
    within(screen.getByRole("group", { name: group }))
      .getByRole("button", { name })
      .getAttribute("aria-pressed"),
  ).toBe("true");
}

function chooseScheduleUnit(group: string, unit: "minutes" | "hours") {
  fireEvent.click(within(screen.getByRole("group", { name: group })).getByRole("button", { name: unit }));
}

describe("untilLabel (M917)", () => {
  const now = 1_000_000_000_000;
  it("renders a coarse countdown, and overdue/now near zero", () => {
    expect(untilLabel(now - 5000, now)).toBe("overdue");
    expect(untilLabel(now + 5000, now)).toBe("now");
    expect(untilLabel(now + 45_000, now)).toBe("in 45s");
    expect(untilLabel(now + 12 * 60_000, now)).toBe("in 12m");
    expect(untilLabel(now + 3 * 3_600_000, now)).toBe("in 3h");
    expect(untilLabel(now + 2 * 24 * 3_600_000, now)).toBe("in 2d");
  });
});

describe("scheduleCounts (M917)", () => {
  const now = 1_000_000_000_000;
  it("tallies enabled/paused and due-within-the-hour (enabled only)", () => {
    const items = [
      { enabled: true, next_run_unix: (now + 10 * 60_000) / 1000 }, // due soon
      { enabled: true, next_run_unix: (now + 5 * 3_600_000) / 1000 }, // later
      { enabled: false, next_run_unix: (now + 60_000) / 1000 }, // paused → not due-soon
      { enabled: true }, // continuous/no next → enabled but not due-soon
    ];
    expect(scheduleCounts(items, now)).toEqual({ total: 4, enabled: 3, paused: 1, dueSoon: 1 });
    expect(DUE_SOON_MS).toBe(3_600_000);
  });
});

describe("schedule target/action labels", () => {
  it("classifies managed sub-agents from identity fields for schedule policy", () => {
    expect(scheduleAgentManaged({ kind: "subagent" })).toBe(true);
    expect(scheduleAgentManaged({ managed: true })).toBe(true);
    expect(scheduleAgentManaged({ direct_callable: false })).toBe(true);
    expect(scheduleAgentManaged({ kind: "custom" })).toBe(false);
    expect(scheduleSelectedAgentIssue("worker", [{ slug: "worker", kind: "subagent" }])).toBe("agent worker is a managed sub-agent");
    expect(scheduleSelectedAgentIssue("paused", [{ slug: "paused", enabled: false }])).toBe("agent paused is paused");
    expect(scheduleSelectedAgentIssue("ready", [{ slug: "ready", enabled: true }])).toBe("");
    expect(scheduleToolAgentIssue("shell", "ops", [{ slug: "ops", tool_deny: ["shell"] }])).toBe(
      "agent ops cannot schedule tool shell: agent tool denylist",
    );
    expect(scheduleToolAgentIssue("shell", "ops", [{ slug: "ops", tool_allow: ["memory"] }])).toBe(
      "agent ops cannot schedule tool shell: not in agent tool allowlist",
    );
    expect(scheduleToolAgentIssue("shell", "ops", [{ slug: "ops", tool_allow: ["shell"] }])).toBe("");
  });

  it("renders structured job targets without treating every schedule as identity instructions", () => {
    expect(scheduleTargetLabel({ target: "workflow", workflow: "nightly" })).toBe("workflow");
    expect(scheduleTargetLabel({ target: "system_task", system_task: "catalog_sync" })).toBe("system task");
    expect(scheduleTargetLabel({ target: "tool", tool: "shell" })).toBe("tool");
    expect(scheduleTargetLabel({ agent: "ops" })).toBe("agent wake");
    expect(scheduleActionTitle({ id: "s1", target: "workflow", workflow: "nightly", intent: "Nightly label" })).toBe("Run workflow nightly");
    expect(scheduleActionTitle({ id: "s2", target: "system_task", system_task: "catalog_sync" })).toBe("Run system task Catalog sync");
    expect(scheduleActionTitle({ id: "s3", target: "tool", tool: "shell" })).toBe("Run tool shell");
    expect(scheduleActionTitle({ id: "s4", agent: "ops", intent: "check disks" })).toBe("Wake ops: check disks");
  });

  it("summarizes schedule target mix for the cockpit band", () => {
    const counts = scheduleTargetCounts([
      { target: "" },
      { target: "workflow" },
      { target: "system_task" },
      { target: "tool" },
      {},
    ]);
    expect(counts).toEqual({ agent: 2, workflow: 1, systemTask: 1, tool: 1 });
    expect(scheduleTargetMixLabel(counts)).toBe("2 agent / 1 workflow / 1 system / 1 tool");
    expect(scheduleTargetMixLabel({ agent: 0, workflow: 0, systemTask: 0, tool: 0 })).toBe("none");
  });

  it("keeps schedule ledgers as cron metadata instead of embedded agent prompts", () => {
    expect(scheduleCronjobLedger({ target: "", agent: "ops", cadence: "every 2h" }).map((item) => [item.label, item.value])).toEqual([
      ["timing", "every 2h"],
      ["target", "ops"],
      ["runner", "agent ops"],
      ["payload", "task text only"],
      ["identity", "agent ops owns soul"],
    ]);
    expect(scheduleCronjobLedger({ target: "workflow", workflow: "nightly-sync", cadence: "daily" }).map((item) => [item.label, item.value])).toEqual([
      ["timing", "daily"],
      ["target", "nightly-sync"],
      ["runner", "system identity"],
      ["payload", "cron passes no workflow payload"],
      ["identity", "system identity"],
    ]);
    expect(scheduleCronjobLedger(
      { target: "system_task", system_task: "catalog_sync", cadence: "daily" },
      [{ name: "catalog_sync", label: "Catalog sync", executor: "daemon", uses_llm: false }],
    ).map((item) => [item.label, item.value])).toEqual([
      ["timing", "daily"],
      ["target", "Catalog sync"],
      ["runner", "daemon authority"],
      ["payload", "payload not accepted"],
      ["identity", "no agent identity"],
    ]);
    expect(scheduleCronjobLedger({ target: "tool", tool: "shell", agent: "builder", payload: { command: "date" }, cadence: "every 5m" }).map((item) => [item.label, item.value])).toEqual([
      ["timing", "every 5m"],
      ["target", "shell"],
      ["runner", "agent builder"],
      ["payload", "cron passes object JSON tool payload"],
      ["identity", "agent builder tool policy"],
    ]);
  });

  it("filters schedule rows by concrete cron target", () => {
    const rows: Parameters<typeof filterScheduleItems>[0] = [
      { id: "agent", target: "", intent: "wake" },
      { id: "workflow", target: "workflow", workflow: "nightly" },
      { id: "system", target: "system_task", system_task: "catalog_sync" },
      { id: "tool", target: "tool", tool: "shell" },
    ];
    expect(filterScheduleItems(rows, "all").map((s) => s.id)).toEqual(["agent", "workflow", "system", "tool"]);
    expect(filterScheduleItems(rows, "agent").map((s) => s.id)).toEqual(["agent"]);
    expect(filterScheduleItems(rows, "workflow").map((s) => s.id)).toEqual(["workflow"]);
    expect(filterScheduleItems(rows, "system_task").map((s) => s.id)).toEqual(["system"]);
    expect(filterScheduleItems(rows, "tool").map((s) => s.id)).toEqual(["tool"]);
    expect(filterScheduleItems(
      [
        { id: "ok", target: "workflow", workflow: "nightly" },
        { id: "missing", target: "workflow", workflow: "gone" },
        { id: "daemon-blocked", target: "workflow", workflow: "nightly", target_status: "blocked", target_error: "unknown workflow: nightly" },
        { id: "fast", target: "", intent: "wake", interval_sec: 60 },
      ],
      "attention",
      [],
      [{ name: "nightly" }],
    ).map((s) => s.id)).toEqual(["missing", "daemon-blocked", "fast"]);
  });

  it("describes the concrete cron execution contract for each target type", () => {
    expect(scheduleExecutionContract({ target: "agent", agent: "ops" })).toBe("cron wakes agent ops with this task");
    expect(scheduleExecutionContract({ target: "workflow", workflow: "nightly-sync" })).toBe("cron triggers workflow nightly-sync under system identity");
    expect(scheduleExecutionContract({ target: "tool", tool: "shell", agent: "builder" })).toBe("cron invokes tool shell as builder");
    expect(
      scheduleExecutionContract({
        target: "system_task",
        systemTask: "catalog_sync",
        systemTaskInfo: { name: "catalog_sync", label: "Catalog sync", executor: "daemon", effect_class: "config_update", uses_llm: false },
      }),
    ).toBe("cron runs system task Catalog sync · daemon · config_update · no LLM");
    expect(
      scheduleRowExecutionContract(
        { target: "system_task", system_task: "catalog_sync" },
        [{ name: "catalog_sync", label: "Catalog sync", executor: "daemon", effect_class: "config_update", uses_llm: false }],
      ),
    ).toBe("cron runs system task Catalog sync · daemon · config_update · no LLM");
    expect(scheduleRowExecutionContract({
      target: "tool",
      tool: "shell",
      agent: "ops",
      execution_contract: "cron invokes tool shell as ops",
    })).toBe("cron invokes tool shell as ops");
    expect(scheduleRowExecutionContract({ target: "tool", tool: "shell", agent: "builder" })).toBe("cron invokes tool shell as builder");
  });

  it("separates row intent and payload contracts from execution", () => {
    expect(scheduleRowIntentContract({ target: "agent" })).toBe("intent is agent task");
    expect(scheduleRowIntentContract({ target: "workflow" })).toBe("intent is label only");
    expect(scheduleRowIntentContract({ target: "system_task" })).toBe("typed system call");
    expect(scheduleRowIntentContract({ target: "tool" })).toBe("tool + payload define call");
    expect(scheduleRowIntentLabel({ target: "agent" })).toBe("intent");
    expect(scheduleRowIntentLabel({ target: "workflow" })).toBe("label");
    expect(scheduleRowIntentLabel({ target: "system_task" })).toBe("label");
    expect(scheduleRowIntentLabel({ target: "tool" })).toBe("label");
    expect(scheduleRowPayloadContract({ target: "agent", payload: { ignored: true } })).toBe("task text only");
    expect(scheduleRowPayloadContract({ target: "system_task" })).toBe("payload not accepted");
    expect(scheduleRowPayloadContract({ target: "workflow" })).toBe("cron passes no workflow payload");
    expect(scheduleRowPayloadContract({ target: "tool", payload: { command: "echo hi" } })).toBe("cron passes object JSON tool payload");
    expect(scheduleCronJobPassport({ target: "agent", agent: "ops" })).toEqual({
      value: "agent wake cronjob",
      detail: "wakes ops; the agent owns identity, memory, tools, model route, retry, and repair",
      tone: "muted",
    });
    expect(scheduleCronJobPassport({ target: "workflow", workflow: "nightly-sync" })).toEqual({
      value: "workflow cronjob",
      detail: "fires workflow nightly-sync under system identity; schedule stores cadence, workflow, and optional payload",
      tone: "good",
    });
    expect(
      scheduleCronJobPassport(
        { target: "system_task", system_task: "catalog_sync" },
        [{ name: "catalog_sync", label: "Catalog sync", executor: "daemon", uses_llm: false }],
      ),
    ).toEqual({
      value: "daemon cronjob",
      detail: "fires Catalog sync as a typed system task with no LLM; schedule stores cadence and target, not identity instructions",
      tone: "good",
    });
    expect(scheduleCronJobPassport({ target: "tool", tool: "shell", agent: "builder" })).toEqual({
      value: "tool cronjob",
      detail: "fires tool shell as builder; schedule stores cadence, tool, and JSON payload",
      tone: "warn",
    });
    expect(
      scheduleCronPassport(
        { target: "system_task", system_task: "catalog_sync", cadence: "every 24h" },
        [{ name: "catalog_sync", label: "Catalog sync", executor: "daemon", effect_class: "config_update", uses_llm: false }],
      ),
    ).toBe("cron every 24h · cron runs system task Catalog sync · daemon · config_update · no LLM · typed system call · payload not accepted");
    expect(scheduleCronPassport({ target: "tool", tool: "shell", agent: "builder", mode: "interval", payload: { command: "date" } })).toBe(
      "cron interval · cron invokes tool shell as builder · tool + payload define call · cron passes object JSON tool payload",
    );
    expect(scheduleCronPassport({ target: "tool", tool: "shell", agent: "ops", cadence: "every 1h", execution_contract: "cron invokes tool shell as ops" })).toBe(
      "cron every 1h · cron invokes tool shell as ops · tool + payload define call · cron passes no tool payload",
    );
    expect(scheduleContractSummary({ target: "agent", agent: "ops", cadence: "every 1h" })).toEqual({
      label: "agent wake contract",
      detail: "cron every 1h · cron wakes agent ops with this task · intent is agent task · task text only",
      tone: "muted",
    });
    expect(scheduleContractSummary({ target: "workflow", workflow: "nightly-sync", agent: "ops", cadence: "every 1h" })).toEqual({
      label: "agent workflow contract",
      detail: "cron every 1h · cron triggers workflow nightly-sync as ops · intent is label only · cron passes no workflow payload",
      tone: "warn",
    });
    expect(
      scheduleContractSummary(
        { target: "system_task", system_task: "catalog_sync", cadence: "daily" },
        [{ name: "catalog_sync", label: "Catalog sync", executor: "daemon", effect_class: "config_update", uses_llm: false }],
      ),
    ).toEqual({
      label: "daemon maintenance contract",
      detail: "cron daily · cron runs system task Catalog sync · daemon · config_update · no LLM · typed system call · payload not accepted",
      tone: "good",
    });
    expect(
      scheduleExecutionManifest(
        { target: "system_task", system_task: "catalog_sync", cadence: "daily", uses_llm: false },
        [{ name: "catalog_sync", label: "Catalog sync", executor: "daemon", effect_class: "config_update", uses_llm: false }],
      ),
    ).toEqual({
      label: "typed daemon cronjob",
      detail: "trigger daily · target system task Catalog sync · executor daemon authority · identity no agent identity · payload not accepted · no LLM",
      tone: "good",
      fields: {
        trigger: "daily",
        target: "system task Catalog sync",
        executor: "daemon authority",
        identity: "no agent identity",
        payload: "payload not accepted",
        llm: "no LLM",
      },
    });
    expect(scheduleExecutionManifest({ target: "tool", tool: "shell", agent: "builder", mode: "interval", payload: { command: "date" } })).toMatchObject({
      label: "tool cronjob",
      tone: "warn",
      fields: {
        trigger: "interval",
        target: "tool shell",
        executor: "agent builder",
        identity: "agent builder tool policy",
        payload: "cron passes object JSON tool payload",
        llm: "tool-defined",
      },
    });
  });

  it("summarizes the concrete runtime class separately from prompt text", () => {
    expect(scheduleRuntimePassport({ target: "agent", agent: "ops" })).toEqual({
      value: "LLM wake · ops",
      detail: "cron wakes agent ops with task text",
      tone: "muted",
    });
    expect(scheduleRuntimePassport({ target: "workflow", workflow: "nightly-sync" })).toEqual({
      value: "workflow via daemon",
      detail: "cron starts workflow nightly-sync under system identity",
      tone: "good",
    });
    expect(scheduleRuntimePassport({ target: "tool", tool: "shell", agent: "builder" })).toEqual({
      value: "tool as builder",
      detail: "cron invokes registered tool shell under builder",
      tone: "warn",
    });
    expect(scheduleRuntimePassport(
      { target: "system_task", system_task: "catalog_sync" },
      [{ name: "catalog_sync", label: "Catalog sync", executor: "daemon", uses_llm: false, effect: "Refresh models.dev/api.json." }],
    )).toEqual({
      value: "daemon · no LLM",
      detail: "Catalog sync: Refresh models.dev/api.json.",
      tone: "good",
    });
  });

  it("summarizes which identity or daemon authority executes the cron target", () => {
    expect(scheduleExecutorPassport({ target: "agent", agent: "ops" })).toEqual({
      value: "agent ops",
      detail: "cron wakes ops; that agent owns the task, memory, tools, and model route",
      tone: "muted",
    });
    expect(scheduleExecutorPassport({ target: "workflow", workflow: "nightly-sync" })).toEqual({
      value: "system identity",
      detail: "workflow nightly-sync runs under daemon/system identity",
      tone: "good",
    });
    expect(scheduleExecutorPassport({ target: "workflow", workflow: "nightly-sync", agent: "ops" })).toEqual({
      value: "agent ops",
      detail: "workflow nightly-sync runs under ops's identity and permissions",
      tone: "warn",
    });
    expect(scheduleExecutorPassport({ target: "tool", tool: "shell", agent: "builder" })).toEqual({
      value: "agent builder",
      detail: "tool shell runs under builder's tool policy",
      tone: "warn",
    });
    expect(scheduleExecutorPassport({
      target: "tool",
      tool: "shell",
      agent: "ops",
      executor: "tool",
      uses_llm: false,
      execution_contract: "cron invokes tool shell as ops",
    })).toEqual({
      value: "tool authority",
      detail: "cron invokes tool shell as ops",
      tone: "warn",
    });
    expect(scheduleExecutorPassport(
      { target: "system_task", system_task: "catalog_sync" },
      [{ name: "catalog_sync", label: "Catalog sync", executor: "daemon" }],
    )).toEqual({
      value: "daemon authority",
      detail: "Catalog sync runs as a system maintenance job; no agent identity is woken",
      tone: "good",
    });
  });

  it("keeps cadence, target, executor, payload, frequency, and status in a stable command strip", () => {
    const now = Date.UTC(2026, 0, 1, 12, 0, 0);
    const items = scheduleCommandStrip(
      {
        id: "sch-tool",
        target: "tool",
        tool: "shell",
        agent: "builder",
        cadence: "every 5m",
        mode: "interval",
        interval_sec: 300,
        next_run_unix: (now + 5 * 60_000) / 1000,
        payload: { command: "date" },
        last_status: "ok",
      } as any,
      now,
      [],
      [{ slug: "builder", kind: "custom" }],
      [],
      [{ name: "shell" }],
    );
    expect(items.map((item) => item.label)).toEqual(["cadence", "target", "executor", "payload", "frequency", "health", "status"]);
    expect(items.map((item) => item.value)).toEqual([
      "in 5m",
      "tool cronjob",
      "agent builder",
      "cron passes object JSON tool payload",
      "cadence ok",
      "target ready",
      "ok",
    ]);
    expect(items.map((item) => item.tone)).toEqual(["accent", "warn", "warn", "warn", "good", "warn", "good"]);

    expect(scheduleCommandStrip(
      {
        target: "system_task",
        system_task: "catalog_sync",
        enabled: false,
        interval_sec: 3600,
      },
      now,
      [{ name: "catalog_sync", label: "Catalog sync", executor: "daemon", uses_llm: false, recommended_interval_sec: 86400 }],
    ).map((item) => item.value)).toEqual([
      "paused",
      "daemon cronjob",
      "daemon authority",
      "payload not accepted",
      "Catalog sync is scheduled more often than its recommended cadence",
      "target ready",
      "not fired",
    ]);
  });

  it("reports whether the concrete cron target is still runnable", () => {
    expect(scheduleTargetHealthPassport({ target: "workflow", workflow: "nightly" }, [], [{ name: "nightly", enabled: true }], [])).toEqual({
      value: "target ready",
      detail: "workflow nightly will run under system identity",
      tone: "good",
    });
    expect(scheduleTargetHealthPassport(
      { target: "workflow", workflow: "nightly", target_status: "blocked", target_error: "unknown workflow: nightly" },
      [],
      [{ name: "nightly", enabled: true }],
      [],
    )).toEqual({
      value: "target blocked",
      detail: "unknown workflow: nightly",
      tone: "bad",
    });
    expect(scheduleTargetHealthPassport({ target: "workflow", workflow: "nightly" }, [], [{ name: "nightly", enabled: false }], [])).toEqual({
      value: "target paused",
      detail: "workflow nightly is disabled",
      tone: "bad",
    });
    expect(scheduleTargetHealthPassport({ target: "tool", tool: "shell", agent: "ops" }, [{ slug: "ops", tool_deny: ["shell"] }], [], [{ name: "shell" }])).toEqual({
      value: "target blocked",
      detail: "agent ops cannot schedule tool shell: agent tool denylist",
      tone: "bad",
    });
    expect(scheduleTargetHealthPassport({ target: "system_task", system_task: "catalog_sync" }, [], [], [], [{ name: "catalog_sync", label: "Catalog sync" }])).toEqual({
      value: "target ready",
      detail: "Catalog sync is available as a typed daemon task",
      tone: "good",
    });
    expect(scheduleTargetHealthPassport({ target: "", agent: "worker" }, [{ slug: "worker", kind: "subagent" }], [], [])).toEqual({
      value: "target blocked",
      detail: "agent worker is a managed sub-agent",
      tone: "bad",
    });
    expect(scheduleNeedsAttention({ target: "tool", tool: "shell", agent: "ops" }, [{ slug: "ops", tool_deny: ["shell"] }], [], [{ name: "shell" }])).toBe(true);
    expect(scheduleAttentionCount(
      [
        { id: "ok", target: "system_task", system_task: "catalog_sync", interval_sec: 86400 },
        { id: "bad", target: "tool", tool: "shell", agent: "ops" },
      ],
      [{ slug: "ops", tool_deny: ["shell"] }],
      [],
      [{ name: "shell" }],
      [{ name: "catalog_sync", recommended_interval_sec: 86400 }],
    )).toBe(1);
    expect(scheduleAttentionReasons(
      { target: "workflow", workflow: "nightly", interval_sec: 60, target_error: "unknown workflow: nightly" },
      [],
      [{ name: "nightly" }],
    )).toEqual(["unknown workflow: nightly"]);
  });

  it("flags schedules that are likely to be too chatty", () => {
    expect(
      scheduleFrequencyIssue(
        { target: "system_task", system_task: "catalog_sync", mode: "", interval_sec: 3600 },
        [{ name: "catalog_sync", label: "Catalog sync", recommended_interval_sec: 86400 }],
      ),
    ).toBe("Catalog sync is scheduled more often than its recommended cadence");
    expect(
      scheduleFrequencyIssue(
        { target: "system_task", system_task: "log_clean", mode: "", interval_sec: 3600 },
        [{ name: "log_clean", label: "Log clean", recommended_interval_sec: 86400 }],
      ),
    ).toBe("Log clean is scheduled more often than its recommended cadence");
    expect(scheduleFrequencyIssue({ target: "", mode: "", interval_sec: 60 })).toBe("agent wake schedule is very frequent");
    expect(
      scheduleFrequencyIssue(
        { target: "", mode: "", interval_sec: 3600, agent: "guardian" },
        [],
        [{ slug: "guardian", kind: "system" }],
      ),
    ).toBe("guardian is a system agent scheduled inside the guardian quiet window");
    expect(scheduleFrequencyIssue({ target: "system_task", system_task: "catalog_sync", mode: "", interval_sec: 86400 })).toBe("");
  });

  it("explains when the schedule text is a task versus an operator label", () => {
    expect(scheduleIntentFieldHint("agent")).toContain("task handed to the selected agent");
    expect(scheduleIntentFieldHint("workflow")).toContain("label only");
    expect(scheduleIntentFieldHint("system_task")).toContain("typed cron call");
    expect(scheduleIntentFieldHint("tool")).toContain("tool and payload define the call");
  });

  it("summarizes typed payload contracts for workflow and tool schedules", () => {
    expect(schedulePayloadContract("agent", "{}")).toBe("");
    expect(schedulePayloadContract("tool", "")).toBe("cron passes no tool payload");
    expect(schedulePayloadContract("tool", '{"command":"echo hi"}')).toBe("cron passes object JSON tool payload");
    expect(schedulePayloadContract("workflow", "[1,2]")).toBe("cron passes array JSON workflow payload");
    expect(schedulePayloadContract("tool", "{")).toBe("invalid tool payload JSON");
  });

  it("states the identity boundary for each schedule target", () => {
    expect(scheduleIdentityBoundary("agent", "ops")).toEqual({
      label: "agent owns identity",
      detail: "schedule only wakes ops; soul, memory, tools, model route, retry, and repair stay on the agent",
      tone: "muted",
    });
    expect(scheduleIdentityBoundary("system_task")).toEqual({
      label: "no agent identity",
      detail: "schedule runs a typed daemon system task; no agent is woken and no LLM prompt is created",
      tone: "good",
    });
    expect(scheduleIdentityBoundary("tool", "builder")).toEqual({
      label: "tool uses agent policy",
      detail: "schedule invokes the tool as builder; payload defines the call and the agent policy gates access",
      tone: "warn",
    });
  });

  it("summarizes form cadence as a cron phrase before the schedule is saved", () => {
    expect(scheduleFormCadenceLabel("interval", "30", "minutes", "09:00", "09:00", "17:00", "")).toBe("every 30 minutes");
    expect(scheduleFormCadenceLabel("continuous", "2", "hours", "09:00", "09:00", "17:00", "")).toBe("cycle after 2 hours");
    expect(scheduleFormCadenceLabel("window", "15", "minutes", "09:00", "09:00", "17:00", "")).toBe("every 15 minutes in 09:00-17:00");
    expect(scheduleFormCadenceLabel("daily", "1", "hours", "08:30", "09:00", "17:00", "")).toBe("daily at 08:30");
    expect(scheduleFormCadenceLabel("once", "1", "hours", "08:30", "09:00", "17:00", "2026-01-01T12:00")).toBe("once at 2026-01-01T12:00");
  });

  it("summarizes typed fire metadata for recent firing rows", () => {
    expect(scheduleFireMeta({
      target: "workflow",
      workflow: "nightly-sync",
      agent: "ops",
      model: "gpt-5",
      schedule_id: "sch-1",
      duration_ms: 42.4,
    })).toEqual(["workflow nightly-sync", "as ops", "model gpt-5", "id sch-1", "42ms"]);
    expect(scheduleFireMeta({
      target: "system_task",
      system_task: "catalog_sync",
      executor: "daemon",
      category: "catalog",
      effect_class: "config_update",
      uses_llm: false,
    })).toEqual(["system Catalog sync", "daemon · catalog · config_update · no LLM"]);
    expect(scheduleFireMeta({ target: "tool", tool: "memory", agent: "scout" })).toEqual(["tool memory", "as scout"]);
  });

  it("uses system task metadata for human-readable maintenance job names", () => {
    expect(systemTaskDisplayName("catalog_sync")).toBe("Catalog sync");
    expect(systemTaskDisplayName("log_clean")).toBe("Log clean");
    expect(systemTaskDisplayName("memory_tidy", [{ name: "memory_tidy", label: "Memory tidy" }])).toBe("Memory tidy");
    expect(systemTaskDisplayName("custom_task", [])).toBe("custom_task");
    expect(scheduleSystemTaskPresetLabel("catalog_sync")).toBe("Catalog sync · every 24 hours");
    expect(scheduleSystemTaskPresetLabel("artifact_collect")).toBe("Artifact collect · every 6 hours");
    expect(scheduleSystemTaskPresetLabel("custom_task", [])).toBe("custom_task");
    expect(systemTaskExecutionLabel({ executor: "daemon", category: "catalog", effect_class: "config_update", uses_llm: false })).toBe("daemon · catalog · config_update · no LLM");
    expect(systemTaskExecutionLabel({ executor: "agent", category: "analysis", uses_llm: true })).toBe("agent · analysis · LLM");
  });
});

afterEach(cleanup);
beforeEach(() => {
  postJSON.mockReset();
  postJSON.mockResolvedValue({ id: "sch-1" });
  postAction.mockReset();
  postAction.mockResolvedValue({});
  getJSON.mockReset();
});

describe("NewScheduleForm", () => {
  it("disables Create until an agent task is entered", () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    const create = screen.getByRole("button", { name: /Create schedule/ }) as HTMLButtonElement;
    expect(create.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "Summarize runs" } });
    expect((screen.getByRole("button", { name: /Create schedule/ }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("posts an interval schedule (minutes → interval_sec)", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "ping" } });
    fireEvent.change(screen.getByLabelText("Interval amount"), { target: { value: "15" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", { intent: "ping", interval_sec: 900 }));
  });

  it("posts a continuous cycle schedule (minutes → cooldown_sec)", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "cycle repo-watch" } });
    fireEvent.click(screen.getByRole("button", { name: /cycle/ }));
    fireEvent.change(screen.getByLabelText("Cycle cooldown amount"), { target: { value: "45" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() => expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", { intent: "cycle repo-watch", cooldown_sec: 2700 }));
  });

  it("edits an existing schedule into a continuous cycle", async () => {
    render(<NewScheduleForm editId="sch-cycle" initialIntent="watch" initialMode="interval" initialIntervalSec={900} onCreated={() => {}} onError={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /cycle/ }));
    fireEvent.change(screen.getByLabelText("Cycle cooldown amount"), { target: { value: "2" } });
    chooseScheduleUnit("Cycle cooldown unit", "hours");
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/edit", {
        id: "sch-cycle",
        intent: "watch",
        agent: "",
        target: "",
        cooldown_sec: 7200,
      }),
    );
  });

  it("posts the selected roster agent as structured schedule metadata", async () => {
    render(
      <NewScheduleForm
        agents={[{ slug: "researcher", name: "Researcher", enabled: true }]}
        onCreated={() => {}}
        onError={() => {}}
      />,
    );
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "brief" } });
    chooseScheduleOption("Roster agent", /Researcher/);
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", {
        intent: "brief",
        agent: "researcher",
        interval_sec: 1800,
      }),
    );
  });

  it("does not offer managed sub-agents as direct schedule targets", async () => {
    render(
      <NewScheduleForm
        agents={[
          { slug: "worker", name: "Worker", enabled: true, managed: true, kind: "subagent", direct_callable: false },
          { slug: "planner-child", name: "Planner Child", enabled: true, kind: "subagent" },
          { slug: "lead", name: "Lead", enabled: true },
        ]}
        onCreated={() => {}}
        onError={() => {}}
      />,
    );
    await waitFor(() => expectScheduleOptionSelected("Roster agent", /Lead/));
    const rosterGroup = within(screen.getByRole("group", { name: "Roster agent" }));
    expect((rosterGroup.getByRole("button", { name: /Worker/ }) as HTMLButtonElement).disabled).toBe(true);
    expect((rosterGroup.getByRole("button", { name: /Planner Child/ }) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "brief" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", {
        intent: "brief",
        agent: "lead",
        interval_sec: 1800,
      }),
    );
  });

  it("posts a system task schedule without requiring task instructions", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    chooseScheduleOption("Schedule target", /System task/);
    expect(screen.getByLabelText("Schedule label")).toBeTruthy();
    expect(screen.getByText("Optional label only; the daemon runs the selected system task as a typed cron call.")).toBeTruthy();
    expect(screen.getByText("Cron contract")).toBeTruthy();
    expect(screen.getByText("daemon maintenance contract")).toBeTruthy();
    expect(screen.getByLabelText("Schedule target manifest")).toBeTruthy();
    expect(screen.getByText("Target manifest · typed daemon cronjob")).toBeTruthy();
    expect(screen.getAllByText("system task Catalog sync").length).toBeGreaterThan(0);
    expect(screen.getAllByText("daemon authority").length).toBeGreaterThan(0);
    expect(screen.getAllByText("payload not accepted").length).toBeGreaterThan(0);
    expect(screen.getAllByText("no LLM").length).toBeGreaterThan(0);
    expect(screen.getByText("Identity boundary")).toBeTruthy();
    expect(screen.getAllByText("no agent identity").length).toBeGreaterThan(0);
    expect(screen.getByText(/schedule runs a typed daemon system task/)).toBeTruthy();
    expect(screen.getByText(/cron every 24 hours .* typed system call .* payload not accepted/)).toBeTruthy();
    chooseScheduleUnit("Interval unit", "minutes");
    fireEvent.change(screen.getByLabelText("Interval amount"), { target: { value: "60" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", {
        target: "system_task",
        system_task: "catalog_sync",
        interval_sec: 3600,
      }),
    );
  });

  it("uses quiet recommended cadence for system task schedules until timing is edited", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    chooseScheduleOption("Schedule target", /System task/);

    await waitFor(() => expect((screen.getByLabelText("Interval amount") as HTMLInputElement).value).toBe("24"));
    expectScheduleOptionSelected("Interval unit", "hours");
    expect(screen.getByText("Recommended cadence: every 24 hours")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", {
        target: "system_task",
        system_task: "catalog_sync",
        interval_sec: 86400,
      }),
    );
  });

  it("applies daemon cron presets as typed system task schedules", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);

    fireEvent.click(screen.getByRole("button", { name: "Artifact collect · every 6 hours" }));

    expectScheduleOptionSelected("Schedule target", /System task/);
    expectScheduleOptionSelected("System task", /Artifact collect/);
    expect((screen.getByLabelText("Schedule label") as HTMLTextAreaElement).value).toBe("Collect run artifacts");
    expect((screen.getByLabelText("Interval amount") as HTMLInputElement).value).toBe("6");
    expectScheduleOptionSelected("Interval unit", "hours");

    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", {
        target: "system_task",
        system_task: "artifact_collect",
        intent: "Collect run artifacts",
        interval_sec: 21600,
      }),
    );
  });

  it("uses backend-provided system task options", async () => {
    render(
      <NewScheduleForm
        systemTasks={["memory_clean", "memory_tidy"]}
        systemTaskInfo={[
          { name: "memory_clean", label: "Memory clean", description: "Clear stale memory records.", category: "memory", executor: "daemon", effect_class: "memory_maintenance", uses_llm: false, effect: "Runs without waking an LLM agent." },
          { name: "memory_tidy", label: "Memory tidy", description: "Run lightweight memory hygiene.", category: "memory", executor: "daemon", effect_class: "memory_maintenance", uses_llm: false, effect: "Tidy private memory in-process." },
        ]}
        onCreated={() => {}}
        onError={() => {}}
      />,
    );
    chooseScheduleOption("Schedule target", /System task/);
    expect(screen.getByText("Clear stale memory records.")).toBeTruthy();
    expect(screen.getByText("daemon · memory · memory_maintenance · no LLM - Runs without waking an LLM agent.")).toBeTruthy();
    chooseScheduleOption("System task", /Memory tidy/);
    expect(screen.getByText("Run lightweight memory hygiene.")).toBeTruthy();
    expect(screen.getByText("daemon · memory · memory_maintenance · no LLM - Tidy private memory in-process.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", {
        target: "system_task",
        system_task: "memory_tidy",
        interval_sec: 1800,
      }),
    );
  });

  it("posts a workflow schedule with structured payload JSON", async () => {
    render(<NewScheduleForm workflows={[{ name: "nightly-sync", enabled: true }]} onCreated={() => {}} onError={() => {}} />);
    chooseScheduleOption("Schedule target", /Workflow/);
    fireEvent.change(screen.getByLabelText("Workflow payload JSON"), { target: { value: '{"force":true}' } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", {
        target: "workflow",
        workflow: "nightly-sync",
        payload: { force: true },
        interval_sec: 1800,
      }),
    );
  });

  it("can bind a workflow schedule to an agent identity", async () => {
    render(
      <NewScheduleForm
        agents={[{ slug: "ops", name: "Ops", enabled: true }]}
        workflows={[{ name: "nightly-sync", enabled: true }]}
        onCreated={() => {}}
        onError={() => {}}
      />,
    );
    chooseScheduleOption("Schedule target", /Workflow/);
    chooseScheduleOption("Run as agent", /Ops/);
    fireEvent.change(screen.getByLabelText("Model override"), { target: { value: "gpt-5" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", {
        target: "workflow",
        workflow: "nightly-sync",
        agent: "ops",
        model: "gpt-5",
        interval_sec: 1800,
      }),
    );
  });

  it("rejects stale workflow run-as bindings to managed sub-agents before posting", async () => {
    const onError = vi.fn();
    render(
      <NewScheduleForm
        editId="sch-managed"
        initialTarget="workflow"
        initialWorkflow="nightly-sync"
        initialAgent="worker"
        workflows={[{ name: "nightly-sync", enabled: true }]}
        agents={[{ slug: "worker", name: "Worker", enabled: true, kind: "subagent", direct_callable: false }]}
        onCreated={() => {}}
        onError={onError}
      />,
    );

    expectScheduleOptionSelected("Run as agent", /Worker/);
    expect(screen.getByText("agent worker is a managed sub-agent")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));

    await waitFor(() => expect(onError).toHaveBeenCalledWith("agent worker is a managed sub-agent"));
    expect(postJSON).not.toHaveBeenCalled();
  });

  it("posts a tool schedule with structured payload JSON", async () => {
    render(<NewScheduleForm tools={[{ name: "shell", description: "Run a command" }]} onCreated={() => {}} onError={() => {}} />);
    chooseScheduleOption("Schedule target", /Tool/);
    expect(screen.getByText("cron invokes tool shell under system identity")).toBeTruthy();
    expect(screen.getByText("daemon tool contract")).toBeTruthy();
    expect(screen.getByText("tool uses system policy")).toBeTruthy();
    expect(screen.getByText(/payload defines the call, not an LLM prompt/)).toBeTruthy();
    expect(screen.queryByLabelText("Model override")).toBeNull();
    expect(screen.getByText(/cron every 30 minutes .* tool \+ payload define call .* cron passes no tool payload/)).toBeTruthy();
    expect(screen.getAllByText("cron passes no tool payload").length).toBeGreaterThan(0);
    fireEvent.change(screen.getByLabelText("Tool payload JSON"), { target: { value: '{"command":"echo scheduled"}' } });
    expect(screen.getAllByText(/cron passes object JSON tool payload/).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", {
        target: "tool",
        tool: "shell",
        payload: { command: "echo scheduled" },
        interval_sec: 1800,
      }),
    );
  });

  it("rejects agent-denied tool schedules before posting", async () => {
    const onError = vi.fn();
    render(
      <NewScheduleForm
        agents={[{ slug: "ops", name: "Ops", enabled: true, tool_deny: ["shell"] }]}
        tools={[{ name: "shell", description: "Run a command" }]}
        onCreated={() => {}}
        onError={onError}
      />,
    );
    chooseScheduleOption("Schedule target", /Tool/);
    chooseScheduleOption("Run as agent", /Ops/);
    expect(screen.getByText("tool uses agent policy")).toBeTruthy();
    expect(screen.getByText(/payload defines the call and the agent policy gates access/)).toBeTruthy();
    expect(screen.getByText("agent ops cannot schedule tool shell: agent tool denylist")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("agent ops cannot schedule tool shell: agent tool denylist"));
    expect(postJSON).not.toHaveBeenCalled();
  });

  it("rejects invalid tool payload JSON before posting", async () => {
    const onError = vi.fn();
    render(<NewScheduleForm tools={[{ name: "shell" }]} onCreated={() => {}} onError={onError} />);
    chooseScheduleOption("Schedule target", /Tool/);
    fireEvent.change(screen.getByLabelText("Tool payload JSON"), { target: { value: "{" } });
    expect(screen.getAllByText("invalid tool payload JSON").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("Schedule target manifest")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith(expect.stringContaining("Invalid tool payload JSON")));
    expect(postJSON).not.toHaveBeenCalled();
  });

  it("rejects invalid workflow payload JSON before posting", async () => {
    const onError = vi.fn();
    render(<NewScheduleForm workflows={[{ name: "nightly-sync", enabled: true }]} onCreated={() => {}} onError={onError} />);
    chooseScheduleOption("Schedule target", /Workflow/);
    fireEvent.change(screen.getByLabelText("Workflow payload JSON"), { target: { value: "{" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith(expect.stringContaining("Invalid workflow payload JSON")));
    expect(postJSON).not.toHaveBeenCalled();
  });

  it("posts a daily schedule (HH:MM → at_minutes, every day)", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "briefing" } });
    fireEvent.click(screen.getByRole("button", { name: "daily at…" }));
    fireEvent.change(screen.getByLabelText("Daily time"), { target: { value: "09:30" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", { intent: "briefing", at_minutes: 570, days: 0 }),
    );
  });

  it("posts a window schedule with interval and time bounds", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "watch" } });
    fireEvent.click(screen.getByRole("button", { name: "within window…" }));
    fireEvent.change(screen.getByLabelText("Window interval amount"), { target: { value: "20" } });
    fireEvent.change(screen.getByLabelText("Window start time"), { target: { value: "09:00" } });
    fireEvent.change(screen.getByLabelText("Window end time"), { target: { value: "17:00" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/add", {
        intent: "watch",
        interval_sec: 1200,
        window_start: 540,
        window_end: 1020,
        days: 0,
      }),
    );
  });

  it("posts a one-shot schedule (datetime → once_at_unix)", async () => {
    render(<NewScheduleForm onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "deploy" } });
    fireEvent.click(screen.getByRole("button", { name: "once at…" }));
    fireEvent.change(screen.getByLabelText("Once date and time"), { target: { value: "2030-01-02T03:04" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() => {
      const call = postJSON.mock.calls.find((c) => c[0] === "/api/schedule/add");
      expect(call).toBeTruthy();
      const args = call![1] as { intent: string; once_at_unix: number };
      expect(args.intent).toBe("deploy");
      expect(args.once_at_unix).toBe(Math.floor(Date.parse("2030-01-02T03:04") / 1000));
    });
  });

  it("surfaces a create error", async () => {
    postJSON.mockRejectedValueOnce(new Error("bad schedule"));
    const onError = vi.fn();
    render(<NewScheduleForm onCreated={() => {}} onError={onError} />);
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: /Create schedule/ }));
    await waitFor(() => expect(onError).toHaveBeenCalledWith("bad schedule"));
  });
});

describe("NewScheduleForm (edit mode, M728)", () => {
  it("prefills the intent and labels the action Save changes", () => {
    render(<NewScheduleForm editId="sch-7" initialIntent="old intent" onCreated={() => {}} onError={() => {}} />);
    expect((screen.getByLabelText("Agent task") as HTMLTextAreaElement).value).toBe("old intent");
    // The create label is gone; the edit label is shown.
    expect(screen.queryByRole("button", { name: /Create schedule/ })).toBeNull();
    expect(screen.getByRole("button", { name: /Save changes/ })).toBeTruthy();
  });

  it("posts to schedule/edit with the id and the new intent + cadence", async () => {
    render(<NewScheduleForm editId="sch-7" initialIntent="old intent" initialAgent="ops" onCreated={() => {}} onError={() => {}} />);
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "new intent" } });
    fireEvent.click(screen.getByRole("button", { name: "daily at…" }));
    fireEvent.change(screen.getByLabelText("Daily time"), { target: { value: "06:15" } });
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/edit", {
        id: "sch-7",
        intent: "new intent",
        agent: "ops",
        target: "",
        at_minutes: 375,
        days: 0,
      }),
    );
  });

  it("prefills existing cadence and does not rewrite timing unless changed", async () => {
    render(
      <NewScheduleForm
        editId="sch-7"
        initialIntent="daily job"
        initialMode="daily"
        initialAtMinutes={570}
        onCreated={() => {}}
        onError={() => {}}
      />,
    );
    expect((screen.getByLabelText("Daily time") as HTMLInputElement).value).toBe("09:30");
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "renamed daily job" } });
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/edit", {
        id: "sch-7",
        intent: "renamed daily job",
        agent: "",
        target: "",
      }),
    );
  });

  it("posts cadence on edit after the timing controls are changed", async () => {
    render(
      <NewScheduleForm
        editId="sch-7"
        initialIntent="daily job"
        initialMode="daily"
        initialAtMinutes={570}
        onCreated={() => {}}
        onError={() => {}}
      />,
    );
    fireEvent.change(screen.getByLabelText("Daily time"), { target: { value: "10:45" } });
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/edit", {
        id: "sch-7",
        intent: "daily job",
        agent: "",
        target: "",
        at_minutes: 645,
        days: 0,
      }),
    );
  });

  it("prefills window cadence and does not rewrite timing unless changed", async () => {
    render(
      <NewScheduleForm
        editId="sch-win"
        initialIntent="window job"
        initialMode="window"
        initialIntervalSec={1800}
        initialAtMinutes={540}
        initialEndMinutes={1020}
        initialDays={0}
        onCreated={() => {}}
        onError={() => {}}
      />,
    );
    expect((screen.getByLabelText("Window interval amount") as HTMLInputElement).value).toBe("30");
    expect((screen.getByLabelText("Window start time") as HTMLInputElement).value).toBe("09:00");
    expect((screen.getByLabelText("Window end time") as HTMLInputElement).value).toBe("17:00");
    fireEvent.change(screen.getByLabelText("Agent task"), { target: { value: "renamed window job" } });
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/edit", {
        id: "sch-win",
        intent: "renamed window job",
        agent: "",
        target: "",
      }),
    );
  });

  it("posts window cadence on edit after a window timing control changes", async () => {
    render(
      <NewScheduleForm
        editId="sch-win"
        initialIntent="window job"
        initialMode="window"
        initialIntervalSec={1800}
        initialAtMinutes={540}
        initialEndMinutes={1020}
        initialDays={0}
        onCreated={() => {}}
        onError={() => {}}
      />,
    );
    fireEvent.change(screen.getByLabelText("Window end time"), { target: { value: "18:00" } });
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() =>
      expect(postJSON).toHaveBeenCalledWith("/api/schedule/edit", {
        id: "sch-win",
        intent: "window job",
        agent: "",
        target: "",
        interval_sec: 1800,
        window_start: 540,
        window_end: 1080,
        days: 0,
      }),
    );
  });

  it("calls onCreated after a successful edit", async () => {
    const onCreated = vi.fn();
    render(<NewScheduleForm editId="sch-7" initialIntent="x" onCreated={onCreated} onError={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /Save changes/ }));
    await waitFor(() => expect(onCreated).toHaveBeenCalled());
  });
});

describe("parseSchedulesJSON (M749)", () => {
  it("reads a bare array and a {schedules:[…]} wrapper", () => {
    const row = { id: "x", intent: "ping", mode: "", interval_sec: 900, source: "operator", enabled: true };
    expect(parseSchedulesJSON(JSON.stringify([row]))).toHaveLength(1);
    expect(parseSchedulesJSON(JSON.stringify({ version: 1, schedules: [row] }))).toHaveLength(1);
  });

  it("rebuilds interval args (mode '' → interval_sec), dropping kernel fields", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([{ id: "x", source: "agent", enabled: false, fires: 4, intent: "ping", mode: "", interval_sec: 900 }]),
    );
    expect(out[0]).toEqual({ intent: "ping", interval_sec: 900 });
  });

  it("rebuilds daily args (at_minutes + days + tz)", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([{ intent: "brief", mode: "daily", at_minutes: 570, days: 0, tz: "Europe/Istanbul" }]),
    );
    expect(out[0]).toEqual({ intent: "brief", at_minutes: 570, days: 0, tz: "Europe/Istanbul" });
  });

  it("rebuilds window args (at_minutes→window_start, end_minutes→window_end, interval_sec)", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([{ intent: "watch", mode: "window", at_minutes: 540, end_minutes: 1020, interval_sec: 1800, days: 0 }]),
    );
    expect(out[0]).toEqual({ intent: "watch", window_start: 540, window_end: 1020, interval_sec: 1800, days: 0 });
  });

  it("rebuilds once args from once_at_unix, falling back to next_run_unix", () => {
    expect(parseSchedulesJSON(JSON.stringify([{ intent: "deploy", mode: "once", once_at_unix: 1893456000 }]))[0]).toEqual({
      intent: "deploy",
      once_at_unix: 1893456000,
    });
    expect(parseSchedulesJSON(JSON.stringify([{ intent: "deploy", mode: "once", next_run_unix: 1893456000 }]))[0]).toEqual({
      intent: "deploy",
      once_at_unix: 1893456000,
    });
  });

  it("keeps explicit model and roster agent metadata across cadence kinds", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([{ intent: "x", mode: "", interval_sec: 60, model: "deepseek-chat", agent: "researcher" }]),
    );
    expect(out[0]).toEqual({ intent: "x", interval_sec: 60, model: "deepseek-chat", agent: "researcher" });
  });

  it("keeps workflow target metadata without requiring agent identity instructions", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([{ mode: "", interval_sec: 60, target: "workflow", workflow: "nightly-sync", agent: "ops", model: "gpt-5", payload: { force: true } }]),
    );
    expect(out[0]).toEqual({
      interval_sec: 60,
      target: "workflow",
      workflow: "nightly-sync",
      agent: "ops",
      model: "gpt-5",
      payload: { force: true },
    });
  });

  it("keeps system task target metadata without requiring an intent prompt", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([{ mode: "", interval_sec: 86400, target: "system_task", system_task: "catalog_sync", agent: "ops", model: "gpt-5" }]),
    );
    expect(out[0]).toEqual({
      interval_sec: 86400,
      target: "system_task",
      system_task: "catalog_sync",
    });
  });

  it("keeps tool target metadata without requiring an intent prompt", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([{ mode: "", interval_sec: 300, target: "tool", tool: "shell", agent: "builder", model: "gpt-5-mini", payload: { command: "echo hi" } }]),
    );
    expect(out[0]).toEqual({
      interval_sec: 300,
      target: "tool",
      tool: "shell",
      agent: "builder",
      payload: { command: "echo hi" },
    });
  });

  it("keeps continuous cycle schedules and skips intentless or invalid-cadence entries", () => {
    const out = parseSchedulesJSON(
      JSON.stringify([
        { intent: "keep", mode: "", interval_sec: 60 },
        { intent: "alive", mode: "continuous", interval_sec: 60 },
        { mode: "", interval_sec: 60 }, // no intent
        { intent: "zero", mode: "", interval_sec: 0 }, // invalid interval
        { intent: "noonce", mode: "once" }, // once with no fire time
      ]),
    );
    expect(out).toHaveLength(2);
    expect(out[0]).toEqual({ intent: "keep", interval_sec: 60 });
    expect(out[1]).toEqual({ intent: "alive", cooldown_sec: 60 });
  });

  it("throws on invalid JSON, a non-array shape, or nothing re-addable", () => {
    expect(() => parseSchedulesJSON("nope")).toThrow();
    expect(() => parseSchedulesJSON('{"foo":1}')).toThrow(/expected an array/);
    expect(() => parseSchedulesJSON("[{}]")).toThrow(/agent task or typed target/);
  });
});

describe("Schedules fire-time preview (M744)", () => {
  const sched = {
    id: "sch-9",
    intent: "morning brief",
    cadence: "daily at 09:00",
    mode: "daily",
    enabled: true,
    next_run_unix: 1893456000,
  };

  it("toggles a forecast of next fire times from /api/schedule/test", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/schedules") return Promise.resolve({ schedules: [sched] });
      if (path === "/api/schedule/test")
        return Promise.resolve({ forecasts: [{ unix: 1893456000 }, { unix: 1893542400 }] });
      return Promise.resolve({});
    });
    render(withUI(<Schedules />));
    await waitFor(() => expect(screen.getByText("morning brief")).toBeTruthy());

    // Open the forecast.
    fireEvent.click(screen.getByRole("button", { name: "next fires" }));
    await waitFor(() =>
      expect(getJSON).toHaveBeenCalledWith("/api/schedule/test", { id: "sch-9", count: "5" }),
    );
    // Two forecast rows render (numbered).
    await waitFor(() => expect(screen.getByText("1.")).toBeTruthy());
    expect(screen.getByText("2.")).toBeTruthy();

    // Toggle hides it.
    expect(screen.getByRole("button", { name: "hide fires" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "hide fires" }));
    await waitFor(() => expect(screen.queryByText("1.")).toBeNull());
  });
});

// The cockpit target filter is now a TabNav (role="tab"; label + count concatenated in the
// accessible name, e.g. "Workflow1"). Radix Tabs activate on pointer-down under jsdom.
const selectScheduleTab = (tab: HTMLElement) => {
  fireEvent.pointerDown(tab, { button: 0, ctrlKey: false });
  fireEvent.mouseDown(tab, { button: 0 });
  fireEvent.click(tab);
};

describe("Schedules job cards", () => {
  it("filters the schedule cockpit by target type", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/schedules")
        return Promise.resolve({
          schedules: [
            { id: "sch-agent", intent: "wake ops", cadence: "every 1h", enabled: true },
            { id: "sch-workflow", target: "workflow", workflow: "nightly-sync", cadence: "every 2h", enabled: true },
            { id: "sch-system", target: "system_task", system_task: "catalog_sync", cadence: "daily", enabled: true },
            { id: "sch-tool", target: "tool", tool: "shell", cadence: "every 3h", enabled: true },
          ],
        });
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/workflows") return Promise.resolve({ workflows: [{ name: "nightly-sync" }] });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [{ name: "shell" }] });
      if (path === "/api/schedule/system_tasks") return Promise.resolve({ system_tasks: ["catalog_sync"] });
      if (path === "/api/schedule/fires") return Promise.resolve({ fires: [] });
      return Promise.resolve({});
    });

    render(withUI(<Schedules />));
    await waitFor(() => expect(screen.getByText("wake ops")).toBeTruthy());
    selectScheduleTab(screen.getByRole("tab", { name: /Workflow\s*1/ }));
    expect(screen.getByText("Run workflow nightly-sync")).toBeTruthy();
    expect(screen.queryByText("wake ops")).toBeNull();
    expect(screen.queryByText("Run system task Catalog sync")).toBeNull();
    expect(screen.queryByText("Run tool shell")).toBeNull();
  });

  it("filters the schedule cockpit to target or cadence attention", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/schedules")
        return Promise.resolve({
          schedules: [
            { id: "sch-ok", target: "workflow", workflow: "nightly-sync", cadence: "every 2h", enabled: true },
            { id: "sch-missing", target: "workflow", workflow: "gone-flow", cadence: "every 2h", enabled: true },
            { id: "sch-fast", target: "", intent: "ping", cadence: "every 1m", mode: "interval", interval_sec: 60, enabled: true },
          ],
        });
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/workflows") return Promise.resolve({ workflows: [{ name: "nightly-sync" }] });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/schedule/system_tasks") return Promise.resolve({ system_tasks: ["catalog_sync"] });
      if (path === "/api/schedule/fires") return Promise.resolve({ fires: [] });
      return Promise.resolve({});
    });

    render(withUI(<Schedules />));
    await waitFor(() => expect(screen.getByText("Run workflow nightly-sync")).toBeTruthy());
    expect(screen.getByText("attention")).toBeTruthy();
    const attentionTab = screen.getByRole("tab", { name: /Attention\s*2/ });
    expect(attentionTab).toBeTruthy();
    selectScheduleTab(attentionTab);
    expect(screen.queryByText("Run workflow nightly-sync")).toBeNull();
    expect(screen.getByText("Run workflow gone-flow")).toBeTruthy();
    expect(screen.getByText("ping")).toBeTruthy();
    expect(screen.getByText("workflow gone-flow is not registered")).toBeTruthy();
    expect(screen.getAllByText("target missing").length).toBeGreaterThan(0);
    expect(screen.getAllByText("agent wake schedule is very frequent").length).toBeGreaterThan(0);
  });

  it("pauses enabled attention schedules in bulk without touching healthy schedules", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/schedules")
        return Promise.resolve({
          schedules: [
            { id: "sch-ok", target: "workflow", workflow: "nightly-sync", cadence: "every 2h", enabled: true },
            { id: "sch-missing", target: "workflow", workflow: "gone-flow", cadence: "every 2h", enabled: true },
            { id: "sch-fast", target: "", intent: "ping", cadence: "every 1m", mode: "interval", interval_sec: 60, enabled: true },
            { id: "sch-paused-bad", target: "tool", tool: "gone-tool", cadence: "every 1h", enabled: false },
          ],
        });
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/workflows") return Promise.resolve({ workflows: [{ name: "nightly-sync" }] });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/schedule/system_tasks") return Promise.resolve({ system_tasks: ["catalog_sync"] });
      if (path === "/api/schedule/fires") return Promise.resolve({ fires: [] });
      return Promise.resolve({});
    });

    render(withUI(<Schedules />));
    await waitFor(() => expect(screen.getByRole("button", { name: /Pause attention/ })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: /Pause attention/ }));
    fireEvent.click(screen.getAllByRole("button", { name: /Pause attention/ }).at(-1)!);

    await waitFor(() => {
      expect(postAction).toHaveBeenCalledWith("/api/schedule/enable", { id: "sch-missing", enabled: "false" });
      expect(postAction).toHaveBeenCalledWith("/api/schedule/enable", { id: "sch-fast", enabled: "false" });
    });
    expect(postAction).not.toHaveBeenCalledWith("/api/schedule/enable", { id: "sch-ok", enabled: "false" });
    expect(postAction).not.toHaveBeenCalledWith("/api/schedule/enable", { id: "sch-paused-bad", enabled: "false" });
  });

  it("uses the structured action as the primary row and keeps labels secondary", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/schedules")
        return Promise.resolve({
          schedules: [
            {
              id: "sch-workflow",
              intent: "Nightly label",
              model: "gpt-5",
              target: "workflow",
              workflow: "nightly-sync",
              cadence: "every 1h",
              enabled: true,
              next_run_unix: 1893456000,
            },
          ],
        });
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/workflows") return Promise.resolve({ workflows: [{ name: "nightly-sync" }] });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/schedule/system_tasks") return Promise.resolve({ system_tasks: ["catalog_sync"] });
      return Promise.resolve({});
    });

    render(withUI(<Schedules />));
    await waitFor(() => expect(screen.getByText("Run workflow nightly-sync")).toBeTruthy());
    expect(screen.getByText("workflow")).toBeTruthy();
    expect(screen.getByText("targets")).toBeTruthy();
    expect(screen.getByText("1 workflow")).toBeTruthy();
    expect(screen.getByText("system workflow contract")).toBeTruthy();
    expect(screen.getByText("cron every 1h · cron triggers workflow nightly-sync under system identity · intent is label only · cron passes no workflow payload")).toBeTruthy();
    expect(screen.getByLabelText("sch-workflow execution manifest")).toBeTruthy();
    expect(screen.getByText("Execution manifest · workflow cronjob")).toBeTruthy();
    expect(screen.getAllByText("trigger").length).toBeGreaterThan(0);
    expect(screen.getAllByText("every 1h").length).toBeGreaterThan(0);
    expect(screen.getAllByText("target").length).toBeGreaterThan(0);
    expect(screen.getAllByText("workflow nightly-sync").length).toBeGreaterThan(0);
    expect(screen.getAllByText("identity").length).toBeGreaterThan(0);
    expect(screen.getAllByText("may use LLM").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("sch-workflow schedule command strip")).toBeTruthy();
    expect(screen.getAllByText("cadence").length).toBeGreaterThan(0);
    expect(screen.getAllByText("executor").length).toBeGreaterThan(0);
    expect(screen.getAllByText("frequency").length).toBeGreaterThan(0);
    expect(screen.getAllByText("cadence ok").length).toBeGreaterThan(0);
    expect(screen.getByText("Cronjob")).toBeTruthy();
    expect(screen.getAllByText("workflow cronjob").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("sch-workflow cronjob ledger")).toBeTruthy();
    expect(screen.getByText("Cronjob ledger")).toBeTruthy();
    expect(screen.getAllByText("timing").length).toBeGreaterThan(0);
    expect(screen.getAllByText("runner").length).toBeGreaterThan(0);
    expect(screen.getAllByText("system identity").length).toBeGreaterThan(0);
    expect(screen.getByText("cron triggers workflow nightly-sync under system identity")).toBeTruthy();
    expect(screen.getAllByText("label").length).toBeGreaterThan(0);
    expect(screen.getByText("intent is label only")).toBeTruthy();
    expect(screen.getAllByText("cron passes no workflow payload").length).toBeGreaterThan(0);
    expect(screen.getByText("model gpt-5")).toBeTruthy();
    expect(screen.getByText("label: Nightly label")).toBeTruthy();
    fireEvent.click(screen.getByTitle("Edit"));
    expect((screen.getByLabelText("Model override") as HTMLInputElement).value).toBe("gpt-5");
  });

  it("shows recent schedule firings as structured actions", async () => {
    getJSON.mockImplementation((path: string, args?: Record<string, string>) => {
      if (path === "/api/schedules")
        return Promise.resolve({
          schedules: [
            {
              id: "sch-workflow",
              target: "workflow",
              workflow: "nightly-sync",
              cadence: "every 1h",
              enabled: true,
            },
          ],
        });
      if (path === "/api/schedule/fires") {
        expect(args).toEqual({ limit: "5" });
        return Promise.resolve({
          fires: [
            {
              schedule_id: "sch-workflow",
              correlation_id: "corr-1",
              action: "Run workflow nightly-sync",
              target: "workflow",
              workflow: "nightly-sync",
              agent: "ops",
              model: "gpt-5",
              status: "completed",
              fired_unix_ms: 1_893_456_000_000,
              duration_ms: 42,
            },
          ],
        });
      }
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/workflows") return Promise.resolve({ workflows: [{ name: "nightly-sync" }] });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/schedule/system_tasks") return Promise.resolve({ system_tasks: ["catalog_sync"] });
      return Promise.resolve({});
    });

    render(withUI(<Schedules />));
    await waitFor(() => expect(screen.getByText("Recent firings")).toBeTruthy());
    expect(screen.getAllByText("Run workflow nightly-sync").length).toBeGreaterThan(0);
    expect(screen.getByText("completed")).toBeTruthy();
    expect(screen.getByText("workflow nightly-sync · as ops · model gpt-5 · id sch-workflow · 42ms")).toBeTruthy();
  });

  it("surfaces backend frequency warnings on schedule cards", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/schedules")
        return Promise.resolve({
          schedules: [
            {
              id: "sch-chatty",
              target: "system_task",
              system_task: "catalog_sync",
              cadence: "every 1h",
              mode: "",
              interval_sec: 3600,
              enabled: true,
              frequency_warning: "system task runs more frequently than its recommended cadence",
            },
          ],
        });
      if (path === "/api/agents") return Promise.resolve({ profiles: [] });
      if (path === "/api/workflows") return Promise.resolve({ workflows: [] });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/schedule/system_tasks") return Promise.resolve({ system_tasks: ["catalog_sync"] });
      if (path === "/api/schedule/fires") return Promise.resolve({ fires: [] });
      return Promise.resolve({});
    });

    render(withUI(<Schedules />));
    await waitFor(() => expect(screen.getByText("Run system task Catalog sync")).toBeTruthy());
    expect(screen.getByText("frequent")).toBeTruthy();
    expect(screen.getAllByText("system task runs more frequently than its recommended cadence").length).toBeGreaterThan(0);
  });
});

describe("Schedules bound agent state", () => {
  it("shows paused bound agents and blocks unsafe resume", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/schedules")
        return Promise.resolve({
          schedules: [{ id: "sch-paused", intent: "agent brief", cadence: "every 2h", enabled: false, agent: "ops" }],
        });
      if (path === "/api/agents") return Promise.resolve({ profiles: [{ slug: "ops", enabled: false }] });
      if (path === "/api/workflows") return Promise.resolve({ workflows: [] });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/schedule/system_tasks") return Promise.resolve({ system_tasks: ["catalog_sync"] });
      return Promise.resolve({});
    });

    render(withUI(<Schedules />));
    await waitFor(() => expect(screen.getByText("Wake ops: agent brief")).toBeTruthy());

    expect(screen.getByTitle("runs as ops")).toBeTruthy();
    expect(screen.getByText("is paused")).toBeTruthy();

    const resume = screen.getAllByTitle("agent ops is paused").find((el) => el.tagName === "BUTTON") as HTMLButtonElement;
    expect(resume.disabled).toBe(true);
    fireEvent.click(resume);
    expect(postAction).not.toHaveBeenCalledWith(
      "/api/schedule/enable",
      expect.objectContaining({ id: "sch-paused", enabled: "true" }),
    );
  });

  it("blocks resume for kind-only sub-agents bound to a schedule", async () => {
    getJSON.mockImplementation((path: string) => {
      if (path === "/api/schedules")
        return Promise.resolve({
          schedules: [{ id: "sch-child", intent: "child brief", cadence: "every 2h", enabled: false, agent: "planner-child" }],
        });
      if (path === "/api/agents") return Promise.resolve({ profiles: [{ slug: "planner-child", kind: "subagent", enabled: true }] });
      if (path === "/api/workflows") return Promise.resolve({ workflows: [] });
      if (path === "/api/tools_catalog") return Promise.resolve({ tools: [] });
      if (path === "/api/schedule/system_tasks") return Promise.resolve({ system_tasks: ["catalog_sync"] });
      return Promise.resolve({});
    });

    render(withUI(<Schedules />));
    await waitFor(() => expect(screen.getByText("Wake planner-child: child brief")).toBeTruthy());

    expect(screen.getByText("is a managed sub-agent")).toBeTruthy();
    const resume = screen.getAllByTitle("agent planner-child is a managed sub-agent").find((el) => el.tagName === "BUTTON") as HTMLButtonElement;
    expect(resume.disabled).toBe(true);
  });
});
