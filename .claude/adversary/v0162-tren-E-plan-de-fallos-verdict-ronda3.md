VETO MANTENIDO

Objeto: `/Users/sebastianmorenosaavedra/Desktop/korvun.nosync/design-drafts/2026-09-25-tren-E-plan-de-fallos-copiloto.md` (v4, 322 líneas). `shasum -a 256` a las 17:10:02 → `262115a84a6511b0d7106ed7beb906d3e8a318b66e6148c816ed92eb78ed9d30`; a las 17:42:24 → el mismo. Coincide con el del copiloto.
Árbol: WT=`/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync` (rama `v0162-mockups`, sin upstream, HEAD `d20dea66…` = `origin/master`). Toda cita `internal/…`, `cmd/…` o `docs/…` es `$WT/…`. Driver: `$(go env GOMODCACHE)/modernc.org/sqlite@v1.59.0`. Go 1.26.6.
Método: la orden prohíbe tests, compilación y mutaciones, y la he cumplido. Todo desenlace dinámico va marcado «predicción por lectura». Lo demás sale de leer la línea citada o de un comando de solo lectura cuya salida copio.

## 1 · Adjudicaciones de la ronda 2 (§10 de la v4)

| Hallazgo | Veredicto | Evidencia |
|---|---|---|
| N1 | CIERRA | El arranque ya no funda (§3 l.42, G-E11). La reproducción de N1 muere en su paso 3. `internal/shell/ledger_standing_test.go:47-49` exige `SUCCEEDED` del acto fundacional, y FinishFounding lo sigue cerrando (leído). Los efectos nuevos de E44 van en R3-5 y R3-11. |
| N2 | CIERRA | E40 retirada, G-E7 acotada. `approval_read_class_test.go:133` sigue como estaba. |
| N3 | CIERRA | E41 retirada y §12 lo declara. Comando: `awk 'NR>=385 && NR<=458' internal/action/sqlite/store.go \| grep -c "IF NOT EXISTS"` → `6`. |
| N4 | CIERRA A MEDIAS | E04 es ahora un PIN alcanzable con mutación real (el `WHERE NOT EXISTS` de `store.go:1505`). Pero «el intercalado entre procesos va a E06-A», y E06-A es una carrera sin forzar (R3-10). |
| N5 | CIERRA | §3 corregida. `grep -rn AcquireProfileLock` → solo `app.go:355` y `receipt.go:447`. E43 (ii) cuadra con la lectura (sección 3). |
| N6 | CIERRA | E13-R abre una conexión nueva sobre un fichero 0444 y su código va declarado como predicción. E13-A usa un seam NUEVO. El caso del directorio se retira (`profilelock.go:34`, MkdirAll). |
| N7, N8 | CIERRAN (transferidos) | La tinta entera pasa al tren F, antes del tag. G-E12 se retira y ninguna frase de la v4 afirma nada de la tinta. |
| N9 | CIERRA A MEDIAS | La reproducción literal acaba ahora en `ledger_busy` 503. Pero G-E10 sigue prometiendo «ninguna clase cae en act_not_recorded», y la clase determinista no tiene nombre en ninguna superficie (R3-4 b). Además, el seam que usa E27 no se alcanza desde la app (R3-7). |
| N10 | CIERRA | E37 tiene cuatro puntos, con un desenlace cada uno. (1)-(3) → `ledger_exists` con la salida de `whats_happening.go:566-570`. |
| N11 | CIERRA | G-E9 ya no afirma nada de los bytes. Se retira la DSN lectora y E23 queda sin oráculo de bytes. |
| P3-a | CIERRA A MEDIAS | D2 queda con su documento y su molde (E24). Siguen sin la marca NUEVO los seams de AdoptLedger, del corte y de «tras el commit» (R3-7). |
| P3-b | CIERRA | Cinco llamadores y una sola función (`store.go:1424-1443`, `config_act.go:404`, `config_act_registry.go:396`, `receipt.go:463`). |
| P3-c | CIERRA | `authority.go:104,162,211,252` son cuatro `openOperatorStoreSealed`, leído. |
| P3-d | CIERRA | E16 y E30 llevan la etiqueta PIN. |
| P3-e | CIERRA A MEDIAS | §7-bis existe, pero sus respuestas a (b), (d) y (e) las contradicen R3-2, R3-8, R3-10, R3-6 y R3-11. |
| P3-f | CIERRA | D3 dice «vuelve a abrir». La maqueta UX-TEMPLATE queda en §13. |
| P3-g | CIERRA | E33 conserva el modo estricto de hoy. Una de sus dos citas no casa (R3-16). |
| P3-h | CIERRA | E28 retirada y el `link` va al tren F. |
| P3-i | CIERRA | ENOENT del lector → `ErrNoActionStore`. El añadido «o de la CLI» es nuevo y no casa con el árbol (R3-14). |
| P3-j, P3-k, P3-l, P3-m, P3-n, P3-o, P3-p | CIERRAN | `driver.go:266-270` («connection hook: %w», y cierra la conexión). `store.go:102-104` (`version INTEGER NOT NULL`). `store.go:1898-1915` (poda y después barrido). E42 y E32 retiradas. |

