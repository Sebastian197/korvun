# ADR-0039: React Flow (`@xyflow/react`) as the builder canvas library

> **Status:** accepted (2026-08-01)
> **Date:** 2026-08-01
> **Deciders:** Sebastián Moreno Saavedra
> **Companion:** BUILDER-CANVAS spec (`docs/superpowers/specs/2026-08-01-builder-canvas-and-brain-persona-design.md`), SP0 spike evidence in `design-drafts/claude-code-report.md`.

## Context

The approved builder-canvas spec turns `web/builder` into a visual node editor
(channels/brains/routes as a graph). That needs an interactive node-canvas
library — pan, drag, handles, edge creation with validation — which is exactly
the kind of dependency the house gates behind an ADR plus Context7 verification
plus the four-axis test (CLAUDE.md). SP0 is the spike that produces the
evidence for this ADR; its stop-on-fail rule says a failed spike does NOT pick
an alternative library — that decision belongs to the copilot.

Hard constraints the candidate must survive, not on faith but against the real
binary:

- **CSP:** the embed serves `default-src 'self'; object-src 'none';
  base-uri 'self'; frame-ancestors 'self'` (`web/builder/embed.go`) — no CDN,
  no `style-src 'unsafe-inline'`. Everything must bundle same-origin via Vite.
- **React 19.2.7** (exact-pinned, ADR-0029 pattern) — the library must support it.
- **Desktop iframe (NC-5):** the canvas must stay interactive (drag,
  pointer-capture, focus) inside the SP6 desktop `<iframe src="/builder/">`.
- **House tokens:** light + dark must ride the ADR-0030 `[data-theme]` variables.

Everything below was verified 2026-08-01 against Context7
(`/websites/reactflow_dev`) and primary sources (the npm registry for the exact
version/license/peer-deps; the installed package's `.d.ts` and bundle for exact
type signatures and DOM class names) — not from memory.

## Decision

Adopt **`@xyflow/react`**, pinned exact at **`12.11.2`** (npm `latest`,
MIT license), as a production dependency of `web/builder` only. No Go-side
change: `go.mod`/`go.sum` untouched (verified by diff).

### Identity and compatibility (verified)

- **Package identity:** the React Flow project renamed its npm package over its
  history; since v12 the one current package is `@xyflow/react` (the migration
  guide is explicit: former `reactflow` users install `@xyflow/react` and update
  imports + the CSS path). The old names are not adopted.
- **React 19.2:** peer deps are `react >=17` / `react-dom >=17` (npm registry),
  upstream announced React 19 support, and the spike ran green on the repo's
  pinned React 19.2.7.
- **Stylesheet:** a real file in the package — `@xyflow/react/dist/style.css` —
  imported from the spike component and bundled by Vite into a same-origin CSS
  asset. No CDN, no runtime `<style>` injection was observed (see CSP verdict).

### API surface adopted (spike scope)

The minimal surface the spike exercises, all named exports of `@xyflow/react`:

- `ReactFlow` (controlled `nodes`/`edges`), `Background`
- `useNodesState` / `useEdgesState`, `addEdge` inside `onConnect`
- `isValidConnection: (edge: Edge | Connection) => boolean` for connection
  validation (signature read from the installed `.d.ts`)
- `Handle` + `Position` inside a custom node registered via `nodeTypes`
- `colorMode` (`'dark' | 'light'`) for the built-in theme class
- Theming via the library's `--xy-*` CSS custom properties, mapped once to the
  house tokens (`--base`, `--surface`, `--border`, `--accent`, `--text-*`) in a
  spike-scoped stylesheet so `[data-theme]` drives both modes.

### CSP verdict: PASS against the real binary

Served by the `make build` binary (real `go:embed` + real header), the spike
page (`/builder/?spike=flow`) produced **zero** `securitypolicyviolation`
events, **zero** external network requests, and **zero** console errors, in
both themes, while dragging and connecting. Two structural reasons this holds:

1. The library ships its styles as an importable CSS file (bundled same-origin);
   it does not inject `<style>` tags at runtime.
2. Its runtime positioning uses React inline styles, which React writes through
   the CSSOM (`element.style`) — not `style` *attributes* — so the missing
   `'unsafe-inline'` does not bite (the existing builder already relies on this,
   e.g. the legend dots in `App.tsx`).

