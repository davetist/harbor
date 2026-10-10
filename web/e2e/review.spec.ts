import { test, expect } from '@playwright/test'

test('controlled API pier export explains migration and displays the stopped source', async ({ page }) => {
  let ship = { id: 'export-source', name: '~zod', nickname: '', identity: '~zod', state: 'running', desiredRunning: true, pierPath: '/tmp/export-source', runtimePath: '/tmp/vere', runtimeVersion: 'Vere 4.6', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 18080, recoveryBlocked: false, recoveryBlockReason: '', error: '' }
  const host = { hostname: 'test-host', platform: 'darwin/arm64', dataDir: '/tmp/export-test', timezone: 'UTC', loomSizes: [31], defaultShipHTTPBindAddress: '127.0.0.1', maintenanceProfile: 'Test profile', maintenanceValidated: false }
  await page.route('**/api/v1/host', route => route.fulfill({ json: host }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [ship] }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: 'event: resync\ndata: {}\n\n' }))
  await page.route('**/api/v1/ships/export-source/schedules', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/export-source/backups', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/export-source/backup', route => route.fulfill({ status: 202, json: { jobId: 'export-job' } }))
  await page.route('**/api/v1/jobs/export-job', route => {
    ship = { ...ship, state: 'stopped', desiredRunning: false }
    return route.fulfill({ json: { id: 'export-job', shipId: ship.id, kind: 'backup', phase: 'completed', result: 'Pier exported. Source ship remains stopped. Keep it stopped when moving to another device.', error: '' } })
  })
  await page.goto('/#ship/export-source/maintenance')
  await page.getByText('Runtime & recovery', { exact: true }).click()
  await expect(page.getByText('Export stops this ship and leaves it stopped.')).toBeVisible()
  await page.getByRole('button', { name: 'Export pier' }).click()
  await expect(page.getByRole('status')).toContainText('Source ship remains stopped')
  await expect(page.locator('.detail-header .ship-state')).toHaveText('Stopped')
  await expect(page.getByRole('button', { name: 'Open ship' })).toBeDisabled()
})

test('failed pre-launch setup can retry without registering another ship', async ({ page }) => {
  const ship = { id: 'retry-ship', name: 'Comet', state: 'stopped' }
  const host = { hostname: 'test-host', platform: 'linux/arm64', dataDir: '/tmp/test-piers', timezone: 'UTC', loomSizes: [31], maintenanceProfile: 'Test profile', maintenanceValidated: false }
  await page.route('**/api/v1/host', route => route.fulfill({ json: host }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [ship] }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: 'event: resync\ndata: {}\n\n' }))
  await page.route('**/api/v1/jobs/failed', route => route.fulfill({ json: { id: 'failed', shipId: ship.id, kind: 'boot_comet', phase: 'failed', retryable: true, error: 'Runtime download failed.' } }))
  await page.route('**/api/v1/jobs/failed/retry', route => route.fulfill({ status: 202, json: { jobId: 'retry' } }))
  await page.route('**/api/v1/jobs/retry', route => route.fulfill({ json: { id: 'retry', shipId: ship.id, kind: 'boot_comet', phase: 'completed' } }))
  await page.goto('/#boot/failed')
  await expect(page.getByRole('alert')).toContainText('Runtime download failed.')
  await page.getByRole('button', { name: 'Retry setup' }).click()
  await expect(page.getByRole('heading', { name: 'Your ship is ready' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Open ship' })).toBeEnabled()
})
