// Opt-in Windows acceptance. Original modules, a physical temporary profile,
// and a loopback model; no installed module or protocol is replaced.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { dirname, join, resolve, isAbsolute } from 'node:path';
import { mkdtempSync, mkdirSync, writeFileSync, realpathSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { createServer } from 'node:http';
import { randomUUID, createHash } from 'node:crypto';

assert.equal(process.platform, 'win32');
const entry = realpathSync(resolve(process.argv[2]));
const sourceHome = resolve(process.argv[3]);
const deployed = process.argv[4] ? readFileSync(resolve(process.argv[4])) : undefined;
const marker = deployed ? process.argv[5] : 'MAGPIE_UNIQUE_BODY_' + randomUUID();
assert.equal(typeof marker, 'string');
assert(marker.length > 0 && (!deployed || deployed.toString('utf8').includes(marker)));
const root = mkdtempSync(join(tmpdir(), 'magpie-dsh-invocation-'));
const work = join(root, 'work');
mkdirSync(work);
const originals = [entry, join(dirname(entry), 'profile-boot.js'),
  ...['profiles/web/package.json', 'profiles/web/cordis.patch.yml', 'cordis.patch.yml'].map(p => join(sourceHome, p))];
const sourceRoots = process.argv[6] ? JSON.parse(readFileSync(resolve(process.argv[6]), 'utf8')) : [];
assert(Array.isArray(sourceRoots) && sourceRoots.every(p => typeof p === 'string' && isAbsolute(p)));
const stamp = () => {
  const seen = new Set(), records = [];
  let bytes = 0;
  function walk(path) {
    let real;
    try { real = realpathSync(path); }
    catch (e) { if (e.code === 'ENOENT') { records.push([path, 'missing']); return; } throw e; }
    records.push([path, real]); // Also detects parent junction retargeting.
    if (seen.has(real)) return;
    seen.add(real);
    assert(seen.size <= 100000, 'Source comparison must remain bounded');
    const info = statSync(real);
    if (info.isDirectory()) {
      records.push([real, 'directory', info.mode]);
      for (const name of readdirSync(real).sort()) if (name !== '.git') walk(join(real, name));
    } else {
      assert(info.isFile());
      bytes += info.size;
      assert(bytes <= 1024 * 1024 * 1024, 'Source comparison exceeds 1 GiB');
      records.push([real, info.mode, createHash('sha256').update(readFileSync(real)).digest('hex')]);
    }
  }
  for (const path of [...new Set([...originals, ...sourceRoots])].sort()) walk(path);
  return createHash('sha256').update(JSON.stringify(records)).digest('hex');
};
const before = stamp();
const write = (p, body) => { mkdirSync(dirname(p), { recursive: true }); writeFileSync(p, body); };
process.env.DSH_HOME = root;
process.env.DSH_AGENTS_HOME = join(root, 'agents');
process.env.MAGPIE_DSH_FIXTURE_KEY = 'fixture';
process.chdir(work);
const resolvers = [createRequire(join(dirname(entry), '../package.json'))];
async function native(name) {
  for (const require of resolvers) {
    let p;
    try { p = require.resolve(name); } catch { continue; }
    resolvers.push(createRequire(p));
    return import(pathToFileURL(p).href);
  }
  throw new Error('Required original native module is unavailable');
}
let received = [], requestSkill = false;
const server = createServer(async (req, res) => {
  let text = '';
  for await (const part of req) text += part;
  received.push(JSON.parse(text));
  res.writeHead(200, { 'content-type': 'text/event-stream' });
  const callSkill = requestSkill && received.length === 1;
  const response = callSkill
    ? [[{ role: 'assistant', tool_calls: [{ index: 0, id: 'load-fixture', type: 'function',
      function: { name: 'skill', arguments: JSON.stringify({ name: 'audit-code' }) } }] }, null], [{}, 'tool_calls']]
    : [[{ role: 'assistant', content: 'OK' }, null], [{}, 'stop']];
  for (const [delta, finish_reason] of response) {
    res.write(`data: ${JSON.stringify({ id: 'fixture', object: 'chat.completion.chunk', created: 1,
      model: 'fixture', choices: [{ index: 0, delta, finish_reason }] })}\n\n`);
  }
  res.end('data: [DONE]\n\n');
});
let runtime, report;
const timer = setTimeout(() => process.exit(1), 60000);
try {
  await new Promise(r => server.listen(0, '127.0.0.1', r));
  const boot = await native('@deepseek-ai/dsh-app-boot');
  const { runProfile } = await import(pathToFileURL(join(dirname(entry), 'profile-boot.js')).href);
  const profileDir = join(root, 'profiles/web');
  write(join(profileDir, 'package.json'), JSON.stringify({ dsh: { profile: { bundles: ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app'] } } }));
  const { symlinkSync } = await import('node:fs');
  symlinkSync(join(sourceHome, 'profiles/node_modules'), join(root, 'profiles/node_modules'), 'junction');
  write(join(profileDir, 'cordis.patch.yml'), `
- id: llm-pi-ai
  config:
    providers:
      magpie-fixture:
        api: openai-completions
        apiKeyEnv: MAGPIE_DSH_FIXTURE_KEY
        baseURL: http://127.0.0.1:${server.address().port}/v1
        models: [{id: fixture, maxTokens: 1000, contextWindow: 100000}]
- id: session-title-llm
  disabled: true
`);
  const profile = boot.loadProfileDirectory('dsh', profileDir, join(dirname(entry), '../package.json'));
  assert.equal(profile.skippedBundles.length, 0);
  runtime = await runProfile({ environment: boot.loadLayeredEnv('dsh'), profile: 'web',
    resolvedProfile: { profile, installAnchor: join(dirname(entry), '../package.json') }, patchFiles: [],
    args: ['--host', '127.0.0.1', '--port', '0', '--no-open'] });
  const { ctx } = runtime;
  const { createUserMessage } = await native('@deepseek-ai/dsh-llm');
  const { installModelSelection } = await native('@deepseek-ai/dsh-agent');
  const presets = await ctx.agentPresets.list();
  const preset = presets.find(p => p.isDefault) ?? presets[0];
  assert(preset && !preset.broken);
  write(join(root, 'skills/audit-code/SKILL.md'), deployed ?? `---\nname: audit-code\ndescription: Acceptance fixture\ndisable-model-invocation: true\nuser-invocable: true\n---\n${marker}\n`);
  async function run(text, modelRequestsSkill = false) {
    received = [];
    requestSkill = modelRequestsSkill;
    const handle = await ctx.agents.create({ sessionId: randomUUID(), meta: { cwd: work, agentPreset: preset.id },
      agentOptions: { provider: 'magpie-fixture', model: 'fixture' },
      setup: async agentCtx => {
        await ctx.agentPresets.mount(agentCtx, preset.id);
        installModelSelection(agentCtx, { current: { provider: 'magpie-fixture', model: 'fixture' }, assembled: undefined });
      } });
    try {
      await handle.agent.whenIdle();
      assert.equal(received.length, 0, 'Creating an idle native Agent cannot submit a model request');
      handle.agent.followup(createUserMessage({ content: [{ type: 'text', text }], source: { kind: 'user' } }));
      await handle.agent.whenIdle();
      assert(received.length > 0, 'Original Agent must contact the loopback model');
      return JSON.stringify(received);
    } finally { await handle.dispose(); }
  }
  assert(!(await run('普通聊天：检查代码边界。')).includes(marker), 'Ordinary chat cannot load an explicit-only body');
  const trailing = '用户后接指令_' + randomUUID();
  const explicit = await run('/audit-code ' + trailing);
  assert(explicit.includes('<skill_content name=\\"audit-code\\">'), 'Original host must expand the explicit gesture');
  assert(explicit.includes(marker));
  assert(explicit.includes(trailing), 'Trailing text remains a native user message');
  assert(!(await run('请自行选择合适的技能。')).includes(marker), 'Implicit discovery cannot expose the body');
  const attempted = await run('请自行加载 audit-code 技能。', true);
  assert(received.length >= 2, 'A real model tool call must reach the native rejection path');
  assert(received[0].tools?.some(tool => tool.function?.name === 'skill'), 'The native skill tool must be offered');
  assert(attempted.includes('not available for model invocation'), 'The native handler must reject the model-requested load');
  assert(!attempted.includes(marker), 'A model tool call cannot load an explicit-only body');
  write(join(root, 'model-requested-load.json'), JSON.stringify(received, null, 2));
  report = { ordinaryChat: 'pass', explicitInvocation: 'pass', trailingText: 'pass', noImplicitBody: 'pass',
    modelRequestedLoad: 'blocked',
    deployedArtifactHash: deployed ? createHash('sha256').update(deployed).digest('hex') : undefined,
    sourcesVerified: sourceRoots.length > 0,
    modelEndpoint: 'loopback only', artifactDirectory: root };
} finally {
  clearTimeout(timer);
  if (runtime) await runtime.shutdown.shutdown(0);
  await new Promise(r => server.close(r));
  assert.deepEqual(stamp(), before, 'Original installation and profile must remain unchanged');
}
process.stdout.write(JSON.stringify(report) + '\n');
