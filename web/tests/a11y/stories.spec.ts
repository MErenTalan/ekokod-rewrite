import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

import { expectNoClippedText, expectNoHorizontalScroll, gotoStory, LOCALES, loadStories, THEMES } from './helpers';

// Full theme × locale matrix for each component's first story and every `open` story; the rest run the
// light/tr and dark/en diagonal, which still covers both themes and both locales (plan I-18).
const stories = loadStories();
const firstOfTitle = new Set(stories.filter((s, i) => stories.findIndex((o) => o.title === s.title) === i).map((s) => s.id));

for (const story of stories) {
  const full = firstOfTitle.has(story.id) || story.tags.includes('open');
  const runs = full
    ? THEMES.flatMap((theme) => LOCALES.map((locale) => ({ theme, locale })))
    : [{ theme: 'light' as const, locale: 'tr' as const }, { theme: 'dark' as const, locale: 'en' as const }];
  for (const { theme, locale } of runs) {
    test(`${story.title} › ${story.name} [${theme}/${locale}]`, async ({ page }) => {
      const errors: string[] = [];
      page.on('console', (m) => {
        if (m.type() === 'error') errors.push(m.text());
      });
      page.on('pageerror', (e) => errors.push(e.message));
      await gotoStory(page, story.id, { theme, locale });
      await expect(page.locator('#storybook-root')).toBeVisible();
      await expect(page.locator('body')).not.toHaveClass(/sb-show-errordisplay/);
      let axe = new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
        .exclude('#storybook-docs');
      if (!story.title.startsWith('Shell/')) axe = axe.disableRules(['region', 'landmark-one-main', 'page-has-heading-one']);
      const { violations } = await axe.analyze();
      expect(violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).join(', ')}`)).toEqual([]);
      await expectNoHorizontalScroll(page);
      await expectNoClippedText(page);
      expect(errors).toEqual([]);
    });
  }
}
