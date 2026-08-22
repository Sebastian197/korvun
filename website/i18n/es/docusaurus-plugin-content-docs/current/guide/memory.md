# Memoria gobernada

Desde v0.8.0 un cerebro puede conservar contexto más allá de la sesión
viva de dos maneras **deliberadas y gobernadas por políticas**: **notas**
persistentes escritas a través de la herramienta gobernada `memory_note`,
y **`/recall`** — una recuperación deliberada, con procedencia visible, de
la cola de la sesión anterior. Nada se recuerda de forma automática: sin
embeddings, sin resúmenes hechos por el modelo, sin arrastre silencioso.

El diseño está en el repositorio:
[ADR-0043](https://github.com/Sebastian197/korvun/blob/master/docs/adr/0043-minimal-memory-notes-and-recall.md)
y la
[spec de memoria mínima](https://github.com/Sebastian197/korvun/blob/master/docs/superpowers/specs/2026-08-16-minimal-memory.md).

## Notas — hechos acotados que sobreviven a los reinicios

Un cerebro agente con bloque `memory` guarda **notas**: hechos breves que
el modelo almacena mediante `memory_note`, con un único campo obligatorio,
normalizadas a una línea, con topes de tamaño y de número. Las notas no
son historial — sobreviven a `/new` y `/reset` a propósito, y viajan en el
prompt de sistema del cerebro en cada mensaje posterior del mismo ámbito
como un bloque de datos inerte y claramente delimitado.

`memory_note` pasa por la misma puerta tri-estado que toda herramienta
([allow / shadow / deny](/guide/tools-and-skills)):

- **`shadow` primero** — anuncia la herramienta en ensayo, observa qué
  *almacenaría* el cerebro (auditado como `tool_shadowed`, visible en
  `/tools` y en el feed de Actividad), y luego aplica en caliente el
  permiso a `allow`.
- **El arranque rechaza la memoria a medio configurar** — listar
  `memory_note` sin su bloque `memory`, o sin un permiso de gobernanza que
  la cubra, es un error de arranque ruidoso, nunca un valor por defecto.
- **Lleno es lleno** — cuando la caja está llena la herramienta se niega;
  el operador la vacía con `/notes clear`. Sin desalojo silencioso.

El operador siempre ve la caja: `/notes` lista las notas del ámbito
numeradas; `/notes clear` vacía el ámbito. Ambos son comandos de sistema
instantáneos — sin llamada al modelo — y vaciar las notas nunca toca la
transcripción.

## Ámbitos — y la garantía estructural de privacidad

`memory.scope` declara hasta dónde llega una nota:

- **`conversation`** (por defecto) — las notas viven y viajan solo dentro
  de la conversación que las almacenó.
- **`brain`** — una caja compartida para todo el cerebro, un opt-in
  explícito que hace que las notas crucen conversaciones. **Este ámbito
  EXIGE que el modelo seleccionado del cerebro sea local**: la memoria
  global de cerebro se niega a arrancar con un modelo cloud seleccionado.
  La garantía es estructural — se impone en el arranque, no mediante un
  atributo en runtime — así que las notas de un cerebro privado nunca
  salen de la máquina.

## `/recall` — recuperación deliberada de un corte de sesión

Las sesiones cortan el contexto en seco (mira
[la consola del operador](/guide/chat)). `/recall` hace ese corte
recuperable **a propósito, nunca por accidente**:

- Se habilita con `session.recall_max` (`0` ⇒ deshabilitado; 1..50). Un
  `/recall` a secas importa hasta el máximo configurado; `/recall <k>`
  acota `k` a ese máximo.
- **Solo en una sesión vacía** — sobre una sesión activa no vacía se niega
  y te señala `/new` primero. Tras una importación la sesión ya no está
  vacía, así que la duplicación es imposible por construcción.
- **Un bloque citado** — los últimos turnos de la sesión archivada más
  reciente llegan como UN bloque citado claramente delimitado cuya
  cabecera nombra la sesión de origen; el acuse dice cuántos turnos
  volvieron. La procedencia queda visible: contexto citado, no mensajes
  nuevos.
- Cero implicación del modelo — es un comando de sistema, gestionado de
  forma agnóstica al canal.

## Configuración

```json
{
  "session": { "recall_max": 10 },
  "brains": [{
    "name": "assistant",
    "sensitivity": "private",
    "policy": {"kind": "priority"},
    "models": [{"provider": "ollama", "model_id": "llama3.2", "locality": "local"}],
    "agent": {
      "tools": ["time", "memory_note"],
      "governance": [
        {"tool": "time", "mode": "allow"},
        {"tool": "memory_note", "mode": "shadow"}
      ],
      "memory": {"scope": "conversation", "max_notes": 10, "max_note_runes": 200}
    }
  }]
}
```

Este cerebro ensaya `memory_note` en shadow — observa qué almacenaría y
luego promociona el permiso a `allow` con una aplicación en caliente. El
detalle campo a campo está en la
[referencia de configuración](/reference/configuration). El bloque
`memory` requiere el bloque `storage` (las notas viven en el almacén
durable).

## Qué queda fuera — a propósito

La recuperación semántica y los embeddings, el arrastre automático o los
resúmenes hechos por el modelo, y la edición nota a nota quedan
deliberadamente fuera de la memoria mínima. El operador vacía un ámbito;
el modelo almacena dentro de sus topes; todo lo demás se mantiene
deliberado y visible.
