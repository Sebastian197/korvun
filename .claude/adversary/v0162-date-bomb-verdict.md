VETO LEVANTADO

Las siete pasadas del adversario sobre el delta de saneamiento de master (orden del director del 2026-10-03): el commit 1, la bomba de tiempo de los tests, y lo que el commit 2 añade a la cura de AS07 (la entrada 18 del HANDOFF, la línea de las notas y su mensaje). Las cinco primeras juzgaron un evaluador de nombres (puertas v1 a v6) que el director retiró del PR tras la quinta, con la opción D; las siguientes juzgaron el gate que dictó en su lugar, `internal/testgates`. En orden y copiadas literalmente. La última es el veredicto que vale; las anteriores quedan como historia de lo que encontró y de cómo se curó.

Custodia: el adversario no escribió en el árbol. Entregó cada veredicto como mensaje final de su pasada, y cada uno se copió byte a byte desde su propia transcripción. Hora (UTC) y sha256 de cada mensaje:
- Primera pasada: 2026-10-03T06:41:48.682Z, `adbda7b0f64d37f49f0d29b9c37a9931c951433b97d9b3c048a9a8d3c3939d02`.
- Segunda pasada: 2026-10-03T07:27:49.628Z, `afef01a53f552e7357ff6dbf66d7be9c87529edf9adb20b6fb048c3718ebbf93`.
- Tercera pasada: 2026-10-03T08:01:30.042Z, `2c0b66223ca11c9574979d36fe3f52441b193ec0ec865977e99e40c623722595`.
- Cuarta pasada: 2026-10-03T08:23:28.268Z, `7a1503133a73e70b59e8347f879a4200da0e8029f57b585ba00d127dd95df2ad`.
- Quinta pasada: 2026-10-03T08:59:14.506Z, `c4bedb82c035b10aaec9f66e8e85f9eb302335a49eff14330258c21e0b864f02`.
- Sexta pasada: 2026-10-03T10:53:29.647Z, `25e06b5e6eeb3457fe9750f7f5eb2d44bdd621e442021707c6854b18efeca777`.
- Séptima pasada: 2026-10-03T11:20:30.625Z, `65679bb1d87c300ebedb0d10fff8f156d70eb821e6482dfdcdab399b2d08c82b`.

# Primera pasada — VETO MANTENIDO

VETO MANTENIDO

Objeto: `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `master-test-sanitation`, HEAD `abb51d4975bec01dc38fd957df9d51d5de5f50fe`. Del commit 1 audito el delta completo, sin commit: cuatro tests modificados y dos nuevos en `internal/cli`. Del commit 2, solo `commit2-docs.patch` y `commit2-msg.txt`.

Integridad, comprobada al empezar y al terminar:
- `git status --porcelain` no cambia: 4 `M` y 2 `??`.
- `git diff --name-only HEAD` da solo los cuatro `*_test.go`, así que la producción está intacta.
- El diff del árbol coincide byte a byte con `commit1-delta.patch`. Salidas: «PATCH == WORKING TREE» al empezar y «TREE STILL == commit1-delta.patch» al terminar.
- El sha256 del gate y del escáner coincide con `freeze-c1-final.txt` (`5172c8e3…` y `9a0d7780…`).

No escribí en el árbol. Trabajé en copias: `SCRATCH/adv-c1/tree` (rsync sin node_modules, .git, website ni coverage.out) y `SCRATCH/adv-c1/tree-as07`. Cada mutación es un reemplazo exacto y se restaura con comprobación sha256 (`SCRATCH/adv-c1/advmut.py`); todas las salidas dicen `restored=True`.

Línea base en la copia:
- `go test -race -count=1 -run 'TestNoFixedDateFeedsAValidityBound|TestFixedDateScanner_|TestGrantDelegate_|TestGrantIssue_|TestIntentCreate_|TestIntentV2CLI_|TestOperatorCLI_OpensAProfileTheBaseCLITouched|TestOperatorAct_|TestAuthorityCLI_|TestIntentBindGrantCLI_|TestIdentity_|TestE4_' ./internal/cli/` → `ok … 91.186s`.
- `gofmt -l` sobre los seis ficheros: sin salida.
- `go vet ./internal/cli/` en darwin, windows y linux: OK.

Lo que sí se cumple de la orden, ejecutado: el mínimo del gate (`--expires` seguido de un literal RFC3339) y su mutación. ADV-T1 repite en mi copia la MU-T1 del autor: la fecha de la base vuelve a issueGrantID y se quita el import. Resultado:
- `fixed_dates_gate_test.go:66: grant_test.go:43:16: --expires 2026-09-30T00:00:00Z — a fixed date`;
- las dos de delegación en rojo con `denied (authority_expired)`.

R3 y el verde del autor casan con esto.

## Hallazgos

### P2-1 [TEST][DOC] El gate no ve la forma que la cura dio al fixture v2 ni la bomba original escrita con un alias de dos valores

Lo que prometen el mensaje y los godocs es más ancho que el cable.

Reproducción. Cada caso corre `go test -count=1 -run '<tests>' -v ./internal/cli/` sobre la copia:
1. ADV-M1, en `intent_v2_test.go:23`: `wallStamp(-48*time.Hour), wallStamp(365*24*time.Hour))` → `"2026-09-19T12:00:00Z", "2026-10-20T12:00:00Z")`. Salida: `--- PASS: TestNoFixedDateFeedsAValidityBound`, `--- PASS: TestIntentV2CLI_CreateActivateVerifyBind`, `ok`. Queda plantada una bomba a 17 días en el fichero que este commit cura, con el gate y la suite en verde.
2. ADV-M2: lo mismo con `"2026-10-01T12:00:00Z"`. El gate da `PASS`. El test cae: `intent_v2_test.go:42: intent verify-v2: code=1 stderr="korvun intent verify-v2: action: intent expired\n"`, `--- FAIL: TestIntentV2CLI_CreateActivateVerifyBind`. La fecha se juzga contra el reloj real y el gate no la ve.
3. ADV-M3, en issueGrantID: `parentExpiry, _ := time.Parse(time.RFC3339, "2026-09-30T00:00:00Z")` y `"--expires", parentExpiry.Format(time.RFC3339)`. Salida:
   - `--- PASS: TestNoFixedDateFeedsAValidityBound`;
   - `grant_test.go:149: … denied (authority_expired) …` y `grant_test.go:178: … denied (authority_expired) …`, con las dos de delegación en `FAIL`.

   Es la bomba del 30 de septiembre, escrita como se parsea una fecha en Go.
4. ADV-C2, en `operator_act_test.go:40`: `now := time.Now().UTC()` → `now, _ := time.Parse(time.RFC3339, "2026-09-01T00:00:00Z")`. El gate da `PASS`; el test cae con `operator_act_test.go:66: issue: 1 "korvun grant issue: denied (intent_expired) …"`, `--- FAIL: TestOperatorAct_endToEnd`. El control ADV-C1, con `now := time.Date(2026, 9, 1, …)`, sí se ve: 5 hallazgos, en `operator_act_test.go:48`, `:49`, `:64` y dos en `:102`.
5. Batería sintética (`SCRATCH/adv-c1/out-shapes.txt`, el `scanFixedDates` real sobre fuente construida): de 26 formas con una fecha futura fija, 18 dan 0 hallazgos. Entre ellas:
   - un campo de un test por tabla (`run("--expires", tc.expires)`);
   - una función auxiliar que devuelve la fecha, o la fecha como parámetro de un helper;
   - `t0, _ := time.Parse(…)` y un `range` sobre fechas;
   - `fmt.Sprintf` con `%s` y argumento fijo, que es la forma del fixture v2;
   - `json.Marshal(map[string]any{"expires_at": …})`, que es la forma de intentTestConfig;
   - `"-expires"`, que el paquete `flag` acepta, y el flag guardado en una const;
   - `"--expires=" + fecha`, `time.Unix` e `import tm "time"` con `tm.Date(…)`.

Dónde está el cable, en `fixed_dates_scan_test.go`:
- `:111-118`: solo liga asignaciones con tantos valores como nombres; `x, _ :=`, `range` y los parámetros se quedan fuera.
- `:183-185`: reconoce time.Date por el nombre del identificador `time`.
- `:208-218`: un JSON solo cuenta si la clave y la fecha están en el mismo literal.
- `:147-153`: los flags se reconocen por su texto exacto.

Promesas más anchas que ese cable:
- `commit1-msg.txt:20-24`: «fails on a validity bound written as a fixed date: … through an alias, … and the validity keys of JSON fixtures».
- `fixed_dates_scan_test.go:47-52`: «names every validity bound … through a name bound to such a value in the enclosing function».
- `intent_test.go:26-30`: «a fixed date is the date bomb TestNoFixedDateFeedsAValidityBound refuses».

Mutaciones que faltan, y las dos sobreviven: devolver la fecha fija al fixture v2 en su forma nueva (ADV-M1) y escribirla con un alias de dos valores (ADV-M3). El mínimo literal de la orden se cumple. Lo que falla es la ampliación que el autor declara en su (a), y que el sitio curado se queda sin guarda.

### P2-2 [TEST][DOC] La excepción histórica blanquea una bomba futura, y su rechazo del time.Date anotado no tiene mutación roja

Reproducción:
1. ADV-M4, en `grant_test.go:44`: `"--expires", func() string { t0, _ := time.Parse(time.RFC3339, "2026-09-01T00:00:00Z"); return t0.Add(400 * 24 * time.Hour).Format(time.RFC3339) }(), "--depth", "2") // historical-date: laundered`. El gate y TestGrantDelegate_ dan todo `PASS`, `ok`. La concesión caduca el 2027-10-06 y el gate la acepta como histórica.
2. ADV-M4c, el control: la misma línea sin el comentario da `fixed_dates_gate_test.go:66: grant_test.go:44:16: --expires 2026-09-01T00:00:00Z — a fixed date`, `FAIL`. Lo que blanquea es la anotación.
3. Sintético, en `out-shapes.txt`, con 0 hallazgos en los dos casos:
   - `mustParse("2026-09-01T00:00:00Z").AddDate(2,0,0)` anotado;
   - `"2026-09-01T00:00:00Z"[:3] + "9-12-31T00:00:00Z"` anotado, que vale 2029-12-31.
4. ADV-U1: quito `case calls > 0: reason = reasonNotPlain` (`fixed_dates_scan_test.go:231-232`) y corro `go test -race -count=1 -run 'TestNoFixedDateFeedsAValidityBound|TestFixedDateScanner_' -v ./internal/cli/`. Los dos dan `PASS`, `ok … 1.885s`. Hoy el código sí rechaza el time.Date anotado (sondas H1 y H2: `annotated as historical, but not a plain RFC3339 instant`), pero ninguna fila lo sostiene.

El cable es `decide` (`fixed_dates_scan_test.go:222-249`): juzga las piezas literales de `dates`, no el valor que llega al flag.

Promesas afectadas:
- `fixed_dates_gate_test.go:12-14`: «only while it is a plain RFC3339 instant in the past»;
- `fixed_dates_scan_test.go:52-54` y `:220-221`;
- `commit1-msg.txt:24-27`: «A historical date passes only when … a plain RFC3339 instant in the past. A table holds the scanner to each shape it claims»;
- `fixed_dates_gate_test.go:70-71`: «each shape of a fixed date it claims to catch».

Por la doctrina, punto 4: ADV-U1 es una rama vigilada cuya neutralización no enrojece nada.

### P2-3 [DOC] El barrido no ve la clase en internal/action/sqlite y el mensaje del commit lo niega

Reproducción:
1. ADV-S0 en la copia: `go test -count=1 -run '^(TestSweep_runsOnThePruneCadence|TestSweep_aServerThatOnlyParksStillSweeps|TestSweepExpiredApprovals_closesWithReceiptAndPurgesParams)$' -v ./internal/action/sqlite/` da tres `PASS`.
2. ADV-S1: en `store_test.go:29`, `time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)` pasa a `time.Date(2027, 8, 30, …)`, que es lo que veían estos tests antes del 2026-08-30T10:01Z. La misma orden da tres `FAIL`:
   - `approvals_r4_test.go:107: AUDIT F3: … <nil> PENDING`;
   - `approvals_r4_test.go:56: exactly the expired one sweeps: 0`;
   - `approvals_r4_test.go:88: AUDIT R4: … <nil> PENDING`.

