# Korvun — Documento Maestro del Proyecto

> **Estado del documento:** especificación inicial completa (v1).
> **Naturaleza:** plan de construcción detallado, etapa por etapa, fase por fase.
> **Idioma de trabajo:** español. Código y comentarios: inglés (estándar de industria).

---

## Nombre del producto

**Kernel for Orchestrated Routing — Versatile Unified Nodes → Korvun**

El producto se llama **Korvun**. El nombre es a la vez un acrónimo descriptivo de la
arquitectura y una palabra única y pronunciable, elegida por estar libre de colisiones
en los registros de paquetes (npm, PyPI, crates.io), sin repositorios homónimos
relevantes en GitHub y sin productos de IA existentes con ese nombre.

**Significado en inglés (acrónimo):** Kernel for Orchestrated Routing — Versatile
Unified Nodes. Núcleo (kernel) que orquesta el enrutado de peticiones sobre nodos
unificados y versátiles —los cerebros, modelos y agentes que conviven en un solo
binario.

**Significado en español:** Núcleo de Orquestación y Routing sobre Nodos Versátiles y
Unificados. "Korvun" condensa la idea central del producto: un único núcleo ligero que
dirige y enruta el tráfico de IA —decidiendo a qué modelo, dónde y cómo— a través de una
malla de nodos (canales, cerebros, modelos y agentes) que funcionan unificados bajo el
mismo binario, adaptables tanto a una Raspberry Pi como a la nube.

**Convenciones derivadas del nombre:** módulo Go `github.com/<org>/korvun`; binario y
comando `korvun` (`korvun up`, `korvun serve`, `korvun --config edge.yaml`). En todo este
documento, el identificador de proyecto/binario es `korvun`.

**Pendiente de verificación externa:** antes de comprometer el nombre como marca, conviene
comprobar la disponibilidad de dominios (`korvun.com`, `.ai`, `.dev`) y realizar una
búsqueda de marca registrada (EUIPO en la UE). La verificación realizada cubre registros
de paquetes, repositorios y productos existentes, no el registro legal de marca.

---

## 0. Resumen ejecutivo

Korvun es un panel de control visual sobre un motor de orquestación de IA dirigido por
políticas, empaquetado como software ultraeficiente y multiplataforma.

Unifica en un solo producto tres categorías que hoy están separadas en el mercado:

1. **Pasarela universal de mensajería** (territorio de OpenClaw): conecta cualquier canal
   —WhatsApp, Telegram, Teams, Discord, Slack, Signal, y cualquier web/API propia vía un
   protocolo genérico— a través de una forma de mensaje normalizada.
2. **Enrutamiento inteligente multi-modelo** (territorio de OpenRouter y más allá): una
   sola puerta a modelos locales y de nube, con decisiones automáticas.
3. **Orquestación multi-cerebro**: varios orquestadores ("cerebros") conviven, y cada uno
   coordina varios modelos en paralelo y dirige varios agentes concurrentes.

La pieza que lo convierte en la mejor opción del mercado y lo hace monetizable es el
**motor de políticas de despacho**: routing consciente de privacidad y coste (lo sensible
se queda en modelos locales y nunca sale de la máquina; lo trivial va al modelo más
barato; lo crítico a varios modelos en consenso), todo configurable sin código desde un
builder visual.

**Promesa central y tensión de diseño:** máxima potencia con mínimos recursos. El mismo
binario debe correr digno en una Raspberry Pi y escalar en la nube, cambiando solo piezas
de I/O por configuración.

---

## 1. Qué es y qué no es

### Es

- Un binario único (núcleo en Go) que actúa como gateway + router + orquestador.
- Un panel visual (aplicación de escritorio nativa y/o web local) para componer canales,
  cerebros, modelos, agentes y políticas sin escribir código.
- Self-hosted por defecto: tus datos no salen salvo que una política lo autorice.
- Multiplataforma real: Linux, Windows y macOS; arquitecturas x86-64 y ARM64.

