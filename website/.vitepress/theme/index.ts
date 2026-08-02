// Custom theme = default theme + the Korvun identity (ADR-0030) as CSS
// overrides. No components yet — SP2 is tokens, fonts, and motion only.
import DefaultTheme from 'vitepress/theme'
import './custom.css'

export default {
  extends: DefaultTheme,
}