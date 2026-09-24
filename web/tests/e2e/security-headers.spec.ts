import { expect, test, type Page } from '@playwright/test';

import { login, USERS } from './fixtures';

// F15a R452/R453: every page runs under a nonce CSP, and nothing it loads is refused.
async function watchViolations(page: Page): Promise<string[]> {
  const violations: string[] = [];
  page.on('console', (m) => {
    if (m.type() === 'error' && /Content Security Policy|Refused to/.test(m.text())) violations.push(m.text());
  });
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (e) => {
      console.error(`Refused to load ${e.blockedURI} (${e.violatedDirective})`);
    });
  });
  return violations;
}

test('the public site and the login page run under the nonce CSP without violations', async ({ page }) => {
  const violations = await watchViolations(page);
  for (const path of ['/', '/about', '/bill-calculator', '/auth/login']) {
    const res = await page.goto(path);
    const csp = res?.headers()['content-security-policy'] ?? '';
    expect(csp, path).toMatch(/script-src 'self' 'nonce-[^']+' 'strict-dynamic'/);
    expect(res?.headers()['x-frame-options']).toBe('DENY');
    await expect(page.locator('main').first()).toBeVisible();
    const nonced = await page.evaluate(() => [...document.scripts].filter((s) => s.src || s.textContent).every((s) => Boolean(s.nonce) || s.type === 'application/ld+json'));
    expect(nonced, `${path}: every executable script carries the nonce`).toBe(true);
  }
  expect(violations).toEqual([]);
});

test('the signed-in screens with charts run without CSP violations', async ({ page }) => {
  const violations = await watchViolations(page);
  await login(page, USERS.companyAdmin.email);
  for (const path of ['/ekorm', '/ekorm/consumption', '/ekorm/solar-plants', '/ekorm/renewable-energy']) {
    await page.goto(path);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible({ timeout: 30_000 });
  }
  expect(violations).toEqual([]);
});
