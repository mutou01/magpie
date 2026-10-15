const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const titles = { en: "Prompt templates", zh: "提示词模板", "zh-TW": "提示詞範本", ja: "プロンプトテンプレート", de: "Prompt-Vorlagen" };
for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: prompt templates preserve drafts, show conflicts, and manage native targets`, async (t) => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch(process.env.CHROMIUM_EXECUTABLE ? { executablePath: process.env.CHROMIUM_EXECUTABLE } : { channel: "chromium" }));
    t.after(() => browser.close());
    for (const lang of Object.keys(titles)) await t.test(lang, async () => {
      const ctx = await browser.newContext({ viewport: { width: 560, height: 860 }, reducedMotion: "reduce" });
      t.after(() => ctx.close());
      const page = await ctx.newPage();
      page.setDefaultTimeout(3000);
      const errors = [], sent = [];
      page.on("pageerror", (e) => errors.push(e.message));
      const targets = [{ id: "claude", name: "Claude Code", detected: true, verification: "pending", reload: "Restart the harness to load updated commands" }, { id: "codex-desktop", name: "Codex Desktop", detected: true, shared: true, verification: "pending", reload: "Skills refresh automatically; restart Codex Desktop if the command is missing" }];
      let items = [], failSave = true, releaseCheck;
      const states = [{ agent: "claude", status: "available" }, { agent: "codex-desktop", status: "conflict", reason: "This name is already in use", source: "/fixture/.agents/skills/审查/SKILL.md" }];
      const prompts = () => ({ items, targets });
      await ctx.addInitScript(() => localStorage.setItem("magpie.libTab", "instructions"));
      await page.route("**/*", async (route) => {
        const url = new URL(route.request().url()), p = url.pathname;
        if (p === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs={lang:${JSON.stringify(lang)},theme:"light",web:true}` });
        if (p === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window={}" });
        if (p === "/api/state") return route.fulfill({ json: { agents: [], profiles: [], settings: { lang, theme: "light" } } });
        if (p === "/api/plugins") return route.fulfill({ json: { plugins: [] } });
        if (p === "/api/providers") return route.fulfill({ json: { providers: [], gateway: { running: true } } });
        if (p === "/api/library") return route.fulfill({ json: { agents: [], servers: [], skills: [], foundSkills: [], foundServers: [], projects: [], prompts: prompts(), instructions: { shared: "", sets: [{ id: "default", name: "", text: "", active: true }], agents: [] } } });
        if (p === "/api/library/prompts") return route.fulfill({ json: prompts() });
        if (p.startsWith("/api/library/prompts/")) {
          const body = route.request().postDataJSON(); sent.push({ p, body });
          if (p.endsWith("/check")) {
            if (body.name === "waiting-template") await new Promise(resolve => { releaseCheck = resolve; });
            return route.fulfill({ json: states });
          }
          if (p.endsWith("/save") && failSave) { failSave = false; return route.fulfill({ status: 500, body: "fixture save failed" }); }
          if (p.endsWith("/save")) items = [{ ...body, id: "p1", states: states.map(s => s.agent === "claude" ? { ...s, status: "enabled" } : s) }];
          if (p.endsWith("/remove")) items = [];
          return route.fulfill({ json: { part: "prompts", prompts: prompts(), result: { changed: ["claude"], problems: [] } } });
        }
        if (p.startsWith("/api/")) return route.fulfill({ json: p === "/api/usage/quotas" ? [] : {} });
        const file = path.join(assets, p === "/" ? "index.html" : p);
        return route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml" }[path.extname(file)] });
      });
      await page.goto("http://magpie.test/");
      await page.locator('button[data-view="library"]').click();
      await page.locator(".lib-prompts").waitFor().catch(async e => { throw new Error(e.message + "\n" + JSON.stringify(errors) + "\n" + await page.locator("#view-library").textContent()); });
      assert((await page.locator(".lib-prompts").textContent()).includes(titles[lang]));
      await page.locator(".lib-prompt-add").click();
      await page.locator('[data-prompt="name"]').fill("waiting-template");
      await page.locator('[data-prompt="text"]').fill("Draft");
      const preflight = page.waitForRequest(r => r.url().endsWith("/prompts/check"));
      await page.locator(".lib-prompt-save").click();
      await preflight;
      try { assert(await page.locator("#modal .bar button").first().isDisabled(), "Cancel stays disabled during the save's preflight"); }
      finally { releaseCheck?.(); }
      await page.locator(".lib-prompt-error").filter({ hasText: "fixture save failed" }).waitFor();
      failSave = true; sent.length = 0;
      await page.locator('[data-prompt="name"]').fill("审查");
      await page.locator('[data-prompt="text"]').fill("检查边界条件。");
      await page.locator(".lib-prompt-check").click();
      await page.locator('.lib-prompt-target[data-agent="codex-desktop"] .lib-prompt-reason').waitFor();
      assert.equal((await page.locator("#status").textContent()).trim(), "", "a fresh availability check clears the previous save error");
      assert((await page.locator('.lib-prompt-target[data-agent="codex-desktop"]').textContent()).includes("/fixture/.agents/skills/审查/SKILL.md"));
      assert(await page.locator('.lib-prompt-target[data-agent="codex-desktop"] input').isDisabled());
      if (process.env.ARTIFACT_DIR && lang === "zh") {
        await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
        await page.evaluate(() => Promise.all(document.getAnimations().filter(a => a.effect?.getTiming().iterations !== Infinity).map(a => a.finished.catch(() => {}))));
        await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `prompt-templates-${engine}.png`), fullPage: true });
      }
      await page.locator(".lib-prompt-save").click();
      await page.locator(".lib-prompt-error").filter({ hasText: "fixture save failed" }).waitFor();
      assert.equal(await page.locator('[data-prompt="text"]').inputValue(), "检查边界条件。");
      await page.locator(".lib-prompt-save").click();
      await page.locator(".lib-prompt-row").waitFor();
      assert.equal(sent.filter(s => s.p.endsWith("/save")).length, 2);
      assert.deepEqual(sent.find(s => s.p.endsWith("/save")).body.agents, ["claude"]);
      await page.locator(".lib-prompt-search").fill("unmatched");
      assert.equal(await page.locator(".lib-prompt-row").count(), 0);
      await page.locator(".lib-prompt-search").fill("边界");
      await page.locator(".lib-prompt-row").click();
      assert.equal(await page.locator('[data-prompt="name"]').inputValue(), "审查");
      assert.equal(await page.locator("#modal select").count(), 0);
      assert(await page.locator("#modal").evaluate(el => el.scrollWidth <= el.clientWidth + 1));
      await page.locator(".lib-prompt-remove").click();
      await page.locator(".lib-prompt-confirm-remove").click();
      await page.locator(".lib-prompt-row").waitFor({ state: "detached" });
      assert.deepEqual(errors, []);
      await ctx.close();
    });
  });
}
