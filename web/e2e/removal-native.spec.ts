import { test, expect } from '@playwright/test'
import { existsSync, realpathSync, readFileSync } from 'node:fs'
import { basename, dirname, isAbsolute, join, relative } from 'node:path'
import { createHash } from 'node:crypto'
import { DatabaseSync } from 'node:sqlite'
import ob from 'urbit-ob'

const target = process.env.HARBOR_REMOVAL_URL
test.use({ trace: 'off' })

test('native removal preserves a pier by default and permanently deletes it only after confirmation', async ({ page }) => {
  test.skip(!target, 'explicit disposable removal service URL required')
  test.setTimeout(120_000)
  const address = new URL(target!)
  expect(address.hostname).toBe('127.0.0.1')
  const host = await (await page.request.get(`${target}/api/v1/host`)).json()
  const dataDir = realpathSync(host.dataDir)
  const trial = dirname(dataDir)
  expect(basename(dataDir)).toBe('data')
  expect(basename(trial)).toMatch(/^harbor-removal-native-[\w-]+$/)
  const ships = await (await page.request.get(`${target}/api/v1/ships`)).json()
  expect(ships.length).toBeGreaterThan(0)
  for (const ship of ships) {
    expect(ship.state).toBe('stopped')
    expect(ship.desiredRunning).toBe(false)
    expect(ship.recoveryBlocked).toBe(false)
    expect(ob.clan(ship.identity)).toBe('comet')
    const pier = relative(join(dataDir, 'piers'), realpathSync(ship.pierPath))
    expect(isAbsolute(pier) || pier.startsWith('..')).toBe(false)
    expect(pier.length).toBeGreaterThan(0)
  }
  // The restored comet is the most recent copy after a lifecycle trial. Older
  // stopped exports must never be booted after their destination has advanced.
  const ship = ships[ships.length - 1]
  const db = new DatabaseSync(join(dataDir, 'harbor.sqlite'), { readOnly: true })
  const archives = db.prepare('SELECT archive_path FROM backups').all().map(row => String(row.archive_path))
  expect(db.prepare("SELECT id FROM processes WHERE phase!='exited'").all()).toHaveLength(0)
  db.close()
  const digest = (path: string) => createHash('sha256').update(readFileSync(path)).digest('hex')
  const before = new Map(archives.map(path => [path, digest(path)]))
  await page.goto(`${target}/#ship/${ship.id}/overview`)
  await page.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('checkbox', { name: 'Permanently delete pier files' })).not.toBeChecked()
  await dialog.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Your ships', exact: true })).toBeVisible()
  expect(existsSync(ship.pierPath)).toBe(true)
  const preserved = await (await page.request.get(`${target}/api/v1/ships`)).json()
  expect(preserved).toHaveLength(ships.length - 1)
  // Register the same stopped disposable pier again through the production API.
  const registration = await page.request.post(`${target}/api/v1/onboarding/existing`, { data: { pierPath: ship.pierPath, loomExponent: ship.loomExponent, loomConfirmed: true } })
  expect(registration.status()).toBe(202)
  const jobId = (await registration.json()).jobId
  let registeredId = ''
  await expect.poll(async () => {
    const job = await (await page.request.get(`${target}/api/v1/jobs/${jobId}`)).json()
    registeredId = job.shipId
    return job.phase
  }, { timeout: 60_000 }).toBe('completed')
  const stop = await page.request.post(`${target}/api/v1/ships/${registeredId}/stop`, { data: {} })
  expect(stop.status()).toBe(202)
  const stopId = (await stop.json()).jobId
  await expect.poll(async () => (await (await page.request.get(`${target}/api/v1/jobs/${stopId}`)).json()).phase, { timeout: 60_000 }).toBe('completed')
  await page.goto(`${target}/#ship/${registeredId}/overview`)
  await page.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  await dialog.getByRole('checkbox', { name: 'Permanently delete pier files' }).check()
  await expect(dialog).toContainText('This cannot be undone')
  await expect(dialog.getByText(ship.pierPath, { exact: true })).toBeVisible()
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
  expect(existsSync(ship.pierPath)).toBe(true)
  await page.getByRole('button', { name: 'Remove from Harbor', exact: true }).click()
  await dialog.getByRole('checkbox', { name: 'Permanently delete pier files' }).check()
  await dialog.getByRole('button', { name: 'Remove and delete pier', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Your ships', exact: true })).toBeVisible()
  expect(existsSync(ship.pierPath)).toBe(false)
  expect(existsSync(ship.runtimePath)).toBe(true)
  for (const [path, hash] of before) expect(digest(path)).toBe(hash)
  const after = await (await page.request.get(`${target}/api/v1/ships`)).json()
  expect(after.map((row: { id: string }) => row.id).sort()).toEqual(preserved.map((row: { id: string }) => row.id).sort())
  for (const other of preserved) expect(existsSync(other.pierPath)).toBe(true)
})
