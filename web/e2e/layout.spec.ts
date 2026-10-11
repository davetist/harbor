import { test, expect, type Page } from '@playwright/test'

const comet = '~fipfes-fipfes-fipfes-fipfes--fipfes-fipfes-fipfes-mipbes'
const longPath = '/srv/harbor/piers/' + 'a'.repeat(180)

async function controlledShip(page: Page, recoveryBlocked = false) {
  const ship = { id: 'layout-ship', name: comet, nickname: 'A ship with a longer nickname for the household server', state: recoveryBlocked ? 'failed' : 'running', desiredRunning: !recoveryBlocked, pierPath: longPath, runtimePath: '/srv/harbor/vere', runtimeVersion: 'Vere 4.6', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 8080, recoveryBlocked, recoveryBlockReason: recoveryBlocked ? 'Verify the interrupted operation before restarting.' : '', error: '' }
  await page.route('**/api/v1/host', route => route.fulfill({ json: { hostname: 'harbor-machine-with-a-very-long-name.example.internal', platform: 'darwin/arm64', dataDir: '/srv/harbor', pierRoot: '/srv/harbor/piers', timezone: 'UTC', loomSizes: [31], maintenanceProfile: 'Paused', maintenanceValidated: false } }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [ship] }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: '' }))
  await page.route('**/api/v1/ships/layout-ship/schedules', route => route.fulfill({ json: [{ id: 'layout-pack', shipId: ship.id, kind: 'pack', intervalDays: 365, localTime: '03:00', timezone: 'UTC', enabled: true, nextDue: '', lastResult: 'Completed successfully' }] }))
  await page.route('**/api/v1/ships/layout-ship/jobs', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/layout-ship/backups', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/layout-ship/logs', route => route.fulfill({ json: { output: 'alpha\nbeta\n~sampel-palnet:dojo>' } }))
  await page.route('**/api/v1/ships/layout-ship/terminal/resize', route => route.fulfill({ status: 204, body: '' }))
  await page.route('**/api/v1/ships/layout-ship/pack', route => route.fulfill({ status: 422, json: { error: { message: 'Cannot open pier: ' + longPath } } }))
}

async function fitsPage(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual((await page.viewportSize())!.width)
}

async function textFits(page: Page, selector: string) {
  const clipped = await page.locator(selector).evaluateAll(elements => elements.flatMap(element => {
    const box = element.getBoundingClientRect()
    const range = document.createRange(); range.selectNodeContents(element)
    return [...range.getClientRects()].filter(rect => rect.left < box.left - 1 || rect.right > box.right + 1).map(() => element.textContent)
  }))
  expect(clipped, `Text should stay inside ${selector}`).toEqual([])
}

