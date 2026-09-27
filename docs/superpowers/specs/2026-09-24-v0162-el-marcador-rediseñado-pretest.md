# El marcador «libro fundado por este perfil», REDISEÑADO — plan de fallos antes del rojo (v0.16.2, tren B)

**Estado: `IMPLEMENTED + VERIFIED` por los moldes y el gate local (§9–§10); pasadas en el informe de sesión.** Nace de la parada del
2026-09-24 (seis rondas oficiales con veto; paquete de parada en
`design-drafts/claude-code-report.md`) y de la orden del director: rediseño,
sonda primero, plan de fallos completo antes del rojo, tope de dos rondas
oficiales.

## 0 · Lo que las seis rondas enseñaron, en tres frases

El bloqueo vivía en la capa de aplicación, puerta a puerta, y su completitud
se confió a un paseo por el fuente que nunca puede ser completo contra quien
escribe puertas nuevas. La marca vivía en la celda de un recibo, bajo `LIKE` y
plegado de mayúsculas, y el lector tuvo que aprender una a una las formas en
que esa celda puede estar mal. Y la adopción —el único acto exento del
bloqueo— heredó la lectura cacheada del estado, con una ceguera que en su
caso es permanente.

## 1 · La sonda (hecha, una hora de tope, 5 minutos de reloj)

`internal/action/sqlite/profile_guard_probe_test.go`, captura en
`docs/superpowers/specs/evidence/v0.16.2/probe-temp-triggers.txt`, con el
driver del repo (`modernc.org/sqlite v1.59.0`), el DSN del store (WAL,
`busy_timeout`, `foreign_keys`) y el esquema real, sobre TRES conexiones
reales a un fichero:

| Pregunta | Respuesta, capturada |
|---|---|
| ¿Un `CREATE TEMP TRIGGER … ON main.actions` se instala por conexión? | Sí: la conexión A tiene 2 triggers en `sqlite_temp_master`, la B tiene 0 (Q0/Q3) |
| ¿Dispara `RAISE(ABORT, 'nombre')` en `BEFORE INSERT`? | Sí, fuera y dentro de una transacción: `constraint failed: ledger_foreign_profile (1811)` (Q1/Q2) |
| ¿Sobrevive a la transacción y a su rollback? | Sí: 2 triggers tras el rollback; la segunda transacción también muere (Q2) |
| ¿Y `BEFORE UPDATE`? | Sí: B actualiza, A muere con nombre (Q4) |
| ¿No existe en la segunda conexión? | Sí: B escribe sin obstáculo (Q3) |
| ¿Se conmuta sin tocar los triggers? | Sí: `UPDATE temp.profile_guard` y A escribe; vuelta atrás y A muere (Q5) |
| ¿Persiste con el pool de `database/sql`? | Sí, con `SetMaxOpenConns(1)` tras cinco consultas (Q6); `ResetSession` del driver no toca objetos temporales (leído) |
| ¿El error es nombrable desde Go? | Sí: `*sqlite.Error`, `Code() == 1811`, texto con el nombre (Q8) |
| **La trampa** | Un `WHEN (SELECT standing FROM temp.profile_guard) <> 'ok'` con la tabla de guarda VACÍA evalúa NULL y el INSERT ENTRA: falla abierto. Con `COALESCE(…, 'unset')` muere con `ledger_guard_unset` (Q7) |

**Veredicto de la sonda: limpia; R1 entra en este tren**, con la trampa de Q7
convertida en fila (R15) y con una pregunta que la sonda NO probó y va a fila
propia (R14): una conexión del pool reemplazada nace sin objetos temporales.

## 2 · Diseño, en las líneas que caben

**R2 · La identidad sale del recibo a su propia tabla.**
`ledger_identity (id INTEGER PRIMARY KEY CHECK (id = 1), owner_digest TEXT NOT NULL CHECK (owner_digest GLOB 'sha256:' || <64 × [0-9a-f]>), founded_by_action TEXT NOT NULL, adopted_by_action TEXT, written_at TEXT NOT NULL)`.
Una fila como máximo; la escriben solo `FinishFounding` (INSERT) y `AdoptLedger`
(UPSERT), en la MISMA transacción que su recibo; se lee con una consulta de una
fila, sin `LIKE`, sin plegado, sin caché. Sin fila y sin recibo marcado:
`legacy_unfounded`. Sin fila y con recibo marcado canónico: `ledger_unreadable`
(fila ausente). Basura en la fila: imposible por `CHECK`; el `UPDATE` a mano
falla en SQLite. `ledger_mark_malformed` queda solo para la migración de un
recibo con marca no canónica (R06). El recibo fundacional sigue llevando la
marca como EVIDENCIA sellada por la cadena; el ESTADO es la fila. Migración de
esquema con su fila en el plan.

**R3 · Transacciones inmediatas y handles con identidad.** Un único punto de
escritura `writeTx(ctx, func(tx) error)`: `BEGIN IMMEDIATE`, lectura de
`ledger_identity`, `UPDATE temp.profile_guard SET standing`, escritura. Toda
puerta de acto y la adopción pasan por él. `Open`, `OpenOperator` y
`OpenReadOnly` pasan a `OpenFor(path, identity)`, `OpenOperatorFor` y
`OpenReadOnlyFor`, con el digest canónico como parámetro obligatorio; los sin
identidad quedan no exportados para el paquete; `SetProfileIdentity`
desaparece. Ocho llamadores de producción migran (R12). Un `SQLITE_BUSY` se
nombra `ledger_busy`, nunca se traga.

**R1 · El bloqueo baja al almacenamiento, por conexión.** Al abrir (tras
migrar, recuperar y podar si el libro es propio), el handle crea
`temp.profile_guard (standing TEXT NOT NULL)` con UNA fila y nueve triggers
temporales: `BEFORE INSERT` sobre `actions`, `approvals`, `intents`,
`intent_versions`, `intent_events`, `grants`, `execution_bindings`,
`BEFORE UPDATE` sobre `intents` y `grants`, y `BEFORE INSERT` sobre
`principals`, `principal_events` y `principal_bindings` (R19), todos con
un trigger por estado que bloquea —`ledger_foreign_profile`,
`ledger_unreadable`— y uno para la guarda ausente o desconocida
(`ledger_guard_unset`), cada uno con `RAISE(ABORT, 'ledger_guard:<estado>')`:
el trigger que dispara NOMBRA el estado que evaluó, y el store traduce ese
nombre al centinela sin guardar copia alguna del estado (treinta y seis
triggers: doce eventos por tres). La fila la juzga el hook sobre la
conexión nueva por ENUMERACIÓN de los estados benignos (§13.1, G1-ter): un
fichero fresco (sin ninguna tabla) o un libro anterior a la fila (`action_schema`
con una sola fila numérica < 16 y sin tabla) abren `ok`; una fila canónica
propia `ok`, ajena `ledger_foreign_profile`; una tabla vacía sin marca `ok`;
TODA otra forma abre `ledger_unreadable` (la ronda 2 mostró que decidir por
formas de corrupción dejaba tres ramas abiertas). Para que una conexión nueva del
pool nazca con la guarda, la instalación va en el hook de conexión del driver,
seleccionado por un parámetro propio del DSN (R14, con sonda antes del rojo).
Los `refuseIfForeign` de Go y los dos paseos estructurales se RETIRAN (R25);
`principals` bajo la guarda se decide en R19.
*Superado (tren E, tandas 1 y 2): «TODA otra forma» ya no es la regla. Un
fichero con solo un prefijo vacío de la siembra v1 es fresco, y de los
fallos de lectura solo el que llega con un código estructural es veredicto;
el resto rehúsa la conexión. La forma de hoy la juzga `judgeShape`.*

**Lo que no cambia:** la identidad del perfil (`app.ProfileIdentity`, la ruta
absoluta con symlinks resueltos); los tres lectores; la pantalla; el modelo de
amenaza (tamper-evident; la edición a mano con un digest válido se declara,
R04); la privacidad del digest (R21); `adopt-ledger` como único acto que un
libro ajeno admite, y solo sobre `foreign` o `legacy`.

**Nombres nuevos:** `ErrLedgerUnreadable` (fila ausente con recibo marcado, o
error de lectura), `ErrLedgerGuardUnset`, `ErrLedgerBusy`; `controlapi.ErrLedgerUnreadable`
y sus dos entradas en el registro `outcomes` de aprobaciones (R13).

## 3 · Garantías, literales

| # | Garantía |
|---|---|
| G1 | Con la guarda de la conexión en `foreign`, `unreadable`, `malformed` o `unset`, todo INSERT en las siete tablas de actos y en las tres de identidad, y todo UPDATE en `intents`/`grants`, muere en SQLite con `ledger_guard` (1811), cualquiera que sea la función de Go que lo intente; el store lo nombra por el estado que juzgó; la guarda de cada conexión la juzga el hook sobre la propia conexión al nacer; y toda puerta de producción escribe por `beginWrite`, que juzga la fila dentro de la transacción (primera línea) |
| G2 | Fuera del paquete `sqlite` no existe un abridor sin identidad; un handle sirve al perfil con que nació, y solo a él |
| G3 | Toda puerta de acto y la adopción escriben bajo `BEGIN IMMEDIATE` con la identidad leída dentro de la misma transacción; `BUSY` se nombra |
| G4 | La fila de identidad solo admite el prefijo y un digest canónico (`CHECK`); no hay forma «malformada» de la fila |
| G5 | La adopción rehúsa sobre `unreadable`/`malformed`, escribe recibo y fila en una transacción, y solo se admite sobre `foreign` o `legacy` |
| G6 | Recibo fundacional y fila nacen juntos o no nace ninguno |
| G7 | La migración crea la fila desde un recibo marcado canónico y rehúsa hacerlo desde uno que no lo sea, nombrándolo |
| G8 | Un libro ajeno se abre sin recuperación, sin poda y sin migrar un esquema existente; el propio hace su mantenimiento antes de instalar la guarda |
| G9 | La puerta HTTP de aprobaciones nombra `ledger_foreign_profile` (409) y `ledger_unreadable` (503) |

## 4 · Plan de fallos, completo, antes de una línea de código

Cada fila lleva los nueve campos que el director pidió. «Los tres moldes»
nombra el molde del store, el de la app o la CLI, y el de la pantalla o el
binario cuando existen; donde solo hay uno o dos, se dice. Toda mutación
citada se ejecuta y se captura antes del canto (doctrina).

### R01 · La secuencia real del operador, orden A: la marca reescrita ANTES de que la app mirase

| Campo | Contenido |
|---|---|
| Garantía | G4/G5: una identidad ilegible se nombra y bloquea todo acto, la adopción incluida; ninguna puerta la entierra |
| Estado inicial | Libro fundado por el perfil A (fila `ledger_identity` con el digest de A, recibo fundacional sellado); la app aún no ha abierto el libro |
| Actores | Un editor externo (segunda conexión real); la app del perfil B con su servidor de administración en loopback; el operador (POST) |
| Secuencia completa | 1. Editor: `UPDATE ledger_identity SET owner_digest = 'profile:xyz'` → la `CHECK` lo RECHAZA en SQLite (la fila no cambia). 2. Editor: `DELETE FROM ledger_identity` → 1 fila. 3. `Build` de B, `Run`. 4. GET `/api/whats-happening`. 5. POST `adopt-ledger {confirm:true}`. 6. GET otra vez. 7. Cuentas |
| Punto de intercalado | Entre la fundación y el primer open de B |
| Resultado esperado, con nombre | Paso 1: error de `CHECK` capturado, fila intacta. Tras el paso 2: GET → `standing: ledger_unreadable`, causa «identity row missing while a marked receipt exists»; POST → 503 `act_not_recorded` con la causa `ledger_unreadable`; GET sigue `ledger_unreadable` |
| Efectos prohibidos | Ninguna fila nueva en `actions`/`receipts`; ninguna fila en `ledger_identity`; ninguna recarga del perfil; la fila «Adoptar libro» no se pinta (unreadable no lleva botón) |
| Evidencia | app real (servidor admin en loopback, mismo proceso) + segunda conexión real; store in-process para la `CHECK` |
| Los tres moldes | store: `TestIdentity_aMissingRowWithAMarkedReceiptIsUnreadable` (dos conexiones); app: `TestMount_anUnreadableIdentityRefusesAdoption/before_the_look`; CLI: `TestLedgerCheck_namesAnUnreadableIdentity` (binario in-process). Mutación: leer «fila ausente» como legacy → los tres enrojecen |

### R02 · La secuencia real del operador, orden B: la app mira, la marca se reescribe, el operador pulsa «Adoptar libro»

| Campo | Contenido |
|---|---|
| Garantía | G5: la adopción lee la identidad DENTRO de su transacción `BEGIN IMMEDIATE`, nunca de una caché; lo que la app mostró antes no la autoriza |
| Estado inicial | Libro fundado por A; app de B abierta y con el GET hecho (fila «Libro de otro perfil» con el botón) |
| Actores | App de B (handle vivo); editor externo (segunda conexión); operador |
| Secuencia completa | 1. GET → `ledger_foreign_profile`, dueño A. 2. Editor: `DELETE FROM ledger_identity` (o `UPDATE` a otro digest canónico C). 3. GET (sin caché en R2: ya dice `ledger_unreadable` / dueño C). 4. POST `adopt-ledger` confirmado. 5. GET. 6. Cuentas |
| Punto de intercalado | Entre el GET del paso 1 y el POST del paso 4 |
| Resultado esperado, con nombre | Con la fila borrada: POST → 503 `ledger_unreadable`, GET sigue `ledger_unreadable`. Con la fila cambiada a C: POST → 200 `adopted` (B adopta un libro que ahora dice C: es la letra de L6, «el último que adopta es el dueño»; la cadena sigue nombrando a A en el recibo fundacional) y el GET dice `ok`; DECLARADO como límite del modelo de amenaza (edición a mano con un digest válido), no como defecto |
| Efectos prohibidos | En el caso «borrada»: cero actos, cero recibos, cero filas de identidad. En el caso «cambiada a C»: exactamente un acto y un recibo de adopción, y la fila pasa de C a B en la MISMA transacción que el recibo |
| Evidencia | app real + segunda conexión, con sincronización real (el UPDATE/DELETE entra tras el GET confirmado) |
| Los tres moldes | store: `TestAdopt_readsTheIdentityInsideItsOwnTransaction` (handle vivo que ya juzgó; canal tras el GET); app: `TestMount_anUnreadableIdentityRefusesAdoption/after_the_look`; pantalla: molde TS de que `unreadable` no ofrece botón. Mutación: leer la identidad antes de `BEGIN IMMEDIATE` desde un valor guardado → el orden B enrojece en store y app |

### R03 · La fila de identidad reescrita a mano con basura

