import { useCallback, useState } from 'react'
import {
  ReactFlow,
  Background,
  Handle,
  Position,
  addEdge,
  useNodesState,
  useEdgesState,
  type ColorMode,
  type Connection,
  type Edge,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import './flow-spike.css'

// SP0 canvas spike (builder-canvas, ADR-0039). NOT linked from the production
// UI — reachable only via /builder/?spike=flow (main.tsx gate) for dev and the
// e2e harnesses. Proves @xyflow/react renders, drags and connects under the
// real binary's CSP with the house tokens.

type SpikeNode = Node<{ label: string }, 'korvun'>

const initialNodes: SpikeNode[] = [
  { id: 'a', type: 'korvun', position: { x: 60, y: 60 }, data: { label: 'channel · telegram' } },
  { id: 'b', type: 'korvun', position: { x: 340, y: 220 }, data: { label: 'brain · support' } },
]

const initialEdges: Edge[] = [{ id: 'a-b', source: 'a', target: 'b' }]

// Spike validation rule: self-connections are rejected, the rest is allowed.
const isValidConnection = (edge: Edge | Connection) => edge.source !== edge.target

function KorvunNode({ data }: NodeProps<SpikeNode>) {
  return (
    <div className="korvun-node">
      <Handle type="target" position={Position.Left} />
      {data.label}
      <Handle type="source" position={Position.Right} />
    </div>
  )
}

const nodeTypes = { korvun: KorvunNode }

export default function FlowSpike() {
  const [nodes, , onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)
  const [mode, setMode] = useState<ColorMode>(() =>
    document.documentElement.dataset.theme === 'light' ? 'light' : 'dark',
  )

  const onConnect = useCallback(
    (params: Connection) => setEdges((eds) => addEdge(params, eds)),
    [setEdges],
  )

  const toggleTheme = () => {
    const next = mode === 'dark' ? 'light' : 'dark'
    document.documentElement.dataset.theme = next
    setMode(next)
  }

  return (
    <div className="flow-spike" data-testid="flow-spike">
      <header>
        <h1>canvas spike — SP0</h1>
        <button type="button" onClick={toggleTheme} data-testid="theme-toggle">
          theme: {mode}
        </button>
      </header>
      <div className="canvas">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          isValidConnection={isValidConnection}
          colorMode={mode}
          fitView
        >
          <Background />
        </ReactFlow>
      </div>
    </div>
  )
}