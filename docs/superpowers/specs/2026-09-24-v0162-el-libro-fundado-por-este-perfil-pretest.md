# v0.16.2 · el marcador durable «libro fundado por este perfil» — plan de fallos

**Orden del director (2026-09-24):**

> El marcador durable «libro fundado por este perfil» ENTRA HOY, en este tren,
> antes del tag v0.16.2. Diseño mínimo: al fundar el libro se sella en su primer
> recibo el digest del perfil que lo funda; en cada apertura, el lector compara
> el perfil actual con ese digest y, si no coincide, lo dice con nombre
> (`ledger_foreign_profile`) en `ledger check`, `receipt verify` y en «¿Qué pasa
> hoy?» — sin bloquear la lectura, sí bloqueando cualquier acto nuevo hasta que
> el operador adopte el libro con un acto explícito (que también deja recibo).
> Libros anteriores a esta versión, sin marca: `legacy_unfounded`, nombrado, no
> corrupto. Plan de fallos corto (perfil movido de sitio, perfil copiado, dos
> perfiles apuntando al mismo libro, adopción con fallo a mitad), tres moldes por
> cura, mutación roja, pasada interna, oficial acotada a esto, PR, «fusiona», tag.

---

## 0 · Qué es «el perfil» para el marcador, dicho antes de sellarlo

El marcador tiene que sobrevivir a las ediciones legítimas del perfil (cada botón
de la pantalla lo edita) y distinguir un perfil MOVIDO, uno COPIADO a otro sitio
y DOS perfiles sobre el mismo libro. Un digest del CONTENIDO del perfil no sirve:
la primera edición dejaría al libro «ajeno». Un identificador escrito dentro del
perfil no distingue una copia (la copia lo lleva) y su adopción exigiría
escribir el perfil, que es un cambio de perfil, que exige un acto… que el
libro ajeno bloquea.

**La identidad de un perfil es la ruta absoluta y limpia de su fichero.** El
digest que se sella es `sha256` sobre el documento canónico
`{"profile":"<ruta absoluta>"}` (`action.HashCanonical`). Es estable ante
cualquier edición, cambia cuando el perfil se MUEVE, y dos ficheros distintos
son dos identidades. Los enlaces simbólicos se RESUELVEN (`/tmp` y
`/private/tmp` son uno; un perfil enlazado se identifica por su destino). Lo
que NO distingue, y se declara: una copia que SUSTITUYE al original en la misma
ruta (es, para el libro, el mismo perfil), y dos grafías de mayúsculas de la
misma ruta en un volumen insensible a mayúsculas (macOS APFS por defecto): solo
en Windows se pliega la caja, y un `--config` tecleado con otra caja vería
`ledger_foreign_profile` sobre su propio libro (la CLI no adopta: sin daño).

**Dónde vive el marcador:** en el campo `result_digest` del recibo del acto
fundacional (`config.enable-storage`, cerrado `SUCCEEDED`), con la forma
`profile:sha256:<hex>`. Ese campo forma parte del recibo firmado; ningún
desenlace de herramienta produce esa forma (los digests de resultado son
`sha256:…`), así que el lector lo reconoce por su forma sin depender de la fila
de la acción, que la retención puede haber podado. **«Primer recibo» se acota a
«recibo fundacional»**: entre el arranque del app nuevo y el cierre del acto
fundacional un cerebro puede sellar un recibo antes; el lector toma el ÚLTIMO
recibo marcado, no el de `chain_seq 0`.

**La adopción** es un acto propio, `config.adopt-ledger`, sellado y cerrado
`SUCCEEDED` en UNA transacción del store (la excepción al bloqueo), cuyo recibo
lleva `profile:sha256:<hex del perfil que adopta>`. Desde ese recibo el dueño es
el adoptante. No toca el perfil, no pasa por el supervisor.

---

## 1 · Diseño, en las líneas que caben

| # | Pieza | Qué hace |
|---|---|---|
| D1 | `app.ProfileIdentity(profilePath) string` | el digest de identidad sobre la ruta ABSOLUTA del fichero cargado (`filepath.Abs`; `EvalSymlinks` cuando resuelve, así `/tmp` y `/private/tmp` son uno; en Windows se pliega a minúsculas). Un `-config korvun.json` relativo identifica el fichero que de verdad se cargó desde ese cwd, que es el que el supervisor persiste (P2-4). La app lo recibe con `WithProfilePath`; `openOperatorStoreSealed` (los escritores de la CLI: grant, authority, intent, approvals) y `openOperatorStore` (los lectores) lo fijan desde `--config` (P1-2) |
| D2 | `Store.SetProfileIdentity(digest)` + `Store.Standing(ctx) (LedgerStanding, owner, error)` | el handle guarda SOLO la identidad; el estado se CALCULA cada vez (una consulta: el último recibo con `result_digest` de forma `profile:…`): ninguno → `legacy_unfounded`; igual → `ok`; distinto → `ledger_foreign_profile`. Nada en memoria que otro handle pueda dejar viejo: un app vivo cuando otro perfil adopta lo ve en su siguiente escritura (delta, P1-2) |
| D3 | el bloqueo | `refuseIfForeign(ctx)` al principio de CADA puerta que inserta un acto, aprobación o intento: `RecordAttempt`, `RecordAttemptIdentified`, `RecordAttemptAuthenticated`, `createApprovalPartsWithIdentity` (el aparcado del cerebro, por sus dos entradas), `decideApprovalWithLaw`, `CreateIntent`, `CreateIntentV2` y `beginAuthorityWrite` (todas las escrituras de autoridad). Rehúsa `ErrLedgerForeignProfile` ANTES de cualquier escritura. Un molde ESTRUCTURAL camina el fuente del paquete: toda función que contenga `INSERT INTO actions`, `approvals`, `intents` o `intent_versions` está en la lista guardada o llama a `beginAuthorityWrite`, y enrojece nombrando la que no (delta, P1-1). Las lecturas no se tocan; `Finish`/cerrar no es un acto nuevo; los cierres de la recuperación de arranque (`closeCrashOrphan`) son cierres, no actos (declarado, P3-9) |
| D4 | `Store.AdoptLedger(ctx, env, d, evidence, digest) (receiptID, error)` | una transacción: inserta la acción `config.adopt-ledger` AUTHORIZED con su decisión y evidencia, la cierra SUCCEEDED con `result_digest = profile:<digest>` y su recibo; sin sellador rehúsa por nombre (`ErrNoSealer`: un marcador sin recibo se perdería en silencio, P3-6). El estado, recalculado, es `ok` desde ese recibo. Con `ok` o `legacy_unfounded` también se permite |
| D5 | el acto fundacional cierra con marca | `CreateLedger` recuerda en el registro `results[acto] = <digest>`; `finishThrough` usa `Store.FinishFounding(ctx, id, digest)` — la ÚNICA puerta que escribe el prefijo `profile:` — SOLO al cerrar `SUCCEEDED` y solo con sellador (`ErrNoSealer`); `FinishWithResult` público rehúsa por nombre un `result_digest` con ese prefijo (`ErrReservedResultDigest`): el cierre de un cerebro no puede reasignar al dueño (P3-5). `actLedger` gana `FinishFounding` |
| D6 | los lectores | `ledger check` y `receipt verify` imprimen una línea `ledger standing: ok \| legacy_unfounded \| ledger_foreign_profile (founded by <digest>; this profile <digest>)`; no cambian el código de salida |
| D7 | la pantalla | el estado viaja en la puerta de LECTURA de la pantalla, `GET /api/whats-happening`, como `ledger: {standing, owner}` (del perfil; también con aprobaciones apagadas, que es cuando `/api/approvals` rehúsa, y con cero cerebros: P3-8). El recorder lo provee (`ActRecorder.LedgerStanding`). La pantalla pinta `ledger_foreign_profile` como fila bloqueante con botón «Adoptar libro» (puerta `adopt-ledger`, con confirmación en un paso) y `legacy_unfounded` como texto («libro anterior a esta versión, sin marca de perfil»). No se añade condición por cerebro al guardián |
| D8 | la puerta `POST /api/whats-happening/adopt-ledger` | `confirm` obligatorio; con `rec` sin libro → `no_ledger`; con libro → `AdoptLedger` por el recorder; responde `adopted` con el acto y su recibo. No pide cambio al supervisor |
| D9 | aritmética | 17 filas, 7 con botón, 10 de texto, 6 puertas; notas y comentario de paquete |
| D10 | CLI | adopción por CLI (`ledger adopt`) FICHADA: la orden nombra la pantalla; un perfil de `serve` ajeno se adopta hoy desde la app apuntada a ese perfil, o se declara |

