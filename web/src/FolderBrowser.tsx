import { useCallback, useEffect, useRef, useState } from 'react'
import type { FolderListing, HarborClient } from './model'
import { ErrorMessage } from './components'

export function FolderBrowser({ client, hostname, initialPath, choose, cancel }: { client: HarborClient; hostname: string; initialPath: string; choose: (path: string) => void; cancel: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const locationInput = useRef<HTMLInputElement>(null)
  const initialFocusPending = useRef(true)
  const breadcrumbs = useRef<HTMLElement>(null)
  const request = useRef(0)
  const [listing, setListing] = useState<FolderListing>()
  const [location, setLocation] = useState(initialPath)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showHidden, setShowHidden] = useState(false)
  useEffect(() => { if (breadcrumbs.current) breadcrumbs.current.scrollLeft = breadcrumbs.current.scrollWidth }, [listing?.path])
  const navigate = useCallback(async (path: string) => {
    const current = ++request.current
    setLoading(true); setError('')
    try {
      const result = await client.folders(path)
      if (current === request.current) { setListing(result); setLocation(result.path) }
    } catch (e) { if (current === request.current) setError(e instanceof Error ? e.message : String(e)) }
    finally { if (current === request.current) setLoading(false) }
  }, [client])
  useEffect(() => {
    const view = dialog.current!
    view.showModal()
    void navigate(initialPath)
    return () => { ++request.current; view.close() }
  }, [initialPath, navigate])
  useEffect(() => {
    if (!loading && initialFocusPending.current) {
      initialFocusPending.current = false
      locationInput.current?.focus()
    }
  }, [loading])
  const close = (path?: string) => { dialog.current?.close(); if (path !== undefined) choose(path); else cancel() }
  const parts = listing?.path.split('/').filter(Boolean) || []
  const entries = listing?.entries.filter(entry => showHidden || !entry.name.startsWith('.')) || []
  return <dialog ref={dialog} className="folder-browser" aria-labelledby="folder-browser-title" aria-describedby="folder-browser-host" onCancel={event => { event.preventDefault(); close() }}>
    <header><h2 id="folder-browser-title">Browse folders</h2><p id="folder-browser-host">Folders on {hostname}</p></header>
    <form className="folder-address" onSubmit={event => { event.preventDefault(); void navigate(location) }}>
      <input ref={locationInput} aria-label="Folder location" disabled={loading} value={location} onChange={event => setLocation(event.target.value)} spellCheck={false} autoComplete="off"/>
      <button type="submit" disabled={loading}>Go</button>
    </form>
    <div className="folder-toolbar">
      <button type="button" disabled={loading || !listing?.parent} onClick={() => listing?.parent && void navigate(listing.parent)} aria-label="Up one folder">↑ Up</button>
      <button type="button" disabled={loading} onClick={() => void navigate('')}>Home</button>
      <label><input type="checkbox" checked={showHidden} onChange={event => setShowHidden(event.target.checked)}/>Show hidden folders</label>
    </div>
    <nav ref={breadcrumbs} className="folder-breadcrumbs" aria-label="Folder path">
      <button type="button" aria-label="Filesystem root" disabled={loading} onClick={() => void navigate('/')}>/</button>
      {parts.map((part, index) => <button type="button" key={index} disabled={loading} aria-current={index === parts.length - 1 ? 'location' : undefined} onClick={() => void navigate('/' + parts.slice(0, index + 1).join('/'))}>{part}<span aria-hidden="true"> /</span></button>)}
    </nav>
    <ErrorMessage message={error}/>
    <div className="folder-contents" aria-busy={loading}>
      {loading && <p role="status">Opening folder…</p>}
      {!loading && !error && entries.length === 0 && <p>No visible folders.</p>}
      <ul aria-label="Folders">{entries.map(entry => <li key={entry.name}><button type="button" className="folder-entry" disabled={loading} aria-label={`Open folder ${entry.name}`} onClick={() => void navigate(entry.path)}><svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h7l2 3h9v11H3V6Z" fill="none" stroke="currentColor" strokeWidth="1.4"/></svg><span>{entry.name}</span><span aria-hidden="true">→</span></button></li>)}</ul>
    </div>
    <footer><button type="button" onClick={() => close()}>Cancel</button><button type="button" disabled={loading || Boolean(error) || !listing || location !== listing.path} onClick={() => listing && close(listing.path)}>Use this folder</button></footer>
  </dialog>
}
