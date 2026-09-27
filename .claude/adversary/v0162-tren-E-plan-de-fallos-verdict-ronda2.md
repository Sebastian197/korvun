VETO MANTENIDO

Objeto revisado: `/Users/sebastianmorenosaavedra/Desktop/korvun.nosync/design-drafts/2026-09-25-tren-E-plan-de-fallos-copiloto.md`, v3, 235 líneas. `shasum -a 256` → `209b37612c4a29a93bf3abc931c64ec9b5e6d2e102edbe6f8620d9909fb157f0` al empezar y al terminar, así que coincide con el que me dieron. La vara es `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync/.claude/adversary/v0162-tren-E-plan-de-fallos-verdict.md` (179 líneas, que leí enteras). El árbol es el worktree `korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `d20dea6` = `origin/master`.

Sobre el método: el prompt de sistema me pide ejecutar, pero la regla 1 de la orden prohíbe tests y compilación. Obedecí la orden: no ejecuté ningún test ni ninguna mutación. Lo que depende de ejecución va marcado como «predicción por lectura». Todo lo demás sale de leer el código en la línea citada o de un comando de solo lectura cuya salida copio.

## Las 26 adjudicaciones del §10

| Hallazgo | Adjudicación v3 | Veredicto | Evidencia |
|---|---|---|---|
| H1 | G-E12, §4, E35, E36 | CIERRA A MEDIAS | La carrera del fichero queda cubierta por el `link` exclusivo. La del registro de la clave pública no está forzada (N8: `signing.go:107,115,117,120`). La recuperación de `.new` no tiene dueño ni candado (N7). |
| H2 | E37 | **NO CIERRA** | La reproducción exacta de H2 es un crash con el acto ya sellado (`config_act.go:439`) y antes de persistir (`supervisor.go:293-294`). Por la regla de la propia E37 («si tiene actos → `ledger_exists`») termina en `ledger_exists`, igual que hoy (N10). |
| H3 | §3 corregido; E04 y E29 con la CLI | CIERRA A MEDIAS | §3 casa con `app.go:355`, `profilelock_unix.go:18` y `receipt.go:447`, y E29 es correcta. Pero E04-R no puede dar su desenlace (N4), y «Dos apps no pueden abrir el mismo libro» es falso vía CreateLedger (N5). |
| H4 | G-E11, E38, D4, §9 | CIERRA A MEDIAS | El defecto del primer arranque queda atacado. A cambio, G-E11 choca con FinishFounding en el camino normal (N1a), su letra también funda un libro sin fila con recibo marcado (N1b), y §9 omite `internal/shell/ledger_standing_test.go:48-49`. |
| H5 | «1 siempre forma»; E41 | CIERRA A MEDIAS | §5 ya es coherente, pero el mecanismo de E41 no existe en el árbol (N3). |
| H6 | `& 0xff`; clases determinista y sin código; E15 | CIERRA A MEDIAS | (a), (b) y (d) quedan cerradas en §5, y (c) también. Pero el test aprobado que fija (c) se rompe con E40 (N2), contra lo que dicen §9 («se conserva») y E15-A («intacto»). |
| H7 | G-E9 acotada; DSN lectora; sha256 | CIERRA A MEDIAS | Queda el checkpoint al cerrar con un WAL caliente (N11). |
| H8 | E12 con `locking_mode`; texto | CIERRA A MEDIAS | El texto queda corregido (línea 44 = `sql.go:1574-1583`). Forzar BUSY en lecturas WAL sigue siendo predicción, y E26 la hereda sin declararla (P3-l). |
| H9 | no arranca con nombre; E05, E13, E42; saltos | CIERRA A MEDIAS | Los saltos están declarados (§8). E13 (a) y (b) no pueden producir su desenlace (N6). |
| H10 | E28 con dos `Sync` + `writeRawConfigAtomic`; E21 con tres puntos | CIERRA | «Falla `Sync(dir)` → aplicado + aviso» elimina la narración falsa de `whats_happening.go:719,738`. El segundo escritor entra y E21 tiene el punto 2. Queda residuo P3 (P3-h). |
| H11 | un desenlace por punto | CIERRA A MEDIAS | E20, E21 y E29 quedan bien. E04 y E13 son exactas pero inalcanzables. E37(1) introduce una disyuntiva nueva de tres ramas (N10). |
| H12 | costura `afterMigrateReadVersion` | CIERRA | Es determinista. Hoy `migrateStep` (`store.go:1246-1272`) no relee la versión, así que la mutación enrojece. |
| H13 | G-E7 acotada; E40 | CIERRA A MEDIAS | La suplantación del mensaje queda cerrada, pero E40 rompe oráculos aprobados (N2). |
| H14 | E39 | CIERRA A MEDIAS | Faltan los cuatro verbos de escritura de `korvun authority` (P3-c). |
| P3-1 | texto | CIERRA | La línea 44 casa con `sql.go:1574-1583` y `driver.go:266-269`. |
| P3-2 | «sale con `os.Exit`» | CIERRA | La línea 84 casa con `authority_as11_test.go:27-30`. |
| P3-3 | E05 con `max_page_count` | CIERRA | Queda como predicción declarada. |
| P3-4 | E01-A retirado; E02 | CIERRA | — |
| P3-5 | etiquetas PIN | CIERRA A MEDIAS | E16 y E30-R son pines sin etiqueta (P3-d). |
| P3-6 | jsdom declarado | CIERRA | §8 |
| P3-7 | E24-A retirado | CIERRA | — |
| P3-8 | E09-A | CIERRA | — |
| P3-9 | §4 | CIERRA | — |
| P3-10 | §3 | CIERRA | `ledger_identity.go:474-476` |
| P3-11 | maqueta de D3 | CIERRA A MEDIAS | UX-TEMPLATE sigue sin nombrarse. D2, E42 y la retirada de la fila legacy (§9) son cambios visibles sin diseño (P3-f). |
| P3-12 | G-E6 acotada | CIERRA | — |

En total: 12 CIERRA, 13 CIERRA A MEDIAS y 1 NO CIERRA.

## Hallazgos nuevos

**N1 · P2 · [PLAN-FILA-AUSENTE][CONTRADICCIÓN] · G-E11, §4 «Fundación», D4, E19, E37(2), §9**

La fundación en el arranque choca con la fundación de «Activar almacén» en su camino normal, sin que haga falta ningún crash.

Cadena en el árbol:
- `whats_happening.go:564` llama a CreateLedger.
- `config_act.go:439` sella el acto con BeginConfigAct (AUTHORIZED) y `:445` hace `rememberFounder`.
- `whats_happening.go:607` pide la recarga con `RequestReload`.
- `supervisor.go:270` ejecuta `nApp, nerr := s.buildAndStart(ctx, req.cfg)`, que es `app.Build` (`shell/controller.go:243`; `korvun serve` hace lo mismo). Ese Build ocurre antes de persistir (`:293-294`) y antes del cierre (`whats_happening.go:625`).
- En ese Build el libro no tiene fila, y el diseño aprobado lo llama legacy a propósito: `profile_standing.go:20` dice «…and a ledger founded whose founding act has not closed yet».
- Con G-E11, ese Build funda. Después, el cierre llega a `config_act.go:217-218` (FinishFounding) y a `profile_standing.go:279`: un `INSERT INTO ledger_identity (id,…) VALUES (1,…)` sin ON CONFLICT contra `store.go:689`, `id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1)`.
- `config_act.go:224` relee el acto, lo encuentra AUTHORIZED, `:233` deja una nota y el cierre devuelve `ok=false`.

Reproducción (predicción por lectura):
1. Perfil sin almacén con la app en marcha. `POST /api/whats-happening/enable-storage`.
2. CreateLedger sella el acto fundacional y pide la recarga.
3. El Build del corte encuentra el libro sin fila y G-E11 lo funda: acto, recibo marcado y fila con id=1.
4. La persistencia sale bien, llega SettleAct y FinishFounding choca con la clave primaria. Hace rollback.
5. `SELECT state FROM actions WHERE action_id=<fundacional>` devuelve `AUTHORIZED`. El acto acabará en OUTCOME_UNKNOWN en la vida siguiente.

Test aprobado que se pone rojo: `internal/shell/ledger_standing_test.go:48-49` (`row.State != "SUCCEEDED"` → «the founding act is %q»). §9 no lo menciona.

Además:
- (b) La letra de G-E11, «Un libro sin fila de identidad lo funda», incluye el caso «sin fila + recibo marcado». El tren B llama ilegible a ese caso (`profile_standing.go:213`), y el test aprobado `TestMount_anUnreadableIdentityRefusesAdoption` exige que siga ilegible (`internal/app/profile_standing_test.go:339,371-373,436,445`: «the corruption was buried»). G-E4 dice que una forma mala nunca se repara, y E38 no tiene fila para el caso negativo.
- (c) La puerta que funda «en una transacción» no existe. FinishFounding cierra un acto sellado en otra transacción (`profile_standing.go:254-287`) y `founded_by_action` es `NOT NULL` (`store.go:691`). Hace falta un acto nuevo con su procedencia; `config_act.go:254` recuerda que atribuir un acto a quien no lo hizo es «a false statement about who did it». §11-Q2 no lo lista como NUEVO y D4 no decide quién firma ese acto.
- (d) E37(2) («el arranque siguiente funda») y E19 («antes: sin recibo ni fila») dan por hecho que el Build del corte no funda.

Es P2 porque rompe la puerta aprobada en su camino normal y pone rojo un test aprobado sin declararlo.

**N2 · P2 · [ADJUDICACIÓN-NO-CIERRA][CONTRADICCIÓN] · E40, G-E7, §9, E15-A**

E40 rompe el test que §9 declara intacto.
- `approval_read_class_test.go:158-161` crea `CREATE TRIGGER purge_class_probe AFTER UPDATE OF canonical_params ON approvals`, un trigger persistente sobre una tabla del almacén.
- En `:163` llama a `ClaimApprovalParamsUnderDigest`, que abre con `beginWrite` (`approvals_v15.go:801`) y de ahí pasa por `judgeIn` y `judgeShape` (`ledger_identity.go:166-176`).
- Con E40 la forma queda mala, el error sale como ErrLedgerUnreadable envuelto en ErrApprovalUnreadable (`approvals_v15.go:803`) y los dos asserts de `:169-176` se ponen rojos.

Escala del problema:
- Comando: `grep -rn "CREATE TRIGGER … ON <tabla>" --include=*_test.go internal | … | uniq -c`.
- Salida: approvals 21, action_schema 11, actions 6, receipts 5, signing_keys 3, intents 2, identity_evidence_v2 1, evidence 1, budget_spent 1, approval_tombstones 1. Son al menos 52 triggers persistentes sobre tablas del almacén en tests aprobados, y la doctrina (punto 5) los nombra como el oráculo por imposibilidad.
- El plan no dice si E40 se aplica también a las formas antiguas. Si se aplica, `TestMigrationV12_revalidatesWithZeroWritesUnderAbortTriggers` (`migration_r11_test.go:67-86`) también se pone rojo.

Reproducción (predicción por lectura):
1. Implementar E40 como exige su mutación.
2. Ejecutar ese test.
3. Rojo en `:169`.

**N3 · P2 · [AFIRMACIÓN-FALSA][ORÁCULO] · E41 (la adjudicación de H5)**

El paso 12 no choca. Comandos y salidas:
- `awk 'NR>=385 && NR<=458' internal/action/sqlite/store.go | grep -c "IF NOT EXISTS"` → `6`
- `… | grep -E "^(CREATE|ALTER|INSERT|DROP|UPDATE|DELETE)" | grep -v "IF NOT EXISTS" | wc -l` → `0`
- `grep -n "^var migrationCopies" -A 7` → `store.go:740-746` contiene solo `10, 11, 13, 14, 15`; el paso 12 no tiene copia.

Los pasos 13 a 15 también son idempotentes: `IF NOT EXISTS`, `INSERT OR IGNORE`, ALTER solo si falta la columna (`store.go:811-819`) y la semilla con `WHERE NOT EXISTS` (`profile_standing.go:243-244`). El diseño aprobado dice lo contrario que E41: `ledger_shape.go:176-177`, «A downgraded fixture carrying a newer table is «older», not bad». Hay tests aprobados que dependen de eso:
- `TestMigrate_theSeedKeepsARowAlreadyThere` (`ledger_shape_test.go:413-427`): rebaja un v16 a 15 y exige que OpenFor devuelva un handle.
- `buildV11LegacyFile` (`migration_r11_test.go:30-47`): rebaja un store completo a v11 y se usa 19 veces.
- `repair_procedure_r13_test.go:126`.

Reproducción (predicción por lectura):
1. Un v16 fundado.
2. `UPDATE action_schema SET version = 12`.
3. OpenFor devuelve un handle y la versión vuelve a 16, sin error.

El «migration from v12 failed» de E41 nunca aparece, así que su mutación nunca se ejecuta. Para ponerlo verde habría que añadir un chequeo nuevo que contradice `ledger_shape.go:176-177` y esos tests, y §9 no lo lista. Además, esto es la clase (h): el paso 4 de H5 era una predicción de la ronda 1 y la v3 lo convirtió en desenlace exacto sin verificarlo.

**N4 · P2 · [ORÁCULO][AFIRMACIÓN-FALSA] · E04 (la adjudicación de H3)**

Las conversaciones crean el fichero antes del candado y antes de OpenFor: `app.go:341`, y dentro de `conversation/sqlite/sqlite.go:367` hace MkdirAll y `:395` ejecuta `db.Exec(createTableStmt)`. La CLI, en `store.go:1429-1438`, ve el fichero existente, va a `probeShape` y con `shapeFresh` devuelve `ErrNoActionStore`.

Reproducción (predicción por lectura; en WAL un lector no espera a la transacción IMMEDIATE de un escritor):
1. La app está en su primer arranque, parada por la barrera dentro de su transacción de siembra. El fichero ya tiene sessions, turns y notes.
2. `korvun intent create` pasa el Stat, `probeShape` lee la instantánea confirmada, ve un fichero fresco y sale al instante con ≠ 0 y «not a korvun store?».

No se da ni R («abre el libro ya sembrado») ni A (`ledger_busy`). La mutación «sembrar sin re-juzgar» nunca se alcanza, porque la CLI no siembra en esa intercalación. Para que siembren los dos, la barrera tiene que ir en la CLI, entre su Stat y su apertura.

**N5 · P2 · [AFIRMACIÓN-FALSA] · §3 «Dos apps no pueden abrir el mismo libro», E04 «se declara», §10**

Comando: `grep -rn "AcquireProfileLock" --include=*.go internal cmd | grep -v _test`. Salida: `app.go:355`, `profilelock.go:31,33` (la definición) y `receipt.go:447`.

CreateLedger abre el libro sin candado: `config_act.go:392` (O_EXCL) y `:404` (`openFresh` = `OpenFor`, `:317`). Una app sin almacén nunca toma el candado (`app.go:348`). La ruta por defecto es la del libro del escritorio, y el propio godoc lo nombra como actor concurrente (`config_act.go:344-348`).

Reproducción (predicción por lectura):
1. El perfil P, sin almacén, pulsa «Activar almacén» y empieza a sembrar `<UserConfigDir>/korvun/korvun.db`.
2. El escritorio D hace su primer arranque con la plantilla (`storage.path ""`, la misma ruta). Encuentra el candado libre y abre con OpenFor.
3. Con G-E11, D funda el libro. El acto de P se rechaza por ajeno (`config_act.go:129`) o, si P selló primero, choca como en N1.

También fallaría con enlaces duros en dos directorios, porque serían dos ficheros de candado distintos (predicción).

**N6 · P2 · [ORÁCULO][AFIRMACIÓN-FALSA] · E13 (la adjudicación de H9)**
- (a) `chmod 0444` con la app en marcha no produce READONLY. Los permisos se comprueban en open(2), y el pool escritor mantiene su única conexión durante toda la vida del handle (`store.go:1485`; `ledger_identity.go:659`: «the pool keeps its connection for the handle's life»). Las escrituras siguen funcionando y la mutación de E13 nunca se ejecuta (predicción por lectura, semántica POSIX).
- (b) «Directorio padre inexistente → ENOENT al tomar el candado» es falso. `profilelock.go:34` hace `os.MkdirAll(profileDir, 0o700)` antes de abrir el candado, y `conversation/sqlite/sqlite.go:367` ya lo hizo antes. El arranque crea el directorio y sigue.

**N7 · P2 · [PLAN-FILA-AUSENTE][CONCURRENCIA] · E36, G-E12, §4**

La recuperación de `.new` no tiene dueño ni candado. EnsureSigningKey es la misma función del arranque (`signing.go:129-133`: «same idempotent generation»), y la llaman procesos que no tienen el candado: `intent.go:124`, `config_act_registry.go:402` y `config_act.go:423`. rotate-key sí tiene el candado, escribe `.new` (`signing.go:147,155`), lo registra (`:158`) y lo renombra (`:162`).

Reproducción (predicción por lectura; el plan no fija dónde vive la recuperación, y el sitio natural es esa función compartida):
1. rotate-key escribe `.new` y todavía no lo ha registrado.
2. `korvun intent create` ve un `.new` sin registrar y lo borra.
3. rotate-key registra la clave nueva y su rename falla con ENOENT.
4. La clave privada nueva solo existe en la memoria del proceso, que termina. El registro queda con la clave nueva activa y el fichero con la vieja, retirada.
5. Todos los arranques siguientes se niegan con el mensaje de `signing.go:109`, y rotate-key también, porque su EnsureSigningKey (`receipt.go:472`) falla primero.

No hay fila para este caso.

**N8 · P2 · [MUTACIÓN-SOBREVIVE] · E35, «registro idempotente»**

`registerPublicKey` lee `GetSigningKey` (`signing.go:107`) y `ActiveSigningKey` (`:115`) por el pool, fuera de cualquier transacción, y `case err == nil:` (`:117`) no compara el `KeyID` activo con el del fichero. `PutSigningKey` se niega si ya hay una clave activa (`signing_keys.go:42-49`).

Reproducción (predicción por lectura):
1. Dos procesos generan clave; el que pierde el `link` adopta el fichero del ganador.
2. El perdedor no encuentra la clave con Get.
3. El ganador confirma su Put.
4. El perdedor encuentra con Active la misma clave y sale con «ink identity conflict; refusing to boot», que es justo el efecto que E35 prohíbe.

La única barrera de E35 está tras GenerateKey. El registro queda al azar y no tiene mutación.

**N9 · P2 · [TAXONOMÍA][MÁS-ANCHO-QUE-SU-CABLE] · G-E10 frente a §5; E27, E39**

G-E10 promete «el mismo vocabulario cerrado», pero §5 (línea 68) le da a cada superficie uno distinto:
- La pantalla usa `environment` y `unavailable`.
- `POST /api/config` usa `ledger_environment` y `ledger_busy`.
- Aprobaciones solo gana `ledger_environment`; su BUSY sigue siendo `unavailable` (`controlapi/approvals.go:58`).
- `ledger_transient` no tiene outcome en ninguna puerta.

Reproducción (predicción por lectura, con costuras que ya existen):
1. Como en `ledger_shape_test.go:191-205`, forzar una conexión nueva con un fallo sin código en el hook.
2. Pulsar una puerta de la pantalla.
3. §5 clasifica el error como ErrLedgerTransient, que no tiene outcome, así que acaba en `act_not_recorded` con el texto del driver, que es lo que E27 prohíbe. No hay fila.

**N10 · P2 · [EITHER/OR][ADJUDICACIÓN-NO-CIERRA] · E37 punto (1)**

«Tras crear y sembrar, antes de persistir» abarca tres estados, y la propia regla de E37 les da tres desenlaces distintos:
- Tras el O_EXCL (`config_act.go:392`) queda un fichero de 0 bytes. «No es un almacén», así que da `ledger_exists` sobre su propia cáscara.
- Sembrado y sin actos, da `applied`.
- Tras tx1 (`:439`), la recarga (`whats_happening.go:607`) y el Build del corte (`supervisor.go:270`), el fichero «tiene actos» y da `ledger_exists` sobre su propio huérfano, que es un efecto que la propia E37 prohíbe. Con G-E11, además, ese huérfano ya está fundado por este mismo perfil.

El texto de hoy ya ofrece la salida (`whats_happening.go:566-570`). La mutación «depender de createdHere» sobrevive en el tercer tramo, que es el realista.

**N11 · P2 · [MÁS-ANCHO-QUE-SU-CABLE][ORÁCULO] · G-E9, E23**

`query_only` no impide checkpoints, y con `mode=rw` la última conexión que cierra hace uno (predicción por lectura). El árbol muestra que ese camino de cierre se ejecuta: `store.go:2044-2049` dice «after Close, only korvun.db».

Reproducción:
1. La app muere con frames pendientes en `-wal`.
2. `shasum -a 256 korvun.db`.
3. `korvun ledger check`.
4. `shasum` de nuevo: el hash es distinto.

E23 no prueba ningún WAL caliente.

**P3 (una línea cada uno)**
- **P3-a [AFIRMACIÓN-FALSA] §11-Q1/Q2.** Hay costuras y puertas nuevas sin la marca NUEVO: la de AdoptLedger (hoy no hay ninguna, `profile_standing.go:294-348`), las de E21, E35, E36, E37 y E38, la puerta de fundación en el arranque y el centinela que E28 necesita (hoy `supervisor.go:294` trata cualquier error como fallo de persistencia). D2 y E24-R citan `repair_procedure_r13_test.go`, que prueba la reparación de tombstones (`docs/operations/tombstone-manual-repair.md`), no restaurar una copia. Solo `docs/releases/v0.16.2.md:155-156` lo afirma, y sin pasos.
- **P3-b [AFIRMACIÓN-FALSA] §11-Q4 «Sembradores: los tres».** También siembran `config_act_registry.go:396` y `receipt.go:463`, a través de `store.go:1429-1443`.
- **P3-c [SUPERFICIE-SIN-FILA] E39.** Falta `korvun authority` (`authority.go:104,162,211,252`).
- **P3-d [PIN-SIN-ETIQUETA].** E16 ya es comportamiento de hoy (`ledger_identity.go:361-373`), y E30-R también (`ledger_identity.go:166-196`).
- **P3-e [CLASE-SIN-FILA] §7.** No hay fila para (h) ni para (i), y la fila (f) responde a otra cosa.
- **P3-f [TEXTO-SIN-CABLE][SEXTA-LEY].** La frase de D3 «Se vuelve a comprobar solo» no tiene cable: la pantalla solo carga al montarse y tras pulsar (`WhatsHappening.tsx:301-303,371`). UX-TEMPLATE no se nombra.
- **P3-g [DECISIÓN-NO-ELEVADA] E33.** Cambia un arranque estricto que hoy falla cerrado (`identity.go:433-434,457-458`) y lo llama «decisión del copiloto», fuera de §13.
- **P3-h [PLATAFORMA].** `Sync(dir)` en Windows (predicción). E28-A no da desenlace para `writeRawConfigAtomic`. La primera generación de la tinta no tiene `Sync(dir)` y la rotación sí. El `link` falla en sistemas de ficheros sin enlaces duros (predicción).
- **P3-i [TAXONOMÍA] E23.** Con una ruta inexistente, el Stat previo (`store.go:2083`) da ENOENT, no CANTOPEN. Además, la ausencia del libro queda nombrada como un fallo de entorno.
- **P3-j [ORÁCULO] E05-A.** «La pantalla muestra entorno» tras un FULL no tiene cable: Standing es una lectura y no hay caché (`profile_standing.go:14-16`).
- **P3-k [AFIRMACIÓN-FALSA] §5.** «La siguiente abre otra conexión» solo es cierto para fallos del hook.
- **P3-l [NIVEL-DE-EVIDENCIA].** E42 se prueba con «pruebas del shell», que no pueden observar la ventana; además la falla ocurriría antes, en `firstrun.go:56`. E26 no declara la predicción que hereda de E12.
- **P3-m [ORÁCULO].** E27 no nombra los códigos HTTP. E03-A `'1'` no tiene desenlace y equivale a v1. «version NULL» es inalcanzable con UPDATE porque la columna es `NOT NULL`.
- **P3-n [ENUMERACIÓN] G-E2.** Omite la siembra de las conversaciones (`sqlite.go:391-395`) y el par poda→barrido de `noteWrite` (`store.go:1898-1915`).
- **P3-o [TAUTOLOGÍA].** El oráculo de pantalla de E38 no puede fallar una vez que §9 borra la fila.
- **P3-p [GARANTÍA-SIN-TEXTO].** E32 cita G-E9, pero lo que prueba no está en ninguna G-E.

## Lo que la v3 hace bien, verificado
- Los datos de §1 son correctos: `git status --short | grep -c '^ M'` → 135 y `grep -c '^??'` → 58 (57 más el veredicto).
- Todas las citas de §3 casan con el árbol.
- El orden pre-RED se cumple: `grep` de `classifySQLite|ErrLedgerEnvironment|…|ledger_transient` no devuelve nada.
- §5 cubre los códigos primarios del 0 al 28 más 100 y 101, y `& 0xff` es la misma máscara que ya usa `ledger_identity.go:356`.
- Las cifras son correctas: 24 outcomes (`grep … | wc -l` → 24) y 34 tablas.
- El ataque de E10 es real: el paso 12 (`store.go:440`) se saltaría en silencio el índice UNIQUE.
- Los diagnósticos de E09, E18 y E25 son correctos (`ledger_shape_boot_test.go:116,121`).
- La costura de E06 es determinista.
- E28 cura de verdad la narración falsa de H10.

## Alcance
- **Leído:** el plan y el veredicto de la ronda 1 enteros. En el worktree, app, config_act, el registro de actos, signing, profilelock, supervisor, whats_happening, approvals, profile_standing, ledger_identity, ledger_shape, store, signing_keys, approvals_v15, upgrade, firstrun, controller, serve, receipt, intent y la apertura del almacén de conversaciones, además de los tests citados. Del driver modernc v1.59.0, `error.go`, `conn.go`, `stmt.go`, `driver.go` y `sqlite.go`; de la stdlib, `sql.go` (Go 1.26.6).
- **Ejecutado:** solo comandos de lectura (git, grep, sed, awk, shasum, wc, go env, go version). Ningún test, ninguna compilación, ninguna mutación.
- **No verificado (predicción):** la semántica WAL de lectores y checkpoints, `locking_mode`, chmod sobre descriptores abiertos, `Sync(dir)` en Windows, enlaces duros, y cualquier desenlace dinámico.
- **Sin examinar:** los puntos de la oficial del tren D, §14.6, `claude-code-report.md`, `tren-e-snapshot`, el e2e-harness y el shell de escritorio más allá de firstrun y controller. Tampoco el estado del gate: no pude ejecutarlo y el plan no lo declara.

**Lo que esta ronda vio y la ronda 1 no:** N1 a N11 y P3-a a P3-p. Casi todo lo introduce la v3; no son nuevos en el árbol los de authority, enlaces duros y noteWrite.

## Integridad
- `git -C <worktree> diff | cmp - /tmp/adv-e2-before.patch` → «DIFF: identical».
- `git status --short | grep '^??' | sort | cmp - /tmp/adv-e2-untracked-before.txt` → «UNTRACKED: identical».
- `shasum -a 256 <plan>` → `209b37612c4a29a93bf3abc931c64ec9b5e6d2e102edbe6f8620d9909fb157f0`. No hay nada en staged.
