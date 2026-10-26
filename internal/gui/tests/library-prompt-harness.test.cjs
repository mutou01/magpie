const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const languages = ["en", "zh", "zh-TW", "ja", "de"];
const countLabel = (n, total) => `${n}/${total}`;

async function fixture(t, engine, lang) {
  const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch(process.env.CHROMIUM_EXECUTABLE ? { executablePath: process.env.CHROMIUM_EXECUTABLE } : { channel: "chromium" }));
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport: { width: 1100, height: 900 }, reducedMotion: "reduce" });
  const page = await ctx.newPage();
  page.setDefaultTimeout(3000);
  const sent = [], errors = [];
  page.on("pageerror", e => errors.push(e.message));
  const targets = [
    { id: "claude", name: "Claude Code", icon: "claudecode-color", detected: true },
    { id: "pi", name: "Pi", icon: "pi", detected: true },
    { id: "codex", name: "Codex", icon: "codex-color", detected: true },
    { id: "dsh", name: "DeepSeek Harness", icon: "deepseek-color", detected: true },
  ];
  const state = {
    enabled: true,
    items: [
      { id: "visible", name: "visible-review", text: "visible body", agents: ["claude", "codex"], states: [{ agent: "claude", status: "enabled" }, { agent: "codex", status: "available", deployed: false }] },
      { id: "hidden", name: "hidden-audit", text: "hidden body", agents: [], states: [{ agent: "claude", status: "available" }, { agent: "codex", status: "enabled", deployed: true }] },
    ],
    checks: targets.map(a => ({ agent: a.id, status: "available", reason: "fixture native check passed" })),
    targets,
  };
  let held, release, problems = [], loseResponse = false, readStarted, releaseRead, undeployed, omitted, omitState = false;
  const view = () => structuredClone(state);
  await ctx.addInitScript(() => localStorage.setItem("magpie.libTab", "instructions"));
  await page.route("**/*", async route => {
    const request = route.request(), url = new URL(request.url()), p = url.pathname;
    if (!["http:", "https:"].includes(url.protocol)) return route.continue();
    if (p === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs={lang:${JSON.stringify(lang)},theme:"light",web:true}` });
    if (p === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window={}" });
    if (p === "/api/state") return route.fulfill({ json: { agents: [], profiles: [], settings: { lang, theme: "light" } } });
    if (p === "/api/plugins") return route.fulfill({ json: { plugins: [] } });
    if (p === "/api/providers") return route.fulfill({ json: { providers: [], gateway: { running: true } } });
    if (p === "/api/library") return route.fulfill({ json: { agents: targets, servers: [], skills: [], foundSkills: [], foundServers: [], projects: [], prompts: view(), instructions: { shared: "", sets: [{ id: "default", name: "", text: "", active: true }], agents: [] } } });
    if (p === "/api/library/prompts") {
      if (readStarted) { readStarted(); readStarted = undefined; await new Promise(resolve => { releaseRead = resolve; }); }
      return route.fulfill({ json: view() });
    }
    if (p.startsWith("/api/library/prompts/")) {
      const body = request.postDataJSON();
      sent.push({ p, method: request.method(), body });
      if (p.endsWith("/harness")) {
        if (held) { held(); held = undefined; await new Promise(resolve => { release = resolve; }); }
        for (const item of state.items) {
          if (problems.some(problem => problem.agent === body.target && problem.what === `prompt:${item.name}`)) continue;
          item.agents = item.agents.filter(a => a !== body.target);
          if (body.enabled) item.agents.push(body.target);
          item.states = item.states.filter(s => s.agent !== body.target);
          const deployed = body.enabled && item.id !== undeployed;
          if (!omitState) item.states.push({ agent: body.target, status: deployed ? "enabled" : "available", deployed });
        }
        if (loseResponse) { loseResponse = false; return route.abort("failed"); }
      }
      if (p.endsWith("/agents")) {
        const item = state.items.find(item => item.id === body.id);
        item.agents = body.agents;
        item.states = targets.map(a => ({ agent: a.id, status: body.agents.includes(a.id) ? "enabled" : "available", deployed: body.agents.includes(a.id) }));
      }
      if (p.endsWith("/enabled")) state.enabled = body.enabled;
      if (p.endsWith("/check")) return route.fulfill({ json: state.checks });
      const prompts = view();
      if (p.endsWith("/harness")) prompts.items = prompts.items.filter(item => item.id !== omitted);
      return route.fulfill({ json: { part: "prompts", prompts, result: { changed: [], problems } } });
    }
    if (p.startsWith("/api/")) return route.fulfill({ json: p === "/api/usage/quotas" ? [] : {} });
    const file = path.join(assets, p === "/" ? "index.html" : p);
    return route.fulfill({ body: await fs.readFile(file), contentType: { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml" }[path.extname(file)] });
  });
  const open = async () => {
    await page.goto("http://magpie.test/");
    await page.locator('button[data-view="library"]').click();
    await page.locator(".lib-prompt-row").first().waitFor();
  };
  t.after(() => assert.deepEqual(errors, []));
  t.after(() => release?.());
  t.after(() => releaseRead?.());
  await open();
  return { page, state, sent, open, icon: id => page.locator(`.lib-prompt-harness-check[data-agent="${id}"]`), hold: () => new Promise(resolve => { held = resolve; }), release: () => release?.(), fail: value => { problems = value; }, undeployed: id => { undeployed = id; }, omitFromReply: id => { omitted = id; }, omitState: () => { omitState = true; }, loseResponse: () => { loseResponse = true; }, holdRecovery: () => new Promise(resolve => { readStarted = resolve; }), releaseRecovery: () => releaseRead?.() };
}

async function started(promise) {
  let timer;
  try { await Promise.race([promise, new Promise((_, reject) => { timer = setTimeout(() => reject(new Error("Harness POST did not start")), 5000); })]); }
  finally { clearTimeout(timer); }
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) for (const lang of languages) {
  test(`${engine}: ${lang}: overview bulk completion names the Harness and confirmed outcome`, async t => {
    const f = await fixture(t, engine, lang), { page } = f;
    const labels = {
      en: ["Pi started", "Pi stopped"], zh: ["Pi启动", "Pi停用"], "zh-TW": ["Pi啟動", "Pi停用"],
      ja: ["Piを有効化", "Piを無効化"], de: ["Pi aktiviert", "Pi deaktiviert"],
    }[lang];
    for (const [i, enabled] of [true, false].entries()) {
      await f.icon("pi").press("Enter");
      await page.waitForFunction(() => document.querySelector('.lib-prompt-harness-check[data-agent="pi"]')?.disabled === false);
      assert(f.state.items.every(item => item.agents.includes("pi") === enabled), "the server confirms every template reached the requested state");
      assert.equal(await page.locator("#status").textContent(), labels[i], "a successful bulk action names its Harness and localized direction");
      assert(await page.locator("#status").evaluate(e => e.classList.contains("ok")), "confirmed complete success uses the green status");
    }
  });

  test(`${engine}: ${lang}: overview omitted template cannot shrink the operation total`, async t => {
    const f = await fixture(t, engine, lang), { page, sent } = f;
    assert.equal(await page.locator(".lib-prompt-row").count(), 2, "the operation starts with two current templates");
    f.omitFromReply("hidden");
    await f.icon("pi").click();
    await page.waitForFunction(() => document.querySelector('.lib-prompt-harness-check[data-agent="pi"]')?.disabled === false);
    assert.deepEqual(sent.at(-1).body, { target: "pi", enabled: true });
    assert.equal(await page.locator(".lib-prompt-row").count(), 1, "the successful public response omitted one original template");
    assert(await page.locator("#status").evaluate(e => e.classList.contains("warn")), "missing deployment evidence cannot become a green success");
    const text = await page.locator("#status").textContent();
    assert(text.includes("Pi") && text.includes("1/2"), "the operation total remains the original snapshot size");
  });

  test(`${engine}: ${lang}: overview HTTP success without complete deployment remains a warning`, async t => {
    const f = await fixture(t, engine, lang), { page } = f;
    f.undeployed("hidden");
    await f.icon("pi").click();
    await page.waitForFunction(() => document.querySelector('.lib-prompt-harness-check[data-agent="pi"]')?.disabled === false);
    assert(f.state.items.every(item => item.agents.includes("pi")), "both templates were selected in the successful HTTP response");
    assert.equal(f.state.items.filter(item => item.states.some(s => s.agent === "pi" && s.deployed)).length, 1, "only one selected template was actually deployed");
    assert(await page.locator("#status").evaluate(e => e.classList.contains("warn")), "an empty problems array is not evidence of complete deployment");
    const text = await page.locator("#status").textContent();
    assert(text.includes("Pi") && text.includes("1/2"), "the warning names the target and reports the returned actual deployment count");
    f.undeployed(undefined);
    await f.icon("pi").click();
    await page.waitForFunction(() => document.querySelector('.lib-prompt-harness-check[data-agent="pi"]')?.disabled === false);
    assert(f.state.items.every(item => item.states.some(s => s.agent === "pi" && s.deployed)), "the disable operation starts with both deployments confirmed");
    f.omitState();
    await f.icon("pi").click();
    await page.waitForFunction(() => document.querySelector('.lib-prompt-harness-check[data-agent="pi"]')?.disabled === false);
    assert(f.state.items.every(item => !item.agents.includes("pi") && !item.states.some(s => s.agent === "pi")), "the successful disable response removed selection but omitted actual target state");
    assert(await page.locator("#status").evaluate(e => e.classList.contains("warn")), "absent target state is not evidence of successful removal");
    const disableText = await page.locator("#status").textContent();
    assert(disableText.includes("Pi") && disableText.includes("0/2"), "no removal is confirmed without returned target state");
  });

  test(`${engine}: ${lang}: overview bulk warning counts actual target success without hiding other Harness problems`, async t => {
    const f = await fixture(t, engine, lang), { page } = f;
    const settled = () => page.waitForFunction(() => document.querySelector('.lib-prompt-harness-check[data-agent="pi"]')?.disabled === false);
    for (const enabled of [true, false]) {
      if (!enabled) {
        f.fail([]);
        await f.icon("pi").click();
        await settled();
        assert(f.state.items.every(item => item.agents.includes("pi")), "disable failure starts with all templates deployed");
      }
      f.fail([{ what: "prompt:hidden-audit", agent: "pi", error: "fixture target deployment conflict" }]);
      await f.icon("pi").click();
      await settled();
      const text = await page.locator("#status").textContent();
      assert(await page.locator("#status").evaluate(e => e.classList.contains("warn")), "HTTP 200 with incomplete deployment remains an orange warning");
      assert(text.includes("1/2"), "the warning reports actual successful templates out of the current total");
      for (const part of ["Pi", "hidden-audit", "fixture target deployment conflict"]) assert(text.includes(part), `the warning preserves the target and item reason: ${part}`);
    }
    f.fail([{ what: "prompt:hidden-audit", agent: "codex", error: "fixture unrelated Codex conflict" }]);
    await f.icon("pi").click();
    await settled();
    assert(f.state.items.every(item => item.agents.includes("pi")), "another Harness problem does not prevent Pi deployment in the public fixture");
    const text = await page.locator("#status").textContent();
    assert(await page.locator("#status").evaluate(e => e.classList.contains("warn")), "a genuine other-Harness problem remains visible as a warning");
    for (const part of ["Pi", "2/2", "Codex", "hidden-audit", "fixture unrelated Codex conflict"]) assert(text.includes(part), `the result preserves target success and attributes the other failure: ${part}`);
  });

  test(`${engine}: ${lang}: overview Harness hover and keyboard focus emphasize only enabled controls`, async t => {
    const f = await fixture(t, engine, lang), { page } = f;
    await page.emulateMedia({ reducedMotion: "no-preference" });
    const icon = f.icon("pi");
    const appearance = () => icon.evaluate(e => {
      const s = getComputedStyle(e);
      return { scale: s.transform === "none" ? 1 : new DOMMatrixReadOnly(s.transform).a, shadow: s.boxShadow, duration: s.transitionDuration };
    });
    const idle = await appearance();
    await icon.hover();
    await page.waitForFunction(() => {
      const s = getComputedStyle(document.querySelector('.lib-prompt-harness-check[data-agent="pi"]'));
      return s.transform !== "none" && new DOMMatrixReadOnly(s.transform).a > 1;
    });
    const hovered = await appearance();
    assert(hovered.scale > 1 && hovered.scale < 1.2, "hover uses a slight enlargement");
    assert(hovered.shadow !== "none" && hovered.shadow !== idle.shadow, "hover adds visible shadow emphasis");
    await page.mouse.down();
    try {
      await page.waitForFunction(() => new DOMMatrixReadOnly(getComputedStyle(document.querySelector('.lib-prompt-harness-check[data-agent="pi"]')).transform).a < 1);
    } finally { await page.mouse.move(0, 0); await page.mouse.up(); }
    await f.icon("claude").focus();
    await page.keyboard.press("Tab");
    assert(await icon.evaluate(e => e === document.activeElement && e.matches(":focus-visible")), "keyboard navigation reaches the real focus-visible Harness control");
    await page.waitForFunction(() => new DOMMatrixReadOnly(getComputedStyle(document.querySelector('.lib-prompt-harness-check[data-agent="pi"]')).transform).a > 1);
    assert.notEqual((await appearance()).shadow, "none", "keyboard focus also has shadow emphasis");
    await page.emulateMedia({ reducedMotion: "reduce" });
    assert((await appearance()).duration.split(",").every(n => parseFloat(n) <= .01), "reduced motion removes animated enlargement transitions");
    f.state.enabled = false;
    await f.open();
    await icon.hover({ force: true });
    assert(await icon.isDisabled());
    assert.equal((await appearance()).scale, 1, "disabled controls do not enlarge on hover");
    assert.equal(f.sent.length, 0, "hover, focus and cancelled pointer presses never submit a mutation");
  });

  test(`${engine}: ${lang}: overview partial result is not retained after row repair and refresh`, async t => {
    const f = await fixture(t, engine, lang), { page, sent } = f;
    f.state.items[1].states.push({ agent: "pi", status: "available" });
    await f.open();
    f.fail([{ what: "prompt:hidden-audit", agent: "pi", error: "fixture repaired template conflict" }]);
    await f.icon("pi").click();
    await page.locator("#status").filter({ hasText: "fixture repaired template conflict" }).waitFor();
    const batchResult = await page.locator("#status").textContent();
    assert(batchResult.includes("hidden-audit"), "the batch result explicitly identifies the failed template");
    assert(f.state.items[0].agents.includes("pi") && !f.state.items[1].agents.includes("pi"), "one template deployed and the conflicting template remained unselected");
    f.fail([]);
    await page.locator('.lib-prompt-row[data-prompt-id="hidden"] .lib-ag[data-agent="pi"]').press("Space");
    await page.waitForFunction(() => !document.querySelector('.lib-prompt-row .lib-agents[aria-busy="true"]'));
    assert.deepEqual(sent.filter(request => request.p.endsWith("/agents")).at(-1)?.body, { id: "hidden", agents: ["pi"] }, "the formerly conflicting template is successfully repaired through the public row mutation");
    assert(f.state.items.every(item => item.agents.includes("pi")));
    assert((await f.icon("pi").getAttribute("title")).includes("2/2"), "the confirmed count reflects the repaired template");
    assert(!(await page.locator("#status").textContent()).includes("fixture repaired template conflict"), "the row success replaces the old transient batch result");
    const staleNotice = () => page.locator(".lib-prompts .lib-problems").filter({ hasText: "fixture repaired template conflict" }).count();
    const afterRepair = await staleNotice();
    const oldRefresh = await page.locator(".lib-prompt-refresh").elementHandle();
    await oldRefresh.press("Space");
    await page.waitForFunction(e => !e.isConnected, oldRefresh);
    const afterRefresh = await staleNotice();
    assert.deepEqual({ afterRepair, afterRefresh }, { afterRepair: 0, afterRefresh: 0 }, "a historical batch failure must not remain as a current persistent problem after successful repair or Refresh");
  });

  test(`${engine}: ${lang}: overview response preserves active search caret`, async t => {
    const f = await fixture(t, engine, lang), { page } = f;
    const pending = f.hold();
    await f.icon("claude").click();
    await started(pending);
    const search = page.locator(".lib-prompt-search");
    await search.fill("visible-review");
    await search.evaluate(e => { e.focus(); e.setSelectionRange(2, 7); });
    const before = await search.evaluate(e => ({ active: e === document.activeElement, value: e.value, start: e.selectionStart, end: e.selectionEnd }));
    assert.deepEqual(before, { active: true, value: "visible-review", start: 2, end: 7 }, "the reader is editing a selected substring while the write is held");
    f.release();
    await page.waitForFunction(() => document.querySelector('#view-library')?.getAttribute("aria-busy") !== "true" && document.querySelector('.lib-prompt-harness-check[data-agent="claude"]')?.disabled === false);
    assert.deepEqual(await search.evaluate(e => ({ active: e === document.activeElement, value: e.value, start: e.selectionStart, end: e.selectionEnd })), before, "batch completion preserves actual search focus, text and selection");
    assert.equal(await page.locator(".lib-prompt-row").count(), 1, "the active filter survives the bulk response");
  });

  test(`${engine}: ${lang}: overview lost response recovers before accepting another write`, async t => {
    const f = await fixture(t, engine, lang), { page, sent } = f;
    f.loseResponse();
    const reading = f.holdRecovery();
    await f.icon("claude").click();
    await started(reading);
    try {
      assert(f.state.items.every(item => item.agents.includes("claude")), "the fixture completed the server write before losing its response");
      assert(await f.icon("claude").isDisabled(), "recovery keeps the first Harness disabled");
      assert(await f.icon("pi").isDisabled(), "recovery keeps other Harnesses disabled");
      await f.icon("claude").dispatchEvent("click");
      await f.icon("pi").dispatchEvent("click");
      await page.locator('.lib-prompts [role="switch"]').dispatchEvent("click");
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
      assert.equal(sent.length, 1, "queued clicks cannot race the recovery GET");
    } finally { f.releaseRecovery(); }
    await page.waitForFunction(() => document.querySelector('.lib-prompt-harness-check[data-agent="claude"]')?.disabled === false);
    assert((await f.icon("claude").getAttribute("title")).includes("2/2"), "GET recovery paints the confirmed server deployment count");
    assert.equal(await f.icon("claude").getAttribute("aria-pressed"), "true");
    await f.icon("claude").click();
    await page.waitForFunction(() => document.querySelector('.lib-prompt-harness-check[data-agent="claude"]')?.disabled === false);
    assert.deepEqual(sent.map(request => request.body), [{ target: "claude", enabled: true }, { target: "claude", enabled: false }], "the next click derives its direction from recovered server state");
  });

  test(`${engine}: ${lang}: overview layout and deployment counts`, async t => {
    const f = await fixture(t, engine, lang), { page } = f;
    const icons = page.locator(".lib-prompt-harness-check");
    assert.equal(await icons.count(), 4);
    assert(await icons.evaluateAll(nodes => nodes.every(node => node.closest(".lib-cardhead"))), "all four Harness controls belong to the title row");
    assert(await icons.first().evaluate(node => node.parentElement.previousElementSibling?.getAttribute("role") === "switch"), "Harness controls immediately follow the global switch");
    for (const [id, n] of [["claude", 1], ["codex", 0], ["pi", 0], ["dsh", 0]]) {
      const icon = f.icon(id);
      assert.equal(await icon.evaluate(e => e.classList.contains("on")), n > 0, `${id}: only selected actual deployments light the overview`);
      for (const attribute of ["title", "aria-label"]) {
        const label = await icon.getAttribute(attribute);
        assert(label.includes(countLabel(n, 2)), `${id}: ${attribute} exposes deployed/total count`);
        if (lang !== "en") assert(!label.includes(`Enabled ${n}/2 templates`), `${id}: the count description is translated`);
      }
    }
    for (const width of [1100, 560, 420, 360]) {
      await page.setViewportSize({ width, height: 900 });
      const layout = await page.locator('.lib-prompts .lib-cardhead').evaluate(head => {
        const nodes = [...head.querySelectorAll('b, [role="switch"], .lib-prompt-harness-check, button:not(.lib-prompt-harness-check):not([role="switch"])')];
        return { overflow: document.documentElement.scrollWidth > innerWidth, boxes: nodes.map(n => { const r = n.getBoundingClientRect(); return { left: r.left, right: r.right, top: r.top, bottom: r.bottom, width: r.width }; }) };
      });
      assert.equal(layout.overflow, false, `${width}px: no horizontal page overflow`);
      for (const [i, a] of layout.boxes.entries()) {
        assert(a.width > 0 && a.left >= 0 && a.right <= width, `${width}px: every title control fits the viewport`);
        for (const b of layout.boxes.slice(i + 1)) assert(a.right <= b.left + 1 || b.right <= a.left + 1 || a.bottom <= b.top + 1 || b.bottom <= a.top + 1, `${width}px: title controls never overlap`);
      }
    }
    assert.equal(f.sent.length, 0, "layout and focus do not launch checks or writes");
  });

  test(`${engine}: ${lang}: overview bulk intents include hidden templates and serialize writes`, async t => {
    const f = await fixture(t, engine, lang), { page, sent } = f;
    await page.locator(".lib-prompt-search").fill("visible-review");
    assert.equal(await page.locator(".lib-prompt-row").count(), 1);
    const pending = f.hold();
    await f.icon("claude").press("Space");
    await started(pending);
    try {
      assert.deepEqual(sent, [{ p: "/api/library/prompts/harness", method: "POST", body: { target: "claude", enabled: true } }], "partial selection enables every current template through one public mutation");
      for (const locator of [f.icon("claude"), f.icon("pi"), page.locator('.lib-prompts [role="switch"]'), page.locator('.lib-prompts .lib-prompt-refresh')]) {
        if (await locator.count() === 1) await locator.dispatchEvent("click");
      }
      await page.locator('.lib-prompt-row .lib-ag[data-agent="claude"]').dispatchEvent("click");
      await page.locator(".lib-prompt-delete").first().dispatchEvent("click");
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
      assert.equal(sent.length, 1, "double clicks, other Harnesses, switch, refresh, row writes and removal cannot cross an active bulk write");
    } finally { f.release(); }
    await page.waitForFunction(() => document.querySelector('.lib-prompt-harness-check[data-agent="claude"]')?.disabled === false);
    assert(f.state.items.every(p => p.agents.includes("claude")), "the hidden template participates in enable-all");
    assert((await f.icon("claude").getAttribute("title")).includes(countLabel(2, 2)));
    await f.icon("claude").click();
    await page.waitForFunction(() => document.querySelector('.lib-prompt-harness-check[data-agent="claude"]')?.disabled === false);
    assert.deepEqual(sent.at(-1).body, { target: "claude", enabled: false }, "a fully enabled Harness toggles all current templates off");
    assert(f.state.items.every(p => !p.agents.includes("claude")));
    await page.locator(".lib-prompt-add").click();
    assert.equal(await page.locator('.lib-prompt-target input:checked').count(), 0, "bulk intent does not create a default selection for new templates");
    await page.locator(".lib-prompt-editor .bar button").nth(1).click();
    await page.locator("#modal").waitFor({ state: "hidden" });
    f.state.enabled = false;
    await f.open();
    const before = sent.length;
    assert(await f.icon("claude").isDisabled(), "the global off state disables Harness mutations");
    await f.icon("claude").dispatchEvent("click");
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    assert.equal(sent.length, before, "a queued event cannot bypass global off");
  });

  test(`${engine}: ${lang}: overview distinguishes actual check failures and reports partial results`, async t => {
    const f = await fixture(t, engine, lang), { page } = f;
    f.state.checks = [
      { agent: "claude", status: "unverified", reason: "Check native command availability again" },
      { agent: "pi", status: "unverified", reason: "Check Pi command availability again" },
      { agent: "dsh", status: "unverified", reason: "fixture native executable rejected inventory" },
    ];
    f.state.items[0].states[0] = { agent: "claude", status: "conflict", deployed: false, reason: "fixture individual template name conflict" };
    await f.open();
    for (const id of ["claude", "pi", "codex"]) assert.equal(await f.icon(id).evaluate(e => e.classList.contains("warn")), false, `${id}: missing or expired check evidence and item conflicts do not raise an overview warning`);
    assert(await f.icon("dsh").evaluate(e => e.classList.contains("warn")), "a completed failed availability check raises an overview warning");
    const color = await f.icon("dsh").evaluate(e => ({ actual: getComputedStyle(e, "::after").backgroundColor, expected: (() => { const n = document.createElement("span"); n.style.color = "var(--red)"; e.append(n); const color = getComputedStyle(n).color; n.remove(); return color; })() }));
    assert.equal(color.actual, color.expected, "actual failed checks use a red indicator");
    f.fail([{ what: "prompt:visible-review", agent: "pi", error: "fixture visible template conflict" }, { what: "prompt:hidden-audit", agent: "pi", error: "fixture hidden template write denied" }]);
    await f.icon("pi").press("Enter");
    await page.locator("#status").filter({ hasText: "fixture hidden template write denied" }).waitFor();
    const status = await page.locator("#status").textContent();
    assert(await page.locator("#status").evaluate(e => e.classList.contains("warn")), "a completely failed batch remains an orange warning");
    assert(status.includes("0/2"), "a completely failed enable batch reports zero confirmed deployments");
    for (const text of ["visible-review", "hidden-audit", "fixture visible template conflict", "fixture hidden template write denied"]) assert(status.includes(text), `partial result identifies each failed template and reason: ${text}`);
    assert.equal(await f.icon("pi").evaluate(e => e.classList.contains("warn")), false, "individual deployment failures do not become global availability failures");
  });
}
