import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const pierPath = '/storage/piers/test-comet-with-a-long-folder-name/to-preserve/on-disk'
async function setup(page: import('@playwright/test').Page, state = 'stopped', reject = false, deletionPending = false) {
  let ships = [{ id: 'test-comet', name: 'Test comet', nickname: '', state, desiredRunning: state === 'running', pierPath, runtimePath: '/tmp/vere', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 18080, recoveryBlocked: deletionPending, recoveryBlockReason: deletionPending ? 'Pier deletion is pending. Confirm permanent deletion again to finish removal.' : '', pierDeletionIdentity: deletionPending ? 'original-folder' : '', error: '' }]
  let removals = 0
  await page.route('**/api/v1/host', route => route.fulfill({ json: { hostname: 'test-host', platform: 'darwin/arm64', dataDir: '/tmp/harbor-test', timezone: 'UTC', loomSizes: [31], maintenanceProfile: 'Test profile', maintenanceValidated: false } }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: 'event: resync\ndata: {}\n\n' }))
  await page.route('**/api/v1/updates', route => route.fulfill({ json: {} }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: ships }))
  await page.route('**/api/v1/ships/test-comet/schedules', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/test-comet', route => {
    if (route.request().method() !== 'DELETE') return route.fulfill({ json: ships[0] })
    removals++
    if (reject) return route.fulfill({ status: 409, json: { error: { message: 'This ship has active or queued work. Wait for it to finish before removing it.' } } })
    ships = []
    return route.fulfill({ status: 204 })
  })
  await page.goto('/#ship/test-comet/overview')
  return () => removals
}

test('removal shows the preserved pier path, cancels safely, and refreshes the registry', async ({ page }) => {
  const removals = await setup(page)
  await page.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Remove Test comet from Harbor?' })
  await expect(dialog.getByText(pierPath, { exact: true })).toBeVisible()
  await expect(dialog).toContainText('Your ship will still be available')
  await expect(dialog).toContainText('Use existing pier')
  await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused()
  const accessibility = await new AxeBuilder({ page }).analyze()
  expect(accessibility.violations).toEqual([])
  await page.screenshot({ path: '../output/playwright/remove-ship-desktop.png' })
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  expect(removals()).toBe(0)
  const remove = page.getByRole('button', { name: 'Remove from Harbor', exact: true })
  await expect(remove).toBeFocused()
  await remove.click()
  await dialog.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Your ships', exact: true })).toBeVisible()
  await expect(page.getByText('Your ships will appear here.')).toBeVisible()
  expect(removals()).toBe(1)
  await page.reload()
  await expect(page.getByText('Your ships will appear here.')).toBeVisible()
})

test('running ships explain why removal is unavailable', async ({ page }) => {
  const removals = await setup(page, 'running')
  await expect(page.getByRole('button', { name: 'Remove from Harbor', exact: true })).toBeDisabled()
  await expect(page.getByText('Stop this ship before removing it from Harbor.')).toBeVisible()
  expect(removals()).toBe(0)
})

test('permanent deletion is opt-in, warns clearly, and sends the confirmed path', async ({ page }) => {
  await setup(page)
  await page.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  const dialog = page.getByRole('dialog')
  const checkbox = dialog.getByRole('checkbox', { name: 'Permanently delete pier files' })
  await expect(checkbox).not.toBeChecked()
  await checkbox.check()
  await expect(dialog).toContainText('This cannot be undone')
  await expect(dialog).toContainText('Exported archives stay on disk')
  await expect(dialog.getByText(pierPath, { exact: true })).toBeVisible()
  await expect(dialog).not.toContainText('You can add it back')
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([])
  await page.screenshot({ path: '../output/playwright/delete-pier-desktop.png' })
  const request = page.waitForRequest(request => request.method() === 'DELETE')
  await dialog.getByRole('button', { name: 'Remove and delete pier', exact: true }).click()
  expect((await request).postDataJSON()).toEqual({ deletePier: true, confirmPierPath: pierPath })
  await expect(page.getByRole('heading', { name: 'Your ships', exact: true })).toBeVisible()
})

test('canceling permanent removal preserves the ship and resets the checkbox', async ({ page }) => {
  const removals = await setup(page)
  const remove = page.getByRole('button', { name: 'Remove from Harbor', exact: true })
  await remove.click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('checkbox', { name: 'Permanently delete pier files' }).check()
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
  expect(removals()).toBe(0)
  await remove.click()
  await expect(dialog.getByRole('checkbox', { name: 'Permanently delete pier files' })).not.toBeChecked()
})

test('interrupted deletion keeps Start disabled and requires explicit deletion confirmation again', async ({ page }) => {
  await setup(page, 'failed', false, true)
  await expect(page.getByRole('button', { name: 'Start ship', exact: true })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'Verify recorded outcome' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('button', { name: 'Remove from Harbor', exact: true })).toBeDisabled()
  await dialog.getByRole('checkbox', { name: 'Permanently delete pier files' }).check()
  await expect(dialog.getByRole('button', { name: 'Remove and delete pier', exact: true })).toBeEnabled()
})

test('a conflicting removal keeps the confirmation and ship available', async ({ page }) => {
  await setup(page, 'stopped', true)
  await page.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  await expect(dialog.getByRole('alert')).toContainText('active or queued work')
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Test comet', exact: true })).toBeVisible()
})

test('the confirmation fits a narrow screen and keeps its full path readable', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 700 })
  await setup(page)
  await page.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByText(pierPath, { exact: true })).toBeVisible()
  await dialog.getByRole('checkbox', { name: 'Permanently delete pier files' }).check()
  await expect(dialog.getByRole('button', { name: 'Remove and delete pier', exact: true })).toBeVisible()
  const overflow = await dialog.evaluate(element => element.scrollWidth > element.clientWidth)
  expect(overflow).toBe(false)
  await page.screenshot({ path: '../output/playwright/delete-pier-mobile.png' })
})
