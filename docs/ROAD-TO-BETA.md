# Korvun — Camino a la Beta Técnica Completa (ROAD TO BETA)

> **Documento vivo.** Reúne, en **orden de prioridad**, TODO lo que falta para
> declarar la beta técnica completa de Korvun. Es el **plan operativo** de lo que
> queda; el contraste MVP↔V1 sigue viviendo en [`ROADMAP-V1.md`](./ROADMAP-V1.md)
> y la definición de "beta lista" en [`../MASTER.md`](../MASTER.md) §8. Este
> documento no los reemplaza: los ordena en un plan de trabajo accionable.

---

> **Objetivo de beta declarado por Chano (2026-07-11): las CUATRO piezas (2 → 3 →
> 4 → 5).** Chano quiere las cuatro completadas para dar la beta por cerrada, en
> **orden secuencial estricto — una a una, cerrando cada una antes de empezar la
> siguiente**. Esto NO cambia los *criterios* V1: solo la **Pieza 2** cierra el 6º
> criterio pendiente; las Piezas **4 y 5 siguen siendo *más alcance*, no requisitos**
> (ver §criterios abajo y el aviso del doc maestro §9 sobre WhatsApp en la Pieza 4).
> Es un compromiso de **ejecución**: hacerlas todas de todos modos.
>
> **Nota honesta:** completar las cuatro es un compromiso de **VARIAS SESIONES**, no un
> sprint. Cada pieza es una **fase de peso** con su ciclo completo — encuadre
> (`/office-hours`) + `/plan-eng-review` + ADR(s) aceptado(s) + TDD (`make quality`
> `-race`) + docs de cierre — hechas **de una en una**. **No hay prisa:** la disciplina
> por-fase manda sobre la velocidad.

## Qué es esto

El plan de las **piezas que faltan** para pasar de "la arquitectura funciona y es
usable por su autor" a "una beta técnica completa que instala, configura y opera
alguien que no soy yo, y que aguanta la producción real". Cada pieza es una unidad
de trabajo con su checklist, sus ADRs previstos, su naturaleza (código / docs /
ambos) y su criterio de "hecho".

## Estado actual (honesto)

Korvun cumple **los 6 de 6** criterios de "esto ya es V1" del
[`ROADMAP-V1.md`](./ROADMAP-V1.md) §5 — **checklist V1 CUMPLIDA**:

- **[x] Mensaje end-to-end por el binario real** (Stage 11, verificado en vivo
  2026-06-21: Telegram → fan-out → política → respuesta).
- **[x] Persiste estado entre reinicios** (Stage 9: memoria de conversación
  durable en SQLite Go-puro, incluido apagado limpio).
- **[x] Es observable** (Stage 12: `slog` + 6 métricas Prometheus en `/metrics` +
  `/healthz`).
