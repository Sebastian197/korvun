# Barridos de letreros del tren del TAG v0.16.1 (2026-09-23)

Persistidos por el ejecutor: el adversario no escribe dentro del árbol que
audita. Dos pasadas, ambas con VETO MANTENIDO, ambas curadas.

Delta auditado: `docs/releases/v0.16.1.md`, los dos ficheros de
`docs/superpowers/specs/evidence/v0.16.1/`, `docs/RELEASE-CHECKLIST.md`,
la sección nueva de `docs/HANDOFF.md`, y las capturas de
`docs/assets/captures/v0.16.1/`.

---

## Barrido 1 — `3 P2, 4 P3` · 5 min 21 s (presupuesto 10, no agotado)

### P2-1 · Las notas daban a la puerta nueva evidencia que no tiene

«La puerta nueva sí: la pasada manual la conduce con el binario construido de
este árbol, en su propio proceso, **y la empaquetada con el `.app`**.» La pasada
empaquetada NO toca `intent bind --grant` en ningún punto: ejerce la pantalla de
aprobaciones. `grep -n -i "bind\|grant" packaged-pass.txt` devuelve UNA línea, y
es un falso positivo léxico («No approval was granted»).

**CURADO**: la viñeta lleva ahora una tabla de dos filas —qué conduce cada pasada
y con qué binario— y la frase «La puerta nueva NO se ha conducido desde el
`.app`, y la pantalla de aprobaciones no se ha conducido desde la CLI».

### P2-2 · Un absoluto roto por contraejemplo del propio fichero

«ninguna cura tiene binario en un proceso del sistema aparte ni crash-restart»,
mientras el mismo fichero, en «La pasada sobre la build EMPAQUETADA», describe la pasada
sobre el `.app` ejerciendo la ficha del recibo acuñado, que ES una cura de esta
release.

**CURADO** junto con el P2-1, y después acotado otra vez por el P3-1 del barrido
2.

### P2-3 · El kit invocaba la Sexta Ley como respaldo de lo que la Sexta Ley prohíbe

La casilla 1 decía «the manual bug bash over the PACKAGED build **(the Sixth
Law)**» y la casilla 6 reasignaba ese acto al ejecutor. La ley, en `CLAUDE.md`,
dice «ninguna release se etiqueta sin la pasada manual **de Chano** sobre la
build empaquetada», y está marcada CRITICAL. El delta reasignaba el acto sin
tocar la ley que lo funda y seguía citándola como autoridad.

**CURADO en dos pasos**: la casilla 1 dejó de citarla; la casilla 6 ganó un
párrafo propio. El primer intento de ese párrafo volvió a caer y lo tumbó el
barrido 2 — ver su P2-1.

### Los cuatro P3

| | Qué estaba mal | Cura |
|---|---|---|
| P3-1 | La casilla 6 nombraba `docs/superpowers/specs/evidence/<tag>/` como sitio de la CAPTURA; la captura aterrizó en `docs/assets/captures/v0.16.1/`, y los otros dos ficheros del delta ya apuntaban ahí | la casilla separa el registro verbatim de la captura y nombra las dos rutas reales |
| P3-2 | «that is what actually happened for v0.15.0, v0.15.1 and v0.16.0» presentado como registro: `find docs -name "*packaged*"` devuelve solo ficheros de la v0.16.1 | escrito como palabra del director, en los dos sitios |
| P3-3 | El bloque de salida de la puerta nueva usaba ids de la referencia del operador, no de esta corrida | sustituido por los ids de `manual-pass.txt` §4, idénticos byte a byte |
| P3-4 | «esta misma árbol» | concordancia |

### Lo que el barrido 1 verificó LIMPIO, ejecutando

El recuento de fichas (siete, contra una tabla de ocho filas con dos marcadas
como no-ficha); «tres puertas lo rehúsan» contra las tres llamadas de producción
a `strictMarkerTx`; los seis datos del bundle contra `shasum`, `wc -c`, `file` y
`plutil`; «los 13 commits» contra `git rev-list`; que el tag no existe; el log
verbatim línea a línea; las cuatro consultas del almacén en `?mode=ro`; la forma
del recibo; el sha256 de la captura contra su original; y las cinco afirmaciones
de la ficha nueva del HANDOFF contra la fuente.

