import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

import { login, USERS } from '../fixtures';

// F12b: every §7.19 page renders without a session, in both locales, with no axe violations.
const PAGES: [string, string][] = [
  ['/', 'Enerji, su ve yakıt tüketiminizi tek yerden yönetin'],
  ['/about', 'EkoKod Hakkında'],
  ['/references', 'Referanslarımız'],
  ['/documents', 'Dokümanlar'],
  ['/toolkit', 'Yazılım Çözümleri'],
  ['/request-demo', 'Demo talebi'],
  ['/contact', 'İletişim'],
  ['/blog', 'Blog'],
  ['/blog/elektrik-faturam-neden-yuksek-1', 'Elektrik Faturam Neden Yüksek?'],
  ['/bill-calculator', 'Elektrik fatura hesaplama'],
];

for (const [path, title] of PAGES) {
  test(`public page ${path} renders for a visitor without violations`, async ({ page }) => {
    const res = await page.goto(path);
    expect(res?.status()).toBe(200);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(title);
    await expect(page).toHaveTitle(/EkoKod/);
    const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
    expect(violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).join(', ')}`)).toEqual([]);
  });
}

test('the site renders in English too', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'NEXT_LOCALE', value: 'en', url: baseURL }]);
  await page.goto('/about');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('About EkoKod');
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
});

test('pricing stays off by default and legacy /site URLs move permanently', async ({ request }) => {
  expect((await request.get('/pricing')).status()).toBe(404);
  const moved = await request.get('/site/billCalculate', { maxRedirects: 0 });
  expect(moved.status()).toBe(308);
  expect(moved.headers().location).toBe('/bill-calculator');
});

test('robots and sitemap describe the public site', async ({ request }) => {
  const robots = await (await request.get('/robots.txt')).text();
  expect(robots).toContain('Disallow: /ekorm');
  expect(robots).toMatch(/Sitemap: .*\/sitemap\.xml/);
  const sitemap = await (await request.get('/sitemap.xml')).text();
  expect(sitemap).toContain('/blog/elektrik-faturam-neden-yuksek-2</loc>');
  expect(sitemap).toContain('/bill-calculator</loc>');
  expect(sitemap).not.toContain('/pricing</loc>');
});

test('the homepage carries Organization structured data and Open Graph', async ({ page, baseURL }) => {
  await page.goto('/');
  const ld = await page.locator('script[type="application/ld+json"]').first().textContent();
  expect(JSON.parse(ld ?? '[]')[0]).toMatchObject({ '@type': 'Organization', name: 'EkoKod' });
  await expect(page.locator('meta[property="og:title"]')).toHaveAttribute('content', /EkoKod/);
  await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', new RegExp(`^${baseURL}/?$`));
});

test('small screens navigate through the drawer', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto('/');
  await page.getByRole('button', { name: 'Menüyü aç' }).click();
  const drawer = page.getByRole('dialog', { name: 'Menü' });
  await drawer.getByRole('link', { name: 'Blog' }).click();
  await expect(page).toHaveURL(/\/blog$/);
  await expect(drawer).toBeHidden();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(overflow).toBeLessThanOrEqual(0);
});

test('a signed-in user sees the way back to the platform (Q-H11)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/');
  await page.getByRole('link', { name: 'Platforma Git' }).click();
  await expect(page).toHaveURL(/\/ekorm/);
});
