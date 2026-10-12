// xterm creates its own scoped style elements. Give only those elements the
// random nonce in the initial HTML policy; do not patch the global document.
export function terminalDocument(): Document {
  const nonce = document.querySelector<HTMLMetaElement>('meta[name="harbor-style-nonce"]')?.content
  if (!nonce) return document
  return new Proxy(document, {
    get(target, property) {
      if (property === 'createElement') return (tag: string, options?: ElementCreationOptions) => {
        const element = target.createElement(tag, options)
        if (tag.toLowerCase() === 'style') element.setAttribute('nonce', nonce)
        if (tag.toLowerCase() === 'div') {
          // xterm 6's viewport bypasses documentOverride when creating styles.
          // Add the nonce before they enter this terminal's screen instead.
          const appendChild = element.appendChild.bind(element)
          element.appendChild = <T extends Node>(child: T): T => {
            if (element.classList.contains('xterm-screen') && child instanceof HTMLStyleElement) child.nonce = nonce
            return appendChild(child)
          }
        }
        return element
      }
      const value = Reflect.get(target, property, target)
      return typeof value === 'function' ? value.bind(target) : value
    },
  })
}
