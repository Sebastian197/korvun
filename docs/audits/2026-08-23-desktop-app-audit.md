> Promoted from the session's working copy (design-drafts/, git-ignored). The
> screenshots it cites are local session artifacts on the audit machine.

# Auditoría de uso — Korvun Desktop v0.9.0 (app empaquetada, WKWebView de macOS 13)

**Fecha:** 2026-08-23 · **Máquina:** iMac 2017 Intel, macOS 13.7.8, Retina 5K
**Método:** app oficial de la release instalada hoy, usada como usuario real por control de escritorio, contra un **perfil sandbox** (HOME aislado en scratchpad; el perfil real ni se miró ni se tocó — verificado por mtimes al cierre). Modelo real: **Qwen3-4B-Instruct-2507 Q4_K_M** servido por llama.cpp (`llama-server` build 10588) en `127.0.0.1:8189/v1` vía la pasarela `openai-compatible` — el GGUF ya estaba en la caché de llmfit, **cero descarga**. READ-ONLY: ningún fix aplicado, ningún commit.
**Capturas:** `design-drafts/builder-audit/` (numeradas, se citan por nombre).

**Config sandbox:** canal `console` + brain `sandbox-brain` (priority/sequential) + modelo `openai-compatible/qwen3-4b-instruct-2507` + bloque `admin` con `KORVUN_SANDBOX_ADMIN_TOKEN`. Nota metodológica: la config real de Chano **no tiene bloque `admin`** — ver hallazgo A4, porque cambia lo que él verá.

---

## Síntoma 1 — "Paste to load the raw config" al entrar al Builder

**Reproducción (capturas 04 y 05):**
1. Gateway en marcha (core vivo, `/healthz OK`).
2. Pestaña Builder → aparece la vista de brains/channels y el panel CONFIG pidiendo "ADMIN BEARER TOKEN — paste to load the raw config". Reproducido a la primera, idéntico a lo que Chano describe.
3. Pegué **la letra "x"** y Load → **el canvas cargó completo**. La puerta acepta cualquier cosa.
4. Cambié de pestaña y volví al Builder → **la puerta reaparece** (captura 17). "Cada vez que entra", confirmado.

**Causa raíz, con la cadena entera:**
- El Builder del desktop es el `web/builder` embebido en un iframe same-origin (`cmd/korvun-desktop/frontend/src/views/BuilderEmbed.tsx:28`, `src="/builder/"`).
- Ese frontend tiene una puerta de token pensada para el uso navegador-contra-gateway-remoto: no llama a `getConfig` hasta que el usuario teclea algo (`web/builder/src/App.tsx:38-51`; el placeholder confuso está en `App.tsx:154`).
- Pero en el desktop, el proxy del shell **sobrescribe** el header Authorization con el bearer real del ciclo (`internal/shell/proxy.go:76-77`, ADR-0035 §4: el token nunca entra al DOM). El usuario **no puede** conocer el bearer real (se genera con crypto/rand en cada arranque, `controller.go:432-438`) y **no le hace falta**: cualquier basura abre la puerta.
- Reaparece en cada entrada porque la pestaña renderiza condicionalmente (`App.tsx:139` del desktop: `{view === 'builder' && <BuilderEmbed />}`) → el iframe se **desmonta** al salir y pierde el token en memoria y todo el estado del canvas.

**No es carrera de arranque.** Con el core parado, el Builder muestra un estado honesto correcto ("El Builder necesita el gateway en marcha", captura 02). La puerta aparece precisamente con el core VIVO. En el iMac 2017 no observé ninguna ventana de carrera frontend-antes-que-admin.

**Clasificación:** hueco de diseño (integración SP6c incompleta: el gate del builder web nunca se adaptó al modo embebido).
**Fix mínimo:** en `web/builder/src/App.tsx`, si `window.self !== window.top` (el flag `embedded` que YA existe en la línea 56), saltarse la puerta auto-cargando con un token dummy (el proxy pone el real). **Tamaño: XS** (unas ~5 líneas + test).
**Extra recomendado:** el placeholder "paste to load the raw config" se lee como "pega la config" — cambiarlo a algo tipo "paste the admin bearer token" incluso para el modo navegador. XS.

---

## Síntoma 2 — Arrastrar un modelo de la paleta al lienzo no funciona

**Reproducción (capturas 06, 07, 08):**
1. Arrastre real (gesto nativo de ratón) de **brain** paleta→lienzo: **FUNCIONA** — nodo "(unnamed)" creado, "1 cambio sin aplicar" (captura 06). El HTML5 drag-and-drop SÍ inicia y suelta en este WKWebView de macOS 13.
2. Arrastre de **model** paleta→**encima del nodo brain**: **FUNCIONA** — modelo nuevo enganchado al brain (captura 07).
3. Arrastre de **model** paleta→**lienzo vacío** (el gesto natural de cualquier usuario): **NADA**. Sin nodo, sin mensaje, sin animación de rechazo. Silencio absoluto (captura 08).

