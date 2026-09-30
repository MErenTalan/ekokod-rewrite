import { expect, type Page } from '@playwright/test';

// internal/seed/fixtures.go and seed/demo.go; passwords come from scripts/e2e-api.sh.
export const PASSWORD = process.env.EKOKOD_E2E_PASSWORD ?? 'Guvenli!Sifre-42';

export const USERS = {
  admin: { email: 'admin@e2e.ekokod.test', name: 'Platform Yöneticisi' },
  companyAdmin: { email: 'ca@a.e2e.ekokod.test', name: 'Ayşe Kaya' },
  companyReadonly: { email: 'cr@a.e2e.ekokod.test', name: 'Can Demir' },
  buildingAdmin: { email: 'ba@a.e2e.ekokod.test', name: 'Burak Şahin' },
  buildingReadonly: { email: 'br@a.e2e.ekokod.test', name: 'Elif Arslan' },
  demo: { email: 'demo@ekokod.com.tr' },
} as const;

/** Signs in and lands on `next` (EKORM's dashboard by default); `next: null` takes the plain login to the chooser (F17 U3). */
export async function login(page: Page, email: string, o: { remember?: boolean; next?: string | null } = {}) {
  const next = o.next === undefined ? '/ekorm' : o.next;
  await page.goto(next ? `/auth/login?next=${encodeURIComponent(next)}` : '/auth/login');
  await page.getByLabel('E-posta').fill(email);
  await page.getByLabel('Şifre', { exact: true }).fill(PASSWORD);
  if (o.remember) await page.getByRole('checkbox', { name: 'Bu Cihazı Hatırla' }).click();
  await page.getByRole('button', { name: 'Giriş Yap' }).click();
  await expect(page).toHaveURL(new RegExp(`${(next ?? '/modules').replace(/[/?]/g, '\\$&')}$`));
}

/** Every docked-sidebar entry's accessible label with groups expanded, in order (aria-hidden badges like "Yakında" excluded). */
export async function sidebarLabels(page: Page): Promise<string[]> {
  const nav = page.getByRole('navigation', { name: 'Ana menü' }).first();
  await expect(nav).toBeVisible();
  const collapsed = nav.locator('button[aria-expanded="false"]');
  while ((await collapsed.count()) > 0) await collapsed.first().click();
  return nav.locator('a, [role="link"]').evaluateAll((els) =>
    els.map((el) => {
      const copy = el.cloneNode(true) as Element;
      copy.querySelectorAll('[aria-hidden="true"]').forEach((n) => n.remove());
      return copy.textContent?.trim() ?? '';
    }),
  );
}
