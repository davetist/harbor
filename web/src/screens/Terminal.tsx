import { useEffect, useRef, useState } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { terminalDocument } from '../api/terminal-document'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import type { HarborClient, Job, Ship } from '../model'
import { ErrorMessage } from '../components'
export function Terminal({ ship, client }: { ship: Ship; client: HarborClient }) {
  const surface = useRef<HTMLDivElement>(null)
  const [jobs, setJobs] = useState<Job[]>([]); const [selectedJob, setSelectedJob] = useState('')
  useEffect(() => { let alive = true; const refresh = () => { client.jobs(ship.id).then(rows => { if (alive) setJobs(rows) }).catch(e => { if (alive) setError(e.message) }) }; refresh(); const unsubscribe = client.subscribe?.(refresh); return () => { alive = false; unsubscribe?.() } }, [client, ship.id])
  const [command, setCommand] = useState('')
  const [error, setError] = useState(''); const [connectionError, setConnectionError] = useState('')
  useEffect(() => {
    if (!surface.current) return
    let active = true; let previous = ''; let loading = false
    const terminal = new XTerm({ documentOverride: terminalDocument(), disableStdin: true, convertEol: true, reflowCursorLine: true, screenReaderMode: true, fontSize: 12, fontFamily: 'Menlo, monospace', scrollback: 2000, theme: { background: '#0a0a0b', foreground: '#b0b0b7', cursor: '#bbb' } })
    const fit = new FitAddon(); terminal.loadAddon(fit); terminal.open(surface.current)
    const resize = () => fit.fit()
    const rendered = terminal.onRender(resize)
    const resized = terminal.onResize(({ rows, cols }) => { if (!selectedJob) void client.resize(ship.id, rows, cols).catch(() => undefined) })
    const observer = new ResizeObserver(resize); observer.observe(surface.current)
    const update = async () => {
      if (loading) return; loading = true
      try { const value = (selectedJob ? await client.jobLogs(selectedJob) : await client.logs(ship.id)); if (active) { if (value.startsWith(previous)) terminal.write(value.slice(previous.length)); else { terminal.reset(); terminal.write(value) }; previous = value; setConnectionError('') } }
      catch (e) { if (active) setConnectionError(e instanceof Error ? e.message : 'Terminal disconnected.') }
      finally { loading = false }
    }
    void update(); const timer = window.setInterval(update, 750)
    return () => { active = false; clearInterval(timer); observer.disconnect(); rendered.dispose(); resized.dispose(); terminal.dispose() }
  }, [client, ship.id, selectedJob])
  async function submit(event: React.FormEvent) { event.preventDefault(); try { await client.input(ship.id, command + '\n'); setCommand(''); setError('') } catch (e) { setError(e instanceof Error ? e.message : 'Input could not be sent.') } }
  return <section className="terminal"><h2>{selectedJob ? 'Operation output' : 'Dojo'}</h2><label className="field-note">Output<select aria-label="Terminal output source" value={selectedJob} onChange={e => setSelectedJob(e.target.value)}><option value="">Live ship</option>{jobs.map(job => <option key={job.id} value={job.id}>{job.kind} · {job.phase} · {job.startedAt && new Date(job.startedAt).toLocaleString()}</option>)}</select></label><div className="terminal-surface" role="log" aria-label="Ship terminal output"><div className="terminal-viewport" ref={surface}/></div><ErrorMessage message={error || connectionError}/>{!selectedJob && <form onSubmit={submit}><label className="command"><span aria-hidden="true">{ship.name}:dojo&gt;</span><input aria-label="Dojo command" value={command} onChange={e => setCommand(e.target.value)} autoComplete="off" spellCheck={false} disabled={ship.state !== 'running'} maxLength={2000}/></label><button disabled={ship.state !== 'running' || !command}>Send ↵</button></form>}<p className="field-note">{selectedJob ? 'Retained output: up to 2 MiB per operation for the latest 100 jobs.' : 'Commands run directly on this ship.'}</p></section>
}