**Causa raíz:** decisión de diseño NC-6 ("un modelo solo existe dentro de un brain"): `onSurfaceDrop` ignora el bloque `model` a propósito (`web/builder/src/canvas/CanvasView.tsx:555-562`); el drop válido de un modelo es solo sobre un `BrainNode` (`CanvasView.tsx:139-146`). El problema es que la regla es **invisible**: el drop inválido no da NINGÚN feedback, y la pista de la paleta dice literalmente "**Arrastra un bloque al lienzo** y conéctalo" (`CanvasView.tsx:619`) — instrucción incorrecta para el bloque model.

**Contraste con dev y e2e:** el e2e (`web/builder/e2e-binary/canvas.spec.ts:28-36`) usa `page.dragAndDrop()` de Playwright — eventos DnD **sintetizados por CDP en Chromium**; nunca se ha probado el inicio de una sesión de drag nativa en el WKWebView empaquetado. En esta auditoría el drag nativo funcionó, así que el motor no es el culpable del síntoma — pero el hueco de cobertura existe y conviene apuntarlo en la deuda de SP8 (validación en hardware).

**Clasificación:** nunca-funcionó-así-por-diseño + hueco de descubribilidad (feedback cero + microcopy engañosa).
**Fix mínimo:** (a) en `onSurfaceDrop`, cuando el bloque sea `model`, mostrar un aviso no-modal ("Suelta el modelo sobre un cerebro"); (b) corregir la pista de la paleta; (c) opcional: resaltar los brains como drop-targets al arrastrar un model (CSS sobre dragover). **Tamaño: S** (a+b son ~15 líneas; c algo más).

---

## Síntoma 3 — El panel de propiedades de un nodo no se puede cerrar

**Reproducción (captura 09 + comprobaciones):**
1. Clic en el nodo `sandbox-brain` → panel de propiedades abierto a la derecha.
2. **No hay botón X** en el panel.
3. **Escape no cierra** (verificado).
4. **Clic fuera (en el lienzo vacío) no cierra** (verificado).
5. Solo se cierra: seleccionando OTRO nodo (cambia el contenido, no cierra) o con "Eliminar nodo…" (destructivo).

**Causa raíz:** `selected` solo cambia en `onNodeClick` (`web/builder/src/canvas/CanvasView.tsx:634`) y en `onDeleted` (`:648`). No hay `onPaneClick`, no hay listener de Escape, y `PropertiesPanel` (`:400-476`) no renderiza ningún control de cierre.
**Clasificación:** hueco de diseño — nunca funcionó en ningún motor (no es cosa del WebView).
**Fix mínimo:** `onPaneClick={() => setSelected(null)}` en el ReactFlow + botón × en el header del panel + keydown Escape. **Tamaño: XS-S** (~20 líneas + tests).

---

## Exploración libre — todo lo demás, priorizado

### A. Rompe-demo (arreglar antes del martes → v0.9.1)

- **A1. El Builder miente sobre la pasarela openai-compatible y puede corromperla** (captura 10). El panel del modelo `openai-compatible/qwen3-4b-instruct-2507` muestra **PROVIDER: ollama** (el `SelectField` cae al primer valor porque `PROVIDERS = ['ollama','groq']` — `web/builder/src/config/schema.ts:156` — no incluye `openai-compatible`), **no hay campo base_url**, y crear un modelo desde la paleta solo puede producir ollama/groq (`newModel()`). La feature estrella de la v0.9.0 (ADR-0044) es **invisible e ineditable en el builder, y editable-a-peor**: tocar el select de provider convertiría el modelo en `ollama` perdiendo el `base_url`. Fix: añadir `openai-compatible` a PROVIDERS + campo base_url condicional en el panel. **Tamaño: S-M.** Prioridad máxima si la demo enseña la pasarela en el builder.
- **A2. Síntoma 1** (la puerta-teatro del token): primera impresión desastrosa y es un XS. Entra sí o sí.
- **A3. Síntoma 2** (feedback del drop de modelos + pista corregida): si la demo incluye montar un brain en directo, el silencio del drop es una trampa en escenario. S.
- **A4. La config real de Chano dejará el Builder ROTO cuando su core arranque.** Sin bloque `admin`, el core **ni monta** `/api/config` **ni monta** `/builder` (`internal/app/app.go:402-408`: "no token => mutation not mounted"; el propio comentario dice que un builder cuyo Save haría 404 es una trampa — pero el desktop lo embebe incondicionalmente). Su iframe recibirá un 404 del proxy. La config que el wizard del desktop genera debe incluir SIEMPRE un bloque `admin` (el shell ya genera y rota el bearer solo — `controller.go:184-197` — pero únicamente si el bloque existe). Fix: wizard/firstrun añade `admin.token_env` por defecto, o el shell inyecta el bloque como inyecta la dirección efímera. **Tamaño: S.** (Nota: en el sandbox SÍ pusimos admin, por eso todo funcionó.)