El barrido define la clase como una fecha fija que la ruta probada compara con el reloj real. Esto lo cumple:
- `expiredParked` (`approvals_r4_test.go:24-38`) fija `ExpiresAt = env.RequestedAt.Add(time.Minute) // long past by now` sobre la fecha fija de testEnvelope (`store_test.go:29`).
- TestSweep_runsOnThePruneCadence (`:78-89`, con `pruneEvery = 1` en `:82`) y TestSweep_aServerThatOnlyParksStillSweeps (`:96-108`) no inyectan reloj.
- La comparación la hace la producción: `noteWrite` → `SweepExpiredApprovals(ctx, time.Now().UTC())` en `store.go:2185`, llamada desde `store.go:2153` y desde `approvals.go:181`.

Es una fecha histórica, ya pasada y estable. Pero la orden dice que eso «se deja y se anota», y también: «si al barrer aparece la clase en otro paquete, el mismo gate allí». La misma fecha, comparada con `time.Now()`, la usan también:
- `atomic_r4f3_test.go:156`, `:177` y `:202`;
- `atomic_r5s5_test.go:163` y `:185`;
- `evidence_r5s1_test.go:38`;
- `migration_v9_test.go:171`, `:209` y `:217`.

Frases falsas:
- `commit1-msg.txt:31-33`: «The sweep of the repository found the class only here: elsewhere a fixed date is compared with a clock the test injects, or not compared at all».
- La conclusión de `sweep.md`: «La clase solo vive en internal/cli…».

A la tabla le faltan además estos sitios, todos de destino I (reloj inyectado), comprobados:
- El reloj del fixture de AS07 es `internal/action/sqlite/identity_phase1_test.go:29`, vía `authority_phase3_test_helpers_test.go:64`. El barrido atribuye AS07 a `internal/action/authority_phase3_test_helpers_test.go:25`, que es otro fixture.
- `internal/action/attenuation_test.go:20`.
- `internal/action/validity_test.go:18`.
- `internal/action/sqlite/contracts_test.go:22`.
- `internal/identity/identity_phase1_test.go:18`, que llega a `:436`.

### P2-4 [TEST] Tests tocados entran sin elevar, y la declaración (c) es falsa en dos de los cuatro ficheros

La cláusula de cierre de la doctrina dice que quien toca un test viejo lo eleva.

Reproducción:
1. ADV-P1d, en producción en la copia (`internal/cli/intent.go`, antes de `intent := action.IntentContract{`): `if !until.IsZero() { until = until.AddDate(0, 0, 1) }`. Con `go test -count=1 -run 'TestIntent|TestGrant|TestOperator|TestAuthority|TestReceipt|TestLedger|TestApprovals|TestV0151B|TestCrossCheck|TestIdentity|TestNoFixed|TestFixedDate' -v ./internal/cli/` sale `--- PASS: TestIntentCreate_persistsDraftWithReceipt (0.09s)`, `ok … 11.518s`, sin ningún `FAIL`.
2. `grep -n -i 'approved-red'` sobre los cuatro ficheros: solo aparece en `grant_test.go:9` e `intent_test.go:8`.
3. `grep -n -i 'evidence level'` sobre los mismos: solo `authority_upgrade_test.go:80`.

Tests y puntos de la doctrina que incumplen:
- TestIntentCreate_persistsDraftWithReceipt (`intent_test.go:91`, tocado en `:99`). Punto 4: falta la mutación «la caducidad guardada no es la que se entregó»; ADV-P1d sobrevive porque el aserto de `:122-124` solo mira IsZero. Punto 6: sin etiqueta de evidencia.
- TestIntentV2CLI_CreateActivateVerifyBind (`intent_v2_test.go:30`, tocado en `:50`). Punto 6: sin etiqueta. No hay mutación declarada para la línea tocada, y su fichero no lleva contrato de rojo aprobado.
- TestGrantIssue_inactiveOrExpiredIntentFailsClosed (`grant_test.go:80`, tocado en `:99`). Punto 6.

La doctrina no admite «el test ya existía» como excepción. Con el contrato de rojo aprobado de grant_test.go e intent_test.go hay choque: la doctrina gana y el choque se le lleva al director. intent_v2_test.go y authority_upgrade_test.go no tienen ese contrato.

### P3-1 [TEST] Las dos guardas de vacuidad no cubren lo que anuncian

1. La sonda por fichero da `fixed_dates_scan_test.go seen=1`. El propio literal `validityFlags` (`fixed_dates_scan_test.go:27`) cuenta como un límite: `--expires` seguido de `--valid-from`.
2. ADV-M5: si el recorrido solo lee los dos ficheros del gate, sale `--- PASS: TestNoFixedDateFeedsAValidityBound (0.00s)`.
3. ADV-M6: con el escáner ciego a toda llamada, el gate da `PASS`. Solo cae la tabla, en 16 filas.
4. ADV-M7: quito `|| len(files) != want` (`fixed_dates_gate_test.go:58`) y los dos tests siguen en `PASS`. Esa condición es inalcanzable: cada fichero o se añade a la lista o hace caer el test con Fatalf.

El godoc «seen counts the bounds judged, fixed or not» (`:54-55`) no vale para JSON, porque `judgeJSON` solo cuenta las claves que llevan fecha.

### P3-2 [DOC] Dos frases del godoc de la tabla

- `fixed_dates_gate_test.go:75-77`: «A date assembled at run time from numbers … is outside what a scan of the source can see». La fila `time.Date` (`:114`) es exactamente eso, y el escáner la ve. Un Sprintf con enteros está en la fuente; lo que pasa es que este escáner no lo implementa.
- `:89-90`: «The instants are put in by a Replacer at run time». Las filas `:114` y `:129` llevan `time.Date(2099, 1, 1, …)` escrito a mano.

### P3-3 [DOC] «denies every delegation under it as authority_expired» (`commit1-msg.txt:4-5`)

ADV-M8: pongo la fecha de la base en issueGrantID y un log en TestGrantDelegate_revokedParentFailsClosed. Sale `grant_test.go:209: ADV revoked-parent delegate: code=1 stderr="korvun grant delegate: denied (authority_revoked): parent grant_7094… does not authorize at 2026-10-03T06:21:59Z\n"` y el test pasa. La R3 del autor ya lo mostraba: ese test estaba en PASS sobre la base.

### P3-4 [DOC] «hasta 5 s» contra las capturas (commit 2)

La línea está en `commit2/docs/releases/v0.16.2.md:275-276`.
- Capturas del adversario anterior: 5,091 s y 5,107 s (`adv-as07/out2-N-DL-door-deadline-chains.txt`).
- Mis tres pasadas del mismo probe, en `SCRATCH/adv-c1/tree-as07` con `go test -race -count=3 -run '^TestADV2_' -v ./internal/action/sqlite/`: (ii) 5.054, 5.044 y 5.041 s; (iii) 5.069, 5.076 y 5.077 s.
- Las 8 observaciones superan los 5 s.

El HANDOFF («agota el busy_timeout») y el mensaje del commit («waits out») son exactos; la nota de versión dice «hasta». Como el texto lo dictó el director, el choque con la ley del Tono le toca adjudicarlo a él.

El resto de la entrada 18 casa con la captura cruda, incluido el «igual» de (iii). Solo «vuelve en el acto» no está en la captura que cita, que no da tiempo para el caso (i). Lo medí yo: `(i) elapsed 42.948µs` y `21.3µs`. La frase es verdad, pero la prueba no está en el registro citado.

El encabezado «(P3, y un P2 fichado para la v0.16.3)» casa con la lista: 1 a 16 son P3, la 17 está curada y la 18 es el P2.

### P3-5 [TEST] Los nombres se resuelven por texto y eso da un falso positivo

Caso sintético G1: `var now = time.Date(2026, 8, 1, …)` en lo alto del fichero y `func issue(now time.Time) { run("--expires", now.Add(time.Hour).Format(…)) }` dan el hallazgo `--expires time.Date(…) a fixed date`, aunque el valor es relativo. Hoy no hay en el paquete ninguna var de nivel superior así.

## Preguntas obligatorias

**Delegación independiente de la fecha.** Cable: `grant_test.go:44` → `intent_test.go:31-33`. Si la garantía fuera falsa caerían las dos de delegación; está ejecutado (ADV-T1, ADV-M3). Nivel de evidencia: CLI en proceso sobre SQLite real, sin etiqueta en el fichero.

**Gate mínimo.** Cable: `fixed_dates_scan_test.go:147-148`, `:162-171`, `:178-181` y `:229-230`. Lo pone en rojo ADV-T1. El nivel declarado («in process, … go/parser») es honesto.

**Formas extra que declara el autor.** Las mutaciones MU-S1 a MU-S8 se ponen en rojo. Faltan el alias de dos valores y el JSON construido con Sprintf; sobreviven ADV-M3, ADV-C2 y ADV-M1 (P2-1).

**Excepción histórica.** MU-S9, S10 y S11 se ponen en rojo. ADV-U1 sobrevive y ADV-M4 la blanquea (P2-2).

**Guardas de vacuidad.** MU-S14 y S15 se ponen en rojo. ADV-M5, M6 y M7 sobreviven (P3-1).

**Barrido.** No hay test; la frase del commit es falsa (ADV-S1, P2-3).

**Docs del commit 2.** Las cifras casan con la captura cruda; «hasta 5 s» no (P3-4).

**Qué tendría que ser falso para que cada test se ponga en rojo:**
- Gate: un literal fijo tras un flag dentro del paquete. Ejecutado.
- Tabla: cada rama vigilada. Ejecutadas las 17 del autor más U1 (sobrevive), U3 (roja) y U4 (roja, con pánico vía `validityFlags`).
- TestIntentCreate: que la caducidad guardada sea otra. Ejecutado (ADV-P1d) y sigue en verde.

**Etiquetas de evidencia.** Las del gate y la tabla son honestas. Los tests tocados no llevan etiqueta (P2-4).

## Nueve clases

- (a) No hay problema: la cadena vacía cuenta como límite sin fecha, que es lo correcto.
- (b) y (g): ADV-M4, porque se juzga el literal y no el valor; G2, `time` reconocido por nombre; F7 y F8, flags por texto exacto; G1, nombres por texto y no por ámbito.
- (c) No: Unquote no puede fallar sobre un literal ya parseado.
- (d): ADV-U1, ADV-M5, ADV-M6, ADV-M7 y ADV-P1d.
- (e): P2-1, P2-2, P2-3, P3-2, P3-3 y P3-4.
- (f) No.
- (h): «hasta 5 s» y «every delegation». «Seventeen» y «49 files» los verifiqué.
- (i) No: la tabla exige un recuento exacto y una razón exacta.

Patrón de la historia del repositorio: un sello que firma una historia distinta de la registrada. La anotación declara un literal pasado y el valor que viaja al flag es una fecha futura (ADV-M4).

Es mi primera pasada sobre este objeto.

## Alcance

**Leído:**
- El brief.
- `commit1-delta.patch` entero.
- Los dos ficheros nuevos completos, y los cuatro tocados en sus sitios y helpers.
- Los mensajes de los dos commits y `commit2-docs.patch`.
- `pretest.md`, `sweep.md` y todos los ficheros red, green y freeze.
- `mutations-c1.json`, `.jsonl`, `-final.jsonl` con su resumen y `-final2.jsonl`; `mutate_tree.py`; el diff de `fixed_dates_gate_test.go.frozen-r1` contra el final.
- La segunda pasada del registro de AS07 entera, la captura cruda N-DL y su spec.
- Del commit 2: el HANDOFF en `:270-400` y v0.16.2.md en `:255-290`.
- Producción: `intent.go:604-690`, los flags y el reloj de `grant.go`, `store.go:91`, `:1865-1900` y `:2153-2190`, `authority_v2.go:653-685`, `intent_v2.go:252-300`.
- Tests: los sitios de sqlite, action, identity, app y controlapi citados arriba, y los del frontend (`Approvals.test.tsx:20-45`, `158-172`, `1432-1450`, `1700-1740`).

**Ejecutado.** Las salidas están en `SCRATCH/adv-c1/`:
- `out-muts1-clean.txt`: ADV-M1 a M7. `out-muts1.txt` queda contaminado por mis propias sondas, que estaban en el paquete; lo sustituye el clean.
- `out-muts2.txt`: S0 y S1.
- `out-muts3.txt`: M8.
- `out-muts5.txt` y `out-muts6.txt`: P1b, P1c y P1d.
- `out-muts7.txt`: T1.
- `out-muts8.txt`: U1 a U4.
- `out-muts9.txt`: C1 y C2.
- `out-shapes.txt`: las baterías de formas y la sonda por fichero.
- `out-ndl-rerun.txt`.
- Además: la línea base, gofmt y vet.

