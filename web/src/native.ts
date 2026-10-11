import type { BootMode } from './model'

declare global {
  interface Window {
    webkit?: { messageHandlers?: { harborFolderPicker?: { postMessage: (request: { path: string; mode: BootMode }) => Promise<string | null> } } }
  }
}

export function chooseNativeFolder(path: string, mode: BootMode) {
  return window.webkit?.messageHandlers?.harborFolderPicker?.postMessage({ path, mode })
}
