import { expect, test } from '@playwright/test';

import { login, sidebarLabels, USERS } from './fixtures';

// F17: EKORM and EKOCM are sibling modules behind one login (R489–R496).

test('neither module opens without a session', async ({ page }) => {
  for (const path of ['/ekorm/consumption', '/ekocm/data-entry', '/modules']) {
    await page.goto(path);
    await expect(page).toHaveURL(`/auth/login?next=${encodeURIComponent(path)}`);
  }
});

test('one sign-in reaches the chooser, then both modules without a second login', async ({ page }) => {
  await login(page, USERS.companyAdmin.email, { next: null });
  await expect(page.getByRole('heading', { name: 'Modül seçin', level: 1 })).toBeVisible();

  await page.getByRole('link', { name: 'EKO-CM modülünü aç' }).click();
  await expect(page).toHaveURL(/\/ekocm$/);
  await expect(page.getByRole('link', { name: 'EKO-CM ana sayfası' })).toBeVisible();
  const ekocm = await sidebarLabels(page);
  expect(ekocm).toContain('Veri Girişi');
  expect(ekocm).not.toContain('Tüketim');

  await page.getByRole('button', { name: 'Modül değiştir' }).click();
  await page.getByRole('menuitem', { name: /EKO-RM/ }).click();
  await expect(page).toHaveURL(/\/ekorm$/);
  await expect(page.getByRole('button', { name: `Kullanıcı menüsü: ${USERS.companyAdmin.name}` })).toBeVisible();
  const ekorm = await sidebarLabels(page);
  expect(ekorm).toContain('Tüketim');
  expect(ekorm).not.toContain('Veri Girişi');

  await page.getByRole('button', { name: 'Modül değiştir' }).click();
  await page.getByRole('menuitem', { name: /EKO-CM/ }).click();
  await expect(page).toHaveURL(/\/ekocm$/);
});

test('old carbon and legacy root links land on their new pages (R493)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email);
  await page.goto('/ekorm/carbon-footprint?tab=status');
  await expect(page).toHaveURL(/\/ekocm\/activity-status$/);
  await expect(page.getByRole('heading', { name: 'Faaliyet Durumu', level: 1 })).toBeVisible();
  await page.goto('/consumption');
  await expect(page).toHaveURL(/\/ekorm\/consumption$/);
});

test('EKOCM pages call no EKORM-only API (R490)', async ({ page }) => {
  await login(page, USERS.companyAdmin.email, { next: '/ekocm' });
  const calls: string[] = [];
  page.on('request', (r) => {
    const { pathname } = new URL(r.url());
    if (pathname.startsWith('/api/v1/')) calls.push(pathname);
  });
  for (const path of ['/ekocm', '/ekocm/data-entry', '/ekocm/activity-status', '/ekocm/emission-factors']) {
    await page.goto(path);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  }
  await page.waitForTimeout(1500);
  expect(calls.filter((c) => /^\/api\/v1\/(consumption|load-profile|bills|tariffs|alarms|messages|forecast|renewable|reports|plants|power-plants|financial|generation|energy-balance|anomaly|iso50001|weather|icmal-imports|solar-tariffs)(\/|$)/.test(c))).toEqual([]);
  expect(calls.some((c) => c.startsWith('/api/v1/carbon/'))).toBe(true);
});
