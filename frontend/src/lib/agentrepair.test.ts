import { describe, it, expect } from "vitest";
import { buildRepairBrief, parseRepairProposal, applyProposal, proposalSummary, type RepairProfile } from "@/lib/agentrepair";

describe("buildRepairBrief", () => {
  it("names the agent, lists its config, and the evidence", () => {
    const brief = buildRepairBrief({
      profile: { slug: "scout", name: "Scout", model: "m1", fallbacks: ["m2", "m3"], task_type: "research", task_model_chain: ["m1", "m2"], workdir: "scout/" },
      fail: { correlation_id: "run-9", status: "tool error" },
      denials: [{ capability: "net.write", tool: "http", reason: "blocked host", hard_denied: true }],
      toolErrors: [{ tool: "shell", output: "'jq' is not recognized" }],
      configIssues: ["AGEZT_MAX_ITER: must be an integer"],
    });
    expect(brief).toContain('"scout"');
    expect(brief).toContain("model=m1");
    expect(brief).toContain("m2 → m3");
    expect(brief).toContain("task_model_chain=m1 → m2");
    expect(brief).toContain("run-9");
    expect(brief).toContain("net.write");
    expect(brief).toContain("hard-denied");
    expect(brief).toContain("'jq' is not recognized");
    expect(brief).toContain("AGEZT_MAX_ITER");
    expect(brief).toContain("```json");
  });

  it("includes the agent resilience and governance contract", () => {
    const brief = buildRepairBrief({
      profile: {
        slug: "worker",
        parent_agent: "lead",
        direct_callable: false,
        retry_policy: { max_attempts: 3, backoff: "exponential", retry_on: ["error", "timeout"] },
        health_policy: { doctor_agent: "guardian-doctor", failure_threshold: 2 },
        self_repair: { enabled: true, max_attempts: 2, escalate_to: "lead" },
        noise_policy: { silent_on_success: true, disable_memory_writes: true, min_notify_severity: "warning", min_notify_interval_sec: 14400 },
      },
    });

    expect(brief).toContain("resilience and governance contract");
    expect(brief).toContain("retry=3x exponential on error/timeout");
    expect(brief).toContain("doctor=guardian-doctor");
    expect(brief).toContain("self_repair=on 2x escalate=lead");
    expect(brief).toContain("call=managed by lead");
    expect(brief).toContain("noise=silent_on_success/memory_writes=off/notify>=warning/notify_cooldown=14400s");
  });

  it("handles a clean agent (no failures) with a latent-weakness prompt", () => {
    const brief = buildRepairBrief({ profile: { slug: "calm" } });
    expect(brief).toContain("No failures");
  });

  it("includes prior rounds so iterate builds on the last attempt", () => {
    const brief = buildRepairBrief({ profile: { slug: "x" }, priorRounds: ["fixed the path bug", "rewrote the skill"] });
    expect(brief).toContain("already tried");
    expect(brief).toContain("Round 1: fixed the path bug");
    expect(brief).toContain("Round 2: rewrote the skill");
  });
});

describe("parseRepairProposal", () => {
  it("extracts the last fenced json block", () => {
    const text = 'I fixed my script.\n\n```json\n{ "soul": "New soul.", "model": "best-model" }\n```\nDone.';
    const p = parseRepairProposal(text);
    expect(p).toEqual({ soul: "New soul.", model: "best-model" });
  });

  it("extracts fallbacks and drops blanks", () => {
    const p = parseRepairProposal('```json\n{"fallbacks":["a"," ","b"]}\n```');
    expect(p).toEqual({ fallbacks: ["a", "b"] });
  });

  it("extracts config_overrides and uppercases the keys", () => {
    const p = parseRepairProposal('```json\n{"config_overrides":{"agezt_max_iter":"6","AGEZT_MODEL":"gpt-5"}}\n```');
    expect(p).toEqual({ config_overrides: { AGEZT_MAX_ITER: "6", AGEZT_MODEL: "gpt-5" } });
  });

  it("extracts task_type and task_model_chain", () => {
    const p = parseRepairProposal('```json\n{"task_type":"code","task_model_chain":["gpt-5"," gpt-4.1 "]} \n```');
    expect(p).toEqual({ task_type: "code", task_model_chain: ["gpt-5", "gpt-4.1"] });
  });

  it("returns null when there's no proposal or no known keys", () => {
    expect(parseRepairProposal("Just text, no block.")).toBeNull();
    expect(parseRepairProposal('```json\n{"other":1}\n```')).toBeNull();
    expect(parseRepairProposal("")).toBeNull();
  });

  it("prefers the last block when several are present", () => {
    const text = '```json\n{"model":"old"}\n```\nlater\n```json\n{"model":"new"}\n```';
    expect(parseRepairProposal(text)).toEqual({ model: "new" });
  });

  it("falls back to a bare object when unfenced", () => {
    expect(parseRepairProposal('text { "model": "x" } tail')).toEqual({ model: "x" });
  });
});

describe("applyProposal / proposalSummary", () => {
  it("merges only the proposed fields, preserving the rest", () => {
    const base = { slug: "a", soul: "old", model: "m1", fallbacks: ["m2"], task_type: "research" };
    const next = applyProposal(base, { model: "m9" });
    expect(next).toEqual({ slug: "a", soul: "old", model: "m9", fallbacks: ["m2"], task_type: "research" });
    expect(base.model).toBe("m1"); // original untouched (for Undo)
  });

  it("can replace the soul and fallbacks", () => {
    const base: RepairProfile = { slug: "a", soul: "old" };
    const next = applyProposal(base, { soul: "new", fallbacks: ["x", "y"] });
    expect(next.soul).toBe("new");
    expect(next.fallbacks).toEqual(["x", "y"]);
  });

  it("can apply config_overrides", () => {
    const base = { slug: "a", config_overrides: { AGEZT_MODEL: "old" } };
    const next = applyProposal(base, { config_overrides: { AGEZT_MODEL: "new", AGEZT_MAX_ITER: "6" } });
    expect(next.config_overrides).toEqual({ AGEZT_MODEL: "new", AGEZT_MAX_ITER: "6" });
  });

  it("can apply task_type", () => {
    const base = { slug: "a", task_type: "research" };
    const next = applyProposal(base, { task_type: "code" });
    expect(next.task_type).toBe("code");
  });

  it("summarizes what changed", () => {
    expect(proposalSummary({ soul: "x", model: "m" })).toContain("soul");
    expect(proposalSummary({ model: "m" })).toContain("model → m");
    expect(proposalSummary({ fallbacks: ["a", "b"] })).toContain("a → b");
    expect(proposalSummary({ task_type: "code" })).toContain("task_type → code");
    expect(proposalSummary({ task_model_chain: ["gpt-5", "gpt-4.1"] })).toContain("task_model_chain → gpt-5 → gpt-4.1");
    expect(proposalSummary({ config_overrides: { AGEZT_MAX_ITER: "6" } })).toContain("AGEZT_MAX_ITER");
    expect(proposalSummary({})).toBe("no identity change");
  });
});
