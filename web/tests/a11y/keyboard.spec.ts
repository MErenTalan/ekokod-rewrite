import { expect, type Page, test } from '@playwright/test';

import { gotoStory, keyboardScope, loadStories, tabbables } from './helpers';

type Focus = { kb: string | null; guard: boolean; inScope: boolean; outline: string; width: number; area: number; tag: string } | null;

const readFocus = (page: Page, scope: string): Promise<Focus> =>
  page.evaluate((scope) => {
    const el = document.activeElement as HTMLElement | null;
    if (!el || el === document.body) return null;
    const s = getComputedStyle(el);
    const r = el.getBoundingClientRect();
    return {
      kb: el.dataset.kb ?? null,
      guard: el.hasAttribute('data-radix-focus-guard'),
      inScope: [...document.querySelectorAll(scope)].some((root) => root.contains(el)),
      outline: s.outlineStyle,
      width: parseFloat(s.outlineWidth),
      area: r.width * r.height,
      tag: el.outerHTML.slice(0, 80),
    };
  }, scope);

// Every tabbable in the story (or in the open modal) is reached by Tab/Shift+Tab and shows the global focus ring (07 §9).
for (const story of loadStories()) {
  test(`keyboard: ${story.title} › ${story.name}`, async ({ page }) => {
    await gotoStory(page, story.id, { theme: 'light', locale: 'tr' });
    const open = story.tags.includes('open');
    const scope = await keyboardScope(page);
    const expected = await tabbables(page, scope);
    const reached = new Set<string>();
    const start = await readFocus(page, scope);
    if (open) expect(start?.inScope, 'focus starts inside the open overlay').toBe(true);

    const record = (focus: Focus) => {
      expect(focus!.outline, `visible focus ring on ${focus!.tag}`).not.toBe('none');
      expect(focus!.width, `ring width on ${focus!.tag}`).toBeGreaterThanOrEqual(2);
      expect(focus!.area, `focused element has a box: ${focus!.tag}`).toBeGreaterThan(0);
      if (focus!.kb) reached.add(focus!.kb);
    };
    if (start?.inScope && start.kb) record(start);

    // Forward from the initial focus, then (for non-trapping overlays) backward from it.
    for (const key of start?.kb ? ['Tab', 'Shift+Tab'] : ['Tab']) {
      if (key === 'Shift+Tab') await page.locator(`[data-kb="${start!.kb}"]`).focus();
      const seen = new Set<string>(start?.kb ? [start.kb] : []);
      for (let i = 0; i < expected.length * 2 + 4; i++) {
        await page.keyboard.press(key);
        const focus = await readFocus(page, scope);
        if (focus?.guard) continue; // Radix focus guards are invisible hops at the document edges
        if (!focus || !focus.inScope || (focus.kb && seen.has(focus.kb))) break;
        record(focus);
        if (focus.kb) seen.add(focus.kb);
      }
      if (expected.every((kb) => reached.has(kb))) break;
    }
    expect(expected.filter((kb) => !reached.has(kb)), 'tabbables never reached').toEqual([]);

    if (open) {
      await page.locator(`[data-kb="${start!.kb}"]`).focus();
      await page.keyboard.press('Escape');
      // Radix restores focus after the close animation/unmount, so poll.
      await expect
        .poll(
          () =>
            page.evaluate(() => {
              const el = document.activeElement;
              return !!el && el !== document.body && !!document.querySelector('#storybook-root')?.contains(el);
            }),
          { message: 'Escape returns focus to the opener' },
        )
        .toBe(true);
    }
  });
}
