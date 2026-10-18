import { useEffect, useRef, useState } from 'react'
import type { HarborClient, Ship } from '../model'
import { ErrorMessage } from '../components'

export function RemoveShip({ ship, hostname, client, busy, removed }: { ship: Ship; hostname: string; client: HarborClient; busy: boolean; removed: () => void }) {
  const [confirming, setConfirming] = useState(false)
  const reason = ship.recoveryBlocked && !ship.pierDeletionIdentity ? "Resolve this ship's recovery hold before removing it from Harbor." : ship.desiredRunning || !['stopped', 'failed'].includes(ship.state) ? 'Stop this ship before removing it from Harbor.' : busy ? 'Wait for the current operation to finish.' : ''
  return <div className="ship-removal">
    <button className="text-button" disabled={Boolean(reason)} aria-describedby="ship-removal-note" onClick={() => setConfirming(true)}>Remove from Harbor</button>
    <p id="ship-removal-note">{reason || (ship.pierDeletionIdentity ? 'Pier deletion is pending. Confirm permanent deletion again to finish removal.' : 'Keep the pier on disk, or permanently delete its files when removing.')}</p>
    {confirming && <RemovalConfirmation ship={ship} hostname={hostname} client={client} reason={reason} cancel={() => setConfirming(false)} removed={removed}/>}
  </div>
}

function RemovalConfirmation({ ship, hostname, client, reason, cancel, removed }: { ship: Ship; hostname: string; client: HarborClient; reason: string; cancel: () => void; removed: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const cancelButton = useRef<HTMLButtonElement>(null)
  const [removing, setRemoving] = useState(false)
  const [error, setError] = useState('')
  const [deletePier, setDeletePier] = useState(false)
  useEffect(() => { const view = dialog.current!; view.showModal(); cancelButton.current?.focus(); return () => view.close() }, [])
  function close() { dialog.current?.close(); cancel() }
  async function remove() {
    setRemoving(true); setError('')
    try { await client.removeShip(ship.id, deletePier ? { deletePier: true, confirmPierPath: ship.pierPath } : undefined); removed() }
    catch (e) { setError(e instanceof Error ? e.message : 'Unable to remove this ship.'); setRemoving(false) }
  }
  return <dialog ref={dialog} className="removal-confirmation" aria-labelledby="removal-title" aria-describedby="removal-description" onCancel={event => { event.preventDefault(); if (!removing) close() }}>
    <h2 id="removal-title">Remove {ship.nickname || ship.name} from Harbor?</h2>
    <div id="removal-description">
      <p>{deletePier ? 'Permanently delete this pier folder and everything inside it' : 'Your ship will still be available at this path'} on {hostname}:</p>
      <code className="preserved-pier-path">{ship.pierPath}</code>
      {!deletePier && <p>You can add it back with “Use existing pier”.</p>}
      <p>Harbor will forget its maintenance settings and history. {deletePier ? 'Exported archives stay on disk.' : 'Pier files and exported archives will stay on disk.'}</p>
      <label className="delete-pier-option"><input type="checkbox" checked={deletePier} disabled={removing} aria-describedby={deletePier ? 'delete-pier-warning' : undefined} onChange={event => setDeletePier(event.target.checked)}/><span>Permanently delete pier files</span></label>
      {deletePier && <p id="delete-pier-warning" className="delete-pier-warning" role="status">This cannot be undone. All ship data and files inside this pier folder will be permanently deleted.</p>}
    </div>
    <ErrorMessage message={error || reason || (ship.pierDeletionIdentity && !deletePier ? 'Deletion has already started. Select permanent deletion to finish removal.' : '')}/>
    <footer><button ref={cancelButton} disabled={removing} onClick={close}>Cancel</button><button className={deletePier ? 'delete-pier-button' : undefined} disabled={removing || Boolean(reason) || Boolean(ship.pierDeletionIdentity && !deletePier)} onClick={() => void remove()}>{removing ? 'Removing…' : deletePier ? 'Remove and delete pier' : 'Remove from Harbor'}</button></footer>
  </dialog>
}
