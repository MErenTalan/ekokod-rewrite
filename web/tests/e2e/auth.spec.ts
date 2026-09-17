import { expect, test } from '@playwright/test';

import { login, sidebarLabels, USERS } from './fixtures';

// 09 §F6 acceptance, against the real API and the production build.
const CORE = [
  'Yönetim Arayüzü',
  'Tüketim',
  'Yük Profili',
  'Tüketim Tahmini',
  'GES Santralleri',
  'Finansal Analiz',
  'Yenilenebilir Enerji',
  'Su',
  'Doğal Gaz',
  'EV Sürücüleri',
  'Faturalar',
  'Tarifeler',
  'Manuel',
  'Mesajlar',
  'Yapay Zeka',
  'Raporlar',
  'Takvim',
  'Ayarlar',
  'Karbon Ayak İzi',
  'ISO 50001 Modülü',
  'Tasarruf Önerileri',
  'İletişim',
];
const WITHOUT_RESTRICTED = CORE.filter((label) => label !== 'GES Santralleri' && label !== 'Finansal Analiz');

const ROLES = [
  { role: 'admin', user: USERS.admin, nav: CORE, switcher: true },
  { role: 'company_admin', user: USERS.companyAdmin, nav: CORE, switcher: false },
  { role: 'company_readonly_admin', user: USERS.companyReadonly, nav: CORE, switcher: false },
  { role: 'building_admin', user: USERS.buildingAdmin, nav: WITHOUT_RESTRICTED, switcher: false },
  { role: 'building_readonly_admin', user: USERS.buildingReadonly, nav: WITHOUT_RESTRICTED, switcher: false },
  { role: 'demo', user: USERS.demo, nav: WITHOUT_RESTRICTED, switcher: false },
] as const;

test.describe('each of the six roles logs in and sees exactly its navigation', () => {
  for (const { role, user, nav, switcher } of ROLES) {
    test(role, async ({ page }) => {
      await login(page, user.email);
      expect(await sidebarLabels(page)).toEqual(nav);
      await expect(page.getByRole('combobox', { name: 'Şirket' })).toHaveCount(switcher ? 1 : 0);
    });
  }
});

test('remember me keeps a persistent refresh cookie', async ({ browser }) => {
  const remembered = await browser.newContext();
  const rememberedPage = await remembered.newPage();
  await login(rememberedPage, USERS.companyAdmin.email, { remember: true });
  const rt = (await remembered.cookies()).find((c) => c.name === 'ekokod_rt');
  const thirtyDays = Date.now() / 1000 + 30 * 24 * 3600;
  expect(rt?.httpOnly).toBe(true);
  expect(Math.abs((rt?.expires ?? 0) - thirtyDays)).toBeLessThan(3600);
  await remembered.close();

  const session = await browser.newContext();
  const sessionPage = await session.newPage();
  await login(sessionPage, USERS.companyReadonly.email);
  expect((await session.cookies()).find((c) => c.name === 'ekokod_rt')?.expires).toBe(-1);
  await session.close();
});

test('device mismatch forces re-login with the reason', async ({ browser }) => {
  const first = await browser.newContext({ userAgent: 'Mozilla/5.0 (X11; Linux x86_64) ekokod-e2e/desktop' });
  await login(await first.newPage(), USERS.buildingAdmin.email);
  const stolen = await first.cookies();
  await first.close();

  const other = await browser.newContext({ userAgent: 'Mozilla/5.0 (iPhone) ekokod-e2e/other-device' });
  await other.addCookies(stolen);
  const page = await other.newPage();
  await page.goto('/ekorm');
  await expect(page).toHaveURL(/\/auth\/login\?reason=device_mismatch$/);
  await expect(
    page.getByRole('alert').filter({ hasText: 'Oturumunuz başka bir cihazda kullanıldığı için sonlandırıldı. Lütfen tekrar giriş yapın.' }),
  ).toBeVisible();
  await other.close();
});

test('logout returns to login and protects /ekorm', async ({ page }) => {
  await login(page, USERS.buildingReadonly.email);
  await page.getByRole('button', { name: `Kullanıcı menüsü: ${USERS.buildingReadonly.name}` }).click();
  await page.getByRole('menuitem', { name: 'Çıkış' }).click();
  await expect(page).toHaveURL(/\/auth\/login$/);
  await page.goto('/ekorm/settings');
  await expect(page).toHaveURL(/\/auth\/login\?next=%2Fekorm%2Fsettings$/);
});

test('expired access token refreshes transparently', async ({ page, context }) => {
  await login(page, USERS.companyAdmin.email);
  const before = (await context.cookies()).find((c) => c.name === 'ekokod_rt')?.value;
  await context.clearCookies({ name: 'ekokod_at' });
  await page.goto('/ekorm');
  await expect(page).toHaveURL(/\/ekorm$/);
  await expect(page.getByRole('button', { name: `Kullanıcı menüsü: ${USERS.companyAdmin.name}` })).toBeVisible();
  const cookies = await context.cookies();
  expect(cookies.find((c) => c.name === 'ekokod_at')?.value).toBeTruthy();
  expect(cookies.find((c) => c.name === 'ekokod_rt')?.value).not.toBe(before);
});

test('forgot password always confirms', async ({ page }) => {
  await page.goto('/auth/forgot-password');
  await page.getByLabel('E-posta').fill('kimse-yok@ornek.com.tr');
  await page.getByRole('button', { name: 'Bağlantı Gönder' }).click();
  await expect(page.getByRole('status')).toContainText('Bu adrese kayıtlı bir hesap varsa şifre sıfırlama bağlantısı gönderildi.');
});
