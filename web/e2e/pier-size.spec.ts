import { test, expect } from '@playwright/test'

const ship = { id: 'size-ship', name: '~sampel-palnet', nickname: '', pierPath: '/tmp/test-piers/sampel-palnet', runtimePath: '/tmp/vere', runtimeVersion: 'Vere 4.6', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 8080, desiredRunning: true, recoveryBlocked: false, recoveryBlockReason: '', state: 'running', error: '', pierBytes: 1024 ** 2, measuredAt: '2026-01-01T00:00:00Z' }

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/host', route => route.fulfill({ json: { hostname: 'test-host', platform: 'darwin/arm64', dataDir: '/tmp/test-piers', timezone: 'UTC', loomSizes: [30, 31, 32], defaultShipHTTPBindAddress: '127.0.0.1', maintenanceProfile: 'Test profile', maintenanceValidated: false } }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [ship] }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: '' }))
  await page.route('**/api/v1/updates', route => route.fulfill({ json: { currentVersion: '0.1.0-dev', latestVersion: '', available: false } }))
  await page.route('**/api/v1/ships/size-ship/schedules', route => route.fulfill({ json: [] }))
  await page.goto('/#ship/size-ship/overview')
})

test('refresh measures the pier and updates its size without changing ship state', async ({ page }) => {
  let finish!: () => void
  const pending = new Promise<void>(resolve => { finish = resolve })
  await page.route('**/api/v1/ships/size-ship/measure', async route => {
    expect(route.request().method()).toBe('POST')
    expect(route.request().postDataJSON()).toEqual({})
    await pending
    await route.fulfill({ json: { ...ship, pierBytes: 3 * 1024 ** 2, measuredAt: '2026-10-12T12:00:00Z' } })
  })
  const refresh = page.getByRole('button', { name: 'Refresh pier size', exact: true })
  const size = page.locator('.facts > div').first().locator('strong')
  await expect(size).toHaveText('1 MB')
  await refresh.click()
  await expect(refresh).toBeDisabled()
  await expect(size).toHaveText('1 MB')
  finish()
  await expect(size).toHaveText('3 MB')
  await expect(refresh).toBeEnabled()
  await expect(page.locator('.ship-state')).toHaveText('Running')
  await expect(page.getByRole('button', { name: 'Stop ship', exact: true })).toBeEnabled()
  await expect(page.getByRole('alert')).toHaveCount(0)
})

test('failed measurement keeps the size and allows another refresh', async ({ page }) => {
  await page.route('**/api/v1/ships/size-ship/measure', route => route.fulfill({ status: 409, json: { error: { code: 'measurement', message: 'Unable to measure this pier. Check that its folder is available and try again.' } } }))
  const refresh = page.getByRole('button', { name: 'Refresh pier size', exact: true })
  await refresh.click()
  await expect(page.getByRole('alert')).toContainText('Unable to measure this pier.')
  await expect(page.locator('.facts > div').first().locator('strong')).toHaveText('1 MB')
  await expect(refresh).toBeEnabled()
  await page.route('**/api/v1/ships/size-ship/measure', route => route.fulfill({ json: { ...ship, pierBytes: 2 * 1024 ** 2 } }))
  await refresh.focus()
  await page.keyboard.press('Enter')
  await expect(page.locator('.facts > div').first().locator('strong')).toHaveText('2 MB')
  await expect(page.getByRole('alert')).toHaveCount(0)
})

test('the refresh control stays beside the label on desktop and narrow screens', async ({ page }) => {
  for (const width of [1280, 390, 320]) {
    await page.setViewportSize({ width, height: 850 })
    const lines = await page.getByText('Pier size', { exact: true }).evaluate(element => {
      const range = document.createRange()
      range.selectNode(element.firstChild!)
      return range.getClientRects().length
    })
    expect(lines).toBe(1)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `../output/playwright/pier-size-overview-${width}.png`, fullPage: true })
  }
})