### No es (al menos no en beta)

- No es un proveedor de modelos: orquesta modelos de terceros y locales, no entrena.
- No es un clon de un solo competidor: abarca varias capas a la vez.
- No pretende soportar todos los canales en beta (eso es objetivo de la 1.0).

---

## 2. Propuesta de valor y monetización

### Diferenciador defendible (moat)

Ni OpenClaw (solo canales) ni OpenRouter (solo nube, los datos salen sí o sí) cubren el
routing híbrido local/nube consciente de privacidad y coste, y ninguno lo expone con un
builder visual sobre un único binario eficiente. Esa combinación es el foso.

### Modelo de negocio: open-core

| Capa | Licencia | Contenido |
|------|----------|-----------|
| Núcleo | Open source, self-hosted gratis | Canales base, multi-cerebro, routing básico, motor de políticas, SQLite, binario único |
| Pro / Empresa | De pago | Políticas avanzadas (privacidad/cumplimiento, consenso), analítica de ahorro, conectores premium, soporte |
| Cloud gestionado | Suscripción | Despliegue gestionado, alta disponibilidad, observabilidad gestionada |
| Marketplace (futuro) | Comisión | Cerebros, agentes y adaptadores de la comunidad |

El núcleo gratuito genera adopción y comunidad; el dinero está en la capa empresarial
(privacidad/coste/consenso/analítica) y en el modo gestionado.

---

## 3. Arquitectura

### 3.1 Flujo de un mensaje

```
Humano
 │ (formato nativo del canal)
 ▼
[ Adaptador de canal ] ──► Envelope (mensaje canónico, multimodal)
 ▼
[ Router / gateway core ]
 ▼
[ Orquestador / cerebro ] ── consulta ──► [ Motor de políticas de despacho ]
 │ decide: ¿local o nube?
 │ ¿1 modelo o consenso? ¿qué agentes?
 ▼
[ Pool de modelos ] (local + nube, en paralelo)
[ Pool de agentes ] (concurrentes, con herramientas)
 ▼
Envelope de respuesta
 ▼
[ Adaptador de canal ] ──► (formato nativo)
 ▼
Humano
```

### 3.2 Capas (mapa conceptual)

1. **Dominio (`envelope`)** — la unidad universal de mensaje. El resto del sistema solo
   habla este lenguaje.
2. **Canales (`channel`)** — interfaz `Receive` / `Send` / `Manifest`; adaptadores
   concretos + adaptador Webhook genérico.
3. **Router (`router`)** — enruta Envelopes entre canales y cerebros.
4. **Modelos (`model`)** — abstracción de proveedor LLM (local y nube) con fan-out
   paralelo.
5. **Motor de políticas (`policy`)** — corazón del producto: decide dónde y cómo se
   despacha cada petición (routing privacidad/coste, consenso). Routing y consenso son
   políticas del mismo motor, no sistemas separados.
6. **Orquestadores / cerebros (`brain`)** — varios coexisten; cada uno usa el motor de
   políticas y gobierna un pool de modelos y agentes.
7. **Agentes (`agent`)** — trabajadores concurrentes que ejecutan tareas y usan
   herramientas, dirigidos por un cerebro.
8. **Persistencia (`store`)** — SQLite embebido por defecto; Postgres por config.
9. **Bus de eventos (`bus`)** — en memoria por defecto; NATS/Redis para distribuido.
10. **API de control (`api`)** — superficie que consume el panel visual.
11. **Panel / builder (`ui`)** — interfaz visual; fuera del camino caliente; compila a la
    misma config ligera que lee el núcleo.

### 3.3 Principio de piezas conmutables

Persistencia y bus se seleccionan por configuración sin tocar el núcleo. Eso da
portabilidad sin perder potencia: en una Pi corre con SQLite + bus en memoria; en la nube,
con Postgres + NATS. Mismo binario.

---

## 4. Stack tecnológico

> Todas las versiones concretas de librerías/frameworks de esta sección deben verificarse
> con **Context7** antes de adoptarse en cada etapa.

