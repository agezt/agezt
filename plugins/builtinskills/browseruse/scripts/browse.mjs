// browser-use driver — a stateless Playwright runner the agent calls via code_exec.
//
// Usage:  node browse.mjs '<json-spec>'   (or pipe the JSON on stdin)
// Spec:   { url, actions?: [...], screenshot?: bool, extract?: "text"|"html"|"<css>", timeout_ms?: number }
// Output: one JSON object on stdout: { ok, url, title, text, screenshot } or { ok:false, error }.
//
// Each invocation opens a fresh headless Chromium, runs the ordered actions,
// optionally screenshots, extracts, and exits — so the agent drives an explicit
// see/act loop without any hidden session state.

import { chromium } from "playwright";
import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

function readSpec() {
  const arg = process.argv[2];
  if (arg && arg.trim() !== "") return JSON.parse(arg);
  // Fall back to stdin (fd 0).
  const data = readFileSync(0, "utf8");
  if (data.trim()) return JSON.parse(data);
  throw new Error("no spec: pass a JSON spec as argv[2] or on stdin");
}

async function run(spec) {
  if (!spec || !spec.url) throw new Error("spec.url is required");
  const timeout = Number(spec.timeout_ms) > 0 ? Number(spec.timeout_ms) : 30000;
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage();
    page.setDefaultTimeout(timeout);
    await page.goto(spec.url, { waitUntil: "domcontentloaded", timeout });

    for (const a of spec.actions || []) {
      switch (a.type) {
        case "goto":
          await page.goto(a.url, { waitUntil: "domcontentloaded", timeout });
          break;
        case "click":
          await page.click(a.selector, { timeout });
          break;
        case "fill":
          await page.fill(a.selector, String(a.value ?? ""), { timeout });
          break;
        case "press":
          await page.press(a.selector, a.key || "Enter", { timeout });
          break;
        case "wait":
          if (a.selector) await page.waitForSelector(a.selector, { timeout });
          else await page.waitForTimeout(Number(a.ms) > 0 ? Number(a.ms) : 1000);
          break;
        default:
          throw new Error("unknown action type: " + a.type);
      }
    }

    const out = { ok: true, url: page.url(), title: await page.title() };

    const extract = spec.extract ?? "text";
    if (extract === "html") {
      out.text = await page.content();
    } else if (extract === "text") {
      out.text = (await page.innerText("body")).slice(0, 200000);
    } else {
      // Treat as a selector: join the matched elements' visible text.
      const texts = await page.locator(extract).allInnerTexts();
      out.text = texts.join("\n").slice(0, 200000);
    }

    if (spec.screenshot !== false) {
      const dir = mkdtempSync(join(tmpdir(), "browseruse-"));
      const shot = join(dir, "page.png");
      await page.screenshot({ path: shot, fullPage: false });
      out.screenshot = shot;
    }

    return out;
  } finally {
    await browser.close();
  }
}

(async () => {
  try {
    const out = await run(readSpec());
    process.stdout.write(JSON.stringify(out));
  } catch (err) {
    process.stdout.write(JSON.stringify({ ok: false, error: String(err && err.message ? err.message : err) }));
    process.exitCode = 1;
  }
})();
