import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: /intel(?:-chain|-video)?\.spec\.js/,
  timeout: 90000,
  use: {
    baseURL: process.env.E2E_BASE_URL || 'http://127.0.0.1:5173',
    viewport: { width: 1440, height: 900 }
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
    { name: 'firefox', use: { browserName: 'firefox' } },
    { name: 'webkit', use: { browserName: 'webkit' } }
  ],
  webServer: process.env.E2E_NO_SERVER ? undefined : [
    {
      command: 'cd ../server && GOTOOLCHAIN=local ZHIGU_INTEL_COOKIE_SECRET=chain-cookie-secret ZHIGU_INTEL_FIXTURE_DIR=../../contracts/intel/fixtures/events-v1 go run ./cmd/intel-demo',
      url: 'http://127.0.0.1:8099/healthz',
      reuseExistingServer: !process.env.CI,
      timeout: 90000
    },
    {
      command: 'npm run dev -- --host 127.0.0.1 --port 5173 --strictPort',
      env: { VITE_INTEL_ENABLED: 'true', ZHIGU_API_PROXY: 'http://127.0.0.1:8099' },
      url: 'http://127.0.0.1:5173',
      reuseExistingServer: !process.env.CI,
      timeout: 60000
    }
  ]
})
