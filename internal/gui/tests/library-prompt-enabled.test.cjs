const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(`${engine}: prompt templates expose a global switch without losing saved choices`, async t => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch(process.env.CHROMIUM_EXECUTABLE ? { executablePath: process.env.CHROMIUM_EXECUTABLE } : { channel: "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) await t.test(lang, async () => {
      const ctx = await browser.newContext({ viewport: { width: 560, height: 860 }, reducedMotion: "reduce" });
      t.after(() => ctx.close());
      const page = await ctx.newPage();
      page.setDefaultTimeout(3000);
      const errors = [], sent = [], traffic = [];
      page.on("pageerror", e => errors.push(e.message));
      let enabled = false, failEnable = true, checks = [];
      let holdCheck = false, checkStarted, releaseCheck;
      let holdAll = false, allStarted, releaseAll;
      let secondHarness = false;
      const items = [{ id: "p1", name: "review-code", text: "检查边界条件。", agents: ["claude"], states: [{ agent: "claude", status: "available" }] }];
      const prompts = () => ({ enabled, items, checks, targets: [{ id: "claude", name: "Claude Code", icon: "claudecode-color", detected: true, verification: "pending" }, ...(secondHarness ? [{ id: "codex", name: "Codex", icon: "codex-color", detected: true }] : [])] });
      await ctx.addInitScript(() => localStorage.setItem("magpie.libTab", "instructions"));
      await page.route("**/*", async route => {
        const url = new URL(route.request().url()), p = url.pathname;
        if (!["http:", "https:"].includes(url.protocol)) return route.continue();
        if (p === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs={lang:${JSON.stringify(lang)},theme:"light",web:true}` });
        if (p === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window={}" });
        if (p === "/api/state") return route.fulfill({ json: { agents: [], profiles: [], settings: { lang, theme: "light" } } });
        if (p === "/api/plugins") return route.fulfill({ json: { plugins: [] } });
        if (p === "/api/providers") return route.fulfill({ json: { providers: [], gateway: { running: true } } });
        if (p === "/api/library") return route.fulfill({ json: { agents: [{ id: "claude", name: "Claude Code", icon: "claudecode-color" }], servers: [], skills: [], foundSkills: [], foundServers: [], projects: [], prompts: prompts(), instructions: { shared: "", sets: [{ id: "default", name: "", text: "", active: true }], agents: [] } } });
        if (p === "/api/library/prompts") { traffic.push({ p, method: "GET" }); return route.fulfill({ json: prompts() }); }
        if (p.startsWith("/api/library/prompts/")) {
          const body = route.request().postDataJSON(); sent.push({ p, body });
          if (p.endsWith("/check")) {
            traffic.push({ p, method: "POST", body });
            if (holdAll && !body.target) {
              holdAll = false;
              allStarted?.();
              await new Promise(resolve => { releaseAll = resolve; });
              return route.fulfill({ json: [{ agent: "claude", status: "available" }, { agent: "codex", status: "unverified", reason: "fixture older full check" }] });
            }
            if (holdCheck) {
              holdCheck = false;
              checkStarted?.();
              await new Promise(resolve => { releaseCheck = resolve; });
              return route.fulfill({ status: 500, body: "fixture stale check failure" });
            }
            const states = [{ agent: body.target || "claude", status: "available", checkedAt: 1792886400000 }];
            if (body.force) { checks = states; items[0].states = [{ ...states[0], status: "enabled" }]; }
            return route.fulfill({ json: states });
          }
          if (p.endsWith("/enabled")) {
            if (failEnable) { failEnable = false; return route.fulfill({ status: 500, body: "fixture enable failed" }); }
            enabled = body.enabled;
            items[0].states = [{ agent: "claude", status: enabled ? "enabled" : "disabled" }];
          }
          if (p.endsWith("/agents")) {
            items[0].agents = body.agents;
            items[0].states = [{ agent: "claude", status: body.agents.includes("claude") ? "enabled" : "available" }];
          }
          return route.fulfill({ json: { part: "prompts", prompts: prompts(), result: { changed: [], problems: [] } } });
        }
        if (p.startsWith("/api/")) return route.fulfill({ json: p === "/api/usage/quotas" ? [] : {} });
        const file = path.join(assets, p === "/" ? "index.html" : p);
        return route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml" }[path.extname(file)] });
      });
      await page.goto("http://magpie.test/");
      await page.locator('button[data-view="library"]').click();
      await page.locator(".lib-prompt-row").waitFor();
      const toggle = page.locator('.lib-prompts [role="switch"]');
      assert.equal(await toggle.count(), 1, "prompt templates need their own global switch");
      assert.equal(await toggle.getAttribute("aria-checked"), "false");
      assert((await toggle.getAttribute("aria-label")).trim(), "the global switch has an accessible localized label");
      assert.equal(sent.length, 0, "reading a disabled prompt library never starts availability checks");
      await toggle.press("Space");
      await page.locator("#status").filter({ hasText: "fixture enable failed" }).waitFor();
      assert.equal(await toggle.getAttribute("aria-checked"), "false", "a failed enable preserves confirmed disabled state");
      await toggle.press("Space");
      await page.waitForFunction(() => document.querySelector('.lib-prompts [role="switch"]')?.getAttribute("aria-checked") === "true");
      await toggle.press("Space");
      await page.waitForFunction(() => document.querySelector('.lib-prompts [role="switch"]')?.getAttribute("aria-checked") === "false");
      assert.deepEqual(sent, [
        { p: "/api/library/prompts/enabled", body: { enabled: true } },
        { p: "/api/library/prompts/enabled", body: { enabled: true } },
        { p: "/api/library/prompts/enabled", body: { enabled: false } },
      ], "only explicit global intents reach the enable endpoint; no separate check or target mutation is needed");
      assert.equal(await page.locator('.lib-prompt-row .lib-ag[data-agent="claude"]').getAttribute("aria-pressed"), "true", "the global off state visibly retains the saved Harness selection");
      assert(await page.locator('.lib-prompts .lib-cardhead button:not([role="switch"])').first().isDisabled(), "global off prevents refresh from starting native Harness checks");
      await page.locator(".lib-prompt-row").press("Enter");
      assert.equal(await page.locator('[data-prompt="text"]').inputValue(), "检查边界条件。", "switching off keeps the library body editable");
      assert(await page.locator('.lib-prompt-target[data-agent="claude"] input').isChecked(), "switching off keeps the saved Harness selection");
      const target = page.locator('.lib-prompt-target[data-agent="claude"] input');
      const beforeTarget = sent.length;
      await target.uncheck();
      assert.equal(sent.length, beforeTarget, "unchecking a Harness never starts a native check");
      await target.check();
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
      assert.deepEqual(sent.slice(beforeTarget), [{
        p: "/api/library/prompts/check",
        body: { id: "p1", name: "review-code", text: "检查边界条件。", target: "claude", force: false, passive: false },
      }], "newly checking one Harness checks only that Harness and this template");
      assert(await target.isChecked(), "a successful target check keeps the requested selection");
      await page.locator(".lib-prompt-editor .bar button").nth(1).click();
      await page.locator("#modal").waitFor({ state: "hidden" });
      await toggle.press("Space");
      await page.waitForFunction(() => document.querySelector('.lib-prompts [role="switch"]')?.getAttribute("aria-checked") === "true");
      const chip = page.locator('.lib-prompt-row .lib-ag[data-agent="claude"]');
      const beforeRemoval = sent.length;
      await chip.press("Space");
      await page.waitForFunction(() => document.querySelector('.lib-prompt-row .lib-ag[data-agent="claude"]')?.getAttribute("aria-pressed") === "false");
      assert.deepEqual(sent.slice(beforeRemoval), [{ p: "/api/library/prompts/agents", body: { id: "p1", agents: [] } }], "removing a list target writes only the selection and never starts checks");
      const beforeAddition = sent.length;
      await chip.press("Space");
      await page.waitForFunction(() => document.querySelector('.lib-prompt-row .lib-ag[data-agent="claude"]')?.getAttribute("aria-pressed") === "true");
      assert.deepEqual(sent.slice(beforeAddition), [
        { p: "/api/library/prompts/check", body: { id: "p1", name: "review-code", text: "检查边界条件。", target: "claude", force: false, passive: false } },
        { p: "/api/library/prompts/agents", body: { id: "p1", agents: ["claude"] } },
      ], "newly selecting a list target checks only that Harness before deployment");
      const beforeRefresh = traffic.length;
      const oldRefresh = await page.locator('.lib-prompts .lib-cardhead button:not([role="switch"])').first().elementHandle();
      await oldRefresh.press("Space");
      await page.waitForFunction(e => !e.isConnected, oldRefresh);
      assert.deepEqual(traffic.slice(beforeRefresh), [
        { p: "/api/library/prompts/check", method: "POST", body: { force: true } },
        { p: "/api/library/prompts", method: "GET" },
      ], "refresh forces a nameless all-Harness check before reading the updated list");
      const checkedTimes = await page.evaluate(({ timestamp, lang }) => [...new Set([new Date(timestamp).toLocaleString(), new Date(timestamp).toLocaleString(lang)])], { timestamp: 1792886400000, lang });
      const cardText = await page.locator(".lib-prompts").textContent();
      assert(checkedTimes.some(time => cardText.includes(time)), "the library shows the actual recent native check time");
      await page.locator(".lib-prompt-row").press("Enter");
      const pendingNote = { en: "Native behavior awaits verification on this installation", zh: "此安装版本的原生行为待验证", "zh-TW": "此安裝版本的原生行為待驗證", ja: "このインストールの動作は未検証です", de: "Natives Verhalten dieser Installation noch nicht geprüft" }[lang];
      assert(!(await page.locator('.lib-prompt-target[data-agent="claude"]').textContent()).includes(pendingNote), "a successful native check replaces the permanent pending-verification note");
      await page.locator(".lib-prompt-editor .bar button").nth(1).click();
      await page.locator("#modal").waitFor({ state: "hidden" });
      const beforeNew = sent.length;
      await page.locator(".lib-prompt-add").click();
      assert.equal(await page.locator('.lib-prompt-target input:checked').count(), 0, "new templates start with no Harness selected so every addition is explicit");
      assert.equal(sent.length, beforeNew, "opening a new template never launches availability checks");
      await page.locator('[data-prompt="name"]').fill("fresh-draft");
      await page.locator('[data-prompt="text"]').fill("original draft");
      holdCheck = true;
      const started = new Promise(resolve => { checkStarted = resolve; });
      await target.check();
      await started;
      try {
        assert(await page.locator(".lib-prompt-save").isDisabled(), "an in-flight native check prevents saving an unconfirmed selection");
        const beforeCancel = sent.length;
        await target.uncheck();
        assert(!(await target.isChecked()), "a pending native check never prevents cancelling its selection");
        assert.equal(sent.length, beforeCancel, "cancelling a pending selection never launches another native check");
        await target.check();
        await page.locator('[data-prompt="text"]').fill("newer draft");
      } finally { releaseCheck?.(); }
      await page.waitForFunction(() => !document.querySelector(".lib-prompt-save").disabled);
      assert.equal(await page.locator(".lib-prompt-error").textContent(), "", "a failed check for an older draft cannot attach an error to the newer draft");
      assert.equal(await page.locator('[data-prompt="text"]').inputValue(), "newer draft");
      assert(await target.isChecked(), "a stale failed check cannot remove the newer selection intent");
      secondHarness = true;
      items[0].states.push({ agent: "codex", status: "available" });
      await page.reload();
      await page.locator('button[data-view="library"]').click();
      await chip.waitFor();
      holdCheck = true;
      const addingCodex = new Promise(resolve => { checkStarted = resolve; });
      const codexChip = page.locator('.lib-prompt-row .lib-ag[data-agent="codex"]');
      const beforeConcurrent = sent.length;
      await codexChip.press("Space");
      await addingCodex;
      try {
        const beforePendingRefresh = sent.filter(s => s.p.endsWith("/check") && s.body.force).length;
        await page.locator('.lib-prompts .lib-cardhead button:not([role="switch"])').first().dispatchEvent("click");
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        assert.equal(sent.filter(s => s.p.endsWith("/check") && s.body.force).length, beforePendingRefresh, "a queued Refresh click cannot start all-Harness checks during a target write");
        await chip.press("Space");
        await codexChip.press("Space");
      } finally { releaseCheck?.(); }
      await page.waitForFunction(() => !document.querySelector('.lib-prompt-row .lib-agents[aria-busy="true"]'));
      assert.deepEqual(sent.slice(beforeConcurrent).filter(s => s.p.endsWith("/agents")).map(s => s.body), [{ id: "p1", agents: [] }], "a failed new-target check must still persist cancellation of existing and pending selections");
      await page.locator(".lib-prompt-row").press("Enter");
      holdAll = true;
      const olderFullCheck = new Promise(resolve => { allStarted = resolve; });
      await page.locator(".lib-prompt-check").click();
      await olderFullCheck;
      const codexPick = page.locator('.lib-prompt-target[data-agent="codex"] input');
      try {
        await codexPick.check();
        await page.waitForFunction(() => document.querySelector('.lib-prompt-target[data-agent="codex"] input')?.getAttribute("aria-busy") === "false");
        assert(await codexPick.isChecked(), "the newer successful target check confirms the selection");
      } finally { releaseAll?.(); }
      await page.waitForFunction(() => !document.querySelector(".lib-prompt-check").disabled);
      assert(await codexPick.isChecked(), "an older full check must not revoke a newer successful single-Harness selection");
      assert(await page.locator(".lib-prompt-save").isEnabled(), "all completed checks restore Save after overlapping full and single-Harness requests");
      const manualRequest = sent.filter(s => s.p.endsWith("/check") && !s.body.target).at(-1);
      assert.equal(manualRequest.body.force, true, "the editor's explicit Check action must force fresh native verification");
      assert.deepEqual(errors, []);
    });
  });
}
