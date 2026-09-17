// pnpm check:contrast — fails when any declared token pair misses its WCAG minimum in either theme.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { exempt, pairs } from './contrast-pairs.ts';
import { contrastRatio, evaluate, parseTokens } from './lib/contrast.ts';

const tokens = parseTokens(readFileSync(join(import.meta.dirname, '../src/styles/tokens.css'), 'utf8'));
for (const [fg, bg, min] of pairs) {
  for (const theme of ['light', 'dark'] as const) {
    const f = tokens[fg]?.[theme];
    const b = tokens[bg]?.[theme];
    if (!f || !b || !f.startsWith('#') || !b.startsWith('#')) continue;
    const ratio = contrastRatio(f, b);
    console.log(`${ratio >= min ? 'PASS' : 'FAIL'} ${fg}/${bg} ${theme} ${ratio.toFixed(2)} ${ratio >= min ? '≥' : '<'} ${min}`);
  }
}
const problems = evaluate(tokens, pairs, exempt);
if (problems.length) {
  console.error(`\ncontrast check failed:\n  ${problems.join('\n  ')}`);
  process.exit(1);
}
const lowest = pairs
  .flatMap(([fg, bg]) => (['light', 'dark'] as const).map((t) => ({ label: `${fg}/${bg} ${t}`, r: contrastRatio(tokens[fg][t], tokens[bg][t]) })))
  .sort((a, b) => a.r - b.r)[0];
console.log(`\ncontrast ok — ${pairs.length} pairs × 2 themes; lowest ${lowest.label} ${lowest.r.toFixed(2)}`);
