VETO MANTENIDO

Nota sobre el objeto. El plan con hash `461fc265ef4717a5…` tiene 188 líneas, no 98, y sus secciones van de §1 a §12. Su §5 es «Taxonomía exacta de errores» y su §7 «Clases obligatorias, una por una», con 16 filas. No tiene una sección de «ocho categorías» ni otra de «textos públicos»: los textos están en E34. Cubrí §5, §7 y E34.

Las tres frases que cita la orden no aparecen en el plan (`grep -n` → exit 1): «no hay otra pareja de escrituras en el camino de este tren», «FAILED/OUTCOME_UNKNOWN con causa» y «OUTCOME_UNKNOWN o SUCCEEDED según lo que se pueda probar». Ataqué lo que sí dice el plan: la tabla de §4 y la línea 169 (§10-P4), «La atomicidad cubre las cinco escrituras del camino».

## Hallazgos

**H1 · P2 · [PLAN-FILA-AUSENTE][AFIRMACIÓN-FALSA] · §4, §10-P4, G-E2, E04**
La tinta se escribe en el camino de arranque y no tiene fila.
- `internal/app/signing.go:87`: `os.WriteFile(keyPath, action.EncodeSigningKeySeed(priv), 0o600)`. La primera generación va sin temporal, sin Sync y sin O_EXCL. Si el fichero queda a medias, el arranque muere para siempre en `signing.go:67`, que devuelve «refusing to regenerate».
- `signing.go:139-141`: la rotación documenta su propio estado intermedio, que no es benigno: «A crash between the two leaves the OLD file against the NEW registry — a state ensureSigningKey refuses closed». G-E2 exige «benigno y enumerado» y el plan no lo enumera.
- Las puertas de escritura de la CLI pasan por `openOperatorStoreSealed` (`internal/cli/intent.go:111-128`: OpenOperatorFor más EnsureSigningKey). Se usa en `approvals.go:224,316`, `grant.go:123,224,302` e `intent.go:230,261,385,457,660,706`. Ninguna toma el candado del perfil; solo lo toma `receipt.go:447`.
- Reproducción, predicción por lectura:
  1. Primer arranque de la app y `korvun intent create` a la vez.
  2. Cada proceso ejecuta Stat → GenerateKey → WriteFile, sin O_EXCL.
  3. El último WriteFile gana el fichero y el registro conserva la primera clave.
  4. El siguiente arranque muere en `signing.go:118` («ink identity conflict; refusing to boot»). La clave privada pisada se pierde.
- Por qué P2: G-E2 se lee literal («Ninguna escritura propia del producto») y la línea 169 del plan es falsa frente al árbol.

**H2 · P2 · [PLAN-FILA-AUSENTE] · G-E2, G-E8, E19, E21**
La puerta «activar almacén» (CreateLedger) no tiene ninguna fila de crash.
- Reproducción, predicción por lectura:
  1. En un perfil sin storage, `config_act.go:392` crea el fichero con O_EXCL.
  2. `config_act.go:400` hace `rememberCreated`, que es memoria del proceso (`config_act_registry.go:131`: `created: map[string]os.FileInfo{}`).
  3. openFresh siembra, y después se escriben la raíz, la tinta y el registro de identidad.
  4. `config_act.go:431-445` sella el acto AUTHORIZED y guarda al fundador en memoria.
- Crash antes de persistir el perfil: el proceso nuevo no tiene `createdHere` y la puerta responde ErrLedgerExists (`whats_happening.go:565-570`).
- Crash después de persistir y antes de tx2:
  - La recuperación cierra el acto fundacional OUTCOME_UNKNOWN (`store.go:1602-1612`).
  - Solo tres sitios escriben `INSERT INTO ledger_identity`: `profile_standing.go:243` (migración), `:279` (FinishFounding) y `:337` (AdoptLedger).
  - FinishFounding solo corre si hay fundador (`config_act.go:217-218`).
  - Resultado: el libro queda legacy_unfounded para siempre y la pantalla dice «Libro anterior a esta versión», sin botón (`WhatsHappening.tsx:560-562`; `WhatsHappening.test.tsx:788-803`).
