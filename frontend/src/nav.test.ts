import { describe, expect, it } from "vitest";
import { NAV, NAV_GROUPS, ROWS, VIEW_ALIASES, groupForView, rowForView, sectionForView } from "@/nav";

describe("operator-job navigation", () => {
  it("uses the eight cockpit jobs in a stable order", () => {
    expect(NAV_GROUPS.map((group) => group.label)).toEqual([
      "Talk",
      "Observe",
      "Automate",
      "Govern",
      "Agents",
      "Knowledge",
      "Connect",
      "Admin",
    ]);
  });

  it("assigns every view exactly once and keeps sections scannable", () => {
    const ids = NAV_GROUPS.flatMap((g) => g.rows.flatMap((r) => r.views.map((v) => v.id)));
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids).toEqual(NAV.map((item) => item.id));
    // A section the operator cannot take in at a glance is not an IA. Rows —
    // not views — are what the sidebar shows, so this is the number that counts.
    expect(Math.max(...NAV_GROUPS.map((g) => g.rows.length))).toBeLessThanOrEqual(6);
  });

  it("gives every row a unique id and at least one view", () => {
    const rowIds = ROWS.map((r) => r.id);
    expect(new Set(rowIds).size).toBe(rowIds.length);
    for (const r of ROWS) expect(r.views.length, r.id).toBeGreaterThan(0);
  });

  it("resolves every view to the row that renders it", () => {
    for (const v of NAV) {
      const r = rowForView[v.id];
      expect(r, v.id).toBeTruthy();
      expect(r.views.some((x) => x.id === v.id), v.id).toBe(true);
    }
  });

  it("maps representative tasks to operator language", () => {
    expect(groupForView.chat).toBe("talk");
    expect(groupForView.runs).toBe("observe");
    expect(groupForView.schedules).toBe("automate");
    expect(groupForView.policy).toBe("govern");
    expect(groupForView.roster).toBe("fleet");
    expect(groupForView.memory).toBe("knowledge");
    expect(groupForView.models).toBe("connect");
    expect(groupForView.configcenter).toBe("admin");

    expect(sectionForView.policy).toBe("Govern");
    expect(sectionForView.models).toBe("Connect");
  });

  it("puts telemetry with the thing it measures, not next to its manager", () => {
    // `providers` is the routing/fallback LOG, not a provider manager — shelving
    // it in Connect is what made "where do I add an API key?" unanswerable.
    expect(groupForView.providers).toBe("observe");
    expect(rowForView.providers.id).toBe("health");
    expect(groupForView.tools).toBe("observe");
    expect(rowForView.tools.id).toBe("health");
    // Provider management lives in Connect, together.
    expect(rowForView.models.id).toBe("providers-models");
  });

  it("keeps Agents and Roster as their own destinations", () => {
    // Folding these two into one row hid "Roster" behind a tab and renamed
    // "Agents" to something cleverer — and the operator immediately asked where
    // they had gone. Seeing what runs and managing identities are two jobs.
    expect(rowForView.agents.id).toBe("agents");
    expect(rowForView.agents.label).toBe("Agents");
    expect(rowForView.roster.id).toBe("roster");
    expect(rowForView.roster.label).toBe("Roster");
  });

  it("labels every view with the name the operator already knows it by", () => {
    // A rename is only worth its cost when the old label was WRONG — a monitor
    // named like a manager. Renaming a correctly-named view spends the
    // operator's recall for nothing. These are the corrective ones; anything
    // else should keep the name it shipped with.
    const CORRECTIVE: Record<string, string> = {
      providers: "Routing log", // was "Providers" — it is telemetry, not a manager
      tools: "Tool usage", // was "Tools" — same
      catalog: "Tool registry", // was "Catalog" — it registers tools, not models
      models: "Models & Keys", // was "Models" — API keys were undiscoverable
      artifacts: "Artifacts & Files", // absorbed the Files view
      standing: "Standing orders", // "Standing" is not a noun
      cache: "Prompt cache", // bare "Cache" is ambiguous
    };
    for (const [id, label] of Object.entries(CORRECTIVE)) {
      expect(NAV.find((v) => v.id === id)?.label, id).toBe(label);
    }
    // The familiar ones, spelled the way they always were.
    const FAMILIAR: Record<string, string> = {
      overview: "Overview",
      mission: "Mission Control",
      feed: "Live Stream",
      activity: "Activity",
      insights: "Insights",
      health: "Health",
      agents: "Agents",
      roster: "Roster",
      toolbox: "Toolbox",
      storage: "Storage",
      connections: "Connections",
    };
    for (const [id, label] of Object.entries(FAMILIAR)) {
      expect(NAV.find((v) => v.id === id)?.label, id).toBe(label);
    }
  });

  it("keeps a retired view id addressable instead of dropping it on the floor", () => {
    // Merging two views must not 404 the loser's hash into the chat fallback:
    // bookmarks, help `related` chips and other views' links still carry it.
    for (const [from, to] of Object.entries(VIEW_ALIASES)) {
      expect(NAV.some((n) => n.id === from), `${from} should be retired, not live`).toBe(false);
      expect(NAV.some((n) => n.id === to), `${from} → ${to} must be a real view`).toBe(true);
    }
    expect(VIEW_ALIASES.files).toBe("artifacts");
    expect(VIEW_ALIASES.config).toBe("configcenter");
    expect(VIEW_ALIASES.system).toBe("health");
    // A rename retires an id just as surely as a merge does.
    expect(VIEW_ALIASES.dashboard).toBe("overview");
  });

  it("makes every view findable by a word that is not in its label", () => {
    for (const v of NAV) {
      const label = v.label.toLowerCase();
      const extras = (v.keywords || "")
        .split(/\s+/)
        .filter((w) => w.length > 2 && !label.includes(w));
      expect(extras.length, `${v.id} needs search synonyms beyond its label`).toBeGreaterThan(2);
    }
  });

  it("routes the questions operators actually ask", () => {
    const find = (q: string) =>
      NAV.filter((v) => `${v.label} ${v.keywords || ""}`.toLowerCase().includes(q)).map((v) => v.id);
    expect(find("api key")).toContain("models");
    expect(find("cron")).toContain("schedules");
    expect(find("deny")).toContain("policy");
    expect(find("disk")).toContain("storage");
    expect(find("webhook")).toContain("workflows");
    expect(find("tts")).toContain("voice");
    expect(find("guardian")).toContain("roster");
    // The merged surfaces must still answer for what they absorbed.
    expect(find("file manager")).toContain("artifacts");
    expect(find("raw")).toContain("configcenter");
    expect(find("daemon")).toContain("health");
  });
});
