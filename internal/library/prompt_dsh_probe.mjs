// Read-only native API client. Runs outside the installation; no check plugin,
// no source patches and no user prompt. DSH/plugin startup is NOT a sandbox.
import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { dirname, resolve } from 'node:path';
let input = '';
for await (const chunk of process.stdin) input += chunk;
const request = JSON.parse(input);
const nativeRequire = createRequire(resolve(dirname(request.entry), '..', 'package.json'));
// Resolve transitive client dependencies from the installed packages that own
// them. A CLI need not expose all Web client's dependencies at its top level.
const nativeResolvers = [nativeRequire];
for (const anchor of ['@deepseek-ai/dsh-api-gateway/client', '@deepseek-ai/dsh-web-app']) {
  try { nativeResolvers.push(createRequire(nativeRequire.resolve(anchor))); } catch {}
}
const native = async spec => {
  progress(`native module ${spec}`);
  for (const resolver of nativeResolvers) {
    let path;
    try { path = resolver.resolve(spec); } catch { continue; }
    return import(pathToFileURL(path).href);
  }
  throw new Error('Required native client dependency is unavailable');
};
const child = spawn(process.execPath, [request.entry, '--profile', 'web', '--host', '127.0.0.1', '--port', '0', '--no-open'], {
  cwd: request.home,
  env: { ...process.env, DSH_HOME: request.home, DSH_AGENTS_HOME: request.agentsHome },
  stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true,
});
let ctx;
let size = 0;
let logs = '';
let stage;
const progress = value => { stage = value; process.stdout.write(JSON.stringify({ stage }) + '\n'); };
progress('startup');
try {
  const ready = await new Promise((accept, reject) => {
    child.once('error', reject);
    child.once('exit', () => reject(new Error('Native startup failed')));
    const receive = chunk => {
      size += chunk.length;
      if (size > 1 << 20) return reject(new Error('Native output exceeded limit'));
      logs += chunk;
      const match = logs.match(/dsh web:\s*(http:\/\/127\.0\.0\.1:\d+\/\?token=[^\s]+)/);
      if (match) accept(match[1]);
    };
    child.stdout.on('data', receive);
    child.stderr.on('data', receive);
  });
  const url = new URL(ready);
  progress('authorization');
  const token = url.searchParams.get('token');
  const auth = new URL('/connect', url.origin);
  auth.searchParams.set('token', token);
  auth.searchParams.set('interactions', 'none');
  const response = await fetch(auth, { redirect: 'manual' });
  const cookies = response.headers.getSetCookie().map(s => s.split(';')[0]).join('; ');
  if (response.status !== 302 || !cookies) throw new Error('Native authorization failed');
  progress('native client initialization');
  const { Context } = await native('@deepseek-ai/cordis');
  const { installConnection } = await native('@deepseek-ai/dsh-client-connection/client');
  const { default: Registry } = await native('@deepseek-ai/dsh-typert-registry');
  const gateway = await native('@deepseek-ai/dsh-api-gateway/client');
  const { default: WebSocket } = await native('ws');
  progress('native connection');
  ctx = new Context();
  installConnection(ctx, { interactions: 'none', transport: {
    streamBaseUrl: `${url.origin}/`,
    fetch: (input, init) => fetch(new URL(input, url.origin), { ...init, headers: { ...Object.fromEntries(new Headers(init?.headers)), cookie: cookies } }),
    createWebSocket: address => new WebSocket(address, { headers: { cookie: cookies } }),
  } });
  await ctx.plugin(Registry);
  progress('native gateway');
  await ctx.plugin({ apply: gateway.apply, inject: gateway.inject });
  for (const spec of ['@deepseek-ai/dsh-api-session-controller/remote', '@deepseek-ai/dsh-commands/remote', '@deepseek-ai/dsh-host-plugin-inventory/remote', '@deepseek-ai/dsh-agent-preset-registry/remote']) {
    await ctx.remote.$mount((await native(spec)).default);
  }
  const unwrap = result => { if (!result.ok) throw new Error('Native inventory request failed'); return result.value; };
  progress('plugin and preset inventory');
  const inventory = unwrap(await ctx.remote.pluginInventory.list());
  const presets = unwrap(await ctx.remote.agentPresets.list());
  if (!Array.isArray(inventory.entries) || !Array.isArray(inventory.agentPresets) || inventory.agentPresets.some(p => p.broken || !Array.isArray(p.rows)) || !Array.isArray(presets.presets) || !presets.presets.length || presets.presets.some(p => !inventory.agentPresets.some(g => g.id === p.id))) throw new Error('Native preset is incomplete');
  const commands = [];
  for (const cwd of request.cwds) for (const preset of presets.presets) {
    progress('temporary session inventory');
    const session = unwrap(await ctx.remote.session.create({ cwd, agentPreset: preset.id }));
    const nativeCommands = unwrap(await ctx.remote.commands.list(session.sessionId));
    const skills = unwrap(await ctx.remote.skills.list({ sessionId: session.sessionId }));
    if (!Array.isArray(nativeCommands) || !Array.isArray(skills.skills)) throw new Error('Native list is incomplete');
    for (const c of nativeCommands) commands.push({ name: c.name, path: '' });
    for (const s of skills.skills) commands.push({ name: s.name, path: s.path ?? '' });
  }
  const entries = [...inventory.entries, ...inventory.agentPresets.flatMap(p => p.rows)];
  if (entries.some(e => typeof e.moduleName !== 'string' || !e.moduleName)) throw new Error('Native plugin source is incomplete');
  const providers = [...new Set(entries.map(e => e.moduleName))].sort();
  // The native API currently drops SkillRegistry.snapshot.complete. It also
  // does not expose the complete effective provider/resource mapping. Returning
  // a list cannot certify these providers. Do not fabricate that evidence.
  process.stdout.write(JSON.stringify({ commands, complete: false,
    reason: `Native skills/list omits provider completeness and effective source mapping; providers requiring verification: ${providers.join(', ')}` }));
} catch {
  // Never forward startup logs: they may contain tokens, credentials or paths
  // embedded by third-party plugins. The caller retains the concrete stage.
  process.stdout.write(JSON.stringify({ complete: false, reason: `Native check failed during ${stage}` }));
} finally {
  if (ctx) await ctx.fiber.dispose();
  child.kill();
}
