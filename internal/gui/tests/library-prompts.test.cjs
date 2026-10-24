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
      const targets = [{ id: "claude", name: "Claude Code", icon: "claudecode-color", detected: true, verification: "pending", reload: "Restart the harness to load updated commands" }, { id: "codex", name: "Codex", invocation: "/skills · $<name>", icon: "codex-color", detected: true, shared: true, verification: "pending", reload: "Restart the harness to load updated commands" }, { id: "pi", name: "Pi", icon: "pi", detected: true, verification: "pending", reload: "Run /reload to load updated commands" }];
      targets.push({ id: "dsh", name: "DeepSeek Harness", icon: "deepseek-color", detected: true, verification: "pending", invocation: "/<name>" });
      let items = [], failSave = true, releaseCheck, checkStarted, holdAgents = false, releaseAgents, failAgents = false;
      const states = [{ agent: "claude", status: "available" }, { agent: "codex", status: "conflict", reason: "This name is already in use", source: "/fixture/.agents/skills/审查/SKILL.md" }, { agent: "pi", status: "unverified", reason: "Check Pi command availability again", source: "/fixture/.pi/agent" }];
      states.push({ agent: "dsh", status: "unverified", reason: "DeepSeek Harness skill source completeness could not be verified", source: "fixture/native-skill-provider" });
      const theme = ["ja", "de"].includes(lang) ? "dark" : "light";
      const prompts = () => ({ items, targets });
      await ctx.addInitScript(() => localStorage.setItem("magpie.libTab", "instructions"));
      await page.route("**/*", async (route) => {
        const url = new URL(route.request().url()), p = url.pathname;
        if (!["http:", "https:"].includes(url.protocol)) return route.continue();
        if (p === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs={lang:${JSON.stringify(lang)},theme:${JSON.stringify(theme)},web:true}` });
        if (p === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window={}" });
        if (p === "/api/state") return route.fulfill({ json: { agents: [], profiles: [], settings: { lang, theme } } });
        if (p === "/api/plugins") return route.fulfill({ json: { plugins: [] } });
        if (p === "/api/providers") return route.fulfill({ json: { providers: [], gateway: { running: true } } });
        if (p === "/api/library") return route.fulfill({ json: { agents: [{ id: "claude", name: "Claude Code", icon: "claudecode-color", instructions: "/fixture/.claude/CLAUDE.md" }], servers: [], skills: [], foundSkills: [], foundServers: [], projects: [], prompts: prompts(), instructions: { shared: "", sets: [{ id: "default", name: "", text: "", active: true }], agents: [] } } });
        if (p === "/api/library/prompts") return route.fulfill({ json: prompts() });
        if (p.startsWith("/api/library/prompts/")) {
          const body = route.request().postDataJSON(); sent.push({ p, body });
          if (p.endsWith("/check")) {
            checkStarted?.();
            if (body.name === "waiting-template") await new Promise(resolve => { releaseCheck = resolve; });
            return route.fulfill({ json: states });
          }
          if (p.endsWith("/save") && failSave) { failSave = false; return route.fulfill({ status: 500, body: "fixture save failed" }); }
          if (p.endsWith("/save")) items = [{ ...body, id: "p1", states: states.map(s => s.agent === "claude" ? { ...s, status: "enabled" } : s) }];
          if (p.endsWith("/agents")) {
            if (holdAgents) { holdAgents = false; await new Promise(resolve => { releaseAgents = resolve; }); }
            if (failAgents) { failAgents = false; return route.fulfill({ status: 500, body: "fixture target update failed" }); }
            items[0] = { ...items[0], agents: body.agents, states: states.map(s => ["available", "enabled"].includes(s.status) ? { ...s, status: body.agents.includes(s.agent) ? "enabled" : "available" } : { ...s }) };
          }
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
      if (lang === "en") assert((await page.locator(".lib-prompts").textContent()).includes("Use the native invocation shown for each harness"), "intro must not promise /name for Codex CLI");
      const finder = page.locator(".lib-prompts .lib-find");
      const refreshList = async () => {
        const old = await page.locator(".lib-prompts .lib-cardhead button").first().elementHandle();
        await old.press("Space");
        await page.waitForFunction(e => !e.isConnected, old);
      };
      assert.equal(await finder.count(), 1, "templates use the skill-style search bar");
      assert.equal(await finder.locator("svg").count(), 1, "the search bar has a search icon");
      assert.equal(await finder.locator("button").count(), 0, "search is immediate, without an action button");
      assert(await finder.locator(".lib-prompt-search").isVisible());
      await page.locator(".lib-prompt-add").press("Space");
      assert.equal(await page.locator('[data-prompt="name"]').getAttribute("placeholder"), { en: "e.g. review-code", zh: "例如 review-code", "zh-TW": "例如 review-code", ja: "例：review-code", de: "z. B. review-code" }[lang]);
      const warning = await page.locator(".lib-prompt-runtime-note").textContent();
      assert(warning.includes({ en: "loads extensions", zh: "加载扩展", "zh-TW": "載入擴充功能", ja: "拡張機能", de: "Erweiterungen" }[lang]), "runtime side effects are explained in every language");
      await page.locator('[data-prompt="name"]').fill("waiting-template");
      await page.locator('[data-prompt="text"]').fill("Draft");
      // Request events precede route handlers. Wait until the fixture has
      // actually recorded the request before asserting its body.
      const preflight = new Promise(resolve => { checkStarted = resolve; });
      await page.locator(".lib-prompt-save").click();
      await preflight;
      assert.equal(sent.at(-1).body.passive, true, "save's preflight must not load Pi extensions");
      try { assert(await page.locator("#modal .bar button").first().isDisabled(), "Cancel stays disabled during the save's preflight"); }
      finally { releaseCheck?.(); }
      await page.locator(".lib-prompt-error").filter({ hasText: "fixture save failed" }).waitFor();
      failSave = true; sent.length = 0;
      await page.locator('[data-prompt="name"]').fill("review-code");
      await page.locator('[data-prompt="text"]').fill("检查边界条件。");
      await page.locator(".lib-prompt-check").click();
      await page.locator('.lib-prompt-target[data-agent="codex"] .lib-prompt-reason').waitFor();
      assert.equal(await page.locator('.lib-prompt-target[data-agent="codex"]').count(), 1, "CLI and Desktop have one Codex configuration");
      assert.equal(await page.locator('.lib-prompt-target[data-agent="codex-desktop"]').count(), 0, "Desktop is not a second target");
      assert.equal((await page.locator("#status").textContent()).trim(), "", "a fresh availability check clears the previous save error");
      assert((await page.locator('.lib-prompt-target[data-agent="codex"]').textContent()).includes("/fixture/.agents/skills/审查/SKILL.md"));
      assert(await page.locator('.lib-prompt-target[data-agent="codex"] input').isDisabled());
      assert((await page.locator('.lib-prompt-target[data-agent="codex"]').textContent()).includes("/skills · $<name>"), "unified Codex shows the CLI entry points");
      assert(!(await page.locator('.lib-prompt-target[data-agent="codex"]').textContent()).includes("Desktop"), "Codex describes only CLI usage, without a separate Desktop note");
      assert((await page.locator('.lib-prompt-target[data-agent="dsh"]').textContent()).includes("fixture/native-skill-provider"), "DSH explains uncovered native providers");
      assert(await page.locator('.lib-prompt-target[data-agent="dsh"] input').isDisabled(), "incomplete DSH sources cannot be enabled");
      assert(!(await page.locator('.lib-prompt-target[data-agent="pi"]').textContent()).includes("/reload"), "reload is a deployment hint, not a check-error remedy");
      assert.equal(sent.at(-1).body.passive, false, "manual check explicitly permits runtime enumeration");
      const checksBefore = sent.filter(s => s.p.endsWith("/check")).length;
      await page.locator(".lib-prompt-check").click();
      await page.waitForFunction(() => !document.querySelector(".lib-prompt-check").disabled);
      assert.equal(sent.filter(s => s.p.endsWith("/check")).length, checksBefore + 1, "manual check always refreshes native commands");
      const originalDSHState = states.at(-1);
      for (const diagnostic of [
        { reason: "DeepSeek Harness plugin dependency could not be loaded", source: "dsh-at-file: @deepseek-ai/dsh-settings", words: { en: "plugin dependency", zh: "插件缺少依赖", "zh-TW": "插件缺少相依", ja: "依存パッケージ", de: "Abhängigkeit" } },
        { reason: "DeepSeek Harness environment variable is restricted to the launching process", source: "C:/fixture/.env: DSH_HOME", words: { en: "launching process", zh: "启动进程", "zh-TW": "啟動程序", ja: "起動プロセス", de: "startenden Prozess" } },
      ]) {
        states[states.length - 1] = { agent: "dsh", status: "unverified", ...diagnostic };
        await page.locator(".lib-prompt-check").click();
        const target = page.locator('.lib-prompt-target[data-agent="dsh"]');
        await target.filter({ hasText: diagnostic.source }).waitFor();
        assert((await target.textContent()).includes(diagnostic.words[lang]), "native failure reason is translated and retains its source");
        assert(await target.locator("input").isDisabled(), "actionable errors must still block deployment");
      }
      states[states.length - 1] = originalDSHState;
      if (process.env.ARTIFACT_DIR && lang === "zh") {
        await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
        await page.evaluate(() => Promise.all(document.getAnimations().filter(a => a.effect?.getTiming().iterations !== Infinity).map(a => a.finished.catch(() => {}))));
        await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `prompt-templates-${engine}.png`), fullPage: true });
      }
      await page.setViewportSize({ width: 1000, height: 900 });
      assert((await page.locator('.lib-prompt-target[data-agent="pi"]').boundingBox()).width > 350, "the runtime warning must not push target cards into the label column");
      await page.setViewportSize({ width: 560, height: 860 });
      await page.locator(".lib-prompt-save").click();
      await page.locator(".lib-prompt-error").filter({ hasText: "fixture save failed" }).waitFor();
      assert.equal(await page.locator('[data-prompt="text"]').inputValue(), "检查边界条件。");
      await page.locator(".lib-prompt-save").click();
      await page.locator(".lib-prompt-row").waitFor();
      await page.locator(".lib-prompt-editor").waitFor({ state: "detached" });
      assert.equal(sent.filter(s => s.p.endsWith("/save")).length, 2);
      assert.deepEqual(sent.find(s => s.p.endsWith("/save")).body.agents, ["claude"]);
      const chip = id => page.locator(`.lib-prompt-row .lib-ag[data-agent="${id}"]`);
      assert.equal(await page.locator(".lib-prompt-row .lib-ag[data-agent]").count(), targets.length, "templates show native target icons");
      assert.equal(await chip("claude").getAttribute("aria-pressed"), "true");
      assert(!(await chip("codex").getAttribute("title")).includes("Desktop"), "the single Codex icon explains CLI usage only");
      assert.equal((await page.locator(".lib-prompt-row .lib-tags").textContent()).trim(), "", "native acceptance pending is not a permanent list badge");
      assert((await chip("claude").getAttribute("title")).includes({ en: "Native behavior", zh: "原生行为", "zh-TW": "原生行為", ja: "動作は未検証", de: "Natives Verhalten" }[lang]), "native acceptance remains explained on the icon");
      assert((await chip("pi").getAttribute("class")).includes("warn"), "unavailable targets retain their warning dot");
      assert.equal(await chip("pi").getAttribute("aria-pressed"), "false");
      assert.equal(await page.locator('.lib-prompt-row .lib-ag[data-all]').count(), 0, "All needs two eligible targets");
      const beforeBlocked = sent.length;
      await chip("pi").press("Space");
      assert((await page.locator("#status").textContent()).includes({ en: "template editor", zh: "模板编辑框", "zh-TW": "範本編輯器", ja: "テンプレートエディター", de: "Vorlageneditor" }[lang]), "expired checks explain where to recheck");
      await chip("codex").press("Space");
      assert((await page.locator("#status").textContent()).includes("/fixture/.agents/skills/审查/SKILL.md"));
      assert.equal(await page.locator(".lib-prompt-editor").count(), 0, "a chip's keyboard action must not open the editor");
      assert.equal(sent.length, beforeBlocked, "a blocked target never writes or starts runtime checks");
      holdAgents = true;
      const firstToggle = page.waitForRequest(r => r.url().endsWith("/prompts/agents"));
      await chip("claude").press("Space");
      await firstToggle;
      try {
        assert.equal(await chip("claude").getAttribute("aria-pressed"), "true", "pending writes do not claim successful deployment");
        assert.equal(await chip("claude").getAttribute("aria-busy"), "true");
        await chip("claude").press("Space");
      } finally { releaseAgents?.(); }
      await page.waitForFunction(() => !document.querySelector('.lib-prompt-row .lib-agents[aria-busy="true"]'));
      assert.equal(await chip("claude").getAttribute("aria-pressed"), "true", "rapid clicks settle on the final intent");
      const targetPosts = sent.filter(s => s.p.endsWith("/agents"));
      assert.deepEqual(targetPosts.map(s => s.body), [{ id: "p1", agents: [] }, { id: "p1", agents: ["claude"] }]);
      failAgents = true;
      await chip("claude").press("Space");
      await page.locator("#status").filter({ hasText: "fixture target update failed" }).waitFor();
      await page.waitForFunction(() => !document.querySelector('.lib-prompt-row .lib-agents[aria-busy="true"]'));
      assert.equal(await chip("claude").getAttribute("aria-pressed"), "true", "a failed write restores actual state");
      assert.equal(await page.locator(".lib-prompt-editor").count(), 0);
      // Two eligible targets get the same All control as skills. A retained,
      // expired Pi selection is excluded from bulk changes but stays removable.
      Object.assign(states.find(s => s.agent === "codex"), { status: "available", reason: "", source: "" });
      items[0].agents.push("pi");
      items[0].states = items[0].states.map(s => s.agent === "codex" ? { ...states.find(x => x.agent === s.agent) } : s);
      await refreshList();
      const all = page.locator('.lib-prompt-row .lib-ag[data-all]');
      await all.waitFor();
      assert.equal(await all.getAttribute("aria-pressed"), "false");
      assert.equal(await page.locator(".lib-prompt-row .lib-tags .warn").count(), 1, "selected abnormal targets retain Needs attention");
      for (const width of [560, 1100, 420]) {
        await page.setViewportSize({ width, height: 900 });
        const row = page.locator(".lib-prompt-row");
        await row.locator(".lib-prompt-delete").focus();
        await page.waitForFunction(() => getComputedStyle(document.activeElement.parentElement).opacity === "1");
        const layout = await row.evaluate(e => {
          const box = el => {
            const r = el.getBoundingClientRect();
            return { left: r.left, right: r.right, top: r.top, bottom: r.bottom, centerY: r.top + r.height / 2 };
          };
          return {
            row: box(e), warning: box(e.querySelector(".lib-tags .warn")),
            targets: [...e.querySelectorAll(".lib-ag[data-agent]")].map(box),
            name: box(e.querySelector(".who")), actions: box(e.querySelector(".lib-rowacts")),
          };
        });
        for (const target of layout.targets) {
          assert(Math.abs(layout.warning.centerY - target.centerY) <= 1, `Needs attention aligns with harness icons at ${width}px (${JSON.stringify(layout)})`);
        }
        const overlaps = (a, b) => a.left < b.right - 1 && a.right > b.left + 1 && a.top < b.bottom - 1 && a.bottom > b.top + 1;
        for (const other of [layout.name, layout.actions, ...layout.targets]) {
          assert(!overlaps(layout.warning, other), `Needs attention does not overlap template text or controls at ${width}px`);
        }
        for (const box of [layout.warning, layout.actions, ...layout.targets]) {
          assert(box.left >= layout.row.left - 1 && box.right <= layout.row.right + 1 && box.top >= layout.row.top - 1 && box.bottom <= layout.row.bottom + 1, `template status and controls stay inside their row at ${width}px`);
        }
        assert(await page.locator("#view-library").evaluate(e => e.scrollWidth <= e.clientWidth + 1), `template list has no horizontal overflow at ${width}px`);
        if (process.env.ARTIFACT_DIR && lang === "zh") {
          await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
          await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `prompt-attention-${engine}-${width}.png`), fullPage: true });
        }
      }
      await page.setViewportSize({ width: 560, height: 860 });
      const finishToggle = () => page.waitForFunction(() => !document.querySelector('.lib-prompt-row .lib-agents[aria-busy="true"]'));
      await all.focus();
      const scrollBefore = await page.locator("#view-library").evaluate(e => e.scrollTop);
      await all.press("Space");
      await finishToggle();
      assert.deepEqual(sent.filter(s => s.p.endsWith("/agents")).at(-1).body.agents.sort(), ["claude", "codex", "pi"]);
      assert.equal(await all.getAttribute("aria-pressed"), "true");
      assert(!(await page.locator("#status").textContent()).includes("/reload"), "reload hints only describe successfully deployed targets");
      assert(await all.evaluate(e => e === document.activeElement), "in-place updates preserve keyboard focus");
      assert.equal(await page.locator("#view-library").evaluate(e => e.scrollTop), scrollBefore, "target toggles preserve page position");
      if (process.env.ARTIFACT_DIR && lang === "zh") {
        for (const color of ["light", "dark"]) {
          await page.evaluate(color => { document.documentElement.dataset.theme = color; }, color);
          for (const width of [1100, 420]) {
            await page.setViewportSize({ width, height: 900 });
            await page.locator(".lib-prompts").scrollIntoViewIfNeeded();
            await page.locator(".lib-prompt-row .lib-prompt-delete").focus();
            await page.waitForFunction(() => getComputedStyle(document.activeElement.parentElement).opacity === "1");
            await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `prompt-list-${engine}-${width}-${color}.png`), fullPage: true });
            assert(await page.locator("#view-library").evaluate(e => e.scrollWidth <= e.clientWidth + 1), "template list fits a narrow window");
          }
        }
        await page.evaluate(() => { document.documentElement.dataset.theme = "light"; });
        await page.setViewportSize({ width: 560, height: 860 });
      }
      await page.locator(".lib-set").first().click();
      await page.locator('textarea[data-lib="set:default"]').fill("尚未保存的共享指令");
      await all.press("Space");
      await finishToggle();
      assert.deepEqual(sent.filter(s => s.p.endsWith("/agents")).at(-1).body.agents, ["pi"], "All preserves a blocked selection");
      assert.equal(await chip("pi").getAttribute("aria-pressed"), "false", "an expired selection is not displayed as deployed");
      await chip("pi").press("Space");
      await finishToggle();
      assert.deepEqual(sent.filter(s => s.p.endsWith("/agents")).at(-1).body.agents, [], "an abnormal target can be explicitly deselected");
      assert.equal(await page.locator(".lib-prompt-row .lib-tags .warn").count(), 0, "Needs attention clears when the abnormal target is deselected");
      await chip("claude").press("Space");
      await finishToggle();
      items[0].agents = ["claude", "codex"];
      await refreshList();
      await all.waitFor();
      assert.equal(await all.getAttribute("aria-pressed"), "false");
      await all.press("Space");
      await finishToggle();
      assert.deepEqual(sent.filter(s => s.p.endsWith("/agents")).at(-1).body.agents.sort(), ["claude", "codex"], "All retries selected targets that are not actually deployed");
      Object.assign(states.find(s => s.agent === "codex"), { status: "conflict", reason: "This name is already in use" });
      await all.press("Space");
      await finishToggle();
      assert.equal(await all.count(), 0, "a newly blocked target removes an ineligible All control");
      assert(await chip("claude").evaluate(e => e === document.activeElement), "focus moves to an agent when the focused All control disappears");
      assert.equal(await page.locator(".lib-prompt-editor").count(), 0);
      assert.equal(await page.locator('textarea[data-lib="set:default"]').inputValue(), "尚未保存的共享指令", "target-only changes retain shared instruction drafts");
      await page.locator(".lib-prompt-search").fill("unmatched");
      await page.locator(".lib-prompt-row").waitFor({ state: "detached" });
      assert.equal(await page.locator(".lib-prompt-row").count(), 0);
      await page.locator(".lib-prompt-search").fill("边界");
      assert.equal(await page.locator(".lib-prompt-row .lib-ag[data-agent]").count(), targets.length, "filtering preserves target controls");
      // IME Enter remains in the search field; it must not activate a row or
      // open the template editor while composition is in progress.
      await page.locator(".lib-prompt-search").evaluate(e => {
        e.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
        e.value = "边界";
        e.dispatchEvent(new InputEvent("input", { bubbles: true, isComposing: true, data: "边界" }));
        e.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, key: "Enter", isComposing: true }));
        e.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "边界" }));
      });
      assert.equal(await page.locator(".lib-prompt-editor").count(), 0);
      assert.equal(await page.locator(".lib-prompt-row").count(), 1);
      items[0].name = "Review";
      await page.locator(".lib-prompt-search").fill("ＲＥＶＩＥＷ");
      // The view uses the last backend snapshot; refresh publishes the renamed
      // template while retaining the search text and shared draft.
      await refreshList();
      await page.locator(".lib-prompt-row").waitFor();
      assert.equal(await page.locator(".lib-prompt-search").inputValue(), "ＲＥＶＩＥＷ");
      assert.equal(await page.locator(".lib-prompt-row").count(), 1, "NFKC and case-insensitive name matching survive refresh");
      items[0].name = "审查";
      await page.locator(".lib-prompt-search").fill("边界");
      await refreshList();
      await page.locator(".lib-prompt-row .name").filter({ hasText: "/审查" }).waitFor();
      assert.equal(await page.locator('textarea[data-lib="set:default"]').inputValue(), "尚未保存的共享指令", "search and refresh also retain the shared draft");
      // Finish the shared-draft scenario through its normal discard action
      // before exercising pointer navigation into the template editor.
      await page.locator(".lib-savebar button").first().click();
      await page.locator(".action-confirm button.primary").click();
      await page.locator(".lib-savebar").waitFor({ state: "hidden" });
      await page.locator("#view-library").hover();
      await page.mouse.wheel(0, -2000);
      await page.waitForFunction(() => document.querySelector("#view-library").scrollTop === 0);
      await page.locator(".lib-set.open .name").click();
      await page.locator(".lib-prompt-row .name").click();
      assert.equal(await page.locator('[data-prompt="name"]').inputValue(), "审查");
      await page.locator('[data-prompt="name"]').fill("review-code");
      assert((await page.locator('.lib-prompt-target[data-agent="claude"]').textContent()).includes({ en: "Native behavior", zh: "原生行为", "zh-TW": "原生行為", ja: "動作は未検証", de: "Natives Verhalten" }[lang]), "editor retains native acceptance information");
      assert.equal(await page.locator("#modal select").count(), 0);
      assert(await page.locator("#modal").evaluate(el => el.scrollWidth <= el.clientWidth + 1));
      // Enable Pi through the page, then expire its runtime answer. Sending an
      // accidental deselection on the following save would remove its deployment.
      const piState = states.find(s => s.agent === "pi");
      Object.assign(piState, { status: "available", reason: "", source: "" });
      await page.locator(".lib-prompt-check").click();
      await page.waitForFunction(() => !document.querySelector(".lib-prompt-check").disabled);
      await page.locator('.lib-prompt-target[data-agent="pi"] input').check();
      await page.locator(".lib-prompt-save").click();
      await page.locator(".lib-prompt-editor").waitFor({ state: "detached" });
      Object.assign(piState, { status: "unverified", reason: "Check Pi command availability again" });
      await page.locator(".lib-prompt-row .name").click();
      await page.locator('[data-prompt="text"]').fill("检查边界条件。更新草稿");
      await page.locator(".lib-prompt-save").click();
      await page.locator(".lib-prompt-editor").waitFor({ state: "detached" });
      assert(sent.filter(s => s.p.endsWith("/save")).at(-1).body.agents.includes("pi"), "expired Pi verification must not become a deployment removal");
      await page.locator(".lib-prompt-row .name").click();
      assert(await page.locator('.lib-prompt-target[data-agent="pi"] input').isChecked(), "retained target selection is visible while awaiting verification");
      assert(await page.locator('.lib-prompt-target[data-agent="pi"] input').isEnabled(), "the user can explicitly turn off an existing unverified target");
      // Exercise the native keyboard toggle without Playwright scrolling a
      // checkbox during the modal's opening animation in WebKit.
      await page.waitForFunction(() => document.activeElement?.dataset.prompt === "name");
      await page.locator('.lib-prompt-target[data-agent="pi"] input').press("Space");
      assert(!await page.locator('.lib-prompt-target[data-agent="pi"] input').isChecked());
      await page.locator(".lib-prompt-save").click();
      await page.locator(".lib-prompt-editor").waitFor({ state: "detached" });
      assert(!sent.filter(s => s.p.endsWith("/save")).at(-1).body.agents.includes("pi"), "an explicit target removal is saved");
      // A retained Codex target stays removable when the Agent reports it
      // unavailable; merely editing the body must not clear that selection.
      const codexState = states.find(s => s.agent === "codex");
      Object.assign(codexState, { status: "available", reason: "", source: "" });
      await page.locator(".lib-prompt-row .name").click();
      await page.locator(".lib-prompt-check").click();
      await page.waitForFunction(() => !document.querySelector(".lib-prompt-check").disabled);
      await page.locator('.lib-prompt-target[data-agent="codex"] input').check();
      await page.locator(".lib-prompt-save").click();
      await page.locator(".lib-prompt-editor").waitFor({ state: "detached" });
      targets.find(a => a.id === "codex").detected = false;
      Object.assign(codexState, { status: "unsupported", reason: "Not installed here" });
      await refreshList();
      await page.locator(".lib-prompt-row .name").click();
      const goneCodex = page.locator('.lib-prompt-target[data-agent="codex"] input');
      assert(await goneCodex.isChecked(), "unavailable existing Codex remains visibly selected");
      assert(await goneCodex.isEnabled(), "unavailable existing Codex can be explicitly removed");
      await page.locator('[data-prompt="text"]').fill("检查边界条件。Codex 不可用时保留选择");
      await page.locator(".lib-prompt-save").click();
      await page.locator(".lib-prompt-editor").waitFor({ state: "detached" });
      assert(sent.filter(s => s.p.endsWith("/save")).at(-1).body.agents.includes("codex"), "passive preflight preserves unavailable existing Codex");
      await page.locator(".lib-prompt-row .name").click();
      await page.waitForFunction(() => document.activeElement?.dataset.prompt === "name");
      await goneCodex.press("Space");
      assert(!await goneCodex.isChecked());
      assert(await goneCodex.isDisabled(), "an unavailable target cannot be re-enabled after explicit removal");
      await page.locator(".lib-prompt-save").click();
      await page.locator(".lib-prompt-editor").waitFor({ state: "detached" });
      assert(!sent.filter(s => s.p.endsWith("/save")).at(-1).body.agents.includes("codex"));
      await page.locator(".lib-prompt-row .name").click();
      assert.equal(await page.locator('.lib-prompt-target[data-agent="codex"]').count(), 0, "unselected and unavailable Codex is not offered");
      await page.locator(".lib-prompt-remove").click();
      await page.locator(".lib-prompt-confirm-remove").click();
      await page.locator(".lib-prompt-row").waitFor({ state: "detached" });
      assert.deepEqual(errors, []);
      await ctx.close();
    });
  });
}
