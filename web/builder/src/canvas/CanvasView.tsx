import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useState,
  type Dispatch,
  type DragEvent,
} from 'react'
import {
  ReactFlow,
  Background,
  Handle,
  Position,
  type Connection,
  type Edge as RFEdge,
  type EdgeChange,
  type Node as RFNode,
  type NodeProps,
  type ReactFlowInstance,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import './canvas.css'
import type { Config } from '../config/schema'
import {
  CHANNEL_TYPES,
  CHANNEL_MODES_BY_TYPE,
  SENSITIVITIES,
  DISPATCHES,
  POLICY_KINDS,
  PROVIDERS,
  LOCALITIES,
} from '../config/schema'
import {
  clone,
  isDirty,
  configReducer,
  newModel,
  pendingChangeCount,
  type ConfigAction,
} from '../config/edit'
import { GovernancePanel, type DetectedSkill } from '../GovernancePanel'
import { graphFromConfig, canConnect, type GraphNode } from './graph'
import { postConfig, getConfig, getReloadStatus, HttpError } from '../api'
import { pollReload, type ReloadStatus, type PollDeps } from '../config/reload'
import { parseSaveError, type SaveError } from '../config/errors'
import { ReloadView, SaveErrorView } from '../ConfigEditor'

// The canvas view (builder-canvas SP3): React Flow over the SP2 projection
// (graphFromConfig), a palette, the properties panel, and the SAME save-bar /
// reload machine as the form editor (pollReload + ReloadView, reused not
// duplicated — FR-HOT-5). The graph is a derived VIEW: every mutation goes
// through configReducer on the working copy; positions are computed from the
// projection and never persisted (NC-6). Normative layout: final-6 mockup.

const realReloadDeps: PollDeps = {
  getStatus: getReloadStatus,
  sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
  now: () => Date.now(),
}

// Dispatch context so the custom nodes (rendered by React Flow) can reach the
// reducer without threading callbacks through node data.
const CanvasDispatch = createContext<Dispatch<ConfigAction>>(() => undefined)

const BLOCK_MIME = 'application/korvun-block'

// Rich node data (SP6, final-6): label plus the facts each kind shows as
// badges / a policy line. All optional so one node type covers the union.
type NodeData = {
  label: string
  sensitivity?: string
  policy?: string
  locality?: string
}
type CanvasFlowNode = RFNode<NodeData, 'channel' | 'brain' | 'model'>

// Spanish labels for the house badges (final-6).
const SENSITIVITY_ES: Record<string, string> = { private: 'privado', public: 'público' }
const LOCALITY_ES: Record<string, string> = { local: 'local', cloud: 'nube' }
const POLICY_ES: Record<string, string> = { priority: 'prioridad', consensus: 'consenso' }

// One tiny inline glyph per kind — a same-origin SVG, never an external asset.
function NodeIcon({ kind }: { kind: GraphNode['kind'] }) {
  const path =
    kind === 'channel'
      ? 'M2 4h12v8H2z M2 4l6 4 6-4' // envelope
      : kind === 'brain'
        ? 'M8 2a4 4 0 0 1 4 4c1 1 1 3 0 4a4 4 0 0 1-8 0c-1-1-1-3 0-4a4 4 0 0 1 4-4z' // node
        : 'M3 8h10 M8 3v10' // model cross
  return (
    <svg
      className="node-icon"
      data-testid="node-icon"
      width="14"
      height="14"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={path} />
    </svg>
  )
}

const kindOf = (id: string): GraphNode['kind'] => id.split(':')[0] as GraphNode['kind']
const indexOf = (id: string): number => Number(id.split(':')[1])

// canConnect operates on GraphNodes; only id+kind matter for the matrix.
const asGraphNode = (id: string): GraphNode => ({ id, kind: kindOf(id), column: 0, row: 0 })

const isValidConnection = (c: { source: string; target: string }): boolean =>
  canConnect(asGraphNode(c.source), asGraphNode(c.target))

function ChannelNode({ id, data }: NodeProps<CanvasFlowNode>) {
  return (
    <div className="canvas-node" data-testid={id} data-kind="channel">
      <span className="node-head">
        <NodeIcon kind="channel" />
        {data.label}
      </span>
      <Handle type="source" position={Position.Right} />
    </div>
  )
}

function BrainNode({ id, data }: NodeProps<CanvasFlowNode>) {
  const dispatch = useContext(CanvasDispatch)
  return (
    <div
      className="canvas-node"
      data-testid={id}
      data-kind="brain"
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e: DragEvent) => {
        if (e.dataTransfer.getData(BLOCK_MIME) !== 'model') return
        // A palette model lands ON a brain (NC-6: models exist only inside
        // brains[i].models — never free on the canvas).
        e.preventDefault()
        e.stopPropagation()
        dispatch({ kind: 'dropModel', brain: indexOf(id), model: newModel() })
      }}
    >
      <Handle type="target" position={Position.Left} />
      <span className="node-head">
        <NodeIcon kind="brain" />
        {data.label}
      </span>
      <span className="node-badges">
        {data.sensitivity && (
          <span className="badge" data-testid="badge-sensitivity">
            {SENSITIVITY_ES[data.sensitivity] ?? data.sensitivity}
          </span>
        )}
      </span>
      {data.policy && (
        <span className="node-policy" data-testid="node-policy">
          {POLICY_ES[data.policy] ?? data.policy}
        </span>
      )}
      <Handle type="source" position={Position.Right} />
    </div>
  )
}

