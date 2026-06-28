# Korvun — Camino a la Versión 1 (producto fiable y potente)

## Propósito

Este documento es el contraste entre el **MVP/beta** (lo que demuestra que la
arquitectura funciona) y la **V1** (lo que la hace fiable en producción real,
del Raspberry Pi a la nube). El MVP prueba que las piezas encajan; la V1 las
endurece, las cablea y las hace operables por alguien que no es su autor.

Es un **documento vivo**: reúne en un solo sitio todo lo que los ADRs y el
HANDOFF han ido marcando como *diferido*, *fuera de alcance v1* o *follow-up*,
más las piezas de robustez que un producto de verdad necesita.

---

## 1. Funcionalidad del producto que aún no existe

> Cada entrada se marca con: **qué falta**, **qué ADR/etapa la cubre**, y
> **por qué se difirió**.

- **✓ HECHO — Selector pre-dispatch (privacidad).** Decide qué modelos entran al
  fan-out ANTES de llamarlos. **Cerrado en Stage 6** (ADR-0015): `policy.SelectModels`
  filtra el `[]model.Model` por una `Sensitivity` declarada **por-Brain** + un
  catálogo de atributos en el wiring. El reframe clave: **no se tocó el `Envelope`**
  — un campo de sensibilidad por-mensaje no tiene escritor correcto hoy e inferirlo
  está prohibido (ADR-0012 §5e), así que se difiere (aditivo) al builder no-code.
  Demostrado por `cmd/demo-selector`.
  - *Pendiente futuro (aditivo):* el campo tipado `Envelope.Sensitivity` + la
    interfaz `Selector` por-mensaje, cuando exista un escritor; el **filtro de
    coste** (`CostTier` en el catálogo), misma maquinaria.

- **✓ HECHO — Coordinator secuencial (fail-over que SÍ ahorra coste).** Hermano del
  fan-out: no llama a Groq si Ollama ya acertó. **Cerrado en Stage 6** (ADR-0016):
  `sequential.Coordinator` hace dispatch serial parando en el primer éxito, reusando
  el primitivo compartido `fanout.CallOne` (no duplica la disciplina de
  panic+latencia+`%w`) y devolviendo el mismo `*fanout.Result`. Demostrado por
  `cmd/demo-sequential`.

- **Coste con estado (budget diario por Brain).**
  - *Qué falta:* el contador de budget con estado y su política de corte.
  - *Estado:* el bloqueo de persistencia **se ha levantado** (Stage 9 entregó el
    seam `Store` durable), pero el budget con estado en sí sigue **fuera de
    alcance** — es trabajo aditivo sobre la capa de persistencia ya existente.

- **Consenso sobre prosa / equivalencia semántica.**
  - *Qué falta:* consenso semántico real (parafraseo), no solo sobre salida
    estructurada normalizable.
  - *Por qué se difirió:* hoy el consenso opera sobre output estructurado; el
    semántico es trabajo futuro.

- **`AsModel` adapter.** El adaptador `Policy → model.Model`.
  - *Qué falta:* implementar el adaptador (no está en master).
  - *Cobertura:* ADR-0012 §1/§6 (anotado como **diferido**); **Stage 7 (Brain)**.
  - *Por qué se difirió:* es la conveniencia lossy *secundaria* (el camino primario
    es `Policy.Apply` sobre un `*fanout.Result`) y no tiene consumidor hasta que el
    Brain lo use; un adaptador lossy sin consumidor no se valida bien.

- **Streaming (`StreamingModel`).**
  - *Qué falta:* implementación; la interfaz `Model` está preparada pero no
    implementada.

- **Embeddings / tool-use / vision.** Familias de modelo más allá de chat.
  - *Cobertura:* nombradas en ADR-0009 como extensiones futuras.

- **WhatsApp y otros canales.**
  - *Cobertura:* ADR-0002 difirió WhatsApp.
  - *Por qué se difirió:* el MVP arranca con un canal; la V1 potente querría más
    de uno.

---

## 2. Robustez y operabilidad (lo que hace un producto, no un prototipo)

- **✓ HECHO — CI/CD multiplataforma en verde.** El pipeline (`quality.yml`)
  corre en los 3 SO (`quality` ×3), genera SBOM y cross-compila las 6
  combinaciones (linux/windows/darwin × amd64/arm64) — 10 jobs, todos verdes en
  master (`548909d`, sesión 2026-06-20). Fixes: `.gitattributes` fuerza LF
  (lint limpio en Windows), guard de cobertura sin `pipefail`/SIGPIPE (macOS),
  job CodeQL retirado (code scanning requiere GHAS en repo privado; SAST sigue
  cubierto por `gosec` + `govulncheck`).
  - *Pendiente futuro:* reañadir CodeQL si el repo pasa a público o se habilita
    GHAS; proteger la rama `master` (ruleset web).