### Four-axis dependency test (capability vs hand-roll cost vs maintenance vs risk/volatility)

| Axis | Verdict |
|------|---------|
| **Capability gain** | Pan/zoom viewport math, node drag with pointer capture, handle-based edge creation with validation hooks, keyboard/a11y wiring (`role="application"`, focusable nodes), dark/light theming, edge routing — the whole interaction layer of a node editor, proven working under Korvun's CSP and inside the desktop iframe on day one. |
| **Hand-roll cost** | High and misplaced. A credible hand-rolled canvas (SVG edges, drag capture across an iframe boundary, connection gesture state machine, a11y) is weeks of UI-platform work that is not Korvun's value; the policy/config semantics on top of the canvas are. |
| **Maintenance** | Low-moderate. One npm dependency in the one frontend that already carries React; exact-pinned (ADR-0029); zero Go-side surface. The lazy chunk adds 178.02 kB JS + 16.68 kB CSS (gzip 57.49 + 2.82 kB) loaded ONLY on the canvas page; the main bundle stayed flat (207.44 → 209.59 kB, the query-param gate). Fonts still dominate `dist` (1.2 M → 1.4 M total). |
| **Risk / volatility** | Low. MIT; v12 is the consolidated stable line of the segment's dominant library; the npm registry and docs agree on identity and support. One honest string attached: the built-in attribution link ("React Flow", bottom-right) is asked to stay visible unless subscribed to Pro — kept visible, restyled onto house tokens because its stock `#999999` fails WCAG AA in light mode (axe caught it; spike CSS fixes contrast without hiding it). |

**Net:** the capability is the entire point of the canvas track, the price is
one pinned MIT dependency plus a lazy-loaded chunk, and the two real risks
(CSP, iframe interaction) are the ones the spike measured directly. The gate
passes.

## Spike GO/NO-GO: **GO**

Evidence, all against the real `make build` binary except (e) which runs the
SP6 desktop harness (`go run` of the same tree):

| Criterion | Result |
|-----------|--------|
| a. Real binary, CSP, no external requests | PASS — 0 CSP violations, 0 external requests, 0 console errors; header served literally |
| b. Node drag + connection create + validation | PASS — drag Δ(83,55) px; valid connect 1→2 edges; self-connection rejected (stays 2) |
| c. Light + dark on house tokens | PASS — `--base` #0a0a0c ↔ #f7f7f9, canvas class follows `colorMode` |
| d. axe on the spike view | PASS — 0 violations in dark AND light (after the attribution restyle) |
| e. NC-5 desktop iframe (drag / pointer-capture / focus) | PASS — Playwright spec `cmd/korvun-desktop/frontend/e2e/spike-sp0.spec.ts` green; capture `design-drafts/sp0-canvas-iframe.png` |
| f. Bundle before/after (informative) | main 207.44→209.59 kB; lazy spike chunk 178.02 kB JS + 16.68 kB CSS (gzip 57.49/2.82) |

Spike artifacts (no commit in SP0; copilot reviews first):
`web/builder/src/spike/FlowSpike.tsx`, `web/builder/src/spike/flow-spike.css`,
the query-param gate in `web/builder/src/main.tsx` (`?spike=flow`, never linked
in the UI), evidence runners `web/builder/e2e/spike-evidence.mjs` and
`cmd/korvun-desktop/frontend/e2e/spike-sp0.spec.ts`.

## Consequences

- SP3 (canvas view) builds on a library already proven under the binary's CSP
  and the desktop iframe — the two failure modes that would have been found
  late are burned down first.
- The canvas should stay a lazy chunk: the non-canvas builder pages keep their
  current weight.
- The attribution stays visible on house tokens; hiding it is a product/pricing
  decision (React Flow Pro), not a technical one.
- **Policy:** the visible, token-restyled attribution IS the decision — zero
  licensing spend, same precedent as code signing. Not to be reopened except by
  Chano's explicit call.
- Version bumps are deliberate events (exact pin), each re-verified against
  Context7 before adoption.

## Alternatives Considered

Per the spec's stop-on-fail rule, alternatives (e.g. hand-rolled SVG canvas,
other graph libraries) were to be evaluated by the copilot ONLY if this spike
failed. It passed on every criterion, so no alternative was pursued; the
hand-roll option is rejected on the four-axis table above.
