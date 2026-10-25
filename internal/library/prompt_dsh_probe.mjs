// Separate client of the original installation. Never patches a module,
// replaces a registry method, installs a plugin or submits an Agent input.
import { createRequire, isBuiltin } from 'node:module';
import { pathToFileURL, fileURLToPath } from 'node:url';
import { dirname, resolve, join, isAbsolute, relative } from 'node:path';
import { existsSync, readFileSync, readdirSync, realpathSync, writeFileSync, lstatSync, statSync, readlinkSync, mkdirSync, symlinkSync } from 'node:fs';
import { randomUUID, createHash } from 'node:crypto';
import { isDeepStrictEqual } from 'node:util';
let input = '';
for await (const chunk of process.stdin) input += chunk;
const request = JSON.parse(input);
process.env.DSH_HOME = request.home;
process.env.DSH_AGENTS_HOME = request.agentsHome;
const entry = realpathSync(request.entry);
const anchor = resolve(dirname(entry), '..', 'package.json');
const require = createRequire(anchor);
const resolvers = [require];
const sources = new Set([anchor]);
const nativeFiles = new Map();
const native = async spec => {
  for (const resolver of resolvers) {
    let file;
    try { file = resolver.resolve(spec); } catch { continue; }
    sources.add(realpathSync(file));
    nativeFiles.set(spec, realpathSync(file));
    resolvers.push(createRequire(file));
    return import(pathToFileURL(file).href);
  }
  throw new Error('Required native dependency is unavailable: ' + spec);
};
let stage = 'native profile';
const progress = value => { stage = value; process.stdout.write(JSON.stringify({ stage }) + '\n'); };
const fail = message => { const error = new Error(message); error.inventoryReason = message; throw error; };
const inside = (path, root) => { const rel = relative(root, path); return rel === '' || !isAbsolute(rel) && rel !== '..' && !rel.startsWith('../') && !rel.startsWith('..\\'); };
const moduleSource = join(request.sourceHome, 'profiles/web/node_modules');
const moduleCopy = join(request.home, 'profiles/web/node_modules');
const copies = [];
const digest = path => createHash('sha256').update(readFileSync(path)).digest('hex');
const linkTarget = path => resolve(dirname(path), readlinkSync(path));
// Preserve the native distinction between physical profile packages and linked
// packages. Changing that distinction changes dependency/peer precedence.
const copyProfileModules = () => {
  let bytes = 0;
  const copy = (from, to) => {
    const stat = lstatSync(from, { throwIfNoEntry: false });
    if (!stat) return;
    if (copies.length >= 50000) fail('Profile dependency copy exceeds 50000 entries');
    if (stat.isSymbolicLink()) {
      const sourceTarget = linkTarget(from);
      const target = inside(sourceTarget, moduleSource) ? join(moduleCopy, relative(moduleSource, sourceTarget)) : sourceTarget;
      const directory = statSync(from, { throwIfNoEntry: false })?.isDirectory() ?? true;
      symlinkSync(target, to, directory ? 'dir' : 'file');
      copies.push({ from, to, kind: 'link', sourceTarget, target });
    } else if (stat.isDirectory()) {
      mkdirSync(to, { recursive: true, mode: 0o700 });
      const names = readdirSync(from).sort();
      copies.push({ from, to, kind: 'directory', names: JSON.stringify(names) });
      for (const name of names) copy(join(from, name), join(to, name));
    } else if (stat.isFile()) {
      bytes += stat.size;
      if (bytes > 1 << 30) fail('Profile dependency copy exceeds 1 GiB');
      const data = readFileSync(from);
      if (data.length !== stat.size) fail('Profile dependencies changed during verification');
      writeFileSync(to, data, { mode: stat.mode & 0o777 });
      copies.push({ from, to, kind: 'file', hash: createHash('sha256').update(data).digest('hex') });
    } else fail('Profile dependency is not a file, directory or link');
  };
  copy(moduleSource, moduleCopy);
};
const verifyCopies = () => {
  for (const entry of copies) {
    const original = lstatSync(entry.from), copied = lstatSync(entry.to);
    const matches = entry.kind === 'file'
      ? original.isFile() && copied.isFile() && digest(entry.from) === entry.hash && digest(entry.to) === entry.hash
      : entry.kind === 'directory'
        ? original.isDirectory() && copied.isDirectory() && JSON.stringify(readdirSync(entry.from).sort()) === entry.names && JSON.stringify(readdirSync(entry.to).sort()) === entry.names
        : original.isSymbolicLink() && copied.isSymbolicLink() && linkTarget(entry.from) === entry.sourceTarget && linkTarget(entry.to) === entry.target;
    if (!matches) fail('Profile dependencies changed during verification');
  }
};
// Only copied configuration, verified module copies and the shared dependency
// link have original counterparts. Generated storage is never source evidence.
const originalSource = path => {
  if (!inside(path, request.home)) return path;
  const rel = relative(request.home, path);
  const original = join(request.sourceHome, rel);
  if (inside(path, moduleCopy) || inside(path, join(request.home, 'profiles/node_modules')) ||
      ['profiles/web/package.json', 'profiles/web/compatibility.json', 'profiles/web/cordis.patch.yml', 'cordis.patch.yml', '.env'].includes(rel.replaceAll('\\', '/'))) return original;
  if (!existsSync(path) && !existsSync(original)) return original;
  fail('Temporary runtime resource has no verified original source');
};
const packageName = value => typeof value === 'string' && value.length <= 214 && /^(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*$/i.test(value);
const exportName = value => typeof value === 'string' && value.length <= 128 && /^[A-Za-z_$][A-Za-z0-9_$]*$/.test(value);
const dependencyDiagnostic = (error, plugin) => {
  if (!packageName(plugin)) return;
  const pending = [error], seen = new Set();
  while (pending.length && seen.size < 32) {
    const current = pending.shift();
    if (!current || typeof current !== 'object' || seen.has(current)) continue;
    seen.add(current);
    if (current.code === 'ERR_MODULE_NOT_FOUND' || current.code === 'MODULE_NOT_FOUND') {
      const match = /^Cannot find (?:package|module) ['"]([^'"\r\n]+)['"]/.exec(current.message ?? '');
      if (match && packageName(match[1])) return { kind: 'dependency', plugin, dependency: match[1] };
    }
    if (current.name === 'SyntaxError' && typeof current.message === 'string') {
      const match = /^The requested module ['"]([^'"\r\n]+)['"] does not provide an export named ['"]([^'"\r\n]+)['"]$/.exec(current.message);
      if (match && packageName(match[1]) && exportName(match[2])) return { kind: 'export', plugin, dependency: match[1], export: match[2] };
    }
    pending.push(current.cause);
    if (Array.isArray(current.errors)) pending.push(...current.errors.slice(0, 32));
  }
};
let runtime;
let result;
try {
  copyProfileModules();
  const boot = await native('@deepseek-ai/dsh-app-boot');
  const { runProfile } = await import(pathToFileURL(join(dirname(entry), 'profile-boot.js')).href);
  const profile = boot.loadProfileDirectory('dsh', join(request.home, 'profiles/web'), anchor);
  if (profile.skippedBundles.length) fail('Native profile skipped a bundle: ' + profile.skippedBundles.map(b => b.packageName).join(', '));
  for (const layer of profile.layers) {
    resolvers.push(createRequire(join(layer.packageDir, 'package.json')));
    sources.add(join(layer.packageDir, 'package.json'));
    for (const path of layer.patchPaths) sources.add(path);
  }
  const homePatches = boot.loadOptionalPatches('dsh', join(request.home, 'cordis.patch.yml')) ?? [];
  const entries = boot.composeEntries([...profile.layers.map(l => l.patches), profile.patches, homePatches]);
  // Preserve every config field and row, including each preset's providers.
  const mapRows = rows => rows.map(row => {
    const mapped = { ...row };
    if (row.name === '@deepseek-ai/dsh-skill-filesystem') {
      mapped.config = { ...row.config, dshHome: row.config?.dshHome ?? request.sourceHome,
        agentsHome: row.config?.agentsHome ?? request.agentsHome };
    } else if (row.name === '@deepseek-ai/dsh-agent-preset') {
      if (!Array.isArray(row.config?.plugins)) fail('Native preset has no composition');
      mapped.config = { ...row.config, plugins: mapRows(row.config.plugins) };
    } else if (row.group) {
      if (!Array.isArray(row.config)) fail('Native group has no composition');
      mapped.config = mapRows(row.config);
    }
    return mapped;
  });
  const mapped = mapRows(entries);
  const yaml = await native('js-yaml');
  const include = await native('@deepseek-ai/cordis-plugin-include');
  // Only temporary configuration is rewritten. Keep bundle identities for
  // native module resolution while freezing the complete effective entry list.
  for (const layer of profile.layers) layer.patches = [];
  profile.patches = [{ insert: mapped }];
  writeFileSync(profile.patchPath, yaml.dump(profile.patches, { schema: include.entryListSchema, noRefs: true }));
  writeFileSync(join(request.home, 'cordis.patch.yml'), '[]\n');
  // Keep the native parser, validation, precedence and layer provenance. The
  // home layer is a physical temporary copy; the project layer is read only.
  progress('native environment');
  let environment;
  try { environment = boot.loadLayeredEnv('dsh', request.envCwd); }
  catch (error) {
    const match = / sets "([A-Za-z_][A-Za-z0-9_]*)", which only the launching environment may set/.exec(error.message ?? '');
    if (match) error.inventoryDiagnostic = { kind: 'environment', variable: match[1],
      layer: error.message.startsWith(`dsh: ${join(request.home, '.env')} sets `) ? 'user' : 'project' };
    throw error;
  }
  progress('native startup');
  runtime = await runProfile({ environment, profile: 'web',
    resolvedProfile: { profile, installAnchor: anchor }, patchFiles: [],
    args: ['--host', '127.0.0.1', '--port', '0', '--no-open'] });
  const { ctx } = runtime;
  const emptyAttributes = attributes => attributes && typeof attributes === 'object' && Reflect.ownKeys(attributes).length === 0;
  const observeFailure = entry => {
    if (entry.fiber !== undefined || entry.disabled) return;
    const loader = ctx.loader.internal;
    if (typeof loader?.resolveSync !== 'function' || typeof loader.loadCache?.get !== 'function') return;
    const base = entry.parent.tree.ctx.baseUrl;
    const resolved = typeof loader.getOrCreateModuleJob === 'function'
      ? loader.resolveSync(base, { specifier: entry.options.name, attributes: {}, phase: 2 })
      : typeof loader.getModuleJobForImport === 'function'
        ? loader.resolveSync(entry.options.name, base, {}) : undefined;
    if (typeof resolved?.url !== 'string' || resolved.importAttributes !== undefined && !emptyAttributes(resolved.importAttributes)) return;
    const job = loader.loadCache.get(resolved.url);
    if (!job || job.url !== resolved.url || !emptyAttributes(job.importAttributes) || typeof job.instantiated?.then !== 'function' || job.module?.getStatus?.() !== 0) return;
    return { loader, job, module: job.module, promise: job.instantiated, url: resolved.url,
      id: entry.id, name: entry.options.name, options: structuredClone(entry.options), parent: entry.parent, tree: entry.parent.tree, base };
  };
  // Native HMR may replace failed jobs as soon as readiness is committed.
  // Retain the startup job before yielding; observe its existing rejection only.
  const startupFailures = new Map();
  for (const entry of ctx.loader.entries()) {
    const observed = observeFailure(entry);
    if (!observed) continue;
    startupFailures.set(entry, observed);
    observed.promise.then(undefined, error => { observed.stack = error?.stack; });
  }
  const { SkillRegistry } = await native('@deepseek-ai/dsh-skill');
  const { scopeOf, ScopedLayers, NamedEntries } = await native('@deepseek-ai/dsh-scope');
  const { FileSystemSkillProvider } = await native('@deepseek-ai/dsh-skill-filesystem');
  const { readPluginInventory } = await native('@deepseek-ai/dsh-host-plugin-inventory');
  const { EntryTree, Group } = await native('@deepseek-ai/cordis-plugin-loader');
  const warnings = new Map();
  const linkErrors = new Map();
  let stderrHash;
  const recordedPackages = new Set();
  const recordAliases = (name, base) => {
    // Node's original search paths retain the declarations that can retarget
    // a package even when the previously resolved physical tree is unchanged.
    for (const directory of createRequire(join(base, 'package.json')).resolve.paths(name) ?? []) {
      const candidate = join(directory, name, 'package.json');
      sources.add(candidate);
      // Later search locations cannot shadow an existing earlier package.
      // Removing that earlier manifest itself invalidates the evidence.
      if (existsSync(candidate)) break;
    }
    for (const directory of ['profiles', 'profiles/web']) sources.add(join(request.sourceHome, directory, 'node_modules', name, 'package.json'));
  };
  const recordPackage = directory => {
    const root = realpathSync(directory);
    if (recordedPackages.has(root)) return;
    recordedPackages.add(root);
    const manifestPath = join(root, 'package.json');
    const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
    if (typeof manifest.name !== 'string' || !manifest.name) fail('Native dependency declaration is missing');
    recordAliases(manifest.name, root);
    sources.add(manifestPath);
    // Published resources plus the actual module's subtree. A source checkout
    // can contain unrelated tests/build caches beyond its published program.
    let entries = Array.isArray(manifest.files) ? manifest.files : readdirSync(root);
    // A wildcard in the top component can select sibling runtime modules,
    // even when main points at one file. Conservatively include that package's
    // actual top-level resources; negative publication patterns need no extra
    // exclusion because a fingerprint may safely cover more installed bytes.
    if (entries.some(entry => typeof entry === 'string' && !entry.startsWith('!') && /[*?{}[\]]/.test(entry.replace(/^\.\//, '').split('/')[0]))) entries = readdirSync(root);
    for (const entry of [...entries, manifest.main, manifest.module].filter(e => typeof e === 'string')) {
      if (entry.startsWith('!')) continue;
      const name = entry.replace(/^\.\//, '').split('/')[0];
      if (name && name !== '..' && !name.includes('*') && name !== 'node_modules' && name !== '.git') sources.add(join(root, name));
    }
    for (const name of new Set([...Object.keys(manifest.dependencies ?? {}), ...Object.keys(manifest.optionalDependencies ?? {}), ...Object.keys(manifest.peerDependencies ?? {})])) {
      recordAliases(name, root);
      const dependency = ctx.pluginPackages.packageOf(name, pathToFileURL(manifestPath).href);
      if (dependency) recordPackage(dependency.dir);
      else if (name in (manifest.dependencies ?? {}) && !(name in (manifest.optionalDependencies ?? {}))) fail('Native runtime dependency is unresolved');
    }
  };
  const recordedModules = new Set();
  const recordModule = (specifier, base) => {
    const key = JSON.stringify([specifier, base]);
    if (recordedModules.has(key)) return;
    if (specifier === 'cordis:include') {
      // app-boot embeds Include in its published bundle. Fingerprint that
      // original owner as well as its runtime dependency graph.
      if (typeof ctx.loader.builtins.include !== 'function') fail('Native include implementation is unavailable');
      specifier = '@deepseek-ai/dsh-app-boot';
      base = pathToFileURL(anchor).href;
    } else if (specifier === 'cordis:group') {
      if (typeof Group !== 'function' || ctx.loader.builtins.group !== Group) fail('Native group implementation is unavailable');
      specifier = '@deepseek-ai/cordis-plugin-loader';
      base = pathToFileURL(anchor).href;
    }
    const pkg = ctx.pluginPackages.packageOf(specifier, base);
    if (!pkg) fail('Native plugin source is unresolved' + (/^[@a-z0-9_:/.-]+$/i.test(specifier) ? ': ' + specifier : ''));
    recordPackage(pkg.dir);
    sources.add(dirname(realpathSync(boot.resolvePluginResource(specifier, base))));
    // These are sources, not a cached inventory. Every graph() still reads the
    // native entries, presets and resolution; source bytes/aliases are stamped
    // between two separate native launches and again before accepting evidence.
    recordedModules.add(key);
  };
  // An import error's text alone cannot distinguish failed ESM linking from
  // plugin code throwing the same SyntaxError after evaluation has started.
  // Inspect only the original loader's already-started job, never run a new job.
  const proveLinkFailure = async (entry, observed) => {
    if (entry.fiber !== undefined || entry.disabled || entry.parent.tree.import !== EntryTree.prototype.import) return;
    if (!observed || entry.id !== observed.id || !isDeepStrictEqual(entry.options, observed.options) || entry.options.name !== observed.name ||
        entry.parent !== observed.parent || entry.parent.tree !== observed.tree || entry.parent.tree.ctx.baseUrl !== observed.base || ctx.loader.internal !== observed.loader) return;
    const { job } = observed;
    if (job.module !== observed.module || job.instantiated !== observed.promise || job.module?.getStatus?.() !== 0) return;
    let diagnostic, rejection;
    try { await job.instantiated; }
    catch (error) { rejection = error; diagnostic = dependencyDiagnostic(error, entry.options.name); }
    if (diagnostic?.kind !== 'export') return;
    const seen = new Set(), pending = [job], graph = [];
    let edges = 0;
    while (pending.length) {
      const current = pending.pop();
      if (seen.has(current)) continue;
      seen.add(current);
      if (seen.size > 10000 || typeof current?.url !== 'string' || !current.module ||
          typeof current.module.getModuleRequests !== 'function') fail('Native failed-module dependency graph is unavailable');
      const requests = current.module.getModuleRequests();
      if (!Array.isArray(requests) || requests.length > 10000 || requests.some(request => request.phase !== 2 || typeof request.specifier !== 'string' || !emptyAttributes(request.attributes))) fail('Native failed-module dependency requests are incomplete');
      let hash;
      if (current.url.startsWith('file:')) {
        const file = realpathSync(fileURLToPath(current.url));
        sources.add(file);
        hash = digest(file);
      }
      else if (!current.url.startsWith('node:')) fail('Native failed-module source is unverifiable');
      for (const request of requests) {
        const specifier = request.specifier;
        if (isBuiltin(specifier) || specifier.startsWith('.') || specifier.startsWith('file:')) continue;
        const pkg = ctx.pluginPackages.packageOf(specifier, current.url);
        if (!pkg || !current.url.startsWith('file:')) fail('Native failed-module package source is unverifiable');
        recordAliases(pkg.name, dirname(fileURLToPath(current.url)));
        recordModule(specifier, current.url);
      }
      const linked = await current.linked;
      if (!Array.isArray(linked) || linked.length > 10000 || (edges += linked.length) > 50000) fail('Native failed-module dependency graph is incomplete');
      graph.push({ url: current.url, hash, requests, linked: linked.map(child => child?.url) });
      for (const child of Array.from(linked)) {
        if (!child || typeof child !== 'object' || typeof child.then === 'function') fail('Native failed-module dependency job is unavailable');
        pending.push(child);
      }
    }
    if (entry.fiber !== undefined || entry.disabled || job.module !== observed.module || job.instantiated !== observed.promise) fail('Native optional plugin changed during verification');
    if (!(rejection instanceof Error) || rejection.cause !== undefined || rejection instanceof AggregateError || typeof rejection.stack !== 'string' || !rejection.stack) fail('Native optional plugin diagnostic is unavailable');
    return { diagnostic, rejection, graph: JSON.stringify(graph.sort((a, b) => a.url.localeCompare(b.url))) };
  };
  const startupProofs = new Map();
  const optionalLinkFailure = async entry => {
    const initial = startupFailures.get(entry);
    if (!initial) return;
    if (!startupProofs.has(entry)) startupProofs.set(entry, await proveLinkFailure(entry, initial));
    const startup = startupProofs.get(entry), observed = observeFailure(entry);
    const current = await proveLinkFailure(entry, observed);
    if (!startup || !current || initial.url !== observed.url || startup.graph !== current.graph ||
        JSON.stringify(startup.diagnostic) !== JSON.stringify(current.diagnostic)) return;
    if (observed.loader.loadCache.get(observed.url) !== observed.job) fail('Native optional plugin changed during verification');
    if (typeof initial.stack !== 'string' || !initial.stack) fail('Native startup diagnostic is unavailable');
    linkErrors.set(current.rejection, initial.stack);
    return current;
  };
  const graph = async () => {
    if (typeof boot.auditStartupEntries !== 'function') fail('Native startup audit is unavailable');
    const resolution = ctx.pluginPackages?.resolution;
    if (!Array.isArray(resolution?.entries) || !Array.isArray(resolution.linkedRoots)) fail('Native package resolution is unavailable');
    for (const entry of ctx.loader.entries()) if (!entry.disabled) recordModule(entry.options.name, entry.parent.tree.ctx.baseUrl);
    // Standing preset trees are deliberately detached from the host tree,
    // and their public composition inventory omits groups. Original runtime
    // fibers retain each live group's actual Entry and resolution base.
    if (typeof ctx.registry?.values !== 'function') fail('Native plugin runtime inventory is unavailable');
    for (const plugin of ctx.registry.values()) {
      if (typeof plugin.fibers?.[Symbol.iterator] !== 'function') fail('Native plugin fiber inventory is unavailable');
      for (const fiber of plugin.fibers) {
        const entry = fiber.entry;
        if (entry?.fiber === fiber && entry.options.group && !entry.disabled) recordModule(entry.options.name, entry.parent.tree.ctx.baseUrl);
      }
    }
    for (const preset of ctx.agentPresets.inspectCompositions()) {
      if (preset.leakedServices.length) fail('Native preset leaked services outside its scope');
      for (const module of preset.modules) recordModule(module.moduleName, module.useHostBase ? ctx.baseUrl : module.baseUrl);
    }
    const inventory = await readPluginInventory(ctx);
    if (!Array.isArray(inventory.entries) || !Array.isArray(inventory.agentPresets)) fail('Native plugin graph is incomplete');
    const running = rows => rows.every(row => typeof row.enabled === 'boolean' && (!row.enabled || row.fiberPhase === 'active'));
    if (inventory.agentPresets.some(p => p.broken || !running(p.rows))) {
      fail('Native plugin graph contains an inactive or unresolved entry');
    }
    const optional = [], failures = [];
    for (const row of inventory.entries) {
      if (row.enabled === false || row.enabled === true && row.fiberPhase === 'active') continue;
      const entry = [...ctx.loader.entries()].find(entry => entry.id === row.entryId && entry.options.name === row.moduleName);
      const proof = row.enabled === true && row.fiberPhase === null && entry ? await optionalLinkFailure(entry) : undefined;
      if (!proof) {
        if (entry) {
          try { await entry.parent.tree.import(entry.options.name); }
          catch (error) {
            const diagnostic = dependencyDiagnostic(error, entry.options.name);
            if (diagnostic) { error.inventoryDiagnostic = diagnostic; throw error; }
          }
        }
        fail('Native plugin graph contains an inactive or unresolved entry');
      }
      const { diagnostic, rejection } = proof;
      failures.push(rejection);
      optional.push({ entryId: row.entryId, ...diagnostic });
      warnings.set(JSON.stringify(diagnostic), diagnostic);
    }
    let audit = '';
    await boot.auditStartupEntries(ctx, 'dsh', text => { audit += text; });
    if (failures.length) {
      // Retain the original audit's formatting and every byte. Only restore
      // the same cached errors' pre-audit stacks in that generated text.
      const stacks = new Map();
      for (const error of failures) {
        const current = error.stack, original = linkErrors.get(error);
        if (typeof current !== 'string' || !current) fail('Native optional plugin diagnostic changed');
        const previous = stacks.get(current);
        if (previous && previous.original !== original) fail('Native optional plugin diagnostic is ambiguous');
        stacks.set(current, { original, count: (previous?.count ?? 0) + 1 });
      }
      for (const current of stacks.keys()) {
        if ([...stacks.keys()].some(other => other !== current && other.includes(current))) fail('Native optional plugin diagnostic is ambiguous');
      }
      for (const [current, { original, count }] of stacks) {
        if (audit.split(current).length !== count + 1) fail('Native optional plugin diagnostic is incomplete');
        audit = audit.replaceAll(current, () => original);
      }
      const hash = createHash('sha256').update(audit).digest('hex');
      if (stderrHash !== undefined && stderrHash !== hash) fail('Native startup diagnostic changed during verification');
      stderrHash = hash;
    } else if (audit) fail('Native startup diagnostic is unverified');
    return JSON.stringify({ entries: inventory.entries.map(({ entryId, moduleName, enabled, fiberPhase }) => ({ entryId, moduleName, enabled, fiberPhase })),
      optional,
      presets: inventory.agentPresets.map(({ id, isDefault, rows }) => ({ id, isDefault, rows })),
      packages: resolution.entries.map(({ name, packageDir, version, scope }) => ({ name, packageDir: realpathSync(packageDir), version, scope })) });
  };
  // Read-only structural capabilities from original classes. A missing or
  // changed layout blocks verification; no release/hash allowlist is used.
  const registry = ctx.skills;
  if (!(registry instanceof SkillRegistry) || !(registry.layers instanceof ScopedLayers) || !Number.isSafeInteger(registry.revision)) fail('Native provider inventory is unavailable');
  const presets = await ctx.agentPresets.list();
  if (!Array.isArray(presets) || !presets.length || presets.some(p => p.broken)) fail('Native preset inventory is incomplete');
  const commands = [];
  const derived = new Map();
  const inspectProviders = async options => {
    const layers = [registry.layers.global, ...registry.layers.chainLayers(options.scope)];
    for (const layer of layers) {
      if (!(layer.providers instanceof NamedEntries) || !(layer.runtime instanceof Map)) fail('Native provider layer is unavailable');
      if (layer.runtime.size) fail('Dynamic runtime skill source is unverifiable');
      for (const { provider } of layer.providers.values()) {
        if (provider instanceof FileSystemSkillProvider) {
          if (provider.constructor !== FileSystemSkillProvider || ['roots', 'list', 'get'].some(name => provider[name] !== FileSystemSkillProvider.prototype[name])) fail('Unknown dynamic skill provider');
          if (typeof provider.roots !== 'function') fail('Native resource roots are unavailable');
          const roots = await provider.roots(options.cwd);
          if (!Array.isArray(roots)) fail('Native resource roots are incomplete');
          for (const root of roots) {
            if (typeof root.path !== 'string' || !isAbsolute(root.path)) fail('Native resource root is invalid');
            sources.add(root.path); // Includes absent and empty directories.
          }
        } else if (provider.name === 'dsh-windows-acl' && process.platform === 'win32') {
          await native('@deepseek-ai/dsh-sandbox-local');
          const acl = await native('@deepseek-ai/dsh-sandbox-windows-acl');
          const candidates = await provider.list(options);
          if (!Array.isArray(candidates) || candidates.length !== 1 || candidates[0].name !== acl.ACL_DIAGNOSIS_SKILL) fail('Native ACL skill inventory changed');
          const candidate = candidates[0];
          const packaged = join(dirname(nativeFiles.get('@deepseek-ai/dsh-sandbox-windows-acl')), '..', 'assets', acl.ACL_DIAGNOSIS_SKILL);
          if (candidate.resourceBase?.kind !== 'directory' || typeof candidate.resourceBase.path !== 'string') fail('Native ACL skill resources are unavailable');
          for (const rel of ['SKILL.md', 'scripts/diagnose-windows-sandbox-acl.ps1']) {
            const original = join(packaged, rel);
            if (!readFileSync(original).equals(readFileSync(join(candidate.resourceBase.path, rel)))) fail('Native ACL skill differs from installed assets');
            sources.add(original);
          }
          const definition = await provider.get(candidate, options);
          const raw = readFileSync(join(packaged, 'SKILL.md'), 'utf8');
          const frontmatter = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/u.exec(raw);
          if (!frontmatter) fail('Native ACL skill metadata is unavailable');
          const content = raw.slice(frontmatter[0].length).trim();
          if (definition?.content !== content) fail('Native ACL skill body differs from installed assets');
          if (!definition || definition.name !== candidate.name || definition.invocation?.userInvocable !== true) fail('Native ACL skill could not load');
          const metadata = yaml.load(frontmatter[1]);
          if (definition.description !== metadata.description || definition.invocation.modelInvocable !== true || definition.provider !== provider.name || definition.source !== 'bundled') fail('Native ACL skill metadata differs from installed assets');
          derived.set(candidate.name, { path: join(packaged, 'SKILL.md'), content });
        } else fail('Unknown dynamic skill provider');
      }
    }
  };
  const readScope = async options => {
    const before = await graph();
    const revision = registry.revision;
    await inspectProviders(options);
    const snapshot = await registry.snapshot(options);
    if (snapshot.complete !== true || !Array.isArray(snapshot.skills)) fail('Native skill discovery is incomplete');
    for (const skill of snapshot.skills) {
      if (typeof skill.name !== 'string' || !skill.name) fail('Native skill name is missing');
      const definition = await registry.get(skill.name, options);
      if (!definition || definition.name !== skill.name || typeof definition.content !== 'string') fail('Native skill could not load');
      const packaged = derived.get(skill.name);
      if (packaged && definition.content !== packaged.content) fail('Native ACL skill body differs from installed assets');
      const path = packaged?.path ?? skill.path;
      if (typeof path !== 'string' || !isAbsolute(path)) fail('Native skill source is missing');
      commands.push({ name: skill.name, path });
    }
    if (revision !== registry.revision || before !== await graph()) fail('Native scoped inventory changed during verification');
  };
  for (const cwd of request.cwds) for (const preset of presets) {
    progress('native scoped inventory');
    const lease = await ctx.agentPresets.acquireScope(preset.id);
    try {
      const options = { cwd, scope: lease.key };
      await readScope(options);
      const handle = await ctx.agents.create({ sessionId: randomUUID(), meta: { cwd, agentPreset: preset.id },
        setup: async agentCtx => { await ctx.agentPresets.mount(agentCtx, preset.id); } });
      try {
        const before = await graph();
        await readScope({ cwd, scope: scopeOf(handle.agent.ctx) });
        const list = ctx.commands.list(handle.agent);
        if (!Array.isArray(list)) fail('Native command discovery is incomplete');
        for (const command of list) {
          if (typeof command.name !== 'string' || !command.name) fail('Native command name is missing');
          commands.push({ name: command.name, path: '' });
        }
        if (before !== await graph()) fail('Native commands changed during verification');
      } finally { await handle.dispose(); }
    } finally { await lease[Symbol.asyncDispose](); }
  }
  verifyCopies();
  if (realpathSync(join(request.home, 'profiles/node_modules')) !== realpathSync(join(request.sourceHome, 'profiles/node_modules'))) fail('Native dependency link changed during verification');
  result = { commands: commands.map(command => ({ ...command, path: command.path ? originalSource(command.path) : '' })),
    sources: [...new Set([...sources].map(originalSource))].sort(), warnings: [...warnings.values()], stderrHash, complete: true };
} catch (error) {
  // Arbitrary startup errors/logs may contain credentials; never forward them.
  const diagnostic = error.inventoryDiagnostic ?? error.entries?.map(entry => dependencyDiagnostic(entry.outcome?.error, entry.module)).find(Boolean);
  result = { complete: false, reason: error.inventoryReason ?? `Native check failed during ${stage}`, diagnostic };
} finally {
  if (runtime) await runtime.shutdown.shutdown(0);
}
process.stdout.write(JSON.stringify(result) + '\n');
