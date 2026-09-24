import { expect, test } from '@playwright/test';

import { login, USERS } from './fixtures';

// F11: the seeded company A, building A1 has a planned project and three notes (seed/iso50001.go).

test('the project home shows progress, the Gantt and the per-clause summary (R343)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/iso-50001');
  await expect(page.getByRole('heading', { name: 'ISO 50001 Enerji Yönetim Sistemi', level: 1 })).toBeVisible();
  await expect(page.getByRole('progressbar', { name: 'Proje İlerlemesi' })).toHaveAttribute('aria-valuenow', '15', { timeout: 30_000 });
  await expect(page.getByRole('list', { name: 'Madde zaman çizelgesi' }).getByRole('listitem')).toHaveCount(5);
  await expect(page.getByText('Tüm maddeler TS EN ISO 50001:2018 standardına dayanmaktadır.')).toBeVisible();
});

test('a note and an evidence file are added on the checklist, and a bad file is refused (R345, R346)', async ({ page }) => {
  await login(page, USERS.buildingAdmin.email);
  await page.goto('/ekorm/iso-50001?tab=checklist');
  await page.getByRole('button', { name: '7. Destek' }).click({ timeout: 30_000 });
  const clause = page.getByRole('article').filter({ hasText: '7.1 Kaynaklar' });
  await clause.getByRole('textbox', { name: /^Not/ }).fill('Enerji ekibi için bütçe onaylandı.');
  await clause.getByRole('button', { name: 'Notu Listeye Ekle' }).click();
  await expect(clause.getByText('Enerji ekibi için bütçe onaylandı.')).toBeVisible();

  await clause.getByLabel(/Gerekli Dosyaları Yükle/).setInputFiles({ name: 'Butce.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.7\n%%EOF\n') });
  await expect(clause.getByText('Butce.pdf')).toBeVisible();
  const download = page.waitForEvent('download');
  await clause.getByRole('button', { name: 'Butce.pdf indir' }).click();
  expect((await download).suggestedFilename()).toBe('Butce.pdf');

  await clause.getByLabel(/Gerekli Dosyaları Yükle/).setInputFiles({ name: 'fatura.pdf', mimeType: 'application/pdf', buffer: Buffer.from('MZ\x90\x00') });
  await expect(clause.getByText(/Bu dosya türüne izin verilmiyor/)).toBeVisible();
});

test('the ISO 50001 folder downloads as a zip (R339)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/iso-50001');
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'ISO 50001 Klasörünü İndir' }).click({ timeout: 30_000 });
  expect((await download).suggestedFilename()).toMatch(/^ISO-50001-.*\.zip$/);
});

test('a building-readonly user reads without write controls (R341)', async ({ page }) => {
  await login(page, USERS.buildingReadonly.email);
  await page.goto('/ekorm/iso-50001?tab=checklist');
  await page.getByRole('button', { name: '5. Liderlik' }).click({ timeout: 30_000 });
  await expect(page.getByText('Genel müdür EnYS taahhüt yazısını imzaladı.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Notu Listeye Ekle' })).toHaveCount(0);
});
