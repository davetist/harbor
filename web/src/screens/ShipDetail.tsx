import { lazy, Suspense, useEffect, useState } from 'react'
import type { HarborClient, Host, Operation, Ship } from '../model'
import { ErrorMessage, formatBytes, State } from '../components'
import { ShipSigil } from '../ShipSigil'
import { Maintenance } from './Maintenance'
import { copyText } from '../clipboard'
import { RemoveShip } from './RemoveShip'
import { APIError } from '../api/client'
import { InterruptConfirmation } from './InterruptConfirmation'
const Terminal = lazy(() => import('./Terminal').then(module => ({ default: module.Terminal })))
export function ShipDetail({ ship, host, client, tab, selectTab, back, run, busy, removed, measured, initialOpenFailure = '' }: { ship: Ship; host: Host; client: HarborClient; tab: string; selectTab: (tab: string) => void; back: () => void; run: (op: Operation) => void; busy: boolean; removed: () => void; measured: (ship: Ship) => void; initialOpenFailure?: string }) {
  const [enabledSchedules, setEnabledSchedules] = useState<number>();
  useEffect(() => { let active = true; const refresh = () => { client.schedules(ship.id).then(rows => { if (active) setEnabledSchedules(rows.filter(row => row.enabled).length) }).catch(() => { if (active) setEnabledSchedules(undefined) }) }; refresh(); const unsubscribe = client.subscribe?.(refresh); return () => { active = false; unsubscribe?.() } }, [client, ship.id, busy]);
  const [code, setCode] = useState('');
  const [copied, setCopied] = useState(false)
  useEffect(() => setCopied(false), [code])
 useEffect(() => { if (ship.state !== 'running') setCode('') }, [ship.state]); const [error, setError] = useState('')
  async function reveal() { if (code) { setCode(''); return } try { setCode(await client.accessCode(ship.id)) } catch (e) { setError(e instanceof Error ? e.message : 'Unable to retrieve your code.') } }
  async function copyCode() { setError(''); setCopied(false); try { await copyText(code); setCopied(true) } catch { setError('Clipboard unavailable. Select and copy the code manually.') } }
  const [measuring, setMeasuring] = useState(false)
  async function refreshPierSize() { if (measuring) return; setMeasuring(true); setError(''); try { measured(await client.measurePier(ship.id)) } catch (e) { setError(e instanceof Error ? e.message : 'Unable to measure this pier. Try again.') } finally { setMeasuring(false) } }
  const [opening, setOpening] = useState(false)
  const [unresponsive, setUnresponsive] = useState(initialOpenFailure)
  const [confirmingInterrupt, setConfirmingInterrupt] = useState(false)
  const [webStatus, setWebStatus] = useState('')
  useEffect(() => { if (ship.state !== 'running') { setUnresponsive(''); setWebStatus('') } }, [ship.state])
  async function openShip() {
    if (opening) return
    setOpening(true); setError(''); setWebStatus('')
    try { const url = await client.openURL(ship.id); setUnresponsive(''); window.open(url, '_blank', 'noopener,noreferrer') }
    catch (e) { if (e instanceof APIError && e.code === 'ship_unresponsive') setUnresponsive(e.message); else setError(e instanceof Error ? e.message : 'Unable to open the ship.') }
    finally { setOpening(false) }
  }
  return <section className="detail"><button className="text-button detail-back" onClick={back}>← Your ships</button><header className="detail-header"><div className="detail-title"><ShipSigil ship={ship} size={48}/><div><h1>{ship.name}</h1><State value={ship.state}/></div></div><button disabled={ship.state !== 'running' || opening} onClick={() => void openShip()}>{opening ? 'Checking ship…' : 'Open ship →'}</button></header>
    <nav className="tabs" role="tablist" aria-label="Ship management">{['Overview','Maintenance','Terminal'].map(name => <button key={name} role="tab" id={`ship-${ship.id}-${name.toLowerCase()}-tab`} aria-controls={`ship-${ship.id}-${name.toLowerCase()}-panel`} tabIndex={tab === name.toLowerCase() ? 0 : -1} aria-selected={tab === name.toLowerCase()} onKeyDown={event => {
      const names = ['overview', 'maintenance', 'terminal']; const current = names.indexOf(tab)
      const next = event.key === 'ArrowRight' ? (current + 1) % 3 : event.key === 'ArrowLeft' ? (current + 2) % 3 : event.key === 'Home' ? 0 : event.key === 'End' ? 2 : -1
      if (next < 0) return; event.preventDefault(); (event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role=tab]')[next])?.focus(); selectTab(names[next])
    }} onClick={() => selectTab(name.toLowerCase())}>{name}</button>)}</nav>
    <ErrorMessage message={error || ship.error || ship.recoveryBlockReason}/>{ship.recoveryBlocked && <button className="recovery-verify" onClick={async () => { try { await client.verifyRecovery(ship.id); setError('') } catch (e) { setError(e instanceof Error ? e.message : 'The recovery hold remains.') } }}>Verify recorded outcome →</button>}
    {unresponsive && <aside className="web-recovery" aria-label="Ship web recovery"><p role="alert">{unresponsive}</p><div className="recovery-actions"><button disabled={opening || ship.state !== 'running'} onClick={() => void openShip()}>Try Open again</button><button disabled={busy || ship.recoveryBlocked || ship.state !== 'running' || opening} onClick={() => setConfirmingInterrupt(true)}>Interrupt current event…</button><button className="text-button" onClick={() => selectTab('terminal')}>Inspect terminal →</button></div></aside>}
    {webStatus && <p className="web-status" role="status">{webStatus}</p>}
    {confirmingInterrupt && <InterruptConfirmation ship={ship} client={client} busy={busy} cancel={() => setConfirmingInterrupt(false)} sent={() => { setConfirmingInterrupt(false); setWebStatus('Interrupt sent. Wait a moment, then try Open again to check the response.') }} responsive={() => { setConfirmingInterrupt(false); setUnresponsive(''); setWebStatus('The ship is responding again. Try Open ship.') }}/>}
    {['overview', 'maintenance', 'terminal'].map(panel => <div key={panel} role="tabpanel" id={`ship-${ship.id}-${panel}-panel`} aria-labelledby={`ship-${ship.id}-${panel}-tab`} hidden={tab !== panel} tabIndex={0}>{tab === panel && (tab === 'maintenance' ? <Maintenance ship={ship} client={client} run={run} busy={busy}/> : tab === 'terminal' ? <Suspense fallback={<p role="status">Connecting terminal…</p>}><Terminal ship={ship} client={client}/></Suspense> : <div className="overview"><h2>Your ship, at a glance.</h2><div className="facts"><div><span className="pier-size-label" title={ship.measuredAt ? `Measured ${new Date(ship.measuredAt).toLocaleString()}` : undefined}>Pier size<button className="pier-size-refresh" type="button" aria-label="Refresh pier size" title="Refresh pier size" aria-busy={measuring} disabled={measuring} onClick={() => void refreshPierSize()}><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M3 12a9 9 0 0 1 15.36-6.36L21 8M21 3v5h-5M21 12a9 9 0 0 1-15.36 6.36L3 16M8 16H3v5"/></svg></button></span><strong aria-live="polite" aria-busy={measuring}>{formatBytes(ship.pierBytes)}</strong></div><div><span>Runtime</span><strong>{ship.runtimeVersion || 'Not reported'}</strong></div><div><span>Loom</span><strong>{2 ** (ship.loomExponent - 30)} GiB</strong></div></div><div className="code-row"><div><h3>Access code</h3><p>Your login to this ship.</p></div><button disabled={ship.state !== 'running'} onClick={reveal}>{code ? 'Hide code' : 'Reveal code'}</button></div>{code && <div className="revealed-code"><code>{code}</code><button onClick={() => void copyCode()}>{copied ? 'Copied' : 'Copy'}</button>{copied && <span className="sr-only" role="status">Access code copied</span>}</div>}<details className="advanced technical"><summary>Ship details</summary><dl><dt>Host</dt><dd>{host.hostname}</dd><dt>Pier</dt><dd>{ship.pierPath}</dd><dt>Runtime</dt><dd>{ship.runtimePath}</dd><dt>HTTP listener</dt><dd>{ship.httpBindAddress}:{ship.webPort}</dd></dl></details><div className="overview-bottom"><div><span className="dot"/> Automatic maintenance <small>{enabledSchedules === undefined ? 'Not available' : enabledSchedules === 0 ? 'Paused' : enabledSchedules === 4 ? 'Enabled' : `${enabledSchedules} operations enabled`}</small></div><button className={ship.state === 'running' ? 'stop-button' : ''} disabled={busy || ship.recoveryBlocked} onClick={() => run(ship.state === 'running' ? 'stop' : 'start')}>{ship.state === 'running' ? 'Stop ship' : 'Start ship'}</button></div></div>)}</div>)}
    {tab === 'overview' && <RemoveShip ship={ship} hostname={host.hostname} client={client} busy={busy} removed={removed}/>}
  </section>
}
