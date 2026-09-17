import { defineConfig, devices } from '@playwright/test';

// Static Storybook build served by python's http.server; no dev server (plan D16, I-1, M-8).
const port = Number(process.env.SB_PORT ?? 6007);

export default defineConfig({
  testDir: 'tests',
  workers: 2,
  fullyParallel: true,
  reporter: 'list',
  use: { baseURL: `http://127.0.0.1:${port}`, viewport: { width: 1280, height: 800 } },
  projects: [{ name: 'a11y', testMatch: 'a11y/**/*.spec.ts', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 800 } } }],
  webServer: {
    command: `python3 -m http.server ${port} --bind 127.0.0.1 --directory storybook-static`,
    url: `http://127.0.0.1:${port}/index.json`,
    reuseExistingServer: false,
  },
});
