VETO MANTENIDO

**Objeto**
- Al empezar (2026-09-25 18:05:23): `wc -l` → 406, `shasum -a 256` → `1803267c2cf33c6f1a3b953bce2da73204b1d3c5a0db91023c034f3ffb32fbbd`, mtime 18:02:06. Coincide con lo que dio el ejecutor.
- Al terminar (2026-09-25 18:28:53): 406 líneas, el mismo sha256.
- Árbol: WT=`/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `d20dea6` = `origin/master`, 135 modificados y 60 sin trackear (los 59 más el veredicto de la ronda 3). Toda cita `internal/…`, `cmd/…` o `docs/…` es `$WT/…`.
- Método: la orden prohíbe tests, compilación y mutaciones, y la cumplí. Es más restrictiva que el prompt de sistema, y manda la orden. Todo desenlace dinámico va marcado como «predicción por lectura».

## 1 · Adjudicaciones de la ronda 3 (§10 de la v5)

| Hallazgo | Veredicto | Evidencia |
|---|---|---|
| R3-1 | CIERRA | §9 declara `internal/app/profile_standing_test.go:441-442`. Lo leí: hoy exige `503 act_not_recorded` nombrando `ledger_unreadable`. También declara el test de contrato `:365-400`, que leí con sus cuatro constantes: 17, 7, 10 y 6. `bootstrap_test.go:292-293` y `config_act_registry_test.go:512-513` y `:900-901` quedan intactos por la excepción de §5-bis, y los leí. D7 decide la fila. La misma clase de fallo reaparece con la cura de R3-11 (R4-1). |
| R3-2 | CIERRA | G-E4 ya no promete abrir el libro tras un paso de migración fallido. Hoy la copia sale sin prefijo (`store.go:1255-1258`) y `Build` antepone «app: open action store:» (`app.go:380-383`). E34 acota la frase de `ledger check`. Quedan la frase gemela del abridor de operador (R4-9) y los pasos que fallan sin código (R4-2). |
| R3-3 | CIERRA A MEDIAS | El gancho, el abridor, la sonda y el lector comparten `judgeShape` (grep: `store.go:1457, 1494, 2098`; `ledger_identity.go:541`; `profile_standing.go:170`), y E02 escribe en las 12 combinaciones: la reproducción de R3-3 acaba con la escritura dentro. El quinto consumidor, `judgeIn`, llama ilegible a todo lo que no es forma actual (`profile_standing.go:174-175`), y E02 prueba «los cinco prefijos, por los cuatro consumidores» (R4-5). |
| R3-4 | CIERRA A MEDIAS | (a) E47 y la excepción de §5-bis nombran la clase, pero E47-A no se alcanza y E47-R depende de un seam de otro paquete (R4-4). (b) La clase determinista ya tiene fila y E27 la ataca. Lo que R3-4 (b) también nombraba, «la guarda con `ledger_guard_unset`», sigue sin fila (R4-10). |
| R3-5 | CIERRA | El orden de §3 casa con `supervisor.go:269-302`: `buildAndStart` en :270, `setCurrent` en :293, `persistConfig` en :294 y `setStatus` en :295/:297, que avisa al observador después de guardar el estado (:442-453). El resto también casa: `config_act_registry.go:152-158` y `config_act.go:218`, `:224` y `:233`. Los seams están donde se cierra el acto (sección 3). La cita `supervisor.go:257-262` no casa (R4-8). |
| R3-6 | CIERRA | G-E8 queda acotada a lo que hace el código: `app.go:416` (`ownsLedger`) y `:473-474` («recovery: skipped…»). E21-A2 reproduce R3-6 y E34 recoge `v0.16.2.md:103-104`. |
| R3-7 | CIERRA | Todos los seams nuevos de la matriz llevan NUEVO, y E27 usa una opción de test NUEVA de la app. El mismo problema de alcance vuelve con el seam de IOERR en `Standing` (R4-4 b). |
| R3-8 | CIERRA | E03 exige «is not a number» (`ledger_shape.go:165-167`), que es distinto de «below 1» (`:172-173`). E01 pone el seam entre dos `Exec` en autocommit (`store.go:1501-1508`) y escribe tras reabrir. E07 lee el fichero crudo antes de reabrir, y su A usa el paso 10 (`store.go:701-706`). |
| R3-9 | CIERRA | (i) §12 y E34 recogen las tres frases de R3-9. (ii) Hay comprobación del dueño tras `DO NOTHING` (sección 5). La cabecera de `ledger_identity.go:4-7` repite la promesa y falta en E34 (R4-9). |
| R3-10 | CIERRA | E06 fuerza dos puntos con un desenlace cada uno, y G-E6 ya no es una disyuntiva. |
| R3-11 | CIERRA | El consentimiento queda atado a lo que mostró la pantalla (hoy `whatsRequest`, `whats_happening.go:287-292`, no lleva dueño). La cura rompe un test aprobado sin declararlo (R4-1 a). |
| R3-12 | CIERRA | E19, E24-A, E37(4) y E43(ii) afirman el estado por la API. La fila nueva E46 vuelve a decir «pulsar «Marcar»» a nivel de app (R4-3). |
| R3-13 | CIERRA | Los dos supervisores persisten con `WriteConfigAtomic` (`serve.go:110`, `controller.go:261`). La siembra es autocommit, «o de la CLI» se retiró y G-E4 casa con `store.go:720-723`. |
| R3-14 | CIERRA | La línea 5 redefine [veredicto] sin afirmar comandos, y E33 cita solo `identity.go:433-434`. Una cita heredada no casa (R4-8). |
| R3-15 | CIERRA A MEDIAS | Gate: `/tmp/q26.txt` acaba en «Quality gate passed. EXIT=0» (14:05). `find $WT -newer <ref 14:05> -type f`, sin `.git` ni `node_modules`, devuelve solo los tres veredictos de `.claude/adversary/`. La séptima ley depende de E46, y su mutación no puede enrojecer (R4-3). |
| R3-16 | CIERRA | D4′ describe el efecto real de la marca (`ledger_identity.go:184-189`), y G-E7 ya no dice «nunca hacerlas pasar». |

Resultado: 13 CIERRA, 3 CIERRA A MEDIAS y 0 NO CIERRA.

| Ronda 2 | Veredicto | Evidencia |
|---|---|---|
| N4 | CIERRA | E04 es un PIN con mutación real (`store.go:1505`). La intercalación entre la CLI y el primer arranque acaba en `ErrNoActionStore` (`store.go:1435-1436`), y E02 y E23 lo dan como desenlace. E06 está forzada. |
| N9 | CIERRA | `ErrLedgerTransient` tiene outcome, la clase determinista tiene fila y E27 alcanza el fallo sin código. Lo que falta de la guarda está en R4-10. |
| P3-a | CIERRA | Los seams llevan NUEVO. |
| P3-e | CIERRA A MEDIAS | (b) casa. (d), «cada fila tiene su mutación», es falso para E46 y E47-A. (e) no ve G-E1 (R4-7) ni la cabecera de `ledger_identity.go` (R4-9). |
| H5 | CIERRA | E08-R ya tiene código detrás, pero solo para el código 1 (R4-2). |

## 2 · (b) El residuo juzgado fresco

`createStmt` tiene cinco sentencias, y la primera es `CREATE TABLE IF NOT EXISTS action_schema` (`store.go:101-129`), así que todo prefijo contiene `action_schema`. Un crash deja k=1..4 dentro de `createStmt` y k=5 antes de la semilla. Después de la semilla el libro ya es un v1. La lista de E02 está completa.

Qué hace cada consumidor con el residuo (predicción por lectura):
- **Gancho** (`ledger_identity.go:545-547`): lo juzga fresco y responde `ok`, sin veredicto pegajoso. Con k≥2 crea triggers temporales sobre `actions` antes de sembrar (`:486-494`). E02 cubre ese caso porque escribe después de abrir.
- **Abridor** (`store.go:1500-1512`): completa el residuo y migra. Cumple G-E1.
- **Sonda de la CLI** (`store.go:1429-1436`): con el fichero presente devuelve `ErrNoActionStore` y no completa nada. Lo mismo pasa con la rotación (`receipt.go:463`) y con el cierre transitorio (`config_act_registry.go:396`). G-E1 dice otra cosa (R4-7).
- **Lector** (`store.go:2103-2106`): `ErrNoActionStore`, coherente con E02 y E23.
- **`judgeIn`** (`profile_standing.go:170-175`): trata lo fresco como `ErrLedgerUnreadable`, un veredicto que `beginWrite` (`ledger_identity.go:179-181`) y `refreshGuard` (`:289-297`) vuelven pegajoso. Sus cinco llamadores (`:176, :212, :284, :332`; `profile_standing.go:155`) trabajan sobre un handle ya abierto, así que por lectura no llega a ver un residuo. La v5 no lo declara ni lo prueba (R4-5).

¿Distingue la v5 el residuo de un fichero ajeno o recortado que se le parezca? Solo por el vacío de las tablas. «Exactamente un prefijo» no dice si se comparan tipos o DDL, y «ninguna otra tabla» deja fuera vistas e índices. Solo E02-A ataca esa frontera (R4-6).

## 3 · (c) El cierre real

- Solo hay dos supervisores en producción y los dos cablean el observador: `korvun serve` (`internal/cli/serve.go:84-119`) y el shell (`internal/shell/controller.go:211-274`). `grep "supervisor.New("` y `grep "WithReloader("` no devuelven ningún otro camino.
- E21 (2) y (3) y E37 (4) caen antes del `setStatus` que dispara el cierre. La barrera de E44-A está en `ObserveReload`, antes de `settleHandle`, que es donde se cierra el acto cuando todo sale bien.
- E44-A es alcanzable. `Start` levanta el servidor admin antes de `Serve` (`app.go:1793-1802`), así que la app nueva atiende `POST adopt-ledger` mientras el observador espera. La puerta no puede cerrar antes: la app vieja se apaga en `supervisor.go:233`, ese apagado espera a que termine el handler en curso, y el handler solo ve `Pending`.
- Ninguna frase ni fila deja ya el cierre en la puerta: `grep SettleAct` en el plan solo da la línea 95, que es correcta.

## 4 · (d) E45, E46 y E47

- **E45.** Es alcanzable con lo que se marca como NUEVO. El libro sin marca se admite (`ledger_identity.go:211`). Su mutación enrojece: sin la comparación, el recibo choca con el trigger, sale un 1811 sin `ledger_guard:` y la respuesta es `act_not_recorded` 503 en vez de `ledger_changed` 409 (predicción). Pero el oráculo cubre menos que lo que la fila prohíbe (R4-11), y la regla «vacío = sin marca» rompe un test aprobado (R4-1 a).
- **E46.** Es alcanzable. v0.16.0 usa el esquema 15 (`git show v0.16.0:internal/action/sqlite/store.go | grep schemaVersionCurrent` → 15; en v0.16.1 también 15). Su mutación sobrevive y su nivel de evidencia queda sin definir (R4-3). La séptima ley no queda probada.
- **E47.** R solo funciona si el seam se alcanza desde el paquete `cli`. A, tal como está escrita, no se puede alcanzar (R4-4).

## 5 · (e) La comprobación del dueño tras `DO NOTHING`

- **Tests aprobados.** `grep -rn "FinishFounding("` da 13 llamadas en tests. Todas fundan un libro sin fila previa y con el digest de su propio perfil (`foundedStore`, `foundedFor`, `foreignFixture`, `identityStoreFixture` sobre `openFull`, `foundedLedger`, `foundLedgerFor` y `foundLedgerWithoutTheCLIPrincipal`), o rehúsan antes de abrir la transacción (`profile_standing_test.go:201` y `:534`). Ningún test espera que falle una segunda fundación. Los que exigen `SUCCEEDED` en `internal/shell` (`bootstrap_test.go:259-264`, `ledger_standing_test.go:48-49`) fundan sobre un libro sin fila. Predicción: no enrojece ninguno.
- **Handle sin identidad.** `beginWrite` no juzga (`ledger_identity.go:172-175`), así que solo la comprobación nueva ve una fila con otro digest (E44-A2). En producción no se llega a ese caso, y §11-Q3 lo declara.
- **Error y rollback.** Sale `ErrLedgerForeignProfile` y el `defer tx.Rollback()` (`profile_standing.go:268`) deshace el cierre y el recibo. La cabeza de la cadena se lee dentro de la transacción, sin caché (`ledger.go:54-76`). El acto se queda AUTHORIZED y `finishThrough` lo anota (`config_act.go:222-234`). No queda ningún acto a medias.

## 6 · Las marcas [veredicto]

`grep -o "\[veredicto\]" | wc -l` → 22 apariciones en 21 líneas (la 319 lleva dos). Cinco solo usan la definición (líneas 5, 302, 319×2 y 343); las otras 17 citan fichero:línea. Los veredictos que respaldan esas citas solo traen comando en N5, y la línea 5 ya no afirma lo contrario.

| Línea | Cita | Veredicto que la trae | ¿Casa en el WT? |
|---|---|---|---|
| 19 | `HANDOFF.md:440` | R3-15 | Sí |
| 32 | poda y barrido de `noteWrite` | R2 P3-n, R3 §4 | Sí: `store.go:1906` y `:1912` |
| 57 | `receipt.go:447` | R2 N5 | Sí |
| 63 | `receipt.go:463` | R2 P3-b | Sí |
| 79 | `whats_happening.go:287-292` | R3-11 | Sí |
| 89 | `supervisor.go:257-262` | R3-5 | **No**: es la rama `reasonShutdown`, no el corte (R4-8) |
| 106 | `supervisor.go:488` | R3-13 | Sí |
| 177 | `store.go:701-706` | R3-8 | Sí |
| 197 | `ledger_identity.go:669` | R3 §4 | Sí |
| 203 | `identity.go:433-434` | R3 §4 | Sí |
| 208 | `authority.go:104,162,211,252` | R2 P3-c | Sí |
| 226, 227, 230 | `v0.16.2.md` 103-104, 112-115, 21/24/29/177 | R3-6, R3-9, R3-1 | Sí |
| 284, 285, 292 | los tests de R3-1 | R3-1 | Sí |

Casan 16 de las 17.

## 7 · Hallazgos nuevos

**R4-1 · P2 · [TEST-APROBADO-NO-DECLARADO] · §4, §7-bis (a), E45, §5, §9.** Con la v5 tal como está escrita, dos tests aprobados se ponen rojos y §9 no nombra ninguno.
- (a) `internal/app/profile_standing_test.go`, `TestMount_aForeignLedgerRefusesEveryDoorButAdopt` (`:120`). El subtest de `:209` hace POST `{"confirm":true}` sobre un libro de otro perfil y exige «want 200 adopted» (`:210-212`). El de `:223` exige después el estado `ok` (`:226`). La v5 lee un dueño esperado vacío como «sin marca, no cualquiera», y un cuerpo sin ese campo llega vacío, así que `beginAdoption` devuelve `ErrLedgerChanged` y la respuesta es 409. Además, el doble `AdoptLedger(context.Context)` (`internal/controlapi/act_external_fake_test.go:52`) tiene que cambiar de firma.
- (b) `internal/action/sqlite/approval_read_class_test.go:199-227`, `TestClaim_anOperationalPurgeFailureStaysTransient`. Con `PRAGMA query_only = ON`, la purga falla con SQLITE_READONLY (8). Hoy eso sale como `ErrApprovalUnreadable` (`approvals_v15.go:448-465`, `:907-909`), y el test lo exige en `:219`. La v5 sustituye `purgeWriteFailure` por `classifySQLite`, mete el 8 en Entorno, y solo conserva el centinela de aprobación para la clase determinista.
- Reproducción (predicción por lectura):
  1. Implementar §4, E45 y el clasificador de §5 tal como están escritos.
  2. `go test ./internal/app -run TestMount_aForeignLedgerRefusesEveryDoorButAdopt` → rojo en `:212` y en `:226`.
  3. `go test ./internal/action/sqlite -run TestClaim_anOperationalPurgeFailureStaysTransient` → rojo en `:219`, salvo que se envuelvan los dos centinelas, cosa que la v5 no dice.
- Por qué P2: es la clase de R3-1 y el mismo criterio que la ronda 3 aplicó.

**R4-2 · P2 · [TAXONOMÍA][AFIRMACIÓN-FALSA] · §5 fila «Sin código SQLite», G-E3, G-E4, §5-bis, E27.** La v5 llama «del momento» a un fallo determinista que no trae código SQLite.
- §5 dice «cualquier otro → `ErrLedgerTransient`». Pero la misma tabla define la clase determinista como la que «fallará siempre igual … nunca del momento».
- En el árbol, la copia 10→11 devuelve un `*TombstoneFault` (`store.go:1219-1221`). Ese tipo no tiene `Code()`: solo `Error()` y `Unwrap()` (`store.go:853-890`). Y su propio texto dice «tombstone_corrupt … corrupt evidence demands human adjudication» (`:871-887`).
- G-E4 promete `ledger_unreadable: <causa del paso>` para cualquier paso que falle. Por §5, un fallo sin código daría `ledger_busy`.
- Hay un contrato aprobado en contra: `TestClaim_aCorruptHistoryRowIsCorruptEvidence` (`claim_classification_v0151_test.go:37-76`) fija que un error de conversión de `Scan`, que tampoco trae código, es evidencia corrupta y no un fallo transitorio.
- Ninguna fila ataca este caso. El único molde sin código, E27, fija `ledger_busy`, justo la clase equivocada.
- Reproducción (predicción por lectura):
  1. Tomar un libro v10 y ejecutar `UPDATE approval_tombstones SET policy_version = 'x' WHERE …`.
  2. Arrancar con el tren E.
  3. La copia 10→11 falla con el fault.
  4. El arranque sale con `app: open action store: ledger_busy: action/sqlite: v11 copy: tombstone_corrupt: …`, y así en cada arranque. Mientras tanto, D3 le dice al operador que vuelva en un momento.
- Por qué P2: una corrupción permanente se presenta como un fallo pasajero en el arranque, en las puertas y en la CLI. Contradice §5, G-E4 y un contrato aprobado, y no hay fila que lo pruebe.

**R4-3 · P3 · [MUTACIÓN-SOBREVIVE][AFIRMACIÓN-FALSA][NIVEL-DE-EVIDENCIA] · E46.**
- La v0.16.0 no escribía marcas. `git grep -c "ProfileMarkPrefix\|profile:sha256" v0.16.0 -- internal/action/sqlite/*.go internal/app/*.go | grep -v _test | wc -l` → 0, y `git grep -c ledger_identity v0.16.0 -- internal/action/sqlite/store.go` no devuelve nada.
- Sin recibo marcado, `seedIdentityRowV15toV16` devuelve nil y no inserta nada (`profile_standing.go:231-233`). El resultado es `legacy_unfounded` con la siembra o sin ella, así que «saltarse la semilla → R rojo (estado distinto)» es falso y la mutación sobrevive.
- La fila no dice si ese perfil real entra en la suite, lo que metería conversaciones privadas en el repositorio (§12: el libro comparte fichero con las conversaciones), o si es una captura local única.
- «Pulsar «Marcar»» a nivel de app contradice el preámbulo de §6.

**R4-4 · P3 · [ORÁCULO][NIVEL-DE-EVIDENCIA] · E47, E13-A, E26-A.**
- **E47-A.** `ledger check` abre una conexión de solo lectura nueva (`ledger.go:85` → `intent.go:98` → `store.go:2086-2098`). Un bloqueo que dure más que `busy_timeout` hace fallar el abridor antes de llegar a `Standing`, y la salida es `korvun ledger check: …` con código 1 (`ledger.go:86-88`). Si el bloqueo llega después de abrir, falla `ListReceipts` (`:96-100`). La fila no pone ninguna barrera entre la apertura y `printLedgerStanding`. Reproducción (predicción):
  1. El proceso B ejecuta `PRAGMA locking_mode=EXCLUSIVE; BEGIN IMMEDIATE; INSERT …` y no suelta.
  2. Lanzar `korvun ledger check --config p`.
  3. A los 5 s sale `…: database is locked`, código 1, sin línea de estado.
- **E47-R, E13-A y E26-A.** Usan un seam que vive en `internal/action/sqlite`, y los moldes están en `internal/cli` y en `internal/app`. Los seams de hoy no se exportan (`ledger_identity.go:669`). Es la objeción de R3-7.

**R4-5 · P3 · [ADJUDICACIÓN-NO-CIERRA R3-3][ORÁCULO] · §3, E02.** `judgeIn` es el quinto consumidor, y lo fresco lo lleva a un veredicto ilegible y pegajoso (evidencia en la sección 2). Nada lo declara inalcanzable y E02 no lo prueba. La frase «una rama que los cinco ya tienen» solo es verdad porque en `judgeIn` esa rama es «ilegible».

**R4-6 · P3 · [MUTACIÓN-SOBREVIVE] clase (g) · §4 fila «Residuo», E02-A.** Tres mutaciones sobreviven a E02: juzgar por nombres sin mirar tipo ni DDL (una vista o una tabla ajena `actions(id)` vacía), aceptar cualquier subconjunto en lugar de un prefijo (`{action_schema, action_decisions}`), e ignorar los índices. Con cualquiera de ellas, `OpenFor` toma por fresco un fichero ajeno e intenta sembrarlo, y G-E4 promete no tocar ese fichero (predicción).

**R4-7 · P3 · [MÁS-ANCHO-QUE-SU-CABLE] (e) · G-E1.** «La apertura siguiente lo completa y todas las puertas escriben» solo es verdad para `OpenFor`. Tras un crash de la CLI en la siembra, el siguiente `korvun intent create` da `no action store in the file … (not a korvun store?)` (`store.go:1435-1436`). Tras el caso (1) de «Activar almacén», volver a pulsar da `ledger_exists` (`config_act.go:393-394`). Las filas E02 y E37 dicen la verdad; la garantía no.

**R4-8 · P3 · [VEREDICTO-SIN-COMANDO][AFIRMACIÓN-FALSA] · §3 paso 8.** `supervisor.go:257-262` es la rama `reasonShutdown` (`s.shuttingDown = true … return nil`, `case reasonAppFailed:`). La app vieja se apaga en `:232-239`, tras `cancel(); <-serveErr` (`:369-371`). Otra cita menor: `RequestReload` ocupa `:379-405`, no `:383-409`.

**R4-9 · P3 · [E34-INCOMPLETA] (e).**
- `ledger_identity.go:4-7` dice «written only by the founding and the adoption in the same transaction as their receipt». Ya hoy es falso, porque la migración escribe la fila sin recibo (`profile_standing.go:243-244`), y con `DO NOTHING` lo será más.
- `store.go:1438` repite «run the server boot to lift the schema», la frase que E34 acota solo en `:2109`.

**R4-10 · P3 · [SUPERFICIE-SIN-FILA] · §5-bis.** La clase «La guarda» con `ledger_guard_unset` (`ErrLedgerGuardUnset`, `ledger_identity.go:372-373`) no tiene fila en §5-bis. No está definido qué responde una puerta ni qué imprime la CLI.

**R4-11 · P3 · [ORÁCULO] · E45.** La fila prohíbe «escribir cualquier cosa», pero el oráculo solo aborta `INSERT` en `receipts` y en `ledger_identity`. `AdoptLedger` escribe antes el acto, su decisión y su evidencia (`profile_standing.go:319`). Un mutante que confirme el acto en otra transacción y después rehúse pasaría E45.

**R4-12 · P3 · [COHERENCIA] · §5-bis, E47, E27, arranque.**
- La excepción de `ledger check` dice «unreadable, environment o busy». Hoy la CLI imprime `ledger_unreadable` y lo fijan `internal/cli/ledger_identity_test.go:84` y `ledger_binary_test.go:64`. La columna CLI de §5-bis usa `ledger_environment` y `ledger_busy`. G-E10 pide un solo nombre por superficie.
- `Build` vuelve a juzgar tras `OpenFor` y envuelve el error con `app: judge the ledger's standing: %w` (`app.go:417-422`), no con el `app: open action store: ledger_busy:` que promete §5-bis.
- E27 exige «las cinco situaciones en las seis puertas», pero un libro recién creado con `O_EXCL` por «Activar almacén» no puede ser ilegible ni traer un trigger persistente sin un seam que la fila no declara.

## 8 · Lo que la v5 hace bien, verificado

- **Orden pre-RED.** `grep -rln -E "classifySQLite|ErrLedgerEnvironment|ErrLedgerTransient|ErrLedgerChanged|seedSeam|…|TestLedgerRestoreProcedure_byBinary" internal cmd docs` no devuelve nada, y no hay `docs/operations/*restore*`.
- **§1 y el gate** están verificados (cifras del árbol, `/tmp/q26.txt` y el `find` de la sección 1).
- **Retirar `shapeSeedPartial`** simplifica de verdad: el gancho, el abridor, la sonda y el lector heredan el juicio sin código nuevo.
- **El cierre real** está en el observador, y los dos supervisores lo cablean. E44 R, A y A2 son alcanzables y sus mutaciones enrojecen (predicción).
- **Cifras re-derivadas:** 53 triggers, 24 outcomes, 6 `IF NOT EXISTS`, 12 combinaciones de `guardedTables` y 34 tablas.
- **Citas:** 16 de las 17 marcas [veredicto] casan.

## 9 · Alcance

- **Leído:**
  - El plan v5 entero y los tres veredictos anteriores enteros. Como contexto y no como fuente, §15.6 de `design-drafts/claude-code-report.md`.
  - Enteros en el WT: `ledger_shape.go`, `ledger_identity.go`, `profile_standing.go`, `config_act.go`, `config_act_registry.go` y `cli/ledger.go`.
  - Por tramos: `store.go`, `supervisor.go` (200-512), `app.go` (336-480 y 1780-1860), `whats_happening.go`, `ledger.go`, `approvals_v15.go`, `intent.go`, `receipt.go`, `serve.go`, `controller.go`, `identity.go`, `authority.go`, `firstrun_template.json`, `HANDOFF.md`, `v0.16.2.md` y `git diff -- CLAUDE.md`.
  - Los tests citados arriba.
- **Ejecutado:** solo comandos de lectura: `date`, `wc`, `shasum`, `stat`, `git` (rev-parse, status, diff, show, grep, tag), `grep`, `sed`, `awk`, `find`, `ls` y `cmp`. Un `touch` creó un fichero de referencia en mi scratchpad, fuera del árbol, para el `find -newer`. Ningún test, ninguna compilación, ninguna mutación.
- **No verificado (predicción):** todo desenlace dinámico. Eso incluye los rojos de R4-1, el camino del fault en R4-2, el BUSY en el abridor, READONLY bajo `query_only`, el oráculo de E45, la semántica de WAL frente a EXCLUSIVE y el código que da un fichero 0444.
- **Sin examinar, por orden o por tiempo:** la instantánea, la ficha UX, la maqueta, el e2e-harness, el almacén de conversaciones, la spec §12-ter y el resto del informe.
- **Tiempo:** de 18:05 a 18:31.
- **Lo que esta ronda vio y la anterior no:** R4-1 a R4-12. R4-1 (a), R4-3 y R4-11 nacen de la v5. R4-2 ya estaba en la tabla desde la v3.

## 10 · Integridad (18:28:53)

- `git -C $WT diff | cmp - …/adv-e4-before.patch` → «DIFF: identical».
- `git -C $WT status --short | grep '^??' | sort | cmp - …/adv-e4-untracked-before.txt` → «UNTRACKED: identical».
- `git -C $WT diff --cached --quiet` → sale con 0.
- Plan: 406 líneas, sha256 `1803267c2cf33c6f1a3b953bce2da73204b1d3c5a0db91023c034f3ffb32fbbd`.