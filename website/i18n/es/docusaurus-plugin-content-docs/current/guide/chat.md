# La consola del operador

La pestaña **Chat** de Korvun Desktop es la consola del operador: lee todas
las conversaciones que fluyen por tus canales, respóndelas tú mismo cuando
quieras y habla con tus cerebros directamente — todo desde la app, todo
persistido en el mismo almacén durable.

Necesita los bloques `storage` y `session` de la
[referencia de configuración](/reference/configuration). El escritorio
aprovisiona ambos automáticamente en el primer arranque **y** al actualizar
una configuración existente, así que normalmente no hay nada que hacer. El
chat directo con la IA necesita además una entrada de canal `console`:

```json
{ "type": "console" }
```

## La bandeja de entrada

El panel izquierdo lista todas las conversaciones del almacén, actividad
más reciente primero:

- **Filtra mientras escribes** — la caja acota la lista al instante por
  clave de conversación y además busca en el **contenido** de los mensajes
  en el servidor.
- **Badges de no leídos** — cada conversación muestra cuántos turnos
  llegaron desde la última vez que la abriste; abrirla la marca como leída.
- **TAKEN OVER** marca las conversaciones donde mantienes el takeover (más
  abajo).

Los turnos van estilizados por rol: el usuario final, el cerebro
(**Korvun**), tú (**Operador**, alineado a la derecha) y líneas de sistema
discontinuas, como los acuses de reinicio de sesión. Los adjuntos llegan
**anunciados**: una foto persiste y se muestra como un marcador `[image]`.

## Sesiones, `/new` y `/reset`

Una conversación es una serie de **sesiones**; solo la más nueva está
activa y solo la activa es el contexto del cerebro. Una sesión termina
cuando:

- el usuario final (o tú, en chat directo) envía un disparador de reinicio
  — `/new` o `/reset` por defecto — como primer token exacto; el canal
  responde con un acuse fijo y se abre una sesión nueva; o
- pasa una expiración configurada `daily_at` / `idle_min`, aplicada de
  forma perezosa en el siguiente mensaje entrante.

Las sesiones viejas siguen almacenadas: las pestañas de sesión sobre la
transcripción las navegan en solo lectura. Reiniciar nunca borra nada —
solo corta el contexto.

## Recuperar un corte: `/recall`

El corte es recuperable **a propósito, nunca por accidente**. `/recall`
(habilitado por `session.recall_max`) cita la cola de la sesión anterior de
vuelta en la activa como UN bloque claramente marcado — la cabecera nombra
la sesión de origen y el acuse dice cuántos turnos volvieron. Solo funciona
sobre una sesión activa VACÍA; sobre una no vacía se niega y te señala
`/new` primero. Recuperación deliberada, procedencia visible, duplicación
imposible por construcción. El diseño completo está en la página de
[memoria gobernada](/guide/memory).

## Notas y `/notes`

Un cerebro con memoria configurada también guarda **notas** — hechos
breves que el modelo almacena a través de la herramienta gobernada
`memory_note` (mira
[herramientas y skills gobernadas](/guide/tools-and-skills)). Las notas no
son historial: SOBREVIVEN a los reinicios de sesión a propósito, viajando
en el contexto del cerebro solo dentro de su ámbito. `/notes` las lista
numeradas; `/notes clear` vacía el ámbito — ambos son comandos de sistema
instantáneos, sin llamada al modelo, y vaciar las notas nunca toca la
transcripción.

## Responder como operador (takeover)

En un canal de red (Telegram, Discord, webhook), responder a mano requiere
primero **Take over**: mientras lo mantienes, el cerebro queda silenciado
para esa conversación — los turnos entrantes siguen persistiendo y
apareciendo en vivo, pero ningún modelo responde. Tus respuestas salen por
el adaptador del propio canal (coste cero — sin llamada al modelo) y
persisten como turnos de operador, así que al pulsar **Release** el cerebro
retoma *con tus respuestas en su contexto*. El takeover es por
conversación, sobrevive a los reinicios de sesión y no es durable tras un
reinicio del core (fail-open: el cerebro vuelve a responder).

## Chat directo con la IA (canal console)

Con un canal `console` configurado, **New chat** inicia una conversación
entre tú y un cerebro — aquí el *usuario* eres tú, así que no hay takeover.
Los mensajes recorren el pipeline completo (política, enrutado,
persistencia); **Thinking…** se muestra mientras el cerebro trabaja. Las
sesiones, `/new`, el borrado, la búsqueda y los badges se comportan igual
que en el resto. Sin ruta explícita, el canal console habla con el primer
cerebro configurado.

Desde el canal console, `/tools` devuelve el informe del gatekeeper del
cerebro de esa conversación — sus permisos efectivos y la actividad
reciente de herramientas.

## Borrar

- **Delete conversation** elimina la conversación entera — cada sesión,
  cada turno — del disco, y libera antes cualquier takeover.
- **Delete session** elimina una sesión **archivada**; la sesión activa no
  puede borrarse (usa `/new` primero si de verdad quieres cortarla).

Ambos son permanentes. No hay deshacer.

## Notas de seguridad

- La UI de escritorio nunca sostiene el bearer de administración: el shell
  lo inyecta en el lado servidor, y el stream de eventos en vivo queda
  libre de secretos.
- Los secretos son siempre nombres de variables de entorno en la
  configuración; los valores viven en el entorno o en el llavero del
  sistema operativo.