- **[x] Las políticas se expresan sin tocar Go** — el **builder no-code**
  (Stage 14 Fase 2: mutación de config en caliente vía **PR #6** + la UI React/TS/
  Vite vía **PR #7**, merge `442f7ea`).
- **[x] Lo configura alguien por fichero, sin recompilar** → **Pieza 1**
  (documentación de usuario + `korvun.example.json` + `config check`), validada en
  hardware (macOS, 2026-07-05: un tercero escribió el fichero y arrancó).
- **[x] Lo instala alguien que no soy yo, siguiendo la documentación** → **Pieza 1
  COMPLETADA/MERGEADA (PR #8)**, validada en hardware (iMac Intel, macOS 13,
  2026-07-05): install + quickstart end-to-end, un mensaje de Telegram con respuesta
  del modelo local, cero nube.
- **[x] Aguanta un proveedor caído sin caerse** → **Pieza 2 COMPLETADA**
  (ADR-0031, resiliencia), **verificada en hardware 2026-07-18**: primera petición
  en frío completada en ~6s sin timeout, en la misma máquina que descubrió F6.

**Orden del plan (las 4 piezas son el objetivo de beta de Chano):** Pieza 1 ✅ →
**Pieza 2 ✅** (errores de producción — cerró el 6º criterio V1; ADR-0031 accepted) →
**Pieza 3 ✅** (CLI subcomandos, ADR-0032, validada end-to-end en la Mac de Chano
2026-07-18) → **Pieza 4** (tercer canal, **próximo trabajo**, **opcional** por el doc
maestro §9) → **Pieza 5** (app Wails, la más pesada). **Secuencial, una a una**,
cerrando cada una antes de la siguiente. La Pieza 3 **no cerró ningún criterio V1**
pero fue **antes que 4-5** porque **reescribió la doc de la Pieza 1** a la forma
canónica `korvun serve --config …`, minimizando el drift. Las Piezas 4 y 5 **no
cierran criterio** (más alcance) pero Chano las quiere hechas igualmente — objetivo de
*ejecución*, no de *criterio*. **Pieza 4:** mantener el aviso del doc maestro §9 —
WhatsApp es "la más traicionera, opcional en beta"; al llegar, valorar WhatsApp
concreto vs un canal más amable (Discord/Slack) si aporta más con menos riesgo.

---

## Cómo se ejecuta este plan (la disciplina, no negociable)

Cada pieza es una **FASE DE PESO**, y se hace con el ciclo completo del proyecto:

1. **`/office-hours`** para encuadrar el espacio de diseño.
2. **`/plan-eng-review`** para estresar el plan antes de escribir código.
3. **ADR(s)** aceptado(s) que fijan las decisiones (Context7 verificado ANTES de
   programar contra cualquier librería / SDK / API externa — regla innegociable).
4. **TDD** (red → green), `make quality` verde con `-race` sobre TODA la suite, y
   su `/review` antes de cerrar.
5. **Docs de cierre** (stage doc / ADR / este plan + HANDOFF actualizados).

**Se hacen DE UNA EN UNA**, cerrando cada pieza antes de empezar la siguiente —
la misma granularidad Stage → Phase → Task de todo el proyecto. **Esto es un
camino de varias sesiones, no un sprint.** No se abren dos piezas a la vez.

---

## PIEZA 1 — Documentación de usuario + validación de instalación

> ✅ **COMPLETADA / MERGEADA a master vía PR #8** (merge commit `60e79df`, 2026-07-05).
> Entregó las guías de usuario (`INSTALL.md` por SO, `QUICKSTART.md` cero-a-mensaje,
> `BUILDER.md`, "Updating Korvun", bloque `admin` en `CONFIGURATION.md`). **Validada en
> hardware real** (iMac Intel, macOS 13, Ollama `llama3.2:1b`): la ruta macOS **y** el
> quickstart completo end-to-end — un mensaje de Telegram recibió la respuesta del
> **modelo local, cero nube**. Linux/Windows escritas por analogía y **marcadas
> no-verificadas** (7 TODO-VERIFY abiertos). **Diferido de esta pieza** (no bloqueante):
> Raspberry Pi (sin hardware), guía de extensión y referencia completa de la Control API
> (docs de contribuidor/integrador, no de instalar-y-usar). **Follow-up de empaquetado:**
> `korvun.example.json` en el paquete de release (el release no trae config de ejemplo).

**PRIORIDAD 1.** Es la pieza que **desbloquea que exista un usuario**: sin ella,
nadie que no sea el autor puede instalar, configurar ni operar Korvun. Es el resto
pendiente de **Stage 16** (el flip público y la maquinaria de release ya están; lo
que falta es la capa developer-facing). Cierra el criterio V1
**"lo instala alguien que no soy yo, siguiendo la documentación"**.

**Naturaleza:** sobre todo **escribir + probar**, poco código. Pero *escribir la
guía de instalación OBLIGA a probar que instala limpio en cada SO y en una Pi
real* → **valida el empaquetado de paso** (el `--snapshot` de Stage 15 se prueba
por fin contra máquinas reales).

### Checklist

- [ ] **Guía de instalación por SO** — Linux, Windows y macOS (x86-64 y ARM64),
      desde el artefacto firmado de release (`gh release download` → verificar
      checksum/cosign → arrancar).
- [ ] **Guía de instalación en Raspberry Pi** (ARM64 real) — el caso "edge" que la
      promesa del proyecto pone en el centro; incluye el `korvun.service` systemd
      endurecido de Stage 16 Phase A.
- [ ] **Quickstart** — de cero a "un mensaje entra y vuelve" en el menor número de
      pasos (parte de `docs/QUICKSTART.md`, ya existente, y complétala).
- [ ] **Referencia de configuración** — todos los campos del config JSON, con la
      aclaración **INEQUÍVOCA** de que `token_env` / `api_key_env` esperan el
      **NOMBRE** de la variable de entorno, **no el valor del secreto** (el hallazgo
      de la prueba en vivo — ROADMAP-V1 §5 (b); un operador podría pegar el secreto
      en el fichero y romper ADR-0010). Amplía `docs/CONFIGURATION.md`.
- [ ] **Guía del builder no-code** — cómo abrir `/builder`, editar brains/canales/
      rutas/políticas/modelos, guardar, y qué hace el reload en caliente (la máquina
      de estados de la Fase 2a).
- [ ] **Guía de extensión** — cómo añadir un **nuevo canal** y un **nuevo agente/
      herramienta** (los seams `channel.Channel` y `tool.Tool`), para contribuidores.
- [ ] **Referencia de la Control API** — `GET /api/brains`, `/api/channels`,
      `/api/events` (SSE), `GET`/`POST /api/config` (gated), `GET /api/reload/{handle}`.
- [ ] **Validación real de instalación** — ejecutar cada guía en su SO objetivo +
      una Pi física; registrar cualquier fricción del empaquetado como issue/ADR.

### ADRs previstos

Probablemente **ninguno de arquitectura** (es documentación). Un ADR corto SOLO si
la validación de instalación descubre un cambio de empaquetado (p. ej. el `getMe`
de boot con timeout fijo de 5s — ROADMAP-V1 §5 (a) — hacerlo configurable/con
reintento sí sería código con ADR).

### Cómo se valida que está hecho

Una persona distinta del autor sigue la guía de su SO (o una Pi) **sin ayuda
fuera del documento**, instala, configura por fichero, arranca, y ve un mensaje
end-to-end. En ese momento se tachan **dos** criterios V1: "lo configura alguien
por fichero" y "lo instala alguien que no soy yo".

---

## PIEZA 2 — Manejo de errores de producción

> ✅ **COMPLETADA — ADR-0031 `accepted`** (resiliencia: timeouts + retry +
> degradación). Encuadre (`/office-hours`) + `/plan-eng-review` + segunda voz
> adversarial absorbidos. Entregado en master, **SP1–SP7**: timeouts de dos capas
> (per-attempt + ceiling de router DERIVADO por forma de brain), cancel-on-first-
> usable-success con **carve-out de consenso** (SV1), mapeo HTTP del adapter de
> Ollama, retry decorator sobre el seam `Model` (SV2), la **invariante de arranque en
> frío F6** (un disconnect DURANTE la carga hace que Ollama ABORTE la carga
> `aborting load`/499 — el fix es timeout generoso por intento + warmup, no retry),
> **boot warmup** best-effort por el path decorado, y **fallback diferenciado +
> métricas de retry**.
>
> **Cierra el 6º criterio V1 ("aguanta un proveedor caído sin caerse") y está
> VERIFICADO en hardware (2026-07-18)** — en la misma máquina que descubrió F6 (iMac
> Intel, macOS 13), la **primera petición en frío completó en ~6s sin timeout**,
> donde `v0.1.0` fallaba. Ver `docs/notes/piece-2-framing.md` y el bloque "Motivación
> DEMOSTRADA" más abajo (el timeout Korvun→Ollama en frío, ya resuelto).

**PRIORIDAD 2.** Cierra el criterio V1 **☐ "aguanta un proveedor caído sin
caerse"**. Hoy los adapters **mapean** los errores (la gramática de sentinelas
`ErrProviderUnavailable` / `ErrRateLimited` + `*RateLimitError{RetryAfter}` está
intacta end-to-end), pero **la política de reintentos vive en el consumidor, que
aún no existe**. Referencia: [`ROADMAP-V1.md`](./ROADMAP-V1.md) §2 "Manejo de
errores de producción".

