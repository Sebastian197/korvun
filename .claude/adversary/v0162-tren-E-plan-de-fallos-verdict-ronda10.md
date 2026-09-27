VETO MANTENIDO

**Objeto (v11, reducido)**
- Al empezar, 2026-09-26 07:44:38: `wc -l` = 359, `shasum -a 256` = `31338ea2c9742495c5f3ce3e4b39b4a02c8668c434717c1e068d9cb0b4f2d1ad`, mtime 07:36:46. Coincide con lo que midió el ejecutor.
- Al terminar, 2026-09-26 08:17:39: 359 líneas, el mismo sha256 y el mismo mtime.
- Extracto §14.3/§14.6: 63 líneas, sha `9c277079…`. Lista de contraste: 677 líneas, sha `c207620c…`. He leído entero el veredicto de la ronda 9.
- Árbol: WT = `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `d20dea6` = `origin/master`. Las citas `internal/…` son del WT. Driver: `modernc.org/sqlite@v1.59.0`. Toolchain: go1.26.6.
- Método: la orden prohíbe tests, compilaciones y mutaciones, y no he ejecutado ninguno. Todo desenlace dinámico va marcado como «predicción por lectura».

## (a) §10 frente a la ronda 9

| Hallazgo | Cura en la v11 | Veredicto | Evidencia |
|---|---|---|---|
| N9-1 | `judgeWithoutRow` dentro del juez; E11-B, E11-L-B | CURADO (predicción) | Repro literal, paso 3: la lectura de la marca (`profile_standing.go:202-211`) falla con código 1 → forma → veredicto → `refreshGuard` lo acepta (`ledger_identity.go:289-292`) → abre bloqueado. Paso 6: `approve` → `nameTouch` (`approvals_adapter.go:618-619`) → 503 `ledger_unreadable`. La variante CORRUPT (paso 5) sigue sin molde: N10-3. |
| N9-2 | un seam por sitio | CIERRA A MEDIAS | M1 en `refreshGuard` pone E12-R en rojo (OpenFor abre con la guarda pegajosa). M2, el paso 7 de la ronda 9 («el gancho admite BUSY/entorno, con el seam comprobado antes de la consulta»), deja E12-H en verde: `hookShapeFault` está en `ledger_identity.go:464-468` y devuelve antes de `judgeOnConn` (`:469`). Ver N10-2. |
| N9-3 | retirado de (i); «Qué no se promete»; (vi) | CIERRA A MEDIAS | La cláusula falsa ya no está en (i). Sigue la parte [PLAN-FILA-AUSENTE]: G-E2 (l.65) mantiene 11, 20, 24 y 26 como forma dentro del juez y ninguna fila los fuerza ahí. E14 fuerza 26 fuera del juez (`store.go:2091-2093`). Ver N10-3. |
| N9-4 | aprobación pendiente y aprobaciones encendidas | CURADO (predicción) | E10-P (l.158). |
| N9-5 | E34 ampliada; D07 en §9; cita del pretest | CURADO en su letra | HANDOFF `:377-383`, `:385-387`, `:396-397` y pretest `:84`, `:646` casan. Las omisiones nuevas van en N10-8. |
| N9-6 | variante retirada; E47-A declara su predicción | CIERRA A MEDIAS | La variante de E12-H ya no está en §7 ni en §8. E47-A sigue diciendo «una transacción EXCLUSIVE» sin precisar cuál. Esa era la otra mitad literal de N9-6. Con el libro en WAL, `BEGIN EXCLUSIVE` no bloquea a los lectores (documentación de SQLite, no verificada aquí), así que «salida 1 y `ledger_busy`» se queda sin mecanismo. |
| N9-7 | E10-L y E10-A5 | CURADO (predicción) | M3 pone E10-L en rojo y M4 pone E10-A5 en rojo. El ataque adyacente va en N10-7. |
| N9-8 | error de `Scan` en el juez = forma; E11-C | CURADO en su reproducción (predicción) | `readOwner` (`ledger_identity.go:616`) y `judgeIn` (`profile_standing.go:178`) pasan a forma. El gancho ya daba `unreadable` (`driverScalar`, `:602-609`). Cómo se reconoce ese error: N10-3. |
| N9-9 | §4 lo declara | CURADO en su letra | La cura rompe G-E5: N10-1. |
| N9-10 | `standingJudgeFault`; mutación de E12-H; E11-CLI; §11-3 | CURADO en su letra | La nueva mutación de E12-H no llega a la rama de producción (N10-2). |
| N8-1 | — | CURADO (predicción) | Con `judgeWithoutRow` dentro, ninguna consulta del juicio de identidad queda fuera cuando falla. |
| N8-2 | — | CURADO | (i) ya no lleva las cláusulas falsas de N9-1 y N9-3. |
| N8-3 | — | CIERRA A MEDIAS | Como N9-2. |
| N8-4 | — | CURADO | Vía N9-4. |

**Resultado:** de los 10 hallazgos de la ronda 9, 7 quedan CURADOS (N9-5, N9-9 y N9-10 solo en su letra; la cura de N9-9 abre N10-1) y 3 CIERRAN A MEDIAS (N9-2, N9-3 y N9-6). De los que venían de antes, N8-1, N8-2 y N8-4 quedan curados y N8-3 cierra a medias.

## (b) El juez como funciones enteras

Consultas que, sobre un v16, pueden cerrar `OpenFor`, `refreshGuard` o `beginWrite`. Todas son predicción por lectura.

| Consulta | ¿En el juez? | ¿Declarada? |
|---|---|---|
| Pragmas de la DSN al nacer la conexión (`store.go:91`; `conn.go:151`) | no | Sí para 11 y 26 («Qué no se promete»). El resto recibe clase por (vi). |
| Gancho: `CREATE/DELETE/INSERT temp.profile_guard` (`ledger_identity.go:473-481`) | no | Solo por (vi). |
| Gancho: catálogo (`:486-489`) | no | Sí (11 y 26). |
| Gancho: `CREATE TEMP TRIGGER` (`:490-494`) | no | **No**: N10-6(a). |
| `judgeOnConn`, `judgeShape`, `readOwner`, `judgeIn` y `judgeWithoutRow` | sí | — |
| `migrate`: `SELECT version … Scan(&v int)` (`store.go:714-716`) | no | **No**: N10-6(b). |
| `requireCurrentSchema` (`ledger_identity.go:633`) | no | **No**: N10-6(b). |
| `Ping` (`store.go:1538`) | no | Sí: solo falla si falla el nacimiento. |
| `installGuard`: catálogo y triggers (`:312-320`) | no | Catálogo sí. Los triggers son redundantes con el gancho. |
| `setGuardIn` en `refreshGuard` (`:297`) | no | Solo por (vi). |
| `Standing` dentro de `OpenFor` (`:91-97`) | sí | Falta en la ruta del §3 (N10-9). |
| Poda: `BeginTx` → `mapGuardError`; `COUNT(*) FROM actions`; `DELETE`; `Commit` (`store.go:1768-1808`) | no | Solo con palabras genéricas: §12, «CORRUPT o NOTADB fuera del juez hacen fallar la apertura». |
| `beginWrite`: `BeginTx` (`:168-171`) y `setGuardIn(tx,"ok")` (`:193-196`) | no | Se quedan como hoy. Las puertas son del tren H. |

Hay un efecto de primer orden: el juez del pool (`judgeShape` en `store.go:1494`) recibe el rechazo del gancho como fallo de su propia consulta, y la cadena conserva el código interno (`driver.go:269` «connection hook: %w»). Si el gancho rehúsa por 11 o 26, fuera del juez, ese juez del pool lo juzga forma. `Ping` vuelve a fallar después (`store.go:1538-1540`), así que la apertura falla igual. Es coherente con «Qué no se promete».

**¿Son (i)–(vi) todos los efectos?** No.
- El cambio de `readOwner` abre bloqueado un libro **más nuevo** (N10-1).
- `printLedgerStanding` también cambia la línea de `receipt verify` (N10-4).
- La pata `OpenOperatorFor` de (vi) no tiene molde (N10-4).

## (c) Seams por sitio

- `hookShapeFault` se consume en `ledger_identity.go:464-468`, dentro del gancho. El gancho corre dentro de quien haga nacer la conexión. Hoy lo usan tres tests:
  - D07 (`ledger_shape_test.go:197`), a través de `BeginTx`;
  - D16 (`:352`), que lo hace consumir por `refreshGuard` con `SetConnMaxLifetime(1ms)`;
  - D20 (`:486`), que lo hace consumir por el `Standing` de `OpenFor` vía `poolLifetimeForTest`.
- En producción no nace ninguna conexión nueva dentro de `refreshGuard`, `beginWrite` ni `Standing`: `SetMaxOpenConns(1)` y sin lifetime (`store.go:1485-1488`). Solo nace si el pool desecha la que tiene.
- `OpenFor` llama a `refreshGuard` una vez (`:321`), pero llega a `judgeIn` tres veces:
  - por `refreshGuard` (`:284`);
  - por `Standing` (`:91` → `profile_standing.go:155`);
  - por la poda → `beginWrite` (`:105` → `store.go:1768` → `:176`).
- `Standing` no llama a `refreshGuard`. `standingJudgeFault` armado antes de un `OpenFor` lo consume el propio `OpenFor`.

Mutaciones (predicción por lectura):
- **E12-H:** rojo solo si la mutación se escribe en la rama del seam (`:464-468`), que solo existe para los tests. En `judgeOnConn` o en `:470-472` sigue verde (N10-2).
- **E12-R:** con M1, rojo. Con `isVerdict` ampliado, rojo: `OpenFor` abre con la guarda pegajosa.
- **E12-W:** con `isVerdict` ampliado, rojo: la segunda escritura muere en el trigger 1811 (`:370-371`). Pero su primer oráculo, «falla con la clase», no es alcanzable si el seam sustituye a `judgeIn` en la llamada (N10-2).
- **E13:** sin la máscara, 522 queda sin clase: rojo. «Todo `unreadable`» pone U en rojo. `522 & 0xff = 10`.
- **`codedFault`:** pasa por `errors.As` igual que `*sqlite.Error` (`error.go:21`, receptor puntero; `interface{Code() int}` casa con receptor valor). Las envolturas `%w` (`:466`, `:471`; `driver.go:269`) conservan la cadena, y `classify` lo alcanza.

## (d) El cambio de `readOwner`

- Único llamador: `store.go:1516`. `grep -rn 'readOwner' --include='*.go'` → `store.go:1516` y la definición en `ledger_identity.go:612-615`. No lo llaman la fundación, la adopción, `Standing`, `ProfileIdentity` ni la CLI.
- Se ejecuta solo con forma actual o **más nueva** y con identidad: `if shape.shape != shapeOlder && identity != ""` (`store.go:1515`). Con forma anterior no se llama.
- Tests aprobados: ninguno llega a su rama nueva (predicción). Lo he comprobado así:
  - `grep ledger_identity` en tests: solo `DELETE`, `DROP TABLE`, `UPDATE` canónicos o rechazados. El `SET owner_digest = NULL` lo rechaza NOT NULL en `ledger_identity_test.go:256`.
  - `grep 'RENAME COLUMN|DROP COLUMN|DROP INDEX'`: nada del juicio.
  - `grep 'version = 17|99'`: los fixtures más nuevos conservan `ledger_identity` (`ledger_shape_test.go:299-314`, `ledger_hook_test.go:451-467`) o van por `openFull` sin identidad (`migration_test.go:248-267`).
- Lo que sí rompe es G-E5: N10-1.

## (e) Tests aprobados

Fuera de D21 y del comentario de D07 no encuentro ningún assert que cambie (predicción):
- D07, D16 y D20 inyectan `errors.New` sin código.
- D15 usa el pool cerrado, que no trae código. Solo sigue verde si `judgeIn` devuelve `LedgerStandingUnreadable` junto a un error que no es veredicto (`profile_standing.go:171-172`; assert en `ledger_shape_test.go:326`). El plan no fija ese valor.
- `TestWriteTx_busyIsNamed` usa `BeginTx` → `mapGuardError`.
- `TestConfigActRecorder_anUnreadableStandingIsNamed` usa un doble con `errors.New("database is locked")` (`config_act_registry_test.go:919-921`). Sigue en `unreadable` solo si el grabador decide por centinela o código, nunca por texto.
- Los de fichero corrupto solo exigen `err != nil`.
- La siembra fallida no es residuo: 34 tablas y un trigger sobre `action_schema`.
- `grep` de `err.Error() ==` o `HasPrefix(err.Error()` en sqlite, cli y app: ninguno.
- La lista «No cambian» del §9 casa entrada por entrada. `TestGuard_refreshGuardReturns…` es D16, contado dos veces.

Excepciones:
- El **nombre** de D07 y dos textos suyos se vuelven falsos (N10-8).
- D02 cambia si las 34 tablas pasan a derivarse «en memoria» (N10-7).

## Marcas [veredicto]

`grep -o '\[veredicto\]' | wc -l` = 12: la definición (l.19), una referencia (l.302) y diez citas. Todas casan en el WT:

| Línea | Cita | Resultado |
|---|---|---|
| l.61 | `profile_standing.go:190-211` | Parcial: la función va de 200 a 217 y el rango deja fuera 212-216. |
| l.88 | `:312-315`, `:284-291`, `:321`, `:105-108` | Casa, pero los triggers están en 316-320 y la ruta omite `Standing` (`:91-97`). |
| l.89 | `store.go:1467-1545`, `:1513-1531` | Casa; la función acaba en 1547. |
| l.90 | `store.go:1430-1456` | Parcial: la llamada a `judgeShape` está en 1457. |
| l.92 | `:450-496`, `:464-468`, `:486-489` | Casa, aunque «lo usa D07» es incompleto: también D16 y D20. |
| l.93 | `:167-211`, `:202-211` | Casa. |
| l.96 | `conn.go:151` (`applyQueryParams`) | Casa. |
| l.157 | `ledger_shape_test.go:95-123` | Casa. |
| l.185 | pretest `:84`, `:646` | Casa. |
| l.186 | HANDOFF `:377-383`, `:385-387`, `:396-397` | Casa; el título está en `:319`. |

## Hallazgos nuevos

**N10-1 · P2 · [AFIRMACIÓN-FALSA][PLAN-FILA-AUSENTE][TAXONOMÍA] · §4 fila `readOwner` (l.105), G-E5 segunda viñeta (l.74), (i)-(vi), §3 (l.89).**

El cambio de `readOwner` hace que un libro más nuevo cuyo dueño no se puede leer abra bloqueado como `ledger_unreadable`, en vez de dar `ErrSchemaFromTheFuture`.

Evidencia:
- `readOwner` corre con forma más nueva (`store.go:1515-1516`). Con veredicto, la v11 «sigue el camino de la forma mala: sin migrar, Ping y un Store bloqueado». Así se salta `migrate`, que es lo que da `ErrSchemaFromTheFuture` al dueño (`store.go:719-722`), y `requireCurrentSchema`, que se lo da al ajeno (`ledger_identity.go:636-637`).
- El gancho admite una forma más nueva como `unreadable` (`:549-550`), y `judgeIn` la da como veredicto «schema vN, newer» (`profile_standing.go:174-175`).
- El mismo fichero da `ErrSchemaFromTheFuture` por `OpenOperatorFor` (`store.go:1439-1440`) y por `OpenReadOnlyFor` (`:2110-2112`).
- En el plan, `grep ErrSchemaFromTheFuture` solo sale en l.74. Ninguna fila prueba una forma más nueva.

Reproducción (predicción por lectura):
1. `path := buildV1File(t)` (`migration_test.go:58`), y en conexión cruda `UPDATE action_schema SET version = 99`, como en `:255`.
2. `OpenFor(path, profileA)`.
3. Hoy: `readOwner` falla con «no such table: ledger_identity» (código 1) y `OpenFor` devuelve ese texto (`store.go:1517-1519`). Ya hoy no sale `ErrSchemaFromTheFuture`.
4. Con la v11: `readOwner` devuelve el veredicto → camino de la forma mala → `Ping` → Store. Después `refreshGuard` → veredicto «schema v99, newer than this binary's v16» → guarda `unreadable`. `Standing` da veredicto y no hay poda. `OpenFor` **devuelve un handle**.
5. `Build`: `unreadable` (`app.go:410`) → la app arranca y la pantalla pinta D2 (l.351): «sustituye {ruta} por una copia… aparta esos ficheros y Korvun empezará un libro nuevo».
6. `korvun ledger check` sobre el mismo fichero dice «schema version from the future».

Caso real: bajar de versión sobre un v17 de un Korvun posterior que cambie `ledger_identity`.

Por qué P2: la cura de N9-9 vuelve falsa una garantía literal del tren. Además, un libro solo más nuevo se presenta como ilegible, con un remedio que lleva a abandonarlo, y tres puertas nombran distinto el mismo fichero. No es P1: el código no destruye nada y la escritura falla cerrada.

**N10-2 · P2 · [SEAM-INALCANZABLE][MUTACIÓN-SOBREVIVE][AFIRMACIÓN-FALSA] · preámbulo §6 (l.138-142), E12-H (l.163), E12-W (l.165), G-E3 (l.66), §4 l.111, §7-bis (d) (l.231).**

Ningún seam llega a la decisión nueva del juez, que es la rama peligrosa de G-E3.

Evidencia:
- G-E2 (l.65) pone la decisión nueva *dentro* del juez.
- `hookShapeFault` devuelve antes de `judgeOnConn` (`ledger_identity.go:464-469`).
- `refreshGuard` y `Standing` llaman a `judgeIn(ctx, s.db, me)` con el mismo querier (`:284`; `profile_standing.go:155`). Dentro de `judgeIn` no se distinguen, así que «solo en la llamada de X» obliga a poner el seam en la llamada, antes de `judgeIn` o en su lugar.
- `beginWrite` devuelve el error de `judgeIn` sin clasificar (`:176-183`), y sus llamadores solo envuelven con `%w` (`store.go:1839-1842`). (vi) clasifica solo las cuatro funciones públicas.
- §4 l.111 dice que el código de `refreshGuard` y de `beginWrite` «no cambia», pero los seams van precisamente en ellas.

Reproducción (predicción):
1. Mutación MJ-h, en `judgeOnConn`: un fallo de consulta con código 5 o 10 → `return unreadable, nil`.
2. E12-H: el seam dispara en `:464-468` y el gancho rehúsa. El `judgeShape` del pool recibe código 5 → error de clase busy → `OpenFor` falla con `ErrLedgerBusy`. Queda **verde**. Ninguna otra fila hace fallar con 5 o 10 una consulta de `judgeOnConn`.
3. Mutación MJ-p, en `judgeIn`: 5 o 10 → veredicto. E12-R, E12-W y E13 inyectan en la llamada y no ejecutan `judgeIn`: **verdes**.
4. E12-W sin mutar: la escritura devuelve el `codedFault` crudo y `errors.Is(err, ErrLedgerBusy)` es falso. «Esa escritura falla con la clase» no puede pasar a verde sin clasificar en el camino de escritura, cosa que §4 excluye.
5. Efecto en producción de MJ-h: un BUSY o IOERR transitorio mientras el gancho juzga deja la conexión `unreadable` para toda la vida del handle (`store.go:1485-1488`). Es justo el efecto que prohíbe G-E3.

Por qué P2: es la mitad sin cerrar de N9-2, puntos 3 y 4 de la doctrina.

**N10-3 · P3 · [MUTACIÓN-SOBREVIVE][TAXONOMÍA][AFIRMACIÓN-FALSA] · G-E2 (l.65), §5 (l.119-131), §7-bis (g) (l.234).**

- Ninguna fila fuerza 11, 20 ni 24 dentro del juez. El 26 solo aparece en E14, fuera del juez.
  - Mutación «quitar 11 de la forma del juez»: nada se pone rojo.
  - En producción, CORRUPT en `receipts` de un libro legado: `judgeWithoutRow` da error, `OpenFor` se cierra (`:289-291`, `:82-85`) y el arranque muere.
- La regla del `Scan` no tiene cable definido por código.
  - Los errores de conversión de `database/sql` no llevan tipo: `convert.go:440` («converting NULL to %s is unsupported»), envuelto en `sql.go:3401`.
  - `QueryRowContext(...).Scan` (`profile_standing.go:178`; `ledger_identity.go:616`) devuelve por la misma llamada el error de la consulta, el de la conversión, `context.Canceled`, `ErrTxDone` y «database is closed».
  - «La decisión va por código» es falso para esta regla.
- Mutación «todo error sin código en un `Scan` del juez es forma»: sobrevive. D15 falla antes, en el `QueryContext` de `judgeShape` (`ledger_shape.go:106-108`).

**N10-4 · P3 · [PLAN-FILA-AUSENTE][AFIRMACIÓN-FALSA] · (v) (l.49), (vi) (l.50), «cada efecto tiene su molde» (l.36), G-E4 (l.67-71).**

- Ninguna fila saca un error con código de `OpenOperatorFor`:
  - E02 da `ErrNoActionStore`, un centinela.
  - En E10-P y E11-CLI la apertura va bien y el rechazo llega después, por `EnsureSigningKey`.
  - Mutación «`OpenOperatorFor` no clasifica»: sobrevive. Camino alcanzable: `probeShape` con `PRAGMA query_only` (`store.go:1454-1456`) sobre un fichero de texto.
- `printLedgerStanding` también sirve a `receipt verify` (`cli/receipt.go:114`, `:123`), y R22 lo ejecuta (`cli/ledger_identity_test.go:175-181`). Es un efecto fuera de una lista que el plan declara cerrada.

**N10-5 · P3 · [ORÁCULO][EITHER/OR] · E11 (l.159).**

La fila deja abierto el desenlace de B («lo que diga el recorrido… predicción»), y se puede derivar leyendo: `ListReceipts` selecciona `result_digest` (`ledger.go:507-509`).

Reproducción (predicción):
1. Montar E11-B.
2. `korvun ledger check`.
3. Resultado: primero la línea con `ledger_unreadable`, luego «korvun ledger check: … no such column: result_digest» y salida 1 (`cli/ledger.go:96-100`).

Esto contradice el «en los cuatro … salida 0» de la propia fila, y su mutación no aplica a B.

**N10-6 · P3 · [PLAN-FILA-AUSENTE] · «Qué no se promete» (l.54), §12 (l.345-346).**

(a) Una tabla vigilada sustituida por una tabla virtual.
- FTS5 y RTREE vienen compilados: `lib/sqlite_darwin_amd64.go`, «-DSQLITE_ENABLE_FTS5 … -DSQLITE_ENABLE_RTREE».
- `judgeShape` sigue viendo `table:` (`ledger_shape.go:136-140`), así que la forma es actual.
- Sobre una tabla virtual no se pueden crear triggers (semántica de SQLite, no verificada aquí). `:490-494` falla con código 1 y el arranque muere con la clase `ledger_unreadable`, mientras `ledger check`, que no tiene gancho, dice `ok`.

(b) La versión guardada como BLOB con un espacio.
- `judgeShape` la acepta con `Atoi(TrimSpace)` (`ledger_shape.go:165`). `migrate` (`store.go:714-716`) y `requireCurrentSchema` (`ledger_identity.go:633`) la leen con `ParseInt` sin recortar (`convert.go:442-446`).
- Reproducción:
  1. `foundedFor`.
  2. `UPDATE action_schema SET version = CAST('16 ' AS BLOB)`.
  3. `OpenFor` → predicción: «read schema version for migration: sql: Scan error … converting driver.Value type []uint8 ("16 ") to a int», **sin clase**.
- Es la clase de P2-2 fuera del juez, sin declarar.

**N10-7 · P3 · [MUTACIÓN-SOBREVIVE][TEST-APROBADO-NO-DECLARADO] · G-E7 (l.79-81), E10 (l.156), E10-L (l.157).**

- G-E7 comprueba el nombre, la tabla y la marca UNIQUE, pero dos de los tres índices son parciales (`store.go:440-442`, `:479-482`).
- Reproducción (predicción):
  1. `DROP INDEX execution_bindings_active_selector;`
  2. `CREATE UNIQUE INDEX execution_bindings_active_selector ON execution_bindings(actor_principal_id, channel, ifnull(conversation_id,'')) WHERE 0;`
  3. Cumple G-E7 a la letra → forma actual → dos bindings ACTIVE con el mismo selector entran en silencio. Pasa lo mismo añadiendo una columna a `budget_debits_sequence` (`:610`).
- «Derivados en memoria» choca con `schemaTablesV16` (`ledger_identity.go:674`), que usan D02 (`ledger_shape_test.go:118`) y D21 (a través de `SchemaTablesForTest`). O D02 cambia sin estar declarado, o la frase es falsa.

**N10-8 · P3 · [E34-INCOMPLETA][TEST-APROBADO-NO-DECLARADO][FUERA-QUE-BLOQUEA G-E10] · E34, §9 (l.256), G-E10 (l.84), §1 «Fuera» (l.34).**

Frases que la v11 vuelve falsas y que E34 no recoge:
- De D07: el nombre `TestShape_theHookRefusesTheConnectionOnAQueryError` (`:191`), su «PROBING MUTATION: the hook maps the failure to ledger_unreadable» (`:190`), que ahora es el diseño para los códigos de forma, y el mensaje «the hook's query failure was judged a verdict» (`:204`). Corregirlos va más allá de «solo el comentario».
- El godoc de `hookShapeFault` (`:667-668`).
- Por N10-1: el godoc de `ErrSchemaFromTheFuture` (`store.go:83-84`) y la frase del HANDOFF `:388`, «actual y más nuevo como manda el dueño».
- El texto del centinela, «the ledger's identity row cannot be read» (`:39`). El §5 lo pega a NOTADB (E14) y a los fallos de migración (E08, E48) y deja el arreglo para el tren H. Eso choca con G-E10.

**N10-9 · P3 · [AFIRMACIÓN-FALSA] · §6 l.138, §3 l.88, G-E2 l.65.**

- «El molde sabe qué llamador consumió el fallo» no es una propiedad del sitio donde está el seam:
  - D16 (`:342-354`) y D20 (`:483-497`) ya consumen `hookShapeFault` a través de `refreshGuard` y del `Standing` de `OpenFor`.
  - `standingJudgeFault` lo consume `OpenFor:91` si se arma antes de abrir, y la ruta del §3 omite ese `Standing`.
- G-E2 no lista `beginAdoption` (`:212-216`, que deja la guarda pegajosa) ni `refuseMaintenance` (`:332`; `store.go:1576`), y los dos reciben el veredicto nuevo.

## Lo que la v11 hace bien, verificado

- **Gate:** `/tmp/q26.txt` termina en «Quality gate passed.» y «EXIT=0». `find $WT -newer /tmp/q26.txt` solo devuelve `.claude/adversary/` y 9 veredictos.
- **Orden pre-RED:** `grep -rlnE 'func classify\(|ErrLedgerEnvironment|codedFault|seedSeam|…JudgeFault|…Migration…'` → `exit=1`.
- **Aritmética:**
  - 34 tablas.
  - 11 índices: `grep -hoE 'CREATE (UNIQUE )?INDEX …' | sort -u | wc -l` → 11.
  - 3 UNIQUE (`store.go:440`, `:479`, `:610`); 5 sentencias; 12 combinaciones.
  - 50 asserts y 45 tests distintos en la lista (`grep -c` → 50; `sort -u | wc -l` → 45).
  - P2 por ronda: 14, 11, 6, 2, 3, 4, 4, 3 y 2 (recuento por fichero de veredicto).
- **Residuo:** `createStmt` es idéntico en todas las etiquetas desde la v0.11.0 hasta la v0.16.1 y en el WT (`cmp` → «same» en las nueve), así que el residuo de cualquier release casa.
- **Cura de N9-1:** rompe la reproducción literal. M1 pone E12-R en rojo y la máscara con 522 pone E13 en rojo (predicción).
- **Coherencia:** el §8 casa con la columna «Nivel» de las 32 filas. Las leyes sexta y séptima quedan declaradas: UX con «LISTO PARA RED» y perfil real en el tren G.

## Alcance

**Leído:**
- El plan v11 entero, el veredicto de la ronda 9 entero y el extracto §14.3/§14.6.
- De la lista: la cabecera, el resumen y la sección de `openWithIdentity`.
- En el WT, enteros: `ledger_identity.go`, `ledger_shape.go`, `profile_standing.go` y `ledger_shape_test.go`.
- En el WT, por tramos:
  - `store.go`: 80-135, 700-760, 1236-1300, 1396-1560, 1755-1835 y 2030-2121.
  - `cli/ledger.go` (60-166), `intent.go` (85-140) y `receipt.go` (455-475).
  - `app.go` (365-520), `config_act.go`, `config_act_registry.go` (380-420) y `approvals_adapter.go` (596-654).
  - `whats_happening.go` y `act.go`.
  - Tests: `ledger_hook_test.go`, `ledger_identity_test.go`, `cli/ledger_identity_test.go`, `ledger_binary_test.go`, `cli/ledger_test.go`, `repair_procedure_r13_test.go`, `repair_r11_test.go`, `errors_test.go`, `migration_test.go`, `identity_phase1_test.go`, `authority_as08_test.go`, `config_act_registry_test.go` y `app/ledger_shape_boot_test.go`.
  - Documentos: HANDOFF (370-400), nota v0.16.2 (125-165), `operator-cli.md` (125-165), pretest `:84` y `:646`, `red.txt` (380-392) y el CLAUDE.md del WT (384-416), con la RULE 2 idéntica en las dos copias según `diff`.
- Del driver: `error.go`, `driver.go` (225-300), `conn.go` (95-160) y los flags de compilación.
- De GOROOT: `database/sql/convert.go` (438-450) y `sql.go:3401`.

**Ejecutado:** solo lectura: `date`, `wc`, `shasum`, `stat`, `git diff/status/show/tag/rev-parse`, `grep`, `sed`, `awk`, `find`, `ls`, `cmp`, `go env` y `go version`. Ficheros auxiliares, en mi scratchpad y fuera del árbol: `r10-create-v0161.txt`, `r10-create-wt.txt`, `r10-cs.txt` y `r10-owi-section.txt`. Ni tests, ni compilaciones, ni mutaciones.

**No verificado (predicción):**
- Todos los desenlaces dinámicos.
- La semántica de SQLite en cuatro puntos: el tipo de una tabla virtual y el rechazo de triggers sobre ella, BLOB con afinidad INTEGER, `BEGIN EXCLUSIVE` en WAL y NOTADB con 1 byte.
- Cómo se implementará la regla del `Scan`.

**Sin examinar:** la instantánea, la ficha UX, las decisiones y la maqueta (por la orden); el TSX; el resto de la lista; los trenes G, F y H.

**Observación sin adjudicar** (queda fuera de la letra de G-E2, porque no es un fallo de consulta): si `ledger_identity` se reconstruye sin su CHECK y con una fila de id ≠ 1, `judgeOnConn` (que cuenta con `COUNT`) y `judgeIn` (que busca `id = 1`) deciden distinto. Viene del tren D.

**Tiempo:** de 07:44 a 08:19, unos 35 minutos. Me pasé cinco del presupuesto.

**Lo que esta ronda vio y la anterior no:**
- `readOwner` con forma más nueva.
- La decisión del juez para BUSY y entorno, a la que ningún seam llega.
- 11, 20 y 24 y la regla del `Scan` sin molde ni cable.
- La pata `OpenOperatorFor` de (vi) y `receipt verify`.
- El desenlace de E11-B.
- Las consultas fuera del juez sin declarar.
- El UNIQUE más débil.
- El nombre de D07.

## Integridad (08:17:39)

- `git -C $WT diff | cmp - …/adv-e10-before.patch` → «DIFF: identical». Al empezar (07:44:50) dio lo mismo.
- `git -C $WT status --short | grep '^??' | sort | cmp - …/adv-e10-untracked-before.txt` → «UNTRACKED: identical». Al empezar, también.
- `git -C $WT diff --cached --quiet` → `cached_exit=0`.
- El plan sigue con 359 líneas, sha256 `31338ea2c9742495c5f3ce3e4b39b4a02c8668c434717c1e068d9cb0b4f2d1ad` y mtime 07:36:46. El extracto y la lista siguen con `9c277079…` y `c207620c…`.