| Campo | Contenido |
|---|---|
| Garantía | G4: la fila solo admite `profile:sha256:<64 hex minúsculas>`; la basura la rechaza el almacenamiento, no un lector |
| Estado inicial | Libro fundado (fila válida) |
| Actores | Editor externo con segunda conexión real (y `sqlite3` en la prueba con binario) |
| Secuencia completa | Para cada forma —`PROFILE:…`, `profile:`, `profile: `, `profile:xyz`, `profile:` + `char(1)`, hex en MAYÚSCULAS, cadena vacía, NULL— `UPDATE ledger_identity SET owner_digest = <forma>` |
| Punto de intercalado | Cualquier momento |
| Resultado esperado, con nombre | Cada `UPDATE` falla con `CHECK constraint failed` (o `NOT NULL`); la fila conserva el digest de A; `Standing` sigue `ok` para A y `ledger_foreign_profile` para B |
| Efectos prohibidos | Ninguna forma aterriza; ningún lector ve otra cosa que el digest de A |
| Evidencia | store in-process con segunda conexión; binario en proceso aparte con `sqlite3` para dos formas |
| Los tres moldes | store: `TestIdentity_theRowRefusesEveryNonCanonicalForm` (tabla de ocho formas); CLI/binario: `ledger check` tras cada intento sigue `ok`; app: GET sigue `ok`. Mutación: quitar la `CHECK` del esquema → la tabla de formas enrojece |

### R04 · La fila de identidad reescrita a mano con OTRO digest canónico (edición a mano válida)

| Campo | Contenido |
|---|---|
| Garantía | L6 y el modelo de amenaza: tamper-evident, no tamper-proof. El lector dice lo que la fila dice; la cadena de recibos dice quién fundó; `ledger check` no cambia de veredicto por la fila |
| Estado inicial | Libro fundado por A |
| Actores | Editor externo |
| Secuencia completa | 1. `UPDATE ledger_identity SET owner_digest = <digest de C>`. 2. `ledger check` con el perfil A. 3. `intent create` con A. 4. `ledger check` con C (perfil real en otra ruta) |
| Punto de intercalado | Cualquier momento |
| Resultado esperado, con nombre | Paso 2: `ledger standing: ledger_foreign_profile (founded or adopted by C…)` y `ledger main: … chain intact` (la fila NO es parte de la cadena; se dice). Paso 3: exit 1 nombrando `ledger_foreign_profile`. Paso 4: `ok`. DECLARADO en el papel y en las notas: la fila es el estado, el recibo fundacional es la evidencia; una edición a mano del estado no se detecta por sí sola |
| Efectos prohibidos | Ningún acto de A aterriza; ninguna puerta «repara» la fila sola |
| Evidencia | binario en proceso aparte + `sqlite3` |
| Los tres moldes | CLI: `TestLedgerCheck_theRowIsTheStateAndTheReceiptIsTheEvidence`; store: `TestStanding_followsTheRowNotTheReceipt`; app: GET nombra a C. Mutación: derivar el estado del recibo en vez de la fila → los tres enrojecen |

### R05 · Dos conexiones adoptando a la vez bajo `BEGIN IMMEDIATE`

| Campo | Contenido |
|---|---|
| Garantía | L6/G5: la adopción es una transacción inmediata; dos adopciones concurrentes se SERIALIZAN; la segunda lee la fila que la primera dejó y actúa sobre ella; nunca dos filas ni un recibo sin fila |
| Estado inicial | Libro fundado por A; handles vivos de B y C (dos procesos o dos conexiones reales) |
| Actores | B y C, con sincronización real (canal tras el `BEGIN IMMEDIATE` confirmado de B) |
| Secuencia completa | 1. B: `BEGIN IMMEDIATE`, lee la fila (A), sella su recibo, `UPDATE ledger_identity`, señala y ESPERA. 2. C: `AdoptLedger` → su `BEGIN IMMEDIATE` bloquea (busy_timeout 5000 ms). 3. B: `COMMIT`. 4. C entra: lee la fila (ya B), adopta encima. 5. Estados de B, C y de un handle fresco |
| Punto de intercalado | C intenta entrar mientras B está dentro de su transacción |
| Resultado esperado, con nombre | B: `adopted`, recibo `rcpt_b`; C: `adopted`, recibo `rcpt_c` posterior en la cadena; fila final = C; B en su siguiente escritura: `ledger_foreign_profile` (L6 en su letra); fresco: dueño C. Si C agota el `busy_timeout`: `SQLITE_BUSY` nombrado como `ledger_busy`, nada escrito por C |
| Efectos prohibidos | Nunca dos filas; nunca un recibo de adopción sin su `UPDATE` en la misma transacción; C nunca lee A (la fila vieja) después de que B haya confirmado |
| Evidencia | dos conexiones reales, canal tras `BEGIN IMMEDIATE` confirmado; sonda de conteo de recibos |
| Los tres moldes | store: `TestAdopt_twoAdoptionsSerialize` (dos conexiones); store: `TestAdopt_busyIsNamedNotSwallowed` (un `BEGIN IMMEDIATE` ajeno mantenido más que el timeout); app: dos apps reales sobre el mismo fichero, dos POST casi simultáneos (nivel: mismo proceso, dos servidores). Mutación: `BeginTx` deferred en la adopción → la lectura de C ve A y el molde enrojece en la fila final o en el orden de recibos |

### R06 · Migración: un libro con la marca en el recibo (esta rama sin publicar) y sin fila

| Campo | Contenido |
|---|---|
| Garantía | G7: la migración crea la fila desde el ÚLTIMO recibo `SUCCEEDED` con marca canónica, dentro de la migración de esquema y antes de instalar la guarda; una marca no canónica NO se convierte en fila: el libro abre `ledger_mark_malformed` (clase ilegible), bloquea todo y lo dice |
| Estado inicial | Libro con esquema anterior, recibo fundacional con `profile:sha256:…` (o con basura, o con `PROFILE:`), sin tabla `ledger_identity` |
| Actores | El open del store (migración); un handle con identidad A o B |
| Secuencia completa | 1. `OpenFor(path, A)` → migra: crea la tabla; busca la marca; inserta la fila. 2. `Standing`. 3. Repetir con un recibo de marca basura. 4. Repetir con dos recibos marcados (fundación A, adopción B): manda el último |
| Punto de intercalado | Durante la migración, antes de que exista la guarda |
| Resultado esperado, con nombre | Caso canónico: fila = digest del último recibo marcado; `ok` para el dueño, `foreign` para otro. Caso basura: sin fila; `Standing` = `ledger_mark_malformed` con la causa; todo acto rehúsa; la adopción rehúsa. Caso dos recibos: fila = B |
| Efectos prohibidos | La migración nunca escribe una fila desde una marca que no pase la `CHECK`; nunca deja el libro abierto sin decidir (fila o estado ilegible nombrado) |
| Evidencia | store in-process sobre un fichero construido con el esquema anterior (fixture de bytes); binario para `ledger check` sobre ese fichero |
| Los tres moldes | store: `TestMigrate_identityRowFromTheReceiptMark` (tres casos); CLI: `TestLedgerCheck_namesAMalformedReceiptMarkAtOpen`; app: `TestBuild_aMalformedReceiptMarkBlocksEveryDoor`. Mutación: convertir la marca basura en fila → el caso basura enrojece |

### R07 · Libro sin nada: `legacy_unfounded`

| Campo | Contenido |
|---|---|
| Garantía | L5: sin fila y sin recibo marcado, el libro es legacy: nombrado, no bloquea, no es corrupción; la pantalla no ofrece marcarlo; la adopción por HTTP lo admite (D4) |
| Estado inicial | Libro de una versión anterior: tabla creada vacía por la migración, ningún recibo con marca |
| Actores | Handle con identidad; app; CLI |
| Secuencia completa | 1. `OpenFor`. 2. `Standing`. 3. Un acto cualquiera. 4. GET de la pantalla. 5. POST `adopt-ledger` confirmado por HTTP |
| Punto de intercalado | — |
| Resultado esperado, con nombre | `legacy_unfounded` en los tres lectores; el acto entra; la pantalla pinta la fila legacy sin botón; el POST adopta y el libro pasa a `ok` con fila |
| Efectos prohibidos | Ningún bloqueo; ninguna fila creada por la lectura |
| Evidencia | store, CLI in-process, app real |
| Los tres moldes | store: `TestStanding_aLedgerWithNoRowAndNoMarkIsLegacy`; CLI: `TestLedgerCheck_namesTheStanding/legacy`; app + TS: fila legacy sin botón. Mutación: leer «sin fila» como foreign → los tres enrojecen |

### R08 · El trigger existe en la conexión identificada y NO existe en otra conexión al mismo fichero

| Campo | Contenido |
|---|---|
| Garantía | G1 (R1): la guarda es de la CONEXIÓN (`TEMP`), no del fichero; una segunda conexión sin identidad ni la ve ni la hereda; la conexión identificada la tiene desde el open hasta el close |
| Estado inicial | Libro fundado por A; handle de B (foreign) |
| Actores | Handle de B; segunda conexión raw |
| Secuencia completa | 1. En B: `SELECT COUNT(*) FROM sqlite_temp_master WHERE type='trigger'`. 2. En la raw: lo mismo. 3. En la raw: INSERT en `actions`. 4. En B: INSERT en `actions` (una puerta cualquiera) |
| Punto de intercalado | — |
| Resultado esperado, con nombre | B: 9 triggers (7 INSERT + 2 UPDATE); raw: 0; el INSERT raw entra; el de B muere con `constraint failed: ledger_guard (1811)` que el store nombra `ledger_foreign_profile` |
| Efectos prohibidos | Ningún trigger en el fichero (`sqlite_master` sin triggers de guarda); la raw nunca bloqueada |
| Evidencia | dos conexiones reales (la sonda `probe-temp-triggers.txt` ya lo capturó con el driver del repo) |
| Los tres moldes | store: `TestGuard_livesInTheConnectionNotInTheFile`; store: `TestGuard_everyActDoorDiesInSQLite` (la tabla de 16 puertas, ahora con el trigger real como oráculo, sin triggers de aborto del test); CLI: `receipt rotate-key` desde una copia sigue rehusando. Mutación: `CREATE TRIGGER` sin `TEMP` → el conteo de la raw enrojece |

### R09 · Un INSERT desde una función nueva de Go SIN ninguna guarda (el trigger la mata)

| Campo | Contenido |
|---|---|
| Garantía | G1: el bloqueo no depende de cómo esté escrita la puerta: cualquier INSERT/UPDATE en las tablas de actos, por cualquier función presente o futura, muere en SQLite si la guarda dice que no |
| Estado inicial | Libro fundado por A; handle de B |
| Actores | Una puerta escrita A PROPÓSITO en un fichero de test del paquete (`zz_door_test.go`): `func (s *Store) UnguardedDoor(ctx, id)` con `s.db.ExecContext(INSERT …)` en la grafía que quiera (minúsculas, comillas simples, comentario, Sprintf, const) |
| Secuencia completa | 1. `s.UnguardedDoor(ctx, "act_x")` con B. 2. Cuentas. 3. Con A (dueño): entra |
| Punto de intercalado | — |
| Resultado esperado, con nombre | Con B: `ErrLedgerForeignProfile` (mapeado desde 1811 + `ledger_guard`); con A: `nil` y una fila |
| Efectos prohibidos | Con B: cero filas; sin paseo estructural en el árbol (los dos paseos se RETIRAN: su trabajo lo hace SQLite) |
| Evidencia | store in-process |
| Los tres moldes | store: `TestGuard_anUnguardedDoorDiesAnyway` (tabla de seis grafías, incluidas las de las rondas 3–5); store: la tabla de 16 puertas reales; app: el builder y las cinco puertas de la pantalla. Mutación: quitar el trigger de `actions` → la puerta a propósito escribe y enrojece |

### R10 · `prune`, `recover` y la migración con la guarda desactivada, y declarado

| Campo | Contenido |
|---|---|
| Garantía | G8: la secuencia del open es migrar → recuperar → podar → juzgar → instalar guarda; un libro AJENO se abre por la puerta del operador (sin recuperación, sin poda, sin migración de un esquema existente) porque no es de este perfil; un libro propio hace su mantenimiento antes de que exista la guarda |
| Estado inicial | Libro fundado por A con actos a recuperar y recibos a podar |
| Actores | El open del store con identidad A y con identidad B |
| Secuencia completa | 1. `OpenFor(path, A)`: recuperación y poda corren; después la guarda se instala en `ok`. 2. `OpenFor(path, B)`: juzga primero; con `foreign`, salta recuperación y poda (declarado) e instala la guarda en `foreign`. 3. Cuentas de recuperación/poda en cada caso |
| Punto de intercalado | Antes de instalar la guarda (paso 1); antes de decidir mantenimiento (paso 2) |
| Resultado esperado, con nombre | A: recuperados N, podados M, guarda `ok`; B: recuperados 0, podados 0, guarda `foreign`, y el open lo DICE en su nota |
| Efectos prohibidos | B nunca cierra un acto ajeno como `OUTCOME_UNKNOWN` ni poda un recibo ajeno; A nunca instala la guarda antes del mantenimiento (un mantenimiento bajo guarda `foreign` moriría en el trigger) |
| Evidencia | store in-process; sonda de conteo |
| Los tres moldes | store: `TestOpen_maintenanceRunsBeforeTheGuardForTheOwner`; store: `TestOpen_aForeignLedgerGetsNoMaintenance`; app: `Build` de B sobre el libro de A no recupera. Mutación: instalar la guarda antes de podar → el caso A enrojece con 1811 |

### R11 · Crash entre el recibo fundacional y la fila de identidad

| Campo | Contenido |
|---|---|
| Garantía | L1/G6: recibo y fila nacen en la MISMA transacción; no hay estado intermedio observable |
| Estado inicial | Bootstrap en curso (`CreateLedger`) |
| Actores | El store; una sonda que aborta la transacción en el punto exacto (seam `beforeIdentityRow`) |
| Secuencia completa | 1. `FinishFounding` dentro de `writeTx`: recibo → (seam) → `INSERT ledger_identity`. 2. La sonda aborta en el seam. 3. Reabrir con una conexión fresca. 4. `Standing`, cuentas |
| Punto de intercalado | Entre el recibo y la fila |
| Resultado esperado, con nombre | Tras el abort: cero recibos nuevos, cero filas; el acto sigue `AUTHORIZED` y el arranque siguiente lo cierra `OUTCOME_UNKNOWN`; `Standing` = `legacy_unfounded` (no hay marca ni fila) |
| Efectos prohibidos | Nunca un recibo marcado sin fila ni una fila sin recibo |
| Evidencia | store in-process con seam de crash; segunda conexión para observar |
| Los tres moldes | store: `TestFounding_receiptAndRowAreOneTransaction` (sonda en el seam); app: `TestCreateLedger_anAbortedFoundingLeavesNoMark`; CLI: `ledger check` sobre ese fichero. Mutación: escribir la fila fuera de la transacción → la sonda deja fila sin recibo y enrojece |

### R12 · Un escritor de la CLI que abre el store por su cuenta: NO COMPILA

