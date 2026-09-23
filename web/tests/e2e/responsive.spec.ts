import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

// 07 §11: every screen fits its viewport at the four reference widths. What is
// too wide for a phone (a 29-column table, seven day columns) scrolls inside its
// own box; the page itself never scrolls sideways.
const WIDTHS = [375, 768, 1024, 1440];
const SCREENS = [
  ['dashboard', '/ekorm'],
  ['consumption', '/ekorm/consumption'],
  ['load-profile', '/ekorm/load-profile'],
  ['settings', '/ekorm/settings'],
  ['calendar', '/ekorm/calendar'],
  ['alarms', '/ekorm/alarms'],
  ['messages', '/ekorm/messages'],
  ['bills', '/ekorm/bills'],
  ['tariffs', '/ekorm/tariffs'],
  ['reports', '/ekorm/reports'],
  ['solar-plants', '/ekorm/solar-plants'],
  ['renewable-energy', '/ekorm/renewable-energy'],
  ['financial-analysis', '/ekorm/financial-analysis'],
  ['carbon-footprint', '/ekorm/carbon-footprint'],
] as const;

for (const width of WIDTHS) {
  test(`no page-level horizontal scroll at ${width}px`, async ({ page }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width, height: 900 });
    await login(page, USERS.companyAdmin.email);
    for (const [name, path] of SCREENS) {
      await page.goto(path);
      // networkidle never settles here: the screens keep polling.
      await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
      await page.waitForTimeout(500);
      const overflow = await page.evaluate(() => ({
        scroll: document.documentElement.scrollWidth,
        inner: window.innerWidth,
      }));
      expect(overflow.scroll, `${name} at ${width}px`).toBeLessThanOrEqual(overflow.inner + 1);
    }
  });
}