### 4.1 Núcleo (backend)

**Lenguaje: Go (1.22+).** Razón: cross-compila a un binario estático único para
Linux/Windows/macOS y x86-64/ARM64; arranque en milisegundos; bajo consumo de RAM;
concurrencia barata (goroutines) ideal para muchos canales/modelos/agentes simultáneos.

**Dependencias:** stdlib siempre que sea razonable. Cada dependencia externa requiere
justificación documentada (ADR) y verificación Context7.

### 4.2 Capa visual (frontend / builder)

La parte visual tiene dos formas de ejecución para garantizar "corre en todos los SO":

- **App de escritorio nativa: Wails.** Empaqueta el frontend web + el binario Go en una
  app nativa para Linux, Windows y macOS, sin cargar un Chromium completo (usa el WebView
  del sistema). Coherente con el ethos de eficiencia. (Alternativa evaluada: Tauri (Rust).
  Se elige Wails por afinidad con el núcleo Go.)
- **Panel web local:** el mismo binario sirve la UI en `localhost`, accesible desde
  cualquier navegador en cualquier SO. Útil para servidores sin entorno gráfico
  (Pi/cloud).

**Tecnología del frontend:**

- React + TypeScript (strict) con Vite como bundler.
- Tailwind CSS para estilos.
- React Flow para el editor de nodos del builder visual.
- Vitest + React Testing Library (unit/componente) y Playwright (e2e).

> Verificar con Context7 las versiones y APIs de Wails, React, Vite, Tailwind, React Flow
> y Playwright al inicio de la etapa del panel.

### 4.3 Datos e infraestructura

- **Persistencia:** SQLite (embebido, por defecto) → PostgreSQL (por config).
- **Bus de eventos:** in-memory (por defecto) → NATS o Redis Streams (distribuido).
- **Modelos:** proveedor local vía API compatible OpenAI / Ollama; proveedores de nube
  (Anthropic, OpenAI, etc.) tras una interfaz común.
- **Configuración:** ficheros YAML (`edge.yaml`, `cloud.yaml`, …) + variables de entorno
  (principios 12-factor).

---

## 5. Estándares de programación (nivel "big tech")

Estos estándares son obligatorios y se verifican en CI.

### 5.1 Estilo y calidad de código

- **Go:** `gofmt` + `goimports` obligatorios; linter `golangci-lint` (incluye `govet`,
  `staticcheck`, `errcheck`, `gosec`). Seguir Effective Go y Google Go Style Guide.
  Errores envueltos con `%w`; `context.Context` en toda operación cancelable; sin estado
  global mutable; sin `panic` en rutas normales.
- **TypeScript:** modo `strict`; ESLint + Prettier; sin `any` implícito; componentes con
  tipos de props explícitos.

### 5.2 Testing

- **TDD innegociable:** los tests se escriben antes de la implementación y se confirman en
  rojo.
- **Go:** tests table-driven; ejecución con `-race`; cobertura mínima ≥ 85% en paquetes de
  núcleo (`policy`, `router`, `envelope`, `brain`).
- **Frontend:** unit/componente con Vitest; e2e críticos con Playwright.
- Tipos de test por nivel: unitarios → integración → contrato (entre capas) → end-to-end.
- **Puerta de calidad:** ninguna etapa se cierra sin `make quality` en verde (lint + vet +
  tests + cobertura) sobre toda la suite, no solo lo nuevo.

### 5.3 Control de versiones y revisión

- Conventional Commits (`feat:`, `fix:`, `test:`, `docs:`, `refactor:`…).
- SemVer para versionado.
- Ramas protegidas; PRs con revisión obligatoria; commits firmados.

### 5.4 CI/CD

- GitHub Actions con matriz multiplataforma: `{linux, windows, macos} × {amd64, arm64}`.
- Pipeline: lint → vet → test -race → cobertura (con umbral) → build cross-compile → SAST
  (`gosec`, CodeQL) → escaneo de vulnerabilidades (`govulncheck`, Dependabot) → generación
  de SBOM.