- **✓ HECHO (parcial) — Persistencia.** **Cerrado en Stage 9**
  (`docs/stages/STAGE-09.md`, ADR-0018 + ADR-0019): la capa de storage existe —
  el seam `conversation.Store` append-only + atómico, con `MemStore` in-memory y
  `SqliteStore` durable (SQLite vía `modernc.org/sqlite` Go-puro, escritor único,
  transacción por grupo crash-consistente, boot-fatal-vs-stateless). Es la
  **primera dependencia externa** más allá de `go-telegram/bot`, adoptada tras el
  test de cuatro ejes + gate de dependencia (ganó el eje de cross-compile).
  - *Qué cubre hoy:* memoria de conversación durable. *Aditivo detrás del mismo
    seam (no en v1 todavía):* budget con estado, historial analítico/queryable,
    Postgres/multi-nodo, compactación/retención (el `ts`+`seq` ya viaja en cada
    fila → query aditiva, no migración).

- **✓ HECHO — Observabilidad.** **Cerrado en Stage 12** (`docs/stages/STAGE-12.md`,
  ADR-0020, merge `cee4a20`). Logs estructurados estandarizados en los funnels +
  un seam `Metrics` (interfaz en el dominio, impl Prometheus en
  `internal/metrics/prom`) + un admin `http.Server` (`internal/httpserver`,
  default-on, loopback `127.0.0.1:2112`) sirviendo `/metrics` (seis series
  `korvun_*`, entre ellas la métrica de saturación `DroppedCount` que ADR-0008
  §4c dejó como dependencia dura de esta etapa, expuesta vía pull `NewCounterFunc`)
  y `/healthz` (liveness-only). Trazas **diferidas**; dashboards/alerting son del
  lado operador. El control API que montará en el mismo mux es Stage 13.

- **✓ HECHO — El ensamblaje real (`main.go`).** **Cerrado en Stage 11**
  (ADR-0017, `docs/stages/STAGE-11.md`): el binario `korvun` lee un config JSON
  (`configs/korvun.example.json`) y cablea canal → router → brain → canal en un
  proceso de larga duración. El router ahora posee el pump inbound;
  `Orchestrator.coord` es la interfaz `brain.Coordinator` (fan-out o secuencial
  desde config); `internal/config` (parse+validate) + `internal/app` (wiring +
  getMe de boot + lifecycle) + `cmd/korvun` (main fino). Los siete `cmd/demo-*`
  borrados — el binario los reemplaza.

- **Configuración.** Un producto self-hosted necesita config por fichero (los
  perfiles `edge.yaml` para Raspberry Pi / `cloud.yaml` ya previstos), no
  variables de entorno sueltas.

- **Manejo de errores de producción.** Reintentos con backoff, circuit breakers
  para proveedores caídos, degradación elegante. Hoy los adapters mapean errores
  pero la política de reintentos vive en el consumidor, que aún no existe.

- **Seguridad.** Gestión de secretos más allá de env vars (la V1 querría
  integración con un secret manager); rate limiting propio; validación de entrada
  en los canales.

---

## 3. Lo que hace a Korvun USABLE por terceros

- **El builder no-code (Stage 14).** El diferenciador de cara al usuario:
  expresar políticas de forma declarativa y visual. La V1 potente lo necesita;
  el MVP solo tiene las políticas en código.
  - **✓ HECHO — Fase 1 (fundamentos, read-only). Cerrada**
    (`docs/stages/STAGE-14.md`, ADR-0023 + ADR-0024): el **bus de eventos**
    (`internal/bus`, Stage 10 absorbida y cerrada correctamente — construido cuando,
    y solo cuando, llegó un consumidor que lo valida) + el **live-view read-only**
    (`internal/liveview`: SSE `GET /api/events` + UI `/ui` embebida con `go:embed`).
    El bus despierta en `app` (real solo si observability ON), `onRouterError`
    publica MessageDropped/HandleFailed, drops del bus y del SSE como métricas pull.
    F2 resuelto por desacople; frames secret-free por construcción. go.mod sigue en
    3 deps (SSE stdlib, UI go:embed).
  - *Pendiente — Fase 2+ (el builder propiamente dicho, futuros ADRs):* **mutación**
    del wiring (add-only o reload-and-rebuild, **NUNCA edición granular en vivo** —
    el registro del router es de boot) + **AUTH** (el disparador de la mutación;
    read-only es lo que mantiene válido el loopback-sin-auth hoy) + la UI de edición
    + el lienzo visual (donde React/TS/Vite gana su token). NATS/persistencia/replay
    de eventos siguen fuera (el `Bus` es el punto de entrada del futuro `natsBus`).

