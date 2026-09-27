VETO MANTENIDO

**Objeto (v8, reducido)**
- Al empezar, 2026-09-26 05:31:54: `wc -l` = 274, `shasum -a 256` = `3cd8740dda8458cad8a02073c81d9558c2918621b51303f5117a0677890e1fbc`, mtime 05:29:40. Coincide con lo medido por el ejecutor.
- Al terminar, 2026-09-26 06:01:16: 274 líneas y el mismo sha256.
- Lista de contraste: 677 líneas, sha256 `c207620c…2412247`, sin cambios.
- Árbol: WT = `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `d20dea6`. Las citas `internal/…` y `cmd/…` son de `$WT`. El driver es `$(go env GOMODCACHE)/modernc.org/sqlite@v1.59.0`.
- Nota: el encargo habla de «26 filas», pero §6 tiene 25 (`awk 'NR>=112&&NR<=138' | grep -c '^| E'` → 25). El plan no afirma ninguna cifra, así que no es hallazgo.
- Método: la orden prohíbe tests, compilación y mutaciones, y no he ejecutado ninguno. Todo desenlace dinámico va marcado «predicción por lectura».

## (a) §10 frente a la ronda 6

| Hallazgo | Destino en la v8 | Veredicto | Evidencia |
|---|---|---|---|
| N-1 | tren G (l.220, l.246) | EN SU SITIO | Ya no hay G-E13: `grep 'G-E1[0-9]'` solo da l.222 («G-E12 anterior»). §1 «Fuera» lo nombra. |
| N-2 | tren G (l.245) | EN SU SITIO | Ídem. No queda ninguna promesa «nadie más lo tiene abierto». |
| N-3 | tren H (l.256) | EN SU SITIO | §1 l.31 retira los clasificadores de autoridad. |
| N-4 | aquí (§4 l.87, G-E3 l.43, E13-U) | EN SU SITIO | `config_act_registry_test.go:917-943` usa `errors.New("database is locked")`: sin código, cae en «lo demás» y da `unreadable`. El test sigue verde (predicción). |
| N-5 | aquí (§8) | MAL LLEVADO (a medias) | La categoría existe, pero E04 está mal archivado (N7-9). |
| N-6 | tren G | EN SU SITIO | |
| N-7 | aquí (§4 l.94, E02-R3) | EN SU SITIO | {`action_schema`} + trigger con `tbl_name`=`sessions`: el trigger no es entrada, así que queda k=1, residuo y fresco. |
| N-8 | aquí (G-E2 l.37) | EN SU SITIO | «Hasta reiniciar» desapareció y E27 salió. La reproducción de N-8 (txExec) queda «como hoy», sin promesa. La no-pegajosidad nueva es otro hallazgo (N7-4). |
| N-9 | tren G (l.248) | EN SU SITIO | |
| N-10 | tren H (l.257) | EN SU SITIO | |
| N-11 | aquí (G-E4 l.50-51) | EN SU SITIO | Cuadra con E06-A (l.119). |
| N-12 | E27 fuera; §12 l.261 | EN SU SITIO | |
| R5-2 | sin fila (`grep -c` = 0) | EN SU SITIO por sucesor | Su resto es N-2, que va al tren G. |
| R5-3 | sin fila (0) | EN SU SITIO por sucesor | Su resto es N-4, aquí. |
| R5-11 | sin fila (0) | MAL LLEVADO | Su resto es N-5, curado a medias. |
| R4-12 | sin fila (0) | EN SU SITIO por sucesor | Sus restos son N-1, N-6 y N-2, todos al tren G. |

Resultado: de N-1 a N-12, 11 EN SU SITIO y 1 MAL LLEVADO (N-5). De los cuatro anteriores, 3 están en su sitio a través de su sucesor, aunque §10 no los nombra, y 1 está MAL LLEVADO (R5-11). Ninguno queda SIN SITIO. Además se reabre R4-4 (cerrado en la ronda 5): véase (e).

## (b) El residuo de §4

- **Acepta k=1…5.** `createStmt` tiene cinco sentencias (`store.go:101-129`). Su texto es idéntico en v0.15.1, v0.16.0, v0.16.1 y el WT; el `awk` del bloque da sha `4790eb96…` en los cuatro. La semilla de v0.16.1 tiene la misma forma: dos `Exec`, en `git show v0.16.1:…store.go` l.1439 y l.1444.
- **Lo implícito.** `actions` y `action_decisions` son WITHOUT ROWID, y SQLite no crea entrada en `sqlite_schema` para su PRIMARY KEY. Lo dice el comentario del propio driver: «(3) Bypass the creation of the sqlite_schema table entry for the PRIMARY KEY» (`lib/sqlite_g_0000000000000044.go:3790-3792`). `action_schema` no tiene restricciones. Cada prefijo da, por tanto, exactamente k entradas.
- **Las conversaciones.** Sus objetos tienen `name`/`tbl_name` `sessions`/`turns`/`notes`, que no son entradas del almacén.
- **Las siete variantes de E02-A se rechazan.** (1) una fila en `actions` falla la condición 2. (2) La vista se compara por tipo. (3) El otro DDL se compara por SQL. (4), (5), (6) y (7) no forman el conjunto de ningún prefijo.
- **No contradice E09.** Un trigger `actions` ON `sessions` no es entrada.
- **Hueco de definición, sin severidad propia.** «Nombre del almacén» no está definido: ¿solo las 34 tablas (`schemaTablesV16`) o también los índices? E10-R lo fija solo para `execution_bindings_active_selector` (véase N7-8).

## (c) G-E2, prefijo más `%w`, frente a los tests aprobados

- **Ningún test cae por el prefijo más `%w` más allá de §9.**
  - `grep 'HasPrefix('` en `*_test.go`: 74 usos, ninguno sobre errores de las cinco funciones.
  - `grep '.Error() ==/!='`: ninguno sobre ellas.
  - Los asserts negativos de veredicto están en `ledger_shape_test.go:203`, `:326`, `:355` y `:498`. Todos inyectan errores sin código (`:196`, `:348`, `:482`, o `sql: database is closed`), que caen en «Resto» y no cambian.
  - Los `TestMigrationV1x_*`, `TestManualRepairProcedure_*` y `repair_procedure_r13_test.go:207,213` usan `Contains` o `errors.As`.
- **§9 declara de más** (N7-7). `TestOpen_seedFailureIsBootFatal` no se rompe, y el fixture de `WhatsHappening.test.tsx:838-847` no tiene por qué cambiar.
- **Errores de la lista.**
  - Los «50» de `openWithIdentity` son 50 asserts en 45 tests distintos (`sed -n 38,87p | … | sort | uniq | wc -l` → 45).
  - `TestTombstoneFault_classFollowsTheDoor` (`tombstone_r12_test.go:642-686`) no pasa por `migrateStep`. Lo mete la búsqueda por frase; es inofensivo.
- **Cambios de control no declarados en §4 en dos de las cinco funciones** (N7-11):
  - E11 pide «arranque bloqueado», así que `openWithIdentity` debe tratar un veredicto de Forma de `judgeShape` como `shapeBad`. Hoy devuelve el error (`store.go:1494-1498`).
  - E11 pide también «línea `ledger_unreadable`», así que `openReadOnly` debe abrir ante ese veredicto. Hoy devuelve el error (`store.go:2098-2101`).
  - No encontré ningún test aprobado que se rompa por ello: ningún test fuerza un error con código de Forma en el catálogo esperando que la apertura falle con identidad.

## (d) Puertas y verbos de escritura

§1 dice que no cambia nada. Es falso; véase N7-3.
- Las puertas de aprobación pasan de `unavailable` a `ledger_unreadable`, y la guarda queda pegajosa.
- Los verbos de escritura de la CLI, sobre un residuo, pasan de rehusar como ilegible a `ErrNoActionStore`, por diseño de G-E1.
- La adopción y la fundación pasan por `judgeIn`.
- Las seis puertas de la pantalla y `POST /api/config` siguen dando `act_not_recorded`, pero el detalle cambia de texto y la guarda también queda pegajosa.

## (e) Seams sin exportar

- **En el mismo paquete, alcanzables.** `seedSeam` (E01), el seam de pragma (E05), `afterMigrateReadVersion` y `beforeMigrationCommit` (E06), `afterMigrationCommit` (E07), `standingQueryFault` (E13-R) y la DSN de test (E06, E12).
- **E47-R no se alcanza** (N7-10). Su ataque usa «el seam de E13», que es una variable sin exportar del paquete `sqlite`, y su molde vive en `internal/cli`. La alternativa del doble no existe: `printLedgerStanding(ctx, out, store *actionsqlite.Store)` (`cli/ledger.go:149`) recibe un tipo concreto. Así se reabre R4-4, que la ronda 5 cerró gracias a un seam exportado que la v8 retira.
- **E23, «la app retenida en `seedSeam`», no se alcanza.** La app no puede poner una variable sin exportar de otro paquete, y un test interno de `sqlite` no puede importar `internal/app` porque sería un ciclo de importación. Solo `OpenFor` retenido en un proceso hijo es alcanzable.

## Marcas [veredicto]

`grep -n '\[veredicto\]'` da 10 marcas en 10 líneas. Las de l.19 y l.229 son de definición.
- l.66: `store.go:101-129`, respaldada por R4 l.42 y R5 l.39. Casa.
- l.68: `ledger_shape.go:125`, respaldada por la lista (l.18, l.187). Casa.
- l.71: `ledger_identity.go:175-181`, respaldada por R6 N-8. Casa.
- l.75: `config_act.go:185-192`, respaldada por R6 N-4. Casa.
- l.197: la lista. Existe y su sha coincide.
- l.200: `errors_test.go:269-286`, respaldada por la lista (l.44). Casa.
- l.133: «hoy» 503 `act_not_recorded` con `ledger_unreadable` en D21. **Ningún veredicto lo dice de esa puerta.** R3 lo afirma para adopt-ledger (`profile_standing_test.go:441-442`). Por mi lectura casa: `whats_happening.go:481-485` más `config_act.go:127-135`.
- l.202: `WhatsHappening.test.tsx:838-847`. **Ningún veredicto ni la lista cita ese rango** (R1 cita `:25,842`). En el WT casa: en `:838` está el `it(...)` con el fixture `'database is locked'`.

## Hallazgos nuevos

**N7-1 · P2 · [AFIRMACIÓN-FALSA][TAXONOMÍA] · G-E3 (CLI: «Si falla la apertura, sale con 1 y el nombre de la clase»), E14-CLI, E47-A, frente a la lista cerrada de sitios de G-E2 (l.36).**

Evidencia:
- `ledger check` abre por `openOperatorStore` → `OpenReadOnlyFor` (`cli/intent.go:90-99`) → `openReadOnly`.
- La conexión nace en `db.Exec("PRAGMA query_only = 1")` (`store.go:2091-2093`, error «seal read-only connection»), antes de `judgeShape` (`:2098`).
- En el driver, `newConn` → `applyQueryParams` (`conn.go:151`) aplica los `_pragma` con `busy_timeout` primero y luego `journal_mode(WAL)` (`sqlite.go:436-455`). Los hooks corren después (`driver.go:266`), y la DSN del lector no lleva nonce.
- NOTADB (fichero de texto) y BUSY (EXCLUSIVE de otro proceso) salen en el sellado, que no es sitio de G-E2, y se imprimen tal cual (`cli/ledger.go:85-88`).
- La ronda 5 ya lo vio (R4-4: «la primera conexión en PRAGMA query_only … da BUSY»). La v8 cierra la lista de sitios sin incluirlo.

Reproducción (predicción por lectura):
1. Poner `storage.path` en un fichero de texto (E14).
2. `korvun ledger check --config p`.
3. Sale `korvun ledger check: action/sqlite: seal read-only connection "…": … not a database …`, código 1, sin clase. E14-CLI y E47-A quedan rojos para siempre, o exigen un sitio no declarado.

Por qué P2: es una garantía literal más ancha que su cable, y los dos moldes de esa superficie son inalcanzables. Es una regresión de un cierre anterior.

**N7-2 · P2 · [PLAN-FILA-AUSENTE][TAXONOMÍA] · G-E2 (sitios), G-E3 (arranque), E11-A, P2-2 del tren D.**

Evidencia:
- `openWithIdentity`, en la rama `shapeCurrent` con identidad, llama a `readOwner(db)` (`store.go:1516`).
- `readOwner` (`ledger_identity.go:615-623`) devuelve «action/sqlite: read the identity row: %w» y no está en la lista de G-E2.

Reproducción (predicción por lectura):
1. Libro v16 fundado.
2. `ALTER TABLE ledger_identity RENAME COLUMN owner_digest TO od;`
3. `Build`. El gancho, con la v8, admite la conexión como `unreadable`. `judgeShape` da `shapeCurrent`. `readOwner` falla con «no such column: owner_digest» y el arranque muere: `app: open action store: action/sqlite: read the identity row: …`, sin clase.

Es la mitad «columna de identidad» de P2-2, que E11-A dice cubrir. Por qué P2: el arranque muere con texto del driver, que es el síntoma literal del P2 que el tren cura.

**N7-3 · P2 · [AFIRMACIÓN-FALSA][PLAN-FILA-AUSENTE] · §1 l.31 («No cambia ninguna puerta … Tampoco aprobaciones, los verbos de escritura de la CLI»), §9 l.208 («Adopción y fundación … Este tren no los toca»).**

Evidencia:
- `beginWrite` → `judgeIn` (`ledger_identity.go:175-181`), que es sitio de G-E2. Un veredicto hace `setGuardIn(unreadable)`, y es pegajoso (`:268`).
- `decideApprovalWithLaw` envuelve el error: `"%w: %w", ErrApprovalUnreadable, err` (`approvals.go:252-254`).
- `nameTouch` comprueba `ErrLedgerUnreadable` (`approvals_adapter.go:618-619`) antes que `ErrApprovalUnreadable` (`:640-644`).
- Hoy responde 503 `unavailable` («this is transient», `controlapi/approvals.go:201-202`). Con la v8, 503 `ledger_unreadable` («only restoring the ledger lifts it», `:244-245`).
- `AdoptLedger` → `beginAdoption` → `judgeIn` (`ledger_identity.go:203-216`), y `FinishFounding` → `beginWrite` (`profile_standing.go:254-270`). Los dos pasan por un sitio de G-E2.
- En la CLI, G-E1 cambia la respuesta de la sonda ante un residuo: hoy cae a `openWithIdentity` (`store.go:1430-1443`), abre ilegible y rehúsa; con la v8 da `ErrNoActionStore` (`:1434-1436`).

Reproducción (predicción por lectura):
1. App en marcha con una aprobación pendiente. El handle conserva su conexión.
2. Desde otra conexión: `ALTER TABLE action_schema RENAME COLUMN version TO v;`
3. `POST approve`: con la v8 sale 503 `ledger_unreadable`; hoy sale 503 `unavailable`.
4. `ALTER TABLE action_schema RENAME COLUMN v TO version;`
5. `POST approve` otra vez: con la v8 vuelve a rehusar, porque la guarda es pegajosa hasta reiniciar; hoy registra.

Ninguna fila, molde ni mutación cubre este cambio. Por qué P2: la afirmación de alcance es literal y falsa, y deja un cambio de outcome de puerta sin molde.

**N7-4 · P2 · [MUTACIÓN-SOBREVIVE][AFIRMACIÓN-FALSA] · G-E2 («Del momento y entorno … nunca son pegajosos»), §5 columna «Pegajoso», E12 (PIN), E13.**

Evidencia:
- Ninguna fila observa la guarda después de un error BUSY o de entorno en un sitio que la escribe (`beginWrite`, `beginAdoption`, `refreshGuard`, el gancho).
- E12 y E13 van por `Standing`, que nunca escribe la guarda (`profile_standing.go:150-156`). E05 y E06-A no dejan handle.
- El PIN de E12, «D07 y D16 existentes (BUSY en el gancho y en refreshGuard …)», es falso: D07 y D16 inyectan `errors.New("injected: the catalog could not be read")` (`ledger_shape_test.go:196`, `:348`). Sin código, eso es «Resto», nunca BUSY.

Reproducción (mutación; predicción por lectura):
1. Añadir `|| errors.Is(err, ErrLedgerBusy) || errors.Is(err, ErrLedgerEnvironment)` a `isVerdict` (`ledger_identity.go:247-249`).
2. E05, E06, E12, E13, E47, D07 y D16 siguen verdes.
3. En producción, un IOERR en `judgeIn` dentro de una escritura deja la guarda `unreadable` hasta reiniciar.

Por qué P2: la garantía literal no tiene ningún test que se ponga rojo, y el PIN declarado no fija lo que dice (punto 4 de la doctrina).

**N7-5 · P3 · [ORÁCULO][AFIRMACIÓN-FALSA] · G-E3 l.45, G-E4 l.50, E08, E14, E47-A, E48; E08-A, E11.**

Los literales «`app: open action store: ledger_unreadable: …`», «`korvun ledger check: ledger_busy: …`» y «`korvun ledger check: ledger_unreadable: …`» no pueden aparecer:
- Los textos de los centinelas empiezan por `action/sqlite: ledger_unreadable: the ledger's identity row cannot be read` (`ledger_identity.go:39`) y `action/sqlite: ledger_busy: another writer held…` (`:45`).
- `openWithIdentity` antepone `action/sqlite: migrate %q:` (`store.go:1511`, `:1526`).

