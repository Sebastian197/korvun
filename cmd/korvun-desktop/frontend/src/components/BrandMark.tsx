// The canonical "K terminal" mark — COPIED from assets/brand/
// korvun-logo-hero.svg (the versioned source of truth; ADR-0030 reserves the
// teal→violet gradient for identity moments, and the sidebar tile is exactly
// that). Geometry and gradient stops are byte-faithful to the source; only
// the def ids are namespaced (kv-*) so the inline copy can never collide
// with another document id. This file is palette-scan-allowlisted for the
// same reason the token table is: it IS the brand source, colors live here
// by definition.
import type { JSX } from 'react'

export function BrandMark({ size = 34 }: { size?: number }): JSX.Element {
  return (
    <svg viewBox="0 0 64 64" style={{ width: size, height: size }} role="img" aria-label="Korvun">
      <defs>
        <linearGradient
          id="kv-brand-g"
          gradientUnits="userSpaceOnUse"
          x1="6"
          y1="6"
          x2="58"
          y2="58"
        >
          <stop offset="0" stopColor="#2BC8B7" />
          <stop offset="1" stopColor="#7A5AF5" />
        </linearGradient>
        <mask id="kv-brand-k">
          <rect width="64" height="64" fill="#fff" />
          <path
            d="M23 17 L23 47"
            stroke="#000"
            strokeWidth="6.5"
            strokeLinecap="round"
            fill="none"
          />
          <path
            d="M45 18 L33 32 L45 46"
            stroke="#000"
            strokeWidth="6.5"
            strokeLinecap="round"
            strokeLinejoin="round"
            fill="none"
          />
        </mask>
      </defs>
      <rect
        x="2"
        y="2"
        width="60"
        height="60"
        rx="14"
        fill="url(#kv-brand-g)"
        mask="url(#kv-brand-k)"
      />
    </svg>
  )
}