- E19 solo cubre un crash dentro de la transacción de FinishFounding. E21 se pondría verde, porque el acto sí queda OUTCOME_UNKNOWN, aunque el libro pierda su marca.

**H3 · P2 · [AFIRMACIÓN-FALSA] · E04, E29, §7 «Conexiones reales múltiples»**
Dos apps no pueden sembrar el mismo fichero, y el primer arranque no funda nada.
- `app.go:355` llama a `AcquireProfileLock(filepath.Dir(storagePath(cfg)))` antes de OpenFor (`app.go:380`). El candado es `unix.LOCK_EX|unix.LOCK_NB` (`profilelock_unix.go:18`), así que la segunda app muere con ErrProfileLocked antes de tocar el libro.
- Build no funda: los únicos escritores son los tres INSERT citados en H2.
- Consecuencia: E29 («dos apps de DOS perfiles… primer arranque simultáneo… uno funda») describe un camino que no existe, y la actora «otra app» de E04 nunca llega a la siembra.
- Los otros sembradores reales no tienen fila:
  - `openOperatorWithIdentity` siembra cuando el fichero no existe (`store.go:1429-1443`, «keeps the intent-create-before-first-boot flow alive»).
  - `CreateLedger` siembra con openFresh, que es OpenFor (`config_act.go:317,404`).

**H4 · P2 · [TEXTO-PÚBLICO-FALSO][SUPERFICIE-SIN-FILA] · E34, G-E10**
El primer arranque del escritorio crea un libro que la pantalla llama «anterior a esta versión».
- `internal/shell/firstrun_template.json:26` incluye `"storage"`. Build siembra y no funda (H3), y `judgeWithoutRow` devuelve legacy_unfounded (`profile_standing.go:200-209`).
- `WhatsHappening.tsx:560-562` (sin trackear, código del tren D) dice: «El libro de acciones no lleva marca de perfil: es anterior a esta versión.»
- Lo apoya `TestMount_aLedgerWithNoMarkIsLegacy` (`internal/app/profile_standing_test.go:235-250`): arranca sobre un almacén recién preparado y obtiene legacy_unfounded.
- Es predicción por lectura: no ejecuté un primer arranque. E34 no lista esta frase.

**H5 · P2 · [INCOHERENCIA] · §5 frente a E08-A y E11-A**
Según la tabla de §5 (línea 76), un código 1 fuera de las consultas de forma es «del momento» y acaba en ErrLedgerTransient («the ledger did not answer»). Dos filas esperan otra cosa:
- E08-A (línea 96) espera «el paso falla con código 1 dentro de su transacción → forma».
- E11-A (línea 99) renombra una columna de receipts en un libro fundado:
  - `judgeOnConn` solo lee receipts en `case "0":` (`ledger_identity.go:572`). Con fila de identidad (`case "1":`, línea 560) nunca la toca.
  - El primer error sale de un INSERT, así que por la tabla sería «del momento».
- Ataque sin fila (dependencia mentirosa), predicción por lectura:
  1. `UPDATE action_schema SET version = 12` sobre un libro v16.
  2. El juicio lo ve shapeOlder (`ledger_shape.go:174-178`).
  3. openWithIdentity migra sin mirar el dueño (`store.go:1515`).
  4. El paso 12 choca con objetos que ya existen (código 1) y la pantalla diría «no se pudo comprobar ahora» para siempre.
- G-E3 promete clasificar «por el código primario», pero para el código 1 la tabla decide por sitio.

