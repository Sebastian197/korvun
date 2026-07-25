// The cleartext-risk heuristic (ADR-0028 F10) with the F3 desktop-context
// recognition: inside Korvun Desktop the Wails WebView origin is the
// framework's custom scheme / wails.localhost host (verified at the wails/v2
// source), all in-process, so no warning THERE — while a real browser on a
// non-loopback host still warns.
import { describe, expect, it } from 'vitest'
import { isCleartextRisk } from './cleartext'

describe('isCleartextRisk', () => {
  it('does NOT warn on the Wails desktop origins (macOS/Linux wails://, Windows wails.localhost)', () => {
    expect(isCleartextRisk('wails:', 'wails')).toBe(false) // darwin/linux: wails://wails/
    expect(isCleartextRisk('http:', 'wails.localhost')).toBe(false) // windows
    expect(isCleartextRisk('wails:', 'wails.localhost')).toBe(false)
  })

  it('does NOT warn on https or loopback (unchanged)', () => {
    expect(isCleartextRisk('https:', 'example.com')).toBe(false)
    expect(isCleartextRisk('http:', 'localhost')).toBe(false)
    expect(isCleartextRisk('http:', '127.0.0.1')).toBe(false)
    expect(isCleartextRisk('http:', '[::1]')).toBe(false)
  })

  it('STILL warns on plain http to a real, non-loopback host (a browser over the network)', () => {
    expect(isCleartextRisk('http:', 'korvun.example.com')).toBe(true)
    expect(isCleartextRisk('http:', '192.168.1.50')).toBe(true)
  })

  it('warns on a bare http host literally named "wails" — only the wails: scheme is trusted', () => {
    // Defense-in-depth: a LAN host (or spoofed DNS) named `wails` must NOT
    // silence the warning without the framework's own `wails:` scheme. The
    // genuine macOS/Linux desktop origin is always `wails://wails/`.
    expect(isCleartextRisk('http:', 'wails')).toBe(true)
  })
})
