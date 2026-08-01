import { lazy, StrictMode, Suspense } from 'react'
import { createRoot } from 'react-dom/client'
import './styles/index.css'
import { App } from './App.tsx'

// Dev/harness-only pages, never linked in the UI. Lazy so each lands in its
// own chunk and the main bundle stays flat: ?spike=flow is the SP0 React Flow
// spike (ADR-0039); ?spike=canvas mounts the SP3 CanvasView over a demo config
// (the SP4 e2e's entry until the real App view-switch lands in SP4).
const FlowSpike = lazy(() => import('./spike/FlowSpike.tsx'))
const CanvasHarness = lazy(() => import('./canvas/CanvasHarness.tsx'))
const spike = new URLSearchParams(window.location.search).get('spike')

const root = document.getElementById('root')
if (!root) throw new Error('root element missing')
createRoot(root).render(
  <StrictMode>
    {spike === 'flow' ? (
      <Suspense fallback={null}>
        <FlowSpike />
      </Suspense>
    ) : spike === 'canvas' ? (
      <Suspense fallback={null}>
        <CanvasHarness />
      </Suspense>
    ) : (
      <App />
    )}
  </StrictMode>,
)
