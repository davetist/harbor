import { useEffect, useRef, useState } from 'react'
import type { HarborClient, Ship } from '../model'
import { APIError } from '../api/client'
import { ErrorMessage } from '../components'

export function InterruptConfirmation({ ship, client, busy, cancel, sent, responsive }: { ship: Ship; client: HarborClient; busy: boolean; cancel: () => void; sent: () => void; responsive: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const cancelButton = useRef<HTMLButtonElement>(null)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const reason = ship.recoveryBlocked ? ship.recoveryBlockReason : ship.state !== 'running' ? 'Only a running ship can be interrupted.' : busy ? 'Wait for the current operation to finish.' : ''
  useEffect(() => { const view = dialog.current!; view.showModal(); cancelButton.current?.focus(); return () => view.close() }, [])
  function close() { dialog.current?.close(); cancel() }
  async function interrupt() {
    setSending(true); setError('')
    try { await client.interrupt(ship.id); dialog.current?.close(); sent() }
    catch (e) {
      if (e instanceof APIError && e.code === 'ship_responsive') { dialog.current?.close(); responsive(); return }
      setError(e instanceof Error ? e.message : 'Unable to send an interrupt.'); setSending(false)
    }
  }
  return <dialog ref={dialog} className="removal-confirmation interrupt-confirmation" aria-labelledby="interrupt-title" aria-describedby="interrupt-description" onCancel={event => { event.preventDefault(); if (!sending) close() }}>
    <h2 id="interrupt-title">Interrupt the current event?</h2>
    <div id="interrupt-description">
      <p>Harbor will send Ctrl+C to {ship.nickname || ship.name} to cancel its current computation. A long compilation may still be making progress; you can cancel this dialog and wait instead.</p>
      <p>This does not restart the ship. The canceled work may need to be retried. Harbor will check again before sending the interrupt.</p>
    </div>
    <ErrorMessage message={error || reason}/>
    <footer><button ref={cancelButton} disabled={sending} onClick={close}>Cancel</button><button disabled={sending || Boolean(reason)} onClick={() => void interrupt()}>{sending ? 'Checking and sending…' : 'Interrupt current event'}</button></footer>
  </dialog>
}