### 5.5 Documentación

- godoc en todo símbolo exportado.
- ADR (Architecture Decision Records) en `docs/adr/` para cada decisión relevante
  (incluida cada dependencia externa).
- Documento de cierre por etapa en `docs/stages/`.
- Este documento maestro se mantiene actualizado.

### 5.6 Seguridad y operación

- Secretos solo por entorno/gestor de secretos; nunca en el repo.
- Validación de entrada en todos los bordes; TLS en endpoints expuestos.
- Principios 12-factor.
- Observabilidad: logging estructurado (`slog`), métricas Prometheus, trazas
  OpenTelemetry.

---

## 6. Metodología de trabajo

### 6.1 Ciclo obligatorio por fase (no solo por etapa)

1. **Context7** → consultar documentación versión-específica de toda librería implicada.
2. **Tests primero** → escribir la suite que define el contrato de la fase. Confirmar en
   rojo.
3. **Implementación** → solo el código necesario para poner los tests en verde.
4. **Calidad** → `make quality` sobre TODA la suite, en verde.
5. **Documentación** → actualizar docs de la etapa, ADRs y este documento.

### 6.2 Granularidad

- **Etapa** = bloque grande con un objetivo de producto.
- **Fase** = sub-bloque coherente dentro de la etapa.
- **Tarea** = trozo pequeño y atómico dentro de una fase (lo que se trabaja "de un tirón" y
  se entiende del todo).

No se avanza de fase sin cerrar el ciclo. No se avanza de etapa sin todas sus fases
cerradas.

### 6.3 Regla innegociable de Context7

Antes de implementar cualquier cosa que use una biblioteca, framework, SDK o API externa,
se consulta su documentación actualizada y versión-específica a través de Context7. Está
prohibido programar de memoria contra una API externa.

---

## 7. Desglose completo de etapas

### ETAPA 0 — Fundamentos del proyecto

**Objetivo:** dejar el repositorio, el tooling y las puertas de calidad listos antes de
escribir lógica de negocio.

- **Fase 0.1 — Estructura y módulo.** Inicializar repo Git; crear `go.mod`
  (`module github.com/<org>/korvun`); estructura de carpetas (`internal/`, `cmd/`, `docs/`,
  `ui/`); `.gitignore`; README y este documento. *Tests:* test de humo que compila el
  módulo vacío (`go build ./...`).
- **Fase 0.2 — Tooling de calidad.** Makefile (`quality`, `test`, `lint`, `cover`,
  `build`); configurar `golangci-lint`; `gofmt`/`goimports`. *Tests:* `make quality` corre
  y pasa sobre el esqueleto.
- **Fase 0.3 — CI/CD.** Workflow de GitHub Actions con matriz multiplataforma; gates de
  lint/test/cobertura; SAST y `govulncheck`; SBOM. **Context7** (acciones y versiones).
  *Tests:* el pipeline pasa en verde en las tres plataformas.
- **Fase 0.4 — Convenciones.** Plantilla de ADR; plantilla de doc de etapa; configurar
  Conventional Commits; plantilla de PR.
- **Cierre:** `docs/stages/STAGE-00.md`.

### ETAPA 1 — Dominio: Envelope canónico

**Objetivo:** definir la unidad universal de mensaje, multimodal y agnóstica al canal.

- **Fase 1.1 — Tipos base.** `Direction`, `PartType`, `Participant`, `Part`, `Envelope`.
  *Tests:* construcción con valores por defecto; `Meta` inicializado.
- **Fase 1.2 — Identidad y construcción.** `NewID()` (único, ordenable por tiempo, sin
  dependencias); `New()`; builder encadenable (`AddText`, `AddMedia`). *Tests:* unicidad de
  `NewID` (1000 sin colisión); builder añade partes correctas.