for (const width of [320, 390, 760, 1280]) {
  test(`long ship names, prompts and errors fit at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 850 })
    await controlledShip(page)
    await page.goto('/#ships')
    await expect(page.getByRole('button', { name: `Manage ${comet}`, exact: true })).toBeVisible()
    await fitsPage(page)
    const name = await page.locator('.ship-name').boundingBox()
    const actions = await page.locator('.ship-actions').boundingBox()
    expect(name!.x + name!.width).toBeLessThanOrEqual(actions!.x)
    await page.getByRole('button', { name: `Manage ${comet}`, exact: true }).click()
    await textFits(page, '.detail-header>button')
    await fitsPage(page)
    await page.getByRole('tab', { name: 'Terminal', exact: true }).click()
    await expect(page.getByRole('textbox', { name: 'Dojo command', exact: true })).toBeVisible()
    await textFits(page, '.command')
    expect((await page.getByRole('textbox', { name: 'Dojo command', exact: true }).boundingBox())!.width).toBeGreaterThanOrEqual(80)
    await fitsPage(page)
    await page.getByRole('tab', { name: 'Maintenance', exact: true }).click()
    await page.getByRole('button', { name: 'Configure Pack' }).click()
    expect(await page.getByRole('checkbox', { name: 'Enabled', exact: true }).evaluate(input => input.closest('label')!.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
    await textFits(page, '.operation-right button')
    await page.getByRole('button', { name: 'Run Pack now' }).click()
    await expect(page.getByRole('alert')).toContainText(longPath)
    await textFits(page, '.feedback .error')
    await fitsPage(page)
  })
}

for (const width of [320, 390, 1280]) {
  test(`setup buttons and checkbox targets fit at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 850 })
    for (const mode of ['existing', 'keyfile', 'comet']) {
      await page.goto(`/preview.html#setup/${mode}`)
      const button = page.getByRole('button', { name: 'Browse folders on host' })
      const offset = await button.evaluate(element => {
        const box = element.getBoundingClientRect(); const range = document.createRange(); range.selectNodeContents(element); const text = range.getBoundingClientRect()
        return Math.abs((text.left + text.right - box.left - box.right) / 2)
      })
      expect(offset).toBeLessThanOrEqual(1)
      await textFits(page, '.input-action button')
      await fitsPage(page)
    }
    await page.goto('/preview.html#setup/existing')
    expect(await page.getByRole('checkbox').evaluate(input => input.closest('label')!.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
    await page.getByRole('button', { name: 'Browse folders on host' }).click()
    const hidden = page.getByRole('checkbox', { name: 'Show hidden folders' })
    expect(await hidden.evaluate(input => input.closest('label')!.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
    await fitsPage(page)
  })
}

test('terminal renders newline-only logs from the start of each row', async ({ page }) => {
  await controlledShip(page)
  await page.goto('/#ship/layout-ship/terminal')
  await expect(page.getByRole('log')).toBeVisible()
  await expect.poll(async () => (await page.getByRole('log').getByRole('listitem').allTextContents()).slice(0, 2)).toEqual(['alpha', 'beta'])
  await expect.poll(async () => (await page.locator('.xterm-rows>div').allTextContents()).join('\n')).toContain('~sampel-palnet:dojo>')
  const grid = await page.locator('.xterm-screen').boundingBox()
  const surface = await page.getByRole('log').boundingBox()
  expect(grid!.x + grid!.width).toBeLessThanOrEqual(surface!.x + surface!.width - 12)
  expect(grid!.y + grid!.height).toBeLessThanOrEqual(surface!.y + surface!.height - 12)
})

test('resizing retains the full final terminal line without a newline', async ({ page }) => {
  await controlledShip(page)
  const prompt = comet + ':dojo>'
  await page.route('**/api/v1/ships/layout-ship/logs', route => route.fulfill({ json: { output: prompt } }))
  await page.goto('/#ship/layout-ship/terminal')
  const renderedText = () => page.locator('.xterm-rows>div').allTextContents().then(rows => rows.join(''))
  await expect.poll(renderedText).toContain(prompt)
  await page.setViewportSize({ width: 320, height: 850 })
  await expect.poll(renderedText).toContain(prompt)
  await page.setViewportSize({ width: 1280, height: 850 })
  await expect.poll(renderedText).toContain(prompt)
})

test('recovery controls leave space before the overview content', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 850 })
  await controlledShip(page, true)
  await page.goto('/#ship/layout-ship/overview')
  const verify = await page.getByRole('button', { name: 'Verify recorded outcome' }).boundingBox()
  const heading = await page.getByRole('heading', { name: 'Your ship, at a glance.' }).boundingBox()
  expect(heading!.y - verify!.y - verify!.height).toBeGreaterThanOrEqual(16)
})

test('welcome return navigation stays above the footer on narrow screens', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 740 })
  await page.goto('/preview.html#welcome')
  const back = await page.getByRole('button', { name: 'Your ships' }).boundingBox()
  const footer = await page.locator('.app-footer').boundingBox()
  expect(back!.y + back!.height).toBeLessThanOrEqual(footer!.y)
})
