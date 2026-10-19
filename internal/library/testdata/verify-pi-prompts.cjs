// Opt-in Windows acceptance against an installed Pi CLI entry point. No keys,
// live configuration, external model requests or additional packages are used.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const http = require("node:http");
const readline = require("node:readline");
const { spawn, execFileSync } = require("node:child_process");

async function main() {
  assert.equal(process.platform, "win32", "This acceptance procedure targets Windows");
  const cli = path.resolve(process.argv[2] || "");
  assert(cli.endsWith(".js"), "Pass the installed Pi CLI JavaScript entry point");
  const root = await fs.mkdtemp(path.join(path.parse(process.cwd()).root, "magpie-pi-verification-"));
  const agent = path.join(root, "agent"), cwd = path.join(root, "work");
  const write = async (file, body) => { await fs.mkdir(path.dirname(file), { recursive: true }); await fs.writeFile(file, body, "utf8"); };
  const env = { ...process.env, HOME: root, USERPROFILE: root, PI_CODING_AGENT_DIR: agent, NO_COLOR: "1" };
  let received;
  const server = http.createServer(async (req, res) => {
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
      await assert.rejects(fs.stat(path.join(d, ".agents", "skills")), { code: "ENOENT" });
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
    assert.equal(version, "0.87.1", "Re-verify the protocol before accepting another Pi version");
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
    for (const name of ["review", "审查"]) {
      await write(path.join(agent, "prompts", name + ".md"), "检查边界条件。\n\n$ARGUMENTS\n");
      const body = await run(`/${name} 用户后接指令`);
      const message = body?.messages?.findLast(m => m.role === "user");
      assert(message, "Pi sent a native template request to the local fixture model");
      assert(JSON.stringify(message.content).includes("检查边界条件。"));
      assert(JSON.stringify(message.content).includes("用户后接指令"));
      assert(!JSON.stringify(message.content).includes("$ARGUMENTS"));
    }
    let httpAPI = "not requested";
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
        assert.equal(pi(await api("check", { name: "api-review", passive: true })).status, "unverified");
        assert.equal(await counter(), "", "Passive HTTP check did not start Pi");
        assert.equal(pi(await api("check", { name: "api-review" })).status, "available");
        const before = await counter();
        assert(before, "Explicit HTTP check loaded real Pi");
        const saved = await api("save", { name: "api-review", text: "原始正文", agents: ["pi"] });
        const item = saved.prompts.items.find(p => p.name === "api-review");
        assert.equal(pi(item.states).status, "enabled");
        await api("");
        assert.equal(await counter(), before, "Saving and reading did not start Pi");
        await write(extension, observer + "\n// resource changed\n");
        const change = { id: item.id, name: item.name, text: "更新正文", agents: ["pi"], passive: true };
        assert.equal(pi(await api("check", change)).status, "unverified");
        await api("save", change);
        assert.equal(await fs.readFile(path.join(agent, "prompts", "api-review.md"), "utf8"), "原始正文\n\n$ARGUMENTS\n");
        assert.equal(await counter(), before, "Expired verification did not launch Pi or replace its deployment");
        await write(extension, observer + '\nexport const fixture = 1;\n');
        await write(path.join(agent, "extensions", "conflict.ts"), 'export default pi => { pi.registerCommand("api-conflict", { description: "Fixture", handler: async () => {} }); }');
        assert.equal(pi(await api("check", { name: "api-conflict" })).status, "conflict");
        httpAPI = "pass";
      } finally { web.kill(); await webExit; }
    }
    console.log(JSON.stringify({ version, eventOnlyExtension: "pass", registeredCommand: "pass", skillNamespace: "pass", englishTemplate: "pass", chineseTemplate: "pass", httpAPI, modelEndpoint: "loopback fixture only" }));
  } finally {
    await new Promise(resolve => server.close(resolve));
    assert.equal(path.dirname(root), path.parse(process.cwd()).root);
    assert(path.basename(root).startsWith("magpie-pi-verification-"));
    await fs.rm(root, { recursive: true, force: true });
  }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
