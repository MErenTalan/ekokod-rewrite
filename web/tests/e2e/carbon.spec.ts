import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

// F10: the seeded company A, building A1 declares six activities and holds four manual records (seed/carbon.go).
// F17: each section is its own /ekocm page (R488).

test('overview shows the seeded records and "see all" opens the status page (R322)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email, { next: '/ekocm' });
  await expect(page.getByRole('heading', { name: 'Genel Bakış', level: 1 })).toBeVisible();
  await expect(page.getByText('Toplam karbon ayak izi')).toBeVisible({ timeout: 30_000 });
  const recent = page.getByRole('table', { name: 'Son faaliyetler' });
  await expect(recent.getByRole('row', { name: /Ortam Isıtması/ })).toBeVisible();
  await page.getByRole('button', { name: 'Tümünü gör' }).click();
  await expect(page).toHaveURL(/\/ekocm\/activity-status$/);
});

test('a pending record is approved on the status page (R325)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekocm/activity-status');
  const row = page.getByRole('row', { name: /İş Seyahati/ }).first();
  await expect(row).toBeVisible({ timeout: 30_000 });
  await row.getByRole('button', { name: 'Onayla' }).click();
  await expect(page.getByText('Kayıt onaylandı', { exact: true })).toBeVisible();
  await expect(page.getByRole('row', { name: /İş Seyahati/ }).first()).toContainText('Onaylandı');
});

test('a GHG report is generated and its PDF downloads (R312, R326)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekocm/reports');
  await page.getByRole('button', { name: 'Raporu oluştur' }).click({ timeout: 30_000 });
  await expect(page.getByText('Rapor oluşturuldu', { exact: true })).toBeVisible();
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: /PDF indir$/ }).first().click();
  expect((await download).suggestedFilename()).toMatch(/^carbon-ghg-.*\.pdf$/);
});

test('a factor is overridden for the company and restored (R303, R304)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekocm/emission-factors');
  await page.getByRole('searchbox', { name: 'Faktör ara' }).fill('grid_electricity');
  await page.getByRole('button', { name: /faktörünü değiştir/ }).first().click({ timeout: 30_000 });
  const dialog = page.getByRole('dialog', { name: 'Faktörü değiştir' });
  await dialog.getByRole('textbox', { name: /Faktör değeri/ }).fill('0,44');
  await dialog.getByRole('button', { name: 'Kaydet' }).click();
  await expect(page.getByText('Şirkete özel')).toBeVisible();
  await page.getByRole('button', { name: 'Varsayılana dön' }).first().click();
  await page.getByRole('dialog').getByRole('button', { name: 'Varsayılana dön' }).click();
  await expect(page.getByText('Şirkete özel')).toHaveCount(0);
});

test('a building admin reads the module without write controls (R314)', async ({ page }) => {
  await login(page, USERS.buildingAdmin.email);
  await page.goto('/ekocm/activity-status');
  await expect(page.getByRole('table', { name: 'Faaliyet kayıtları' })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole('button', { name: 'Onayla' })).toHaveCount(0);
  await page.goto('/ekocm/activity-selection');
  await expect(page.getByText('Faaliyet seçimini yalnızca şirket yöneticileri değiştirebilir.')).toBeVisible();
});
