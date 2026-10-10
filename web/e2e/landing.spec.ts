import { test, expect } from '@playwright/test'

const host = { hostname: 'test-host', platform: 'linux/arm64', dataDir: '/tmp/test-piers', timezone: 'UTC', loomSizes: [30, 31, 32], defaultShipHTTPBindAddress: '127.0.0.1', maintenanceProfile: 'Test profile', maintenanceValidated: false }
const ship = { id: 'existing-ship', name: '~sampel-palnet', nickname: '', pierPath: '/tmp/test-piers/sampel-palnet', runtimePath: '/tmp/vere', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 8080, desiredRunning: false, recoveryBlocked: false, recoveryBlockReason: '', state: 'stopped', error: '' }

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/host', route => route.fulfill({ json: host }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: 'event: resync\ndata: {}\n\n' }))
})

test('root opens registered ships and still allows explicit onboarding', async ({ page }) => {
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [ship] }))
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Your ships', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Manage ~sampel-palnet' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Welcome to Harbor' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Add a ship' }).click()
  await expect(page.getByRole('heading', { name: 'Welcome to Harbor' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('button', { name: 'Use an existing pier' })).toBeVisible()
  await page.evaluate(() => { location.hash = '' })
  await expect(page.getByRole('heading', { name: 'Your ships', exact: true })).toBeVisible()
})

test('root opens new ship choices when no ships are registered', async ({ page }) => {
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [] }))
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Welcome to Harbor' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Create a comet' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Boot from a keyfile' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Use an existing pier' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Your ships', exact: true })).toHaveCount(0)
})
