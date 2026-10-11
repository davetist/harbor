import { useEffect, useRef, useState } from 'react'
import type { HarborClient, Ship } from '../model'
import { ErrorMessage } from '../components'

export function RemoveShip({ ship, hostname, client, busy, removed }: { ship: Ship; hostname: string; client: HarborClient; busy: boolean; removed: () => void }) {
  const [confirming, setConfirming] = useState(false)
  const reason = ship.recoveryBlocked ? "Resolve this ship's recovery hold before removing it from Harbor." : ship.desiredRunning || !['stopped', 'failed'].includes(ship.state) ? 'Stop this ship before removing it from Harbor.' : busy ? 'Wait for the current operation to finish.' : ''
  return <div className="ship-removal">
    <button className="text-button" disabled={Boolean(reason)} aria-describedby="ship-removal-note" onClick={() => setConfirming(true)}>Remove from Harbor</button>
    <p id="ship-removal-note">{reason || 'The pier stays on disk.'}</p>
    {confirming && <RemovalConfirmation ship={ship} hostname={hostname} client={client} reason={reason} cancel={() => setConfirming(false)} removed={removed}/>}
  </div>
}

function RemovalConfirmation({ ship, hostname, client, reason, cancel, removed }: { ship: Ship; hostname: string; client: HarborClient; reason: string; cancel: () => void; removed: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const cancelButton = useRef<HTMLButtonElement>(null)
  const [removing, setRemoving] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { const view = dialog.current!; view.showModal(); cancelButton.current?.focus(); return () => view.close() }, [])
  function close() { dialog.current?.close(); cancel() }
  async function remove() {
    setRemoving(true); setError('')
    try { await client.removeShip(ship.id); removed() }
    catch (e) { setError(e instanceof Error ? e.message : 'Unable to remove this ship.'); setRemoving(false) }
  }
  return <dialog ref={dialog} className="removal-confirmation" aria-labelledby="removal-title" aria-describedby="removal-description" onCancel={event => { event.preventDefault(); if (!removing) close() }}>
    <h2 id="removal-title">Remove {ship.nickname || ship.name} from Harbor?</h2>
    <div id="removal-description">
      <p>Your ship will still be available at this path on {hostname}:</p>
      <code className="preserved-pier-path">{ship.pierPath}</code>
      <p>You can add it back with “Use existing pier”.</p>
      <p>Harbor will forget its maintenance settings and history. Pier files and exported archives will stay on disk.</p>
    </div>
    <ErrorMessage message={error || reason}/>
    <footer><button ref={cancelButton} disabled={removing} onClick={close}>Cancel</button><button disabled={removing || Boolean(reason)} onClick={() => void remove()}>{removing ? 'Removing…' : 'Remove from Harbor'}</button></footer>
  </dialog>
}
