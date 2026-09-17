import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

import { expectNoHorizontalScroll, gotoStory, THEMES } from './helpers';

// §F5 acceptance: the shell at 375/768/1024/1440 in both themes (07 §7, plan I-16, D24).
const STORIES = ['default', 'horizontal', 'collapsed', 'boxed'];
const WIDTHS = [375, 768, 1024, 1440];
const NOT_YET = 'Bu bölüm henüz kullanıma açılmadı';

for (const story of STORIES) {
  for (const width of WIDTHS) {
    for (const theme of THEMES) {
      test(`shell ${story} @${width} ${theme}`, async ({ page }) => {
        await page.setViewportSize({ width, height: 900 });
        await gotoStory(page, `shell-appshell--${story}`, { theme, locale: 'tr' });

        // The skip link is the first tab stop, checked before anything moves the focus starting point.
        await page.keyboard.press('Tab');
        await expect(page.locator(':focus')).toHaveAttribute('data-skip-link');
        await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());

        await expectNoHorizontalScroll(page);

        const nav = page.getByRole('navigation', { name: 'Ana menü' });
        if (width >= 1024) {
          await expect(nav).toBeVisible();
        } else {
          await expect(nav).toBeHidden();
          await page.getByRole('button', { name: 'Menüyü Aç/Kapat' }).click();
          await expect(page.getByRole('dialog', { name: 'Ana menü' }).getByRole('navigation', { name: 'Ana menü' })).toBeVisible();
        }

        await page.locator('[role="link"][aria-disabled="true"]:visible').first().focus();
        await expect(page.getByRole('tooltip')).toContainText(NOT_YET);
        await page.keyboard.press('Escape');
        // Detached, not merely hidden: axe reads a tooltip mid-exit as low contrast.
        await expect(page.getByRole('tooltip')).toHaveCount(0);

        if (width < 1024) {
          await page.getByRole('dialog').getByRole('button', { name: 'Kapat' }).click();
          await expect(page.getByRole('dialog')).toHaveCount(0);
        }
        // Closing the drawer hands focus back to the disabled link, which opens
        // its tooltip again. Axe reads a fade in flight as a blend of the two
        // colours and calls it low contrast, so measure the settled state.
        await page.addStyleTag({ content: '*, *::before, *::after { animation: none !important; transition: none !important; }' });

        const { violations } = await new AxeBuilder({ page })
          .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
          .exclude('#storybook-docs')
          .analyze();
        expect(violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).join(', ')}`)).toEqual([]);

        await page.screenshot({ path: `test-results/shell-${story}-${width}-${theme}.png` });
      });
    }
  }
}
