// Copyright 2026 Sebastián Moreno Saavedra
// The SP6 governance panel — the agent-brain "Herramientas y skills" section.
// Rendered inside the canvas BrainPanel (the shipped properties panel). The
// visual contract is design-drafts/governance-panel/; the config→UI→config
// transitions live in config/edit.ts, the derivations in config/governance.ts.
import type { Dispatch } from 'react'
import type { Config } from './config/schema'
import type { ConfigAction } from './config/edit'
import {
  TOOL_MODES,
  CAGE_TOOLS,
  effectiveToolAttr,
  shieldShown,
  sensitiveCloudWarning,
  grantMode,
  grantChannels,
  type ToolMode,
} from './config/governance'
import './governance-panel.css'

type D = Dispatch<ConfigAction>

/** A skill detected in the agent's skills_dir (name + description, read-only). */
export interface DetectedSkill {
  name: string
  description: string
}

/** SP6 governance panel — the agent-brain "Herramientas y skills" section. The
 *  visual contract is design-drafts/governance-panel/: tri-state grant per tool
 *  (violet=allow, amber-dashed=shadow, neutral=deny), attribute chips, channel
 *  chips, the derived non-editable network shield, the three cages with a simple
 *  allow-list editor, and a read-only detected-skills list. Governed by the
 *  existing save-bar (the hot shadow→allow promotion IS the usual Apply). */
