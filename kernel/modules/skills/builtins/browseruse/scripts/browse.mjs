// browser-use driver — a Playwright runner the agent calls via code_exec.
//
// Usage:  node browse.mjs '<json-spec>'   (or pipe the JSON on stdin)
// Spec:   { url, actions?: [...], screenshot?: bool, full_page?: bool,
//           snapshot?: bool, snapshot_limit?: number, events?: bool,
//           downloads?: bool, cookies?: bool,
//           profile?: "isolated"|"session"|"user-attached"|"remote-cdp",
//           session_id?: string, user_data_dir?: string, cdp_url?: string, extract?: "text"|"html"|"<css>",
//           timeout_ms?: number, viewport?: { width, height } }
// Output: one JSON object on stdout:
//   { ok, url, title, text, snapshot, screenshot, downloads, cookies, events } or { ok:false, error }.
//
// Each invocation opens headless Chromium, runs the ordered actions, optionally
// screenshots, extracts, and exits. The default isolated profile has no hidden
// state; profile=session persists state in an AGEZT-managed user_data_dir.

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
  const eventLimit = clampInt(spec.event_limit, 50, 0, 200);
  const snapshotLimit = clampInt(spec.snapshot_limit, 60, 0, 200);
  const wantEvents = spec.events !== false;
  const wantDownloads = spec.downloads !== false;
  const wantCookies = spec.cookies === true;
  const profile = String(spec.profile || "isolated");
  const session = await openBrowserSession(spec, wantDownloads);
  const consoleEvents = [];
  const pageErrors = [];
  const networkEvents = [];
  const requestFailures = [];
  const downloads = [];
  const downloadPromises = [];
  let page;

  try {
    page = await session.context.newPage();
    page.setDefaultTimeout(timeout);

    if (wantEvents) {
      page.on("console", (msg) => {
        pushLimited(consoleEvents, eventLimit, {
          type: msg.type(),
          text: truncate(String(msg.text()), 500),
        });
      });
      page.on("pageerror", (err) => {
        pushLimited(pageErrors, eventLimit, truncate(String(err && err.message ? err.message : err), 500));
      });
      page.on("response", (res) => {
        const req = res.request();
        pushLimited(networkEvents, eventLimit, {
          method: req.method(),
          url: truncate(res.url(), 500),
          status: res.status(),
          type: req.resourceType(),
        });
      });
      page.on("requestfailed", (req) => {
        pushLimited(requestFailures, eventLimit, {
          method: req.method(),
          url: truncate(req.url(), 500),
          type: req.resourceType(),
          error: truncate(String(req.failure()?.errorText || ""), 300),
        });
      });
    }

    if (wantDownloads) {
      const downloadDir = mkdtempSync(join(tmpdir(), "browseruse-downloads-"));
      page.on("download", (download) => {
        const p = (async () => {
          const suggested = sanitizeFileName(download.suggestedFilename() || "download.bin");
          const target = join(downloadDir, uniqueDownloadName(downloads, suggested));
          await download.saveAs(target);
          downloads.push({
            url: truncate(download.url(), 500),
            suggested_filename: suggested,
            path: target,
          });
        })();
        downloadPromises.push(p);
      });
    }

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
        case "type":
          await page.type(a.selector, String(a.value ?? ""), { timeout, delay: clampInt(a.delay_ms, 0, 0, 1000) });
          break;
        case "press":
          await page.press(a.selector, a.key || "Enter", { timeout });
          break;
        case "select":
          await page.selectOption(a.selector, String(a.value ?? ""), { timeout });
          break;
        case "check":
          await page.check(a.selector, { timeout });
          break;
        case "uncheck":
          await page.uncheck(a.selector, { timeout });
          break;
        case "hover":
          await page.hover(a.selector, { timeout });
          break;
        case "scroll":
          await scrollPage(page, a);
          break;
        case "wait":
          if (a.selector) await page.waitForSelector(a.selector, { timeout });
          else await page.waitForTimeout(Number(a.ms) > 0 ? Number(a.ms) : 1000);
          break;
        default:
          throw new Error("unknown action type: " + a.type);
      }
    }

    const out = { ok: true, url: page.url(), title: await page.title(), profile: { mode: profile } };
    if (spec.session_id) out.profile.session_id = String(spec.session_id);
    if (spec.tab_id) {
      out.profile.tab_id = String(spec.tab_id);
      out.tab_id = String(spec.tab_id);
    }

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
      await page.screenshot({ path: shot, fullPage: spec.full_page === true });
      out.screenshot = shot;
    }

    if (spec.snapshot !== false && snapshotLimit > 0) {
      out.snapshot = await snapshotPage(page, snapshotLimit);
    }

    if (downloadPromises.length > 0) {
      await Promise.allSettled(downloadPromises);
    }
    if (downloads.length > 0) {
      out.downloads = downloads;
    }
    if (wantEvents) {
      out.events = {
        console: consoleEvents,
        page_errors: pageErrors,
        network: networkEvents,
        request_failed: requestFailures,
      };
    }
    if (wantCookies) {
      out.cookies = await session.context.cookies(page.url());
    }

    return out;
  } finally {
    if (session.closePage && page) {
      await page.close().catch(() => {});
    }
    if (session.closeContext && session.context) {
      await session.context.close().catch(() => {});
    }
    if (session.closeBrowser && session.browser) {
      await session.browser.close().catch(() => {});
    }
  }
}

