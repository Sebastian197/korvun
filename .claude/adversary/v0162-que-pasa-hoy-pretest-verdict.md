# VETO MANTENIDO — lectura adversaria INTERNA del papel pre-test de «¿Qué pasa hoy?» (v0.16.2)

Persistido por el ejecutor. `3 P1, 5 P2, 7 P3.` Tiempo consumido: **28 min** sobre
un presupuesto de 30. Objeto: la PRIMERA versión de
`docs/superpowers/specs/2026-09-23-v0162-que-pasa-hoy-pretest.md`, antes de una
línea de código. Escrituras dentro del árbol auditado: cero; el arnés vivió en el
scratchpad, sobre una copia hecha con `git archive HEAD`.

**Los tres P1 se verificaron uno a uno contra el árbol antes de rediseñar, y los
tres eran ciertos.**

## P1-1 · G1 y G2 son INCOMPATIBLES contra el cable que el papel citaba

El papel decía que la pantalla «consume el veredicto del núcleo, no lo recalcula»
y a la vez que «nombra cuál de las cinco condiciones falla». El cable no puede dar
las dos cosas:

- `brainCanPark` **cortocircuita en la condición 3** y devuelve `false` antes de
  evaluar la 4 y la 5 — las que la maqueta pinta con ✓ en su segundo estado.
- `ApprovalGate` transporta cuatro números y **ningún motivo**.
- Su propio godoc declara lo contrario de lo que el papel le atribuía: «It is a
  declared UPPER BOUND, not a promise… judged PER CHANNEL… no
  channel-independent boolean exists».

**Capturado ejecutando**, con tres perfiles sobre una copia del árbol:

```
no-ceiling profile: brainCanPark=false brainsThatCanPark=0
no-ceiling + no tool + denied:  brainCanPark=false brainsThatCanPark=0
IDENTICAL OUTPUT from the cable => the screen CANNOT derive which of the five failed

shadow (mockup state 3, AMBER)   brainsThatCanPark=0  ApprovalGate{BrainsTotal:1, BrainsCanPark:0}
no parkable tool                 brainsThatCanPark=0  ApprovalGate{BrainsTotal:1, BrainsCanPark:0}
not an agent brain               brainsThatCanPark=0  ApprovalGate{BrainsTotal:1, BrainsCanPark:0}
```

La fila ÁMBAR de la maqueta y la ROJA producen el MISMO `BrainsCanPark = 0`.
Agravante: con dos herramientas que fallan condiciones distintas, el cable sigue
devolviendo un solo `false`, así que «LA condición que falla» no es función de su
salida.

**RESUELTO**: el núcleo publica el motivo, **junto a** `BrainsCanPark` y sin
tocar su valor, para no mover el literal que AS-19 fija en la bandeja. Y G2 pasa
a prometer **todas** las condiciones que fallen, no la primera.

## P1-2 · El papel INVERTÍA la invariante que el árbol ya garantiza

`internal/supervisor/supervisor.go`, en su cabecera y en `reasonReload`: «the new
config is persisted ONLY after the new app's Start returns nil… persist is NEVER
called on a failed cutover, so the -config on disk is untouched».

El papel escribía el fichero ANTES de la recarga. Con eso inventaba una ventana de
fichero-adelantado-al-proceso que hoy no existe, inventaba `ErrRollbackFailed`
sobre el fichero —que hoy no puede ocurrir— y la llamaba «la más grave» sin notar
que la estaba creando él.

**Agravante: la recarga NO es síncrona.** `POST /api/config` responde
`202 Accepted` + handle, que se sondea. No hay puerta que devuelva «la recarga
falló» al que la pidió, así que el paso 5 del papel presuponía un retorno que no
existe: el POST volvía en verde y la pantalla no podía decir «no se aplicó».

Y la clase simétrica que el árbol SÍ tiene y el papel no mencionaba:
`StatePersistFailed` — la app nueva sirviendo y el disco sin actualizar.

**RESUELTO**: el orden se invierte y pasa a ser el del árbol. La vuelta atrás
**se hereda** del supervisor y el canto lo dirá así. La pantalla sondea el handle,
y de ahí nace un estado de UI que la maqueta no tenía.

## P1-3 · El molde de A1 entraba sin mutación probatoria

