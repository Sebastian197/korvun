// The shell-status store mirrors Desktop.Status() (bindings surface): the
// sidebar chip and the Settings info rows paint it. Outside the window
// (bindings absent) it degrades to null — the chrome still renders.
import { beforeEach, describe, expect, it } from 'vitest'
import { getShellStatus, pollShellOnce, resetShellForTests } from './shell'

interface FakeWindow {
  go?: { shell?: { Desktop?: unknown } }
}

beforeEach(() => {
  resetShellForTests()
  delete (window as unknown as FakeWindow).go
})

describe('shell status store', () => {
  it('without bindings the status is null (plain-browser degradation)', async () => {
    await pollShellOnce()
    expect(getShellStatus()).toBeNull()
  })

  it('with bindings the status mirrors Desktop.Status()', async () => {
    ;(window as unknown as FakeWindow).go = {
      shell: {
        Desktop: {
          Status: () =>
            Promise.resolve({
              Running: true,
              ConfigPath: '/tmp/dir/korvun.json',
              AdminAddr: '127.0.0.1:52814',
              TokenEnv: 'KORVUN_ADMIN_TOKEN',
            }),
        },
      },
    }
    await pollShellOnce()
    expect(getShellStatus()).toEqual({
      Running: true,
      ConfigPath: '/tmp/dir/korvun.json',
      AdminAddr: '127.0.0.1:52814',
      TokenEnv: 'KORVUN_ADMIN_TOKEN',
    })
  })

  it('a rejected Status() keeps the last good value (poll noise never blanks the chip)', async () => {
    const win = window as unknown as FakeWindow
    win.go = {
      shell: {
        Desktop: {
          Status: () =>
            Promise.resolve({ Running: false, ConfigPath: '/a.json', AdminAddr: '', TokenEnv: '' }),
        },
      },
    }
    await pollShellOnce()
    win.go = {
      shell: { Desktop: { Status: () => Promise.reject(new Error('binding timeout')) } },
    }
    await pollShellOnce()
    expect(getShellStatus()?.ConfigPath).toBe('/a.json')
  })
})