**Naturaleza:** **código**, con su ADR + TDD. Es la primera pieza de robustez con
concurrencia/tiempo real desde el supervisor de la Fase 2a — tratarla con el mismo
cuidado (`-race`, tests que muerden).

### Motivación DEMOSTRADA en hardware — timeout de Ollama en frío

> **No es hipotético: reproducido durante la validación del quickstart** (iMac
> Intel, macOS 13, Ollama `llama3.2:1b`, 2026-07-05). Con el modelo **sin cargar**,
> el **primer mensaje SIEMPRE falla**. Log real de Korvun:
> `brain: no usable answer ... "model: provider unavailable: Post
> http://127.0.0.1:11434/api/chat: context deadline exceeded"`; del lado de Ollama,
> `client connection closed before llama-server finished loading` con el
> `POST /api/chat` cancelado a **~5.2s**. Con el modelo **ya caliente**
> (`ollama run llama3.2:1b` previo), el bot respondió al instante.
>
> **Diagnóstico:** el timeout de Korvun hacia el proveedor (~5s) es demasiado corto
> para hardware que carga el modelo en frío. Es distinto del timeout de boot `getMe`
> (Pieza 1 / ROADMAP-V1 §5 (a)): aquí es el timeout **Korvun→proveedor de modelo** en
> el camino caliente.
>
> **Qué debe resolver la Pieza 2 (NO ahora — es código con su ADR):** el timeout
> Korvun→proveedor **configurable y/o más generoso**, y/o que Korvun **reintente
> mientras el proveedor está cargando** (model warmup). Mientras tanto, el quickstart
> documenta el workaround (calentar el modelo) en su sección de troubleshooting.