**La tercera parte del §10, las 26 de la v3:** es cierta en todo salvo en H5.
- H2 CIERRA: E37(4) y el botón de D4′ curan el «legacy para siempre, sin botón». E37 (1)-(3) aceptan `ledger_exists` y lo declaran en §12.
- H5 CIERRA A MEDIAS: E08-R no tiene cable (R3-2).
- H1, H3, H4, H6, H7, H8, H9, H10, H11, H12, H13, H14, P3-5 y P3-11 CIERRAN. Los P3-1 a P3-4, P3-6 a P3-10 y P3-12 se conservan.

## 2 · Filas retiradas

| Fila | Qué moldeaba | ¿Queda molde o está acotada? |
|---|---|---|
| E28 | La durabilidad del perfil con `Sync` (H10) | El corte de luz queda fuera (§1, §12). El crash del proceso lo cubren E21 y E37 en sus puntos. El par temporal→rename del escritor del perfil no tiene fila, aunque §3 lo afirma como absoluto (R3-14). |
| E32 | Ninguna garantía | Bien retirada. |
| E35, E36 | G-E12, la tinta | G-E12 se retira en §2. Tren F. |
| E40 | El juicio de los triggers persistentes | G-E7 queda acotada y el límite está en §12. Sigue viva, sin molde, «Puede hacer fallar escrituras, nunca hacerlas pasar» (R3-18). |
| E41 | La versión reescrita | §12 la saca del modelo de amenaza. H5 pasa a E08, que falla (R3-2). |
| E42 | El escritorio ante un Build fallido | Fuera y NO VERIFICADO (§1, §12). |

## 3 · E38 y E44 contra el código (predicción por lectura)

- **E38.** `beginAdoption` admite un libro sin marca: `judgeIn` devuelve legacy sin error (`ledger_identity.go:203-226`). AdoptLedger escribe el acto, el cierre y el recibo `profile:<digest>`. Sin fila previa, el `INSERT` va por la rama sin conflicto, y deja `founded_by_action = adopted_by_action = env.ActionID` (`profile_standing.go:337-340`). Standing pasa a `ok`. El desenlace prometido sale. Lo que no está resuelto es el contrato de la pantalla (R3-1 c).
- **E44, mismo perfil.** FinishFounding entra por `beginWrite`. La DSN escritora lleva `_txlock=immediate` (`store.go:96`) y el driver emite «begin immediate» (`tx.go:23-24`). El juicio va dentro de la transacción (`ledger_identity.go:167-198`), y una fila que nombra al perfil del handle pasa. `finishWithResultTx` cierra `SUCCEEDED` con la marca (`profile_standing.go:269`), y `DO NOTHING` no toca la fila. Sale lo prometido. La mutación, el `INSERT` de hoy, choca con la PK (`store.go:689`), deshace la transacción y deja el acto AUTHORIZED. `finishThrough` lo anota (`config_act.go:222-234`): R rojo, válido.
- **Perfiles distintos.** Una fila ajena la rechaza `beginWrite` con `ErrLedgerForeignProfile` antes del `INSERT`. El acto fundacional queda AUTHORIZED y cada cierre posterior falla igual. Los arranques de P se saltan la recuperación sobre un libro ajeno (`app.go:416`, `:473-474`), así que solo lo cierra el arranque del dueño. No hay fila para esto. Con el candado del perfil solo se alcanza con dos ficheros de candado sobre un mismo libro (enlaces duros).
- **¿Oculta `DO NOTHING` una fila ajena o corrupta?** Con un handle que lleva identidad, no. Sí la oculta si el handle no tiene identidad o si `digest` ≠ identidad del handle (R3-11 ii).
- **¿Hay barrera real entre `buildAndStart` y `SettleAct`?** Hoy no hay ningún seam (`grep` de seams: solo `beforePruneDelete`, `beforeIdentityRow`, `openStandingSeam`, `openPruneSeam`, `hookShapeFault`). Y el cierre real no es `SettleAct` (R3-5).