La doctrina lo prohíbe con esas palabras —«sin mutación roja capturada, el test no
existe»— y añade que ninguna instrucción futura puede suspenderla. A1 es además
la fila que la séptima ley exige, y el papel la eximía. Y la etiquetaba «binario
en proceso OS aparte» cuando una pasada a mano sobre la app es binario **más
humano**, que es otra cosa.

**RESUELTO**: A1 deja de listarse como molde. Es una PASADA MANUAL con su
etiqueta, y su hermano sintético —perfil fabricado en schema 12 y migrado en un
test— sí es molde, sí corre en CI y sí lleva mutación.

## Los cinco P2

| | Qué estaba mal | Cómo se resolvió |
|---|---|---|
| P2-1 | «el `rename` es atómico: o está el viejo o está el nuevo» — **falso con un perfil que es enlace simbólico**, capturado ejecutando: el enlace desaparece, el destino sigue vivo donde nadie lo lee, y queda un fichero regular en su sitio. Sin un solo error | La pieza deja de escribir el fichero, así que no lo hereda. Y la deuda **sube a tren mínimo propio** tras la v0.16.2, por decisión del director: es pérdida de datos |
| P2-2 | §4 omitía once clases que el camino real ya produce, incluidas `409 reload_in_progress` —el ataque de dos ventanas, que la matriz no tenía— y `409 config_would_self_lock`, que define perfiles enteros donde **ningún botón puede funcionar nunca**. Y colapsaba en una sola clase los CUATRO estados del núcleo, incluido `unknown`, que es el del **primer render** — el caso más frecuente ahora que la pantalla es Inicio | §4 se reescribe entera; nacen A15, A16 y A18 |
| P2-3 | A5 decía «oráculo por imposibilidad» y comparaba mtime + sha256, que es un dump igual. Y el autoengaño: un escritor que tocara el fichero y lo restaurara **sin registrar acto** pasaba los dos oráculos | El directorio del perfil en `0o500`, que el adversario capturó abortando `os.CreateTemp` ruidosamente, **más** el conteo de actos |
| P2-4 | G7 decía que el estado de fallo «tira la hora» mientras §2 del mismo papel decía lo contrario. La verdadera es la segunda: `lastOkAt` **nunca se borra**; es el rótulo quien deja de leerlo. Y la mutación de A12 atacaba el **store** en vez del **rótulo**, que es donde vive la rama vigilada | Las dos frases corregidas y la mutación movida al rótulo |
| P2-5 | A8 no decía qué digest compara. Capturado: `config.Load` + `MarshalIndent` **no es byte-preservante** —1076 vs 1142 bytes, digest distinto— sin una sola edición ajena. Si compara el struct, se rehúsa siempre | A8 cambia de forma: ataca el campo desconocido de un perfil migrado, que es el caso real |

Y siete P3, todos ciertos y todos plegados: los dos vecinos que más dolían
ausentes de §2 —`internal/supervisor` y la rama de `Approvals.tsx` que hoy recita
las cinco condiciones—; que `POST /api/config` lleva bearer **sin** envoltura de
loopback, a diferencia de aprobaciones y consola; que «tres por cura» era falso
contra su propia tabla —15 moldes para 14 ataques, y solo uno con dos—; que el
veredicto es por herramienta y por canal mientras la frase es por instalación; que
A13 no fuerza la rama del guardián `if (polling) return`, donde un ms rancio pasa
sus dos condiciones; que §1 enumeraba tres veredictos y la maqueta compromete
cinco; y que `WriteConfigAtomic` no hace `fsync`, así que A7 solo puede hablar de
muerte de proceso, no de máquina.

## Lo NO revisado, por su nombre

No corrió `make quality`, ni `go vet ./...`, ni la suite del frontend: no sabe si
el árbol está verde. No leyó entero `Approvals.tsx` ni `Home.tsx`. No examinó el
camino de migración 12 → 15 ni verificó la afirmación del HANDOFF de que
`grep -ci migrat` sobre el log da 0. No abrió la maqueta en un navegador: leyó su
texto extraído. Las reproducciones del orden de persistencia y del ataque de dos
ventanas son **PREDICCIONES** declaradas como tales — no hay código de la pieza
que ejecutar.