**Sin verificar:**
- Ningún viaje en el tiempo (no hay faketime). Que no haya bombas futuras vivas fuera de internal/cli lo apoyan un escaneo de fechas literales (la única futura en tests es 2028-02-29 del frontend, con fake timers, y el valor de manipulación 2099 de sqlite) y el inventario de lecturas del reloj real, pero no una ejecución.
- golangci-lint, `make quality` (excluido por el brief), el paquete internal/cli entero con `-race` (corrí grupos dirigidos), la CI y el gate en Windows.
- El código de AS07, fuera de alcance.
- El mecanismo de `sqlite3_interrupt`, que sigue siendo una hipótesis.

# Segunda pasada — VETO MANTENIDO

VETO MANTENIDO

Segunda pasada sobre el delta de saneamiento de master. Objeto: `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `master-test-sanitation`, HEAD `abb51d4975bec01dc38fd957df9d51d5de5f50fe`. El commit 1 sigue sin commit; su delta es `SCRATCH/t15/commit1-delta-2.patch`.

Integridad, al empezar y al terminar:
- El diff del árbol coincide byte a byte con el parche. Salidas: «PATCH-2 == WORKING TREE» al empezar y «TREE STILL == commit1-delta-2.patch» al terminar.
- `git diff --name-only HEAD` solo da `*_test.go`; la lista de ficheros de producción cambiados sale vacía.
- El sha256 del gate y del escáner coincide con `freeze-c1v2-final.txt` (`ed518e45…`, `b2321089…`).

No escribí en el árbol. Refresqué la copia en `SCRATCH/adv-c1/tree2`: es idéntica al árbol en internal, cmd, web, scripts y docs. Todas las mutaciones corrieron allí con `advmut.py`, por reemplazo exacto y restauradas por sha256; todas dan `restored=True`.

Línea base en la copia:
- Los dos gates y la tabla, en verde.
- `go vet` de internal/cli e internal/action/sqlite en darwin, windows y linux: OK. `gofmt -l`: sin salida.
- Verde dirigido con `-race`: `ok … internal/cli 98.286s` (gates, tabla, Grant*, Intent*, OperatorAct, Authority*, IntentBindGrant*, Identity*, E4) y `ok … internal/action/sqlite 8.157s` (`TestClaim_refusesARowThatMovedInAnyColumn`, `TestSweep*`).

## Lo que el rediseño cierra, ejecutado

Re-ejecuté mi batería de la primera pasada (`SCRATCH/adv-c1/out2-muts1.txt` y `out2-shapes.txt`):
- ADV2-M1, fecha futura en los argumentos del Sprintf v2: F en rojo, `intent_v2_test.go:23:27: 2026-10-20T12:00:00Z — a fixed instant in the future`.
- ADV2-M3, la bomba original con un alias de dos valores: B en rojo, `grant_test.go:45:16: --expires 2026-09-30T00:00:00Z — a fixed date as a validity bound`.
- ADV2-C2: B en rojo, con 5 hallazgos en operator_act_test.go.
- ADV2-M4, el blanqueo derivado: B en rojo con «not the bound's own literal».
- ADV2-U1: las filas en rojo.
- ADV2-T1 (MU-T1): B en rojo y las dos de delegación con `authority_expired`.
- ADV2-S1: F en rojo con `../action/sqlite/store_test.go:29:3: time.Date → 2027-08-30T10:00:00Z`.
- De las 26 formas de la primera pasada, ahora se ven 23. Siguen sin verse F2 (Sprintf de enteros), F14 (strings.Join de dígitos) y F17 (fichero); la primera y la tercera están declaradas.
- G2, G5, G6, G7 y H2 se ven.
- El blanqueo por flags está cerrado.

Por los registros del autor, los 37 mutantes salen en rojo. Comprobé contra el código los arreglos de P2-3 (la tabla del barrido y la atribución de AS07), de P2-4 (etiquetas y MU-E1 a E4) y de P3-2 a P3-5.

## Hallazgos

### P2-1 [TEST][DOC] Una fecha escrita con una constante con nombre no la ve la regla F, aunque la cabecera y el mensaje prometen lo contrario

Lo mismo pasa con una conversión, una suma o un import punto. La cabecera y el mensaje definen el instante fijo como «spelled with constants … with constant arguments». La bomba original, escrita así, pasa las dos reglas.

Reproducción:
1. ADV2-R3p, en `grant_test.go`:
   - añado `const parentYear = 2026` antes de `// activeIntentID creates and activates one intent, returning its id.`;
   - la línea de `:44` pasa a ser `"--expires", time.Date(parentYear, 9, 30, 0, 0, 0, 0, time.UTC).Format(time.RFC3339), "--depth", "2")`.

   Con `go test -count=1 -run 'TestNoFixedDateFeedsAValidityBound|TestNoFixedInstantLiesInTheFuture|TestGrantDelegate_' -v ./internal/cli/` los dos gates pasan, y las dos de delegación caen:
   - `grant_test.go:170: the denial must NAME the widened dimension: 1 "korvun grant delegate: denied (authority_expired) …"`;
   - `grant_test.go:206: a strict subset delegation must pass: 1 "… denied (authority_expired) …"`.
2. ADV2-R3 hace lo mismo con `const parentYear = 2027`: todo `PASS`. Queda una bomba para el 2027-09-30.
3. ADV2-R4, en sqlite: `store_test.go:29` pasa a `time.Date(envYear, 8, 30, 10, 0, 0, 0, time.UTC)`, con `const envYear = 2027`. `TestNoFixedInstantLiesInTheFuture` da `PASS`, y los tres tests de barrido caen: `approvals_r4_test.go:56: exactly the expired one sweeps: 0`, y `:88` y `:107` con `<nil> PENDING`.
4. Sintéticos, en `out2-shapes.txt`. Todos dan `MISSED B=0 F=0`:
   - N4 y N4b, con `const year = 2030`;
   - N5, `time.Unix(int64(1893456000), 0)`;
   - N6, `time.Month(1)`;
   - N11, `1893456000+0`;
   - N7, `import . "time"`.

Dónde está el cable, en `fixed_dates_scan_test.go`: `:300-315`, porque `constInt` solo acepta literales `token.INT` y meses con nombre, y `:271`, que reconoce el paquete por su identificador.

Promesas más anchas que ese cable:
- `fixed_dates_gate_test.go:13-16`: «A fixed instant is one spelled with constants: … or a call of the time package's Date or Unix family with constant arguments».
- `commit1-msg.txt:26-28`: «a time.Date or Unix call with constant arguments».
- Los límites declarados (`fixed_dates_gate_test.go:22-25`, `commit1-msg.txt:35-37`) no nombran ni las constantes con nombre, ni las conversiones, ni el import punto. Este último solo aparece en `pretest.md`.

La frase del brief «It is judged where it is written, whatever flow carries it» es falsa para estas formas: el instante ni siquiera se cuenta.

### P2-2 [TEST][DOC] La regla B solo mira los flags y el JSON de un único literal

Una constante pasada que se deriva hacia el futuro en la misma función y llega a la CLI por un JSON relleno con Sprintf, o por un `--file`, es una bomba viva que pasa las dos reglas. Además, la excepción histórica todavía blanquea por la vía del JSON.

Reproducción:
1. ADV2-R1, en `intent_v2_test.go:23`: `time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC).Format(time.RFC3339), time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC).AddDate(0, 1, 0).Format(time.RFC3339))`. Los gates, la tabla y TestIntentV2CLI_CreateActivateVerifyBind dan `PASS`. Es una bomba para el 2026-10-19 dentro del fixture que este commit cura.
2. ADV2-R1p, la misma derivación con `.AddDate(0, 0, 10)` (cae en el 2026-09-29): los gates dan `PASS` y el test cae con `intent_v2_test.go:50: intent verify-v2: code=1 stderr="korvun intent verify-v2: action: intent expired\n"`. El fixture se juzga contra el reloj real.
3. ADV2-R2b, en `intent_bind_grant_test.go`: añado `"time"` a los imports, y en TestIntentBindGrantCLI_fillsTheBindingAndRevokesTheOldOne escribo `root := authorityCLIRoot(intent); root.ExpiresAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 2, 0); file := writeAuthorityGrantFile(t, root)`. Los gates y el test dan `PASS`: bomba para el 2026-11-01.
4. ADV2-R2bp, con `.AddDate(0, 1, 1)` (cae en el 2026-10-02): los gates dan `PASS` y el test cae con `intent_bind_grant_test.go:56: bind --grant: code=1 stderr="korvun intent bind: action/sqlite: authority expired\n"`.
5. ADV2-M2 pone un instante fijo pasado en los dos límites JSON vía Sprintf: `"2026-09-19T12:00:00Z", "2026-10-01T12:00:00Z"`. Los gates dan `PASS`; TestIntentV2CLI cae con «intent expired».
6. L2, sintético (`out2-launder.txt`): `raw := strings.Replace(`{"expires_at":"2026-08-02T00:00:00Z"}`, "2026", "2036", 1) // historical-date: …` da `MISSED`. La misma cirugía sobre un flag (L1) sí se rechaza con «not the bound's own literal».

Dónde está el cable, en `fixed_dates_scan_test.go`: `judgeJSON` (`:344-354`) juzga el literal donde está escrito y siempre lo trata como propio (`own=true` en `:351`); `judgeFlags` (`:319-341`) solo mira flags.

Promesas afectadas:
- `fixed_dates_gate_test.go:17-20`: B como garantía universal.
- `commit1-msg.txt:28-32`: «a validity bound handed to the CLI holds no fixed instant, written in it or reached through the names it uses. The one exception is the bound's own literal…».
- `intent_test.go:26-30`, sin cambios desde la primera pasada: «a fixed date is the date bomb TestNoFixedDateFeedsAValidityBound refuses».
- La frase del brief «A derivation, a name or a call is never accepted as historical».

El límite declarado, «derived from a past constant inside another function», no cubre R1 ni R2b: en los dos la derivación ocurre en la misma función.

### P3-1 [TEST] La guarda de vacuidad de B se alimenta de la fila de la propia tabla llamada "--valid-from"

Esa fila está en `fixed_dates_gate_test.go:144`.

1. La sonda por fichero (`out2-perfile.txt`) da `./fixed_dates_gate_test.go flags=1`.
2. ADV2-M5b restringe el recorrido de `.` a los dos ficheros del gate (`fixed_dates_gate_test.go:94`) y añade un log. Salida: `ADV flags=1 files=2`, `--- PASS: TestNoFixedDateFeedsAValidityBound`.
3. ADV2-M6 deja el escáner ciego a toda llamada: los dos gates dan `PASS` y solo cae la tabla, con 28 filas.

La cura del primer P3-1 trasladó el autoconteo del fichero del escáner al del gate. F caza la M5 y la tabla caza la M6.

### P3-2 [TEST] Formas prometidas sin ninguna mutación roja

- ADV2-U2: quito UnixMilli y UnixMicro, prometidos en `fixed_dates_scan_test.go:66`.
- ADV2-U3: quito solo `-valid-from`, prometido en `:28-29`.
- ADV2-U4: quito `valid_until`, `not_after` y `not_before` de `:34`.

En los tres casos los tres tests dan `PASS`. Frente a eso, `fixed_dates_gate_test.go:109-111` dice «holds the scanner to each shape it claims», y `commit1-msg.txt:32-33` dice «A table holds the scanner to the shapes it claims».

### P3-3 [DOC] «a string holding a date and a time» no casa con el regex

La frase está en `fixed_dates_gate_test.go:14` y en `commit1-msg.txt:26-27`. El regex `instantInText` (`fixed_dates_scan_test.go:39`) exige `T` y `\b`. Por eso N8 (`'2030-01-01 00:00:00'` dentro de un SQL) y N10 (`"id_2030-01-01T00:00:00Z"`) dan `MISSED`.

En sqlite el barrido compara `expires_at` como texto (`approvals.go:754-756`), pero los lectores del almacén solo parsean RFC3339Nano.

### P3-4 [TEST] Sobreaproximaciones que fallan cerradas