## 4 · Las marcas [veredicto]

El plan dice en la línea 5 y en §10 que las acepta «por su comando y su salida». Solo N5 trae comando y salida; N2 lo trae abreviado con «…». Las demás son `fichero:línea`.

| Línea del plan | Afirmación | ¿Trae comando el veredicto? | ¿Casa en el worktree? |
|---|---|---|---|
| 25 | par poda→barrido de `noteWrite` | No (P3-n, fichero:línea) | Sí: `store.go:1906` poda, `:1912` barre |
| 39 | `receipt.go:447` | Sí (N5) | Sí |
| 41 | `receipt.go:463` | No (P3-b) | Sí, `OpenOperatorFor` |
| 123 | `ledger_shape_test.go:191–205` | No (N9) | El test sí existe (190-209). Pero `hookShapeFault` no se exporta (`ledger_identity.go:669`), y E27 no lo alcanza desde la app |
| 129 | `identity.go:433–434`, `:457–458` | No (P3-g) | 433-434 sí. **457-458 no**: es la declaración de `ErrAuthorityActivatedProfileNeedsStrict`, del arranque NO estricto |
| 134 | `authority.go:104,162,211,252` | No (P3-c) | Sí |
| 174 | «unos 52 triggers» | Abreviado (N2) | Mi comando `grep -rhoiE "CREATE TRIGGER[^\`\"]*? ON +[a-z_0-9]+" --include=*_test.go internal \| grep -viE TEMP \| wc -l` → `53` (approval_tombstones 2, no 1). «Unos» lo cubre. El «6» sin marca: mi comando da 6 |
| 197 | `internal/shell/ledger_standing_test.go:48–49` | Fichero:línea y fragmento (N1) | Sí |
| 197 | TestClaim… y los tests con triggers | Abreviado (N2) | Sí, `approval_read_class_test.go:133` |
| 197 | TestMigrate_theSeedKeepsARowAlreadyThere, `buildV11LegacyFile`, `repair_procedure_r13_test.go:126` | No (N3) | Sí: `ledger_shape_test.go:413`, `migration_r11_test.go:30`, `internal/cli/repair_procedure_r13_test.go:126` |
| §10 | «Por la salida de comando del veredicto: N6-a, P3-c, P3-g, P3-n» | No: N6-a es «predicción por lectura, semántica POSIX»; los otros tres, fichero:línea | — (R3-16) |

## 5 · Hallazgos nuevos

**R3-1 · P2 · [TEST-APROBADO-NO-DECLARADO][EITHER/OR] · §9, E27, §5, D4′.** El contrato de la v4 pone en rojo tests aprobados que §9 no nombra, y deja dos nombres para una misma situación.
- (a) `internal/app/profile_standing_test.go:441-442` exige que adopt-ledger sobre una identidad ilegible dé `503 act_not_recorded` nombrando `ledger_unreadable`. E27 y §5-bis dan en las seis puertas «Forma → `ledger_unreadable`, 503».
- (b) `internal/shell/bootstrap_test.go:292-293` («enable-storage over an unwritable dir … want 503 ledger_not_created») y `internal/app/config_act_registry_test.go:512-513` y `:900-901` (`ErrLedgerNotCreated`). §5 clasifica EACCES y ENOSPC como Entorno, y E27 exige `ledger_environment` en «las seis puertas», enable-storage incluida. Hoy `config_act.go:396-397` y `:411` envuelven en `ErrLedgerNotCreated`.
- (c) El botón de D4′: `internal/controlapi/whats_happening_contract_test.go:365-400` fija 17 filas, 7 con botón y 6 puertas, y exige esas cifras en palabras en `docs/releases/v0.16.2.md` (líneas 21, 24, 29, 177). El godoc de `whats_happening.go:13-15` dice «SEVENTEEN rows, SEVEN of them with a button». La pantalla dibuja los botones por `rowFor(rule)` (`WhatsHappening.tsx:520-534`). Añadir una fila cambia el test, el godoc y las notas. No añadirla deja un botón sin fila de contrato. El plan no decide.