**H6 · P2 · [MUTACIÓN-SOBREVIVE][TAXONOMÍA] · §5, E15, §9**
La tabla cerrada de 31 códigos no casa con el driver ni con los clasificadores ya aprobados.
- a) Códigos extendidos:
  - modernc activa los códigos extendidos (`conn.go:113`, `c.extendedResultCodes(true)`), y `Code()` devuelve el rc tal cual (`error.go`: `func (e *Error) Code() int { return e.code }`; `conn.go:876-891`).
  - `mapGuardError` enmascara (`ledger_identity.go:356`, `switch code & 0xff`), pero el plan nunca nombra la máscara.
  - E15-R prueba con 31 códigos primarios, y E15-A con errores reales que casi siempre son primarios (5, 8, 14, 26).
  - Por eso la mutación «clasificar Code() sin & 0xff» sobrevive a los dos. Códigos como 517, 261, 266 o 1032 se quedarían sin clase.
- b) Errores sin código SQLite:
  - El driver devuelve `ctx.Err()` (`stmt.go:102,111`).
  - El molde aprobado del hook inyecta `errors.New("injected: …")` (`ledger_shape_test.go:197`).
  - «Sin clase por defecto» (línea 70) no dice qué pasa con estos errores. §9 (línea 158) conserva ese test «para BUSY y los códigos del momento», aunque el error que inyecta no lleva código.
- c) Un 1811 sin el mensaje `ledger_guard:` no cae en ninguna fila: la línea 77 exige el mensaje y la 78 habla de «otro extendido». El test aprobado `TestClaim_aDeterministicPurgeFailureIsCorruptionNotWeather` (`approval_read_class_test.go:133,160-176`) exige ErrApprovalEvidenceCorrupt justo para ese caso.
- d) ABORT (4) va a «del momento» y MISMATCH (20) a «forma, pegajoso». `purgeWriteFailure` (`approvals_v15.go:452-463`) los clasifica como evidencia corrupta de esa aprobación, y §9 no declara el cambio.

**H7 · P2 · [AFIRMACIÓN-FALSA][ORÁCULO] · G-E9, E23**
«Los lectores nunca escriben» contradice evidencia ejecutada que ya está en el árbol.
- El godoc de `store.go:2042-2069` recoge lo siguiente:
  - journal_mode(WAL) se aplica antes que query_only.
  - «Executed — sha256 … DIFFERENT for one left in journal_mode=delete».
  - La apertura crea `-wal` y `-shm`.
  - Una ruta borrada entre os.Stat y la conexión «IS created, empty».
- El oráculo de E23 («sqlite_master idéntico») no ve ni la cabecera ni esos ficheros. Falla el punto 5 de la doctrina y el punto 3 de la ley de verificación cruzada.

**H8 · P2 · [MUTACIÓN-SOBREVIVE] · E12, E26**
El mecanismo previsto no fuerza BUSY en las lecturas.
- El DSN lleva `journal_mode(WAL)&busy_timeout(5000)` (`store.go:91`).
- El driver aplica busy_timeout primero (modernc `sqlite.go:436-441`) y ejecuta el hook después (`driver.go:266-269`).
- Un lock breve lo absorbe busy_timeout. En WAL, un BEGIN EXCLUSIVE de otro proceso no bloquea a los lectores; esto es predicción por lectura de la semántica de SQLite, no ejecutada.
- «breve: ninguno» y «la conexión se rechaza y el pool abre otra» se excluyen: database/sql solo reintenta `driver.ErrBadConn` (`$GOROOT/src/database/sql/sql.go:1574-1583`).
- La mutación «BUSY clasificado forma» sobrevive a E12-R y a E26-R.

**H9 · P2 · [ORÁCULO][NIVEL-DE-EVIDENCIA] · E05, E13, E12-A, D3**
En el arranque, el clasificador del libro nunca ve los fallos de entorno, y no está definido qué hace Build con las clases nuevas.
- Antes del libro, Build abre las conversaciones sobre el mismo fichero (`app.go:341`; texto crudo en `app.go:886-894`) y el candado en el mismo directorio (`app.go:355`; `profilelock.go:37-40`, con O_CREATE). Con un directorio sin permiso, o con el disco lleno, muere ahí. Solo NOTADB tiene fila (E14).
- Build trata como fatal todo lo que no es un veredicto (`app.go:410-414`) y cualquier fallo de OpenFor (`app.go:383`). El texto D3 de entorno da por hecho que la app sigue viva, y E05, E12-A y E13 no dicen si arranca bloqueada o muere.
- chmod: la CI corre `go test` en windows-latest (`quality.yml:20,198-202`), y el repo ya salta estos moldes en Windows y como root (`config_act_registry_test.go:494`). E13 no lo declara.

