import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './styles/index.css'
import { App } from './App.tsx'
import { startFeed } from './feed/store'
import { startSnapshot } from './snapshot/store'
import { startShellPolling } from './status/shell'
import { startPolling } from './status/store'
import { applyTheme, storedTheme } from './theme'

applyTheme(storedTheme())
startPolling()
startShellPolling()
startFeed()
startSnapshot()

const root = document.getElementById('root')
if (!root) throw new Error('root element missing')
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
