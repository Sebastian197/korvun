import '@fontsource/geist-sans/400.css'
import '@fontsource/geist-sans/500.css'
import '@fontsource/geist-sans/600.css'
import '@fontsource/geist-sans/700.css'
import '@fontsource/geist-mono/400.css'
import '@fontsource/geist-mono/500.css'
import { useEffect, type ReactNode } from 'react'
import { armBrandMotion } from './brandMotion'
import { armStorytelling } from './storytelling'

export default function Root({ children }: { children: ReactNode }) {
  useEffect(() => {
    const cleanups = [armStorytelling(), armBrandMotion()]
    return () => cleanups.forEach((cleanup) => cleanup())
  }, [children])

  return children
}
