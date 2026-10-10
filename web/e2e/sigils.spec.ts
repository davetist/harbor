import { test, expect } from '@playwright/test'

const host = { hostname: 'test-host', platform: 'linux/arm64', dataDir: '/tmp/test-piers', timezone: 'UTC', loomSizes: [30, 31, 32], defaultShipHTTPBindAddress: '127.0.0.1', maintenanceProfile: 'Test profile', maintenanceValidated: false }
const baseShip = { nickname: '', pierPath: '/tmp/test-piers/ship', runtimePath: '/tmp/vere', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 8080, desiredRunning: false, recoveryBlocked: false, recoveryBlockReason: '', state: 'stopped', error: '' }

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/host', route => route.fulfill({ json: host }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: 'event: resync\ndata: {}\n\n' }))
  await page.route('**/api/v1/ships/*/schedules', route => route.fulfill({ json: [] }))
})

test('ship rows and management header show sigils from the identity rather than the nickname', async ({ page }) => {
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [
    { ...baseShip, id: 'galaxy', name: 'fake-zod', identity: '~zod', nickname: 'My ship' },
    { ...baseShip, id: 'planet', name: 'sampel-palnet' },
  ] }))
  await page.goto('/')
  const galaxy = page.getByRole('img', { name: 'Sigil for ~zod', exact: true })
  const planet = page.getByRole('img', { name: 'Sigil for ~sampel-palnet', exact: true })
  await expect(galaxy.locator('svg')).toHaveCount(1)
  await expect(planet.locator('svg')).toHaveCount(1)
  expect(await galaxy.innerHTML()).not.toBe(await planet.innerHTML())
  await expect(page.getByText('My ship', { exact: true })).toBeVisible()
  await page.screenshot({ path: '../output/playwright/sigils-desktop.png' })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.reload()
  await expect(page.getByRole('img', { name: 'Sigil for ~zod', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.screenshot({ path: '../output/playwright/sigils-mobile.png', fullPage: true })
  await page.getByRole('button', { name: 'Manage fake-zod' }).click()
  await expect(page.locator('.detail-header').getByRole('img', { name: 'Sigil for ~zod', exact: true }).locator('svg')).toHaveCount(1)
})

test('unavailable or unsupported identities never fabricate another ship sigil or break the list', async ({ page }) => {
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [
    { ...baseShip, id: 'pending', name: 'my-comet' },
    { ...baseShip, id: 'invalid', name: 'bad-identity', identity: '<svg onload=alert(1)>' },
    { ...baseShip, id: 'moon', name: '~sampel-palnet-migtyl-wallys', identity: '~sampel-palnet-migtyl-wallys' },
  ] }))
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Your ships', exact: true })).toBeVisible()
  await expect(page.locator('.ship-row')).toHaveCount(3)
  await expect(page.getByRole('img', { name: 'Sigil unavailable', exact: true })).toHaveCount(3)
  await expect(page.locator('.ship-row svg')).toHaveCount(0)
})
