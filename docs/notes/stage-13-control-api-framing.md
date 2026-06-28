# Stage 13 — Control API — ENCUADRE (pre-ADR, decisión PENDIENTE)

> **Estado:** encuadre de framing producido con `/office-hours` +
> `/plan-eng-review` (2026-06-28). **NO es un ADR. NO hay código.** Documento
> para revisión conjunta (usuario + copiloto) antes de cualquier ADR. Cuando se
> decida el corte, este encuadre alimenta el ADR de Stage 13.

---

## Verificación del repo (leído, no de memoria)

- **El seam existe y es exacto:** `httpserver.Handle(pattern string, h http.Handler)`.
  Mux = `http.ServeMux`. **`Handle` debe llamarse ANTES de `Start`** (el mux no
  es seguro de mutar sirviendo) → las rutas de la control API se registran en
  `Build()`, no en caliente.
- **El servidor admin SOLO existe si `observability.enabled`** (default-on,
  loopback `127.0.0.1:2112`). Si se desactiva, no hay servidor → la control API
  depende de su existencia. Punto de diseño para el ADR.
- **El config NO lleva secretos** — solo NOMBRES de env vars (`token_env`,
  `api_key_env`). Exponer config activa es secret-free por construcción. Pero
  `App` **descarta** `*config.Config` tras `Build` (solo retiene router,
  channels, store, metrics) → habría que retenerla o armar summaries en `wire()`.
- **Brains:** en el mapa privado `router.brains`, **sin accessor público** →
  listar brains necesita un seam reader nuevo (additive). **Channels:** `App` ya
  los tiene (`[]Channel`, `Name()`, `DroppedCount()`).
- **Conversaciones:** `conversation.Store` es append-only + `LoadRecent(key, n)`;
  **sin list/query** → navegarlas necesita capacidad nueva del Store → difiere.
- **`/healthz` es liveness-only por decisión** (ADR-0014 §3 desacopla liveness de
  la salud de un provider). Salud per-provider **no se trackea hoy**.
- **Precedentes de seguridad:** loopback sin auth = seguro por `127.0.0.1`;
  bindear `0.0.0.0` = el operador "owns auth/TLS/firewall"; `readHeaderTimeout`
  cierra Slowloris (gosec G112); secrets env-only. **Go 1.26** → method-pattern
  routing nativo (`"GET /api/brains"`).

---

## (a) Veredicto Step 0 — el corte mínimo

**Control API SOLO-LECTURA (introspección) sobre el servidor admin loopback
existente, bajo prefijo `/api`, sin auth (mismo perfil de riesgo que
`/metrics`). TODA mutación se difiere a Stage 14, cuando el builder es el
consumidor real — el mismo lente honesto que aplicamos al bus.**

Diferencia clave con el bus: **la lectura SÍ tiene consumidor real e inmediato**
(un operador quiere ver qué está cableado y vivo sin grepear logs). El bus tenía
cero. Y Stage 12 construyó el httpserver EXPLÍCITAMENTE para que Stage 13 montara
aquí.

**Qué entra (corte mínimo; consumidor = operador con curl):**
- `GET /api/brains` — brains **resueltos**: nombre, sensitivity, policy,
  dispatch, y **los modelos que SOBREVIVIERON al selector de privacidad**. Esto
  NO es visible en el config crudo (el selector puede excluir cloud de un brain
  Private) ni en `/metrics` ni en el fichero — es el valor real que solo el
  binario en caliente conoce.
- `GET /api/channels` — canales: type, mode, name, dropped count.
- (opcional) `GET /api/info` — vista resuelta secret-free del despliegue.

**Qué se difiere (sin consumidor real hasta Stage 14):**
- **TODA mutación** (POST/PUT/DELETE de brains, hot-reload de config, start/stop
  de canales). El consumidor real de mutación en vivo es el builder. Además el
  registry del router se construye en **boot** (`RegisterBrain`/`Route` son
  boot-time); mutar en vivo bajo carga es un cambio de concurrencia/lifecycle
  grande, no un handler.
- **Navegación de conversaciones** (necesita query nuevo en el Store; feature del
  builder).
- **Salud detallada per-provider** (no se trackea; necesitaría probing nuevo).
- Auth/token, RBAC, TLS, exposición a red, UI (Stage 14).

