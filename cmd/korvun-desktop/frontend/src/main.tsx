import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './styles/index.css'
import { App } from './App.tsx'
import { startPolling } from './status/store'
import { applyTheme, storedTheme } from './theme'

applyTheme(storedTheme())
startPolling()

const root = document.getElementById('root')
if (!root) throw new Error('root element missing')
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
