import { defineConfig } from 'vite'
export default defineConfig({ build: { outDir: 'dist' }, server: { port: 5173, strictPort: true, proxy: { '/api': 'http://127.0.0.1:18789' } } })