---

## 2 · Garantías, literales

| # | Garantía |
|---|---|
| L1 | El libro fundado por `enable-storage` lleva, en el recibo del acto fundacional cerrado `SUCCEEDED`, `profile:sha256:<digest del perfil fundador>`; un cierre `FAILED` no marca |
| L2 | Abierto por el mismo perfil (misma ruta absoluta): `ok`. Movido a otra ruta, copiado a otra ruta, o abierto por otro perfil: `ledger_foreign_profile`, nombrado en `ledger check`, `receipt verify` y la pantalla |
| L3 | Con `ledger_foreign_profile`, toda puerta del store que inserta un acto, una aprobación o un intento rehúsa `ErrLedgerForeignProfile` ANTES DE CUALQUIER ESCRITURA (probado con triggers de aborto), desde cualquier handle que conozca su identidad: la app, los escritores de la CLI, el aparcado del cerebro; las puertas de configuración responden `ledger_foreign_profile` con 0 recargas. Las lecturas siguen. Fuera del alcance, declarado: un handle abierto sin identidad (el cierre transitorio del registro, que solo cierra) |
| L4 | La adopción es UNA transacción: o queda el acto SUCCEEDED con su recibo marcado y el estado pasa a `ok`, o no queda nada y el estado no cambia |
| L5 | Un libro sin marca (anterior a esta versión, o fundado y aún sin cerrar) es `legacy_unfounded`: nombrado, no bloquea, no es corrupción |
| L6 | Dos perfiles sobre un libro: el que lo fundó/adoptó por última vez es el dueño; el otro es `foreign` hasta que adopte, y entonces el primero pasa a serlo EN SU SIGUIENTE ESCRITURA, también con su handle ya abierto (dos conexiones reales). El libro tiene UN dueño a la vez, y la historia de adopciones queda en los recibos |

### Lo que estas garantías NO dicen

- Una copia que sustituye al original en la MISMA ruta es el mismo perfil.
- Un perfil enlazado (symlink) se identifica por su DESTINO resuelto; el tren
  del symlink decide si eso es lo que el operador espera.
- La adopción por CLI no existe (D10, fichada).
- La marca protege contra el uso ACCIDENTAL de un libro por otro perfil; quien
  edita el fichero SQLite a mano puede escribir un recibo marcado (mismas
  condiciones que SECURITY.md declara para todo el libro: tamper-evident, no
  tamper-proof).

---

## 3 · Plan de fallos

| # | Fallo | Desenlace exigido | Molde | Nivel |
|---|---|---|---|---|
| G1 | Perfil MOVIDO de sitio (mismo fichero, otra ruta) | `ledger_foreign_profile` en `SetProfileIdentity`, `ledger check`, `receipt verify`; `RecordAttempt` rehúsa por nombre; `ListReceipts` sigue leyendo | `TestStanding_aMovedProfileIsForeign` (sqlite, store real), `TestLedgerCheck_namesAForeignProfile` (cli in-process), `TestReceiptVerify_namesTheStanding` (cli) | store real; CLI in-process |
| G2 | Perfil COPIADO a otra ruta apuntando al mismo libro | la copia: `foreign`; el original: `ok` | `TestStanding_aCopiedProfileIsForeignAndTheOriginalIsNot` (sqlite) | store real |
| G3 | DOS perfiles sobre el mismo libro, el segundo adopta | tras adoptar, el segundo `ok` y el primero `foreign`; el recibo de adopción lleva el digest del segundo; `ledger check` cuenta un recibo más y la cadena sigue intacta | `TestAdopt_theBookHasOneOwnerAtATime` (sqlite), `TestAdopt_theDoorAdoptsAndAnswersTheReceipt` (controlapi + app real) | store real; app real HTTP |
| G4a | Adopción cuyo recibo el store rehúsa al nacer (sellador que firma en vacío → `receipt_unsigned`) | ningún acto, ningún recibo, `foreign`, `RecordAttempt` sigue rehusando | `TestAdopt_anUnsignedReceiptLeavesNothing` (sqlite) | store real |
| G4b | Adopción abortada por SQLite en el recibo (trigger de aborto sobre `receipts`) | ídem, con el error del trigger nombrado, y la fila de la acción NO sobrevive (una transacción) | `TestAdopt_anAbortedReceiptRollsTheActBack` (sqlite) | store real + segunda conexión |
| G4c | Adopción sin sellador | `ErrNoSealer`, nada escrito | `TestAdopt_refusesWithoutASealer` (sqlite) | store real |
| G5 | Libro anterior (sin marca) | `legacy_unfounded`; nada bloquea; la pantalla lo nombra como texto | `TestStanding_aLedgerWithNoMarkIsLegacy` (sqlite), molde TS | store real; jsdom |
| G6 | Libro fundado por la puerta, abierto después por el mismo perfil, y el app que lo fundó lo ve `ok` SIN reiniciar | `ok` en `ledger check`; el gate del app vivo publica `ok` tras el cierre fundacional (estado recalculado, no cacheado: P2-3) | `TestBootstrap_theFoundedLedgerKnowsItsProfile` (shell, cutover real: funda, cierra, `GET /api/approvals` → `ok`; reabre por CLI `ledger check` → `ok`) | cutover real + CLI in-process |
| G7 | Con `foreign`, cada puerta de config rehúsa | `ledger_foreign_profile`, 0 recargas, 0 actos | `TestAct_aForeignLedgerRefusesEveryDoor` (controlapi, doble: prueba el NOMBRE que propaga la puerta) + `TestMount_aForeignLedgerRefusesEveryDoorButAdopt` (app REAL construida con `WithProfilePath` de otro perfil sobre un libro fundado: prueba el bloqueo real de punta a punta) | httptest; app real, servidor admin real en loopback |
| G8 | El bloqueo cubre la puerta ENTERA | `RecordAttempt`, `RecordAttemptIdentified`, `RecordAttemptAuthenticated`, `CreateApprovalRequest`, `CreateApprovalRequestAuthenticated`, `DecideApprovalUnderLaw`, `CreateIntent`, `CreateIntentV2`, `ParkAuthorization`, `StartAuthorization`: cada uno rehúsa con el nombre ANTES DE CUALQUIER ESCRITURA — oráculo por imposibilidad: triggers de aborto sobre `actions`, `approvals`, `receipts`, `intents`, `intent_versions` y `authority_write_lock` instalados por una segunda conexión; una puerta que llegara a SQLite respondería el error del trigger, no el nombre | `TestForeign_everyActDoorRefusesBeforeAnyWrite` (sqlite, tabla) + `TestForeign_everyInsertSiteIsGuarded` (estructural: camina el fuente) | store real + segunda conexión; estructural |
| G9 | Un cerebro cierra con `result_digest = profile:…` | `ErrReservedResultDigest`; el dueño no cambia | `TestMark_theProfilePrefixIsReservedToTheFoundingDoors` (sqlite) | store real |
| G10 | Dos handles vivos: A dueño, B adopta | A, sin reabrir, rehúsa su siguiente acto; `Standing` de A lo dice | `TestAdopt_aLiveOwnerLosesTheBookOnItsNextWrite` (sqlite, dos conexiones) | store real + segunda conexión |
| G11 | La CLI escribe sobre un libro ajeno (`grant`, `intent create`, `approvals approve`) | cada verbo rehúsa nombrando `ledger_foreign_profile`, exit 1, nada escrito | `TestCLI_aForeignLedgerRefusesEveryWriter` (cli in-process, tabla) | CLI in-process, store real |