Reproducción (predicción por lectura):
1. Implementar §5-bis tal cual para adopt-ledger.
2. Ejecutar `TestMount_anUnreadableIdentityRefusesAdoption` → rojo en `:441`.
3. Implementar «Entorno → ledger_environment» en enable-storage y ejecutar `TestCreateLedger_anUnwritableDirCreatesNoLedger` → rojo en `:512`.

Por qué P2: la ley del rojo aprobado pide declararlo antes, y es el mismo criterio que la ronda 2 aplicó a N1.

**R3-2 · P2 · [ORÁCULO][AFIRMACIÓN-FALSA][ADJUDICACIÓN-NO-CIERRA H5] · E08-R, G-E4, §7-bis (b).** El desenlace «ErrLedgerUnreadable («migration from v15: … no such table: receipts»); la app arranca bloqueada» no tiene cable.
- Hoy, un fallo de migración mata el arranque. `migrateStep` devuelve el error de la copia sin prefijo (`store.go:1256-1258`); `openWithIdentity` lo envuelve en «action/sqlite: migrate %q» y cierra (`store.go:1517-1521`); Build responde «app: open action store:» (`app.go:380-383`). El texto «migration from v15» solo aparece en errores de DDL (`store.go:1253`).
- §4 no declara abrir ilegible tras una migración fallida.
- Aunque se abriera, Standing recalcula la causa: `judgeIn` → `shapeOlder` → «schema v15, older than this binary's v16» (`profile_standing.go:174-175`). Es la clase (b), que §7-bis dice haber retirado.
- `ledger check` diría `ErrSchemaBehind` y «run the server boot to lift the schema» (`store.go:2107-2109`). Es falso: el arranque no puede subirla.

Reproducción (predicción por lectura):
1. Un v16 fundado.
2. `DROP TABLE receipts; UPDATE action_schema SET version = 15`.
3. `OpenFor` → error, y la app no arranca.
4. `korvun ledger check` → «schema is behind … run the server boot».

**R3-3 · P2 · [PLAN-FILA-AUSENTE][ORÁCULO] · `shapeSeedPartial`, E02, §11-Q4.** La forma nueva tiene cinco consumidores de `judgeShape`, y el plan solo nombra `openWithIdentity`.
- El gancho juzga la primera conexión antes que el abridor (`driver.go:266-270`; la primera consulta de `store.go:1497` crea la conexión).
- `judgeOnConn` manda toda forma no enumerada a `unreadable` (`ledger_identity.go:545-551`).
- Ese veredicto es pegajoso: «WHERE standing <> 'ledger_unreadable'» (`ledger_identity.go:268`).
- El oráculo de E02 («residuo completado; nada más cambia») no escribe, y la poda borra sin pasar por la guarda (`guardedTables` son solo INSERT/UPDATE, `:399-407`).

Reproducción (predicción por lectura):
1. Un residuo k=5 (las cinco sentencias, `action_schema` vacía) junto a las conversaciones.
2. `shapeSeedPartial` añadida a `judgeShape` y a `openWithIdentity`, pero no al gancho.
3. `OpenFor` completa el residuo y devuelve el handle: E02 queda verde.
4. `RecordAttempt` → «ledger_guard:ledger_unreadable». Build moriría en `ensureRootIntentIf`, y G-E1 («nunca lo juzga forma mala») sería falsa.

La sonda de la CLI (`store.go:1433-1441`), el lector (`:2103-2113`) y `judgeIn` tampoco tienen desenlace para esta forma.