Además, ese centinela narra una causa falsa, «identity row cannot be read», para una lápida corrupta (E48), una tabla `receipts` ausente (E08) o una columna `version` renombrada (E11).

Hay citas que tampoco son el texto del árbol:
- «missing table intents» frente a `table %s missing at schema v%d` (`ledger_shape.go:182`).
- «no column version» frente a «no such column: version» del driver (`lib/sqlite_g_0000000000000003.go`).

Reproducción:
1. Leer E08 y E48 contra `ledger_identity.go:39` y `store.go:1511`.
2. Un `strings.Contains(out, "app: open action store: ledger_unreadable:")` da rojo (predicción).

**N7-6 · P3 · [E34-INCOMPLETA] · G-E9, E34.**

G-E2 vuelve falsos estos textos, y E34 no los recoge:
- «a query failure is an error, never a verdict» (`ledger_shape.go:10-11`, `:122-124`).
- `isVerdict` (`ledger_identity.go:244-245`).
- `guardHook`, «A query failure while judging refuses the connection» (`:446-447`).
- `judgeOnConn` (`:534-536`).
- `ErrLedgerUnreadable`, «whose identity cannot be read» (`:34-39`).
- `ErrLedgerBusy`, «a write that waited», que ahora también devuelven lecturas (`:43-45`).
- `LedgerStandingUnreadable`, «a read that failed», que con la v8 pasa a ser `unavailable` o `environment` (`controlapi/act.go:75-77`).
- La lista de estados de `act.go:163-165` y `whats_happening.go:352-353`.

