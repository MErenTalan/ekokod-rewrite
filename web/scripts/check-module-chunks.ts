// pnpm check:module-chunks (after `next build`) — F17 R490: the browser code of every EKOCM route holds no
// EKORM-only API path, and EKORM's holds no carbon path. Complements the lint rule and the import-walk test.
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const NEXT = join(import.meta.dirname, '..', '.next');
const pages: Record<string, string[]> = JSON.parse(readFileSync(join(NEXT, 'app-build-manifest.json'), 'utf8')).pages;
const EKORM_API = /\/api\/v1\/(consumption|load-profile|bills|tariffs|alarms|messages|forecast|renewable|reports|plants|power-plants|financial|generation|energy-balance|anomaly|iso50001|weather|icmal-imports|solar-tariffs)\b/g;
const CARBON_API = /\/api\/v1\/carbon\b/g;

function hits(route: string, re: RegExp): string[] {
  const found = new Set<string>();
  for (const chunk of pages[route] ?? []) {
    const file = join(NEXT, chunk);
    if (existsSync(file)) for (const m of readFileSync(file, 'utf8').matchAll(re)) found.add(m[0]);
  }
  return [...found];
}

const problems: string[] = [];
const routes = Object.keys(pages);
for (const route of routes.filter((r) => r.startsWith('/ekocm'))) {
  for (const h of hits(route, EKORM_API)) problems.push(`${route} loads EKORM code (${h})`);
}
for (const route of routes.filter((r) => r.startsWith('/ekorm'))) {
  for (const h of hits(route, CARBON_API)) problems.push(`${route} loads EKOCM code (${h})`);
}
// Positive controls: the scan must see each module's own API, or it proves nothing.
if (!hits('/ekocm/page', CARBON_API).length) problems.push('/ekocm/page: no carbon API found — is the build current?');
if (!hits('/ekorm/consumption/page', EKORM_API).length) problems.push('/ekorm/consumption/page: no EKORM API found — is the build current?');

if (problems.length) {
  console.error(`module chunk check failed:\n  ${problems.join('\n  ')}`);
  process.exit(1);
}
console.log(`module chunks ok — ${routes.filter((r) => r.startsWith('/ekocm')).length} EKOCM and ${routes.filter((r) => r.startsWith('/ekorm')).length} EKORM entries`);