**R3-4 · P2 · [SUPERFICIE-SIN-FILA][TAXONOMÍA] · G-E10, G-E3, §5-bis.**
- (a) La línea de estado de la CLI llama `ledger_unreadable` a cualquier error de Standing y no cambia el código de salida (`internal/cli/ledger.go:149-155`; también en `receipt.go:123`). Reproducción (predicción por lectura): con el seam NUEVO de IOERR de E13-A, `korvun ledger check --config p` imprime «ledger standing: ledger_unreadable (…IOERR…)» y sale con 0. §5-bis promete `ledger_environment` y salida ≠ 0. Ninguna fila cubre este caso.
- (b) La clase determinista (y la guarda con `ledger_guard_unset`) no tiene fila en §5-bis. Las puertas solo mapean `ErrNoLedger` y `ErrLedgerForeign` (`whats_happening.go:465-485`, `:664-677`). Reproducción (predicción por lectura): `CREATE TRIGGER t BEFORE INSERT ON actions BEGIN SELECT RAISE(ABORT,'x'); END;`, que G-E7 declara posible; luego `POST /api/whats-happening/enable-approvals {"confirm":true}` → 1811 sin `ledger_guard:` → determinista → `act_not_recorded` 503 con texto del driver. G-E10 dice «Ninguna clase cae en act_not_recorded».

**R3-5 · P2 · [AFIRMACIÓN-FALSA][MUTACIÓN-SOBREVIVE] · §3 «Orden de Activar almacén», E44-A, E21(3), E37(4).** Con el supervisor real, el cierre no es `SettleAct` en `whats_happening.go:625`.
- `RequestReload` es asíncrono (canal, `supervisor.go:383-409`), y `:622-626` solo cierra si el estado ya es terminal. En el éxito nunca lo es, porque el corte apaga primero la app que atiende la petición (`supervisor.go:257-262`).
- Cierra el observador: `persistConfig` → `setStatus` → `observe` (`supervisor.go:293-297`, `:441-453`) → `ObserveReload` (`config_act_registry.go:152-158`), cableado en `internal/shell/controller.go:274` y `internal/cli/serve.go:119`.

Reproducción (predicción por lectura):
1. Poner la barrera de E44-A «entre buildAndStart y SettleAct» donde la sitúa §3, antes de `:625`.
2. `POST enable-storage`. El observador cierra la fundación justo después de persistir.
3. `POST adopt-ledger` → UPSERT sin conflicto.
4. Con la mutación (el `INSERT` de hoy), E44-A queda verde.

**R3-6 · P2 · [MÁS-ANCHO-QUE-SU-CABLE] · G-E8 y `docs/releases/v0.16.2.md:103-104`.** Según G-E8, lo cierra «el arranque siguiente que abre ese libro». Pero un arranque para el que el libro es ajeno o ilegible se salta la recuperación: `app.go:416` (`ownsLedger`), `:473-474` («recovery: skipped…») y `store.go:1576`. E34 no lista la frase de las notas.

Reproducción (predicción por lectura):
1. Libro marcado por P en la ruta por defecto.
2. P pulsa enable-approvals y muere en el seam (1) de E21.
3. Arranca antes el escritorio D, con la plantilla y la misma ruta → su estado es `ledger_foreign_profile` → no hay recuperación.
4. `SELECT state FROM actions WHERE action_id=<acto>` → `AUTHORIZED`.

**R3-7 · P3 · [ADJUDICACIÓN-NO-CIERRA P3-a].** §11-Q2 dice estar completa, y siguen sin la marca NUEVO:
- los dos puntos de E20 (AdoptLedger no tiene seam, `profile_standing.go:294-348`);
- los tres de E21 y los puntos (2)-(4) de E37 (no hay seams en el corte);
- «tras el commit» de E19 y de E07.

E27 usa `hookShapeFault`, que no se exporta y solo sirve desde `internal/action/sqlite`. Su nivel «app real» necesita un seam nuevo.

**R3-8 · P3 · [MUTACIÓN-SOBREVIVE][COHERENCIA] · E03, E01, E07 (predicción por lectura).**
- E03: «aceptar 'abc' como 0» cae en `version < 1` (`ledger_shape.go:172-173`), con el mismo centinela. El oráculo solo pide `ErrLedgerUnreadable`, así que la mutación sobrevive.
- E01: con el seam dentro de la transacción, sacar la semilla fuera hace que el crash deshaga `createStmt`, y `sqlite_master` queda igual.
- E07: subir la versión fuera de la transacción deja igual el punto «dentro». El punto «tras» solo se ve si se sondea antes de reabrir, porque los pasos 13-15 son idempotentes.
- E07 además no es coherente: parte de un v12, R ataca 15→16 y espera «v12/v13». Su A («un paso con copy y post») no se alcanza desde un v12: `migrationsPost` solo tiene el paso 10 (`store.go:701-706`).

