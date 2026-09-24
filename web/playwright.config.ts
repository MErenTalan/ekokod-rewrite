import { defineConfig, devices } from '@playwright/test';

// R171: two projects with their own servers, chosen by PW_PROJECT so a run only boots what it needs.
// a11y: static Storybook build served by python's http.server (plan D16, I-1, M-8).
// e2e: the real API (scripts/e2e-api.sh) and the production Next build (`pnpm build` first).
const project = process.env.PW_PROJECT ?? 'a11y';
const sbPort = Number(process.env.SB_PORT ?? 6007);
const apiPort = Number(process.env.E2E_API_PORT ?? 18080);
const webPort = Number(process.env.E2E_WEB_PORT ?? 13000);
// localhost, not 127.0.0.1: the session cookies are Secure and Chromium trusts localhost as a secure origin.
const webURL = `http://localhost:${webPort}`;

export default defineConfig({
  testDir: 'tests',
  workers: 2,
  fullyParallel: true,
  reporter: 'list',
  use: { viewport: { width: 1280, height: 800 } },
  projects: [
    {
      name: 'a11y',
      testMatch: 'a11y/**/*.spec.ts',
      use: { ...devices['Desktop Chrome'], baseURL: `http://127.0.0.1:${sbPort}`, viewport: { width: 1280, height: 800 } },
    },
    {
      name: 'e2e',
      testMatch: 'e2e/**/*.spec.ts',
      use: { ...devices['Desktop Chrome'], baseURL: webURL, viewport: { width: 1280, height: 800 }, locale: 'tr-TR' },
    },
  ],
  webServer:
    project === 'e2e'
      ? [
          {
            command: 'bash scripts/e2e-api.sh',
            url: `http://127.0.0.1:${apiPort}/health/live`,
            env: { E2E_API_PORT: String(apiPort), E2E_WEB_URL: webURL },
            timeout: 300_000,
            reuseExistingServer: false,
          },
          {
            command: `pnpm exec next start -p ${webPort} -H localhost`,
            url: `${webURL}/auth/login`,
            env: { EKOKOD_INTERNAL_API_URL: `http://127.0.0.1:${apiPort}`, EKOKOD_PUBLIC_URL: webURL },
            timeout: 120_000,
            reuseExistingServer: false,
          },
        ]
      : {
          command: `python3 -m http.server ${sbPort} --bind 127.0.0.1 --directory storybook-static`,
          url: `http://127.0.0.1:${sbPort}/index.json`,
          reuseExistingServer: false,
        },
});
