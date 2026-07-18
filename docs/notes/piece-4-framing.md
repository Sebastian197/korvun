# Pieza 4 — Tercer canal: Discord (encuadre)

> **Estado: ENCUADRE, pendiente de la revisión del copiloto.** Este documento NO es
> el ADR y NO autoriza una sola línea de código. Encuadra el canal Discord para que
> el copiloto vete la decisión técnica gorda (cliente WebSocket) y resuelva los
> `[NEEDS CLARIFICATION]` antes de escribir ADR + TDD.
>
> Método: office-hours (challenge de premisas + alternativas honestas + doc-only, sin
> código). La API de Discord se verificó con **Context7 sobre la doc oficial**
> (`/discord/discord-api-docs`) ANTES de fijar nada (regla CLAUDE.md).

## 0. Decisión de producto (de Chano, 2026-07-18) — ya tomada

El tercer canal es **Discord**. Descartados con razón registrada:

- **WhatsApp** — verificación de empresa + pago por mensaje = fricción inasumible
  para el onboarding self-hosted. Candidato futuro post-tracción. (ADR-0002 ya lo
  difirió; el doc maestro §9 lo llama "la más traicionera, opcional en beta".)
- **Slack** — desde 2026-03-03 limita apps fuera de su Marketplace a 1 req/min en
  lectura de conversaciones; términos hostiles a gateways de IA (OpenClaw afectado,
  verificado).
- **Discord (elegido)** — bot gratis en minutos; desde junio 2026 el Message Content
  intent se activa sin revisión por debajo de un umbral grande, y **cada usuario de
  Korvun corre SU propio bot**, así que nunca se acerca al umbral **por
  construcción**. (Umbral exacto → ver `[NC-1]`.)

El seam `channel.Channel` ya existe y lo validan Telegram + Webhook, así que Discord
es **un adaptador nuevo tras el mismo contrato**, no un cambio de arquitectura.

## 1. Realidad de la API de Discord (VERIFICADA con Context7)

### Recepción de mensajes libres = Gateway WebSocket, ÚNICO camino

**Confirmado en la doc oficial: los eventos `MESSAGE_CREATE` se entregan SOLO por el
Gateway. No existe un camino HTTP-only para leer mensajes libres entrantes.** (La REST
API sirve para *enviar* y para *leer historial de un canal que ya conoces*, no para
recibir en tiempo real lo que la gente escribe.) Esto obliga a hablar el Gateway.

Flujo del Gateway (opcodes verificados):

1. Conexión WSS a la Gateway URL (`GET /gateway/bot` da la URL + shards recomendados).
2. Recibir **Hello (op 10)** con `heartbeat_interval` (ms).
3. Enviar **Identify (op 2)**: `token`, `properties {os, browser, device}`, `intents`.
4. Recibir **Ready**: guarda `session_id` y `resume_gateway_url`.
5. Recibir **Dispatch (op 0)** con número de secuencia `s` (guardar el último).
6. Enviar **Heartbeat (op 1)** cada `heartbeat_interval`; recibir **Heartbeat ACK
   (op 11)**. Sin ACK → conexión zombi → reconectar.
7. **Reconnect (op 7)** / **Invalid Session (op 9, `d=true`)** / close-codes
   recuperables / desconexión sin close-code → **Resume (op 6)** con `session_id` +
   `seq` sobre `resume_gateway_url` (NO re-Identify). Si el resume falla → Identify
   nuevo desde cero.

**Intents** (bitfield; valores verificados). Para leer texto de canales de guild y de
DMs, Korvun necesita:

| Intent | Bit | Valor |
|---|---|---|
| `GUILDS` | `1<<0` | 1 |
| `GUILD_MESSAGES` | `1<<9` | 512 |
| `DIRECT_MESSAGES` | `1<<12` | 4096 |
| `MESSAGE_CONTENT` (**privilegiado**) | `1<<15` | 32768 |

Suma = **37377**. `MESSAGE_CONTENT` es uno de los **tres intents privilegiados**: se
habilita **manualmente en el Developer Portal** del bot y controla el acceso a los
campos de contenido (`content`, `embeds`, `attachments`) en TODOS los eventos.
**Política vigente (RESUELTO, `[NC-1]`):** el toggle self-serve del intent aguanta
hasta **10.000 USUARIOS** (anuncio oficial de Discord del **2026-06-11**), no 100
servidores — la doc que trajo Context7 va por detrás del anuncio. La **verificación de
bot a los 100 servidores sigue existiendo como proceso SEPARADO** (afecta a otras
capacidades del bot, no al toggle del intent). El modelo per-usuario de Korvun (cada
usuario corre SU propio bot) queda muy por debajo de **cualquiera** de los dos regímenes
→ **no bloqueante** por construcción. Fuentes:
`support-dev.discord.com/hc/en-us/articles/40281523410967` ·
`docs.discord.com/developers/gateway/getting-started-with-privileged-intent-review`.

