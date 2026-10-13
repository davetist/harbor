import { test, expect } from '@playwright/test'
import { readFileSync, realpathSync } from 'node:fs'
import { basename, dirname, isAbsolute, join, relative } from 'node:path'
import { DatabaseSync } from 'node:sqlite'
import ob from 'urbit-ob'

const target = process.env.HARBOR_WEB_RECOVERY_URL
test.use({ trace: 'off' }) // Access codes and session cookies stay in memory.

test('a disposable native ship recovers from a busy event through the confirmed UI and logs in again', async ({ page, context }) => {
  test.skip(!target, 'explicit disposable web-recovery service URL required')
  test.setTimeout(180_000)
  const address = new URL(target!)
  expect(['127.0.0.1', 'localhost']).toContain(address.hostname)
  const host = await (await page.request.get(`${target}/api/v1/host`)).json()
  expect(host.dataDir).toMatch(/\/harbor-web-recovery-[\w-]+\/data$/)
  const ships = await (await page.request.get(`${target}/api/v1/ships`)).json()
  expect(ships).toHaveLength(1)
  const ship = ships[0]
  expect(ship.nickname).toBe('web-recovery-trial')
  expect(ship.state).toBe('running')
  expect(ob.isValidPatp(ship.identity)).toBe(true)
  expect(ob.clan(ship.identity)).toBe('comet')
  const dataDir = realpathSync(host.dataDir)
  const trial = dirname(dataDir)
  expect(basename(dataDir)).toBe('data')
  expect(basename(trial)).toMatch(/^harbor-web-recovery-[\w-]+$/)
  const pier = relative(trial, realpathSync(ship.pierPath))
  expect(isAbsolute(pier) || pier.startsWith('..')).toBe(false)
  expect(pier).toMatch(/^data[/\\]piers[/\\]/)
  function ownedProcess() {
    const db = new DatabaseSync(join(dataDir, 'harbor.sqlite'), { readOnly: true })
    try {
      const rows = db.prepare("SELECT payload FROM processes WHERE ship_id=? AND phase!='exited'").all(ship.id)
      expect(rows).toHaveLength(1)
      const record = JSON.parse(String(rows[0].payload))
      expect(record.kind).toBe('ship')
      expect(record.phase).toBe('live')
      return { pid: record.pid, launchId: record.launchId }
    } finally { db.close() }
  }
  const processBefore = ownedProcess()
  let blocked = false
  let computation: Promise<unknown> | undefined
  try {
    await expect.poll(async () => (await page.request.get(`${target}/api/v1/ships/${ship.id}/open`)).status(), { timeout: 30_000 }).toBe(200)
    const ports = readFileSync(join(realpathSync(ship.pierPath), '.http.ports'), 'utf8').split('\n').map(line => line.trim().split(/\s+/)).filter(parts => parts.length === 3 && parts[1] === 'insecure' && parts[2] === 'loopback')
    expect(ports).toHaveLength(1)
    const port = Number(ports[0][0])
    expect(Number.isInteger(port) && port > 0 && port <= 65535).toBe(true)
    const control = `http://127.0.0.1:${port}/`
    const identity = await page.request.post(control, { data: { source: { dojo: 'our' }, sink: { stdout: null } } })
    expect(identity.status()).toBe(200)
    const returnedIdentity = await identity.json()
    expect(typeof returnedIdentity).toBe('string')
    expect(returnedIdentity.trim()).toBe(ship.identity)
    // Use the private Dojo query for a bounded, allocation-light computation;
    // this avoids depending on Vere's persisted interactive editor buffer.
    computation = page.request.post(control, { data: { source: { dojo: '=|  n=@ud  |-  ?:  =(n 1.000.000.000.000)  n  $(n +(n))' }, sink: { stdout: null } }, timeout: 30_000 }).catch(() => undefined)
    blocked = true
    await expect.poll(async () => (await page.request.get(`${target}/api/v1/ships/${ship.id}/open`)).status(), { timeout: 10_000 }).toBe(503)
    await page.goto(`${target}/#ship/${ship.id}/overview`)
    await page.getByRole('button', { name: 'Open ship' }).click()
    await expect(page.getByRole('complementary', { name: 'Ship web recovery' })).toContainText('not responding')
    expect(context.pages()).toHaveLength(1)
    await page.getByRole('button', { name: 'Interrupt current event', exact: false }).click()
    const dialog = page.getByRole('dialog', { name: 'Interrupt the current event?' })
    await dialog.getByRole('button', { name: 'Interrupt current event', exact: true }).click()
    await expect(dialog).toHaveCount(0, { timeout: 10_000 })
    await expect(page.getByRole('status')).toContainText('Interrupt sent')
    await expect.poll(async () => (await page.request.get(`${target}/api/v1/ships/${ship.id}/open`)).status(), { timeout: 15_000 }).toBe(200)
    blocked = false
    const current = await (await page.request.get(`${target}/api/v1/ships/${ship.id}`)).json()
    expect(current).toMatchObject({ state: 'running', desiredRunning: true, recoveryBlocked: false, identity: ship.identity })
    expect(ownedProcess()).toEqual(processBefore)
    const popup = page.waitForEvent('popup')
    await page.getByRole('button', { name: 'Try Open again' }).click()
    const opened = await popup
    await opened.waitForLoadState()
    expect(new URL(opened.url()).hostname).toBe(address.hostname)
    expect(new URL(opened.url()).port || '80').toBe(String(current.webPort))
    const response = await page.request.post(`${target}/api/v1/ships/${ship.id}/access-code`, { data: {} })
    expect(response.status()).toBe(200)
    const code = (await response.json()).code
    await opened.locator('input[type="password"]').fill(code)
    await opened.locator('input[type="password"]').press('Enter')
    await expect.poll(async () => opened.evaluate(async expected => {
      const response = await fetch('/session.js', { cache: 'no-store' })
      if (response.status !== 200 || !response.headers.get('content-type')?.includes('javascript')) return false
      const session = await response.text()
      const identity = session.match(/window\s*\.\s*ship\s*=\s*['"](~?[a-z-]+)['"]/)?.[1]
      return identity?.replace(/^~/, '') === expected.replace(/^~/, '')
    }, current.identity), { timeout: 120_000 }).toBe(true)
    await opened.close()
  } finally {
    if (blocked) {
      // Preserve a usable fixture after failure; this fallback cannot pass the
      // recovery test and is limited to the guarded disposable service above.
      await page.request.post(`${target}/api/v1/ships/${ship.id}/terminal/input`, { data: { input: '\u0003' } })
    }
    await computation
  }
})
