# Adversario oficial sobre el tren A de v0.16.1 — dos vueltas

Persistido por el ejecutor: el adversario no escribe dentro del árbol que audita.

## Vuelta 1 — VETO MANTENIDO · `2 P1, 4 P2, 7 P3` · ~35 min

**P1-1 · La puerta no juzgaba el CANAL.** `resolveAuthorityTx` exige el canal en
CADA grant de la cadena; `BindExecutionWithGrant` no lo comparaba con nada, y el
canal es una columna de la propia fila. Capturado en las dos superficies:

```
GAP CONFIRMED at the operator's door: exit 0,
stdout="binding bind_act_963a8560293b7cb1b6383d152a7daa3c -> int_cli_v2 version 1 ACTIVE under grant grant_cli_root",
a binding on channel webhook to a grant whose channels are [console]
```

El godoc prometía «The same verification runs here». No corría.

**P1-2 · La reparación publicada en los dos idiomas NO funciona.** Decía que ante
un grant revocado se podía re-atar «sin la bandera para caer en la cláusula»; ese
camino entra por `PutExecutionBinding`, un INSERT llano contra el índice único
parcial:

```
step 3 (documented repair) code=1 stderr="korvun intent bind: constraint failed:
UNIQUE constraint failed: index 'execution_bindings_active_selector' (2067)"
ACTIVE bindings = 1, of which still pointing at the revoked grant = 1
```

**P2-1** corrupción reportada como ausencia tras leer la cabeza · **P2-2** la
prueba de inalcanzabilidad de `ErrBindingRaceLost` no tocaba la ventana de la que
hablaba, y la razón escrita era la equivocada · **P2-3** cuatro guardas sin
molde, dos de ellas inalcanzables, y un assert either/or · **P2-4** la activación
estricta del inicio no corría en la puerta.

Siete P3: `hasFlag` por texto, «3 moldes» son 4, los identificadores de ejemplo
fuera de formato, el addendum de dos líneas, la esquina de `recordAuthorityAct`,
la tripleta cruda frente a `nullString`, y `README` nombrando un tag que no
existe.

**Los seis de producto, curados**, con su molde y su mutación roja. Entre medias,
la pasada INTERNA (`v0161-a-internal-verdict.md`) devolvió otro P1 y cinco P2,
también curados.

## Vuelta 2 — VETO MANTENIDO · `2 P2, 3 P3` · CERO PRODUCTO · ~40 min

> «Ninguno es un defecto de la lógica de la puerta: las cinco curas de código que
> el encargo mandaba verificar aguantan, y lo capturé ejecutando.»

Clasificación del propio adversario: el primer P2 es **«LETRERO, no producto: el
código hace exactamente lo diseñado»**; el segundo, **«un hallazgo de DOCTRINA,
no un defecto vivo»**.

**P2 · «omite `--conversation` para reemplazar todas» es FALSO**, en los dos
idiomas — y nació DENTRO de la cura de la pasada interna, en la dirección
contraria a la que ella capturó. Ninguna de las dos pasadas la había ejecutado.

```
[ADV-2] ACTIVE bindings after the 'replace every conversation' step = 2
[ADV-2] the row a START for conv_A resolves through: binding=bind_act_de995…
[ADV-2] the row a START for conv_other resolves through: binding=bind_act_51e1b…
```

El cable: `authority_v2.go:2293` da precedencia a la fila con conversación
nombrada. Es la instrucción que un operador sigue para ROTAR autoridad
comprometida. **Curado** en los dos idiomas, y el hueco fichado.

**P2 · El aviso del cierre tenía cable en 1 de 5 llamantes.** `nil` en los otros
cuatro y el paquete entero verde:

```
4
ok  github.com/Sebastian197/korvun/internal/cli  8.257s
```

**Curado por construcción**: `recordAuthorityAct` es ahora un método de `*cli`,
así que no hay nil que pasar, más moldes para `issue` y `revoke`.

**P3** · «el comando lo dice no imprimiendo `revoked binding`» es falso en la
segunda atada · la cifra de moldes rojos es **nueve**, no ocho, y el que faltaba
era el que la interna acababa de añadir · el canto se contradecía entre
«dieciséis» y «trece». Los tres curados.

## Lo que atacó y aguantó, con captura

El comodín `*` rehusado de punta a punta por la CLI; `--grant ""` como error de
uso; la receta del re-atado sin `--grant` reproduciendo el texto crudo de SQLite
**carácter por carácter** con lo publicado, y el acto FALLIDO donde el doc dice;
la costura `authorityAfterSelectorRead` **nil en producción**, verificada por
grep sobre todo `internal/`; M-A16 reproduciendo con su molde nombrado y solo
ése.

