import { expect, test, type Page } from '@playwright/test';

import { login, USERS } from './fixtures';

const TITLE = 'E2E Bakım Günü';

const chips = (page: Page) => page.getByRole('button', { name: TITLE });

async function openVacations(page: Page) {
  await page.getByRole('button', { name: 'Tatil yönetimi' }).click();
  return page.getByRole('dialog');
}

async function setWeekendDays(page: Page, days: string[]) {
  const dialog = await openVacations(page);
  for (const day of ['Pazartesi', 'Salı', 'Çarşamba', 'Perşembe', 'Cuma', 'Cumartesi', 'Pazar']) {
    // Exact: "Cuma" is also a prefix of "Cumartesi".
    const box = dialog.getByRole('checkbox', { name: day, exact: true });
    const wanted = days.includes(day);
    if ((await box.isChecked()) !== wanted) await box.click();
  }
  await dialog.getByRole('button', { name: 'Kaydet' }).click();
  await expect(dialog).toHaveCount(0);
}

// 09 §F6 acceptance: the calendar writes events and the weekend configuration
// that the load profile reads back (R137).
test.describe.configure({ mode: 'serial' });

test.describe('calendar', () => {
  test('a company admin creates an all-day event', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/calendar' });
    await page.getByRole('button', { name: 'Etkinlik ekle' }).click();
    await page.getByLabel(/Başlık/).fill(TITLE);
    await page.getByRole('radio', { name: 'Camgöbeği' }).click();
    await page.getByRole('button', { name: 'Kaydet' }).click();
    await expect(chips(page).first()).toBeVisible();
  });

  test('the weekend days it saves reach the load profile', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/calendar' });
    await setWeekendDays(page, ['Cuma', 'Cumartesi']);

    await page.goto('/ekorm/load-profile');
    await expect(page.getByText(/Hafta sonu günleri: Cuma, Cumartesi \(şirket takvimi\)/)).toBeVisible();

    await page.goto('/ekorm/calendar');
    await setWeekendDays(page, ['Cumartesi', 'Pazar']);
    await page.goto('/ekorm/load-profile');
    // The API decides the order it stores the two days in; only the pair matters here.
    await expect(page.getByText(/Hafta sonu günleri: (Cumartesi, Pazar|Pazar, Cumartesi) \(şirket takvimi\)/)).toBeVisible();
  });

  test('a read-only role sees the event but cannot add one', async ({ page }) => {
    await login(page, USERS.buildingReadonly.email, { next: '/ekorm/calendar' });
    await expect(chips(page).first()).toBeVisible();
    await expect(page.getByRole('button', { name: 'Etkinlik ekle' })).toHaveCount(0);
    const dialog = await openVacations(page);
    await expect(dialog.getByRole('button', { name: 'Kaydet' })).toHaveCount(0);
  });

  test('a company admin deletes the event again', async ({ page }) => {
    await login(page, USERS.companyAdmin.email, { next: '/ekorm/calendar' });
    while ((await chips(page).count()) > 0) {
      await chips(page).first().click();
      await page.getByRole('button', { name: 'Etkinliği sil' }).click();
      await expect(page.getByRole('dialog')).toHaveCount(0);
    }
    await expect(chips(page)).toHaveCount(0);
  });
});