Reproducción:
1. Contrastar cada una de esas líneas con G-E2 y con la fila `LedgerStanding` de §4.

**N7-7 · P3 · [TEST-APROBADO-MAL-DECLARADO] · §9 l.200 y l.202.**

- **`TestOpen_seedFailureIsBootFatal`.** Su fixture es un v16 con `action_schema` vacío más el trigger `block_seed` ON `action_schema` (`errors_test.go:278-281`). Eso no es un prefijo, así que es `shapeBad` («0 rows», `ledger_shape.go:161-163`), `openFull` falla como hoy y el test sigue verde. Exigirle que «complete el residuo» contradice G-E4.
- **Clase (d).** Desde el tren D ese test no alcanza el `INSERT` de la semilla (`store.go:1505-1508`).
- **El fixture de `WhatsHappening.test.tsx:838-847`.** Describe `'database is locked'` sin código, que es exactamente lo que E13-U mantiene como `unreadable`. Moverlo a `unavailable` contradice la cura de N-4.

Reproducción:
1. Aplicar la definición de §4 al fixture de `errors_test.go:269-289`.

**N7-8 · P3 · [MUTACIÓN-SOBREVIVE][EITHER/OR] · E10, E25.**

- **E10.** El v16 tiene tres índices UNIQUE (`store.go:440`, `:479`, `:610`), y E10 solo ataca uno.
  - Mutación que sobrevive: «la forma solo conoce `execution_bindings_active_selector`».
  - E10-A agrupa tres ataques bajo un solo «→ ok» sin nombrar el desenlace de cada uno.
