// Opt-in Windows acceptance against an installed Pi CLI entry point. No keys,
// live configuration, external model requests or additional packages are used.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const http = require("node:http");
const readline = require("node:readline");
const { createHash } = require("node:crypto");
const { spawn, execFileSync } = require("node:child_process");

async function main() {
  assert.equal(process.platform, "win32", "This acceptance procedure targets Windows");
  const cli = path.resolve(process.argv[2] || "");
  assert(cli.endsWith(".js"), "Pass the installed Pi CLI JavaScript entry point");
  assert(process.argv[3], "Pass the built magpie executable for public Save acceptance");
  const originalAgent = process.env.PI_CODING_AGENT_DIR || path.join(process.env.USERPROFILE, ".pi", "agent");
  const originals = [cli, ...["settings.json", "models.json"].map(p => path.join(originalAgent, p))];
  const fingerprint = () => Promise.all(originals.map(async p => {
    try { return createHash("sha256").update(await fs.readFile(p)).digest("hex"); }
    catch (error) { if (error.code === "ENOENT") return "missing"; throw error; }
  }));
  const originalFingerprints = await fingerprint();
  const root = await fs.mkdtemp(path.join(path.parse(process.cwd()).root, "magpie-pi-verification-"));
  const agent = path.join(root, "agent"), cwd = path.join(root, "work");
  const write = async (file, body) => { await fs.mkdir(path.dirname(file), { recursive: true }); await fs.writeFile(file, body, "utf8"); };
  const env = { SystemRoot: process.env.SystemRoot, COMSPEC: process.env.COMSPEC, PATH: `${path.dirname(process.execPath)};${process.env.SystemRoot}/System32`, HOME: root, USERPROFILE: root, PI_CODING_AGENT_DIR: agent, APPDATA: path.join(root, "appdata"), LOCALAPPDATA: path.join(root, "localappdata"), TEMP: root, TMP: root, NO_COLOR: "1", HTTP_PROXY: "http://127.0.0.1:9", HTTPS_PROXY: "http://127.0.0.1:9", NO_PROXY: "127.0.0.1,localhost" };
  let received;
  const server = http.createServer(async (req, res) => {
    assert.equal(req.socket.remoteAddress, "127.0.0.1");
    let body = "";
    for await (const part of req) body += part;
    received = JSON.parse(body);
    res.writeHead(200, { "content-type": "text/event-stream" });
    for (const [delta, finish_reason] of [[{ role: "assistant", content: "OK" }, null], [{}, "stop"]]) {
      res.write(`data: ${JSON.stringify({ id: "fixture", object: "chat.completion.chunk", created: 1, model: "fixture", choices: [{ index: 0, delta, finish_reason }] })}\n\n`);
    }
    res.end("data: [DONE]\n\n");
  });
  try {
    // Refuse an ancestor skill tree: the fixture must not load a live resource.
    for (let d = path.dirname(root); ; d = path.dirname(d)) {
      for (const relative of [".agents/skills", ".pi"]) await assert.rejects(fs.stat(path.join(d, relative)), { code: "ENOENT" });
      if (path.dirname(d) === d) break;
    }
    await fs.mkdir(cwd, { recursive: true });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    await write(path.join(agent, "settings.json"), JSON.stringify({ defaultProvider: "fixture", defaultModel: "fixture", defaultProjectTrust: "always", autoCompaction: false }));
    await write(path.join(agent, "models.json"), JSON.stringify({ providers: { fixture: { baseUrl: `http://127.0.0.1:${server.address().port}/v1`, api: "openai-completions", apiKey: "fixture", models: [{ id: "fixture", contextWindow: 100000, maxTokens: 1000 }] } } }));
    const extension = path.join(agent, "extensions", "fixture.ts");
    await write(extension, `export default pi => { pi.on("session_start", async () => {}); }`);
    await write(path.join(agent, "skills", "d", "SKILL.md"), "---\nname: d\ndescription: Fixture\n---\nFixture skill\n");
    const version = execFileSync(process.execPath, [cli, "--version"], { cwd, env, windowsHide: true, encoding: "utf8", timeout: 30000 }).trim();
    assert(version, "Record the installed version for diagnostics; test native capabilities below");
    async function run(prompt) {
      received = undefined;
      const child = spawn(process.execPath, [cli, "--mode", "rpc", "--no-session", "--offline"], { cwd, env, windowsHide: true, stdio: ["pipe", "pipe", "pipe"] });
      let stderr = "", ended = false;
      child.stderr.on("data", b => { stderr += b; assert(stderr.length < 1 << 20); });
      const exit = new Promise(resolve => child.on("exit", resolve));
      const lines = readline.createInterface({ input: child.stdout });
      const result = new Promise((resolve, reject) => {
        child.on("error", reject);
        child.on("exit", code => { if (!ended) reject(new Error(`Pi exited ${code}: ${stderr}`)); });
        lines.on("line", line => {
          try {
            const msg = JSON.parse(line);
            if (msg.type === "extension_error" || msg.type === "extension_ui_request") throw new Error(line);
            if (msg.id === "commands" && msg.type === "response") {
              assert(msg.success);
              if (!prompt) { ended = true; resolve(msg.data.commands); }
              else child.stdin.write(JSON.stringify({ id: "prompt", type: "prompt", message: prompt }) + "\n");
            }
            if (prompt && msg.type === "agent_end") { ended = true; resolve(received); }
          } catch (error) { reject(error); }
        });
      });
      const timer = setTimeout(() => child.kill(), 30000);
      try {
        child.stdin.write('{"id":"commands","type":"get_commands"}\n');
        const value = await result;
        assert.equal(stderr.trim(), "", "No partial resource loading diagnostics");
        return value;
      } finally {
        ended = true; clearTimeout(timer); child.kill(); lines.close(); await exit;
      }
    }
    let commands = await run();
    assert(commands.some(c => c.name === "skill:d" && c.source === "skill"));
    assert(!commands.some(c => c.name === "d"), "An event-only extension does not register /d");
    await write(extension, `export default pi => { pi.registerCommand("d", { description: "Fixture", handler: async () => {} }); }`);
    commands = await run();
    assert(commands.some(c => c.name === "d" && c.source === "extension" && c.sourceInfo.path));
    await write(extension, `export default pi => {}`);
    const initialBody = "MAGPIE_PI_INITIAL_BODY_初稿_7f6132", templateBody = "MAGPIE_PI_PUBLIC_BODY_编辑后正文_7f6132", tail = "MAGPIE_PI_TAIL_用户后接指令_7f6132";
    let httpAPI = "not requested", publicSaveArtifactSHA256;
    if (process.argv[3]) {
      const exe = path.resolve(process.argv[3]), bin = path.join(root, "bin");
      for (const file of [process.execPath, cli]) assert(!/["%!\r\n]/.test(file), "Unsupported batch path");
      await write(path.join(bin, "pi.cmd"), `@echo off\r\n"${process.execPath}" "${cli}" %*\r\n`);
      const starts = path.join(root, "starts.txt");
      const observer = `import { appendFileSync } from "node:fs"; export default pi => { pi.on("session_start", () => { appendFileSync(${JSON.stringify(starts)}, "start\\n"); }); };`;
      await write(extension, observer);
      const counter = async () => fs.readFile(starts, "utf8").catch(() => "");
      const portProbe = require("node:net").createServer();
      await new Promise(resolve => portProbe.listen(0, "127.0.0.1", resolve));
      const port = portProbe.address().port;
      await new Promise(resolve => portProbe.close(resolve));
      const url = `http://127.0.0.1:${port}`;
      const web = spawn(exe, ["web", "--addr", `127.0.0.1:${port}`, "--no-open"], { cwd, env: { ...env, PATH: `${bin};${path.dirname(process.execPath)};${process.env.SystemRoot}/System32`, XDG_CONFIG_HOME: path.join(root, "config"), APPDATA: path.join(root, "appdata"), LOCALAPPDATA: path.join(root, "localappdata"), MAGPIE_TEST_SANDBOX: root, MAGPIE_NO_ACCOUNTS: "1", MAGPIE_WEB_KEY: "fixture-pi-http-verification" }, windowsHide: true, stdio: "ignore" });
      const webExit = new Promise(resolve => web.on("exit", resolve));
      try {
        let cookie;
        for (let i = 0; i < 150; i++) {
          const response = await fetch(url + "/?k=fixture-pi-http-verification", { redirect: "manual", signal: AbortSignal.timeout(1000) }).catch(() => null);
          if (response) { cookie = response.headers.get("set-cookie")?.split(";")[0]; break; }
          await new Promise(resolve => setTimeout(resolve, 200));
        }
        assert(cookie, "Isolated magpie web started and issued its authentication cookie");
        const api = async (action, body) => {
          const response = await fetch(url + "/api/library/prompts" + (action ? "/" + action : ""), { method: body ? "POST" : "GET", headers: { Cookie: cookie, Origin: url, "Content-Type": "application/json" }, body: body && JSON.stringify(body), signal: AbortSignal.timeout(40000) });
          assert.equal(response.status, 200, await response.clone().text());
          return response.json();
        };
        const pi = states => states.find(s => s.agent === "pi");
        assert.equal((await api("")).enabled, false, "An empty library starts with prompt templates off");
        assert.equal(pi(await api("check", { name: "api-review", passive: true })).status, "disabled");
        assert.equal(await counter(), "", "Passive HTTP check did not start Pi");
        assert.equal((await api("enabled", { enabled: true })).prompts.enabled, true, "Explicitly enable templates before native verification");
        assert.equal(pi(await api("check", { name: "api-review" })).status, "available");
        const before = await counter();
        assert(before, "Explicit HTTP check loaded real Pi");
        const saved = await api("save", { name: "api-review", text: initialBody, agents: ["pi"] });
        const item = saved.prompts.items.find(p => p.name === "api-review");
        assert.equal(pi(item.states).status, "enabled");
        const deployedFile = path.join(agent, "prompts", "api-review.md");
        assert((await fs.readFile(deployedFile, "utf8")).includes(initialBody), "Initial public Save deploys its body");
        const edited = await api("save", { id: item.id, name: item.name, text: templateBody, agents: ["pi"] });
        const editedItem = edited.prompts.items.find(p => p.id === item.id);
        assert(editedItem); assert.equal(editedItem.text, templateBody); assert.equal(pi(editedItem.states).status, "enabled");
        await api("");
        assert.equal(await counter(), before, "Saving and reading did not start Pi");
        const deployed = await fs.readFile(deployedFile);
        publicSaveArtifactSHA256 = createHash("sha256").update(deployed).digest("hex");
        assert(deployed.toString("utf8").includes(templateBody) && !deployed.toString("utf8").includes(initialBody), "Editing through public Save replaces the same artifact with the current body");
        const disabled = await api("harness", { target: "pi", enabled: false });
        assert.deepEqual(disabled.result.problems.filter(p => p.agent === "pi"), [], "Batch disable has no Pi problems");
        const disabledItem = disabled.prompts.items.find(p => p.id === item.id);
        assert(disabledItem && !disabledItem.agents.includes("pi"), "Batch disable removes the current template's Pi selection");
        await assert.rejects(fs.readFile(deployedFile), { code: "ENOENT" }, "Batch disable retires the unchanged owned artifact");
        assert.equal(await counter(), before, "Batch disable did not start Pi");
        const restored = await api("harness", { target: "pi", enabled: true });
        assert.deepEqual(restored.result.problems.filter(p => p.agent === "pi"), [], "Batch enable has no Pi problems");
        const restoredItem = restored.prompts.items.find(p => p.id === item.id);
        assert(restoredItem && restoredItem.agents.includes("pi"), "Batch enable restores the same template's Pi selection");
        assert.equal(restoredItem.text, templateBody, "Batch enable preserves the edited Library body");
        assert.equal(pi(restoredItem.states).status, "enabled");
        assert.equal(pi(restoredItem.states).deployed, true, "Batch enable actually deploys the template");
        assert.deepEqual(await fs.readFile(deployedFile), deployed, "Batch enable restores the exact public Save/Edit artifact before native invocation");
        const explicit = await run(`/api-review ${tail}`);
        const explicitInput = JSON.stringify(explicit?.messages);
        assert(explicitInput.includes(templateBody), "Original Pi expands the public Save artifact into the model request");
        assert(explicitInput.includes(tail), "Original Pi preserves the user instruction after the template name");
        assert(!explicitInput.includes("$ARGUMENTS"), "Original Pi substitutes the native argument marker");
        assert(!explicitInput.includes(initialBody), "Native invocation does not inject the replaced initial body");
        const plain = await run(`普通聊天 ${tail}`);
        const plainInput = JSON.stringify(plain?.messages);
        assert(plainInput.includes(tail) && !plainInput.includes(templateBody) && !plainInput.includes(initialBody), "Ordinary chat preserves the request without injecting either version of the template body");
        assert.deepEqual(await fs.readFile(deployedFile), deployed, "Native invocation leaves the public Save artifact unchanged");
        const afterInvocations = await counter();
        await write(extension, observer + "\n// resource changed\n");
        const change = { id: item.id, name: item.name, text: "更新正文", agents: ["pi"], passive: true };
        assert.equal(pi(await api("check", change)).status, "unverified");
        await api("save", change);
        assert.equal(await fs.readFile(deployedFile, "utf8"), templateBody + "\n\n$ARGUMENTS\n");
        assert.equal(await counter(), afterInvocations, "Expired verification did not launch Pi or replace its deployment");
        await write(extension, observer + '\nexport const fixture = 1;\n');
        await write(path.join(agent, "extensions", "conflict.ts"), 'export default pi => { pi.registerCommand("api-conflict", { description: "Fixture", handler: async () => {} }); }');
        assert.equal(pi(await api("check", { name: "api-conflict" })).status, "conflict");
        httpAPI = "pass";
      } finally { web.kill(); await webExit; }
    }
    assert.deepEqual(await fingerprint(), originalFingerprints, "Original Pi entry point and configuration fingerprints remain unchanged");
    console.log(JSON.stringify({ version, eventOnlyExtension: "pass", registeredCommand: "pass", skillNamespace: "pass", publicAPI: "disabled -> Enable -> Check -> Save -> Edit Save -> batch disable -> batch enable -> original Pi invocation", publicSaveArtifact: "pass", publicSaveArtifactSHA256, batchRestoreSameArtifact: "pass", batchPiProblems: 0, explicitTemplate: "pass", ordinaryChatNegativeControl: "pass", httpAPI, originalFingerprintsUnchanged: originals.length, originalConfigurationWritten: false, modelEndpoint: "loopback fixture only" }));
  } finally {
    await new Promise(resolve => server.close(resolve));
    assert.equal(path.dirname(root), path.parse(process.cwd()).root);
    assert(path.basename(root).startsWith("magpie-pi-verification-"));
    await fs.rm(root, { recursive: true, force: true });
  }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
