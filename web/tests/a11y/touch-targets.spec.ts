import { expect, test } from '@playwright/test';

import { gotoStory, loadStories, tabbables } from './helpers';

// Under pointer: coarse every target is at least 44×44 (07 §9, plan D13).
test.use({ hasTouch: true, isMobile: true, viewport: { width: 375, height: 812 } });

for (const story of loadStories()) {
  test(`touch targets: ${story.title} › ${story.name}`, async ({ page }) => {
    await gotoStory(page, story.id, { theme: 'light', locale: 'tr' });
    const ids = await tabbables(page);
    const small: string[] = [];
    for (const kb of ids) {
      const box = await page.evaluate((kb) => {
        const el = document.querySelector<HTMLElement>(`[data-kb="${kb}"]`);
        // Tab panels and scroll regions are keyboard stops, not tap targets.
        if (!el || el.matches('[role="tabpanel"], [role="region"]')) return null;
        el.focus(); // reveals focus-only elements such as the skip link (M-6)
        const target = el.closest<HTMLElement>('[data-touch-target], label') ?? el;
        const r = target.getBoundingClientRect();
        return { w: r.width, h: r.height, tag: el.outerHTML.slice(0, 80) };
      }, kb);
      if (box && (box.w < 44 || box.h < 44)) small.push(`${Math.round(box.w)}×${Math.round(box.h)} ${box.tag}`);
    }
    expect(small).toEqual([]);
  });
}
