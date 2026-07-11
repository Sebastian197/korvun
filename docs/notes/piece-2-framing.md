# Pieza 2 — Manejo de errores de producción (resiliencia)

> **Notas de trabajo, no doc de producto.** Reúne el ENCUADRE aprobado por el
> copiloto (2026-07-11) + los hallazgos del `/plan-eng-review` que lo estresó
> ANTES del ADR. El ADR **aún no está escrito**; se redacta después de absorber
> estos hallazgos. No toca código ni docs de producto.

---

## 1. Encuadre aprobado (los cuatro puntos)

Cierra el criterio V1 **"aguanta un proveedor caído sin caerse"**. Direcciones
aprobadas por el copiloto:

- **(a) Timeout en frío:** configurable **+** default más generoso (60–120s) **+**
  reintento como caso del retry general. **Bloqueante previo:** reconciliar las
  capas de timeout incoherentes. El techo real hoy es el **`DefaultBrainHandlerTimeout
  = 5s` del router** (`internal/router/doc.go:67`), que envuelve `Brain.Handle` y
  cuyo ctx fluye hasta el POST — **ese es el ~5.2s de Chano**, no el per-model de
  30s (`internal/app/app.go:55`), que nunca llega a disparar porque su padre muere
  antes.
- **(b) Retry:** backoff exponencial + jitter en un **decorador de `model.Model`**
  (heredado por fan-out y sequential sin tocarlos). Reintenta
  `ErrProviderUnavailable` / `ErrRateLimited` (respetando `RetryAfter`); **nunca**
  `ErrAuthInvalid` / 4xx / validación. Cierra el hueco de mapeo de Ollama
  (5xx→`Unavailable`, 429→`RateLimited`, como Groq).
- **(c) Circuit breaker:** **YAGNI → diferido post-beta** como decisión consciente
  (re-clasificar en `ROAD-TO-BETA.md`).
- **(d) Degradación:** la degradación-con-supervivientes **ya existe**
  (fan-out+priority en `priority.go:88`, sequential en `sequential.go:119`). El caso
  de Chano es un brain de **un solo modelo** que solo salva timeout+retry. Retry
  aplicado **por-modelo, debajo del coordinator** + fallback diferenciado
  (arrancando vs caído).

Hechos anclados en el código (referencias):

| Capa | Constante / sitio | Valor hoy | ¿Config JSON? |
|---|---|---|---|
| Router → `Brain.Handle` | `DefaultBrainHandlerTimeout` (`router/doc.go:67`) | **5s** | ❌ |
| Coordinator → per-model | `DefaultPerModelTimeout` (`app.go:55`); `CallOne` lo aplica en `fanout.go:254` | 30s | ❌ |
| Adapter → HTTP | `WithRequestTimeout` (`ollama.go:57`, `groq.go:72`), alimentado con `perModelTimeout` (`app.go:636,646`) | 30s | ❌ |

- **El router NO llama `WithBrainHandlerTimeout`** (`app.go:217-224` solo pone
  `WithErrorHandler` + `WithEventPublisher`) → corre con el default de 5s.
- **Reintentos hoy: cero.** `CallOne` llama `Generate` una vez (`fanout.go:265`).
- Gramática de errores intacta (`model/errors.go`); Groq mapea bien
  (`groq.go:208`); **Ollama mapea TODO no-2xx a `ErrProviderResponse`**
  (`ollama.go:121`) — el hueco.

---

## 2. Hallazgos del /plan-eng-review (lo que el encuadre NO cerró)

Formato: `[SEVERIDAD] (confianza N/10) — hallazgo`. Ordenados por lo que más
mueve el diseño del ADR.

### F1 — [P1] (9/10) Semántica del timeout: ¿per-INTENTO o per-modelo-TOTAL?

El encuadre dice "el deadline de Handle debe ser ≥ per-model × reintentos". Esa
frase **asume que `perModelTimeout` es el presupuesto por INTENTO**. Pero hoy
`CallOne` aplica `perModelTimeout` como **un único** `context.WithTimeout` que
envuelve **una** llamada a `Generate` (`fanout.go:254-258`). Si el retry vive en
un decorador **debajo** de `CallOne` (envolviendo `model.Model`), entonces el
`WithTimeout(perModelTimeout)` de `CallOne` envuelve **TODOS los reintentos** →
pasa a ser el presupuesto **per-modelo-TOTAL**, y cada intento individual queda
**sin deadline propio** (el primer intento puede comerse todo el presupuesto).
Contradice "per-model × reintentos".

