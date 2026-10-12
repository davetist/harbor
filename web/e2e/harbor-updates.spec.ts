import AxeBuilder from '@axe-core/playwright'
import { test, expect } from '@playwright/test'

const host = { hostname: 'test-host', platform: 'darwin/arm64', harborVersion: '0.1.0-main.9', dataDir: '/tmp/harbor-update-fixture', pierRoot: '/tmp/harbor-update-fixture/piers', allowedRoots: ['/tmp/harbor-update-fixture/piers'], timezone: 'UTC', loomSizes: [30, 31, 32], defaultShipHTTPBindAddress: '127.0.0.1', maintenanceProfile: 'Test profile', maintenanceValidated: false }
const current = { currentVersion: '0.1.0-main.9', latestVersion: '0.1.0-main.9', comparisonKnown: true, available: false, prerelease: true, releaseURL: 'https://github.com/davetist/harbor/releases/tag/v0.1.0-main.9', checkedAt: '2026-10-12T00:00:00Z' }
const newer = { ...current, latestVersion: '0.1.0-main.10', available: true, releaseURL: 'https://github.com/davetist/harbor/releases/tag/v0.1.0-main.10' }

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/host', route => route.fulfill({ json: host }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: 'event: resync\ndata: {}\n\n' }))
})

test('a newer Harbor version is a light yellow release link in the bottom-right footer', async ({ page }) => {
  await page.route('**/api/v1/updates', route => route.fulfill({ json: newer }))
  await page.goto('/')
  const footer = page.getByRole('contentinfo')
  const version = footer.getByRole('link', { name: 'Harbor 0.1.0-main.10 available', exact: true })
  await expect(version).toBeVisible()
  await expect(version).toHaveCSS('color', 'rgb(232, 211, 149)')
  await expect(version).toHaveAttribute('href', 'https://github.com/davetist/harbor/releases')
  await expect(version).toHaveAttribute('rel', 'noopener noreferrer')
  await expect(version).toHaveAttribute('title', /Installed: Harbor 0.1.0-main.9/)
  const popup = page.waitForEvent('popup')
  await page.route('https://github.com/davetist/harbor/releases', route => route.fulfill({ contentType: 'text/html', body: '<h1>Harbor releases</h1>' }))
  await version.click()
  const releases = await popup
  expect(releases.url()).toBe('https://github.com/davetist/harbor/releases')
  await releases.close()
  for (const width of [1280, 390, 320]) {
    await page.setViewportSize({ width, height: 850 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const versionBounds = await version.boundingBox()
    const footerBounds = await footer.boundingBox()
    expect(versionBounds!.x + versionBounds!.width).toBeGreaterThan(footerBounds!.x + footerBounds!.width * .8)
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze()
    expect(results.violations).toEqual([])
    await page.screenshot({ path: `../output/playwright/harbor-update-${width}.png`, fullPage: true })
  }
})

test('Check for updates refreshes a cached current version and discovers a new release', async ({ page }) => {
  await page.route('**/api/v1/updates', route => route.fulfill({ json: current }))
  await page.route('**/api/v1/updates/check', route => route.fulfill({ json: newer }))
  await page.goto('/')
  await expect(page.getByRole('contentinfo').getByRole('link', { name: /available/ })).toHaveCount(0)
  await page.getByRole('contentinfo').getByRole('button', { name: /^Harbor / }).click()
  await expect(page.getByRole('contentinfo').getByRole('link', { name: 'Harbor 0.1.0-main.10 available', exact: true })).toBeVisible()
})

test('a successful manual check confirms that Harbor is current', async ({ page }) => {
  await page.route('**/api/v1/updates', route => route.fulfill({ json: current }))
  await page.route('**/api/v1/updates/check', route => route.fulfill({ json: current }))
  await page.goto('/')
  await page.getByRole('contentinfo').getByRole('button', { name: /^Harbor / }).click()
  await expect(page.getByRole('contentinfo')).toContainText('Harbor is up to date.')
  await expect(page.getByRole('contentinfo').getByRole('link', { name: /available/ })).toHaveCount(0)
})

test('offline checks stay quiet in the background and manual errors do not block Harbor', async ({ page }) => {
  const offline = { ...current, latestVersion: '', checkedAt: null, error: 'Could not check for updates. Check your connection and try again.' }
  await page.route('**/api/v1/updates', route => route.fulfill({ json: offline }))
  await page.route('**/api/v1/updates/check', route => route.fulfill({ json: offline }))
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Welcome to Harbor' })).toBeVisible()
  await expect(page.getByText(offline.error)).toHaveCount(0)
  await page.getByRole('contentinfo').getByRole('button', { name: /^Harbor / }).click()
  await expect(page.getByRole('contentinfo')).toContainText(offline.error)
  await expect(page.getByRole('heading', { name: 'Welcome to Harbor' })).toBeVisible()
  await expect(page.getByText('Harbor is up to date.')).toHaveCount(0)
})

test('a failed manual check can still expose a known update link', async ({ page }) => {
  await page.route('**/api/v1/updates', route => route.fulfill({ json: current }))
  await page.route('**/api/v1/updates/check', route => route.fulfill({ json: { ...newer, error: 'Could not check for updates. Check your connection and try again.' } }))
  await page.goto('/')
  await page.getByRole('contentinfo').getByRole('button', { name: /^Harbor / }).click()
  await expect(page.getByRole('contentinfo')).toContainText('Could not check for updates.')
  await expect(page.getByRole('contentinfo').getByRole('link', { name: 'Harbor 0.1.0-main.10 available', exact: true })).toHaveAttribute('href', 'https://github.com/davetist/harbor/releases')
})

test('a development build can find a release without claiming it is an outdated installation', async ({ page }) => {
  const dev = { ...current, currentVersion: '0.1.0-dev', latestVersion: '0.1.0-main.10', comparisonKnown: false }
  await page.route('**/api/v1/host', route => route.fulfill({ json: { ...host, harborVersion: '0.1.0-dev' } }))
  await page.route('**/api/v1/updates', route => route.fulfill({ json: dev }))
  await page.route('**/api/v1/updates/check', route => route.fulfill({ json: dev }))
  await page.goto('/')
  await page.getByRole('contentinfo').getByRole('button', { name: /^Harbor / }).click()
  await expect(page.getByRole('contentinfo')).toContainText('Development build · latest release 0.1.0-main.10')
  await expect(page.getByRole('contentinfo').getByRole('link', { name: /available/ })).toHaveCount(0)
  await expect(page.getByText('Harbor is up to date.')).toHaveCount(0)
})

test('a background failure keeps a known update visible without showing a new error', async ({ page }) => {
  let backgroundFailed = false
  const error = 'Could not check for updates. Check your connection and try again.'
  await page.route('**/api/v1/updates', route => route.fulfill({ json: backgroundFailed ? { ...newer, error } : current }))
  await page.route('**/api/v1/updates/check', route => route.fulfill({ json: current }))
  await page.goto('/')
  await page.getByRole('contentinfo').getByRole('button', { name: /^Harbor / }).click()
  await expect(page.getByRole('contentinfo')).toContainText('Harbor is up to date.')
  backgroundFailed = true
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await expect(page.getByRole('contentinfo').getByRole('link', { name: 'Harbor 0.1.0-main.10 available', exact: true })).toBeVisible()
  await expect(page.getByText(error)).toHaveCount(0)
})

test('the version itself checks for updates and shows progress until the response arrives', async ({ page }) => {
  let completeCheck!: () => void
  const pending = new Promise<void>(resolve => { completeCheck = resolve })
  await page.route('**/api/v1/updates', route => route.fulfill({ json: current }))
  await page.route('**/api/v1/updates/check', async route => { await pending; await route.fulfill({ json: current }) })
  await page.goto('/')
  const footer = page.getByRole('contentinfo')
  await expect(footer.getByRole('button', { name: 'Check for updates', exact: true })).toHaveCount(0)
  try {
    await footer.getByRole('button', { name: 'Harbor 0.1.0-main.9', exact: true }).click()
    await expect(footer.getByRole('button', { name: 'Checking for updates…', exact: true })).toBeDisabled()
  } finally { completeCheck() }
  await expect(footer.getByRole('button', { name: 'Harbor 0.1.0-main.9', exact: true })).toBeEnabled()
  await expect(footer).toContainText('Harbor is up to date.')
})