| Campo | Contenido |
|---|---|
| Garantía | G2 (R3): fuera del paquete `sqlite` no existe ningún abridor sin identidad: `OpenFor`, `OpenOperatorFor` y `OpenReadOnlyFor` exigen el digest canónico como parámetro y rehúsan `ErrProfileIdentityMalformed`/`ErrNoProfileIdentity`; los abridores sin identidad son no exportados; `SetProfileIdentity` desaparece |
| Estado inicial | El árbol |
| Actores | Los ocho llamadores de producción: `e2e-harness/main.go:837`, `config_act.go:379`, `app.go:372`, `app.go:854`, `config_act_registry.go:375`, `intent.go:95`, `intent.go:124`, `receipt.go:459`; los tests que abren |
| Secuencia completa | 1. Renombrar y exigir identidad. 2. `go build ./...` y `go vet ./...` limpios. 3. Un fichero de test fuera del paquete que llame a `actionsqlite.OpenOperator(path)` → error de compilación |
| Punto de intercalado | — |
| Resultado esperado, con nombre | El paquete no exporta ningún abridor sin identidad (molde por reflexión sobre los nombres exportados del paquete: solo los `*For`); el `e2e-harness` y los `Build` sin `WithProfilePath` pasan una identidad de prueba EXPLÍCITA (`app.TestProfileIdentity`), nunca vacía |
| Efectos prohibidos | Ningún handle sin identidad fuera del paquete; ningún handle re-identificable |
| Evidencia | compilación + reflexión in-process |
| Los tres moldes | store: `TestOpeners_everyExportedOpenerTakesAnIdentity` (reflexión sobre `reflect.TypeOf` de los símbolos exportados, o `go/ast` sobre las firmas — AST solo sobre FIRMAS, no sobre cuerpos); CLI: `TestCLI_aForeignLedgerRefusesEveryWriter` sigue (dos verbos); app: `Build` sin identidad rehúsa por nombre. Mutación: exportar de nuevo `OpenOperator` → el molde de firmas enrojece |

### R13 · La puerta HTTP de aprobaciones nombra el libro ajeno y el ilegible

| Campo | Contenido |
|---|---|
| Garantía | G9: `POST /api/approvals/{id}/approve|reject` responde 409 `ledger_foreign_profile` y 503 `ledger_unreadable` con texto, nunca 500 «internal error»; el registro `outcomes` gana las dos entradas y su sección §12-ter de la especificación (FR-TEST-6 cruza ambas) |
| Estado inicial | Libro fundado por A; app de B con una aprobación pendiente (o la puerta con `Approvals` falso) |
| Actores | El adaptador (`approvals_adapter.go`: mapea `ErrLedgerForeignProfile` → `controlapi.ErrLedgerForeign`, `ErrLedgerUnreadable` → `controlapi.ErrLedgerUnreadable`); `writeApprovalError` |
| Secuencia completa | 1. POST approve con B. 2. Cuerpo y código. 3. Lo mismo con la fila borrada (ilegible). 4. `TestApprovals_theRegistryCrossesTheSpecAnchorsBothWays` sigue verde con las dos entradas nuevas en la spec |
| Punto de intercalado | — |
| Resultado esperado, con nombre | 409 `{error: ledger_foreign_profile, message: …}`; 503 `{error: ledger_unreadable, message: …}`; nada escrito; la decisión no se consume |
| Efectos prohibidos | Nunca 500; nunca una decisión registrada en un libro ajeno |
| Evidencia | httptest con `Approvals` falso (controlapi) + app real con store real (app) |
| Los tres moldes | controlapi: `TestApprovals_aForeignLedgerIsNamed` y `…anUnreadableLedgerIsNamed`; app: `TestMount_aForeignLedgerRefusesTheApprovalDoorsByName`; el molde FR-TEST-6 existente. Mutación: quitar las dos entradas del registro → los tres enrojecen (y FR-TEST-6 por la spec) |

### R14 · La conexión del pool se REEMPLAZA y la guarda debe seguir (la trampa que la sonda no probó)

| Campo | Contenido |
|---|---|
| Garantía | G1: la guarda es de cada conexión que el handle use, no solo de la primera: `database/sql` puede descartar una conexión (`ErrBadConn`, `ConnMaxLifetime`) y abrir otra; la nueva nace con la guarda o el handle no escribe |
| Estado inicial | Handle de B con guarda `foreign` |
| Actores | El pool de `database/sql`; el hook de conexión del driver (`sqlite.Driver.RegisterConnectionHook`, por DSN: el store abre con un parámetro propio en la query del DSN que lleva la identidad, y el hook instala `profile_guard` + triggers leyendo `ledger_identity` en la conexión nueva) |
| Secuencia completa | 1. `db.SetConnMaxLifetime(1 * time.Millisecond)` (seam de test) + una espera. 2. Una consulta (fuerza conexión nueva). 3. `sqlite_temp_master` en la conexión nueva. 4. INSERT por una puerta |
| Punto de intercalado | Entre dos operaciones del handle |
| Resultado esperado, con nombre | La conexión nueva tiene 12 triggers y `profile_guard` = `foreign` (juzgado por el hook sobre la conexión nueva); el INSERT muere con nombre, también por una puerta que no re-juzgue |
| Efectos prohibidos | Nunca una conexión del handle sin guarda; si el hook falla, la conexión NO se entrega (fail closed: el open/consulta devuelve el error del hook) |
| Evidencia | store in-process con el pool real |
| Los tres moldes | store: `TestGuard_survivesAPoolReconnect`; store: `TestGuard_aHookThatFailsClosesTheConnection`; sonda previa de que el hook del driver corre por DSN (extender `probe-temp-triggers.txt` con una Q9 antes del rojo). Mutación: instalar la guarda solo en el open (sin hook) → tras la reconexión el INSERT entra y enrojece |

### R15 · La guarda vacía o ausente falla CERRADO (la trampa que la sonda SÍ encontró: Q7)

| Campo | Contenido |
|---|---|
| Garantía | G1: un `WHEN` sobre `profile_guard` sin fila no puede evaluar a NULL y dejar pasar: `COALESCE(…, 'unset') <> 'ok'`; y un handle cuya fila de guarda falte rehúsa `ledger_guard_unset` |
| Estado inicial | Handle de A (`ok`) |
| Actores | Una sonda in-package que borra la fila de `temp.profile_guard` por la propia conexión del handle |
| Secuencia completa | 1. `DELETE FROM temp.profile_guard` vía `s.db`. 2. Un acto |
| Punto de intercalado | — |
| Resultado esperado, con nombre | `ErrLedgerGuardUnset` (mapeado de 1811 + `ledger_guard_unset`); cero filas |
| Efectos prohibidos | Nunca un INSERT con la guarda ausente |
| Evidencia | store in-process |
| Los tres moldes | store: `TestGuard_anEmptyGuardTableFailsClosed`; store: el trigger del molde de la sonda (Q7) queda como test; app: n/a (declarado). Mutación: quitar el `COALESCE` → enrojece (la sonda ya lo capturó: `naive WHEN err=<nil>`) |

### R16 · Toda puerta de acto escribe bajo `BEGIN IMMEDIATE` con la identidad leída dentro (cierra §8 fila 5)

| Campo | Contenido |
|---|---|
| Garantía | G3 (R3): el punto único `writeTx` abre `BEGIN IMMEDIATE`, lee `ledger_identity`, actualiza `temp.profile_guard` y ejecuta la escritura; otra conexión no puede cambiar la fila entre la lectura y la escritura de la misma transacción |
| Estado inicial | Libro fundado por A; handle de B (foreign) y handle de A |
| Actores | A dentro de `writeTx` (canal tras `BEGIN IMMEDIATE`); B intenta adoptar mientras |
| Secuencia completa | 1. A: `writeTx` entra, señala, espera. 2. B: `AdoptLedger` → bloquea. 3. A: escribe y confirma. 4. B entra, adopta. 5. A: siguiente acto → `foreign` |
| Punto de intercalado | B intenta entrar mientras A está dentro |
| Resultado esperado, con nombre | A confirma su acto bajo el dueño A; B adopta después; el siguiente acto de A rehúsa `ledger_foreign_profile` por la fila nueva (leída dentro de su nueva transacción, sin caché) |
| Efectos prohibidos | Nunca un acto de A bajo el dueño B; nunca `SQLITE_BUSY` tragado (se nombra `ledger_busy`) |
| Evidencia | dos conexiones reales, canal tras `BEGIN IMMEDIATE` |
| Los tres moldes | store: `TestWriteTx_theIdentityIsReadInsideTheImmediateTransaction`; store: `TestWriteTx_busyIsNamed`; app: dos apps reales. Mutación: `BeginTx` deferred → la lectura de B puede intercalarse y el molde enrojece |

### R17 · Una puerta que se salta `writeTx` actúa sobre la ÚLTIMA guarda juzgada, nunca sobre ninguna

| Campo | Contenido |
|---|---|
| Garantía | Límite declarado de R1: la guarda de la conexión vale lo último que `writeTx` juzgó; una escritura fuera de `writeTx` (una puerta nueva mal hecha) sigue muriendo si la última guarda era `foreign`/`unreadable`, y pasa si era `ok` aunque el libro haya cambiado de dueño entre medias |
| Estado inicial | Handle de A con guarda `ok`; B adopta por otra conexión |
| Actores | Una puerta a propósito que escribe con `s.db.Exec` sin `writeTx` |
| Secuencia completa | 1. B adopta. 2. La puerta a propósito de A escribe |
| Punto de intercalado | — |
| Resultado esperado, con nombre | La escritura ENTRA (la guarda de A aún dice `ok`): esto es el límite, y el molde lo DOCUMENTA como tal; la siguiente puerta normal de A (`writeTx`) rehúsa y actualiza la guarda |
| Efectos prohibidos | Ninguna promesa de que el límite no exista: la doctrina exige el molde que lo muestra, con su nombre |
| Evidencia | store in-process con dos conexiones |
| Los tres moldes | store: `TestGuard_aDoorOutsideWriteTxSeesTheLastJudgedStanding` (molde de límite); revisión: `writeTx` es el único sitio del paquete con `BeginTx` para escrituras de actos (molde por AST sobre las FIRMAS/llamadas a `BeginTx`, declarado como aviso, no como guarda). Mutación: n/a (es un límite documentado, no una garantía) |

### R18 · El trigger nombra el estado que evaluó; el store traduce ese nombre al centinela

| Campo | Contenido |
|---|---|
| Garantía | G1: el error de SQLite lleva `ledger_guard:<estado>` (1811), el nombre lo pone el trigger que disparó; el store traduce ese nombre al centinela (`ErrLedgerForeignProfile`, `ErrLedgerUnreadable`, `ErrLedgerGuardUnset`) sin leer ni guardar copia del estado (reescrito tras F2 de la oficial 1) |
| Estado inicial | Handle de B |
| Actores | El mapeador de errores del store |
| Secuencia completa | Un acto con cada estado de guarda |
| Punto de intercalado | — |
| Resultado esperado, con nombre | Cada estado produce SU centinela; un 1811 con otro mensaje (una `CHECK` de tabla) NO se traduce a guarda |
| Efectos prohibidos | Nunca un `either/or`: cada estado, su nombre |
| Evidencia | store in-process |
| Los tres moldes | store: `TestGuard_theErrorIsNamedByTheStanding` (tres estados por la puerta sin juicio + un 1811 ajeno; rehecho en §13 C07: la ronda 2 vio que sus patas escribían por `RecordAttempt` y nunca llegaban al mapeador). Mutación: traducir unreadable a foreign → la pata `unreadable` enrojece; unset a foreign → la pata `unset` enrojece |

### R19 · La registración de identidad de la CLI (`principals`) corre antes del rechazo — declarada

| Campo | Contenido |
|---|---|
| Garantía | Límite declarado: las tres tablas de identidad no están bajo la guarda (no son actos); un principal desconocido se escribiría en un libro ajeno antes de que el acto rehúse |
| Estado inicial | Libro fundado por A; CLI con perfil copiado y un principal que el libro no conoce |
| Actores | `wireOperatorIdentity` → `RegisterIdentity` |
| Secuencia completa | 1. `intent create` desde la copia con un registro de principals distinto. 2. Cuentas de `principals` |
| Punto de intercalado | — |
| Resultado esperado, con nombre | El principal se registra (+1) y el acto rehúsa `ledger_foreign_profile`: EJECUTADO esta vez, no predicho |
| Efectos prohibidos | Ningún acto ni recibo |
| Evidencia | CLI in-process con store real |
| Los tres moldes | CLI: `TestCLI_identityRegistrationLandsBeforeTheRefusal` (molde de límite, ejecutado). Decisión para el director en el papel: ¿poner `principals` bajo la guarda? Coste: la fundación y la adopción registran principals dentro de `writeTx`. Propuesta: sí, en este tren, si cabe; si no, declarado |

### R20 · El handle sin identidad del reintento de fundación (`createdHere`) desaparece

| Campo | Contenido |
|---|---|
| Garantía | R3: `CreateLedger` abre con `OpenOperatorFor(path, identidad del perfil)`; el reintento sobre un libro que este proceso creó y no llegó a fundar abre con identidad, juzga `legacy_unfounded` (sin fila) y funda |
| Estado inicial | Bootstrap que retrocedió tras crear el fichero |
| Actores | `CreateLedger` con `createdHere` |
| Secuencia completa | 1. Primer bootstrap crea el fichero y falla antes de fundar (seam). 2. Segundo bootstrap: `createdHere` → `OpenOperatorFor` → funda |
| Punto de intercalado | Entre la creación y la fundación |
| Resultado esperado, con nombre | Fila e identidad del perfil; intento raíz, clave y registro materializados por el mismo handle identificado (dicho) |
| Efectos prohibidos | Ningún handle sin identidad en ningún camino de la app |
| Evidencia | app in-process con seam |
| Los tres moldes | app: `TestCreateLedger_theRetryOpensWithTheIdentity`; store: n/a; CLI: n/a. Mutación: reabrir con el abridor interno sin identidad → no compila fuera del paquete; dentro, el molde de firmas |

### R21 · Privacidad del digest: los caminos no cambian y se declaran

| Campo | Contenido |
|---|---|
| Garantía | El digest de la ruta absoluta viaja al recibo fundacional (evidencia), a la fila de identidad, a los tres lectores, a la puerta de lectura (loopback por defecto) y al log del servidor cuando un cerebro es rehusado; la ruta nunca |
| Estado inicial | — |
| Actores | — |
| Secuencia completa | grep de los sumideros al cerrar |
| Punto de intercalado | — |
| Resultado esperado, con nombre | La lista de sumideros del papel coincide con el grep; ninguno nuevo |
| Efectos prohibidos | Ningún sumidero no listado |
| Evidencia | lectura + grep |
| Los tres moldes | n/a (frase, no cable); barrido de letreros al cerrar |

### R22 · Los tres lectores nombran el estado desde la fila

| Campo | Contenido |
|---|---|
| Garantía | L2: `ledger check`, `receipt verify` y «¿Qué pasa hoy?» dicen `ok`/`legacy_unfounded`/`ledger_foreign_profile`/`ledger_unreadable`/`ledger_mark_malformed` con el dueño, y el veredicto de la cadena no cambia por el estado |
| Estado inicial | Un libro en cada estado |
| Actores | CLI, app, pantalla |
| Secuencia completa | Para cada estado: `ledger check`, `receipt verify`, GET |
| Punto de intercalado | — |
| Resultado esperado, con nombre | Cada lector, cada estado, su nombre; `ledger check` dice además si la fila NO es parte de la cadena (una línea nueva: «identity row: not covered by the chain») |
| Efectos prohibidos | Ningún lector bloquea; ningún lector escribe |
| Evidencia | CLI in-process, app real, jsdom |
| Los tres moldes | CLI: `TestLedgerCheck_namesTheStanding` (cinco estados); app: `TestMount_theReadDoorNamesEveryStanding`; TS: cinco filas. Mutación: no imprimir el estado → los tres enrojecen |

