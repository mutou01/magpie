// Acceptance fixture only: preserve physical package/peer precedence and every
// selected configuration byte, without running or modifying the source home.
import assert from 'node:assert/strict';
import { lstatSync, statSync, readdirSync, readFileSync, writeFileSync, mkdirSync, readlinkSync, symlinkSync } from 'node:fs';
import { resolve, join, dirname, relative, isAbsolute } from 'node:path';
import { createHash } from 'node:crypto';
const source = resolve(process.argv[2]), target = resolve(process.argv[3]);
assert(source !== target);
const records = [];
let bytes = 0;
const inside = (path, root) => { const rel = relative(root, path); return !isAbsolute(rel) && rel !== '..' && !rel.startsWith('..\\') && !rel.startsWith('../'); };
function copy(from, to) {
  const info = lstatSync(from, { throwIfNoEntry: false });
  if (!info) return;
  assert(records.length < 50000);
  mkdirSync(dirname(to), { recursive: true });
  if (info.isSymbolicLink()) {
    const original = resolve(dirname(from), readlinkSync(from));
    const destination = inside(original, source) ? join(target, relative(source, original)) : original;
    symlinkSync(destination, to, statSync(from).isDirectory() ? 'junction' : 'file');
    records.push([relative(source, from), 'link', original, destination]);
  } else if (info.isDirectory()) {
    mkdirSync(to, { recursive: true });
    const names = readdirSync(from).sort();
    records.push([relative(source, from), 'directory', names]);
    for (const name of names) copy(join(from, name), join(to, name));
  } else {
    assert(info.isFile());
    bytes += info.size;
    assert(bytes <= 1024 * 1024 * 1024);
    const data = readFileSync(from);
    writeFileSync(to, data, { mode: info.mode & 0o777 });
    assert(readFileSync(to).equals(data));
    records.push([relative(source, from), createHash('sha256').update(data).digest('hex')]);
  }
}
for (const path of ['profiles/web/package.json', 'profiles/web/compatibility.json', 'profiles/web/cordis.patch.yml', 'cordis.patch.yml', '.env', 'profiles/web/node_modules', 'skills']) copy(join(source, path), join(target, path));
for (const [rel, kind, originalTarget, copiedTarget] of records) {
  const from = join(source, rel), to = join(target, rel);
  if (kind === 'link') {
    assert.equal(resolve(dirname(from), readlinkSync(from)), originalTarget);
    assert.equal(resolve(dirname(to), readlinkSync(to)), copiedTarget);
  } else if (kind === 'directory') {
    assert.deepEqual(readdirSync(from).sort(), originalTarget);
    assert.deepEqual(readdirSync(to).sort(), originalTarget);
  } else {
    assert.equal(createHash('sha256').update(readFileSync(from)).digest('hex'), kind);
    assert.equal(createHash('sha256').update(readFileSync(to)).digest('hex'), kind);
  }
}
mkdirSync(join(target, 'profiles'), { recursive: true });
symlinkSync(join(source, 'profiles/node_modules'), join(target, 'profiles/node_modules'), 'junction');
console.log(JSON.stringify({ copiedEntries: records.length, copiedBytes: bytes, copyHash: createHash('sha256').update(JSON.stringify(records)).digest('hex') }));
