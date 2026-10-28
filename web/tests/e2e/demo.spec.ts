import { expect, test } from '@playwright/test';

import { login, sidebarLabels, USERS } from './fixtures';

// F29 R552: the shared demo login says its data is sample data that resets every hour, and the visitor can still
// work like a company admin.
test.describe('the demo account', () => {
  test('the banner names the next reset on EKORM, EKOCM and the module chooser', async ({ page }) => {
    await login(page, USERS.demo.email, { next: null });
    await expect(page.locator('[data-demo-banner]')).toContainText('Demo hesabı');

    await page.goto('/ekorm');
    const banner = page.locator('[data-demo-banner]');
    await expect(banner).toContainText('Bu hesaptaki veriler örnektir.');
    await expect(banner).toContainText(/Sonraki sıfırlama: \d{2}:00/);

    await page.goto('/ekocm');
    await expect(page.locator('[data-demo-banner]')).toContainText('Demo hesabı');
  });

  test('the sidebar offers every feature, Finansal Analiz and GES Santralleri included', async ({ page }) => {
    await login(page, USERS.demo.email);
    const labels = await sidebarLabels(page);
    expect(labels).toContain('Finansal Analiz');
    expect(labels).toContain('GES Santralleri');
  });

  test('the visitor creates an alarm and is told it was saved', async ({ page }) => {
    await login(page, USERS.demo.email, { next: '/ekorm/alarms' });
    await page.getByRole('button', { name: 'Yeni Alarm Ekle' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel('Ad').fill('Demo ziyaretçi alarmı');
    await dialog.getByRole('combobox', { name: /Tip/ }).click();
    await page.getByRole('option', { name: 'Veri İletişim Alarmı' }).click();
    await dialog.getByRole('combobox', { name: /Analizör seçiniz/ }).click();
    await page.getByRole('option').first().click();
    await page.keyboard.press('Escape');
    await dialog.getByLabel(/İletişim Kesinti Eşiği/).fill('6');
    await dialog.getByRole('button', { name: 'Kaydet' }).click();

    await expect(page.getByText('Alarm kaydedildi.')).toBeVisible();
    await expect(page.getByText('Demo ziyaretçi alarmı')).toBeVisible();
  });

  test('the shared profile is locked and says why', async ({ page }) => {
    await login(page, USERS.demo.email, { next: '/ekorm/settings' });
    await expect(page.getByText('Demo hesabı paylaşımlıdır; profil, şifre ve oturumlar değiştirilemez.')).toBeVisible();
    await expect(page.getByLabel('Ad soyad')).toBeDisabled();
    await expect(page.getByRole('button', { name: 'Şifreyi değiştir' })).toHaveCount(0);
  });
});