### R23 · El rendimiento del `WHEN` por escritura, medido (no es garantía)

| Campo | Contenido |
|---|---|
| Garantía | Cifra, no promesa: el coste del trigger por INSERT sobre un libro de 200k recibos |
| Estado inicial | Libro grande de prueba |
| Actores | benchmark |
| Secuencia completa | `BenchmarkRecordAttempt` con y sin guarda |
| Punto de intercalado | — |
| Resultado esperado, con nombre | Una cifra en el canto, con la máquina |
| Efectos prohibidos | Ninguna frase de «sin coste» |
| Evidencia | benchmark in-process |
| Los tres moldes | n/a |

### R24 · La pantalla ante `ledger_unreadable`: fila sin botón, causa visible, y el POST directo rehúsa

| Campo | Contenido |
|---|---|
| Garantía | UX + G5: `unreadable` no ofrece «Adoptar libro»; un POST hecho a mano rehúsa 503 con la causa; N intentos son N 503 |
| Estado inicial | App de B sobre libro ilegible |
| Actores | pantalla, curl |
| Secuencia completa | 1. GET. 2. Render. 3. Tres POST seguidos |
| Punto de intercalado | — |
| Resultado esperado, con nombre | Fila «El libro no se puede leer» con la causa; sin botón; tres 503 nombrados; nada escrito |
| Efectos prohibidos | Nunca un botón sobre `unreadable` |
| Evidencia | jsdom + app real |
| Los tres moldes | TS: `unreadable` sin botón (existe); app: tres POST. Mutación: pintar `unreadable` como foreign → el molde TS enrojece |

### R25 · Retirada de los paseos estructurales y de `refuseIfForeign`: nada queda sin su sustituto

| Campo | Contenido |
|---|---|
| Garantía | Cada guarda de Go retirada tiene su fila en la tabla de 16 puertas contra el trigger real; los dos paseos se borran; el molde de firmas (R12) y la puerta a propósito (R09) los sustituyen |
| Estado inicial | El árbol |
| Actores | — |
| Secuencia completa | 1. Borrar `refuseIfForeign` y las guardas. 2. Borrar los dos paseos. 3. La tabla de puertas corre contra el trigger |
| Punto de intercalado | — |
| Resultado esperado, con nombre | Las 16 puertas rehúsan por nombre con cero filas, sin ninguna guarda de Go |
| Efectos prohibidos | Ninguna guarda de Go que quede «por si acaso» sin declarar |
| Evidencia | store in-process |
| Los tres moldes | store: la tabla de 16 puertas (`TestForeign_everyActDoorRefusesBeforeAnyWrite` renombrada a `TestGuard_everyActDoorDiesInSQLite`). Mutación: quitar un trigger → su puerta enrojece |


## 5 · Tests aprobados que cambian (y por qué)

Los 16 de la tabla de puertas cambian de oráculo (el trigger real en vez de
triggers de aborto del test) y de nombre; los moldes de `SetProfileIdentity`
desaparecen con el método; `TestForeign_everyInsertSiteIsGuarded` y
`TestCLI_everyStoreOpenSetsTheProfileIdentity` se BORRAN (su trabajo lo hacen
R09, R12 y SQLite); `TestMark_aMalformedMarkIsNamedNotAbsent` se convierte en
R03 + R06; `foundedStore`/`foundedLedger` fundan con fila. Ningún aserto se
relaja: cada uno se traslada a la garantía que ahora lo sostiene, y el canto lo
declara uno a uno.

## 6 · Lo no verificable, declarado

- Windows (`ToLower` de la ruta) y symlink: por lectura; tren propio.
- La edición a mano de la fila con un digest válido (R04): límite del modelo, no
  se «detecta».
- Una puerta que se salte `writeTx` (R17): actúa sobre la última guarda, no
  sobre ninguna; molde de límite.
- El coste del `WHEN` (R23): cifra, no promesa.
- La app empaquetada y la pasada manual del director: fuera de los moldes.

## 7 · Orden de trabajo tras la aprobación del plan

1. Sonda Q9 (hook de conexión por DSN) — 20 min, con captura.
2. Todos los moldes en ROJO (R01–R25 salvo R21/R23), `red.txt`.
3. R2 (tabla + migración + lectores) → R3 (`writeTx`, abridores con identidad,
   ocho llamadores) → R1 (guarda por conexión, hook, retirada de guardas de Go
   y de los paseos) → R13 (aprobaciones y §12-ter).
4. Mutaciones, canto, pasada interna sobre el diff, barrido de letreros,
   oficial (30 min); tope de DOS rondas.
5. HANDOFF (la ficha pasa de CERRADO a la verdad), notas, gate completo con la
   cifra, commit, marcador, push, PR, «fusiona», tag v0.16.2.

## 8 · Las cinco preguntas, antes de la lectura del copiloto

1. ¿Alguna frase más ancha que su cable? Las garantías G1–G9 tienen cada una su
   fila y sus moldes; los límites (R04, R17, R19, R21, R23) están nombrados
   como límites.
2. ¿Cada símbolo citado existe o está marcado como NUEVO? Los nuevos llevan
   nombre propuesto; los ocho llamadores y las tres firmas de abridor son del
   árbol (grep de hoy).
3. ¿Se vende un nivel de evidencia por otro? Cada fila dice el suyo.
4. ¿Cifras de memoria? La sonda tiene captura; el resto no tiene cifras.
5. ¿Algo a medias sin decir? R14 depende de una sonda que no se ha hecho (Q9),
   y se dice.

## 9 · Canto: garantía → molde que la rompe → mutación ejecutada → nivel de evidencia

| Garantía / fila | Molde | Mutación | Evidencia |
|---|---|---|---|
| R01 fila ausente con recibo marcado → `ledger_unreadable`, bloquea a todos, adopción incluida | `TestIdentity_aMissingRowWithAMarkedReceiptIsUnreadable`; `TestMount_anUnreadableIdentityRefusesAdoption/deleted_before…`; `TestLedgerCheck_namesEveryStandingFromTheRow` | M224 (leer «sin fila» como legacy) — roja en los tres | store + 2ª conexión; app real en loopback; CLI in-process |
| R02 la adopción lee la fila DENTRO de su transacción | `TestAdopt_readsTheIdentityInsideItsOwnTransaction` (desaparecida / cambiada de manos); `TestAdopt_seesARowThatVanishedWhileItWaited` (desaparece mientras espera); `…/deleted_after_the_app_looked` (app) | M225 (admitir ilegible) VERDE → rama muerta retirada, declarado; M226 (juzgar antes de la transacción) VERDE sobre el molde de serialización → M226-bis roja sobre el molde que la ve desaparecer | store + 2ª conexión con canal; app real |
| R03 la fila rechaza toda forma no canónica en SQLite | `TestIdentity_theRowRefusesEveryNonCanonicalForm` (nueve formas) | M227 (sin `CHECK`) roja | store + 2ª conexión |
| R04 la fila es el estado; el recibo, la evidencia (límite declarado) | `TestStanding_followsTheRowNotTheReceipt`; `TestLedgerCheck_theRowIsTheStateAndTheReceiptIsTheEvidence` | M228 (derivar del recibo) roja en ambos | store; CLI in-process |
| R05/R16 adopciones y puertas serializan bajo `BEGIN IMMEDIATE`; `BUSY` nombrado | `TestAdopt_twoAdoptionsSerialize`; `TestWriteTx_theIdentityIsReadInsideTheImmediateTransaction`; `TestWriteTx_busyIsNamed` | M229 (DSN sin `_txlock`) roja | store + 2ª conexión con canal; 5 s de timeout real |
| R06 migración: marca canónica → fila; no canónica → sin fila y nombrada | `TestMigrate_identityRowFromTheReceiptMark` (tres casos); `TestLedgerCheck_namesAMalformedReceiptMarkAfterTheMigration` | M230 (convertir basura en fila) roja en ambos | store sobre fichero degradado a v15; CLI |
| R07 sin fila y sin marca → legacy, no bloquea, adoptable | `TestStanding_aLedgerWithNoRowAndNoMarkIsLegacy` | M231 (leer como ajeno) roja | store |
| R08 la guarda vive en la conexión, no en el fichero | `TestGuard_livesInTheConnectionNotInTheFile` (12 triggers en B, 0 en la raw, 0 en `sqlite_master`) | M232 (`CREATE TRIGGER` sin `TEMP`) roja | dos conexiones reales |
| R09/R25 una puerta sin guarda muere en SQLite; 16 puertas reales rehúsan por nombre | `TestGuard_anUnguardedDoorDiesAnyway` (seis grafías); `TestGuard_everyActDoorDiesInSQLite` (16 puertas, oráculo el trigger real) | M233 (`actions` fuera de la guarda) roja en ambos | store |
| R10 sin mantenimiento sobre libro ajeno; el arranque no recupera ni registra | `TestOpen_aForeignLedgerGetsNoMaintenance`; `TestMount_aForeignLedgerRefusesEveryDoorButAdopt` (arranca) | M234 (no juzgar) roja; M245 (el arranque recupera) roja | store; app real |
| R11 recibo fundacional y fila son una transacción | `TestFounding_receiptAndRowAreOneTransaction` (seam) | M235 no aplicada (ancla); M235-bis VERDE (seam antes del commit); M235-ter (commit antes del seam) roja — declarado | store + seam + 2ª conexión |
| R12 ningún abridor exportado sin identidad; sin setter | `TestOpeners_everyExportedOpenerTakesAnIdentity` (go/ast sobre FIRMAS) | M236 (exportar `OpenOperator`) roja | AST |
| R13 aprobaciones nombran ajeno/ilegible | `TestApprovals_aForeignOrUnreadableLedgerIsNamed`; `TestMount_theApprovalDoorsNameAForeignLedger` (app real, petición aparcada por el fundador); FR-TEST-6 | M242 (sin entradas en el registro) roja; M243 (adaptador sin nombrar) roja | httptest; app real |
| R14 la guarda sobrevive a la reconexión del pool | `TestGuard_survivesAPoolReconnect` (`ConnMaxLifetime` 1 ms) | M237 (hook sin triggers) roja | pool real |
| R15 guarda vacía falla cerrada | `TestGuard_anEmptyGuardTableFailsClosed` | M238 (sin `COALESCE`) roja | store |
| R17 puerta fuera de `beginWrite`: última guarda juzgada (límite) | `TestGuard_aDoorOutsideWriteTxSeesTheLastJudgedStanding` | M239 (rechazo sin actualizar la fila de guarda) roja | store + 2ª conexión |
| R18 el nombre lo pone el trigger que disparó | `TestGuard_theErrorIsNamedByTheStanding` (foreign, unreadable, unset por la puerta sin juicio; `CHECK` ajena) — rehecho en §13 C07 | M265 (unreadable → foreign), M266 (unset → foreign), M272 (foreign → unreadable) rojas; M240 enrojeció `TestGuard_anEmptyGuardTableFailsClosed`, no este molde (hallazgo de la ronda 2) | store |
| R19 tablas de identidad bajo la guarda | `TestCLI_identityRegistrationIsRefusedOnAForeignLedger` (principals 0 → 0) | M241 VERDE (solo triggers: `beginWrite` ya rehúsa) → M241-bis (triggers Y `beginWrite`) roja — declarado: dos líneas | CLI in-process |
| R22 los lectores nombran cada estado desde la fila | `TestLedgerCheck_namesEveryStandingFromTheRow` (cuatro estados, dos verbos) | M244 (no imprimir) roja | CLI in-process |
| R20 el reintento de fundación abre con identidad | por construcción (`OpenOperatorFor(path, l.profile)`); molde existente `TestCreateLedger_readoptsOnlyWhatThisProcessCreated` | — (firma) | app |
| R21 privacidad | frases acotadas en godoc, notas y HANDOFF | — | lectura |
| R23 coste del `WHEN` | sin benchmark en este tren: DECLARADO como pendiente; ninguna frase de rendimiento en las notas | — | — |
| R24 pantalla ante ilegible | molde TS existente (`unreadable` sin botón) | M122 (tren anterior) | jsdom |

**Tests aprobados que cambiaron, declarados:** los 16 de la tabla de puertas
(oráculo: el trigger real; entradas VÁLIDAS para las siete puertas que
validaban antes de escribir); `TestIdentity_MigrationDoesNotUpgradeHistoricalClaims`
(el literal 15 → `schemaVersionCurrent`); `TestApprovals_EveryNameIsInTheRegistry`
y hermanas (22 → 24 nombres, con las dos filas de §12-ter); el molde de los
abridores (nombres de parámetros aplanados); los tests del paquete `sqlite`
pasan a los abridores internos y al setter interno; los de los demás paquetes
abren con `testProfileIdentity` y los `Build` con `withTestProfile()`;
RETIRADOS: `TestForeign_everyInsertSiteIsGuarded`, `TestCLI_everyStoreOpenSetsTheProfileIdentity`,
`TestMark_aMalformedMarkIsNamedNotAbsent`, `TestLedgerCheck_aMalformedMarkIsUnreadable`
(sustituidos por R03, R06, R09, R12). Ningún aserto se relajó.

## 10 · Recuento y estado

Mutaciones del rediseño (M224–M254 y sus repeticiones, con las de la pasada
interna y de la oficial, ronda 1): 38 cabeceras, 1 no aplicada (M235), 37
ejecutadas, 1 que no compilaba (M250, repetida como M250-bis), 6 verdes o
bloqueadas que fueron hallazgo del instrumento o del molde y se repitieron en
rojo o se declararon (M225 rama muerta; M226 → M226-bis; M235-bis → M235-ter;
M241 → M241-bis; M248 → obsoleta con el espejo retirado, sustituida por M253;
M253 bloqueó → M253-bis), 30 rojas. Del tren de hoy (regla de §9 del papel
anterior, desde M57, con la convención no escrita de entonces — la regla
escrita y el guion que la deriva están en §13.9 y en `scripts/mutation_tally.py`): 216 cabeceras, 3 no aplicadas, 1 declarada, 212
ejecutadas, 7 no compilaban, 9 verdes o bloqueadas repetidas o declaradas,
196 rojas.

Estado: `IMPLEMENTED + VERIFIED` por los moldes de las 25 filas (rojo capturado
en `red.txt`, verde tras R2 → R3 → R1 → R13) y por el gate local; las pasadas
(interna, barrido de letreros, oficial con tope de dos rondas) en el informe.

## 11 · Delta tras la pasada interna sobre el diff (30 min, VETO MANTENIDO → curado)

Dos P2 de producto con la misma causa —la guarda tenía dos estados, la fila
temporal y el espejo Go, que divergían en tres secuencias— y dos puertas de
producción escribían fuera de `beginWrite`; siete P3.