**Mutaciones previstas** (M94+): no marcar el recibo fundacional; comparar solo
`chain_seq 0`; tratar `legacy` como `foreign`; bloquear también las lecturas;
dejar una puerta sin guarda (por cada una de las cinco); adopción en dos
transacciones (el trigger de aborto entre ambas la enrojece); `AdoptLedger`
sin cambiar el estado; la puerta `adopt-ledger` sin confirm; el lector que
resuelve la ruta sin `Abs`.

## 4 · Tests aprobados que cambian

`TestContract_theHeaderArithmeticIsTrue` (16/6/10/5 → 17/7/10/6, por diseño);
`whatsDoorPaths` (+1); los dobles del libro ganan `AdoptLedger`/`Standing`;
`recorderWith`/`realRecorder` pasan la identidad. Ningún aserto se relaja.

## 5 · Lo no verificable, declarado

Adopción desde la CLI (fichada); symlink (tren siguiente); binario en proceso OS
aparte; la app empaquetada.


---

## 6 · Delta tras la lectura adversaria interna (4 min, VETO MANTENIDO: 2 P1, 2 P2)

| # | Hallazgo | Dónde se plegó |
|---|---|---|
| 1 P1 | «todo acto nuevo» con cinco puertas guardadas: aparcar por aprobación y crear intentos no pasaban por ninguna | D3 (guarda en cada sitio que inserta), L3, G8 ampliada, molde ESTRUCTURAL que camina el fuente |
| 2 P1 | estado en memoria por handle, solo al abrir: la CLI escribía libre; un app vivo no se enteraba de la adopción de otro | D2 (estado RECALCULADO en cada escritura), D1 (la CLI fija la identidad desde `--config`), L6, G10, G11 |
| 3 P2 | el app que funda se queda en `legacy` hasta reiniciar; `actLedger` sin `FinishWithResult` | D2 (recalculado), D5 (`FinishFounding` en `actLedger`), G6 ampliada |
| 4 P2 | `-config` relativo: identidad por cwd | D1 (`Abs` + `EvalSymlinks` + minúsculas en Windows; identifica el fichero cargado) |
| 5 P3 | el prefijo `profile:` no estaba reservado | D5 (`FinishFounding` única puerta; `FinishWithResult` rehúsa el prefijo), G9 |
| 6 P3 | sin sellador el marcador se perdía en silencio | D4/D5 (`ErrNoSealer`), G4c |
| 7 P2 [INSTRUMENT] | G4 either/or; G7 doble; G8 «no abre transacción» sin instrumento | G4a/G4b/G4c; G7 con app real; G8 «antes de cualquier escritura» con triggers de aborto |
| 8 P3 [INSTRUMENT] | condición por cerebro; con cero cerebros nada | D7 (`LedgerStanding` del perfil en el gate) |
| 9 P3 | la recuperación escribe recibos antes de la comparación | D3 (cierres, no actos; declarado) |

Verificado a favor por la lectura: `result_digest` está dentro del material
firmado; `Prune` no borra recibos; partición única `main`.


---

## 7 · Canto: garantía → molde que la rompe → mutación ejecutada → nivel de evidencia

| Garantía | Molde | Mutación (roja, capturada) | Nivel |
|---|---|---|---|
| L1 el libro fundado lleva la marca del fundador en el recibo del acto fundacional; un cierre `FAILED` no marca | `TestCreateLedger_theFoundingCloseMarksTheFounder`, `TestBootstrap_theFoundedLedgerKnowsItsProfile` | M110 | store real; cutover REAL + CLI in-process |
| L2 movido/copiado/otro perfil → `ledger_foreign_profile`, nombrado en los tres lectores | `TestStanding_aMovedProfileIsForeign`, `…aCopiedProfileIsForeignAndTheOriginalIsNot` (dos conexiones), `TestLedgerCheck_namesTheStanding`, `TestReceiptVerify_namesTheStanding`, `TestMount_aForeignLedgerRefusesEveryDoorButAdopt` | M97, M105, M106, M109, M112 | store real; CLI; app real HTTP |
| L3 con `foreign` toda puerta que crea actos rehúsa ANTES de cualquier escritura; las lecturas siguen | `TestForeign_everyActDoorRefusesBeforeAnyWrite` (16 puertas, triggers de aborto sobre INSERT y, para el ciclo de vida de los contratos, sobre UPDATE), `TestForeign_everyInsertSiteIsGuarded` (estructural, por sentencias de primer nivel), `TestAct_aForeignLedgerRefusesEveryDoor`, `TestConfigAct_aForeignLedgerRefusesTheBuilderDoor`, `TestCLI_aForeignLedgerRefusesEveryWriter` (`intent create` y `receipt rotate-key`), `TestCLI_everyStoreOpenSetsTheProfileIdentity` (estructural) | M97, M100, M107, M111, M123, M124-bis, M128–M132 | store real + 2ª conexión; estructural; httptest; CLI |
| L4 la adopción es UNA transacción con recibo marcado, o nada | `TestAdopt_theBookHasOneOwnerAtATime`, `…anUnsignedReceiptLeavesNothing`, `…anAbortedReceiptRollsTheActBack`, `…refusesWithoutASealer`, `TestAdopt_theDoorAdoptsAndAnswersTheReceipt`, `TestAdopt_needsConfirmation` | M101, M102, M108 | store real (+trigger por 2ª conexión); httptest |
| L5 sin marca → `legacy_unfounded`, no bloquea | `TestStanding_aLedgerWithNoMarkIsLegacy`, `TestMount_aLedgerWithNoMarkIsLegacy`, molde TS | M99, M116 | store real; app real; jsdom |
| L6 un dueño a la vez, visto por un handle vivo en su siguiente escritura | `TestAdopt_aLiveOwnerLosesTheBookOnItsNextWrite` (dos conexiones), `…theBookHasOneOwnerAtATime` | M98-bis (M98 fue un `[build failed]`, repetida), M96 | store real + 2ª conexión |
| el prefijo del marcador es de las dos puertas; sin sellador no se marca | `TestMark_theProfilePrefixIsReservedToTheFoundingDoors` | M103, M104 | store real |
| la pantalla: fila «Libro de otro perfil» con «Adoptar libro» tras confirmación; legacy como texto; el rechazo nombra la salida | tres moldes TS | M113, M114, M115, M116 | jsdom |

Mutaciones del marcador: el recuento vive en §9, derivado del fichero con su
regla escrita (la oficial halló esta cifra desfasada dos veces: 42 ejecutadas,
4 que no compilaban y se repitieron en rojo, 38 rojas). El corredor distingue
un `[build failed]` de un rojo desde M93-bis.

**Las cinco preguntas:** (1) frases: «toda puerta que crea actos» está sostenida
por el molde estructural más la tabla de dieciséis; «un dueño a la vez» por dos
conexiones reales; «nunca» no aparece sin molde — sí se declara lo que no
distingue (copia en la misma ruta, symlink por destino). (2) cada test citado
existe con ese nombre (grep al cerrar). (3) los moldes del store entran por los
métodos exportados; el estructural camina el fuente, declarado. (4) las
hermanas del bloqueo son las dieciséis puertas de la tabla; en la CLI,
`intent create` y `receipt rotate-key` probados, y el paseo por sentencias de
la CLI exige la identidad en toda función que abra el store (`grant`,
`authority`, `approvals` comparten `openOperatorStoreSealed`, la costura
probada — declarado, no probadas una a una). (5) ninguna cura sin molde y rojo.


---

## 8 · Delta tras la pasada interna sobre el diff (10 min, VETO LEVANTADO)

