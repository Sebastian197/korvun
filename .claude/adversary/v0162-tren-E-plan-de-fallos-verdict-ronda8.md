VETO MANTENIDO

**Objeto (v9, reducido)**
- Al empezar, 2026-09-26 06:12:27: `wc -l` = 324, `shasum -a 256` = `d88c744d11e4b0c720314899e40946de48a768cf89a3982028b9409b2ff1e3c6`, mtime 06:09:22. Coincide con lo medido por el ejecutor.
- Al terminar, 2026-09-26 06:33:46: 324 líneas, el mismo sha256 y el mismo mtime.
- Extracto §14.3/§14.6: 63 líneas, sha `9c277079…`. Lista de contraste: 677 líneas, sha `c207620c…`. Veredicto de la ronda 7: sha `afb5f706…`, el que cita el plan en su l.7.
- Árbol: WT = `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `d20dea6` = `origin/master`. Todas las citas `internal/…` son del WT. El driver es `$(go env GOMODCACHE)/modernc.org/sqlite@v1.59.0`.
- Método: la orden de esta ronda prohíbe tests, compilaciones y mutaciones, y no he ejecutado ninguno. Todo desenlace dinámico va marcado como «predicción por lectura». No he escrito ningún fichero auxiliar.

## (a) §10 frente a la ronda 7

| Hallazgo | Destino en la v9 | Veredicto | Evidencia |
|---|---|---|---|
| N7-1 sellado del lector | salida de `openReadOnly` (G-E2), E14 | CURADO (predicción) | El sellado es un `return` de `openReadOnly` (`store.go:2091-2094`). Clasificado a la salida, NOTADB (26) da `ledger_unreadable` y BUSY (5) da `ledger_busy`. La reproducción de N7-1 acaba con la clase. |
| N7-2 `readOwner` | salida de `openWithIdentity` y veredicto en `readOwner` (E11-A) | **NO CIERRA** | N8-1. La reproducción literal acaba con el mismo texto, porque `OpenFor` vuelve a juzgar en `installGuard`→`refreshGuard`→`judgeIn`. |
| N7-3 «no cambia ninguna puerta» | `judgeIn` sin tocar; (i) y (ii) declarados | CIERRA A MEDIAS | La reproducción de N7-3 ya da el resultado de hoy (503 `unavailable`, sin pegajosidad). Pero el efecto (i) declarado es falso (N8-2) y queda un tercer efecto sin declarar (N8-4). El (ii) es correcto y tiene molde (E02). |
| N7-4 pegajosidad sin molde | E12-H y E12-W | **NO CIERRA** | N8-3. La mutación de `isVerdict` sigue sin poner rojo nada; E12-H no pasa por el gancho. |
| N7-5 literales imposibles | `errors.Is` + clase + causa | CURADO | «table %s missing at schema v%d» (`ledger_shape.go:182`) y «no such column: …» casan. |
| N7-6 E34 incompleta | E34 ampliada | CURADO | Los nueve textos de N7-6 están en E34, o en H de forma declarada (el centinela). Los godocs nuevos que la v9 vuelve falsos son otro hallazgo (N8-7). |
| N7-7 §9 de más | quitados | CURADO | `errors_test.go:278-281` es un v16 completo más un trigger, no un residuo. El fixture TSX no lleva código. |
| N7-8 E10/E25 | tres UNIQUE; mutaciones de producción | CURADO | Hay resto P3 (N8-10). |
| N7-9 §8 | §8 corregida | CURADO | He cruzado la columna Nivel de las 27 filas con §8 y no hay discrepancias. |
| N7-10 E47-R/E23 | interfaz sin exportar; hijo + lector | CURADO | Hay resto P3 (N8-9, N8-11). |
| N7-11 lector/E11 either/or | §4 lo declara; E11 fija la línea | CIERRA A MEDIAS | La línea distingue los dos caminos, pero el código de salida sigue sin fijar (N8-8). |
| N7-12 clase por sitio | una tabla y una excepción | CIERRA A MEDIAS | N8-6: G-E5 frente a «sin clase» en §5. |
| N7-13 §10 y «50» | nombrados; 45 | CURADO | `sed -n 37,88p lista \| grep -c '^- \`'` → 50; con `sort -u \| wc -l` → 45. |
| N-5 (= N7-9) | §8 | CURADO por sucesor | |
| R5-11 (→ N-5) | §8 | CURADO por sucesor | |

**Resultado:** de los 13 hallazgos de la ronda 7, 8 CURADOS, 3 CIERRAN A MEDIAS (N7-3, N7-11, N7-12) y 2 NO CIERRAN (N7-2 y N7-4, los dos P2). N-5 y R5-11 quedan curados por sucesor.

## (b) Clasificar a la salida

**Los `return` con error:**
- `openWithIdentity`: `store.go:1470` (Abs), `:1474` (MkdirAll), `:1480` (sql.Open), `:1494-1498` (judgeShape, que incluye el nacimiento de la conexión: pragmas de la DSN y rechazos del gancho), `:1501-1503` (bootstrap), `:1505-1507` (seed), `:1509-1511` y `:1524-1526` (migrate), `:1516-1519` (readOwner), `:1528-1530` (requireCurrentSchema), `:1533-1536` (forma mala sin identidad, centinela) y `:1538-1540` (Ping).
  - Dentro de `migrate` (`:716`, `:722`, `:727`, `:729-730`) están los de `migrateStep`: `:1249`, `:1253`, `:1256-1257` (la copia, en crudo), `:1261`, `:1265` y `:1268`.
  - `readOwner` devuelve en `ledger_identity.go:623`; `requireCurrentSchema`, en `:634`, `:637` y `:640`.
- `openReadOnly`: `store.go:2081`, `:2084`, `:2088`, `:2091-2094` (el sellado), `:2098-2101` (judgeShape) y los centinelas de `:2106`, `:2109` y `:2112`.
- `judgeOnConn`: `ledger_identity.go:541-544`, `:552-555`, `:561-564` y `:573-578`.

**¿La clasificación a la salida los cubre?** Cubre todos esos `return`, incluido el sellado del lector. Pero quedan fuera los pasos que el arranque y la CLI ejecutan después de salir de esas funciones:
- en `OpenFor`, `installGuard`: `ledger_identity.go:312-315`, `:316-320` y `:321`→`refreshGuard` `:284-291`; y la poda, `:105-108`;
- en `OpenOperatorFor`, `probeShape` (`store.go:1430-1433`, sellado de la sonda en `:1454-1456`);
- en el gancho, los `return` que no son de `judgeOnConn`: `ledger_identity.go:459-462`, `:464-468`, `:478-480`, `:486-489` y `:490-494`.

El plan dice (l.23) «Así ningún paso queda fuera». Es falso: `refreshGuard` es justo el paso que deja E11 en rojo (N8-1).

**¿Tratar como `shapeBad` la forma nacida en judgeShape o readOwner rompe algún test aprobado?** No he encontrado ninguno (predicción por lectura):
- `TestOpenWithCap_corruptFileFailsTheSameWay` (`errors_test.go:119-129`): NOTADB en la primera consulta, sin identidad → `:1533-1536` sigue devolviendo error.
- `TestOpen_parentBlockedByAFileFailsLoud` (`:107-117`): el error del SO queda sin clase y la apertura sigue fallando.
- D07, D16 y D20 (`ledger_shape_test.go:196`, `:348`, `:482`) inyectan `errors.New` sin código, que queda sin clase.
- El pool cerrado de `:315-329` no trae código.
- D19 (`:436-467`) y `ledger_hook_test.go:91-126` y `:202-219` son forma por nombre.
- En la CLI: `ledger_binary_test.go:64`, `ledger_identity_test.go:84` (ErrLedgerMarkMalformed no trae código) y `ledger_test.go:177` (Stat).
- `grep 'RENAME COLUMN|DROP COLUMN'` en los tests solo toca `approvals` y `actions`, nunca una columna del juicio.

## (c) Puertas, aprobaciones y CLI de escritura

- **(i) es falso con el diseño de la v9**, y E11-P no puede llegar a su desenlace (N8-2). Aunque el gancho admita la conexión, cada puerta vuelve a juzgar con `judgeIn` antes de escribir y recibe el error crudo.
- **(ii) es correcto.** La sonda (`store.go:1430-1436`) ante un residuo da `ErrNoActionStore`; hoy cae en `openWithIdentity` y rehúsa por ilegible. Tiene molde (E02, CLI compilada) y una mutación que pone rojo («residuo malo»).
- **Hay un tercer efecto sin declarar** (N8-4): con G-E7, un v16 al que le falta un UNIQUE queda ilegible en todas las puertas.
- **Reproducción de N7-3 con la v9** (predicción): como `judgeIn` no cambia, `approve` da 503 `unavailable`, y al deshacer el renombrado vuelve a registrar. Es lo mismo que hoy.
- **Reproducción de N7-4 con la v9** (predicción): la mutación sobrevive a todas las filas, E12-W incluida (N8-3).

## (d) Seams e interfaz

- **Seams, alcanzables en el paquete `sqlite` o en un hijo reejecutado de ese paquete:**
  - `seedSeam` (E01, E23);
  - el seam de pragma (E05);
  - `afterMigrateReadVersion`, `beforeMigrationCommit` y la DSN de test (E06);
  - `afterMigrationCommit` (E07);
  - `standingQueryFault` (E13-R);
  - `openPruneSeam`, que ya existe (`ledger_identity.go:654`, E22).
- **E13-U:** `actLedger` es una interfaz (`config_act.go:60`) y ya hay un doble (`config_act_registry_test.go:938`).
- **E47-R:** una interfaz sin exportar en `internal/cli`, con un doble en `package cli`, es legal en Go.
  - El cuerpo también llama a `store.ProfileIdentity()` (`cli/ledger.go:158`), así que la interfaz necesita ese método (N8-11).
  - Los llamadores de producción (`cli/ledger.go:95`, `cli/receipt.go:123`) pasan un `*actionsqlite.Store`, que tiene `Standing` (`profile_standing.go:150`) y `ProfileIdentity` (`:137`). Siguen compilando (predicción).

## (e) Tests aprobados

Fuera de D21 no cambia ningún test aprobado que haya encontrado.
- Ningún test borra ni nombra los tres UNIQUE: `grep` de `DROP INDEX` y de sus nombres en `*_test.go` → 0.
- Ningún test construye un residuo: `grep '0 rows, want 1|DELETE FROM action_schema'` → solo `errors_test.go:278`, que no es residuo.
- El `ErrLedgerBusy` de `ledger_identity_test.go:356` sigue saliendo por `BeginTx`.
- Los 7 asserts de `openReadOnly` de la lista caen en Stat o en centinelas.
- Los 11 asserts de `printLedgerStanding` fijan su salida a través de la CLI, no de una llamada directa. La línea de §9 sobre «los tests que llaman a `printLedgerStanding`» habla de un conjunto vacío (N8-11).

## Marcas [veredicto]

`grep -n '\[veredicto\]'` da 10 líneas. La l.19 es la definición y la l.288, una referencia.

| Línea | Cita | ¿Casa? |
|---|---|---|
| l.52 | `ledger_binary_test.go:64` | Respaldada por la lista (l.405) y casa en el WT, pero solo fija dos subcadenas y descarta el código de salida (`:63`). Véase N8-8. |
| l.64 y l.157 | `store.go:440`, `:479`, `:610` | Casan, respaldadas por N7-8 y P3-2. |
| l.79 | `ledger_identity.go:615-623` | Casa (N7-2). |
| l.97 | `cli/ledger.go:149` | Casa (ronda 7, sección e). |
| l.169 | `whats_happening.go:481-485` | Casa (ronda 7 l.82). |
| l.244 | la lista | Existe y su sha coincide. |
| l.264 | `sqlite/approvals.go:252-254` y los anteriores | Casan. |

## Hallazgos nuevos

**N8-1 · P2 · [ADJUDICACIÓN-NO-CIERRA][PLAN-FILA-AUSENTE][AFIRMACIÓN-FALSA] · G-E2 y G-E4 (arranque), E11 R y A, §4 fila «Veredicto de forma…», l.23, §10 N7-2.**

Evidencia:
- Después de `openWithIdentity`, `OpenFor` llama a `installGuard` (`ledger_identity.go:82-85`), que termina en `return s.refreshGuard(ctx)` (`:321`).
- `refreshGuard` hace `standing, _, err := s.judgeIn(ctx, s.db, me)` (`:284`) y después `if !isVerdict(err) { return err }` (`:289-291`).
- `judgeIn` devuelve crudo el error de `judgeShape` (`profile_standing.go:170-173`) y el de la fila: `fmt.Errorf("action/sqlite: read the identity row: %w", err)` (`:192-193`).
- §4 deja sin cambio `judgeIn` e `isVerdict` y pone el veredicto solo en «los abridores». `installGuard`, `refreshGuard` y `OpenFor` no tienen fila.

Reproducción (la de N7-2, literal; predicción por lectura):
1. `foundedFor(t, profileA)`.
2. `ALTER TABLE ledger_identity RENAME COLUMN owner_digest TO od;`
3. `Build`. El gancho admite la conexión como `unreadable`. `readOwner` falla, se toma como veredicto y `Ping` devuelve el Store. Después `installGuard`→`refreshGuard`→`judgeIn` falla sin veredicto y `OpenFor` cierra el handle. El error: `app: open action store: action/sqlite: read the identity row: SQL logic error: no such column: owner_digest (1)`, sin clase. Es el mismo final que predijo N7-2.
4. Con `ALTER TABLE action_schema RENAME COLUMN version TO v` (paso 5 de P2-2): `app: open action store: action/sqlite: read action_schema: … no such column: version (1)`.

E11 («`OpenFor` abre bloqueado; la app arranca bloqueada») no se puede poner verde sin un cambio no declarado en `refreshGuard`, la tercera puerta del veredicto, cuyo molde es D16. Ese cambio no tiene fila ni mutación. Sus mutaciones («clasificar solo judgeShape…») tampoco prueban nada, porque la fila ya está roja.

Por qué P2: la cura principal de P2-2 no se sostiene con el diseño escrito, y la reproducción literal de N7-2 acaba igual (regla 1). No es P1 porque falla cerrado y no destruye evidencia.

**N8-2 · P2 · [TAXONOMÍA][AFIRMACIÓN-FALSA][ORÁCULO] · §1 efecto (i), E11-P, G-E3 («`judgeIn` … no cambian»), l.24, §10 N7-3.**

Evidencia:
- **Aprobación.** `DecideApprovalUnderLaw`→`beginWrite` envuelve el error con `"%w: %w", ErrApprovalUnreadable, err` (`sqlite/approvals.go:252-254`). Como `judgeIn` devuelve el error crudo, `nameTouch` no encuentra `ErrLedgerUnreadable` (`approvals_adapter.go:618`), cae en `ErrApprovalUnreadable` y devuelve `ErrApprovalsUnavailable` (`:641-645`). Resultado: 503 `unavailable`, «this is transient» (`controlapi/approvals.go:202`).
- **Pantalla.** `BeginConfigAct`→`RecordAttemptAuthenticated`→`beginWrite` (`identity_v2.go:170-172`) devuelve «record the operator act: …» (`config_act.go:132`). `whats_happening.go:481-485` responde con el texto del driver y sin `ledger_unreadable`.
- **La guarda `unreadable` que pone el gancho nunca se consulta:** `judgeIn` falla antes de cualquier INSERT (`ledger_identity.go:176-182`).
- **Solo hay una forma de que (i) sea cierto:** meter el veredicto dentro de `judgeShape`. Pero entonces `beginWrite:179-181` deja la guarda pegajosa y vuelve el paso 5 de N7-3, contra la l.24 y G-E3.
- **El estado inicial de E11-P** («app real, arrancada sobre el libro de E11») no se alcanza (N8-1).

Reproducción (predicción):
1. Una conexión de la app admitida `unreadable` sobre el libro de E11.
2. `POST /api/approvals/{id}/approve` → 503 `unavailable`.
3. `POST /api/whats-happening/enable-approvals {"confirm":true}` → 503 `act_not_recorded`, con detalle «…read action_schema: SQL logic error: no such column: version (1)».
4. La mutación de E11-P, «gancho que rehúsa», da el mismo 503 `unavailable`: no distingue nada.

Por qué P2: la declaración que sustituye al alcance falso de N7-3 es falsa a su vez, su molde no llega a su desenlace, y el único diseño que la haría cierta reintroduce la pegajosidad que la v9 dice haber retirado.

**N8-3 · P2 · [MUTACIÓN-SOBREVIVE][ADJUDICACIÓN-NO-CIERRA] · G-E3, E12-W, E12-H, §7-bis (d) («E12-W prueba `isVerdict`»), §10 N7-4 («E12-H (gancho y abridor)»).**

Evidencia:
- La DSN del escritor lleva `_txlock=immediate` (`store.go:96`), y el driver emite `sql = "begin " + c.beginMode` (modernc `tx.go:23-24`).
- En `beginWrite`, `tx, err := s.db.BeginTx(ctx, nil); if err != nil { return nil, s.mapGuardError(err) }` (`ledger_identity.go:168-171`). El BUSY de otro escritor IMMEDIATE sale ahí, antes de la rama `isVerdict` (`:176-182`). `TestWriteTx_busyIsNamed` (`ledger_identity_test.go:343-362`) ya recorre este camino.
- E12-H abre el lector, que no lleva nonce (`buildFileDSN`, `store.go:2086`), y el gancho vuelve sin juzgar (`ledger_identity.go:455-458`).

Reproducción (la mutación de N7-4, literal; predicción):
1. Añadir `|| errors.Is(err, ErrLedgerBusy) || errors.Is(err, ErrLedgerEnvironment)` a `isVerdict` (`ledger_identity.go:247-249`).
2. E12-W: el BUSY sale en `:170`, da `ErrLedgerBusy` y la escritura siguiente entra. Verde.
3. E12-H (lector, sin gancho), E05, E06, E13, E47, D07 y D16: verdes.
4. Segunda mutación: en el gancho, admitir BUSY o entorno como `unreadable`. Ninguna fila hace que el gancho encuentre BUSY o entorno. Todo verde.

Por qué P2: la garantía de no pegajosidad no tiene, en ninguna de sus dos mitades, un test que se ponga rojo (punto 4 de la doctrina), y §7-bis y §10 afirman lo contrario.

**N8-4 · P3 · [PLAN-FILA-AUSENTE][AFIRMACIÓN-FALSA] · §1 (efectos (i) y (ii) como lista cerrada; «Todo lo demás sigue igual»), G-E7, E10-A1 y A2.**

Evidencia:
- G-E7 mete en la forma los tres UNIQUE (`store.go:440`, `:479`, `:610`).
- `judgeIn` usa `judgeShape` (`profile_standing.go:170-176`), y un veredicto hace que `beginWrite` deje la guarda pegajosa (`ledger_identity.go:179-181`).
- Hoy la versión actual solo exige tablas (`ledger_shape.go:179-186`).

Reproducción (predicción):
1. v16 fundado; `DROP INDEX budget_debits_sequence;`
2. Hoy la app arranca, `approve` decide y `korvun intent create` entra.
3. Con la v9 la app arranca bloqueada, `approve` da 503 `ledger_unreadable`, las puertas de la pantalla dan 503 `act_not_recorded` y la CLI de escritura rehúsa. D2 manda restaurar una copia o empezar un libro sin historial.

Ningún molde cubre ese cambio en las puertas, y ningún test aprobado se rompe (grep → 0). Por qué P3: falla cerrado y el estado es artificial, pero es la misma clase que N7-3.

**N8-5 · P3 · [TAXONOMÍA][MUTACIÓN-SOBREVIVE] · §5, E13, E05, E12-H.**

Evidencia:
- El driver activa los códigos extendidos en cada conexión (modernc `conn.go:113`, `c.extendedResultCodes(true)`), y `Code()` devuelve ese código (`error.go:21`).
- `mapGuardError` enmascara con `code & 0xff` (`ledger_identity.go:356`) y además compara el extendido 1811 (`:361`).
- §5 da los códigos sueltos, sin decir si son primarios.

Reproducción (predicción):
1. Implementar `classify` comparando `Code()` sin máscara.
2. E05 (13), E08 y E11 (1), E14 (26) y E12-H (5) usan códigos simples. E13 inyecta un código que el plan no fija. Todas verdes.
3. En producción, un IOERR_SHORT_READ (522) queda «sin clase» y la pantalla dice `unreadable` en vez de `environment`.

**N8-6 · P3 · [TAXONOMÍA] · G-E5 l.58 («sale con la clase: forma…, `ledger_busy` o `ledger_environment`») frente a §5 l.133 («cualquier otro código … sale como hoy»), §10 N7-12.**

Evidencia:
- Un paso de migración que falla por una restricción sobre el contenido (por ejemplo, la tabla nueva `approval_digest TEXT NOT NULL UNIQUE`, `store.go:366`, y su copia «v11 copy insert %q: %w», `store.go:1231`) da 19/2067 y queda «sin clase».
- La justificación de la excepción («la copia es determinista sobre el contenido») vale igual para ese caso.
- Un `TombstoneFault` cuya causa sí trae código cae en «sin clase», y otro sin código cae en «forma».

Reproducción:
1. Comparar l.58 con l.130 y l.133.

**N8-7 · P3 · [E34-INCOMPLETA] · G-E10, E34.**

La v9 vuelve falsos estos textos, y E34 no los recoge:
- `readOwner`: «A failure to read is an error of the open, never a verdict» (`ledger_identity.go:612-614`), frente a E11-A.
- `shapeFresh`: «has no action-store table at all» (`ledger_shape.go:32-33`), cuando un residuo con hasta tres tablas es fresco.
- `shapeBad`: «zero or several version rows» (`:44-47`).
- La cabecera: «a fresh action store (no table of its own, whatever» (`:9`). E34 solo cita las líneas 10–11.
- `printLedgerStanding`: «nothing repairs it but restoring the ledger from a copy» (`cli/ledger.go:153-154`), dentro de la rama que ahora imprime `ledger_busy` y `ledger_environment`.

Reproducción:
1. Contrastar cada línea con §4 y G-E2.

**N8-8 · P3 · [ORÁCULO] · E11 (CLI), G-E4 l.52, §10 N7-11.**

Evidencia:
- `ledger_binary_test.go:63` descarta el código de salida (`out, _ := check.CombinedOutput()`), y `:64` solo mira dos subcadenas.
- En E11-R el lector abre y después recorre la cadena (`cli/ledger.go:95-142`). Predicción: «chain intact» y código 0. El plan no fija ninguno de los dos.

Reproducción:
1. Mutar para que `ledger check` salga con 1 tras una línea `ledger_unreadable`.
2. El oráculo «la salida es la de hoy» sigue verde (predicción).

**N8-9 · P3 · [NIVEL-DE-EVIDENCIA] (punto 3 de la doctrina) · E23.**

La fila no declara ninguna sincronización con el hijo:
- Con el fichero ausente, el lector da «read-only open» (`store.go:2083-2085`).
- Con solo las conversaciones, da fresco y `ErrNoActionStore` (`:2104-2106`).

Reproducción:
1. Lanzar el hijo.
2. El lector abre antes del `createStmt` del hijo (`:1501`).
3. Sale `ErrNoActionStore` sin haber tocado el residuo: el test queda verde y la mutación «residuo malo» no lo pone rojo.

**N8-10 · P3 · [MUTACIÓN-SOBREVIVE] · E10 (A1, A3); §4 «o de sus índices».**

- A1 y A3 no tienen mutación declarada.
- Ninguna fila pone un nombre de índice no UNIQUE del almacén (`actions_by_correlation`, `store.go:122`) sobre una tabla de conversaciones.

Mutación: «solo los tres UNIQUE cuentan como nombre del almacén» → todas las filas verdes (predicción).

**N8-11 · P3 · [AFIRMACIÓN-FALSA] (menores) · §9 l.248, §4 fila `printLedgerStanding`, §11 P3, G-E2.**

- §9: no hay ningún test que llame a `printLedgerStanding`; `grep` → solo `ledger.go:95` y `receipt.go:123`, los dos de producción.
- §4: una interfaz que solo tenga `Standing` no compila, porque el cuerpo llama a `ProfileIdentity()` (`ledger.go:158`).
- §11 P3: además de E05 y E13-R, entran por seams del paquete E01, E06, E07 y E23.
- G-E2: «Todo error … lleva el nombre de su clase … resolución de ruta, directorio». Esos pasos (`store.go:1468-1476`, `:2079-2085`) solo dan errores del SO sin código, y §5 los deja «sin clase».

## Lo que la v9 hace bien, verificado

- **Gate.** `/tmp/q26.txt` (14:05) acaba en «Quality gate passed.» y «EXIT=0». `find $WT -newer /tmp/q26.txt` devuelve solo `.claude/adversary/` con sus 7 veredictos.
- **Orden pre-RED.** `grep -rlnE 'func classify\(|ErrLedgerEnvironment|ledger_environment|seedSeam|standingQueryFault|afterMigrateReadVersion|beforeMigrationCommit|afterMigrationCommit|LedgerStanding(Unavailable|Environment)' internal cmd docs` sale con 1.
- **Aritmética:**
  - 34 tablas (`ledger_identity.go:674-687`);
  - 5 sentencias en `createStmt`;
  - 12 entradas en `guardedTables`;
  - 3 UNIQUE;
  - 45 tests;
  - 27 filas en §6;
  - «14, 11, 6, 2, 3, 4 y 4» P2 por ronda, que cuadra con los siete veredictos.
- **El residuo es estable en todas las versiones.** El sha de `createStmt` es `4790eb96…` en todas las etiquetas desde v0.11.0 hasta v0.16.1; antes de v0.11.0 el fichero no existe.
- **Curas que se sostienen:** N7-1, N7-5, N7-7, N7-9 y N7-13. El efecto (ii) y E02 también.
- **Citas de §3 y E34 que casan en el WT:** `store.go:1438`, `:1485`, `:2109`; `ledger_identity.go:39`, `:45`, `:244-249`, `:446-447`, `:534-536`; `act.go:75-78`, `:163-165`; `whats_happening.go:352-353`; `app.go:430`, `:451`, `:474`, `:493` (el `err` reutilizado de P3-6).

## Alcance

- **Leído:**
  - el plan v9 entero, el veredicto de la ronda 7 entero y el extracto §14.3/§14.6;
  - de la lista: la cabecera, el resumen y las secciones de `openWithIdentity`, `printLedgerStanding` y `openReadOnly`;
  - en el WT:
    - `store.go` (80-140, 712-733, 1244-1547, 2040-2121);
    - `ledger_identity.go` y `ledger_shape.go` enteros;
    - `profile_standing.go` entero;
    - `cli/ledger.go`, `cli/intent.go` (60-140), `cli/receipt.go` (95-130);
    - `app.go` (330-516), `config_act.go` (59-200, 255-279), `approvals_adapter.go` (440-652);
    - `sqlite/approvals.go` (225-275), `identity_v2.go` (160-190);
    - `whats_happening.go` (340-500), `act.go` (65-170);
  - los tests `ledger_binary_test.go`, `ledger_shape_boot_test.go`, `ledger_shape_test.go` (180-215, 300-362, 425-504), `ledger_hook_test.go` (80-220), `errors_test.go` (95-132, 262-290), `ledger_identity_test.go` (318-362), `cli/ledger_identity_test.go` (55-91), `cli/ledger_test.go` (162-183), `authority_as08_test.go` (60-105), `config_act_registry_test.go` (929-944) y `tombstone_r13_test.go` (670-719);
  - del driver: `tx.go:23-24`, `conn.go:113`, `:736-750`, `:868-895`, `error.go:21`, `sqlite.go:385-390`;
  - `git diff -- CLAUDE.md` del WT (séptima ley).
- **Ejecutado:** solo lectura (`date`, `wc`, `shasum`, `stat`, `git`, `grep`, `sed`, `awk`, `find`, `ls`, `cmp`, `go env`). Ningún test, ninguna compilación, ninguna mutación, ningún fichero auxiliar.
- **No verificado (predicción):**
  - todas las rutas de N8-1 a N8-3 en ejecución;
  - que el BUSY del sellado surja en el pragma `journal_mode`;
  - EXCLUSIVE frente a WAL;
  - el código extendido que devolvería un IOERR real;
  - que una colisión UNIQUE en la copia v10→v11 sea alcanzable sin corromper el libro a mano.
- **Sin examinar:**
  - la instantánea, la ficha UX, las decisiones y la maqueta (por la orden), y con ellas el texto de entorno de D3;
  - el TSX de la pantalla, salvo el fixture citado;
  - la lista de contraste fuera de las tres secciones leídas.
- **Tiempo:** de 06:12 a 06:34.
- **Lo que esta ronda vio y la anterior no:**
  - que `OpenFor` vuelve a juzgar después de la salida clasificada, en `refreshGuard`;
  - que el efecto (i) contradice que `judgeIn` no cambie;
  - que el BUSY de E12-W nunca llega a `isVerdict`;
  - los códigos extendidos del driver;
  - el efecto en las puertas de los UNIQUE en la forma.

## Integridad (06:33:46)

- `git -C $WT diff | cmp - …/adv-e8-before.patch` → «DIFF: identical». Al empezar (06:12) dio lo mismo.
- `git -C $WT status --short | grep '^??' | sort | cmp - …/adv-e8-untracked-before.txt` → «UNTRACKED: identical». Al empezar, también.
- `git -C $WT diff --cached --quiet` → `cached_exit=0`.
- Plan: 324 líneas, sha256 `d88c744d11e4b0c720314899e40946de48a768cf89a3982028b9409b2ff1e3c6`, mtime 06:09:22, sin cambios.
- Lista y extracto: sha `c207620c…` y `9c277079…`, sin cambios.