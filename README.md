# Korvun

**Kernel for Orchestrated Routing — Versatile Unified Nodes**
*Núcleo de Orquestación y Routing sobre Nodos Versátiles y Unificados*

Korvun es un panel de control visual sobre un motor de orquestación de IA dirigido
por políticas, empaquetado como un binario único, ultraeficiente y multiplataforma.
El mismo binario corre digno en una Raspberry Pi y escala en la nube, cambiando solo
piezas de I/O por configuración.

## Qué unifica

Korvun reúne en un solo producto tres categorías que hoy están separadas:

- **Pasarela universal de mensajería** — conecta cualquier canal (WhatsApp, Telegram,
  Teams, Discord, Slack, Signal, o cualquier web/API propia vía un webhook genérico)
  mediante una forma de mensaje normalizada (*Envelope*).
- **Enrutamiento inteligente multi-modelo** — una sola puerta a modelos locales y de
  nube, con decisiones automáticas.
- **Orquestación multi-cerebro** — varios orquestadores ("cerebros") conviven; cada uno
  coordina varios modelos en paralelo y dirige varios agentes concurrentes.

## El diferenciador

El corazón del producto es el **motor de políticas de despacho**: routing consciente de
privacidad y coste, configurable sin código desde un builder visual.

- Lo sensible se queda en modelos locales y nunca sale de la máquina.
- Lo trivial va al modelo más barato.
- Lo crítico se despacha a varios modelos en consenso (opt-in).

Cada decisión queda auditada. Routing y consenso son políticas del mismo motor, no
sistemas separados.

## Características

- **Self-hosted por defecto** — tus datos no salen salvo que una política lo autorice.
- **Multiplataforma real** — Linux, Windows y macOS; x86-64 y ARM64.
- **Piezas conmutables por configuración** — persistencia SQLite → PostgreSQL; bus de
  eventos in-memory → NATS/Redis. Mismo binario, distinto perfil (`edge.yaml`,
  `cloud.yaml`).

## Stack

- **Núcleo:** Go (1.22+), stdlib siempre que sea razonable.
- **Builder visual:** React + TypeScript (strict) + Vite + Tailwind + React Flow.
- **Escritorio:** Wails (WebView del sistema, sin Chromium completo).
- **Persistencia:** SQLite (por defecto) → PostgreSQL (por config).
- **Bus de eventos:** in-memory (por defecto) → NATS / Redis Streams.
- **Configuración:** YAML + variables de entorno (12-factor).

## Modelo de negocio

Open-core: núcleo open source y self-hosted gratuito; capa Pro/Empresa de pago
(políticas avanzadas, analítica de ahorro, conectores premium, soporte); cloud
gestionado por suscripción.

## Estado

Especificación inicial completa (v1). En construcción por etapas — ver el documento
maestro del proyecto y `docs/stages/`.

---

> El nombre **Korvun** es a la vez un acrónimo descriptivo de la arquitectura y una
> palabra única y pronunciable.
