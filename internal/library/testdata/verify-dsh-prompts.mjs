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
const preserveProfile = process.argv[7] === '--preserve-profile';
const replacedBody = process.argv[8];
const expectedOptionalFailure = process.argv[9] ?? 'dsh-at-file: @deepseek-ai/dsh-settings (settingsNamespace)';
const deployed = process.argv[4] ? readFileSync(resolve(process.argv[4])) : undefined;
const marker = deployed ? process.argv[5] : 'MAGPIE_UNIQUE_BODY_' + randomUUID();
assert.equal(typeof marker, 'string');
assert(marker.length > 0 && (!deployed || deployed.toString('utf8').includes(marker)));
const evidence = mkdtempSync(join(tmpdir(), 'magpie-dsh-invocation-'));
// Configured acceptance invokes the same isolated home that Check and Save
// accepted; only the model overlay differs, and it is an explicit last layer.
const root = preserveProfile ? sourceHome : evidence;
const work = preserveProfile ? process.cwd() : join(root, 'work');
if (!preserveProfile) mkdirSync(work);
const originals = [entry, join(dirname(entry), 'profile-boot.js'),
  ...['profiles/web/package.json', 'profiles/web/compatibility.json', 'profiles/web/cordis.patch.yml', 'cordis.patch.yml', '.env'].map(p => join(sourceHome, p))];
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
if (!preserveProfile) process.env.DSH_AGENTS_HOME = join(root, 'agents');
process.env.MAGPIE_DSH_FIXTURE_KEY = 'fixture';
process.chdir(work);
const resolvers = [createRequire(join(dirname(entry), '../package.json'))];
const nativePaths = new Map();
async function native(name) {
  for (const require of resolvers) {
    let p;
    try { p = require.resolve(name); } catch { continue; }
    resolvers.push(createRequire(p));
    nativePaths.set(name, realpathSync(p));
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
  const offered = received.at(-1).tools ?? [];
  const ptc = !offered.some(tool => tool.function?.name === 'skill')
    && offered.some(tool => tool.function?.name === 'run_code');
  const response = callSkill
    ? [[{ role: 'assistant', tool_calls: [{ index: 0, id: 'load-fixture', type: 'function',
      function: ptc
        ? { name: 'run_code', arguments: JSON.stringify({
          code: "try { console.log(await tools.skill({ name: 'audit-code' })); } catch (error) { console.log(String(error)); }",
          description: 'Attempt the native explicit-only skill policy check',
        }) }
        : { name: 'skill', arguments: JSON.stringify({ name: 'audit-code' }) } }] }, null], [{}, 'tool_calls']]
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
  if (!preserveProfile) {
    write(join(profileDir, 'package.json'), JSON.stringify({ dsh: { profile: { bundles: ['@deepseek-ai/dsh-base', '@deepseek-ai/dsh-web-app'] } } }));
    const { symlinkSync } = await import('node:fs');
    symlinkSync(join(sourceHome, 'profiles/node_modules'), join(root, 'profiles/node_modules'), 'junction');
  }
  const modelOverlay = preserveProfile ? join(evidence, 'model-overlay.yml') : join(profileDir, 'cordis.patch.yml');
  write(modelOverlay, `
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
    resolvedProfile: { profile, installAnchor: join(dirname(entry), '../package.json') }, patchFiles: preserveProfile ? [modelOverlay] : [],
    args: ['--host', '127.0.0.1', '--port', '0', '--no-open'] });
  const { ctx } = runtime;
  let retainedOptionalFailure;
  if (preserveProfile && expectedOptionalFailure) {
    const match = /^((?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*): ((?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*) \(([A-Za-z_$][A-Za-z0-9_$]*)\)$/.exec(expectedOptionalFailure);
    assert(match, 'Only a verified native missing-export warning may be retained');
    const [, plugin, dependency, missingExport] = match;
    const failed = [...ctx.loader.entries()].filter(entry => entry.options.name === plugin && !entry.disabled);
    assert.equal(failed.length, 1, 'Configured failing optional plugin must remain enabled');
    assert.equal(failed[0].fiber, undefined, 'Configured optional plugin must retain its original link failure');
    await assert.rejects(failed[0].parent.tree.import(failed[0].options.name), error => error instanceof SyntaxError && error.message === `The requested module '${dependency}' does not provide an export named '${missingExport}'`);
    retainedOptionalFailure = expectedOptionalFailure;
  }
  const { createUserMessage } = await native('@deepseek-ai/dsh-llm');
  const { installModelSelection } = await native('@deepseek-ai/dsh-agent');
  const { scopeOf } = await native('@deepseek-ai/dsh-scope');
  const originalToolSkill = await native('@deepseek-ai/dsh-tool-skill');
  const originalFilesystem = await native('@deepseek-ai/dsh-skill-filesystem');
  const defaultId = ctx.agentPresets.defaultId;
  assert.equal(typeof defaultId, 'string');
  const presets = (await ctx.agentPresets.list()).map(preset => ({ ...preset, isDefault: preset.id === defaultId }));
  let preset = presets.find(p => p.isDefault);
  assert(preset && !preset.broken);
  const invocationPresets = preserveProfile ? presets : [preset];
  assert(invocationPresets.every(p => !p.broken));
  if (preserveProfile) {
    assert(deployed && readFileSync(join(root, 'skills/audit-code/SKILL.md')).equals(deployed), 'Invoke the unchanged publicly saved artifact in place');
  } else write(join(root, 'skills/audit-code/SKILL.md'), deployed ?? `---\nname: audit-code\ndescription: Acceptance fixture\ndisable-model-invocation: true\nuser-invocable: true\n---\n${marker}\n`);
  let scopeEvidence;
  const presetResults = [];
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
      const snapshot = await ctx.skills.snapshot({ cwd: work, scope: scopeOf(handle.agent.ctx) });
      const savedSkill = snapshot.skills.find(skill => skill.name === 'audit-code');
      const composition = ctx.agentPresets.inspectCompositions().find(item => item.id === preset.id);
      assert(composition, 'Every live preset must expose its original composition');
      const hasOriginalModule = name => {
        const modules = composition.modules.filter(module => module.moduleName === name);
        for (const module of modules) {
          const base = module.useHostBase ? ctx.baseUrl : module.baseUrl;
          assert.equal(realpathSync(boot.resolvePluginResource(name, base)), nativePaths.get(name), 'The live composition must resolve the original installed module');
        }
        return modules.length > 0;
      };
      scopeEvidence = {
        index: invocationPresets.indexOf(preset),
        preset: ['standard', 'ptc', 'minimal', 'cordis'].includes(preset.id) ? preset.id : 'custom',
        isDefault: !!preset.isDefault, skillToolVisible: !!ctx.tools.get('skill', handle.agent),
        originalPluginRegistered: !!ctx.registry.get(originalToolSkill),
        originalFilesystemRegistered: !!ctx.registry.get(originalFilesystem),
        compositionHasOriginalModule: hasOriginalModule('@deepseek-ai/dsh-tool-skill'),
        compositionHasOriginalFilesystem: hasOriginalModule('@deepseek-ai/dsh-skill-filesystem'),
        snapshotComplete: snapshot.complete, savedMetadataPresent: !!savedSkill,
        userInvocable: savedSkill?.invocation.userInvocable, modelInvocable: savedSkill?.invocation.modelInvocable,
      };
      const message = createUserMessage({ content: [{ type: 'text', text }], source: { kind: 'user' } });
      if (text.startsWith('/audit-code ')) {
        assert.equal(message.source.kind, 'user');
        assert.equal(message.content.length, 1);
        assert.equal(message.content[0].text, text);
        assert(/(^|\s)\/([a-z0-9]+(?:-[a-z0-9]+)*)(?=\s|$)/g.test(message.content[0].text));
      }
      handle.agent.followup(message);
      await handle.agent.whenIdle();
      if (!received.length) process.stderr.write(`MAGPIE_SAFE_NO_REQUEST ${JSON.stringify({ ...scopeEvidence, stage: modelRequestsSkill ? 'tool' : text.startsWith('/audit-code ') ? 'explicit' : text.includes('自行选择') ? 'implicit' : 'ordinary' })}\n`);
      assert(received.length > 0, 'Original Agent must contact the loopback model');
      if (replacedBody) assert(!JSON.stringify(received).includes(replacedBody), 'The replaced pre-edit body cannot reach the model');
      return JSON.stringify(received);
    } finally { await handle.dispose(); }
  }
  for (const currentPreset of invocationPresets) {
    preset = currentPreset;
    assert(!(await run('普通聊天：检查代码边界。')).includes(marker), 'Ordinary chat cannot load an explicit-only body');
    const capabilities = { ...scopeEvidence };
    assert(capabilities.snapshotComplete);
    assert.equal(capabilities.skillToolVisible, capabilities.compositionHasOriginalModule);
    assert.equal(capabilities.skillToolVisible, capabilities.compositionHasOriginalFilesystem);
    assert.equal(capabilities.savedMetadataPresent, capabilities.compositionHasOriginalFilesystem);
    if (capabilities.skillToolVisible) {
      assert(capabilities.originalPluginRegistered && capabilities.originalFilesystemRegistered && capabilities.savedMetadataPresent);
      assert.equal(capabilities.userInvocable, true);
      assert.equal(capabilities.modelInvocable, false);
    }
    if (capabilities.isDefault) assert(capabilities.skillToolVisible, 'The default preset must provide native explicit skill invocation');
    const trailing = '用户后接指令_' + randomUUID();
    const explicit = await run('/audit-code ' + trailing);
    if (!capabilities.skillToolVisible) {
      assert(!explicit.includes(marker), 'A preset without the native skill plugin cannot load the body');
      assert(explicit.includes('/audit-code ' + trailing), 'A preset without skill invocation must preserve the original slash text');
      assert(!(await run('请自行选择合适的技能。')).includes(marker));
      presetResults.push({ ...capabilities, ordinaryChat: 'pass', explicitInvocation: 'unsupported-preserved', trailingText: 'pass', noImplicitBody: 'pass', modelRequestedLoad: 'not-offered' });
      process.stderr.write(`MAGPIE_SAFE_PRESET_RESULT ${JSON.stringify(presetResults.at(-1))}\n`);
      continue;
    }
    if (!explicit.includes('<skill_content name=\\"audit-code\\">')) {
      process.stderr.write(`MAGPIE_SAFE_EXPLICIT preset=${invocationPresets.indexOf(currentPreset)} body=${explicit.includes(marker)} trailing=${explicit.includes(trailing)} tag=${explicit.includes('<skill_content')}\n`);
    }
    assert(explicit.includes('<skill_content name=\\"audit-code\\">'), 'Original host must expand the explicit gesture');
    assert(explicit.includes(marker));
    assert(explicit.includes(trailing), 'Trailing text remains a native user message');
    assert(!(await run('请自行选择合适的技能。')).includes(marker), 'Implicit discovery cannot expose the body');
    const attempted = await run('请自行加载 audit-code 技能。', true);
    assert(received.length >= 2, 'A real model tool call must reach the native rejection path');
    assert(received[0].tools?.some(tool => ['skill', 'run_code'].includes(tool.function?.name)), 'The native skill route must be offered');
    assert(attempted.includes('not available for model invocation'), 'The native handler must reject the model-requested load');
    assert(!attempted.includes(marker), 'A model tool call cannot load an explicit-only body');
    presetResults.push({ ...capabilities, ordinaryChat: 'pass', explicitInvocation: 'pass', trailingText: 'pass', noImplicitBody: 'pass', modelRequestedLoad: 'blocked' });
    process.stderr.write(`MAGPIE_SAFE_PRESET_RESULT ${JSON.stringify(presetResults.at(-1))}\n`);
  }
  write(join(evidence, 'model-requested-load.json'), JSON.stringify(received, null, 2));
  assert.equal(presetResults.filter(preset => preset.isDefault && preset.skillToolVisible).length, 1, 'The actual native default must complete the supported invocation path');
  report = { ordinaryChat: 'pass', explicitInvocation: 'pass', trailingText: 'pass', noImplicitBody: 'pass',
    modelRequestedLoad: 'blocked',
    deployedArtifactHash: deployed ? createHash('sha256').update(deployed).digest('hex') : undefined,
    sourcesVerified: sourceRoots.length > 0, preservedProfile: preserveProfile, retainedOptionalFailure, presetCount: invocationPresets.length,
    supportedPresetCount: presetResults.filter(preset => preset.skillToolVisible).length,
    unsupportedPresetCount: presetResults.filter(preset => !preset.skillToolVisible).length, presetResults,
    modelEndpoint: 'loopback only', artifactDirectory: evidence };
} finally {
  clearTimeout(timer);
  if (runtime) await runtime.shutdown.shutdown(0);
  await new Promise(r => server.close(r));
  assert.deepEqual(stamp(), before, 'Original installation and profile must remain unchanged');
}
process.stdout.write(JSON.stringify(report) + '\n');
