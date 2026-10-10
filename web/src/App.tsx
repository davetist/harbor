import { useEffect, useRef, useState } from 'react'
import type { BootMode, BootRequest, HarborClient, Host, Job, Operation, Ship } from './model'
import { ErrorMessage, operationNames } from './components'
import { Welcome } from './screens/Welcome'
import { Onboarding } from './screens/Onboarding'
import { Ships } from './screens/Ships'
import { ShipDetail } from './screens/ShipDetail'
import './styles.css'
export function App({ client, preview = false }: { client: HarborClient; preview?: boolean }) {
  const main = useRef<HTMLElement>(null); const previousRoute = useRef('')
  const [host, setHost] = useState<Host>(); const [ships, setShips] = useState<Ship[]>([])
  const [requestedRoute, setRoute] = useState(location.hash.slice(1) || (new URLSearchParams(location.search).get('screen') === 'ships' ? 'ships' : ''))
  const route = requestedRoute || (ships.length > 0 ? 'ships' : 'welcome')
  const hostReady = host !== undefined
  useEffect(() => {
    if (!hostReady) return
    const sameShip = route.startsWith('ship/') && previousRoute.current.split('/').slice(0, 2).join('/') === route.split('/').slice(0, 2).join('/')
    if (!sameShip) { const heading = main.current?.querySelector('h1'); heading?.setAttribute('tabindex', '-1'); heading?.focus({ preventScroll: true }) }
    previousRoute.current = route
  }, [route, hostReady])
  const [connectionError, setConnectionError] = useState('');
 const [error, setError] = useState(''); const [status, setStatus] = useState(''); const [busy, setBusy] = useState(false)
  const [values, setValues] = useState<BootRequest>({ mode: 'comet', folderName: '', name: '', pierPath: '', loomExponent: 31 })
  const [failedBoot, setFailedBoot] = useState<Job>(); const [retryFile, setRetryFile] = useState<File>()
  const [bootReady, setBootReady] = useState(false); const [bootShipId, setBootShipId] = useState('')
  const navigate = (next: string) => { location.hash = next; setRoute(next); setError(''); setStatus('') }
  useEffect(() => { const change = () => setRoute(location.hash.slice(1)); addEventListener('hashchange', change); return () => removeEventListener('hashchange', change) }, [])
  useEffect(() => { let alive = true; Promise.all([client.host(), client.listShips()]).then(([h, list]) => { if (alive) { setHost(h); setShips(list) } }).catch(e => { if (alive) setConnectionError(e.message) }); return () => { alive = false } }, [client])
  useEffect(() => client.subscribe?.(() => { Promise.all([client.host(), client.listShips()]).then(([h, list]) => { setHost(h); setShips(list); setConnectionError('') }).catch(() => setConnectionError('The Harbor service is disconnected. Reconnecting…')) }), [client])
  useEffect(() => { let active = true; const refresh = () => { Promise.all([client.host(), client.listShips()]).then(([h, list]) => { if (active) { setHost(h); setShips(list); setConnectionError('') } }).catch(() => { if (active) setConnectionError('The Harbor service is disconnected. Reconnecting…') }) }; const timer = setInterval(refresh, 60000); addEventListener('focus', refresh); return () => { active = false; clearInterval(timer); removeEventListener('focus', refresh) } }, [client])
  useEffect(() => { if (!route.startsWith('boot/')) return; let alive = true; setBootReady(false); setFailedBoot(undefined); setRetryFile(undefined); const check = async () => { try { const job = await client.job(route.split('/')[1]); if (alive) { if (job.phase === 'completed') { setBootReady(true); setBootShipId(job.shipId); setError(''); setShips(await client.listShips()) } else if (job.phase === 'failed' || job.phase === 'interrupted') { setFailedBoot(job); setError(job.error || 'Boot interrupted. Your pier has been preserved.') } } } catch (e) { if (alive) setError(String(e)) } }; void check(); const timer = setInterval(check, 400); return () => { alive = false; clearInterval(timer) } }, [client, route])
  async function run(ship: Ship, operation: Operation) { setBusy(true); setError(''); setStatus(`${operationNames[operation]} in progress…`); try { const id = await client.action(ship.id, operation); let result = ''; for (;;) { const job = await client.job(id); if (job.phase === 'completed') { result = job.result || ''; break } if (['failed','skipped','interrupted'].includes(job.phase)) throw new Error(job.error || `${operationNames[operation]} did not complete.`); await new Promise(resolve => setTimeout(resolve, 400)) } setShips(await client.listShips()); setStatus(result || `${operationNames[operation]} completed`) } catch (e) { setStatus(''); setError(e instanceof Error ? e.message : 'Operation failed.') } finally { setBusy(false) } }
  async function retryBoot() { if (!failedBoot) return; setBusy(true); try { const id = await client.retryBoot(failedBoot.id, retryFile); navigate(`boot/${id}`) } catch (e) { setError(e instanceof Error ? e.message : 'Unable to retry setup.') } finally { setBusy(false) } }
  async function primary(ship: Ship) { if (ship.state === 'running') { try { window.open(await client.openURL(ship.id), '_blank', 'noopener,noreferrer') } catch (e) { setError(String(e)) } } else await run(ship, 'start') }
  const [, selectedId, tab = 'overview'] = route.split('/'); const selected = ships.find(ship => ship.id === selectedId)
  return <><main ref={main}>{!host ? <section className="connecting"><h1>Harbor</h1><p>{connectionError ? 'The Harbor service could not be reached.' : 'Connecting…'}</p><ErrorMessage message={connectionError}/>{connectionError && <button onClick={() => location.reload()}>Try again →</button>}</section> : route.startsWith('setup/') ? <Onboarding key={selectedId} mode={selectedId as BootMode} host={host} client={client} values={values} change={setValues} back={() => navigate('welcome')} booted={id => navigate(`boot/${id}`)}/> : route.startsWith('boot/') ? <section className="boot-screen"><div className="boot-message" role="status" aria-live="polite">{!bootReady && !error && <span className="spinner" aria-hidden="true"/>}<div><h1>{error ? 'Your ship needs attention' : bootReady ? 'Your ship is ready' : 'Booting your ship'}</h1><p>{bootReady ? 'Welcome to your corner of Urbit.' : 'This can take a few minutes.'}</p></div></div><ErrorMessage message={error}/><button disabled={!bootReady} onClick={() => { const ship = ships.find(ship => ship.id === bootShipId); if (ship) void primary(ship) }}>Open ship →</button>{failedBoot?.retryable && <div>{failedBoot.kind === 'boot_keyfile' && <label>Select the keyfile again<input type="file" accept=".key" aria-label="Retry keyfile" onChange={e => setRetryFile(e.target.files?.[0])}/></label>}<button disabled={busy || (failedBoot.kind === 'boot_keyfile' && !retryFile)} onClick={() => void retryBoot()}>Retry setup →</button></div>}{(bootReady || failedBoot) && <button className="text-button" onClick={() => navigate('ships')}>Your ships →</button>}</section> : route.startsWith('ship/') && selected ? <ShipDetail key={selected.id} ship={selected} host={host} client={client} tab={tab} selectTab={next => navigate(`ship/${selected.id}/${next}`)} back={() => navigate('ships')} run={op => run(selected, op)} busy={busy}/> : route === 'ships' ? <Ships ships={ships} manage={id => navigate(`ship/${id}/overview`)} add={() => navigate('welcome')} action={primary}/> : <Welcome choose={mode => navigate(`setup/${mode}`)} hasShips={ships.length > 0} back={() => navigate('ships')}/>}</main>
    {!route.startsWith('boot/') && host && <div className="feedback"><ErrorMessage message={error || connectionError}/>{status && <p role="status">{status}</p>}</div>}
    <footer className="app-footer">{preview ? <span>Visual prototype · simulated ships and operations</span> : host && <span>{host.hostname}</span>}<span>Harbor</span></footer>
  </>
}