async function openBrowserSession(spec, wantDownloads) {
  const profile = String(spec.profile || "isolated");
  const contextOptions = {
    acceptDownloads: wantDownloads,
    viewport: viewportFromSpec(spec),
  };
  switch (profile) {
    case "isolated": {
      const browser = await chromium.launch({ headless: true });
      const context = await browser.newContext(contextOptions);
      return { browser, context, closeContext: true, closeBrowser: true };
    }
    case "session": {
      if (!spec.user_data_dir) throw new Error("profile session requires user_data_dir");
      const context = await chromium.launchPersistentContext(spec.user_data_dir, {
        ...contextOptions,
        headless: true,
      });
      return { context, closeContext: true, closeBrowser: false };
    }
    case "user-attached": {
      if (!spec.user_data_dir) throw new Error("profile user-attached requires user_data_dir");
      const context = await chromium.launchPersistentContext(spec.user_data_dir, {
        ...contextOptions,
        headless: true,
      });
      return { context, closeContext: true, closeBrowser: false };
    }
    case "remote-cdp": {
      if (!spec.cdp_url) throw new Error("profile remote-cdp requires cdp_url");
      const browser = await chromium.connectOverCDP(spec.cdp_url);
      let context = browser.contexts()[0];
      let closeContext = false;
      if (!context) {
        context = await browser.newContext(contextOptions);
        closeContext = true;
      }
      return { browser, context, closePage: !closeContext, closeContext, closeBrowser: false };
    }
    default:
      throw new Error("unknown profile: " + profile);
  }
}

async function scrollPage(page, action) {
  const x = Number.isFinite(Number(action.x)) ? Number(action.x) : 0;
  const y = Number.isFinite(Number(action.y)) ? Number(action.y) : 800;
  if (action.selector) {
    const locator = page.locator(action.selector).first();
    await locator.scrollIntoViewIfNeeded();
    await locator.evaluate((el, delta) => {
      if (typeof el.scrollBy === "function") el.scrollBy(delta.x, delta.y);
      else window.scrollBy(delta.x, delta.y);
    }, { x, y });
    return;
  }
  await page.mouse.wheel(x, y);
}