Ningún P1/P2 de producto. Un P2 de instrumento y diez P3, adjudicados:

| # | Hallazgo | Cura |
|---|---|---|
| 1 P2 [INSTRUMENT] | el canto contaba M103 y M105 como rojas y eran `[build failed]` (la tanda corrió antes de corregir el corredor) | repetidas como M103-bis y M105-bis, rojas; recuento corregido |
| 2 P3 | `ActivateIntentV2`, `RevokeIntentV2`/`ExpireIntentV2` (`transitionIntentV2`) y `PutExecutionBinding` escribían recibos en un libro ajeno, y `transitionContract` movía el estado de un contrato (UPDATE, sin recibo) — sus llamadores de producción sellan antes por una puerta guardada; el e2e-harness no | guarda en los cuatro; tres filas más en la tabla (de 11 a 14: `ActivateIntentV2`, `RevokeIntentV2`, `PutExecutionBinding`); `transitionContract` quedó guardado SIN fila propia — la oficial lo halló (§9, fila 2) y hoy tiene dos (16); M118, M119 |
| 3 P3 | `LIKE` pliega mayúsculas y `HasPrefix` no: `PROFILE:` dejaba el libro ajeno para todos | la refusal es insensible a mayúsculas; el lector solo acepta el prefijo exacto; molde ampliado; M117 |
| 4 P3 | `ownerMark` escanea `receipts` en cada escritura (44 ms sobre 200k recibos) | caché por handle ATADA a la longitud de la cadena (un `MAX(chain_seq)` indexado): solo rescanea si llegó un recibo; nunca un veredicto ciego (M121 lo enrojece; M98-bis sigue rojo) |
| 5 P3 | la guarda corre fuera de la transacción: A pasa la guarda, B adopta y confirma, A inserta | DECLARADO y fichado: cerrar la ventana exige `BEGIN IMMEDIATE` en cada puerta de acto (la disciplina de un escritor por handle lo acota a dos handles distintos); L6 se cumple en su letra («en su siguiente escritura») |
| 6 P3 | macOS APFS insensible a mayúsculas: dos cajas, dos identidades | DECLARADO en §0 (solo Windows pliega; la CLI no adopta, sin daño) |
| 7 P3 | un fallo al leer la marca llegaba a la pantalla como «nada que decir» | `unreadable` con su causa en el recorder, la puerta de lectura y la pantalla; `TestConfigActRecorder_anUnreadableStandingIsNamed`, molde TS; M120, M122 |
| 8 P3 [INSTRUMENT] | el molde estructural leía comentarios, veía solo `INSERT INTO` literal y saltaba una entrada sin comprobar llamador | comentarios fuera; regex con `INSERT OR … INTO`, `intent_events`, `execution_bindings` y los appenders de recibo; sin entradas que salten |
| 9 P3 [INSTRUMENT] | el papel decía que el symlink no se resuelve; el código lo resuelve | §0 y §2 corregidos |
| 10 P3 [INSTRUMENT] | `printLedgerStanding` con un parámetro muerto | firma `(ctx, out, store)` |
| 11 P3 | el builder pinta el 409 como «save failed (HTTP 409): <message>» | el mensaje nombra «Adoptar libro» y la pantalla; tratamiento propio fichado |

---

## 9 · Delta tras la oficial acotada (ronda 1, 30 min, VETO MANTENIDO → curado)

Un P2 de producto y siete P3. Lo que esta pasada vio y la interna no: una
puerta de la CLI que abría el store por su cuenta, fuera de la costura probada.

| # | Hallazgo | Cura |
|---|---|---|
| 1 P2 [PRODUCTO] | `korvun receipt rotate-key` abría el store con `actionsqlite.OpenOperator` directo, sin identidad: desde un perfil copiado registraba un acto y rotaba la clave de un libro ajeno (captura de la oficial: exit 0, `actions` 2 → 3, `receipts` 2 → 3) | fija la identidad desde `--config` tras abrir (`internal/cli/receipt.go`, «The durable mark: this handle serves the profile named by --config»); fila `receipt rotate-key` en `TestCLI_aForeignLedgerRefusesEveryWriter`, con oráculo sobre los bytes de la clave del fundador y la rotación del fundador aún en pie; molde estructural nuevo `TestCLI_everyStoreOpenSetsTheProfileIdentity` (árbol sintáctico: toda función de la CLI que abra el store por cualquiera de sus tres puertas fija la identidad en la forma guardada, como sentencia de primer nivel del cuerpo); M123, M127, M131, M132 |
| 2 P3 | `transitionContract` guardado sin molde: el paseo estructural mira sitios de INSERT y un UPDATE no lo ve | dos filas (`TransitionIntent`, `TransitionGrant`) sobre contratos nacidos DRAFT que escribió el fundador, con triggers de aborto sobre UPDATE en `intents` y `grants`; M124 enrojeció por la arista ilegal («"" → ACTIVE», contratos sin estado), no por el trigger — declarado y repetido como M124-bis, que llega al UPDATE y muere en él |
| 3 P3 [INSTRUMENT] | la aritmética del §7 (30 / 27 / «tres») estaba desfasada y decía «12 puertas» sobre una tabla de 14 | recuento derivado del fichero con la regla escrita (abajo); la tabla tiene 16 filas y el §7 lo dice |
| 4 P3 [INSTRUMENT] | §8 fila 2: «los cuatro escribían recibos» era falso para `transitionContract` (solo UPDATE, sin recibo) y «12 → 15… 14» no cuadraba | fila reescrita: de 11 a 14, con `transitionContract` guardado sin fila propia hasta esta ronda |
| 5 P3 | la pantalla prometía «el primer acto de adopción o de activación lo marcará» sobre un libro legacy, que no tiene botón (la activación rehúsa con almacén; la adopción solo se ofrece sobre un libro ajeno) | «No bloquea nada, y esta pantalla no ofrece marcarlo»; el molde TS fija la frase nueva y prohíbe la anterior |
| 6 P3 | HANDOFF («nunca se cachea») y el godoc de `Standing` («one query») contradecían la caché atada a `MAX(chain_seq)` | ambos dicen lo que hace el código: el veredicto no se cachea; la marca se relee cuando la cadena creció desde la última mirada del handle |
| 7 P3 [INSTRUMENT] | el paseo estructural del store: clave por nombre (un método y una función podían confundirse), guarda aceptada en cualquier posición y en cualquier forma (`_ = s.refuseIfForeign(ctx)` pasaba) | clave `Receptor.nombre` con detección de duplicados; la guarda cuenta solo como sentencia de PRIMER NIVEL del cuerpo, en su forma con retorno, y ANTES de la sentencia que escribe (o de la llamada al helper que escribe); M128, M129, M130 |
| 8 P3 | privacidad: la marca es un digest de una ruta absoluta de baja entropía (lleva el nombre de cuenta) y viaja en el recibo, en `ledger check`, en `receipt verify` y por la puerta de lectura, sin declararlo | DECLARADO en el godoc de `ProfileIdentity`, en las notas de la release y aquí: no es un secreto ni una frontera de privacidad; confirma una ruta adivinada; llega a los lectores del propio operador (la CLI y la puerta de lectura del servidor de administración, en loopback por defecto) y, cuando a un cerebro se le rehúsa un acto, al log estructurado del servidor con los dos digests (la ronda 2 acotó las dos frases, §10); la ruta en sí nunca se escribe en el libro |

**Lo que la mutación halló y ninguna pasada vio:** M125 (la guarda de
`CreateGrant` metida en un `defer` al inicio del cuerpo) enrojeció la tabla de
puertas —el trigger la vio— pero NO el paseo estructural por texto: la guarda
estaba, en el texto, antes del INSERT. El paseo se reescribió sobre las
sentencias de primer nivel del cuerpo (una clausura diferida no es una
sentencia `if`), y M128 repite la forma y enrojece los dos moldes. El paseo de
la CLI nació con la misma regla (M131).

