import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';

import { describe, expect, it } from 'vitest';

import owners from './feature-owners.json';

const SRC = join(import.meta.dirname, '..');
const IMPORT = /(?:from|import)\s*\(?\s*['"]([^'"]+)['"]/g;
// Type-only imports are erased at build time: they load no code.
const TYPE_IMPORT = /(?:import|export)\s+type\s[^;]*?from\s*['"][^'"]+['"]/gs;

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return files(path);
    return /\.tsx?$/.test(name) && !/\.(test|stories)\.tsx?$/.test(name) ? [path] : [];
  });
}

function resolve(from: string, spec: string): string | null {
  const base = spec.startsWith('@/') ? join(SRC, spec.slice(2)) : spec.startsWith('.') ? join(dirname(from), spec) : null;
  if (!base) return null;
  for (const candidate of [base, `${base}.ts`, `${base}.tsx`, join(base, 'index.ts'), join(base, 'index.tsx')]) {
    if (existsSync(candidate) && statSync(candidate).isFile() && /\.tsx?$/.test(candidate)) return candidate;
  }
  return null;
}

/** Every source file a set of entry files loads, transitively. */
function reach(entries: string[]): Set<string> {
  const seen = new Set<string>();
  const queue = [...entries];
  while (queue.length) {
    const file = queue.pop()!;
    if (seen.has(file)) continue;
    seen.add(file);
    for (const [, spec] of readFileSync(file, 'utf8').replace(TYPE_IMPORT, '').matchAll(IMPORT)) {
      const next = resolve(file, spec);
      if (next) queue.push(next);
    }
  }
  return seen;
}

const featuresOf = (reached: Set<string>) =>
  new Set([...reached].map((f) => /^features\/([^/]+)\//.exec(relative(SRC, f))?.[1]).filter((f): f is string => !!f));

describe('module boundary (F17 R490)', () => {
  it('classifies every feature folder', () => {
    const all = [...owners.ekorm, ...owners.ekocm, ...owners.shared].sort();
    expect(readdirSync(join(SRC, 'features')).sort()).toEqual(all);
  });

  it('EKOCM pages load no EKORM feature code', () => {
    const reached = featuresOf(reach(files(join(SRC, 'app/ekocm'))));
    expect([...reached].filter((f) => owners.ekorm.includes(f))).toEqual([]);
    expect(reached).toContain('carbon');
  });

  it('EKORM pages load no EKOCM feature code', () => {
    const reached = featuresOf(reach(files(join(SRC, 'app/ekorm'))));
    expect([...reached].filter((f) => owners.ekocm.includes(f))).toEqual([]);
    expect(reached).toContain('consumption');
  });
});