**Decisión que el ADR debe fijar sin ambigüedad:** ¿`perModelTimeout` es el
presupuesto por intento o el total? Recomendación: **colapsar las capas, no
reconciliar tres.** El decorador de retry pasa a ser el **único dueño** de (i) el
deadline **por-intento** (un `WithTimeout` por cada `Generate`) y (ii) el
**presupuesto total** de reintentos. Se **elimina** el `WithPerModelTimeout` del
coordinator y el `WithRequestTimeout` del adapter (o se deja solo el del adapter
como el por-intento que el decorador orquesta). Quedan **dos** capas claras en vez
de tres incoherentes:

```
  brainHandlerTimeout (router, ceiling)  >=  retry-total-budget (decorador)
      retry-total-budget                  =  N * (per-attempt-timeout + backoff)
          per-attempt-timeout            aplicado por-Generate dentro del decorador
```

*Preferencia: explícito > listo; DRY (un solo dueño del deadline, no tres sitios
aplicando WithTimeout).*

### F2 — [P1] (9/10) Retry en fan-out: el superviviente ESPERA al modelo moribundo

`fanout.Run` hace `wg.Wait()` — **espera a TODAS** las goroutines
(`fanout.go:187`). Hoy un modelo caído falla rápido (un timeout) y el fan-out
devuelve. **Con retry**, la goroutine del modelo caído tarda **todo su presupuesto
de reintentos** (N × timeout + backoff) antes de volver, y el fan-out **bloquea en
ella aunque un modelo sano haya respondido en 200 ms**. Resultado: en un brain de
fan-out, la latencia del request pasa a ser la del **modelo más lento reintentando**,
no la del más rápido que respondió.

Es exactamente la clase de bug "intersección de dos features correctas por separado"
que ya mordió dos veces (4.3 `%w`, 9 `AppendTurns`, ver HANDOFF). Retry (correcto) +
fan-out wait-all (correcto) = el usuario espera los reintentos del proveedor muerto
aunque uno vivo ya contestó.

**Mitigaciones a evaluar en el ADR:**
- Presupuesto de retry **modesto** (N bajo, backoff corto) para acotar el peor caso.
- **Retry-enabled configurable por-brain**: un brain de consenso/fan-out con
  supervivientes puede querer retry=off (el superviviente ya cubre); un brain de
  un-solo-modelo lo quiere on. El caso de Chano (Ollama solo) es fan-out de 1 → no
  hay a quién esperar, retry ayuda limpio.
- El `brainHandlerTimeout` (ceiling) debe fijarse **con el peor caso del fan-out en
  mente**: el usuario espera hasta ese techo.

*Este hallazgo es también munición honesta para la deferral del breaker (ver F7):
el breaker es justo lo que acotaría esto. No revierte la deferral, pero el ADR debe
reconocer el coste, no venderlo como YAGNI puro.*

### F3 — [P1] (9/10) Parent-ctx cancelado: NO reintentar; ambos casos son `Unavailable`

Cuando el ctx EXTERNO (el `brainHandlerTimeout` del router, o un shutdown) se
cancela, el adapter devuelve `ErrProviderUnavailable` envolviendo `context deadline
exceeded` / `context canceled` (`ollama.go:117`, `groq.go:171`). Si el decorador
trata `ErrProviderUnavailable` como reintentable a secas, **reintentará contra un
ctx padre ya muerto** → o falla al instante en bucle, o (peor) ignora la
cancelación y alarga el shutdown.

**El decorador DEBE distinguir** "mi deadline por-intento disparó" (reintentable) de
"el ctx del caller fue cancelado" (parar ya). Ambos casos cumplen
`errors.Is(err, ErrProviderUnavailable)`, así que la clasificación por sentinela
**no basta**: hay que inspeccionar `ctx.Err() != nil` (padre cancelado → parar)
antes de cada reintento. Correctness landmine, no cosmético.

### F4 — [P2] (8/10) Nivel del campo de config: **per-modelo**, no top-level ni brain

