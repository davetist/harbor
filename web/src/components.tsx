import type { ShipState } from './model'
export function Mark({ size = 52 }: { size?: number }) {
  return <svg width={size} height={size} viewBox="0 0 56 56" fill="none" aria-hidden="true"><circle cx="28" cy="28" r="24.5" stroke="currentColor" strokeWidth="2.5"/><path d="M18 18v13c0 6 4 10 10 10s10-4 10-10V18M12 28h32" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"/></svg>
}
export function Wordmark() { return <div className="wordmark"><Mark/><span>harbor</span></div> }
export function Arrow() { return <span aria-hidden="true">→</span> }
export function State({ value }: { value: ShipState }) {
  const labels: Record<ShipState, string> = { stopped: 'Stopped', starting: 'Starting', running: 'Running', stopping: 'Stopping', maintaining: 'Maintaining', externally_running: 'Managed elsewhere', failed: 'Needs attention' }
  return <span className={`ship-state ${value}`}><i/>{labels[value]}</span>
}
export function ErrorMessage({ message }: { message: string }) { return message ? <p className="error" role="alert">{message}</p> : null }
export const operationNames = { start: 'Start', stop: 'Stop', pack: 'Pack', meld: 'Meld', roll: 'Roll', chop: 'Chop', update: 'Update runtime', replay: 'Replay', backup: 'Export pier' }
export function formatBytes(bytes?: number) { if (bytes === undefined) return 'Not measured'; return bytes > 1024 ** 3 ? `${(bytes / 1024 ** 3).toFixed(1)} GB` : `${Math.round(bytes / 1024 ** 2)} MB` }
