VETO LEVANTADO

## Objeto y custodia

- **Árbol.** `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `v0162-mockups`, HEAD `518ba15683595a0a9a9338f756f65d96a44c2a8a`: `d20dea6` más el fast-forward de `website/package-lock.json`, que es ajeno al tren. El tren E y la cura de B están sin commit.
- **Base del diff real.** `d20dea66…` más `t1/diff-start.patch`, que se aplica limpio en `adv-e-final/base` (salida: `APPLIED`). Los ficheros que estaban sin seguimiento al empezar se juzgan enteros. `ledger_class.go`, `ledger_judge_seams.go`, `ledger_residue.go`, `docs/operations/ledger-restore.md` y los moldes E no aparecen en `untracked-start.txt`, así que son del tren.
- **Plan.** 3270 líneas, sha256 `1f7f8b296e84fbea6a8d3f8433b25ece64882bcce8cd3728b9572c54e3bfc916`, medido por mí.
- **Custodia.** Entre 10:39:20 y 11:14:27 el árbol no cambió:
  - `git status --porcelain | shasum` = `209fa7d6…` y `git diff | shasum` = `5edec8bb…`, iguales al empezar y al acabar;
  - los 110 ficheros sin seguimiento conservan su sha256 uno por uno (el `diff` de las dos listas sale vacío).
- **Dónde trabajé.** Mutaciones y sondas, solo en dos copias: `…/scratchpad/adv-e-final/tree` y `…/adv-e-final/tree2`. Cada mutación se restauró y se comprobó por sha256 contra el árbol.

## Hallazgos

No hay P1 ni P2. Van de más a menos grave.

H7, H8, H9 y H10 son frases que se saben falsas. Por la ley del tono y por la regla del gate de Codex («may NOT leave a known false public claim alive»), no pueden ir a la lista de cierre: se curan en este tren aunque sean P3. H11 incumple una ley CRITICAL, y la cura cabe en una línea por comentario.

### H1 · P3 · [PRODUCT] — Una fundación que confirma entre las dos lecturas del juicio produce un veredicto falso

Un juicio del libro lee la fila y la marca en dos instantáneas. Si otra conexión confirma una fundación entre las dos, sale `ledger_unreadable` sin que el libro tenga nada. En el hook, además, ese veredicto queda pegado a la conexión.

**Reproducción.** Ejecutada en proceso, con dos pools reales sobre el mismo fichero. El entrelazado lo fuerza una pausa que inserté en el código de producción de la copia `tree2`. Es instrumentación, no un tiempo natural.

1. Libro legado: `identityStoreFixture` más `mustRecord(act_1, AUTHORIZED)`. No hay fila de identidad ni marca.
2. `reader := OpenReadOnlyFor(path, profileA)`; `reader.Standing` devuelve `legacy_unfounded`, nil.
3. Una segunda llamada a `reader.Standing` queda retenida después de que `readIdentityRows` lee cero filas (`profile_standing.go:200-207`) y antes de leer la marca (`:290`).
4. Desde el otro pool, `fixture.FinishFounding(ctx, "act_1", profileA)` confirma el recibo con la marca y la fila en una sola transacción.
5. Se suelta la pausa.
6. Variante del hook: `OpenOperatorFor(path, profileA)` queda retenido en `judgeOnConn` después de `COUNT(*) = 0` (`ledger_identity.go:592`) y antes de la marca (`:619`). Se repiten los pasos 4 y 5, y después se miran `Standing`, la fila de guarda y un `RecordAttempt` por ese handle.

Comando: `go test -count=1 -run 'TestADV_P4' -v ./internal/action/sqlite/` en `tree2`. Salida:
```
ADV OpenReadOnlyFor before: Standing = "legacy_unfounded" <nil>
ADV OpenReadOnlyFor torn: Standing = "ledger_unreadable" action/sqlite: ledger_unreadable: the ledger's identity row cannot be read: the identity row is missing while a marked receipt exists
ADV OpenReadOnlyFor next: Standing = "ok" owner="sha256:b14250fa…" <nil>
ADV handle born through the torn hook: Standing = "ok" owner="sha256:b14250fa…" <nil>
ADV its guard row = "ledger_unreadable"
ADV a write through that handle = action/sqlite: insert action "act_adv_1": action/sqlite: ledger_unreadable: the ledger's identity row cannot be read (this connection's guard)
```

**Dónde está.**
- `judgeIn` fija una sola conexión (`profile_standing.go:175-186`), pero no abre transacción de lectura. Cada lectura de `judgeOn` (`:193`, `:200`, y luego `:290` dentro de `judgeWithoutRow`) es una sentencia autocommit con su propia instantánea WAL.
- El hook hace lo mismo (`ledger_identity.go:592`, `:604`, `:619`), y su veredicto es pegajoso por el `WHERE standing <> 'ledger_unreadable'` (`:299`).
- El plan cerró solo la carrera COUNT→owner del juez de la base (§7, C1-13) y dejó el driver «with the same stable-data outcomes». El par fila→marca de `Standing` no está curado ni declarado en ningún texto.

**Cuándo pasa de verdad.** Hace falta que otro pool confirme una fundación o una adopción mientras dura el juicio. En la app, sus escrituras y su GET comparten el único pool y se serializan. Lo alcanzable es la CLI (`ledger check`, `receipt verify` o un acto de operador) en la ventana entre la persistencia del perfil y el cierre del acto fundacional. La CLI imprime entonces `ledger_unreadable`, cuyo remedio documentado es sustituir el fichero (`ledger-restore.md:3-8`, `v0.16.2.md:152-158`). El handle de operador rechaza toda escritura mientras vive.

**Desenlace exigido.** Con una fundación o adopción concurrente, el juicio devuelve el estado de antes (`legacy_unfounded`) o el de después (`ok`), nunca un veredicto armado con dos instantáneas. Ningún molde fuerza hoy una confirmación entre dos lecturas del mismo juicio.

### H2 · P3 · [PRODUCT] — D2 manda borrar lo que el procedimiento manda guardar

D2 en pantalla manda borrar el `-wal` y el `-shm` del libro dañado. El procedimiento del mismo tren manda guardarlos como evidencia.

**Reproducción.** Por lectura del texto; no hay nada que ejecutar.

1. D2 con ruta (`WhatsHappening.tsx:154`): «…si tu copia es un solo fichero, borra también ${path}-wal y ${path}-shm.»
2. `docs/operations/ledger-restore.md:33-35`: «Keep the damaged files. Move the main file, its -wal and its -shm together, unchanged, into a folder of their own. Do not delete them: they are the evidence of what failed.»
3. TE66 fija el texto «borra también» (`WhatsHappening.e4.test.tsx:208`). TE51 borra los sidecars dañados con `e4RemoveSidecars` (`internal/cli/ledger_e4_test.go:653`). El molde del procedimiento sigue a D2, no a su propio documento.

**Consecuencia.** Quien sigue la pantalla destruye lo que el documento llama evidencia. Con ello pierde las transacciones confirmadas que siguen solo en el `-wal`, y ya no se pueden recuperar. D2 es copia aprobada por el director; la contradicción entre las dos instrucciones es de este tren.

**Desenlace exigido.** La pantalla y el documento dan la misma instrucción sobre el `-wal` y el `-shm` dañados; la copia la decide el director. Ningún molde puede fijar la instrucción contraria.

### H3 · P3 · [PRODUCT] — La fila del acto promete un recibo que no llegará

Hermano de B. Cuando el sondeo llega a un estado terminal sin `receipt_id`, la fila del acto sigue diciendo que el recibo «llegará cuando el cambio termine», al lado de «No se aplicó» o de «Hecho».

**Reproducción.** jsdom, ejecutado. La mitad de Go la saco por lectura; no la ejecuté.

1. El POST de `enable-approvals` responde el cuerpo real, `fixtures/whats-happening-applying.json`.
2. `/api/reload/reload-1` responde primero `{state:'pending'}` y después `{state:'rolled-back', action_id:'act_01'}`, sin `receipt_id`. En otra pasada, `succeeded`.

Comando: `npx vitest run --reporter=verbose src/views/zz_adv_noreceipt.test.tsx` en `tree`. Salida:
```
ADV succeeded outcome="Hecho. El cambio está en marcha y guardado en tu perfil." act="Queda en el libro como acto act_01 — el recibo sella el desenlace y llegará cuando el cambio termine"
ADV rolled-back outcome="No se aplicó. Tu perfil sigue exactamente como estaba." act="Queda en el libro como acto act_01 — el recibo sella el desenlace y llegará cuando el cambio termine"
```

**De dónde sale ese cuerpo.** Es lo que sirve la puerta de estado cuando el cierre no llega a hacerse:
- `ConfigActRegistry.settle` devuelve el acto sin recibo cuando `ok=false` (`internal/app/config_act_registry.go:330-355`);
- `statusHandler` omite entonces `receipt_id` (`internal/controlapi/mutation.go:267-271`).

Lo provoca un libro que se vuelve ilegible u ocupado durante el cutover: el cierre lo rechaza por nombre. Y sobre un libro ilegible la recuperación se salta (`app.go:488-489`), así que ese recibo no llega nunca. La pantalla deja de sondear en el estado terminal (`WhatsHappening.tsx:383-403`, `:834`).

**Desenlace exigido.** El contrato de B («one outcome, one sentence») cubre también esta fila: después de un estado terminal no queda ninguna frase escrita para un cambio en curso.

### H4 · P3 · [PRODUCT] — Agotado el sondeo, la fila afirma algo que la pantalla no sabe

Hermano de B. Cuando se agota el presupuesto de sondeo, la fila afirma «tu perfil en disco todavía no ha cambiado», y la pantalla no lo sabe.

**Reproducción.** jsdom, ejecutado.

1. El POST responde el cuerpo real, `whats-happening-applying.json`.
2. `/api/reload/reload-1` responde 503 en los 120 sondeos (30 s).

Comando: `npx vitest run --reporter=verbose src/views/zz_adv_timeout.test.tsx`. Salida:
```
ADV polls=120 row="Aplicando… tu perfil en disco todavía no ha cambiado. Sigue aplicando; el estado tardó más de lo que esta pantalla espera. Vuelve a mirar en un momento."
```

**Dónde está.** La frase de `applying` está en `WhatsHappening.tsx:108`, y la rama del presupuesto conserva `outcome='applying'` (`:399-402`). Justo después, `load()` vuelve a leer `/api/config` (`:409`). Si el cambio sí se aplicó y esa lectura responde, la jaula enseña el host nuevo al lado de «todavía no ha cambiado»: la misma contradicción que vio TE50 con B. Esto último es una predicción por lectura; no lo ejecuté con un core real.

**Desenlace exigido.** Con el sondeo agotado, la fila no afirma nada sobre el estado del disco.

### H5 · P3 · [PRODUCT] — El paso de migración decide con el valor parseado y confirma con el valor crudo

Una versión guardada como BLOB que el parser común lee como anterior es «behind» para el lector y `ledger_unreadable` para el arranque.

**Reproducción.** Ejecutada en proceso, con SQLite nativo; sonda `TestADV_P1_blobOlderVersion` en `tree`.

1. `e3OldFile(t, 12)`: bootstrap v1 más los pasos de producción del 1 al 11, en una copia cerrada.
2. `UPDATE action_schema SET version = CAST('12 ' AS BLOB)`.
3. Se llama a `probeShape`, a `OpenReadOnlyFor(path, profileA)` y a `OpenFor(path, profileA)`.

Salida:
```
ADV stored typeof(version) = blob
ADV probeShape: shape=1 (… 1 older …) version=12 reason="schema v12, older than this binary's v16" err=<nil>
ADV OpenReadOnlyFor: handle=false err=action/sqlite: the ledger's schema is behind this binary: store "…/korvun.db" is at schema v12, this binary reads v16 — a read-only consult never migrates; run the server boot to lift the schema
ADV OpenFor: handle=false err=action/sqlite: migrate "…/korvun.db": action/sqlite: ledger_unreadable: the ledger's identity row cannot be read: the bump from v12 to v13 changed 0 version rows, want 1
ADV versions after: [12 ]
```

**Dónde está.**
- La relectura parsea el valor: `store.go:1297-1301`, con `parseStoredVersion` (`ledger_class.go:163-181`, cuyo godoc dice «the ONE contract of every reader of it … the migration»).
- El bump, en cambio, compara la celda cruda con un entero (`store.go:1331`, `WHERE version = ?`). Un BLOB nunca es igual a un INTEGER, así que `changed = 0`, y `:1340` lo nombra `ErrLedgerUnreadable`.
- TE17 solo prueba `'16 '`, donde no corre ningún paso.

**Alcance.** Falla cerrado y el paso se revierte entero. El problema es que la CLI manda al operador al arranque (`store.go:2395`), y el arranque declara ilegible (remedio: sustituir el fichero) un libro cuya versión acepta el propio parser. Solo se llega aquí con una versión BLOB escrita a mano.

**Desenlace exigido.** Un valor que el parser común acepta como versión anterior o se migra, o se rechaza con un nombre coherente con lo que aconseja el lector. Un molde lo fija.

### H6 · P3 · [TEST] — La rama de `Build` sobre una marca malformada no tiene molde

La rama de `app.Build` que arranca sobre una marca malformada no tiene molde: si se quita, `./internal/app` sigue en verde.

**Reproducción.** Mutación ejecutada en `tree2`, con la app real en proceso.

1. Sonda sin mutar: libro fundado; `DELETE FROM ledger_identity`; `UPDATE receipts SET result_digest = 'PROFILE:' || substr(result_digest, 9) WHERE result_digest LIKE 'profile:%'`; luego `e1Boot`.
2. Mutación GM-Z en `internal/app/app.go:425`: `unreadable := errors.Is(standingErr, actionsqlite.ErrLedgerUnreadable)`, sin `|| errors.Is(standingErr, actionsqlite.ErrLedgerMarkMalformed)`.
3. Con la mutación puesta: `go test -count=1 ./internal/app/` y la misma sonda.

Salida:
```
sin mutar: ADV GET ledger = map[owner:action/sqlite: ledger_mark_malformed: … path:…/korvun.db standing:unreadable]
con GM-Z, el paquete entero: ok  	github.com/Sebastian197/korvun/internal/app	44.398s
con GM-Z, la sonda: Build over the damaged ledger = app: judge the ledger's standing: action/sqlite: ledger_mark_malformed: …
```

**Por qué importa.**
- El tren tocó esa misma línea (`standingErr`).
- GE8 habla de «an unreadable standing», que en el código es esta variable, la de los dos centinelas. TE40 solo la ejerce con `ErrLedgerUnreadable`.
- La CLI sí cubre la marca malformada (`internal/cli/ledger_identity_test.go:64-89`), pero no pasa por `Build`.
- Incumple el punto 4 de la doctrina: rama peligrosa sin mutación roja.

**Desenlace exigido.** Un molde que se ponga rojo con GM-Z: el arranque sigue vivo, nombra `ledger_mark_malformed` y sus avisos llevan esa causa.

### H7 · P3 · [DOC] · no aplazable — La definición de `ledger_busy` es falsa para el límite D-1

La definición que dan las notas, el procedimiento y el godoc no se cumple en el límite conocido D-1.

**Reproducción.** Ejecutada en proceso, con la sonda `TestADV_P5_busyAtBirthOnANewPath` en `tree2`: 60 rondas de dos `OpenFor` que nacen a la vez sobre una ruta que todavía no existe. Salida:
```
ADV 120 opens: ok=85 busy=35 other=0; fastest busy after 347.149µs: action/sqlite: ledger_busy: another writer held the ledger past the busy timeout: action/sqlite: obtain a connection: database is locked (5) (SQLITE_BUSY)
```
Nadie esperó el busy_timeout: 347 µs frente a 5000 ms. El fallo está en el nacimiento de la conexión, no en una escritura ni en una lectura del juez.

**Textos del tren que dicen otra cosa.**
- `docs/releases/v0.16.2.md:160-161`: «`ledger_busy` es otro escritor que lo retuvo más allá del tiempo de espera». Lo añadió la tanda 4 (`t4/green.diff`).
- `docs/operations/ledger-restore.md:11-12`: «another writer held the ledger past the busy timeout».
- El godoc de `ErrLedgerBusy` (`internal/action/sqlite/ledger_identity.go:47-51`, tanda 1).

Las notas tienen que declarar además ese límite (D-1), así que la misma página se contradiría. El texto del propio centinela es de H y ya está adjudicado; estas definiciones son del tren E.

### H8 · P3 · [DOC] · no aplazable — «se funda como siempre» es falso en esta release

Las notas dicen que un fichero sin almacén de actos «se funda como siempre». En esta release, fundar es escribir la marca y la fila, y ese fichero queda `legacy_unfounded`.

**Reproducción.** Por lectura, apoyada en evidencia que ya existe.

1. `docs/releases/v0.16.2.md:140-141`: «un fichero que aún no tiene el almacén de actos (por ejemplo, el que ya lleva las conversaciones) es un almacén fresco y se funda como siempre».
2. La misma página define fundar en `:114`: la fila es «escrita solo por el acto fundacional —el que deja «Activar almacén»—».
3. Lo que hace de verdad la apertura es sembrar, y el libro queda `legacy_unfounded`. Lo muestran:
   - el caso sano de TE50 (§42.5: `ledger standing: legacy_unfounded`);
   - la aserción de TE51 (`internal/cli/ledger_e4_test.go:701`);
   - `ledger-restore.md:68`, «That ledger is not founded»;
   - el límite D-1.

La frase viene del tren D, pero cae en el rango que el §7 del plan asigna a E (`v0.16.2.md:125-156`; en la copia anterior a la tanda 4 está en la línea 141).

### H9 · P3 · [DOC] · no aplazable — Los «cuatro casos» de HANDOFF no son todos

HANDOFF dice que el arranque sobre un libro ilegible sigue vivo «salvo en cuatro casos con nombre». Los moldes del propio tren nombran más arranques que mueren.

**Reproducción.** Por lectura. Los moldes que cito pasaron en mi ejecución del subconjunto E.

1. `docs/HANDOFF.md:465-472` enumera cuatro excepciones: TE58, el juicio sin veredicto, TE18 y TE31.
2. TE32 (`ledger_e1_birth_test.go`, «The writers hand out nothing»): con una tabla `actions` virtual, `OpenFor` devuelve nil y `Build` muere. La forma es MALA y no está entre las cuatro.
3. TE36 y TE38 (`internal/app/ledger_e3_boot_test.go:87-110` y `:159-196`): un paso de migración que falla hace morir a `Build` con `ErrLedgerUnreadable`.
4. TE29 (el fallo de una sentencia del hook deja la apertura en nil, con su clase) y la carrera de la siembra con forma mala (`store.go:1684-1689`).

### H10 · P3 · [DOC] · no aplazable — Ningún recorder responde `controlapi.ErrLedgerUnreadable`

El godoc que corrigió el barrido previo al PR sigue diciendo que «un recorder» responde `controlapi.ErrLedgerUnreadable`, y ninguno lo hace.

**Reproducción.** `grep -rn 'controlapi\.ErrLedger' --include=*.go internal cmd | grep -v _test.go`:
- el único productor de `controlapi.ErrLedgerUnreadable` es `internal/app/approvals_adapter.go:618-619`;
- `configActRecorder` devuelve el error del almacén envuelto («record the operator act: %w», `internal/app/config_act.go:132`).

El texto está en `internal/controlapi/act.go:106-111`. Su lista de veredictos deja fuera además `ErrLedgerMarkMalformed`, que el adaptador mapea a este mismo centinela (`:618`), y los conjuntos de fila inválidos (`profile_standing.go:256-284`).

### H11 · P3 · [DOC] — Comentarios que localizan por posición, y una premisa falsa en la lista de cierre

Tres comentarios del tren localizan por posición («above», «below»), contra una ley CRITICAL. La lista de cierre registra el primero como «ajeno al tren», y eso es falso.

1. Los tres comentarios:
   - `internal/action/sqlite/store.go:1696`: «Seeded above…»;
   - `internal/action/sqlite/ledger_class.go:30`: «the three classes below»;
   - `internal/action/sqlite/profile_standing.go:174`: «the reads below».
2. Son del tren:
   - `grep -c 'Seeded above' base/internal/action/sqlite/store.go` da 0 en la base, y lo inserta el GREEN de la tanda 2 (`t2/green/apply_green.py:411`);
   - `profile_standing.go:174` es una línea `+` de la tanda 1 (`t1/diffs/profile_standing.go.diff:31`);
   - `ledger_class.go` no existía al empezar.
3. `docs/HANDOFF.md:191-193`, punto 6 de la lista de cierre: «ajeno al tren, hallado en la tanda 5». Esa adjudicación se apoya en una premisa falsa y deja fuera dos comentarios hermanos.

## Preguntas obligatorias, por garantía atacada

| Garantía | Cable | Molde que se pone rojo | Mutación ejecutada | Nivel | Lo que falta |
|---|---|---|---|---|---|
| GE1/GE1b | `store.go:1761-1799`, `:1646-1675`; `ledger_residue.go:107-176` | TE01–05, TE57, TE68–70 | las del autor MU01–05, MU57, MU68–70, todas rojas; ADV-GM-B roja (TE03, TE18, TE57) | TE01 y TE04 en procesos OS separados; TE68–70 con códigos sintéticos: honesto | — |
| GE2/GE3 | `ledger_class.go:59-109`, `readVerdict` `:83-87`; `judgeOn`; `judgeOnConn` | TE19–31 | 77 de 78 del autor (MU32-swallow-trigger adjudicada con TE29); ADV-GM-A roja (TE20–23, TE45, TE61/62); ADV-GM-W roja (TE26) | honesto | H1 |
| GE4 | `ledger_class.go:168-189`; `store.go:1297-1301`, `:2076` | TE17, TE65 | MU17, MU65 | honesto | H5, en el bump |
| GE5 | `ledger_residue.go:186-252` | TE09–11 | MU09–11; ADV-GM-C roja (TE10) | honesto | — |
| GE6 | `store.go:1281-1398` | TE33–39 | MU33–39 (27) | TE34/35 en procesos: honesto | H5 |
| GE7 | `config_act.go:183-214`; `whats_happening.go:359-380`; `WhatsHappening.tsx:465-476`; `cli/ledger.go:159-195` | TE42/43/48/49/55/56/60/66 | las del autor; ADV-TSM-E roja (el entorno deja de bloquear); ADV-TSM-F roja (unavailable pasa a bloquear) | unidad, HTTP real en proceso, jsdom, cada uno con su nombre: honesto | — |
| GE8 | `app.go:424-510` | TE40 | MU40 | honesto | H6 |
| GE9 | D21 sin tocar (adjudicado); TE41 | TE41 | MU41-index-v2 | — | — |
| GE10 | textos E | TE54 | MU54 ×3 | editorial | H7–H11 |
| B | `WhatsHappening.tsx:392-398` | `WhatsHappening.terminal.test.tsx` y el pin `whats_happening_applying_wire_test.go` | MU-B-screen y MU-B-wire del autor; ADV-TSM-G roja (rolled-back, failed, persist-failed) | jsdom; HTTP en proceso con fakes, declarado: honesto | H3, H4 |

## Clases (a)–(i)

- **(a)** `openOperatorWithIdentity` toma cualquier error de `os.Stat` como ausencia (`store.go:1556-1557`). En la práctica coincide con un fallo de apertura, así que no lo cuento como hallazgo.
- **(b)** H5.
- **(c)** Nada nuevo: los errores de `Fprintf` y de `Close` descartados ya estaban declarados.
- **(d)** H6.
- **(e)** H7–H10.
- **(f)** Nada: TE55 compara el JSON serializado.
- **(g)** Nada nuevo; el texto de `mapGuardError` es de H.
- **(h)** Verificado contra capturas: 1605,1 s (`t3/baseline-analysis.txt`), 1511,3 s (`after-parallel-analysis.txt`), 1734,190 s (`t2/quality.txt:51`); 1800 − 1734 = 66.
- **(i)** Ninguna aserción either/or en los moldes E (grep).

## Familias de ataque

- **Lecturas viejas y TOCTOU:** H1.
- **Parcial, rollback y crash:** los cubren TE01, TE35 y TE51; nada nuevo.
- **Ausencia frente a corrupción:** H5 y H6.
- **Error transitorio tomado por veredicto:** la rama sintética del hook está declarada (§7). H1 es la versión real: una escritura legítima.

**Doctrina.** Los moldes de B cumplen los seis puntos. H6 es un hueco del punto 4. Una observación que no cuento como hallazgo: en TE60, la mitad bloqueada de la pata «foreign ledger and its adoption» es vacía por construcción. «Adoptar libro» no puede pintarse con una posición ilegible o de entorno, porque las dos posiciones se excluyen.

## Alcance

**Leído.**
- Plan §1–§13, entero hasta el Anexo A.
- Informe §42, y de §37 a §41 por muestreo en sus mutaciones y aprobados.
- Enteros: `ledger_class.go`, `ledger_judge_seams.go`, `ledger_shape.go`, `ledger_residue.go`, `ledger_identity.go`, `profile_standing.go`, `config_act.go`, `controlapi/act.go`, `WhatsHappening.tsx`, `cli/ledger.go`, notas v0.16.2 y `ledger-restore.md`.
- Por diff contra la base: `store.go`, `app.go`, `quality.yml`, `Makefile`, `HANDOFF.md`, `approvals.go` y `approvals_test.go`.
- Por tramos: `whats_happening.go`, `config_act_registry.go`, `approvals_adapter.go:560-652` y `mutation.go:247-277`.
- Moldes: terminal, wire B, e4 de la pantalla (TE60/66), recorder (TE48/55/56/49), puertas (TE44/45/61/62), CLI (TE13/15/31/52/51), migración (TE33/34).

**Ejecutado** (en copias; el árbol solo con `go vet`, que no escribe en él):
- Subconjunto E sin -race (`-run 'TestE[1-5]_|TestB_'`): sqlite 91,1 s, app 12,3 s, cli 62,6 s, controlapi 2,6 s, e2e-harness 1,3 s; EXIT 0.
- Paquetes enteros sin -race: sqlite 225,8 s, app 75,3 s, cli 112,6 s; EXIT 0.
- controlapi y e2e-harness con -race: ok.
- vitest: los tres ficheros de WhatsHappening, 49 tests en verde; la suite completa, 50 ficheros y 561 tests (los 48/558 del autor más mis dos ficheros de sonda).
- `go vet` sobre los 5 paquetes: limpio.
- Mutaciones mías: A, B, C, W, E, F y G rojas; Z sobrevive.
- Sondas P1–P6, con las salidas citadas arriba.

**No verificado.**
- `make quality`: no lo ejecuté, por orden. Tampoco `-race` sobre sqlite, app y cli. El último `make quality` del autor (09:10) es anterior a la cura de B y al comentario de `act.go`, así que hay que volver a pasar el gate.
- TE50: el caso sano corrió sobre un artefacto de un árbol anterior a B. El ilegible y el no disponible no se han ejecutado. Es gate previo al PR (§13.8) y este veredicto no lo cubre.
- El tiempo de sqlite en el runner de Windows frente a los 60 m.
- `govulncheck`: no lo re-ejecuté.
- Las notas y el HANDOFF todavía no tienen los límites de D-1 y D-2. Los borradores del autor fuera del árbol (`t7/notes-*.md`, `handoff-card.md`) no los audité. Uno de ellos ya describe casos de TE50 que §42 da por no ejecutados.
- Las ~290 mutaciones del autor: no las re-ejecuté; revisé sus registros. MU41-index y MU61-verdictbranchonly sobrevivieron primero y cayeron con las versiones v2/close4.
- La mitad de Go de H3 la saco por lectura.
- La nota «fuera de verificación» de los triples (origen, sitio, fixture) inalcanzables: no la audité.

**Rutas de la evidencia.** Sondas y mutaciones en `/private/tmp/claude-501/-Users-sebastianmorenosaavedra-Desktop-korvun-nosync/9058be48-6f33-4a92-b847-a9d29f3100f5/scratchpad/adv-e-final/`:
- `tree/` y `tree2/` (ficheros `zz_adv_*`, y la instrumentación `zz_adv_seam.go` junto a los dos puntos de pausa, solo en `tree2`);
- `mut/go1-results.jsonl` y `mut/ts1.json`;
- `baseline-trainE.txt` y `full-pkgs.txt`;
- `untracked-hashes-start.txt` y `untracked-hashes-end.txt`.

---

*Persistido por el ejecutor el 2026-09-27, copiado del mensaje de entrega del adversario sin cambios en el texto; solo se quitó la sangría de dos espacios que añade el arnés. El adversario no escribió en el árbol: su definición se lo prohíbe.*
