import { useEffect, useState } from 'react'
import type { BackupRecord, HarborClient } from '../model'
export function Export({ shipId, client }: { shipId: string; client: HarborClient }) {
  const [exports, setExports] = useState<BackupRecord[]>([])
  const [error, setError] = useState('')
  useEffect(() => { let alive = true; const load = () => client.backups(shipId).then(rows => { if (alive) { setExports(rows); setError('') } }).catch(() => { if (alive) setError('Unable to read pier exports.') }); void load(); const timer = setInterval(load, 2000); return () => { alive = false; clearInterval(timer) } }, [shipId, client])
  return <div className="backup-list">{error && <p role="alert">{error}</p>}{exports.length > 0 && <p className="field-note">For migration, use the export made after the source’s final shutdown. Older exports are not live-network restore points. If the source has restarted, create a fresh export.</p>}{exports.map(archive => <a key={archive.id} href={`/api/v1/backups/${archive.id}/download`}>Download pier export · {new Date(archive.createdAt).toLocaleString()} →</a>)}</div>
}
