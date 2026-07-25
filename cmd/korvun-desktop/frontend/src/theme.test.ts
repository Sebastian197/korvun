// Theme behavior (FR-WIN-5 + the 6a review rider): 'system' must FOLLOW the
// OS — a live prefers-color-scheme flip repaints without any user action.
// jsdom has no matchMedia; a controllable fake stands in for the OS.
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { applyTheme } from './theme'

type Listener = (ev: { matches: boolean }) => void

class FakeMediaQueryList {
  matches: boolean
  listeners = new Set<Listener>()
  constructor(matches: boolean) {
    this.matches = matches
  }
  addEventListener(_type: 'change', l: Listener): void {
    this.listeners.add(l)
  }
  removeEventListener(_type: 'change', l: Listener): void {
    this.listeners.delete(l)
  }
  flip(matches: boolean): void {
    this.matches = matches
    for (const l of this.listeners) l({ matches })
  }
}

let mql: FakeMediaQueryList

beforeEach(() => {
  mql = new FakeMediaQueryList(false) // OS prefers dark
  ;(window as unknown as { matchMedia: unknown }).matchMedia = () => mql
  localStorage.clear()
})

afterEach(() => {
  applyTheme('dark') // detach any system listener between tests
})

describe('system theme', () => {
  it("'system' resolves against the current OS preference", () => {
    applyTheme('system')
    expect(document.documentElement.dataset.theme).toBe('dark')
  })

  it("a live OS flip repaints while the choice is 'system'", () => {
    applyTheme('system')
    mql.flip(true) // OS switches to light
    expect(document.documentElement.dataset.theme).toBe('light')
  })

  it('an explicit choice detaches the OS listener', () => {
    applyTheme('system')
    applyTheme('dark')
    mql.flip(true)
    expect(document.documentElement.dataset.theme).toBe('dark')
  })
})