- **E25.** Sus sondas («eximir → rojo», «aceptar cualquier código → rojo») mutan el oráculo, no el código. El arranque sobre una forma mala nunca crea índices, así que volver a eximir `index:` (`ledger_shape_boot_test.go:121`) no enrojece sin un fallo de producción que el plan no nombra.

Reproducción:
1. Endurecer E10 y E25 tal como están escritos.
2. Aplicar las mutaciones.
3. Siguen verdes (predicción).

**N7-9 · P3 · [NIVEL-DE-EVIDENCIA][ADJUDICACIÓN N-5] · §8 frente a §6.**

- E04 es «en proceso» en §6 (l.117) y «Conexiones reales en proceso» en §8 (l.190): nivel inflado.
- E22 al revés: su molde existente, D08, usa dos handles reales (`ledger_shape_test.go:220-251`).
- A E08, E11 y E48 les falta su parte «en proceso» en §8.

Reproducción:
1. Cruzar la columna «Nivel» de §6 con las listas de §8.

**N7-10 · P3 · [SEAM-INALCANZABLE] · §6 l.110 («sus moldes viven en el mismo paquete. Este tren no exporta ningún seam»), E47-R, E23.**

- La evidencia está en (e).
- E47 se contradice: el ataque dice «seam de E13» y el molde dice «doble».
- Junto con N7-1, la línea `ledger_environment` de `ledger check` se queda sin ningún molde alcanzable.
- Calibrado como R4-4 (P3), pero es una regresión.

