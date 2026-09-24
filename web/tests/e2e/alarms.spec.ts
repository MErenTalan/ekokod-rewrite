import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

test('lists the seeded rules and never labels one with voltage (R212)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/alarms');

  await expect(page.getByRole('heading', { level: 1, name: 'Alarmlar' })).toBeVisible();
  await expect(page.getByText('Endüktif izleme')).toBeVisible();
  await expect(page.getByText('Veri İletişim Alarmı')).toBeVisible();
  await expect(page.getByText('Güç Alarmı')).toBeVisible();
  // Legacy's label promised two measurements the product never had.
  await expect(page.getByText('Akım - Voltaj - Güç')).toHaveCount(0);
});

test('filters by active and passive', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/alarms');
  await expect(page.getByText('Endüktif izleme')).toBeVisible();

  await page.getByRole('radio', { name: 'Pasif' }).click();
  // 'Güç sınırı' is the one disabled rule in the seed.
  await expect(page.getByText('Güç sınırı')).toBeVisible();
  await expect(page.getByText('Endüktif izleme')).toHaveCount(0);
});

test('creates a data-communication rule end to end', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/alarms');

  await page.getByRole('button', { name: 'Yeni Alarm Ekle' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Ad').fill('E2E iletişim');
  await dialog.getByRole('combobox', { name: /Tip/ }).click();
  await page.getByRole('option', { name: 'Veri İletişim Alarmı' }).click();
  await dialog.getByRole('combobox', { name: /Analizör seçiniz/ }).click();
  await page.getByRole('option').first().click();
  await page.keyboard.press('Escape');
  await dialog.getByLabel(/İletişim Kesinti Eşiği/).fill('6');
  await dialog.getByRole('button', { name: 'Kaydet' }).click();

  await expect(page.getByText('E2E iletişim')).toBeVisible();
});

test('refuses to save a rule with no limit (R230)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/alarms');

  await page.getByRole('button', { name: 'Yeni Alarm Ekle' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Ad').fill('Eksik');
  await expect(dialog.getByRole('button', { name: 'Kaydet' })).toBeDisabled();
});

test('says SMS is not sent yet when the channel is ticked (R211)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/alarms');

  await page.getByRole('button', { name: 'Yeni Alarm Ekle' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('checkbox', { name: 'SMS' }).click();
  await expect(dialog.getByText(/SMS bildirimi henüz etkin değil/)).toBeVisible();
});

test('shows a dry-run verdict and the alarm log', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/alarms');

  const row = page.getByRole('row', { name: /Endüktif izleme/ });
  await row.getByRole('button', { name: 'Şimdi değerlendir' }).click();
  await expect(page.getByText(/Bu bir denemedir/)).toBeVisible();
  await page.getByRole('button', { name: 'Kapat' }).click();

  await row.getByRole('button', { name: 'Detaylar' }).click();
  // The seed has one delivered firing and one that reached nobody. Scoped to
  // the dialog and counted, because both states must be present exactly once:
  // an earlier version of this screen stamped a failed delivery as delivered,
  // and a laxer matcher would not have noticed.
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByText('Gönderildi', { exact: true })).toHaveCount(1);
  await expect(dialog.getByText('Gönderilemedi', { exact: true })).toHaveCount(1);
});

test('a read-only role sees no write control', async ({ page }) => {
  await login(page, USERS.companyReadonly.email);
  await page.goto('/ekorm/alarms');

  await expect(page.getByText('Endüktif izleme')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Yeni Alarm Ekle' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Şimdi değerlendir' })).toHaveCount(0);
});