### Envío = REST puro (HTTP)

`POST /channels/{channel.id}/messages`, body `{content (≤2000 chars), allowed_mentions,
embeds}`. Devuelve el message object y dispara un `MESSAGE_CREATE` en el Gateway. Es
HTTP `net/http` normal — **el envío NO necesita el WebSocket**.

**Rate limits (verificados):** un 429 trae header `Retry-After` + `retry_after` (float
segundos) en el JSON + `X-RateLimit-*` + `scope` (user/global/shared). Esto **mapea
directamente a la gramática `model.RateLimitError{Provider, RetryAfter}` que ya existe**
(la usan Groq y Ollama). Buckets por-ruta + límite global; el Gateway además tiene su
propio límite de envío (~120 eventos/60s) y de Identify.

## 2. LA decisión técnica gorda — el cliente WebSocket (para el VETO del copiloto)

**Go stdlib NO trae WebSocket.** Recibir en Discord obliga a un cliente WS (RFC 6455).
Dos caminos, presentados honestos:

### Opción (a) — cliente RFC 6455 propio, acotado (solo cliente)

Implementar handshake HTTP Upgrade (`Sec-WebSocket-Key`/`Accept`), framing (FIN,
opcode, **masking obligatorio del lado cliente**, longitudes 7/16/64-bit), frames de
control (ping/pong/close), fragmentación, validación UTF-8, y el close handshake.

- ✅ **Cero dependencias nuevas** — `go.mod` se queda en 3 deps directas.
- ✅ Control total, sin superficie de supply-chain, en la línea del **ethos hecho a
  mano de los adapters Ollama/Groq** (*honestidad: Telegram NO es hecho a mano — usa
  `go-telegram/bot`, una de las 3 deps; el precedente hand-rolled son los adapters de
  modelo sobre `net/http`, no un WebSocket*).
- ❌ RFC 6455 es un **protocolo de verdad**, no un GET: ~500–1000 LOC sensibles a
  seguridad (el masking cliente es obligatorio; un framing mal hecho = bugs sutiles).
- ❌ Reinventar una rueda resuelta y bien testeada; mantenimiento continuo.
- ❌ **El cliente WS NO es el valor de Korvun** (el diferencial es el motor de
  políticas). Alto esfuerzo en código commodity.

### Opción (b) — 4ª dependencia directa: `coder/websocket`

- ✅ **Cero deps transitivas** (verificado: "zero dependencies") — no arrastra árbol.
- ✅ Minimalista, idiomático, **context-first** (encaja con la disciplina de `context`
  de Korvun); `Dial(ctx,...)`, `wsjson.Read/Write`, `Ping(ctx)`, `Close(status,...)`,
  `CloseError` con `errors.As`. Pure-Go, CGO-free → cruza el eje cross-compile.
- ✅ Reputado, muy usado, mantenido; alto benchmark en Context7 (89.85).
- ✅ Libera el esfuerzo para la lógica **Discord-específica** (protocolo Gateway,
  mapeo a Envelope), que sí es el trabajo de valor.
- ❌ **Rompe la racha de 3 deps** → 4ª dep directa. (Mitigado: la regla del maestro es
  "stdlib **si es razonable**", y un cliente WS **no es razonablemente stdlib** — Go no
  lo trae.)
- ❌ Superficie de supply-chain (mitigada: cero transitivas, reputada); exige ADR de
  dependencia + pasar el **test de 4 ejes** del proyecto.

**Recomendación (para el veto, NO decidida aquí): (b) `coder/websocket`.** Un cliente
WS no es razonablemente stdlib (Go carece de él) y es código commodity sensible a
seguridad que **no es el valor del producto**; `coder/websocket` es cero-transitivas +
context-idiomático y cruza limpio el gate de dependencias (ADR justificativo + Context7
+ 4 ejes). Hand-rollear RFC 6455 gastaría el esfuerzo en la rueda equivocada. **Pero el
copiloto decide** (ver `[NC-2]`).

## 3. Contrato del canal (tras `channel.Channel`)

