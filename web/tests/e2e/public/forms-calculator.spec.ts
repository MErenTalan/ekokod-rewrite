import { expect, test } from '@playwright/test';

// The e2e API runs without EKOKOD_PUBLIC_FORMS_*: a genuine submission reaches 503 forms_not_configured,
// which the page explains with the operator e-mail. Delivery itself is F12a's integration test.
test('a contact message that passes the bot checks is refused honestly when forms are not configured', async ({ page }) => {
  await page.goto(`/contact?subject=${encodeURIComponent('Doküman talebi: Teknik Şartname')}`);
  await expect(page.getByLabel(/Konu/)).toHaveValue('Doküman talebi: Teknik Şartname');
  await page.getByLabel(/Ad soyad/).fill('Ayşe Yılmaz');
  await page.getByLabel(/E-posta/).fill('ayse@example.com');
  await page.getByLabel(/Mesajınız/).fill('Teknik şartnameyi rica ediyoruz.');
  await page.waitForTimeout(2100); // R353: a person takes at least 2 s; faster is treated as a bot.
  await page.getByRole('button', { name: 'Gönder' }).click();
  await expect(page.getByRole('alert').filter({ hasText: 'İletişim formu şu anda kullanılamıyor' })).toContainText('info@ekokod.com');
});

test('a bot that fills the honeypot is told it succeeded', async ({ page }) => {
  await page.goto('/request-demo');
  await page.getByLabel(/Ad soyad/).fill('Bot');
  await page.getByLabel(/E-posta/).fill('bot@example.com');
  await page.getByLabel(/Telefon/).fill('0555 000 00 00');
  await page.getByLabel(/Şirket/).fill('Spam A.Ş.');
  await page.locator('input[name="website"]').fill('https://spam.example', { force: true });
  await page.getByRole('button', { name: 'Gönder' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Mesajınız iletildi' })).toBeVisible();
});

test('the form refuses a typo before spending the rate limit', async ({ page }) => {
  let calls = 0;
  page.on('request', (r) => { if (r.url().includes('/api/v1/public/contact')) calls++; });
  await page.goto('/contact');
  await page.getByLabel(/E-posta/).fill('ayse@');
  await page.getByRole('button', { name: 'Gönder' }).click();
  await expect(page.getByRole('alert').filter({ hasText: 'Formu göndermeden önce' })).toBeVisible();
  expect(calls).toBe(0);
});

/** The F12a hand-computed commercial MV binomial bill (TestPublicCalculator), for `days` days, in exact cents. */
function expectedTotal(days: number): string {
  // µTL × 30 so the days/30 pro-rating stays an integer: energy 10000×3.307886, distribution 10000×1.668345,
  // power 89.14752×100×days/30, overuse 20×178.29504, VAT 20 %.
  const fixed = (33_078_860_000n + 16_683_450_000n + 3_565_900_800n) * 30n;
  const base30 = fixed + 8_914_752_000n * BigInt(days);
  const num = base30 * 12n;
  const den = 30n * 10n * 10_000n; // → cents
  const cents = (2n * num + den) / (2n * den);
  const tl = cents / 100n;
  const kurus = String(cents % 100n).padStart(2, '0');
  return `₺${tl.toLocaleString('tr-TR')},${kurus}`;
}

test('the calculator prices every line, power charge included, from the seeded national schedule', async ({ page }) => {
  await page.goto('/bill-calculator');
  await page.getByRole('combobox', { name: /Abone grubu/ }).click();
  await page.getByRole('option', { name: /Ticarethane/ }).click();
  await page.getByRole('radio', { name: 'Orta gerilim (OG)' }).check();
  await page.getByRole('radio', { name: 'Çift terimli' }).check();
  await page.getByLabel(/Toplam tüketim/).fill('10000');
  await page.getByLabel(/Sözleşme gücü/).fill('100');
  await page.getByLabel(/Ölçülen güç/).fill('120');
  await page.getByRole('button', { name: 'Hesapla' }).click();

  const result = page.getByRole('region', { name: 'Fatura detayı' });
  const period = await result.getByText(/günlük dönem/).textContent();
  const days = Number(/(\d+) günlük/.exec(period ?? '')?.[1]);
  expect(days).toBeGreaterThanOrEqual(28);
  await expect(result).toContainText('₺33.078,86');
  await expect(result).toContainText('₺16.683,45');
  await expect(result).toContainText('₺3.565,90');
  await expect(result).toContainText(expectedTotal(days));
});

test('total and T1/T2/T3 that disagree are refused on the total field (Q-H4)', async ({ page }) => {
  await page.goto('/bill-calculator');
  await page.getByRole('radio', { name: 'Çok zamanlı (T1/T2/T3)' }).check();
  await page.getByLabel(/Toplam tüketim/).fill('300');
  await page.getByLabel(/T1 gündüz/).fill('100');
  await page.getByLabel(/T2 puant/).fill('100');
  await page.getByLabel(/T3 gece/).fill('50');
  await page.getByRole('button', { name: 'Hesapla' }).click();
  await expect(page.getByText("Toplam tüketim T1, T2 ve T3'ün toplamına eşit olmalı.")).toBeVisible();
});
