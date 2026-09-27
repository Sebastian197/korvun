VETO MANTENIDO

**Objeto (v7)**
- Al empezar, 2026-09-25 19:48:50: `wc -l` = 409, `shasum -a 256` = `73ae817f8ac4b86804ca7342173e28aa7097859085fd245bb97dd0e19c5870b7`, mtime 19:46:38. Coincide con lo que midió el ejecutor.
- Al terminar, 2026-09-25 20:23:10: 409 líneas y el mismo sha256.
- Árbol: WT=`/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `d20dea6`. Las citas `internal/…` y `cmd/…` se refieren a `$WT/…`.
- Método: la orden prohíbe tests, compilación y mutaciones, y no he ejecutado ninguno. Todo desenlace dinámico va marcado como «predicción por lectura».

## 1 · Adjudicaciones de la ronda 5 (§10 de la v7)

| Hallazgo | Veredicto | Evidencia |
|---|---|---|
| R5-1 | CIERRA | Con el campo opcional siguen valiendo los POST `{"confirm":true}` de `TestAdopt_*` (`controlapi/ledger_standing_test.go:67`, `:100`, `:118`) y el de `app/profile_standing_test.go:439`. Los dos dobles están declarados (`act_fake_test.go:81`, `act_external_fake_test.go:52`). Las 16 llamadas del almacén (grep: 8+6+1+1) compilan con el variádico vacío. Los dobles de `actLedger` en los tests incrustan la interfaz (`config_act_registry_test.go:182`, `:524`, `:917`). Los defectos que trae la propia cura están en N-9. |
| R5-2 | CIERRA A MEDIAS | La frase «solo ve entorno» ya no está y §5-bis nombra paso a paso. Pero la reproducción literal de R5-2 (P retenido en `seedSeam` y un segundo proceso cualquiera con `EXCLUSIVE`) sigue acabando en `os.Remove` sobre un fichero que otro tiene abierto (`config_act.go:404-409`). La v7 lo niega en §5-bis: «G-E13 garantiza que nadie más lo tiene abierto». E27 (vii) pone el `sqlite3` después de la siembra y no reproduce la primera rama de R5-2 (N-2). |
| R5-3 | CIERRA A MEDIAS | Los veredictos pasan a forma, así que `ledger_identity_test.go:84` y `ledger_binary_test.go:64` pasan (predicción). Versión y Ausencia tienen fila. «Sin clase» solo está definido para las puertas; en el estado de `/api/whats-happening` choca con un test aprobado que §9 no declara (N-4). |
| R5-4 | CIERRA | El fallo del gancho sale de E27 y E27-U lo prueba en unitario, con el nivel declarado. |
| R5-5 | CIERRA | Semántica comprobada en el fuente de Go (sección e). |
| R5-6 | CIERRA | 12, 18, 21 y 25 pasan a determinista y E15-R los enumera. Ningún test los fija: `grep "MISUSE\|TOOBIG\|SQLITE_RANGE\|SQLITE_NOTFOUND"` solo devuelve menciones a `isBusyClass`. |
| R5-7 | CIERRA | G-E7 dice que el código 1 es forma y E27 (v) fija `ledger_unreadable`. La reproducción de R5-7 acaba ahora en lo que el plan declara. Lo que añade la v7, «hasta reiniciar», no tiene cable ni molde (N-8). |
| R5-8 | CIERRA | Pasa a determinista con «reinicia Korvun». La fila que falta no se recrea (`ledger_identity.go:251-253`, `:268`). |
| R5-9 | CIERRA | Definición por conjunto y por `name`/`tbl_name`, con E02-R2 y E02-A(6)/(7). Contar también por `name` deja un contraejemplo a G-E1 (N-7). |
| R5-10 | CIERRA | E45-J existe y §9 declara que crece `WhatsHappening.test.tsx:778` (`toMatchObject({ confirm: true })`). |
| R5-11 | NO CIERRA | `grep -c "R5-11" plan` devuelve 0: no tiene fila en §10 ni aparece en ninguna parte de la v7. §8 (líneas 292-300) sigue sin la categoría «en proceso» (N-5). |

Resultado: 8 CIERRA, 2 CIERRA A MEDIAS y 1 NO CIERRA.

| Anteriores | Veredicto | Evidencia |
|---|---|---|
| R4-2 | CIERRA | La reproducción literal (`policy_version='x'` en v10) acaba en E48 con `ledger_unreadable … tombstone_corrupt` y la versión en 10. Los pasos de migración sin código van a forma. |
| R4-10 | CIERRA | La guarda sin poner es determinista y tiene fila en §5-bis, con su detalle. |
| R4-12 | CIERRA A MEDIAS | (1) y (2) se mantienen. En (3) la frase falsa desapareció, pero dos pasos de «Activar almacén» se apoyan ahora en premisas falsas nuevas: «Candado ocupado → `ledger_exists` porque quien lo tiene ya abrió el fichero» (N-1, N-6) y «se borra … G-E13 garantiza…» (N-2). |
| R3-4 | CIERRA | (b): la guarda sin poner ya es determinista y el gancho sale de E27. |
| P3-e | CIERRA | §7 y §7-bis (d) exceptúan E34. §7-bis (e) retira la frase de R5-2, aunque no recoge las promesas nuevas de G-E13 (N-2). |

## 2 · (b) El candado de «Activar almacén»

- **Cómo funciona, por lectura.** `AcquireProfileLock` hace `MkdirAll(dir,0o700)`, abre `dir/korvun.lock` con `O_CREATE|O_RDWR` y toma `flock(LOCK_EX|LOCK_NB)`. Si está ocupado devuelve `ErrProfileLocked` en el acto (`profilelock.go:33-50`, `profilelock_unix.go:17-23`); en Windows usa `LockFileEx` con `FAIL_IMMEDIATELY` (`profilelock_windows.go:18-27`). Se suelta con `Release` o cuando muere el proceso.
- **No es reentrante entre dos aperturas del mismo proceso.** F3 (`shell/bootstrap_test.go:199`) y F8 (`:398`) toman el candado desde el propio test, y el `Build` del mismo proceso falla en él. El gate q26 los pasa (`/tmp/q26.txt:42`: `ok … internal/shell 22.245s`).
- **App en marcha.** Solo un `Build` con almacén toma el candado (`app.go:350-365`) y lo conserva toda la vida (`:648`, `:1892`). `CreateLedger` solo existe en el grabador sin almacén (`config_act.go:363`, que rehúsa si `Storage != nil`), y esa app no tiene candado. En producción, `CreateLedger` puede tomarlo.
- **Corte.** La puerta pide `RequestReload` (`whats_happening.go:607`) después de que `CreateLedger` vuelve (`:564`), así que el `Build` nuevo encuentra el candado libre, salvo que otro lo tome en ese hueco (E43 ii). El flujo normal se mantiene (predicción).
- **Ruta por defecto frente a otra carpeta.** `CreateLedger` solo usa `<UCD>/korvun/korvun.db` (`app.go:902-911`), pero el candado es del directorio. Lo comparten el escritorio (`firstrun_template.json:26-28`, `storage.path: ""`), cualquier perfil cuyo fichero viva en ese directorio y `rotate-key` sobre esos perfiles (`cli/receipt.go:441-447`). Las demás órdenes de la CLI no lo toman (`profilelock.go:10-12`).
- **Tests.** Rompe `TestBootstrap_aRolledBackCutoverClosesTheFoundingActAsFailed` (N-1). Por lectura siguen verdes: `TestCreateLedger_anUnwritableDirCreatesNoLedger` y `TestBootstrap_anUnwritableProfileDirCreatesNoLedger` (con 0500, `open profile lock: permission denied` da `ledger_not_created`), `…readoptsOnlyWhatThisProcessCreated`, `…aReplacedFileIsNeverReadopted`, `…anOpenThatFailsLeavesNothing`, `TestBootstrap_theFoundedLedgerKnowsItsProfile`, `secrets_reload_test.go:127`, `TestMount_aProfileWithNoStoreRefusesEveryDoorButOne` y los F8. Ninguno lista el directorio (`grep ReadDir|Glob` sin resultados). `internal/supervisor` y `internal/controlapi` no usan el candado.

## 3 · (c) `expected_owner` opcional y variádico

- **Firmas actuales.**
  - `ActRecorder.AdoptLedger(ctx)` (`act.go:161`).
  - `configActRecorder.AdoptLedger` (`config_act.go:166-179`) llama a `r.ledger.AdoptLedger(ctx, env, d, evidence, r.profile)` a través de `actLedger` (`:51`). Esa interfaz también tiene que pasar a variádico o `*Store` deja de cumplirla.
  - `ledgerlessRecorder.AdoptLedger` (`:450`).
  - `Store.AdoptLedger(..., digest string)` (`profile_standing.go:294`) y `beginAdoption(ctx, me)` (`ledger_identity.go:203`), dentro de una transacción IMMEDIATE (`store.go:96`).
- **Tests intactos.** Las 16 llamadas compilan, porque Go admite cero argumentos en un variádico, y recorren la rama «ausente» (predicción).
- **Puertas de producción.** Solo hay una, `whats_happening.go:664`. No hay adopción desde la CLI, la recuperación ni el arranque. Si el adaptador pierde el campo, E45 R/A lo detectan: sus triggers convierten la escritura en un rechazo determinista en vez de un 409 (predicción).
- **Más de un valor en el variádico.** El plan no dice qué pasa (N-9).
- **Parámetros sellados.** Nada sella el campo con un molde (N-9).

## 4 · (d) La reclasificación de §5

- **Declarados en §9 y comprobados:** el gancho (`ledger_shape_test.go:191-214`), `profile_standing_test.go:441-442`, 18/8/6 y el registro de aprobaciones 24 → 25.
- **Siguen intactos por lectura:**
  - `ledger_identity_test.go:906-907` y `:1015-1016`, porque el centinela `ErrLedgerGuardUnset` se conserva.
  - `:1020-1026`: un CHECK (275) va a determinista, no es nil y no es centinela de guarda.
  - Los tres `TestClaim_*`: `RAISE(ABORT)` es 1811 sin `ledger_guard:` y va a determinista; `query_only` es READONLY 8, entorno, dentro de `ErrApprovalUnreadable`.
  - `ledger_shape_boot_test.go:112-117`.
  - Las puertas con `errors.New` sin código, que como «sin clase» conservan el detalle de hoy: `whats_happening_act_test.go:157`, `mutation_act_test.go:92`, `ledger_standing_test.go:126` y `whats_happening_bootstrap_test.go:146`.
- **Cambia sin estar declarado:** `TestConfigActRecorder_anUnreadableStandingIsNamed` (N-4). También se tocarían los tests de autoridad si se cura E33 (N-3).

## 5 · (e) Moldes con `StandingFaultForTest`

- **La semántica, comprobada.** `go doc testing.T.Parallel` dice: «run in parallel with (and only with) other parallel tests». En el fuente, `testing.go:1802-1803` (`t.signal <- true; <-t.parent.barrier`) y `:1990` (`close(t.barrier)` después de que vuelve la función del padre). Un test de nivel superior sin `t.Parallel()` termina antes de que se reanude ningún paralelo. Así que sí: secuencial con `Cleanup` basta para aislarse de los demás tests del paquete.
- **Dónde viven.** E13-A y E26-A están en `internal/app` (80 `t.Parallel()` en 24 ficheros) y E47-R en `internal/cli` (107 en 28). Nada les obliga a ser paralelos.
- **Dos matices, sin severidad.**
  - El plan no dice que la variable sea atómica. Los cuatro seams que ya existen son `atomic.Pointer` (`ledger_identity.go:654`, `:660`, `:665`, `:669`).
  - Una goroutine que sobrevive a su test no es un test. No he encontrado ninguna que llame a `Standing` en segundo plano; sus llamadas están en `app.go:409`, `config_act.go:187`, `cli/ledger.go:150` y `ledger_identity.go:91`.

## 6 · Marcas [veredicto]

`awk` da 20 marcas en 19 líneas; la 88 lleva dos. Las de las líneas 10, 339 y 366 son de definición. Las demás casan en el WT:
- 73 `receipt.go:447`; 80 `:463` (`OpenOperatorFor`);
- 88 `identity_v2.go:189-230` (INSERT en `actions` y `action_decisions`, y `insertEvidenceTx`) y las 16 llamadas;
- 104 `serve.go:110` y `controller.go:261`;
- 224 `identity.go:433-434`;
- 229 `authority.go:104,162,211,252`;
- 247, 248 y 251: `v0.16.2.md` 103-104, 112-115, 21, 24, 29 y 177;
- 287 (16 llamadas);
- 307 `:441-442`; 308 `:81` y `:52`;
- 309 `:778`; 311 `:365-400` (17/7/10/6);
- 316 (`:67`, `:100`, `:118`); 320 `ledger_test.go:162-183`.

## 7 · Hallazgos nuevos

**N-1 · P2 · [TEST-APROBADO-NO-DECLARADO][AFIRMACIÓN-FALSA] · G-E13, §4 «Candado…», §5-bis «Candado ocupado», §9.**

Evidencia:
- `shell/bootstrap_test.go:202`: `lock, err := app.AcquireProfileLock(filepath.Dir(ledger))`.
- `:213`: `pressDoor(t, srv, "enable-storage", `{}`)`.
- `:219-221`: `if code != http.StatusOK || out.Outcome != "applying" && out.Outcome != "not_applied" { t.Fatalf(… want 200 applying/not_applied) }`.
- §9 cita `bootstrap_test.go:292-293`, pero no este test. En el plan, `grep "RolledBack|F3"` no devuelve nada.
- En el propio escenario de F3 (otro servidor tiene el candado del directorio y no hay fichero), la respuesta nueva es falsa. El detalle dice «no se activó: ya hay un libro en <path>…» (`whats_happening.go:566-570`) y la pantalla dice «ya hay un libro en la ruta por defecto…» (`WhatsHappening.tsx:118-119`).

Reproducción (predicción por lectura):
1. Implementar G-E13 tal como está escrito: candado antes del `O_EXCL`, y `ErrProfileLocked` → `ledger_exists`.
2. `go test ./internal/shell -run TestBootstrap_aRolledBackCutoverClosesTheFoundingActAsFailed`.
3. Rojo en `:219-221`: sale 409 `ledger_exists`, mientras `stat <UCD>/korvun/korvun.db` da que no existe.

Por qué P2: es el criterio de R5-1 y R4-1 (un test aprobado y del gate que se rompe sin declararlo), y además el desenlace nuevo afirma algo falso.

**N-2 · P2 · [AFIRMACIÓN-FALSA][CONCURRENCIA][PLAN-FILA-AUSENTE] · G-E13, §5-bis «El fichero se borra, y G-E13 garantiza que nadie más lo tiene abierto», E49, §11-Q4, D9.**

La promesa literal es: «ningún otro Korvun abre ese directorio … el fichero que la puerta borra … no lo tiene abierto nadie más». Es falsa por tres vías:
- (a) `Build` abre las conversaciones sobre ese mismo fichero antes del candado: `app.go:341` → `:890` `sqlite.Open` → `conversation/sqlite/sqlite.go:395` `Exec(createTableStmt)`, en WAL (`:225`). El candado llega después, en `app.go:355`. El §3 del propio plan lo dice (línea 73).
- (b) Las órdenes de operador de la CLI no toman nunca el candado: `profilelock.go:10-12`, «HOUSE AMENDMENT: no other operator act takes the lock», fijado por `cli/rotatekey_r2_test.go:109`.
- (c) Un proceso que no es Korvun no mira un `flock` consultivo; es el paso 2 de R5-2.

Además, D9 añade el texto «Otro proceso de Korvun lo está usando», justo para el caso que G-E13 declara imposible.

Reproducciones (predicción por lectura; las barreras son NUEVAS, como las de E43/E49):
- **A.**
  1. P: perfil sin almacén. D: `storage.path` vacío, sin arrancar.
  2. P pulsa «Activar almacén» y se retiene tras el `O_EXCL`, con un seam de `openFresh` que falla.
  3. D arranca: `openStore` escribe `sessions/turns/notes` en el fichero de P y D queda retenido entre `app.go:341` y `:355`.
  4. Se suelta P: `os.Remove(path)` (`config_act.go:409`), `ledger_not_created` y el candado queda libre.
  5. Se suelta D: el candado entra, y `OpenFor` crea un `korvun.db` nuevo. Las conversaciones de D siguen en el inodo borrado y comparten por nombre `-wal`/`-shm` con el fichero nuevo.
  6. Se reinicia D: lo que escribió en el paso 5 ya no está.

  E49 solo ejerce el intercalado en que todo el `Build` de D cae dentro de la ventana de P.
- **B, R5-2 literal.**
  1. P retenido en `seedSeam`.
  2. `sqlite3 <ruta>`: `PRAGMA locking_mode=EXCLUSIVE; BEGIN IMMEDIATE;`.
  3. Se suelta P: BUSY, `os.Remove` del fichero que `sqlite3` tiene abierto.
- **C.**
  1. `openFresh` de P falla después de la siembra (por ejemplo, disco lleno al migrar), con una barrera antes del fallo.
  2. `korvun intent create --config D.json`, con el almacén en la ruta por defecto: abre sin candado el libro v16 sin marca, escribe y sale con 0.
  3. Se suelta P: `os.Remove`, y el intent ya no existe.

Por qué P2: es una garantía literal presentada como cura de una pérdida de datos (líneas 17 y 69), falla por tres caminos independientes, uno de ellos la reproducción textual de R5-2 (regla 1), y ninguna fila cubre el intercalado peligroso.

**N-3 · P2 · [TAXONOMÍA][AFIRMACIÓN-FALSA] · §5 «Un solo clasificador… Sustituye a isBusyClass, mapGuardError y… purgeWriteFailure», G-E3, G-E10, E33, E39, §7-bis (c)(g), §11-Q4 «los tres clasificadores por texto usan uno solo».**

Evidencia:
- `authority_v2.go:704-706` (`mapAuthorityStoreError`) clasifica por texto: `strings.ToLower(err.Error())` con «busy», «locked», «interrupted», y además `context`, todo a `ErrAuthorityStoreBusy`.
- `:721-729` (`authorityReadFailure`): lo que no es busy devuelve `corrupt` y pierde la causa.
- `grep -n "mapAuthorityStoreError\|authorityReadFailure" | grep -v _test | wc -l` devuelve 51 usos.
- Arranque estricto: `identity.go:433` → `RequireAuthorityActivation` (`authority_v2.go:380-…`), que lee a través de `authorityReadFailure(ctx, err, ErrAuthorizationSnapshotCorrupt)`.
- Detalle de una aprobación: `approvals_v15.go:361` → `:548-553`. Una lectura de clave que falla sin ser busy se convierte en `ErrAuthorizationSnapshotCorrupt`, y `approvals_adapter.go:217-218` la traduce a `ErrApprovalEvidenceCorrupt`.
- Hay tests aprobados que fijan otra taxonomía: `authority_unreadable_test.go:30` (un `context` vencido es «store busy», cuando §5 dice «tal cual, sin clase») y `authority_as08_test.go:72`.
- Queda otra guarda por texto: `approvals.go:492` (`"UNIQUE constraint failed"`).

Reproducción (predicción por lectura):
1. Perfil estricto con una aprobación `apr3_` pendiente.
2. Un IOERR (10) al leer `signing_keys` en `publicKeyTx`.
3. `GET` del detalle responde `evidence_corrupt`, permanente, y no `ledger_environment` 503.
4. Arranque estricto sin `approval_birth_heads`: sale `app: verify strict authority activation: action/sqlite: authorization snapshot corrupt`, sin clase.

E33 exige «con la clase en el error» y no puede ponerse verde sin tocar `authorityReadFailure`, cambio que ni §5 ni §9 declaran.

Por qué P2: G-E3 y G-E10 son falsas en una superficie de §5-bis y en el arranque; es el criterio de R5-2 y R3-4.

**N-4 · P2 · [TEST-APROBADO-NO-DECLARADO][TAXONOMÍA] · §5 «Sin clase», columna de estado de §5-bis, §12 «salen … sin nombre de clase», §9.**

Evidencia:
- `app/config_act_registry_test.go:917-943`: `Standing` devuelve `errors.New("database is locked")`, un error sin código, y el test exige `standing == LedgerStandingUnreadable` (`:941`).
- Hoy, `config_act.go:185-192` convierte cualquier error en `unreadable`.
- En la v7 ese error es «sin clase». La columna de estado solo nombra forma, entorno y momento, y §12 prohíbe darle nombre de clase.

Reproducción (predicción por lectura):
1. Implementar `LedgerStanding` según §5-bis y §12.
2. `go test ./internal/app -run TestConfigActRecorder_anUnreadableStandingIsNamed`.
3. O sale rojo en `:941`, o el estado le pone el nombre de forma a un error que el plan dice que no tiene clase.

Por qué P2: es el criterio de R5-1 y R5-3.

**N-5 · P3 · [ADJUDICACIÓN-NO-CIERRA R5-11][NIVEL-DE-EVIDENCIA] · §8, §10.**

`grep -c "R5-11"` devuelve 0. En §6 aparecen como «en proceso» E03, E04, E05, E06, E09, E10, E22, E31 y E39-R, y como «conexiones reales» E16 y E30; §8 sigue sin recogerlos.

Reproducción:
1. Comparar la columna Nivel de §6 con las listas de §8.

**N-6 · P3 · [AFIRMACIÓN-FALSA][ORÁCULO][E34-INCOMPLETA] · §4 línea 133, E37, E34.**

«Quien tiene el candado de ese directorio ya abrió el fichero» es falso:
- El candado es del directorio (`profilelock.go:37`).
- `rotate-key` lo toma antes de abrir su libro (`receipt.go:447` y `:463`), y para su propio fichero.
- G-E13 lo toma antes del `O_EXCL`.
- Un perfil cuyo almacén es `<UCD>/korvun/otro.db` lo mantiene toda su vida.

Hay godocs que pasan a ser falsos y que E34 no recoge: `whats_happening.go:259-263` («found a file already at the path») y `act.go:150`. Además, en E37 el efecto prohibido «candado sin soltar» no tiene oráculo: con el candado atascado o con el fichero presente, la salida es el mismo `ledger_exists` con el mismo texto.

Reproducción (predicción por lectura):
1. `korvun serve` con `storage.path=<UCD>/korvun/otro.db` en marcha.
2. Un perfil P sin almacén pulsa «Activar almacén».
3. Sale 409 «ya hay un libro en la ruta por defecto», y `korvun.db` no existe.

**N-7 · P3 · [AFIRMACIÓN-FALSA] · G-E1 «Nunca se juzga forma mala», §4 «Residuo», E09.**

E09 declara sembrable un fichero con un trigger `actions` sobre `sessions`. La definición de §4 cuenta ese trigger por `name`. Hoy `judgeShape` también lo cuenta (`ledger_shape.go:126-151`); E09 lo cambia solo para k=0.

Reproducción (predicción por lectura):
1. Partir del fichero de E09.
2. Crash en `seedSeam` con k=1.
3. Al reabrir, las entradas son {`action_schema`, trigger `actions`}, que no es el conjunto de k=1: forma mala.

**N-8 · P3 · [AFIRMACIÓN-FALSA][MUTACIÓN-SOBREVIVE] · G-E7 «hasta reiniciar», §5 Forma «pegajoso», E27 (v).**

Solo un veredicto de `judgeIn` marca la guarda (`ledger_identity.go:175-181`, `:212-216`). Una escritura que falla pasa por `txExec`/`mapGuardError` (`:325-328`, `:347-376`) sin marcar nada. §3 y §4 no añaden cable. El oráculo de E27 (v) solo mira el nombre.

Reproducción (predicción por lectura):
1. `CREATE TRIGGER t BEFORE INSERT ON actions BEGIN INSERT INTO nope VALUES (1); END;`
2. `POST enable-approvals` responde 503 `ledger_unreadable`.
3. `DROP TRIGGER t`.
4. `POST enable-approvals` vuelve a registrar. Con eso, la mutación «no pegajoso» sobrevive a E27.

**N-9 · P3 · [TAXONOMÍA][MUTACIÓN-SOBREVIVE] (a) · §4 «Dueño esperado», D6.**

Hoy se sella la constante `{"door":"adopt-ledger"}` (`config_act.go:170`). El plan no define cómo se sellan un campo ausente, `""` o `null` de JSON. Ninguna fila lee el digest de parámetros de la adopción: los únicos pines construyen su propio sobre (`ledger_identity_test.go:87`, `profile_standing_test.go:361`, `:420`). Un variádico con más de un valor no tiene desenlace con nombre. `whatsRequest` es común a todas las puertas (`whats_happening.go:291-297`).

Reproducción (predicción por lectura):
1. Sellar como hoy, o con `omitempty`.
2. E38, E45 y E45-J siguen verdes.
3. Llamar a `AdoptLedger(ctx, env, d, ev, pB, "", "sha256:…")`: el plan no dice qué pasa.

**N-10 · P3 · [TAXONOMÍA][PLAN-FILA-AUSENTE] · §5 «Sustituye a isBusyClass».**

`isBusyClass` (`store.go:1687-1690`) decide si se salta con nota o se aborta en la recuperación (`:1646`) y en el barrido de caducidad (`approvals.go:773`). En el driver fijado, SQLITE_LOCKED dice «database table is locked» (`grep -rho` en `lib/`: 16 apariciones de esa frase y 17 de «database is locked»), así que hoy aborta. La v7 no dice qué clase provoca el salto; LOCKED, o todo «del momento», pasaría a saltarse. No hay fila, y CLAUDE.md exige probar que una condición es benigna antes de degradarla.

Reproducción (predicción por lectura):
1. Forzar LOCKED en `closeCrashOrphan`.
2. Hoy el arranque aborta; con la v7 no hay desenlace definido.

**N-11 · P3 · [COHERENCIA] · G-E4, tercer punto, frente a G-E6/E06-A y §4.**

G-E4 dice que un paso de migración que falla da `ledger_unreadable`. E06-A espera `ledger_busy` para un paso que falla por BUSY, y §4 excluye de ilegible los errores de entorno y del momento.

Reproducción:
1. Leer E06-A literal frente a la línea 47 del plan.

**N-12 · P3 · [NIVEL-DE-EVIDENCIA] · E27 (iii)/(vii), §12.**

E27 (vii), «sostiene EXCLUSIVE», y (iii) dependen de cómo se comporta EXCLUSIVE frente a WAL, algo sin ejecutar. §12 declara esa predicción solo para E12, E26 y E47, y la columna de riesgo de E27 dice «24 → 25».

Reproducción:
1. Comparar §12 con E27.

## 8 · Lo que la v7 hace bien, verificado

- **Orden pre-RED.** `grep -rln -E "classifySQLite|ErrLedgerEnvironment|…|ledger_changed" internal cmd docs` sale con 1 y no devuelve nada. No hay `docs/operations/*restore*`.
- **Gate.** `/tmp/q26.txt` acaba en «Quality gate passed.» / «EXIT=0». `find -newer` solo devuelve los cinco veredictos.
- **Recuentos.** La matriz tiene 46 filas. Las cifras de P2 por ronda (14, 11, 6, 2, 3) casan con `grep` sobre los veredictos.
- **Citas de §3**, todas casan: `config_act.go:392`, `:404`; `whats_happening.go:578-583`, `:651-656`; `ledger_identity.go:211`, `:251-253`, `:440-450`, `:656-660`; `store.go:1253`, `:1256-1258`, `:1438`, `:1485`, `:1501-1508`, `:1597`, `:1610`, `:2109`; `profile_standing.go:175`, `:184`, `:213`, `:215`; `supervisor.go:232-239`, `:270`, `:379`; `config_act_registry.go:152-158`, `:396`; `app.go:473-474`; `driver.go:266-269`; `sql.go:1574-1583` (go1.26.6).
- **Cura de R5-1.** No rompe `TestAdopt_*`, y la adopción se compara dentro de una transacción IMMEDIATE.
- **Oráculo de E45.** Cubre las cinco tablas persistentes que escribe `AdoptLedger`.

## 9 · Alcance

- **Leído:**
  - el plan v7 entero, el veredicto de la ronda 5 entero, R4-2, R4-10 y R4-12, y `git diff -- CLAUDE.md` del WT (séptima ley);
  - en el WT, los ficheros y tramos citados arriba;
  - fuera del WT: `testing.go`, `database/sql`, y `error.go`, `conn.go`, `driver.go` y `lib/` de modernc v1.59.0.
- **Ejecutado:** solo comandos de lectura: `date`, `wc`, `shasum`, `stat`, `git`, `grep`, `sed`, `awk`, `find`, `ls`, `cat`, `cmp`, `go env`, `go doc` y `go version`. Ningún test, ninguna compilación, ninguna mutación y ningún fichero auxiliar.
- **No verificado (predicción):** los rojos de N-1 y N-4, los intercalados de N-2, la ruta IOERR de N-3, N-7, N-8, N-10 y EXCLUSIVE frente a WAL.
- **Sin examinar, por la orden:** la instantánea, la ficha UX, las decisiones UX y la maqueta. Tampoco `design-drafts/tren-e-tests-afectados.md`, que se modificó a las 20:03 durante esta ronda.
- **Tiempo:** de 19:48 a 20:23, tres minutos por encima del presupuesto.
- **Lo que esta ronda vio y la anterior no:** N-1 a N-12. N-1, N-2, N-4, N-6, N-7, N-8, N-9 y N-10 nacen de curas de la v7. N-3 existe en el árbol y la v7 lo niega en §5 y §11.

## 10 · Integridad (20:23:10)

- `git -C $WT diff | cmp - …/adv-e6-before.patch` → «DIFF: identical».
- `git -C $WT status --short | grep '^??' | sort | cmp - …/adv-e6-untracked-before.txt` → «UNTRACKED: identical».
- `git -C $WT diff --cached --quiet` → sale con 0.
- Plan: 409 líneas, sha256 `73ae817f8ac4b86804ca7342173e28aa7097859085fd245bb97dd0e19c5870b7`.