### Checklist

> **Entregado (ADR-0031 `accepted`, SP1–SP7 en master).** El diseño se afinó
> respecto al encuadre original: timeouts de dos capas + retry + degradación en vez
> de un circuit breaker (ver el ítem del breaker abajo).

- [x] **Timeout Korvun→proveedor por intento + warmup** — timeouts de dos capas
      (per-attempt configurable + ceiling de router DERIVADO por forma de brain), y
      **boot warmup** best-effort. Resuelve el arranque en frío (la invariante F6:
      un disconnect durante la carga hace que Ollama ABORTE la carga, así que el fix
      es timeout generoso + warmup, **no** retry). El workaround manual queda en el
      quickstart, marcado como innecesario post-`v0.1.0`.
- [x] **Retry con backoff** — retry decorator sobre el seam `Model` para errores
      recuperables (`ErrProviderUnavailable`, `ErrRateLimited` respetando `RetryAfter`),
      backoff exponencial con jitter, tope de intentos; **nunca** `ErrAuthInvalid`.
- [ ] **Circuit breaker por proveedor** — **DIFERIDO a post-beta** (ADR-0031 §7,
      decisión consciente): tras SV1/SV2 no hace falta para *no caerse*; lo que
      mitigaría es residual. No es un pendiente de beta.
- [x] **Degradación elegante** — cancel-on-first-usable-success con **carve-out de
      consenso** (SV1) + **fallback diferenciado**: con un proveedor caído el Brain
      responde con los supervivientes; si TODOS caen, el contrato de fallback
      (ADR-0014 §3) da respuesta honesta, no un crash.
