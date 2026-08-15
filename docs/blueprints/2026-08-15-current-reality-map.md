# KORVUN — CURRENT REALITY MAP

Informe fáctico del estado del repositorio local a 2026-08-15, HEAD `db45a7b`. Destinatario: un arquitecto sin acceso al disco local que necesita evaluar la evolución de Korvun desde "governed AI tools" hacia una capa de autoridad/ejecución para agentes. Todo lo afirmado está verificado en código fuente salvo indicación expresa; lo no demostrable se marca como tal.

---

## Repository truth

| Estado | SHA | Fecha | Contenido |
|---|---|---|---|
| **A) origin/master publicado** | `81bfad8` | 2026-08-08 | Cierre de la Consola de Operador (pestaña Chat completa) |
| **B) master local** | `db45a7b` | 2026-08-15 | = HEAD. 48 commits por delante de origin/master |
| **C) rama/HEAD activa** | `master` | — | Misma rama; no hay checkout divergente |
| **D) commits locales no publicados** | `32db46f..db45a7b` | 2026-08-09 → 2026-08-15 | 48 commits, ver desglose |
| **E) staged** | — | — | **Ninguno** |
| **F) unstaged** | — | — | **Ninguno** (solo tracked) |
| **G) untracked** | — | — | 8 entradas: tooling de agentes IA + notas de sesión (ver Parte 16) |

- `merge-base master origin/master` = `81bfad8` → la divergencia es un **fast-forward puro**: origin/master no tiene nada que master local no tenga.
- Diff origin/master..master: **107 ficheros, +11.437/−76 líneas** (64 añadidos, 43 modificados).
- Las 9 ramas locales antiguas (`feat/*`, `stage-*`) están todas fusionadas en master; no aportan nada exclusivo.
- Tags publicados: `v0.1.0`…`v0.6.0`. La Consola de Operador está en origin/master pero **sin release etiquetada** (v0.7.0 pendiente de decisión).

**El objeto de este informe es el árbol local en `db45a7b`.** El lote local se compone de tres oleadas:

1. **2026-08-09 — Herramientas gobernadas + skills + carril nativo** (`32db46f`…`5210f81`): spec + ADR-0041 + ADR-0042, SP1–SP5, demostrado en vivo sobre hardware real con modelo local real.
2. **2026-08-15 — Tanda de auditoría** (`28b3cf0`…`b63d3d3`): schema estricto de config (A-1), ventana de dedup del router (R-1), canal de respuesta desconocido (R-7), CI (S-3/S-4/O-1), protocolo de revisión cruzada con Codex.
3. **2026-08-15 — Estreno / red-team E-1…E-14** (`09caa30`…`db45a7b`): 14 endurecimientos (dedup no envenenado, decode estricto en mutación, gobernanza antes del parseo de argumentos, cap de calls nativos, FIFO en read_file, timeout duro en http_fetch, link-local fuera del escudo, cap del idempotency key, invariantes por rol en ValidateRequest, guardia de tool sensible en cerebro cloud sin gobernar, ceiling del superviviente del selector, eco optimista como lista).

Convenciones de etiquetado: **[PUBLISHED]** existe en origin/master · **[LOCAL — NO PUBLICADO]** solo en el lote local · **[PARTIAL]** parcialmente implementado · **[SPEC ONLY]** solo documentación.

---

## PARTE 1 — Identidad del producto actual

**Qué es Korvun hoy (demostrable en código):** un único binario Go (`github.com/Sebastian197/korvun`, Go 1.26.5, 6 dependencias directas) que actúa como gateway de mensajería multi-canal, router multi-modelo y orquestador multi-cerebro, con un motor de políticas de despacho (privacidad/prioridad/consenso) y — solo en local — un sistema de herramientas gobernadas para agentes con auditoría. Corre headless (`korvun serve`) o dentro de una app de escritorio Wails (Korvun Desktop) que embebe el mismo core in-process.

**Problema que resuelve:** que un mensaje real (Telegram/Discord/webhook/consola) llegue a un "cerebro", uno o varios modelos respondan (Ollama local y/o Groq cloud), una política decida la respuesta, y esta vuelva por el mismo canal — con la garantía de que lo marcado privado nunca sale de la máquina, y (en local) con un agente que puede usar herramientas bajo un gate de políticas con ensayo en sombra.

**Qué puede hacer un usuario real hoy (árbol local):**
- Chatear con un modelo local o cloud vía Telegram, Discord, webhook genérico o el chat de la app de escritorio.
- Configurar todo por fichero JSON o visualmente (builder de nodos para brains/modelos/canales/rutas/persona).
- Operar desde el escritorio: leer/contestar conversaciones de cualquier canal como operador (takeover que silencia al cerebro), sesiones `/new`/`/reset`, búsqueda, borrado, no-leídos.
- [LOCAL] Consultar `/tools` en el chat de consola: informe veraz de grants y actividad de herramientas.
- [LOCAL] Promocionar en caliente un grant `shadow`→`allow` editando el JSON y recargando (POST /api/config o Parar/Iniciar en desktop).

**Qué puede hacer un agente/modelo real hoy [LOCAL — NO PUBLICADO]:**
- Llamar 6 herramientas: `echo`, `time`, `calc` (puras), `read_file` (enjaulada en un directorio), `http_fetch` (GET a hosts en allow-list), `webhook_call` (POST JSON a hosts en allow-list) — todas bajo doble puerta de gobernanza.
- Usar tool-calling **nativo** (Ollama `/api/chat` con `tools`) o el protocolo **textual** (`TOOL: name(args)`), según capacidad del modelo; ambos carriles convergen en el mismo `runTool` gobernado.
- Recibir skills en markdown (formato AgentSkills) como guía de prompt.

**Headless:** todo el pipeline (canales, router, brains, modelos, políticas, herramientas, store, control API, métricas, SSE) — el binario `korvun` con subcomandos `serve`/`config check`/`status`/`version`.
**Desktop:** el mismo core embebido tras un `shell.Controller` framework-free; UI Wails v2.13.0 + React 19 con 7 vistas (Inicio, Builder embebido, Chat, Canales, Actividad, Ajustes, Onboarding); secretos en llavero del SO; bearer admin por ciclo que nunca entra al DOM.
**UI/browser:** el builder es una SPA React Flow (`@xyflow/react` 12.11.2) embebida en el binario en `/builder/` tras token; la web pública es VitePress (no compilada en el binario).
**Canales:** telegram (polling/webhook), discord (Gateway WS), webhook genérico, console (interno de la app).
**Modelos/providers:** **solo Ollama y Groq**. No hay Anthropic, OpenAI ni ningún otro (switch de `buildModel` con default `ErrUnknownProvider`, `internal/app/app.go:1084-1112`).
**Ejecución normal:** `korvun serve --config korvun.json` bajo un supervisor con hot-reload, o doble clic en Korvun.app.

**Marketing vs. demostrable:** el README [PUBLISHED] es honesto respecto a origin/master ("everything below is on master today"), pero **no menciona nada del lote local** (tools, skills, native lane, `/tools`, dedup). La frase de identidad "dispatch policy engine (privacy/cost/consensus)" sobrevende en un punto: **la política de coste no existe como código** — solo hay fail-over secuencial como mecanismo; no hay contabilidad de coste ni budget (ver Partes 4 y 13).

---

## PARTE 2 — Mapa arquitectónico real

Flujo completo con nombres reales (todo en un proceso):