El encuadre ofreció brain-level vs top-level. Ambos son demasiado gruesos: un brain
de fan-out tiene modelos de latencias **radicalmente distintas** (llama3.2:1b en
frío ~40s; Groq <1s). Un único valor compartido obliga a: o generoso para todos
(un Groq caído tarda 60s en fallar en vez de 1s), o ajustado (el local en frío nunca
entra). El timeout es propiedad **del proveedor/modelo**, no del brain.

**Recomendación:** campo en `ModelConfig` (`config.go:150`, ya existe la estructura),
con un **default top-level (o brain) que el per-modelo sobre-escribe**. Aditivo, y el
reload en caliente (supervisor) ya sabe recomponer configs. *Preferencia: manejar
más casos borde, no menos; explícito.*

### F5 — [P1] (9/10) Son CUATRO consumidores, y el que importa (router) está sin cablear

El encuadre habla de "tres capas". Son **cuatro**: router `brainHandlerTimeout` +
coordinator `perModelTimeout` + adapter `WithRequestTimeout` + (el nuevo) decorador.
Y el **que gobierna el peor caso — el del router — hoy no lo pone nadie**
(`app.go:217-224`), así que corre con el default de 5s. La reconciliación no es solo
"colapsar el per-model y el adapter": es que **el campo de config debe alimentar
DOS valores derivados** — (1) el ceiling del router (`WithBrainHandlerTimeout`,
≥ presupuesto total de retry) **y** (2) el por-intento del decorador. El wiring en
`app.go` debe dejar de aplicar `perModelTimeout` por partida doble (`app.go:636,646`)
y empezar a fijar explícitamente el timeout del router. Test concreto: un modelo que
siempre expira, N reintentos, y `Handle` vuelve **por el ceiling**, no en N×timeout.

### F6 — [P1] (10/10, VERIFICADO EN HARDWARE 2026-07-11) El retry NO arregla el arranque en frío; Ollama ABORTA la carga al desconectar

**Incógnita RESUELTA empíricamente** (iMac Intel, macOS 13, Ollama 0.30.8,
llama3.2:1b, corrido por Claude Code en el Mac de Chano). Cuando el cliente
(= Korvun al saltar su timeout) cierra la conexión **durante la carga del modelo**,
Ollama **ABORTA la carga**. Log del servidor, reproduciendo la línea EXACTA del
repro de Chano:

```
WARN source=llama_server.go:1137 msg="client connection closed before
     llama-server finished loading, aborting load"
[GIN] 499 | 1.17s | POST "/api/chat"
```

Tras el aborto, `ollama ps` queda **vacío** (el modelo NO sigue cargando solo).
Contraste medido: si el corte cae DESPUÉS de que el modelo cargó (durante la
generación), el runner **sí** sobrevive caliente (`ollama ps` = 5 min). Pero el
fallo de Chano es la carga misma superando el timeout, no la generación.

Medidas: carga en frío ~5.8s total / caliente **0.86s**. (Matiz de fidelidad: la
carga "fría" de Ollama medida fue ~1.5s porque el fichero ya estaba en la caché de
disco del SO; el fallo real de Chano es con **disco frío** —1.3 GB sin cachear tras
arrancar— y por eso superó los 5s. El comportamiento de **abortar es determinista**;
los segundos varían con la caché.)

**Consecuencia de diseño, ahora sobre HECHOS:**
1. **El retry NO arregla el caso de Chano.** Cada reintento con el mismo timeout
   corto re-dispara una carga en frío y la **vuelve a abortar en el mismo punto** →
   falla idéntico. Peor: es **contraproducente** — nunca deja completar la carga y
   **desperdicia CPU** tirando el trabajo parcial en cada aborto.
2. **El fix es el timeout por-intento GENEROSO** (≥ peor caso de carga en frío con
   disco frío) para que el **primer** intento deje completar la carga. Esta es la
   palanca, no el retry.
3. **Complementario (probablemente lo mejor): precalentar el modelo en boot** para
   proveedores locales (un `Generate` trivial al arrancar, o `keep_alive`) — una vez
   caliente todo va a <1s. **Sube de "alternativa/TODO" a candidato de primera línea
   del ADR** para el arranque en frío.