**Declarado, no curado:** la registración de identidad de la CLI
(`wireOperatorIdentity` → `RegisterIdentity`, tablas `principals`,
`principal_events` y `principal_bindings`) corre antes del rechazo y no está
tras la guarda. No escribe acto ni recibo y es idempotente (inserta solo sobre
`sql.ErrNoRows`); que un libro ajeno abierto por una CLI cuyo principal no
conozca lo registraría antes de rehusar el acto es predicción por lectura, no
ejecutada (la ronda 2 lo verificó por lectura y lo dejó así nombrado). Fichado
en el HANDOFF.

**Recuento** (derivado de `evidence/v0.16.2/mutations.txt` con esta regla:
cabecera `Mn ·` desde M96; «no aplicada» = `ANCHOR COUNT`; «declarada» =
`DECLARADA SIN ROJO ALCANZABLE`, no ejecutada; «no compila» = `[build failed]`
en la captura del comando, no en un título ni en la prosa; roja = el resto):
a la salida de la ronda 1: 43 cabeceras, 1 no aplicada (M126, ancla no
hallada, repetida como M126-bis), 42 ejecutadas, 4 no compilaban (M98, M103,
M105, M121, repetidas en rojo como `-bis`), 38 rojas — dos con salvedad
declarada arriba (M124, M125). El recuento tras la ronda 2 está en §10.

---

## 10 · Delta tras la oficial acotada (ronda 2, VETO MANTENIDO → curado)

Dos P2 de instrumento, cinco P3 (uno de producto). La regla que la ronda fijó
y aplicó: una forma que el paseo acepte Y que la tabla de puertas tampoco
vería es P2. Las dos se capturaron con escrituras reales sobre un libro ajeno
mientras el paseo seguía verde.

| # | Hallazgo | Cura |
|---|---|---|
| F1 P2 [INSTRUMENT] | el paseo del store era ciego a (A) un INSERT en un literal de función a nivel de paquete (`var x = func(...)`, un GenDecl que el paseo no visitaba) y (B) un TERCER llamador de un helper listado (el mapa nombraba UN llamador y solo miraba ese); las dos puertas escribieron dos actos en un libro ajeno con el paseo verde | `TestForeign_everyInsertSiteIsGuarded` reescrito: visita FuncDecl y literales de función a nivel de paquete; un helper se juzga por TODOS los llamadores que el paseo encuentra (el mapa es helper → llamadores permitidos SIN guarda, solo `Store.AdoptLedger` para `recordAuthenticatedTx`); un helper listado sin llamador, o un permitido que no lo llama, es entrada obsoleta y falla; y una cuenta de completitud: todo INSERT del fichero debe estar dentro de un cuerpo visitado, o el paseo falla nombrando el fichero; M142 (A), M143 (B) |
| F2 P2 [INSTRUMENT] | el paseo de la CLI guardaba por texto del identificador: alias de import (D1), abridor entre paréntesis (D2), referencia por valor (D3) y `SetProfileIdentity` sobre OTRO handle con cuerpo vacío (D4) pasaban; con dos filas en la tabla, un escritor nuevo por cualquiera de esas formas era invisible a los dos instrumentos | `TestCLI_everyStoreOpenSetsTheProfileIdentity` reescrito: el paquete del store se halla por RUTA de import (alias, nombre propio o dot import); toda referencia a un abridor, esté donde esté en el fichero, debe ser la única forma directa `handle, err := pkg.Open*(...)` como sentencia de primer nivel (paréntesis, valor o anidado se rehúsan como tales: el paseo no puede seguir el handle); la sentencia de primer nivel siguiente al open (pasados su `if err != nil` y el `defer` de cierre del handle) debe ser `if err := handle.SetProfileIdentity(app.ProfileIdentity(...)); err != nil { … return … }` sobre ESE handle, con argumento calculado y cuerpo con retorno; cuenta de completitud sobre las referencias del fichero; M144 (D1), M145 (D2), M146 (D3), M147 (D4), M148 (literal a nivel de paquete) |
| F3 P3 [INSTRUMENT] | `guardIf` terminaba en `return\b`: `return nil` pasaba el paseo (la tabla lo veía) | la forma exige `return [<ceros>, ]err`; M133 |
| F4 P3 [INSTRUMENT] | el paseo de la CLI no juzgaba la posición: la identidad tras el acto pasaba (la tabla lo veía) | la regla de «sentencia siguiente al open»; M134 (movida al final), M140 (diferida), M141 (digest literal) |
| F5 P3 [PRODUCTO] | `scanOwnerMark` leía una marca con el prefijo en otra grafía (`PROFILE:`, que `LIKE` pliega) como AUSENCIA: el libro pasaba a `legacy_unfounded` y quien era ajeno un instante antes escribía; corrupción disfrazada de ausencia | `ErrLedgerMarkMalformed`, nombrado: la marca existe y no se lee; `Standing` lo devuelve (la CLI imprime `unreadable (…ledger_mark_malformed…)`, la pantalla `unreadable` con su causa) y `refuseIfForeign` lo devuelve, así que TODO acto rehúsa, el del dueño también, hasta juzgar la cadena; `TestMark_aMalformedMarkIsNamedNotAbsent` (segunda conexión real para la reescritura, handle fresco para juzgar, dos identidades, cero actos) y `TestLedgerCheck_aMalformedMarkIsUnreadable` (CLI in-process); M135. Límite declarado: la caché de un handle vivo, atada a `MAX(chain_seq)`, no ve una reescritura in situ —un UPDATE no alarga la cadena—; esa reescritura la denuncia el `hash_mismatch` de `ledger check`, no esta caché |
| F6 P3 [DOC] | «no sale de los lectores del propio operador» y «en loopback» eran más anchas que el cable: el rechazo lleva los dos digests al log estructurado del servidor cuando un cerebro es rehusado (`internal/brain/agent.go`, `a.logger.Warn("agent: action record failed", …)`), y la puerta de lectura es loopback por DEFECTO (bind del servidor de administración, elección del operador), no por construcción | godoc de `ProfileIdentity`, notas de la release, HANDOFF y §9 fila 8 acotados a eso |
| F7 P3 [DOC] | «tabla `principals`» son tres tablas (`principals`, `principal_events`, `principal_bindings`), y «sí se escribiría» era una predicción por lectura vendida como hecho | HANDOFF y §9 corregidos: tres tablas; sin acto ni recibo, idempotente sobre `sql.ErrNoRows`; la escritura de un principal desconocido en un libro ajeno queda nombrada como predicción por lectura, no ejecutada |

**Lo que esta ronda vio y la anterior no:** las cinco frases del veredicto de
la ronda 2, verbatim en el informe de sesión: los dos paseos por texto de
identificador y por nombre de llamador; la marca malformada como ausencia; el
log del servidor y el bind por defecto; las tres tablas.

**Verificado a favor por la ronda 2, con captura (binario compilado en
proceso aparte):** la reproducción literal de la ronda 1 —`receipt rotate-key`
desde el perfil copiado— termina en exit 1 nombrando `ledger_foreign_profile`,
con `actions`, `receipts`, `principals` y `signing_keys` sin crecer y la clave
del fundador con los mismos bytes; el fundador rota (exit 0). La aritmética de
§9 cuadró en las diez cifras por ejecución.

**Recuento** (misma regla que §9, sobre el fichero tras esta ronda): desde
M96, 59 cabeceras, 1 no aplicada (M126), 58 ejecutadas, 4 no compilaban (M98,
M103, M105, M121), 54 rojas — dos con salvedad declarada en §9 (M124, M125).
Del tren de hoy (desde M57): 102 cabeceras, 2 no aplicadas (M80, M126), 1
declarada (M79), 99 ejecutadas, 5 no compilaban (M93, M98, M103, M105, M121),
2 verdes que fueron hallazgo del instrumento y se repitieron en rojo (M66,
M84), 92 rojas.

---

