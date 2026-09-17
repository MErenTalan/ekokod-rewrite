// @vitest-environment node
import { readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

import * as fontkit from 'fontkit';
import { describe, expect, it } from 'vitest';

const families = ['lexend', 'source-sans-3', 'jetbrains-mono'];

describe('self-hosted fonts', () => {
  for (const family of families) {
    it(`${family} covers Turkish and is subset`, () => {
      const file = join(import.meta.dirname, 'fonts', `${family}.woff2`);
      const font = fontkit.create(readFileSync(file)) as fontkit.Font;
      for (const ch of 'ğĞşŞıİçÇöÖüÜ0123456789.,%') {
        expect(font.hasGlyphForCodePoint(ch.codePointAt(0)!), `${family} ${ch}`).toBe(true);
      }
      expect(font.hasGlyphForCodePoint(0x0416)).toBe(false);
      expect(statSync(file).size).toBeLessThanOrEqual(150_000);
      expect(font.variationAxes.wght).toBeDefined();
      console.info(`${family}: ₺ ${font.hasGlyphForCodePoint(0x20ba)}, ₂ ${font.hasGlyphForCodePoint(0x2082)}`);
    });
  }
});