**H10 · P2 · [PLAN-FILA-AUSENTE][AFIRMACIÓN-FALSA] · E28, E21, §3.8, §10-P4**
La cura de E28 crea una narración falsa, y la misma clase tiene una segunda puerta.
- El supervisor persiste después del corte (`supervisor.go:293-294`). Si la persistencia falla, StatePersistFailed cuenta como aplicado (`whats_happening.go:719`) y la pantalla dice «al reiniciar volverá atrás» (`:738`).
- El plan propone un Sync del directorio después del rename (líneas 52 y 116). Si ese Sync falla, el perfil ya está reemplazado y la pantalla anunciaría una vuelta atrás que no ocurre. E28-A no distingue los dos Sync.
- `writeRawConfigAtomic` (`internal/shell/upgrade.go:88-110`) es otro escritor temporal+rename sin Sync y no está en el plan. Por eso es falso el «No» de §10-P4, línea 169.
- §3.8 omite que el corte ocurre antes de persistir, y E21 no prueba el punto «tras el corte, antes de persistir».

**H11 · P2 · [EITHER/OR] · E04, E13, E20, E21, E29**
Estos desenlaces aceptan dos resultados donde el punto de interrupción forzado solo permite uno:
- E04 (línea 92): «el segundo espera o ve el libro sembrado».
- E13 (101): «(READONLY / CANTOPEN / IOERR)», sin decir qué escenario da cada código.
- E20 (108): «o las dos cosas o ninguna», con el crash fijo dentro de la transacción; lo único posible es «ninguna».
- E21 (109): «el perfil es el viejo o el nuevo», con dos puntos fijos; es viejo en el primero y nuevo en el segundo.
- E29 (117): «uno funda», sin decir cuál.

**H12 · P2 · [ORÁCULO] · E06-R**
«Las 80 aperturas del auditor» (línea 94) es una carrera estadística. No hay costura entre la lectura de versión fuera de la transacción (`store.go:715`) y el BEGIN del paso (`store.go:1247`). Choca con el punto 3 de la doctrina, y la mutación puede sobrevivir a una ejecución.

**H13 · P2 · [MÁS-ANCHO-QUE-SU-CABLE] · G-E7, E09**
G-E7 (línea 26) dice «Un trigger ajeno no bloquea», pero E09 solo prueba un trigger ON sessions.
- Un trigger ajeno ON receipts con RAISE(ABORT) bloquea todas las escrituras.
- La guarda se reconoce por texto (`ledger_identity.go:361-373`, y el plan lo conserva en la línea 77). Un trigger ajeno que lance `'ledger_guard:ledger_foreign_profile'` se leería como veredicto de la guarda (clase g).
- Es predicción por lectura.

**H14 · P2 · [PLAN-FILA-AUSENTE] · G-E10, E27**
G-E10 nombra la CLI, pero solo `ledger check` tiene filas. Las puertas de escritura de H1 y rotate-key (`receipt.go:447-488`) no tienen fila ante un libro ilegible, un entorno roto o un lock largo. Lo que imprimen hoy es predicción por lectura.

**P3**
1. §5, línea 76: «el pool reintenta» es falso. `sql.go:1574-1583` solo reintenta ErrBadConn, y `driver.go:266-269` cierra la conexión y devuelve el error.
2. §8 dice «matado», pero el precedente (`authority_as11_test.go:27-30`) aclara «not a signal sent by the parent»: el hijo sale con os.Exit.
3. E05 dice que un FULL real no se puede fabricar. Con `PRAGMA max_page_count` sí se puede (predicción).
4. E01-A:
   - El driver ejecuta createStmt como un único guion dentro de un Exec (`stmt.go:41-56`). Una costura entre sentencias obliga a partirlo en producción, y §4 no lo declara.
   - A corre con el código nuevo, así que no puede verificar cómo confirmaba cada sentencia v0.16.1.