### B. Molesto (candidatos a v0.9.1 si caben)

- **B1. Panel sin cierre** (síntoma 3) — XS-S, entra fácil.
- **B2. El iframe del Builder ignora el tema claro** (captura 17): shell claro con un rectángulo negro dentro. `CanvasView` lee `document.documentElement.dataset.theme` de SU documento (`CanvasView.tsx:636`), que el shell nunca estampa en el iframe. Fix: pasar el tema por query param al iframe o postMessage. S.
- **B3. Sin log en fichero del desktop.** Lanzada desde Finder, la app pierde TODO el stderr (cero bytes hasta que arranca el core, y aun así solo si la lanzas desde terminal). Ya nos costó el diagnóstico a ciegas del token de Telegram esta mañana. Un sink `slog` a `~/Library/Logs/Korvun/` es operacionalmente urgente aunque no salga en la demo. S.
- **B4. Autostart del gateway OFF por defecto** (`autostart.ts:9-14`, localStorage, default false): la app SIEMPRE abre con "El gateway está detenido" hasta que el usuario descubra el toggle en Ajustes (captura 15). Defendible como default conservador, pero para el usuario del martes "abro la app y el bot no contesta" es un susto. Decisión de producto: o default ON, o un empujón visual al toggle desde Inicio. XS (si es solo el default).
- **B5. Anomalía en log:** una línea `ERR | Blocked request from not main frame` (runtime Wails) apareció durante el uso del Builder embebido (~07:08, cerca del "Aplicar cambios"). El POST de config funcionó (la validación del core volvió y se pintó), así que bloqueó OTRA petición del iframe. Sin causa determinada — merece 30 min con la app lanzada desde terminal y el runtime en debug antes de descartarla.

### C. Cosmético (puede esperar)

- **C1. Mezcla de idiomas por todas partes:** shell en español ("Aplicar cambios", "Arrastra un bloque…") con builder e hijos en inglés ("BRAINS", "CHANNELS", "paste to load…", "Held in memory only…"), Chat entero en inglés ("New chat", "No conversations yet", "Take over", "Delete conversation"), y el remate "**2 cambios sin aplicar · unsaved changes**" — el mismo dato dos veces en dos idiomas (capturas 05-08).
- **C2. Chat:** la sesión recién creada no aparece en la lista de conversaciones hasta el primer mensaje — coexisten "No conversations yet." y una sesión abierta a la derecha (captura de las 07:11).
- **C3. Feed:** horas sin cero inicial ("7:12:50") y cabeceras es/en mezcladas también ahí (HORA/EVENTO vs "received/sent/dropped/failed" en la leyenda) (captura 14).
- **C4. Ajustes:** la ruta del fichero de config se trunca con "…" sin tooltip ni botón copiar (captura 15) — el botón Copiar existe para el panel admin pero no para la ruta.
- **C5. Ventana pequeña (880×620):** el layout aguanta (todo apila, captura 18); solo se corta la microcopy inferior del CONFIG. Correcto en general.

### Lo que está BIEN (que también es dato)

- Estados honestos del core parado en todas las vistas (Inicio con CTA claro, Builder con "necesita el gateway", Chat/Canales vacíos sin mentir).
- **La pasarela openai-compatible funciona end-to-end en la app**: chat real contra Qwen3-4B local vía llama.cpp — pregunta → respuesta correcta en pantalla.
- Feed de Actividad en vivo, preciso al segundo (recibido 7:12:44 → respuesta 7:12:50).
- El error de validación del core llega al builder y se pinta legible (captura 11: `brains[0].models[1].model_id: required`).
- Canales: microcopy de secretos honesta, ruta legible, "cambiar en el Builder →".
- El drag nativo HTML5 funciona en el WKWebView de macOS 13 (brain→lienzo, model→brain).

### Latencia observada (Qwen3-4B Q4_K_M, iMac 2017 Intel, CPU)

| Métrica | Valor |
|---|---|
| Carga del modelo (2,3 GB) | ~35 s |
| Prompt eval | 24-27 tok/s |
| Generación | **5,8-7,1 tok/s** (~140-172 ms/token) |
| Respuesta corta (34 tokens) end-to-end en la app | **~6 s** (feed: 7:12:44 → 7:12:50) |