Un falso positivo que descartó ejecutando: `approvals.canonical_params` está
vacío en el almacén mientras el fichero registra que la pantalla pintó «el
almacén devolvió parámetros que re-derivan este digest». No es la clase (a): la
purga de parámetros ocurre en la decisión y la pantalla leyó antes.

---

## Barrido 2 — re-pasada acotada · `1 P2, 3 P3` · ~11 min

Siete de los ocho puntos volvieron limpios, con captura. El que no:

### P2-1 · La afirmación de cumplimiento de la Sexta Ley no se retiró: se le cambió el sujeto

La cura del P2-3 escribió «the Sixth Law's own requirement — HIS hands on the
packaged build — **is met by the curtain of this same box**», y su gemela en las
notas. Falso, o al menos sin cable:

1. La cortina está definida en el mismo recuadro como «local build in HIS
   browser» — ni es una pasada, ni es la build empaquetada.
2. El acto que la ley nombra es el bug bash del recuadro 1, y este mismo diff lo
   delega.
3. Las únicas manos del director sobre un artefacto empaquetado están en el
   recuadro 8, **después** del tag, y la ley habla de etiquetar.
4. El titular decía «stated plainly and unresolved» y el cuerpo resolvía que sí.

**CURADO**: la afirmación de cumplimiento se retira entera. El párrafo escribe el
hueco —la ley nombra dos cosas y el kit no asigna ninguna al director antes del
tag— y dice con todas las letras «Nothing in this box claims the Sixth Law is
satisfied». Las notas dicen lo mismo.

### Los tres P3

| | Qué estaba mal | Cura |
|---|---|---|
| P3-1 | «El nivel de evidencia de los MOLDES es in-process, **sin excepción**» sin palabra de alcance. Dentro del delta el absoluto VALE —ningún fichero de test del rango lanza binario ni hace crash-restart, comprobado por grep—, pero el árbol tiene tres moldes de trenes anteriores que sí. **El barrido 2 contó ONCE ficheros y son DOCE**: se le escapó `scripts/wails_pin_test.py`, el molde de la guarda de Wails. Lo cazó el barrido 3, y la garantía sustantiva aguanta sobre los doce | la frase dice «los moldes DE ESTA RELEASE», nombra el rango `v0.16.0..HEAD`, y reconoce los de fuera |
| P3-2 | La ficha del HANDOFF prometía «la captura y el relato … §9», y el §9 no nombraba ninguna captura | se ejerció la captura que faltaba: el panel en «El núcleo está parado» con la barra en «En marcha :61801», los dos lados del defecto en un fotograma, nombrada en la ficha y en el §9 |
| P3-3 | La casilla 6 prometía que la pasada «exercises the surfaces the release actually changed», y la primera ejecución declara que NO cubre `intent bind --grant` | la casilla acota a las superficies **que la app empaquetada expone** y exige nombrar a mano las que por eso no alcanza |

Y una nota menor que el adversario no elevó a hallazgo, curada igual: «nothing
could reach it» sobre `hooks.acme.io` seguía redactado como hecho con el descargo
detrás. Ahora el límite va delante.

### Lo que el barrido 2 verificó LIMPIO

Las cuatro celdas de la tabla de pasadas contra sus dos ficheros; que ninguna
frase del delta dice que la delegación cumpla la ley ni que la ley cambió; la
ruta de la captura concordante a tres bandas; la aserción histórica declarada
como palabra en los dos sitios; los ids del bloque de salida **idénticos por
md5** a los de `manual-pass.txt`; y las filas del almacén contrastadas de nuevo
en `?mode=ro`, incluido `select count(*) from actions where state like '%DENIED%'`
→ **0**.

### Lo NO verificable con lo que tenía, por su nombre

- Los nombres de los elementos AX que el fichero cita: haría falta relanzar el
  `.app`. El nivel de evidencia declarado queda **no verificado por él**, ni
  confirmado ni refutado.
