#!/usr/bin/env node
// Fails when the tr and en catalogues do not contain exactly the same keys.
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', 'web', 'messages');

const flatten = (obj, prefix = '') =>
  Object.entries(obj).flatMap(([key, value]) => {
    const path = prefix ? `${prefix}.${key}` : key;
    return value && typeof value === 'object' ? flatten(value, path) : [path];
  });

const load = (locale) => new Set(flatten(JSON.parse(readFileSync(join(root, `${locale}.json`), 'utf8'))));

const tr = load('tr');
const en = load('en');

const missingInEn = [...tr].filter((k) => !en.has(k));
const missingInTr = [...en].filter((k) => !tr.has(k));

if (missingInEn.length || missingInTr.length) {
  if (missingInEn.length) console.error(`Missing in en.json:\n  ${missingInEn.join('\n  ')}`);
  if (missingInTr.length) console.error(`Missing in tr.json:\n  ${missingInTr.join('\n  ')}`);
  process.exit(1);
}

console.log(`i18n parity ok — ${tr.size} keys in both locales`);
