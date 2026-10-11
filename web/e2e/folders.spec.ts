import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/host', route => route.fulfill({ json: { hostname: 'test-host', platform: 'darwin/arm64', dataDir: '/storage/manager', pierRoot: '/storage/piers', allowedRoots: ['/storage/piers', '/external/piers'], timezone: 'UTC', loomSizes: [31], maintenanceProfile: 'Paused', maintenanceValidated: false } }))
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/events', route => route.fulfill({ contentType: 'text/event-stream', body: '' }))
  await page.route('**/api/v1/folders?*', route => {
    const path = new URL(route.request().url()).searchParams.get('path') || ''
    const requested = path || '/home/test-host'
    const folders: Record<string, string[]> = {
      '/': ['storage', 'external', 'home'],
      '/storage': ['piers', 'another folder & spaces'],
      '/storage/piers': ['ship'],
      '/storage/piers/ship': ['child'],
      '/storage/another folder & spaces': ['nested', '.hidden'],
      '/storage/another folder & spaces/nested': [],
      '/storage/another folder & spaces/.hidden': [],
      '/external': ['piers'],
      '/external/piers': [],
      '/home/test-host': [],
    }
    return requested in folders ? route.fulfill({ json: { path: requested, parent: requested === '/' ? null : requested.slice(0, requested.lastIndexOf('/')) || '/', entries: folders[requested].map(name => ({ name, path: `${requested === '/' ? '' : requested}/${name}` })) } }) : route.fulfill({ status: 422, json: { error: { message: 'Folder is unavailable.' } } })
  })
})

for (const { mode, label } of [{ mode: 'existing', label: 'Pier folder' }, { mode: 'keyfile', label: 'Destination' }, { mode: 'comet', label: 'Destination' }]) {
  test(`${mode} path field has its own accessible name`, async ({ page }) => {
    await page.goto(`/#setup/${mode}`)
    const path = page.getByRole('textbox', { name: label })
    await expect(path).toHaveAccessibleName(label)
    await path.fill('/storage/piers/ship')
    await page.getByText(label, { exact: true }).click()
    await expect(path).toBeFocused()
    await page.getByRole('button', { name: 'Browse folders on host', exact: true }).click()
    const browser = page.getByRole('dialog', { name: 'Browse folders' })
    await expect(browser.getByRole('textbox', { name: 'Folder location' })).toHaveValue('/storage/piers/ship')
    await page.keyboard.press('Escape')
    await expect(browser).toHaveCount(0)
    await expect(path).toHaveValue('/storage/piers/ship')
  })
}

test('Browse uses the desktop picker and fills the selected host folder', async ({ page }) => {
  await page.addInitScript(() => {
    Object.assign(window, { webkit: { messageHandlers: { harborFolderPicker: { postMessage: async () => '/storage/piers/ship' } } } })
  })
  await page.goto('/#setup/existing')
  await page.getByRole('button', { name: 'Browse folders on host' }).click()
  await expect(page.getByRole('textbox', { name: 'Pier folder' })).toHaveValue('/storage/piers/ship')
  await expect(page.getByRole('group', { name: 'Folders on Harbor host' })).toHaveCount(0)
})

test('cancelling the desktop picker keeps the previous path', async ({ page }) => {
  await page.addInitScript(() => {
    Object.assign(window, { webkit: { messageHandlers: { harborFolderPicker: { postMessage: async () => null } } } })
  })
  await page.goto('/#setup/existing')
  await page.getByRole('textbox', { name: 'Pier folder' }).fill('/storage/piers/ship')
  await page.getByRole('button', { name: 'Browse folders on host' }).click()
  await expect(page.getByRole('textbox', { name: 'Pier folder' })).toHaveValue('/storage/piers/ship')
  await expect(page.getByRole('group', { name: 'Folders on Harbor host' })).toHaveCount(0)
})

