import { test, expect } from '@playwright/test'

test('native Harbor release builds: live update detection, version check and GitHub releases', async ({ page, context }) => {
  const target = process.env.HARBOR_ACCEPTANCE_URL
  const currentTarget = process.env.HARBOR_UPDATE_CURRENT_URL
  test.skip(!target, 'explicit disposable native release-build service required')
  expect(currentTarget, 'a disposable current-version service is required').toBeTruthy()
  const nativeHost = await (await page.request.get(`${target}/api/v1/host`)).json()
  const before = await (await page.request.get(`${target}/api/v1/ships`)).json()
  const checked = await (await page.request.post(`${target}/api/v1/updates/check`, { data: {} })).json()
  expect(checked.error).toBeUndefined()
  expect(checked.comparisonKnown).toBe(true)
  expect(checked.currentVersion).toBe(nativeHost.harborVersion)
  expect(checked.available).toBe(true)
  expect(checked.latestVersion).not.toBe('')
  await page.goto(`${target}/#ships`)
  const version = page.getByRole('contentinfo').getByRole('link', { name: `Harbor ${checked.latestVersion} available`, exact: true })
  await expect(version).toBeVisible()
  await expect(version).toHaveCSS('color', 'rgb(232, 211, 149)')
  await expect(version).toHaveAttribute('title', `Installed: Harbor ${nativeHost.harborVersion}. Open GitHub releases.`)
  await expect(version).toHaveAttribute('href', 'https://github.com/davetist/harbor/releases')
  const releasesResponse = await page.request.get('https://github.com/davetist/harbor/releases')
  expect(releasesResponse.status()).toBe(200)
  const installer = nativeHost.platform.startsWith('darwin/') ? `Harbor-${checked.latestVersion}-macos-universal.dmg` : `harbor-${checked.latestVersion}-linux-${nativeHost.platform.split('/')[1]}.run`
  const download = await page.request.head(`${checked.releaseURL.replace('/tag/', '/download/')}/${installer}`)
  expect(download.status()).toBe(200)
  const popup = context.waitForEvent('page')
  await version.click()
  const releases = await popup
  await releases.waitForLoadState('domcontentloaded')
  expect(releases.url()).toBe('https://github.com/davetist/harbor/releases')
  await releases.close()
  const after = await (await page.request.get(`${target}/api/v1/ships`)).json()
  expect(after.map((ship: { id: string; state: string; desiredRunning: boolean }) => ({ id: ship.id, state: ship.state, desiredRunning: ship.desiredRunning }))).toEqual(before.map((ship: { id: string; state: string; desiredRunning: boolean }) => ({ id: ship.id, state: ship.state, desiredRunning: ship.desiredRunning })))
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 850 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `../output/playwright/harbor-update-native-${width}.png`, fullPage: true })
  }
  const currentHost = await (await page.request.get(`${currentTarget}/api/v1/host`)).json()
  await page.goto(`${currentTarget}/`)
  const checkedResponse = page.waitForResponse(response => response.url() === `${currentTarget}/api/v1/updates/check` && response.request().method() === 'POST')
  await page.getByRole('contentinfo').getByRole('button', { name: `Harbor ${currentHost.harborVersion}`, exact: true }).click()
  const fresh = await (await checkedResponse).json()
  expect(fresh.error).toBeUndefined()
  expect(fresh.currentVersion).toBe(currentHost.harborVersion)
  expect(fresh.available).toBe(false)
  await expect(page.getByRole('contentinfo')).toContainText('Harbor is up to date.')
})
