import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

async function setup(page: Page, options: { conflict?: string; held?: boolean; recovers?: boolean; entry?: 'list'; responseCode?: string } = {}) {
  const { conflict = '', held = false } = options
  let responsive = false
  let interrupts = 0
  const ship = { id: 'test-comet', name: 'Test comet', nickname: '', state: 'running', desiredRunning: true, pierPath: '/test/pier', runtimePath: '/test/vere', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 5173, recoveryBlocked: held, recoveryBlockReason: held ? 'Preserve uncertain utility outcome.' : '', error: '' }
  await page.route('**/api/v1/host', route => route.fulfill({ json: { hostname: 'test-host', platform: 'darwin/arm64', dataDir: '/test/harbor', timezone: 'UTC', loomSizes: [31], maintenanceProfile: 'Test profile', maintenanceValidated: false } }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: 'event: resync\ndata: {}\n\n' }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [ship] }))
  await page.route('**/api/v1/ships/test-comet/schedules', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/test-comet/open', route => responsive ? route.fulfill({ json: { url: 'http://127.0.0.1:5173/ship-login' } }) : route.fulfill({ status: 503, json: { error: { code: 'ship_unresponsive', message: 'The ship may be busy with a long computation.' } } }))
  await page.route('**/api/v1/ships/test-comet/interrupt', route => {
    interrupts++
    expect(route.request().method()).toBe('POST')
    expect(route.request().postDataJSON()).toEqual({})
    if (conflict) return route.fulfill({ status: 409, json: { error: { code: options.responseCode || 'conflict', message: conflict } } })
    responsive = options.recovers !== false
    return route.fulfill({ status: 204 })
  })
  await page.goto(options.entry === 'list' ? '/#ships' : '/#ship/test-comet/overview')
  await page.getByRole('button', { name: options.entry === 'list' ? 'Open Test comet' : 'Open ship' }).click()
  await expect(page.getByText('The ship may be busy with a long computation.')).toBeVisible()
  return () => interrupts
}

test('unresponsive Open offers manual recovery, cancellation sends nothing, and confirmed recovery allows retry', async ({ page, context }) => {
  const interrupts = await setup(page)
  expect(context.pages()).toHaveLength(1)
  expect(interrupts()).toBe(0)
  await page.getByRole('button', { name: 'Interrupt current event', exact: false }).click()
  const dialog = page.getByRole('dialog', { name: 'Interrupt the current event?' })
  await expect(dialog).toContainText('cancel its current computation')
  await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused()
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([])
  await page.screenshot({ path: '../output/playwright/web-recovery-desktop.png' })
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  expect(interrupts()).toBe(0)
  await page.getByRole('button', { name: 'Interrupt current event', exact: false }).click()
  await dialog.getByRole('button', { name: 'Interrupt current event', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByRole('status')).toContainText('Interrupt sent')
  expect(interrupts()).toBe(1)
  expect(context.pages()).toHaveLength(1)
  const popup = page.waitForEvent('popup')
  await page.getByRole('button', { name: 'Try Open again' }).click()
  const opened = await popup
  await expect(opened).toHaveURL('http://127.0.0.1:5173/ship-login')
  await expect(page.getByRole('button', { name: 'Interrupt current event' })).toHaveCount(0)
})

test('a conflicting interrupt stays reviewable and never claims recovery', async ({ page }) => {
  const interrupts = await setup(page, { conflict: "Wait for this ship's active or queued operation to finish." })
  await page.getByRole('button', { name: 'Interrupt current event', exact: false }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Interrupt current event', exact: true }).click()
  await expect(dialog.getByRole('alert')).toContainText('active or queued operation')
  expect(interrupts()).toBe(1)
  await expect(page.getByText(/Interrupt sent/)).toHaveCount(0)
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Interrupt current event', exact: false })).toBeFocused()
})

test('recovery holds disable interruption and the mobile explanation fits', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 740 })
  const interrupts = await setup(page, { held: true })
  await expect(page.getByRole('button', { name: 'Interrupt current event', exact: false })).toBeDisabled()
  expect(interrupts()).toBe(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: '../output/playwright/web-recovery-mobile.png' })
})

test('the interrupt confirmation fits a narrow screen', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 740 })
  await setup(page)
  await page.getByRole('button', { name: 'Interrupt current event', exact: false }).click()
  const dialog = page.getByRole('dialog')
  expect(await dialog.evaluate(element => element.scrollWidth > element.clientWidth)).toBe(false)
  await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused()
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([])
  await page.screenshot({ path: '../output/playwright/web-recovery-mobile-dialog.png' })
})

test('Open from the ship list reaches recovery and an interrupt is not reported as a successful response', async ({ page, context }) => {
  const interrupts = await setup(page, { recovers: false, entry: 'list' })
  await expect(page.getByRole('heading', { name: 'Test comet', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Interrupt current event', exact: false }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Interrupt current event', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Interrupt sent')
  await page.getByRole('button', { name: 'Try Open again' }).click()
  await expect(page.getByText('The ship may be busy with a long computation.')).toBeVisible()
  await expect(page.getByRole('status')).toHaveCount(0)
  expect(interrupts()).toBe(1)
  expect(context.pages()).toHaveLength(1)
})

test('a ship responding at the fresh check closes recovery without sending a second action', async ({ page }) => {
  const interrupts = await setup(page, { conflict: 'The ship is responding again. Try Open ship.', responseCode: 'ship_responsive' })
  await page.getByRole('button', { name: 'Interrupt current event', exact: false }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Interrupt current event', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Interrupt current event', exact: false })).toHaveCount(0)
  await expect(page.getByRole('status')).toHaveText('The ship is responding again. Try Open ship.')
  expect(interrupts()).toBe(1)
})