- **`type: "discord"`, `mode: "gateway"`** (único modo v1; el envío REST no es un modo
  aparte, viaja dentro del mismo adaptador).
- **Secreto env-only:** `token_env` con el NOMBRE de la variable de entorno del bot
  token (patrón ADR-0010: nunca argv, config, logs ni errores). Fatal-loud si falta.
- **Mapeo evento → Envelope:** de `MESSAGE_CREATE` (guild channels + DMs) →
  `channel="discord"`, `conversation.id = channel.id` de Discord (para keyear la
  memoria de conversación, como Telegram), autor (id + display name), texto
  (`content`). **Ignorar los mensajes del propio bot** (evitar bucles). **Adjuntos
  fuera de v1** (igual que Telegram v1). Validación de entrada en el borde del canal
  (regla de seguridad del proyecto).
- **Outbound:** `POST /channels/{id}/messages` sobre `net/http`; **429 →
  `RateLimitError` con `RetryAfter`** reutilizando la gramática existente; respetar el
  bucket global. `content` troceado/acotado a 2000 chars.
- **Lifecycle:** `Start`/`Stop` limpio (goroutine del Gateway con su ctx; heartbeat;
  resume/reconnect con backoff; cierre WS ordenado en Stop). **`DroppedCount`** atómico
  como Telegram. `Receive(ctx) <-chan *envelope.Envelope`, `Send(ctx, env)`, `Name()`,
  `Manifest()` — el seam ya existente.
- **Seguridad de salida:** `allowed_mentions` por defecto a **ninguna mención**
  (`parse: []`), para que la salida del modelo NO pueda pingear `@everyone`/`@here`/
  roles. (Decisión de seguridad; ver `[NC-4]`.)

## 4. Alcance v1 honesto — y qué queda FUERA

**Dentro de v1:** recibir texto de canales de guild + DMs vía Gateway (`MESSAGE_CREATE`
con `MESSAGE_CONTENT`); responder texto vía REST `createMessage`; lifecycle del Gateway
(identify, heartbeat, resume/reconnect); manejo de 429/Retry-After; `DroppedCount`;
Start/Stop limpio; validación de entrada.

**Explícitamente FUERA de v1** (cada uno es alcance futuro, no un olvido):

- Threads, foros, canales de voz.
- Slash commands / interactions (requieren endpoint de interactions + firma Ed25519).
- Embeds ricos, botones/components, selects, modals.
- Adjuntos/media (in y out) — igual que Telegram v1.
- Reactions, ediciones, borrados de mensajes.
- **Sharding** — solo obligatorio a partir de ~2500 guilds; el modelo per-usuario-bot
  nunca se acerca, así que v1 es single-shard por construcción.
- Presence / listas de miembros de guild (intents privilegiados que Korvun no necesita).
- Compresión del Gateway (`zlib-stream`/`zstd`) — opcional, diferida.

## 5. Riesgos y mitigaciones

- **Resume/reconnect del Gateway** (el riesgo principal). Mitigación: máquina de estado
  con `session_id` + `seq` + `resume_gateway_url`; manejar op 7 / op 9(`d=true`) /
  close-codes recuperables vs no-recuperables; fallback a Identify si el resume falla.
- **Conexión zombi / heartbeat.** Enviar Heartbeat cada `heartbeat_interval`; si no
  llega el ACK (op 11) antes del siguiente ciclo, forzar reconexión. Un jitter en el
  primer heartbeat (recomendado por Discord) evita thundering-herd.
- **Rate limits.** REST 429 → `RateLimitError`/`Retry-After` (ya existe). Gateway:
  respetar el límite de envío (~120/60s) y de Identify; backoff exponencial en los
  reconnect para no entrar en tormenta.
- **Cambios de política de Discord** (umbral del Message Content intent). Mitigación
  estructural: **cada usuario corre su propio bot**, siempre por debajo de cualquier
  umbral; documentar el paso manual de habilitar el intent en el Developer Portal (como
  el paso de BotFather en Telegram).
- **Seguridad: menciones.** `allowed_mentions.parse=[]` por defecto (arriba) impide que
  el texto del modelo dispare pings masivos.
- **Habilitar el intent privilegiado es un paso MANUAL del operador.** Documentarlo en
  el setup del canal (no es código; es doc, como el token de BotFather).

## 6. Naturaleza y salida esperada

