# <Pieza + sub-fase> — <asunto>: Diseño de experiencia (UX)

> **Estado:** borrador | aprobado por Chano | supersedido.
> **Ley (CLAUDE.md — UX-DESIGN-FIRST + MANOS-DE-CHANO, 2026-08-23):** ninguna
> pieza visible para el usuario abre fase RED sin este documento aprobado por
> Chano. La aprobación se decide MIRANDO — mockups o maquetas renderizadas
> acompañan la prosa (lección del rediseño aurora, 2026-08-08); la prosa sola
> no se aprueba. Todo punto sin definir se marca `[NEEDS CLARIFICATION]` y
> bloquea la aprobación hasta resolverse.

## Qué ve el usuario (pantalla a pantalla)

Cada pantalla o vista que la pieza toca, en orden de recorrido: qué hay en
ella, qué cambia respecto a hoy y con qué texto exacto. Un mockup (imagen o
texto) por pantalla.

## Ciclo completo de cada elemento interactivo

Para CADA elemento nuevo o alterado (botón, desplegable, campo, fila, atajo):
cómo se abre, cómo se cierra, si la acción se puede deshacer (y cómo) y cómo
se cancela a mitad. Un elemento sin su ciclo completo es un
`[NEEDS CLARIFICATION]`.

## Estados de error EN PANTALLA

Por cada fallo que el usuario puede provocar o presenciar: qué dice la
pantalla (texto exacto), en qué idioma, y qué botón de arreglo ofrece.
"Falla en silencio" y "se ve en el log" no son estados de error de UX.

## Estados vacíos y de carga

Qué se ve la primera vez (sin datos), qué se ve mientras se espera, y qué
texto acompaña cada estado.

## Qué NO hace (out of scope declarado)

Lo que un usuario razonable esperaría de esta pieza y deliberadamente no se
entrega, dicho aquí para que la sorpresa no exista — con el destino de cada
exclusión (otra ola, decisión con datos, descartado).

## Criterio de aceptación = la pasada de Chano

El diseño se acepta cuando Chano lo recorre y da el sí explícito: sobre los
mockups antes de abrir RED, y sobre la build empaquetada (bug bash) antes de
cualquier tag que lleve la pieza. Anotar aquí fecha y resultado de cada
pasada. Sin el sí, no hay RED; sin la pasada sobre la build, no hay tag.
