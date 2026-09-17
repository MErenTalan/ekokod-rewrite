import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

// 09 §F6 acceptance: all eight seasonal labels are correct (10 item 14), and the
// statistics tab exports the hourly matrix.
test.describe('load profile', () => {
  test('labels every seasonal profile from its key', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/load-profile' });
    await page.getByRole('tab', { name: 'Mevsimsel' }).click();

    for (const [key, label] of [
      ['winter_weekday', 'Kış – Hafta içi'],
      ['winter_weekend', 'Kış – Hafta sonu'],
      ['spring_weekday', 'İlkbahar – Hafta içi'],
      ['spring_weekend', 'İlkbahar – Hafta sonu'],
      ['summer_weekday', 'Yaz – Hafta içi'],
      ['summer_weekend', 'Yaz – Hafta sonu'],
      ['autumn_weekday', 'Sonbahar – Hafta içi'],
      ['autumn_weekend', 'Sonbahar – Hafta sonu'],
    ] as const) {
      await expect(page.locator(`figure[data-profile-key="${key}"]`)).toContainText(label);
    }
  });

  test('shows the daily curves and the statistics of every profile', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/load-profile' });
    await expect(page.locator('figure[data-profile-key="weekday"]')).toContainText('Hafta içi');
    await expect(page.getByText(/Hafta sonu günleri: .* · \d+ tatil dönemi/)).toBeVisible();

    await page.getByRole('tab', { name: 'Detaylar' }).click();
    // Every seasonal label also contains "Hafta içi", so match the profile cell itself.
    const table = page.getByRole('table', { name: 'Profil istatistikleri' });
    await expect(table.getByRole('cell', { name: 'Hafta içi', exact: true })).toBeVisible();
    await expect(table.getByRole('cell', { name: 'Yaz – Hafta sonu', exact: true })).toBeVisible();

    const download = page.waitForEvent('download');
    await page.getByRole('button', { name: 'Dışa aktar' }).click();
    await page.getByRole('menuitem', { name: 'Excel' }).click();
    expect((await download).suggestedFilename()).toMatch(/\.xlsx$/);
  });

  test('the demo user picks an analyzer and sees its profiles', async ({ page }) => {
    await login(page, USERS.demo.email, { next: '/ekorm/load-profile' });
    // The demo building has two analyzers, so none is chosen for the user.
    await expect(page.getByText('Yük profili için bir analizör seçin')).toBeVisible();
    await page.getByRole('combobox', { name: 'Analizör' }).click();
    await page.getByRole('option').first().click();
    await expect(page.locator('figure[data-profile-key="weekday"]')).toBeVisible();
  });
});
