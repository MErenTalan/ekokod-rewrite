import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { StaggerGrid } from './stagger-grid';

describe('StaggerGrid', () => {
  it('numbers its children so the CSS can delay each one (D26)', () => {
    const r = renderWithProviders(
      <StaggerGrid className="md:grid-cols-3">
        <p>bir</p>
        <p>iki</p>
        <p>üç</p>
      </StaggerGrid>,
    );
    const items = r.container.querySelectorAll('.stagger-item');
    expect(items).toHaveLength(3);
    expect([...items].map((el) => (el as HTMLElement).style.getPropertyValue('--i'))).toEqual(['0', '1', '2']);
  });

  it('animates only when motion is welcome', () => {
    const css = readFileSync(join(import.meta.dirname, '..', '..', 'app', 'globals.css'), 'utf8');
    const block = css.slice(css.indexOf('.stagger-item') - 200, css.indexOf('.stagger-item'));
    expect(block).toContain('prefers-reduced-motion: no-preference');
    expect(css).toContain('calc(var(--i, 0) * var(--duration-stagger-step))');
  });
});
