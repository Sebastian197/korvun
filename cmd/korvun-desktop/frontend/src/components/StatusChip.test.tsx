// The sidebar chip's three states over a seeded shell store (review
// finding: 'Sin configurar', the port derivation, and the '—' fallback had
// no coverage anywhere).
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { pollShellOnce, resetShellForTests } from '../status/shell'
import { pollOnce } from '../status/store'
import { StatusChip } from './StatusChip'

interface FakeWindow {
  go?: { shell?: { Desktop?: unknown } }
}

function installStatus(status: {
  Running: boolean
  ConfigPath: string
  AdminAddr: string
  TokenEnv: string
}): void {
  ;(window as unknown as FakeWindow).go = {
    shell: { Desktop: { Status: () => Promise.resolve(status) } },
  }
}

async function seedCore(running: boolean): Promise<void> {
  await pollOnce((() =>
    Promise.resolve(
      running
        ? new Response('ok')
        : new Response(JSON.stringify({ error: 'core stopped' }), {
            status: 503,
          }),
    )) as typeof fetch)
}

beforeEach(() => {
  resetShellForTests()
  delete (window as unknown as FakeWindow).go
})

describe('StatusChip', () => {
  it('no config loaded → Sin configurar', async () => {
    installStatus({
      Running: false,
      ConfigPath: '',
      AdminAddr: '',
      TokenEnv: '',
    })
    await pollShellOnce()
    await seedCore(false)
    render(<StatusChip />)
    expect(screen.getByText('Sin configurar')).toBeInTheDocument()
  })

  it('running → En marcha with the effective port and the config name', async () => {
    installStatus({
      Running: true,
      ConfigPath: '/tmp/dir/korvun.json',
      AdminAddr: '127.0.0.1:52814',
      TokenEnv: 'KORVUN_ADMIN_TOKEN',
    })
    await pollShellOnce()
    await seedCore(true)
    render(<StatusChip />)
    expect(screen.getByText('En marcha')).toBeInTheDocument()
    expect(screen.getByText(':52814')).toBeInTheDocument()
    expect(screen.getByText('korvun.json')).toBeInTheDocument()
  })

  it('stopped with a config → Detenido, no port', async () => {
    installStatus({
      Running: false,
      ConfigPath: '/tmp/dir/korvun.json',
      AdminAddr: '',
      TokenEnv: 'KORVUN_ADMIN_TOKEN',
    })
    await pollShellOnce()
    await seedCore(false)
    render(<StatusChip />)
    expect(screen.getByText('Detenido')).toBeInTheDocument()
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})
