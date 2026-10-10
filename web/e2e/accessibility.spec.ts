import AxeBuilder from '@axe-core/playwright'
import { test, expect } from '@playwright/test'

for (const width of [1280, 390, 320]) {
  test(`accessibility and reflow at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 850 })
    for (const route of ['welcome', 'setup/comet', 'setup/keyfile', 'setup/existing', 'ships', 'ship/preview-planet/overview', 'ship/preview-planet/maintenance', 'ship/preview-planet/terminal']) {
      await page.goto(`/preview.html#${route}`)
      await expect(page.locator('main h1')).toBeVisible()
      if (route.endsWith('/maintenance')) {
        await page.getByRole('button', { name: 'Configure Pack' }).click()
        await page.getByText('Runtime & recovery', { exact: true }).click()
      }
      if (route.endsWith('/terminal')) await expect(page.getByLabel('Dojo command')).toBeVisible()
      const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze()
      expect(results.violations, route).toEqual([])
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), route).toBe(true)
      const targets = await page.locator('button:visible, summary:visible, a:visible').evaluateAll(elements => elements.filter(element => !element.hasAttribute('disabled')).map(element => ({ name: element.textContent?.trim(), width: element.getBoundingClientRect().width, height: element.getBoundingClientRect().height })))
      expect(targets.filter(target => target.width < 44 || target.height < 44), route).toEqual([])
    }
    if (width === 390) await page.screenshot({ path: '../output/playwright/readiness-terminal-mobile.png', fullPage: true })
  })
}

test('tabs keep keyboard focus, arrow/Home/End selection and associated panels', async ({ page }) => {
  await page.goto('/preview.html#ship/preview-planet/overview')
  await expect(page.locator('main h1')).toBeFocused()
  const overview = page.getByRole('tab', { name: 'Overview', exact: true })
  const maintenance = page.getByRole('tab', { name: 'Maintenance', exact: true })
  const terminal = page.getByRole('tab', { name: 'Terminal', exact: true })
  await overview.focus()
  await page.keyboard.press('ArrowRight')
  await expect(maintenance).toBeFocused()
  await expect(maintenance).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('tabpanel', { name: 'Maintenance', exact: true })).toBeVisible()
  await expect(overview).toHaveAttribute('tabindex', '-1')
  await page.keyboard.press('End')
  await expect(terminal).toBeFocused()
  await expect(page.getByLabel('Dojo command')).toBeVisible()
  await page.keyboard.press('Home')
  await expect(overview).toBeFocused()
  await page.keyboard.press('ArrowLeft')
  await expect(terminal).toBeFocused()
  await page.getByRole('button', { name: 'Your ships' }).click()
  await expect(page.getByRole('heading', { name: 'Your ships', exact: true })).toBeFocused()
})

test('archive selector exposes selection and uses the permitted pier root', async ({ page }) => {
  await page.route('**/api/v1/host', route => route.fulfill({ json: { hostname: 'test', platform: 'linux/arm64', dataDir: '/local/manager', pierRoot: '/external/piers', allowedRoots: ['/external/piers'], timezone: 'UTC', loomSizes: [31], maintenanceProfile: 'Paused', maintenanceValidated: false } }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: '' }))
  await page.goto('/#setup/existing')
  await expect(page.getByPlaceholder('/external/piers')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Existing folder', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await page.getByRole('button', { name: 'Upload an archive', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Upload an archive', exact: true })).toHaveAttribute('aria-pressed', 'true')
})