**Por qué solo-lectura de-riesga la etapa (insight de seguridad):** no muta
estado → **el cálculo de seguridad loopback-sin-auth queda IGUAL que `/metrics`,
ya aceptado en Stage 12.** Cero superficie de ataque nueva. La **mutación** es lo
que cambiaría el cálculo; diferirla mantiene la decisión de Stage 12 correcta por
construcción.

---

## (b) Espacio de diseño (abierto, no cerrado)

**SEGURIDAD / AUTH (lo central):**
- Read-only loopback sin auth = mismo perfil que `/metrics`. Respuestas
  secret-free (config ya lo es; defensa en profundidad: aserción de no-secretos
  en los handlers). `readHeaderTimeout` ya cubre Slowloris.
- **El disparador de auth es la MUTACIÓN, no la lectura.** Cuando llegue mutación
  (Stage 14), auth pasa a esencial — un token (el builder puede correr como
  proceso aparte y/o el operador puede bindear más allá de loopback). Se nombra
  como trigger, no se construye ahora.
- Lectura → loopback ok; mutación → auth obligatorio. **Empezar read-only es la
  decisión que mantiene el requisito bajo.**

**FORMA:**
- REST stdlib sobre el `http.ServeMux` que ya existe (Go 1.26 → method-pattern
  routing nativo). **Cero deps nuevas** — Korvun no ha añadido framework web;
  boring-by-default dice no empezar por un puñado de GETs. `net/http` = Layer-1.
- **Seam:** handlers dependen de interfaces **reader** pequeñas
  (`BrainSummaries()`, `ChannelSummaries()`), no de tipos concretos de
  `router`/`brain`. `App` las implementa. Mismo patrón Store/Metrics.

**COEXISTENCIA:**
- Mismo mux, mismo servidor. `/healthz` + `/metrics` intactos. API bajo `/api`.
- **Acoplamiento a decidir en el ADR:** el servidor solo existe si observability
  está on. ¿La API cabalga el servidor de observability (off si observability off
  — lo simple/boring) o tiene enablement propio? Mínimo: cabalga el existente.

**CONFIG:** default-on loopback (igual que observability) **porque es read-only**
(no muta → el default-on no cambia el cálculo). Si fuera mutación, opt-in.

**REVERSIBILIDAD / BLAST RADIUS:** additive — nuevo paquete `internal/controlapi`
(handlers + interfaces reader), `App` retiene config/arma summaries durante
`wire()`, montaje en `Build()`. Dominio intacto, router intacto.

```
HOY:  ServeMux ── /healthz (liveness)        [loopback 127.0.0.1:2112, sin auth]
                └─ /metrics (prom)

CORTE MÍNIMO (additive, MISMO servidor):
      ServeMux ── /healthz
                ├─ /metrics
                └─ /api/brains, /api/channels   (GET, read-only, secret-free)
                       ▲ handlers ── interfaces reader (no el dominio concreto)
      mutación (POST/PUT/DELETE) ─▶ DIFERIDA a Stage 14 (builder = consumidor real;
                                     ahí auth se vuelve esencial)
```

**NOT in scope:** mutación, hot-reload, gestión en vivo de brains/canales,
navegación de conversaciones, salud per-provider, auth/RBAC/TLS, exposición a
red, UI, deps nuevas.

---

## (c) ¿Aportaron las skills?

- **`/office-hours`: marginal** — mal encaje producto/startup para arquitectura
  interna. Único valor: forzar la alternativa "solo-lectura primero / diferir
  mutación" y el challenge de premisa. Confirmó.
- **`/plan-eng-review`: la que aportó.** El Step 0 scope-challenge + el lente de
  seguridad son el marco. Insight más fuerte: **"read-only mantiene intacto el
  cálculo de seguridad de Stage 12; la mutación es lo que lo rompe, así que
  diferir mutación ES la decisión de seguridad."**
- **Net:** las skills no cambiaron la hipótesis; la estresaron y aguantó.

---

## (d) Decisión a confirmar (NO ADR todavía)

1. **Read-only ahora + mutación diferida a Stage 14** (RECOMENDADA).
2. Read-only + UNA mutación concreta ahora (solo si hay una con consumidor real
   HOY; exigiría auth desde el principio y tocaría el lifecycle boot-time del
   router).
3. Diferir TODA la control API a Stage 14 (como el bus; contra: desperdicia el
   valor de lectura barato y ya habilitado por el seam de Stage 12).