- [x] **Métricas de retry** — reintentos expuestos en `/metrics` (extiende el seam
      `Metrics` de Stage 12, aditivo). *(Métricas de breaker: van con el breaker,
      diferidas.)*
- [x] **Dónde vive** — decidido: **retry decorator sobre `model.Model`** + ceiling de
      router derivado por el app, respetando la frontera mecanismo/política (ADR-0011).

### ADRs previstos

**1 ADR** ("retry + circuit breaker + degradación"), encuadrando la ubicación en la
capa mecanismo vs política y la interacción con fan-out/secuencial. Context7 solo si
se adopta una librería de circuit-breaking (preferir stdlib / hand-roll acotado
salvo que el test de cuatro ejes justifique una dependencia).

### Cómo se valida que está hecho

Un test de integración con un proveedor fake que **cae a mitad de operación**:
el binario sigue en pie, el breaker se abre, los reintentos respetan el backoff, y
el Brain responde con los proveedores sanos (o da fallback honesto si no queda
ninguno) — todo bajo `-race`. En vivo: matar Ollama/Groq mientras el bot conversa
y ver que Korvun **no se cae**.

---

## PIEZA 3 — CLI con subcomandos (estilo git/docker)

> ✅ **COMPLETADA (SP1–SP5, ADR-0032 `accepted`); validada end-to-end en la Mac de
> Chano (2026-07-18).** Reemplazó `./korvun -config x.json` (un binario con flags) por
> una CLI que se siente como herramienta instalada: `korvun serve` / `config check` /
> `status` / `version` / `help`, con banner de arranque gated a stderr. Estilo
> pure-stdlib (color 256 por defecto, truecolor bajo `COLORTERM`; guard VT en Windows),
> `go.mod` intacto en 3 deps. Contrato pineado por ADR-0032 y la suite de `internal/cli`.

**PRIORIDAD 3 — DX / pulido, NO cierra ningún criterio V1.** No cierra ninguno de los
6 criterios de beta; es ergonomía de cara al usuario. **Dos ejes de prioridad:** la
**Pieza 2 mantiene mayor prioridad de _criterio_** (cierra "aguanta proveedor caído"),
por eso va antes; pero la CLI tiene **mayor prioridad de _timing_ que las piezas 4-5**
porque **reescribe la doc de la Pieza 1** recién mergeada (INSTALL/QUICKSTART/BUILDER/
`korvun.service` usan `./korvun -config`). El shim de retrocompat evita que esa doc se
**rompa** (solo deja de ser canónica), así que no fuerza urgencia — pero conviene hacerla
pronto tras la Pieza 2 para minimizar el drift.

**Naturaleza:** **código** Go + **ADR corto de CONTRATO de interfaz** (no de dependencia)
+ TDD + **una sub-fase de actualización de la doc de la Pieza 1**.

### Resumen del encuadre aprobado

- **stdlib, NO Cobra.** Subcomandos con `flag.NewFlagSet` en un paquete `internal/cli`
  (`Run(args, stdout, stderr) int`, testable; `main.go` queda de 3 líneas). Decisión
  razonada: set pequeño y estable (4-5 comandos, un nivel de anidamiento), disciplina
  zero-deps de Korvun (3 deps hoy), mandato del maestro "stdlib si es razonable"; Cobra
  no cruza el listón (completions / árboles grandes = YAGNI). Se reevaluaría con su ADR
  si la CLI creciera a docenas de comandos.
- **Retrocompat:** shim de ~5 líneas → `korvun -config x.json` sigue funcionando
  (= `serve` implícito), para **no invalidar la doc/systemd validados en hardware**.
  Forma canónica nueva `korvun serve --config` (stdlib `flag` acepta `-`/`--` igual).
- **`config check`:** split **offline `config.Validate()` por defecto** + **`--preflight`
  online** (reusa `app.Preflight`; getMe + secretos + selector de privacidad).
