import { expect, type Page, test } from '@playwright/test';

import { gotoStory, keyboardScope, loadStories, tabbables } from './helpers';

type Focus = { kb: string | null; guard: boolean; item: boolean; inScope: boolean; outline: string; width: number; area: number; tag: string; composite: boolean } | null;

const readFocus = (page: Page, scope: string): Promise<Focus> =>
  page.evaluate((scope) => {
    const el = document.activeElement as HTMLElement | null;
    if (!el || el === document.body) return null;
    const s = getComputedStyle(el);
    const r = el.getBoundingClientRect();
    return {
      kb: el.dataset.kb ?? null,
      guard: el.hasAttribute('data-radix-focus-guard') || (r.width <= 1 && r.height <= 1 && !el.textContent?.trim()),
      item: el.matches('[role="option"], [role="menuitem"], [role="menuitemcheckbox"], [role="menuitemradio"], [role="gridcell"], [role="tab"], [role="radio"], td button'),
      inScope: [...document.querySelectorAll(scope)].some((root) => root.contains(el)),
      outline: s.outlineStyle,
      width: parseFloat(s.outlineWidth),
      area: r.width * r.height,
      tag: el.outerHTML.slice(0, 80),
      // Chromium walks a time input's inner fields with the host still active.
      composite: el.matches('input[type="time"], input[type="date"], input[type="datetime-local"]'),
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
      // A time input is three inner fields to Chromium: it stays the active
      // element while Tab walks them and drops :focus-visible on the hop out,
      // so the ring is asserted on the field itself, not on that transition.
      if (!focus!.composite) {
        expect(focus!.outline, `visible focus ring on ${focus!.tag}`).not.toBe('none');
        expect(focus!.width, `ring width on ${focus!.tag}`).toBeGreaterThanOrEqual(2);
      }
      expect(focus!.area, `focused element has a box: ${focus!.tag}`).toBeGreaterThan(0);
      if (focus!.kb) reached.add(focus!.kb);
    };
    if (start?.inScope && start.kb) record(start);
    // Menus and listboxes focus their container; arrow keys move focus to items, which must show the ring too.
    if (open && start?.inScope && !start.kb && !start.item) {
      await page.keyboard.press('ArrowDown');
      const item = await readFocus(page, scope);
      if (item?.item) record(item);
    }

    // Forward from the initial focus, then (for non-trapping overlays) backward from it.
    for (const key of start?.kb ? ['Tab', 'Shift+Tab'] : ['Tab']) {
      if (key === 'Shift+Tab') await page.locator(`[data-kb="${start!.kb}"]`).focus();
      const seen = new Set<string>(start?.kb ? [start.kb] : []);
      for (let i = 0; i < expected.length * 4 + 8; i++) {
        await page.keyboard.press(key);
        const focus = await readFocus(page, scope);
        if (focus?.guard) continue; // Radix focus guards are invisible hops at the document edges
        // A composite input keeps the host focused while Tab walks its inner fields.
        if (focus?.composite && focus.kb && seen.has(focus.kb)) continue;
        if (!focus || !focus.inScope || (focus.kb && seen.has(focus.kb))) break;
        // A menu/listbox container keeping programmatic focus (Radix prevents Tab) ends the walk; items still need a ring.
        if (!focus.kb && !focus.item) break;
        record(focus);
        if (focus.kb) seen.add(focus.kb);
      }
      if (expected.every((kb) => reached.has(kb))) break;
    }
    expect(expected.filter((kb) => !reached.has(kb)), 'tabbables never reached').toEqual([]);

    if (open) {
      if (start?.kb) await page.locator(`[data-kb="${start.kb}"]`).focus();
      const restored = () =>
        page.evaluate(() => {
          const el = document.activeElement;
          return !!el && el !== document.body && !!document.querySelector('#storybook-root')?.contains(el);
        });
      // One Escape per layer (a tooltip inside a popover closes first); Radix restores focus after unmount.
      for (let i = 0; i < 3 && !(await restored()); i++) {
        await page.keyboard.press('Escape');
        await page.waitForTimeout(300);
      }
      expect(await restored(), 'Escape returns focus to the opener').toBe(true);
    }
  });
}
