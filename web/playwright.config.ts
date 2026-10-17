import { defineConfig, devices } from '@playwright/test'

// End-to-end smoke test against the production binary (UI embedded) and a
// deterministic fake upstream. Requires `npm run build` first so the binary
// can embed web/dist. No live upstream access is needed.
const APP_PORT = 18180
const UPSTREAM_PORT = 18181

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${APP_PORT}`,
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: `go run ./cmd/fakeupstream -addr 127.0.0.1:${UPSTREAM_PORT} -repeat 2`,
      cwd: '..',
      url: `http://127.0.0.1:${UPSTREAM_PORT}/Pwr/lastSong/`,
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command:
        'go build -tags embedui -o bin/pwr-tracker-e2e ./cmd/pwr-tracker && ' +
        'rm -rf tmp/e2e && mkdir -p tmp/e2e && bin/pwr-tracker-e2e',
      cwd: '..',
      url: `http://127.0.0.1:${APP_PORT}/readyz`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        LISTEN_ADDR: `127.0.0.1:${APP_PORT}`,
        DATABASE_PATH: 'tmp/e2e/e2e.db',
        UPSTREAM_URL: `http://127.0.0.1:${UPSTREAM_PORT}/Pwr/lastSong/`,
        POLL_INTERVAL: '1s',
        STARTUP_DELAY: '0s',
        LOG_LEVEL: 'warn',
      },
    },
  ],
})
