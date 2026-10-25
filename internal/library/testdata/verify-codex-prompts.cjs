// Windows native capability acceptance. The original installation is read and
// executed unchanged; only temporary resources and a loopback Responses model
// are used. No request bodies, configuration values or native logs are printed.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const http = require("node:http");
const { createHash } = require("node:crypto");
const { spawn, execFileSync } = require("node:child_process");

async function main() {
  assert.equal(process.platform, "win32");
  const bin = path.resolve(process.argv[2] || "");
  assert.equal(path.basename(bin), "codex.exe", "Pass the installed native Codex executable");
  const originals = [bin, ...[".codex/config.toml", ".agents/skills/git-workflow/agents/openai.yaml", ".codex/skills/git-workflow/agents/openai.yaml"].map(p => path.join(process.env.USERPROFILE, p))];
  async function fingerprint() {
    return Promise.all(originals.map(async p => {
      try { return createHash("sha256").update(await fs.readFile(p)).digest("hex"); }
      catch (error) { if (error.code === "ENOENT") return "missing"; throw error; }
    }));
  }
  const before = await fingerprint();
  const root = await fs.mkdtemp(path.join(path.parse(process.cwd()).root, "magpie-codex-policy-"));
  const home = path.join(root, "home"), cwd = path.join(root, "work");
  const body = "MAGPIE_EXPLICIT_BODY_正文_7f6132";
  const tail = "MAGPIE_TRAILING_用户尾参_7f6132";
  const skill = path.join(home, "skills", "magpie-policy-probe");
  const requests = [];
  let publicAPI = "not requested";
  const server = http.createServer(async (req, res) => {
    assert.equal(req.socket.remoteAddress, "127.0.0.1");
    if (req.url !== "/v1/responses") { res.writeHead(404); return res.end("{}"); }
    let raw = "";
    for await (const chunk of req) raw += chunk;
    requests.push(JSON.parse(raw));
    res.writeHead(200, { "content-type": "text/event-stream" });
    const item = { id: "msg_fixture", type: "message", role: "assistant", status: "completed", content: [{ type: "output_text", text: "OK", annotations: [] }] };
    const response = { id: "resp_fixture", object: "response", created_at: 1, model: "fixture", status: "completed", output: [item], usage: { input_tokens: 10, output_tokens: 1, total_tokens: 11 } };
    const emit = (type, data) => res.write(`event: ${type}\ndata: ${JSON.stringify({ type, ...data })}\n\n`);
    emit("response.created", { response: { ...response, status: "in_progress", output: [] } });
    emit("response.output_item.added", { output_index: 0, item: { ...item, status: "in_progress", content: [] } });
    emit("response.content_part.added", { item_id: item.id, output_index: 0, content_index: 0, part: { type: "output_text", text: "", annotations: [] } });
    emit("response.output_text.delta", { item_id: item.id, output_index: 0, content_index: 0, delta: "OK" });
    emit("response.output_text.done", { item_id: item.id, output_index: 0, content_index: 0, text: "OK" });
    emit("response.content_part.done", { item_id: item.id, output_index: 0, content_index: 0, part: item.content[0] });
    emit("response.output_item.done", { output_index: 0, item });
    emit("response.completed", { response });
    res.end();
  });
  try {
    await fs.mkdir(cwd, { recursive: true });
    const policy = path.join(skill, "agents", "openai.yaml");
    await fs.mkdir(home, { recursive: true });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    await fs.writeFile(path.join(home, "config.toml"), `disable_response_storage = true\nmodel = "fixture"\nmodel_provider = "fixture"\napproval_policy = "never"\nsandbox_mode = "read-only"\n[model_providers.fixture]\nname = "Loopback fixture"\nbase_url = "http://127.0.0.1:${server.address().port}/v1"\nwire_api = "responses"\nrequires_openai_auth = false\n`);
    const foreign = path.join(root, ".agents", "skills", "foreign-warning");
    await fs.mkdir(path.join(foreign, "agents"), { recursive: true });
    await fs.writeFile(path.join(foreign, "SKILL.md"), "---\nname: foreign-warning\ndescription: Independent metadata warning fixture\n---\nForeign body\n");
    await fs.writeFile(path.join(foreign, "agents", "openai.yaml"), "policy:\n\tallow_implicit_invocation: false\n");
    const env = { ...process.env, CODEX_HOME: home, HOME: root, USERPROFILE: root };
    for (const key of ["OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_API_BASE", "CODEX_API_KEY"]) delete env[key];
    if (process.argv[3]) {
      const exe = path.resolve(process.argv[3]);
      const portProbe = require("node:net").createServer();
      await new Promise(resolve => portProbe.listen(0, "127.0.0.1", resolve));
      const port = portProbe.address().port;
      await new Promise(resolve => portProbe.close(resolve));
      const url = `http://127.0.0.1:${port}`;
      const web = spawn(exe, ["web", "--addr", `127.0.0.1:${port}`, "--no-open"], { cwd, env: { ...env, PATH: `${path.dirname(bin)};${path.dirname(process.execPath)};${process.env.SystemRoot}/System32`, XDG_CONFIG_HOME: path.join(root, "config"), APPDATA: path.join(root, "appdata"), LOCALAPPDATA: path.join(root, "localappdata"), MAGPIE_TEST_SANDBOX: root, MAGPIE_NO_ACCOUNTS: "1", MAGPIE_WEB_KEY: "fixture-codex-http-verification" }, windowsHide: true, stdio: "ignore" });
      const webExit = new Promise(resolve => web.on("exit", resolve));
      try {
        let cookie;
        for (let i = 0; i < 150; i++) {
          const response = await fetch(url + "/?k=fixture-codex-http-verification", { redirect: "manual", signal: AbortSignal.timeout(1000) }).catch(() => null);
          if (response) { cookie = response.headers.get("set-cookie")?.split(";")[0]; break; }
          await new Promise(resolve => setTimeout(resolve, 200));
        }
        assert(cookie, "Isolated magpie web must issue its authentication cookie");
        const api = async (action, body) => {
          const response = await fetch(url + "/api/library/prompts" + (action ? "/" + action : ""), { method: body ? "POST" : "GET", headers: { Cookie: cookie, Origin: url, "Content-Type": "application/json" }, body: body && JSON.stringify(body), signal: AbortSignal.timeout(40000) });
          assert.equal(response.status, 200, "Public prompt API must succeed");
          return response.json();
        };
        const codex = states => states.find(s => s.agent === "codex");
        assert.equal((await api("")).enabled, false, "An empty library starts with prompt templates off");
        assert.equal(codex(await api("check", { name: "magpie-policy-probe", passive: true })).status, "disabled");
        assert.equal((await api("enabled", { enabled: true })).prompts.enabled, true, "Explicitly enable templates before native verification");
        const checked = codex(await api("check", { name: "magpie-policy-probe" }));
        assert.equal(checked.status, "available");
        assert.equal(checked.reason, "Codex ignored unrelated configuration or skill metadata; this template is unaffected");
        const saved = await api("save", { name: "magpie-policy-probe", text: body, agents: ["codex"] });
        const item = saved.prompts.items.find(p => p.name === "magpie-policy-probe");
        assert(item, "Public Save must return the new template");
        assert.equal(codex(item.states).status, "enabled");
        assert((await fs.readFile(path.join(skill, "SKILL.md"), "utf8")).includes(body), "Invoke the exact public Save artifact");
        publicAPI = "disabled read/passive check -> Enable -> Check -> Save -> original CLI invocation of the same artifact";
      } finally { web.kill(); await webExit; }
    } else {
      await fs.mkdir(path.join(skill, "agents"), { recursive: true });
      await fs.writeFile(path.join(skill, "SKILL.md"), `---\nname: magpie-policy-probe\ndescription: Explicitly invoke this skill to apply these instructions to the accompanying request.\n---\n\n${body}\n`);
      await fs.writeFile(policy, "policy:\n  allow_implicit_invocation: false\n");
    }
    async function run(name, prompt) {
      requests.length = 0;
      const child = spawn(bin, ["exec", "--skip-git-repo-check", "--ephemeral", "--ignore-rules", "--json", "--color", "never", prompt], { cwd, env, windowsHide: true, stdio: ["ignore", "pipe", "pipe"] });
      let diagnostics = "", outputBytes = 0;
      child.stdout.on("data", b => { outputBytes += b.length; });
      child.stderr.on("data", b => { diagnostics += b; });
      const timer = setTimeout(() => {
        try { execFileSync("taskkill.exe", ["/PID", String(child.pid), "/T", "/F"], { windowsHide: true, stdio: "ignore" }); } catch {}
      }, 45000);
      let code;
      try { code = await new Promise((resolve, reject) => { child.on("exit", resolve); child.on("error", reject); }); }
      finally { clearTimeout(timer); }
      const input = requests.map(r => JSON.stringify(r.input)).join("\n");
      const result = { name, exit: code, requests: requests.length, bodyLoaded: input.includes(body), skillNamedInInput: input.includes("magpie-policy-probe"), tailPresent: input.includes(tail), metadataWarning: diagnostics.includes("invalid openai.yaml"), ownMetadataWarning: diagnostics.replaceAll("\\", "/").includes("/magpie-policy-probe/agents/openai.yaml"), outputBytes };
      assert.equal(code, 0, `${name}: original CLI must complete with the loopback model`);
      assert(requests.length > 0, `${name}: the local model must observe the request`);
      assert(result.tailPresent, `${name}: preserve the user instruction`);
      return result;
    }
    const explicit = await run("explicit", `$magpie-policy-probe ${tail}`);
    assert(explicit.bodyLoaded, "Explicit invocation must inject the body");
    assert(!explicit.ownMetadataWarning, "The new template's own policy must load cleanly");
    const plain = await run("plain-chat", `普通聊天：检查边界。 ${tail}`);
    assert(!plain.bodyLoaded, "Plain chat must not inject the explicit-only body");
    assert(!plain.skillNamedInInput, "The explicit-only skill must be absent from implicit discovery");
    assert(!plain.ownMetadataWarning, "The new template's independent policy must remain valid");
    await fs.writeFile(policy, "policy:\n\tallow_implicit_invocation: false\n");
    const broken = await run("broken-policy-native-control", `$magpie-policy-probe ${tail}`);
    assert(broken.ownMetadataWarning, "The original loader must diagnose this template's malformed policy");
    assert(broken.bodyLoaded, "The native default policy control must expose why magpie must block a broken own policy");
    const brokenPlain = await run("broken-policy-implicit-control", `普通聊天：检查边界。 ${tail}`);
    assert(brokenPlain.ownMetadataWarning && brokenPlain.skillNamedInInput, "A broken native policy defaults the skill into implicit discovery");
    assert(!brokenPlain.bodyLoaded, "Native implicit discovery lists metadata before a model elects to load the body");
    assert.deepEqual(await fingerprint(), before, "Selected original program and configuration fingerprints must remain unchanged");
    console.log(JSON.stringify({ probe: "original Codex CLI with temporary policy and loopback model", publicAPI, cases: [explicit, plain, broken, brokenPlain], brokenPolicyIsNativeFailOpen: broken.bodyLoaded, originalFingerprintsUnchanged: originals.length, originalConfigurationWritten: false }));
  } finally {
    server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
    assert.equal(path.dirname(root), path.parse(process.cwd()).root);
    assert(path.basename(root).startsWith("magpie-codex-policy-"));
    await fs.rm(root, { recursive: true, force: true });
  }
}
main().catch(error => { console.error(error.message); process.exitCode = 1; });
