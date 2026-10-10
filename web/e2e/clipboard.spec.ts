import { test, expect } from '@playwright/test'

const sampleCode = 'clipboard-regression-sentinel'
const target = process.env.HARBOR_BROWSER_URL || ''
const ship = { id: 'clipboard-ship', name: '~sampel-palnet', nickname: '', pierPath: '/tmp/test-piers/sampel-palnet', runtimePath: '/tmp/vere', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 8080, desiredRunning: true, recoveryBlocked: false, recoveryBlockReason: '', state: 'running', error: '' }
const host = { hostname: 'test-host', platform: 'linux/arm64', dataDir: '/tmp/test-piers', timezone: 'UTC', loomSizes: [30, 31, 32], defaultShipHTTPBindAddress: '127.0.0.1', maintenanceProfile: 'Test profile', maintenanceValidated: false }

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/host', route => route.fulfill({ json: host }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [ship] }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: 'event: resync\ndata: {}\n\n' }))
  await page.route('**/api/v1/ships/clipboard-ship/schedules', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/clipboard-ship/access-code', route => route.fulfill({ json: { code: sampleCode } }))
  await page.route('**/api/v1/ships/clipboard-ship/logs', route => route.fulfill({ json: { output: '' } }))
  await page.route('**/api/v1/ships/clipboard-ship/jobs', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/clipboard-ship/terminal/resize', route => route.fulfill({ status: 204 }))
})

for (const unavailable of ['missing', 'denied'] as const) {
  test(`Copy writes the real clipboard when the modern API is ${unavailable}`, async ({ page }) => {
    await page.addInitScript(mode => {
      Object.defineProperty(navigator, 'clipboard', { configurable: true, value: mode === 'missing' ? undefined : { writeText: () => Promise.reject(new Error('Clipboard permission denied')) } })
    }, unavailable)
    await page.goto(`${target}/#ship/clipboard-ship/overview`)
    if (target.startsWith('http://100.')) expect(await page.evaluate(() => window.isSecureContext)).toBe(false)
    await page.getByRole('button', { name: 'Reveal code', exact: true }).click()
    await page.getByRole('button', { name: 'Copy', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Copied', exact: true })).toBeVisible()
    await expect(page.getByRole('alert')).toHaveCount(0)
    await page.getByRole('tab', { name: 'Terminal', exact: true }).click()
    const input = page.getByLabel('Dojo command')
    await input.press(process.platform === 'darwin' ? 'Meta+V' : 'Control+V')
    await expect(input).toHaveValue(sampleCode)
    await page.getByRole('tab', { name: 'Overview', exact: true }).click()
    await page.getByRole('button', { name: 'Hide code', exact: true }).click()
    await page.getByRole('button', { name: 'Reveal code', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Copy', exact: true })).toBeVisible()
  })
}

test('blocked copy reports failure without claiming the clipboard changed', async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
    document.execCommand = () => false
  })
  await page.goto(`${target}/#ship/clipboard-ship/overview`)
  await page.getByRole('button', { name: 'Reveal code', exact: true }).click()
  await page.getByRole('button', { name: 'Copy', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Select and copy the code manually.')
  await expect(page.getByRole('button', { name: 'Copied', exact: true })).toHaveCount(0)
})
