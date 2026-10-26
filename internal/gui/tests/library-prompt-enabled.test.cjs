const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
async function requestStarted(promise, label) {
  let timer;
  try { return await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(`Expected fixture check request did not start: ${label}`)), 5000); })]); }
  finally { clearTimeout(timer); }
}
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
      let secondHarness = false, overviewHarnesses = false;
      const items = [{ id: "p1", name: "review-code", text: "检查边界条件。", agents: ["claude"], states: [{ agent: "claude", status: "available" }] }];
      const prompts = () => ({ enabled, items, checks, targets: [{ id: "claude", name: "Claude Code", icon: "claudecode-color", detected: true, verification: "pending" }, ...(secondHarness || overviewHarnesses ? [{ id: "codex", name: "Codex", icon: "codex-color", detected: true }] : []), ...(overviewHarnesses ? [{ id: "pi", name: "Pi", icon: "pi", detected: true }, { id: "dsh", name: "DeepSeek Harness", icon: "deepseek-color", detected: true }] : [])] });
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
            const states = (overviewHarnesses && !body.target ? ["claude", "codex", "pi", "dsh"] : [body.target || "claude"]).map(agent => ({ agent, status: "available", reason: "fixture native check passed", source: `fixture/${agent}/config`, checkedAt: 1792886400000 }));
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
      const recheckLabel = { en: "Recheck availability", zh: "重新检查可用性", "zh-TW": "重新檢查可用性", ja: "利用可否を再確認", de: "Verfügbarkeit erneut prüfen" }[lang];
      assert.equal((await page.locator(".lib-prompt-refresh").innerText()).trim(), recheckLabel, "the title action explicitly names the availability recheck in the current language");
      assert(await page.getByRole("button", { name: recheckLabel, exact: true }).isVisible(), "the availability recheck has the same visible accessible name");
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
      assert(await page.locator('.lib-prompts .lib-prompt-refresh').first().isDisabled(), "global off prevents refresh from starting native Harness checks");
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
      overviewHarnesses = true;
      const beforeRefresh = traffic.length;
      const oldRefresh = await page.locator('.lib-prompts .lib-prompt-refresh').first().elementHandle();
      await oldRefresh.press("Space");
      await page.waitForFunction(e => !e.isConnected, oldRefresh);
      assert.deepEqual(traffic.slice(beforeRefresh), [
        { p: "/api/library/prompts/check", method: "POST", body: { force: true } },
        { p: "/api/library/prompts", method: "GET" },
      ], "refresh forces a nameless all-Harness check before reading the updated list");
      const checkedTimes = await page.evaluate(({ timestamp, lang }) => [...new Set([new Date(timestamp).toLocaleString(), new Date(timestamp).toLocaleString(lang)])], { timestamp: 1792886400000, lang });
      const overview = page.locator(".lib-prompt-harness-check");
      assert.equal(await overview.count(), 4, "the overview contains one health icon for each installed supported Harness");

      const passedLabel = { en: "Availability check passed", zh: "可用性检查通过", "zh-TW": "可用性檢查通過", ja: "利用可否の確認に成功", de: "Verfügbarkeitsprüfung bestanden" }[lang];
      for (const [index, agent] of ["claude", "codex", "pi", "dsh"].entries()) {
        const icon = overview.nth(index);
        await icon.focus();
        assert(await icon.evaluate(e => e === document.activeElement), `${agent}: overview health is keyboard focusable`);
        assert.equal(await icon.locator(".ic[data-icon]").count(), 1, `${agent}: the overview renders the Harness icon`);
        const tooltip = await icon.getAttribute("title") || "";
        assert(tooltip.includes(passedLabel), `${agent}: successful global checks have an explicit localized passed status`);
        assert(tooltip.includes("fixture native check passed"), `${agent}: tooltip preserves the check reason`);
        assert(tooltip.includes(`fixture/${agent}/config`), `${agent}: tooltip identifies the check source`);
        assert(checkedTimes.some(time => tooltip.includes(time)), `${agent}: tooltip includes the actual check time`);
        assert.equal(await icon.evaluate(e => e.classList.contains("on")), agent === "claude", `${agent}: successful checks do not light Harnesses with no selected deployment`);
        assert(tooltip.includes(`${agent === "claude" ? 1 : 0}/1`), `${agent}: overview reports selected deployed templates independently of check success`);
      }
      for (const width of [560, 420]) {
        await page.setViewportSize({ width, height: 860 });
        const layout = await overview.evaluateAll(icons => icons.map(icon => {
          const rect = icon.getBoundingClientRect(), card = icon.closest(".lib-prompts").getBoundingClientRect();
          return { left: rect.left, right: rect.right, width: rect.width, cardLeft: card.left, cardRight: card.right, viewport: innerWidth };
        }));
        assert.equal(layout.length, 4);
        for (const box of layout) assert(box.width > 0 && box.left >= Math.max(0, box.cardLeft) && box.right <= Math.min(box.viewport, box.cardRight), `all four overview icons fit the card at ${width}px`);
      }
      await page.setViewportSize({ width: 560, height: 860 });
      await page.locator(".lib-prompt-row").press("Enter");
      const pendingNote = { en: "Native behavior awaits verification on this installation", zh: "此安装版本的原生行为待验证", "zh-TW": "此安裝版本的原生行為待驗證", ja: "このインストールの動作は未検証です", de: "Natives Verhalten dieser Installation noch nicht geprüft" }[lang];
      assert(!(await page.locator('.lib-prompt-target[data-agent="claude"]').textContent()).includes(pendingNote), "a successful native check replaces the permanent pending-verification note");
      await page.locator(".lib-prompt-editor .bar button").nth(1).click();
      await page.locator("#modal").waitFor({ state: "hidden" });
      // Concurrency below is independent of the narrow-card layout checked above.
      await page.setViewportSize({ width: 1100, height: 900 });
      const beforeNew = sent.length;
      await page.locator(".lib-prompt-add").click();
      assert.equal(await page.locator('.lib-prompt-target input:checked').count(), 0, "new templates start with no Harness selected so every addition is explicit");
      assert.equal(sent.length, beforeNew, "opening a new template never launches availability checks");
      await page.locator('[data-prompt="name"]').fill("fresh-draft");
      await page.locator('[data-prompt="text"]').fill("original draft");
      holdCheck = true;
      const started = new Promise(resolve => { checkStarted = resolve; });
      await target.check();
      await requestStarted(started, "new draft target");
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
      await page.evaluate(() => {
        const events = [], types = ["keydown", "keyup", "click", "focusin", "focusout"];
        const describe = e => ({ tagName: e?.tagName, agent: e?.closest?.("[data-agent]")?.dataset.agent });
        const record = e => {
          const chip = document.querySelector('.lib-prompt-row .lib-ag[data-agent="codex"]');
          if (events.length === 32) events.shift();
          events.push({ type: e.type, key: e.key, target: describe(e.target), active: describe(document.activeElement), connected: chip?.isConnected, disabled: chip?.disabled, blocked: chip?.getAttribute("aria-disabled"), busy: chip?.getAttribute("aria-busy"), pressed: chip?.getAttribute("aria-pressed") });
        };
        for (const type of types) document.addEventListener(type, record, true);
        window.fixtureCodexEvents = { events, stop: () => types.forEach(type => document.removeEventListener(type, record, true)) };
      });
      try {
        await codexChip.press("Space");
        await requestStarted(addingCodex, "list adds Codex");
      }
      catch (error) {
        error.message += JSON.stringify({ events: await page.evaluate(() => window.fixtureCodexEvents.events), sent: sent.slice(beforeConcurrent), errors });
        throw error;
      } finally {
        await page.evaluate(() => { window.fixtureCodexEvents.stop(); delete window.fixtureCodexEvents; });
      }
      try {
        const beforePendingRefresh = sent.filter(s => s.p.endsWith("/check") && s.body.force).length;
        await page.locator('.lib-prompts .lib-prompt-refresh').first().dispatchEvent("click");
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
      await requestStarted(olderFullCheck, "editor full check");
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
  test(`${engine}: prompt rows distinguish deployment facts from expired verification`, async t => {
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    t.after(() => browser.close());
    for (const lang of ["en", "zh", "zh-TW", "ja", "de"]) await t.test(lang, async () => {
      const ctx = await browser.newContext({ viewport: { width: 1100, height: 900 }, reducedMotion: "reduce" });
      t.after(() => ctx.close());
      const page = await ctx.newPage();
      const errors = [], requests = [];
      page.on("pageerror", e => errors.push(e.message));
      const checkedAt = 1792886400000;
      const targets = [{ id: "claude", name: "Claude Code", icon: "claudecode-color", detected: true }, { id: "codex", name: "Codex", icon: "codex-color", detected: true }, { id: "pi", name: "Pi", icon: "pi", detected: true }];
      const item = { id: "deployed", name: "review-code", text: "正文", agents: ["claude", "codex", "pi"], states: [{ agent: "claude", status: "unverified", deployed: true, reason: "Check native command availability again", source: "fixture/claude/managed.md", checkedAt }, { agent: "codex", status: "enabled" }, { agent: "pi", status: "unverified", deployed: true, reason: "Check Pi command availability again", source: "fixture/pi/prompts", checkedAt }] };
      const prompts = () => ({ enabled: true, items: [item], targets, checks: [] });
      await ctx.addInitScript(() => localStorage.setItem("magpie.libTab", "instructions"));
      await page.route("**/*", async route => {
        const url = new URL(route.request().url()), p = url.pathname;
        if (!["http:", "https:"].includes(url.protocol)) return route.continue();
        if (p === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs={lang:${JSON.stringify(lang)},theme:"light",web:true}` });
        if (p === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window={}" });
        if (p === "/api/state") return route.fulfill({ json: { agents: [], profiles: [], settings: { lang, theme: "light" } } });
        if (p === "/api/plugins") return route.fulfill({ json: { plugins: [] } });
        if (p === "/api/providers") return route.fulfill({ json: { providers: [], gateway: { running: true } } });
        if (p === "/api/library") return route.fulfill({ json: { agents: targets, servers: [], skills: [], foundSkills: [], foundServers: [], projects: [], prompts: prompts(), instructions: { shared: "", sets: [{ id: "default", name: "", text: "", active: true }], agents: [] } } });
        if (p === "/api/library/prompts") return route.fulfill({ json: prompts() });
        if (p.startsWith("/api/")) { requests.push(p); return route.fulfill({ json: p === "/api/usage/quotas" ? [] : {} }); }
        const file = path.join(assets, p === "/" ? "index.html" : p);
        return route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml" }[path.extname(file)] });
      });
      const open = async () => { await page.goto("http://magpie.test/"); await page.locator('button[data-view="library"]').click(); await page.locator(".lib-prompt-row").waitFor(); };
      await open();
      const chip = id => page.locator(`.lib-prompt-row .lib-ag[data-agent="${id}"]`);
      assert.equal(await chip("claude").getAttribute("aria-pressed"), "true", "verified managed deployment stays lit when only native-check cache expires");
      assert.equal(await chip("codex").getAttribute("aria-pressed"), "true", "legacy status=enabled remains visibly deployed");
      assert.equal(await chip("pi").getAttribute("aria-pressed"), "true", "Pi's recognized cache-expiry reason preserves its actual deployment");
      assert.equal(await chip("pi").evaluate(e => e.classList.contains("warn")), false, "Pi cache expiry alone does not paint an orange target warning");
      assert.equal(await chip("claude").evaluate(e => e.classList.contains("warn")), false, "cache expiry alone does not paint an orange target warning");
      assert.equal(await page.locator(".lib-prompt-row .lib-tags .warn").count(), 0, "cache expiry alone does not produce a row alarm");
      await chip("claude").hover();
      const tooltip = await chip("claude").getAttribute("title") || "";
      const expiredReason = { en: "Check native command availability again", zh: "需要重新检查原生命令可用性", "zh-TW": "需要重新檢查原生命令可用性", ja: "ネイティブコマンドの利用可否を再確認してください", de: "Native Befehlsverfügbarkeit erneut prüfen" }[lang];
      assert(tooltip.includes(expiredReason) && tooltip.includes("fixture/claude/managed.md"), "hover details retain expired verification reason and source");
      const localTime = await page.evaluate(timestamp => new Date(timestamp).toLocaleString(), checkedAt);
      assert(tooltip.includes(localTime), "hover details retain the previous check time");
      await chip("claude").focus();
      assert(await chip("claude").evaluate(e => e === document.activeElement), "the same deployment status remains keyboard reachable");
      assert.equal(await chip("claude").getAttribute("title"), tooltip, "keyboard focus retains the same status details");
      for (const status of ["edited", "disabled", "missing", "unverified"]) {
        item.states[0] = { agent: "claude", status, deployed: false, reason: `fixture ${status} deployment`, source: "fixture/claude/managed.md", checkedAt };
        await open();
        assert.equal(await chip("claude").getAttribute("aria-pressed"), "false", `${status}: an absent or changed managed deployment is not lit`);
        assert(await chip("claude").evaluate(e => e.classList.contains("warn")), `${status}: a real deployment failure paints a target warning`);
        assert.equal(await page.locator(".lib-prompt-row .lib-tags .warn").count(), 1, `${status}: a real selected-target deployment failure raises one row alarm`);
        assert((await chip("claude").getAttribute("title")).includes(`fixture ${status} deployment`));
      }
      item.states[0] = { agent: "claude", status: "unverified", deployed: true, reason: "fixture unknown native verification failure" };
      await open();
      assert.equal(await chip("claude").getAttribute("aria-pressed"), "true", "an unknown verification failure preserves the separate managed-deployment fact");
      assert(await chip("claude").evaluate(e => e.classList.contains("warn")), "an unknown verification failure is never hidden as harmless cache expiry");
      assert.equal(await page.locator(".lib-prompt-row .lib-tags .warn").count(), 1, "unknown native verification failures retain the row alarm even when deployed");
      assert.equal(requests.filter(p => /\/prompts\/(check|save|agents)$/.test(p)).length, 0, "reading, hovering and focusing deployment facts never trigger a check or write");
      assert.deepEqual(errors, []);
    });
  });
}
