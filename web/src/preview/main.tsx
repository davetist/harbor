import { createRoot } from 'react-dom/client'
import { App } from '../App'
import { PreviewClient } from './fixtures'
createRoot(document.getElementById('root')!).render(<App client={new PreviewClient()} preview/>)