- Las §1-§7 de `manual-pass.txt`: su almacén no se le entregó, así que las filas
  de `execution_bindings` y los actos por desenlace siguen siendo relato no
  contrastado por un tercero.
- Quién corrió de hecho la pasada empaquetada en las tres releases anteriores.
- El contenido de los moldes del delta más allá del grep de spawn.

---

## Barrido 3 — re-pasada acotada · `1 P2, 4 P3` · ~6 min

El punto 3 volvió limpio con la imagen abierta: la captura del defecto muestra
exactamente lo que sus dos frases dicen, en un solo fotograma. Y el punto 1
volvió limpio: `grep -niE 'sixth law|sexta ley|cumpl|satisf|met by|cortina'`
sobre los seis ficheros no devuelve ninguna frase que afirme cumplimiento. Lo
que apareció fue otra cosa, un fichero más allá.

### P2-1 · La ficha nueva del HANDOFF se eximía de la Sexta Ley con un criterio que la ley no tiene

Escribí «la sexta ley no exige maqueta para esto —no hay pantalla nueva—». Tres
cables la tumban:

1. La ley dice «ninguna **pieza visible para el usuario** abre fase RED sin un
   diseño de experiencia aprobado por Chano». El sujeto es pieza visible, no
   pantalla nueva, y la cura cambia lo que el operador ve.
2. La ficha hermana de la MISMA pantalla, en el mismo fichero, se paró por esa
   ley sin haber pantalla nueva tampoco: «No se toca hasta tener maqueta
   aprobada antes de abrir rojo, por la sexta ley».
3. El mismo delta acababa de escribir «only he moves its text» sobre esa ley.
   Acotarla por fiat del ejecutor en otro fichero es justo lo que ese párrafo se
   declaró incompetente para hacer.

**CURADO en una línea**: la ficha dice ahora que **lo adjudica el director**.

### Los cuatro P3

| | Qué estaba mal | Cura |
|---|---|---|
| P3-1 | «los **once** ficheros de test del rango» — son **DOCE**, y el que faltaba es `scripts/wails_pin_test.py`, el molde de una cura de esta misma release. Re-derivado por dos vías: `git diff --name-status v0.16.0..HEAD` y la unión sobre los 13 commits | la cifra dice doce y desglosa diez Go, uno TS y el python. La garantía sustantiva aguanta sobre los doce: cero spawns, cero crash-restart |
| P3-2 | El fichero de veredictos transcribía ese once como «comprobado por grep» | corregido, con el error atribuido a quien lo cometió |
| P3-3 | La descripción del hueco citaba la cortina a medias —«local build in HIS browser» omitiendo «artifacts to open»— y coronaba con un «the ONLY place his own hands touch a packaged artifact is box 8», absoluto que el propio recuadro 6 rompe. Y nada en el árbol define qué son esos artefactos, y entonces la frase aparecía UNA sola vez, en el recuadro 6 | el párrafo declara la ambigüedad en vez de resolverla a su favor, y añade «qué significa artifacts to open» a lo que el director adjudica |
| P3-4 | El recuadro 6 exige «names by hand any surface it therefore cannot reach» y el fichero lo cubría con un descargo en bloque | el fichero lista a mano las cuatro: la puerta CLI, la cura grave, las tres puertas del marcador estricto —con su `authority_snapshot_required = 0` leído del almacén— y las dos clases de error. Y las notas lo repiten |

Y una nota que el adversario no elevó: «`hooks.acme.io` exists ONLY as an
allow-list entry» era falso — el host estaba además en los parámetros de la
acción, que la pantalla imprimió. Curado.

### Lo que el barrido 3 re-derivó ejecutando

`git rev-list --count` → 13; los tres llamantes de `strictMarkerTx`; que el tag
no existe; `length(canonical_params)` → 0; `count(*) from actions where state
like '%DENIED%'` → 0; el recuento de fichas; y `authority_snapshot_required = 0`
sobre la petición de la pasada.

### Lo NO verificado, tercer barrido consecutivo

Los nombres de los elementos AX que `packaged-pass.txt` cita: harían falta
relanzar el `.app`. Su nivel de evidencia declarado sigue sin confirmar ni
refutar por un tercero. Y las §1-§7 de `manual-pass.txt`, cuyo almacén no se le
entregó.