for (const { outcome, selected } of [{ outcome: 'cancellation', selected: null }, { outcome: 'selection', selected: '/external/piers' }]) {
  test(`desktop picker restores Browse focus after ${outcome}`, async ({ page }) => {
    await page.addInitScript(() => {
      Object.assign(window, { webkit: { messageHandlers: { harborFolderPicker: { postMessage: () => new Promise<string | null>(resolve => { Object.assign(window, { finishFolderPicker: resolve }) }) } } } })
    })
    await page.goto('/#setup/existing')
    const path = page.getByRole('textbox', { name: 'Pier folder', exact: true })
    await path.fill('/storage/piers/ship')
    const browse = page.getByRole('button', { name: 'Browse folders on host', exact: true })
    await browse.click()
    await expect(browse).toBeDisabled()
    await expect(path).toHaveValue('/storage/piers/ship')
    await page.evaluate(selection => { (window as Window & { finishFolderPicker: (path: string | null) => void }).finishFolderPicker(selection) }, selected)
    await expect(browse).toBeEnabled()
    await expect(path).toHaveValue(selected ?? '/storage/piers/ship')
    await expect(browse).toBeFocused()
    await expect(page.getByRole('dialog', { name: 'Browse folders' })).toHaveCount(0)
  })
}

test('desktop Browse can select another folder while import still validates it', async ({ page }) => {
  await page.addInitScript(() => {
    Object.assign(window, { webkit: { messageHandlers: { harborFolderPicker: { postMessage: async () => '/outside/pier' } } } })
  })
  await page.route('**/api/v1/onboarding/existing', route => route.fulfill({ status: 422, json: { error: { message: 'Choose a folder within the permitted roots on the Harbor host.' } } }))
  await page.goto('/#setup/existing')
  await page.getByRole('button', { name: 'Browse folders on host' }).click()
  await expect(page.getByRole('textbox', { name: 'Pier folder' })).toHaveValue('/outside/pier')
  await page.getByRole('checkbox', { name: 'This matches my pier’s original loom size.' }).check()
  await page.getByRole('button', { name: 'Import ship' }).click()
  await expect(page.getByText('Choose a folder within the permitted roots on the Harbor host.')).toBeVisible()
  await expect(page.getByRole('textbox', { name: 'Pier folder' })).toHaveValue('/outside/pier')
})

test('browser Browse traverses the real directory hierarchy outside configured pier roots', async ({ page }) => {
  await page.goto('/#setup/existing')
  const input = page.getByRole('textbox', { name: 'Pier folder' })
  await page.getByRole('button', { name: 'Browse folders on host' }).click()
  const browser = page.getByRole('dialog', { name: 'Browse folders' })
  await expect(browser).toBeVisible()
  await browser.getByRole('button', { name: 'Open folder ship', exact: true }).click()
  await expect(browser.getByRole('button', { name: 'Open folder child', exact: true })).toBeVisible()
  await expect(input).toHaveValue('')
  await browser.getByRole('button', { name: 'Up one folder' }).click()
  await expect(browser.getByRole('button', { name: 'Open folder ship', exact: true })).toBeVisible()
  await browser.getByRole('button', { name: 'Up one folder' }).click()
  await browser.getByRole('button', { name: 'Open folder another folder & spaces', exact: true }).click()
  await browser.getByRole('button', { name: 'Open folder nested', exact: true }).click()
  await expect(browser.getByText('No visible folders.')).toBeVisible()
  await browser.getByRole('button', { name: 'Use this folder', exact: true }).click()
  await expect(input).toHaveValue('/storage/another folder & spaces/nested')
  await expect(browser).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Browse folders on host' })).toBeFocused()
  await page.getByRole('button', { name: 'Browse folders on host' }).click()
  await browser.getByRole('button', { name: 'Home', exact: true }).click()
  await expect(browser.getByRole('textbox', { name: 'Folder location' })).toHaveValue('/home/test-host')
  await browser.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(input).toHaveValue('/storage/another folder & spaces/nested')
})