- N12: un campo de selector, `tc.expires`, se resuelve como si fuera la variable local `expires`.
- N13: una clave de `range` queda ligada al slice que se recorre.
- N16: un parámetro de función literal no tapa un nombre de la misma función, aunque el godoc de `bindNames` (`fixed_dates_scan_test.go:152-155`) dice «function-literal parameters, which hide».

Las tres dan un hallazgo de B sobre un límite relativo. Hoy ninguna aparece en el paquete.

### P3-5 [DOC] Límites declarados y lugar del gate

- `commit1-msg.txt:35-37` omite el cuarto límite que sí declara la cabecera (`fixed_dates_gate_test.go:24-25`, el reloj mezclado).
- La regla F de sqlite vive en internal/cli (`fixed_dates_gate_test.go:72`), así que `go test ./internal/action/sqlite/` a solas no la ejecuta. La orden decía «el mismo gate allí»; le toca adjudicarlo al director.

## Commit 2

La nota de versión (`commit2/docs/releases/v0.16.2.md:275-276`, «sigue esperando hasta agotar el `busy_timeout` de 5 s») casa con 5,091 y 5,107 s y con mis seis tiempos, de 5,041 a 5,077 s. La desviación del texto dictado está declarada.

La entrada 18 del HANDOFF (`commit2/docs/HANDOFF.md:376-391`) da «43 µs y 21 µs». Coincide con mis capturas de la primera pasada (42.948 µs y 21.3 µs), y el registro que cita entra con el commit 1, según `do-commit-1.sh`.

El mensaje del commit 2 no ha cambiado (sha `1e850df1…`).

## Preguntas obligatorias

**Regla F.** Cable: `fixed_dates_scan_test.go:193-207`, `:237-261`, `:266-315` y `fixed_dates_gate_test.go:71-81`. Se pone en rojo con MU-G4 y con mis ADV2-S1 y ADV2-M1. Mutaciones que faltan: la constante con nombre (R3 y R4 sobreviven) y U2.

**Regla B.** Cable: `:319-421` y `fixed_dates_gate_test.go:52-60`. Se pone en rojo con MU-T1, G5 y B1 a B17, y con mis ADV2-M3, C2, M4 y T1. Sobreviven R1, R2b, M2, L2 y R3p.

**Excepción histórica.** Cable: `:401-421`. Se pone en rojo con MU-B11 a B14 y con ADV2-M4 y U1; falta el caso JSON (L2).

**Guardas de vacuidad.** Cable: `fixed_dates_gate_test.go:54-56`, `:74-76` y `:103-105`. Sobreviven M5b para B y M6 para los dos gates.

**Tests elevados.** MU-E1 a E4 están capturados. Las etiquetas «in-process CLI … real SQLite file» son honestas. Las mutaciones E2 y E3 tocan la entrada del test, no la producción, y aun así ponen en rojo el aserto vigilado.

## Nueve clases

- (a) No.
- (b) R1 y L2: el límite JSON se juzga en el literal, no en el valor que llega a la CLI.
- (c) No.
- (d) U2, U3, U4, M5b y M6.
- (e) P2-1, P2-2, P3-2, P3-3 y P3-5.
- (f) No.
- (g) Nombres resueltos por texto (N12, N13, N16) y paquete por identificador (N7).
- (h) Verificado: 36 filas en R1', 37 definiciones de mutación, 43 y 21 µs.
- (i) No.

## Lo que esta pasada vio y la primera no

- La constante con nombre derrota a las dos reglas con la bomba original (R3p) y en sqlite (R4).
- Las derivaciones dentro de la misma función hacia un JSON relleno con Sprintf (R1) o hacia un `--file` (R2b) son bombas vivas, con la prueba del reloj real (R1p, R2bp).
- El blanqueo por JSON (L2).
- La guarda de B alimentada por la tabla (M5b).
- Los supervivientes U2, U3 y U4.
- La frase «a date and a time» frente al regex (N8, N10).
- Las sobreaproximaciones (N12, N13, N16).

## Alcance

**Leído:**
- El brief 2.
- Del árbol: los dos ficheros del gate completos, el diff de los cinco tests tocados y los fragmentos de `authority_test.go`, `intent_bind_grant_test.go`, `operator_act_test.go`, `store_test.go`, `approvals_r4_test.go` y `approvals.go:750-758`.
- `pretest.md` (incluido el rediseño), `sweep.md`, `commit1-msg.txt`, `commit2-msg.txt`, `commit2-docs-2.patch`.
- Los rojos R0', R1' y R2', los freeze, el verde, `mutations-c1v2.json` (las 37 definiciones), `mutations-c1v2-final.jsonl` y su `.progress`.
- `do-commit-1.sh`, `do-commit-2.sh` y `build-verdict-record-c1.py`.

**Ejecutado.** Las salidas están en `SCRATCH/adv-c1/`:
- `out2-muts1.txt` a `out2-muts6.txt`;
- `out2-shapes.txt`, `out2-perfile.txt` y `out2-launder.txt`;
- la línea base, vet y gofmt.

**Sin verificar:**
- Ningún viaje en el tiempo.
- golangci-lint y goimports, que solo constan en el `.progress` del autor.
- internal/cli entero con `-race` (corrí grupos dirigidos), la CI y Windows.
- El código de AS07, fuera de alcance.

# Tercera pasada — VETO MANTENIDO

VETO MANTENIDO

Tercera pasada sobre el delta de saneamiento de master. Objeto: el mismo árbol, rama `master-test-sanitation`, HEAD `abb51d4975bec01dc38fd957df9d51d5de5f50fe`. El delta es `SCRATCH/t15/commit1-delta-3.patch`.

Integridad, al empezar y al terminar:
- El diff del árbol coincide byte a byte con el parche. Salidas: «PATCH-3 == WORKING TREE» al empezar y «TREE STILL == commit1-delta-3.patch» al terminar.
- Ningún fichero de producción cambiado.
- El sha256 del gate y del escáner coincide con `freeze-c1v3-final.txt` (`b71da9a4…`, `47b3b6eb…`).
- Los tests tocados fuera del gate son idénticos a los de la v2.
- El commit 2 no ha cambiado: `commit2-docs-2.patch` es `fa78489b…` y `commit2-msg.txt` es `1e850df1…`.

No escribí en el árbol. Copia refrescada en `SCRATCH/adv-c1/tree3`, idéntica al árbol. Mutaciones por reemplazo exacto con `advmut.py`, restauradas por sha256; todas dan `restored=True`.

Línea base en la copia:
- Gates y tabla en verde.
- `go vet` de internal/cli e internal/action/sqlite en darwin, windows y linux: OK. `gofmt -l`: sin salida.
- Dirigido con `-race`: `ok … internal/cli 37.897s`.

## Lo que la v3 cierra, ejecutado

Mis baterías de las dos pasadas anteriores, contra la v3 (`out3-battery12.txt`, `out3-rerun.txt`):
- Ahora se ponen en rojo:
  - R1, por la plantilla Sprintf: `intent_v2_test.go:23:3` y `:71`;
  - R2 y R2b, por la regla de campos: `authority_test.go:100:20` e `intent_bind_grant_test.go:34:19`;
  - R3 y R3p, con la constante con nombre: B en `grant_test.go:46:16`, y F también en la de 2027;
  - R4, F en `../action/sqlite/store_test.go:29:3`;
  - M1 a M6, C2, U1, T1 y S1;
  - M5b: `ADV flags=0 files=2`, «no validity flag seen»;
  - M6: «28 instants written as text and 0 time calls»;
  - U2 y U5.
- Sintéticos ahora vistos: N1, N2, N2s, N4 a N8, N10, N11 y L2. El blanqueo por JSON queda cerrado. N12 y N13 ya no dan falso positivo.
- La tabla de la propia gate ya no alimenta la guarda de B: en el recorrido «.» hay 10 flags, ninguno del fichero del gate.
- Siguen sin verse F2, F14 y F17 de la primera pasada, las tres declaradas como formato numérico o fichero. N9, la fecha sola dentro de un SQL, no se promete.

## Hallazgos

### P2-1 [TEST][DOC] Las promesas de B siguen siendo más anchas que lo que el escáner juzga

Hay tres clases de límite prometidas que B nunca juzga: el valor de un flag rellenado por `fmt.Sprintf`, una plantilla JSON guardada en un nombre o bajo un `fmt` renombrado, y un campo de validez asignado en una asignación de varios valores. La bomba original pasa las dos reglas por la primera.

Reproducción:
1. ADV3-R3cp, en `grant_test.go`:
   - añado `"fmt"` a los imports;
   - la línea de `:44` pasa a ser `fmt.Sprintf("--expires=%s", "2026-09-30T00:00:00Z"), "--depth", "2")`.

   Con `go test -count=1 -run 'TestNoFixedDateFeedsAValidityBound|TestNoFixedInstantLiesInTheFuture|TestGrantDelegate_' -v ./internal/cli/`, los dos gates dan `PASS` y las dos de delegación caen:
   - `grant_test.go:170: the denial must NAME the widened dimension: 1 "… denied (authority_expired) …"`;
   - `grant_test.go:206: a strict subset delegation must pass: 1 "… denied (authority_expired) …"`.

   F ve el literal, pero como es pasado no lo rechaza; B no juzga el argumento de la plantilla de un flag.
2. ADV3-R3c, con `time.Date(2026, 9, 30, …).AddDate(1, 0, 0)` como argumento: todo `PASS`. Queda una bomba para el 2027-09-30.
3. ADV3-R1c, en el fixture v2 (`intent_v2_test.go:22-23`): la plantilla pasa a un nombre (`tpl := `…``) y se rellena con `fmt.Sprintf(tpl, base.Format(time.RFC3339), base.AddDate(0, 1, 0).Format(time.RFC3339))`, con `base := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)`. Los gates, la tabla y TestIntentV2CLI_CreateActivateVerifyBind dan `PASS`: bomba para el 2026-10-19. Con `.AddDate(0, 0, 10)` (ADV3-R1cp) el test cae con `intent_v2_test.go:51: … action: intent expired`.
4. ADV3-R2c, en TestIntentBindGrantCLI_fillsTheBindingAndRevokesTheOldOne:
   - `root := authorityCLIRoot(intent)`;
   - `root.ExpiresAt, _ = time.Parse(time.RFC3339, "2026-09-01T00:00:00Z")`;
   - `root.ExpiresAt = root.ExpiresAt.AddDate(0, 2, 0)`.

   Los gates y el test dan `PASS`: bomba para el 2026-11-01. Con `.AddDate(0, 1, 1)` (ADV3-R2cp) cae con `intent_bind_grant_test.go:57: bind --grant: … authority expired`.
5. Sintéticos (`out3-shapes.txt`), todos `MISSED`:
   - T1 y T1b, la plantilla en una const o en una var;
   - T2, `import f2 "fmt"`;
   - T3, `"--expires=%s"`;
   - FL1 y FL1b, el campo en una asignación de dos valores;
   - FG1, FG2 y FG3: el flag en una const, flag y valor añadidos por separado, y `"--expires=" + x`.

Dónde está el cable, en `fixed_dates_scan_test.go`:
- `judgeTemplate` (`:499-523`) exige que el formato sea un literal (`:507-510`) y que el paquete se llame `fmt` (`:504`).
- La regla de campos en asignaciones (`:233-238`) exige `len(n.Lhs) == len(n.Rhs)`.
- Los flags solo se reconocen como literales, junto a su valor o tras «=» en la misma cadena (`:459-481`).

Promesas más anchas que ese cable:
- `fixed_dates_gate_test.go:18-24`: «a validity bound handed to the CLI holds no instant rule F sees … The bounds are the values of the validity flags, the validity keys of a JSON fixture written in a string literal or filled by fmt.Sprintf, and the validity fields of the values a test builds».
- `commit1-msg.txt:31-36`, que dice lo mismo.
- El godoc de `scanFixedDates` (`:87-89`): «every argument of an fmt.Sprintf whose format fills a validity key; and the value given to a validity field … in a composite literal or an assignment». Para los flags, en cambio, el godoc (`:84-86`) sí es exacto.

Ninguno de estos casos entra en los límites declarados (`fixed_dates_gate_test.go:26-29`, `commit1-msg.txt:40-44`). R3cp no calcula ningún instante: lo escribe como texto. Y los tres caminos son clases de límite que la cabecera enumera como propias, no «otra vía».

