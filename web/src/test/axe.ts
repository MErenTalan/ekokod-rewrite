import axe from 'axe-core';
import { expect } from 'vitest';

/** jsdom cannot compute colour or page regions; those are checked in Playwright (tests/a11y). */
export async function expectNoAxeViolations(el: Element): Promise<void> {
  const { violations } = await axe.run(el, {
    rules: { 'color-contrast': { enabled: false }, region: { enabled: false } },
  });
  expect(violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).join(', ')}`)).toEqual([]);
}