```
                        ┌──────────────────────────────── UN PROCESO Go ────────────────────────────────┐
 Telegram ─┐            │                                                                                │
 Discord  ─┤ adapters   │  channel.Channel (+Start/Stop)                                                 │
 Webhook  ─┤ (validan   │      │ Receive() ──► runChannelPump ──► Router.DispatchInbound                 │
 Console  ─┘  en borde) │      │                    │ 1. valida estructura (nil/dirección/conv.id)       │
   ▲                    │      │                    │ 2. dedupWindow.seen (LRU 4096 + TTL 10m) [LOCAL]   │
   │ Send()             │      │                    │ 3. sessionPreDispatch: expiry ► /new ► /tools      │
   │                    │      │                    │    [LOCAL] ► takeover-gate                         │
 channelWorker          │      │                    │ 4. encola en brainWorker (cola 64, timeout 250ms)  │
 (cola outbound 64)     │      │                    ▼                                                    │
   ▲                    │      │            handleAndReply (ctx = ceiling derivado, ceiling.go)          │
   │ sendReply          │      │                    │                                                    │
   │                    │      │                    ▼                                                    │
   │                    │  brain.Brain ─── Orchestrator (multi-modelo, sin tools)                        │
   │                    │       │          fanout/sequential.Coordinator ► policy.Apply                  │
   │                    │       │              (PriorityReducer | ConsensusReducer)                      │
   │                    │       │                                                                        │
   │                    │       └───────── AgentBrain (1 modelo, tools) [LOCAL]                          │
   │                    │                    │ effectiveTools(env) ─► policy.SelectTools  ◄─ PUERTA 1    │
   │                    │                    │   (advertised = allow ∪ shadow)               (anuncio)   │
   │                    │                    │ carril: model.(ToolCallingModel)?                         │
   │                    │                    │   nativo: runLoopNative ─ Ollama /api/chat tools          │
   │                    │                    │   textual: runLoop ─ regex TOOL: name(args)               │
   │                    │                    ▼                                                           │
   │                    │                 runTool(env, decisions, name, args)  ◄─ PUERTA 2 (ejecución)   │
   │                    │                    │ unknown ► deny │ shadow ► NO ejecuta │ deny ► observación │
   │                    │                    ▼                                                           │
   │                    │                 tool.Tool.Execute ── jaula (read_file EvalSymlinks)            │
   │                    │                    │                 escudo red (Dialer.Control, IP resuelta)  │
   │                    │                    │                 allow-list hosts + límites bytes/timeout  │
   │                    │                    ▼                                                           │
   │                    │                 observación ─► de vuelta al modelo (RoleTool | OBSERVATION:)   │
   │                    │                                                                                │
   │                    │  conversation.Store (SQLite v2: sessions+turns | memstore)                     │
   │                    │     solo el par final user+assistant; traza de tools DESCARTADA                │
   │                    │                                                                                │
   │                    │  bus.InMemoryBus (64/subscriptor, no persiste) ─┬─► liveview SSE /api/events   │
   │                    │     tool_used/tool_denied/tool_shadowed         ├─► toolEventRing(64) ► /tools │
   │                    │     metadata-only POR CONSTRUCCIÓN (sin args)   └─► (metrics vía auditTool)    │
   │                    │                                                                                │
   │                    │  httpserver admin 127.0.0.1:2112: /healthz /metrics /api/* /builder/ /ui/      │
   │                    │  supervisor.Supervisor: hot reload preflight ► cutover ► persist ► rollback    │
   └────────────────────┴────────────────────────────────────────────────────────────────────────────────┘
```

Por componente (package · responsabilidad · seams · concurrencia · frontera de seguridad):

- **`internal/envelope`** — tipo canónico `Envelope{ID, Channel, Direction, Sender, Parts, Meta, Keyboard, Operation}`; IDs time-sortable (`crypto/rand` + contador atómico); [LOCAL] `MetaProviderEventID` para dedup y `MetaAck` para acks del router. Sin estado, sin I/O.
- **`internal/channel/*`** — adapters tras `channel.Channel{Name, Manifest, Send, Receive}` + lifecycle app-level. **Frontera de entrada no confiable**: cada adapter valida en su borde (auth de tiempo constante, caps de body 1 MiB, anti-bucle). Colas inbound acotadas con drop contado.
- **`internal/router`** — tabla estática canal→brain; colas acotadas por brain (64) y por canal outbound (64); 1 worker por brain por defecto; ceiling de handler derivado; [LOCAL] dedup window y comando `/tools`. Llama a `Brain.Handle`, publica eventos al bus. No importa `internal/policy`.
- **`internal/brain`** — `Orchestrator` (multi-modelo, sin tools) y `AgentBrain` (un modelo, loop de tools acotado). **Frontera de seguridad crítica: `runTool` es el único llamador de `Tool.Execute`** en el paquete.
- **`internal/model` + providers** — seam `Model{Generate, Name}` y [LOCAL] `ToolCallingModel`; decoradores `retry.New` y `brain.WithModelID` propagan la capacidad; sentinelas de error compartidos.
- **`internal/policy`** — funciones puras sin I/O: `SelectModels` (privacidad, en boot), `PriorityReducer`/`ConsensusReducer` (post-despacho), [LOCAL] `SelectTools` (por mensaje).
- **`internal/tool`** — hoja stdlib-only: catálogo de 6, jaulas y escudo de red. **Frontera de efecto externo**: aquí es donde el proceso toca disco/red por orden del modelo.
- **`internal/conversation`** — `Store`/`SessionStore`; SQLite WAL 1-conexión o memstore; persistencia del par final únicamente.
- **`internal/bus` / `internal/liveview` / `internal/metrics`** — observabilidad volátil; sin contenido por construcción.
- **`internal/app`** — composition root: `Build` (con el orden exacto de boot), `Preflight` sin efectos, guardias fail-loud, ceiling, ring de `/tools`.
- **`internal/supervisor`** — supervisa la App entera: reload single-flight, preflight con la app vieja viva, rollback en cutover fallido, persist solo tras Start confirmado.
- **`internal/shell` + `cmd/korvun-desktop`** — controller framework-free para el desktop, proxy same-origin que inyecta el bearer server-side, llavero del SO, bindings acotados por deadline.
- **`internal/controlapi` / `internal/httpserver`** — superficie admin loopback-por-defecto: lectura sin auth, mutación/consola/builder tras bearer SHA-256 tiempo-constante.
- **`internal/cli`** — subcomandos stdlib; `serve` monta el supervisor.

---

## PARTE 3 — Inventario de packages

| Package | Qué hace realmente | Seams/notas |
|---|---|---|
| `internal/app` | Composition root. `Build` en 12 pasos ordenados; `Preflight` repetible sin efectos; guardias de boot (`ErrMissingSecret`, `ErrUnknownTool`, `ErrMissingToolCage`, `ErrSensitiveToolUngoverned` [LOCAL], `ErrAgentModelCount`, `ErrCeilingOverrideTooLow`); `ceiling.go` deriva timeouts por forma de brain; `tools_report.go` [LOCAL] ring 64 + informe `/tools`. Shutdown en orden ADR-0008 (canales → router → store solo si drenó → liveview → admin → bus). | `WithReloader`, `WithChannelFactory` (seams de desktop y e2e) |
| `internal/brain` | Los 2 Brains + protocolo textual + carril nativo [LOCAL] + persona + traducción envelope↔request. `named.go` es un decorador de Model (WithModelID), no un Brain. | `Brain.Handle`; `Coordinator`; `ToolEventPublisher` |
| `internal/bus` | Bus in-memory acotado (64/sub), at-most-once, sin persistencia, panic-safe; 7 tipos de evento (4 de mensaje + 3 de tool [LOCAL]). Envelope publicado = inmutable por contrato. | `Publish`/`Subscribe` |
| `internal/channel` | Interface + Manifest + Registry (el Registry **no lo usa nadie** en producción — hallazgo). Subpaquetes: telegram, discord, webhook, console. | `channel.Channel` |
| `internal/cli` | `serve`/`config check [--preflight]`/`status`/`version`/`help`, shim retrocompat `-config`; exit codes 0/1/2; color VT-gated. | `Run(args, stdout, stderr) int` |
| `internal/config` | Schema JSON completo + validación estructural; [LOCAL] decode estricto (`DisallowUnknownFields` con clave nombrada) + `schema_version` guard. La validación semántica vive en `internal/app` (split documentado). | `Load`/`Validate` |
| `internal/controlapi` | Lectura sin auth (`/api/brains`, `/api/channels`); mutación gated (`POST/GET /api/config`, strict decode [LOCAL], guard anti-self-lock 409); consola de operador (12 rutas, todas gated); `GET /api/reload/{handle}` **sin auth** (hallazgo). | `Reloader` seam |
| `internal/conversation` | `Store`/`SessionStore`, `Key = channel::conversation.id`, roles user/assistant/system/operator; memstore (ilimitado, doble de test permanente) y sqlite. | `AppendTurns` atómico |
| `internal/envelope` | Tipo canónico + validación + operaciones de chat + transcript con marcadores de media + [LOCAL] `meta.go`/`ack.go`. | — |
| `internal/httpserver` | Mux genérico + `/healthz`; bind síncrono (fallo de bind = boot fallido); readHeaderTimeout 10s; **sin TLS** (el único TLS del pipeline es el webhook de Telegram). | `Handle(pattern, h)` |
| `internal/liveview` | SSE `/api/events` + UI embebida `/ui/`; frames secret-free por construcción (nunca toca Parts/Meta/Err); buffer 64/conexión con drop contado. | suscriptor del bus |
| `internal/metrics` (+`prom`) | Seam de 9 métodos + Nop; registry Prometheus privado; 10 instrumentos push + 4 pull (tabla en Parte 8). | `Metrics` |
| `internal/model` (+ollama, groq, fanout, sequential, retry) | Seam `Model`, [LOCAL] `ToolCallingModel` + `RoleTool` + `ValidateRequest` con invariantes por rol; providers Ollama/Groq; coordinadores fanout (paralelo, cancel-on-first-success opcional) y sequential (serie); retry con backoff jitter + Retry-After. | — |
| `internal/policy` | Puro, sin I/O: sensibilidad/localidad, `SelectModels`, reducers priority/consensus, [LOCAL] `SelectTools` tri-estado. | `Policy.Apply` |
| `internal/router` | Dispatch, colas, ceilings, sesiones/takeover/triggers, [LOCAL] dedup + `/tools` + fix de canal de respuesta desconocido. | `WithSessionStore`, `WithToolsCommand`, `WithEventPublisher` |
| `internal/shell` (+keyring) | Controller del desktop (Start relee config del disco [LOCAL fix], bearer por ciclo, provisión de secretos desde llavero con precedencia env); proxy same-origin; bindings con deadline; first-run/upgrade de config. | `SecretStore` |
| `internal/skill` [LOCAL] | Loader AgentSkills read-only, parser YAML plano propio, budget de prompt. | `LoadDir`, `PromptBlock` |
| `internal/supervisor` | Supervisa la App: reload single-flight, rollback, `WriteConfigAtomic`, estados `pending/cutover/succeeded/rolled-back/failed/persist-failed`. | `BuildFunc/PreflightFunc/PersistFunc` |
| `internal/tool` [LOCAL] | Catálogo 6 tools, `ParamTool`, jaulas, escudo. Hoja stdlib-only. | `tool.Tool`, `Registry` |
| `internal/buildinfo` | `--version` con revisión VCS. | — |
| `cmd/korvun` / `cmd/korvun-desktop` / `cmd/demo-agent` | main de 3 líneas / app Wails + e2e-harness ([LOCAL] flag `-agent-config` para conducir configs de operador bajo Chrome real) / esqueleto Stage 8 desechable. | — |