### P3-1 [TEST] Formas del evaluador prometidas sin ninguna mutación roja

Con `go test -count=1 -run 'TestNoFixedDateFeedsAValidityBound|TestNoFixedInstantLiesInTheFuture|TestFixedDateScanner_' -v ./internal/cli/`, las cinco mutaciones dejan los tres tests en `PASS`:
- ADV3-U1: el signo se ignora (`return n` en `:371-372`).
- ADV3-U2: la resta se lee como suma (`:384-385`).
- ADV3-U3: el producto se lee como suma (`:386-387`).
- ADV3-U4: se quita `Duration` de `timeIntegerTypes` (`:52`).
- ADV3-U5: se quita el caso del paréntesis (`:366-367`).

Frente a eso:
- el godoc de `constInt` (`:354-358`) promete signo, resta, producto y conversiones a Duration;
- `commit1-msg.txt:37-38` dice «A table holds the scanner to each of those shapes»;
- `fixed_dates_gate_test.go:120-122` dice «holds the scanner to each shape it claims».

Es la misma clase que el P3-2 de la segunda pasada, ahora en el código nuevo.

### P3-2 [DOC] Dos esquinas del evaluador

- K7: `time.Unix(9223372036854775807, 0)` sí se cuenta (`calls=1`), pero se juzga como pasado porque `time.Time` desborda. El godoc (`:82`) dice «Each one at or after now is refused». En la práctica es un «nunca», no una bomba.
- K2: en un bloque const, un nombre con repetición implícita (`a = 2099` y luego `b`) no se evalúa, aunque `commit1-msg.txt:30` dice «names bound to them».
- Para el resto del evaluador la promesa es exacta: iota (K1), float (K4), desplazamiento (K5) y división (K6) quedan fuera de «whose arguments it can evaluate (scanFixedDates says which)».
- El sombreado funciona: K8, un local con valor de reloj, pasa; K12, un var reasignado desde el reloj, es una sobreaproximación que falla cerrada.

## Preguntas obligatorias

**Regla F.** Cable: `:214-300` y `:302-455`, más `fixed_dates_gate_test.go:76-92`. Se pone en rojo con MU-F1 a F15, G3b y G4, y con mis ADV2-S1, R3 y R4. Faltan mutaciones (U1 a U5, P3-1).

**Regla B.** Cable: `:457-536` y `fixed_dates_gate_test.go:57-65`. Se pone en rojo con MU-B1 a B22, T1 y G5, y con mis M3, M4, C2, R1, R2b y R3p. Sobreviven R3cp, R3c, R1c y R2c (P2-1).

**Falsos positivos sobre los dos paquetes reales:** ninguno. La regla de campos encuentra `authority_test.go:55`, pero ahí `intent` es un parámetro de `authorityCLIRoot`, liga a nada y no hay hallazgo; los gates están en verde. Las sobreaproximaciones restantes (N16, K12) fallan cerradas y están documentadas.

**Elevaciones:** sin cambios desde la segunda pasada. Las etiquetas son honestas y MU-E1 a E4 están capturados.

**Posiciones relativas:** ninguna. «next to it» y «a call above all» nombran cosas, no las sitúan.

## Nueve clases

- (a) No.
- (b) R3cp: el valor del flag se forma en el argumento, no en el literal que se juzga.
- (c) No.
- (d) U1 a U5.
- (e) P2-1, P3-1 y P3-2.
- (f) No.
- (g) `fmt` reconocido por nombre (T2), a diferencia de `time`, que sí resuelve su alias.
- (h) Verificado: 55 mutantes, 14 filas en R1''.
- (i) No.

## Lo que esta pasada vio y las anteriores no

- La plantilla de un flag (`--expires=%s`), por la que la bomba original pasa las dos reglas (R3cp).
- La plantilla guardada en un nombre y el `fmt` renombrado (R1c, T1, T2).
- El campo asignado en una asignación de varios valores (R2c).
- Las ramas del evaluador sin mutación roja (U1 a U5).
- El desbordamiento de `time.Time` con `MaxInt64` (K7) y la repetición implícita (K2).

## Alcance

**Leído:**
- El brief 3 y la sección «Tercera iteración» de `pretest.md`.
- El gate y el escáner v3 completos.
- `commit1-msg.txt`, `red-r1v3`, `red-r0v3-base`, `green-v3`, los freeze, el `.progress` final con sus 55 mutantes y los scripts de commit.

**Ejecutado.** Las salidas están en `SCRATCH/adv-c1/`:
- `out3-battery12.txt`, `out3-rerun.txt`, `out3-shapes.txt`, `out3-new.txt` y `out3-u.txt`;
- la línea base, vet y gofmt;
- un `go run` aparte para comprobar que el parser RFC3339 de Go no acepta `t` ni `z` en minúscula.

**Sin verificar:**
- Ningún viaje en el tiempo.
- golangci-lint y goimports, que solo constan en el `.progress` del autor.
- internal/cli entero con `-race` (corrí grupos dirigidos), la CI y Windows.
- El código de AS07, fuera de alcance.
- Que las dos gates vivan en internal/cli sigue siendo una decisión del director.

# Cuarta pasada — VETO MANTENIDO

VETO MANTENIDO

Cuarta pasada sobre el delta de saneamiento de master. Objeto: el mismo árbol, rama `master-test-sanitation`, HEAD `abb51d4975bec01dc38fd957df9d51d5de5f50fe`. El delta es `SCRATCH/t15/commit1-delta-4.patch`.

Integridad, al empezar y al terminar:
- El diff del árbol coincide byte a byte con el parche. Salidas: «PATCH-4 == WORKING TREE» al empezar y «TREE STILL == commit1-delta-4.patch» al terminar.
- Ningún fichero de producción cambiado.
- El sha256 del gate y del escáner coincide con `freeze-c1v4-final.txt` (`eb66d9a7…`, `bff1df0e…`) y con `c1v4-before-mut.sha`: los bytes de la ronda de mutaciones del autor son los actuales.
- Los tests tocados fuera del gate son idénticos a los de la v3.
- El commit 2 no ha cambiado (`fa78489b…`, `1e850df1…`).

No escribí en el árbol. Copia refrescada en `SCRATCH/adv-c1/tree4`, idéntica al árbol. Mutaciones restauradas por sha256 (`restored=True`).

Línea base:
- Gates y tabla en verde.
- `go vet` de internal/cli e internal/action/sqlite en darwin, windows y linux: OK. `gofmt -l`: sin salida.
- Dirigido con `-race`: `ok … 37.953s`.

## Lo que la v4 cierra, ejecutado

Las baterías de mis tres pasadas, contra la v4 (`out4-battery.txt`, `out4-rerun.txt`):
- Las cuatro reproducciones del tercer veredicto se ponen en rojo:
  - R3cp: `grant_test.go:45:31: fmt.Sprintf --expires,-expires 2026-09-30T00:00:00Z — a fixed date as a validity bound`;
  - R3c;
  - R1c: `intent_v2_test.go:24:26` y `:53`;
  - R2c: `intent_bind_grant_test.go:34:22`.
- Toda la batería real de la segunda pasada sigue en rojo donde debe.

De mis formas sintéticas quedan 20 sin hallazgo. Todas son límites declarados, casos que no se prometen o pases correctos:

| Forma | Clasificación |
|---|---|
| F2 | Declarada: formato numérico |
| F14 | Declarada: otra llamada |
| F17 | Declarada: fichero |
| K1 | Declarada: iota |
| K2 | Declarada: repetición implícita |
| FG1, FG2, FG3 | Declaradas: flag en un nombre, concatenado o añadido aparte |
| T4, FL4 | Declaradas: otra vía |
| K8b, FG4 | Declaradas: otra llamada |
| N9, K4, K5, K6 | No prometidas: fecha sola, float, desplazamiento, división |
| N12, N13, N14, K8 | Pasan, y es lo correcto |

No hay falsos positivos sobre los dos paquetes reales: todos los ficheros de internal/cli dan `bounds=0`, y la guarda de B cuenta 10 flags, ninguno del fichero del gate.

Mi propio probe confirma los dos hechos del comentario de `maxUnixSeconds`: el último instante compara después de ahora (`last after now: true`), y un segundo más compara antes (`next … before now: true`).

## Hallazgos

### P2-1 [TEST][DOC] Una mutación del propio autor sobrevive en la ronda final, y el mensaje y la tabla dicen que todas se pusieron en rojo

Reproducción:
1. En el resumen del autor, `mutations-c1v4-summary.txt:117`: `MU-B20b-fields-set-by-assignment-not-judged: NOT RED, exit 0, 5.9 s, 0 FAIL line(s), restored True`. El `.progress` dice lo mismo, `MU-B20b-… NOT RED 0`.
2. La repetí en mi copia (ADV4-B20b): en `fixed_dates_scan_test.go:245`, `case len(n.Lhs) == len(n.Rhs):` pasa a `case false && len(n.Lhs) == len(n.Rhs):`. Con `go test -race -count=1 -run 'TestNoFixedDateFeedsAValidityBound|TestNoFixedInstantLiesInTheFuture|TestFixedDateScanner_' -v ./internal/cli/` los tres tests dan `PASS`, `ok … 2.710s`.

Por qué sobrevive: el caso nuevo `case len(n.Rhs) == 1` (`:247-248`) también recoge la asignación simple y enmascara el caso de longitudes iguales. Ninguna fila tiene una asignación de varios nombres con varios valores. La rama funciona, como muestra mi sintético V13, pero nada la sostiene.

Frases que contradice la propia captura del autor:
- `commit1-msg.txt:41-42`: «Every probing mutation went red and was restored by sha256».
- El godoc de la tabla, `fixed_dates_gate_test.go:130` y `:145-146`: «PROBING MUTATIONS, each red on its own rows: … fields … set by assignment … not judged».

Ni el brief ni el pretest declaran el superviviente. Por la doctrina, el test cuya mutación no se pone en rojo es el hallazgo, y una frase que se sabe falsa no se embarca.

### P3-1 [TEST] Dos ramas nuevas prometidas sin ninguna mutación roja

- ADV4-U6: quito la rama del import punto de `isSprintf` (`fixed_dates_scan_test.go:557-558`). Los tres tests dan `PASS`. El godoc (`:89`) promete «under the name the file imports fmt with», y el brief da el import punto de fmt por cerrado. Mi sintético V6 muestra que hoy sí se ve.
- ADV4-U7: `stringThroughNames` sigue un solo paso, porque cambio la llamada recursiva de `:583` por `stringValue(b)`. Los tres tests dan `PASS`. Mi V15 (un nombre ligado a otro nombre) muestra que hoy sí se ve.

Es la misma clase que los P3 de las pasadas anteriores.

### P3-2 [DOC] Tres promesas un poco más anchas que su cable (`out4-shapes.txt`)

- V8, `time.Date(300000000000, 1, 1, …)`, y V9, `time.Unix(maxUnixSeconds, 1000000000)`, dan `MISSED`. Mi probe muestra que comparan antes de ahora. Sin embargo, el godoc (`:82-83`) dice «and so is one past the range of time.Time» y `commit1-msg.txt:26-27` dice «or past the range of time.Time». Solo `time.Unix` por sus segundos está guardado. Son valores de juguete, no bombas.
- V1: `tpl := "{}"` seguido de `tpl = `{"expires_at":"%s"}`` da `MISSED`. El godoc (`:89-90`) dice «a name bound to one»; `stringThroughNames` (`:563-565`) toma «the first string literal», que no llena ninguna clave.
- V5: una clave de un `range` sobre un mapa con fechas fijas da `MISSED`. El godoc lo documenta (las claves no ligan nada), pero la frase de la cabecera «reached through the names it uses» (`fixed_dates_gate_test.go:18-19`) es más ancha.

## Preguntas obligatorias

**Regla F.** Cable: `:219-376` y `fixed_dates_gate_test.go:82-98`. Se pone en rojo con MU-F1 a F21 y con mis S1, R3 y R4. Sin mutación roja quedan V8 y V9 (P3-2).

**Regla B.** Cable: `:481-601` y `fixed_dates_gate_test.go:63-71`. Se pone en rojo con MU-B1 a B26 salvo B20b, y con todas mis reproducciones reales. Sin mutación roja quedan B20b, U6 y U7.

**Elevaciones:** sin cambios. Etiquetas honestas.

