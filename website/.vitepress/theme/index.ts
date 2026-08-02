// Custom theme = default theme + the Korvun identity (ADR-0030) as CSS
// overrides + the SP2b scroll-storytelling arming (progressive enhancement
// only — see reveal.ts for the never-hide law).
import type { EnhanceAppContext } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import './custom.css'
import { armStorytelling } from './reveal'

export default {
  extends: DefaultTheme,
  enhanceApp({ router }: EnhanceAppContext) {
    if (typeof window === 'undefined') return
    const arm = () => {
      // enhanceApp runs before the app mounts — retry by frame until the
      // layout exists, then arm (armStorytelling itself no-ops off-home).
      let tries = 0
      const attempt = () => {
        if (document.querySelector('.Layout')) armStorytelling()
        else if (++tries < 180) requestAnimationFrame(attempt)
      }
      attempt()
    }
    router.onAfterRouteChange = arm
    arm()
  },
}