import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import fs from 'node:fs';

import { login, USERS } from './fixtures';

// F15c R481: axe on every signed-in page and every auth page. Critical and
// serious fail (09 §F15); every finding is written to web/a11y-results/ for
// docs/accessibility.md.
const APP = [
  '/ekorm',
  '/ekorm/consumption',
  '/ekorm/load-profile',
  '/ekorm/bills',
  '/ekorm/tariffs',
  '/ekorm/alarms',
  '/ekorm/messages',
  '/ekorm/reports',
  '/ekorm/calendar',
  '/ekorm/carbon-footprint',
  '/ekorm/iso-50001',
  '/ekorm/financial-analysis',
  '/ekorm/renewable-energy',
  '/ekorm/solar-plants',
  '/ekorm/forecast',
  '/ekorm/predict',
  '/ekorm/ai',
  '/ekorm/settings',
];
const AUTH = ['/auth/login', '/auth/forgot-password', '/auth/register', '/auth/reset-password?token=x', '/auth/error', '/auth/maintenance'];

type Finding = { page: string; user: string; id: string; impact: string; help: string; targets: string[] };

async function audit(page: Page, path: string, user: string): Promise<Finding[]> {
  await page.goto(path);
  // Pages poll, so networkidle never settles: wait for the heading, then let data land.
  await expect(page.getByRole('heading').first()).toBeVisible({ timeout: 20_000 });
  await page.waitForTimeout(1500);
  const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
  return violations.map((v) => ({
    page: path,
    user,
    id: v.id,
    impact: v.impact ?? 'unknown',
    help: v.help,
    targets: v.nodes.map((n) => n.target.join(' ')).slice(0, 5),
  }));
}

function report(name: string, findings: Finding[]) {
  fs.mkdirSync('a11y-results', { recursive: true });
  fs.writeFileSync(`a11y-results/${name}.json`, JSON.stringify(findings, null, 2));
  const blocking = findings.filter((f) => f.impact === 'critical' || f.impact === 'serious');
  expect(blocking.map((f) => `${f.page} [${f.user}] ${f.impact} ${f.id}: ${f.targets.join(', ')}`)).toEqual([]);
}

test.describe('page accessibility sweep', () => {
  for (const [name, email] of [
    ['demo', USERS.demo.email],
    ['admin', USERS.admin.email],
    ['building-readonly', USERS.buildingReadonly.email],
  ] as const) {
    for (const path of APP) {
      test(`${path} has no critical or serious violations (${name})`, async ({ page }) => {
        await login(page, email);
        report(`${name}${path.replaceAll('/', '_')}`, await audit(page, path, name));
      });
    }
  }

  for (const path of AUTH) {
    test(`${path} has no critical or serious violations (visitor)`, async ({ page }) => {
      report(`visitor${path.replace(/\?.*/, '').replaceAll('/', '_')}`, await audit(page, path, 'visitor'));
    });
  }
});
