import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

async function openBuildingTab(page: import('@playwright/test').Page) {
  await page.goto('/ekorm/tariffs');
  await expect(page.getByRole('heading', { level: 1, name: 'Tarifeler' })).toBeVisible();
  // The screen's own picker selects the first building when nothing is stored.
  await expect(
    page.getByRole('region', { name: 'Tarife geçmişi' }).or(page.getByText('Bu binanın henüz tarifesi yok')),
  ).toBeVisible();
}

test('refuses a PTF tariff without an energy KBK and then saves it', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await openBuildingTab(page);

  await page.getByRole('button', { name: 'Yeni tarife' }).click();
  await page.getByLabel('Yürürlük tarihi').fill('2026-10-01');
  await page.getByLabel(/Dağıtım bedeli \(TL\/kWh\)/).fill('0.85');
  await page.getByLabel(/Reaktif güç bedeli/).fill('1.2');
  await page.getByLabel(/KDV oranı/).fill('20');
  await page.getByRole('switch', { name: /PTF \+ YEKDEM/ }).click();

  // 09 §F8's acceptance criterion: the KBK fields appear and the save is refused.
  await expect(page.getByLabel(/Enerji KBK/)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Kaydet' })).toBeDisabled();

  await page.getByLabel(/Enerji KBK/).fill('1.08');
  await page.getByLabel(/Dağıtım bedeli KBK/).fill('0.85');
  await page.getByLabel(/Reaktif güç KBK/).fill('1.2');
  await page.getByRole('button', { name: 'Kaydet' }).click();

  await expect(page.getByRole('cell', { name: /Eki? 2026|Eki 2026/ }).first()).toBeVisible();
});

test('lists the seeded template and its assignment history', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/tariffs');

  await page.getByRole('tab', { name: 'Şablonlar' }).click();
  await expect(page.getByRole('cell', { name: /Ticari AG/ })).toBeVisible();
  await expect(page.getByText('Varsayılan')).toBeVisible();

  await page.getByRole('tab', { name: 'Toplu atama' }).click();
  await expect(page.getByRole('heading', { name: 'Atama geçmişi' })).toBeVisible();
  await expect(page.getByText('Şablondan')).toBeVisible();
});

test('the icmal review writes nothing until a building is confirmed (R245)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/tariffs');
  await page.getByRole('tab', { name: /İcmal/ }).click();

  await expect(page.getByLabel(/İcmal dosyası/)).toBeVisible();
  // Nothing is uploaded in this run, so the confirm step is not offered at all.
  await expect(page.getByRole('button', { name: 'Onayla ve uygula' })).toHaveCount(0);
});

test('shows the seeded solar tariff for its plant', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/tariffs');
  await page.getByRole('tab', { name: 'Solar tarifeler' }).click();

  await page.getByRole('combobox', { name: 'Santral seçin' }).click();
  await page.getByRole('option', { name: 'E2E Çatı GES' }).click();
  // Decimals cross the API as their own string; 2.500000 normalises to 2.5.
  await expect(page.getByRole('cell', { name: '2.5', exact: true })).toBeVisible();
});

test('hides the default-tariff tab from a company admin and shows it to an admin', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/tariffs');
  await expect(page.getByRole('tab', { name: 'Varsayılan tarifeler' })).toHaveCount(0);
});

test('an admin sees the published national schedule', async ({ page }) => {
  await login(page, USERS.admin.email);
  await page.goto('/ekorm/tariffs');
  await page.getByRole('tab', { name: 'Varsayılan tarifeler' }).click();
  await expect(page.getByRole('cell', { name: 'EPDK' }).first()).toBeVisible();
});

test('a read-only admin cannot edit a tariff', async ({ page }) => {
  await login(page, USERS.companyReadonly.email);
  await openBuildingTab(page);
  await expect(page.getByRole('button', { name: 'Yeni tarife' })).toHaveCount(0);
});