Reproducción:
1. Escribir E47-R en `package cli` referenciando `standingQueryFault`: no compila (predicción).
2. Intentarlo con un doble: `*actionsqlite.Store` no lo admite.

**N7-11 · P3 · [ORÁCULO][EITHER/OR] · E11 («`ledger check` (línea `ledger_unreadable`)»), §4.**

- La «línea» exige que `openReadOnly` abra ante un veredicto de Forma. Hoy devuelve el error (`store.go:2098-2101`), y §4 no lo declara.
- E11 no fija el código de salida.
- El camino alternativo (la apertura falla con `ledger_unreadable` en stderr) también contiene el nombre, así que el oráculo acepta dos desenlaces.

Reproducción:
1. Implementar solo la clasificación de G-E2.
2. `korvun ledger check` sobre el libro de E11 da `korvun ledger check: …ledger_unreadable…`, código 1, sin línea de estado (predicción).

**N7-12 · P3 · [TAXONOMÍA] · §5 fila Forma («en migrateStep, cualquier error que no sea del momento ni de entorno»), G-E4.**

- NOMEM (7), INTERRUPT (9), PROTOCOL (15), SCHEMA (17) y TOOBIG (18) se nombran `ledger_unreadable` en `migrateStep`.
- En las consultas del juicio esos mismos códigos son «Resto».
- La misma condición recibe clase distinta según el sitio. No hay fila.