| # | Hallazgo | Cura |
|---|---|---|
| F1 P2 [PRODUCTO] | el hook instalaba la guarda en `ok` sin juzgar la conexión nueva (R14 decía lo contrario); `CreateIntent` y `CreateGrant` escribían por el pool sin `beginWrite`: tras una reconexión del pool, un handle ajeno escribió un intento y un grant | el hook JUZGA cada conexión nueva sobre ella misma (`judgeOnConn`: la fila, y sin fila los recibos) e instala la guarda con ese estado; `CreateIntent` y `CreateGrant` escriben por `beginWrite` (ninguna puerta de producción queda fuera); `TestGuard_survivesAPoolReconnect` exige ahora la fila de guarda juzgada y que la puerta sin guarda y las dos puertas mueran; `TestGuard_everyDoorJudgesEvenWithAStaleGuard` fuerza la guarda a `ok` y exige que cada puerta juzgue la fila por sí misma; M246 (hook en `ok`), M249 (`CreateIntent` por el pool) rojas |
| F2 P2 [PRODUCTO] | el rechazo de la adopción sobre un libro ilegible fijaba solo el espejo: la fila de guarda seguía en `ok` y una puerta sin juicio entraba | el rechazo fija fila y espejo (`setGuardIn` por el pool tras el rollback); `TestAdopt_aRefusedOrAbortedAdoptionLeavesTheGuardConsistent/refused…`; M247 rojo |
| F3 P3 [PRODUCTO+INSTRUMENT] | tras una adopción abortada el rollback revertía la fila temporal pero no el espejo: el nombre era `ledger_guard_unset` en vez del estado | el levantamiento dentro de la adopción es solo de la FILA (`setGuardRowIn`), y la adopción re-juzga la guarda al salir por cualquier camino (`refreshGuard` en su `defer`); `…/aborted…`; M248 verde (el re-juicio ya cubría) → M248-bis (las dos líneas) rojo |
| F4 P3 [DOC] | «el arranque sobre un libro ajeno arranca» solo con las claves del perfil junto al libro | acotado en notas y HANDOFF |
| F5 P3 [DOC] | la migración no busca «el último canónico»: toma el último marcado y rehúsa si no es canónico | acotado en HANDOFF; G7 y R06 lo decían ya en su letra |
| F6 P3 [DOC] | HANDOFF decía «cifra en el canto» para R23 | «sin benchmark en este tren, declarado pendiente» |
| F7 P3 [PRODUCTO] | `refreshGuard` y `installGuard` tragaban el error del refresco | `refreshGuard` devuelve el error; los abridores lo propagan (un open cuya guarda no se puede fijar falla cerrado); en las ramas de rechazo el error del refresco queda detrás del rechazo que ya se devuelve (declarado) |
| F8 P3 [DOC] | «un libro ajeno se abre sin migrar» no cubría v15 → v16 (sin fila que juzgar) | acotado: ese paso lo hace el primer perfil que abra el libro; desde v16 un libro ajeno no se migra |
| F9 P3 [DOC/INSTRUMENT] | «nueve triggers» en §2/R08 frente a doce; `(UPDATE)` frente a `UPSERT`; comentario rancio en la tabla de puertas; `mapGuardError` solo veía los códigos BUSY primarios | cifras y comentario corregidos; el mapeo pliega los códigos extendidos al primario (`code & 0xff`) |

**Lo que la pasada vio y el plan no:** que la guarda por conexión tiene dos
estados (fila y espejo) y tres secuencias en que divergen — reconexión del
pool, rechazo de la adopción, adopción abortada — y que dos puertas escribían
fuera del punto único. Las tres secuencias tienen molde y mutación roja.

## 12 · Delta tras la oficial (ronda 1 de 2, VETO MANTENIDO → curado)

Un P2 de producto, siete P3. Las cinco reproducciones de la regla 1 con el
binario compilado terminaron como el mandato exige.

| # | Hallazgo | Cura |
|---|---|---|
| F1 P2 [PRODUCTO] | el hook leía «no such table» como ausencia también sobre un esquema v16 con la tabla borrada (corrupción disfrazada de ausencia) e instalaba `ok` en cada conexión de reemplazo: la segunda línea abierta sobre un libro corrupto | (cura de la ronda 1, SUPERADA por §13) `judgeOnConn` consultaba `action_schema.version` en la misma conexión y leía «sin tabla y versión < 16, o sin `action_schema`» como fichero fresco: la ronda 2 mostró que un fichero sin `action_schema` no es fresco y que otras dos ramas seguían abiertas; la cura definitiva enumera los estados benignos (§13.1). `TestGuard_aDroppedIdentityTableIsUnreadableForTheHook` (tabla borrada, reconexión, puerta sin juicio muere por nombre); M250-bis rojo |
| F2 P3 [PRODUCTO] | el hook juzgaba la fila pero el NOMBRE salía del espejo Go, que ninguna reconexión actualizaba: `ledger_guard_unset` donde el trigger evaluó `foreign` | el espejo desaparece: un trigger por estado nombra el estado que evaluó (`ledger_guard:<estado>`), `mapGuardError` traduce por ese nombre; nada que re-sincronizar; M252 rojo |
| F3 P3 [PRODUCTO+DOC] | `approvalDetail`, una lectura, abría por `beginWrite` (sustitución mecánica) y rehusaba por ajeno/ilegible con `BEGIN IMMEDIATE` | vuelve a una transacción de solo lectura; `TestApprovalDetail_isAReadAndNeverBlocks`; M251 rojo; «Reads never block» vuelve a ser cierto |
| F4 P3 [DOC+INSTRUMENT] | el `defer` con `refreshGuard` que §11-F3 y la salvedad de M248 describían había quedado en `FinishFounding` (un anclaje casó la primera aparición) y no en `AdoptLedger`; M248 a solas enrojecía | con el espejo retirado, el levantamiento de la guarda es una escritura dentro de la transacción de la adopción que revierte con el aborto: no hay `defer` ni copia; M248/M248-bis declaradas obsoletas y sustituidas por M253 (levantar por el pool, fuera de la transacción → el aborto deja `ok` y el molde enrojece) |
| F5 P3 [DOC] | la nota decía «toda escritura … que se saltara ese juicio» donde el cable son INSERT (diez tablas) y UPDATE (intentos, grants) | acotada |
| F6 P3 [INSTRUMENT] | either/or en la pata «aborted» del molde de consistencia (`err == nil`) | exige el nombre `receipt_unsigned` |
| F7 P3 [INSTRUMENT] | el molde de abridores miraba nombres que empiezan por `Open`; un `NewStore` habría pasado | juzga por TIPO de retorno (`*Store`); M254 (`NewStore`) rojo |
| F8 P3 [DOC] | «only restoring the ledger lifts it» (una fila escrita a mano también lo levanta: límite R04); godoc de `guardHook` rancio; `Warn` del arranque nombraba «ajeno» también para ilegible | los tres textos acotados. *Superado (tren E, tanda 5): el 503 ya no dice «only»; dice que su remedio es sustituir el fichero del libro.* |

**Lo que la oficial vio y la interna no:** el «no such table» del hook como
corrupción-disfrazada-de-ausencia; el nombre que salía de un espejo que la
reconexión no tocaba; la lectura alcanzada por la sustitución mecánica; el
`defer` en la función equivocada; el either/or; el molde de abridores por
nombre.

## 13 · Tren C (2026-09-25): la cura acotada de F-1 y los seis P3 de la oficial ronda 2 — plan, antes de una línea de código

**Autorización.** El director, 2026-09-25, tras el paquete de parada de la
ronda 2 (§12 del informe): `judgeOnConn` cambia de POLARIDAD; cuatro moldes
con la reproducción A1–A4 del auditor como criterio; dos pines de los estados
benignos; una mutación por molde; los seis P3; cobertura de `sqlite` al 85 %
con pruebas reales; HANDOFF y papel con la verdad; pasada interna; barrido de
letreros; UNA ronda oficial acotada al delta. Si mantiene producto, parada y
paquete; si levanta, gate, commit, marcador, push, PR, «fusiona», tag. El
plan de abajo es el §12.6 del informe que el director aprobó; lo que añade
respecto a ese texto se marca como tal.

### 13.1 · La garantía, literal

**G1-ter.** El hook de conexión juzga la conexión nueva sobre sí misma y abre
`ok` SOLO en estas formas: (1) fichero fresco, `sqlite_master` sin ningún
objeto Y `freelist_count` cero (un libro vaciado de sus tablas conserva sus
páginas libres: no es fresco); (2) libro anterior a la fila, `action_schema`
legible con EXACTAMENTE una fila, numérica, de 1 a 15, y sin tabla
`ledger_identity`; (3) sobre un libro con `action_schema` de exactamente una
fila numérica ≥ 16, las diez tablas guardadas (doce eventos) y la tabla
`receipts` presentes, fila canónica propia; (4) sobre ese mismo libro, tabla
`ledger_identity` presente y vacía y sin marca `profile:` en un recibo
`SUCCEEDED` de la partición principal. Una fila canónica ajena abre
`ledger_foreign_profile`. TODA otra forma —error de lectura, tabla que falta
(de esquema, de identidad, de recibos o cualquiera de las guardadas), versión
no numérica o fuera de rango, dos filas o ninguna en `action_schema`, fila con
otro `id`, fila no canónica, marca sin fila, fichero sin objetos con páginas
libres— abre `ledger_unreadable`. El hook no interpreta texto de error:
ninguna rama decide por «no such table». Y el veredicto `ledger_unreadable`
es PEGAJOSO en la conexión: `setGuardRowIn` no lo pisa y `beginWrite` y
`beginAdoption` rehúsan por nombre sobre él aunque la primera línea juzgue la
fila `ok`; solo una conexión nueva, cuyo hook juzga de cero, lo levanta. El
abridor tampoco siembra un esquema en un fichero que no sea fresco
(`ErrFileNotFresh`): un libro vaciado se nombra, no se refunda.
*Superado (tren E, tandas 1 y 2): «TODA otra forma —error de lectura…» ya no
es la regla. Un error de lectura solo es veredicto con un código estructural,
y un prefijo vacío de la siembra v1 es fresco.*

Añadido respecto al §12.6 (declarado): con `ledger_identity` presente, la
versión del esquema se exige numérica, única y ≥ 16 (una tabla de identidad
sobre un esquema anterior no es un estado que el store escriba: la migración
v15→v16 la crea y sube la versión en la misma transacción). Y una tabla de
identidad con filas cuyo `id` no es 1 es ilegible, aunque la `CHECK` lo impida
a las puertas del store.

### 13.2 · Diseño de la cura (en las líneas que caben)

`judgeOnConn` pasa a un procedimiento de decisión sobre la conexión nueva, en
este orden, con `driverScalar` (una consulta, un valor):

1. `SELECT COUNT(*) FROM sqlite_master` → error: `ledger_unreadable`; `0` → `PRAGMA freelist_count` debe ser `0` → `ok` (fresco: ningún objeto, ni tabla ni vista ni índice, y ninguna página libre); páginas libres → `ledger_unreadable`.
2. Presencia de `action_schema`, `ledger_identity` y `receipts` por `sqlite_master` (una consulta por tabla). Sin `action_schema`: `ledger_unreadable`.
3. `SELECT COUNT(*) FROM action_schema` debe ser `1`; `SELECT version FROM action_schema` debe convertir a entero; si no: `ledger_unreadable`.
4. Sin `ledger_identity`: versión de 1 a 15 → `ok` (la migración viene después); otra → `ledger_unreadable`.
5. Con `ledger_identity`: versión < 16 → `ledger_unreadable`; cualquiera de las diez tablas guardadas ausente → `ledger_unreadable` (el hook no podría instalar su trigger: no abre); `receipts` ausente → `ledger_unreadable` (con fila o sin ella: la pasada interna del tren C halló que solo se exigía sin fila). `SELECT COUNT(*) FROM ledger_identity` debe ser `0` o `1`; con `1`, `SELECT owner_digest … WHERE id = 1` debe encontrar la fila (si no, `ledger_unreadable`): canónica propia `ok`, canónica ajena `ledger_foreign_profile`, no canónica `ledger_unreadable`.
6. Con `0` filas: la consulta de la marca con error → `ledger_unreadable`; con marca (canónica o no) → `ledger_unreadable`; sin marca → `ok` (legacy o recién creado: la guarda no bloquea nada).
7. Pegajoso (añadido tras la pasada interna): el `UPDATE` de la fila de guarda lleva `WHERE standing <> 'ledger_unreadable'`, y `beginWrite`/`beginAdoption` leen la fila tras el juicio de `judgeIn` y rehúsan `ErrLedgerUnreadable` si dice ilegible; el nombre más específico de `judgeIn` (`ledger_mark_malformed`) gana porque se juzga antes. El abridor (`openWithIdentity`, y la sonda del operador `storedSchemaVersion`) exige `requireFreshOrStore` antes de sembrar esquema alguno: con `action_schema`, sigue; sin ella, cero objetos y cero páginas libres o `ErrFileNotFresh`.

Lo que NO cambia: el bucle de triggers del hook sigue tolerando «no such
table» al CREAR un trigger (sobre un fichero fresco las tablas no existen y
nada puede insertarse en una tabla que no existe; `installGuard` los crea tras
el esquema). `judgeIn` (la primera línea) no cambia. `ownsOrLegacy` no cambia
(§13.6).

**F-6.** `guardHook` hace UNA carga del registro: `identity, ok :=
guardRegistry.Load(nonce)`; sin `ok`, error con nombre («unknown handle»).
**F-7.** `Prune` abre por `beginWrite` (juicio dentro de la transacción
inmediata), cuenta y borra DENTRO de la misma transacción (`tx.QueryRowContext`,
`txExec`) y confirma; `refuseMaintenance` queda solo para `RecoverPreviousLife`.
Un seam `beforePruneDelete` entre el juicio y el DELETE, solo para el molde.
**F-2.** El molde de nombres escribe por la puerta sin juicio
(`unguardedDoorForTest`) tras fijar la guarda de la conexión en cada estado:
así el `RAISE` dispara y `mapGuardError` es quien nombra.
**F-3.** `returnsStore` recorre el tipo de resultado entero (puntero, slice,
array, map, chan, campos de struct literal, resultados de func) y resuelve los
tipos con nombre del paquete (struct con nombre que contenga un `*Store`); el
paseo incluye los métodos de receptores exportados distintos de `Store`. Límite
declarado: un `*Store` detrás de una interfaz no se ve estáticamente.
**F-4.** Los ocho textos de §12.5 del informe se corrigen (ficheros: `ledger_identity.go`,
`profile_standing.go`, `internal/app/profile_identity.go`, `ledger_identity_test.go`,
este papel §2 R1 y §4 R18). **F-5.** La mutación del rechazo se ejecuta y se
captura (M255).

### 13.3 · Plan de fallos del delta (filas C01–C13)

