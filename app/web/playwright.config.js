import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  use: {
    baseURL: 'http://127.0.0.1:5173',
    viewport: { width: 390, height: 844 },
    channel: process.env.ZHIGU_BROWSER_CHANNEL || undefined
  },
  projects: [{ name: 'chromium', use: { ...devices['Pixel 7'] } }],
  webServer: process.env.E2E_NO_SERVER ? undefined : {
    command: 'npm run dev -- --host 127.0.0.1 --port 5173 --strictPort',
    url: 'http://127.0.0.1:5173',
    reuseExistingServer: !process.env.CI,
    timeout: 60000
  }
})