- **`status`:** **cliente HTTP fino de la read-only control API YA existente**
  (`GET /api/brains` + `/api/channels` + `/healthz` en `127.0.0.1:2112`; flag `--addr`;
  **sin token**; **cero código de servidor nuevo**; fallo honesto si el admin está off).
- **`version`:** porta el `--version` actual (`buildinfo.Format`) a subcomando.
- **Logo ASCII** `[placeholder]` a **STDERR nunca stdout** (no contaminar salida
  machine-readable); el arte concreto se decide aparte (se genera, no es logo de marca).
- **Exit codes:** `0` ok / `1` fallo runtime/validación / `2` error de uso.

### Checklist (5 sub-fases TDD, una a una)

- [x] **1 — scaffold + dispatch** — `internal/cli`, `version`, `help`/no-args, banner.
- [x] **2 — `serve` + shim de retrocompat** — arranque tras el seam `c.boot`;
      `main.go` de 3 líneas llama `cli.Run`; shim estrechado a `-config`/`--config`.
- [x] **3 — `config check`** — offline `config.Load`/`Validate` + `--preflight` online
      (seam `c.preflight`); primeros roles de color + guard VT de Windows (R4).
- [x] **4 — `status`** — cliente HTTP fino de la control API; tests con `httptest`.
- [x] **5 — docs-update + re-validación macOS** — docs reescritas a `korvun serve
      --config …` (legacy mencionado una vez), ADR-0032, `korvun.example.json` en la
      raíz + en los archives, ldflags retargeteados a `internal/cli.version`, y
      **re-validado end-to-end en la Mac de Chano** (2026-07-18).

### ADR previsto

**1 ADR corto de contrato** (set de subcomandos + retrocompat + exit codes + la decisión
stdlib-vs-Cobra y su porqué). **Sin Context7** (stdlib). Sin nueva dependencia.

### Cómo se valida que está hecho

Los subcomandos parsean y despachan bien (tests table-driven de `cli.Run`: exit codes,
`version`, `config check` válida/inválida, `status` contra un admin server `httptest`),
`make quality` verde `-race`, y en vivo `korvun serve --config …` arranca el mismo
sistema que hoy en el Mac de Chano (la re-validación de la sub-fase 5).

---

## PIEZA 4 — Un tercer canal (WhatsApp u otro)

**PRIORIDAD 4 — DESEABLE, NO beta-crítico.** Ya hay **2 canales** (Telegram +
Webhook genérico); un tercero es **MÁS ALCANCE, no un requisito de beta**.

> ⚠️ **Aviso del documento maestro ([`MASTER.md`](../MASTER.md) §9):** *"Integración
> de WhatsApp — la más traicionera → tratarla como **OPCIONAL en beta**; priorizar
> Telegram + Webhook."* La definición de beta lista (§8.2) dice literalmente
> "Telegram + Webhook genérico, **y WhatsApp si llega**". Cubierto por **ADR-0002**
> (que difirió WhatsApp).

**Naturaleza:** **código**, con su ADR + TDD. El seam `channel.Channel` ya existe
(Telegram lo valida), así que un tercer canal es un adaptador nuevo detrás del mismo
contrato, no un cambio de arquitectura.

### Checklist

- [ ] **Decidir el canal** — WhatsApp (Business Cloud API) **o** una alternativa
      menos traicionera (Discord/Slack/Signal) si aporta más con menos riesgo.
- [ ] **ADR del canal** — proveedor, modo (webhook/polling), verificación de firma
      de entrada, contrato de secreto env-only (ADR-0010), Context7 del SDK/API.
- [ ] **Adaptador** detrás de `channel.Channel`, con su lifecycle (arranque/paro,
      `DroppedCount`), TDD como Telegram/Webhook.
- [ ] **Validación de entrada** en el borde del canal (regla de seguridad del
      proyecto).

### ADRs previstos

**1 ADR** por canal nuevo (extiende la línea de ADR-0002). Context7 **obligatorio**
para el SDK/API del proveedor antes de una sola línea.