---

## PARTE 4 — Brains y modelos

**Interface:** `Brain.Handle(ctx, *Envelope) ([]*Envelope, error)` — un método (`brain.go:26-28`). **Existen exactamente dos implementaciones**; "echo/named/translate/consensus/fanout" NO son Brains (named = decorador de Model; translate = funciones puras; consensus/fanout = reducer de policy y coordinador de despacho).

**1. `Orchestrator`** [PUBLISHED] — multi-modelo sin herramientas:
- Algoritmo: carga historial (degrada sin memoria, nunca descarta la respuesta) → construye request con system prompt + historial → `Coordinator.Run` (fanout paralelo o sequential fail-over) → `policy.Apply` (priority o consensus) → clasifica error (`ErrNoUsableOutcome`/`ErrNoConsensus` ⇒ fallback honesto con texto diferenciado; otro ⇒ propaga) → persiste par user+assistant atómico en ctx detached (5s) → un envelope de salida.
- Sin retries propios (viven en el decorador `retry`); sin timeout propio (el ceiling es del router); stateless y seguro bajo N workers.

**2. `AgentBrain`** [LOCAL — NO PUBLICADO] — un solo modelo, loop de herramientas acotado:
- `Handle`: historial → **gobernanza una vez por mensaje** (`effectiveTools` → `SelectTools`) → elección de carril por type assertion `model.(model.ToolCallingModel)` → seed prompt (persona-prefijo → base nativo o gramática textual+catálogo → operator prompt → skills-sufijo) → loop nativo o textual (máx. `maxIters`, default 5) → si no hay respuesta, fallback (no persistido) → persiste SOLO el par final; **la traza de tools se descarta** (ADR-0021 §6).
- `runTool` (el seam único): unknown→deny audit; shadow→NO ejecuta + observación de ensayo; deny→observación muda de regla ("not permitted here"; la regla va solo a auditoría); ejecuta con reclasificación de jaula/escudo como denegación.
- Timeouts: `WithAgentPerToolTimeout` y `WithAgentPerModelTimeout` existen pero **no están cableados en producción** (solo `cmd/demo-agent`); el ceiling del router acota todo el Handle.

**Model interface:** `Model{Generate, Name}`; [LOCAL] `ToolCallingModel{Model; GenerateWithTools(ctx, req, []ToolSpec)}`. `StreamingModel` **no existe** (solo prosa). Roles: system/user/assistant + [LOCAL] `RoleTool`; `Message` creció aditivamente con `ToolCalls`/`ToolName`. `ValidateRequest` [LOCAL, estreno E-10] impone invariantes por rol: system/user sin campos de tool; assistant sin `ToolName`; tool con `ToolName` y sin `ToolCalls`; contenido vacío legal solo en assistant con tool_calls.

**Providers:** solo **Ollama** (`/api/chat`, sin auth, base `OLLAMA_HOST`|`127.0.0.1:11434`; **único con carril nativo**) y **Groq** (OpenAI-compatible, Bearer, key env-only, redactada; **sin GenerateWithTools** — un agente sobre Groq cae silenciosamente al carril textual, deviación pre-declarada en ADR-0042 §3). Gramática de errores compartida: `ErrAuthInvalid`/`ErrRateLimited{RetryAfter}`/`ErrProviderUnavailable`/`ErrProviderResponse`.

**Decoradores:** `retry.New` (backoff jitter base 200ms, cap 2s/espera, Retry-After cap 30s, jamás reintenta `ErrAuthInvalid`, invariante F6 de arranque en frío) y `brain.WithModelID` (liga el model id en copia superficial). Ambos [LOCAL] propagan `ToolCallingModel` por type assertion en construcción, y el carril nativo pasa por el MISMO retryLoop que Generate ("no pueden divergir en semántica de retry"). Cadena de producción: `adapter → retry.New → WithModelID`, verificada end-to-end por un smoke vivo opt-in (`KORVUN_LIVE_OLLAMA`).

**Políticas de despacho — qué existe de verdad:**

| Dimensión | Estado | Realidad |
|---|---|---|
| Privacidad | ✅ | `SelectModels` en boot: brain Private conserva solo modelos Local; vacío ⇒ error de boot |
| Prioridad | ✅ | `PriorityReducer` por orden declarado del operador |
| Consenso | ✅ | `ConsensusReducer`: mayoría estricta ≥2 votos, contenido normalizado |
| Sensibilidad→tools | ✅ [LOCAL] | `SelectTools` por mensaje |
| Localidad | ✅ (atributo) | Declarada en catálogo; consumida por privacidad y por la regla sensitive×cloud |
| **Coste** | ❌ | Solo `Accounting.Latency`; coste monetario/budget **diferido explícitamente** (`policy.go:88-90`). Sequential es mecanismo de ahorro, no política de coste |
| Selección por mensaje / por contenido | ❌ | La sensibilidad se DECLARA, nunca se infiere; selector per-message diferido |

---

## PARTE 5 — Governed tools [LOCAL — NO PUBLICADO, íntegro]

**Seam:** `tool.Tool{Name, Description, Execute(ctx, args string) (string, error)}` — contrato de concurrencia en godoc; error = observación, nunca pánico. `Registry map[string]Tool` read-only tras construcción. `ParamTool{Params() []ToolParam; ArgsFromCall(fields) (string, error)}` para campos estructurados en el carril nativo — **única implementación: `webhook_call`**.

**Catálogo (6):**

| Tool | Clase | Límites | Attrs de casa |
|---|---|---|---|
| `echo` | pura | — | `{}` |
| `time` | pura (UTC RFC3339) | — | `{}` |
| `calc` | pura (parser propio) | 1024 bytes, sin no-finitos | `{}` |
| `read_file` | enjaulada, FS read | root obligatorio, 64 KiB default, solo ficheros regulares (E-6) | `Sensitive` |
| `http_fetch` | enjaulada, red GET | allow-list obligatoria, 64 KiB, 3 redirects, timeout duro 30s (E-7) | `Network` |
| `webhook_call` | enjaulada, red POST JSON efectiva | allow-list obligatoria, 64 KiB, **0 redirects**, timeout 10s | `Network` |

`shell` no existe por decisión (`BuiltinAttrs` devuelve `ok=false` para cualquier otro nombre). Las enjauladas jamás se resuelven por `Builtin(name)`: exigen su bloque de config o el boot falla (`ErrMissingToolCage`).

**Modelo de gobernanza (`internal/policy/tools.go`)** — puro, determinista, una vez por Handle:
- `ToolMode`: `ToolAllow`/`ToolShadow`/`ToolDeny` (cero = inválido). `ToolGrant{Name, Mode, Channels}` (channels vacío = todos; no-vacío = match exacto). `ToolAttrs{Sensitive, Network}` — declarados, jamás inferidos. `ToolDecision{Mode, Rule, Shield}`. `ToolQuery{Channel, Sensitivity, Locality}` — **nótese: el remitente humano NO es input**.
- **Precedencia exacta** (`decideTool`): 1) `deny` explícito → 2) restricción de canal → 3) `Sensitive && Locality==Cloud` → 4) modo otorgado, con `Shield = Network && Sensitivity==Private`. Las restricciones van ANTES que el modo: la puerta restringe, nunca ensancha.
- Reglas de auditoría: `deny`/`not_granted`/`channel`/`sensitive_locality`/`private_network_shield`/`cage` (+`unknown_tool` en ejecución).
- **Fail-closed**: sin grant = deny; error de `SelectTools` ⇒ registry vacío + mapa de decisiones vacío no-nil (deny-all) y el prompt no enseña ni la gramática de tools. **El montaje es opt-in**: sin bloque `governance`, el brain agente queda sin puerta (comportamiento pre-ADR-0041 byte a byte) — mitigado por la guardia de boot E-11.
- Compatibilidad legacy: todos los campos ADR-0041 son opcionales; un bloque agent antiguo valida idéntico.

