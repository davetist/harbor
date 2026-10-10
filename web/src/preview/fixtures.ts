import type { BootRequest, HarborClient, Host, Job, Operation, Schedule, Ship } from '../model'
const storageKey = 'harbor-visual-prototype-v1'
interface PreviewData { ships: Ship[]; jobs: Job[]; schedules: Schedule[]; logs: Record<string, string> }
function initial(): PreviewData {
  const ships: Ship[] = [
    { id: 'preview-planet', name: '~sampel-palnet', nickname: '', pierPath: '/srv/harbor/piers/sampel-palnet', runtimePath: '/srv/harbor/runtimes/vere', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 8080, desiredRunning: true, recoveryBlocked: false, recoveryBlockReason: '', state: 'running', error: '', pierBytes: 2.6 * 1024 ** 3, runtimeVersion: 'Vere 4.6' },
    { id: 'preview-moon', name: '~mister-dister', nickname: '', pierPath: '/srv/harbor/piers/mister-dister', runtimePath: '/srv/harbor/runtimes/vere', loomExponent: 31, httpBindAddress: '127.0.0.1', webPort: 8081, desiredRunning: false, recoveryBlocked: false, recoveryBlockReason: '', state: 'stopped', error: '', pierBytes: 940 * 1024 ** 2, runtimeVersion: 'Vere 4.6' },
  ]
  const schedules: Schedule[] = ships.flatMap(ship => (['pack','meld','roll','chop'] as Operation[]).map((kind, i) => ({ id: `${ship.id}-${kind}`, shipId: ship.id, kind, intervalDays: kind === 'meld' ? 28 : 7, localTime: ['03:00','03:15','03:05','03:10'][i], timezone: 'Europe/Madrid', enabled: true, nextDue: '', lastResult: 'Example · last run completed', afterScheduleId: kind === 'chop' ? `${ship.id}-roll` : undefined })))
  return { ships, jobs: [], schedules, logs: Object.fromEntries(ships.map(ship => [ship.id, 'vere: starting\nloom: mapped 2048MB\npier: loading ship state\nhttp: web interface ready\n\n' + ship.name + ':dojo>'])) }
}
export class PreviewClient implements HarborClient {
  async jobs(id: string) { return structuredClone(this.data.jobs.filter(j => j.shipId === id).reverse()) }
  async jobLogs() { return '[simulated operation output]' }
  async retryBoot(): Promise<string> { throw new Error('Preview setup does not retry real registrations.') }
  async verifyRecovery() { throw new Error('Preview recovery cannot verify a real pier.') }
  async backups() { return [] }
  private data: PreviewData = JSON.parse(localStorage.getItem(storageKey) || 'null') || initial()
  private save() { localStorage.setItem(storageKey, JSON.stringify(this.data)) }
  async host(): Promise<Host> { return { hostname: 'harbor.local', platform: 'linux/arm64', dataDir: '/srv/harbor', pierRoot: '/srv/harbor/piers', allowedRoots: ['/srv/harbor/piers'], timezone: 'Europe/Madrid', loomSizes: [30,31,32], defaultShipHTTPBindAddress: '127.0.0.1', maintenanceProfile: 'Example · enabled by default; weekly Pack and Roll/Chop; 28-day conditional Meld; retains two epochs', maintenanceValidated: true } }
  async listShips() { return structuredClone(this.data.ships) }
  async ship(id: string) { const ship = this.data.ships.find(s => s.id === id); if (!ship) throw new Error('Ship not found.'); return structuredClone(ship) }
  async boot(request: BootRequest) {
    const id = crypto.randomUUID(); const ship: Ship = { ...this.data.ships[0], id, name: request.name || '~new-comet', nickname: request.mode === 'comet' ? request.folderName : '', pierPath: request.pierPath || `/srv/harbor/piers/${request.folderName || 'new-ship'}`, loomExponent: request.loomExponent, state: 'starting', webPort: 8082 }
    this.data.ships.push(ship); this.data.jobs.push({ id, shipId: id, kind: 'boot', phase: 'executing', error: '', startedAt: new Date().toISOString() }); this.save(); return id
  }
  async action(shipId: string, operation: Operation) { const id = crypto.randomUUID(); this.data.jobs.push({ id, shipId, kind: operation, phase: 'executing', error: '', startedAt: new Date().toISOString() }); this.save(); return id }
  async job(id: string) {
    const job = this.data.jobs.find(j => j.id === id); if (!job) throw new Error('Job not found.')
    if (job.phase === 'executing' && Date.now() - Date.parse(job.startedAt) > 1300) {
      const fail = new URLSearchParams(location.search).get('failure') === job.kind
      job.phase = fail ? 'failed' : 'completed'; job.error = fail ? 'Pack could not complete. Your pier has been preserved.' : ''
      const ship = this.data.ships.find(s => s.id === job.shipId)
      if (ship && !fail) { if (['start','boot'].includes(job.kind)) ship.state = 'running'; if (['stop','backup'].includes(job.kind)) { ship.state = 'stopped'; ship.desiredRunning = false } if (job.kind === 'backup') job.result = 'Pier exported. Source ship remains stopped. Keep it stopped when moving to another device.' }
      this.save()
    }
    return structuredClone(job)
  }
  async schedules(id: string) { return structuredClone(this.data.schedules.filter(s => s.shipId === id).map(row => { const job = [...this.data.jobs].reverse().find(j => j.shipId === id && j.kind === row.kind); return job ? { ...row, lastResult: job.phase + (job.error ? ' · ' + job.error : '') } : row })) }
  async saveSchedule(schedule: Schedule) { this.data.schedules = this.data.schedules.map(row => row.id === schedule.id ? schedule : row.shipId === schedule.shipId && ['roll','chop'].includes(row.kind) && ['roll','chop'].includes(schedule.kind) ? { ...row, intervalDays: schedule.intervalDays, timezone: schedule.timezone, anchorDate: schedule.anchorDate } : row); this.save() }
  async resize() {}
  async logs(id: string) { return this.data.logs[id] || 'No output yet.' }
  async input(id: string, command: string) { this.data.logs[id] = (this.data.logs[id] || '') + '\n' + command + '[simulated terminal output]\n'; this.save() }
  async accessCode() { return 'sampel-code-for-the-preview' }
  async openURL() { return '/preview.html#ships' }
  async folders() { return ['/srv/harbor/piers', '/srv/harbor/piers/sampel-palnet'] }
}