## 11 · Delta tras la oficial acotada (ronda 3, VETO MANTENIDO → curado y adjudicado)

Dos P2 de instrumento (misma regla que la ronda 2), tres P3: uno de producto
del linaje de F5, uno de producto en la puerta HTTP de aprobaciones, uno de
doc. La reproducción literal de la ronda 2 (marca `PROFILE:`) terminó como la
regla 1 exige, con el binario real; «el del dueño también» verificado.

| # | Hallazgo | Cura / adjudicación |
|---|---|---|
| F1 P2 [INSTRUMENT] | el paseo del store juzgaba por TEXTO: `INSERT( OR \w+)? INTO tabla` en mayúsculas y con un espacio, y la guarda por la cadena `s.refuseIfForeign(ctx)` sin atar `s` al receptor. Seis grafías que SQLite acepta (minúsculas, `REPLACE INTO`, tabla entre comillas, `main.actions`, salto de línea, concatenación de literales) y una guarda sobre un `s` local con receptor `st` pasaban; dos actos en un libro ajeno con el paseo verde | `TestForeign_everyInsertSiteIsGuarded` reescrito sobre el árbol: el SQL se lee como lo lee SQLite —todo literal de cadena, y toda concatenación de literales plegada en una, casada sin distinguir mayúsculas contra `insert|replace [or …] into [esquema.] [«tabla»]` con cualquier blanco—; la guarda se reconoce como sentencia `if` de primer nivel con `Init` = `err := <receptor>.refuseIfForeign(ctx)` sobre el NOMBRE del receptor del método, condición `err != nil` y cuerpo exactamente `return [ceros,] err` (o `tx, err := <receptor>.beginAuthorityWrite(ctx)` seguido de ese `if`); los helpers se detectan por llamada en el árbol; la cuenta de completitud usa el mismo lector. M159–M164 (las seis grafías), M165 (receptor `st`), M166 (`const`), M167/M168 (F1-A/B), M150–M153 (quitar, diferir, `return nil`, `_ =`), todas rojas nombrando la puerta |
| F2 P2 [INSTRUMENT] | el `skip` de `if err != nil {` del paseo de la CLI saltaba una sentencia con rama `else` que escribía por el handle antes de la identidad; `app.ProfileIdentity` se reconocía por nombre del selector, no por ruta de import (un `zzFake.ProfileIdentity` pasaba); `return 0` y un `return` dentro de una clausura pasaban como salida de error | `TestCLI_everyStoreOpenSetsTheProfileIdentity` reescrito: los DOS paquetes (store y app) se hallan por ruta de import; entre el open y la identidad solo se admiten `if err != nil { … }` SIN `else` y sin mención del handle, y el `defer` de cierre del handle; el bloque de identidad exige `app.ProfileIdentity(...)` del paquete real, sin `else`, con un `return` directo como última sentencia del cuerpo que lleve `err` o un entero distinto de 0. M169 (`else` que escribe), M170 (`ProfileIdentity` ajeno), M156 (`return 0`), M157 (return en clausura), M171 (dot import), M172–M175 (D1–D4), M155/M158 (quitar, mover), todas rojas nombrando la función |
| F3 P3 [PRODUCTO] | `profile:` con digest VACÍO pasaba el prefijo exacto y se leía como dueño `""` → `legacy_unfounded` → quien era ajeno escribía; las dos puertas escritoras rehúsan un digest vacío, así que la marca nunca fue suya | `scanOwnerMark` nombra `ErrLedgerMarkMalformed` también con digest vacío; el molde `TestMark_aMalformedMarkIsNamedNotAbsent` es una tabla de dos manipulaciones (grafía, vacío), cada una en su store, con segunda conexión y handle fresco; M149 rojo |
| F4 P3 [PRODUCTO] | la puerta HTTP de aprobaciones (`writeApprovalError`) solo mapea sus tres centinelas y responde 500 «internal error» ante `ledger_foreign_profile` y `ledger_mark_malformed`: el bloqueo se cumple (nada escrito), el NOMBRE se pierde en esa puerta | FICHADO, no curado hoy: nombrarlo exige una entrada en el registro `outcomes` de aprobaciones, que está anclado en ambos sentidos con su especificación (§12-ter, FR-TEST-6) y es un tren propio; ninguna frase pública afirma que la puerta de aprobaciones nombre el libro ajeno (G7 acota el nombre a las puertas de configuración, la pantalla y la CLI). Ficha en el HANDOFF |
| F5 P3 [DOC] | el godoc de `ErrLedgerMarkMalformed` y el comentario del molde decían «un recibo» donde el cable juzga solo el ÚLTIMO recibo `SUCCEEDED` que `LIKE` halla: una marca malformada anterior a una bien formada, o en un recibo `FAILED`, se ignora (la cadena la denuncia) | los dos textos dicen exactamente eso |

**Lo que la mutación halló:** M154 quiso sombrear el receptor con un `s := …`
de primer nivel y NO COMPILA (Go declara el receptor en el bloque del cuerpo y
prohíbe redeclararlo ahí): la forma es imposible; el paseo llevaba una
comprobación para ella que nunca podía disparar y se retiró como código
muerto. La forma alcanzable (receptor `st`, guarda sobre un `s` local) es M165.

**Anotado por la ronda 3, fuera del delta, sin graduar:** `internal/app/config_act.go`
reabre por `OpenOperator` sin identidad en el reintento de fundación
(`createdHere`); ese handle sella el acto fundacional y materializa, si
faltan, el intento raíz (`ensureRootIntent` → `CreateIntent`), la clave de
firma (`ensureSigningKey`) y el registro de identidad (`RegisterIdentity`), y
se cierra en el mismo `CreateLedger` (`defer store.Close()`); no registra
otros actos (la ronda 4 corrigió la frase: un intento es una puerta que L3
guarda, y ese handle lo escribe sin identidad porque el libro es suyo desde
la creación). Leído, no ejecutado; fichado.

**Lo que esta ronda vio y la anterior no:** las siete frases del veredicto,
verbatim en el informe de sesión.

**Recuento** (regla de §9, sobre el fichero tras esta ronda): desde M96, 86
cabeceras, 1 no aplicada (M126), 85 ejecutadas, 5 no compilaban (M98, M103,
M105, M121, M154), 80 rojas — dos con salvedad declarada en §9 (M124, M125).
Del tren de hoy (desde M57): 129 cabeceras, 2 no aplicadas (M80, M126), 1
declarada (M79), 126 ejecutadas, 6 no compilaban (M93, M98, M103, M105, M121,
M154), 2 verdes que fueron hallazgo del instrumento y se repitieron en rojo
(M66, M84), 118 rojas.

---

## 12 · Delta tras la oficial acotada (ronda 4, VETO MANTENIDO → curado)

Un P2 de instrumento, cinco P3 (uno de producto, dos de instrumento, dos de
doc). Todo lo que §11 declara rojo se re-ejecutó sobre el árbol final por la
ronda y enrojece; la reproducción literal de la ronda 3 terminó como la
regla 1 exige, con el binario compilado.