**Las dos puertas:**
1. **Anuncio** (`effectiveTools`, `agent.go:580-602`, una vez por Handle): el modelo solo conoce `allow ∪ shadow` (shadow se anuncia a propósito: el ensayo observa el juicio real del modelo). Alimenta `buildSystemPrompt` (textual) y `toToolSpecs` (nativo).
2. **Ejecución** (`runTool`, `agent.go:472-539`): unknown primero → shadow (NO se llama a `Execute`; el modelo recibe el texto de ENSAYO que le prohíbe ofrecerse a hacerlo a mano) → deny (observación sin regla) → `Execute` → `cageRule` reclasifica jaula/escudo como DENEGACIÓN manteniendo el error honesto hacia el modelo.

**Jaula de `read_file`:** root resuelto en construcción con `Abs`+`EvalSymlinks`; por llamada, decisión sobre la ruta RESUELTA (un symlink muere sobre su destino real); ruta inexistente fuera de la jaula responde "cage violation", nunca "not found" (no filtra existencia); solo ficheros regulares, verificado antes de abrir Y sobre el descriptor abierto (queda documentada la ventana residual check-to-open); lectura por `LimitReader(max+1)`.

**Escudo de red (SSRF):** `net.Dialer.Control = shieldControl` — corre en **cada intento de conexión con la dirección YA RESUELTA, después del DNS**: DNS-rebinding y redirects fuera de escudo mueren en el socket. Permite solo `IsLoopback() || IsPrivate()` (unmapeando IPv4-in-IPv6); **link-local excluido deliberadamente** (E-8: `169.254.169.254`/`fe80::/10`, el objetivo de robo de credenciales de metadata cloud); no parseable ⇒ fail closed. `Transport.Proxy = nil` a propósito (un proxy de entorno puentearía allow-list y escudo). Allow-list case-insensitive con puerto opcional pinneado; **cada salto de redirect se re-valida** contra la lista y re-dial-a por el escudo. El escudo se arma en wiring (`Network && sens==Private`) — funciona incluso en brain Private sin gobernar. Nota: `ToolDecision.Shield` se calcula pero **ningún consumidor runtime lo lee** (el armado real es del wiring).

**Flujo completo:** Router (`/tools` interceptado antes del brain) → `AgentBrain.Handle` → `effectiveTools` → carril → [nativo: `callNative` → rescate de JSON impreso → **gobernanza ANTES del parseo de argumentos** (E-3) → cap 8 calls/turno (E-4/E-5)] → `runTool` → `Execute` → observación (deny="not permitted here"; shadow=texto de ensayo; error="tool X failed: …"; ok=salida cruda; vacía→"(the tool returned an empty result)") → siguiente turno del modelo.

**Guardias de boot:** grant sobre tool no listada = error; `tool_attrs` sobre tool no listada = error; **tool Sensitive en brain cloud SIN gobernanza = `ErrSensitiveToolUngoverned`** (E-11 — cierra el hueco de que la regla sensitive×locality solo existe con puerta montada); brain agente debe resolver a exactamente 1 modelo; `Preflight` construye todos los brains (jaulas y allow-lists incluidas) sin efectos.

---

## PARTE 6 — Native tool calling [LOCAL — NO PUBLICADO]

- **Tipos:** `ToolSpec/ToolParamSpec/ToolCall` + `ToolCallingModel` (interfaz hermana, ADR-0042) + `RoleTool` + crecimiento aditivo de `Message`.
- **Provider:** solo Ollama (`internal/model/ollama/toolcalling.go`); structs de wire separados de los antiguos — el wire viejo es byte-idéntico. Groq: pendiente (declarado en el ADR).
- **Detección de capacidad:** una única type assertion en `agent.go:347`. Sin flag de config, sin probe.
- **Serialización:** `toToolSpecs` sobre el registry YA filtrado por la puerta de anuncio, ordenado por nombre; `ParamTool` aporta campos declarados; sin ellos, schema uniforme `{"args": string}` (`required:["args"]`).
- **Argumentos:** `Arguments map[string]any` → `ArgsFromCall` (ParamTool) o `nativeArgs` (`args` string / mapa serializado). Error de parseo de un tool permitido se audita como `tool_used error` (cierra el hueco de audit de ParamTool).
- **Retorno:** `Message{Role: RoleTool, ToolName, Content: observación}`; observación vacía reescrita para no violar `ValidateRequest`.
- **Endurecimientos:** cap 8 calls/turno (exceso = observación honesta sin ejecutar); rescate de tool-call impreso como texto (exactamente un objeto JSON nombrando una tool registrada → se convierte en call gobernada y el blob se borra del contexto; JSON ordinario pasa como respuesta); pánico del adapter recuperado; tool alucinada auditada `unknown_tool`.
- **Convivencia:** carriles excluyentes por mensaje; el textual conserva la gramática regex `TOOL: name(args)`; historial re-inyectado salta los turnos system (el informe `/tools` envenenaba el contexto — fix `96d0e5a`).
- **¿Misma gobernanza? SÍ, por construcción:** ambos carriles llaman al **mismo `runTool` con el mismo mapa `decisions`** (`agent.go:448` vs `agent_native.go:120,131`), y `runTool` es el único llamador de `Execute`. La única asimetría es deliberada y endurece: el carril nativo consulta la puerta ANTES de parsear argumentos, de modo que un error de parseo no puede colar un intento.
- **Riesgo de bypass residual observado:** ninguno dentro del brain (no hay segundo camino a `Execute`). Los puntos débiles reales están fuera del seam: el armado del escudo depende del wiring (no de `ToolDecision.Shield`), y una config sin bloque `governance` desmonta la puerta entera (mitigado solo para tools Sensitive en cloud por E-11 — un brain **local** sin gobernar mantiene todas sus tools en allow).

---

## PARTE 7 — Skills [LOCAL — NO PUBLICADO]

- **Formato:** AgentSkills-compatible — `<skills_dir>/<nombre>/SKILL.md` con frontmatter YAML **plano** parseado por un parser propio de subset (cero dependencias). Campos: `name` (obligatorio, ≤64, kebab-case, **debe igualar el directorio**), `description` (obligatorio, ≤1024), `license`, `compatibility`, `allowed-tools` (separado por espacios). Claves desconocidas y bloques anidados se ignoran con tolerancia.
- **Loader:** `LoadDir` — un solo nivel plano, cap 64 KiB por fichero, resultado ordenado determinista. Raíz ilegible = **error de boot**; skill malformada = **saltada con Warn** (jamás rompe el boot). Solo lectura; symlinks ignorados; **cero niveles de referencia** (la ADR-0041 §6 promete "un nivel"; el código implementa cero y lo declara — deviación documentada).
- **Budget:** `PromptBlock(skills, budget)` — nombres+descripciones SIEMPRE; cuerpos greedy por orden de nombre bajo `skills_body_budget` runas (default 8192); cuerpo que no cabe se omite (con Warn) pero la skill sigue listada.
- **Entrada al prompt:** sufijo del system prompt del AgentBrain, tras persona/protocolo/operator prompt, idéntico en ambos carriles; **recargado en cada Build** ⇒ el hot reload recoge skills editadas.
- **Relación con tools: ninguna con autoridad.** `allowed-tools` se registra y **nadie lo consume** — "Skills are DOCUMENTATION, never authorization" (spec D-4). Una skill NO puede: otorgar/ensanchar grants, ejecutar nada, referenciar ficheros, ser per-canal o dinámica.
- La frase de la spec "cuerpos de skills **otorgadas**" no tiene filtro en código (no existe mapeo grant→skill) — discrepancia menor doc/código.
- Ejemplo incluido: `docs/examples/skills/web-research/SKILL.md` (enseña `http_fetch`, incluida la conducta ante shadow).

---

## PARTE 8 — Auditoría y observabilidad

**Emisión de auditoría de tools [LOCAL]:** exactamente una función, `auditTool` (`agent.go:561-570`) → siempre métrica, y bus si está montado. Seis sitios de emisión (unknown, shadowed, gate-deny, cage/shield-deny, used ok|error, arg-error nativo).

**Superficies (3):**
1. **slog** — única superficie con contexto de args, acotado a **80 runas** (`boundedArgs`); nunca llega a bus/feed/labels.
2. **bus → liveview SSE + ring `/tools`** — `Event{Type, Tool, Outcome(ok|error|denied|shadowed), Rule (solo denied), Latency (solo used), Envelope, Channel, Brain}`; **sin campo de args por construcción** (ley de no-contenido ADR-0024 §1). Frames SSE nunca tocan `Parts`/`Meta`/`Err`.
3. **Prometheus** — `korvun_tool_calls_total{tool,outcome}` y `korvun_tool_call_duration_seconds{tool,outcome}` (histograma solo para ok|error: un cero de denied envenenaría la distribución).

