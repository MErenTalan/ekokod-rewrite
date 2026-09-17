// pnpm check:i18n-parity — tr/en namespace files must agree on keys and ICU arguments (plan D7).
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

import { checkCatalogues, flatten } from './lib/i18n.ts';

const root = join(import.meta.dirname, '..', 'messages');
const load = (locale: string) =>
  Object.fromEntries(
    readdirSync(join(root, locale))
      .filter((f) => f.endsWith('.json'))
      .map((f) => [f.replace(/\.json$/, ''), JSON.parse(readFileSync(join(root, locale, f), 'utf8'))]),
  );

const tr = load('tr');
const en = load('en');
const problems = checkCatalogues(tr, en);
if (problems.length) {
  console.error(`i18n parity failed:\n  ${problems.join('\n  ')}`);
  process.exit(1);
}
const keys = Object.values(tr).reduce((n, ns) => n + flatten(ns).size, 0);
console.log(`i18n parity ok — ${Object.keys(tr).length} namespaces, ${keys} keys in both locales`);
