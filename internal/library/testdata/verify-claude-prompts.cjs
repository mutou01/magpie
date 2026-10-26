// Usage: node verify-claude-prompts.cjs <installed-claude.exe> <built-magpie.exe>
// Public Save -> the original CLI, using isolated homes and a loopback model.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const http = require("node:http");
const { createHash } = require("node:crypto");
const { spawn, execFileSync } = require("node:child_process");

async function main() {
  assert.equal(process.platform, "win32");
  assert(process.argv[2] && process.argv[3], "Supply the installed Claude CLI and built magpie executable");
  const cli = path.resolve(process.argv[2]), exe = path.resolve(process.argv[3]);
  assert.equal(path.basename(cli), "claude.exe");
  const originalHome = process.env.USERPROFILE;
  const originals = [cli, path.join(originalHome, ".claude.json"), path.join(process.env.CLAUDE_CONFIG_DIR || path.join(originalHome, ".claude"), "settings.json")];
  const fingerprint = () => Promise.all(originals.map(async p => {
    try { return createHash("sha256").update(await fs.readFile(p)).digest("hex"); }
    catch (error) { if (error.code === "ENOENT") return "missing"; throw error; }
  }));
  const before = await fingerprint();
  const root = await fs.mkdtemp(path.join(path.parse(process.cwd()).root, "magpie-claude-verification-"));
  const home = path.join(root, ".claude"), cwd = path.join(root, "work");
  const initialBody = "MAGPIE_CLAUDE_INITIAL_BODY_初稿_7f6132", templateBody = "MAGPIE_CLAUDE_PUBLIC_BODY_编辑后检查边界_7f6132", tail = "MAGPIE_CLAUDE_TAIL_用户后接指令_7f6132";
  const requests = [];
  const model = http.createServer(async (req, res) => {
    assert.equal(req.socket.remoteAddress, "127.0.0.1");
    let raw = "";
    for await (const chunk of req) raw += chunk;
    if (req.url.startsWith("/v1/messages/count_tokens")) { res.writeHead(200, { "content-type": "application/json" }); return res.end('{"input_tokens":10}'); }
    if (!req.url.startsWith("/v1/messages")) { res.writeHead(404); return res.end("{}"); }
    const body = JSON.parse(raw); requests.push(body);
    const message = { id: "msg_fixture", type: "message", role: "assistant", model: body.model, content: [{ type: "text", text: "OK" }], stop_reason: "end_turn", stop_sequence: null, usage: { input_tokens: 10, output_tokens: 1 } };
    if (!body.stream) { res.writeHead(200, { "content-type": "application/json" }); return res.end(JSON.stringify(message)); }
    res.writeHead(200, { "content-type": "text/event-stream" });
    const emit = (type, data) => res.write(`event: ${type}\ndata: ${JSON.stringify({ type, ...data })}\n\n`);
    emit("message_start", { message: { ...message, content: [], stop_reason: null, usage: { input_tokens: 10, output_tokens: 0 } } });
    emit("content_block_start", { index: 0, content_block: { type: "text", text: "" } });
    emit("content_block_delta", { index: 0, delta: { type: "text_delta", text: "OK" } });
    emit("content_block_stop", { index: 0 });
    emit("message_delta", { delta: { stop_reason: "end_turn", stop_sequence: null }, usage: { output_tokens: 1 } });
    emit("message_stop", {}); res.end();
  });
  try {
    for (let d = path.dirname(root); ; d = path.dirname(d)) {
      for (const relative of [".claude", "CLAUDE.md", "CLAUDE.local.md", ".mcp.json", ".agents/skills"]) {
        await assert.rejects(fs.stat(path.join(d, relative)), { code: "ENOENT" });
      }
      if (path.dirname(d) === d) break;
    }
    await fs.mkdir(cwd, { recursive: true }); await fs.mkdir(home, { recursive: true });
    await new Promise(resolve => model.listen(0, "127.0.0.1", resolve));
    const env = { SystemRoot: process.env.SystemRoot, COMSPEC: process.env.COMSPEC, PATH: `${path.dirname(cli)};${path.dirname(process.execPath)};${process.env.SystemRoot}/System32`, HOME: root, USERPROFILE: root, CLAUDE_CONFIG_DIR: home, APPDATA: path.join(root, "appdata"), LOCALAPPDATA: path.join(root, "localappdata"), XDG_CONFIG_HOME: path.join(root, "config"), TEMP: root, TMP: root, ANTHROPIC_API_KEY: "fixture-loopback-only", ANTHROPIC_BASE_URL: `http://127.0.0.1:${model.address().port}`, CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1", DISABLE_AUTOUPDATER: "1", CLAUDE_CODE_DISABLE_AUTO_MEMORY: "1", HTTP_PROXY: "http://127.0.0.1:9", HTTPS_PROXY: "http://127.0.0.1:9", NO_PROXY: "127.0.0.1,localhost" };
    await fs.writeFile(path.join(home, "settings.json"), JSON.stringify({ env: { ANTHROPIC_BASE_URL: env.ANTHROPIC_BASE_URL }, permissions: { defaultMode: "default" } }));
    await fs.writeFile(path.join(root, ".claude.json"), JSON.stringify({ hasCompletedOnboarding: true }));
    const probe = require("node:net").createServer();
    await new Promise(resolve => probe.listen(0, "127.0.0.1", resolve));
    const port = probe.address().port; await new Promise(resolve => probe.close(resolve));
    const url = `http://127.0.0.1:${port}`;
    const web = spawn(exe, ["web", "--addr", `127.0.0.1:${port}`, "--no-open"], { cwd, env: { ...env, MAGPIE_TEST_SANDBOX: root, MAGPIE_NO_ACCOUNTS: "1", MAGPIE_WEB_KEY: "fixture-claude-public-save" }, windowsHide: true, stdio: "ignore" });
    const webExit = new Promise(resolve => { web.once("exit", resolve); web.once("error", resolve); });
    let deployedFile, deployed;
    try {
      let cookie;
      for (let i = 0; i < 150; i++) {
        const response = await fetch(url + "/?k=fixture-claude-public-save", { redirect: "manual", signal: AbortSignal.timeout(1000) }).catch(() => null);
        if (response) { cookie = response.headers.get("set-cookie")?.split(";")[0]; break; }
        await new Promise(resolve => setTimeout(resolve, 200));
      }
      assert(cookie, "Isolated magpie issues its authentication cookie");
      const api = async (action, body) => {
        const response = await fetch(url + "/api/library/prompts" + (action ? "/" + action : ""), { method: body ? "POST" : "GET", headers: { Cookie: cookie, Origin: url, "Content-Type": "application/json" }, body: body && JSON.stringify(body), signal: AbortSignal.timeout(40000) });
        assert.equal(response.status, 200, "Public prompt API succeeds"); return response.json();
      };
      const state = states => states.find(s => s.agent === "claude");
      const deploymentDiagnostic = async (stage, response, item) => JSON.stringify({ stage,
        selected: item?.agents?.includes("claude") ?? false,
        status: state(item?.states || [])?.status, deployed: state(item?.states || [])?.deployed ?? false,
        reason: state(item?.states || [])?.reason || "",
        artifact: await (async () => {
          const file = path.join(home, "commands", "public-save-probe.md");
          try {
            const info = await fs.stat(file), bytes = await fs.readFile(file, "utf8");
            return { exists: true, mode: info.mode & 0o777, links: info.nlink,
              initialBody: bytes.includes(initialBody), editedBody: bytes.includes(templateBody) };
          } catch (error) { return { exists: false, code: error.code }; }
        })(),
        problems: (response.result?.problems || []).map(p => ({ agent: p.agent,
          what: p.what?.startsWith("prompt:") ? "prompt" : "other",
          operation: /^(open|rename|link|mkdir|remove|CreateFile|chmod|read|write)\b/i.exec(p.error || "")?.[1] || "unknown",
          pathRole: (p.error || "").includes(path.join(home, "commands")) ? "deployed-command" : "other",
          category: /access.*denied|permission/i.test(p.error) ? "permission" : /used by another process|sharing violation/i.test(p.error) ? "sharing" : /no such file|cannot find/i.test(p.error) ? "missing" : /externally edited|user-owned/i.test(p.error) ? "protected" : "other" })) });
      assert.equal((await api("")).enabled, false);
      assert.equal(state(await api("check", { name: "public-save-probe", target: "claude", passive: true })).status, "disabled");
      assert.equal((await api("enabled", { enabled: true })).prompts.enabled, true);
      assert.equal(state(await api("check", { name: "public-save-probe", target: "claude", force: true })).status, "available");
      const saved = await api("save", { name: "public-save-probe", text: initialBody, agents: ["claude"] });
      const item = saved.prompts.items.find(p => p.name === "public-save-probe");
      assert(item); assert.equal(state(item.states).status, "enabled", state(item.states).status === "enabled" ? undefined : await deploymentDiagnostic("initial-save", saved, item));
      deployedFile = path.join(home, "commands", "public-save-probe.md");
      assert((await fs.readFile(deployedFile, "utf8")).includes(initialBody), "Initial public Save deploys its body");
      const edited = await api("save", { id: item.id, name: item.name, text: templateBody, agents: ["claude"] });
      const editedItem = edited.prompts.items.find(p => p.id === item.id);
      assert(editedItem); assert.equal(editedItem.text, templateBody); assert.equal(state(editedItem.states).status, "enabled", state(editedItem.states).status === "enabled" ? undefined : await deploymentDiagnostic("edit-save", edited, editedItem));
      deployed = await fs.readFile(deployedFile);
      assert(deployed.toString("utf8").includes(templateBody) && !deployed.toString("utf8").includes(initialBody), "Editing through public Save replaces the same artifact with the current body");
    } finally { web.kill(); await webExit; }
    async function run(name, prompt) {
      requests.length = 0;
      const child = spawn(cli, ["--print", "--output-format", "json", "--no-session-persistence", "--model", "claude-sonnet-4-5-20250929", "--tools", "", "--strict-mcp-config", "--setting-sources", "user", "--max-turns", "1", prompt], { cwd, env, windowsHide: true, stdio: ["ignore", "pipe", "pipe"] });
      let outputBytes = 0;
      child.stdout.on("data", b => { outputBytes += b.length; }); child.stderr.on("data", () => {});
      const timer = setTimeout(() => { try { execFileSync("taskkill.exe", ["/PID", String(child.pid), "/T", "/F"], { windowsHide: true, stdio: "ignore" }); } catch {} }, 45000);
      let code;
      try { code = await new Promise((resolve, reject) => { child.once("exit", resolve); child.once("error", reject); }); }
      finally { clearTimeout(timer); }
      const input = requests.map(r => JSON.stringify(r.messages)).join("\n");
      assert.equal(code, 0, `${name}: original Claude completes with loopback model`);
      assert(requests.length > 0 && input.includes(tail), `${name}: local model sees the user instruction`);
      assert(!input.includes(initialBody), `${name}: the replaced initial body is never injected`);
      return { name, exit: code, requests: requests.length, bodyLoaded: input.includes(templateBody), tailPresent: input.includes(tail), outputBytes };
    }
    const explicit = await run("public-save-explicit", `/public-save-probe ${tail}`);
    assert(explicit.bodyLoaded, "Original Claude expands the public Save artifact");
    const plain = await run("ordinary-chat", `普通聊天 ${tail}`);
    assert(!plain.bodyLoaded, "Ordinary chat does not inject the template body");
    assert.deepEqual(await fs.readFile(deployedFile), deployed, "Invocation leaves the public Save artifact unchanged");
    assert.deepEqual(await fingerprint(), before, "Original program and configuration fingerprints remain unchanged");
    console.log(JSON.stringify({ publicAPI: "disabled -> Enable -> Check -> Save -> Edit Save -> original Claude invocation", cases: [explicit, plain], publicSaveArtifactSHA256: createHash("sha256").update(deployed).digest("hex"), originalFingerprintsUnchanged: originals.length, originalConfigurationWritten: false, modelEndpoint: "loopback fixture only" }));
  } finally {
    model.closeAllConnections(); await new Promise(resolve => model.close(resolve));
    assert.equal(path.dirname(root), path.parse(process.cwd()).root); assert(path.basename(root).startsWith("magpie-claude-verification-"));
    await fs.rm(root, { recursive: true, force: true });
  }
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
