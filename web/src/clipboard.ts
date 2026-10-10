export async function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    try { await navigator.clipboard.writeText(text); return } catch { /* Try the user-initiated selection API below. */ }
  }
  const active = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  const selection = window.getSelection()
  const ranges = selection ? Array.from({ length: selection.rangeCount }, (_, i) => selection.getRangeAt(i).cloneRange()) : []
  const field = document.createElement('textarea')
  field.value = text
  field.readOnly = true
  field.tabIndex = -1
  field.setAttribute('aria-hidden', 'true')
  Object.assign(field.style, { position: 'fixed', top: '0', left: '0', width: '1px', height: '1px', opacity: '0', fontSize: '16px' })
  document.body.append(field)
  try {
    field.focus({ preventScroll: true })
    field.select()
    field.setSelectionRange(0, field.value.length)
    if (!document.execCommand('copy')) throw new Error('Copy was blocked')
  } finally {
    field.remove()
    active?.focus({ preventScroll: true })
    selection?.removeAllRanges()
    for (const range of ranges) if (range.commonAncestorContainer.isConnected) selection?.addRange(range)
  }
}