test('browser folder location accepts a typed path and recovers from unavailable directories', async ({ page }) => {
  await page.goto('/#setup/existing')
  await page.getByRole('button', { name: 'Browse folders on host' }).click()
  const browser = page.getByRole('dialog', { name: 'Browse folders' })
  const location = browser.getByRole('textbox', { name: 'Folder location' })
  await location.fill('/missing')
  await location.press('Enter')
  await expect(browser.getByRole('alert')).toContainText('Folder is unavailable.')
  await expect(browser.getByRole('button', { name: 'Use this folder', exact: true })).toBeDisabled()
  await location.fill('/storage/another folder & spaces')
  await location.press('Enter')
  await expect(browser.getByRole('button', { name: 'Open folder nested', exact: true })).toBeVisible()
  await expect(browser.getByRole('button', { name: 'Open folder .hidden', exact: true })).toHaveCount(0)
  await browser.getByRole('checkbox', { name: 'Show hidden folders' }).check()
  await expect(browser.getByRole('button', { name: 'Open folder .hidden', exact: true })).toBeVisible()
  await browser.getByRole('button', { name: 'Filesystem root', exact: true }).click()
  await expect(location).toHaveValue('/')
  await expect(browser.getByRole('button', { name: 'Up one folder' })).toBeDisabled()
  await page.keyboard.press('Escape')
  await expect(browser).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Browse folders on host' })).toBeFocused()
})

for (const initialOutcome of ['opens', 'fails']) {
  test(`browser folder location receives focus after the initial listing ${initialOutcome}`, async ({ page }) => {
    let releaseInitial!: () => void
    let releaseNavigation!: () => void
    const initialResponse = new Promise<void>(resolve => { releaseInitial = resolve })
    const navigationResponse = new Promise<void>(resolve => { releaseNavigation = resolve })
    let requests = 0
    await page.route('**/api/v1/folders?*', async route => {
      const current = requests++
      await (current === 0 ? initialResponse : navigationResponse)
      if (current === 0 && initialOutcome === 'fails') return route.fulfill({ status: 422, json: { error: { message: 'Folder is unavailable.' } } })
      return route.fallback()
    })
    await page.goto('/#setup/existing')
    await page.getByRole('button', { name: 'Browse folders on host' }).click()
    const browser = page.getByRole('dialog', { name: 'Browse folders' })
    const location = browser.getByRole('textbox', { name: 'Folder location' })
    await expect(browser).toBeVisible()
    await expect(location).toBeDisabled()
    releaseInitial()
    await expect(location).toBeEnabled()
    await expect(location).toBeFocused()
    if (initialOutcome === 'fails') await expect(browser.getByRole('alert')).toContainText('Folder is unavailable.')

    await browser.getByRole('button', { name: 'Home', exact: true }).click()
    await expect(location).toBeDisabled()
    const hidden = browser.getByRole('checkbox', { name: 'Show hidden folders' })
    await hidden.focus()
    releaseNavigation()
    await expect(location).toHaveValue('/home/test-host')
    await expect(location).toBeEnabled()
    await expect(hidden).toBeFocused()
  })
}

test('folder browser supports keyboard, narrow screens and accessible controls', async ({ page }) => {
  await page.goto('/#setup/existing')
  for (const width of [1280, 390, 320]) {
    await page.setViewportSize({ width, height: 850 })
    await page.getByRole('button', { name: 'Browse folders on host' }).click()
    const browser = page.getByRole('dialog', { name: 'Browse folders' })
    await expect(browser.getByRole('button', { name: 'Open folder ship', exact: true })).toBeEnabled()
    const location = browser.getByRole('textbox', { name: 'Folder location' })
    await location.fill('/storage/another folder & spaces')
    await location.press('Enter')
    await expect(browser.getByRole('button', { name: 'Open folder nested', exact: true })).toBeVisible()
    const breadcrumbs = await browser.getByRole('navigation', { name: 'Folder path' }).locator('button').evaluateAll(elements => elements.map(element => ({ name: element.textContent, width: element.clientWidth, content: element.scrollWidth })))
    expect(breadcrumbs.filter(crumb => crumb.content > crumb.width), 'breadcrumb labels must fit their buttons without overlapping').toEqual([])
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze()
    expect(results.violations).toEqual([])
    await page.keyboard.press('Escape')
    await expect(browser).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Browse folders on host' })).toBeFocused()
  }
})