**R3-9 · P3 · [HISTORIA-DOBLE][MÁS-ANCHO-QUE-SU-CABLE] · `DO NOTHING`.**
- (i) Tras E44, `founded_by_action` nombra el acto de adopción, mientras el acto fundacional real cierra `SUCCEEDED` y con marca después. Ningún código de producción lee esas columnas (`grep founded_by_action|adopted_by_action` → solo `store.go:691-692` y los tres INSERT). Quedan más anchos que su cable: `docs/releases/v0.16.2.md:112-115` («sabe quién lo fundó… en la misma transacción que su recibo»), `profile_standing.go:10-11` y el comentario «The row and the receipt are one transaction» (`:277-278`). Ni §12 ni E34 lo declaran.
- (ii) La frase de §4 «beginWrite ya garantiza… que una fila existente nombra a este perfil» compara con la identidad del handle (`ledger_identity.go:172-175`), no con `digest`. FinishFounding no comprueba que coincidan, y un handle sin identidad no juzga. El test aprobado `profile_standing_test.go:86` llama a FinishFounding sobre `openFull`, sin identidad. En producción no se alcanza, porque los abridores exportados exigen identidad.

**R3-10 · P3 · [ORÁCULO][EITHER/OR] · E06-A.** «Dos procesos del SO que abren el mismo fichero fresco a la vez» es una carrera sin sincronizar, contra el punto 3 de la doctrina, y no da desenlace. G-E6 («esperan … o fallan con ledger_busy») es una disyuntiva. Es la respuesta del plan a N4.

**R3-11 · P3 · [TOCTOU][PLAN-FILA-AUSENTE] · G-E11 «con confirmación».** `beginAdoption` admite un libro ajeno (`ledger_identity.go:211`), y la petición no lleva el dueño que vio la pantalla (`whats_happening.go:287-292`). Un «Marcar» consentido sobre un libro sin marca puede quedarse con uno que otro perfil acaba de marcar. Se alcanza con dos ficheros de candado (enlaces duros), predicción. No hay fila. El fichero de decisiones UX del copiloto, de las 17:29 y fuera del objeto, describe el mismo hueco.

**R3-12 · P3 · [NIVEL-DE-EVIDENCIA].** E19, E24-A, E37(4) y E43(ii) afirman que «la pantalla ofrece Marcar» con niveles de proceso hijo, binario o dos procesos. §8 solo declara jsdom para E17, E26 y E38.

**R3-13 · P3 · [AFIRMACIÓN-FALSA] · §3, §5, G-E4.**
- §3 atribuye la persistencia del supervisor a `writeRawConfigAtomic`. El supervisor persiste con `WriteConfigAtomic` (`internal/supervisor/supervisor.go:488`); `writeRawConfigAtomic` es la mejora del shell (`internal/shell/upgrade.go:89`).
- §3 dice «db.Begin() … en la siembra», pero hoy la siembra son dos `Exec` en autocommit (`store.go:1501-1508`).
- «Si el proceso muere, el perfil queda entero … como mucho sobra un temporal» es un absoluto sin fila: ningún punto de E21 cae entre la escritura del temporal y el rename.
- §5, «ENOENT … de la CLI → ErrNoActionStore», contradice al abridor de operador, que siembra (`store.go:1428-1443`; test aprobado `store_c4_test.go:99`).
- G-E4 dice «más nueva que sea mala … arranca bloqueada», pero `judgeShape` no tiene «más nueva y mala», y un libro más nuevo mata el arranque (`store.go:720-723`).

**R3-14 · P3 · [VEREDICTO-SIN-COMANDO].** Las afirmaciones de procedencia de la línea 5 y de §10 son falsas para la mayoría de las marcas (sección 4). `identity.go:457-458` no respalda E33.

**R3-15 · P3 · [NORMA].** El plan leyó el CLAUDE.md principal. El del worktree añade, sin commitear, la séptima ley: prueba con un perfil REAL migrado de dos versiones atrás (`git -C $WT diff -- CLAUDE.md`, +25 líneas). Ninguna fila la cumple para D4′. HANDOFF (`docs/HANDOFF.md:440`) dice «ninguna ronda se abre sin el gate completo verde sobre el árbol que audita», y la v4 no declara el estado del gate. No lo ejecuté (lo prohíbe la orden).

