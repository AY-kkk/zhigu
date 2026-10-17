import { defineConfig } from '@playwright/test'

const shared = {
  baseURL: process.env.E2E_BASE_URL || 'http://127.0.0.1:5173',
  channel: process.env.ZHIGU_BROWSER_CHANNEL || undefined,
  locale: 'zh-CN',
  trace: 'retain-on-failure',
  screenshot: 'only-on-failure',
  video: 'retain-on-failure',
}

const webServers = process.env.E2E_NO_SERVER
  ? undefined
  : [
      ...(process.env.CI || process.env.PLAYWRIGHT_SMOKE === '1'
        ? [{
            command: 'node e2e/mock-api.mjs',
            url: 'http://127.0.0.1:8080/healthz',
            reuseExistingServer: false,
            timeout: 30000,
          }]
        : []),
      {
        command: 'npm run dev -- --host 127.0.0.1 --port 5173 --strictPort',
        env: { VITE_INTEL_ENABLED: 'true', VITE_FUTURES_ENABLED: 'true' },
        url: 'http://127.0.0.1:5173',
        reuseExistingServer: !process.env.CI,
        timeout: 60000,
      },
    ]

export default defineConfig({
  testDir: './e2e',
  testIgnore: process.env.PLAYWRIGHT_SMOKE === '1' ? /strategy-market-chain\.spec\.js/ : undefined,
  timeout: 30000,
  expect: { timeout: 8000 },
  fullyParallel: false,
  workers: process.env.CI ? 2 : 1,
  use: { ...shared, viewport: { width: 1440, height: 900 } },
  projects: [
    { name: 'chromium', use: { ...shared, viewport: { width: 1440, height: 900 }, browserName: 'chromium' } },
    { name: 'firefox', use: { ...shared, viewport: { width: 1024, height: 768 }, browserName: 'firefox' } },
    { name: 'webkit', use: { ...shared, viewport: { width: 390, height: 844 }, browserName: 'webkit' } },
  ],
  webServer: webServers,
})
