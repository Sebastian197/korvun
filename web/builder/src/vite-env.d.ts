/// <reference types="vite/client" />

// Minimal ambient declaration so the SP3 token-scan test (CanvasView.test.tsx,
// which reads canvas.css as text via node:fs — vitest stubs every css IMPORT
// to an empty module) typechecks without @types/node (deliberately: zero new
// deps). Only the one signature used is declared; runtime is node.
declare module 'node:fs' {
  export function readFileSync(path: string | URL, encoding: 'utf-8'): string
  export function existsSync(path: string | URL): boolean
}