**R3-16 · P3 · [TEXTO][RETIRADA-SIN-MOLDE].**
- D4′ dice «Korvun avisará cuando otro perfil lo abra». Marcar hace que se rehúsen los actos del otro perfil (`ledger_identity.go:184-189`; notas 124-128). Es un texto de consentimiento más estrecho que su efecto.
- «nunca hacerlas pasar», en G-E7, queda sin fila tras retirar E40.

## 6 · Lo que la v4 hace bien, verificado

- §1 es correcto. `git status --short | grep -c '^ M'` → 135, `grep -c '^??'` → 59, y `rev-parse` HEAD = origin/master = `d20dea66…`, sin upstream.
- El orden pre-RED se cumple. `git grep -E "classifySQLite|ErrLedgerEnvironment|ErrLedgerTransient|shapeSeedPartial|seedSeam|afterMigrateReadVersion|beforeMigrationCommit|ledger_environment|TestLedgerRestoreProcedure_byBinary"` → vacío, y tampoco hay `docs/operations/*restore*`.
- Casi todas las citas de §3 casan: 341, 348, 355, 392, 404, 439, 445, 1505, 1424-1443, 1435-1436, 689, 211, 167-190, 356, 474-476, `sql.go:1574-1583` y `supervisor.go:270`.
- Las cifras son correctas: 34 tablas, 24 outcomes y 6 `IF NOT EXISTS`.
- Retirar la fundación en el arranque, E40 y E41 casa con el árbol. E37 y E43 dan desenlaces únicos y coherentes con la lectura. E44-R es un molde válido.

## 7 · Alcance

- **Leído:** el plan v4 entero; los veredictos de las rondas 1 y 2 enteros. En el worktree, los ficheros siguientes enteros, o en los tramos citados: `profile_standing.go`, `ledger_identity.go`, `ledger_shape.go`, `store.go`, `config_act.go`, `config_act_registry.go`, `whats_happening.go`, `supervisor.go`, `app.go`, `cli/ledger.go`, `cli/intent.go`, `cli/receipt.go`, `profilelock.go`, `identity.go`, `controlapi/approvals.go`, `WhatsHappening.tsx` y su test, los tests citados, `v0.16.2.md` y `HANDOFF.md`. Del driver: `driver.go`, `error.go`, `conn.go` y `tx.go`. De la stdlib, `sql.go`. Como contexto, fuera del objeto: §15.6 y §23 del informe y el fichero de decisiones UX. No usé la instantánea.
- **Ejecutado:** solo comandos de lectura: `shasum`, `wc`, `git` (status, rev-parse, diff, grep), `grep`, `sed`, `awk`, `cmp`, `diff`, `ls`, `find`, `go env`, `go version`, y un `python3` que solo imprime líneas del plan. Ningún test, ninguna compilación, ninguna mutación.
- **No verificado (predicción):** todo desenlace dinámico (FULL, EXCLUSIVE frente a WAL, 0444, la trampa del gancho, los flujos de E43 y E44, el rojo de los tests aprobados) y el gate.
- **Sin examinar:** e2e-harness, el shell de escritorio, `internal/conversation/sqlite`, la spec §12-ter y la maqueta.
- **Tiempo:** 17:10 a 17:43, un poco por encima de los 30 minutos.
- **Lo que esta ronda vio y la anterior no:** R3-1 a R3-16. Nacen de la v4: `DO NOTHING`, D4′ y la barrera de E44. Ya estaban en la v3: G-E8, E07, E03 y el arranque de E08-R.

## 8 · Integridad (17:41:52 y 17:42:24)

- `git -C $WT diff | cmp - …/adv-e3-before.patch` → «DIFF: identical».
- `git -C $WT status --short | grep '^??' | sort | cmp - …/adv-e3-untracked-before.txt` → «UNTRACKED: identical».
- `git -C $WT diff --cached --quiet` → sale con 0.
- `shasum -a 256` del plan → `262115a84a6511b0d7106ed7beb906d3e8a318b66e6148c816ed92eb78ed9d30`.