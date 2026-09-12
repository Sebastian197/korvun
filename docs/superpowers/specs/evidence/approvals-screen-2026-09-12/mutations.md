# Mutaciones probatorias — la pantalla de aprobaciones (2026-09-12)

Dieciocho mutaciones sobre `src/views/Approvals.tsx` y `src/views/approvalsText.ts`,
cada una aplicada sola, la suite completa de la pantalla ejecutada, la mutación
revertida. Nivel de evidencia: en proceso, jsdom, contra un `fetch` de mentira.

| # | Qué neutraliza | Rojas (1ª pasada) | Rojas (tras cerrar el agujero) |
|---|---|---|---|
| M1 | escapar bytes no confiables: se pinta el carácter crudo | 4 | — |
| M2 | forma del digest: cualquier cadena pasa por digest | 1 | — |
| M3 | detector de colisión de cola: nunca marca | 1 | — |
| M4 | armado: aprobar se ofrece sin teclear la cola | 4 | — |
| M5 | inercia del segundo clic: el botón no se deshabilita al enviar | 1 | — |
| M6 | Esc acotado: queda activo con la decisión ya tomada | 2 | — |
| M7 | cuerpo ilegible: degrada a documento vacío (fail-open) | 1 | — |
| M8 | cuerpo que no es objeto: degrada a documento vacío | 0 | 1 |
| M9 | puerta no montada: el 404 sin cuerpo nuestro se trata como error genérico | 0 | 1 |
| M10 | gate apagado: la lista vacía se pinta como «no hay nada» | 0 | 1 |
| M11 | parámetros ausentes: se ofrece el sí igualmente | 3 | — |
| M12 | cerebro ido: se ofrece el sí igualmente | 1 | — |
| M13 | caducada por reloj: se ofrece el sí igualmente | 1 | — |
| M14 | present con cuerpo vacío: se acepta como oferta | 1 | — |
| M15 | agrupación del digest: ocho grupos de ocho pasan a cuatro de dieciséis | 0 | 1 |
| M16 | nombre desconocido: cae a un texto genérico en vez de su letrero | 8 | — |
| M17 | escalera por rango: la clase fuera de la escalera se pinta como anomalía | 2 | — |
| M18 | nombre no reconocido: se degrada a la nada en vez de su letrero | 1 | — |

Cuatro no enrojecieron en la primera pasada —M8, M9, M10 y M15— y ninguna era una
mutación mal hecha: eran ramas que el código sí tiene y que ningún molde recorría.
Se cerraron con los cuatro moldes del `describe("MUT · ramas sin vigilar")`, y las
cuatro mutaciones se re-ejecutaron contra ellos: enrojecen.
