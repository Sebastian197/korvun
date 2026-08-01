import { lazy, StrictMode, Suspense } from 'react'
import { createRoot } from 'react-dom/client'
import './styles/index.css'
import { App } from './App.tsx'

// SP0 canvas spike (ADR-0039): dev/harness-only page, never linked in the UI.
// Lazy so the canvas lands in its own chunk and the main bundle stays flat.
const FlowSpike = lazy(() => import('./spike/FlowSpike.tsx'))
const spike = new URLSearchParams(window.location.search).get('spike') === 'flow'

const root = document.getElementById('root')
if (!root) throw new Error('root element missing')
createRoot(root).render(
  <StrictMode>
    {spike ? (
      <Suspense fallback={null}>
        <FlowSpike />
      </Suspense>
    ) : (
      <App />
    )}
  </StrictMode>,
)