4. El retry SIGUE siendo válido para errores **transitorios post-carga** (un 503, una
   conexión caída), pero **NO** como el mecanismo del arranque en frío. Esto reordena
   el encuadre: el arranque en frío = timeout generoso + warmup; el retry = otra clase
   de fallo.

*Este hallazgo REEMPLAZA la suposición del encuadre de que "retry con reintento
durante la carga" salvaba el caso de Chano. La verificación en hardware lo tumbó —
exactamente el valor de probar el cimiento antes de escribir el ADR.*

### F7 — [P2] (7/10) `brainWorkers=1` + Handle lento reintentando = la cola del brain se atasca

`DefaultBrainWorkers = 1` (`router/doc.go`), cola por-brain cap 64, enqueue timeout
250 ms. Si `Handle` pasa a tardar 30–60s reintentando, el **único** worker queda
bloqueado, la cola inbound se llena y los mensajes nuevos reciben `ErrBrainSaturated`.
**Un mensaje en arranque en frío puede atascar la cola de ese brain un minuto.**
Lens "systems over heroes" / "blast radius". Es el segundo coste real de diferir el
breaker (F2). Mitigación mínima: documentarlo; considerar subir `brainWorkers` o
acotar el presupuesto total. No bloqueante para beta, pero honesto listarlo.

### F8 — [P3] (8/10) La métrica de latencia cambia de significado (total incl. reintentos)

`CallOne` captura `Latency = now().Sub(start)` alrededor de **toda** la llamada
decorada (`fanout.go:260-263`). Con retry, `ObserveProviderDuration` pasa de medir
"una llamada a `Generate`" a "la suma de todos los intentos + backoff". Ni "primer"
ni "último" intento — la **suma**. Decisión de observabilidad: mantener el total
(renombrar/documentar) **y** que el decorador emita métricas **por-intento**
(reintentos por proveedor, ver §5 del encuadre). Fijar el significado con un test.

### F9 — [P3] (8/10) El refinamiento de Ollama es completitud, NO el fix de Chano

Ojo con vender el mapeo 5xx/429 de Ollama como lo que arregla a Chano. Su error es
`context deadline exceeded` desde `client.Do` → **ya** mapea a `ErrProviderUnavailable`
(`ollama.go:117`), que **ya** es reintentable bajo la clasificación aprobada. El
refinamiento (5xx→Unavailable, 429→RateLimited) es **correcto por completitud** pero
de bajo valor para el caso real (Ollama local rara vez devuelve 429). Hacerlo, sí;
no confundir su motivación.

### F10 — [P3] (7/10) `RetryAfter` mayor que el presupuesto restante → rendirse, no dormir

Si un 429 trae `RetryAfter` que excede el presupuesto/ceiling restante, no
dormir-para-luego-fallar: **capar `RetryAfter` al presupuesto restante** y rendirse
temprano si no cabe. El encuadre dijo "respetar RetryAfter"; falta el borde "y no
esperes más allá del techo".

---

## 3. Secciones del review

### Arquitectura
Los hallazgos load-bearing son F1 (semántica del timeout), F2 (amplificación en
fan-out), F3 (parent-ctx), F5 (cuatro consumidores). El decorador sobre `model.Model`
es la ubicación correcta (mecanismo, se compone por decoración; respeta ADR-0011),
**siempre que** F1/F3 se cierren: el decorador debe ser **por-instancia** (uno por
modelo), nunca compartido — comprobar con `-race` bajo `Generate` concurrente.

### Calidad de código
DRY: F1/F5 eliminan la triple aplicación de `WithTimeout`. Un único paquete
`internal/model/retry` (o similar) con clock+jitter inyectables. No re-derivar la
clasificación por adapter (así entró el bug P1 `%w` de 4.3): la clasificación vive
en UN sitio (el decorador), consumiendo la gramática de sentinelas existente.

### Tests (gaps a añadir sobre las sub-fases del encuadre)
- **Regresión F2:** fan-out con 1 modelo caído (reintentando) + 1 rápido → assert
  que el rápido gana y el total ≤ ceiling (documenta la amplificación wait-all).
- **F3:** parent-ctx cancelado a mitad → el decorador NO reintenta; para ya.
- **F5:** modelo que siempre expira, N reintentos → `Handle` vuelve por el **ceiling**
  del router, no en N×timeout (integración).
