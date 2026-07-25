// Sidebar + chrome icons, EXTRACTED byte-faithful from the design-track
// standalone source (`design-drafts/Korvun Desktop (standalone).html`) — the
// 6a review rider: real design icons, never re-invented. Stroke rides
// currentColor so the nav color states tint them.
import type { JSX } from 'react'

interface IconProps {
  size?: number
}

function Svg({
  size = 16,
  strokeWidth = 1.75,
  children,
}: IconProps & {
  strokeWidth?: number
  children: React.ReactNode
}): JSX.Element {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      {children}
    </svg>
  )
}

/** Inicio — the design's house. */
export function IconHome(p: IconProps): JSX.Element {
  return (
    <Svg {...p}>
      <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
      <path d="M9 22V12h6v10" />
    </Svg>
  )
}

/** Builder — the design's branches (git-branch). */
export function IconBuilder(p: IconProps): JSX.Element {
  return (
    <Svg {...p}>
      <path d="M6 3v12" />
      <circle cx="18" cy="6" r="3" />
      <circle cx="6" cy="18" r="3" />
      <path d="M18 9a9 9 0 0 1-9 9" />
    </Svg>
  )
}

/** Canales — the design's send arrow. */
export function IconChannels(p: IconProps): JSX.Element {
  return (
    <Svg {...p}>
      <path d="M22 2L11 13" />
      <path d="M22 2l-7 20-4-9-9-4 20-7z" />
    </Svg>
  )
}

/** Actividad — the design's pulse. */
export function IconActivity(p: IconProps): JSX.Element {
  return (
    <Svg {...p}>
      <path d="M22 12h-4l-3 9L9 3l-3 9H2" />
    </Svg>
  )
}

/** Ajustes — the design's gear. */
export function IconSettings(p: IconProps): JSX.Element {
  return (
    <Svg {...p}>
      <circle cx="12" cy="12" r="3" />
      <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
    </Svg>
  )
}

/** Detener — the design's stop square. */
export function IconStop(p: IconProps): JSX.Element {
  return (
    <Svg {...p} strokeWidth={1.75}>
      <rect x="5" y="5" width="14" height="14" rx="2" />
    </Svg>
  )
}

/** Iniciar — the design's play triangle. */
export function IconPlay(p: IconProps): JSX.Element {
  return (
    <Svg {...p} strokeWidth={2}>
      <path d="M6 3l14 9-14 9V3z" />
    </Svg>
  )
}

/** Incidencia — the design's warning triangle. */
export function IconWarning(p: IconProps): JSX.Element {
  return (
    <Svg {...p}>
      <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
      <path d="M12 9v4" />
      <path d="M12 17h.01" />
    </Svg>
  )
}

/** Copiar — the design's copy glyph. */
export function IconCopy(p: IconProps): JSX.Element {
  return (
    <Svg {...p}>
      <rect x="9" y="9" width="13" height="13" rx="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </Svg>
  )
}
