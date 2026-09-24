import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

// 09 §F6: "Consumption table shows every column listed in §7.3", plus the
// exports and the grouped detailed graphs (R193).
test.describe('consumption', () => {
  test('shows the seeded rows, every column and exports them', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/consumption' });

    const table = page.getByRole('table', { name: 'Tüketim verileri' });
    await expect(table.locator('tbody tr').first()).toBeVisible();
    for (const header of ['Dönem', 'Aktif endeks', 'U1 endeks', 'Endüktif oran (%)', 'Maks. demant (kW)']) {
      await expect(table.getByRole('columnheader', { name: header, exact: false }).first()).toBeVisible();
    }

    const download = page.waitForEvent('download');
    await page.getByRole('button', { name: 'Dışa aktar' }).click();
    await page.getByRole('menuitem', { name: 'CSV' }).click();
    expect((await download).suggestedFilename()).toMatch(/\.csv$/);
  });

  test('groups the detailed graphs by weekday and weekend', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/consumption' });
    await page.getByRole('tab', { name: 'Detaylı grafikler' }).click();
    await page.getByRole('combobox', { name: 'Grupla' }).click();
    await page.getByRole('option', { name: 'Hafta içi / hafta sonu' }).click();
    await page.getByRole('button', { name: 'Veri tablosunu göster' }).first().click();
    const grouped = page.getByRole('table', { name: 'Gruplanmış tüketim' });
    await expect(grouped).toContainText('Hafta içi');
    await expect(grouped).toContainText('Hafta sonu');
  });

  test('a read-only role sees no alarm check and no refresh', async ({ page }) => {
    await login(page, USERS.companyReadonly.email, { next: '/ekorm/consumption' });
    await expect(page.getByRole('table', { name: 'Tüketim verileri' }).locator('tbody tr').first()).toBeVisible();
    await expect(page.getByRole('button', { name: 'Saatlik değerleri yenile' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: /İşlemler/ })).toHaveCount(0);
  });
});