- **F10:** `RetryAfter` > presupuesto restante → rendirse sin dormir de más.
- **F8:** fijar el significado de la métrica de latencia (total incl. reintentos).
- **EL TEST DE CHANO:** proveedor lento que responde tras el timeout corto en el
  primer intento y bien en el segundo → ahora responde (reintento). *Ver F6: validar
  además el comportamiento real de Ollama en hardware.*
- `-race` sobre el decorador con `Generate` concurrente (fan-out).

### Rendimiento
F2 (amplificación wait-all) y F7 (atasco de cola con `brainWorkers=1`) son los dos
findings de rendimiento. Ambos acotables con presupuesto de retry modesto + ceiling
bien puesto. Ninguno bloquea beta; ambos deben quedar documentados en el ADR como
coste consciente de diferir el breaker.

---

## 4. NOT in scope (diferido conscientemente)

- **Circuit breaker** — post-beta (YAGNI para "no caerse"; F2/F7 son su coste real,
  reconocido). Re-clasificar en `ROAD-TO-BETA.md`.
- ~~**Precalentar modelo en boot** — alternativa diferida.~~ **PROMOVIDO a scope**
  (F6 verificado: el retry NO cubre el arranque en frío). El ADR debe decidir entre
  timeout-generoso y/o warmup-en-boot; ya no es un TODO opcional.
- **Native function-calling / streaming timeouts** — otro seam, no toca.
- **Timeout de boot `getMe`** (ROADMAP-V1 §5a) — seam distinto (arranque, no camino
  caliente); fuera de esta pieza.

## 5. Qué ya existe (reusar, no reconstruir)

- Degradación-con-supervivientes: fan-out+priority (`priority.go`) y sequential
  (`sequential.go`) — **ya funciona**; no reconstruir.
- Gramática de sentinelas + `*RateLimitError{RetryAfter}` (`model/errors.go`) — el
  decorador la consume, no la reinventa.
- Seam `metrics.Metrics` (Stage 12) — extensión aditiva para contadores de retry.
- Mapeo HTTP de Groq (`groq.go:208`) — plantilla para el refinamiento de Ollama.
- **No hay** helper de backoff reusable; el "poll del reload" es polling de estado
  cliente a intervalo fijo, no backoff exponencial. Se escribe uno pequeño (stdlib).

## 6. Sin dependencias externas

Todo con stdlib (`context`, `time`, `math/rand` con seam determinista). El único que
tentaría una lib (breaker) está diferido. **Sin Context7 necesario** para esta pieza.

---

## 7. Resumen para el ADR (qué debe fijar)

1. **Semántica del timeout (F1):** colapsar a dos capas — por-intento (decorador) +
   ceiling (router). Eliminar la doble/triple aplicación de `WithTimeout`.
2. **Nivel de config (F4):** `ModelConfig.request_timeout` (o similar) per-modelo +
   default top-level/brain. Alimenta el por-intento del decorador; el ceiling del
   router se deriva ≥ presupuesto total de retry (F5).
3. **Arranque en frío (F6, VERIFICADO):** el fix es **timeout por-intento generoso**
   (≥ carga en frío con disco frío) **y/o precalentado-en-boot** para modelos locales.
   El retry con timeout corto NO sirve aquí (Ollama aborta la carga) y es
   contraproducente. Decidir en el ADR: generoso vs warmup vs ambos.
4. **Retry (F2/F3):** decorador por-instancia sobre `model.Model` para errores
   **transitorios** (no para el arranque en frío); clasificación por sentinela **+**
   guardia de `ctx.Err()` (parent cancelado → parar); presupuesto modesto; considerar
   retry-enabled por-brain para acotar la amplificación de fan-out.
5. **Clasificación (F9/F10):** confirmada; refinar Ollama por completitud; capar
   `RetryAfter` al presupuesto restante.
6. **Degradación:** fallback diferenciado (arrancando vs caído); reconocer el coste
   de diferir el breaker (F2/F7), no venderlo como YAGNI puro.
7. **Observabilidad (F8):** métricas de retry por proveedor; fijar el significado de
   la latencia (total incl. reintentos).

**Estado:** encuadre + hallazgos listos. Siguiente paso del workflow: absorber estos
hallazgos en el **ADR de política de resiliencia**, luego TDD (rojo) desde la
sub-fase 1. ADR **aún no escrito**.
