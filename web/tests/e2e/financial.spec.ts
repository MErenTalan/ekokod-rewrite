import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

test('the company view: headline, netting, tariffs and the twelve-month table (R294)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/financial-analysis');
  await expect(page.getByRole('heading', { name: 'Finansal Analiz', level: 1 })).toBeVisible();
  await expect(page.getByText(/12 ayın \d+'/)).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole('heading', { name: 'Mahsuplaşma (hesaplanan)' })).toBeVisible();
  await expect(page.getByText('E2E Arazi GES')).toBeVisible();
  const table = page.getByRole('table', { name: 'Aylık finansal tablo' });
  await expect(table.getByRole('row')).toHaveCount(14);
  await expect(table.getByRole('row', { name: /Yıl toplamı/ })).toContainText('₺');
});

test('a building admin has no financial screen (R296)', async ({ page }) => {
  await login(page, USERS.buildingAdmin.email);
  await page.goto('/ekorm/financial-analysis');
  await expect(page.getByText('Finansal analiz şirket düzeyindedir')).toBeVisible({ timeout: 30_000 });
});