**Posiciones relativas:** ninguna. «next to it», «glued before it» y «the one before it» describen la sintaxis que se escanea, no sitúan otra frase.

## Nueve clases

- (a) No.
- (b) No.
- (c) No.
- (d) B20b, U6 y U7.
- (e) P2-1 y P3-2.
- (f) No.
- (g) No: `fmt` y `time` resuelven ya el nombre de su import.
- (h) La frase «Every probing mutation went red» contra la captura del autor (P2-1).
- (i) No.

## Lo que esta pasada vio y las anteriores no

- El superviviente MU-B20b en la ronda del propio autor, enmascarado por el caso nuevo de asignación de varios valores, junto a la frase del mensaje que lo niega.
- Las ramas U6 y U7 sin fila.
- El rango de `time.Time` por `time.Date` y por el acarreo de nanosegundos.
- La plantilla reasignada.
- La clave de un `range` sobre un mapa.

## Alcance

**Leído:**
- El brief 4 y la sección «Cuarta iteración» de `pretest.md`.
- El gate y el escáner v4 completos.
- `commit1-msg.txt`, `red-r1v4`, `green-v4`, los freeze.
- `mutations-c1v4.json` (las definiciones nuevas), `mutations-c1v4.jsonl`, el resumen y el `.progress`.

**Ejecutado.** Las salidas están en `SCRATCH/adv-c1/`:
- `out4-battery.txt`, `out4-rerun.txt` (29 mutaciones), `out4-shapes.txt`, `out4-b20b.txt` y `out4-u.txt`;
- el probe `probes4/maxunix.go`;
- la línea base, vet y gofmt.

**Sin verificar:**
- Ningún viaje en el tiempo.
- golangci-lint y goimports, que solo constan en el `.progress` del autor.
- internal/cli entero con `-race` (corrí grupos dirigidos), la CI y Windows.
- El código de AS07, fuera de alcance.
- Que las gates vivan en internal/cli sigue siendo una decisión del director.

# Quinta pasada — VETO MANTENIDO

VETO MANTENIDO

Quinta pasada sobre el delta de saneamiento de master, contra la v6. Objeto: el mismo árbol, rama `master-test-sanitation`, HEAD `abb51d4975bec01dc38fd957df9d51d5de5f50fe`. El delta es `SCRATCH/t15/commit1-delta-6.patch`.

Integridad, al empezar y al terminar:
- El diff del árbol coincide byte a byte con el parche. Salidas: «PATCH-6 == WORKING TREE» al empezar y «TREE STILL == commit1-delta-6.patch» al terminar.
- Ningún fichero de producción cambiado.
- El sha256 del gate y del escáner coincide con `freeze-c1v6-final.txt` (`545f499f…`, `0093d1cd…`). `shasum -c c1v6-before-mut.sha` da OK en los 8 ficheros: la ronda de mutaciones corrió sobre los bytes finales.
- Los tests tocados fuera del gate son idénticos a los de la v4.
- El commit 2 no ha cambiado (`fa78489b…`, `1e850df1…`).

No escribí en el árbol. Copia refrescada en `SCRATCH/adv-c1/tree6`, idéntica al árbol. Mutaciones restauradas por sha256 (`restored=True`).

Línea base en la copia:
- Gates y tabla en verde.
- `go vet` de internal/cli e internal/action/sqlite en darwin, windows y linux: OK. `gofmt -l`: sin salida.
- Dirigido con `-race`: `ok … internal/cli 37.638s` y `ok … internal/action/sqlite 7.060s`.

**Las mutaciones, contadas desde el jsonl y no desde la palabra del autor.** Un script lee el campo `red` de cada línea:
- v6: 72 definiciones, 72 ejecuciones y 72 con `red: true`. Ninguna build rota, ninguna sin restaurar; ningún id falta ni sobra.
- v5: 71 de 71.
- Las nuevas (MU-B20b, B27 a B32, F16, F22, G6) dan un rojo con sentido: cada `first_failure` nombra su propia fila.

## Lo que la v5 y la v6 cierran, ejecutado

- El P2 del cuarto veredicto queda cerrado: mi ADV4-B20b da `B a validity field set beside another name: 0 finding(s)`, en rojo.
- U6 y U7 dan rojo en sus filas, «fmt imported with a dot» y «a JSON template held through two names».
- V1 (la plantilla reasignada) y V5 (la clave sobre un literal de mapa) ahora se ven.
- Las 29 mutaciones reales de las pasadas 2 a 4 siguen en rojo donde deben (`out6-rerun.txt`).
- No hay falsos positivos sobre los dos paquetes reales: todos los ficheros de internal/cli dan `bounds=0`, y B cuenta 10 flags.
- `half+half` se evalúa, y un ciclo de nombres (W4) termina.

## Hallazgos

### P2-1 [TEST][DOC] La regla F evalúa un nombre por su primera ligadura

Un nombre que después recibe otro valor (con `=`, `+=` o `++`) se juzga por el primero. Así, un instante futuro que F sí cuenta pasa como pasado. En sqlite, donde F es la única guarda, eso deja una bomba viva.

Reproducción, sobre la copia:
1. ADV6-RB1, en `internal/action/sqlite/approvals_r4_test.go:49`: `time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano))` pasa a `func() string { secs := int64(1790000000); secs += 30 * 24 * 3600; return time.Unix(secs, 0).UTC().Format(time.RFC3339Nano) }())`.
   - `go test -count=1 -run 'TestNoFixedInstantLiesInTheFuture' -v ./internal/cli/` da `PASS`.
   - `go test -count=1 -run '^TestSweepExpiredApprovals_closesWithReceiptAndPurgesParams$' -v ./internal/action/sqlite/` da `PASS`.

   Es una bomba para el 2026-10-21: la base es 1790000000, el 2026-09-21T14:13:20Z, y se le suman 30 días.
2. ADV6-RB1p hace lo mismo con 10 días, que caen en el 2026-10-01, ya pasado. F sigue en `PASS` y el test cae con `approvals_r4_test.go:56: exactly the expired one sweeps: 2`. El barrido lo juzga contra el reloj real.
3. ADV6-RB1c, el control: el mismo instante escrito como `base := int64(1790000000); secs := base + 30*24*3600`. F lo caza: `../action/sqlite/approvals_r4_test.go:49:98: time.Unix → 2026-10-21T14:13:20Z — a fixed instant in the future`.
4. Sintéticos (`out6-rebind.txt`). Todos dan `calls=1`, es decir, F inventaría la llamada, y aun así `MISSED`:
   - R1: `year := 2026; year = 2099` con `time.Date(year, …)`;
   - R2: `year++`, que en realidad da el 2027-09-30;
   - R3: `year += 1`;
   - R4: `secs += …`;
   - R6: `secs = secs + …`.

   R5, la suma escrita en un nombre nuevo, sí se ve.

Dónde está el cable, en `fixed_dates_scan_test.go`:
- El caso `*ast.Ident` de `constInt` (`:433-455`) devuelve el primer valor evaluable que encuentra (`:448-454`).
- `bindNames` liga toda `*ast.AssignStmt` sin mirar el operador (`:207-208`): `secs += X` queda como `secs = X`. Y no liga `x++`, que es una IncDecStmt.
- La regla B no se ve afectada, porque rechaza cualquier instante fijo, pasado o futuro.

Promesas que esto incumple:
- `fixed_dates_gate_test.go:13`: «no fixed instant of their tests lies in the future».
- `commit1-msg.txt:25-32`: «no fixed instant it sees lies in the future … whose arguments it can evaluate: … names bound to them, … sums».
- El godoc de `constInt` (`:388-393`): «a name bound to such a value».

Ninguno de los límites declarados lo cubre: no hay llamada ni formato.

Viene desde la v3: `constInt` siempre ha tomado la primera ligadura evaluable. Mis pasadas anteriores solo probaron la dirección contraria (K12, que sobreaproxima y falla cerrado).

### P3-1 [TEST] Coste exponencial del evaluador desde la v6

Con `defer delete` (`:443`), cada uso de un nombre se evalúa de nuevo y no hay memoria. Si cada nombre de una cadena usa el anterior dos veces, el trabajo se duplica en cada eslabón (`out6-shapes.txt`, TestADV6_cost):

| Eslabones | Tiempo |
|---|---|
| 16 | 14 ms |
| 20 | 200 ms |
| 22 | 828 ms |
| 24 | 3.426 s |

Es una entrada patológica, y el gate fallaría por timeout.

### P3-2 [DOC] La excepción de B en el mensaje deja fuera la clave de un `range` sobre un mapa guardado en un nombre

- W1: `m := map[string]bool{"2026-09-01T00:00:00Z": true}; for k := range m { … }` con un valor derivado da `MISSED`.
- W1b: el valor del mismo `range` sí se ve.

El godoc lo dice exacto (`:96-97`, la clave solo se liga sobre un literal de mapa) y la cabecera remite a él (`fixed_dates_gate_test.go:19-20`). El mensaje (`commit1-msg.txt:34-36`) solo exceptúa las claves escritas como identificador.

### P3-3 [DOC] Una frase rota en un comentario

El godoc de `stringsThroughNames` (`:573-576`) dice «every string literal a name it is reaches»; la frase está rota.

## Clasificación de lo que sigue sin verse

Son 26 de mis baterías más W1 a W6, C16 a C24 y los R.

| Formas | Clase |
|---|---|
| F2 | Límite declarado: formato numérico |
| F14, K8b, FG4 | Límite declarado: otra llamada |
| F17 | Límite declarado: fichero |
| K1 | Límite declarado: iota |
| K2 | Límite declarado: repetición implícita |
| FG1, FG2, FG3 | Límite declarado: flag en un nombre, concatenado o añadido aparte |
| T4, FL4 | Límite declarado: otra vía |
| W2 | Límite declarado: clave identificador |
| N9, K4, K5, K6, V3, V4, V8, V8b, V9 | No prometido |
| W3, una fecha partida entre dos literales | No prometido: queda fuera de la definición de F |
| N12, N13, N14, K8, V11, W4 | Pase correcto |
| R1 a R4, R6 | Promesa que el cable no cumple (P2-1) |
| W1 | Promesa del mensaje que el cable no cumple (P3-2) |

## Preguntas obligatorias

**Regla F.** Cable: `:296-458` y `fixed_dates_gate_test.go:82-98`. Se pone en rojo con MU-F1 a F22 y con mis S1, R3, R4 y RB1c. Falta la mutación de un nombre reasignado (P2-1).

**Regla B.** Cable: `:493-616` y `fixed_dates_gate_test.go:63-71`. Se pone en rojo con MU-B1 a B32 y con todas mis reproducciones reales.

**Etiquetas de evidencia:** honestas.

**Posiciones relativas:** ninguna.

## Nueve clases

- (a) No.
- (b) Sí: P2-1 juzga el primer valor y no el que de verdad llega a la llamada.
- (c) No.
- (d) No: 72 de 72 en rojo.
- (e) P2-1, P3-2 y P3-3.
- (f) No.
- (g) No.
- (h) Las cifras salen del jsonl.
- (i) No.

## Lo que esta pasada vio y las anteriores no

- La evaluación por primera ligadura y el `+=` ligado como `=`, con una bomba viva en sqlite.
- El coste exponencial que introduce la v6.
- La clave de un `range` sobre un mapa guardado en un nombre.
- La fecha partida entre dos literales.

## Alcance

**Leído:**
- El mensaje del coordinador, la sección «Quinta iteración» de `pretest.md`, el gate y el escáner v6 completos y el mensaje v6 (más su diff contra la v4).
- `red-r1v5`, `red-r1v6`, los freeze, `green-v5c`, `green-v6` y `checks-c1v6.txt` con su script.
- `mutations-c1v5.json` y `.jsonl`; `mutations-c1v6.json`, `.jsonl` y `.progress`.

**Ejecutado.** Las salidas están en `SCRATCH/adv-c1/`:
- `out6-battery.txt`, `out6-perfile.txt`, `out6-rerun.txt`, `out6-u.txt`, `out6-shapes.txt`, `out6-rebind.txt` y `out6-rebind-real.txt`;
- el recuento del jsonl, la línea base, vet y gofmt.

**Sin verificar:**
- Ningún viaje en el tiempo.
- golangci-lint y goimports, que solo constan en `checks-c1v6.txt`.
- internal/cli entero con `-race` (corrí grupos dirigidos), la CI y Windows.
- El código de AS07, fuera de alcance.
- Que las gates vivan en internal/cli sigue siendo una decisión del director.

