import { expect, test } from '@playwright/test';

import { gotoStory } from './helpers';

const seconds = (v: string) => parseFloat(v) * (v.endsWith('ms') ? 0.001 : 1);
const dialogDuration = async (page: import('@playwright/test').Page) =>
  seconds(await page.getByRole('dialog').evaluate((el) => getComputedStyle(el).animationDuration));

// Overlays animate through the T1 keyframe tokens: 250 ms in, collapsed under reduced motion (07 §8, plan I-14).
test('Dialog enters over 250 ms', async ({ page }) => {
  await gotoStory(page, 'ui-dialog--default', { theme: 'light', locale: 'tr' });
  await page.locator('#storybook-root button').first().click();
  expect(await dialogDuration(page)).toBeCloseTo(0.25, 3);
});

test('Dialog animation collapses under reduced motion', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await gotoStory(page, 'ui-dialog--open', { theme: 'light', locale: 'tr' });
  expect(await dialogDuration(page)).toBeLessThanOrEqual(0.001);
});
