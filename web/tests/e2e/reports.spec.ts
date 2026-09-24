import { expect, test, type Page } from '@playwright/test';

import { login, USERS } from './fixtures';

/** The seeded report and bills are for the month before now (internal/seed/reports.go). */
function lastMonthLabel(): string {
  const now = new Date();
  const d = new Date(now.getFullYear(), now.getMonth() - 1, 1);
  return new Intl.DateTimeFormat('tr-TR', { month: 'long', year: 'numeric' }).format(d);
}

const info = (page: Page) => page.getByRole('table', { name: 'Bilgi tablosu' });

async function openMonthlyForBothBuildings(page: Page) {
  await page.goto('/ekorm/reports');
  await expect(info(page)).toBeVisible({ timeout: 30_000 });
  await page.getByRole('button', { name: 'Tümünü seç' }).click();
}

test('the monthly preview shows consumption and the seeded building invoice', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await openMonthlyForBothBuildings(page);
  await expect(info(page).getByRole('row', { name: /^Rapor dönemi/ })).toContainText(lastMonthLabel(), { ignoreCase: true });
  await expect(info(page).getByRole('row', { name: /^Aylık toplam tüketim/ })).toContainText('kWh');
  // Only A1 has a building invoice: its sum, with the coverage named (R258).
  await expect(info(page).getByRole('row', { name: /^Elektrik faturası/ })).toContainText('48.250,75');
  await expect(info(page).getByRole('row', { name: /^Elektrik faturası/ })).toContainText('1 / 2 bina');
});

test('the yearly tab compares the utility-scale plant with its target (E-1)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/reports');
  await page.getByRole('tab', { name: 'Yıllık rapor' }).click();
  const solar = page.getByRole('table', { name: 'Güneş enerjisi üretim raporu' });
  await expect(solar).toBeVisible({ timeout: 30_000 });
  await expect(solar.getByRole('row', { name: /Hedeflenen üretim/ })).toContainText('120.000');
  await expect(solar.getByRole('row', { name: /Hedef gerçekleşme oranı/ })).toContainText('%90');
  await expect(page.getByRole('table', { name: 'Karbon emisyonu' })).toBeVisible();
});

test('generating a report ends ready, downloads, and a mail without SMTP says why', async ({ page }) => {
  test.setTimeout(120_000);
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/reports');
  await expect(info(page)).toBeVisible({ timeout: 30_000 });
  await page.getByRole('button', { name: 'Raporu oluştur' }).click();

  const actions = page.getByRole('table', { name: 'İndir ve gönder' });
  await expect(actions.getByText('Hazır', { exact: true })).toBeVisible({ timeout: 60_000 });

  const [pdf] = await Promise.all([page.waitForEvent('download'), actions.getByRole('button', { name: 'PDF indir' }).click()]);
  expect(pdf.suggestedFilename()).toMatch(/^rapor-\d{4}-\d{2}\.pdf$/);
  const [xlsx] = await Promise.all([page.waitForEvent('download'), actions.getByRole('button', { name: 'Excel indir' }).click()]);
  expect(xlsx.suggestedFilename()).toMatch(/^rapor-\d{4}-\d{2}\.xlsx$/);

  await actions.getByRole('button', { name: 'E-posta ile gönder' }).click();
  await page.getByRole('textbox', { name: /Alıcı e-posta adresi/ }).fill('yonetici@firma.com.tr');
  await page.getByRole('button', { name: 'Gönder' }).click();
  await expect(page.getByText(/SMTP ayarları yok/).first()).toBeVisible({ timeout: 60_000 });
});

test('the archive lists the seeded report as available', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/reports');
  await page.getByRole('tab', { name: 'Rapor arşivi' }).click();
  await expect(page.getByText(/Toplam rapor: \d+/)).toBeVisible({ timeout: 30_000 });
  const seeded = page.getByRole('listitem', { name: new RegExp(lastMonthLabel(), 'i') }).first();
  // A parallel test may be regenerating this very report: the worker finishes it.
  await expect(seeded).toContainText('Mevcut', { timeout: 60_000 });
  await expect(seeded.getByRole('button', { name: /PDF/ })).toBeVisible();
});

test('a read-only admin is offered no report generation', async ({ page }) => {
  await login(page, USERS.companyReadonly.email);
  await page.goto('/ekorm/reports');
  await expect(info(page)).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole('button', { name: 'Raporu oluştur' })).toHaveCount(0);
});

// R267: a finished billing.generate used to answer 404, so the bills screen
// said "Fatura oluşturulamadı" even when the invoice was computed.
test('bill generation ends ready or with a named data condition, never the generic failure', async ({ page }) => {
  test.setTimeout(120_000);
  await login(page, USERS.companyAdmin.email);
  const now = new Date();
  const d = new Date(now.getFullYear(), now.getMonth() - 1, 1);
  const month = new Intl.DateTimeFormat('tr-TR', { month: 'long' }).format(d);
  await page.goto('/ekorm/bills');
  await page.getByRole('button', { name: /Ay seçimi/ }).click();
  if ((await page.getByText(String(d.getFullYear()), { exact: true }).count()) === 0) {
    await page.getByRole('button', { name: 'Önceki yıl' }).click();
  }
  await page.getByRole('button', { name: month.charAt(0).toUpperCase() + month.slice(1) }).click();
  await page.getByRole('button', { name: 'Şirket faturası' }).click();
  const outcome = page.getByText(
    /Fatura hazır|Bina için tarife bulunamadı|tüketim verisi yok|anomalisi var|Dönem henüz kapanmadı|PTF verisi eksik|Fatura oluşturulamadı/,
  );
  await expect(outcome.first()).toBeVisible({ timeout: 60_000 });
  await expect(page.getByText('Fatura oluşturulamadı')).toHaveCount(0);
});
