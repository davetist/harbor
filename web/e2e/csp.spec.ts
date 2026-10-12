import { test, expect } from '@playwright/test'
import { spawn, type ChildProcess } from 'node:child_process'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { resolve, join } from 'node:path'

let service: ChildProcess
let directory: string
let target: string

test.beforeAll(async () => {
  directory = await mkdtemp(join(tmpdir(), 'harbor-csp-'))
  const config = join(directory, 'config.json')
  await writeFile(config, JSON.stringify({ listenAddress: '127.0.0.1:0', dataDir: join(directory, 'data') }), { mode: 0o600 })
  service = spawn(resolve(import.meta.dirname, '../../dist/harbor'), ['serve', '--config', config], { stdio: ['ignore', 'pipe', 'pipe'] })
  let output = ''
  service.stdout!.on('data', chunk => { output += chunk.toString() })
  await expect.poll(() => {
    if (service.exitCode !== null) throw new Error('Disposable CSP service exited before listening')
    target = output.match(/http:\/\/127\.0\.0\.1:\d+/)?.[0] || ''
    return target
  }).not.toBe('')
})

test.afterAll(async () => {
  if (service && service.exitCode === null) {
    service.kill('SIGTERM')
    await expect.poll(() => service.exitCode, { timeout: 15000 }).toBe(0)
  }
  if (directory) await rm(directory, { recursive: true })
})

test('production CSP renders sigils and terminal styles without violations', async ({ page }) => {
  const violations: string[] = []
  page.on('console', message => {
    if (message.type() === 'error' && /Content Security Policy/.test(message.text())) violations.push(message.text())
  })
  // Exercise the embedded production assets and actual response policy. Ship
  // responses are display fixtures; this service has no registered piers.
  const base = { nickname: '', pierPath: '/fixture/pier', runtimePath: '/fixture/vere', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 8080, desiredRunning: false, recoveryBlocked: false, recoveryBlockReason: '', state: 'stopped', error: '' }
  await page.route('**/api/v1/ships', route => route.fulfill({ json: [
    { ...base, id: 'galaxy', name: 'fake-zod', identity: '~zod' },
    { ...base, id: 'planet', name: '~sampel-palnet' },
  ] }))
  await page.route('**/api/v1/ships/*/schedules', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/*/jobs', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/ships/*/logs', route => route.fulfill({ json: { output: 'CSP terminal fixture\r\n' } }))
  await page.route('**/api/v1/updates', route => route.fulfill({ json: { currentVersion: '0.1.0-dev', latestVersion: '', available: false, comparisonKnown: false, prerelease: false, releaseURL: '', checkedAt: null } }))
  const response = await page.goto(target)
  const policy = response!.headers()['content-security-policy']
  expect(policy).toContain("style-src 'self' 'nonce-")
  expect(policy).not.toContain('unsafe-inline')
  expect(policy).not.toContain('unsafe-hashes')
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 850 })
    await page.goto(`${target}/#ships`)
    for (const identity of ['~zod', '~sampel-palnet']) {
      const svg = page.getByRole('img', { name: `Sigil for ${identity}`, exact: true }).locator('svg')
      await expect(svg).toBeVisible()
      await expect(svg).toHaveCSS('display', 'block')
      await expect(svg.locator('rect').first()).toHaveAttribute('fill', '#000000')
    }
    await page.screenshot({ path: test.info().outputPath(`csp-ships-${width}.png`), fullPage: true })
    await page.getByRole('button', { name: 'Manage fake-zod' }).click()
    await expect(page.locator('.detail-header svg')).toBeVisible()
    await expect(page.locator('.detail-header svg')).toHaveCSS('display', 'block')
    await page.getByRole('tab', { name: 'Terminal', exact: true }).click()
    await expect(page.locator('.xterm-rows')).toContainText('CSP terminal fixture')
    const terminalStyles = await page.locator('.xterm-screen style').evaluateAll(elements => elements.map(element => ({ nonce: (element as HTMLStyleElement).nonce, rules: (element as HTMLStyleElement).sheet?.cssRules.length || 0 })))
    const nonce = await page.locator('meta[name="harbor-style-nonce"]').getAttribute('content')
    expect(terminalStyles.length).toBeGreaterThan(0)
    for (const style of terminalStyles) {
      expect(style.nonce).toBe(nonce)
      expect(style.rules).toBeGreaterThan(0)
    }
    expect(await page.evaluate(() => document.createElement('style').nonce)).toBe('')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(violations).toEqual([])
    await page.screenshot({ path: test.info().outputPath(`csp-terminal-${width}.png`), fullPage: true })
  }
})