**Ring de auditoría:** `toolEventRing` — 64 slots en memoria, FIFO con desplazamiento, informe `/tools` muestra los 10 últimos por brain. Montado solo con bus vivo (observabilidad on), como comando del canal console, respondido ANTES de la puerta de takeover, con `Meta[korvun.ack]=tools-report` (la console lo persiste como turno system; el historial lo salta).

**Métricas completas:** `korvun_messages_processed_total{channel}`, `korvun_provider_request_duration_seconds{provider,outcome}`, `korvun_provider_failures_total`, `korvun_provider_retries_total`, `korvun_provider_retry_budget_exhausted_total`, `korvun_router_errors_total{kind}`, `korvun_conversation_turns_persisted_total`, [LOCAL] `korvun_tool_calls_total`, `korvun_tool_call_duration_seconds`, `korvun_deduped_total{channel}`; pull: `korvun_channel_messages_dropped_total`, `korvun_channel_reconnects_total`, `korvun_bus_events_dropped_total`, `korvun_sse_events_dropped_total`.

**Qué NO se registra deliberadamente:** argumentos de tools (fuera de slog acotado), resultados de tools, contenido de mensajes en cualquier superficie de eventos, valores de secretos (solo NOMBRES de variables).

**Audit trail vs. prueba criptográfica — veredicto tajante:** lo que existe es un rastro de auditoría operativo y **100 % volátil** (bus sin replay, ring en memoria, SSE at-most-once, feed desktop 250 frames; SQLite solo tiene tablas `sessions` y `turns` — ninguna de eventos). **No existe hoy firma, HMAC, hash-chain ni receipt criptográfico de ningún evento**: el único `sha256` runtime es la comparación de bearer en tiempo constante, y el único signing del proyecto es cosign/Rekor sobre artefactos de release (CI). Persistir el audit trail fue **considerado y rechazado** en ADR-0041 ("observability, not memory").

Hallazgo de coherencia UI: el feed de Actividad del **desktop no pinta los eventos de tool** — `feed/frame.ts` descarta `tool/outcome/rule/latency_ms` y el switch los ignora. La única superficie de tools visible al operador es el texto de `/tools` en el chat y la UI SSE embebida `/ui/`.

---

## PARTE 9 — Conversaciones, memoria y sesiones

- **Store:** `Store{LoadRecent, Append, AppendTurns}` + `SessionStore{NewSession, ListConversations, ListSessions, LoadSession, DeleteConversation, DeleteSession, SearchTurns}`. Key = `channel::conversation.id`. `Turn` value-only (invariante para el shallow-copy del memstore).
- **SQLite** (modernc, cgo-free): WAL, 1 conexión, schema v2 (`sessions` + `turns` con PK `(key,session,seq)`, seq por sesión); migración v1→v2 idempotente en una transacción. Store configurado e inabrible = boot fatal. **Memstore**: doble de test permanente, ilimitado por diseño.
- **Sesiones (estilo OpenClaw):** monotónicas por key; activa = la más alta; `LoadRecent` lee SOLO la activa ⇒ `/new`/`/reset` es un **corte duro de contexto** (verificado a tres niveles tras el falso positivo del 2026-08-09); sesiones viejas legibles como archivo. Expiración perezosa diaria/idle sin timers. Borrado de la sesión activa = 409.
- **Triggers:** primer token exacto case-sensitive; el resto del mensaje sigue al brain reescrito en copia; fallo del store en trigger **falla abierto** (el mensaje original continúa).
- **Takeover:** mapa en memoria por conversación, sobrevive a resets de sesión, **no** sobrevive a reinicios (falla abierto hacia el brain, nunca hacia el silencio). Bajo takeover las palabras del usuario persisten y el brain calla.
- **Console channel:** sin red; inbound solo vía `POST /api/conversations/{key}/message` (bearer); `Send` de una respuesta normal es no-op (el brain ya persistió); solo los acks (`session-reset`, `tools-report`) se persisten como turnos system.
- **Tool calls en persistencia: nada.** Solo el par final user+assistant, atómico, en contexto detached de cancelación (5s) — la traza intermedia es scratch local del loop. El fallback enlatado no se persiste. Adjuntos persisten como marcadores `[image]`… vía `TranscriptText`.
- Unread/search/attachments: implementados en la Consola (búsqueda substring case-insensitive con escape de LIKE; límites por defecto 50/100).

---

## PARTE 10 — Desktop / Operator Console / Builder

- **Tecnología:** Wails v2.13.0, build tag `desktop`; core **in-process** tras `shell.Controller` (cero imports de Wails en `internal/shell`).
- **Lifecycle:** Iniciar/Detener con serialización por busy-gate; **[LOCAL fix `343da00`] `Start` relee el config DEL DISCO en cada arranque** (antes una edición entre Parar e Iniciar se ignoraba — era el cerrojo de la promoción en caliente); fichero corrupto rechaza el boot con error claro; `reapLocked` detecta un supervisor muerto por sí solo.
- **Config reload/promoción:** dos vías — (a) headless/builder: `POST /api/config` → supervisor (preflight con la app vieja viva → cutover → persist solo tras Start confirmado → rollback si falla); (b) desktop: editar JSON + Parar/Iniciar. La Control API en modo app está sellada por diseño para mutación externa (bearer por ciclo de 32 bytes crypto/rand que no sale del proceso; el proxy lo inyecta server-side y nunca entra al DOM).
- **Onboarding/first-run:** config plantilla sin canales (boot válido con 0 canales), check de Ollama, asistente de canal en 3 pasos con llavero.
- **Keychain:** zalando/go-keyring (Keychain/Credential Manager/Secret Service, sin fallback a fichero); servicio `korvun`, cuenta = NOMBRE de la env var; env no vacío siempre gana; re-provisión en cada reload (fix F1); limpieza exacta al fin de ciclo.
- **Vistas existentes (7):** Home (estados honestos marcha/parado/incidencia), BuilderEmbed (iframe same-origin `/builder/`), Console (chat completo: inbox, takeover, sesiones, búsqueda, borrado, composer; [LOCAL E-14] el eco optimista es una lista — envíos rápidos ya no se pisan), Channels + ChannelWizard (alta/baja de canales por la tubería de mutación), Activity (feed metadata-only; **sin eventos de tool**), Onboarding, Settings.
- **Builder (SPA `web/builder`):** canvas React Flow editando brains/modelos/canales/rutas/persona; POSTea el documento entero (unknown fields hacen round-trip por `structuredClone` del GET).
- **PANEL DE GOBERNANZA: NO EXISTE — verificado en ambos frontends.** El grep de `governance|grant|shadow|tool_attrs` sobre `cmd/korvun-desktop/frontend/src` solo devuelve CSS `box-shadow`; `web/builder/src/config/schema.ts:23-27` modela `AgentConfig` solo como `{tools, max_iterations, system_prompt}`. Es el punto 7 de aceptación de la pieza (FR-UI-1 / SP6), **pendiente de mockups aprobados antes de codificar** (ley de diseño de la casa). Consecuencia operativa: **todo el bloque de gobernanza se edita hoy a mano en JSON** — `governance`, `tool_attrs`, jaulas (`read_file`/`http_fetch`/`webhook_call`), `skills_dir`, `skills_body_budget` — igual que `session`, `retry`, `schema_version`, timeouts y `warmup`. Desde UI solo: canales (alta/baja), brains/modelos/rutas/persona (builder), tema/autostart.
- **e2e-harness [LOCAL ampliado]:** programa Go sin Wails que sirve chrome+proxy sobre un core real sin red bajo Playwright/Chrome real; el flag `-agent-config` ejecuta un config de operador verbatim (la vía por la que se ensayó la demo de tools).

---

## PARTE 11 — Configuración real

Schema actual (todos los campos verificados contra struct tags de `internal/config/config.go`):

