import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

// internal/seed/solar.go: the grid plant is linked to a stub credential with
// two inverters (42,5 + 18,25 kW), sixty days of totals and two faults.

test('the linked plant shows its inverters, production and revenue (§7.7)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/solar-plants');
  await expect(page.getByRole('heading', { name: 'GES Santralleri', level: 1 })).toBeVisible();
  await expect(page.getByText('60,75').first()).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText(/Son eşitleme|Henüz eşitlenmedi|Bağlantı hatası/).first()).toBeVisible();
  await expect(page.getByRole('region', { name: 'Gelir' })).toBeVisible();
  await expect(page.getByText('7.030,75 kWh')).toBeVisible(); // this month: Σ inverters' month yields

  await page.getByRole('tab', { name: 'Cihazlar' }).click();
  await expect(page.getByRole('cell', { name: 'E2E-INV-1' })).toBeVisible();
  await expect(page.getByRole('cell', { name: 'Depolamalı inverter' })).toBeVisible();

  await page.getByRole('tab', { name: 'Alarmlar' }).click();
  await expect(page.getByText('String akımı düşük').first()).toBeVisible();
});

test('"Verileri güncelle" without a reachable iSolarCloud ends in a sentence, not a crash', async ({ page }) => {
  test.setTimeout(120_000);
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/solar-plants');
  await page.getByRole('button', { name: 'Verileri güncelle' }).click();
  await expect(page.getByText(/kimlik bilgisi yok|yetkilendirmesi geçersiz|yanıt vermiyor/).first()).toBeVisible({ timeout: 90_000 });
  await expect(page.getByRole('heading', { name: 'GES Santralleri', level: 1 })).toBeVisible();
});

test('a company reader sees the plant but cannot trigger a sync (R296)', async ({ page }) => {
  await login(page, USERS.companyReadonly.email);
  await page.goto('/ekorm/solar-plants');
  await expect(page.getByText('60,75').first()).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole('button', { name: 'Verileri güncelle' })).toHaveCount(0);
});
