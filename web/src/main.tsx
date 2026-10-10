import { createRoot } from 'react-dom/client'
import { App } from './App'
import { APIClient } from './api/client'
createRoot(document.getElementById('root')!).render(<App client={new APIClient()}/>)
