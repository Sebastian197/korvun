# Referencia de configuración

Korvun lee **un fichero JSON**, pasado con `--config` (por defecto
`korvun.json`):

```sh
korvun serve --config /etc/korvun/korvun.json
```

La forma de los campos es un **contrato**: una vez escrita una
configuración, los nombres y la estructura son estables entre releases.
Valida cualquier fichero sin conexión con `korvun config check <file>`
(añade `--preflight` para resolver además los secretos y alcanzar los
proveedores).

> **Los secretos son variables de entorno, por NOMBRE — nunca por valor.**
> Los campos que terminan en `_env` (`token_env`, `api_key_env`) guardan el
> **nombre** de una variable de entorno; Korvun lee el valor al arrancar.
> Un secreto nunca se lee de la línea de comandos, del fichero de
> configuración, de logs ni de mensajes de error. Un secreto ausente es un
> error de arranque claro y con nombre.

## Nivel superior

| Campo | Tipo | Obligatorio | Significado |
|---|---|---|---|
| `channels` | array | **sí** (≥1) | Canales de mensajería a arrancar. |
| `brains` | array | **sí** (≥1) | Cerebros orquestadores. |
| `routes` | array | **sí** (≥1) | Vínculos de un canal con un cerebro. |
| `storage` | objeto | no | Almacén durable de conversaciones. **Ausente ⇒ sin estado.** |
| `observability` | objeto | no | Servidor HTTP de administración. **Ausente ⇒ ACTIVO (loopback).** |
| `admin` | objeto | no | La superficie de escritura + el builder. **Ausente ⇒ solo lectura.** |

La asimetría es deliberada: sin `storage` significa *apagado* (sin estado),
sin `observability` significa *encendido* con valores loopback seguros, sin
`admin` significa *solo lectura* — cada uno el valor seguro.

## `channels[]`

| Campo | Tipo | Obligatorio | Valores / significado |
|---|---|---|---|
| `type` | string | **sí** | `telegram`, `discord` o `webhook`. |
| `mode` | string | condicional | `telegram` → `polling`; `discord` → `gateway`; **`webhook` no lleva `mode`**. |
| `token_env` | string | **sí** | **Nombre** de la variable de entorno con el secreto del canal (token del bot, o el secreto Bearer de entrada del webhook). |
| `webhook` | objeto | solo webhook | El bloque webhook (abajo). |

Un canal se registra con su `type` como nombre — ese es el valor que
referencian las `routes`.

```json
{ "type": "telegram", "mode": "polling", "token_env": "TELEGRAM_BOT_TOKEN" }
```

```json
{ "type": "discord", "mode": "gateway", "token_env": "DISCORD_BOT_TOKEN" }
```

Discord necesita un interruptor manual en el Developer Portal — la
[guía de Discord](/channels/discord) lo recorre paso a paso.

### El bloque `webhook`

| Campo | Tipo | Obligatorio | Valores / significado |
|---|---|---|---|
| `bind` | string | no | Dirección de escucha. Por defecto **`127.0.0.1:8090`** (loopback). Un bind no-loopback avisa al arrancar. |
| `path` | string | no | Ruta del POST de entrada. Por defecto **`/webhook`**. |
| `outbound_url` | string | **sí** | Adónde se envían (POST) las respuestas del cerebro. |
| `outbound_token_env` | string | no | **Nombre** de la variable con un secreto Bearer de salida. Si se nombra, debe resolver al arrancar. |
| `mapping` | objeto | no | Tus nombres de campo JSON → los campos de mensaje de Korvun. |

Validación en el borde en cada petición: solo `POST` (405),
`application/json` (415), cuerpo ≤ 1 MiB (413), buffer lleno responde 503
(reintenta luego). La [guía del webhook](/channels/webhook) es el camino
de cero al round-trip.

## `brains[]`

| Campo | Tipo | Obligatorio | Valores / significado |
|---|---|---|---|
| `name` | string | **sí** | Nombre único, referenciado por `routes`. |
| `sensitivity` | string | **sí** | `public` \| `private`. **`private` descarta los modelos cloud antes del despacho** — lo sensible nunca sale de tu máquina. |
| `dispatch` | string | no | `fanout` (por defecto: todos los modelos en paralelo) \| `sequential` (en orden, para en el primer éxito — un proveedor de pago solo se contacta si el local falló). |
| `policy` | objeto | **sí** | El reductor que elige la respuesta (abajo). |
| `models` | array | **sí** (≥1) | El catálogo de proveedores de este cerebro. |
| `agent` | objeto | no | Monta un agente acotado con herramientas en vez del orquestador por defecto. |

### `brains[].policy`

| Campo | Tipo | Obligatorio | Valores / significado |
|---|---|---|---|
| `kind` | string | **sí** | `priority` \| `consensus`. |
| `order` | array | no | Lista de prioridad de proveedores que usan ambos reductores. |

- **`priority`** — la respuesta del proveedor de mayor prioridad que
  contestó, según `order`.
- **`consensus`** — la respuesta en la que coincide una mayoría estricta de
  los proveedores que contestaron (mínimo dos; un empate o un único éxito ⇒
  sin consenso).

### `brains[].models[]`

| Campo | Tipo | Obligatorio | Valores / significado |
|---|---|---|---|
| `provider` | string | **sí** | `ollama` \| `groq`. |
| `model_id` | string | **sí** | El nombre del modelo en el proveedor (p. ej. `llama3.2`). |
| `locality` | string | **sí** | `local` \| `cloud` — **declarado**, no derivado; el selector de privacidad enruta sobre él. |
| `base_url` | string | no | Sobrescribe el valor por defecto del adaptador (Ollama: `http://127.0.0.1:11434`). |
| `api_key_env` | string | solo cloud | **Nombre** de la variable con la clave de API. **Obligatorio para `groq`.** |