---

## Barrido 4 — re-pasada acotada · `1 P2, 1 P3` · 10 min 40 s

### P2-1 · La lista «uno a uno» afirmaba completitud y era incompleta

El fichero de la pasada empaquetada nombraba CUATRO superficies no alcanzadas
bajo un titular que no es un descargo, sino una afirmación de completitud. Dos
contraejemplos, los dos fichados por las propias notas:

- `korvun approvals list` — otra puerta de CLI pura:
  `ApprovalsOutsideKnownStatuses` tiene un único llamante de producción,
  `internal/cli/approvals.go`.
- La mitad de CLI de la ficha del recibo: `internal/cli/receipt.go` cambió en
  este rango de `strings.HasPrefix(id, "rcpt_")` a `action.ValidReceiptID(id)`.
  La pasada ejerció la mitad de pantalla; la de CLI no.

Y las dos frases «uno a uno» —la del fichero y la de las notas— enumeraban
conjuntos DISTINTOS.

**CURADO**: la lista se sustituye por una TABLA sobre el inventario de la propia
release, de modo que la completitud no la elige nadie.

### P3-1 · El recuento que me falsifiqué al citar

«the phrase appears once in the whole tree», escrito en el mismo párrafo que
acababa de citarla por segunda vez. **CURADO** retirando el recuento; la
conclusión —que nada en el árbol define qué son esos artefactos— se sostiene sin
él.

### Lo que el barrido 4 dio limpio

Que ninguna otra frase del delta acota o exime de una ley del director; los DOCE
ficheros de test del rango con su desglose y su garantía sustantiva; el resto del
párrafo del hueco, con el absoluto ya retirado; las dos mitades de la nota de
`hooks.acme.io`; y la fidelidad de la sección «Barrido 3» de este fichero.

---

## Barrido 5 — sobre las dos curas del 4 · `2 P2, 3 P3` · ~10 min

**Los dos P2 eran aritmética mía, en cada mitad del par de frases gemelas.**

1. La derivación de completitud decía que las filas SON la tabla de fichas «one
   row each». Son diez filas para ocho fichas: la cura grave y la puerta nueva
   se quedaban sin fuente declarada, que es justo lo que el P2 del barrido 4
   pedía cerrar. **CURADO**: el párrafo declara las tres fuentes, 8 + 1 + 1.
2. Las notas repartían las SEIS filas «No» en categorías que cubrían CINCO. La
   que se caía era el error determinista de la purga — y además estaba mal
   clasificado: `ClaimApprovalParamsUnderDigest` se llama desde `internal/app`,
   es camino de SERVIDOR, mientras `GetApprovalByAction` sí es puerta de CLI
   (`internal/cli/receipt.go`). Las dos etiquetas quedaban distintas en los dos
   documentos. **CURADO**: las seis se enumeran por su nombre y las dos etiquetas
   se corrigen en los dos sitios.

**Los tres P3**: el recuento auto-falsificante sobrevivía en este mismo fichero,
que era el papel que lo denunciaba; el fichero de la pasada se llamaba a sí mismo
«this bullet» sin tener una sola viñeta; y las notas localizaban una tabla por
«arriba» cuando la más cercana era otra. **Los tres curados**, y con ellos los
siete localizadores relativos que el barrido siguiente enumeró.

---

## Barrido 6 — solo aritmética y clasificación · **LIMPIO** · ~3 min

Las cinco comprobaciones cuadran, todas ejecutadas: 8 + 1 + 1 = las diez filas,
biyectivas y sin huecos; las dos etiquetas nuevas ciertas por llamante único de
producción; las seis filas «No» enumeradas una a una y clasificadas igual en los
dos documentos; 3 + 6 + 1 = 10 contra el tallado de veredictos; y el recuento
acotado, cierto para el instante que describe.

Sin P1 ni P2. Su único P3 —siete localizadores relativos vivos en el delta, con
el peor sin referente nombrado— se curó en una sola pasada antes del commit, y
un grep sobre los ficheros del delta ya no devuelve ninguno.
