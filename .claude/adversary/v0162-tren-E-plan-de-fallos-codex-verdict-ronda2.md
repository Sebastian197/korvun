VETO MANTENIDO

## Objeto

- Fichero: `/Users/sebastianmorenosaavedra/Desktop/korvun.nosync/design-drafts/2026-09-26-tren-E-plan-de-fallos-codex.md` (v2).
- Medición al empezar, 2026-09-26 13:03:50: 3112 líneas, 777348 bytes, acaba en `0a`, sha256 `e6432247cdfa705c19e8b3b3c26a4b54c0edc6603a6a8087db68e5bbe2dba3f0`, mtime 12:35:04. Coincide con la del ejecutor.
- Medición al terminar, 13:33:55: 3112 líneas, 777348 bytes, el mismo sha256 y el mismo mtime 12:35:04.
- Material, con hash comprobado por mí: veredicto de la ronda 1 `2d2e6433…` (30562 B), ronda 9 `50eaaefd…`, ronda 10 `1a5e657f…`, extracto §14.3/§14.6 `9c277079…`. Las dos copias de la v1 (`scratchpad/plan-codex-v1.md` y `/private/tmp/tren-e-v1-preserved.md`) dan `8effacbe651e3eac…4b330`.
- Árbol: WT `korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `d20dea6`, 135 M y 71 ??, índice vacío. El diff y la lista de ficheros sin seguimiento son idénticos a las referencias `adv-c2-*`.
- Método: la orden prohíbe tests, compilaciones y mutaciones, y no he ejecutado ninguno. Todo desenlace dinámico va como **predicción por lectura**. Lo que sí he ejecutado son comandos de solo lectura, con su salida citada.

## (a) La tabla «Ronda 1» del §11 (l.767–791) frente al árbol

Los rangos de «Source lines» de C1-1…C1-15 casan uno a uno con el veredicto de la ronda 1 (145–162 … 307–311). El recuento «4 P2 y 11 P3» es correcto. Veredicto por fila:

- **C1-1 · CURADO (predicción).**
  - Solo los dos centinelas de veredicto dan `unreadable`; lo demás va a `environment` o `unavailable` (l.26, l.40, l.160, TE48/55).
  - Las dos ediciones de tests aprobados están declaradas y casan con el árbol (config_act_registry_test.go:919-944, WhatsHappening.test.tsx:832-849).
  - La reproducción literal de la ronda 1 acaba ahora en `unavailable`/D3.
  - La CLI no la cubre esta fila: va aparte, en C2-2.
- **C1-2 · CIERRA A MEDIAS.**
  - La reproducción literal (Qcatalog, código 11 en pool-open-shape sobre `foundedFor`) ya es el desenlace esperado de l.278 (predicción).
  - Pero la tabla de un solo impacto no mira el fixture. Con el fixture obligatorio de Qresidue, tres filas no se alcanzan (C2-1).
- **C1-3 · CURADO (predicción).** `withLedgerConnection` adquiere la conexión antes de la consulta. El fallo del CREATE TEMP TABLE en el nacimiento sale de `db.Conn` como `connectionBirthError`: handle nil, clasificado.
- **C1-4 · CURADO (predicción).**
  - `ledgerKey` existe (config_act.go:108).
  - El GET recibe el grabador concreto sin envoltorio (app.go:668-695), así que la aserción de capacidad opcional funciona.
  - Los dobles act_fake_test.go:86 y act_external_fake_test.go:67 no se tocan.
- **C1-5 · CURADO (predicción).** Hay origen propio, adaptador de tx, tabla de estados y TE57 con plazo.
- **C1-6 · CURADO (predicción).** MU04 muere por el contador de despachos de B (≠0). MU37 va sobre la comprobación común ledger_shape.go:180-184.
- **C1-7 · CURADO.** TE28 cubre 2,4,7,9,12,15,16,17,18,19,21,25,27,28 y un código inesperado; «15 must fail».
- **C1-8 · CURADO (predicción).** Envoltorios privados. Las llamadas directas aprobadas de ledger_shape_test.go:322/326/354/383/446, ledger_hook_test.go:72 e identity_phase1_test.go:633/691/1230 siguen compilando.
- **C1-9 · CURADO (predicción).**
  - El prefijo de TE58 sale de identity.go:448-449.
  - `mapAuthorityStoreError` (authority_v2.go:700-708) no transforma el texto de E, porque no contiene «busy», «locked» ni «interrupted».
  - TE59 sigue identity.go:437-452.
- **C1-10 · CURADO (predicción).** Seis puertas (whats_happening.go:304-311), etiquetas en TSX:135-142 y canPress/confirm/trigger en :715/:772-773/:788-797, todo verificado.
- **C1-11 · NO CIERRA.** Declarado BLOQUEADO. Para `unavailable` propongo receta en (f); para `environment` no la hay determinista.
- **C1-12 · CURADO en el plan (con la UX pendiente).**
  - D2 es byte a byte el texto del copiloto (copiloto.md:351).
  - La frase añadida es cierta contra el código: conversaciones y libro usan el mismo `storagePath(cfg)` (app.go:380, app.go:887-893, comentario app.go:896-900).
  - D3-entorno termina en la frase de la decisión (l.384).
  - Nota menor: con la frase insertada dentro de D2, «D2 itself remains verbatim» (l.390) ya no es literal.
- **C1-13 · CURADO (predicción).** TE63/64. C16 se conserva (ledger_hook_test.go:231-257).
- **C1-14 · CURADO.** Godocs de guardHook y ErrLedgerBusy; SchemaVersion con sus 25 llamadas y 21 tests (verificado, ver (g)).
- **C1-15 · CURADO (predicción).** TE15 usa `rd`; TE61/62 cubren el paso 6 y TE67 el paso 5 nativo.
- **N9-1 · CURADO (predicción).** Los seis pasos del Anexo D son idénticos a la ronda 9, l.141-153 (`diff` → idénticos).
- **OBS-1 · CURADO, ahora dentro de E.** TE63/64.

## (b) Las 13 filas revisadas y TE55–TE67

- **TE04:** exacta. MU04 muere (contador de B = 6 ≠ 0).
- **TE15:** exacta. MU15 muere: sin clase en judgeWithoutRow, refreshGuard devuelve un no-veredicto → OpenFor falla → Build fatal.
- **TE20–23:** exactas en fixtures current, e inalcanzables en los pares Qresidue de los tres abridores de observación (C2-1). En los pares alcanzables, MU20–23 matan. El «immediate structural result and classifier receipt» también caza al mutante que se traga el error y lo lee como fresco.
- **TE28:** exacta. MU28 mata.
- **TE29:** exacta. MU29 mata: sin la conexión retenida, el nacimiento ocurre dentro de la consulta → shapeBad → handle.
- **TE37:** exacta. MU37 mata.
- **TE48:** exacta. MU48 mata.
- **TE49:** exacta en jsdom. MU49 mata.
- **TE50:** bloqueada (declarado).
- **TE54:** auditoría sin mutante propio (declarado).
- **TE55 y TE56:** exactas. MU55 y MU56 matan (para `storage:{}`, el camino crudo "" ≠ la ruta absoluta).
- **TE57:** exacta según la tabla de siembra. MU57 mata por plazo. Riesgo (predicción): el plazo de 2 s cubre todo OpenFor, incluidas 15 migraciones en el horario «older», así que con -race en una CI lenta podría dar un rojo falso.
- **TE58 y TE59:** exactas. Las mutaciones matan.
- **TE60:** exacta, con sus cinco piernas. «Adoptar libro» en ilegible solo existe como prueba de componente (el plan lo declara). MU60 mata.
- **TE61:** exacta.
  - `nameTouch` comprueba ErrLedgerUnreadable (approvals_adapter.go:618) antes que ErrApprovalUnreadable (:641).
  - Ninguna lectura previa de la aprobación toca `receipts`: `grep 'FROM receipts\|JOIN receipts' approvals*.go authority*.go` no devuelve nada.
  - MU61 mata.
- **TE62:** exacta. `enableApprovals` no tiene atajo (whats_happening.go:794-801). MU62 mata.
- **TE63, TE64, TE65 y TE66:** exactas. Sus MU matan.
- **TE67:** calibración obligatoria. MU67 mata: el gancho rechaza la conexión → Build fatal.
- **Afectada sin estar revisada:** TE42. Su subcaso «unknown error» no tiene salida exacta (C2-2).

## (c) «41 filas de la v1 conservadas byte a byte»: VERIFICADO

- Comando: `sed -n '141,201p' v1 | grep '^| TE[0-9]'` da 54 filas; `sed -n '171,244p' v2 | grep '^| TE[0-9]'` da 67. Comparadas por identificador, con los auxiliares `c2-v1-s6.txt` y `c2-v2-s6.txt`.
  - IGUALES (41): TE01, 02, 03, 05, 06, 07, 08, 09, 10, 11, 12, 13, 14, 16, 17, 18, 19, 24, 25, 26, 27, 30, 31, 32, 33, 34, 35, 36, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 51, 52, 53.
  - DISTINTAS (13): TE04, 15, 20, 21, 22, 23, 28, 29, 37, 48, 49, 50, 54. Es exactamente la lista de l.167.
- Garantías: GE1, 3, 5, 6, 8, 9 y 10 son idénticas; cambian GE2, GE4 y GE7, como se declara.
- En el §8 solo cambia la fila de TE50 (`c2-v1-s8.txt` frente a `c2-v2-s8.txt`). Las columnas de evidencia del §6 y del §8 coinciden en 67/67 (`c2-ev6.tsv` frente a `c2-ev8.tsv`).
- MU: 66 identificadores distintos; `grep -o MU54` da 0.
- TE30: su fila es idéntica. Los subcasos de nacimiento y el mutante nuevo de «removing each exclusion» solo se declaran en prosa (l.167, l.298), sin identificador MU. Su resultado esperado no cambia; ese mutante es C2-3.
- Fuera de la matriz, `diff v1 v2` muestra cambios en:
  - el §11 histórico: N-4 (l.682), N7-7 (l.704), N9-1 (l.736) y OBS-1 (l.762). No aparecen en la custodia de l.163-169 (C2-5);
  - los anexos A/B (l.1014, 1032, 1070, 1075, 1081, 1812-1813) y el C (l.2694-2696). Todos llevan su alcance honesto, y el de l.1070 corrige la inexactitud de la ronda 1.

## (d) Mecanismos nuevos frente al código real

**Interbloqueo con `SetMaxOpenConns(1)` (store.go:1485), camino por camino** (predicción por lectura):
- **Apertura.** pool-open-shape usa `withLedgerConnection`, que adquiere la única conexión; esta nace ahí, con el gancho sobre la conexión cruda (driver.go:249, 266-269). Se libera antes de la siembra.
- **Siembra.** `BeginTx` reutiliza la conexión ociosa y el re-juicio va por la tx (l.302).
- **Migración.** `db.QueryRow().Scan` libera. `migrateStep` lee QversionTx sobre la tx.
- **readOwner.** Filas ≤2 y Close antes de migrar (l.87).
- **refreshGuard y Standing.** Fijan la conexión y `setGuardIn` va después. Ningún llamador de producción de Standing tiene una tx abierta (config_act.go:187, app.go:409, cli/ledger.go:150, ledger_identity.go:91).
- **Poda y beginWrite.** judgeIn va sobre la tx, y `setGuardIn(s.db)` tras el Rollback.
- **installGuard.** Lee el catálogo por Conn y lo libera antes de los `ExecContext` de los triggers.

No encuentro anidamiento. El único molde con plazo es TE57, para la siembra.

**Exclusión por etapa.**
- En `isVerdict` solo tiene efecto en OpenFor:92, donde Standing ya ha clasificado el error. En refreshGuard, beginWrite y beginAdoption es redundante.
- En el filtro de :105 no tiene ninguna entrada alcanzable (C2-3).
- Tests aprobados que llaman a `isVerdict` directamente: ledger_shape_test.go:326, 355 y 498. Siguen verdes (predicción).

**Re-juicio sobre la tx.** Sin tests aprobados rotos:
- `TestOpen_seedFailureIsBootFatal` (errors_test.go:269-289) no es un prefijo;
- los tests de fichero fresco dan el mismo esquema (predicción).

**Regla de filas de identidad.** `CASE WHEN id = 1 …` nunca da NULL, porque NULL cae en ELSE 0. Es coherente con el conteo y el id del gancho (ledger_identity.go:552-585). R03 y R04 (ledger_identity_test.go:248 y :270) siguen igual. Ningún camino legítimo produce dos filas (CHECK en store.go:690; `ON CONFLICT` en profile_standing.go:337-341).

**LedgerPath.** Los consumidores Go del GET leen con `map` o `struct` y toleran un campo nuevo:
- ledger_shape_boot_test.go:111;
- profile_standing_test.go:174-420;
- shell/ledger_standing_test.go:59.

El único consumidor TSX es WhatsHappening.tsx.

**Envoltorios con contexto.** Sin cambios de firma. D07, D15, D16 y D20 siguen verdes (predicción), con una condición: `connectionBirthError.Error()` tiene que conservar el texto «injected», porque D16 y D20 comprueban ese texto (ledger_shape_test.go:355, 498).

**Tests aprobados rotos sin declarar:** no encuentro ninguno.

## (e) `printLedgerStanding` con un error sin clasificar

- El «existing fallback» es la rama `case err != nil:` de internal/cli/ledger.go:152-155, que imprime `ledger standing: ledger_unreadable (<causa>)`. El consejo de restaurar solo está en el comentario de :153-154; no se imprime.
- Con un error sin código, un pool cerrado o uno de los 14 códigos no categorizados, `ledger check` y `receipt verify` (receipt.go:123) imprimen esa línea. Luego el recorrido decide: con una cadena sana sale «chain intact» o «OK» y exit 0.
- Repite la mitad «ilegible sin veredicto» de C1-1. La documentación pública ata esa ficha a «solo restaurar el libro desde una copia lo levanta» (docs/releases/v0.16.2.md:152-156). Es C2-2.

## (f) TE50: receta nativa

**`unavailable`: receta candidata** (predicción; no ejecutada).
- **Sitio:** GET `/api/whats-happening` → `configActRecorder.LedgerStanding` (config_act.go:187) → `Standing` → `judgeIn` → primera lectura, Qcatalog (ledger_shape.go:130), en la conexión única del pool. Origen public-Standing.
- **Mecanismo:** con la app empaquetada en marcha, la pantalla cargada una vez y `lsof` mostrando `<ruta>-shm` abierto, un ayudante del mismo usuario abre `<ruta>-shm` y toma `fcntl(F_SETLK, F_WRLCK)` sobre los bytes 123–127, las marcas WAL_READ_LOCK(0..4).
  - La base es 120 = (22+SQLITE_SHM_NLOCK)·4, con SQLITE_SHM_NLOCK=8 (lib/sqlite.go:4282).
  - En darwin, `_unixShmSystemLock` usa F_SETLK y convierte cualquier fallo en SQLITE_BUSY (sqlite_g_000000000001c003.go).
- **Señal de listo:** el ayudante imprime ARMED solo cuando tiene los cinco bytes; si la app estaba leyendo, reintenta. El padre lo confirma con F_GETLK.
- **Código esperado:** SQLITE_PROTOCOL (15).
  - `walTryBeginRead` devuelve WAL_RETRY con los cerrojos ocupados y, pasado WAL_RETRY_PROTOCOL_LIMIT = 100 (lib/sqlite.go:9538; rama en sqlite_g_000000000001feab.go), devuelve PROTOCOL.
  - Espera acumulada: `awk` sobre la fórmula (cnt-9)²·39 µs → `total_sleep_us=9958498 (9.958 s)`.
  - WAL_RETRY_BLOCKED_MASK = 0 (lib/sqlite.go:9536), así que no hay cerrojos con plazo en esta compilación (predicción).
- **Proyección:** 15 no está categorizado → no es veredicto → `unavailable` → D3 del momento, con los controles activos. El texto de D3 («otro proceso lo está usando») es cierto en este caso.
- **Liberación:** el ayudante cierra el fd; F_GETLK no ve cerrojos; el siguiente GET da `ok`.
- **Para cerrar en el preflight:** capturar el 15 real; mientras está armado fallan también las escrituras de la app; y hay que confirmar que `getJSON` no corta antes de 10 s (no verificado).

**`environment`: no encuentro receta nativa y determinista.** Argumento por lectura:
1. Standing solo lee, en una conexión ya nacida. En producción no hay ConnMaxLifetime: `poolLifetimeForTest` solo lo fija un molde (ledger_identity.go:656-660; store.go:1486-1488). Cambiar permisos, dueño o ruta no afecta a descriptores ni al mapeo del -shm ya abiertos.
2. Los códigos de entorno en una lectura WAL exigen una E/S que falle. En macOS eso pasa por un desmontaje forzado, un FS en red o FUSE. Con la caché de páginas caliente y la cabecera WAL sin cambios, la lectura sale de memoria; y si toca el -shm mapeado, puede dar SIGBUS en vez de 10. No es determinista.
3. El fallo de un cerrojo de shm siempre sale como BUSY, nunca como IOERR (punto 2 de la receta anterior).
4. FULL, READONLY, CANTOPEN, PERM, AUTH y NOLFS no salen de estas lecturas sobre una conexión abierta (predicción).
5. En el arranque, el almacén de conversaciones muere antes (app.go:887-893), así que no hay pantalla. La ficha UX l.47 dice lo mismo.

El subcaso de entorno sigue BLOQUEADO.

## (g) Anexo D, entero

- **«Inventory before proposal»:** muestreé unas 30 citas y todas casan, entre ellas:
  - ledger_shape.go:55-60 y 105-119; store.go:1434-1443, 1457, 1478, 1485, 1494, 1501, 1505, 1509, 1517-1519, 1532-1537 y 2103-2120;
  - ledger_identity.go:82, 91-98, 110, 176-182, 212-217, 285-297, 464-466, 474-476, 479/488/492, 539, 550, 552-585, 589-609, 616 y 623;
  - driver.go:249 y 266-269; conn.go:151-154;
  - ledger_shape_test.go:196-204, 322-327, 354-365 y 446/456-462; ledger_hook_test.go:72/162/181 y 231-257; ledger_identity_test.go:248 y 270; errors_test.go:269-289.
  - «Ningún test llama a readOwner» también es cierto: `grep 'readOwner(' *_test.go` no devuelve nada.
- **C1-14 (SchemaVersion):** `grep -rn '\.SchemaVersion('` sin EffectiveSchemaVersion da 25 sitios. Los mapeé a funciones con awk: 21 tests, exactamente los nombres y líneas de la tabla. Revisé 7 líneas de assert (ledger_test.go:109-114, migration_test.go:122-130, store_test.go:50-62, errors_test.go:42, identity_phase1_test.go:653, migration_v3_test.go:214, approvals_test.go:151); todas casan.
  - Ojo: `git grep` no ve los tests sin seguimiento. Lo verifiqué con `grep -r`.
- **C1-4:** config_act.go:61-65 y 108, app.go:670-672, storagePath (comentario en :896-900, función desde :901), config_act_registry.go:103-111, whats_happening.go:363-367 y TSX:65-75. Casan.
- **C1-10:** las filas Go de whats_happening.go (:99-101, 104-105, 108-109, 115-117, 129-130, 136-138, 152-154), las líneas TSX y las 23 líneas del test casan. No hay clic sobre «Poner el techo». Los tests estrictos están en authority_prepare_test.go:26/35, authority_strict_doors_test.go:366/383/386/397/407 y authority_activated_boot_test.go:191-198. Un rótulo falla: «no-agent nil-store contract `:407`» (C2-5).
- **N9-1:** idéntico a la ronda 9, l.141-153.

## Tabla de los hallazgos de la ronda 1

| Hallazgo | Veredicto | Destino |
|---|---|---|
| C1-1 | CURADO (predicción) | BIEN |
| C1-2 | CIERRA A MEDIAS (C2-1) | BIEN |
| C1-3 | CURADO (predicción) | BIEN |
| C1-4 | CURADO (predicción) | BIEN |
| C1-5 | CURADO (predicción) | BIEN |
| C1-6 | CURADO (predicción) | BIEN |
| C1-7 | CURADO | BIEN |
| C1-8 | CURADO (predicción) | BIEN |
| C1-9 | CURADO (predicción) | BIEN |
| C1-10 | CURADO (predicción) | BIEN |
| C1-11 | NO CIERRA (bloqueado y declarado) | BIEN |
| C1-12 | CURADO en el plan (UX pendiente) | BIEN |
| C1-13 | CURADO (predicción) | BIEN |
| C1-14 | CURADO | BIEN |
| C1-15 | CURADO (predicción) | BIEN |
| N9-1 | CURADO (predicción) | BIEN |
| OBS-1 | CURADO, trasladado a E | BIEN |

**Resultado:** de los 17, 15 quedan CURADOS, 1 CIERRA A MEDIAS (C1-2) y 1 NO CIERRA (C1-11). Los 17 van bien destinados.

## Hallazgos nuevos

### C2-1 · P2 · [SEAM-INALCANZABLE][TAXONOMÍA][AFIRMACIÓN-FALSA]

**Dónde:** §7, tabla de un solo impacto (l.278, l.279, l.281); Qresidue (l.260); l.264; TE20–23 (l.196-199); GE1 (l.34).

**Qué:** la tabla da a pool-open-shape, operator-probe y read-only-open un desenlace sano sin mirar el fixture. Con el prefijo vacío que Qresidue exige, la regla nueva (una lectura estructural da shapeBad, l.156) desvía los tres abridores. En el operador y el lector rompe además la letra de GE1.

**Evidencia:**
- l.260: «Qresidue … | Each relevant empty prefix»; l.264: «Execute each reachable origin/query pair across the complete §5 fault grid».
- store.go:1532-1541: `default:` con identidad → Ping → Store, sin siembra.
- profile_standing.go:174-175: `if shape.shape != shapeCurrent { return LedgerStandingUnreadable, "", fmt.Errorf("%w: %s", ErrLedgerUnreadable, shape.reason) }`.
- store.go:1434-1443: el switch no tiene caso para shapeBad → `return openWithIdentity(path, identity)`.
- store.go:2103-2120: el lector se devuelve igualmente.

**Reproducción (predicción por lectura):**

Par A (pool-open-shape):
1. Fichero con las tablas de conversación y el prefijo k=1 (`CREATE TABLE IF NOT EXISTS action_schema (version INTEGER NOT NULL)`, vacío).
2. Armar `judgeReadFault`: origen pool-open-shape, sitio Qresidue, código 11, un solo impacto.
3. `OpenFor(path, profileA)`. El gancho, sin armar, ve fresco y pone la guarda en ok; el juicio del pool da shapeBad, nil.
4. Rama `default` → sin siembra → Ping → Store.
5. refreshGuard → judgeIn → ErrLedgerUnreadable → la guarda pasa a ilegible (ledger_identity.go:289-297).
6. El Standing de OpenFor da veredicto → handle, nil (:91-98, 110).

Resultado: handle sobre un fichero sin sembrar, Standing ErrLedgerUnreadable, escrituras rechazadas. La fila l.278 («Standing ok, guarded write accepted») no se alcanza. Build sigue adelante (app.go:409-410) y el GET pinta D2 («…aparta esos ficheros…») sobre un fichero sano con conversaciones.

Par B (operator-probe):
1. El mismo fichero; armar el origen operator-probe.
2. `probeShape` → shapeBad → caída a `openWithIdentity` → siembra y migra hasta v16.

Es una escritura del operador, también desde `korvun ledger check`, que GE1 prohíbe («An operator or reader opening an existing prefix returns ErrNoActionStore»), igual que el godoc de OpenOperator (store.go:1413-1420). La fila l.279 pide «Later fresh judgment ok», pero el libro sembrado es `legacy_unfounded` (profile_standing.go:208-209). Ninguna columna de TE20–23 prohíbe la siembra.

Par C (read-only-open): el lector se devuelve (no ErrNoActionStore) y Standing da ilegible. l.281 no se alcanza.

**Por qué P2:** es la clase de C1-2. La rejilla que prueba GE2 no puede pasar a verde tal como está escrita, y la cura abre un camino por el que un fallo de lectura de un solo impacto hace que la puerta del operador escriba esquema. No es P1: la siembra y la migración son aditivas.

### C2-2 · P2 · [TAXONOMÍA][ORÁCULO][MUTACIÓN-SOBREVIVE]

**Dónde:** §5 l.160 («with its existing fallback»); GE3 (l.36); §5 l.141-142; TE42 (l.218); TE43 (l.219).

**Qué:** el fallback imprime la ficha de veredicto para errores que el propio §5 declara «not a verdict». TE42 no fija salida exacta para ese subcaso.

**Evidencia:**
- ledger.go:152-155: `case err != nil:` → `fmt.Fprintf(out, "ledger standing: %s (%v)\n", actionsqlite.LedgerStandingUnreadable, err)`.
- TE42 ataca con «each known class and unknown error», pero su salida exacta solo dice «busy/env/unreadable exact line», y MU42 es «all errors print unreadable».
- docs/releases/v0.16.2.md:152-156.

**Reproducción (predicción):**
1. Seam de TE42: el Standing del doble devuelve `errors.New("database is locked")`, el mismo error que config_act_registry_test.go:919-921.
2. stdout: `ledger standing: ledger_unreadable (database is locked)`.
3. `ledger check` → «chain intact», exit 0 (ledger.go:141-142). `receipt verify` → «OK», exit 0. Es la misma forma que TE13/43 esperan para un defecto real.
4. MU42 coincide con lo planeado para el subcaso desconocido, así que ahí sobrevive.

**Por qué P2:** es la clase de C1-1 en otra superficie pública: una ficha de veredicto sin veredicto, que la documentación ata a «restaurar desde una copia». Rompe GE3 y viola los puntos 1, 2 y 4 de la doctrina. No es P1: no ejecuta nada por sí misma.

### C2-3 · P3 · [MUTACIÓN-SOBREVIVE][AFIRMACIÓN]

**Dónde:** §7 l.298.

**Qué:** la exclusión de nacimiento en el filtro de ledger_identity.go:105 no tiene ninguna entrada alcanzable, así que el mutante declarado «removing each exclusion» sobrevive allí.

**Evidencia:**
- beginWrite pasa el error de `BeginTx` por `mapGuardError` (ledger_identity.go:168-171).
- `mapGuardError` solo traduce 5/6 y 1811 (:347-377); el código 11 pasa intacto.
- Prune no es una de las cuatro fronteras públicas (l.157).
- Además, app.go:410 decide «unreadable» con `errors.Is` sin la exclusión, y l.298 promete «cannot be made into … skipped maintenance»; hoy es inalcanzable, así que solo es documental.

**Reproducción (predicción):**
1. Subcaso de TE30: `poolLifetimeForTest` = 1 ms, como en ledger_shape_test.go:493-496, y el CREATE TEMP TABLE del gancho fallando con 11, armado desde `openPruneSeam`.
2. Prune → BeginTx: nace la conexión, el gancho falla y el error llega sin clasificar.
3. En :105, `errors.Is(err, ErrLedgerUnreadable)` ya es false → OpenFor falla.
4. Quitando la exclusión, el resultado es el mismo: sobrevive.

**Por qué P3:** l.268 obliga a registrar y rediseñar la mutación que sobrevive; ninguna garantía de ejecución cae.

### C2-4 · P3 · [ADJUDICACIÓN-NO-CIERRA]

**Dónde:** l.3, l.807, l.810.

**Qué:** l.810 dice «To lift this author's VETO, C1-11 needs a concrete native preflight for both blocked packaged subcaptures». Eso convierte TE50 en condición previa a RED, cuando la decisión dice «puerta antes del PR, no antes de RED».

**Reproducción:**
1. Según la decisión, RED puede empezar con TE50 pendiente.
2. Según l.810, no, mientras falte el preflight nativo de `environment`.
3. Según (f), ese preflight no existe de forma determinista, así que el veto del autor no se levantaría nunca.

**Por qué P3:** cumple la letra (l.335 sitúa TE50 antes del PR), pero su condición de salida la contradice. La adjudicación es del copiloto.

### C2-5 · P3 · [AFIRMACIÓN][ARITMÉTICA]

**Qué:** textos del documento a medio actualizar:
- l.497: «The 67 rows are design cases, not 54 tests run.» Quedó a medias; la v1 l.371 decía «54 … 54».
- l.538: «all ten round inventories». Con la tabla «Ronda 1» (l.767) son 11.
- La custodia de l.163-169 no enumera los cuatro cambios del §11 histórico (N-4, N7-7, N9-1 y OBS-1; diff de v1 l.556/578/610/636 frente a v2 l.682/704/736/762).
- Anexo D l.3092: «no-agent nil-store contract `:407`». La línea 407 de authority_strict_doors_test.go solo prueba el almacén nil, sin nada de agentes.

**Reproducción:** el `diff` y las lecturas citadas.

**Por qué P3:** afecta a la custodia documental, no a las garantías.

## Lo que esta ronda encontró y la anterior no vio

- La tabla de un solo impacto no mira el fixture. Además, «lectura estructural → shapeBad» sumado a la caída de shapeBad del operador deja que `ledger check` siembre o migre un fichero existente (C2-1).
- El fallback de la CLI imprime la ficha de veredicto para errores sin clasificar, y TE42 no tiene salida exacta para ellos (C2-2).
- La exclusión de nacimiento en :105 está muerta (C2-3).
- La condición del veto del autor choca con la decisión sobre TE50 (C2-4).
- Una receta nativa candidata para `unavailable` a partir del límite de 100 reintentos WAL del driver, y el argumento de por qué no la hay determinista para `environment` (f).

## Alcance

**Leído:**
- Plan v2: §1–§12 enteros, la matriz entera y el Anexo D entero. De los anexos A, B y C, solo las líneas cambiadas.
- Veredicto de la ronda 1 entero. Rondas 9 (l.130-160) y 10 (l.262-280).
- En el WT, enteros: ledger_identity.go, profile_standing.go y ledger_shape.go.
- En el WT, por tramos:
  - store.go: 700-740, 1238-1297, 1380-1547, 1755-1830 y 2070-2121;
  - app.go: 355-460, 660-695 y 880-911;
  - config_act.go: 55-115, 175-200 y 445-465;
  - whats_happening.go: 290-500 y 794-801;
  - act.go: 60-95 y 150-172;
  - approvals_adapter.go: 440-500 y 600-660;
  - controlapi/approvals.go: 185-255;
  - sqlite/approvals.go: 235-262;
  - authority_v2.go: 380-420, 653-673 y 700-725;
  - config_authority.go: 86-116;
  - identity.go: 425-453;
  - signing.go: 40-140;
  - cli/ledger.go: 55-166;
  - cli/receipt.go: 100-180;
  - WhatsHappening.tsx: 30-80, 120-145, 270-352, 415-640 y 700-801;
  - WhatsHappening.test.tsx: 826-852 y las líneas del Anexo D;
  - tests: ledger_shape_test.go 180-219 y 300-505; ledger_hook_test.go 70-75, 160-183 y 229-258; errors_test.go 22-34 y 107-290; config_act_registry_test.go 910-950; docs/releases/v0.16.2.md 120-160.
- Del driver: driver.go 245-275, conn.go 140-160, walTryBeginRead y `_unixShmSystemLock` transpilados, y las constantes.

**Ejecutado (solo lectura):** `date`, `wc`, `shasum`, `stat`, `tail|xxd`, `git diff/status/rev-parse/grep`, `grep -r`, `sed`, `awk` (incluido el cálculo de 9,958 s), `diff`, `cmp`, `ls`, `uname`, `go env`.

Auxiliares en mi scratchpad, fuera de los árboles: `c2-v1-s6.txt`, `c2-v2-s6.txt`, `c2-v1-s8.txt`, `c2-v2-s8.txt`, `c2-ev6.tsv`, `c2-ev8.tsv`, `c2-d2-copilot.txt` y `c2-d2-plan.txt`.

Ni tests, ni compilaciones, ni mutaciones.

**Muestreado:**
- Anexo D: unas 30 citas del inventario, las 25 de SchemaVersion más 7 asserts, las 23 líneas del TSX y los 3 tests estrictos. Todo casa salvo el rótulo de :407.
- Filas de la matriz: las 26 revisadas o nuevas, leídas contra el código.

**No verificado:**
- Todos los desenlaces dinámicos.
- El código SQLite real de la receta de `unavailable` (15 frente a 5).
- Si `getJSON` tiene plazo.
- Que un -shm mapeado dé SIGBUS al desmontar.
- La calibración de TE67.
- Los tiempos de TE57 bajo -race.
- Los recuentos 117/117 y 103/103 del JSON histórico.
- La documentación fuera de los tramos citados.

**Tiempo:** de 13:03 a 13:40, dentro del presupuesto.

## Integridad (13:33:55)

- `git -C WT diff | cmp - …/adv-c2-before.patch` → «DIFF: identical».
- `git -C WT status --short | grep '^??' | sort | cmp - …/adv-c2-untracked-before.txt` → «UNTRACKED: identical».
- `git -C WT diff --cached --quiet` → `cached_exit=0`. HEAD sigue en `d20dea6`.
- Plan: 3112 líneas, 777348 bytes, sha256 `e6432247…3f0`, mtime 12:35:04.
- Copia de la v1: `8effacbe651e3eac…`.
- El veredicto de la ronda 1 (`2d2e6433…`) y el extracto (`9c277079…`) no han cambiado.