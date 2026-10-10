import type { ShipState } from './model'
export function Mark({ size = 52 }: { size?: number }) {
  return <img className="harbor-mark" src="/harbor-logo.svg" width={size} height={size} alt="" aria-hidden="true"/>
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