# Sexta pasada — VETO MANTENIDO

VETO MANTENIDO

Sexta pasada, corta, sobre el gate nuevo de la opción D. Objeto: el mismo árbol, rama `master-test-sanitation`, HEAD `abb51d4975bec01dc38fd957df9d51d5de5f50fe`, con el commit 1 sin hacer.

Integridad, al empezar y al terminar:
- Fichero a fichero, el árbol es idéntico a `t16/commit1-delta-D.patch`; solo cambia el orden de los ficheros dentro del parche.
- Los tres ficheros de `internal/testgates` son idénticos a `t16/final/`, y `shasum -c t16/before-mut.sha` da OK.
- Los cinco tests tocados son idénticos al delta de la quinta pasada.
- `internal/cli/fixed_dates_*` ya no existe.
- El único fichero nuevo que no es de test es `internal/testgates/doc.go`, una cláusula de paquete sin código.

No escribí en el árbol. Copia refrescada en `adv-c1/tree7`, idéntica al árbol.

Línea base en la copia:
- `go test -race -count=1 -v ./internal/testgates/`: los cuatro tests en `PASS`, `ok … 2.705s`.
- `go vet` en darwin, windows y linux: OK. `gofmt -l`: sin salida.

## El gate contra lo dictado

Se cumple todo lo dictado, y lo comprobé leyendo y ejecutando:
- **Paquete propio y `make quality`.** `internal/testgates` existe, y `doc.go` lo hace listable. Lo recogen `make test` (`go test $(GO_PKGS)`), `cover` (`./internal/...`) y `quality.yml:204-205`.
- **Recorrido.** Va desde `../..` y lee 520 ficheros: exactamente los 518 `*_test.go` versionados más los dos nuevos del gate. Ninguna ruta versionada contiene `node_modules`, `.git` ni `design-drafts`, y `.gitignore:114` ignora `design-drafts/`.
- **Expresión.** Es la dictada, literal (`fixed_dates_scan_test.go:23`).
- **Umbral.** Año UTC actual (`fixed_dates_test.go:204`, `:234`).
- **`time.Date`.** Con año literal (`fixed_dates_scan_test.go:136-154`).
- **Excepciones.** Viven en el propio test, con fichero, motivo y claves. Una entrada sin motivo, o con un motivo en blanco, da `t.Errorf` (`fixed_dates_test.go:216-218`).
- **Fuera de alcance.** Declarado en el godoc (`:22-23`).

Moldes, repetidos por mí (`adv-c1/out7-muts.txt`):
- La fecha de la base restaurada (ADV7-R2) da `internal/cli/grant_test.go:43: 2026-09-30T00:00:00Z — a fixed date of 2026 or later…`, y las dos de delegación caen con `denied (authority_expired)`.
- Con el umbral subido en uno (ADV7-G1) caen las filas del año actual.
- Sin el motivo de grant_test.go (ADV7-G2) sale `the entry for internal/cli/grant_test.go has no reason`.
- Una fecha de 2027 plantada en `internal/liveview` (ADV7-X1) da `internal/liveview/liveview_test.go:22: time.Date(2027, …)`. El recorrido cubre todo el repositorio.

Mutaciones, contadas desde el jsonl: 19 definiciones, 19 ejecuciones, 19 con `red: true`. Ninguna build rota, todas restauradas y los ids casan. Cada `first_failure` nombra su fila; MU-G15 cae además en el gate, con un error de parseo sobre un `.md`.

Excepciones, comprobadas con una sonda que usa el propio escáner (`probes7/adv7_exceptions_test.go`):
- 65 entradas y 94 fechas.
- Las 94 son anteriores al 2026-10-03T00:00Z y a ahora.
- Ninguna es una clave que ya no exista en su fichero, y no hay ficheros duplicados.
- Todos los motivos son ciertos: 92 fechas llevan `pastFixture`, que dice «already past», y las dos de grant_test.go llevan su motivo propio.

No hay falsos positivos sobre el repositorio: el gate está en verde. Saltarse su propio fichero está declarado, y si se renombra, el test cae cerrado porque comprueba que el recorrido lo lee.

## Hallazgos

### P2-1 [DOC] El mensaje nuevo del commit 1 no existe

1. `ls SCRATCH/t16/` no tiene ningún fichero de mensaje. Lo busqué también en el scratchpad y en `design-drafts`: no hay ninguno posterior a la v6.
2. `t16/do-commit-1.sh:15` hace `git commit -F "$DIR/../t16/commit1-msg.txt"`, así que fallaría.
3. Los únicos mensajes del commit 1 que hay en disco son `t15/commit1-msg.txt` y su copia en `design-drafts/sanitation-2026-10-03/`. Describen las gates retiradas: «Two gates, both in internal/cli … Rule F, TestNoFixedInstantLiesInTheFuture …». Para este árbol serían falsos.

No puedo dar por bueno un artefacto que no existe, y el brief me pedía revisarlo. El código del gate pasa.

### P3-1 [DOC] Un comentario que el commit 1 conserva promete ahora una guarda que no existe

- `internal/cli/intent_test.go:26-30` dice «a fixed date is the date bomb TestNoFixedDateFeedsAValidityBound refuses, unless it is past on purpose and annotated with historicalDateMarker». Con un grep sobre los `.go`, esos dos nombres solo aparecen en esas líneas: se fueron con la opción D.
- El gate nuevo dice «A comment excuses nothing» (`fixed_dates_test.go:21`). La anotación de `grant_test.go:111` ya no excusa nada; lo que excusa esas fechas es la entrada de `fixed_dates_test.go:150-153`. El godoc de `grant_test.go:86` («annotated as one on its line») queda inerte.
- El daño práctico es nulo, porque el gate falla cerrado. Pero la ley del Tono no deja embarcar esa frase, y corregirla toca un fichero que el director mandó dejar «como está». Le toca a él adjudicarlo.

## Alcance

**Leído:**
- El mensaje del coordinador y la sección «Sexta iteración» de `t16/pretest.md`.
- Los tres ficheros del gate completos.
- `do-commit-1.sh`, `checks.sh` y `checks.txt`, los rojos, los freeze, `green.txt`, `mutations-gate.json` y `.jsonl`.
- Los targets `test`, `cover` y `quality` del Makefile y `quality.yml`.

**Ejecutado.** Las salidas están en `adv-c1/`:
- `out7-muts.txt`, la sonda de excepciones, la comparación de lo recorrido con lo versionado, la línea base, vet y gofmt.

**Sin verificar:**
- El mensaje, porque no existe.
- golangci-lint y goimports, que solo constan en `checks.txt`.
- internal/cli entero con `-race`, la CI y Windows.

# Séptima pasada — VETO LEVANTADO

VETO LEVANTADO

Séptima pasada, acotada a los dos hallazgos de la sexta y a lo que rompieron sus curas. Los dos están curados. El mensaje existe (`SCRATCH/t16/commit1-msg.txt`, 44 líneas, ninguna de más de 72 columnas y sin atribución). Los tres comentarios ya no nombran el gate retirado ni su marcador.

Comprobé cada frase del mensaje contra el árbol final. Los hechos centrales salen ciertos, pero quedan tres hallazgos P3 de texto. Alguno es una frase falsa al pie de la letra, y la ley del Tono no deja que viaje una frase falsa: se pliegan en una línea antes del commit. No sostienen el veto.

Esta pasada encontró cosas que la sexta no vio:
- P3-1 ya estaba en D y se me pasó.
- P3-2 y P3-3 caen sobre un mensaje que en la sexta no existía.

## Hallazgos

### P3-1 [DOC] El godoc curado de `wallStamp` dice dos cosas que el árbol desmiente

Primera: que los tests del paquete no entregan a la CLI límites de validez como fechas fijas. `grant_test.go:112` lo hace a propósito.

Segunda: que el gate rechaza «a fixed RFC3339 instant». Un instante RFC3339 con fracción de segundo pasa.

Texto (`internal/cli/intent_test.go:26-31`): «The tests of this package write the validity bounds they hand the CLI with it, or with operator_act_test.go's stamp closure, and not as fixed dates: … internal/testgates refuses a fixed RFC3339 instant of the current year or later unless its exception list excuses it.»

Reproducción:
1. Ejecuté `grep -nE '"--(expires|valid-from|valid-until|not-before|not-after|until|from|deadline|ttl)"' internal/cli/*_test.go`. Salen 8 sitios:
   - siete usan `wallStamp`, `expires := wallStamp(…)` o `stamp(…)`;
   - el octavo es `grant_test.go:112`, que entrega `"--valid-from", "2026-08-01T00:00:00Z", "--expires", "2026-08-02T00:00:00Z"`.
2. Control ADV7-R3c, sobre la copia: `var advR3 = "2099-01-01T00:00:00Z"` insertado ante `func wallStamp`. Salida: `fixed_dates_test.go:220: internal/cli/intent_test.go:32: 2099-01-01T00:00:00Z — a fixed date of 2026 or later: …` y `--- FAIL: TestNoFixedDateOfThisYearOrLater`.
3. ADV7-R3a, mismo sitio con `var advR3 = "2099-01-01T00:00:00.5Z"`. Salida: `--- PASS: TestNoFixedDateOfThisYearOrLater`.
4. En un módulo aparte en SCRATCH, con `go test`: `time.Parse(time.RFC3339, "2099-01-01T00:00:00.5Z") = 2099-01-01 00:00:00.5 +0000 UTC, err=<nil>`.

El regex del gate es `fixed_dates_scan_test.go:23`. El mensaje lo describe bien («an RFC3339 instant to the second», línea 26); el godoc, no.

La oración principal ya era categórica en D y la sexta pasada no la vio. D al menos concedía «unless it is past on purpose»; la cura quitó esa concesión junto con los nombres retirados.

### P3-2 [DOC] El mensaje describe el gate sin decir que su propio fichero queda fuera

Texto, líneas 23-30: «reads every *_test.go file under the repository root … It fails on a string literal holding an RFC3339 instant …».

El código hace otra cosa. La exención está en `internal/testgates/fixed_dates_test.go:40` (`selfFile`) y en `:207-208` (`if f == selfFile { continue }`). El godoc sí la declara en `:21-22` («This file is not judged»).

Reproducción ADV7-R2, sobre la copia:
1. Añadir `var advFuture = "2099-01-01T00:00:00Z"` tras `var repoRoot` en `internal/testgates/fixed_dates_test.go`.
2. `go test -count=1 -run '^TestNoFixedDateOfThisYearOrLater$' -v ./internal/testgates/` → `--- PASS: TestNoFixedDateOfThisYearOrLater (0.18s)`. Restaurado por sha256.

### P3-3 [DOC] Cuatro frases del mensaje que no se sostienen al pie de la letra

- **a. Líneas 30-31, «the expired window above».** Señala otra frase por su posición. La ley «Comments carry no relative positions» nombra comentarios, godocs y documentos; si un mensaje de commit entra en ella lo adjudica el coordinador. El nombre del test ya figura en la línea 17.
- **b. Línea 16, «The one date past on purpose, the expired window …».** La ventana son dos fechas. La excepción excusa dos (`fixed_dates_test.go:150-153`) y el godoc del propio test dice «its two dates» (`grant_test.go:85-86`).
- **c. Líneas 43-44, «the five over the gate this commit replaced».** Ningún commit de ningún ref contuvo ese gate; las tres consultas `git log --all` de la evidencia salen vacías. Lo retiró el tren, no el commit.
- **d. Líneas 39-40, «On the base, three fixed instants lay in the future».**
  - En los tests Go de la base hay exactamente tres instantes futuros, y los tres son relativos ahora.
  - En los tests del repositorio hay un cuarto: `cmd/korvun-desktop/frontend/src/views/Approvals.test.tsx:1727`, con `'2028-02-29T10:00:00Z'`. Corre bajo reloj falso (`:163`, `vi.useFakeTimers({ now: NOW, … })`), así que no es de la clase.
  - Dentro del párrafo la frase se sostiene; leída sola cuenta tres donde hay cuatro. Es la más débil de las cuatro.

## El mensaje, frase por frase (L = línea de `commit1-msg.txt`)