### Cómo se valida que está hecho

Un mensaje real entra por el tercer canal, hace el round-trip completo (canal →
router → brain → política → canal) por el binario real, y su lifecycle
(arranque/paro limpio, drops contados) pasa `-race`. **No bloquea la beta** si se
decide dejarlo como Telegram + Webhook.

---

## PIEZA 5 — App de escritorio Wails

**PRIORIDAD 5 — la más pesada y la menos crítica.** La **maquinaria de empaquetado
ya está** (Stage 15 cerrado: GoReleaser, ×6 binarios, checksums, SBOM, systemd).
Lo que falta es la **app nativa Wails en sí**: empaquetar el frontend + el binario
en una aplicación de escritorio para los 3 SO. Deseable para una presentación
"profesional" de cara a no-técnicos, pero **NO es un criterio de la checklist V1**.

**Naturaleza:** **ambos** (código Go+frontend + empaquetado), la pieza de mayor
superficie. **Requiere Context7 para Wails** antes de programar contra él.

### Checklist

- [ ] **ADR de Wails** — versión, cómo embebe el frontend del builder ya existente
      (reusar `web/builder`, no reescribir), cómo convive con el binario headless
      (misma lógica, otra carcasa), impacto en `go.mod` / build (Context7 primero).
- [ ] **Shell de escritorio** — ventana nativa que sirve el builder + arranca/
      supervisa el runtime de Korvun embebido.
- [ ] **Empaquetado por SO** — `.app` (macOS), instalador Windows, binario Linux;
      encajar con o extender el pipeline GoReleaser existente.
- [ ] **Firma/notarización** por SO si se distribuye (macOS notarization, etc.).

### ADRs previstos

**≥1 ADR** (toolchain Wails + arquitectura de la app + empaquetado). Context7 de
Wails es **prerrequisito duro** (regla innegociable de librería externa).

### Cómo se valida que está hecho

La app de escritorio arranca en los 3 SO desde el artefacto empaquetado, muestra el
builder, y opera un Korvun embebido end-to-end — sin terminal. `make quality` verde
y el binario headless intacto (la app es una carcasa, no un fork de la lógica).

---

## Criterios de "esto ya es V1" (del ROADMAP-V1 §5, actualizados)

> La checklist honesta de cuándo dejar de llamarlo beta. Estado a **2026-07-05**.

- [x] **Un mensaje real entra, se enruta, varios modelos responden, una política
      decide, y la respuesta vuelve — en el binario real.** COMPLETO (Stage 11,
      verificado en vivo 2026-06-21).
- [x] **Persiste estado entre reinicios.** COMPLETO (Stage 9).
- [x] **Es observable.** COMPLETO (Stage 12).
- [x] **Las políticas se expresan sin tocar Go (builder no-code).** COMPLETO
      (Stage 14 Fase 2 — **PR #6** mutación + **PR #7** UI, merge `442f7ea`).
- [ ] **Lo configura alguien por fichero, sin recompilar.** El **mecanismo** existe
      desde Stage 11 (config JSON); su validación de cara a un tercero se cierra con
      la documentación de **Pieza 1**.
- [ ] **Lo instala alguien que no soy yo, en su máquina, siguiendo la
      documentación.** → **PIEZA 1**.
- [ ] **Aguanta un proveedor caído sin caerse.** → **PIEZA 2**.

**Próximo paso:** **Pieza 2 EN CURSO** — encuadre + `/plan-eng-review` hechos, F6
verificado en hardware, **ADR-0031 en borrador** (`status: proposed`); sigue la revisión
del copiloto → segunda voz de Codex → TDD. Luego Piezas 3 → 4 → 5, una a una (objetivo
de beta de Chano, 2026-07-11).

---

> **Nota:** documento **VIVO**. Al cerrar cada pieza, tachar su checklist, actualizar
> el estado, y reflejarlo en `docs/HANDOFF.md` y en `docs/ROADMAP-V1.md` §5.