### `brains[].agent` (opcional)

Presente ⇒ el cerebro es un agente acotado con herramientas.

| Campo | Tipo | Obligatorio | Significado |
|---|---|---|---|
| `tools` | array | **sí** (≥1) | Herramientas integradas a registrar. Puras: `time`, `echo`, `calc`. Enjauladas (cada una EXIGE su bloque de jaula): `read_file`, `http_fetch`, `webhook_call`, `memory_note`. |
| `max_iterations` | int | no | Tope duro del bucle. |
| `system_prompt` | string | no | Prompt del operador añadido tras el bloque de protocolo. |
| `governance` | array | no | Permisos tri-estado — `tool`, `mode` (`allow` \| `shadow` \| `deny`), `channels` opcional. Ausente ⇒ sin gobernar: toda herramienta listada, permitida en todos los canales. |
| `read_file` | object | con la herramienta | La jaula: `root` (**obligatorio**), `max_bytes`. |
| `http_fetch` | object | con la herramienta | La jaula: `allow_hosts` (**obligatorio**), `max_bytes`, `max_redirects`. |
| `webhook_call` | object | con la herramienta | La jaula: `allow_hosts` (**obligatorio**), `max_bytes`, `timeout_seconds`. |
| `memory` | object | con la herramienta | El bloque de memoria de `memory_note`: `scope` (`conversation` por defecto \| `brain`), `max_notes`, `max_note_runes`, `budget_runes`. Requiere `storage`; `scope: "brain"` exige que el modelo seleccionado del cerebro sea **local**. |
| `skills_dir` | string | no | Directorio de skills compatible con AgentSkills. |
| `skills_body_budget` | int | no | Presupuesto total en runas para los cuerpos de skills inyectados. |

La guía operativa completa — el alcance de cada herramienta, el ensayo en
shadow, el escudo de red, `/tools`, cómo escribir una skill — está en
[herramientas y skills gobernadas](/guide/tools-and-skills); las notas y el
recall, en [memoria gobernada](/guide/memory).

## `routes[]`

| Campo | Tipo | Obligatorio | Significado |
|---|---|---|---|
| `channel` | string | **sí** | El nombre de tipo de un canal configurado. |
| `brain` | string | **sí** | El nombre de un cerebro configurado. |

```json
{ "channel": "telegram", "brain": "assistant" }
```

## `storage` (opcional)

| Campo | Tipo | Obligatorio | Significado |
|---|---|---|---|
| `path` | string | no | Fichero SQLite. Vacío ⇒ `<os user config dir>/korvun/korvun.db`. |

Presente ⇒ memoria durable por conversación que sobrevive a reinicios.
Ausente ⇒ sin estado. Bajo la unidad systemd endurecida, usa
`/var/lib/korvun/korvun.db`.

## `session` (opcional)

| Campo | Tipo | Obligatorio | Significado |
|---|---|---|---|
| `triggers` | array | no | Comandos de reinicio por primer token exacto. Omitido ⇒ `["/new", "/reset"]`. |
| `daily_at` | string | no | Frontera `"HH:MM"` en hora local para la expiración diaria. Vacía ⇒ ninguna. |
| `idle_min` | int | no | Expiración por inactividad en minutos enteros. `0` ⇒ ninguna. |
| `recall_max` | int | no | Habilita `/recall` (`0` ⇒ deshabilitado; 1..50): importa la cola de la sesión anterior como UN bloque citado — solo en una sesión vacía, solo bajo demanda. |

Presente ⇒ el despacho por sesiones está activo: una conversación es una
serie de sesiones, la más nueva activa, y un disparador (o una expiración
perezosa diaria/por inactividad) corta el contexto en seco. **Requiere el
bloque `storage`.** Ausente ⇒ ningún comportamiento de sesión. Un bloque
vacío (`"session": {}`) habilita los disparadores por defecto sin
expiración automática — es lo que aprovisiona el escritorio. La vista del
operador sobre sesiones, `/recall` y `/notes` está en
[la consola del operador](/guide/chat).

## `observability` (opcional)

| Campo | Tipo | Obligatorio | Significado |
|---|---|---|---|
| `enabled` | bool | no | Sin definir ⇒ `true`. |
| `addr` | string | no | Dirección de escucha. Vacía ⇒ `127.0.0.1:2112`. |

El servidor de administración expone `/metrics` (Prometheus), `/healthz`,
la API de control de solo lectura y la vista en vivo en `/ui`. Escucha en
**loopback** por defecto, así que un arranque recién hecho no expone nada a
la red; escuchar en `0.0.0.0` es una decisión consciente que pone
auth/TLS/cortafuegos de tu lado.

## `admin` (opcional)

| Campo | Tipo | Obligatorio | Significado |
|---|---|---|---|
| `token_env` | string | **sí** (si el bloque está) | **Nombre** de la variable con el token bearer de administración. |

El bloque `admin` enciende la superficie de escritura y el
[builder visual](/guide/builder) en `/builder`. Sin bloque, o con la
variable sin definir ⇒ solo lectura: el builder ni se monta. El token viaja
como `Authorization: Bearer` (comparación en tiempo constante, nunca una
cookie) y solo es seguro sobre el bind loopback por defecto o tras TLS.

```json
{ "admin": { "token_env": "KORVUN_ADMIN_TOKEN" } }
```