| # | Hallazgo | Cura |
|---|---|---|
| F1 P2 [INSTRUMENT] | `writeSQL` no leía como SQLite dos cosas que SQLite lee como blanco o como esquema: un comentario de bloque o de línea entre tokens (`INSERT /*x*/ INTO`, `INSERT -- x⏎INTO`) y un esquema entrecomillado (`"main"."actions"`); tres actos en un libro ajeno con el paseo verde | el blanco entre tokens es `(?:\s|/\*.*?\*/|--[^\n]*(?:\n|$))+` y el esquema admite comillas, acentos graves o corchetes; M184, M185, M186 rojas nombrando la puerta, y M187 (`[actions]`), M188 (`WITH … INSERT`) también |
| F2 P3 [DOC] | «SQL assembled at runtime from non-literal parts (a table name in a variable)» no cubría `fmt.Sprintf` con partes literales ni una `const` como parte; `strings.Join` y `strings.Builder` enrojecían por accidente (los literales de una sentencia se leen unidos por `\n`) | el godoc dice exactamente eso: fuera de su vista queda todo SQL que no sea un literal o una cadena de `+` de literales (Sprintf, Join, Builder, const o var como parte); la unión por `\n` atrapa algunas de esas formas por accidente, no por diseño |
| F3 P3 [PRODUCTO] | el lector aceptaba como dueño cualquier sufijo no vacío tras `profile:`: una marca con basura (` `, un carácter de control, `xyz`) se nombraba `ledger_foreign_profile`, la CLI invitaba a adoptar y la pantalla ofrecería «Adoptar libro» sobre corrupción; el carácter de control salía crudo | la marca es canónica o malformada: el lector exige `sha256:` y 64 hex minúsculas (`canonicalDigest`) y nombra `ledger_mark_malformed` para todo lo demás; las DOS puertas que escriben la marca (`FinishFounding`, `AdoptLedger`) y el handle (`SetProfileIdentity`) rehúsan con `ErrProfileIdentityMalformed` toda identidad no canónica, así que el libro nunca lleva una marca que el lector tendría que adivinar; el molde de la marca malformada es una tabla de seis manipulaciones (grafía, vacío, blanco, control, basura, hex mayúsculas) y el de la identidad rehúsa cinco formas por nombre en el handle y en la fundación; M176 (lector), M177 (handle), M178 (fundación) rojas |
| F4 P3 [INSTRUMENT] | `app := zzFakeApp{}` antes del open sombreaba el nombre local del paquete y un `ProfileIdentity` falso pasaba el bloque canónico | el paseo de la CLI rehúsa toda función juzgada que DECLARE un nombre igual al nombre local de cualquiera de los dos paquetes (parámetro, `:=`, var, variable de `range`); M197 rojo nombrando la función y el nombre |
| F5 P3 [INSTRUMENT] | reasignación entre guarda y escritura: `s = otro` en el store, `h = otro` en la CLI; el paseo no ve flujo de datos | los dos paseos rehúsan toda REASIGNACIÓN (`=`, no `:=`) del receptor o del handle en cualquier punto del cuerpo: la guarda juzgó un store y la escritura podría llegar a otro; M179 (store), M180 (CLI) rojas |
| F6 P3 [DOC] | «ese handle solo sella el acto fundacional … no registra otros actos» era más estrecha que el cable: el handle sin identidad del reintento también materializa el intento raíz, la clave y el registro de identidad si faltan | §11 y el HANDOFF lo dicen así |

**Verificado a favor por la ronda 4, con captura:** M150–M153 y M159–M168
rojas sobre el árbol final; C1, C6–C9, D1–D4 rojas; nuevas rojas que la
ronda probó y el paseo ya rehusaba: `INSERT INTO [actions]`, `WITH … INSERT`,
una función plana con `s *Store` como parámetro que inserta, un helper con
`*sql.Tx` llamado por una guardada y otra sin guarda, `return -1`, `return 1,
nil`, un `defer` de cierre que escribe; un receptor por valor `(s Store)` pasa
el paseo pero la guarda es real y `go vet` lo denuncia (no es escape);
`OpenOperator` devuelve `nil, err` en toda salida de error. F4 de la ronda 3
(puerta de aprobaciones): 500 «internal error» verificado por `httptest`,
ficha exacta, ninguna frase pública afirma que esa puerta nombre el libro
ajeno.

**Notas de la ronda, atendidas:** las capturas de M159–M168 citan un número de
línea del molde anterior a la retirada de la comprobación muerta (M154); la
ronda las repitió sobre el árbol final y hoy se repitieron de nuevo como
M181–M196 sobre los paseos finales. M80 figura dos veces como cabecera (ancla
no hallada y repetición) sin `-bis`; el recuento cuenta ambas y cuadra.

**Lo que esta ronda vio y la anterior no:** las seis frases del veredicto,
verbatim en el informe de sesión.

**Recuento** (regla de §9, sobre el fichero tras esta ronda): desde M96, 116
cabeceras, 1 no aplicada (M126), 115 ejecutadas, 5 no compilaban (M98, M103,
M105, M121, M154), 110 rojas — dos con salvedad declarada en §9 (M124, M125).
Del tren de hoy (desde M57): 159 cabeceras, 2 no aplicadas (M80, M126), 1
declarada (M79), 156 ejecutadas, 6 no compilaban (M93, M98, M103, M105, M121,
M154), 2 verdes que fueron hallazgo del instrumento y se repitieron en rojo
(M66, M84), 148 rojas.

---

## 13 · Delta tras la oficial acotada (ronda 5, VETO MANTENIDO → curado)

Dos P2 —uno de producto, uno de instrumento— y dos P3 de instrumento. Lo que
§12 declara curado se re-ejecutó por la ronda y enrojece; la reproducción
literal de la ronda 4 terminó como la regla 1 exige, con el binario compilado;
ningún llamador de producción pasa otra identidad que `app.ProfileIdentity`
(verificado por la ronda, sin P1).

| # | Hallazgo | Cura |
|---|---|---|
| F2 P2 [PRODUCTO] | `AdoptLedger` —el único acto que un libro AJENO admite— no juzgaba la marca: sobre una marca MALFORMADA, un POST confirmado a `adopt-ledger` respondía 200 `adopted`, el estado pasaba de `unreadable` a `ok` y toda puerta volvía a aplicar; la corrupción quedaba enterrada bajo una marca válida (capturado en el store y en una app real por HTTP). Las frases «bloquea todo acto nuevo, también al dueño, hasta juzgar la cadena» y «no door offers to adopt it» eran falsas en el cable | `AdoptLedger` juzga la marca (`ownerMark`) antes de abrir la transacción y devuelve `ErrLedgerMarkMalformed` (o el error de lectura) por nombre; la puerta HTTP lo entrega como 503 `act_not_recorded` con la causa, y el estado sigue `unreadable`. Moldes: la tabla de seis manipulaciones de `TestMark_aMalformedMarkIsNamedNotAbsent` gana la pata de adopción por el handle sellado (cero actos, cero recibos, estado intacto), y `TestMount_aMalformedMarkRefusesAdoption` en `internal/app` con app REAL y servidor de administración en loopback (GET `unreadable` → POST confirmado 503 nombrando `ledger_mark_malformed` → GET sigue `unreadable`, cero recargas). M206 rojo en los dos (la app capturó el 200 `adopted` de la forma anterior) |
| F1 P2 [INSTRUMENT] | SQLite acepta un literal entre comillas SIMPLES donde espera un identificador (`INSERT INTO 'actions'`, `'main'.actions`, `'main'.'actions'`) y `writeSQL` no lo leía: un acto en un libro ajeno con el paseo verde. El godoc «in any spelling SQLite accepts» era más ancho que el cable | la comilla simple entra en las clases de apertura y cierre del esquema y de la tabla; el godoc del molde y el de `writeSQL` dicen ahora que el paseo lee las grafías que las pasadas hallaron y para las que se hizo (cuatro clases de comillas, blancos, comentarios), y que lo que SQLite pueda aceptar más allá NO se afirma; M211, M212, M213 rojas nombrando la puerta |
| F3 P3 [INSTRUMENT] | un alias de tipo local (`type app = zzFakeT`) sombreaba el nombre del paquete por debajo de `declares` (que miraba campos, `:=`, `var`, `range`) | `declares` mira también `TypeSpec`; M215 rojo nombrando la función y el nombre |
| F4 P3 [INSTRUMENT] | la reasignación del handle o del receptor A TRAVÉS DE UN PUNTERO (`p := &s; *p = otro`; `zzSwap(&store)` con `**Store`) era invisible a `reassigns`/`reassignsName` (solo el `Lhs` con nombre) | los dos paseos rehúsan además que se TOME LA DIRECCIÓN del receptor o del handle (`&s`, `&store`) en cualquier punto del cuerpo: un puntero, una clausura o un callee podrían reasignarlo; M207 (store, puntero), M208 (CLI, puntero), M214 (store, callee), M216 (CLI, callee) rojas |

