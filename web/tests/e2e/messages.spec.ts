import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

test('filters the seeded messages by type, status and search', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/messages');

  await expect(page.getByRole('heading', { level: 1, name: 'Tüm Mesajlar' })).toBeVisible();
  await expect(page.getByText('Alarm tetiklendi: Endüktif izleme')).toBeVisible();
  await expect(page.getByText('Oturum açıldı')).toBeVisible();

  await page.getByRole('combobox', { name: /Tip/ }).click();
  await page.getByRole('option', { name: 'Alarmlar' }).click();
  await expect(page.getByText('Oturum açıldı')).toHaveCount(0);
  await expect(page.getByText('Alarm tetiklendi: Endüktif izleme')).toBeVisible();

  await page.getByRole('combobox', { name: /Tip/ }).click();
  await page.getByRole('option', { name: 'Tümü' }).click();
  await page.getByRole('searchbox', { name: 'Arama' }).fill('Oturum');
  await expect(page.getByText('Oturum açıldı')).toBeVisible();
  await expect(page.getByText('Alarm tetiklendi: Endüktif izleme')).toHaveCount(0);
});

test('an admin can trigger a job (R220)', async ({ page }) => {
  await login(page, USERS.admin.email);
  await page.goto('/ekorm/messages');

  await page.getByRole('tab', { name: 'İş Geçmişi' }).click();
  // The admin's own scope is the platform company, which the seed gives no
  // runs: the history is empty until they pick a tenant, and the trigger is
  // what this test is about.
  await expect(page.getByText('Henüz iş kaydı yok.')).toBeVisible();

  await page.getByRole('button', { name: /alarm\.evaluate — Şimdi çalıştır/ }).click();
  // Either outcome is honest: the job is queued, or R225 reports that the same
  // hour's run is already queued — Redis outlives the e2e database, so a second
  // run inside the same hour legitimately hits the de-duplication. What must
  // NEVER appear is the generic 'unexpected error'.
  await expect(
    page.getByText(/İş kuyruğa alındı|Kuyruğa alındı|Çalışıyor|Tamamlandı|zaten kuyrukta/).first(),
  ).toBeVisible();
  await expect(page.getByText('Beklenmeyen bir hata oluştu.')).toHaveCount(0);
});

test('a company admin reads their own job history but cannot trigger', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/messages');

  await page.getByRole('tab', { name: 'İş Geçmişi' }).click();
  // The seeded runs are this tenant's: one success and one partial.
  await expect(page.getByRole('cell', { name: 'alarm.evaluate' })).toBeVisible();
  await expect(page.getByText('Kısmi')).toBeVisible();
  await expect(page.getByRole('button', { name: /Şimdi çalıştır/ })).toHaveCount(0);
});

test('a building admin sees messages but no job history', async ({ page }) => {
  await login(page, USERS.buildingAdmin.email);
  await page.goto('/ekorm/messages');

  await expect(page.getByRole('heading', { level: 1, name: 'Tüm Mesajlar' })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'İş Geçmişi' })).toHaveCount(0);
});