- **Fase 1.3 — Validación y serialización.** `Validate()`; round-trip JSON. *Tests:* casos
  de error (canal vacío, dirección inválida, sin partes, texto vacío, media sin fuente);
  round-trip preserva todos los campos.
- **Cierre:** `docs/stages/STAGE-01.md`, cobertura ≥ 90%.

### ETAPA 2 — Canales: interfaz + adaptadores

**Objetivo:** poder recibir y enviar Envelopes desde canales reales, con un adaptador
genérico que convierta cualquier API web JSON en un canal.

- **Fase 2.1 — Interfaz Channel.** Definir `Receive` / `Send` / `Manifest`; tipo
  `Manifest` (capacidades: texto, audio, imagen, botones); registro de canales. *Tests:*
  doble de prueba (mock) que implementa la interfaz; registro/baja de canales.
- **Fase 2.2 — Adaptador Webhook genérico.** Servidor HTTP que recibe JSON → Envelope;
  mapeo configurable de campos; envío saliente vía HTTP POST. **Context7** (router HTTP
  elegido). *Tests:* payload JSON entrante produce el Envelope esperado; payloads
  malformados se rechazan; envío saliente forma la petición correcta.
- **Fase 2.3 — Adaptador Telegram.** Integración con Bot API de Telegram (long-polling o
  webhook). **Context7** (Telegram Bot API / cliente Go). *Tests:* traducción de update de
  Telegram → Envelope y viceversa (con fixtures, sin red real).
- **Cierre:** `docs/stages/STAGE-02.md`.

### ETAPA 3 — Router / gateway core

**Objetivo:** enrutar Envelopes entre canales y cerebros de forma concurrente y segura.

- **Fase 3.1 — Núcleo de enrutado.** Tabla de rutas canal↔cerebro; despacho de Envelopes
  entrantes; correlación de conversación. *Tests:* un Envelope entrante llega al cerebro
  correcto; respuestas vuelven al canal de origen.
- **Fase 3.2 — Concurrencia y resiliencia.** Manejo concurrente (goroutines + `context`);
  timeouts; backpressure; reintentos. *Tests:* carga concurrente sin condiciones de carrera
  (`-race`); timeout corta correctamente; el router no se bloquea con un canal lento.
- **Cierre:** `docs/stages/STAGE-03.md`.

### ETAPA 4 — Abstracción de modelos

**Objetivo:** una interfaz común para modelos locales y de nube, con llamadas paralelas.

- **Fase 4.1 — Interfaz Model.** `Complete(ctx, request) → response`; metadatos (coste,
  latencia, capacidades, si es local); registro de modelos. *Tests:* mock de modelo;
  registro y selección por nombre.
- **Fase 4.2 — Proveedor local (Ollama / compatible OpenAI).** Cliente HTTP al endpoint
  local. **Context7** (API de Ollama / formato OpenAI-compatible). *Tests:* petición/
  respuesta con fixtures; manejo de errores de conexión.
- **Fase 4.3 — Proveedor de nube.** Cliente para un proveedor de nube tras la misma
  interfaz. **Context7** (SDK/API del proveedor). *Tests:* serialización de petición
  correcta; parsing de respuesta; manejo de rate limits.
- **Fase 4.4 — Fan-out paralelo.** Llamar a N modelos en paralelo y recoger resultados con
  `context`. *Tests:* N llamadas concurrentes; cancelación propaga; agregación de
  resultados.
- **Cierre:** `docs/stages/STAGE-04.md`.

### ETAPA 5 — Motor de políticas / despacho — **CORAZÓN DEL PRODUCTO**

**Objetivo:** decidir, para cada petición, dónde y cómo se despacha, mediante políticas
componibles.

- **Fase 5.1 — Abstracción de política.** Interfaz `Policy` que recibe un contexto de
  petición (sensibilidad, presupuesto, capacidades requeridas) y devuelve un plan de
  despacho (qué modelos, local/nube, simple/consenso). *Tests:* política trivial "siempre
  el modelo X"; composición de políticas; precedencia.
