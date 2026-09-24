import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

// 09 §F13: the e2e API runs without the ML service (EKOKOD_ML_URL points nowhere), so these specs
// prove the acceptance's degradation end to end: forecasting shows as unavailable, nothing else breaks.
// The happy path is TestForecastRunReadAndScope (fake ML over the real API) and the web unit tests.
test.describe('forecasting', () => {
  test('Predict keeps the actuals and explains that forecasting is unavailable', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/forecast' });
    await expect(page.getByRole('heading', { level: 1, name: 'Tüketim Tahmini' })).toBeVisible();
    await expect(page.getByText(/kayıtlı tahmin yok/)).toBeVisible();
    await page.getByRole('button', { name: 'Tahmin et' }).click();
    await expect(page.getByText(/Tahmin servisi şu anda kullanılamıyor/)).toBeVisible({ timeout: 15_000 });
    await expect(page.getByRole('figure', { name: 'Gerçekleşen ve tahmin' })).toBeVisible();

    // No other feature breaks: the consumption screen still loads.
    await page.goto('/ekorm/consumption');
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  });

  test('the old /ekorm/predict path lands on the screen', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/predict' });
    await expect(page).toHaveURL(/\/ekorm\/forecast$/);
  });

  test('AI Analysis degrades per tab and read-only roles do not see it', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/ai' });
    await page.getByRole('tab', { name: 'Anomali kontrolü' }).click();
    await page.getByRole('button', { name: 'Kontrol et' }).click();
    await expect(page.getByText(/Tahmin servisi şu anda kullanılamıyor/)).toBeVisible({ timeout: 15_000 });
  });

  test('a read-only role reads Predict without running it and has no AI Analysis entry', async ({ page }) => {
    await login(page, USERS.companyReadonly.email, { next: '/ekorm/forecast' });
    await expect(page.getByText(/Yeni tahmin çalıştırma yetkiniz yok/)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Tahmin et' })).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'Yapay Zekâ Analizi' })).toHaveCount(0);
  });
});
