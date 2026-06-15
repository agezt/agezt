import { describe, it, expect } from "vitest";
import { buildRepairBrief, parseRepairProposal, applyProposal, proposalSummary, type RepairProfile } from "@/lib/agentrepair";

describe("buildRepairBrief", () => {
  it("names the agent, lists its config, and the evidence", () => {
    const brief = buildRepairBrief({
      profile: { slug: "scout", name: "Scout", model: "m1", fallbacks: ["m2", "m3"], task_type: "research", workdir: "scout/" },
      fail: { correlation_id: "run-9", status: "tool error" },
      denials: [{ capability: "net.write", tool: "http", reason: "blocked host", hard_denied: true }],
      toolErrors: [{ tool: "shell", output: "'jq' is not recognized" }],
    });
    expect(brief).toContain('"scout"');
    expect(brief).toContain("model=m1");
    expect(brief).toContain("m2 → m3");
    expect(brief).toContain("run-9");
    expect(brief).toContain("net.write");
    expect(brief).toContain("hard-denied");
    expect(brief).toContain("'jq' is not recognized");
    expect(brief).toContain("```json");
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

  it("summarizes what changed", () => {
    expect(proposalSummary({ soul: "x", model: "m" })).toContain("soul");
    expect(proposalSummary({ model: "m" })).toContain("model → m");
    expect(proposalSummary({ fallbacks: ["a", "b"] })).toContain("a → b");
    expect(proposalSummary({})).toBe("no identity change");
  });
});
