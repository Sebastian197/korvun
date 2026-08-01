import { CanvasView } from './CanvasView'
import type { Config } from '../config/schema'

// SP3 dev/e2e harness page (SP0's spike-gate precedent): mounts CanvasView
// over a static demo config at /builder/?spike=canvas — never linked in the
// production UI. It exists so (1) the canvas ships as its own lazy chunk and
// (2) the SP4 e2e can exercise the real canvas against the real binary before
// the App view-switch lands (that switch is SP4's, not SP3's). No token: the
// demo is interaction-only; Aplicar would 401 against a real core, honestly.

const demoBaseline: Config = {
  channels: [
    { type: 'telegram', mode: 'polling', token_env: 'TELEGRAM_TOKEN' },
    { type: 'discord', mode: 'gateway', token_env: 'DISCORD_BOT_TOKEN' },
    {
      type: 'webhook',
      mode: '',
      token_env: 'KORVUN_HOOK',
      webhook: { outbound_url: 'https://downstream.example/reply' },
    },
  ],
  brains: [
    {
      name: 'asistente',
      sensitivity: 'private',
      policy: { kind: 'priority', order: ['ollama', 'groq'] },
      dispatch: 'sequential',
      models: [
        { provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' },
        { provider: 'groq', model_id: 'llama-3.3-70b-versatile', locality: 'cloud', api_key_env: 'GROQ_API_KEY' },
      ],
    },
    {
      name: 'general',
      sensitivity: 'public',
      policy: { kind: 'consensus' },
      dispatch: 'fanout',
      models: [{ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }],
    },
  ],
  routes: [
    { channel: 'telegram', brain: 'asistente' },
    { channel: 'discord', brain: 'general' },
  ],
}

export default function CanvasHarness() {
  return <CanvasView baseline={demoBaseline} token="" />
}