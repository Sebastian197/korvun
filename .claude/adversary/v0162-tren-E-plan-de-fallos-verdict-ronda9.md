VETO MANTENIDO

**Objeto (v10, reducido)**
- Al empezar, 2026-09-26 06:46:19: `wc -l` = 314, `shasum -a 256` = `4df37ef8749ab1afa1a1a077ad967c0d1ff72a69e345cc20ea1877cd65881176`, mtime 06:42:55. Coincide con lo que midió el ejecutor.
- Al terminar, 2026-09-26 07:18:58: 314 líneas, el mismo sha256 y el mismo mtime.
- Extracto §14.3/§14.6: 63 líneas, sha `9c277079…`. Lista de contraste: 677 líneas, sha `c207620c…`. Veredicto de la ronda 8 leído entero.
- Árbol: WT = `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `d20dea6` = `origin/master`. Todas las citas `internal/…` son del WT. El driver está en `$(go env GOMODCACHE)/modernc.org/sqlite@v1.59.0`.
- Método: la orden prohíbe tests, compilaciones y mutaciones, y no he ejecutado ninguno. Todo desenlace dinámico va marcado como «predicción por lectura». No he escrito ningún fichero auxiliar.

## (a) §10 frente a la ronda 8

| Hallazgo | Cura en la v10 | Veredicto | Evidencia |
|---|---|---|---|
| N8-1 | el veredicto lo decide el juez (G-E2) | CURADO en su reproducción literal (predicción) | owner_digest renombrada: el gancho admite `unreadable`; `refreshGuard`→`judgeIn`→fila con código 1→`ErrLedgerUnreadable`→`isVerdict` (`ledger_identity.go:284-292`)→abre bloqueado. Con `version` renombrada, igual. El mismo mecanismo sigue vivo en otra consulta del juicio: N9-1. |
| N8-2 | (i) cierto y pegajoso; E11-P, E11-L | CURADO en su reproducción (predicción) | `approve`: `nameTouch` encuentra `ErrLedgerUnreadable` antes que `ErrApprovalUnreadable` (`approvals_adapter.go:618-619`), así que da 503 `ledger_unreadable`. La puerta da 503 `act_not_recorded` y el detalle lleva `err.Error()` (`whats_happening.go:481-485`). Pero (i) tiene cláusulas nuevas falsas: N9-1 y N9-3. |
| N8-3 | `codedFault` + `judgeQueryFault`; E12-H/R/W | CIERRA A MEDIAS | E12-W llega a su sitio y se pone rojo. E12-R no puede llegar a `refreshGuard`, y que E12-H llegue al gancho depende de una colocación del seam que el plan no fija (N9-2). |
| N8-4 | efecto (iii) + E10-P | CIERRA A MEDIAS | El efecto está declarado. Pero la rama `approve` de E10-P no tiene aprobación pendiente y sale 404 o 409 (N9-4). |
| N8-5 | `& 0xff`; E13 con 522 | CURADO | Comparar sin máscara deja 522 sin clase, y E13 se pone rojo. |
| N8-6 | en migración, todo lo que no es del momento ni de entorno es forma; E08-A2 | CURADO | G-E5 (l.64) y §5 (l.126) dicen lo mismo. A2 es construible: v10 tiene PK `action_id` y `approval_id` sin UNIQUE (`store.go:342-352`), y v11 tiene PK `approval_id` y `approval_digest UNIQUE` (`:364-376`). Predicción. |
| N8-7 | E34 ampliada | CURADO | Los cinco textos de N8-7 están en E34. Las omisiones nuevas van en N9-5. |
| N8-8 | «chain intact» y salida 0 | CURADO (predicción) | `cli/ledger.go:141-142`. |
| N8-9 | «at-seam» y comprobación cruda | CURADO | |
| N8-10 | mutaciones de A1 y A3, más A4 | CURADO | El resto de la lista de índices va en N9-7. |
| N8-11 | menores | CURADO | Queda `standingQueryFault` (N9-10). |
| N7-3 (a medias en la ronda 8) | (i) y (iii) declarados; E11-L y E11-P | CURADO | Paso 3: 503 `ledger_unreadable`. Paso 5: sigue rehusando por la guarda pegajosa, y ahora está declarado. |
| N7-11 | E11 fija la línea, la cadena y la salida | CURADO | |
| N7-12 | clase por sitio | CURADO | Igual que N8-6. |

**Resultado:** de los 11 de la ronda 8, 9 quedan CURADOS (N8-1 y N8-2 solo en su reproducción literal) y 2 CIERRAN A MEDIAS (N8-3 y N8-4). Los tres que la ronda 8 dejó a medias (N7-3, N7-11 y N7-12) quedan curados.

## (b) La forma decidida dentro del juez

Todo lo que sigue es predicción por lectura.

**Reproducción de N8-1 (owner_digest renombrada):**
1. El gancho: `judgeOnConn` lee `owner_digest` (`ledger_identity.go:561`), falla con código 1 y la v10 lo trata como forma. Admite la conexión con guarda `ledger_unreadable`.
2. `openWithIdentity`: `judgeShape` da forma actual y luego `readOwner` falla con código 1 (`store.go:1516`). No hay fila de §4 que diga qué hace el abridor con ese veredicto (N9-9). Si lo trata como forma mala, sigue a `Ping` y devuelve el Store.
3. `installGuard` lee el catálogo y crea los triggers sin problema. `refreshGuard`→`judgeIn`→la fila da `ErrLedgerUnreadable`, `isVerdict` lo acepta y `setGuardIn` no toca nada (ya estaba `unreadable`).
4. `Standing` da veredicto, así que no hay poda. Abre bloqueado.

**Paso 5 de P2-2 (`version` renombrada):** el gancho y `openWithIdentity` ven forma mala, entran por `default` (`store.go:1532-1537`) y siguen a `Ping`. `refreshGuard` recibe el veredicto. Abre bloqueado.

En los dos casos no hace falta tocar `refreshGuard` ni `isVerdict`. **Sí** para las consultas que el plan pone en el juez.

**Qué hace `beginWrite` con la forma mala:** `isVerdict` la acepta y llama a `setGuardIn(ctx, s.db, unreadable)` (`:179-181`). La guarda queda pegajosa.
- Aprobaciones: 503 `ledger_unreadable` (`controlapi/approvals.go:244`).
- Puertas de la pantalla y adopción: 503 `act_not_recorded` con `ledger_unreadable` en el detalle (`whats_happening.go:481-485`, `:674-679`).
- CLI de escritura: `probeShape` ve forma mala, cae a `openWithIdentity` (`store.go:1434-1443`), abre bloqueado y rehúsa.
- Tras deshacer el renombrado: `setGuardIn(tx,"ok")` no pisa la guarda (`:268`). El trigger salta con 1811 y `mapGuardError` lo traduce a `ErrLedgerUnreadable` (`:370-371`). Queda pegajoso hasta reiniciar, como declaran (i) y E11-L.

**¿Son (i) a (v) todos los efectos?** No.
- La lectura de la marca en `judgeWithoutRow` queda fuera del juez: el gancho la toma por veredicto y `refreshGuard` recibe el error crudo (N9-1).
- CORRUPT o NOTADB en la consulta del catálogo no abren bloqueado (N9-3).
- `owner_digest` NULL da un error de `Scan` sin código (N9-8).
- Además, §4 clasifica la salida de `Standing`, así que un código 1 de `judgeWithoutRow` sale como `ErrLedgerUnreadable` en los lectores. Pero `beginWrite` y `refreshGuard` llaman a `judgeIn` directamente y lo reciben crudo. Los lectores y las puertas nombran distinto el mismo fallo.
- Molde por efecto: les falta molde a la rama de la CLI de escritura de (i) y (iii) (N9-10) y a la rama `approve` de (iii) (N9-4).

## (c) `codedFault` y `judgeQueryFault`

**`errors.As`:** `interface{ Code() int }` casa con cualquier tipo de la cadena que tenga ese método. `*sqlite.Error` lo tiene (`error.go:21`), y un `codedFault` con receptor por valor también. `mapGuardError` ya funciona así (`ledger_identity.go:351-356`). Las envolturas conservan `%w`: «connection hook: %w» (`driver.go:269`) y «connection guard: judge: %w» (`ledger_identity.go:471`). Es equivalente al error real (predicción).

**Alcance del seam:**
- E12-W: llega.
- E12-R: no llega.
- E12-H: depende de dónde se compruebe el seam.

El detalle está en N9-2.

**Mutaciones:**
- E12-W se pone rojo con `isVerdict` + BUSY o entorno: la segunda escritura muere en el trigger 1811.
- La mutación declarada de E12-R no lo pone rojo.
- La de E12-H solo lo pone rojo si el seam se comprueba después de ejecutar la consulta.
- Una mutación local de `refreshGuard` sobrevive a toda la matriz.

## (d) `classify` con `Code() & 0xff`

**El error real del driver:**
- Es un `*sqlite.Error{msg, code}` con `Code()` (`error.go:12-21`).
- Los códigos extendidos se activan en cada conexión (`conn.go:113`).
- `errstrForDB` construye `&Error{…, code: int(rc)}` con el `rc` que devuelven step o prepare (`conn.go:868-893`; `step` en `conn.go:446`, `rows.go:118`, `stmt.go:380`). Así, 522, 517 o 779 llegan extendidos y `& 0xff` da el primario.

**Errores envueltos:**
- `database/sql` no envuelve los errores del driver en Query, Exec ni Scan.
- `%w` y el gancho conservan la cadena.
- `mapGuardError` usa `"%w: %v"` (`ledger_identity.go:358`, `:373`). Pierde `Code()` pero deja un centinela del paquete, y §5 no vuelve a clasificar lo que ya lleva uno.

**Errores sin `Code()`:**
- `ctx.Err()` al cancelar (`stmt.go:111`, `:304`).
- Las validaciones de la DSN (`sqlite.go:310-405`).
- «sqlite3_db_config … returned %d» (`sqlite.go:170`, `:211`), con el código solo en el texto.
- `rows.Next` con destinos de más o de menos (`rows.go:127`).
- malloc (`conn.go:812`).
- Los de `database/sql` («database is closed», `ErrConnDone`, `ErrTxDone`) y los errores de conversión de `Scan`.

La v10 los trata como «sin clase», y dentro del juez como error. Es coherente con §5 y con D15, D16 y D20, salvo el NULL de N9-8.

## (e) Tests aprobados

Fuera de D21 no he encontrado ningún test aprobado cuyo assert cambie.
- D07, D16 y D20 inyectan `errors.New` sin código.
- El pool cerrado de D15 da `errDBClosed`, sin código.
- `TestOpen_corruptFileFailsAtOpenNotFirstWrite` y `TestOpenWithCap_corruptFileFailsTheSameWay` solo exigen `err != nil` (`errors_test.go:21-31`, `:119-129`). Con NOTADB tratado como forma y sin identidad siguen fallando (`store.go:1533-1536`).
- `TestOpen_seedFailureIsBootFatal` tiene un trigger sobre `action_schema` (`errors_test.go:277-280`), así que no es residuo.
- Los `TestMigrationV1x_*` y `mustFault` usan `errors.As(TombstoneFault)` y `Contains` (`tombstone_r13_test.go:322-331`). Sobreviven si la envoltura de forma usa `%w`.
- En `cli/ledger_identity_test.go:84` el código 1 viene de la cadena, no de la línea de estado.
- En `:189-192` el `DELETE` produce un centinela sin código.
- El doble `unreadableLedger` devuelve `errors.New` (`config_act_registry_test.go:919-921`).
- El fixture TSX (`WhatsHappening.test.tsx:838-847`) es un payload literal.
- `grep 'RENAME COLUMN|DROP COLUMN|DROP INDEX'` en los tests solo toca `approvals.decision_at`, `approvals.comment` y `actions.op_version`, nada del juicio.

**Excepción:** el comentario de D07 se vuelve falso con la v10 (N9-5).

## Marcas [veredicto]

`grep -n '\[veredicto\]'` da 10 líneas: la l.19 es la definición y la l.278, una referencia. Las otras ocho casan en el WT:

| Línea | Cita | ¿Casa? |
|---|---|---|
| l.70 | `store.go:440`, `:479`, `:610` | Sí (los tres `CREATE UNIQUE INDEX`). |
| l.86 | `ledger_identity.go:82-85`, `:284-291`, `:321` | Sí (ronda 8, N8-1). |
| l.89 | `profile_standing.go:192-193` | Sí. |
| l.90 | `:455-458` | Sí (ronda 8, N8-3). |
| l.92 | `store.go:1430-1456` | Sí (ronda 8, sección b). |
| l.93 | `conn.go:113`, `error.go:21` | Sí (ronda 8, N8-5). |
| l.95 | `cli/ledger.go:149`, `:158` | Sí (ronda 8, N8-11). |
| l.165 | `whats_happening.go:481-485` | Sí. |

## Hallazgos nuevos

**N9-1 · P2 · [ADJUDICACIÓN-NO-CIERRA][AFIRMACIÓN-FALSA][PLAN-FILA-AUSENTE] · G-E2 (l.54), efecto (i) (l.36-43), §10 (l.271), §11 P4 (l.284), E11.**

La lectura de la marca en `judgeWithoutRow` queda fuera del juez que define G-E2. Sin embargo, es lo que ejecuta `judgeIn` en todo libro sin fila de identidad. El gancho convierte esa misma lectura en veredicto y `refreshGuard` recibe el error crudo. Es el mecanismo de N8-1, en otra consulta.

Evidencia:
- G-E2: «El juez está formado por `judgeShape`, `readOwner`, la lectura de la fila en `judgeIn` y las lecturas del gancho en `judgeOnConn`». §3 limita `judgeIn` a `profile_standing.go:167–195`.
- Sin fila, `judgeIn` hace `return s.judgeWithoutRow(ctx, q)` (`profile_standing.go:190-191`). Esa función lee `SELECT result_digest FROM receipts WHERE partition = ? AND outcome = ? AND result_digest LIKE ? ORDER BY chain_seq DESC LIMIT 1` (`:202-206`) y, si falla, devuelve `fmt.Errorf("action/sqlite: read the profile mark: %w", err)` (`:210-211`), sin veredicto.
- `judgeOnConn` hace la misma lectura cuando `rows == "0"` (`ledger_identity.go:572-578`). Es una «lectura del gancho», así que con la v10 es veredicto.
- `refreshGuard` tiene `if !isVerdict(err) { return err }` (`:289-291`), y `OpenFor` cierra el handle (`:82-85`).
- A quién afecta: a todo libro anterior a la v0.16.2 migrado sin marca («Ledgers with no mark get no row», `profile_standing.go:222`; nota `docs/releases/v0.16.2.md:144`) y a todo libro antes de fundarse.

Reproducción (predicción por lectura):
1. `OpenFor(t.TempDir()+"/korvun.db", profileA)` y `Close()`. Queda un v16 sin fila, `legacy_unfounded`, como en D01 y C05.
2. En conexión cruda: `ALTER TABLE receipts RENAME COLUMN result_digest TO rd;`
3. `OpenFor` de nuevo:
   - El gancho ve COUNT = "0". La lectura de la marca falla con «no such column: result_digest (1)», eso es forma, y admite la conexión `unreadable`.
   - `openWithIdentity`: forma actual; `readOwner` da `ErrNoRows`; `migrate` no hace nada; `Ping`; devuelve el Store.
   - `refreshGuard`→`judgeIn`→`judgeWithoutRow` recibe el mismo error de código 1, sin veredicto. `OpenFor` cierra el handle.
4. `Build` da `app: open action store: action/sqlite: read the profile mark: SQL logic error: no such column: result_digest (1)` (`app.go:380-383`). Pasa en cada arranque y el error no lleva clase: la salida de `installGuard` no es ninguna de las tres que clasifica §4.
5. Variante: CORRUPT (11) en una página de `receipts` del mismo libro. Mismo final.
6. Con la app en marcha sobre un libro legado, aplicando el paso 2 desde otra conexión:
   - `POST approve`: `beginWrite` devuelve el error crudo, que acaba envuelto con `"%w: %w", ErrApprovalUnreadable` (`sqlite/approvals.go:252-254`). Resultado: 503 `unavailable`, «this is transient» (`approvals_adapter.go:641-645`, `controlapi/approvals.go:201-202`).
   - `enable-approvals`: 503 `act_not_recorded` con el texto del driver.
   - Mientras tanto `Standing`, clasificado, dice `ledger_unreadable`.

E11 solo usa un «v16 fundado», donde `judgeWithoutRow` nunca corre.

Por qué P2: la cura de P2-2 (el arranque real muere con texto de driver) no se sostiene en el estado más común de un libro tras actualizar, y el plan la da por curada (§10, §11 P4). No es P1: falla cerrado y no destruye nada.

**N9-2 · P2 · [SEAM-INALCANZABLE][MUTACIÓN-SOBREVIVE][ADJUDICACIÓN-NO-CIERRA] · preámbulo de §6 (l.136), E12-R, E12-H, G-E3, §7-bis (d), §10 N8-3.**

Evidencia:
- El preámbulo dice «en la primera consulta del juez, sea cual sea su llamador …, una sola vez».
- En `OpenFor` sobre un v16 sano, las consultas del juez van en este orden:
  1. `judgeShape` de `openWithIdentity` (`store.go:1494`). Su QueryContext hace nacer la conexión, y dentro corre el gancho: `judgeOnConn`→`judgeShape` (`ledger_identity.go:469`, `:541`).
  2. `readOwner` (`store.go:1516`).
  3. Por último, `refreshGuard`→`judgeIn` (`:321`, `:284`).
- Entre la primera y la tercera no hay ningún punto de armado (`:74-85`), y la fila no declara ninguno.
- E12-R: el fallo se gasta en la primera consulta. `openWithIdentity` devuelve la clase (`store.go:1494-1498`) y E12-R se pone verde sin que `refreshGuard` vea nada. Su mutación (`isVerdict` más BUSY o entorno) no lo pone rojo, porque ese camino no llama a `isVerdict`.
- E12-H: si el seam se comprueba antes de emitir la consulta, el primero que lo consulta es el juez de pool de `openWithIdentity`, que devuelve antes de que nazca la conexión. El gancho no ve el fallo.
- Ninguna de las dos filas observa qué llamador consumió el fallo (punto 3 de la doctrina).
- El árbol ya resolvió estos dos alcances de otra forma: el gancho con un seam propio (`hookShapeFault`, `:464-468`, D07) y `refreshGuard` con una llamada directa (D16, `ledger_shape_test.go:354`).

Reproducción (predicción por lectura):
1. M1: solo en `refreshGuard`, `if !isVerdict(err) && !errors.Is(err, ErrLedgerBusy) && !errors.Is(err, ErrLedgerEnvironment) { return err }`.
2. E12-R, tal como está escrita: verde.
3. E12-W: verde, porque M1 no toca `beginWrite`.
4. E12-H: verde.
5. D16 inyecta `errors.New` (`ledger_shape_test.go:345-351`): verde.
6. M1 sobrevive, y su efecto es el prohibido por G-E3: abrir bloqueado.
7. M2, en el gancho, «BUSY o entorno pasan a `unreadable`», con el seam comprobado antes de la consulta: E12-H verde.

§7-bis (d) («E12-R … ponen rojo `isVerdict`») y §10 N8-3 son falsos para E12-R.

Por qué P2: una de las tres patas de G-E3 no tiene rojo posible y otra depende de una colocación que el plan no declara. Es el punto 4 de la doctrina, igual que N8-3.

**N9-3 · P3 · [AFIRMACIÓN-FALSA][PLAN-FILA-AUSENTE] · efecto (i), l.36-37 («o con CORRUPT o NOTADB en esa consulta … El arranque abre bloqueado») y l.35.**

- NOTADB falla al nacer la conexión, en `journal_mode(WAL)` de la DSN (`conn.go:151`→`sqlite.go:451`), antes del gancho (`driver.go:266-270`). Luego `Ping` (`store.go:1538-1540`) vuelve a fallar. `OpenFor` devuelve error y no abre bloqueado.
- CORRUPT en `sqlite_master`: también leen el catálogo, fuera del juez, el gancho (`ledger_identity.go:486-489`) e `installGuard` (`:312-315`). El gancho rehúsa la conexión y `OpenFor` falla.
- Ninguna fila inyecta CORRUPT ni NOTADB en una consulta del juez.

Por qué P3: falla cerrado y con clase. Lo falso es el ejemplo de la declaración, y además no tiene molde.

**N9-4 · P3 · [ORÁCULO] · E10-P (l.151).**

El estado inicial no incluye una aprobación pendiente.
- `Approve` empieza por `ApprovalStatusOf` (`approvals_adapter.go:479-482`). Sin fila da `ErrApprovalNotFound` (`approvals_v15.go:1007-1008`) y un 404 (`controlapi/approvals.go:199`).
- Con las aprobaciones apagadas da 409 (`:203`; `approvals_adapter.go:471-473`).

Así que «la aprobación, 503 `ledger_unreadable`» es inalcanzable. E11-P sí declara la aprobación pendiente.

**N9-5 · P3 · [E34-INCOMPLETA][TEST-APROBADO-NO-DECLARADO] · E34, G-E10, §9.**

Estos textos se vuelven falsos con la v10 y no están en E34:
- `docs/HANDOFF.md:385-387`: «un FALLO DE CONSULTA en el hook rehúsa la conexión … nunca es un veredicto».
- `:396-397`: «ninguna de las tres puertas del veredicto … convierte un fallo de consulta en veredicto».
- `:377-383`: las definiciones de FRESCO y MALA (con la v10 el residuo es fresco y un UNIQUE que falta es forma mala).
- El comentario de D07, un test aprobado (`ledger_shape_test.go:186-188`): «a query failure while judging a new connection REFUSES the connection». Corregirlo toca un test que §9 no declara; no corregirlo viola G-E10.

Además, E34 (l.174) sitúa «TODA otra forma…» en el HANDOFF. `grep -n -i 'otra forma' docs/HANDOFF.md` solo da la l.748, en otro contexto. La frase está en `docs/superpowers/specs/2026-09-24-v0162-el-marcador-rediseñado-pretest.md:84` y `:646`.

**N9-6 · P3 · [NIVEL-DE-EVIDENCIA] · la variante real de E12-H; §7 y §8 («Dos procesos: E12-H (variante)»); E47-A.**

- El árbol ya registra que no llega: `ledger_shape_test.go:5-7` («D06, the real exclusive lock, could not reach the hook — captured in red.txt — and was retired»), con la captura en `red.txt:385-386`. El papel lo había previsto: «si el lock exclusivo hace fallar la conexión ANTES del hook (los pragmas del DSN)» (pretest `:972-974`).
- Un lock que bloquea a los lectores hace fallar el pragma en `newConn` (`conn.go:151`) antes del gancho.
- `BEGIN EXCLUSIVE` en WAL no bloquea a los lectores (documentación de SQLite; no verificado aquí).
- Si hay «mismo desenlace», vendrá del pragma o de la poda (`ledger_identity.go:105-108`), no del gancho.
- La fila tampoco dice de qué EXCLUSIVE se trata.

**N9-7 · P3 · [MUTACIÓN-SOBREVIVE] · G-E7 (l.69-70), E10.**

- `grep -rhoE --exclude='*_test.go' 'CREATE (UNIQUE )?INDEX …' internal/action/sqlite | sort -u` da 11 nombres. E10 solo ejercita los 3 UNIQUE y `actions_by_correlation`, y no hay un molde tipo D02 (`ledger_shape_test.go:95-123`) para la lista de índices.
- M3: quitar `grant_events_by_grant` de la lista deja todas las filas verdes.
- M4: la forma actual comprueba nombre y `tbl_name`, no que el índice sea UNIQUE. Con `DROP INDEX budget_debits_sequence; CREATE INDEX budget_debits_sequence ON budget_debits(account_id,operation_key,sequence);` la forma sigue siendo actual y E10 queda verde. Es el ataque de P3-2 sobre la tabla correcta.

Predicción.

**N9-8 · P3 · [TAXONOMÍA] · G-E2 («o sin código, devuelve el error»), la clase de P2-2.**

- El gancho lee un `owner_digest` NULL como "<nil>" (`driverScalar`, `ledger_identity.go:602-609`), que no es canónico, y responde `unreadable` (`:565-567`).
- `readOwner` (`:616`) y `judgeIn` (`profile_standing.go:178`) hacen `Scan` sobre un `string` y reciben un error de conversión sin `Code()`.

Reproducción (predicción):
1. Reconstruir `ledger_identity` sin NOT NULL y hacer `UPDATE … SET owner_digest = NULL`.
2. `Build` da `app: open action store: action/sqlite: read the identity row: sql: Scan error … converting NULL to string is unsupported`, en cada arranque.

El gancho y el juez vuelven a no coincidir.

**N9-9 · P3 · [PLAN-FILA-AUSENTE] · §4, fila del juez; E11-A.**

`readOwner` se llama dentro de `case shapeOlder, shapeCurrent, shapeNewer` (`store.go:1513-1531`), y hoy su error cierra la apertura (`:1516-1519`). La forma mala se trata en `default` (`:1532-1537`), que desde ahí no se alcanza. Su firma `(owner, found, err)` (`ledger_identity.go:615`) no tiene sitio para un veredicto. Ninguna fila de §4 dice qué hace el abridor con él, y E11-A depende de ese cambio.

**N9-10 · P3 · [AFIRMACIÓN-FALSA], menores.**
- §11 P2 declara `standingQueryFault` como NUEVO, pero ninguna fila lo usa: E13 va por `judgeQueryFault` (l.136).
- §10 N8-3 dice «E12-H … con la mutación de `isVerdict`», pero el gancho no llama a `isVerdict` (`ledger_identity.go:450-496`).
- Ninguna fila ejecuta un verbo de escritura de la CLI bajo (i) ni bajo (iii): la pata CLI de E11 es `ledger check`, que es un lector. Eso contradice «cada efecto tiene su molde» (l.35).
- La rama «Gancho: ok» de E02 entra por `judgeOnConn`, un helper privado (como `judgeOnRawConn`, `ledger_hook_test.go:58-80`), y §11 P3 no lo declara.

## Lo que la v10 hace bien, verificado
- **Gate.** `/tmp/q26.txt` (14:05) acaba en «Quality gate passed.» y «EXIT=0». `find $WT -newer /tmp/q26.txt -not -path '*/.git/*'` solo devuelve `.claude/adversary/` y sus 8 veredictos.
- **Orden pre-RED.** `grep -rlnE 'func classify\(|ErrLedgerEnvironment|…|judgeQueryFault|codedFault|…' internal cmd docs` sale con 1.
- **Aritmética.** 34 tablas; 12 entradas en `guardedTables`; 5 sentencias en `createStmt`; 3 UNIQUE; 45 tests distintos entre 50 asserts; 30 filas en §6; P2 por ronda 14, 11, 6, 2, 3, 4, 4 y 3.
- **La forma dentro del juez** abre bloqueado en las reproducciones literales de N8-1 y del paso 5 de P2-2 sin tocar `refreshGuard` ni `isVerdict` (predicción).
- **E12-W** llega a su sitio y se pone rojo. `classify` con `& 0xff` reconoce el error real y sus códigos extendidos. E08-A2 es construible.
- **§8 casa** con la columna «Nivel» de las 30 filas.

## Alcance
- **Leído:**
  - el plan v10 entero, el veredicto de la ronda 8 entero, el N7-3 de la ronda 7 y el extracto §14.3/§14.6;
  - de la lista: la cabecera, el resumen y la sección de `openWithIdentity`;
  - en el WT, enteros: `ledger_identity.go`, `ledger_shape.go`, `profile_standing.go`, `cli/ledger.go` y `ledger_shape_boot_test.go`;
  - en el WT, por tramos: `store.go` (60-150, 338-380, 690-750, 1180-1560, 1812-1830, 2030-2121), `app.go` (370-505), `approvals_adapter.go` (440-654), `config_act.go` (110-200), `whats_happening.go` (340-360, 455-500, 640-690), `act.go` (60-90, 155-175), `sqlite/approvals.go` (240-262), `approvals_v15.go` (1003-1014), `cli/intent.go` (90-139), `ledger_shape_test.go` (1-505), `ledger_hook_test.go` (60-300), `errors_test.go` (18-135, 258-292), `authority_as08_test.go`, `authority_as11_test.go` (20-47), `cli/ledger_binary_test.go`, `cli/ledger_identity_test.go` (55-195), `cli/ledger_test.go` (160-185), `config_act_registry_test.go` (860-944), `tombstone_r13_test.go` (322-347), `tombstone_r12_test.go` (276-295), `WhatsHappening.test.tsx` (820-860), el HANDOFF (370-400), la nota v0.16.2 (130-160), el pretest (80-88, 640-652, 945-1021), `red.txt` (355-401) y `git diff -- CLAUDE.md`;
  - del driver: `error.go`, `conn.go` (50-160, 425-460, 690-760, 850-895), `driver.go` (225-300), `sqlite.go` (285-480), `rows.go` (105-160) y `stmt.go` (280-380).
- **Ejecutado:** solo lectura (`date`, `wc`, `shasum`, `stat`, `git`, `grep`, `sed`, `awk`, `find`, `ls`, `cmp`, `go env`). Ni tests, ni compilaciones, ni mutaciones, ni ficheros auxiliares.
- **No verificado (predicción):**
  - todos los desenlaces dinámicos de (b), (c) y de N9-1 a N9-9;
  - EXCLUSIVE frente a WAL;
  - que el FULL dé 13;
  - que NOTADB salte en el pragma `journal_mode`.
- **Sin examinar:**
  - la instantánea, la ficha UX, las decisiones y la maqueta (por la orden);
  - el TSX, salvo el fixture citado;
  - la lista de contraste fuera de las secciones leídas.
- **Tiempo:** de 06:46 a 07:19, 33 minutos. Me pasé tres del presupuesto.
- **Lo que esta ronda vio y la anterior no:**
  - `judgeWithoutRow` queda fuera del juez, en el estado más común de los libros;
  - el seam no alcanza `refreshGuard` y depende de su colocación para llegar al gancho;
  - D06 ya había demostrado que un lock real no llega al gancho;
  - los textos del HANDOFF y de D07 que la v10 vuelve falsos.

## Integridad (07:18:58)
- `git -C $WT diff | cmp - …/adv-e9-before.patch` da «DIFF: identical». Al empezar (06:46:32) dio lo mismo.
- `git -C $WT status --short | grep '^??' | sort | cmp - …/adv-e9-untracked-before.txt` da «UNTRACKED: identical». Al empezar, también.
- `git -C $WT diff --cached --quiet` da `cached_exit=0`.
- El plan sigue con 314 líneas, sha256 `4df37ef8749ab1afa1a1a077ad967c0d1ff72a69e345cc20ea1877cd65881176` y mtime 06:42:55, sin cambios. El extracto y la lista siguen con `9c277079…` y `c207620c…`.