// Runs before `next dev` / `next build` (F23). MapLibre 6 finds its worker through import.meta.url,
// which the bundle rewrites to a build-machine file:// path, so the worker URL came out empty and no
// map ever loaded. The worker and the chunk it imports are served from public/ instead; map.tsx
// points setWorkerUrl at them. `.js`, not `.mjs`: a module worker needs a JavaScript MIME type.
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';

const require = createRequire(import.meta.url);
const pkg = require.resolve('maplibre-gl/package.json');
const { version } = JSON.parse(readFileSync(pkg, 'utf8')) as { version: string };
const dist = join(dirname(pkg), 'dist');
const out = join(import.meta.dirname, '..', 'public', 'maplibre');

const worker = readFileSync(join(dist, 'maplibre-gl-worker.mjs'), 'utf8');
const imports = worker.split('"./maplibre-gl-shared.mjs"').length - 1;
if (imports !== 1) throw new Error(`maplibre-gl ${version}: expected one import of the shared chunk in the worker, found ${imports}`);

rmSync(out, { recursive: true, force: true });
mkdirSync(join(out, version), { recursive: true });
writeFileSync(join(out, version, 'maplibre-gl-worker.js'), worker.replace('"./maplibre-gl-shared.mjs"', '"./maplibre-gl-shared.js"'));
writeFileSync(join(out, version, 'maplibre-gl-shared.js'), readFileSync(join(dist, 'maplibre-gl-shared.mjs')));
console.log(`maplibre worker ${version} → public/maplibre/${version}/`);