- **Naturaleza:** código Go + **1 ADR de canal** (extiende la línea de ADR-0002;
  proveedor, modo gateway, contrato de secreto, y — si (b) — el ADR de dependencia de
  `coder/websocket` con el test de 4 ejes) + TDD como Telegram/Webhook. Context7 del
  cliente WS elegido antes de una línea (ya hecho para el candidato).
- **Validación de "hecho":** un mensaje real entra por Discord, hace el round-trip
  completo (canal → router → brain → política → canal) por el binario real, y su
  lifecycle (arranque/paro limpio, drops contados, resume tras corte) pasa `-race`. **No
  bloquea la beta** (Pieza 4 es más alcance, no criterio V1 — los 6 ya están cerrados).

## 7. `[NEEDS CLARIFICATION]` — TODOS RESUELTOS (2026-07-18)

- **`[NC-1]` Umbral del Message Content — RESUELTO.** Política vigente: **10.000
  usuarios** (anuncio oficial 2026-06-11; la doc de Context7 iba por detrás). La
  verificación de bot a 100 servidores es un proceso SEPARADO. No bloqueante bajo
  cualquiera de los dos regímenes (ver §1). Corregido arriba.
- **`[NC-2]` Cliente WS — RESUELTO (decisión de Chano): 4ª dependencia,
  `coder/websocket`.** La opción (a) hand-rolled queda descartada. → **ADR-0034**.
- **`[NC-3]` DMs — RESUELTO: INCLUIDOS en v1** (intent `DIRECT_MESSAGES` 4096; ya en la
  suma 37377). → **ADR-0033**.
- **`[NC-4]` `allowed_mentions` — RESUELTO: ninguna mención por defecto (`parse: []`)**.
  → **ADR-0033** (§seguridad).
- **`[NC-5]` `mode` — RESUELTO: `"gateway"`** (literal del contrato de config). →
  **ADR-0033**.

---

**Estado:** encuadre cerrado, los 5 `[NC]` resueltos. Escritos **ADR-0033** (canal
Discord) y **ADR-0034** (dependencia `coder/websocket`), ambos `status: proposed`
pendientes del flip a `accepted` por el copiloto antes del TDD. `go.mod` sin tocar
(la 4ª dep entra con SP1).

## 8. Desglose TDD propuesto (para el visto del copiloto)

Una sub-fase a la vez, rojo→verde, `/review`, `make quality` verde `-race` antes de
cerrar. Sin proveedor real: un **fake Gateway** (servidor WS sobre `httptest`) +
`httptest` para el REST simulan identify/heartbeat/resume/dispatch/429.

- **SP1 — Dependencia + config + esqueleto.** `go get github.com/coder/websocket@v1.8.15`
  (la 4ª dep entra AQUÍ, ADR-0034); paquete `internal/channel/discord`; parseo de config
  `type:"discord"`/`mode:"gateway"`/`token_env`; validación (falta token_env → fatal);
  seam `channel.Channel` (`Name`/`Manifest`/`Mode`). Rojo: config + construcción.
- **SP2 — Mapeo inbound (puro).** `MESSAGE_CREATE` JSON → Envelope (guild + DM,
  `conversation.id = channel.id`, autor, `content`); ignorar mensajes del propio bot;
  validación de entrada en el borde. Función pura, table-driven, sin red.
- **SP3 — Lifecycle del Gateway.** Máquina de estado sobre `coder/websocket`: identify,
  heartbeat + ACK/detección de zombi, dispatch → canal inbound, `Receive`. Fake Gateway;
  `-race` sobre `Start`/`Stop` + `DroppedCount`.
- **SP4 — Resume/reconnect.** `session_id` + `seq` + `resume_gateway_url`; op7/op9/close-
  codes; backoff; fallback a Identify. Fake Gateway que fuerza corte y resume.
- **SP5 — Outbound REST.** `Send` → `POST /channels/{id}/messages` sobre `net/http`;
  `content` ≤2000; `allowed_mentions:[]` por defecto; 429 → `RateLimitError`/`Retry-After`.
  `httptest` (200 + 429).
- **SP6 — Wiring + docs + validación en vivo.** Montar el canal en `app` (config →
  adaptador); doc de setup (el paso MANUAL del Message Content intent, como BotFather);
  config de ejemplo; **round-trip real de Discord validado en hardware**.

Notas: SP1 es la ÚNICA sub-fase que toca `go.mod`. SP4 puede fusionarse con SP3 si el
copiloto lo prefiere (misma máquina de estado); se dejan separadas por blast-radius
(el resume es la parte con más aristas).