- **✓ HECHO (corte read-only) — Control API (Stage 13).** **Cerrado**
  (`docs/stages/STAGE-13.md`, ADR-0022, `ac88478`): `internal/controlapi` sirve
  `GET /api/brains` (brains resueltos, incl. los modelos que sobrevivieron al
  selector de privacidad) + `GET /api/channels` en el mismo mux del
  `internal/httpserver` de Stage 12, bajo `/api`, read-only, secret-free, additive
  (router intacto). Read-only mantiene el cálculo loopback-sin-auth de Stage 12
  intacto — diferir la mutación ES la decisión de seguridad.
  - *Diferido a Stage 14 (mutación, el consumidor real es el builder):* gestionar
    brains/políticas/canales en caliente (POST/PUT/DELETE), hot-reload de config,
    navegación de conversaciones, salud per-provider, auth/token (el trigger de la
    mutación), TLS, exposición a red, `GET /api/info`. El seam `Reader` sobrevive:
    en Stage 14 su impl pasa de snapshot de boot a vista viva sin cambiar la
    interfaz.
  - *Follow-up P3 (F1):* los agent brains reportan `dispatch`/`policy` inertes en
    `/api/brains` — decisión de forma de API para agents, diferida (ADR-0022 §2).

- **Documentación de producto y presentación del repo (Stage 16).**
  - **✓ ADELANTADO (rama `chore/repo-hygiene`, ejecutado antes de Stage 12) —
    presentación profesional del repo.** Por decisión de Chano, esta parte salió
    del orden de roadmap: ~~README con badges (CI, Go Report Card, Go version,
    License, OpenSSF Scorecard, release), `SECURITY.md`, `CONTRIBUTING.md`,
    `CODEOWNERS`, plantillas `.github/` (issues + PR), OpenSSF Scorecard
    (`scorecard.yml`), mejoras `.gitignore`.~~ Hecho en `chore/repo-hygiene`,
    pendiente de revisión + merge. **No reintroducir en Stage 16** (se deja
    anotado aquí para no duplicarlo por error). Badges dependen de billing de
    Actions desbloqueado + repo público — ver `docs/HANDOFF.md`.
  - *Resto del alcance de Stage 16 (SIGUE PENDIENTE):* documentación
    developer-facing completa — quickstart, instalación por SO (incluido
    Raspberry Pi), perfiles `edge`/`cloud`, referencia de config, guía de
    extensión (nuevo canal, nuevo agente), referencia de la control API, guía del
    builder no-code; GoReleaser; skill `/cso`. Sin esto, nadie que no seas tú
    puede usarlo.

- **Empaquetado y distribución (Stage 15).** Binarios por plataforma,
  instaladores, contenedores.

---

## 4. Multi-brain y agentes (la potencia)

- **Registro multi-brain (Stage 7).** Límites de brains concurrentes, recursos
  por brain (cola acotada + workers por ADR-0003), un número concreto de brains
  concurrentes soportados.

- **Agentes (Stage 8). CERRADA** (`docs/stages/STAGE-08.md`, ADR-0021). Un
  `AgentBrain` (B2 — `brain.Brain` hermano del Orchestrator) corre un bucle
  acotado modelo→herramienta→modelo de un solo modelo. El primer corte es un
  *slice de validación de seam*: el `Tool` seam (`internal/tool`, leaf) + tres
  herramientas PURAS (`time`/`echo`/`calc`; `calc` es un parser propio acotado,
  sin `eval` — decisión de seguridad). Tool-use por prompt-protocol (D2, cero
  cambio a `model.Model`); function-calling nativo diferido como interfaz hermana
  `ToolCallingModel`. Invariantes de seguridad (max-iter duro, timeout heredado
  del `Handle`, per-tool timeout, tool-failure como observación, model-failure →
  fallback), stateless con test `-race` (fake tool con estado), persistencia
  solo-par-final. Diferido: herramientas peligrosas (shell/HTTP/fs/ERP), agentes
  multi-model, planning, multi-agente, native function-calling. Concurrencia
  pesada — pasó por `/review` (1 P2 + 3 P3 arreglados).

- **✓ HECHO — Bus de eventos (Stage 10, absorbido como Stage 14 Fase 1a).**
  Diferido conscientemente como YAGNI tras el encuadre (`/office-hours` +
  `/plan-eng-review`, 2026-06-28) y **construido cuando llegó su consumidor real**
  (el live-view SSE de la Fase 1b). **Cerrado** (`internal/bus`, ADR-0023,
  `464f8c2`): pub/sub in-process best-effort, non-blocking, drop+contador por
  suscriptor lento, panic-safe, `-race` bajo `brainWorkers>1`; + un hook aditivo
  nil-safe `WithEventPublisher` en el router. La disciplina se cumplió: ningún seam
  sin consumidor que lo valide. El `Bus` interface es el punto de entrada del
  futuro `natsBus` (NATS fuera). Sketch en `docs/notes/bus-design-sketch.md`.

