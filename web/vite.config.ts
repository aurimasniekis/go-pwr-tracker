/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development the Go API runs separately (default :8080) and Vite proxies
// /api, /healthz and /readyz to it, so the browser sees a single origin and
// no CORS configuration is needed.
const apiTarget = process.env.PWR_API_URL ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': apiTarget,
      '/healthz': apiTarget,
      '/readyz': apiTarget,
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 800,
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