function ModelNode({ id, data }: NodeProps<CanvasFlowNode>) {
  return (
    <div className="canvas-node" data-testid={id} data-kind="model">
      <Handle type="target" position={Position.Left} />
      <span className="node-head">
        <NodeIcon kind="model" />
        {data.label}
      </span>
      {data.locality && (
        <span className="badge" data-testid="badge-locality">
          {LOCALITY_ES[data.locality] ?? data.locality}
        </span>
      )}
    </div>
  )
}

const nodeTypes = { channel: ChannelNode, brain: BrainNode, model: ModelNode }

function dataFor(n: GraphNode, cfg: Config): NodeData {
  switch (n.kind) {
    case 'channel':
      return { label: cfg.channels[indexOf(n.id)]?.type ?? '' }
    case 'brain': {
      const b = cfg.brains[indexOf(n.id)]
      return { label: b?.name || '(unnamed)', sensitivity: b?.sensitivity, policy: b?.policy.kind }
    }
    case 'model': {
      const [bi, mi] = n.id.split(':')[1].split('.').map(Number)
      const mc = cfg.brains[bi]?.models[mi]
      return { label: mc ? mc.model_id || mc.provider : '', locality: mc?.locality }
    }
  }
}

function SelectField({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: string
  options: readonly string[]
  onChange: (v: string) => void
}) {
  return (
    <label className="field">
      <span className="lbl">{label}</span>
      <select className="txt" value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </select>
    </label>
  )
}

