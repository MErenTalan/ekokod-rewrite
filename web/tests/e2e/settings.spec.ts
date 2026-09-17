import { expect, test, type Page } from '@playwright/test';

import { login, USERS } from './fixtures';

const tabs = (page: Page) => page.getByRole('tab');

// 09 §F6 acceptance: every role sees exactly the tabs 01 §2's matrix specifies.
const EXPECTED: [keyof typeof USERS, string[]][] = [
  ['admin', ['Hesap', 'Entegrasyonlar', 'Şirket', 'Binalar', 'Güneş Santralleri', 'Analizörler', 'Kullanıcılar', 'SMTP Ayarları']],
  ['companyAdmin', ['Hesap', 'Şirket', 'Binalar', 'Güneş Santralleri', 'Analizörler', 'Kullanıcılar']],
  ['companyReadonly', ['Hesap', 'Şirket', 'Binalar', 'Güneş Santralleri', 'Analizörler', 'Kullanıcılar']],
  ['buildingAdmin', ['Hesap', 'Analizörler']],
  ['buildingReadonly', ['Hesap', 'Analizörler']],
  ['demo', ['Hesap']],
];

test.describe('settings', () => {
  for (const [user, expected] of EXPECTED) {
    test(`${user} sees exactly their tabs`, async ({ page }) => {
      await login(page, USERS[user].email, { next: '/ekorm/settings' });
      await expect(tabs(page)).toHaveText(expected);
    });
  }

  test('a company admin creates and deletes a building', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/settings?tab=buildings' });
    await page.getByRole('button', { name: 'Bina ekle' }).click();
    await page.getByLabel(/Bina adı/).fill('E2E Geçici Bina');
    await page.getByRole('button', { name: 'Kaydet' }).click();
    await expect(page.getByRole('row', { name: /E2E Geçici Bina/ })).toBeVisible();

    await page.getByRole('row', { name: /E2E Geçici Bina/ }).getByRole('button', { name: 'Binayı sil' }).click();
    await page.getByRole('button', { name: 'Sil' }).click();
    await expect(page.getByRole('row', { name: /E2E Geçici Bina/ })).toHaveCount(0);
  });

  test('a read-only company role sees the buildings without any control', async ({ page }) => {
    await login(page, USERS.companyReadonly.email, { next: '/ekorm/settings?tab=buildings' });
    await expect(page.getByRole('row', { name: /A1 Fabrika/ })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Bina ekle' })).toHaveCount(0);
  });

  test('a building admin gets the analyzers with refresh but no assignment', async ({ page }) => {
    await login(page, USERS.buildingAdmin.email, { next: '/ekorm/settings?tab=analyzers' });
    await page.getByRole('button', { name: /İşlemler/ }).first().click();
    await expect(page.getByRole('menuitem', { name: 'Binaya ata' })).toHaveCount(0);
    await page.getByRole('menuitem', { name: 'Saatlik değerleri yenile' }).click();
    // No integration is configured for the fixtures, and the API says so plainly.
    await expect(page.getByText(/entegrasyon/i)).toBeVisible();
  });

  test('a forbidden tab in the URL falls back to the account tab', async ({ page }) => {
    // Not through `next`: the app corrects the URL, which the login helper's own
    // assertion would read as a failed redirect.
    await login(page, USERS.companyAdmin.email);
    await page.goto('/ekorm/settings?tab=smtp');
    await expect(page).toHaveURL(/tab=account/);
    await expect(page.getByRole('tab', { name: 'Hesap' })).toHaveAttribute('aria-selected', 'true');
  });
});
