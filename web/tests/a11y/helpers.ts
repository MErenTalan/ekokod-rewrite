import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { expect, type Locator, type Page } from '@playwright/test';

export type StoryEntry = { id: string; title: string; name: string; tags: string[] };
export const THEMES = ['light', 'dark'] as const;
export const LOCALES = ['tr', 'en'] as const;
export type Theme = (typeof THEMES)[number];
export type Locale = (typeof LOCALES)[number];

/** Stories from storybook-static/index.json, minus `no-sweep`, filtered by STORIES title prefixes. */
export function loadStories(): StoryEntry[] {
  const index = JSON.parse(readFileSync(join(import.meta.dirname, '../../storybook-static/index.json'), 'utf8')) as {
    entries: Record<string, StoryEntry & { type: string }>;
  };
  const prefixes = (process.env.STORIES ?? '').split(',').map((s) => s.trim()).filter(Boolean);
  return Object.values(index.entries)
    .filter((e) => e.type === 'story' && !e.tags.includes('no-sweep'))
    .filter((e) => prefixes.length === 0 || prefixes.some((p) => e.title.startsWith(p)))
    .map(({ id, title, name, tags }) => ({ id, title, name, tags }));
}

/** Navigates and waits until Storybook's render (including `play`) has finished. */
export async function gotoStory(page: Page, id: string, o: { theme: Theme; locale: Locale }) {
  await page.goto(`/iframe.html?id=${id}&viewMode=story&globals=theme:${o.theme};locale:${o.locale}`);
  await page.waitForFunction(
    () => {
      const phase = (window as unknown as { __STORYBOOK_PREVIEW__?: { currentRender?: { phase?: string } } })
        .__STORYBOOK_PREVIEW__?.currentRender?.phase;
      return phase === 'completed' || phase === 'afterEach' || phase === 'finished' || phase === 'errored';
    },
    undefined,
    { timeout: 20_000 },
  );
  await page.evaluate(() => document.fonts.ready);
}

export async function expectNoHorizontalScroll(page: Page) {
  const { scrollWidth, innerWidth } = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }));
  expect(scrollWidth, 'page scrolls horizontally').toBeLessThanOrEqual(innerWidth);
}

/** Text cut off by overflow hidden/clip without an ellipsis. */
export async function expectNoClippedText(page: Page) {
  const clipped = await page.evaluate(() =>
    [...document.querySelectorAll<HTMLElement>('#storybook-root *, [data-radix-popper-content-wrapper] *')]
      .filter((el) => {
        const s = getComputedStyle(el);
        return (
          el.scrollWidth > el.clientWidth + 1 &&
          (s.overflowX === 'hidden' || s.overflowX === 'clip') &&
          s.textOverflow !== 'ellipsis' &&
          !el.classList.contains('sr-only') &&
          (el.textContent ?? '').trim() !== ''
        );
      })
      .map((el) => `${el.tagName.toLowerCase()}.${[...el.classList].slice(0, 4).join('.')}: ${el.textContent?.trim().slice(0, 40)}`),
  );
  expect(clipped, 'clipped text').toEqual([]);
}

const TABBABLE =
  'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), summary';

// Radix modals mark siblings aria-hidden instead of setting aria-modal; popper content (Popover) is non-modal.
const MODAL = '[aria-modal="true"], body > [role="dialog"], body > [role="alertdialog"], body > div > [role="dialog"], body > div > [role="alertdialog"]';

/**
 * Keyboard scope: an open modal; else popper content holding focus (Radix loops Tab inside it); else the story
 * root plus portalled popper content (plan I-7).
 */
export async function keyboardScope(page: Page): Promise<string> {
  const kind = await page.evaluate((MODAL) => {
    if ([...document.querySelectorAll(MODAL)].some((el) => !el.closest('[data-radix-popper-content-wrapper], #storybook-root'))) {
      return 'modal';
    }
    return document.activeElement?.closest('[data-radix-popper-content-wrapper]') ? 'popper' : 'page';
  }, MODAL);
  return kind === 'modal' ? MODAL : kind === 'popper' ? '[data-radix-popper-content-wrapper]' : '#storybook-root, [data-radix-popper-content-wrapper]';
}

/** Visible, focusable elements in the scope, tagged with data-kb so the Tab walk can identify them. */
export async function tabbables(page: Page, scope?: Locator | string): Promise<string[]> {
  const selector = typeof scope === 'string' ? scope : '#storybook-root, [data-radix-popper-content-wrapper]';
  return page.evaluate(
    ({ selector, TABBABLE }) => {
      const ids: string[] = [];
      let n = 0;
      for (const root of document.querySelectorAll(selector)) {
        for (const el of root.querySelectorAll<HTMLElement>(TABBABLE)) {
          const rect = el.getBoundingClientRect();
          const style = getComputedStyle(el);
          if (el.closest('[inert], [aria-hidden="true"]') || style.visibility === 'hidden' || style.display === 'none') continue;
          if (rect.width === 0 && rect.height === 0 && !el.hasAttribute('data-skip-link')) continue;
          if (el.tabIndex < 0) continue;
          el.dataset.kb ??= String(n++);
          ids.push(el.dataset.kb);
        }
      }
      return ids;
    },
    { selector, TABBABLE },
  );
}
