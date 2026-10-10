import AxeBuilder from '@axe-core/playwright'
import { test, expect } from '@playwright/test'

test('native disposable ship: accessibility, Dojo, Open and mobile layout', async ({ page, context }) => {
  const target = process.env.HARBOR_ACCEPTANCE_URL
  test.skip(!target, 'explicit disposable native acceptance URL required')
  const ships = await (await page.request.get(`${target}/api/v1/ships`)).json()
  const ship = ships.find((row: { state: string }) => row.state === 'running')
  expect(ship, 'a disposable running ship is required').toBeTruthy()
  await page.goto(`${target}/#ship/${ship.id}/overview`)
  await expect(page.locator('.ship-state')).toHaveText('Running')
  await expect(page.getByText('Visual prototype · simulated ships and operations')).toHaveCount(0)
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 850 })
    for (const name of ['Overview', 'Maintenance', 'Terminal']) {
      await page.getByRole('tab', { name, exact: true }).click()
      if (name === 'Maintenance') {
        await page.getByRole('button', { name: 'Configure Pack' }).click()
        await page.getByText('Runtime & recovery', { exact: true }).click()
      }
      if (name === 'Terminal') await expect(page.getByLabel('Dojo command')).toBeVisible()
      const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze()
      expect(results.violations, `${name} ${width}px`).toEqual([])
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      if (name === 'Maintenance') await page.screenshot({ path: `../output/playwright/readiness-native-maintenance-${width}.png`, fullPage: true })
    }
  }
  await page.getByLabel('Dojo command').fill('(mul 113 127)')
  await page.getByRole('button', { name: 'Send', exact: false }).click()
  await expect(page.getByLabel('Dojo command')).toHaveValue('')
  await expect(page.locator('.xterm-rows')).toContainText('14.351', { timeout: 15000 })
  const popup = context.waitForEvent('page')
  await page.getByRole('button', { name: 'Open ship', exact: false }).click()
  const opened = await popup
  await opened.waitForLoadState()
  expect(new URL(opened.url()).port).toBe(String(ship.webPort))
  await expect(opened.locator('input[type=password]')).toBeVisible()
  await opened.close()
})