- **Fase 5.2 — Política de coste.** Seleccionar el modelo más barato que cumpla la
  calidad/capacidad requerida. *Tests:* dado un set de modelos con costes, elige el
  correcto; respeta límites de presupuesto.
- **Fase 5.3 — Motor de evaluación.** Pipeline que evalúa las políticas activas y produce
  el plan final; trazas de la decisión (por qué se eligió X). *Tests:* el plan resultante
  coincide con la combinación de políticas; la traza explica la decisión.
- **Cierre:** `docs/stages/STAGE-05.md`, cobertura ≥ 90%.

### ETAPA 6 — Políticas avanzadas: privacidad/coste y consenso

**Objetivo:** las dos capacidades estrella, implementadas como políticas del motor de la
etapa 5.

- **Fase 6.1 — Clasificación de sensibilidad.** Clasificar la petición por sensibilidad de
  datos (reglas + opcional modelo local clasificador). *Tests:* entradas sensibles se
  marcan correctamente; configurable por reglas.
- **Fase 6.2 — Política de privacidad (routing híbrido local/nube).** Si es sensible →
  forzar modelo local; si no → permitir nube según coste/calidad. Garantía de "los datos no
  salen". *Tests:* dato sensible nunca se envía a un modelo de nube; dato no sensible puede
  ir a nube; auditoría de cada decisión.
- **Fase 6.3 — Política de consenso multi-modelo.** Despachar a N modelos y reconciliar
  (votación/agregación/árbitro); opt-in, nunca por defecto. *Tests:* N respuestas se
  reconcilian con la estrategia elegida; desempates; coste se contabiliza.
- **Cierre:** `docs/stages/STAGE-06.md`.

### ETAPA 7 — Orquestadores (cerebros) y registro multi-cerebro

**Objetivo:** varios cerebros conviviendo, cada uno usando el motor de políticas y su
propio pool de modelos.

- **Fase 7.1 — Abstracción Brain.** Un cerebro recibe un Envelope, consulta el motor de
  políticas, despacha a modelos/agentes, produce respuesta; configuración por cerebro
  (dominio, modelos disponibles, políticas). *Tests:* un cerebro responde a un Envelope
  usando un plan de despacho mockeado.
- **Fase 7.2 — Registro multi-cerebro.** Registrar/dar de baja cerebros en caliente;
  selección de cerebro por ruta/criterio. *Tests:* varios cerebros activos a la vez; el
  router selecciona el correcto; alta/baja sin downtime.
- **Fase 7.3 — Aislamiento y concurrencia.** Que un cerebro lento o que falla no tumbe a
  los demás. *Tests:* fallo aislado; ejecución concurrente de varios cerebros con `-race`.
- **Cierre:** `docs/stages/STAGE-07.md`.

### ETAPA 8 — Agentes y dirección concurrente

**Objetivo:** un cerebro lanza y coordina varios agentes en paralelo.

- **Fase 8.1 — Abstracción Agent y herramientas.** Interfaz de agente; protocolo de
  herramientas (tool use); ciclo de ejecución. *Tests:* agente ejecuta una herramienta mock
  y devuelve resultado; manejo de error de herramienta.
- **Fase 8.2 — Dirección concurrente.** Un cerebro despacha varios agentes en paralelo y
  agrega resultados; límites de concurrencia. *Tests:* N agentes concurrentes; cancelación;
  agregación; sin condiciones de carrera.
- **Cierre:** `docs/stages/STAGE-08.md`.

### ETAPA 9 — Persistencia

**Objetivo:** almacenamiento conmutable, SQLite por defecto, Postgres por config.

- **Fase 9.1 — Interfaz Store.** Definir operaciones (conversaciones, mensajes, config,
  auditoría de decisiones). *Tests:* contra un store en memoria de prueba.
- **Fase 9.2 — SQLite embebido.** Implementación; migraciones. **Context7** (driver SQLite
  y herramienta de migraciones). *Tests:* CRUD; migraciones idempotentes; funciona sin
  servicios externos.
