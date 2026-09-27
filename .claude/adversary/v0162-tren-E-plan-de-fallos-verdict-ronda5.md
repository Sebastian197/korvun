VETO MANTENIDO

**Objeto**
- Al empezar (2026-09-25 19:03:43): `wc -l` da 417 y `shasum -a 256` da `fe055ae857578cee117c6f9dcd032921698d6d0f4470edd6ff0c01c346f0d909`, con mtime 19:01:44. Coincide con lo que midió el ejecutor.
- Al terminar (2026-09-25 19:25:06): 417 líneas y el mismo sha256.
- Árbol: WT=`/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `d20dea6`. Las citas `internal/…`, `cmd/…` y `docs/…` son `$WT/…`.
- Método: la orden prohíbe tests, compilación y mutaciones, y no ejecuté ninguno. Todo desenlace dinámico va marcado como «predicción por lectura». Los comandos que sí ejecuté son de lectura y aparecen en cada punto.

## 1 · Adjudicaciones de la ronda 4 (§10 de la v6)

| Hallazgo | Veredicto | Evidencia |
|---|---|---|
| R4-1 (a) | CIERRA | §9 declara `TestMount_aForeignLedgerRefusesEveryDoorButAdopt`. Leí `:209-212` («want 200 adopted») y `:223-226` (estado `ok`), y el doble de `act_external_fake_test.go:52`. La reproducción literal de R4-1 queda cubierta. La cura (campo obligatorio) rompe más tests aprobados sin declararlos: R5-1. |
| R4-1 (b) | CIERRA | `approval_read_class_test.go:218-225` exige `errors.Is(err, ErrApprovalUnreadable)` y que no sea `ErrApprovalEvidenceCorrupt`. §4 conserva el centinela exterior y envuelve la clase, así que el test pasa tal cual. |
| R4-2 | CIERRA A MEDIAS | E48 es la reproducción literal. `policy_version` es `INTEGER NOT NULL` (`store.go:276…370`) y el juez solo admite `integer`/`null` (`store.go:1024`), así que `'x'` produce un `*TombstoneFault` en la copia 10→11 (`store.go:1219-1221`), con el texto «tombstone_corrupt» (`:871-887`). `migrateStep` deshace el paso (`:1246-1272`) y la versión queda en 10. La mutación es la correcta. La regla «sin código» sigue sin estar cerrada (R5-3). |
| R4-3 | CIERRA | Fixture v15 en la suite, captura manual fuera y `POST adopt-ledger` por API. Sin marca, `seedIdentityRowV15toV16` no inserta nada (`profile_standing.go:231-233`), así que la mutación de «fundación automática» deja el estado en `ok` y enrojece (predicción). |
| R4-4 | CIERRA | En E47-A el bloqueo va antes de abrir: `openOperatorStore` → `OpenReadOnlyFor` (`intent.go:98`) → la primera conexión en `PRAGMA query_only` (`store.go:2093-2097`) da BUSY y sale por `ledger.go:86-88` antes de `printLedgerStanding` (predicción). `StandingFaultForTest` exportado se alcanza desde `internal/app` y `internal/cli`, que están en el mismo módulo. Queda el riesgo del paralelismo (R5-5). |
| R4-5 | CIERRA | §3 lo declara inalcanzable por lectura y E02 lo fija. Los abridores completan el residuo (`store.go:1500-1512`) o rehúsan lo fresco (`:1435-1436`, `:2103-2106`) antes de que exista un `Store`. |
| R4-6 | CIERRA | Las tres mutaciones de R4-6 enrojecen: nombres → A(2) y A(3); subconjunto → A(4); índices → A(5). Hay dos huecos nuevos en la definición (R5-9). |
| R4-7 | CIERRA | G-E1 casa con `store.go:1435-1436` y `config_act.go:392-394`. |
| R4-8 | CIERRA | Leí `supervisor.go:232-239` (`shutdownApp`), `:270` (`buildAndStart`) y `:379` (`RequestReload`). |
| R4-9 | CIERRA | E34 recoge `ledger_identity.go:4-7` y `store.go:1438`. |
| R4-10 | CIERRA A MEDIAS | La guarda sin poner ya tiene fila y nombre, pero su clase («del momento») es falsa en producción (R5-8). |
| R4-11 | CIERRA | `recordAuthenticatedTx` escribe `actions`, `action_decisions` e `identity_evidence_v2` (`identity_v2.go:189-230`, `insertEvidenceTx`), y `signEvidenceTx` no escribe (`:252-271`). Con eso, el oráculo cubre todas las tablas persistentes que escribe `AdoptLedger`. |
| R4-12 | CIERRA A MEDIAS | (1) `ledger check` usa los nombres `ledger_*`. (2) El prefijo del arranque es el del sitio que falla (`app.go:383`, `:414`). (3) La excepción de «Activar almacén» se apoya en una frase falsa (R5-2). |

Resultado: 10 CIERRA, 3 CIERRA A MEDIAS y 0 NO CIERRA, contando R4-1 como dos filas.

| Anteriores | Veredicto | Evidencia |
|---|---|---|
| R3-3 | CIERRA | Por R4-5. |
| R3-4 | CIERRA A MEDIAS | (a) E47 R y A se alcanzan. (b) La clase determinista tiene fila (E27, `RAISE(ABORT,'x')`) y la guarda sin poner tiene nombre, pero con una clase falsa (R5-8), y el fallo del gancho de E27 no llega a las puertas (R5-4). |
| R3-15 | CIERRA | `/tmp/q26.txt` (14:05) acaba en «Quality gate passed.» / «EXIT=0». `find $WT -newer /tmp/q26.txt -type f`, sin `.git` ni `node_modules`, devuelve solo los cuatro veredictos de `.claude/adversary/`. E46 declara su captura. |
| P3-e | CIERRA A MEDIAS | §7 y §7-bis (d) dicen «cada fila tiene su mutación», y es falso para E34, cuya columna de mutación dice «—». §7-bis (e) no ve G-E10 en «Activar almacén» (R5-2). |

## 2 · (b) El residuo de §4

Lo que la definición hace bien (por lectura):
- `createStmt` tiene cinco sentencias (`store.go:101-129`). Su texto es idéntico en todas las etiquetas desde v0.11.0 hasta v0.16.1 y en el WT. Lo comprobé con `git show <tag>:…store.go | awk '/^const createStmt/…' | shasum`, que da `ddb269733632` en todas.
- `actions` y `action_decisions` son `WITHOUT ROWID` y `action_schema` no tiene PK ni AUTOINCREMENT, así que no hay `sqlite_autoindex_*` ni `sqlite_sequence` (predicción).
- Las conversaciones solo crean `sessions`, `turns` y `notes` (`internal/conversation/sqlite/sqlite.go:233-280`), también `WITHOUT ROWID`. Ningún residuo real queda rechazado por «otra entrada con nombre del almacén».
- `grep VACUUM` en el código de producción no devuelve nada.

Huecos (R5-9): la cláusula «en ese orden» no tiene molde, y la definición no dice si «entradas del almacén» se cuentan por `name` o por `tbl_name`.

## 3 · (c) La regla «sin código SQLite»

Errores sin código en los caminos que el plan clasifica:
- **Migración.** `*TombstoneFault` (`store.go:1219-1221`) y el `Scan` de la copia (`:1207`) son forma. Coherente con E48.
- **Siembra y aperturas.** Los errores del SO de `MkdirAll` (`store.go:1473-1475`) van a la fila Entorno solo si son `EACCES` o `ENOSPC`. Para `ENOTDIR` o `EROFS` la tabla no decide.
- **Lectura.** `ENOENT` del `Stat` pasa a Ausencia (NUEVO). `TestLedgerCheck_missingStoreFailsHonestWithoutCreatingIt` (`internal/cli/ledger_test.go:162-183`) exige «action/sqlite: read-only open» en stderr (`:176`), y §9 no lo nombra en ninguna de sus dos listas. Se mantiene solo si el error nuevo conserva ese prefijo (predicción).
- **Aprobaciones.** Sus centinelas de dominio van a determinista «sin cambio». Casa con `approvals_adapter_test.go:170-182` y `:684-705`.
- **`Scan` fijado.** El `Scan` que fija `TestClaim_aCorruptHistoryRowIsCorruptEvidence` (`claim_classification_v0151_test.go:47`) va a determinista. Casa.
- **Veredictos del almacén.** Aquí está el conflicto con tests aprobados (R5-3).
- **`context`.** Sale «tal cual» y ninguna superficie le da nombre de clase.

## 4 · (d) `expected_owner` obligatorio

Busqué con `grep -rn "adopt-ledger" … --include=*_test.go --include=*.test.tsx` y `grep -rn "\.AdoptLedger(" --include=*_test.go`. Rompe tests y dobles aprobados que §9 no declara. El detalle está en R5-1. No hay e2e que toque la puerta: busqué `adopt|Adoptar|whats-happening` en todos los `*.spec.ts` y no hay nada. El test jsdom de «Adoptar libro» no se rompe, porque usa `toMatchObject`, pero por eso mismo no vigila nada (R5-10).

## 5 · (e) Alcance de los seams

- `StandingFaultForTest` se alcanza desde `internal/app` e `internal/cli`, porque está en el mismo módulo y dentro de `internal/`. Es global de proceso; el riesgo de paralelismo está en R5-5.
- La «opción de test de la app para el gancho» no se alcanza como está escrita. `hookShapeFault` y `poolLifetimeForTest` son variables del paquete sin exportar (`ledger_identity.go:660`, `:669`), y aunque se exportaran, el gancho no corre en una app en marcha (R5-4).

## 6 · Las marcas [veredicto]

`grep -o "\[veredicto\]" | wc -l` da 20, y `grep -c` da 19 líneas; la 299 lleva dos marcas. Tres solo usan la definición (líneas 10, 324 y 361). Las otras 17 citan fichero:línea y las 17 casan en el WT:
- `cli/receipt.go:447` (candado) y `:463` (`OpenOperatorFor`);
- `controlapi/whats_happening.go:287-292`;
- `cli/serve.go:110` y `shell/controller.go:261` (`WriteConfigAtomic`);
- `app/identity.go:433-434`;
- `cli/authority.go:104,162,211,252`;
- `v0.16.2.md` 21, 24, 29, 103-104, 112-115 y 177;
- `app/profile_standing_test.go:209-212`, `:223-226` y `:441-442`;
- `act_external_fake_test.go:52`;
- `whats_happening_contract_test.go:365-400` (17, 7, 10 y 6);
- `approval_read_class_test.go:199-227`;
- `TestClaim_aCorruptHistoryRowIsCorruptEvidence`;
- `cli/ledger_identity_test.go:84` y `ledger_binary_test.go:64`;
- `shell/bootstrap_test.go:292-293` y `config_act_registry_test.go:512-513` y `:900-901`.

La de la línea 298 casa, pero el cambio que declara está incompleto (R5-1).

## 7 · Hallazgos nuevos

**R5-1 · P2 · [TEST-APROBADO-NO-DECLARADO] · §4 «Marcado o adopción», D6, G-E11 y §9.** Con el campo obligatorio se rompen tests y dobles aprobados que §9 no nombra.

- `internal/controlapi/ledger_standing_test.go`:
  - `TestAdopt_theDoorAdoptsAndAnswersTheReceipt`: POST `{"confirm":true}` en `:72`, exige 200 en `:73`.
  - `TestAdopt_aRefusedAdoptionAnswersByName`: dos subtests, POST en `:134`, exigen 503 `no_ledger` y 503 `act_not_recorded`.
  - `TestAdopt_needsConfirmation`: POST `{}` en `:105`, exige 428. La v6 no dice si el 400 va antes o después de la confirmación, que hoy se comprueba primero (`whats_happening.go:651-656`).
- El doble del paquete, `internal/controlapi/act_fake_test.go:81`, `fakeActs.AdoptLedger(context.Context)`, implementa la misma interfaz `ActRecorder` (`act.go:161`). Si la interfaz cambia, el paquete de tests no compila. §9 solo declara el doble externo.
- `internal/app/profile_standing_test.go:439` envía `{"confirm":true}`. Sin el campo saldría 400 antes de la transacción, nunca el 503 `ledger_unreadable` que declara §9.
- Comparar «dentro de `beginAdoption`» obliga a pasar el dueño esperado al `AdoptLedger` del almacén. Hay 16 llamadas en tests de `internal/action/sqlite`: `ledger_identity_test.go` (8), `profile_standing_test.go` (6), `ledger_hook_test.go` (1) y `ledger_shape_test.go` (1). O cambian de firma, o queda una puerta sin comparación.

Reproducción (predicción por lectura):
1. Implementar §4 y D6 tal como están.
2. `go test ./internal/controlapi -run TestAdopt_` da 400 en `:73` y en los dos subtests de `:134`.
3. Cambiar `ActRecorder.AdoptLedger` sin tocar `act_fake_test.go:81` deja el paquete sin compilar.

Por qué P2: es la clase y el criterio de R4-1, y nace de su cura.

**R5-2 · P2 · [AFIRMACIÓN-FALSA][SUPERFICIE-SIN-FILA] · G-E10, regla «Activar almacén» de §5-bis, E27.** La v6 dice: «esta puerta solo puede ver entorno, nunca forma, determinista ni del momento». Es falso.

- Tras `openFresh`, `CreateLedger` devuelve errores sin centinela: raíz, clave, identidad y registro (`config_act.go:421`, `:425`, `:430`, `:433`) y el sellado de tx1 (`:439-441`).
- La puerta los manda a `act_not_recorded` 503 («el libro se creó en … pero su primer acto no se pudo registrar», `whats_happening.go:578-583`).
- «Activar almacén» no toma candado (`config_act.go:392` y `:404`; §3 lo dice), así que otro proceso con la misma ruta, como el D de E43, puede abrir el fichero después del `O_EXCL`.
- G-E10 promete que entorno y del momento nunca caen en `act_not_recorded`, y E27 solo ataca un directorio sin permiso de escritura.

Reproducción (predicción por lectura):
1. Perfil sin almacén. P pulsa «Activar almacén» y queda retenido en el seam (1) de E37, tras el `O_EXCL`.
2. Un segundo proceso abre la ruta y ejecuta `PRAGMA locking_mode=EXCLUSIVE; BEGIN IMMEDIATE;` sin soltar.
3. Se suelta P:
   - Si el bloqueo cae en la siembra, sale `ledger_not_created`: un fallo del momento con el texto de entorno. Además, `os.Remove(path)` borra el fichero que tiene abierto el otro proceso (`config_act.go:404-411`, preexistente).
   - Si cae en `ensureRootIntent` o en tx1 (BUSY, o FULL con disco lleno), sale `act_not_recorded` 503.

Por qué P2: G-E10 es falsa en una superficie de §5-bis, la frase que la justifica es falsa y no hay fila. Es el criterio de R3-4.

**R5-3 · P2 · [TAXONOMÍA][ADJUDICACIÓN-NO-CIERRA R4-2] · §5 filas «Sin código» y «Determinista», G-E3, §5-bis, §9.** Al pie de la letra, «Un error de dominio con centinela propio conserva su clase (fila determinista)» incluye los veredictos del propio almacén.

- Esos veredictos nacen sin código y en lecturas: `profile_standing.go:175` (`ErrLedgerUnreadable` con la razón de la forma), `:184`, `:213` y `:215` (`ErrLedgerMarkMalformed`).
- §5-bis dice que la clase determinista «una lectura no la produce», y su columna de `ledger check` es «—».
- Dos tests aprobados los fijan como `ledger standing: ledger_unreadable`:
  - `TestLedgerCheck_namesAMalformedReceiptMarkAfterTheMigration` (`internal/cli/ledger_identity_test.go:64`, aserción en `:84`);
  - `ledger_binary_test.go:64` (`DROP TABLE actions`, que produce un `ErrLedgerUnreadable` sin código).
- §9 dice que no cambian «para la forma», pero el criterio de la fila Forma (códigos 1, 11, 24 y 26, y pasos de migración) no incluye esos veredictos.
- La última cláusula («solo en el gancho y en las aperturas de conexión») deja sin clase cualquier otro error sin código, así que G-E3 («toda falla se clasifica») es falsa tal como está redactada.
- `ErrSchemaBehind`, `ErrSchemaFromTheFuture` y `ErrNoActionStore` no tienen fila de clase, y Ausencia no tiene fila en §5-bis. Choca con «Detrás va siempre el nombre de la clase» del arranque, frente al «como hoy» de G-E4.

Reproducción (predicción por lectura):
1. Implementar `classifySQLite` con el texto literal de §5.
2. `go test ./internal/cli -run TestLedgerCheck_namesAMalformedReceiptMarkAfterTheMigration` da rojo en `:84`, porque determinista no tiene nombre en `ledger check`.
3. Lo mismo con el test binario de `ledger_binary_test.go:64`.

Por qué P2: es la regla que exigía R4-2, contradice §5-bis, E11, E17 y dos tests aprobados, y no queda cerrada.

**R5-4 · P3 · [ORÁCULO][NIVEL-DE-EVIDENCIA] · E27 (fallo sin código del gancho), §11 «opción de test de la app».**
- El gancho solo corre cuando el pool crea una conexión (`ledger_identity.go:442-449`, `:464-468`).
- El handle de la app mantiene una sola conexión toda su vida: `SetMaxOpenConns(1)` (`store.go:1485`), y el tiempo de vida solo lo cambia `poolLifetimeForTest` (`:1486-1488`). El godoc de `ledger_identity.go:656-660` lo dice: «the pool keeps its connection for the handle's life».
- Las cinco puertas, `/api/config` y aprobaciones escriben por ese handle. Si el fallo salta, salta en el `Build` de la app nueva durante el corte, y la puerta responde el desenlace de la recarga, no `ledger_busy`.
- Hace falta un segundo global exportado, o un tercero, que la v6 no declara (§6 y §12 solo declaran `StandingFaultForTest`).

Reproducción (predicción):
1. App real con libro sano; activar la opción de test.
2. `POST enable-approvals {"confirm":true}`.
3. tx1 va por la conexión viva, el gancho no corre y la respuesta no es `ledger_busy`.

**R5-5 · P3 · [ALCANCE] · §6 (preámbulo), §12.**
- `StandingFaultForTest` es global de proceso. `Build` lo atraviesa dos veces (`ledger_identity.go:91`, `app.go:409`) y `ledger check` una (`ledger.go:150`).
- Hay 80 llamadas a `t.Parallel()` en 24 ficheros de `internal/app` y 107 en 28 de `internal/cli` (`grep -h "t.Parallel()" … | wc -l`).
- El precedente citado (`poolLifetimeForTest`, `hookShapeFault`, `openStandingSeam`) no está exportado, y sus únicos setters son tests secuenciales con `Cleanup` (`ledger_shape_test.go:197`, `:352`, `:486-496`).
- La v6 no exige que E13-A, E26-A y E47-R sean secuenciales. Predicción: si uno de ellos es paralelo, otro test paralelo del mismo paquete verá `ledger_environment`, según cómo se repartan las goroutines.

**R5-6 · P3 · [TAXONOMÍA] · §5 «Del momento».** 18 TOOBIG, 21 MISUSE, 25 RANGE y 12 NOTFOUND fallan igual siempre sobre la misma sentencia, que es la definición que da el plan de determinista. Sin embargo, van a `ErrLedgerTransient`, con D3 («vuelve a abrir esta pantalla»). E15 no ataca esa colocación.

**R5-7 · P3 · [AFIRMACIÓN-FALSA] · G-E7 y fila Forma («1 ERROR, todo el SQL de producción es constante»).**
- Un trigger persistente ejecuta su cuerpo dentro de nuestra sentencia. Por ejemplo, `CREATE TRIGGER t BEFORE INSERT ON actions BEGIN INSERT INTO nope VALUES (1); END;` hace que el siguiente INSERT falle con «no such table» y código 1.
- Eso lo clasifica como forma: `ledger_unreadable`, pegajoso en la conexión. E11-A razona igual («el INSERT falla con 1 → forma»).
- G-E7 dice que un trigger que no imita a la guarda «es de la clase determinista».

Reproducción (predicción):
1. v16 fundado y app en marcha.
2. Crear ese trigger con un escritor manual.
3. `POST enable-approvals` da 503 `ledger_unreadable`, no `act_not_recorded` con el detalle de la decisión UX 6.

**R5-8 · P3 · [TAXONOMÍA] · §5 «La guarda» → «del momento»; §12.**
- La fila de la guarda nunca se recrea: lo dice `ledger_identity.go:251-253`, y el `UPDATE … WHERE` de `:268` toca 0 filas sin dar error.
- La conexión vive lo que vive el handle (`:656-660`), así que un reintento repite el mismo rechazo hasta que se reinicia la app, y D3 sería falso en cada intento.
- La predicción de §12 («se arregla con una conexión nueva») no tiene camino en producción.

**R5-9 · P3 · [MUTACIÓN-SOBREVIVE][ORÁCULO] · §4 «Residuo», E02-A.**
- «en ese orden» no tiene molde. A(4) es un conjunto que no es prefijo y enrojece con o sin comparar el orden, así que una mutación que ignore el orden sobrevive. Su único efecto real sería un rechazo falso: tras un `VACUUM` manual, las tablas pasan delante de los índices (T,T,I,I,T se vuelve T,T,T,I,I) y el residuo se juzgaría forma mala (predicción).
- No está claro si las «entradas del almacén» se cuentan por `name` o por `tbl_name`. Un índice `u` UNIQUE o un trigger `x` sobre `actions` dentro de un residuo pasa como residuo con la primera lectura y es forma mala con la segunda, y ninguna fila de E02-A lo decide.

**R5-10 · P3 · [PLAN-FILA-AUSENTE][MUTACIÓN-SOBREVIVE] · G-E11, D6, §11-Q4, E38.**
- E38 vigila «Marcar». El único test de «Adoptar libro» en la ventana exige `expect(posts[0].body).toMatchObject({ confirm: true })` (`WhatsHappening.test.tsx:778`), y §9 no lo nombra.
- Sobrevive la mutación «la ventana no envía `expected_owner` al adoptar». En producción, cada clic respondería 400 y la única salida de un libro ajeno quedaría muerta con la suite en verde (predicción).

**R5-11 · P3 · [NIVEL-DE-EVIDENCIA] · §8.** No tiene categoría «en proceso» ni «conexiones reales en proceso». Por eso faltan E03, E04, E05, E06, E09, E10, E16, E22, E30, E31, E15-A y E39-R.

## 8 · Lo que la v6 hace bien, verificado

- **Orden pre-RED.** `grep -rln -E "classifySQLite|ErrLedgerEnvironment|…|expected_owner|StandingFaultForTest|seedSeam|…|TestLedgerRestoreProcedure_byBinary" internal cmd docs` sale con 1 y no devuelve nada. Tampoco hay `docs/operations/*restore*`.
- **Gate de §1** verificado: `/tmp/q26.txt` y el `find -newer` de la sección 1.
- **Premisa de E48** verificada: tipo de la columna, juez, fault y texto.
- **Oráculo de E45** completo sobre las tablas persistentes.
- **Citas:** las 17 marcas [veredicto] con cita casan.
- **`createStmt`** es constante desde v0.11.0, no crea objetos implícitos y sus nombres no chocan con los de las conversaciones.
- **Otras citas de §3** que comprobé: `profilelock.go:34`, `store.go:1597` y `:1610`, `ledger_shape.go:165-167`, `firstrun_template.json:26` y `authority_as11_test.go:27-30`.

## 9 · Alcance

- **Leído:** el plan v6 entero, el veredicto de la ronda 4 entero, los hallazgos de la ronda 3 y P3-e de la ronda 2, y `git diff -- CLAUDE.md`. En el WT:
  - `profile_standing.go`;
  - `ledger_identity.go`;
  - `cli/ledger.go`;
  - por tramos: `store.go`, `app.go`, `controlapi/whats_happening.go`, `config_act.go`, `config_act_registry.go`, `approvals_v15.go`, `approvals_adapter.go`, `identity_v2.go`, `intent.go`, `supervisor.go`, `conversation/sqlite/sqlite.go`, `docs/operations/tombstone-manual-repair.md` y `v0.16.2.md`;
  - los tests citados arriba.
- **Ejecutado:** solo comandos de lectura: `date`, `wc`, `shasum`, `stat`, `git` (rev-parse, diff, status, show, tag, log), `grep`, `sed`, `awk`, `find`, `ls` y `cmp`. Ningún test, ninguna compilación, ninguna mutación, ningún fichero auxiliar.
- **No verificado (predicción):** todo desenlace dinámico. Eso incluye los rojos de R5-1 y R5-3, la BUSY de R5-2, R5-4, R5-7, el orden tras `VACUUM`, la ausencia de autoindex en las tablas `WITHOUT ROWID` y el comportamiento de EXCLUSIVE frente a WAL.
- **Sin comprobar:** la versión del perfil real de E46. Que sea «de dos versiones atrás», y no un v16 ya tocado por una build de desarrollo, depende de lo que diga el canto.
- **Sin examinar, por la orden:** la instantánea, la ficha UX, la maqueta y el fichero de decisiones.
- **Tiempo:** de 19:03 a 19:25.
- **Lo que esta ronda vio y la anterior no:** R5-1 a R5-11. R5-1, R5-2, R5-3, R5-8 y R5-10 nacen de curas de la v6.

## 10 · Integridad (19:25:06)

- `git -C $WT diff | cmp - …/adv-e5-before.patch` → «DIFF: identical».
- `git -C $WT status --short | grep '^??' | sort | cmp - …/adv-e5-untracked-before.txt` → «UNTRACKED: identical».
- `git -C $WT diff --cached --quiet` → sale con 0.
- Plan: 417 líneas, sha256 `fe055ae857578cee117c6f9dcd032921698d6d0f4470edd6ff0c01c346f0d909`.