Reproducción:
1. Comparar §5 l.101 con l.104.

**N7-13 · P3 · [ADJUDICACIÓN][ARITMÉTICA (h)] · §10, §9 l.205.**

- `grep -c` de R5-2, R5-3, R5-11 y R4-12 en el plan da 0: §10 no los nombra.
- «Los 50 tests»: son 45 tests (sección c).

## Lo que la v8 hace bien, verificado

- **Gate.** `/tmp/q26.txt` (14:05) acaba en «Quality gate passed.» / «EXIT=0». `find $WT -newer /tmp/q26.txt` solo devuelve los seis veredictos.
- **Orden pre-RED.** `grep -rlnE 'classifySQLite|ErrLedgerEnvironment|ledger_environment|seedSeam|standingQueryFault|afterMigrateReadVersion|beforeMigrationCommit|afterMigrationCommit|LedgerStanding(Unavailable|Environment)' internal cmd docs` sale con 1.
- **Citas de §3** que casan en el WT: `app.go:341`, `:355`, `:473-474`; `store.go:96`, `:101`, `:129`, `:1253`, `:1256-1258`, `:1438`, `:1485`, `:1501-1508`, `:2103-2109`; `ledger_identity.go:175-181`, `:268`, `:440-450`, `:545-551`, `:656-660`; `profile_standing.go:167-176`; `authority_as11_test.go:27-30`; `config_act.go:185-192`.
- **TombstoneFault.** No tiene `Code()`, pero sí `Unwrap` (`store.go:890`), así que la clasificación recorre su causa.
- **Aritmética.** 34 tablas, 5 sentencias y 12 combinaciones de `guardedTables`, contadas en el fuente.
- **Curas verificadas.** N-4, N-7 y N-11 casan con su reproducción literal. La definición del residuo (b) se sostiene.

