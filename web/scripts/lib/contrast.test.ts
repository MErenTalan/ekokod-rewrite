// @vitest-environment node
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import { exempt, pairs } from '../contrast-pairs.ts';
import { contrastRatio, evaluate, parseTokens } from './contrast.ts';

describe('contrast', () => {
  it('contrastRatio matches WCAG reference values', () => {
    expect(contrastRatio('#FFFFFF', '#000000')).toBe(21);
    expect(Math.round(contrastRatio('#767676', '#FFFFFF') * 100) / 100).toBe(4.54);
  });

  it('parseTokens reads light and dark from light-dark()', () => {
    expect(parseTokens('--color-primary: light-dark(#047857, #34D399);')).toEqual({
      primary: { light: '#047857', dark: '#34D399' },
    });
  });

  it("evaluate fails the spec's original primary pair", () => {
    const tokens = parseTokens(
      '--color-primary: light-dark(#059669, #34D399); --color-on-primary: light-dark(#FFFFFF, #04211A);',
    );
    expect(evaluate(tokens, [['on-primary', 'primary', 4.5]], {})).toEqual(['on-primary/primary light 3.77 < 4.5']);
  });

  it('evaluate reports a colour token no pair covers', () => {
    const tokens = parseTokens(
      '--color-primary: light-dark(#047857, #34D399); --color-on-primary: light-dark(#FFFFFF, #04211A); --color-orphan: light-dark(#000000, #FFFFFF);',
    );
    expect(evaluate(tokens, [['on-primary', 'primary', 4.5]], {})).toContain('uncovered token: orphan');
  });

  it('the committed tokens pass', () => {
    const css = readFileSync(join(import.meta.dirname, '../../src/styles/tokens.css'), 'utf8');
    expect(evaluate(parseTokens(css), pairs, exempt)).toEqual([]);
  });
});
