// @vitest-environment node
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';

import { describe, expect, it } from 'vitest';

const src = join(import.meta.dirname, '..');
const RAW_COLOR = /(?<![\w&-])#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})(?![\w-])|\b(?:rgba?|hsla?|oklch|oklab|lab|lch|hwb)\(/g;

function cssFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? cssFiles(join(dir, e.name)) : e.name.endsWith('.css') ? [join(dir, e.name)] : [],
  );
}

describe('CSS colour literals', () => {
  it('no CSS file except tokens.css holds a colour literal', () => {
    const hits = cssFiles(src)
      .filter((f) => relative(src, f) !== join('styles', 'tokens.css'))
      .flatMap((f) => (readFileSync(f, 'utf8').match(RAW_COLOR) ?? []).map((m) => `${relative(src, f)}: ${m}`));
    expect(hits).toEqual([]);
  });
});