> **Orden de trabajo (actualizado 2026-06-28):**
> **Stage 13 (control API) ✓ → Stage 14 Fase 1 (fundamentos: bus + live-view) ✓
> → Stage 14 Fase 2 (builder: mutación + auth + UI edición + lienzo) O Stage 15
> (packaging) → Stage 16 (hardening + release).**
> Stages 0–9, 11, 12, 13 y **14 Fase 1** cerradas. Stage 10 (bus) cerrada dentro
> de 14 Fase 1a. Próximo = decidir 14 Fase 2 vs 15.

---

## 5. Criterios de "esto ya es V1"

> Una checklist honesta para saber cuándo parar de llamarlo beta.

- [x] Un mensaje real entra por un canal, se enruta, varios modelos responden,
      una política decide, y la respuesta vuelve — todo en un binario real
      (`main.go`), no en demos. **COMPLETO (Stage 11).**
      - *Verificado EN VIVO (2026-06-21):* el operador arrancó `cmd/korvun` con un
        config real (Telegram polling + brain con Ollama `llama3.2:1b` local +
        Groq `llama-3.3-70b-versatile` cloud + `PriorityReducer`), escribió
        "hola" al bot por Telegram y recibió la respuesta del modelo de vuelta en
        el chat — conversación completa, incluido cambio de idioma. El round-trip
        end-to-end (Telegram → fan-out → política → respuesta) corrió por el
        binario real, no por un demo.
      - *También observado en vivo:* el **contrato de fallback** (ADR-0014 §3)
        cuando los modelos fallaban (antes de corregir el `model_id`), y luego el
        camino feliz tras corregirlo. Los demos están borrados; el binario es el
        producto.
      - *Hallazgos de la prueba en vivo (para hardening, Stage 16):*
        - **(a)** el `getMe` de boot tiene un **timeout fijo de 5s** (interno a
          `bot.New` de go-telegram/bot) y dio `context deadline exceeded`
          intermitente en redes lentas — candidato a hacerlo
          configurable / con reintento en la etapa de hardening.
        - **(b)** documentar de forma inequívoca en el config de ejemplo que
          `token_env` / `api_key_env` esperan el **NOMBRE** de la variable de
          entorno, no el valor del secreto — un operador podría confundirse y
          pegar el secreto en el fichero (rompería ADR-0010).
- [x] Persiste estado entre reinicios. **COMPLETO (Stage 9).**
      - *Entregado:* memoria de conversación durable keyed por
        `channel::conversation.id`, que sobrevive a reinicios — incluido un
        **apagado limpio** (el último turno se persiste sobre un contexto
        desacoplado de la cancelación del shutdown). ADR-0018 (interfaz
        append-only + `MemStore`) + ADR-0019 (`SqliteStore` durable con
        `modernc.org/sqlite` Go-puro, escritor único `MaxOpenConns(1)`,
        transacción por grupo atómica + crash-consistente).
      - *Alcance honesto:* persiste **memoria de conversación**, no aún budget
        con estado, historial analítico, ni Postgres (todos aditivos detrás del
        mismo seam `Store`, ver Sección 1 y 2).
- [x] Es observable (sé qué está pasando dentro sin leer el código). **COMPLETO
      (Stage 12).**
      - *Entregado:* logs `slog` estandarizados en los funnels + seis métricas
        Prometheus en `/metrics` (mensajes procesados, histograma de latencia por
        provider, fallos por provider, errores de router por kind, mensajes
        dropeados por canal, turnos persistidos) detrás de un seam `Metrics`, más
        `/healthz` liveness-only. Admin server default-on en loopback, con forma
        de seam para el control API de Stage 13. ADR-0020.
      - *Alcance honesto:* sin trazas distribuidas (diferidas), sin dashboards ni
        alerting (lado operador), sin auth/TLS en el admin server (Stage 13).
- [ ] Lo configura alguien por fichero, sin recompilar.
- [ ] Lo instala alguien que no soy yo, en su máquina, siguiendo la documentación.
- [ ] Aguanta un proveedor caído sin caerse.
- [ ] Las políticas se expresan sin tocar Go (builder no-code).

---

> **Nota:** este documento es **VIVO**. Cada vez que un ADR difiera algo "a
> producción" o "fuera de v1", añádelo a la sección correspondiente. Se revisa al
> cerrar cada etapa junto con el HANDOFF.
