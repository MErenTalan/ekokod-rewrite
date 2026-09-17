import { expect, test } from '@playwright/test';

import { gotoStory } from './helpers';

const seconds = (v: string) => parseFloat(v) * (v.endsWith('ms') ? 0.001 : 1);

test('Button hover transition is 150 ms and collapses under reduced motion', async ({ page }) => {
  await gotoStory(page, 'ui-button--variants', { theme: 'light', locale: 'tr' });
  const duration = () => page.locator('#storybook-root button').first().evaluate((el) => getComputedStyle(el).transitionDuration);
  expect(await duration()).toBe('0.15s');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  expect(seconds(await duration())).toBeLessThanOrEqual(0.001);
});

test('IconButton tooltip animation collapses under reduced motion', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await gotoStory(page, 'ui-iconbutton--default', { theme: 'light', locale: 'tr' });
  await page.locator('#storybook-root button').first().focus();
  const content = page.locator('[data-radix-popper-content-wrapper] > [data-state]').first();
  await expect(content).toBeVisible();
  expect(seconds(await content.evaluate((el) => getComputedStyle(el).animationDuration))).toBeLessThanOrEqual(0.001);
});