async function snapshotPage(page, limit) {
  const selector = [
    "a[href]",
    "button",
    "input",
    "textarea",
    "select",
    "summary",
    "[role]",
    "[aria-label]",
    "[contenteditable='true']",
  ].join(",");
  return await page.locator(selector).evaluateAll((elements, max) => {
    function visible(el) {
      const style = window.getComputedStyle(el);
      const rect = el.getBoundingClientRect();
      return style &&
        style.visibility !== "hidden" &&
        style.display !== "none" &&
        rect.width > 0 &&
        rect.height > 0;
    }
    function textOf(el) {
      const tag = el.tagName.toLowerCase();
      const inputType = (el.getAttribute("type") || "").toLowerCase();
      const raw = el.getAttribute("aria-label") ||
        el.getAttribute("title") ||
        el.getAttribute("alt") ||
        el.getAttribute("placeholder") ||
        (tag === "input" && ["button", "submit", "reset"].includes(inputType) ? el.value : "") ||
        el.innerText ||
        el.textContent ||
        "";
      return String(raw).replace(/\s+/g, " ").trim().slice(0, 160);
    }
    function roleOf(el) {
      const explicit = el.getAttribute("role");
      if (explicit) return explicit;
      const tag = el.tagName.toLowerCase();
      const type = (el.getAttribute("type") || "").toLowerCase();
      if (tag === "a") return "link";
      if (tag === "button") return "button";
      if (tag === "select") return "combobox";
      if (tag === "textarea") return "textbox";
      if (tag === "summary") return "button";
      if (tag === "input") {
        if (["button", "submit", "reset"].includes(type)) return "button";
        if (type === "checkbox") return "checkbox";
        if (type === "radio") return "radio";
        return "textbox";
      }
      return "element";
    }
    function esc(s) {
      if (window.CSS && typeof window.CSS.escape === "function") return window.CSS.escape(s);
      return String(s).replace(/[^a-zA-Z0-9_-]/g, "\\$&");
    }
    function cssPath(el) {
      if (el.id) return "#" + esc(el.id);
      const parts = [];
      for (let n = el; n && n.nodeType === Node.ELEMENT_NODE && n !== document.body; n = n.parentElement) {
        let part = n.tagName.toLowerCase();
        const parent = n.parentElement;
        if (parent) {
          const same = Array.from(parent.children).filter((child) => child.tagName === n.tagName);
          if (same.length > 1) part += `:nth-of-type(${same.indexOf(n) + 1})`;
        }
        parts.unshift(part);
        if (parts.length >= 5) break;
      }
      return parts.join(" > ");
    }
    const out = [];
    for (const el of elements) {
      if (out.length >= max) break;
      if (!visible(el)) continue;
      out.push({
        ref: `e${out.length + 1}`,
        role: roleOf(el),
        name: textOf(el),
        selector: cssPath(el),
        tag: el.tagName.toLowerCase(),
        disabled: Boolean(el.disabled || el.getAttribute("aria-disabled") === "true"),
      });
    }
    return out;
  }, limit);
}

function viewportFromSpec(spec) {
  const width = clampInt(spec.viewport?.width, 1280, 320, 3840);
  const height = clampInt(spec.viewport?.height, 720, 240, 2160);
  return { width, height };
}

function clampInt(value, fallback, min, max) {
  const n = Number(value);
  if (!Number.isFinite(n)) return fallback;
  return Math.max(min, Math.min(max, Math.floor(n)));
}

function pushLimited(list, limit, value) {
  if (limit <= 0 || list.length >= limit) return;
  list.push(value);
}

function truncate(value, max) {
  const s = String(value ?? "");
  if (s.length <= max) return s;
  return s.slice(0, max) + "...[truncated]";
}

function sanitizeFileName(name) {
  const cleaned = String(name).replace(/[\\/:*?"<>|]/g, "_").trim();
  return cleaned || "download.bin";
}

function uniqueDownloadName(downloads, name) {
  if (!downloads.some((d) => d.suggested_filename === name)) return name;
  const dot = name.lastIndexOf(".");
  const base = dot > 0 ? name.slice(0, dot) : name;
  const ext = dot > 0 ? name.slice(dot) : "";
  return `${base}-${downloads.length + 1}${ext}`;
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
