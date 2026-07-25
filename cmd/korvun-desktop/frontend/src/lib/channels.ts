// The ONE channel catalog for the whole chrome (Home panel, Activity rows,
// Canales list/detail, the wizard) — a new channel type is added HERE, and
// only here. Consolidates what used to be three drifting sources: the glyph
// table, the wizard's TYPES, and Canales' TOKEN_DEFAULT (review finding).

export interface ChannelType {
  id: string
  label: string
  mode: string
  tokenEnv: string
  blurb: string
}

/** The connectable channel types, in wizard order (mirrors config.Validate:
 * one channel per type; the mode is type-determined). */
export const CHANNEL_TYPES: readonly ChannelType[] = [
  {
    id: 'telegram',
    label: 'Telegram',
    mode: 'polling',
    tokenEnv: 'TELEGRAM_TOKEN',
    blurb: 'bot con @BotFather · polling',
  },
  {
    id: 'discord',
    label: 'Discord',
    mode: 'gateway',
    tokenEnv: 'DISCORD_BOT_TOKEN',
    blurb: 'bot del Portal de desarrolladores · gateway',
  },
]

const GLYPH: Record<string, string> = { telegram: 'TG', discord: 'DC' }

/** Two-letter tile glyph for a channel type (design's TG/DC tiles). */
export function channelGlyph(type: string | undefined): string {
  if (type === undefined || type === '') return '??'
  return GLYPH[type] ?? type.slice(0, 2).toUpperCase()
}

/** Human label for a channel type (falls back to a Capitalized id). */
export function channelLabel(type: string): string {
  return (
    CHANNEL_TYPES.find((t) => t.id === type)?.label ?? type.charAt(0).toUpperCase() + type.slice(1)
  )
}

/** The suggested env-var NAME for a type, or '' if unknown. */
export function defaultTokenEnv(type: string): string {
  return CHANNEL_TYPES.find((t) => t.id === type)?.tokenEnv ?? ''
}
