import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  use: {
    baseURL: process.env.E2E_BASE_URL || 'http://127.0.0.1:5173',
    viewport: { width: 1440, height: 900 },
    channel: process.env.ZHIGU_BROWSER_CHANNEL || undefined
  },
  projects: [{ name: 'chromium' }],
  webServer: process.env.E2E_NO_SERVER ? undefined : {
    command: 'npm run dev -- --host 127.0.0.1 --port 5173 --strictPort',
    env: { VITE_INTEL_ENABLED: 'true' },
    url: 'http://127.0.0.1:5173',
    reuseExistingServer: !process.env.CI,
    timeout: 60000
  }
})
