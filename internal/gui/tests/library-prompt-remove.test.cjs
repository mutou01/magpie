const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: prompt list deletion preserves drafts, search and keyboard position`, async t => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) await t.test(lang, async () => {
      const ctx = await browser.newContext({ viewport: { width: 560, height: 860 }, reducedMotion: "reduce" });
      t.after(() => ctx.close());
      const page = await ctx.newPage();
      page.setDefaultTimeout(4000);
      const errors = [], sent = [], targetPosts = [], checkPosts = [];
      page.on("pageerror", e => errors.push(e.message));
      const targets = [{ id: "claude", name: "Claude Code", icon: "claudecode-color", detected: true, verification: "pending" }];
      let items = ["a", "b", "c"].map(id => ({ id, name: `template-${id}`, text: "needle 正文", agents: ["claude"], states: [{ agent: "claude", status: "enabled" }] }));
      let failRemove = true, holdAgents = true, releaseAgents, failAgents = false, holdRead = false, releaseRead, holdRemove = false, releaseRemove;
      const prompts = () => ({ enabled: true, items, targets });
      const theme = ["ja", "de"].includes(lang) ? "dark" : "light";
      await ctx.addInitScript(() => localStorage.setItem("magpie.libTab", "instructions"));
      await page.route("**/*", async route => {
        const url = new URL(route.request().url()), p = url.pathname;
        if (!["http:", "https:"].includes(url.protocol)) return route.continue();
        if (p === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs={lang:${JSON.stringify(lang)},theme:${JSON.stringify(theme)},web:true}` });
        if (p === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window={}" });
        if (p === "/api/state") return route.fulfill({ json: { agents: [], profiles: [], settings: { lang, theme } } });
        if (p === "/api/plugins") return route.fulfill({ json: { plugins: [] } });
        if (p === "/api/providers") return route.fulfill({ json: { providers: [], gateway: { running: true } } });
        if (p === "/api/library") return route.fulfill({ json: { agents: [{ id: "claude", name: "Claude Code", icon: "claudecode-color" }], servers: [], skills: [], foundSkills: [], foundServers: [], projects: [], prompts: prompts(), instructions: { shared: "", sets: [{ id: "default", name: "", text: "", active: true }], agents: [] } } });
        if (p === "/api/library/prompts/check") { checkPosts.push(route.request().postDataJSON()); return route.fulfill({ json: [{ agent: "claude", status: "available" }] }); }
        if (p === "/api/library/prompts") {
          const view = prompts();
          if (holdRead) { holdRead = false; await new Promise(resolve => { releaseRead = resolve; }); }
          return route.fulfill({ json: view });
        }
        if (p === "/api/library/prompts/agents") {
          const body = route.request().postDataJSON(); targetPosts.push(body);
          if (holdAgents) { holdAgents = false; await new Promise(resolve => { releaseAgents = resolve; }); }
          if (failAgents) { failAgents = false; return route.fulfill({ status: 500, body: "fixture target failure" }); }
          items = items.map(x => x.id === body.id ? { ...x, agents: body.agents, states: [{ agent: "claude", status: body.agents.length ? "enabled" : "available" }] } : x);
          return route.fulfill({ json: { part: "prompts", prompts: prompts(), result: { changed: [], problems: [] } } });
        }
        if (p === "/api/library/prompts/remove") {
          const body = route.request().postDataJSON(); sent.push(body);
          if (failRemove) { failRemove = false; return route.fulfill({ status: 500, body: "fixture remove failed" }); }
          if (holdRemove) { holdRemove = false; await new Promise(resolve => { releaseRemove = resolve; }); }
          items = items.filter(x => x.id !== body.id);
          return route.fulfill({ json: { part: "prompts", prompts: prompts(), result: { changed: [], problems: body.id === "b" ? [{ agent: "claude", error: "fixture externally edited deployment kept" }] : [] } } });
        }
        if (p.startsWith("/api/")) return route.fulfill({ json: p === "/api/usage/quotas" ? [] : {} });
        const file = path.join(assets, p === "/" ? "index.html" : p);
        return route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml" }[path.extname(file)] });
      });
      await page.goto("http://magpie.test/");
      await page.locator('button[data-view="library"]').click();
      await page.locator(".lib-prompt-row").first().waitFor();
      const row = id => page.locator(`.lib-prompt-row[data-prompt-id="${id}"]`);
      const remove = id => row(id).locator(".lib-prompt-delete");
      const dialog = page.locator(".lib-prompt-delete-confirm");
      const submit = () => dialog.locator(".danger-fill");
      assert.equal(await remove("b").count(), 1, "templates expose the same row delete control as skills");
      assert(await remove("b").evaluate(e => e.classList.contains("lib-icon") && e.closest(".lib-rowacts")), "delete reuses the skill icon styles");
      await remove("b").focus();
      await page.waitForFunction(() => getComputedStyle(document.activeElement.parentElement).opacity === "1");
      assert.equal(await remove("b").evaluate(e => getComputedStyle(e.parentElement).opacity), "1", "keyboard focus reveals the delete button");
      await page.locator(".lib-prompt-search").fill("needle");
      await remove("b").press("Space");
      await dialog.waitFor();
      assert((await dialog.textContent()).includes("/template-b"));
      assert((await dialog.textContent()).includes({ en: "backup", zh: "备份", "zh-TW": "備份", ja: "バックアップ", de: "Sicherung" }[lang]), "the localized confirmation explains backup and deployment effects");
      assert.equal(await page.locator(".lib-prompt-editor").count(), 0, "deleting does not also open the editor");
      await dialog.locator(".bar button").first().press("Space");
      await page.locator("#modal").waitFor({ state: "hidden" });
      assert.equal(sent.length, 0, "cancel never posts a removal");
      const chip = () => row("a").locator('[data-agent="claude"]');
      const busy = () => page.waitForFunction(() => !document.querySelector('.lib-prompt-row .lib-agents[aria-busy="true"]'));
      const agentRequest = page.waitForRequest(r => r.url().endsWith("/prompts/agents"));
      await chip().press("Space");
      await agentRequest;
      try {
        assert(await remove("b").isDisabled(), "another template's target update blocks every list deletion");
        const refresh = await page.locator('.lib-prompts .lib-cardhead button:not([role="switch"])').first().elementHandle();
        const checkedBefore = checkPosts.length;
        await refresh.press("Space");
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        assert.equal(checkPosts.length, checkedBefore, "refresh cannot start native checks during a target write");
        assert(await remove("b").isDisabled(), "a list refresh cannot bypass the pending-target guard");
        await row("b").press("Enter");
        assert(await page.locator(".lib-prompt-remove").isDisabled(), "editor deletion observes the same global guard");
        await page.locator(".lib-prompt-remove").dispatchEvent("click");
        await page.locator(".lib-prompt-confirm-remove").dispatchEvent("click");
        assert.equal(sent.length, 0, "the editor's final handler rechecks the global guard");
      } finally { releaseAgents?.(); }
      await busy();
      assert(await page.locator(".lib-prompt-remove").isEnabled(), "completion restores editor deletion");
      await page.locator(".lib-prompt-editor .bar button").nth(1).press("Space");
      await page.locator("#modal").waitFor({ state: "hidden" });
      failAgents = true; holdRead = true;
      const reread = page.waitForRequest(r => r.url().endsWith("/library/prompts"));
      await chip().press("Space");
      await reread;
      try { assert(await remove("b").isDisabled(), "failed target writes hold deletion until the recovery read finishes"); }
      finally { releaseRead?.(); }
      await busy();
      assert(await remove("b").isEnabled());
      await page.locator("#view-library").hover();
      await page.mouse.wheel(0, -2000);
      await page.waitForFunction(() => document.querySelector("#view-library").scrollTop === 0);
      await page.locator(".lib-set .name").first().click();
      await page.locator('textarea[data-lib="set:default"]').fill("尚未保存的共享指令");
      holdRead = true;
      const staleRead = page.waitForRequest(r => r.url().endsWith("/library/prompts"));
      const staleRefresh = await page.locator('.lib-prompts .lib-cardhead button:not([role="switch"])').first().elementHandle();
      await staleRefresh.press("Space");
      await staleRead;
      await remove("b").press("Space");
      await dialog.waitFor();
      if (process.env.ARTIFACT_DIR && lang === "zh") {
        await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
        await page.evaluate(() => Promise.all(document.getAnimations().filter(a => a.effect?.getTiming().iterations !== Infinity).map(a => a.finished.catch(() => {}))));
        await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `prompt-delete-confirm-${engine}.png`), fullPage: true });
      }
      await submit().press("Space");
      await page.locator("#status").filter({ hasText: "fixture remove failed" }).waitFor();
      assert(await dialog.isVisible(), "a failed delete leaves confirmation open for retry");
      assert(await submit().isEnabled());
      assert.deepEqual(sent, [{ id: "b" }], "only the stable ID is sent");
      const top = await page.locator("#view-library").evaluate(e => e.scrollTop);
      holdRemove = true;
      const deleting = page.waitForRequest(r => r.url().endsWith("/prompts/remove"));
      await submit().press("Space");
      await deleting;
      try {
        assert(await submit().isDisabled(), "a pending deletion cannot be submitted twice");
        await submit().dispatchEvent("click");
        await page.keyboard.press("Escape");
        assert(await dialog.isVisible(), "pending deletion cannot dismiss its progress dialog");
        const before = targetPosts.length;
        // A queued click from an older frame must also respect the write guard.
        await chip().dispatchEvent("click");
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        assert.equal(targetPosts.length, before, "deletion prevents target writes even for a queued click");
        assert.equal(sent.length, 2);
      } finally { releaseRemove?.(); }
      await page.locator("#modal").waitFor({ state: "hidden" });
      await row("b").waitFor({ state: "detached" });
      releaseRead();
      await page.waitForFunction(e => !e.disabled, staleRefresh);
      assert.equal(await row("b").count(), 0, "a refresh started before deletion cannot restore an older template list");
      assert((await page.locator("#status").textContent()).includes("externally edited deployment kept"), "protected deployment problems are reported");
      assert.equal(await page.locator(".lib-prompt-search").inputValue(), "needle");
      assert.equal(await page.locator('textarea[data-lib="set:default"]').inputValue(), "尚未保存的共享指令");
      assert(await row("c").evaluate(e => e === document.activeElement), "focus returns to the next visible template");
      assert.equal(await page.locator("#view-library").evaluate(e => e.scrollTop), await page.locator("#view-library").evaluate((e, top) => Math.min(top, e.scrollHeight - e.clientHeight), top), "deletion keeps the page position within the shorter list's bounds");
      await page.locator(".lib-prompt-search").fill("needle ");
      await page.locator(".lib-prompt-search").fill("needle");
      assert.equal(await row("b").count(), 0, "redrawing the cache cannot resurrect a deleted template");
      for (const id of ["c", "a"]) {
        await remove(id).press("Space");
        await submit().press("Space");
        await page.locator("#modal").waitFor({ state: "hidden" });
        await row(id).waitFor({ state: "detached" });
        assert(await (id === "c" ? row("a") : page.locator(".lib-prompt-search")).evaluate(e => e === document.activeElement), "focus falls back to the previous row, then search");
      }
      assert.deepEqual(sent, [{ id: "b" }, { id: "b" }, { id: "c" }, { id: "a" }]);
      assert.deepEqual(errors, []);
      await ctx.close();
    });
  });
}