export function GovernancePanel({
  b,
  index,
  dispatch,
  channelNames,
  detectedSkills,
}: {
  b: Config['brains'][number]
  index: number
  dispatch: D
  channelNames: string[]
  detectedSkills: DetectedSkill[]
}) {
  const agent = b.agent
  if (!agent) return null
  const warned = sensitiveCloudWarning(b)
  const toolChannels = [...channelNames, 'console']

  return (
    <div data-testid={`governance-section-${index}`}>
      <hr className="section-divider" />
      <div className="sec-title">Herramientas y skills</div>
      <p className="sec-sub">
        Qué puede ver y ejecutar el agente. La promoción se aplica en caliente con la cabecera.
      </p>

      {warned.length === 0 &&
        agent.tools.map((tool) => {
          const mode: ToolMode = grantMode(agent, tool) ?? 'allow'
          const scoped = grantChannels(agent, tool)
          const sensitive = effectiveToolAttr(agent.tool_attrs, tool, 'sensitive')
          const network = effectiveToolAttr(agent.tool_attrs, tool, 'network')
          const ungoverned = grantMode(agent, tool) === undefined
          return (
            <div className="tool" key={tool}>
              <div className="tool-top">
                <span className="tool-name">{tool}</span>
                <span className="tool-attrs">
                  {sensitive && <span className="attr on">sensible</span>}
                  {network && <span className="attr on">red</span>}
                </span>
              </div>
              <div className="tri" role="group" aria-label={`modo de ${tool}`}>
                {TOOL_MODES.map((m) => {
                  const active = !ungoverned && mode === m
                  const label = m === 'allow' ? 'Permitir' : m === 'shadow' ? 'Ensayo' : 'Denegar'
                  return (
                    <button
                      key={m}
                      type="button"
                      data-testid={`tri-${tool}-${m}`}
                      className={active ? `on-${m}` : ''}
                      aria-pressed={active}
                      onClick={() => dispatch({ kind: 'setToolMode', brain: index, tool, mode: m })}
                    >
                      <span className="sw" />
                      {label}
                    </button>
                  )
                })}
              </div>
              <div className="tool-meta">
                <span className="meta-lead">canales</span>
                {scoped.length === 0 ? (
                  <span className="chip ch">todos</span>
                ) : (
                  scoped.map((c) => (
                    <span className="chip ch" key={c}>
                      {c}
                    </span>
                  ))
                )}
                {shieldShown(b, tool) && (
                  <span className="derived" data-testid={`shield-${tool}`}>
                    <span className="lock">🔒</span>escudo de red · privado
                  </span>
                )}
                <select
                  className="chip-select"
                  aria-label={`canales de ${tool}`}
                  value=""
                  onChange={(e) => {
                    const ch = e.target.value
                    if (!ch) return
                    const next = scoped.includes(ch)
                      ? scoped.filter((x) => x !== ch)
                      : [...scoped, ch]
                    dispatch({ kind: 'setToolChannels', brain: index, tool, channels: next })
                  }}
                >
                  <option value="">± canal</option>
                  {toolChannels.map((c) => (
                    <option key={c} value={c}>
                      {scoped.includes(c) ? `− ${c}` : `+ ${c}`}
                    </option>
                  ))}
                </select>
              </div>
            </div>
          )
        })}

      {warned.length > 0 && (
        <div className="empty-note">
          Este cerebro no tiene reglas de gobierno. Sus herramientas listadas quedan en{' '}
          <b>Permitir</b> en todos los canales.
          <div className="house-warn" data-testid={`sensitive-cloud-warning-${index}`}>
            <span className="ic">▲</span>
            <span className="tx">
              <b>{warned.join(', ')} es una herramienta sensible sobre un modelo en la nube.</b> Sin
              gobierno, su salida puede viajar al proveedor en la nube. Añade un bloque de gobierno,
              o marca <code>sensible: no</code> a conciencia. El arranque lo rechaza hasta entonces.
            </span>
          </div>
        </div>
      )}

      {/* Cages */}
      {CAGE_TOOLS.filter((c) => agent.tools.includes(c)).length > 0 && (
        <span className="lbl govspace">Jaulas</span>
      )}
      {agent.tools.includes('read_file') && (
        <div className="cage">
          <div className="cage-h">
            <span className="t">read_file</span> · raíz y tamaño
          </div>
          <div className="field nomb">
            <span className="lbl">raíz</span>
            <input
              className="txt mono"
              data-testid="cage-read_file-root"
              value={agent.read_file?.root ?? ''}
              onChange={(e) =>
                dispatch({
                  kind: 'setCageField',
                  brain: index,
                  cage: 'read_file',
                  field: 'root',
                  value: e.target.value,
                })
              }
            />
          </div>
          <div className="field nomb">
            <span className="lbl">máx. tamaño</span>
            <input
              className="txt mono"
              data-testid="cage-read_file-max"
              value={agent.read_file?.max_bytes ?? ''}
              onChange={(e) =>
                dispatch({
                  kind: 'setCageField',
                  brain: index,
                  cage: 'read_file',
                  field: 'max_bytes',
                  value: Number(e.target.value) || 0,
                })
              }
            />
          </div>
        </div>
      )}
      {(['http_fetch', 'webhook_call'] as const)
        .filter((c) => agent.tools.includes(c))
        .map((cage) => (
          <div className="cage" key={cage}>
            <div className="cage-h">
              <span className="t">{cage}</span> · hosts permitidos
            </div>
            <div className="hostlist">
              {(agent[cage]?.allow_hosts ?? []).map((h, hi) => (
                <div className="host" key={hi}>
                  <input
                    className="txt mono"
                    data-testid={`cage-${cage}-host-${hi}`}
                    value={h}
                    onChange={(e) =>
                      dispatch({
                        kind: 'setCageHost',
                        brain: index,
                        cage,
                        index: hi,
                        value: e.target.value,
                      })
                    }
                  />
                  <button
                    className="x"
                    type="button"
                    aria-label={`quitar host ${hi} de ${cage}`}
                    onClick={() =>
                      dispatch({ kind: 'removeCageHost', brain: index, cage, index: hi })
                    }
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
            <button
              className="host-add"
              type="button"
              data-testid={`cage-${cage}-add`}
              onClick={() => dispatch({ kind: 'addCageHost', brain: index, cage })}
            >
              + añadir host
            </button>
          </div>
        ))}

      {/* Skills */}
      <span className="lbl govspace">Skills</span>
      <div className="row2">
        <div className="field">
          <span className="lbl">carpeta</span>
          <input
            className="txt mono"
            data-testid="skills-dir"
            value={agent.skills_dir ?? ''}
            onChange={(e) =>
              dispatch({
                kind: 'setSkillsField',
                brain: index,
                field: 'skills_dir',
                value: e.target.value,
              })
            }
          />
        </div>
        <div className="field">
          <span className="lbl">presup. cuerpos</span>
          <input
            className="txt mono"
            data-testid="skills-budget"
            value={agent.skills_body_budget ?? ''}
            onChange={(e) =>
              dispatch({
                kind: 'setSkillsField',
                brain: index,
                field: 'skills_body_budget',
                value: Number(e.target.value) || 0,
              })
            }
          />
        </div>
      </div>
      {detectedSkills.length > 0 && (
        <div className="cage skills-box" data-testid={`skills-list-${index}`}>
          {detectedSkills.map((s) => (
            <div className="skill" key={s.name}>
              <span className="s-name">{s.name}</span>
              <span className="s-desc">{s.description}</span>
              <span className="ro-flag">solo lectura</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