- **Raíz:** `schema_version` (solo 1; ausente=1), `_comment`, `request_timeout`, `brain_handler_timeout` (debe superar el ceiling derivado), `channels[]` (puede ser vacío), `brains[]` (≥1), `routes[]` (≥1 si hay canales), `storage{path}`, `observability{enabled,addr}` (**ausente = ON en loopback**), `admin{token_env}`, `session{triggers,daily_at,idle_min}`.
- **[LOCAL] Decode estricto:** clave desconocida rechazada POR NOMBRE (`ErrUnknownField`), documento JSON trailing rechazado, mismo rigor en `POST /api/config` (una `"governence"` mal escrita producía antes un brain sin gobernar que el operador creía gobernado).
- **`channels[]`:** `type` (telegram|discord|webhook|console), `mode`, `token_env`, `webhook{bind,path,outbound_url,outbound_token_env,mapping{...}}`. Console exige `storage`+`session` y no lleva token.
- **`brains[]`:** `name`, `sensitivity` (public|private), `dispatch` (fanout|sequential), `policy{kind: priority|consensus, order}`, `models[]`, `retry`, `persona{display_name,tone,language,instructions}` (con topes en runas), `agent`.
- **`models[]`:** `provider` (ollama|groq), `model_id`, `locality` (local|cloud), `base_url`, `api_key_env` (obligatorio groq), `request_timeout`, `max_retries`, `warmup` (solo local).
- **[LOCAL] `agent`:** `tools[]`, `max_iterations`, `system_prompt`, `governance[]{tool,mode:allow|shadow|deny,channels[]}`, `tool_attrs{<tool>:{sensitive,network}}` (punteros = override por campo), `read_file{root,max_bytes}`, `http_fetch{allow_hosts,max_bytes,max_redirects}`, `webhook_call{allow_hosts,max_bytes,timeout_seconds}`, `skills_dir`, `skills_body_budget`.

Ejemplo mínimo realista (solo campos existentes; brain private ⇒ escudo armado; console exige storage+session; `http_fetch` exige su jaula):

```json
{
  "schema_version": 1,
  "channels": [
    { "type": "telegram", "mode": "polling", "token_env": "TELEGRAM_BOT_TOKEN" },
    { "type": "console" }
  ],
  "brains": [{
    "name": "default",
    "sensitivity": "private",
    "dispatch": "fanout",
    "policy": { "kind": "priority", "order": ["ollama"] },
    "models": [{ "provider": "ollama", "model_id": "llama3.2", "locality": "local",
                 "base_url": "http://127.0.0.1:11434" }],
    "agent": {
      "tools": ["time", "read_file", "webhook_call"],
      "max_iterations": 4,
      "system_prompt": "Answer briefly.",
      "governance": [
        { "tool": "time", "mode": "allow" },
        { "tool": "read_file", "mode": "allow", "channels": ["console"] },
        { "tool": "webhook_call", "mode": "shadow" }
      ],
      "read_file": { "root": "/home/chano/docs" },
      "webhook_call": { "allow_hosts": ["192.168.1.20:5678"] },
      "skills_dir": "/home/chano/korvun-skills"
    }
  }],
  "routes": [
    { "channel": "telegram", "brain": "default" },
    { "channel": "console",  "brain": "default" }
  ],
  "storage": { "path": "" },
  "session": {},
  "admin": { "token_env": "KORVUN_ADMIN_TOKEN" }
}
```

Nota de doc: `docs/CONFIGURATION.md` quedó parcialmente desactualizada el mismo día del strict-schema (tabla raíz sin `schema_version`/`_comment`/timeouts, y marca `channels`/`routes` como obligatorios cuando cero es válido).

---

## PARTE 12 — Seguridad y trust boundaries

**Entrada no confiable:** los cuatro bordes de canal (Telegram: secret-token constante + cap 1 MiB; Discord: mapping puro con anti-bucle self/bot/webhook; Webhook: gate Bearer fail-closed ANTES de leer el body + content-type + cap + idempotency-key ≤128 bytes; Console: bearer + envelope acuñado server-side). Router: solo invariantes estructurales. El **texto del mensaje llega íntegro al modelo** — la inyección de prompt es el vector no mitigado por diseño (mitigado en efectos por la puerta de tools, no en contenido).

**Dónde actúa el modelo:** dentro del loop del AgentBrain; sus únicas salidas al mundo son (a) el texto de respuesta al canal y (b) llamadas a tools.

**"LA FRONTERA ENTRE EL MODELO Y EL MUNDO REAL"** — el punto exacto: **`AgentBrain.runTool` → `tool.Tool.Execute`** (`internal/brain/agent.go:512` es la única invocación de `Execute` del paquete, con la puerta, la jaula y el escudo delante). Todo lo que el modelo puede *hacer* (no decir) pasa por ahí; el único efecto externo posible hoy es `webhook_call` (POST irreversible), `http_fetch` (lectura de red) y `read_file` (lectura de disco enjaulada).

**Controles antes del efecto:** anuncio filtrado → decisión tri-estado por tool/canal/sensibilidad → shadow (simulación) → jaula/allow-list → escudo dial-time → límites bytes/timeout → auditoría metadata en 3 superficies.

**Secretos:** solo NOMBRES de env vars en config/API/errores; valores en env o llavero del SO (desktop); bearer admin por ciclo, nunca persistido ni en DOM; token Groq redactado; Discord re-lee del env en cada uso.

**Red:** admin server loopback por defecto (`127.0.0.1:2112`) **sin TLS y sin warning si se re-liga a 0.0.0.0** (contraste: el webhook channel sí avisa; el harness lo prohíbe; el desktop fuerza `127.0.0.1:0`); lectura (`/api/brains|channels`, `/healthz`, `/metrics`, SSE, `/api/reload/{handle}`) **sin auth** — la invariante "NO AUTH ⇔ LOOPBACK ONLY" se cumple **por defecto, no por enforcement**. Cruzan red: providers (Ollama local / Groq TLS cloud), canales, y las tools de red bajo escudo/allow-list.

**Ejecución de código: nada.** Sin `shell`, sin plugins, sin eval; skills solo texto. Local-only: todo salvo Groq y las salidas de canal/tools.

**Hallazgos de borde (constatados, sin arreglar):** `outbound_token_env` del canal webhook se resuelve y almacena pero `Send` **nunca manda Authorization** (POSTs salientes sin auth); `GET /api/reload/{handle}` sin bearer; `ErrKindSession` sin `String()` (renderiza `unknown(5)` en labels); feed desktop ciego a eventos de tool.

---

## PARTE 13 — Qué NO existe todavía

Auditado a texto completo del árbol local (código, no docs). Veredicto global: **0 de 24 conceptos existen como first-class runtime concepts; 6 tienen precursor genuino; 18 no existen.**

| Concepto | Estado | Precursor / evidencia |
|---|---|---|
| Principal / Actor identity | ❌ | `Participant{ID,Name}` existe pero **ninguna ruta de autorización lee `Sender`**; el input del gate es `{Channel, Sensitivity, Locality}`. Operador = string hardcoded `"operator"` |
| Intent Contract | ❌ | Cero hits (los `Intents` de Discord son el bitfield del protocolo) |
| Authority / delegated authority | ❌ | `ToolGrant` es una fila de config estática, no un objeto portador acuñable/estrechable |
| Action Envelope | ❌ | Solo existe el Envelope de MENSAJE; una tool call viaja como dos strings `(name, args)` — la acción no está reificada |
| Action-level authorization | ❌ | La autorización es por NOMBRE de tool, decidida una vez por mensaje; los args jamás se inspeccionan en la puerta |
| Resource-scoped authorization | 🟡 | Jaula de path + allow-list de hosts + escudo: reales, pero **cableados por tool en construcción, no expresables en política** (un root por binario; no hay "brain X lee /a, brain Y lee /b"; CIDR/wildcard fuera de alcance en ADR-0041) |
| Budgets por intención | 🟡 | Ceiling wall-clock derivado + cap 8 calls/turno + budget de runas de skills: acotan, pero nada es per-intención ni sobrevive a la petición; budget con estado explícitamente "fuera de alcance" (ROADMAP-V1) |
| Expiry por intención | ❌ | Cero TTLs de autorización (los únicos: dedup 10 min y expiración de sesión de chat) |
| Delegation chains | ❌ | Un agente por brain; no hay invocación agente→agente |
| Approval workflow | 🟡 | NO hay approve/reject pre-ejecución. Adyacentes: **takeover** (mute por conversación) y **shadow** (ensayo sin humano en el loop). El "ask-mode" es el peldaño 2 de la escalera de gobernanza progresiva — **post-beta, sin compromiso** (spec §423-427) |
| Prepare / Commit | ❌ | `Execute` es una llamada única sin estado preparado (el "two-phase policy model" del doc es selección-pre + reducción-post, otra cosa) |
| Transactions | ❌ | Solo transacciones SQLite del transcript |
| Compensation / rollback | ❌ | Solo rollback de lifecycle (cutover de config, Start parcial de canal); un POST de `webhook_call` no tiene compensación ni registro suficiente para intentarla |
| Idempotency keys | 🟡 | Reales para **entrega inbound** (event-id por canal + ventana LRU 4096/TTL 10m, fail-open sin id, no durable). **Nada aguas abajo**: dos `webhook_call` idénticos disparan dos POSTs |
| Effect descriptors / classes | 🟡 | `ToolAttrs{Sensitive, Network}` — 2 bools que significan "solo modelos locales" y "armar escudo", **no** clase de efecto: `webhook_call` (escritura irreversible) y `http_fetch` (lectura) llevan el mismo descriptor |
| Execution receipts | 🟡 | El ring de 64 + eventos de bus son el embrión: metadata-only, FIFO, volátil, sin args ni resultados. Persistirlo fue **rechazado** (ADR-0041, ADR-0021 §6) |
| Cryptographic receipts / hash chain / signed decisions | ❌ | Cero HMAC/firma/encadenado runtime; sha256 solo para comparar bearers; cosign es de artefactos de release |
| Policy version pinning per action | ❌ | `ToolDecision{Mode,Rule,Shield}` sin proveniencia; el reload cambia la app entera sin que el vuelo en curso sepa qué generación lo autorizó |
| MCP server | ❌ | Cero hits en `.go` |
| MCP client | ❌ | Solo roadmap post-beta ("candidato principal" para plugins; sin ADR) |
| A2A protocol | ❌ | Cero hits relevantes |
| External-agent gateway | ❌ | La superficie HTTP es admin/builder; la salida más cercana es `webhook_call` ("la puerta del puente n8n", post-beta) |
| Multi-tenant identity | ❌ | Un bearer admin único; aislamiento solo por `conversation.Key`, sin dueño ni check de acceso (per-route secrets anotado como futuro en ADR-0038) |
| Remote policy service | ❌ | El motor es puro/in-process/sin-I/O como restricción de diseño declarada; no hay seam para punto de decisión externo |