| Fila | Garantía | Estado inicial y actores | Secuencia | Resultado con nombre | Efectos prohibidos | Evidencia y molde | Mutación |
|---|---|---|---|---|---|---|---|
| C01 | G1-ter rama A1 | Libro v16 fundado por A; handle guardado de A; segunda conexión real | `DELETE FROM ledger_identity; DROP TABLE receipts`; `Standing()`; `SetConnMaxLifetime(1ms)`; lectura de `temp.profile_guard`; puerta sin juicio; `RecordAttempt` | guarda `ledger_unreadable`; puerta `ErrLedgerUnreadable`; `RecordAttempt` `ErrLedgerUnreadable` | `actions rows = 1` | in-process, conexiones reales, pool real: `TestGuard_theHookFailsClosedOnEveryCorruptShape/A1` | la rama 6 devuelve `ok` cuando falta `receipts` → roja |
| C02 | rama A2 | ídem | `DROP TABLE ledger_identity; DROP TABLE action_schema` | ídem | ídem | `…/A2` | la rama 2 devuelve `ok` sin `action_schema` → roja |
| C03 | rama A3 | ídem | `DROP TABLE ledger_identity; UPDATE action_schema SET version = 'sixteen'` | ídem | ídem | `…/A3` | la rama 3 devuelve `ok` cuando la versión no convierte → roja |
| C04 | rama A4 | ídem | `DROP TABLE ledger_identity; DELETE FROM action_schema; INSERT (1); INSERT (16)` | ídem | ídem | `…/A4` | la rama 3 no cuenta las filas → roja |
| C05 | benigno 1 | Fichero nuevo; conexión del driver | `judgeOnConn` sobre la conexión cruda | `ok`; y `OpenFor` funda | — | in-process, conexión del driver: `TestGuard_theHookOpensTheTwoBenignStates/fresh` | `0` tablas → `ledger_unreadable` → roja |
| C06 | benigno 2 | Libro v16 rebajado a v15 (tabla borrada, versión 15) | `judgeOnConn`; `OpenFor` migra y siembra | `ok`; tras migrar, `ok` de A | — | `…/v15` | versión < 16 → `ledger_unreadable` → roja |
| C07 | R18 rehecha | Handle guardado de A | guarda en `foreign` (fila ajena + reconexión), en `unreadable` (fila borrada + reconexión), en `unset` (`UPDATE temp.profile_guard SET standing = 'junk'`); puerta sin juicio en cada una; `CHECK` ajena | `ErrLedgerForeignProfile`, `ErrLedgerUnreadable`, `ErrLedgerGuardUnset`; la `CHECK` no se traduce | — | store in-process: `TestGuard_theErrorIsNamedByTheStanding` | `mapGuardError` unreadable → foreign (roja la pata unreadable); unset → foreign (roja la pata unset) |
| C08 | R12 ampliada | Paseo AST | tres abridores nuevos: `[]*Store`, struct con campo `*Store`, método de otro receptor | los tres, ofensores | — | `TestOpeners_everyExportedOpenerTakesAnIdentity` | fichero nuevo con cada forma → roja (y verde ANTES de la cura, capturado: el rojo del instrumento) |
| C09 | F-7 | A dueño, handle guardado con `capRows = 0`; B con handle propio | `a.Prune`; en el seam, B adopta en una goroutine | B NO confirma mientras `Prune` tiene la transacción; `Prune` borra los terminales de A; B confirma después | una adopción confirmada entre el juicio y el DELETE | in-process, dos handles reales: `TestPrune_judgesAndDeletesInOneImmediateTransaction` | volver al juicio por el pool y al DELETE por el pool → roja |
| C10 | F-6 | nonce registrado y olvidado | `guardHook` directo con ese nonce | error «unknown handle», sin `panic` | `panic` | unit in-process: `TestGuard_theHookNamesAForgottenHandle` | ignorar `ok` y afirmar el tipo → `panic` → roja. Declarado: la carrera `Close`/reconexión NO se reproduce; queda cerrada por construcción (una sola carga) |
| C11 | F-5 | molde de consistencia, pata «refused» | quitar `setGuardIn(…, unreadable)` del rechazo | la pata enrojece | — | M255 capturada en `mutations.txt` | es la mutación |
| C12 | F-4 | los ocho textos | `grep` de cada frase rancia tras la cura | cero apariciones | — | verificación por lectura, sin molde | — |
| C14 (añadida en el verde) | G1-ter: libro v16 sin una tabla guardada | libro fundado por A; segunda conexión | `DROP TABLE intents`; reconexión; puerta sin juicio | guarda `ledger_unreadable`; puerta `ErrLedgerUnreadable` | `actions rows = 1` | `TestGuard_theHookRequiresEveryGuardedTable` | el hook deja de exigir las tablas guardadas → roja |
| C13 | cobertura | `sqlite` 84,9 % | ataques reales sobre los abridores (identidad malformada, vacía, con mayúsculas: `ErrProfileIdentityMalformed` en los tres), el lector de solo lectura sobre un libro ajeno (lee, nombra `foreign`, no escribe), un libro ajeno de un esquema más nuevo (no se migra ni se rebaja) | los nombres exactos | — | in-process; medida con `go test -race -coverprofile` del paquete solo | una por molde |

### 13.4 · Lo que la cura NO cubre, declarado

- El veredicto del hook sobre los dos estados benignos no es observable por la
  secuencia de apertura: entre el hook e `installGuard` no hay escritura en
  tabla guardada (`migrate` escribe `action_schema` y crea tablas), e
  `installGuard` vuelve a juzgar por `judgeIn`. Por eso C05 y C06 juzgan la
  función sobre una conexión cruda del driver, y el fin a fin (fundar, migrar)
  lo prueban los moldes que ya existen. Se declara: no es una garantía que
  una puerta pueda romper hoy.
- La carrera de F-6 (C10) no se reproduce; la cura la cierra por construcción.

### 13.5 · Lo visto fuera del alcance (registrado, no curado)

- `ownsOrLegacy` (`ledger_identity.go`) decide «migrar» por el texto «no such
  table», la misma clase de F-1. Consecuencia analizada: sobre un libro v16 sin
  tabla de identidad, `migrate` no hace nada (versión al día) e `installGuard`
  juzga `ledger_unreadable`; sobre un fichero sin `action_schema`, el abridor
  intenta migrar desde v1 y muere en «duplicate column» (captura A6 del
  auditor): cerrado por accidente, no por diseño. Backlog: que `ownsOrLegacy`
  use la misma enumeración de estados benignos que el hook. (Cerrado en el
  tren D: `ownsOrLegacy` desaparece y `readOwner` no lee texto; §14.)

### 13.6 · Orden de trabajo

Rojo de C01–C10 y C13 (captura en `red.txt`) → verde F-1 → verde F-7, F-6,
F-2, F-3 → F-4 textos → M255 y las mutaciones de C01–C10, C13 → cobertura
medida → HANDOFF, notas, este papel → pasada interna → barrido de letreros →
oficial (una ronda, acotada al delta).

### 13.7 · Las cinco preguntas, antes del rojo

1. ¿Alguna frase más ancha que su cable? §13.4 acota los pines; C10 declara la carrera no reproducida; §13.5 declara lo no curado. No.
2. ¿Cada nombre citado existe? Los moldes nuevos aún no: se nombran aquí y nacen con ese nombre. Los existentes, comprobados por `grep`.
3. ¿Se vende un nivel de evidencia por otro? No: C05/C06 son juicio sobre conexión cruda; C09 dos handles reales en proceso; nada es binario aparte.
4. ¿Cifras de memoria? La cobertura (84,9 %) es del `coverage.out` del gate q19; las líneas de `judgeOnConn` se citan por frase, no por número.
5. ¿Algo a medias sin decir? El §9 y el §12 F1 se corrigen cuando existan los números de mutación; dicho.

### 13.8 · Canto del tren C: garantía → molde que la rompe → mutación ejecutada → nivel de evidencia

Rojo capturado antes de la cura en `evidence/v0.16.2/red.txt`, sección «Tren
C»: C01–C04 rojos con la fila de guarda en `ok` tras la reconexión (la frase
exacta del auditor), C09 rojo con la adopción de B confirmada entre el juicio y
el `DELETE`; los pines (C05, C06, C10, C13) y el molde de nombres rehecho
verdes antes de la cura, como §13.3 declara: su rojo es la mutación.

| Garantía | Molde | Mutación ejecutada | Nivel |
|---|---|---|---|
| C01 A1 `receipts` borrada sin fila → `ledger_unreadable` | `TestGuard_theHookFailsClosedOnEveryCorruptShape/A1` | M259 (sin `receipts` → `ok`) roja | in-process, segunda conexión real, reconexión del pool |
| C02 A2 identidad y esquema borrados | `…/A2` | M260 (sin `action_schema` → `ok`) roja | ídem |
| C03 A3 versión no numérica | `…/A3` | M261 (`Atoi` con error → `ok`) roja | ídem |
| C04 A4 dos filas de esquema | `…/A4` | M262 (sin contar filas) roja | ídem |
| C05 fichero sin tablas → `ok` | `TestGuard_theHookOpensTheTwoBenignStates/fresh` | M263 (cero tablas → `ledger_unreadable`) roja | conexión cruda del driver sobre fichero real |
| C06 libro anterior a la fila → `ok` y migra | `…/v15` | M264 (versión < 16 → `ledger_unreadable`) roja | ídem + `OpenFor` in-process |
| C07 el nombre lo pone el trigger; el mapeador traduce | `TestGuard_theErrorIsNamedByTheStanding` (foreign, unreadable, unset por la puerta sin juicio; `CHECK` ajena) | M265 (unreadable → foreign), M266 (unset → foreign), M272 (foreign → unreadable) rojas, cada una en SU pata | store in-process |
| C08 todo abridor que entregue un `Store` pide identidad | `TestOpeners_everyExportedOpenerTakesAnIdentity` | M256 VERDE antes de la cura (la captura del hallazgo), M256-bis (`[]*Store`), M257 (struct con nombre), M258 (método de otro receptor) rojas | paseo AST sobre el paquete |
| C09 la poda juzga y borra en una transacción inmediata | `TestPrune_judgesAndDeletesInOneImmediateTransaction` (oráculo por imposibilidad desde la pasada interna: una tercera conexión con `busy_timeout(0)` DEBE recibir `SQLITE_BUSY` dentro del seam; la adopción de B aterriza solo después) | M267 (juicio y `DELETE` por el pool, sin transacción) roja con el oráculo por plazo; M267-bis roja con el oráculo por imposibilidad | tres conexiones reales en proceso, seam entre juicio y `DELETE` |
| C10 un handle olvidado se nombra, sin `panic` | `TestGuard_theHookNamesAForgottenHandle` | M268 (aserción sin comprobar `ok` → `panic`) roja | unit in-process; la carrera NO se reproduce (§13.4) |
| C11 el rechazo fija la guarda | `TestAdopt_aRefusedOrAbortedAdoptionLeavesTheGuardConsistent/refused…` | M255 roja | store in-process |
| C14 libro v16 sin una tabla guardada → `ledger_unreadable` | `TestGuard_theHookRequiresEveryGuardedTable` | M273 (sin exigir las tablas guardadas) roja; M263-bis (cero objetos → `ledger_unreadable`) roja tras pasar de «tablas» a «objetos» | in-process, segunda conexión real, reconexión del pool |
| C15 `receipts` exigida también con la fila propia | `TestGuard_theHookRequiresTheReceiptsTableWithTheOwnersRow` | M274 (sin exigir `receipts` con fila) roja | in-process, segunda conexión real, reconexión del pool |
| C16 el veredicto ilegible es pegajoso en la conexión | `TestGuard_anUnreadableVerdictIsStickyForTheConnection` (guarda en ilegible con la fila intacta: la primera línea dice `ok` y el acto rehúsa por nombre; una conexión nueva lo levanta) | M275 (`beginWrite` sin la comprobación pegajosa y el `UPDATE` sin `WHERE`) roja | in-process, reconexión del pool |
| C17 un libro vaciado no es fresco: el hook lo abre ilegible y ningún abridor le siembra esquema | `TestOpen_aWipedLedgerIsNotFresh` | M276 (`requireFreshOrStore` admite todo) roja; M277 (el hook no mira las páginas libres) roja | in-process, segunda conexión real, conexión cruda del driver |
| C08 (ampliada por la pasada interna) genérico, callback y parámetro de salida | `TestOpeners_everyExportedOpenerTakesAnIdentity` | M278 (`Box[*Store]`), M279 (`func(*Store)`), M280 (`**Store`) rojas | paseo AST |
| C13 identidad malformada rehusada ANTES de tocar el fichero | `TestOpeners_refuseAMalformedIdentityByName` | M269 VERDE (defecto del molde: `setIdentity` rehusaba después de abrir; declarado en `mutations.txt`), molde afilado con ruta inexistente y oráculo «el fichero no existe después», M269-bis roja | in-process |
| C13 el lector de solo lectura nombra y no escribe | `TestReadOnly_aForeignLedgerIsReadAndNeverWritten` | M270 (`query_only = 0`) roja | in-process |
| C13 un libro ajeno de un esquema más nuevo no se migra ni rebaja | `TestOpen_aForeignLedgerFromANewerSchemaIsNotMigrated` | M271 (`requireCurrentSchema` sin comparar) roja | in-process |

**Lo que el tren C vio que el plan de §13.3 no decía:** que la mutación de
`checkIdentity` (M269) no enrojecía porque `setIdentity` rehúsa la misma
identidad después de abrir — el molde no distinguía quién rehusaba ni si el
fichero se había tocado; afilado y capturado (M269-bis).

### 13.9 · Recuento del tren C (regla de §9, por ejecución de un recuento con esa regla; el guion vive en el cuaderno del ejecutor, no en el árbol, y la pasada interna lo repitió con el suyo)

Re-derivado por `scripts/mutation_tally.py` (tren D; la regla literal está en
su cabecera y su test en el gate): desde M255, 30 cabeceras, 0 no aplicadas,
30 ejecutadas, 0 que no compilan, 2 verdes declaradas (M256, M269: ambas
repetidas en rojo), **28 rojas**. Tren B entero desde M224: 68 cabeceras, 1 no
aplicada (M235), 67 ejecutadas, 1 que no compilaba (M250), 8 verdes o
bloqueadas y repetidas, **58 rojas**. Día completo desde M57: 246 cabeceras,
3 no aplicadas (M80, M126, M235), 1 declarada sin rojo alcanzable (M79), 242
ejecutadas, 9 cuya captura muestra `[build failed]` (M90, M93, M95, M98,
M103, M105, M121, M154, M250: las repetidas en rojo cuentan aparte), 11
verdes o bloqueadas y repetidas, **222 rojas**, 0 sin marcador. La cabecera
`M80` estaba duplicada (la primera no se aplicó): la segunda se renombró
`M80-bis` al re-derivar. Las cifras anteriores de este párrafo (7 y 224)
salían de una convención no escrita; la regla escrita da 9 y 222.

**Cobertura de `internal/action/sqlite`**, medida sola con `go test -race
-coverprofile` sobre el paquete entero tras las curas: **85,2 %** tras el
verde, **85,1 %** tras las curas de la pasada interna (la corrida del gate
q19, antes del tren, marcó 84,9 %). La cifra no es determinista entre
corridas (una décima arriba o abajo sobre el mismo árbol, visto en este
mismo tren); el umbral del gate mide el total del módulo, no el paquete. El
helper `exec` del pool, sin llamador tras llevar la poda a la transacción, se
retiró en el mismo tren.

### 13.10 · Delta tras la pasada interna sobre el delta (30 min, VETO MANTENIDO → curado)

