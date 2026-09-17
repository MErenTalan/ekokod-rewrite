import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

// 09 §F6: the dashboard renders with real seeded data (R194) and the sectoral
// comparison is visible, populated, ranked and exportable (10 §13).
test.describe('dashboard', () => {
  test('a company admin sees their own buildings, the latest bill and the sectoral comparison', async ({ page }) => {
    await login(page, USERS.companyAdmin.email);

    const list = page.getByRole('region', { name: 'Binalar' }).first();
    await expect(page.getByRole('checkbox', { name: 'A1 Fabrika' })).toBeVisible();
    await expect(page.getByRole('checkbox', { name: 'A2 Depo' })).toBeVisible();
    await expect(page.getByRole('checkbox', { name: 'B1 Ofis' })).toHaveCount(0);
    await expect(page.locator('[data-map-counts]')).toContainText('Aktif 2');

    await page.getByRole('button', { name: 'Seç' }).first().click();
    await expect(page.getByText('₺48.250,75')).toBeVisible();

    await expect(page.getByText(/Sektör: Üretim · \d+ bina/)).toBeVisible();
    await expect(page.getByText(/Aylık tüketim \(kWh\/ay\): \d+ bina içinde \d+\./)).toBeVisible();

    const download = page.waitForEvent('download');
    await page.getByRole('button', { name: 'Dışa aktar' }).click();
    await page.getByRole('menuitem', { name: 'CSV' }).click();
    const file = await download;
    expect(file.suggestedFilename()).toContain('sektor-karsilastirma.csv');
    void list;
  });

  test('the consumption panel shows seeded rows for the selected building', async ({ page }) => {
    await login(page, USERS.companyAdmin.email);
    await page.getByRole('button', { name: 'Seç' }).first().click();
    const table = page.getByRole('table', { name: 'Tüketim verileri' });
    await expect(table.getByRole('row')).not.toHaveCount(1);
  });

  test('a building admin only sees their own building', async ({ page }) => {
    await login(page, USERS.buildingAdmin.email);
    await expect(page.getByRole('checkbox', { name: 'A1 Fabrika' })).toBeVisible();
    await expect(page.getByRole('checkbox', { name: 'A2 Depo' })).toHaveCount(0);
  });

  test('the demo user sees only the synthetic company', async ({ page }) => {
    await login(page, USERS.demo.email);
    await expect(page.getByRole('checkbox', { name: 'Demo Fabrika' })).toBeVisible();
    await expect(page.getByRole('checkbox', { name: /A1 Fabrika|A2 Depo|B1 Ofis/ })).toHaveCount(0);
  });
});