- **Fase 9.3 — PostgreSQL conmutable.** Implementación tras la misma interfaz; selección
  por config. **Context7** (driver Postgres). *Tests:* la misma suite de contrato pasa con
  ambos backends.
- **Cierre:** `docs/stages/STAGE-09.md`.

### ETAPA 10 — Bus de eventos

**Objetivo:** desacoplar componentes; en memoria por defecto, NATS/Redis para distribuido.

- **Fase 10.1 — Interfaz Bus.** Publish/subscribe; tipos de evento. *Tests:* contra bus en
  memoria.
- **Fase 10.2 — Implementación distribuida.** Adaptador NATS o Redis Streams; selección por
  config. **Context7.** *Tests:* misma suite de contrato con ambos backends.
- **Cierre:** `docs/stages/STAGE-10.md`.

### ETAPA 11 — Ensamblaje por config y binario único

**Objetivo:** un solo binario cuyo comportamiento se define por ficheros de config; mismo
binario en Pi y nube.

- **Fase 11.1 — Carga de configuración.** Parseo de YAML + variables de entorno;
  validación; perfiles (`edge.yaml`, `cloud.yaml`). **Context7** (librería de config).
  *Tests:* config válida monta el sistema; config inválida falla con error claro.
- **Fase 11.2 — Composición (`cmd/korvun`).** Ensamblar canales, cerebros, modelos, store y
  bus según config; arranque/parada limpia. *Tests:* arranque end-to-end con un perfil
  mínimo; apagado sin fugas de goroutines.
- **Cierre:** `docs/stages/STAGE-11.md`.

### ETAPA 12 — Observabilidad

**Objetivo:** ver qué hace el sistema en producción.

- **Fase 12.1 — Logging estructurado.** `slog` en todo el núcleo; niveles; correlación por
  petición. *Tests:* los eventos clave se registran con los campos esperados.
- **Fase 12.2 — Métricas y trazas.** Métricas Prometheus (latencia, coste por petición,
  decisiones de routing); trazas OpenTelemetry. **Context7.** *Tests:* las métricas se
  exponen; las trazas cubren el flujo de una petición.
- **Cierre:** `docs/stages/STAGE-12.md`.

### ETAPA 13 — API de control

**Objetivo:** la superficie que consumirá el panel visual.

- **Fase 13.1 — API de gestión.** Endpoints para CRUD de cerebros, modelos, canales,
  políticas; estado del sistema; autenticación. **Context7** (framework HTTP/auth).
  *Tests:* cada endpoint con casos de éxito y error; autenticación obligatoria.
- **Fase 13.2 — API de observación.** Endpoints de métricas/decisiones/analítica de ahorro
  para el panel. *Tests:* respuestas con el esquema esperado.
- **Cierre:** `docs/stages/STAGE-13.md`.

### ETAPA 14 — Panel visual / builder no-code

**Objetivo:** componer canales/cerebros/modelos/agentes/políticas arrastrando, sin código.

- **Fase 14.1 — Andamiaje del frontend.** Proyecto React + TS + Vite + Tailwind;
  estructura; cliente de la API; configurar Vitest/Playwright. **Context7** (todas las
  versiones). *Tests:* render de la app; smoke e2e que carga el panel.
- **Fase 14.2 — Editor de nodos.** Lienzo con React Flow; nodos para
  canal/cerebro/modelo/agente/política; conexiones; serialización a la config YAML que lee
  el núcleo. **Context7** (React Flow). *Tests:* componer un grafo produce la config
  esperada; validación de conexiones inválidas.
- **Fase 14.3 — Paneles de gestión y analítica.** Vistas de estado, coste/ahorro,
  decisiones de routing. *Tests:* componente con datos mock; e2e del flujo "crear cerebro →
  conectar modelo → guardar".
- **Cierre:** `docs/stages/STAGE-14.md`.

### ETAPA 15 — Empaquetado multiplataforma

