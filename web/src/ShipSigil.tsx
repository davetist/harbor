import { useMemo } from 'react'
import sigil from '@urbit/sigil-js/core'
import { isValidPatp } from 'urbit-ob'
import type { Ship } from './model'

export function ShipSigil({ ship, size = 36 }: { ship: Ship; size?: number }) {
  const image = useMemo(() => {
    const name = (ship.identity || ship.name).trim()
    const point = name.startsWith('~') ? name : `~${name}`
    if (!/^~(?:[a-z]{3}|[a-z]{6}|[a-z]{6}-[a-z]{6})$/.test(point) || !isValidPatp(point)) return undefined
    try {
      // The stylesheet already sets display; upstream's inline style violates CSP.
      return { point, svg: sigil({ point, size, foreground: '#eeeeee', background: '#000000', space: 'default', detail: size < 48 ? 'none' : 'default' }).replace('style="display: block;"', '') }
    } catch { return undefined }
  }, [ship.identity, ship.name, size])
  return image
    ? <span className="ship-sigil" role="img" aria-label={`Sigil for ${image.point}`} title={image.point} dangerouslySetInnerHTML={{ __html: image.svg }}/>
    : <span className="ship-sigil sigil-placeholder" role="img" aria-label="Sigil unavailable" title="Sigil unavailable for this identity"><span aria-hidden="true">~</span></span>
}
