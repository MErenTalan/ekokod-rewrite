import { expect, test } from '@playwright/test';

test('the blog filters by category and both legacy articles render with their images', async ({ page }) => {
  await page.goto('/blog');
  const articles = page.getByRole('main').getByRole('heading', { level: 2 });
  await expect(articles).toHaveText(['Elektrik Faturam Neden Yüksek? Yazı-2', 'Elektrik Faturam Neden Yüksek?']);

  await page.getByRole('link', { name: 'Doğal Gaz' }).click();
  await expect(page.getByText('Bu kategoride henüz yazı yok.')).toBeVisible();
  await page.getByRole('link', { name: 'Elektrik', exact: true }).click();
  await expect(articles).toHaveCount(2);

  for (const slug of ['elektrik-faturam-neden-yuksek-1', 'elektrik-faturam-neden-yuksek-2']) {
    await page.goto(`/blog/${slug}`);
    const images = page.locator('main img');
    await expect(images.first()).toBeVisible();
    for (const img of await images.all()) {
      await img.scrollIntoViewIfNeeded();
      await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth > 0), { message: await img.getAttribute('src') ?? '' }).toBe(true);
    }
  }
});

test('an article links its contents and footnotes', async ({ page }) => {
  await page.goto('/blog/elektrik-faturam-neden-yuksek-1');
  await page.getByRole('navigation', { name: 'İçindekiler' }).getByRole('link', { name: 'Güç Faktörü (cosφ)' }).click();
  await expect(page).toHaveURL(/#guc-faktoru-cos$/);
  await page.locator('#ref-1 a').click();
  await expect(page).toHaveURL(/#footnote-1$/);
  await expect(page.locator('#footnote-1')).toBeInViewport();
  expect((await page.request.get('/blog/unknown-article')).status()).toBe(404);
});
