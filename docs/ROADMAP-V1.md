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
  - *Qué falta:* una capa de persistencia que el proyecto aún no tiene.
  - *Por qué se difirió:* bloqueado por un ADR de persistencia.

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

- **Persistencia.** Ninguna etapa hecha tiene capa de storage. La V1 necesita
  decidir SQLite/Postgres/NATS (ya en el stack previsto) y un ADR de
  persistencia — es prerequisito de budget, historial y estado de los brains.

- **Observabilidad.** Métricas, logs estructurados, trazas. ADR-0008 §4c dejó la
  métrica de saturación (`DroppedCount`) como dependencia dura de Stage 12. La
  *provenance* del motor de políticas es la base para depurar políticas, pero
  falta el sistema de observabilidad que la consuma.

- **El ensamblaje real (`main.go`).** Hoy las piezas (canal, router, modelos,
  política) funcionan por separado con demos. Stage 11 las cablea en un binario
  real. Sin esto no hay producto, solo componentes.

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

- **Control API (Stage 13).** Gestionar brains, políticas y canales en caliente.

- **Documentación de producto (Stage 16).** Quickstart, instalación por SO
  (incluido Raspberry Pi), guía de extensión (nuevo canal, nuevo agente),
  referencia de config. Sin esto, nadie que no seas tú puede usarlo.

- **Empaquetado y distribución (Stage 15).** Binarios por plataforma,
  instaladores, contenedores.

---

## 4. Multi-brain y agentes (la potencia)

- **Registro multi-brain (Stage 7).** Límites de brains concurrentes, recursos
  por brain (cola acotada + workers por ADR-0003), un número concreto de brains
  concurrentes soportados.

- **Agentes (Stage 8).** Que los brains llamen a herramientas/sistemas externos
  (ERP, etc.). Concurrencia pesada — zona de `/review`.

- **Bus de eventos (Stage 10).** Comunicación entre componentes. Concurrencia
  pesada — zona de `/review`.

---

## 5. Criterios de "esto ya es V1"

> Una checklist honesta para saber cuándo parar de llamarlo beta.

- [ ] Un mensaje real entra por un canal, se enruta, varios modelos responden,
      una política decide, y la respuesta vuelve — todo en un binario real
      (`main.go`), no en demos.
      - *Estado:* **el motor de políticas completo está hecho** — post-dispatch
        (reducers, Stage 5), pre-dispatch (selector de privacidad, Stage 6.1) y
        el fail-over secuencial que ahorra coste (Stage 6.2), todo orquestado por
        el Brain (Stage 7, `cmd/demo-brain`). Lo **único** que falta para marcar
        este criterio es el **ensamblaje en un binario real** (`cmd/korvun`,
        Stage 11): hoy los demos llaman a las piezas directamente, sin canal +
        router vivos cableados en un `main.go`. Ese cableado es el último paso.
- [ ] Persiste estado entre reinicios.
- [ ] Es observable (sé qué está pasando dentro sin leer el código).
- [ ] Lo configura alguien por fichero, sin recompilar.
- [ ] Lo instala alguien que no soy yo, en su máquina, siguiendo la documentación.
- [ ] Aguanta un proveedor caído sin caerse.
- [ ] Las políticas se expresan sin tocar Go (builder no-code).

---

> **Nota:** este documento es **VIVO**. Cada vez que un ADR difiera algo "a
> producción" o "fuera de v1", añádelo a la sección correspondiente. Se revisa al
> cerrar cada etapa junto con el HANDOFF.
