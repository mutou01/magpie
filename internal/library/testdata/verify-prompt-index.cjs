// Windows acceptance: two real magpie processes share only an isolated profile.
// Usage: node verify-prompt-index.cjs <feature.exe> [pre-template-writer.exe]
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const net = require("node:net");
const { spawn } = require("node:child_process");

(async () => {
  assert.equal(process.platform, "win32", "This acceptance procedure is for Windows");
  assert(process.argv[2], "Supply a built magpie executable");
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "magpie-prompt-index-"));
  const data = path.join(root, "data"), index = path.join(data, "library/prompts.json");
  const env = { SystemRoot: process.env.SystemRoot, COMSPEC: process.env.COMSPEC,
    PATH: path.join(process.env.SystemRoot, "System32"), HOME: root, USERPROFILE: root,
    APPDATA: root, LOCALAPPDATA: root, XDG_CONFIG_HOME: root, XDG_CACHE_HOME: root,
    XDG_DATA_HOME: root, XDG_STATE_HOME: root, TEMP: root, TMP: root,
    MAGPIE_TEST_SANDBOX: root, MAGPIE_NO_ACCOUNTS: "1", MAGPIE_WEB_KEY: "fixture-prompt-index",
    HTTP_PROXY: "http://127.0.0.1:9", HTTPS_PROXY: "http://127.0.0.1:9", NO_PROXY: "127.0.0.1,localhost" };
  const probe = net.createServer();
  await new Promise(resolve => probe.listen(0, "127.0.0.1", resolve));
  const port = probe.address().port;
  await new Promise(resolve => probe.close(resolve));
  const url = `http://127.0.0.1:${port}`;
  let current, cookie, logs = "";
  const browsers = [], pages = [];
  const stop = async () => {
    if (!current) return;
    if (current.child.exitCode === null) current.child.kill();
    await current.exit;
    current = null;
  };
  const start = async executable => {
    const child = spawn(executable, ["web", "--addr", `127.0.0.1:${port}`, "--no-open"], { cwd: root, env, windowsHide: true, stdio: ["ignore", "pipe", "pipe"] });
    current = { child, exit: new Promise(resolve => { child.once("exit", resolve); child.once("error", resolve); }) };
    child.on("error", e => { logs = String(e); });
    for (const out of [child.stdout, child.stderr]) out.on("data", b => { logs = (logs + b).slice(-4000); });
    for (let i = 0; i < 100; i++) {
      const r = await fetch(url + "/?k=fixture-prompt-index", { redirect: "manual", signal: AbortSignal.timeout(1000) }).catch(() => null);
      if (r?.headers.get("set-cookie")) { cookie = r.headers.get("set-cookie").split(";")[0]; return; }
      if (child.exitCode !== null) throw new Error(logs);
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    throw new Error("Isolated web server did not start: " + logs);
  };
  const api = async (route, body) => {
    const r = await fetch(url + "/api/library" + route, { method: body ? "POST" : "GET", headers: { Cookie: cookie, Origin: url, "Content-Type": "application/json" }, body: body && JSON.stringify(body), signal: AbortSignal.timeout(30000) });
    assert.equal(r.status, 200, await r.clone().text());
    return r.json();
  };
  try {
    await fs.writeFile(path.join(root, ".portable"), "");
    await fs.mkdir(data, { recursive: true });
    // No harness is installed in this fixture. Explicitly keep the Library
    // visible instead of using the automatic gateway-only presentation.
    await fs.writeFile(path.join(data, "settings.json"), JSON.stringify({ gatewayMode: "off" }));
    const feature = path.join(root, "feature.exe");
    await fs.copyFile(path.resolve(process.argv[2]), feature);
    await start(feature);
    for (const name of ["review-login", "代码审查"]) await api("/prompts/save", { name, text: "保留模板正文。", agents: [] });
    const before = (await api("/prompts")).items;
    assert.equal(before.length, 2);
    if (process.env.PROMPT_INDEX_UI === "1") {
      const { chromium, webkit } = require("playwright");
      for (const engine of [chromium, webkit]) {
        const browser = await engine.launch(); browsers.push(browser);
        const page = await browser.newPage({ viewport: { width: 560, height: 860 } });
        await page.addInitScript(() => localStorage.setItem("magpie.libTab", "instructions"));
        await page.goto(url + "/?view=library&k=fixture-prompt-index");
        await page.locator('button[data-view="library"]').click();
        await page.locator(".lib-prompt-row").first().waitFor();
        assert.equal(await page.locator(".lib-prompt-row").count(), 2);
        pages.push(page);
      }
    }
    await stop();
    if (process.argv[3]) {
      const legacy = path.join(root, "legacy.exe");
      await fs.copyFile(path.resolve(process.argv[3]), legacy);
      await start(legacy);
      await api("/all/sync", {}); // the actual old binary rewrites library.json
      await stop();
    } else {
      const file = path.join(data, "library.json"), old = JSON.parse(await fs.readFile(file, "utf8"));
      delete old.prompts;
      for (const a of Object.values(old.applied || {})) delete a.prompts;
      await fs.writeFile(file, JSON.stringify(old));
    }
    assert(!JSON.parse(await fs.readFile(path.join(data, "library.json"), "utf8")).prompts, "Legacy writer must actually drop template metadata");
    await start(feature);
    assert.deepEqual((await api("/prompts")).items.map(({ id, name, text }) => ({ id, name, text })), before.map(({ id, name, text }) => ({ id, name, text })));
    for (const page of pages) {
      await page.reload();
      await page.locator(".lib-prompt-row").first().waitFor();
      assert.equal(await page.locator(".lib-prompt-row").count(), 2);
      assert((await page.locator(".lib-prompts").textContent()).includes("代码审查"));
    }
    // A bad index must be an error, never a successful empty-list response.
    const good = await fs.readFile(index).catch(() => null);
    assert(good, "Independent index must exist after saving templates");
    await fs.writeFile(index, "{}");
    const bad = await fetch(url + "/api/library/prompts", { headers: { Cookie: cookie } });
    assert(bad.status >= 400);
    await fs.writeFile(index, good);
    assert.equal((await api("/prompts")).items.length, 2);
    console.log(JSON.stringify({ platform: "Windows", legacyWriter: process.argv[3] ? "real binary" : "fixture rewrite", restart: "pass", stableIdsAndText: "pass", corruptIndex: "pass", browsers: pages.length ? "Chromium and WebKit: pass" : "not requested" }));
  } finally {
    for (const browser of browsers) await browser.close();
    await stop();
    assert.equal(path.dirname(root), os.tmpdir());
    assert(path.basename(root).startsWith("magpie-prompt-index-"));
    await fs.rm(root, { recursive: true, force: true });
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