| Hallazgo | Qué era | Cura |
|---|---|---|
| P2 [PRODUCTO+DOC] | G1-ter nombraba «tabla de recibos» entre las que faltan y el hook solo la exigía sin fila: con la fila propia y `receipts` borrada, guarda `ok`, puerta sin juicio entra, `RecordAttempt` entra (`actions rows = 3`, capturado por el auditor) | el hook exige `receipts` en toda forma v16 (§13.2 paso 5); y el veredicto ilegible es PEGAJOSO en la conexión (§13.2 paso 7), porque la primera línea, al hallar la fila `ok`, levantaba la guarda del hook: `TestGuard_theHookRequiresTheReceiptsTableWithTheOwnersRow` (C15), `TestGuard_anUnreadableVerdictIsStickyForTheConnection` (C16); M274, M275 rojas |
| P3 [INSTRUMENT] | el paseo dejaba pasar `Box[*Store]` (instanciación genérica), `func(*Store)` por callback y `**Store` por parámetro de salida | `handsOutStore`: resultados de cualquier tipo, genéricos incluidos; parámetros callback cuyos parámetros lleven `Store`; parámetros puntero a algo que lo lleve (`*Store` a solas es entrada). Límites declarados: interfaz; entrega en slice o map que dimensiona el llamador. M278, M279, M280 rojas |
| P3 [PRODUCTO] | un libro VACIADO (todos los objetos borrados, 63 páginas libres) era «fresco» por la letra de G1-ter (1) y el arranque lo refundaba como legacy | fresco exige `freelist_count` cero en el hook, y el abridor (`requireFreshOrStore`, también en la sonda del operador) rehúsa por nombre `ErrFileNotFresh` sin sembrar esquema: `TestOpen_aWipedLedgerIsNotFresh` (C17); M276, M277 rojas. **Superada por el tren D:** el gate completo mostró que el libro comparte el fichero con el almacén de conversaciones, así que ni las páginas libres ni «cero objetos» sirven; C17 se retiró y «fresco» es relativo al almacén de actos (§14.1). Un esquema de actos vaciado por completo en un fichero compartido es un almacén fresco por el fichero solo: límite declarado (§14.4) |
| P3 [DOC] | la nota pública «una tabla que falta … nace bloqueada» tenía dos contraejemplos (v16 con `receipts` borrada → `ok`; v15 sin `intents` → `installGuard` muere por texto de driver) | el primero es la cura P2; la frase se acota a «un libro de esta versión al que le falta una tabla … nace bloqueado; un libro anterior que tampoco tiene su forma no abre» |
| P3 [DOC] | «doce tablas guardadas» son diez tablas y doce eventos | HANDOFF, §13.1, §13.2 corregidos |
| P3 [DOC] | godoc de `refuseMaintenance` («recovery and prune»); cabecera de `ledger_hook_test.go` sin C14 | corregidos |
| P3 [INSTRUMENT] | el oráculo de C09 era un plazo de 300 ms (verde falso posible con la goroutine de B planificada tarde) | oráculo por imposibilidad: una TERCERA conexión real con `busy_timeout(0)` intenta `BEGIN IMMEDIATE` dentro del seam y DEBE recibir `SQLITE_BUSY` (5); M267-bis roja |
| Nota | «por ejecución del script»: el guion no está en `scripts/` | acotado en el título de §13.9 |

**Lo que la pasada vio y §13.3 no:** que §13.1 y §13.2 se contradecían en
`receipts`; que la primera línea levantaba el veredicto del hook; que
«vacío» no es «fresco»; tres formas más del paseo; el oráculo por plazo.

## 14 · Tren D (2026-09-25): el gate primero, UNA función de forma, y los diez hallazgos de la oficial — plan aprobado por el director

**Autorización.** El director, 2026-09-25, tras el paquete de parada del tren C
(§13 del informe): el §13.4 entero más una norma. (1) PRIMERO el gate: «fresco»
= sin `action_schema` y sin ninguna tabla del almacén de actos (el fichero
puede llevar conversaciones); con `action_schema` → existente, NUNCA se
resiembra (F-C1); tablas de actos sin `action_schema` → forma mala; páginas
libres retiradas como señal; molde del fichero compartido en el paquete
`sqlite`; C17 reescrito; gate completo VERDE antes de nada más. (2) UNA función
de forma, `judgeShape`, con la lista constante de TODAS las tablas del esquema
v16, consultada por el hook, por los abridores (rehusar o abrir ilegible, nunca
reparar) y por los lectores. (3) F-C3, F-C4, F-C5, F-C7, F-C8, F-C9, F-C10.
(4) Textos de §13.5 y HANDOFF a la verdad. (5) NORMA: gate completo verde ANTES
de abrir cualquier ronda oficial. Pasada interna; barrido; UNA ronda oficial
(30 min); si mantiene producto, parada y paquete; si levanta, entrega.

### 14.1 · Garantías, literales

- **G-D1 · la forma.** `judgeShape(conn)` clasifica una conexión con la lista
  constante `schemaTablesV16` (las 34 tablas que un `OpenFor` fresco crea,
  capturadas por ejecución; un molde ata la constante a esa captura). FRESCO:
  ninguna tabla de esa lista existe (otras tablas, las de conversaciones del
  fichero compartido, no cuentan). ANTERIOR: `action_schema` con exactamente
  una fila, numérica, de 1 a 15; qué tablas tiene esa versión es asunto de la
  migración, no de la forma (la comprobación de la tabla de identidad se
  retiró en el verde, §14.4). ACTUAL:
  `action_schema` con exactamente una fila numérica igual a la versión del
  binario y las 34 tablas presentes. MÁS NUEVO: una fila numérica mayor que la
  versión del binario. MALA: todo lo demás — tablas de actos sin
  `action_schema`, 0 o 2 filas, versión no numérica o menor que 1, cualquier
  tabla de la lista ausente en la versión actual. Un FALLO DE CONSULTA es un
  error, nunca una forma. Los
  índices no forman parte de la forma (§14.4).
- **G-D2 · el hook.** Fresco y anterior → `ok`; actual → juicio de la fila
  (canónica propia `ok`, ajena `ledger_foreign_profile`, no canónica
  `ledger_unreadable`; tabla vacía sin marca `ok`, con marca
  `ledger_unreadable`; dos filas o fila con otro `id` → `ledger_unreadable`);
  más nuevo y mala → `ledger_unreadable`, pegajoso. Un error de consulta en el
  hook REHÚSA la conexión (el hook devuelve el error; el pool abre otra en la
  llamada siguiente): un error transitorio nunca se convierte en un veredicto.
- **G-D3 · los abridores.** Fresco → se siembra `createStmt`, se sube a v1 y se
  migra. Anterior → se migra (sin `createStmt`). Actual y más nuevo → propio o
  legacy migra (más nuevo: `ErrSchemaFromTheFuture`); ajeno exige la versión
  actual y nombra `ErrSchemaFromTheFuture` cuando es más nueva. MALA →
  `OpenFor` y `OpenOperatorFor` ABREN con la guarda ilegible: sin migrar, sin
  sembrar, sin podar, sin recuperar; NUNCA se recrea una tabla (`sqlite_master`
  idéntico antes y después, por nombre y por `sql`); los abridores internos sin
  identidad rehúsan por nombre. `ownsOrLegacy` desaparece: `readOwner` lee
  la fila y un fallo de lectura es un error del abridor, no texto interpretado.
- **G-D4 · los lectores.** `judgeIn` (y con él `Standing` y `beginWrite`) juzga
  la forma antes de la fila: mala → `ledger_unreadable` con la causa (la tabla
  que falta, la versión que no convierte). `OpenReadOnlyFor` abre CUALQUIER
  forma mala (con o sin `action_schema`) y la nombra por `Standing`;
  `korvun ledger check` la imprime.
- **G-D5 · la poda del arranque.** `OpenFor` deja que `Prune` decida: un
  rechazo por nombre de `Prune` (ajeno o ilegible, una adopción confirmada entre
  el juicio y la poda) es «sin poda», no un arranque fallido.
- **G-D6 · pegajoso, una sola rama.** El `WHERE standing <> 'ledger_unreadable'`
  del `UPDATE` de la guarda es la ÚNICA rama; las comprobaciones de
  `beginWrite` y `beginAdoption` se retiran (solo nombraban; F-C3). Un error
  de consulta en `judgeIn` no fija la guarda (solo los veredictos lo hacen).
- **G-D7 · el paseo.** Un callback por TIPO CON NOMBRE y un canal parámetro
  (`chan`, `chan<-`) que lleven `Store` son entregas.
- **G-D8 · centinelas.** `ErrGuardHandleUnknown` (el hook sobre un handle
  olvidado); `ErrSchemaFromTheFuture` en el rechazo del abridor sobre un libro
  ajeno más nuevo.
- **G-D9 · el recuento.** `scripts/mutation_tally.py` deriva el recuento de
  `mutations.txt` con la regla literal, tiene su test y corre en el gate; §13.9
  y §10 se reescriben con su salida o se retiran.
- **G-D10 · la norma.** Ninguna ronda (interna u oficial) se abre sin el gate
  completo verde sobre el árbol que audita.

### 14.2 · Diseño

`judgeShape` toma un `shapeQuerier` (una consulta, todas las filas de la
primera columna): un adaptador para la conexión del driver (el hook) y otro
para `*sql.DB`/`*sql.Tx` (abridores, lectores, `judgeIn`). Tres consultas:
`SELECT name FROM sqlite_master WHERE type = 'table'`, `SELECT version FROM
action_schema` (todas las filas: la cuenta se hace en Go) y nada más; la
decisión es la de G-D1 y devuelve `(forma, causa, error)`. `judgeOnConn` =
forma + fila, y devuelve `(estado, error)`: el hook rehúsa la conexión con
error. `openWithIdentity`: conecta, `judgeShape(db)`; fresco → `createStmt`,
semilla v1, `migrate`; anterior → `migrate`; actual/más nuevo → lee la fila
(`readOwner`: sin fila legacy, propia, ajena; error → error) y migra o exige la
versión; mala → guarda pegajosa (el hook ya la puso), ningún paso más;
`installGuard` como hoy. `openOperatorWithIdentity` juzga la forma en su sonda
(`storedSchemaVersion` desaparece): fresco → el error de hoy («not a korvun
store?», no se renombra), anterior → el error de hoy («never migrates»), más
nuevo → `ErrSchemaFromTheFuture`, mala/actual → `openWithIdentity`.
`openReadOnly`: forma fresco/anterior/más nuevo → los errores de hoy, con
centinela (`ErrNoActionStore`, `ErrSchemaBehind`, `ErrSchemaFromTheFuture`);
mala → abre; actual → abre. `judgeIn`: forma primero, con un `ledgerQuerier`
(fila y filas). `installGuard` crea los triggers solo sobre las tablas
guardadas presentes en el catálogo. `Prune` y `OpenFor` como G-D5 (seam de
paquete `openPruneSeam`; `openStandingSeam` y `poolLifetimeForTest` para
D20). `guardHook`: un seam de paquete `hookShapeFault` (solo tests) inyecta
un fallo de consulta; el driver CIERRA la conexión cuando el hook devuelve
error (`driver.go`: `c.Close()` antes de envolver el error, leído en
`modernc.org/sqlite v1.59.0`). Retirados: `requireFreshOrStore`,
`ErrFileNotFresh`, la regla de páginas libres, las dos comprobaciones
pegajosas, `ownsOrLegacy`; `refuseMaintenance` sigue (recuperación).
*Superado (tren E): el recuento de «tres consultas» ya no vale. `judgeShape`
lee hoy también los índices UNIQUE y, si lo hay, el residuo de la siembra.*

### 14.3 · Plan de fallos del tren D

| Fila | Garantía | Escenario | Resultado con nombre | Efectos prohibidos | Molde y nivel | Mutación |
|---|---|---|---|---|---|---|
| D01 | G-D1 fresco relativo | el almacén de CONVERSACIONES (`internal/conversation/sqlite.Open`, el real) abre el fichero primero; después `OpenFor` | juicio crudo `ok`; `OpenFor` funda; `Standing` legacy; un acto entra | `file_not_fresh`, `ledger_unreadable` | `TestShape_theSharedFileIsFreshForTheActionStore`; conexión cruda + store in-process, dos almacenes reales | fresco = cero objetos → roja |
| D02 | G-D1 la lista | `schemaTablesV16` contra `sqlite_master` de un `OpenFor` fresco | iguales | — | `TestShape_theTableListIsTheFreshStores`; in-process | quitar un nombre → roja |
| D03 | G-D3/G-D4 forma mala, handle NUEVO | libro fundado por A; segunda conexión borra UNA tabla (tabla por fila: `actions`, `action_decisions`, `receipts`, `approval_tombstones`, `authorization_snapshots`, `intents`); `OpenOperatorFor` y `OpenFor` nuevos | abren (`err == nil`) con guarda `ledger_unreadable`; `Standing` → `ledger_unreadable` con el nombre de la tabla; `RecordAttempt` → `ErrLedgerUnreadable`; lector `OpenReadOnlyFor` → `Standing` ilegible | `sqlite_master` (nombre y `sql`) idéntico antes y después; `actions rows` sin cambio | `TestShape_aBadShapeOpensUnreadableAndIsNeverRepaired`; in-process, segunda conexión real | `createStmt` incondicional → roja (la tabla vuelve); `judgeIn` sin forma → roja (`Standing` dice `ok`) |
| D04 | G-D1 tablas de actos sin `action_schema` | `DROP TABLE action_schema` | igual que D03 | ídem | fila de D03 | ídem |
| D05 | G-D3/G-D4 PROCESO NUEVO | binario `korvun` compilado; perfil con libro fundado; `DROP TABLE actions`; `korvun ledger check --config`; `korvun receipt rotate-key --config` | `check` imprime `ledger standing: ledger_unreadable (… actions …)`; `rotate-key` sale ≠ 0 nombrando `ledger_unreadable` | `sqlite_master` idéntico tras ambos | `TestLedgerBinary_aBadShapeIsNamedAndNeverRepaired` (`internal/cli`); binario en proceso OS aparte | `createStmt` incondicional → roja |
| D06 | G-D2 error ≠ forma (conexión) | conexión cruda con `locking_mode = EXCLUSIVE` en WAL sostiene un escritor; el store abre conexión nueva | la llamada falla con la clase BUSY (`ErrLedgerBusy` si llega mapeada), NUNCA `ErrLedgerUnreadable`; al soltar, la conexión siguiente juzga `ok` y un acto entra | guarda ilegible pegajosa | `TestShape_aTransientErrorAtBirthIsNotAVerdict`; conexiones reales; se confirma en el rojo si el lock alcanza al hook (§14.4) | el hook mapea error → `ledger_unreadable` → roja |
| D07 | G-D2 error ≠ forma (hook) | seam `hookShapeFault` devuelve un error en la conexión nueva | la llamada del store falla con ESE error; la siguiente (seam limpio) abre `ok` y escribe | ídem | `TestShape_theHookRefusesTheConnectionOnAQueryError` *(Superado (tren E, tanda 1): hoy `TestShape_theHookRefusesANonVerdictFailure`)*; seam, in-process | ídem |
| D08 | G-D5 | seam entre `Standing` y `Prune` en `OpenFor`; B adopta dentro con su handle | `OpenFor` devuelve handle, `Standing` ajeno, `Prune` no borró | arranque fallido | `TestOpen_anAdoptionBetweenTheJudgementAndThePruneIsNotBootFatal`; dos handles reales | `OpenFor` falla ante el rechazo → roja |
| D09 | G-D6 | C16 (`TestGuard_anUnreadableVerdictIsStickyForTheConnection`) | igual | — | existente | SOLO el `WHERE` retirado → roja (M275 se declara superada) |
| D10 | G-D7 | paseo | dos ofensores más | — | `TestOpeners_everyExportedOpenerTakesAnIdentity` | `type StoreFn func(*Store)` por parámetro; `chan<- *Store` → rojas |
| D11 | G-D8 | handle olvidado; libro ajeno v17 | `ErrGuardHandleUnknown`; `ErrSchemaFromTheFuture` | texto sin centinela | moldes existentes afilados | centinela sustituido por texto → roja |
| D12 | G-D3 `ownsOrLegacy` sin texto | libro propio actual, legacy y ajeno | migra, migra, exige versión | — | moldes existentes (`TestOpen_aForeignLedgerGetsNoMaintenance`, migración) | — (la rama de texto desaparece) |
| D13 | G-D9 | `scripts/mutation_tally.py` sobre `mutations.txt` y sobre un fichero sintético | los recuentos de §13.9 se re-derivan | — | `scripts/mutation_tally_test.py`; gate | — |
| D14 | G-D10 | gate completo | verde | — | `/tmp/q21.txt` y siguientes | — |
| D15 (añadida en el verde: el gate marcó `sqlite` en 84,8 %) | G-D3/G-D4 cada puerta nombra la forma que rehúsa | fichero existente con solo conversaciones; esquema anterior (v15); esquema más nuevo (v17); forma pedida por un pool cerrado | «not a korvun store» (operador y lector); «never migrates» (operador y lector); `ErrSchemaFromTheFuture` (operador, lector y el arranque del dueño); un error que no es veredicto | un handle; `ledger_unreadable` sobre un fallo de consulta | `TestShape_theDoorsNameEveryShapeTheyRefuse`; in-process, almacén de conversaciones real | el lector abre un esquema anterior → roja; el operador pierde el centinela → roja |

