# VETO MANTENIDO — pasada INTERNA sobre las curas del tren A (2026-09-23)

Persistida por el ejecutor: el adversario no escribe dentro del árbol que audita.
Es la pasada que la norma «bien a la primera» exige ANTES de la oficial.
`1 P1, 5 P2, 3 P3.` Tiempo consumido: **~50 min** sobre un presupuesto de 30.

Árbol congelado; `git status --porcelain` devolvió las mismas 16 entradas al
cierre que al inicio. Todas las mutaciones sobre copia en scratchpad.

## P1 · `verifyAuthorityActivationTx` sin molde: neutralizarla deja todo verde

Sustituir la llamada por `if false { return "", err }` y correr los dos paquetes:

```
ok  github.com/Sebastian197/korvun/internal/action/sqlite  39.925s
ok  github.com/Sebastian197/korvun/internal/cli             8.439s
```

`TestBindWithGrant_anArmedStoreRefusesAnotherProfile` ataca solo la PRIMERA de
las dos comprobaciones de la rama armada. La segunda —el cable que el inicio
corre en `authority_v2.go:2055`— no tenía un solo assert. Misma forma que el
hallazgo del tren C un commit antes.

**Curado**: `TestBindWithGrant_anArmedStoreRefusesABrokenActivation`, con el
perfil COINCIDIENDO y la evidencia de activación borrada; mutación M-A14 roja.

## P2 · «Nothing is written» es falso, en los dos idiomas

Receta publicada, ejecutada verbatim por `cli.Run` sobre SQLite real:

```
[R2] code=1 stderr="korvun intent bind: constraint failed: UNIQUE constraint failed: index 'execution_bindings_active_selector' (2067)"
[R2] actions rows 5 -> 6 ; execution_bindings 2 -> 2 ; intent/bind actions rows=1
```

Se escribe una fila en `actions` (el acto FALLIDO) y su recibo. Lo que no se
escribe es el ENLACE. **Curado** en los dos idiomas.

Nota: las dos recetas publicadas **reproducen exactamente** el texto — la cura
documental de P1-2 funciona.

## P2 · «rojo en 4 moldes» son 8

Borrando el `UPDATE … status='REVOKED'`, ocho rojos con la misma frase
`(2067)`: `TestAuthority_ProtectedReadersUseTransactionReceiver/bind_with_a_grant`,
`…_writesTheTripleWholeOrNotAtAll`, `…_rebindRevokesTheOldRowAndCountsOn`,
`…_anArmedStoreRefusesAnotherProfile`, `…_aRivalThatCommitsFirstIsSeenNotOverwritten`,
`…_theStartStopsResolvingThroughTheConfigClause`, `…_aRivalInsideTheReadInsertWindow`,
`TestIntentBindGrantCLI_fillsTheBindingAndRevokesTheOldOne`.

Clase (h), sobre superficie pública, en la frase que declaraba haberla
verificado. **Curado**, con la razón de por qué la cifra caduca.

## P2 · Re-atar con `--grant` no reemplaza cuando la conversación difiere

```
[CONV] fixture holder: channel="webhook" conversation=<NULL>
[CONV] bind --conversation conv_A: revoked="" err=<nil>
[CONV]   row bind_conv              conv=conv_A status=ACTIVE grant=grant_root
[CONV]   row binding_authority_root conv=<NULL>  status=ACTIVE grant=grant_root
```

Dos filas ACTIVAS sobre el mismo `(actor, channel)`. La puerta usa
`ifnull(conversation_id,'')=ifnull(?,'')`; el inicio usa
`(conversation_id=? OR conversation_id IS NULL)` — comodín. El doc no mencionaba
`--conversation` en ninguna línea. **Curado** en los dos idiomas.

## P2 · `--channel '*'` escribe un enlace que ningún inicio resuelve

```
[C*] bind --channel '*' code=0 stdout="binding bind_act_… ACTIVE under grant grant_cli_root" rows=1
[GLOB2] start on the real channel with a '*' row = action/sqlite: authority missing
```

`containsAuthorityString` honra `"*"` en el GRANT; `bindingAuthorityTx` compara
`channel=?` literal. Clase (a). **Curado**: la puerta rehúsa el comodín como
canal de un enlace, con molde y mutación M-A15.

## P2 · Un bind CONFIRMADO reportado como rechazo

`recordAuthorityAct` devolvía el error del cierre como el del comando, con la
revocación y la inserción ya confirmadas. Es la clase que `85013fd` curó para
cuatro escritores el 2026-09-22; este es el quinto llamante y el primero cuya
mutación destruye una fila anterior. **PREDICCIÓN del adversario, no ejecutada**
por él. **Curado y AHORA CAPTURADO**: molde con TRIGGER real que aborta el
cierre; mutación M-A16 roja.

## Los tres P3

El letrero del molde del store armado decía «not by assignment» sobre una línea
que asigna; el molde de la ventana exigía la razón solo del UPDATE y aceptaba
«locked o busy» —y la mitad del INSERT estaba garantizada por el índice, no por
la garantía—; y el addendum enumeraba las comprobaciones sin la activación. Los
tres plegados.

## Lo que atacó y salió LIMPIO

La enumeración de `resolveAuthorityTx` contra la puerta, línea a línea, con
quince filas: las decidibles desde un enlace están todas, las que dependen de una
petición están correctamente ausentes. Las dos guardas retiradas **no se
alcanzan** —lo intentó por los dos caminos—. `openOperatorStoreSealed` no arma:
cierto, el único llamante de `RequireAuthorityActivation` fuera de tests es
`internal/app/identity.go:402`. Corrupción ≠ ausencia: sin otro camino que las
confunda. Los dos centinelas nuevos son alcanzables y sus asserts exigen su
nombre. `wasSet` por `Visit`: sin discrepancia. Atomicidad: todas las
comprobaciones preceden al `UPDATE`, y la costura es `nil` en producción. Y la
razón nueva escrita en el código —el escritor único de SQLite— es **cierta**.

## Alcance declarado

**No verificado**: el fallo de `Finish` (predicción suya; ejecutado después por
el ejecutor con un trigger); el recorrido completo CLI→inicio del enlace `'*'`
en una sola ejecución; el comportamiento bajo `-race` de los dos moldes de
carrera. **Sin examinar**: `docs/HANDOFF.md`, la spec, `make quality` completo,
`check-parity`, `govulncheck`.