**Verificado a favor por la ronda 5, con captura:** las tres formas de F1 de la
ronda 4 rojas; `'profile: '`, `'profile:' || char(1)` y hex en mayúsculas →
`unreadable (…ledger_mark_malformed…)` con el binario; un digest canónico que
no nombra a nadie (64 ceros) → `ledger_foreign_profile` con dueño imposible,
cierre en fallo por diseño (indistinguible de otro perfil real; `ledger check`
denuncia la reescritura); `app.ProfileIdentity` siempre canónica (el `ToLower`
de Windows es sobre la ruta, antes del hash); ningún llamador de producción
pasa otra cosa; `unreadable` sin botón en la pantalla y en el molde TS; 35
grafías de SQL contra el driver de producción, con `temp.actions` y `actions$`
como falsos positivos en la dirección segura; sombrear con una etiqueta no
sombrea (correcto).

**Lo que esta ronda vio y la anterior no:** las cuatro frases del veredicto,
verbatim en el informe de sesión.

**Recuento** (regla de §9, sobre el fichero tras esta ronda): desde M96, 127
cabeceras, 1 no aplicada (M126), 126 ejecutadas, 5 no compilaban (M98, M103,
M105, M121, M154), 121 rojas — dos con salvedad declarada en §9 (M124, M125).
Del tren de hoy (desde M57): 170 cabeceras, 2 no aplicadas (M80, M126), 1
declarada (M79), 167 ejecutadas, 6 no compilaban (M93, M98, M103, M105, M121,
M154), 2 verdes que fueron hallazgo del instrumento y se repitieron en rojo
(M66, M84), 159 rojas.

---

## 14 · Delta tras la oficial acotada (ronda 6, VETO MANTENIDO → curado)

Un P2 de producto, dos P3 de instrumento, un P3 de doc. Lo que §13 declara
curado se re-ejecutó por la ronda y enrojece (M206 en el store y en la app);
las comillas simples, el alias de tipo y las direcciones tomadas, rojas.

| # | Hallazgo | Cura |
|---|---|---|
| F1 P2 [PRODUCTO] | la guarda de la adopción (ronda 5) leía la marca desde la CACHÉ del handle (`ownerMark`, atada a `MAX(chain_seq)`): en la secuencia real del operador —la pantalla lee (y el handle cachea al fundador), la marca se reescribe in situ, se pulsa «Adoptar libro»— el POST confirmado seguía respondiendo 200 `adopted` y el estado pasaba a `ok`; capturado en el store y en una app real. La adopción es el único acto cuya ceguera sería permanente: su propio recibo válido tapa la marca rota para todo handle. El molde de la app curaba solo el orden en que la caché está vacía | `AdoptLedger` lee la marca SIN caché y DENTRO de su propia transacción (`scanOwnerMarkIn(ctx, tx)`, sobre la seam `rowQuerier` que el store ya tenía para las aprobaciones), así que una reescritura posterior a la última mirada del handle rehúsa igual; la lectura del ESTADO conserva su caché y su límite declarado. Moldes: la tabla de seis manipulaciones del store hace ahora la secuencia real —el handle vivo juzga ANTES de la reescritura (cachea al fundador), la reescritura entra por segunda conexión, el estado cacheado sigue diciendo «ajeno» (declarado) y la adopción por ESE handle rehúsa por nombre; los estados frescos se juzgan con un handle por identidad—; `TestMount_aMalformedMarkRefusesAdoption` tiene dos órdenes: «reescrita antes de que la app mirase» (GET `unreadable`) y «reescrita después» (GET `ajeno` con el botón, reescritura, GET sigue `ajeno` por la caché, POST 503 nombrando `ledger_mark_malformed`, cero actos, cero recibos, cero recargas). M217-bis rojo en los dos (la app capturó el 200 `adopted`); M218 (adopción sin lectura) rojo en los dos |
| F2 P3 [INSTRUMENT] | un SEGUNDO `SetProfileIdentity` tras el bloque canónico —directo con digest fijo, o en un callee que recibe el puntero— era invisible al paseo de la CLI; con él, un libro ajeno rotó su clave con el paseo verde | cerrado a nivel de PRODUCTO, no solo de instrumento: un handle sirve a un perfil toda su vida (`SetProfileIdentity` es de una vez: la misma identidad es no-op, una distinta rehúsa `ErrProfileIdentityAlreadySet` y la primera queda), así que la segunda identidad —en el cuerpo o en cualquier callee— la rehúsa el store; el paseo exige además exactamente UNA llamada en el cuerpo y declara que la del callee es del store. Molde: la puerta de identidad rehúsa la segunda y conserva la primera; M219 (el handle acepta la segunda) y M220 (segunda llamada en `rotate-key`) rojas |
| F3 P3 [INSTRUMENT] | el CAMPO del receptor reasignado (`s.db = otro`) o el puntero cedido sin `&` a un callee que lo reata pasaban el paseo del store con guarda correcta; un acto aterrizó en otro libro | `reassigns` rehúsa también un `Lhs` que sea campo del receptor (`s.db = …`); el callee que recibe `s` y reata un campo se DECLARA como fuera de la vista del paseo (el receptor es un puntero) y de la revisión; M221 rojo |
| F4 P3 [DOC] | «hasta que `korvun ledger check` juzgue la cadena» prometía un remedio que no existe: `ledger check` solo lee, ninguna puerta reescribe la marca | notas de la release, HANDOFF y godoc de `ErrLedgerMarkMalformed` dicen: bloquea todo acto nuevo, también al dueño y también la adopción; `ledger check` nombra el recibo roto; solo restaurar el libro desde una copia lo levanta, ninguna puerta lo repara. *Superado (tren E, tandas 4 y 5): las notas y el godoc ya no dicen «solo restaurar»; el remedio de `ledger_unreadable` es sustituir el fichero (docs/operations/ledger-restore.md).* |

**Lo que la mutación halló:** M217 puso la lectura cacheada DENTRO de la
transacción y BLOQUEÓ (con `SetMaxOpenConns(1)` la consulta por el pool espera
a la única conexión, que tiene la transacción) hasta el timeout de `go test`:
ni rojo ni verde, un bloqueo de la forma mutada; declarada y repetida como
M217-bis con la forma exacta de la ronda 5 (lectura cacheada antes de la
transacción), roja en los dos moldes.

**Verificado a favor por la ronda 6, con captura:** la reproducción literal de
§13 (reescritura antes del `Build`) exacta en app real; adoptar un libro `ok`
(recibo redundante) o `legacy_unfounded` por la puerta HTTP se admite por
diseño (D4) y es coherente con la pantalla y las notas; `h2 := store` no es
escape (mismo puntero, misma identidad); `app` como parámetro de una clausura
se nombra (sobreestricto, dirección segura); 17 grafías más contra el driver
de producción sin hallar una aceptada-y-no-leída; `FinishFounding` con un solo
llamador de producción.

**Lo que esta ronda vio y la anterior no:** las cuatro frases del veredicto,
verbatim en el informe de sesión.

**Recuento** (regla de §9, sobre el fichero tras esta ronda): desde M96, 135
cabeceras, 1 no aplicada (M126), 134 ejecutadas, 5 no compilaban (M98, M103,
M105, M121, M154), 1 verde que fue bloqueo del instrumento y se repitió en
rojo (M217), 128 rojas — dos con salvedad declarada en §9 (M124, M125). Del
tren de hoy (desde M57): 178 cabeceras, 2 no aplicadas (M80, M126), 1
declarada (M79), 175 ejecutadas, 6 no compilaban (M93, M98, M103, M105, M121,
M154), 3 verdes que fueron hallazgo o bloqueo del instrumento y se repitieron
en rojo (M66, M84, M217), 166 rojas.
