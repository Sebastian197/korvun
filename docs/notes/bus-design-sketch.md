# Bus de eventos — sketch de diseño (Stage 10 DIFERIDA)

> **Estado:** Stage 10 (bus) **diferida** por decisión consciente de YAGNI, no
> es deuda ni un hueco inexplicado. Se absorbe como la **primera fase de
> Stage 14** (builder no-code), cuando el live-view del builder defina el
> **primer suscriptor real** y el seam `Bus` se diseñe/valide contra él.
> Este documento conserva el espacio de diseño que salió del encuadre
> (`/office-hours` + `/plan-eng-review`, 2026-06-28) para que no se pierda.
> **No es un ADR.** Cuando se construya el bus, este sketch alimenta su ADR.

---

## Por qué se difirió (el veredicto del Step 0)

El bus es **infra especulativa hoy**:

- **Cero suscriptores reales.** El desacople canal↔router↔brain que un bus
  daría **ya existe** vía las colas point-to-point del router (cola inbound
  acotada por-brain + N workers, cola outbound por-canal, error hook async,
  saturación con `ErrBrainSaturated` / `ErrKindOutboundSaturated` /
  `DroppedCount`).
- **El segundo-consumidor que podría haberlo justificado ya se resolvió sin
  bus:** las métricas Prometheus de Stage 12 se cablearon **directamente** a
  los funnels, no vía un bus.
- **Stage 13 (control API) no es consumidor de eventos** — es request/response
  CRUD sobre el mux del httpserver.
- **El primer suscriptor real es el live-view del builder de Stage 14**, a dos
  etapas de distancia.

**Argumento decisivo — reversibilidad.** Korvun ya añade seams aditivamente
cuando llega el consumidor (`Store→SqliteStore`, `Metrics→prom`,
`Coordinator→fanout/sequential`). El router lleva intacto y testeado a `-race`
desde Stage 3. Por tanto "constrúyelo ahora que el router está fresco en la
cabeza" **no es load-bearing**: añadirlo después no reescribe nada. Diferir es
gratis y reversible. Coherente con la disciplina del proyecto: `AsModel`,
`Envelope.Sensitivity` y el Selector pre-dispatch se difirieron por la misma
razón — no construir un seam sin un consumidor que lo valide.

---

## Espacio de diseño (para CUANDO se construya, dentro de Stage 14 fase 1)

### Qué es un "evento"

El `Envelope` es el **mensaje**, NO el evento. Un evento es un **hecho de
ciclo de vida tipado** que **envuelve** una referencia al Envelope + metadata:

- `MessageReceived` — entró un Envelope por un canal.
- `ReplySent` — se entregó una respuesta a un canal.
- `MessageDropped` — saturación: un Envelope/respuesta se descartó.
- `HandleFailed` — `Brain.Handle` falló (mapea al `RouterError` kind).

No conflar evento con Envelope: el evento lleva el tipo + timestamp + canal/brain
y una referencia al Envelope, no es el Envelope mismo.

### Interfaz

```go
type Bus interface {
    Publish(ctx context.Context, ev Event)
    Subscribe(t EventType, h Handler) (unsubscribe func())
}
```

Misma forma de seam que `Store`/`Metrics`: interfaz en el dominio,
`inMemoryBus` hoy, `natsBus` mañana detrás de la misma interfaz.

### Coexistencia con las colas del router: VIVE AL LADO

El bus **nunca reemplaza** las colas point-to-point del router. El router
conserva sus colas como **mecanismo de entrega**; el bus es un **tap
secundario de observación** al que el router publica desde sus funnels.
Reemplazar las colas testeadas a `-race` = blast radius alto = **rechazado**.

```
funnels router ──publish(best-effort, no-bloqueante)──▶ Bus ──▶ suscriptor A (builder live-view)
                                                            └─▶ suscriptor B (futuro)
  el hot path NUNCA bloquea en el bus ; suscriptor lento ─▶ drop + contador
```

### Contrato de concurrencia / saturación (la zona de `/review`)

- `Publish` es **best-effort y no-bloqueante**.
- **Jamás** backpressure sobre el hot path: si el bus/suscriptor se atasca, el
  router no se frena.
- Suscriptor lento/bloqueado → **drop + contador** (reusar el precedente
  `DroppedCount` y el seam `Metrics`).
- Entrega **at-most-once**, sin persistencia ni replay.
- Seguro bajo `brainWorkers>1`: `Publish` concurrente, como el seam `Store`.

### Seam NATS futuro

`inMemoryBus` hoy → `natsBus` futuro detrás de la misma interfaz `Bus`,
idéntico a `Store→SqliteStore`. **NATS queda fuera** ahora: rompería la
promesa del binario único en una Raspberry Pi (el roadmap lo tiene como
planned/futuro).

### Fuera de scope (incluso cuando se construya la fase 1)

NATS, persistencia de eventos, event sourcing, distribución multi-proceso,
replay, webhooks salientes.