### 14.4 · Límites declarados

- Sobre un fichero compartido, un esquema de actos vaciado por completo es un
  almacén fresco por el fichero solo: el P3 «libro vaciado» de la pasada
  interna del tren C se re-adjudica como límite (una señal fuera del libro es
  un diseño propio). La regla de páginas libres y `ErrFileNotFresh` se retiran.
- Los índices no forman parte de la forma: un índice ausente es rendimiento,
  no evidencia. *Superado (tren E, tanda 2): los tres índices UNIQUE forman
  parte de la forma; solo un índice de rendimiento que falta queda sin juzgar.*
- La forma «anterior» se ejercita sobre un v16 rebajado; un v15 real de la
  v0.16.1 no se ejercita. La comprobación «tabla de identidad presente
  exactamente cuando su versión la tiene» se retiró en el verde: el segundo
  gate del tren la vio rehusar veinte fixtures v11 de `internal/cli` (v16
  rebajados que conservan `ledger_identity`), y no vigila ninguna forma que un
  proceso real produzca.
- D06: si el lock exclusivo hace fallar la conexión ANTES del hook (los
  pragmas del DSN), el molde prueba la garantía a nivel de conexión y la rama
  del hook la prueba D07 con seam; se declara en el canto lo que ocurrió.
- El paseo sigue sin ver una interfaz ni una entrega en slice o map que
  dimensiona el llamador.
- (Tras la pasada interna.) El campo `rowEra` se retiró, y con él la
  constante `identityRowSchemaVersion`: siendo igual a `schemaVersionCurrent`
  nunca era cierto para
  «anterior»; un libro AJENO rebajado a un esquema anterior lo migra el
  primer perfil que lo abra (como el HANDOFF ya declaraba para v15→v16) y la
  semilla de la fila no pisa una fila existente (D18). La forma y el
  cinturón resuelven los nombres como SQLite, sin distinguir mayúsculas
  (D17, D19). El modo estricto (`PrepareStrictAuthority`) sobre una forma
  mala no se ejercita: la app de los moldes arranca en modo no estricto.
  Los límites de la regla del recuento están declarados en la cabecera de
  `scripts/mutation_tally.py`.

### 14.5 · Orden de trabajo

Rojo D01–D08, D10, D11 → verde (forma, hook, abridores, lectores, poda,
pegajoso, paseo, centinelas) → gate completo → mutaciones → `mutation_tally`
con su test y su sitio en el gate → §13.9/§10 re-derivados → textos (HANDOFF,
notas, `store.go`, §13.5, §13.10) → gate completo → pasada interna → curas →
barrido → gate completo → oficial (una) → entrega si levanta.

### 14.6 · Las cinco preguntas, antes del rojo

1. ¿Frase más ancha que su cable? §14.4 acota; D06 se declara condicional. No.
2. ¿Cada nombre existe? Los moldes nacen con estos nombres; los existentes, por `grep`.
3. ¿Nivel de evidencia? D05 es binario en proceso aparte y se etiqueta así; D07 es seam; el resto in-process con conexiones reales.
4. ¿Cifras de memoria? Las 34 tablas salen de una captura ejecutada hoy.
5. ¿A medias? El recuento del día (§10, §13.9) se reescribe con el guion; dicho.

### 14.7 · Canto del tren D: garantía → molde que la rompe → mutación ejecutada → nivel de evidencia

Rojo capturado antes de la cura en `evidence/v0.16.2/red.txt`, sección «Tren
D»: D01 (el hook leía el fichero compartido como ilegible), D03/D04 (guarda
`ok` con tablas ausentes; `intents` ausente mataba `installGuard` por texto de
driver; `action_schema` ausente daba `file_not_fresh`), D05 (`ledger check`
decía `legacy_unfounded` y moría por texto de driver), D07 y D08 (los seams no
existían), D11 (los dos centinelas no existían); D02 pin. D06 retirada
(captura en la misma sección; `mutations.txt`, salvedades del tren D).

| Garantía | Molde | Mutación ejecutada | Nivel |
|---|---|---|---|
| G-D1 fresco relativo al almacén de actos | `TestShape_theSharedFileIsFreshForTheActionStore` (el almacén de conversaciones REAL abre el fichero primero) | M281 (fresco = cero tablas en el fichero) roja | conexión cruda del driver + dos almacenes reales in-process |
| G-D1 la lista es la del almacén fresco | `TestShape_theTableListIsTheFreshStores` | M282 (sin `receipts`) roja | in-process |
| G-D3/G-D4 forma mala: abrir ilegible, nunca reparar, nombrar | `TestShape_aBadShapeOpensUnreadableAndIsNeverRepaired` (siete tablas, dos abridores, el lector) | M283 (`createStmt` sobre forma mala) roja; M284 (`judgeIn` sin forma) roja | in-process, segunda conexión real; oráculo: catálogo idéntico por nombre y `sql` |
| G-D3/G-D4 desde un PROCESO NUEVO | `TestLedgerBinary_aBadShapeIsNamedAndNeverRepaired` (`internal/cli`) | M285 (`createStmt` sobre forma mala) roja | **binario compilado en proceso OS aparte**, fichero real |
| G-D2 error de consulta ≠ veredicto | `TestShape_theHookRefusesTheConnectionOnAQueryError` *(Superado (tren E, tanda 1): hoy `TestShape_theHookRefusesANonVerdictFailure`)* | M286 (el fallo instala `ledger_unreadable`) roja | seam en el hook, in-process (declarado: D06 con lock real no alcanzó al hook) |
| G-D5 la poda decide | `TestOpen_anAdoptionBetweenTheJudgementAndThePruneIsNotBootFatal` | M287 (`OpenFor` falla ante el rechazo) roja | dos handles reales, seam |
| G-D6 pegajoso por una sola rama | `TestGuard_anUnreadableVerdictIsStickyForTheConnection` | M288 (`WHERE` retirado, A SOLAS) roja | in-process |
| G-D7 el paseo ve callback con nombre y canal | `TestOpeners_everyExportedOpenerTakesAnIdentity` | M289 (`type StoreFn func(*Store)`), M290 (`chan<- *Store`) rojas | paseo AST |
| G-D8 centinelas | `TestGuard_theHookNamesAForgottenHandle`, `TestOpen_aForeignLedgerFromANewerSchemaIsNotMigrated` | M291 (texto sin centinela), M292 (sin `%w`) rojas | in-process |
| G-D3/G-D4 cada puerta nombra la forma que rehúsa (D15) | `TestShape_theDoorsNameEveryShapeTheyRefuse` (fresco existente, anterior, más nuevo, pool cerrado) | M293 (el lector abre un esquema anterior), M294 (el operador sin centinela) rojas | in-process, almacén de conversaciones real |
| G-D2 la tercera puerta del veredicto (D16) | `TestGuard_refreshGuardReturnsATransientFailureInsteadOfAVerdict` | M295 (`refreshGuard` fija ilegible ante cualquier error) roja | seam en el hook, pool real |
| G-D1/G-D2 nombres como SQLite (D17) | `TestGuard_theBeltResolvesTableNamesLikeSQLite` | M296 (cinturón byte a byte) roja | in-process, segunda conexión real |
| G-D3 la semilla no pisa una fila (D18) | `TestMigrate_theSeedKeepsARowAlreadyThere` | M297 (`INSERT … VALUES`) roja | in-process |
| G-D1 un objeto homónimo no es fresco (D19) | `TestShape_aHomonymousObjectIsNotAFreshFile` | M298 (solo tablas, byte a byte) roja | in-process |
| G-D3 un fallo de juicio es el error del abridor (D20) | `TestOpen_aFailureToJudgeTheStandingIsTheOpensError` | M299 (`OpenFor` traga el fallo) roja | seams, pool real |
| G-D4 el arranque real sobre las 34 formas (D21) | `TestBuild_bootsOverABadShapeAndNamesIt` (`internal/app`) | M300 (la tinta se registra siempre), M301 (la activación se lee siempre), M302 (el barrido corre siempre) rojas | **app real en loopback**, fichero real |
| G-D9 el recuento se deriva | `scripts/mutation_tally_test.py` (seis ataques: cada clase forzada, la prosa no decide, el rango por número) | son sus propios tests | gate (`make mutation-tally-probe`, paso de `quality.yml`) |

**Recuento del tren D** (`scripts/mutation_tally.py --from M281`, tras la
pasada interna): 22 cabeceras, 22 ejecutadas, 0 verdes, **22 rojas**. Trenes
C+D desde M255: 52 cabeceras, 52 ejecutadas, 2 verdes declaradas y repetidas,
**50 rojas**. Tren B entero desde M224: 90 / 1 no aplicada / 89 / 1 que no
compilaba / 8 verdes / **80 rojas**. Día desde M57: 268 / 3 / 1 / 264 / 7 /
11 / **246 rojas**, 0 sin marcador, 0 cabeceras duplicadas. (Tras la pasada
interna la regla cierra un bloque en el siguiente encabezado `## `: dos
bloques, M90 y M95, dejaron de contar como «no compilaban» una captura
`[build failed]` que vivía en la sección de salvedades posterior; el guion
los da hoy por rojos, que es lo que su propia captura muestra.)

**Cobertura de `internal/action/sqlite`.** El tercer gate del tren (verde,
total del módulo 88,3 %) marcó el paquete en 84,8 %: las ramas de forma de
los abridores y del lector no tenían ataque; D15 las fuerza. Medido a solas
después (`go test -race -coverprofile` del paquete entero): **85,1 %**. La
cifra sigue sin ser determinista entre corridas y el gate mide el total del
módulo, no el paquete.

### 14.8 · Delta tras la pasada interna sobre el delta (30 min, VETO MANTENIDO → curado)

| Hallazgo | Qué era | Cura |
|---|---|---|
| P2-1 [PRODUCTO] | `refreshGuard` fijaba `ledger_unreadable` (pegajoso) ante CUALQUIER error de `judgeIn`, veredicto o fallo de consulta: un fallo transitorio en `installGuard` dejaba un libro sano ilegible hasta el reinicio (capturado por el auditor con el seam fallando una vez) — la tercera puerta del veredicto, fuera de `isVerdict` | solo un veredicto fija la guarda; un fallo se devuelve como error del abridor: `TestGuard_refreshGuardReturnsATransientFailureInsteadOfAVerdict` (D16); M295 roja |
| P2-2 [PRODUCTO+DOC] | el arranque REAL de la app moría por texto de driver con `approval_birth_heads`, `approvals` o `signing_keys` ausentes (la activación, el barrido y el registro de la tinta leían el libro sin pasar por el estado); la nota decía que la pantalla nombra la tabla | el arranque juzga el estado UNA vez: ilegible → la tinta se carga del fichero sin registrarla en el libro (`ensureSigningKeyIn(…, false)`), el check de activación y el barrido se saltan con aviso, la recuperación ya se saltaba; un fallo de juicio que no es veredicto es arranque fallido por nombre: `TestBuild_bootsOverABadShapeAndNamesIt` (las 34 tablas, app real en loopback: arranca, la pantalla dice `unreadable` con la tabla en la causa, una puerta rehúsa por nombre, el catálogo del almacén de actos no cambia); M300, M301, M302 rojas |
| P3 [PRODUCTO] | el cinturón casaba nombres byte a byte y SQLite resuelve `Actions` como `actions`: sin trigger sobre la tabla renombrada, la puerta sin juicio entraba | forma y cinturón en minúsculas: `TestGuard_theBeltResolvesTableNamesLikeSQLite` (D17); M296 roja |
| P3 [PRODUCTO] | `rowEra` nunca era cierto para «anterior» (código muerto) y un libro ajeno rebajado a v15 moría en la semilla por `UNIQUE` | `rowEra` retirado; la semilla no pisa una fila existente: `TestMigrate_theSeedKeepsARowAlreadyThere` (D18); M297 roja |
| P3 [PRODUCTO] | un fichero con un objeto homónimo (`Actions`, una vista `actions`) pasaba por fresco y el bootstrap moría por texto | la forma cuenta objetos de cualquier tipo con nombre del esquema, en minúsculas: mala, abierta ilegible, nada se repara: `TestShape_aHomonymousObjectIsNotAFreshFile` (D19); M298 roja |
| P3 [PRODUCTO] | `OpenFor` tragaba un fallo de `Standing` como «sin poda» | un fallo que no es veredicto es el error del abridor: `TestOpen_aFailureToJudgeTheStandingIsTheOpensError` (D20, seam `openStandingSeam` + `poolLifetimeForTest` + `hookShapeFault`); M299 roja |
| P3 [INSTRUMENT] | `judgeIn` saltaba la forma si el querier no era `rowsQuerier` | el parámetro es `ledgerQuerier` (fila y filas): la aserción desaparece, en compilación |
| P3 [INSTRUMENT] | D15 aseveraba por texto | centinelas `ErrNoActionStore` y `ErrSchemaBehind`; D15 los exige |
| P3 [INSTRUMENT+DOC] | límites de la regla del recuento sin declarar; un encabezado `## Salvedades` dentro del bloque M290 | un encabezado de sección cierra el bloque; los límites, declarados en la cabecera y probados (`test_aSectionHeadingEndsTheBlockBeforeIt`, `test_theDeclaredLimitsHoldAsWritten`) |
| P3 [DOC] | `beforeOpenPrune`, «`ownsOrLegacy` deja de leer texto», «`installGuard` como hoy», «cuya `action_schema` se puede leer», D06 en la cabecera del fichero de moldes | corregidos en §14.2, G-D3, G-D4 y la cabecera |

**Lo que la pasada vio y §14 no:** la tercera entrada del veredicto; el
arranque real sobre las 34 formas; la resolución de nombres de SQLite; el
código muerto de `rowEra`; el encabezado de sección dentro de un bloque.

