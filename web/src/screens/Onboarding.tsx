import { useEffect, useRef, useState } from 'react'
import type { BootMode, BootRequest, HarborClient, Host } from '../model'
import { Arrow, ErrorMessage } from '../components'
import { chooseNativeFolder } from '../native'
import { FolderBrowser } from '../FolderBrowser'
export function Onboarding({ mode, host, client, values, change, back, booted }: { mode: BootMode; host: Host; client: HarborClient; values: BootRequest; change: (values: BootRequest) => void; back: () => void; booted: (id: string) => void }) {
  const [file, setFile] = useState<File>(); const [archive, setArchive] = useState(false)
  const [error, setError] = useState(''); const [busy, setBusy] = useState(false)
  const [folderBrowser, setFolderBrowser] = useState(false)
  const [browsing, setBrowsing] = useState(false)
  const browseButton = useRef<HTMLButtonElement>(null)
  const nativeFocusPending = useRef(false)
  useEffect(() => {
    if (!browsing && nativeFocusPending.current) {
      nativeFocusPending.current = false
      browseButton.current?.focus()
    }
  }, [browsing])
  const update = (key: keyof BootRequest, value: string | number) => change({ ...values, [key]: value })
  const title = { comet: 'Create a comet', keyfile: 'Boot from a keyfile', existing: 'Use an existing pier' }[mode]
  async function browse(path = values.pierPath || host.pierRoot || '') {
    setError(''); setBrowsing(true)
    try {
      const selection = chooseNativeFolder(path, mode)
      if (selection) {
        nativeFocusPending.current = true
        const selected = await selection
        if (selected) update('pierPath', selected)
      } else setFolderBrowser(true)
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBrowsing(false) }
  }
  async function submit(event: React.FormEvent) {
    event.preventDefault(); setError(''); setBusy(true)
    try { const id = await client.boot({ ...values, mode, importArchive: archive }, file); booted(id) }
    catch (e) { setError(e instanceof Error ? e.message : 'Unable to start your ship.'); setBusy(false) }
  }
  return <section className="wizard">
    <button className="text-button back" onClick={back} aria-label="Back">←</button>
    <header><h1>{title}</h1><p>{mode === 'comet' ? 'A new identity. A place of your own.' : mode === 'keyfile' ? 'Use your keyfile for the first boot of your ship.' : 'Bring an existing ship into Harbor.'}</p></header>
    <form onSubmit={submit}>
      {mode === 'keyfile' && <><label>Ship identity<input value={values.name} onChange={e => update('name', e.target.value)} placeholder="~sampel-palnet" required autoComplete="off"/></label><label>Keyfile<input className="file" type="file" accept=".key" aria-label="Keyfile" onChange={e => { setFile(e.target.files?.[0]); if (!values.name) update('name', (e.target.files?.[0]?.name ?? '').replace(/\.key$/, '')) }} required/></label><p className="field-note">Select a file from this computer. Confirm the identity above.</p></>}
      {mode === 'existing' && <div className="segmented"><button type="button" aria-pressed={!archive} className={!archive ? 'selected' : ''} onClick={() => setArchive(false)}>Existing folder</button><button type="button" aria-pressed={archive} className={archive ? 'selected' : ''} onClick={() => setArchive(true)}>Upload an archive</button></div>}
      <p className="field-note">Folders are on {host.hostname}. Uploaded files come from this computer.</p>
      {archive ? <label>Pier archive<input className="file" type="file" accept=".gz,.tgz" aria-label="Pier archive" onChange={e => setFile(e.target.files?.[0])} required/></label> : <>
        {mode === 'comet' && <label>Folder name<input value={values.folderName} onChange={e => update('folderName', e.target.value)} placeholder="my-comet" required pattern="[A-Za-z0-9][A-Za-z0-9._-]*"/><span className="field-note">A local label. Urbit generates the identity.</span></label>}
        <div className="path-field"><label htmlFor="pier-path">{mode === 'existing' ? 'Pier folder' : 'Destination'}</label><div className="input-action"><input id="pier-path" value={values.pierPath} placeholder={host.pierRoot ?? host.dataDir} onChange={e => update('pierPath', e.target.value)} required={mode === 'existing'}/><button ref={browseButton} type="button" aria-label="Browse folders on host" disabled={browsing} onClick={() => void browse()}>Browse</button></div></div>
      </>}
      <details className="advanced"><summary>Advanced settings <span>{2 ** (values.loomExponent - 30)} GiB loom</span></summary><label>Loom size<select value={values.loomExponent} onChange={e => update('loomExponent', Number(e.target.value))}>{(mode === 'existing' ? host.loomSizes : [31]).map(size => <option key={size} value={size}>{2 ** (size - 30)} GiB</option>)}</select></label><p className="field-note">Memory available to your ship. Harbor keeps this setting across restarts.</p></details>
      {mode === 'existing' && !archive && <label className="loom-confirm"><input type="checkbox" required checked={Boolean(values.loomConfirmed)} onChange={e => change({ ...values, loomConfirmed: e.target.checked })}/> This matches my pier’s original loom size.</label>}
      <div className="setup-maintenance"><span className={(host.maintenanceValidated && values.loomExponent === 31 && !archive) ? "dot" : "dot unvalidated"}/><div><strong>Automatic maintenance</strong><p>{archive ? 'Selected from the archive’s original loom setting after import.' : values.loomExponent !== 31 ? 'This loom size has no measured automatic profile yet; its schedules will start paused.' : host.maintenanceProfile}</p></div></div>
      {archive && <label>Destination on {host.hostname}<input value={values.pierPath} placeholder={host.pierRoot ?? host.dataDir} onChange={e => update('pierPath', e.target.value)}/><span className="field-note">A new folder will be created here on {host.hostname}.</span></label>}
      <ErrorMessage message={error}/><footer className="form-actions"><button disabled={busy || browsing || folderBrowser} type="submit">{busy ? 'Preparing…' : mode === 'comet' ? 'Boot comet' : mode === 'existing' ? 'Import ship' : 'Boot ship'} <Arrow/></button></footer>
    </form>
    {folderBrowser && <FolderBrowser client={client} hostname={host.hostname} initialPath={values.pierPath || host.pierRoot || ''} choose={path => { update('pierPath', path); setFolderBrowser(false) }} cancel={() => setFolderBrowser(false)}/>}
  </section>
}
