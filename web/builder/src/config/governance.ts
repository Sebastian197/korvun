// Governance derivations for the SP6 panel (ADR-0041). Pure functions the
// component and the tests share, so the shield and the safe-default warning can
// never drift from the config: they are recomputed each render from the same
// fields the server reads. None of this is stored — the shield mirrors
// ToolDecision.Shield (caged.go), the warning mirrors ErrSensitiveToolUngoverned
// (the boot guard, estreno E-11).

import type { AgentConfig, BrainConfig, ToolAttrsConfig } from './schema'

/** Tri-state grant modes, mirroring policy.ToolMode. */
export const TOOL_MODES = ['allow', 'shadow', 'deny'] as const
export type ToolMode = (typeof TOOL_MODES)[number]

/** The built-in tool catalog, mirroring tool.BuiltinAttrs (caged.go). A UX
 *  convenience for the panel; the server re-validates every POST. */
export const CATALOG_TOOLS = [
  'time',
  'echo',
  'calc',
  'read_file',
  'http_fetch',
  'webhook_call',
] as const

/** The house-default attributes, an exact espejo of tool.BuiltinAttrs:
 *  read_file is sensitive; http_fetch/webhook_call reach the network; the rest
 *  carry nothing. */
const HOUSE_ATTRS: Record<string, { sensitive: boolean; network: boolean }> = {
  time: { sensitive: false, network: false },
  echo: { sensitive: false, network: false },
  calc: { sensitive: false, network: false },
  read_file: { sensitive: true, network: false },
  http_fetch: { sensitive: false, network: true },
  webhook_call: { sensitive: false, network: true },
}

/** The network tools that carry a cage (ADR-0041 §4). */
export const CAGE_TOOLS = ['read_file', 'http_fetch', 'webhook_call'] as const

/** The effective attribute of a tool: the declared override wins over the house
 *  default (ADR-0015 declared-not-inferred). */
export function effectiveToolAttr(
  overrides: Record<string, ToolAttrsConfig> | undefined,
  tool: string,
  attr: 'sensitive' | 'network',
): boolean {
  const override = overrides?.[tool]?.[attr]
  if (override !== undefined) return override
  return HOUSE_ATTRS[tool]?.[attr] ?? false
}

/** The network shield is shown, non-editable, when the brain is private AND the
 *  tool's effective network attribute is true (FR-DERIVE-1, mirrors
 *  ToolDecision.Shield). */
export function shieldShown(brain: BrainConfig, tool: string): boolean {
  if (brain.sensitivity !== 'private') return false
  return effectiveToolAttr(brain.agent?.tool_attrs, tool, 'network')
}

/** The safe-default warning fires for an UNGOVERNED agent brain on a CLOUD model
 *  that lists a tool whose effective sensitive attribute is true (FR-WARN-1,
 *  mirrors ErrSensitiveToolUngoverned). Returns the offending tool names. */
export function sensitiveCloudWarning(brain: BrainConfig): string[] {
  const agent = brain.agent
  if (agent === undefined) return []
  if (agent.governance !== undefined && agent.governance.length > 0) return []
  const model = brain.models[0]
  if (model === undefined || model.locality !== 'cloud') return []
  return agent.tools.filter((t) => effectiveToolAttr(agent.tool_attrs, t, 'sensitive'))
}

/** The current grant mode of a tool, or undefined when ungoverned (advertise +
 *  execute default). */
export function grantMode(agent: AgentConfig | undefined, tool: string): ToolMode | undefined {
  const g = agent?.governance?.find((x) => x.tool === tool)
  return g ? (g.mode as ToolMode) : undefined
}

/** The channels a grant is scoped to (empty = all channels). */
export function grantChannels(agent: AgentConfig | undefined, tool: string): string[] {
  return agent?.governance?.find((x) => x.tool === tool)?.channels ?? []
}
