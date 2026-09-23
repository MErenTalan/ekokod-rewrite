import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

/** The seeded invoices are for the month before now (internal/seed/tariffs.go). */
function lastMonth(): { label: string; year: number; month: number } {
  const now = new Date();
  const d = new Date(now.getFullYear(), now.getMonth() - 1, 1);
  const label = new Intl.DateTimeFormat('tr-TR', { month: 'long' }).format(d);
  return { label: label.charAt(0).toUpperCase() + label.slice(1), year: d.getFullYear(), month: d.getMonth() + 1 };
}

async function openSeededMonth(page: import('@playwright/test').Page) {
  const { label, year } = lastMonth();
  await page.goto('/ekorm/bills');
  await page.getByRole('button', { name: /Ay seçimi/ }).click();
  // The picker opens on the current year; the seeded month may be in the previous one.
  const yearLabel = page.getByText(String(year), { exact: true });
  if ((await yearLabel.count()) === 0) await page.getByRole('button', { name: 'Önceki yıl' }).click();
  await page.getByRole('button', { name: label }).click();
}

test('shows the month’s invoices with a total that is the sum of its rows', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await openSeededMonth(page);

  await expect(page.getByRole('heading', { level: 1, name: 'Faturalar' })).toBeVisible();
  await expect(page.getByRole('cell', { name: 'E2E-A1' }).first()).toBeVisible();

  // 09 §F8: each building's total is the sum of the rows above it. The
  // sections follow the order the bills come back in, so match by name.
  await expect(page.getByRole('row', { name: /Ara toplam.*A1 Fabrika/ })).toContainText('25.400,25');
  await expect(page.getByRole('row', { name: /Ara toplam.*A2 Depo/ })).toContainText('18.320,50');
});

test('says the building invoice differs from its rows instead of hiding it (R234)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await openSeededMonth(page);

  await expect(page.getByRole('row', { name: /Bina faturası/ }).first()).toContainText('48.250,75');
  await expect(page.getByRole('note')).toContainText('bina tarifesi toplam tüketime bir kez uygulanır');
});

test('names the plant section and says its data source does not exist yet (R235)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await openSeededMonth(page);

  // The sidebar has a "GES Santralleri" entry of its own; this is the section.
  await expect(page.getByRole('heading', { name: 'GES santralleri' })).toBeVisible();
  await expect(page.getByText(/veri kaynağı henüz yok/i)).toBeVisible();
});

test('downloads the whole dashboard as a workbook', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await openSeededMonth(page);

  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Hepsini indir' }).click();
  expect((await download).suggestedFilename()).toMatch(/fatura-panosu-\d{4}-\d{2}\.xlsx/);
});

test('downloads one invoice as a PDF', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await openSeededMonth(page);

  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Fatura PDF' }).first().click();
  expect((await download).suggestedFilename()).toMatch(/\.pdf$/);
});

test('a building admin sees only their own building', async ({ page }) => {
  await login(page, USERS.buildingAdmin.email);
  await openSeededMonth(page);

  await expect(page.getByRole('cell', { name: 'E2E-A1' }).first()).toBeVisible();
  await expect(page.getByRole('cell', { name: 'E2E-A2' })).toHaveCount(0);
});

test('a read-only admin is offered no generation card', async ({ page }) => {
  await login(page, USERS.companyReadonly.email);
  await openSeededMonth(page);

  await expect(page.getByRole('button', { name: 'Şirket faturası' })).toHaveCount(0);
});