## Alcance

- **Leído:**
  - el plan v8 entero, el veredicto de la ronda 6 entero, R4-4…R4-12 de la ronda 4 y la ronda 5 por `grep`;
  - de la lista, las secciones de `openWithIdentity`, `judgeShape`, `migrateStep`, `isBusyClass`, `printLedgerStanding` y `openReadOnly`;
  - en el WT: `store.go` (80-140, 712-735, 1236-1560, 2060-2140), `ledger_shape.go`, `ledger_identity.go` (1-700), `profile_standing.go` (1-320), `cli/ledger.go`, `cli/intent.go` (60-140), `app/app.go` (330-510), `config_act.go` (118-200), `approvals.go` (248-262), `approvals_adapter.go` (590-700) y partes de controlapi;
  - los tests `ledger_shape_test.go` (entero), `ledger_hook_test.go` (45-200), `errors_test.go`, `ledger_shape_boot_test.go`, `identity_phase1_test.go` (615-700), `tombstone_r12_test.go` (600-700), `repair_procedure_r13_test.go` (195-240), `authority_as08_test.go`, `app/v0151b_test.go` (155-240), `migration_test.go` (15-70) y `WhatsHappening.test.tsx` (825-860);
  - de modernc v1.59.0: `conn.go`, `driver.go`, `sqlite.go`, `error.go` y el comentario de `lib`;
  - `git diff -- CLAUDE.md` del WT (séptima ley).
- **Ejecutado:** solo lectura (`date`, `wc`, `shasum`, `stat`, `git`, `grep`, `sed`, `awk`, `find`, `ls`, `cmp`, `go env`). Ningún test, ninguna compilación, ninguna mutación, ningún fichero auxiliar.
- **No verificado (predicción):**
  - las rutas en ejecución de N7-1, N7-2, N7-3 y N7-4;
  - EXCLUSIVE frente a WAL;
  - que un `-wal` viejo junto a una copia de un solo fichero se reaplique al seguir D2: es un riesgo que no he examinado a fondo.
- **Sin examinar:**
  - la instantánea, la ficha UX, las decisiones y la maqueta, por la orden;
  - §14.3 y §14.6 del informe, que no se me dieron, así que los «pasos 1–6 del auditor» de E11 no se pueden comprobar;
  - el texto de entorno de D3, que remite a la maqueta.
- **Tiempo:** de 05:31 a 06:01.
- **Lo que esta ronda vio y la anterior no:** N7-1 a N7-13. N7-1 y N7-10 son regresiones que trae la reducción: la lista cerrada de sitios y los seams sin exportar.

## Integridad (06:01:16)

- `git -C $WT diff | cmp - …/adv-e7-before.patch` → «DIFF: identical».
- `git -C $WT status --short | grep '^??' | sort | cmp - …/adv-e7-untracked-before.txt` → «UNTRACKED: identical».
- `git -C $WT diff --cached --quiet` → sale con 0.
- Plan: 274 líneas, sha256 `3cd8740dda8458cad8a02073c81d9558c2918621b51303f5117a0677890e1fbc`.