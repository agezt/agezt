// scripts/probe-taste.cjs — visit Taste directly, dump the body.
const cp = require("node:child_process");

const SCRIPT = `
const { chromium } = require("playwright");
(async () => {
  const browser = await chromium.launch();
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log("[pageerror]", String(e)));
  page.on("console", (m) => {
    if (m.type() === "error") console.log("[console.error]", m.text());
  });
  await page.addInitScript(() => localStorage.setItem("agezt.setup.skipped", "1"));
  await page.goto(process.env.URL, { waitUntil: "domcontentloaded" });
  // Visit memory first so the chunk is cached, then switch to taste.
  await page.evaluate(() => location.hash = "#/memory");
  await page.waitForTimeout(2000);
  await page.evaluate(() => location.hash = "#/taste");
  await page.waitForTimeout(3000);
  const text = (await page.locator("main").first().textContent()) || "";
  const html = await page.locator("[data-view-root]").innerHTML();
  console.log("---main text---");
  console.log(text.slice(0, 500));
  console.log("---data-view-root html (first 1500)---");
  console.log(html.slice(0, 1500));
  console.log("---view-root data-view---");
  console.log(await page.locator("[data-view-root]").getAttribute("data-view"));
  await page.screenshot({ path: process.env.OUT, fullPage: false });
  await browser.close();
})();
`;

const env = {
  ...process.env,
  URL: process.env.AGEZT_WEBUI_URL,
  OUT: process.env.OUT || "D:/tmp/taste.png",
};
cp.execSync(`node -e ${JSON.stringify(SCRIPT)}`, { stdio: "inherit", env });