function Field({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  return (
    <label className="field">
      <span className="lbl">{label}</span>
      <input
        className="txt"
        value={value}
        placeholder={placeholder ?? ''}
        onChange={(e) => onChange(e.target.value)}
      />
    </label>
  )
}

function ChannelPanel({ c, index, dispatch }: { c: Config['channels'][number]; index: number; dispatch: Dispatch<ConfigAction> }) {
  const modes = CHANNEL_MODES_BY_TYPE[c.type as keyof typeof CHANNEL_MODES_BY_TYPE] ?? []
  const setType = (value: string) => {
    dispatch({ kind: 'setChannelField', channel: index, field: 'type', value })
    // The mode follows the new type's single valid transport (or none) —
    // mirrors config.Validate's type-aware mode rule.
    const next = CHANNEL_MODES_BY_TYPE[value as keyof typeof CHANNEL_MODES_BY_TYPE] ?? []
    dispatch({ kind: 'setChannelField', channel: index, field: 'mode', value: next[0] ?? '' })
  }
  return (
    <>
      <h2>channel · {c.type}</h2>
      <SelectField label="type" value={c.type} options={CHANNEL_TYPES} onChange={setType} />
      {modes.length > 0 && (
        <SelectField
          label="mode"
          value={c.mode}
          options={modes}
          onChange={(value) => dispatch({ kind: 'setChannelField', channel: index, field: 'mode', value })}
        />
      )}
      <Field
        label="token_env"
        value={c.token_env}
        onChange={(value) => dispatch({ kind: 'setChannelField', channel: index, field: 'token_env', value })}
      />
      {c.type === 'webhook' && (
        <>
          {/* The nested webhook block (ADR-0038 §1), editable since SP4 via
              setWebhookField — the first edit materializes a missing block. */}
          {(['bind', 'path', 'outbound_url', 'outbound_token_env'] as const).map((field) => (
            <Field
              key={field}
              label={field}
              value={c.webhook?.[field] ?? ''}
              onChange={(value) => dispatch({ kind: 'setWebhookField', channel: index, field, value })}
            />
          ))}
          {/* The mapping's six fields (SP6): which inbound JSON keys map to
              Envelope fields. Empty resolves to the canonical default server-side
              (EffectiveMapping), so the panel never inflates the file. */}
          <span className="lbl">mapping</span>
          {(
            ['sender_id', 'sender_name', 'text', 'media_url', 'media_type', 'conversation_id'] as const
          ).map((field) => (
            <Field
              key={field}
              label={field}
              value={c.webhook?.mapping?.[field] ?? ''}
              onChange={(value) =>
                dispatch({ kind: 'setWebhookMappingField', channel: index, field, value })
              }
            />
          ))}
        </>
      )}
    </>
  )
}

function BrainPanel({
  b,
  index,
  dispatch,
  channelNames,
  detectedSkills,
}: {
  b: Config['brains'][number]
  index: number
  dispatch: Dispatch<ConfigAction>
  channelNames: string[]
  detectedSkills: DetectedSkill[]
}) {
  const set = (field: 'name' | 'sensitivity' | 'dispatch') => (value: string) =>
    dispatch({ kind: 'setBrainField', brain: index, field, value })
  const persona = (field: 'display_name' | 'tone' | 'language' | 'instructions') => (value: string) =>
    dispatch({ kind: 'setPersonaField', brain: index, field, value })
  return (
    <>
      <h2>brain · {b.name || '(unnamed)'}</h2>
      <Field label="name" value={b.name} onChange={set('name')} />
      <SelectField label="sensitivity" value={b.sensitivity} options={SENSITIVITIES} onChange={set('sensitivity')} />
      <SelectField label="dispatch" value={b.dispatch} options={DISPATCHES} onChange={set('dispatch')} />
      <SelectField
        label="policy"
        value={b.policy.kind}
        options={POLICY_KINDS}
        onChange={(value) => dispatch({ kind: 'setPolicyKind', brain: index, value })}
      />
      <span className="lbl">persona</span>
      <Field label="display_name" value={b.persona?.display_name ?? ''} onChange={persona('display_name')} />
      <Field label="tone" value={b.persona?.tone ?? ''} onChange={persona('tone')} />
      <Field label="language" value={b.persona?.language ?? ''} onChange={persona('language')} />
      <Field label="instructions" value={b.persona?.instructions ?? ''} onChange={persona('instructions')} />
      {/* SP6: the governed-tools section, only for agent brains (ADR-0041). */}
      <GovernancePanel
        b={b}
        index={index}
        dispatch={dispatch}
        channelNames={channelNames}
        detectedSkills={detectedSkills}
      />
    </>
  )
}

// DeleteNodeControl is the "Eliminar nodo…" affordance (final-6), gated behind
// a confirmation because it cascades (SP5). It dispatches the kind-specific
// remove action and then clears the selection (the node it edited is gone).
function DeleteNodeControl({
  action,
  onDeleted,
  dispatch,
}: {
  action: ConfigAction
  onDeleted: () => void
  dispatch: Dispatch<ConfigAction>
}) {
  const [confirming, setConfirming] = useState(false)
  if (confirming) {
    return (
      <div className="delete-confirm" data-testid="delete-node-confirm">
        <span>¿Eliminar este nodo?</span>
        <button
          type="button"
          className="btn-danger-sm"
          onClick={() => {
            dispatch(action)
            onDeleted()
          }}
        >
          Sí, eliminar
        </button>
        <button type="button" className="btn-ghost-sm" onClick={() => setConfirming(false)}>
          Cancelar
        </button>
      </div>
    )
  }
  return (
    <button type="button" className="delete-node" onClick={() => setConfirming(true)}>
      Eliminar nodo…
    </button>
  )
}

function PropertiesPanel({
  selected,
  cfg,
  dispatch,
  onDeleted,
  onClose,
  detectedSkills,
}: {
  selected: string
  cfg: Config
  dispatch: Dispatch<ConfigAction>
  onDeleted: () => void
  onClose: () => void
  detectedSkills: DetectedSkill[]
}) {
  const kind = kindOf(selected)
  let body = null
  let deleteAction: ConfigAction | null = null
  if (kind === 'channel') {
    const i = indexOf(selected)
    const c = cfg.channels[i]
    body = c ? <ChannelPanel c={c} index={i} dispatch={dispatch} /> : null
    if (c) deleteAction = { kind: 'removeChannel', channel: i }
  } else if (kind === 'brain') {
    const i = indexOf(selected)
    const b = cfg.brains[i]
    body = b ? (
      <BrainPanel
        b={b}
        index={i}
        dispatch={dispatch}
        channelNames={cfg.channels.map((c) => c.type)}
        detectedSkills={detectedSkills}
      />
    ) : null
    if (b) deleteAction = { kind: 'removeBrain', brain: i }
  } else {
    const [b, m] = selected.split(':')[1].split('.').map(Number)
    const mc = cfg.brains[b]?.models[m]
    if (mc) deleteAction = { kind: 'removeModel', brain: b, model: m }
    // Editable since SP4, through the existing updateModel action.
    const up = (patch: Partial<NonNullable<typeof mc>>) =>
      dispatch({ kind: 'updateModel', brain: b, model: m, patch })
    body = mc ? (
      <>
        <h2>model · {mc.model_id || '(unset)'}</h2>
        <SelectField
          label="provider"
          value={mc.provider}
          options={PROVIDERS}
          onChange={(provider) => up({ provider })}
        />
        <Field label="model_id" value={mc.model_id} onChange={(model_id) => up({ model_id })} />
        <SelectField
          label="locality"
          value={mc.locality}
          options={LOCALITIES}
          onChange={(locality) => up({ locality })}
        />
        {/* Full endpoint prefix; the server appends /chat/completions and
            never guesses (ADR-0044). REQUIRED for openai-compatible, an
            optional adapter-default override for ollama/groq. */}
        <Field
          label="base_url"
          value={mc.base_url ?? ''}
          onChange={(base_url) => up({ base_url })}
          placeholder={
            mc.provider === 'openai-compatible' ? 'https://host/v1 (required)' : 'optional override'
          }
        />
        {/* The env-var NAME, never the value (ADR-0010) — the house microcopy. */}
        <Field
          label="api_key_env"
          value={mc.api_key_env ?? ''}
          onChange={(api_key_env) => up({ api_key_env })}
          placeholder="env var name"
        />
      </>
    ) : null
  }
  if (!body) return null
  return (
    <aside className="properties-panel" data-testid="properties-panel">
      {/* v0.9.1 (symptom 3): the panel had no close path at all. */}
      <button
        type="button"
        className="panel-close"
        aria-label="Cerrar panel"
        onClick={onClose}
      >
        ×
      </button>
      {body}
      {deleteAction && (
        <DeleteNodeControl action={deleteAction} onDeleted={onDeleted} dispatch={dispatch} />
      )}
    </aside>
  )
}

export function CanvasView({
  baseline,
  token,
  reloadDeps = realReloadDeps,
  onAuthError,
  detectedSkills = [],
}: {
  baseline: Config
  token: string
  reloadDeps?: PollDeps
  onAuthError?: () => void
  detectedSkills?: DetectedSkill[]
}) {
  const [base, setBase] = useState<Config>(baseline)
  const [wc, dispatch] = useReducer(configReducer, baseline, clone)
  const [selected, setSelected] = useState<string | null>(null)
  // v0.9.1 (symptom 2): a model dropped on the empty surface used to be a
  // silent no-op. NC-6 stands (models exist only inside brains) — the rule
  // just becomes visible through this hint instead of dead air.
  const [surfaceHint, setSurfaceHint] = useState(false)
  const [reload, setReload] = useState<ReloadStatus>({ phase: 'idle' })

  // v0.9.1 (symptom 3): Escape closes the properties panel. Bound only
  // while a node is selected; removed on close/unmount.
  useEffect(() => {
    if (selected === null) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setSelected(null)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [selected])
  const [saveError, setSaveError] = useState<SaveError | null>(null)

  const locked = reload.phase === 'polling'
  const dirty = isDirty(wc, base)
  const pending = pendingChangeCount(base, wc)

  // Dropped blocks must always land IN VIEW: React Flow fits only on mount,
  // so a node added outside the fitted viewport would be clipped (and
  // unreachable). Re-fit whenever the node count changes. (The jsdom test
  // seam never calls onInit — flow stays null there, a clean no-op.)
  const [flow, setFlow] = useState<ReactFlowInstance | null>(null)

  const graph = useMemo(() => graphFromConfig(wc), [wc])
  const nodes = useMemo<CanvasFlowNode[]>(
    () =>
      graph.nodes.map((n) => ({
        id: n.id,
        type: n.kind,
        // Derived layout only (NC-6): columns/rows from the projection.
        position: { x: n.column * 260, y: n.row * 110 },
        data: dataFor(n, wc),
      })),
    [graph, wc],
  )
  // B8 (v0.9.2): edge SELECTION lives here. The edges are controlled, and in
  // controlled mode React Flow delivers selection as edge changes
  // (type:'select') through onEdgesChange — without the handler the change
  // is dropped and no edge can ever be selected, which made onEdgesDelete
  // unreachable from the UI (the bug-bash's undeletable cable). Selection is
  // the ONLY change applied: structure keeps deriving from the working copy.
  const [selectedEdges, setSelectedEdges] = useState<ReadonlySet<string>>(new Set())
  const onEdgesChange = (changes: EdgeChange<RFEdge>[]) => {
    setSelectedEdges((prev) => {
      let next: Set<string> | null = null
      for (const ch of changes) {
        if (ch.type !== 'select') continue
        next ??= new Set(prev)
        if (ch.selected) next.add(ch.id)
        else next.delete(ch.id)
      }
      return next ?? prev
    })
  }

  const edges = useMemo<RFEdge[]>(
    () =>
      graph.edges.map((e) => ({
        id: e.id,
        source: e.source,
        target: e.target,
        selected: selectedEdges.has(e.id),
        ...(e.excluded ? { className: 'edge-excluded' } : {}),
      })),
    [graph, selectedEdges],
  )

  // fitView options: a comfortable padding and a maxZoom so a two-node graph
  // never balloons — nodes stay reachable, not lost in the void (SP6).
  const fitViewOptions = { padding: 0.2, maxZoom: 1 }
  useEffect(() => {
    flow?.fitView(fitViewOptions)
    // fitViewOptions is a stable literal; only re-fit when the node count moves.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [flow, nodes.length])

  const onConnect = (c: Connection) => {
    // The guard runs here too: React Flow's isValidConnection covers the
    // drag UI, this covers the dispatch — an invalid pair mutates NOTHING.
    if (!isValidConnection(c)) return
    dispatch({ kind: 'connectRoute', channel: indexOf(c.source), brain: indexOf(c.target) })
  }

  // React Flow's edge-delete hook (Delete/Backspace on a selected edge). Only
  // ROUTE edges are cable-deletable → disconnectRoute by its index; a
  // composition edge is ignored (a model leaves via its panel — schema mirror).
  const onEdgesDelete = (deleted: Array<{ id: string }>) => {
    for (const e of deleted) {
      if (e.id.startsWith('route:')) dispatch({ kind: 'disconnectRoute', route: indexOf(e.id) })
    }
    // A deleted edge leaves the selection too — stale ids must not re-mark
    // the reindexed routes the graph projects next render (B8).
    setSelectedEdges((prev) => {
      const next = new Set(prev)
      for (const e of deleted) next.delete(e.id)
      return next
    })
  }

  const onSurfaceDrop = (e: DragEvent) => {
    e.preventDefault()
    const block = e.dataTransfer.getData(BLOCK_MIME)
    // channel/brain blocks create fresh entries; a model on empty canvas
    // creates NOTHING — a model exists only inside a brain (NC-6, no
    // orphans) — but answers with the hint instead of silence (v0.9.1).
    if (block === 'channel') dispatch({ kind: 'addChannel' })
    else if (block === 'brain') dispatch({ kind: 'addBrain' })
    else if (block === 'model') {
      setSurfaceHint(true)
      return
    }
    setSurfaceHint(false)
  }

  async function save() {
    setSaveError(null)
    try {
      const r = await postConfig(token, wc)
      const final = await pollReload(r.handle, token, reloadDeps, setReload)
      if (final.phase === 'succeeded') {
        const applied = await getConfig(token)
        setBase(applied)
        dispatch({ kind: 'reset', config: applied })
      }
    } catch (e) {
      setReload({ phase: 'idle' })
      if (e instanceof HttpError) {
        const se = parseSaveError(e.status, e.body)
        if (se.kind === 'unauthorized') {
          onAuthError?.()
          return
        }
        setSaveError(se)
      } else {
        setSaveError({ kind: 'other', status: 0, message: 'save failed' })
      }
    }
  }

  return (
    <CanvasDispatch.Provider value={dispatch}>
      <div className="canvas-shell">
        <div className="korvun-canvas">
          <aside className="canvas-palette">
            {(
              [
                { block: 'channel', section: 'Canales' },
                { block: 'brain', section: 'Cerebros' },
                { block: 'model', section: 'Modelos' },
              ] as const
            ).map(({ block, section }) => (
              <div key={block} className="palette-section">
                <h2 className="palette-section-title">{section}</h2>
                <div
                  className="palette-block"
                  data-testid={`palette:${block}`}
                  draggable="true"
                  onDragStart={(e) => e.dataTransfer.setData(BLOCK_MIME, block)}
                >
                  <span className="block-icon" data-testid="block-icon">
                    <NodeIcon kind={block} />
                  </span>
                  {block}
                  <span className="drag-dots" data-testid="drag-dots" aria-hidden="true">
                    ⠿
                  </span>
                </div>
              </div>
            ))}
            <p className="palette-hint">
              Arrastra canales y cerebros al lienzo; un modelo se suelta sobre un cerebro.
            </p>
          </aside>
          <div
            className="canvas-surface"
            data-testid="canvas-surface"
            onDrop={onSurfaceDrop}
            onDragOver={(e) => e.preventDefault()}
          >
            <ReactFlow
              nodes={nodes}
              edges={edges}
              nodeTypes={nodeTypes}
              isValidConnection={isValidConnection}
              onConnect={onConnect}
              onEdgesChange={onEdgesChange}
              onEdgesDelete={onEdgesDelete}
              deleteKeyCode={['Backspace', 'Delete']}
              onNodeClick={(_, node) => setSelected(node.id)}
              onPaneClick={() => setSelected(null)}
              onInit={setFlow}
              colorMode={document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'}
              fitView
              fitViewOptions={fitViewOptions}
            >
              <Background />
            </ReactFlow>
            {surfaceHint && (
              <p className="surface-hint" role="status" data-testid="surface-hint">
                Un modelo va sobre un cerebro: suéltalo encima de un nodo brain.
              </p>
            )}
          </div>
          {selected && (
            <PropertiesPanel
              selected={selected}
              cfg={wc}
              dispatch={dispatch}
              onDeleted={() => setSelected(null)}
              onClose={() => setSelected(null)}
              detectedSkills={detectedSkills}
            />
          )}
        </div>

        <div className="save-bar">
          <button className="btn primary" type="button" disabled={!dirty || locked} onClick={save}>
            {locked ? 'Aplicando…' : 'Aplicar cambios'}
          </button>
          <button
            className="btn"
            type="button"
            disabled={pending === 0 || locked}
            onClick={() => dispatch({ kind: 'reset', config: base })}
          >
            Descartar
          </button>
          {pending > 0 && (
            <span className="muted" data-testid="pending-count">
              {pending} {pending === 1 ? 'cambio' : 'cambios'} sin aplicar
            </span>
          )}
          <span className="muted">{dirty ? 'unsaved changes' : 'no changes'}</span>
          <ReloadView status={reload} onRetry={save} />
          {saveError && <SaveErrorView error={saveError} />}
        </div>
      </div>
    </CanvasDispatch.Provider>
  )
}