Usable para demo con respuestas cortas; una respuesta de 200 tokens serían ~30 s. Si el martes hay demo en vivo en esta máquina, ensayar con `max_tokens`/instrucciones de brevedad o considerar el 3B de las demos anteriores para lo interactivo.

---

## Añadido 2 — ¿Dónde está el canal webhook? (verificado en la app empaquetada)

**Pregunta:** ¿la paleta de canales del Builder ofrece los tres tipos (telegram, discord, webhook)?

**Respuesta corta: el webhook ESTÁ en la app, pero en el único sitio donde nadie mira.** No es el estado degradado del síntoma 1 (se verificó con core vivo y puerta abierta) y no es exactamente un bug de la paleta: es un hueco de descubribilidad doble.

**Lo verificado, con capturas:**
1. **La paleta del Builder no tiene tipos**: ofrece UN solo bloque genérico "channel" (secciones CANALES/CEREBROS/MODELOS con un bloque cada una — capturas 05 y 19). Al soltarlo, el nodo nace como **telegram** por defecto.
2. **El webhook SÍ existe en el Builder**: clic en el nodo de canal → panel de propiedades → dropdown TYPE con **telegram / discord / webhook** (captura 19, dropdown abierto en la app empaquetada). Al elegir webhook aparecen sus campos anidados (`bind`, `path`, `outbound_url`, `outbound_token_env` — `CanvasView.tsx:281-292`; la lista de tipos es `CHANNEL_TYPES = ['telegram','discord','webhook']`, `web/builder/src/config/schema.ts:161`).
3. **El wizard "+ Añadir canal" de Canales — el camino visible — solo ofrece Telegram y Discord** (captura 20: "¿Qué canal quieres conectar?" con dos tarjetas). El webhook no está en su catálogo (`cmd/korvun-desktop/frontend/src/lib/channels.ts:16-31`). Por el comentario de cabecera del wizard (`ChannelWizard.tsx:1-9`, SP6c) parece un recorte de alcance deliberado: los pasos 2-3 del wizard giran alrededor de UN token al keychain, y el webhook tiene otra forma (bind/path/outbound_url + secreto inbound).

**Clasificación:** hueco de UX/descubribilidad (Builder: la ruta bloque-genérico→nodo→dropdown es invisible; nada indica que un canal "telegram" recién soltado sea cambiable de tipo) + recorte de alcance sin rastro para el usuario (wizard: ni una línea tipo "¿webhook? configúralo en el Builder"). Un usuario que busque "webhook" recorrerá Canales→wizard y la paleta del Builder y concluirá, como Chano, que no existe.

**Fix mínimo:** (a) en el wizard, una tercera tarjeta "Webhook" aunque solo enlace al Builder con instrucciones ("se configura en el Builder: suelta un canal y cámbiale el tipo") — XS; (b) más ambicioso, la tarjeta Webhook con su propio paso 2 (bind/path/outbound_url) — M. Para la paleta: bloques por tipo (TG/DC/WH) en vez del genérico sería la solución de fondo — M — pero (a) quita el "no existe" del martes por casi nada.

## Veredicto honesto

**Para el martes (v0.9.1 patch, todo pequeño):**
1. **A2/Síntoma 1** — bypass del token gate en modo embebido (XS). Es la primera pantalla que verá cualquiera que abra el Builder: hoy parece rota.
2. **A1** — `openai-compatible` en PROVIDERS + base_url en el panel (S-M). Sin esto, la feature de la release se desmiente a sí misma en su propio builder.
3. **A4** — bloque `admin` garantizado en la config del desktop (S). Sin esto, el Builder de Chano estará roto de otra manera nueva cuando su core arranque, con el token nuevo ya puesto.
4. **A3 + B1** — feedback del drop de modelos + cierre del panel (S). Baratos y quitan las dos fricciones más visibles del canvas.
5. **Añadido 2** — tarjeta "Webhook" en el wizard, aunque solo señale al Builder (XS). Sin ella, para el usuario el canal webhook de la release "no existe".

**Puede esperar (v0.10):** tema del iframe (B2), log a fichero (B3 — aunque como deuda operacional yo lo subiría), default de autostart (B4, decisión de producto), toda la sección C (i18n de una pasada, no a parches).

**Los tres síntomas de Chano, en una línea cada uno:** (1) no es un bug del gateway sino una puerta de seguridad que en el desktop no protege nada y solo estorba — se quita con 5 líneas; (2) el drag funciona, lo que no funciona es que la app no te dice que los modelos van SOBRE los cerebros; (3) el panel no se cierra porque nadie escribió el código de cerrarlo.

**Estado de la máquina al cierre:** app sandbox cerrada, llama-server parado, perfil real intacto (verificado), perfil sandbox conservado en el scratchpad de la sesión por si quieres inspeccionarlo. Sin commits, sin cambios en código.
