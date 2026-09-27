VETO MANTENIDO

**Objeto**
- Al empezar (2026-09-26 11:16:17 WEST): `wc -l` = 2828, 715843 bytes, `shasum -a 256` = `8effacbe651e3eac6de7f99c9377676de620677f608b130d8cc1313b79b4a330`, mtime 11:05:47, acaba en salto de línea. Coincide con lo que midió el ejecutor.
- Al terminar (11:53:20): las mismas 2828 líneas, los mismos bytes, el mismo sha256 y el mismo mtime.
- Material de apoyo: extracto §14.3/§14.6 con 63 líneas y sha `9c277079…`. Los diez veredictos coinciden en bytes y en prefijo de sha. He leído enteros los de las rondas 9 y 10.
- Árbol: WT = `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `d20dea6` = `origin/master`, 135 M y 70 ??, índice vacío (medido al empezar).
- Método: la orden prohíbe tests, compilaciones y mutaciones, y no he ejecutado ninguno. Todo desenlace dinámico va marcado como predicción por lectura.

## (a) Anexos A y B frente al árbol

**Anexo A: unas 70 entradas muestreadas, todas casan.**
- Cada `return` con error de `judgeShape` (132, 159), `Standing` (153, 155), `judgeIn` (172, 175, 184, 191, 193), `judgeWithoutRow` (211, 213, 215) y `readOwner` (623).
- `judgeOnConn` (543, 554, 563, 577) y `guardHook` (461, 466, 471, 479, 488, 492).
- `openWithIdentity` (las 12 de store.go:1470–1540) y `OpenFor` (72, 76, 80, 84, 96, 107).
- `OpenOperatorFor`, `openOperatorWithIdentity`, `probeShape`, `openReadOnly` (2081–2112), `migrate` (716–730), `migrateStep` (1249–1269) y `Prune` (1770–1807).
- Las 16 salidas de las copias de migración y de `tableColumns`.
- Llamadores: los de `judgeShape` y `judgeIn`. Los de `Standing` salen 4 con `grep` (ledger_identity.go:91, app.go:409, config_act.go:187, cli/ledger.go:150). Los abridores públicos (intent.go:98 y :120, receipt.go:463, config_act.go:317/379/404, registry.go:396, harness main.go:837). Los 22 sitios de la CLI del §3 l.75 coinciden exactamente con `grep`.
- Una omisión: el hecho 9 (l.862) cuenta «three schema-version interpretation sites». El árbol tiene un cuarto lector exportado, `SchemaVersion` (store.go:1819-1824, `Scan(&v)` sobre `int`), sin llamador de producción.

**Anexo B: unas 40 citas muestreadas, casan.**
- §2: D07 :186-211, D21 :112-129 y D02 :118-120.
- §3: errors_test.go:29/113/125/263/286; ledger_identity_test.go:110/256/356/359/1092; ledger_hook_test.go:109/289/331/457/461/465; readonly_test.go:49/104.
- §4: identity_phase1_test.go:633/691/1230, config_act_registry_test.go:919-921/941-944, cli/ledger_identity_test.go:64/84/167 y cli/ledger_test.go:177.
- §6: las primeras filas de `openWithIdentity` y de `judgeShape`.
- §9: seis declaraciones (canonical_execution_test.go:841, approval_read_class_test.go:24/133/199, approvals_c2_test.go:34, approvals_f1r_test.go:22).
- Una descripción inexacta menor: l.918 dice que en :691 «the aborted copy must fail». Lo abortado es la subida de versión, por el trigger de identity_phase1_test.go:686-690.
- No he podido verificar 870/10.157, 117/117 ni 103/103: el JSON no me lo dieron.

**¿Cambia algún test aprobado, aparte de D07 y D21?** Todo es predicción por lectura.
- Quedan igual:
  - D02 (se mantiene `schemaTablesV16`).
  - D15 :326 (el plan fija el valor en l.135).
  - `TestConfigActRecorder_anUnreadableStandingIsNamed`.
  - `TestOpen_seedFailureIsBootFatal`: 34 tablas y el trigger `block_seed`, así que no es prefijo.
  - D18: pasa por la migración, y la relectura ve 15, igual a `from`.
  - Los de fichero corrupto: solo exigen `err != nil`.
  - No hay asserts de texto exacto ni de prefijo: `grep` de `err.Error() ==`, `HasPrefix(err.Error()` y `stderr.String() ==` no devuelve nada.
- El fixture TSX (WhatsHappening.test.tsx:838-849) sigue verde, pero con D2 pinta un remedio destructivo: C1-1.
- Excepciones posibles, sin declarar: la firma para pasar el origen (C1-8) y la interfaz para `{ruta}` (C1-4).

## (b) §11 frente a los hallazgos que siguieron abiertos

| Hallazgo | Disposición del plan | Veredicto | Destino | Evidencia |
|---|---|---|---|---|
| N9-1 | E: TE15, 20, 46 | CIERRA A MEDIAS | BIEN | TE15 cubre los pasos 1-4, pero renombra a `r` y no a `rd`. El paso 5 solo va sintético (TE20, código 11 en QmarkDB). El paso 6 no lo reproduce ninguna fila (C1-15). |
| N9-2 | E: TE19–30, §7 | CURADO (predicción) | BIEN | M1 (`isVerdict` ampliado en `refreshGuard`) → TE24–26 con origen refresh: OpenFor devuelve un handle con la guarda pegajosa y la fila exige nil, así que rojo. M2 (el gancho pasa busy a unreadable) queda alcanzado, porque el seam va en el resultado de la lectura (l.206): rojo. |
| N10-1 | E: TE18 | CURADO (predicción) | BIEN | La reproducción literal (`buildV1File` + `version = 99`) está en TE18. MU18 → rojo. |
| N10-2 | E: TE19, 24–26, 46 | CURADO (predicción) | BIEN | MJ-h y MJ-p quedan alcanzados por el fallo con origen. La l.225 excluye los mutantes que solo tocan el seam. |
| N10-3 | E: TE16, 19–23 | CURADO (predicción) | BIEN | 11/20/24/26 forzados por sitio: «quitar 11» → TE20 rojo. NULL vía destino permisivo. El oráculo de causa se discute en C1-2. |
| N10-4 | E: TE31, 43, 47 | CURADO (predicción) | BIEN | MU31 va sobre `probeShape` (store.go:1454-1456). `receipt verify` está en O6 y TE43. |
| N10-5 | E: TE15, 42, 43 | CURADO (predicción) | BIEN | Desenlace fijado: línea unreadable, error de `ListReceipts` (ledger.go:507) y salida 1. |
| N10-6 | E: TE17, 32 | CURADO (predicción) | BIEN | `CAST('16 ' AS BLOB)` literal y MU17. FTS5 con validación del tipo virtual y MU32. |
| N10-7 | E: TE10, 11 | CURADO (predicción) | BIEN | SQL literal en l.243-246. D02 intacto. |
| N10-8 | E/H: TE54 y D07 | CURADO en su letra | BIEN | Quedan declarados el nombre, la prosa, la mutación y el mensaje de D07, y el godoc de `hookShapeFault`. Con GE4, el godoc de `ErrSchemaFromTheFuture` vuelve a ser cierto. Omisiones nuevas en C1-14. |
| N10-9 | E: TE30, 46, anexo A | CURADO | BIEN | La ruta de l.47 incluye el `Standing` de :91, y `beginAdoption` y `refuseMaintenance` están entre los orígenes. |
| OBS-1 | excluido salvo garantía ampliada | NO CIERRA | MAL | La condición de exclusión la dispara el propio GE2 (tercera frase de l.33). Ver C1-13. |

**Resultado:** de los 12, diez quedan CURADOS (predicción; N10-8 solo en su letra), N9-1 CIERRA A MEDIAS y OBS-1 NO CIERRA, con destino MAL.

## (c) La siembra IMMEDIATE con re-juicio dentro del candado

- **Tests aprobados** (predicción): no encuentro ninguno roto.
  - `TestOpen_seedFailureIsBootFatal` no es resto.
  - D18 va por la migración.
  - En D01 (ledger_shape_test.go:59-93), el pool de conversaciones abierto no retiene escritura.
  - C05/C06 (ledger_hook_test.go:158-200) siguen igual.
  - Las llamadas directas a `migrateStep` (identity_phase1_test.go:633/691/1230) tienen la versión igual a `from`.
  - `grep` no encuentra ningún test de dos aperturas concurrentes sobre un fichero fresco.
- **Compatibilidad con los restos históricos:**
  - Verificado: `createStmt` es idéntico en todas las etiquetas de v0.11.0 a v0.16.1 y en el WT (hash `4790eb968e06`). Antes de v0.11.0 no existía el almacén de actos.
  - Los cinco objetos no tienen autoíndices: `actions` y `action_decisions` son WITHOUT ROWID, y `action_schema` no tiene restricciones (store.go:102-129).
  - El gancho de la conexión que siembra comparte `judgeShape` (ledger_identity.go:541): verá el resto como fresco y pondrá la guarda en `ok`.
  - Las migraciones solo hacen ADD COLUMN sobre `actions` (store.go:151-153), así que los triggers temporales que el gancho pone sobre `actions` en un resto k≥2 no chocan (predicción).
- **El candado:** en WAL, BEGIN IMMEDIATE no bloquea a los lectores. Otro escritor espera `busy_timeout(5000)` (store.go:91), igual que hoy con `createStmt` en autocommit.
- **Lo que queda abierto:** C1-5 (origen del re-juicio y posible interbloqueo) y C1-6 (MU04).

## (d) Seams del §7

**Alcance por origen.**
- Cada par (origen, sitio) llega al clasificador si el origen se pasa hacia abajo.
- `refreshGuard`, `Standing` de :91, `Standing` público, `beginWrite` y poda, `beginAdoption` y `refuseMaintenance` comparten `judgeIn(ctx, q, me)`. El de :91 y el público comparten además `Standing(ctx)`, así que distinguirlos exige tocar la firma o el contexto (C1-8).
- El gancho va anidado dentro del primer `QueryContext` del `judgeShape` del pool (store.go:1494). Ese es el sitio que falla: C1-3.
- Varios orígenes absorben el veredicto sin guardarlo en ningún sitio: C1-2.

**S1–S5.**
- S1 y S2 son NUEVOS y viven dentro de la transacción NUEVA. Hoy `createStmt` va en un solo `Exec` (store.go:1501); partirlo lo implica l.83.
- Los sitios de S3 existen: ledger_identity.go:474-494 y 312-320, store.go:1768-1808, 1454, 2091 y 715-1269.
- S4 cabe entre store.go:715 y el `Begin` de :1247. S5 va en las etapas de `migrateStep`.

**Mutaciones (predicción por lectura).**

| Filas | Mutación | Desenlace |
|---|---|---|
| TE01 | MU01 | Rojo por el oráculo crudo antes de reabrir. |
| TE02, TE03, TE05 | MU02, MU03, MU05 | Rojo. |
| TE04 | MU04 | Sobrevive (C1-6). MU33 no es determinista en esta fila; TE33 sí la pone roja. |
| TE06 | MU06 | Rojo, si el FULL llega a la siembra. |
| TE07 | MU07 | Rojo por la variante D18. El «INSERT de versión incondicional» no se alcanza reabriendo un libro actual. |
| TE19 | MU19 | Rojo. |
| TE20–23 | MU20–23 | Rojo, pero el desenlace sin mutar tampoco se alcanza en cinco orígenes (C1-2). |
| TE24–27 | MU24–27 | Rojo. |
| TE28 | MU28 | Rojo en los seis códigos listados. Para 2/4/15/16/17 no hay fila (C1-7). |
| TE29 | MU29 | Rojo con 5 y 10. Con 1/11/26, el desenlace sin mutar es inalcanzable (C1-3). |
| TE30, TE33–36, TE38, TE39, TE46 | sus MU | Rojo. TE33 lo consigue por el contador de DDL = 0. |
| TE37 | MU37 | Sobrevive (C1-6). |

La l.225 deja fuera las mutaciones que solo tocan el seam o su observador, como pide la orden.

## (e) O1–O12

Faltan tres efectos:
- La ruta `{ruta}` que tiene que llegar a la pantalla (C1-4).
- Los botones de escritura desactivados en ilegible y en entorno. TE49 lo exige y O2 no lo declara (C1-10).
- El arranque estricto: su respuesta heredada, que el §1 obliga a probar (C1-9).

Todo lo demás está cubierto, o no aplica:
- El parser único cubre `migrate`, `requireCurrentSchema`, el lector y el operador. `SchemaVersion` queda fuera (C1-14).
- La clase como prefijo en las cuatro fronteras: O6–O10, con la salida 1 intacta.
- Los estados del grabador: O2. `receipt verify`: O6.
- Descriptores UNIQUE frente a libros migrados: el DDL de los tres índices es byte a byte igual en v0.16.0, v0.16.1 y el WT (hash `c8c44d0a…`), y solo aparece desde v0.16.0 (esquema v15). El descriptor casa con cualquier libro migrado.

## TE49 y TE50

**TE49**, juzgada con la frase nueva («Mientras tanto, los botones de esta pantalla no registran nada.»):
- El riesgo de D3 del §12 (l.653) queda cerrado. La frase ya no promete nada sobre la ejecución global: solo habla de los botones.
- Queda un residuo P3. La frase habla de todos los botones de la pantalla, y la fila solo prueba «disabled write controls» sobre un fixture sin concretar (C1-10).

**TE50** está mal definida (C1-11):
- Los perfiles están bien: el real es de v0.16.0, dos versiones atrás. Pero la celda mezcla «isolated migrated fixture».
- La build no está fijada.
- No dice cómo llevar la app empaquetada a los estados de entorno y del momento con fallos nativos.
- El criterio «agrees with API» es circular.
- No sitúa la prueba respecto al PR, que es donde la séptima ley la pone.

## Filas que meten trabajo de otro tren

Ninguna.
- TE46 y TE47 (CreateLedger, registro, rotate-key y adopción) se quedan en la respuesta heredada, con las fronteras G/F dichas en la propia fila.
- TE44 y TE45 solo prueban la clase estructural existente.
- TE49 deja fuera «Última acción» y los textos de puerta, que son del tren H.

## Hallazgos nuevos

**C1-1 · P2 · [AFIRMACIÓN-FALSA][TAXONOMÍA] · D2 (§7 l.265-267), GE7 (l.38), §5 l.121-122 y l.138, TE49 (l.195), GE10 (l.41), §12**

El plan pinta D2 para toda proyección `unreadable`, y según su propio grabador eso incluye cualquier error que no sea busy ni de entorno: los errores sin código y los códigos «uncategorized», de los que dice «not a verdict». Así, el remedio que manda sustituir o apartar el libro aparece sobre un libro que el almacén no ha juzgado.

Evidencia:
- config_act.go:187-192: cualquier `err` de `Standing` sale como `LedgerStandingUnreadable` con `err.Error()`.
- El fixture aprobado de WhatsHappening.test.tsx:842 trae `owner: 'database is locked'`.
- El fichero es el compartido con las conversaciones (store.go:1390-1391: «normally the SAME file the conversation store uses»; app.go:366-367).
- El plan quita ese consejo en la CLI (l.298) pero lo deja en la pantalla.

Reproducción (predicción):
1. Fixture aprobado: `/api/whats-happening` devuelve `{ledger:{standing:'unreadable', owner:'database is locked'}}`.
2. Implementar TE49 tal como la fila lo pide: D2 exacto para `unreadable`.
3. La pantalla dice: «El libro no se puede leer: database is locked. Korvun no lo repara ni lo recrea. Detén Korvun y sustituye {ruta} por una copia tomada antes del fallo; … Si no tienes copia, aparta esos ficheros y Korvun empezará un libro nuevo, sin el historial de este perfil.» El assert aprobado de :846 sigue verde.
4. En producción: `Standing` falla con un error sin código (el de pool cerrado que D15 fija como no-veredicto, ledger_shape_test.go:315-329) o con 7, 9 o 21. Se proyecta `unreadable` y se pinta D2.

Por qué P2: es la clase de N10-1. Un libro que no está corrupto se presenta con un remedio que lleva a abandonarlo, y aquí también a perder las conversaciones. GE10 queda violado por diseño, y el §12 no lo recoge (solo recoge D3). Pide adjudicación, porque dos tests aprobados fijan `unknown → unreadable`. No es P1: ningún código destruye nada por sí solo.

**C1-2 · P2 · [ORÁCULO][EITHER/OR][SEAM-INALCANZABLE] · TE20–23 (l.166-169), §7 l.221 y l.223, §5 l.134, §4 l.107**

Con el protocolo de un solo impacto por (handle, origen, sitio, etapa), el desenlace de la fila («opening blocked where no later birth failure; … cause and original code retained», con «accepted write» prohibido) no se produce en cinco orígenes. Nada persiste su veredicto, y todos los juicios siguientes leen datos sanos.

Evidencia:
- openWithIdentity trata shapeBad con identidad en `default` sin tocar la guarda (store.go:1532-1537).
- OpenFor:91-98: un veredicto de `Standing` solo salta la poda.
- §4 l.107 enumera tres puertas del veredicto y ninguna nueva.
- La causa no tiene dónde ir:
  - `shapeVerdict.reason` es un string (ledger_shape.go:55-60);
  - `judgeOnConn` devuelve solo una cadena de estado (ledger_identity.go:539-586);
  - la guarda guarda solo el estado (:474-476).

Reproducción (predicción):
1. `foundedFor(profileA)` y cerrar.
2. Armar `judgeReadFault` de un solo impacto: origen pool-open-shape, sitio Qcatalog, código 11.
3. OpenFor: el gancho (otro origen, sin fallo) ve el libro sano y pone la guarda en `ok`. El `judgeShape` del pool recibe 11 y, por l.134, devuelve shapeBad y error nil.
4. Rama `default` → `Ping` (store.go:1538) → Store. `refreshGuard` y el `Standing` de :91 leen sano; la poda corre.
5. OpenFor devuelve un handle sano, y una escritura posterior se acepta. La fila falla por construcción.
6. El mismo desenlace sale con los orígenes «OpenFor's Standing», operator-probe y readOwner-at-open. «Only the current-shape opener translates into a blocked handle» (l.134) exigiría una cuarta puerta del veredicto que l.107 no declara.
7. Con el origen del gancho sí bloquea, pero la causa y el código se pierden, y `Standing` dice `ok`.

Por qué P2: la rejilla que tiene que probar GE2 no puede pasar a verde tal como está escrita. Arreglarla obliga a rediseñar: dar un desenlace por origen, o añadir puertas del veredicto. Son los puntos 1-2 de la doctrina.

**C1-3 · P2 · [TAXONOMÍA][AFIRMACIÓN-FALSA] · GE2 (l.33) frente al §5 l.136, el §5 l.113 y l.134, TE29 (l.175), §7 l.223 y l.233**

Un fallo estructural en una etapa del gancho que no es de juicio llega al `judgeShape` del pool como resultado de su consulta. Tiene `Code()` estructural, así que GE2 y l.134 lo convierten en shapeBad. l.136 dice lo contrario («fatal opening error, even if an enclosing shape query first sees it»). La tabla de l.115-128 no tiene clase para un fallo de nacimiento, y el clasificador de l.113 decide solo por `Code() & 0xff`.

Evidencia:
- El gancho envuelve su error (ledger_identity.go:479, 488, 492) y el driver añade «connection hook: %w» (driver.go:266-269).
- Un error de pragma llega sin envoltura propia (conn.go:151-154).

Reproducción (predicción):
1. `foundedFor(profileA)` y cerrar.
2. S3 de un solo impacto: código 11 en `CREATE TEMP TABLE IF NOT EXISTS profile_guard` (ledger_identity.go:474).
3. OpenFor → `judgeShape` del pool (store.go:1494; consulta en ledger_shape.go:130) → nace la conexión → el juicio del gancho sale bien → el CREATE falla con 11 → «connection hook: …» con `Code()` = 11.
4. Queda shapeBad con error nil → `default` → `Ping` hace nacer otra conexión; el fallo ya se consumió → gancho sano.
5. OpenFor devuelve un handle sano y err nil. TE29 exige «Structural→fatal nil handle ErrLedgerUnreadable» y prohíbe el handle usable.

Por qué P2: es una contradicción interna en la regla de sitio, que es el núcleo del tren. El rojo no puede pasar a verde sin un tipo que el plan no nombra, o sin rebajar la fila, que el rojo aprobado prohíbe.

**C1-4 · P2 · [PLAN-FILA-AUSENTE][AFIRMACIÓN-FALSA][TEST-APROBADO-NO-DECLARADO] · D2 y D3 (l.267, l.273), TE49 «actual configured path» (l.195, l.280), O2 (l.63), GE7**

`{ruta}` no tiene cable.

Evidencia:
- El GET solo lleva rows, conditions y ledger{standing, owner} (whats_happening.go:363-367). La interfaz es `LedgerStanding(ctx) (standing, owner string)` (act.go:167).
- El grabador tiene la ruta (config_act.go:65, rellenada con `storagePath(cfg)` en app.go:670-671), pero no la entrega.
- La ruta por defecto solo existe resuelta en Go (app.go:902-911).
- La única configuración que lee la pantalla es `/api/config` (WhatsHappening.tsx:286).

Reproducción (predicción):
1. Perfil con `"storage": {}`; se funda el libro.
2. `ALTER TABLE action_schema RENAME COLUMN version TO v` (TE13); arrancar.
3. El GET no trae ruta.
4. D2 y D3 se pintan sin `{ruta}`.
5. TE49, en jsdom, se alimenta de un fixture que puede traer una ruta que producción nunca manda: verde. Es la clase (d).
6. Traerla obliga a cambiar la interfaz, y con ella los dobles aprobados act_fake_test.go:86 y act_external_fake_test.go:67. Eso no está en la lista cerrada de l.302.

Por qué P2: GE7 y la lista cerrada de efectos son falsos tal como están planeados, y el único molde no puede ponerse rojo por el lado Go.

**C1-5 · P3 · [PLAN-FILA-AUSENTE] · §4 l.83, lista de orígenes l.221**

El re-juicio dentro del candado es un consumidor nuevo del juez y no está en la lista de orígenes.
- Sus desenlaces para libro actual, más nuevo, forma mala o fallo de lectura no están dichos: solo «skips the seed if another opener committed it».
- Por la regla del propio plan (l.221: «adding a query without a grid entry leaves the guarantee unproved»), eso queda sin probar.
- El plan tampoco ata el re-juicio a la transacción. Si va por `db`, con `SetMaxOpenConns(1)` (store.go:1485) espera la única conexión, que tiene la tx de siembra, y se queda interbloqueado sin plazo (`context.Background()`). Es el patrón de interbloqueo de conexión única que ya vivió este repositorio, y ningún molde le pone un oráculo con plazo (predicción).

**C1-6 · P3 · [MUTACIÓN-SOBREVIVE] · TE04 MU04 (l.150), TE37 MU37 (l.183)**

- MU04 (quitar el re-juicio): B vuelve a ejecutar los cinco DDL con IF NOT EXISTS y el `INSERT … WHERE NOT EXISTS` de store.go:1505, que no hacen nada. TE04 sigue viendo una fila de versión, v16 y dos aperturas nil: verde. El plan no fija la forma del INSERT (l.83).
- MU37 no nombra un sitio que exista. El único juicio posterior a la migración en la ruta es `refreshGuard` → `judgeIn` → `judgeShape` (ledger_identity.go:284; profile_standing.go:170-175), y quitarlo es la mutación de D03/D04. Un chequeo explícito añadido en `openWithIdentity` quedaría tapado por ese re-juicio: TE37 verde (predicción).

**C1-7 · P3 · [TAXONOMÍA] · tabla del §5 (l.115-128), TE28**

La tabla no cubre todos los códigos primarios. Faltan 2 INTERNAL, 4 ABORT, 15 PROTOCOL, 16 EMPTY y 17 SCHEMA (lib/sqlite.go:3754, 3012, 4170, 3460, 4254), el 19 fuera de la migración y el 27/28. TE28 solo prueba 7, 9, 12, 18, 21 y 25.

Mutación que sobrevive: clasificar el 15 (protocolo de cerrojo de WAL, transitorio) como estructural. Pondría una guarda pegajosa, que GE3 prohíbe, y ninguna fila se pone roja.

**C1-8 · P3 · [TEST-APROBADO-NO-DECLARADO] · §7 l.206 («an origin is passed explicitly down»), §4 l.107, l.302 y l.650**

El plan no dice cómo baja el origen. Si va como parámetro, dejan de compilar llamadas de tests aprobados:
- ledger_shape_test.go:322 y :326 (D15), :354 (D16), :383 (D17) y :446 (D19);
- ledger_hook_test.go:72, en `judgeOnRawConn`, que usan D01 (:75) y C05/C06 (:162, :181);
- identity_phase1_test.go:633, :691 y :1230.

La lista cerrada «D07 y D21» no queda demostrada.

**C1-9 · P3 · [PLAN-FILA-AUSENTE] · §1 l.22, O1 l.62 («Non-strict boot»)**

El §1 obliga a probar la respuesta heredada del arranque estricto (del tren H) a los errores estructurales nuevos, pero no hay fila.

Evidencia: la rama estricta ejecuta `PrepareStrictAuthority` antes de `else if unreadable` (app.go:442-452). Esa función lee la activación y escribe cláusulas por cada cerebro agente (identity.go:433-450).

Reproducción (predicción):
1. Perfil estricto activado; aplicar el rename de TE13.
2. Hoy, OpenFor muere (§14.3 P2-2, pasos 3 y 5).
3. Tras E, OpenFor devuelve un handle bloqueado. Con cerebros agente, el arranque muere por otro error. Sin ellos, el arranque estricto levanta sobre un libro ilegible. Es un cambio de comportamiento no declarado.

**C1-10 · P3 · [ORÁCULO] · TE49 (l.195), O2 (l.63)**

La frase nueva habla de todos los botones de la pantalla, pero la fila solo prueba «try disabled write controls» sobre un fixture sin concretar. Hoy ningún botón depende del estado del libro: `canPress` (WhatsHappening.tsx:715), el botón de confirmar (:773) y las filas con botón (:519, :569). Desactivarlos es comportamiento nuevo que O2 no declara.

Reproducción:
1. Fixture en estado de entorno, con aprobaciones apagadas, una jaula y una condición bloqueada.
2. Mutación: dejar activo el botón «Encender aprobaciones».
3. TE49 no lo prueba, porque no está entre los «disabled»: verde, con la frase falsa.
4. Con un fixture sin ningún BlockedRow, la fila ni siquiera prueba nada.

**C1-11 · P3 · [ORÁCULO][DEFINICIÓN] · TE50 (l.196), §8 l.369, §9 l.400**

Estos puntos dependen de la definición de la fila, no de su ejecución:
- La build no está fijada: ni commit, ni hash del artefacto. l.369 admite incluso «dirty diff digest».
- No hay procedimiento nativo para llevar la app empaquetada a entorno o a «del momento» sobre una conexión WAL viva. El truco de TE52 es DELETE + EXCLUSIVE contra el sellado del lector de la CLI.
- El criterio «agrees with API» es circular.
- «isolated migrated fixture» va en la misma celda que el perfil real.
- La séptima ley pide la prueba con el perfil real «BEFORE its PR» (CLAUDE.md del WT:392-402), y la fila no la sitúa respecto al PR.

**C1-12 · P3 · [SEXTA-LEY][ORÁCULO] · D2 (l.265-267), §12**

- El D2 del plan (copiloto.md:351) no coincide con cómo lo pinta la ficha: la ficha (MAIN, design-drafts/2026-09-25-tren-E-estados-del-libro-UX.md:30-38) usa otro título, «restaura tu copia, o apártalo…», la línea del documento, «Mientras no se lea…» y el motivo junto a los botones.
- El «accepted copy» de TE49 tiene así dos candidatos, y el plan solo recoge la divergencia de D3.
- D2 tampoco dice que `{ruta}` es el fichero compartido con las conversaciones. TE51 (l.282) se lo exige al documento de restauración, no a la pantalla.

**C1-13 · P3 · [ADJUDICACIÓN-NO-CIERRA] · §11 OBS-1 (l.636), l.418, §9 (a) (l.390) y l.379**

La exclusión dice «unless newly widened guarantee», y el propio GE2 amplía la garantía: «Absence, failed reads and invalid values remain separate».

Reproducción (predicción):
1. `foundedFor(profileA)`.
2. Reconstruir `ledger_identity` sin el `CHECK (id = 1)`, conservar la fila 1 e insertar una fila 2 con `owner_digest` de B.
3. OpenFor: el gancho ve COUNT = 2 y responde unreadable (ledger_identity.go:552, 583-585), con la guarda pegajosa. `judgeIn` lee `WHERE id = 1` (profile_standing.go:178) y responde ok.
4. `Standing` y la pantalla dicen que el libro está sano mientras todas las puertas rehúsan con el 1811.
5. Variante en un libro legado con solo la fila 2: `judgeIn` devuelve legacy («No bloquea nada»).

La respuesta (a) del §9 no recoge este caso.

**C1-14 · P3 · [AFIRMACIÓN] · inventario de textos (l.288-300), GE4 (l.35), anexo A l.862**

Faltan textos que E vuelve falsos:
- El godoc de `guardHook` (ledger_identity.go:442-449), «A query failure while judging refuses the connection…»: falso con los códigos estructurales.
- El godoc de `ErrLedgerBusy` (:43-44), «names a write that waited…»: tras E también sale de lecturas (TE24/25 en `Standing`, TE52 en el sellado del lector).
- `SchemaVersion` (store.go:1819-1824) sigue leyendo con `int` y queda fuera del contrato único.

**C1-15 · P3 · [ADJUDICACIÓN-NO-CIERRA] · N9-1 (§11 l.610)**

- Ninguna fila reproduce el paso 6: app viva sobre un libro legado, `receipts.result_digest` renombrada desde otra conexión, y luego `POST approve` y `enable-approvals`. TE15 arranca después del daño, TE44 daña antes del arranque, y TE45 renombra `action_schema.version`: Qversion falla antes de llegar a la marca.
- TE15 renombra a `r`; la reproducción de la ronda 9 usa `rd` (regla 1).

## Lo que el plan hace bien, verificado

- El estado del §1 coincide con el árbol: HEAD, 135 M, 70 ?? e índice vacío.
- El inventario de la CLI (l.75) coincide sitio por sitio con `grep`, y el muestreo de los anexos A y B casa.
- Las columnas de nivel de evidencia del §6 y del §8 coinciden en las 54 filas. Lo comparé con dos ficheros auxiliares de mi scratchpad: `c1-ev6.tsv` y `c1-ev8.tsv`.
- Los niveles son honestos: sintético, jsdom y harness van etiquetados como tales.
- Aritmética:
  - 148 hallazgos = 26+27+16+12+11+12+13+11+10+10;
  - 54 filas TE y 53 MU distintos;
  - 34 tablas, 11 nombres de índice y 3 UNIQUE.
- Los rangos de líneas de las rondas 9 y 10 casan con sus veredictos. Los códigos del §5 casan con lib/sqlite.go.
- `createStmt` y el DDL UNIQUE son estables entre releases (lo verifiqué con hashes por etiqueta).
- GE4/TE18 cura la regresión de N10-1 (predicción). El seam en el resultado de la lectura alcanza la decisión nueva de `judgeOnConn` y `judgeIn` en errores busy y de entorno (predicción).
- La frase nueva de D3 cierra el riesgo del §12.
- No hay filas de otro tren, y los mutantes que solo tocan el seam quedan excluidos.

## Alcance

**Leído:**
- El plan entero del §1 al §12 (l.1-663) y el anexo A entero.
- Del anexo B: §1-§5 y §8, más muestras de §6 y §9. Del anexo C: A, B y el comienzo de E.
- Los veredictos de las rondas 9 y 10, enteros, y el extracto §14.3/§14.6.
- En el WT, enteros: `ledger_shape.go`, `ledger_identity.go`, `profile_standing.go` y `ledger_shape_test.go`.
- En el WT, por tramos:
  - `store.go`: 60-150, 405-490, 590-770, 1230-1310, 1390-1600, 1750-1850 y 2025-2121;
  - `app.go`: 360-520, 668-676 y 902-911;
  - `config_act.go`: 59-112, 170-200 y 300-420;
  - `whats_happening.go` 340-380, `act.go` 60-90 y 155-172, `approvals_adapter.go` 600-655 y `controlapi/approvals.go` 190-250;
  - `cli/ledger.go` 60-166, `cli/intent.go` 85-140, `cli/receipt.go` 105-175 y 455-470, y `harness/main.go` 825-845;
  - `WhatsHappening.tsx` 420-801 y `WhatsHappening.test.tsx` 826-875;
  - tests: `ledger_hook_test.go` 53-235, `errors_test.go`, `identity_phase1_test.go` 600-700 y 1210-1235, `approval_read_class_test.go` 120-226, `ledger_test.go` 110-160, `migration_test.go` 24-100, `config_act_registry_test.go` 915-945 y `ledger_shape_boot_test.go` 40-135;
  - `conversation/sqlite/sqlite.go` 225-262 y el CLAUDE.md del WT 384-413.
- Del driver: `driver.go` 250-290, `conn.go` 56-160 y las constantes de `lib/sqlite.go`.
- De MAIN: la ficha UX, 1-80, y la línea 351 del plan del copiloto.

**Ejecutado:** solo lectura: `date`, `wc`, `shasum`, `stat`, `git` (diff, status, show, tag, rev-parse, grep), `grep`, `sed`, `awk`, `cmp`, `ls`, `go env`. Ficheros auxiliares, fuera de los árboles: `c1-ev6.tsv` y `c1-ev8.tsv`.

**Muestreado:** unas 70 entradas del anexo A y unas 40 del anexo B, con los resultados dichos en (a).

**No verificado (predicción):**
- Todos los desenlaces dinámicos.
- La semántica de SQLite: la normalización de `IF NOT EXISTS` en `sqlite_master`, triggers temporales frente a ADD COLUMN, triggers sobre FTS5, y cuándo aparecen PROTOCOL o SCHEMA.
- Los recuentos del JSON del anexo B.
- Los destinos del §11 para las rondas 1 a 8, más allá de los recuentos.
- El TSX fuera de los tramos citados.

**Tiempo:** de 11:16 a unos 11:56, dentro del presupuesto.

**Lo que esta ronda vio y las anteriores no:**
- El remedio D2 sobre errores desconocidos.
- Los orígenes que absorben el veredicto.
- El fallo de nacimiento que se juzga como forma.
- `{ruta}` sin cable.
- El re-juicio sin origen.
- MU04 y MU37.
- Los códigos que no están en la taxonomía.
- El arranque estricto.

## Integridad (11:53:20)

- `git -C $WT diff | cmp - …/adv-c1-before.patch` → «DIFF: identical». Al empezar (11:16) dio lo mismo.
- `git -C $WT status --short | grep '^??' | sort | cmp - …/adv-c1-untracked-before.txt` → «UNTRACKED: identical». Al empezar, también.
- `git -C $WT diff --cached --quiet` → `cached_exit=0`.
- El plan sigue con 2828 líneas, 715843 bytes, sha256 `8effacbe651e3eac6de7f99c9377676de620677f608b130d8cc1313b79b4a330` y mtime 11:05:47. HEAD sigue en `d20dea6`. El extracto sigue en `9c277079…`.