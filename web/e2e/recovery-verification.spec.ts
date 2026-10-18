import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

test('recovery verification shows progress, prevents duplicate clicks and refreshes without an event', async ({ page }) => {
  const ship = { id: 'held', name: 'Held ship', nickname: '', state: 'failed', desiredRunning: true, pierPath: '/test/pier', runtimePath: '/test/vere', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 8080, recoveryBlocked: true, recoveryBlockReason: 'Pier held for recovery verification.', error: 'Ship readiness was not confirmed. The process is still tracked; use Start ship to check again, or inspect the terminal.' }
  let verified = false
  let requests = 0
  let finish!: () => void
  const pending = new Promise<void>(resolve => { finish = resolve })
  await page.route('**/api/v1/host', route => route.fulfill({ json: { hostname: 'test-host', platform: 'darwin/arm64', dataDir: '/test/harbor', timezone: 'UTC', loomSizes: [31], maintenanceProfile: 'Test profile', maintenanceValidated: false } }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: '' }))
  await page.route('**/api/v1/updates', route => route.fulfill({ json: { currentVersion: 'test', available: false } }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [verified ? { ...ship, state: 'stopped', desiredRunning: false, recoveryBlocked: false, recoveryBlockReason: '', error: '' } : ship] }))
  await page.route('**/api/v1/ships/held', route => route.fulfill({ json: verified ? { ...ship, state: 'stopped', desiredRunning: false, recoveryBlocked: false, recoveryBlockReason: '', error: '' } : ship }))
  await page.route('**/api/v1/ships/held/schedules', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/held/recovery/verify', async route => {
    requests++
    expect(route.request().method()).toBe('POST')
    expect(route.request().postDataJSON()).toEqual({})
    await pending
    verified = true
    await route.fulfill({ status: 204 })
  })
  await page.goto('/#ship/held/overview')
  await expect(page.getByRole('alert')).toHaveText('Pier held for recovery verification.')
  await expect(page.getByRole('button', { name: 'Start ship', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Verify recorded outcome' }).click()
  const button = page.getByRole('button', { name: 'Verifying' })
  await expect(button).toBeDisabled()
  await expect(button).toHaveAttribute('aria-busy', 'true')
  await expect(page.getByRole('status')).toContainText('several minutes')
  await button.dispatchEvent('click')
  expect(requests).toBe(1)
  for (const width of [1280, 320]) {
    await page.setViewportSize({ width, height: 850 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `../output/playwright/recovery-verification-${width}.png`, fullPage: true })
  }
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([])
  finish()
  await expect(page.getByRole('button', { name: 'Start ship', exact: true })).toBeEnabled()
  await expect(page.locator('.ship-state')).toHaveText('Stopped')
  await expect(page.getByRole('button', { name: 'Verifying' })).toHaveCount(0)
  await expect(page.getByRole('alert')).toHaveCount(0)
  expect(requests).toBe(1)
})

test('a refused verification keeps the hold and permits another attempt', async ({ page }) => {
  const ship = { id: 'held', name: 'Held ship', nickname: '', state: 'failed', desiredRunning: true, pierPath: '/test/pier', runtimePath: '/test/vere', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 8080, recoveryBlocked: true, recoveryBlockReason: 'Pier held for recovery verification.', error: '' }
  await page.route('**/api/v1/host', route => route.fulfill({ json: { hostname: 'test-host', platform: 'darwin/arm64', dataDir: '/test/harbor', timezone: 'UTC', loomSizes: [31], maintenanceProfile: 'Test profile', maintenanceValidated: false } }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: '' }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [ship] }))
  await page.route('**/api/v1/updates', route => route.fulfill({ json: { currentVersion: 'test', available: false } }))
  await page.route('**/api/v1/ships/held/schedules', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/held/recovery/verify', route => route.fulfill({ status: 409, json: { error: { code: 'recovery_hold', message: 'Offline inspection rejected the temporary copy.' } } }))
  await page.goto('/#ship/held/overview')
  await page.getByRole('button', { name: 'Verify recorded outcome' }).click()
  await expect(page.getByRole('alert')).toContainText('Offline inspection rejected')
  await expect(page.getByRole('button', { name: 'Verify recorded outcome' })).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Start ship', exact: true })).toBeDisabled()
  await expect(page.getByText('Verification in progress')).toHaveCount(0)
})
