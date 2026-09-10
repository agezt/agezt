import { describe, expect, it } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { NAV } from "@/nav";

// The console felt like a pile of pages because the same visual idea had been
// re-implemented over and over under different names: twenty-one page-local
// `*Panel` components, ten private tone→colour maps, five `Metric` tiles, three
// byte formatters. None of them were wrong on their own; together they meant a
// status chip, a stat, or the colour "warning" looked different depending on
// which page you were standing on.
//
// This guard keeps the primitives primitive. It is deliberately a small,
// specific list — it does not police layout or forbid page-specific components,
// only the handful of things that must look the same everywhere.

const SRC = new URL("./", import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1");

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) out.push(...walk(p));
    else if (/\.tsx?$/.test(entry) && !entry.includes(".test.")) out.push(p);
  }
  return out;
}

const FILES = [join(SRC, "components")]
  .flatMap(walk)
  .map((p) => ({ path: p.replace(SRC, "").replace(/\\/g, "/"), src: readFileSync(p, "utf8") }));

/** Files allowed to define a primitive — the primitive's own module. */
const owns = (path: string, ...owners: string[]) => owners.some((o) => path.endsWith(o));

describe("design system", () => {
  it("has files to check", () => {
    expect(FILES.length).toBeGreaterThan(30);
  });

  it("keeps one titled-section primitive", () => {
    // ui/section-panel is the shape: icon plate + title + status + body.
    const offenders = FILES.filter(
      (f) =>
        !owns(f.path, "ui/section-panel.tsx") &&
        // A local component that re-creates the icon-plate header. The plate is
        // the tell: a bordered size-8 square holding the section's icon.
        // ConfigSectionPanel slipped an earlier, narrower version of this check
        // by pairing the plate with an <h4> instead of an <h3>.
        /grid size-8 shrink-0 place-items-center rounded-lg border/.test(f.src),
    ).map((f) => f.path);
    expect(offenders, "use <SectionPanel> instead of re-creating its header").toEqual([]);
  });

  it("keeps one colour language", () => {
    // A private tone→class lookup means "warning" can drift per page.
    const offenders = FILES.filter(
      // An object-literal lookup only — `const toneCls = tone === "x" ? …` is a
      // single choice, not a private colour scale.
      (f) => !owns(f.path, "lib/tone.ts") && /const \w*[Tt]oneCls\b[^=]*=\s*\{/.test(f.src),
    ).map((f) => f.path);
    expect(offenders, "import the maps from @/lib/tone").toEqual([]);
  });

  it("keeps one compact stat tile", () => {
    // The copies did not all call themselves Metric: Tools and Reflect had a
    // `Tile`, Overseer a `Stat`. Naming was never the tell, so the guard now
    // reserves all four names instead of only the one it happened to catch.
    const offenders = FILES.filter(
      (f) => !owns(f.path, "ui/metric-widget.tsx") && /\nfunction (Metric|Tile|Stat|StatCard)\(/.test(f.src),
    ).map((f) => f.path);
    expect(offenders, "use <StatTile> from ui/metric-widget").toEqual([]);
  });

  it("keeps one byte formatter", () => {
    const offenders = FILES.filter(
      (f) => /\n(export )?function (humanSize|fmtBytes|formatBytes)\(/.test(f.src),
    ).map((f) => f.path);
    expect(offenders, "import { bytes } from @/lib/format").toEqual([]);
  });

  it("keeps one pill-toggle control", () => {
    // The hand-rolled pill: rounded-full border + the 11px type scale. It is a
    // real control (radio / switch) and the copies had no ARIA at all.
    const offenders = FILES.filter(
      (f) =>
        !owns(f.path, "ui/segmented.tsx") &&
        // px-2 and px-2.5 are the same pill to the eye, and pinning the exact
        // padding let five copies through (Board, Memory, Tools, World, the
        // event feed) — every one a real control with no ARIA at all. Anchored
        // to <button so a static badge in the same pill language is left alone.
        /<button[\s\S]{0,400}?rounded-full border[^"]*\bpx-2(\.5)?\b[^"]*text-\[11px\]/.test(f.src),
    ).map((f) => f.path);
    expect(offenders, "use <Segmented> or <ToggleChip> from ui/segmented").toEqual([]);
  });

  it("never repeats the filter row's counts as a metric band", () => {
    // Found seven times in one review pass — Alerts, Runs, Roster, Channels,
    // Standing orders, the marketplace and the fleet census all printed a row
    // of stat tiles directly above a filter strip carrying the very same
    // counts. Two renderings of one set of numbers, a line apart, and only one
    // of them is clickable. The filter row wins: it answers "how many" AND
    // narrows to them.
    const offenders: string[] = [];
    for (const f of FILES) {
      const tiles = new Set<string>();
      for (const m of f.src.matchAll(/<(?:MetricWidget|StatTile)[\s\S]{0,300}?label=\{?["`]([^"`]+)["`]/g)) {
        tiles.add(m[1].trim().toLowerCase());
      }
      if (tiles.size === 0) continue;
      const cuts = new Set<string>();
      for (const m of f.src.matchAll(/label:\s*["`]([^"`]+)["`]/g)) cuts.add(m[1].trim().toLowerCase());
      for (const m of f.src.matchAll(/<(?:Segmented|TabNav)[\s\S]{0,900}?label=["`]([^"`]+)["`]/g)) {
        cuts.add(m[1].trim().toLowerCase());
      }
      const shared = [...tiles].filter((t) => cuts.has(t));
      // One shared word is a coincidence ("All"); two is the page saying the
      // same thing twice. This is a smell test, not a proof: Schedules got
      // through with a single overlap ("attention") and was still the same
      // bug, and lowering the bar to one flags two honest pages where the
      // tiles and the chips live on different tabs. Look at the page too.
      if (shared.length >= 2) offenders.push(`${f.path} (${shared.join(", ")})`);
    }
    expect(offenders, "let the filter chips carry the counts; drop the tiles").toEqual([]);
  });

  it("titles each page after the nav label the operator clicked", () => {
    // The sidebar said "Overview", the tab said "Overview", the page heading
    // said "Dashboard" — three words for one destination. A page may still omit
    // a title (Chat, Jarvis and the other headerless surfaces); it just must not
    // contradict the nav.
    const offenders: string[] = [];
    for (const item of NAV) {
      const f = FILES.find((x) => x.path.toLowerCase().endsWith(`/${item.id.replace(/-/g, "")}.tsx`));
      if (!f) continue;
      const m = f.src.match(/<Page[\s\S]{0,400}?title="([^"]+)"/);
      if (!m) continue;
      // Not "identical" — a heading may elaborate ("Journal search" for Search,
      // "Backup & Restore" for Backup). It must not be an UNRELATED word: the
      // nav said "Routing log" while the page still said "Providers", because a
      // rename landed in nav.tsx and nowhere else.
      const title = m[1].toLowerCase();
      const label = item.label.toLowerCase();
      if (!title.includes(label) && !label.includes(title)) {
        offenders.push(`${f.path.split("/").pop()}: <Page title="${m[1]}"> vs nav "${item.label}"`);
      }
    }
    expect(offenders, "match <Page title> to the nav label").toEqual([]);
  });

  it("keeps the tone opacities on one scale", () => {
    // Eight border opacities for "warning" and thirteen background tints for
    // "accent" is drift, not design — it is why the same amber outline looked
    // subtly different on every page. Borders read at three strengths
    // (30 subtle / 40 standard / 60 strong) and tone tints at four
    // (5 / 10 / 15 / 20). Anything heavier than /20 on a background is a scrim
    // or a solid fill, not a tint, and is left alone.
    const BORDER_STEPS = new Set(["30", "40", "60"]);
    const BG_STEPS = new Set(["5", "10", "15", "20"]);
    const offenders: string[] = [];
    for (const f of FILES) {
      for (const m of f.src.matchAll(/(border|bg)-(good|warn|bad|accent)\/(\d+)/g)) {
        const [cls, slot, , value] = m;
        if (slot === "border" ? !BORDER_STEPS.has(value) : Number(value) <= 20 && !BG_STEPS.has(value)) {
          offenders.push(`${f.path}: ${cls}`);
        }
      }
    }
    expect([...new Set(offenders)], "snap to the tone scale (see @/lib/tone)").toEqual([]);
  });

  it("does not use TabNav as a filter row", () => {
    // TabNav switches PANELS and owns their content; Segmented switches how the
    // SAME content is shown. Four views passed `content: null` to fake a filter
    // row out of a tablist — which renders empty tab panels, and tells a screen
    // reader "tab" for what is really an exclusive choice among filters.
    const offenders = FILES.filter((f) => /content:\s*null/.test(f.src)).map((f) => f.path);
    expect(offenders, "use <Segmented> for filters; TabNav is for real panels").toEqual([]);
  });

  it("lets the Page scaffold draw the page header", () => {
    // A view that renders its own gradient <h2> inside <Page> ends up with two
    // competing titles and its own spacing — Sandbox did exactly this.
    const offenders = FILES.filter(
      (f) => /<Page[\s>]/.test(f.src) && /<h2 className="[^"]*text-gradient/.test(f.src),
    ).map((f) => f.path);
    expect(offenders, "pass icon/title/description/actions to <Page>").toEqual([]);
  });
});