5. Hay pines sin etiqueta: E03 (`ledger_shape.go:165-167` ya da forma mala hoy), E21 (`recovery_authorized_test.go:34`) y E31 (`store.go:1787-1789`).
6. E26 dice «app real + jsdom», pero jsdom come un fixture escrito a mano (`WhatsHappening.test.tsx:25,842`).
7. E24-A: si el aviso es texto fijo, el test pasa igual con restauración en caliente que sin ella (clase d).
8. E09: la mutación «solo cuentan tablas y vistas» sobrevive, porque ninguna fila prueba un índice homónimo (`ledger_shape_test.go:437`).
9. E14: el texto que espera de la CLI exige un cambio en el abridor que §4 no declara, y hoy `ledger.go:85-88` imprime el error crudo (predicción).
10. §3.4 dice «solo se fija por UPDATE», pero el hook hace DELETE e INSERT (`ledger_identity.go:474-476`).
11. D3 añade estados visibles y no menciona el diseño UX-TEMPLATE que exige la sexta ley.
12. G-E6 choca con busy_timeout(5000): un paso de migración de más de 5 s da BUSY al segundo abridor (predicción).

## Lo que el plan hace bien (verificado)

- Los datos de §1 son correctos: HEAD d20dea66… coincide con origin/master, la rama no tiene upstream, y hay 135 ficheros modificados y 57 sin trackear.
- Todas las citas de §3 casan con el árbol, línea a línea.
- createStmt tiene cinco sentencias y su texto es idéntico en los 9 tags que lo contienen y en el worktree (hash `4790eb968e06`). Congelarlo desde v0.16.1 es representativo.
- Las cifras son correctas: 34 tablas (`ledger_identity.go:674-684`) y 24 outcomes de aprobación (`grep -c` da 24).
- Los tests y costuras que nombra existen; `beforeMigrationCommit` no existe, y el plan la marca como NUEVO.
- Estos diagnósticos se confirman por lectura:
  - `TestOpen_seedFailureIsBootFatal` pasa por la forma mala, no por la semilla.
  - E18: `app.go:430` reasigna `err` antes de las líneas 451, 474 y 493.
  - E11 es un rojo real hoy.
- El NO VERIFICADO de §9 sobre `TestShape_aHomonymousObjectIsNotAFreshFile` se resuelve leyendo: el test solo cubre una tabla con otras mayúsculas y una vista, así que E09 no lo cambia.

## Alcance

- Leí el plan entero y, en el worktree, el código de producción y los tests citados arriba. Del driver modernc v1.59.0 leí error.go, conn.go, stmt.go, sqlite.go y driver.go, y de la stdlib database/sql.
- Comandos ejecutados: git, grep, sed, awk, shasum, wc y go env/version. No compilé, no ejecuté tests y no edité nada. No abrí `claude-code-report.md`: ninguna evidencia sale de él.
- Queda como predicción, sin verificar:
  - la semántica WAL frente a BEGIN EXCLUSIVE;
  - el FULL con max_page_count;
  - los códigos de chmod en cada sistema;
  - la carrera de la tinta;
  - la cadena de crash de CreateLedger;
  - el texto de la CLI ante NOTADB.
- Sin examinar:
  - internal/shell más allá de sus escritores;
  - el e2e-harness;
  - el coste de la derivación en memoria de E08 dentro del hook de cada conexión.

## Integridad

- `cmp /tmp/adv-e-after-start.patch /tmp/adv-e-before.patch`: idénticos.
- El diff final, guardado en el scratchpad fuera del árbol, es idéntico a `/tmp/adv-e-before.patch`.
- La lista de sin trackear es idéntica a `/tmp/adv-e-untracked-before.txt`.
- El `shasum -a 256` del plan empieza por `461fc265ef4717a5` al empezar y al terminar.
