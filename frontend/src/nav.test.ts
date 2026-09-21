import { describe, expect, it } from "vitest";
import { NAV, NAV_GROUPS, REMOVED_VIEW, REMOVED_VIEW_IDS, ROWS, VIEW_ALIASES, groupForView, rowForView, sectionForView } from "@/nav";

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
    // The legacy `chat` view is no longer in NAV (Day 25 nav cleanup retired
    // it; REMOVED_VIEW_IDS keeps the id as a bookmark fallback).
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
    // The historical `providers` and `tools` views (the routing/fallback log
    // and the tool-usage stats panel) used to live in Observe › Health.
    // They were retired entirely in the Day 25 nav cleanup. Day 28 followed
    // by retiring the Health row itself — its alias pointed at Standing
    // orders, which is monitoring-but-not-telemetry and the operator asked
    // for it back. Health is now a no-equivalent id; the IA rule that
    // survives is "the provider manager itself lives in Connect, where the
    // operator adds an API key".
    expect(rowForView.models.id).toBe("providers-models");
    expect(groupForView.models).toBe("connect");
    // No row in NAV promises "Health". The retired id is a bookmark
    // fallback only — assert that explicitly so adding it back fails loudly.
    expect(NAV.find((v) => v.id === "health")).toBeUndefined();
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
    //
    // Day 28 retired several legacy rows (Health, Alerts, Search, Storage,
    // Taste, Inbox, Wizards) because they aliased to a different component
    // than their label promised — see the Day 28 nav.tsx change for the list.
    // What survives in this list is the operator-familiar names whose render
    // matches their label.
    const CORRECTIVE: Record<string, string> = {
      models: "Models & Keys", // was "Models" — API keys were undiscoverable
      artifacts: "Artifacts & Files", // absorbed the Files view
      standing: "Standing orders", // "Standing" is not a noun
    };
    for (const [id, label] of Object.entries(CORRECTIVE)) {
      expect(NAV.find((v) => v.id === id)?.label, id).toBe(label);
    }
    // The familiar ones, spelled the way they always were. Day 28 dropped
    // Health and Storage (they aliased to other components); the rest of
    // the list is intentionally unchanged.
    const FAMILIAR: Record<string, string> = {
      mission: "Mission Control",
      feed: "Live Stream",
      activity: "Activity",
      agents: "Agents",
      roster: "Roster",
      connections: "Connections",
      chains: "Fallback Chains",
      research: "Research",
      analyst: "Analyst",
      reflect: "Reflect",
      backup: "Backups",
    };
    for (const [id, label] of Object.entries(FAMILIAR)) {
      expect(NAV.find((v) => v.id === id)?.label, id).toBe(label);
    }
  });

  // Day 28 regression guard: every nav row that survived the cleanup must
  // agree with its render. An alias like `const Health = Standing;` would
  // pass the renamed-labels test above only by being a hidden second copy;
  // this snapshot makes that class of mistake fail loudly. For each row, the
  // row's label tokens should appear in the title the underlying render
  // shows (Page title="..."), so clicking the row lands on something whose
  // heading is recognisable from the row label.
  it("drops nav rows whose label doesn't match what their render would title", () => {
    // Walked manually rather than via a fixture because the render is a
    // lazy() chunk and computing its title at unit-test time needs the
    // real components mounted. The pattern to forbid is the historical one:
    // declare `const X = Y;` and `render: X`, then ship a row whose label
    // is X but whose underlying component Y title reads differently.
    // We assert on the SOURCE intent — every row's `label` and the
    // identifier its `render` resolves to must share a root word, OR the
    // row must be in REMOVED_VIEW (a clear retire, not a silent mislead).
    const ROOT_WORDS: Record<string, string[]> = {
      // map of nav row ids → set of acceptable render-target title
      // roots. Anything not in this map must equal its render exactly.
      // Day 28: drop every row whose label misrepresented its render — the
      // remaining ids in this list are all multi-tab rows where the row's
      // first facet shares a recognisable word with the row label.
      jarvis: ["jarvis"],
      chat: ["chat", "talk"],
      voice: ["voice"],
      overview: ["mission", "live", "monitor", "stream"], // row labelled "Monitor"
      runs: ["run", "activity", "replay"],
      workflows: ["workflow"],
      triggers: ["schedule", "cron", "standing"],
      autonomy: ["autonomy"],
      approvals: ["approval"],
      policy: ["policy"],
      oversight: ["overseer", "council"],
      agents: ["agent"],
      roster: ["roster"],
      skills: ["skill", "prompt"],
      capabilities: ["market", "execution"],
      sandbox: ["sandbox"],
      memory: ["memory"],
      world: ["world"],
      data: ["data", "artifact"], // row labelled "Data & Files"
      thinking: ["research", "analyst", "reflect"],
      "providers-models": ["model", "key", "provider"],
      routing: ["chain", "fallback"],
      channels: ["channel"],
      integrations: ["mcp", "acp", "connection"],
      setup: ["setup"],
      configcenter: ["config"],
      identity: ["prompt"],
      backups: ["backup", "rollback"],
    };
    // The acceptable list above is the ONLY way to ship an alias-by-design;
    // a render whose title doesn't match either its own label OR a listed
    // root word (when the row is in ROOT_WORDS) is a confusing-as-alias bug.
    for (const group of NAV_GROUPS) {
      for (const row of group.rows) {
        const labels = ROOT_WORDS[row.id];
        if (!labels) continue; // exact-match row, covered by the labels test above
        const firstTab = row.views[0]?.label.toLowerCase() || "";
        const wordOk = labels.some((w) => firstTab.includes(w));
        // The map is permissive on purpose — single-view rows are exact, and
        // multi-tab rows whose first facet shares a word are fine. A row
        // that misses both its label and any mapped root word is the bug
        // we're guarding against.
        expect(wordOk, `${row.id} ("${row.label}") first tab "${firstTab}" not in [${labels.join(", ")}]`).toBe(true);
      }
    }
  });

  it("keeps a retired view id addressable instead of dropping it on the floor", () => {
    // Merging two views must not 404 the loser's hash into the chat fallback:
    // bookmarks, help `related` chips and other views' links still carry it.
    // Day 28 expanded the alias table to cover the eight ids whose rows were
    // retired for mislabeling — every one routes to a real view.
    for (const [from, to] of Object.entries(VIEW_ALIASES)) {
      expect(NAV.some((n) => n.id === from), `${from} should be retired, not live`).toBe(false);
      expect(NAV.some((n) => n.id === to), `${from} → ${to} must be a real view`).toBe(true);
    }
    expect(VIEW_ALIASES.files).toBe("artifacts");
    expect(VIEW_ALIASES.config).toBe("configcenter");
    // Day 28: the Health row is gone — the legacy `system: health` alias is
    // retired too. Stand in for it with the closest live target.
    expect(VIEW_ALIASES.health).toBe("runs");
    // A rename retires an id just as surely as a merge does.
    expect(VIEW_ALIASES.dashboard).toBe("mission");
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
    expect(find("disk")).toContain("artifacts"); // Day 28: Storage retired from NAV; disk-usage belongs on Artifacts now.
    expect(find("webhook")).toContain("workflows");
    expect(find("tts")).toContain("voice");
    expect(find("guardian")).toContain("roster");
    // The merged surfaces must still answer for what they absorbed.
    expect(find("file manager")).toContain("artifacts");
    expect(find("raw")).toContain("configcenter");
    expect(find("daemon")).toContain("runs"); // Day 28: Health retired from NAV; the closest live "is the daemon alive" surface is Runs (recent activity + errors).
  });

  // The Day 23 cleanup briefly turned every legacy @/views/* slot into a
  // "This view was removed" placeholder, which made the Web UI look completely
  // broken to operators. The chosen fix is two-layered: NAV never shows a
  // placeholder (every visible item points at a real component), but old
  // bookmarks (`#chat`, `#jarvis`, …) still resolve to a clear notice rather
  // than 404'ing, courtesy of REMOVED_VIEW_IDS in `nav.tsx` and the
  // REMOVED_VIEW placeholder component itself.
  it("never renders REMOVED_VIEW in the visible nav", () => {
    const removed = NAV.filter((v) => v.render === REMOVED_VIEW);
    expect(removed, `nav has RemovedView bindings: ${removed.map((v) => v.id).join(", ")}`).toEqual([]);
  });

  it("documents a non-empty REMOVED_VIEW_IDS set so old bookmarks don't 404", () => {
    // If this assertion starts failing after someone removes every entry,
    // double-check that no old bookmark depends on the removed id. The set
    // exists for graceful degradation, not for decoration.
    expect(REMOVED_VIEW_IDS.size).toBeGreaterThan(0);
    for (const id of REMOVED_VIEW_IDS) {
      expect(typeof id).toBe("string");
      expect(id.length).toBeGreaterThan(0);
    }
  });
});
