// Real HTTP timing of an installed magpie binary. No model or mock delay.
// Usage: node measure-prompt-http.cjs <magpie.exe> <claude.exe|codex.exe> [samples=7]
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const net = require("node:net");
const { performance } = require("node:perf_hooks");
const { createHash } = require("node:crypto");
const { spawn } = require("node:child_process");

(async () => {
  assert.equal(process.platform, "win32");
  assert(process.argv[2] && process.argv[3], "Supply original magpie and Claude executables");
  const executable = path.resolve(process.argv[2]);
  const harness = path.resolve(process.argv[3]);
  const target = path.basename(harness).toLowerCase() === "codex.exe" ? "codex" : "claude";
  await fs.access(executable); await fs.access(harness);
  const samples = Number(process.argv[4] || 7);
  assert(Number.isInteger(samples) && samples >= 3 && samples <= 50);
  const counts = (process.env.MAGPIE_TIMING_COUNTS || "1,16").split(",").map(Number);
  const sources = Number(process.env.MAGPIE_TIMING_SOURCES || 128);
  assert(counts.length === 2 && counts[0] === 1 && counts[1] > 1 && counts[1] <= 32);
  assert(Number.isInteger(sources) && sources > 0 && sources <= 512);
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "magpie-prompt-http-timing-"));
  const feature = path.join(root, "magpie.exe");
  await fs.copyFile(executable, feature);
  await fs.writeFile(path.join(root, ".portable"), "");
  await fs.mkdir(path.join(root, "data"));
  await fs.writeFile(path.join(root, "data", "settings.json"), JSON.stringify({ gatewayMode: "off" }));
  const commands = path.join(root, target === "claude" ? ".claude" : ".codex", target === "claude" ? "commands" : "skills");
  await fs.mkdir(commands, { recursive: true });
  if (target === "codex") await fs.writeFile(path.join(root, ".codex", "config.toml"), "disable_response_storage = true\n");
  const env = { SystemRoot: process.env.SystemRoot, COMSPEC: process.env.COMSPEC,
    PATH: `${path.dirname(harness)};${process.env.SystemRoot}/System32`, HOME: root, USERPROFILE: root,
    CODEX_HOME: path.join(root, ".codex"),
    APPDATA: root, LOCALAPPDATA: root, XDG_CONFIG_HOME: root, XDG_CACHE_HOME: root,
    XDG_DATA_HOME: root, XDG_STATE_HOME: root, TEMP: root, TMP: root,
    MAGPIE_TEST_SANDBOX: root, MAGPIE_NO_ACCOUNTS: "1", MAGPIE_WEB_KEY: "fixture-prompt-timing",
    HTTP_PROXY: "http://127.0.0.1:9", HTTPS_PROXY: "http://127.0.0.1:9", NO_PROXY: "127.0.0.1,localhost" };
  let current, url, cookie;
  const stop = async () => {
    if (!current) return;
    if (current.child.exitCode === null) current.child.kill();
    await current.exit; current = null;
  };
  const start = async () => {
    const probe = net.createServer();
    await new Promise(r => probe.listen(0, "127.0.0.1", r));
    const port = probe.address().port;
    await new Promise(r => probe.close(r));
    url = `http://127.0.0.1:${port}`;
    const child = spawn(feature, ["web", "--addr", `127.0.0.1:${port}`, "--no-open"], { cwd: root, env, windowsHide: true, stdio: "ignore" });
    current = { child, exit: new Promise(r => { child.once("exit", r); child.once("error", r); }) };
    for (let i = 0; i < 150; i++) {
      const r = await fetch(url + "/?k=fixture-prompt-timing", { redirect: "manual", signal: AbortSignal.timeout(1000) }).catch(() => null);
      if (r?.headers.get("set-cookie")) { cookie = r.headers.get("set-cookie").split(";")[0]; return; }
      assert.equal(child.exitCode, null, "Isolated HTTP process exited");
      await new Promise(r => setTimeout(r, 100));
    }
    throw Error("Isolated HTTP process did not become ready");
  };
  const api = async (action, body) => {
    const begin = performance.now();
    const response = await fetch(url + "/api/library/prompts" + (action ? "/" + action : ""), {
      method: body ? "POST" : "GET", headers: { Cookie: cookie, Origin: url, "Content-Type": "application/json" },
      body: body && JSON.stringify(body), signal: AbortSignal.timeout(90000) });
    assert.equal(response.status, 200, `Public ${action || "read"} failed`);
    const value = await response.json();
    const ms = performance.now() - begin;
    await fs.appendFile(path.join(root, "requests.jsonl"), JSON.stringify({ action: action || "read", ms, templates: value.prompts?.items.length ?? value.items?.length }) + "\n");
    return { value, ms };
  };
  const results = [];
  try {
    await start();
    await api("enabled", { enabled: true });
    assert((await api("")).value.targets.some(t => t.id === target && t.detected), "Original Harness must be detected");
    for (let i = 0; i < sources; i++) {
      if (target === "claude") await fs.writeFile(path.join(commands, `foreign-source-${i}.md`), `Unrelated command ${i}\n`);
      else {
        const dir = path.join(commands, `foreign-source-${i}`);
        await fs.mkdir(dir);
        await fs.writeFile(path.join(dir, "SKILL.md"), `---\nname: foreign-source-${i}\ndescription: Unrelated native source\n---\nUnrelated body\n`);
      }
    }
    const initial = await api("check", { name: "timing-template-0", target, force: true });
    assert.equal(initial.value.find(s => s.agent === target).status, "available");
    for (const count of counts) {
      let view = (await api("")).value;
      for (let i = view.items.length; i < count; i++) await api("save", { name: `timing-template-${i}`, text: "Initial body", agents: [target] });
      view = (await api("")).value;
      const selected = view.items.find(p => p.name === "timing-template-0");
      assert(selected);
      for (let round = 0; round <= samples; round++) {
        if (round === 0) { await stop(); await start(); }
        const body = `Edited body ${count} ${round}`;
        if (round === 0 && target === "codex") {
          const restarted = await api("");
          const state = restarted.value.items.find(p => p.id === selected.id).states.find(s => s.agent === target);
          assert.equal(state.status, "unverified", "Restart must not manufacture runtime evidence");
          if (process.env.MAGPIE_ASSERT_DEPLOYED === "1") assert.equal(state.deployed, true, "Restart must preserve the intact deployment fact");
        }
        const check = await api("check", { id: selected.id, name: selected.name, text: body, target, passive: round !== 0, force: round === 0 });
        const saved = await api("save", { id: selected.id, name: selected.name, text: body, agents: [target] });
        const read = await api("");
        const item = read.value.items.find(p => p.id === selected.id);
        assert.equal(item.text, body);
        assert.equal(item.states.find(s => s.agent === target).status, "enabled");
        const artifact = target === "claude" ? path.join(commands, selected.name + ".md") : path.join(commands, selected.name, "SKILL.md");
        assert((await fs.readFile(artifact, "utf8")).includes(body), "Timed save must publish the edited body");
        results.push({ templates: count, unrelatedSources: sources, targets: [target], state: round === 0 ? "process-cold" : "warm", round,
          checkMs: check.ms, saveMs: saved.ms, readMs: read.ms, totalMs: check.ms + saved.ms + read.ms, requests: 3 });
        await fs.appendFile(path.join(root, "samples.jsonl"), JSON.stringify(results.at(-1)) + "\n");
      }
    }
    const summary = [];
    for (const templates of counts) {
      const rows = results.filter(r => r.templates === templates && r.state === "warm");
      const stats = key => { const values = rows.map(r => r[key]).sort((a, b) => a - b); return { median: values[Math.floor(values.length / 2)], p95: values[Math.ceil(values.length * .95) - 1] }; };
      summary.push({ templates, checkMs: stats("checkMs"), saveMs: stats("saveMs"), readMs: stats("readMs"), totalMs: stats("totalMs") });
    }
    const evidence = { executable, harness, sha256: createHash("sha256").update(await fs.readFile(executable)).digest("hex"), scope: `real HTTP; isolated ${target}-only deployment; no model call; OS filesystem cache not flushed`, root, results, summary };
    const log = path.join(root, "timing.json");
    await fs.writeFile(log, JSON.stringify(evidence, null, 2));
    console.log(JSON.stringify({ log, sha256: evidence.sha256, summary, processCold: results.filter(r => r.state === "process-cold") }));
  } finally { await stop(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
