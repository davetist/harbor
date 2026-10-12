import { subscribeEvents } from './events'
import type { BackupRecord, BootRequest, FolderListing, HarborClient, HarborUpdate, Host, Job, Operation, Schedule, Ship } from '../model'
export class APIClient implements HarborClient {
  jobs(id: string) { return this.request<Job[]>(`/ships/${id}/jobs`) }
  async jobLogs(id: string) { return (await this.request<{output: string}>(`/jobs/${id}/logs`)).output }
  async retryBoot(id: string, file?: File) {
    let uploadId: string | undefined
    if (file) { const data = new FormData(); data.append('file', file); uploadId = (await this.request<{uploadId: string}>('/uploads/keyfile', 'POST', data)).uploadId }
    try { return (await this.request<{jobId: string}>(`/jobs/${id}/retry`, 'POST', { uploadId })).jobId }
    catch (error) { if (uploadId) await this.request(`/uploads/${uploadId}`, 'DELETE').catch(() => undefined); throw error }
  }
  verifyRecovery(id: string) { return this.request<void>(`/ships/${id}/recovery/verify`, 'POST', {}) }
  subscribe = subscribeEvents
  backups(id: string) { return this.request<BackupRecord[]>(`/ships/${id}/backups`) }
  private async request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
    const response = await fetch(`/api/v1${path}`, { method, headers: body instanceof FormData ? undefined : { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : body instanceof FormData ? body : JSON.stringify(body) })
    if (!response.ok) { const value = await response.json().catch(() => ({})); throw new Error(value.error?.message || `Harbor returned ${response.status}.`) }
    return response.status === 204 ? undefined as T : response.json()
  }
  host() { return this.request<Host>('/host') }
  updates(manual = false) { return manual ? this.request<HarborUpdate>('/updates/check', 'POST', {}) : this.request<HarborUpdate>('/updates') }
  listShips() { return this.request<Ship[]>('/ships') }
  ship(id: string) { return this.request<Ship>(`/ships/${id}`) }
  measurePier(id: string) { return this.request<Ship>(`/ships/${id}/measure`, 'POST', {}) }
  removeShip(id: string) { return this.request<void>(`/ships/${id}`, 'DELETE') }
  async boot(request: BootRequest, file?: File) {
    if (file) { const data = new FormData(); data.append('file', file); const upload = await this.request<{uploadId: string}>(`/uploads/${request.importArchive ? 'archive' : 'keyfile'}`, 'POST', data); request = { ...request, uploadId: upload.uploadId } }
    try { return (await this.request<{ jobId: string }>(`/onboarding/${request.mode}`, 'POST', request)).jobId } catch (error) { if (request.uploadId && !request.importArchive) await this.request(`/uploads/${request.uploadId}`, 'DELETE').catch(() => undefined); throw error }
  }
  async action(id: string, operation: Operation) { return (await this.request<{jobId: string}>(`/ships/${id}/${operation}`, 'POST', {})).jobId }
  job(id: string) { return this.request<Job>(`/jobs/${id}`) }
  schedules(id: string) { return this.request<Schedule[]>(`/ships/${id}/schedules`) }
  saveSchedule(schedule: Schedule) { return this.request<void>(`/ships/${schedule.shipId}/schedules`, 'PUT', schedule) }
  resize(id: string, rows: number, cols: number) { return this.request<void>(`/ships/${id}/terminal/resize`, 'POST', { rows, cols }) }
  async logs(id: string) { return (await this.request<{output: string}>(`/ships/${id}/logs`)).output }
  input(id: string, input: string) { return this.request<void>(`/ships/${id}/terminal/input`, 'POST', { input }) }
  async accessCode(id: string) { return (await this.request<{code: string}>(`/ships/${id}/access-code`, 'POST', {})).code }
  async openURL(id: string) { return (await this.request<{url: string}>(`/ships/${id}/open`)).url }
  folders(path = '') { return this.request<FolderListing>(`/folders?${new URLSearchParams({ path })}`) }
}