Y dos ataques de autoridad que montó y murieron contra el árbol: una tripleta con
`leaf.Version == 0` es imposible porque el parseo del grant firmado rechaza
`Version <= 0`; y `grantOriginActorTx` no es un hueco porque `grant_heads
.active_version` solo se INSERTA y nunca se UPDATEA, así que su evento
`revision=1` existe siempre.

**Contra su propia predicción**: sospechó que la fila «cuenta desde la anterior»
no sondaba esa mitad, montó su mutación (`b.Revision = priorRevision + 1` → `1`)
y salieron cuatro rojos. La garantía estaba cubierta, y lo dijo entero.

## Alcance declarado de la vuelta 2

**Trampa declarada por él**: su primer `cp -R` anidó el árbol sobre una copia
PRE-CURA y su primera tanda corrió contra código viejo. Lo detectó porque el
comodín salió `code=0`, repitió todo contra una copia verificada por `diff` y
DESCARTÓ la tanda vieja.

**No verificado**: la receta del grant revocado (su invocación de
`authority admin-revoke --grant` salió `code=2` — esa bandera no existe en ese
verbo— así que el grant seguía ACTIVO y la reatada posterior no prueba nada);
`make quality`, `govulncheck`, `check-parity`, `-race` sobre los dos moldes de
carrera; y M-A14/M-A15 aisladas, que toma de la pasada interna.

**Sin examinar**: `README.md`, `SECURITY.md`, el addendum de la release,
`docs/HANDOFF.md`, la spec y todo `internal/app` — el orden de prioridad que se
le dio puso esas superficies al final y ahí se le acabó el presupuesto.

## Re-pasada ACOTADA a las cinco curas — **VETO LEVANTADO** · ~19 min

Autorizada por el director al agotarse el tope de dos vueltas sin producto. Lista
cerrada de cinco puntos, quince minutos de presupuesto.

> **VETO LEVANTADO.** «Las cinco curas aguantan y lo capturé ejecutando. Un P3
> nuevo, fuera de las cinco, que NO sostiene el veto: es preexistente, no lo
> introduce ninguna cura y ninguna frase publicada es falsa por él.»

**Integridad de la copia, verificada ANTES de medir** —la trampa de la vuelta
anterior no se repitió—: `diff -r -q` con salida vacía y sha256 coincidentes en
los cuatro ficheros críticos. Al cerrar, las mismas 19 entradas de
`git status --short` que al abrir.

| Cura | Veredicto | Cómo lo probó |
|---|---|---|
| La frase de la conversación, dos idiomas | **AGUANTA** | las DOS direcciones ejecutadas con dos grants reales; **las seis frases nuevas**, una a una, ninguna se cayó |
| `recordAuthorityAct` como método de `*cli` | **AGUANTA** | los cinco llamantes por grep sobre producción; sin parámetro que pueda ser nil |
| Los moldes de `issue` y `revoke` | **FUERZAN la rama, por la razón correcta** | **dos** mutaciones: revertir la cura (3 rojos por el código de salida) y tragar el aviso (3 rojos por el stderr vacío) |
| La cifra de moldes rojos | **NUEVE, exacto** | la mutación del `UPDATE`, nueve rojos y nueve `2067` |
| La ficha del hueco | **cierta, buscada** | `grep` de `execution_bindings` fuera de `internal/action/sqlite` sin salida; ningún verbo la lista, ni en la CLI ni en la API de control |

**Su hallazgo nuevo, P3**: la puerta HERMANA —`intent bind` sin `--grant`, por
`recordOperatorAct`— sigue reportando una escritura confirmada como rechazo.
Capturado con trigger real: `ACTIVE bindings written = 1 ; exit code = 1 ;
stdout = ""`. P3 y no P2 porque esa mutación no destruye evidencia y ninguna
frase publicada es falsa por él. **Fichado** en §12 del canto y en el HANDOFF.

Y una línea más, sin desarrollar por la orden de no reabrir: `--conversation ""`
explícito se pliega con la bandera ausente, la asimetría que `--grant ""` rechaza
dos líneas más abajo. También fichada.

**Nivel de evidencia declarado por él**: todo **in-process**. Construyó el
binario y no lo usó; no vende otra cosa. No corrió `make quality`,
`govulncheck`, `-race` ni `check-parity`, y toma por declarados los datos de
cierre del encargo.