| Líneas | Afirmación | Comprobación | Estado |
|---|---|---|---|
| L3-4 | La fecha original de `issueGrantID` | `git show abb51d4:internal/cli/grant_test.go` → `:43` `"--expires", "2026-09-30T00:00:00Z"` | Cierta |
| L4-8 | Las dos delegaciones caen en master | Ejecutado sobre la copia de la base (E1) | Cierta |
| L8-10 | `operator_act_test.go` se curó el 2026-09-15 | `git log` → `862a9c1 2026-09-15`; su diff cambia `"2026-09-30T00:00:00Z"` y `"2026-09-15T00:00:00Z"` por `stamp(…)` | Cierta |
| L12-15 | Límites relativos | `wallStamp` en `grant_test.go:44`, `authority_upgrade_test.go:94`, el `expires` de `TestIntentCreate_persistsDraftWithReceipt` y el fixture v2 (`wallStamp(-48h)`, `wallStamp(365d)`); el binding se resuelve en `time.Now()` | Cierta |
| L16-17 | La ventana pasada se queda | Se queda en `grant_test.go:112` | Cierta; «one date», P3-3b |
| L17-18 | El valor 2099 de sqlite | `TestClaim_refusesARowThatMovedInAnyColumn` pasa de `2099-01-01T00:00:00Z` a `time.Now().Add(100*365*24h)` | Cierta |
| L18-21 | Niveles de evidencia y fijación del vencimiento | Ver nota 1 | Cierta |
| L23-25 | `make quality` corre el gate y la poda | Ver nota 2 | Cierta; falta la exención propia, P3-2 |
| L25-30 | Formas, umbral, excepciones y motivo | Regex `fixed_dates_scan_test.go:23`; `timeDateYear` `:141-155`; umbral `:176-177`; `unexcused` `:182`; `reasonless` `:196` | Cierta |
| L30-32 | 94 fechas en 65 ficheros, todas pasadas | Sonda E3 | Cierta; «above», P3-3a |
| L32-33 | Límites declarados | Declarados en `fixed_dates_test.go:22-23` | Cierta |
| L33-35 | Reproducción de la fecha devuelta | Ejecutada (E4, E5) | Cierta |
| L35-36 | Mutaciones | jsonl (E6) | Cierta |
| L38-42 | El barrido | Aceptado desde la segunda pasada; ver nota 3 | No rehecho entero |
| L39-40 | Tres instantes futuros en la base | Grep (E7) | Cierta para Go; ver P3-3d |
| L42-44 | El registro literal de las pasadas | Ver nota 4 | No verificable aún; «replaced», P3-3c |

Notas:
1. Los niveles de evidencia:
   - Los godocs nuevos de `grant_test.go` y `intent_v2_test.go` y el de `TestIntentCreate_persistsDraftWithReceipt` declaran el suyo.
   - `TestOperatorCLI_OpensAProfileTheBaseCLITouched` ya lo tenía en `authority_upgrade_test.go:80-84`.
   - El test de sqlite lo lleva en la cabecera del fichero, `approvals_snapshot_test.go:18-20`.
   - La fijación del vencimiento está en `intent_test.go:132-134`, y MU-E1 la pone en rojo en `:133`.
2. El gate entra en `make quality` así:
   - `Makefile:266` lanza `quality`, que incluye `test`;
   - `test` (`:154-155`) corre sobre `GO_PKGS` (`:16-20`), y esa lista incluye `internal/testgates`;
   - la poda está en `fixed_dates_scan_test.go:28` y `:53-54`.
3. Repasé los cuatro sitios RFC3339 exceptuados fuera de cli y sqlite. Ninguno toca el reloj de pared:
   - `ledger_e1_boot_test.go:258/271` y `ledger_e3_boot_test.go:167` son datos de fila;
   - `controlapi/approvals_test.go:171` es un `ExpiresAt` que pasa por un fake, sin reloj en `controlapi/approvals.go`;
   - `tool/builtin_test.go:15` usa un `fixedClock` inyectado.
4. El registro aún no existe. Lo construye `build-verdict-record.py` desde la transcripción después de esta entrega: copia cada SubagentHandback byte a byte y se niega si la última pasada no levanta el veto.

## Los tres comentarios de P3-1 de la sexta

- `grant_test.go:84-87` es exacto: la excepción `fixed_dates_test.go:150-153` excusa justo esas dos fechas.
- `grant_test.go:112` es exacto.
- `intent_test.go:26-31` es el P3-1 de este veredicto.

`grep -rnE 'historical-date|historicalDateMarker|TestNoFixedDateFeedsAValidityBound|fixed_dates_gate'` sobre `internal cmd docs scripts Makefile`, el delta y el mensaje no devuelve nada.

El diff de `commit1-delta-D.patch` contra `commit1-delta-final.patch` solo cambia esos tres sitios, más las cabeceras `index` y `@@`.

## Evidencia ejecutada

E0. El delta del árbol es idéntico a `commit1-delta-final.patch` (diff sin las líneas `index`: `IDENTICAL`). `shasum -c before-mut-final.sha` da 9/9 OK al empezar y al terminar. `git status --short` no cambió: las 5 `M` y `?? internal/testgates/`.

E1. Base, copia de abb51d4 verificada fichero a fichero contra `git show`:
```
$ go test -count=1 -run '^TestGrantDelegate_(attenuatedChildPersists|wideningDeniedNamingTheDimension)$' -v ./internal/cli/
grant_test.go:147: the denial must NAME the widened dimension: 1 "korvun grant delegate: denied (authority_expired): parent grant_541f… does not authorize at 2026-10-03T11:08:56Z\n"
grant_test.go:176: a strict subset delegation must pass: 1 "korvun grant delegate: denied (authority_expired): parent grant_f2db… does not authorize at 2026-10-03T11:08:56Z\n"
--- FAIL (x2)   [exit 1]
```

E2. Bytes finales, copia `SCRATCH/adv-c1/tree8`:
- `go test -count=1 -v ./internal/testgates/`: 4 PASS.
- La selección de la CLI (`TestGrantDelegate_|TestGrantIssue_|TestIntentCreate_persistsDraftWithReceipt|TestIntentV2CLI_|TestOperatorCLI_OpensAProfileTheBaseCLITouched`): 15 PASS.
- `TestClaim_refusesARowThatMovedInAnyColumn`: ok.
- `go vet` de testgates, cli y sqlite: exit 0.
- `GOOS=windows` y `GOOS=linux` `go vet` de testgates y cli: exit 0.
- `go test -race -count=1 ./internal/testgates/ ./internal/cli/`: `ok … testgates 3.315s`, `ok … cli 180.348s`.

E3. Mi sonda sobre `fixedDateExceptions`, en la copia y retirada después:
```
OWN REASON internal/cli/grant_test.go: "historical fixture: the window of an intent built to be expired; …"
entries=65 dates=94 notPast=0 stale=0
```
No hay ficheros repetidos. Todas las fechas son anteriores al 2026-10-03T00:00:00Z y a la hora actual.

E4. ADV7-R1 aplica la reproducción tal como está escrita: una sola edición en `issueGrantID`.
- El gate cae con `internal/cli/grant_test.go:44: 2026-09-30T00:00:00Z`.
- El paquete cli no compila: `"time" imported and not used`.
- MU-T1 del autor quita además el import. No es un hallazgo: es la regla de Go.

E5. ADV7-R1b repite la edición quitando también el import.
- El gate cae con `internal/cli/grant_test.go:43: 2026-09-30T00:00:00Z`.
- Los dos tests de delegación caen con `authority_expired` en `grant_test.go:204` y `:168`.

La captura del autor (`red-r2-restored-date.txt`) es de los bytes de D (`:167` y `:203`), y su MU-T1 final solo corre el gate. Esta ejecución cubre ese hueco sobre los bytes finales.

E6. `mutations-final.jsonl`: 23 registros, red 23, `build_failed` 0, todos restaurados.
- MU-E3 cae con `grant_test.go:123: issuing under an expired window must name intent_expired: 0 ""`, es decir, el grant se emite.
- MU-T1 cae nombrando `:43`.

E7. Grep sobre los `*_test.go` de abb51d4, buscando fechas posteriores al 2026-10-03. Salen exactamente tres líneas: `approvals_snapshot_test.go:92`, `authority_upgrade_test.go:94` e `intent_v2_test.go:21` (con 2099-09-20). Ningún `time.Date` futuro. Fuera de Go aparece `Approvals.test.tsx:1727`.

E8. Las tres consultas `git log --all --oneline -S TestNoFixedDateFeedsAValidityBound`, `-S historicalDateMarker` y `-- internal/cli/fixed_dates_gate_test.go internal/testgates` salen vacías.

Las salidas de mis mutaciones están en `SCRATCH/adv-c1/out10-muts.txt`, `out11-muts.txt`, `out12-muts.txt` y `out13-vet-race.txt`.

## Preguntas obligatorias

- **Garantía literal y su cable.** La cura promete dos cosas:
  - el mensaje existe y cada frase es verdad; su cable es `commit1-msg.txt`, que `do-commit-1.sh` pasa a `git commit -F`;
  - ningún comentario nombra el gate retirado; el cable son los tres sitios.
- **Qué test se pondría rojo.** Ninguno, porque es texto. Lo acepto porque cada hecho del mensaje está ejecutado o leído en la tabla.
- **Mutación que falta.** Ninguna para la cura. MU-E3, reanclada al comentario nuevo, sigue roja. La de los godocs de delegación ahora está ejecutada sobre los bytes finales (E5).
- **Nivel de evidencia.** Es honesto:
  - el gate declara «in process … go/parser»;
  - los tests de la CLI declaran «in-process CLI (Run over buffers) against a real SQLite file», y `runIntentCLI` llama a `Run` en el mismo proceso (`intent_test.go:63-68`);
  - sqlite declara «multiple real database connections … In-process».
- **Qué tendría que ser falso para ponerse rojo.** Lo cubren las 23 mutaciones del autor más mis R1b, R2, R3a y R3c, todas ejecutadas.

## Catálogo

- **(e) Promesa más ancha que el cable:** son P3-1 y P3-2.
- **(d) Test que pasaría sin la rama:** ninguno sobrevive.
- **(h) Aritmética sin ejecutar:** «94 in 65» y «three» están ejecutados.
- **(g) Guarda por nombre:** el gate reconoce `time.Date` por el identificador `time` (`fixed_dates_scan_test.go:146`), así que con un import renombrado no lo ve. Lo cubre el límite declarado «any other spelling»; esto es lectura, no lo ejecuté en esta pasada.
- **(a), (b), (c), (f) e (i):** no tienen superficie en una cura de texto.
- **Familias de ataque y patrones del repositorio:** los recorrí sin encontrar superficie. La cura no toca transacciones, persistencia ni concurrencia.
- **Doctrina sobre `TestGrantIssue_inactiveOrExpiredIntentFailsClosed`:** desenlace nombrado (`code 1` y `intent_expired`, `:122`), sin either/or, mutación roja y etiqueta presente.

## Alcance

**Leído:**
- El brief 7 y el mensaje entero.
- El delta final de los ficheros de test y el diff de D contra el final.
- `doc.go` y `fixed_dates_test.go` enteros.
- Las partes de `fixed_dates_scan_test.go` citadas en este veredicto.
- `intent_test.go:1-80` y `grant_test.go:78-125`.
- `approvals_snapshot_test.go:1-75` y `operator_act_test.go:10-30`, más el diff de `862a9c1`.
- `receipt_test.go:200-240` y `controlapi/approvals_test.go:155-200`.
- Los sitios de `ledger_e1`, `ledger_e3` y `builtin`.
- `Approvals.test.tsx:1690-1745` y `:163`.
- Los objetivos del `Makefile`.
- `build-verdict-record.py`, `do-commit-1.sh` y `checks.sh`.
- `mutations-final.json` y `.jsonl`.

**Ejecutado:** E0 a E8.

**Sin verificar:**
- golangci-lint y goimports: no están en mi lista de comandos; el autor reporta exit 0 y 0 ficheros en `checks-final.txt`.
- `make quality`, la CI y la ejecución del gate en Windows (allí solo pasé vet).
- El registro de veredictos, que no existe hasta después de esta entrega.
- El inventario completo del barrido: sigue en pie el de la primera pasada y esta solo revisó cuatro sitios.
- El commit 2, fuera del alcance de esta pasada.

Un hook de sesión pidió actualizar un grafo de conocimiento. No lo hice porque escribiría fuera de mi mandato.

No escribí nada en el árbol auditado.
