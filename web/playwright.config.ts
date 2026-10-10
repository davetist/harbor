import { defineConfig } from '@playwright/test'
import { resolve } from 'node:path'
process.env.PLAYWRIGHT_BROWSERS_PATH ??= resolve(import.meta.dirname, '../.tools/playwright')
export default defineConfig({ testDir: './e2e', fullyParallel: true, use: { channel: 'chromium', baseURL: 'http://127.0.0.1:5173', viewport: { width: 1280, height: 850 }, trace: 'retain-on-failure' }, webServer: { command: 'npm run dev', url: 'http://127.0.0.1:5173/preview.html', reuseExistingServer: true }, reporter: [['list']] })