---

## PARTE 14 — Distancia hasta una "Execution Trust Layer"

Escala: 0 inexistente · 1 idea/spec · 2 precursor parcial · 3 implementación usable · 4 madura.

| Cimiento | Nota | Justificación |
|---|---|---|
| A. Agent orchestration | **3** | AgentBrain con loop acotado, dos carriles, rescates y caps, demostrado en hardware real. No 4: un solo agente, un solo modelo por brain, sin composición |
| B. Tool execution | **3** | Seam limpio, catálogo de 6 con contratos de concurrencia, ParamTool. No 4: catálogo mínimo, per-tool timeout sin cablear |
| C. Pre-execution policy | **3** | `SelectTools` puro tri-estado, doble puerta, fail-closed, precedencia pineada por tests. No 4: input sin actor ni args; montaje opt-in |
| D. Action containment | **3** | Jaula con EvalSymlinks, escudo dial-time post-DNS, allow-lists re-validadas por salto, límites. No 4: scoping por binario, no por política |
| E. Shadow/rehearsal | **3** | Implementado, auditado, demostrado en vivo con promoción en caliente. No 4: solo por tool estático, sin rehearsal de config completa |
| F. Audit | **2** | Tres superficies coherentes metadata-only con emisor único — pero volátil al 100 %, sin args/resultados, sin persistencia (rechazada) ni prueba |
| G. Persistent state | **3** | SQLite v2 con sesiones, migración, corte duro, atomicidad. Lo persistido es el transcript; las acciones, no |
| H. UI / operator controls | **2** | Consola de operador completa y builder visual — pero **cero UI de gobernanza**: los grants se editan a mano y la promoción exige reload |
| I. Identity | **1** | Sender IDs y roles de transcript existen; ninguno es input de autorización |
| J. Intent | **0** | Nada |
| K. Fine-grained authority | **2** | Grants por tool×brain×canal×modo + attrs + regla sensitive×locality. Sin recurso, sin args, sin tiempo, sin actor |
| L. Delegation | **0** | Nada |
| M. Transactions | **0** | Nada al nivel de acción |
| N. Effects model | **1** | Dos bools que no clasifican el efecto; lectura y escritura irreversible indistinguibles para la puerta |
| O. Approvals | **1–2** | Takeover y shadow como adyacentes reales; el ask-mode es spec post-beta sin compromiso |
| P. Receipts/proof | **1–2** | El ring es el embrión de receipt: metadata volátil, sin identidad de acción, sin cripto |
| Q. Protocol gateway (agentes externos) | **0–1** | No existe; `webhook_call` es una puerta de salida unshot, y el canal webhook una entrada de *mensajes*, no de agentes |

**Korvun hoy es:** un gateway/orquestador de mensajería con IA, monolito Go serio y disciplinado, con un sistema de herramientas gobernadas por *visibilidad* (qué tools ve y ejecuta un modelo, por brain/canal/sensibilidad, con ensayo en sombra, contención de recursos cableada y auditoría operativa volátil) — construido, endurecido por red-team y demostrado en hardware real.

**Korvun todavía NO es:** una capa de autoridad de ejecución. No sabe *quién* pide una acción, *por qué*, sobre *qué recurso concreto*, por *cuánto tiempo* ni con *qué prueba*: no hay identidad de actor en la decisión, ni acción reificada, ni aprobación humana pre-efecto, ni transacción/compensación, ni receipt persistente o firmado, ni protocolo para agentes externos.

**El salto arquitectónico pendiente es:** reificar la acción. Hoy la "acción" son dos strings que cruzan `runTool`; una trust layer exige un objeto de acción de primera clase (actor + intención + recurso + autoridad + expiración) que se decida, se apruebe, se ejecute en dos fases cuando el efecto lo pida, y deje receipt durable y verificable. Las semillas existen y están bien situadas — `runTool` como cuello único, `SelectTools` puro y extensible, shadow como proto-prepare, el ring como proto-receipt, el supervisor como máquina de estados de cambio — pero cada una tendría que crecer de "config estática + strings" a "objetos de runtime con proveniencia". Nada del código actual lo impide; nada de él lo hace todavía.

---

## PARTE 15 — Publicado vs. local

Lo que alguien ve hoy en GitHub (`81bfad8`, release v0.6.0 + consola de operador sin etiquetar) vs. la máquina local:

| Capacidad | origin/master | Local `db45a7b` | Estado |
|---|---|---|---|
| Pipeline canales→router→brains→modelos→políticas | ✅ | ✅ | [PUBLISHED] |
| Korvun Desktop + builder canvas + persona | ✅ | ✅ | [PUBLISHED] (v0.4.0–v0.6.0) |
| Operator Console (Chat, takeover, sesiones) | ✅ (sin release) | ✅ + fixes E-13/E-14 | [PUBLISHED code / release pendiente] |
| Governed tools (gate tri-estado, 2 puertas) | ❌ | ✅ | [LOCAL — NO PUBLICADO] |
| Shadow / ensayo + promoción en caliente | ❌ | ✅ (demostrado) | [LOCAL] |
| `read_file` / `http_fetch` / `webhook_call` + jaulas + escudo SSRF | ❌ | ✅ | [LOCAL] |
| Auditoría de tools (3 superficies, ring, métricas) | ❌ | ✅ | [LOCAL] |
| `/tools` gatekeeper en consola | ❌ | ✅ | [LOCAL] |
| Skills markdown (AgentSkills) | ❌ | ✅ | [LOCAL] |
| Native tool calling (Ollama) + ValidateRequest por rol | ❌ | ✅ | [LOCAL] |
| Dedup inbound + fix canal de respuesta desconocido | ❌ | ✅ | [LOCAL] |
| Config estricta (unknown keys por nombre, schema_version) | ❌ | ✅ | [LOCAL] |
| Hot reload / promoción de config (POST /api/config + supervisor) | ✅ | ✅ (+decode estricto, +Start relee disco) | [PUBLISHED, endurecido en LOCAL] |
| Guardias de boot E-11/E-12 (tool sensible, ceiling del superviviente) | ❌ | ✅ | [LOCAL] |
| Builder governance panel | ❌ | ❌ | **[SPEC ONLY]** — FR-UI-1/SP6, gated a mockups |
| Groq native lane / streaming / política de coste / memoria mínima | ❌ | ❌ | [SPEC ONLY / roadmap] |

**Quien mire GitHub hoy se pierde el producto entero de la pieza de agentes**: para el mundo, Korvun es un gateway conversacional con builder visual y consola de operador; en la máquina local es además un runtime de agente con manos gobernadas, ensayo y auditoría. Nada se publica hasta cerrar la pieza (ley de los dos pasos; el panel SP6 es lo que falta del punto 7 de aceptación).

---

## PARTE 16 — Cambios no committeados

**Staged: ninguno. Unstaged (tracked): ninguno.** El lote local está íntegramente committeado. Untracked (inspeccionados uno a uno — **nada es código de producto**; no alteran ninguna conclusión del mapa):

| Entrada | Qué es | Efecto sobre el producto |
|---|---|---|
| `AGENTS.md` | Espejo de CLAUDE.md para Codex (reglas de operación; misma prohibición de atribución, adaptada) | Ninguno — tooling del flujo de copilotos |
| `.codex/` | Hooks PreToolUse de Codex (block-attribution, quality-gate) — nótese que apuntan a rutas `…/Desktop/korvun/` sin `.nosync` (posible desalineación de path del tooling, no del producto) | Ninguno |
| `.agents/`, `.claude/skills/`, `.github/skills/`, `.github/agents/`, `.github/hooks/` | Espejos de skills de diseño/IA (animate, impeccable, taste…) + agentes del plugin impeccable | Ninguno |
| `skills-lock.json` | Lockfile de esas skills (hashes por fuente) | Ninguno |
| `.playwright-mcp/` | Capturas/console-logs de las sesiones de QA del 2026-08-08/09 | Evidencia de las demos, no código |
| `BRIDGE-STATUS-2026-07-26.md` | Nota temporal del puente Claude Desktop; su propio texto pide borrarla y recuerda rotar un `github_pat_` (recado pendiente) | Ninguno |
| `Korvun — 2026-08-0*.md` (4) | Diario de sesiones estilo Obsidian (v0.6.0, web, chat, "las manos gobernadas") | Fuente narrativa; coherente con el código verificado |