**Objetivo:** que el producto se instale y ejecute en Linux, Windows y macOS.

- **Fase 15.1 — App de escritorio (Wails).** Integrar frontend + binario Go vía Wails;
  builds para los tres SO. **Context7** (Wails). *Tests:* build reproducible por plataforma;
  arranque de la app en CI (smoke).
- **Fase 15.2 — Distribución.** Instaladores (`.dmg` / `.exe` / `.deb` / `.AppImage`); firma
  de código donde aplique; binario headless para servidores. *Tests:* instalación verificada
  en CI; firma válida.
- **Cierre:** `docs/stages/STAGE-15.md`.

### ETAPA 16 — Hardening, seguridad y release beta

**Objetivo:** dejar la beta sólida y publicable.

- **Fase 16.1 — Seguridad.** Auditoría de secretos; revisión de superficie expuesta;
  CodeQL/gosec en verde; revisión de la garantía de privacidad (los datos sensibles no
  salen, demostrado con tests de integración). *Tests:* test de integración que verifica
  end-to-end que un dato sensible nunca abandona la máquina.
- **Fase 16.2 — Pruebas end-to-end y carga.** Escenarios e2e completos (canal → cerebro →
  políticas → modelos → respuesta); pruebas de carga básicas; verificación en Raspberry Pi
  y en contenedor cloud con el mismo binario. *Tests:* e2e en verde en las plataformas
  objetivo; perfil de recursos dentro de los límites en Pi.
- **Fase 16.3 — Documentación global y release.** Cerrar README, guía de instalación por
  SO, guía de inicio rápido, documentación de la API y del builder; notas de versión;
  etiqueta `v0.1.0-beta` (SemVer).
- **Cierre:** `docs/stages/STAGE-16.md` y actualización final de este documento.

---

## 8. Definición de "Beta lista"

La beta de Korvun se considera lista cuando:

1. El binario único arranca por config en Linux, Windows y macOS (x86-64 y ARM64).
2. Funcionan 2-3 canales (Telegram + Webhook genérico, y WhatsApp si llega).
3. El motor de políticas enruta por privacidad (local/nube) y coste, con auditoría.
4. Existe ≥ 1 política de consenso opt-in operativa.
5. Multi-cerebro real: ≥ 2 cerebros concurrentes con agentes concurrentes.
6. El panel visual permite componer y guardar una configuración sin tocar código.
7. Persistencia SQLite y bus en memoria funcionando; conmutables a Postgres/NATS por
   config.
8. Toda la suite en verde, cobertura de núcleo ≥ 85%, CI multiplataforma en verde.
9. Test de integración que demuestra que un dato sensible nunca sale de la máquina.
10. Documentación completa de instalación, uso y arquitectura.

---

## 9. Riesgos y mitigaciones

| Riesgo | Mitigación |
|--------|------------|
| Explosión de alcance (todos los canales / builder pulido en beta) | Alcance de beta acotado a propósito (sección 8) |
| La promesa "mínimos recursos" se erosiona | Políticas costosas (consenso) opt-in; builder fuera del camino caliente; perfilar en Pi en etapa 16 |
| Integración de WhatsApp (la más traicionera) | Tratarla como opcional en beta; priorizar Telegram + Webhook |
| Programar contra APIs externas de memoria | Regla innegociable Context7 antes de cada implementación con librerías |
| Deriva de calidad entre etapas | Puerta `make quality` sobre toda la suite en cada cierre de fase |

---

## 10. Pendiente antes de empezar a codificar

1. Conectar **Context7** (regla innegociable) para poder verificar documentación en cada
   fase.
2. Nombre de proyecto / módulo confirmado: `korvun`. Pendiente solo la verificación externa
   de dominios y marca registrada.
3. Confirmar recursos (solo / equipo, full-time / part-time) para fijar el cronograma.

> **Nota:** el código de la prueba anterior queda descartado. Se empieza desde la Etapa 0
> con este documento como guía.