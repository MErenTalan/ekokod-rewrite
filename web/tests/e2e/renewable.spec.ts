import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

// The fixture meters record import registers only: generation figures are
// absent and every panel must say so with its reason, never show 0 (R165, R292).

test('a building admin reads their analyzer: summary, production and the detailed panels', async ({ page }) => {
  await login(page, USERS.buildingAdmin.email);
  await page.goto('/ekorm/renewable-energy');
  await expect(page.getByRole('heading', { name: 'Yenilenebilir Enerji', level: 1 })).toBeVisible();
  await expect(page.getByText('Aktif üretim')).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole('table', { name: 'Üretim tablosu' })).toBeVisible();

  await page.getByRole('tab', { name: 'Detaylı analiz' }).click();
  await expect(page.getByRole('heading', { name: 'Sistem durumu' })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText('Gerilim ve frekans ölçülmüyor.').first()).toBeVisible();
  await expect(page.getByText('Batarya filtresi kullanılamıyor: batarya ölçümü yok.')).toBeVisible();
  // R293: the equivalences are seeded with their sources.
  await expect(page.getByText(/Kaynak: European Environment Agency/).first()).toBeVisible();
  await expect(page.getByText(/kg CO2e\/kWh/)).toBeVisible();
  await expect(page.getByText(/Hava durumu|Konum|yapılandırılmamış/i).first()).toBeVisible();
});