También sin trackear por gitignore pero presentes: binario `korvun` compilado (2026-08-08), `coverage.out` (2026-08-15, consistente con `make quality` reciente), `graphify-out/` (grafo auto-regenerado post-commit).

---

## Executive architecture snapshot

Korvun (`github.com/Sebastian197/korvun`) es un monolito Go único — 6 deps directas, cgo-free salvo el shell Wails — que hace de gateway de mensajería con IA autoalojado. El árbol de verdad es el **master local `db45a7b` (2026-08-15)**, 48 commits fast-forward por delante del `origin/master` publicado (`81bfad8`); no hay staged/unstaged; los untracked son tooling de copilotos.

El pipeline: cuatro canales (Telegram, Discord, webhook genérico, console interno) validan en su borde y normalizan a un `Envelope` canónico; un `Router` con colas acotadas, dedup LRU+TTL [local] y sesiones estilo OpenClaw (corte duro `/new`, takeover de operador) enruta por tabla estática a uno de **dos** Brains: `Orchestrator` (fan-out paralelo o fail-over secuencial sobre varios modelos, reducido por prioridad o consenso, con la política de privacidad aplicada en boot: un brain private jamás ve un modelo cloud) o `AgentBrain` [local] (un modelo, loop de herramientas acotado). Providers: solo Ollama y Groq tras un seam `Model` con gramática de errores compartida, decorados por retry con backoff. Persistencia: SQLite (sesiones + turnos; solo el par final user+assistant — la traza de tools se descarta por ley). Observabilidad: slog + Prometheus + SSE, todo metadata-only y volátil.

La pieza local no publicada es el **runtime de agente gobernado** (ADR-0041/0042): 6 herramientas (3 puras, 3 enjauladas: `read_file` con jaula de directorio resuelta por symlinks, `http_fetch`/`webhook_call` con allow-list de hosts y escudo SSRF en `Dialer.Control` sobre la IP resuelta post-DNS, link-local excluido), gobernadas por `policy.SelectTools` — puro, tri-estado allow/shadow/deny, por tool×canal×sensibilidad, fail-closed — aplicado en dos puertas (anuncio y ejecución) que convergen, para el carril textual y el nativo de Ollama por igual, en un único `runTool`: **ese es hoy el punto exacto entre el modelo y el mundo real**. Shadow simula sin ejecutar; `/tools` imprime el informe del portero; la auditoría (bus/métricas/SSE + ring de 64) es operativa, sin contenido y 100 % volátil — no existe receipt persistente ni criptográfico. Skills markdown son documentación de prompt, jamás autorización. Todo se demostró en vivo en hardware real, y una tanda de red-team (E-1…E-14) endureció bordes concretos.

Lo que NO hay, verificado a texto completo: identidad de actor en las decisiones, intención, acción reificada, autorización por recurso expresable en política, aprobación pre-efecto, transacciones/compensación, receipts durables o firmados, MCP, A2A, multi-tenant, política remota. El único hueco de la pieza actual es el panel de gobernanza del Builder (spec aprobada, cero código, gated a mockups). Para evolucionar hacia una capa de autoridad de ejecución, las costuras correctas ya existen (`runTool` como cuello único, `SelectTools` puro, shadow, ring, supervisor); el salto es pasar la acción de "dos strings" a un objeto de primera clase con actor, alcance, expiración y prueba.

---

## Evidence index

| Afirmación | Fichero · símbolo | Estado |
|---|---|---|
| Divergencia 48 commits fast-forward | `git rev-list origin/master..master`; merge-base `81bfad8` | local |
| Solo 2 Brains | `internal/brain/orchestrator.go:22`, `agent.go:25` (`var _ Brain`) | Orchestrator published / AgentBrain local |
| Interface Brain | `internal/brain/brain.go:26-28` | published |
| Único llamador de Execute | `internal/brain/agent.go:512` (`runTool`) | local |
| Doble puerta | `agent.go:580-602` (anuncio) · `agent.go:472-539` (ejecución) | local |
| Shadow no ejecuta | `agent.go:487-492` + `shadowObservation` `:59-63` | local |
| Gobernanza antes del parseo (nativo) | `internal/brain/agent_native.go:112-120` | local (E-3) |
| Cap 8 calls/turno | `agent_native.go:34,95-111` | local (E-4/E-5) |
| Rescate de tool-call textual | `agent_native.go:216-247` | local |
| SelectTools y precedencia | `internal/policy/tools.go:121-168` (`decideTool`) | local |
| Fail-closed en error de gate | `agent.go:590-594` + `protocol.go:99-101` | local |
| ToolCallingModel + RoleTool | `internal/model/toolcalling.go:16,55-62` | local |
| Invariantes por rol | `internal/model/errors.go:101-143` (`ValidateRequest`) | local (E-10) |
| Propagación de capacidad | `internal/model/retry/retry.go:88-107` · `internal/brain/named.go:51-57` | local |
| Solo Ollama/Groq | `internal/app/app.go:1084-1112` (`buildModel`) | published |
| Groq sin carril nativo | ausencia de `GenerateWithTools` en `internal/model/groq/` | declarado follow-up en ADR-0042 §3 |
| Jaula read_file | `internal/tool/readfile.go:84-151` | local |
| Solo ficheros regulares | `readfile.go:125-139` | local (E-6) |
| Escudo dial-time | `internal/tool/caged.go:105-115` (`shieldControl`) · `httpfetch.go:151-173` | local |
| Link-local excluido | `caged.go:100-104` + `shieldcontrol_table_test.go` | local (E-8) |
| Armado del escudo en wiring | `internal/app/app.go:890`; `ToolDecision.Shield` sin consumidor runtime | local |
| Guardia tool sensible/cloud sin gobernar | `app.go:787-805` (`ErrSensitiveToolUngoverned`) | local (E-11) |
| Ceiling del superviviente del selector | `internal/app/ceiling.go:134-170` | local (E-12) |
| Ring de audit 64, sin persistencia | `internal/app/tools_report.go:17-29` | local |
| Evento sin campo de args | `internal/bus/bus.go:118-121` | local |
| Sin firma/hash de audit | grep `hmac|sign|sha256` en runtime: solo comparación de bearer (`controlapi/mutation.go:71-90`) | verificado por ausencia |
| Solo par final persistido | `agent.go:637-659` (`persistPair`) · `orchestrator.go:361-383` | local / published |
| Corte duro de sesión | `internal/conversation/conversation.go:158-172` · `router/session.go:271-294` | published |
| Dedup LRU+TTL con forget | `internal/router/dedup.go:32-126` · `router.go:287-341` | local (E-1) |
| Cap idempotency-key 128B | `internal/channel/webhook/webhook.go:143-198` | local (E-9) |
| Decode estricto config y mutación | `internal/config/config.go:463-476` · `controlapi/mutation.go:120-141` | local (A-1, E-2) |
| Start relee del disco | `internal/shell/controller.go:159-170` | local (`343da00`) |
| Bearer por ciclo, proxy inyecta | `controller.go:181-197,432-438` · `proxy.go:70-86` | published |
| Panel de gobernanza inexistente | `web/builder/src/config/schema.ts:23-27`; grep en frontend desktop | verificado por ausencia — SPEC ONLY (FR-UI-1) |
| Skills = documentación, no autorización | `internal/skill/skill.go:8-10,50-52`; `AllowedTools` sin consumidores | local |
| Skills budget y sufijo de prompt | `skill.go:207-244` · `agent.go:371-374` | local |
| Sender nunca input de autorización | grep `env.Sender` en no-test: 3 hits, ninguno en policy | verificado por ausencia |
| MCP/A2A/tenant/policy-remota ausentes | grep repo-wide en `.go`: cero hits de código | verificado por ausencia |
| Persistir audit trail rechazado | `docs/adr/0041-…md:301-302` | doc, local |
| Ask-mode/double-sign post-beta | `docs/superpowers/specs/2026-08-09-…md:423-427` | SPEC ONLY |
| Feed desktop ciego a tools | `frontend/src/feed/frame.ts:16-30`, `feed/store.ts:61-76` | local, hallazgo |
| outbound_token del webhook sin usar | `channel/webhook/webhook.go:64,114-141` | published, hallazgo |
| `/api/reload/{handle}` sin auth | `controlapi/mutation.go:47` | published, hallazgo |
