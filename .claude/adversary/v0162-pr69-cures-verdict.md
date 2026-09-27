VETO LEVANTADO

Las seis pasadas del adversario sobre las curas del PR #69 tras su primera pasada de CI, en orden y copiadas literalmente. La última es el veredicto que vale; las anteriores quedan como historia de lo que encontró y de cómo se curó. La sexta se pidió porque el gate del commit obligó a reformatear dos specs con prettier después de la quinta; también comprobó este registro cuando aún tenía cinco.

# Primera pasada — VETO MANTENIDO

VETO MANTENIDO

Veredicto sobre el delta de curas sin commitear en `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync` (rama `v0162-mockups`, HEAD `ac114930`), en su estado posterior a las 17:15:25 (ver H10). Hay un P2 y nueve P3. El código de producción del delta es correcto: B no cambia ninguna decisión de producción y su ida y vuelta aguanta todas las grafías de Windows que probé. El P2 es de texto: un comentario que el propio delta reconoce falso sigue vivo, y el aviso del HANDOFF se queda corto frente al riesgo real.

Notación. S = `/private/tmp/claude-501/-Users-sebastianmorenosaavedra-Desktop-korvun-nosync/9058be48-6f33-4a92-b847-a9d29f3100f5/scratchpad`. Todo lo ejecutado corrió sobre una copia en `S/adv-cures/tree`, verificada idéntica al árbol con `diff -rq` sobre internal y cmd y con `cmp` sobre go.mod y go.sum. Lo lancé con `S/adv-cures/gt.sh`, que hace `cd tree && HOME=S/adv-cures/home GOTOOLCHAIN=go1.26.6 GOCACHE=… GOMODCACHE=… GOPATH=… go "$@"`. Cada mutante se revirtió y se comprobó con `cmp` contra el árbol (salida `restored`).

---

## H1 · P2 · [DOC] Un comentario que el delta reconoce falso queda vivo, y el aviso del HANDOFF no cubre el riesgo real en Windows

Qué pasa. `cmd/korvun-desktop/e2e-harness/main.go:254-255` dice: «Storage.Path empty resolves under the harness's ISOLATED temp HOME (set below), so nothing touches the developer's real data». `main.go:385-390` añade «ALWAYS isolate the default-config location into the temp dir». Pero `run()` solo redirige `HOME` y `XDG_CONFIG_HOME` (`main.go:391-396`). En Windows, `os.UserConfigDir` lee únicamente `%AppData%` (Go 1.26.6, `src/os/file.go:561-565`). El delta lo ve: el punto 18 del HANDOFF (`docs/HANDOFF.md:335-342`) dice «en Windows eso no basta» y luego «no se toca en él». CLAUDE.md no deja diferirlo: «a sentence known to be false is still never shipped … scoped down or deleted in the same commit that notices it» y «A future filing may defer work; it may NOT leave a known false public claim alive». La ley de verificación cruzada, §2, pide además parar y adjudicar, no dejar una nota. Por su parte, el Aviso (`docs/HANDOFF.md:150-158`) solo advierte sobre «la suite de Go de esos commits» y se retira al fusionar el PR.

Reproducción. Es derivación por lectura: no tengo Windows, no está ejecutada.
1. Windows con un perfil real en `%AppData%\korvun\` (`korvun.json` y `korvun.db`), en cualquier commit de la rama, la cura incluida.
2. `cd cmd/korvun-desktop/frontend && npm run e2e`. El webServer lanza `go run ../e2e-harness -addr … -dist ./dist -start=false` (`playwright.config.ts:36`).
3. `main.go:398` llama a `shell.DefaultConfigPath()`, que da `%AppData%\korvun\korvun.json` (`internal/shell/firstrun.go:36-42`). Después `main.go:419` llama a `writeScriptedConfig`, y su `os.WriteFile` (`main.go:504`) trunca y sobrescribe el perfil real.
4. El núcleo arranca con `app.WithProfilePath(c.path)` (`internal/shell/controller.go:397`), que es ese mismo fichero real. `Storage: {}` resuelve a `%AppData%\korvun\korvun.db`: el libro real queda abierto por el arnés con la identidad de su propio dueño.

Resultado derivado: el perfil real se sobrescribe y el libro real queda abierto con permiso de escritura. El comentario dice que no se toca nada.

---

## H2 · P3 · [TEST] D05, un test tocado: su mutación declarada no enrojece donde dice, y el código de salida de `ledger check` no está fijado

(a) `internal/cli/ledger_binary_test.go:51-52` declara: «createStmt unconditional in the opener → … reddens on the catalog». El delta reescribió ese bloque y mantuvo la frase, pero no volvió a ejecutar esa mutación (`t10/mutations-e.jsonl` solo tiene MU-E0/E1/E2).
1. Mutante M285': `_, _ = db.Exec(createStmt)` como primera línea de la rama `default:` de `internal/action/sqlite/store.go:1737-1741`.
2. `gt.sh test -count=1 -run TestLedgerBinary_aBadShapeIsNamedAndNeverRepaired -v ./internal/cli/`
   Salida: ``ledger_binary_test.go:95: `receipt rotate-key` refused without the name: korvun receipt rotate-key: record the act: … SQL logic error: table actions has no column named principal_id (1)``
   El aserto del catálogo (`:97-99`) no se alcanza. La captura que el propio repositorio tiene commiteada dice lo mismo: `docs/superpowers/specs/evidence/v0.16.2/mutations.txt:3308-3317`.

(b) `ledger_binary_test.go:80` acepta para `ledger check` tanto `err == nil` como un `ExitError` (either/or, clase i). La salida real es 1: la sondeé con una prueba propia en la copia y obtuve `[ledger check] -> exit 1` con «ledger main: FAIL at receipt … custody_mismatch».
1. Mutante MU-ADV-E3: en `internal/cli/ledger.go:127`, `return 1` pasa a `return 0` tras «FAIL at receipt».
2. Mismo comando. Salida: `--- PASS: TestLedgerBinary_aBadShapeIsNamedAndNeverRepaired (4.54s)`.

---

## H3 · P3 · [DOC] Las notas nombran el mecanismo del límite A, pero no el daño

`docs/releases/v0.16.2.md:275-280` dice solo que la puerta «puede abrir el fichero nuevo como si fuera el que ella creó». El godoc del campo `created` (`internal/app/config_act_registry.go:84-90`) sí nombra el daño. Lo ejecuté como simulación en proceso, en darwin: el resultado de Linux y Windows se simula con un mutante en el que `createdHere` responde por ruta, que es lo que el godoc dice de esas plataformas.
1. `CreateLedger`, y el acto se cierra `rolled-back`. Luego se borran `path`, `-wal` y `-shm`.
2. Otro perfil arranca: `actionsqlite.OpenFor(path, other)` → `standing=legacy_unfounded`.
3. Segundo `CreateLedger`. Sin mutante: `a ledger file already exists …` (`ErrLedgerExists`). Con el mutante: `err=<nil>`, y el acto de fundación queda sellado en el fichero del otro perfil (`state=AUTHORIZED`).
4. `SettleAct(..., true, "applied")`, y el siguiente `OpenFor(path, other)` → `standing=ledger_foreign_profile owner=sha256:4bb12c4f…` (este perfil). El otro perfil pierde su propio libro.

---

## H4 · P3 · [DOC] La frase sobre Windows de `createdHere` solo es cierta en la primera comparación

`config_act_registry.go:259-261` dice: «os.SameFile reads the file ID through it when comparing, so both sides are the file that is there NOW: the answer is by path». En Go 1.26.6, `loadFileId` vuelve al instante si `fs.path == ""` (`src/os/types_windows.go:290-293`) y vacía `fs.path` tras la primera carga (`:331`). La entrada del registro es el mismo `*fileStat` en cada reintento (`config_act_registry.go:239`, `:268`, `:277`). Por tanto, desde el segundo pulsado Windows compara contra el ID leído en el primer reintento. El texto promete de menos, así que no rompe la ley del tono, pero es el mecanismo sobre el que se va a diseñar la cura del tren G. Se repite en `docs/HANDOFF.md:215-218` y en `config_act_registry_test.go:857-859`.

Dos detalles menores del mismo test:
- «Scoped to macOS» (`:855`) no coincide con el cable, que solo salta en linux y windows (`:867`) y corre en cualquier otro GOOS.
- El texto del skip dice que `os.SameFile` «does not tell» en Linux. Ahí solo falla cuando se reutiliza el número de inodo.

Evidencia: derivación por lectura, sin host Windows.

---

## H5 · P3 · [DOC] El punto 18 del HANDOFF afirma algo falso sobre otros tests

El punto dice «Otros tests anteriores redirigen igual, solo esas dos variables (canonical_execution_test.go y los de internal/shell)». No es así:
- Todos los de `internal/shell` redirigen `AppData` en Windows: `sandbox_test.go:26-32`, `bindings_test.go:185-190`, `upgrade_test.go:194-199` y `firstrun_test.go:420-425`.
- `internal/action/executor/canonical_execution_test.go:910` fija solo `XDG_CONFIG_HOME`, y nada en producción bajo `internal/action/executor` resuelve `os.UserConfigDir`. Los únicos llamadores de producción son `internal/app/app.go:924` e `internal/shell/firstrun.go:37`.

Lo que de verdad sigue redirigiendo solo esas dos variables es el `run()` del arnés (H1). La misma frase aparece en `t10/pretest.md`, en «Riesgos abiertos, C». Es parecido el «Redirecting HOME alone» de `main_e4_test.go:20` y de `ledger_e4_recorder_test.go:222`: el código anterior a la cura fijaba `HOME` y `XDG_CONFIG_HOME`.

---

## H6 · P3 · [TEST] G: los seis asertos del chip han perdido la comprobación de visibilidad

HEAD tenía `getByText('En vivo')).toBeVisible()` en `sp6b.spec.ts:93, 150, 178, 185, 208` y en `sp6c.spec.ts:132`. Ahora es `getByTestId('act-live-chip')).toContainText('En vivo')`.
1. Mutante: `style={{ display: 'none' }}` en el span de `src/views/Activity.tsx:79`.
2. `S/adv-cures/e2e.sh muvis e2e/sp6b.spec.ts e2e/sp6c.spec.ts --grep "AS-2: Start from the UI|Actividad vacía|Incidencia de evento"`
   Salida: `3 passed (7.5s)`, `[playwright exit 0]`.

Ni `docs/HANDOFF.md:252-253` ni el pretest declaran este cambio.

---

## H7 · P3 · [DOC] El Aviso se retira antes de que desaparezca el riesgo

`docs/HANDOFF.md:150` dice «retirarlo cuando se fusione el PR #69». Pero `1239c011` y `ac114930` quedan en la historia de master, porque las fusiones nunca se aplastan. En Windows, un checkout o un bisect de cualquiera de los dos sigue ejecutando el `os.WriteFile` de TE47 sobre `%AppData%\korvun\korvun.db`. Derivación.

---

## H8 · P3 · [DOC] «Curado» sin evidencia de Windows

`docs/HANDOFF.md:243-248` da C, D y E por «curado» sin matiz, cuando sus mitades de Windows solo han corrido en darwin. Solo B (`:241-242`) remite a la CI. IMPLEMENTED no es VERIFIED.

---

## H9 · P3 · [TEST] B: falta la fila planeada y hay conjunciones que nadie vigila

La fila «la ruta vacía» que prometía el pretest no está en la tabla (`internal/action/sqlite/ledger_dsn_path_test.go:28-35`). Además, tres conjunciones de `internal/action/sqlite/ledger_judge_seams.go:149-150` y `:157-159` sobreviven con la tabla del autor:
- MU-ADV-B1, quitando `(len==3 || [3]=='/')` → `ok … 0.506s`
- MU-ADV-B2, con `isDriveLetter` devolviendo `true` → `ok`
- MU-ADV-B3, quitando `urlPath[0]=='/'` → `ok`

Las tres son equivalentes dentro del dominio de los constructores. Mi prueba de ida y vuelta con 17 grafías pasó (`TestAdv_dsnPathForWindowsRoundTrip`, `PASS`): raíz de unidad, UNC, `\\?\C:`, `\\?\UNC`, `\\.\`, unidad en minúscula, `%20`, `%2F`, `#`, `?`, Unicode, tabulador, `RUNNER~1` y `D:`.

---

## H10 · P3 · [DOC] El árbol cambió durante la auditoría

`docs/HANDOFF.md` tiene mtime 17:15:25. Mi primer comando fue a las 17:13:32 (birth de `S/adv-cures`), el brief es de las 17:13 y el patch de las 17:12. El párrafo «El marcador» (`:259-266`) se reescribió, así que `cures-delta.patch` ya no coincide con `git diff HEAD`. Comprobé el texto nuevo contra la cabecera de `scripts/adversary-gate-check.sh` (pasos 4, 7, 8 y 9) y contra `t10/marker-simulation.txt` (17:15:07), y cuadra. Cualquier edición posterior anula este veredicto.

---

## Lo que aguanta, con captura

- Verde de las pruebas afectadas en la copia: `TestDsnPath*`, TE47, `TestE4Normalized`, TE56, TE49, `TestProfileIdentity`, `TestCreateLedger_aReplacedFileIsNeverReadopted` → `ok`. `GOOS=windows` con `test -c` y `vet` sobre los 4 paquetes → `exit=0`.
- B: MU-B1 y MU-B2 enrojecen en `ledger_dsn_path_test.go:46`, reproducidos por mí. El único llamador de producción de `dsnPath` es `guardHook` (`internal/action/sqlite/ledger_identity.go:502`). Su ruta solo la usan seams, y `judgeOnConn` no la lee.
- A: MU-A enrojece en `config_act_registry_test.go:889` sobre darwin. Hice mi propia sonda en APFS: 5000 sustituciones en la misma ruta, `judged-same=0`.
- C: transpuse el fallo de Windows a darwin, dejando sin redirigir la variable que lee este sistema (`HOME`). Salida: `main_e4_test.go:49 … refused before anything is written` y `ledger_e4_recorder_test.go:242 …`, sin directorio `korvun` en el HOME de prueba. MU47 y MU56a, repetidos sobre los tests tocados, enrojecen en `:63` y `:261`.
- D: escape HTML, U+2028 y un directorio que es prefijo de otro → `PASS`. El diff de TE49 en la CI de Windows solo difiere en la línea `path`.
- G: en mi copia, las 4 specs → `15 passed`. Con el badge mutado, `canvas-sp4` y `canvas-header` → `2 failed` («Received string: "activo · …"»). Con el chip mutado, `sp6b:87` y `sp6c:127` → `2 failed`. Los mutantes condicionados del autor son válidos.
- Aritmética: 752 subtests en rojo en Windows, de los que 748 son del paquete sqlite (50+10×6+70+160+400+1+7). Hay 8 «en vivo» y 6 `act-live-chip`. AS07 no aparece en los logs.
- Posiciones relativas: `grep` sobre las líneas añadidas sin ninguna coincidencia.

## Preguntas obligatorias

Garantía literal y dónde está el cable:
- A: `createdHere`/`CreateLedger`, en `config_act_registry.go:266-278` y `config_act.go:397-413`.
- B: `ledger_judge_seams.go:131-154`.
- C: `main_e4_test.go:22-29` y `:46-51`, `ledger_e4_recorder_test.go:236-242`.
- D: `:277-280`.
- E: `ledger_binary_test.go:61-93`.
- F: `profile_standing_test.go:43-58`.
- G: las cuatro specs.

Mutaciones que faltan: la de H2(b), la de H6, las conjunciones de H9 y la ya declarada M285 (H2a). La cura de F no se puede falsar fuera de Windows, y el pretest lo declara.

Nivel de evidencia: las etiquetas de los tests son honestas. El HANDOFF exagera en H8.

Clases conocidas: (d) e (i) → H2, H6, H9. (e) → H1. (b) y (f) → el límite declarado de Windows (H4). (g) → skip por GOOS. (h) → verificada. (a) y (c) → sin hallazgo.

## Alcance

Leí el brief, el patch, el pretest y todas las evidencias de `t10`. Leí los logs de CI (`t8/quality-ubuntu.log`, `t9/quality-windows.clean.log`, `t8/chrome-e2e-failed.log`), los 16 ficheros del delta y el código de producción citado (`store.go`, `ledger_identity.go`, `app.go`, `firstrun.go`, `controller.go`, `main.go` del arnés, `ledger.go` de la CLI, `whats_happening.go`, `controlapi.go`, `HealthzBadge.tsx`, `Activity.tsx`). También las fuentes de Go 1.26.6: `os/file.go`, `types_windows.go`, `stat_windows.go`, `types_unix.go` y `exec/lp_windows.go`.

Ejecuté todo lo citado arriba, en la copia y con HOME de prueba. Para el e2e hice mi propia copia del frontend. Su `web/builder/dist` es el artefacto compilado del clon del autor (`t8/te50/src`), declarado aquí. El árbol auditado no se ha tocado: `git status` es igual al inicio y el último mtime sigue siendo 17:15:25.

No pude verificar:
- Todo lo que dependa de Windows o Linux (sin hosts): H1, H4 y H7 son derivaciones.
- El documento de Apple sobre `apfs_next_obj_id`: no tengo herramienta web.
- Qué harán los 748 subtests cuando las seams se armen en Windows: eso solo lo dice la CI.
- `make quality`, que el brief prohíbe.
- La suite e2e completa, vitest, el lint, MU56b, el almacén de conversaciones, el cuerpo del PR y el canto.

# Segunda pasada — VETO LEVANTADO

VETO LEVANTADO

Segunda pasada, acotada, sobre las curas del PR #69. El objeto es `t10/since-verdict.patch` (12 ficheros). Lo comprobé contra mi propio diff entre `S/adv-cures/tree` (mi copia de la primera pasada) y una copia nueva del worktree (`S/adv-cures/tree2`): las 185 líneas +/- coinciden. El árbol no se movió durante esta pasada; el último mtime es 18:00:39, igual al principio y al final.

No queda ningún P1 ni P2. Cierran con captura H1–H6, H8, H9 y H10. H7 no cierra tal como está escrito (N1). Hay cuatro P3 nuevos. Tres de ellos (N1, N2 y N3) son frases que desde ahora se saben falsas; CLAUDE.md no deja diferirlas, así que se acotan en este mismo commit aunque sean P3.

Notación. S = `/private/tmp/claude-501/-Users-sebastianmorenosaavedra-Desktop-korvun-nosync/9058be48-6f33-4a92-b847-a9d29f3100f5/scratchpad`. Los comandos Go se lanzaron con `S/adv-cures/gt2.sh` (cwd `tree2`, HOME=`S/adv-cures/home2`, toolchain y cachés fijados). Playwright se lanzó con `S/adv-cures/e2e2.sh` (la misma copia; `web/builder/dist` copiado del clon del autor). Cada mutante se revirtió y se comprobó con `cmp` contra el árbol.

---

## N1 · P3 · [DOC] El punto 1 del Aviso nombra SHAs que no estarán en master

`docs/HANDOFF.md:152-153` dice que `1239c011…` y `ac114930…` «quedarán en la historia de `master` al fusionar, porque las fusiones no se aplastan». El procedimiento documentado integra con GitHub rebase: `docs/INTEGRATION.md:114-116` dice «Squash and merge commits do not satisfy this procedure», y `:156-158` dice «Integrate with GitHub rebase … do not delete the source branch». Un rebase reescribe los SHA. El último tren lo muestra, ejecutado:

1. `git merge-base --is-ancestor 6b99083841 518ba15` → `exit=1`. La punta del PR v0161-bump no está en master.
2. `git rev-parse 6b99083^{tree}` y `git rev-parse d20dea6^{tree}` dan el mismo árbol, `5ace32cc9e76ce25e09419b4f142c3777a49e226`. La copia en master tiene otro SHA; tanto `d98d1c8` como `d20dea6` tienen committer date 2026-09-24 05:54:14.
3. Si el PR #69 se integra igual, master tendrá copias de 1239c01 y ac114930 con otros SHA y los mismos árboles. Un bisect de master en Windows las pisa sin que el Aviso las nombre.

No sé cómo se fusionará el PR #69: eso lo decide el director.

## N2 · P3 · [DOC] Tres frases hermanas de H1 siguen prometiendo aislamiento en Windows, y una esconde un borrado

Las frases:
- `cmd/korvun-desktop/e2e-harness/main.go:317-320` (ayuda de `-fresh`): «HOME/XDG_CONFIG_HOME point at a temp dir and NO config is written or loaded — EnsureDefaultConfig's created=true is real».
- `main.go:321-326` (ayuda de `-agent-config`): «it is copied to the isolated HOME's config path verbatim».
- `cmd/korvun-desktop/frontend/e2e/sp6c-onboarding.spec.ts:12-15`: «fresh-reset deletes the config under the fresh harness's temp HOME».

En Windows, `tc.cfgPath` es `shell.DefaultConfigPath()` (`main.go:398`, `:444`), es decir `%AppData%\korvun\korvun.json`. `freshReset` hace `os.Remove(tc.cfgPath)` (`main.go:693`). El spec la llama en su `beforeEach` (`sp6c-onboarding.spec.ts:16-18`), y `npm run e2e` arranca el arnés `-fresh` (`playwright.config.ts:44`).

Reproducción traspuesta en darwin, ejecutada. Compilé el arnés sin redirigir la variable que lee este sistema (HOME), igual que Windows deja `AppData` sin redirigir. Puse un perfil «real» falso en `S/adv-cures/homeX/Library/Application Support/korvun/korvun.json`.
1. `HOME=$HX bin/harness-transposed -fresh -addr 127.0.0.1:43299 -dist …` y después `curl -X POST …/__test/fresh-reset` → `HTTP 204`, `after: the REAL korvun.json is GONE`.
2. `HOME=$HX bin/harness-transposed -start=false -addr 127.0.0.1:43298 -dist …` → el fichero pasa a empezar por `{ "channels": [ { "type": "telegram", …`: sobrescrito.

Lo de Windows es derivación: no tengo host Windows.

Tanto el comentario de `run` (`main.go:387-396`, «(without -fresh) writes…») como el punto 2 del Aviso (`docs/HANDOFF.md:162-166`) solo describen la sobrescritura. El punto 18 (`:352-355`) dice «el PR #69 los acota a macOS y Linux», y eso no es cierto para estas tres frases. La instrucción del Aviso («No corras esa e2e en un Windows con un perfil real») sigue cubriendo el caso.

## N3 · P3 · [DOC] El HANDOFF da por seguros la readopción y el sellado

`docs/HANDOFF.md:228-231` dice «se readopta, y la puerta sella en él su acto de fundación» sin matiz. El caso de CI que ese mismo párrafo cita, un fichero de texto, se readoptó y se rechazó con `ledger_unreadable` sin sellar nada (`t9/a-evidence.txt`). Además, en Linux depende de que se reutilice el inodo. Las notas sí dicen «puede».

## N4 · P3 · [TEST] Dos filas nuevas contradicen el nombre del test y el godoc de `dsnPathFor`

Las filas `ledger_dsn_path_test.go:38-39` fijan salidas que no son la ruta de la que se construyó el DSN. Lo ejecuté en ida y vuelta sobre la copia:
- `"C:x\\korvun.db" -> "\\C:x\\korvun.db" DOES NOT round-trip`
- `"1:\\x\\korvun.db" -> "\\1:\\x\\korvun.db" DOES NOT round-trip`

El nombre del test (`:29`) y el godoc (`ledger_judge_seams.go:142`, «turns a file DSN's URL path back into the path it was built from») prometen la ida y vuelta para toda entrada. El párrafo de nivel de evidencia (`:25-28`) solo habla de las filas del segundo bucle. Las rutas absolutas sí van y vuelven intactas: 12 de 12, incluidas `\\?\`, UNC, `%`, `#`, `?` y Unicode.

---

## Qué quedó cerrado en la primera pasada, con captura

- **H1.** Los dos comentarios citados ya dicen «macOS y Linux» y qué pasa en Windows. El Aviso cubre el arnés en cualquier commit (lo comprobé: `git show 518ba15:…/main.go` solo fija HOME y XDG_CONFIG_HOME, líneas 391 y 394). La transposición no-fresh confirma la sobrescritura. Lo que queda está en N2.
- **H2.** MU-ADV-E3 (`internal/cli/ledger.go:127` pasa de `return 1` a `return 0`) → `ledger_binary_test.go:98: … exited 0, want 1`. M285 → `:109 … refused without the name … no column named principal_id`, como declara ahora el comentario.
- **H3.** El texto de las notas se ciñe a lo que reproduje.
- **H4.** Coincide con Go 1.26.6 (`types_windows.go:290-293`, `:331`). El texto del skip y el «Skipped on Linux and Windows» coinciden con el cable (`config_act_registry_test.go:855`, `:870`).
- **H5.** El punto 18 ya no nombra tests ajenos. «HOME and XDG_CONFIG_HOME alone» está en los dos comentarios.
- **H6.**
  - Verde: las cuatro specs → `15 passed`; la suite completa → `50 passed`, `1 skipped`.
  - Chip oculto: rojo en `sp6b:93` y `sp6c:132` (`Received: hidden`).
  - Chip oculto cuando ya hay frames: rojo en `sp6b.spec.ts:185:82`.
  - Las líneas 150, 178 y 208 las cubren los ficheros del autor; comprobé que su clon es byte a byte igual al worktree en los 17 ficheros.
- **H8.** Correcto.
- **H9.** B1–B3 en rojo (evidencia del autor). Los míos:
  - MU-ADV-B4 (quitar el límite de longitud) → `panic: index out of range` en `:65`. Las filas de ruta vacía vigilan ese límite, pero solo mediante un pánico.
  - MU-ADV-B5 (separador tomado del host) → rojo en `:53`.
- **H10.** Reconocido.
- **Godoc de `sameFile`.** Correcto.

## Respuestas al ataque pedido

- **`filter({ hasText })` frente a `getByText(...).toBeVisible()`.** Tiene la misma semántica que el spec aprobado original: subcadena sin distinguir mayúsculas y `toBeVisible` con las mismas opciones, ahora limitado al chip. MU-ADV-G4 (texto del chip en minúscula) → `3 passed`: ni el original ni la cura distinguen «En vivo» de «en vivo». `docs/HANDOFF.md:265-266` («con «En vivo»») no lo dice. Es solo una nota, no un hallazgo.
- **Posiciones relativas.** No hay. «el punto 1/2 del «Aviso para Windows»» es un ordinal dentro de un bloque con nombre, no una distancia desde la frase que cita.
- **Formato y vet.** `gofmt -l` sobre los 11 ficheros Go → nada. `GOOS=windows go vet` en los 4 paquetes → `exit=0`. Los tests Go tocados, en verde en `tree2`.

## Alcance

Leí `adversary-brief-2.md`, `since-verdict.patch`, la evidencia nueva de `t10` (mutaciones b2, e3 y m285, e2e G2b/G3/G2c2/G3c, `touched-final`, `cross-*`, `frontend-lint2`), el texto final de los 12 ficheros, `docs/INTEGRATION.md`, `playwright.config.ts`, `sp6c-onboarding.spec.ts` y `freshReset`.

Ejecuté todo lo citado arriba en `tree2` y en `S/adv-cures/bin`. El HOME fue siempre de prueba; en `home2` no apareció ningún directorio `korvun`, y los arneses traspuestos se pararon (puertos libres). El árbol auditado no se ha tocado.

No pude verificar el comportamiento en Windows ni en Linux (N2 y H4 son derivaciones, y N2 además traspuesta), cómo se fusionará el PR #69, el lint de frontend (no lo repetí) ni `make quality`.

# Tercera pasada — VETO MANTENIDO

VETO MANTENIDO

Tercera pasada, corta, sobre las curas del PR #69. El objeto es `t11/since-verdict-2.patch` (6 ficheros). Coincide con mi propio diff entre `adv-cures/tree2` y una copia nueva del worktree (`adv-cures/tree3`): 243 líneas +/- idénticas, contando el fichero nuevo. El árbol no se movió: el último mtime es 18:36:47, igual al principio y al final, y hay 19 entradas cambiadas.

El resultado es un P2 y tres P3. N1, N2 (los textos), N4 y la nota del chip quedan cerrados. N3 queda cerrado a medias.

Notación. S = `/private/tmp/claude-501/-Users-sebastianmorenosaavedra-Desktop-korvun-nosync/9058be48-6f33-4a92-b847-a9d29f3100f5/scratchpad`. Los tests Go se lanzaron con `S/adv-cures/gt3.sh` (cwd `tree3`, HOME=`S/adv-cures/home3`, toolchain y cachés fijados). La e2e se lanzó con `S/adv-cures/e2e3.sh` sobre la misma copia, con HOME de prueba vía `E2E_HOME` y el arnés recompilado en cada ejecución. Cada mutante se revirtió y se comprobó con `cmp` contra el árbol.

---

## P2 · [TEST][DOC] Ningún test vigila la llamada a `isolate` dentro de `run`, y el HANDOFF y un test nombran como prueba tests que no llegan a ella

La garantía nueva está en `cmd/korvun-desktop/e2e-harness/main.go:389-396` («ALWAYS isolate the default-config location into the temp dir, on every OS») y en `:256-258`. El cable que la sostiene es `main.go:397` (`if err := isolate(dir)`, antes de `shell.DefaultConfigPath()` en `:401`). Si se quita esa llamada, no se pone rojo ningún test.

Reproducción, ejecutada:
1. En la copia, quitar `if err := isolate(dir); err != nil { return err }` (`main.go:397-399`).
2. `gt3.sh test -count=1 ./cmd/korvun-desktop/e2e-harness/` → `ok … 0.528s`. `TestIsolationEnv`, `TestIsolate` y TE47 siguen en verde.
3. Perfil «real» falso en `S/adv-cures/home3m/Library/Application Support/korvun/korvun.json` = `{"note":"the REAL profile of this fake user"}`. `E2E_HOME=…/home3m e2e3.sh muh4` (e2e de chrome completa) → `50 passed (1.5m)`, `1 skipped`, `[playwright exit 0]`.
4. Después, en ese directorio «real»: `korvun.json` sobrescrito con la configuración de guion (`{ "channels": [ { "type": "telegram", …`), y además `korvun.db` (507904 B), `-wal`, `-shm`, `keys/` y `korvun.lock`.
5. Control sin mutante: `e2e3.sh full3` → `50 passed (1.6m)`, y ningún directorio `korvun` en el HOME de prueba.

Contraste: si `isolate` se mueve detrás de `DefaultConfigPath` (MU-ADV-H6), los tests Go dan `ok`, pero la e2e sí se pone roja: `14 failed`, `6 did not run`, casi todo `locator.click: Test timeout`. El perfil «real» se sobrescribe igualmente. La e2e detecta la incoherencia entre rutas, no la pérdida del aislamiento.

La sesión declaró esa mutación sin ejecutar («only the e2e would see it, and an unisolated harness points at the real profile»). La ejecución desmiente las dos cosas: con un HOME de prueba se puede correr sin riesgo, y la e2e no la ve.

Frases más anchas que el cable:
- `main_isolation_test.go:52-53` dice de `TestIsolate`: «On Windows it runs in CI, and is the harness's evidence there». Ese test prueba `isolate()`, no `run()`, y sigue en verde sin la llamada.
- `docs/HANDOFF.md:173-176` dice «(`isolationEnv`, con sus moldes) … Este punto se queda hasta que la CI de Windows del commit de curas confirme que nada toca la carpeta real: allí corren TE47, TE56 y `TestIsolate…`». Ninguno de esos tres ejecuta `run()`: TE47 y TE56 pertenecen al punto 1, y la CI de Windows no arranca nunca el arnés (`.github/workflows/frontend.yml:93`, la e2e de chrome corre en `ubuntu-latest`). Una CI de Windows en verde retiraría el aviso sin haber confirmado lo que promete. La condición del director tal como está escrita («hasta que la CI de Windows confirme que nada toca la carpeta real») no se puede cumplir con la CI actual. Es un conflicto que hay que llevar al director.
- `docs/HANDOFF.md:372-375` pone el arnés en «Cerrados», mientras el Aviso lo mantiene abierto.

## P3 · [DOC] El godoc de `writeScriptedConfig` quedó pegado a `isolationEnv`

El bloque nuevo se insertó en `main.go:499`, justo debajo de `:497-498` («writeScriptedConfig writes the one-telegram harness config…») sin separarlos. Salida de `go doc -u -c ./cmd/korvun-desktop/e2e-harness isolationEnv`: `writeScriptedConfig writes the one-telegram harness config (pointed at the fake model) to path, creating the parent dir. isolationEnv is the environment…`. `go doc … writeScriptedConfig` ya no muestra ningún comentario (la función está en `:535`). En HEAD, ese comentario precedía a la función.

## P3 · [DOC] N3 sigue siendo demasiado categórica

`docs/HANDOFF.md:239-240` dice «Si la puerta puede abrirlo como libro, sella en él su acto de fundación». Lo simulé ejecutando: el resultado de Linux y Windows se reproduce con un mutante en el que `createdHere` compara por ruta.
1. Otro perfil funda y aplica su libro en la misma ruta (standing `ok` para su dueño).
2. El segundo `CreateLedger` de este perfil lo abre y no sella: `the ledger was created at … but could not be prepared to seal its founding act (identity registry): … ledger_foreign_profile: this ledger was founded or adopted by another profile …`.

El caso de un libro sin marca sí está bien descrito.

## P3 · [DOC] Menores
- «on every OS» (`main.go:257`, `:320`, `sp6c-onboarding.spec.ts:13`) y «every source os.UserConfigDir reads» (`main.go:499-501`): en plan9, `os.UserConfigDir` lee `home` (Go 1.26.6, `src/os/file.go:574-579`), y `isolationEnv` no lo fija. Korvun no apunta a plan9, pero la frase va más allá del cable.
- `main.go:503-505` da por hecho que la configuración y el libro «landed» en el `%AppData%` real. Nunca se ejecutó en Windows; `main_isolation_test.go:15-19` sí dice «never run on Windows».

---

## Qué quedó cerrado, con captura

- **Mutaciones del aislamiento, repetidas por mí sobre los bytes finales.** Las de la sesión corrieron a las 18:34 sobre un `main.go` que cambió a las 18:34:53. MU-H1 (sin `AppData`) → rojo en `main_isolation_test.go:44`. MU-H2 (sin `LocalAppData`) → rojo en `:44`. MU-H3 (`isolate` no fija nada) → rojo en `:69`. MU-ADV-H5 (`isolate` con goos fijo «linux») → `ok` en darwin; es lo esperado, solo lo vería la ejecución de `TestIsolate` en Windows, y así está declarado.
- **Orden.** `isolate` (`:397`) va antes de `DefaultConfigPath` (`:401`). Antes de la llamada, nada resuelve un directorio de usuario: `shell.New` solo guarda opciones, `NewDesktop` guarda `DefaultConfigPath` como función (`internal/shell/bindings.go:127`), y `MkdirTemp` usa a propósito el TEMP real.
- **USERPROFILE, TEMP y `os.UserHomeDir`.** Nada de lo que arranca el arnés los resuelve. Un grep del código no test de `internal/` y `cmd/` solo encuentra el `MkdirTemp` del propio arnés.
- **Restauración del entorno en `TestIsolate`.** Lo comprobé con un test de prueba ordenado detrás de él: HOME, `XDG_CONFIG_HOME` (fijado a ""), `AppData` y `LocalAppData` (sin fijar) vuelven a su valor inicial.
- **N1.** El punto 1 nombra los commits por SHA y por asunto, que coinciden con `git log`. Explica que el rebase deja copias y que el punto no se retira.
- **N2.** Las tres frases son ciertas mientras exista la llamada (e2e sin mutante: ningún `korvun` en el HOME de prueba).
- **N4.** Las filas se movieron al bucle directo, el godoc quedó acotado y las mutaciones de B de la sesión están en rojo.
- **Nota del chip.** Cerrada.
- **Posiciones relativas.** No hay ninguna.

## Alcance

Leí `adversary-brief-3.md`, `since-verdict-2.patch`, la evidencia nueva de `t11` (h-red, h-green, mutations-h, mutations-b3, touched-final, cross, frontend-lint3), el texto final de los 6 ficheros, `run()` hasta `DefaultConfigPath`, y `shell.New` y `NewDesktop`.

Ejecuté todo lo citado arriba, siempre en `tree3` y con HOME de prueba. No queda ningún arnés vivo y los puertos están libres. El árbol auditado no se ha tocado.

No pude verificar nada de Windows (el efecto de `AppData` y `LocalAppData` en un host real, el orden de las variables allí), ni repetir `make quality` o el lint de frontend.

# Cuarta pasada — VETO LEVANTADO

VETO LEVANTADO

Cuarta pasada, corta, sobre las curas del PR #69. El objeto es `t11/since-verdict-3.patch` (5 ficheros). Coincide con mi propio diff entre `adv-cures/tree3` y una copia nueva del worktree (`adv-cures/tree4`): 236 líneas +/- idénticas. El árbol no se movió: el último mtime es 19:07:19, igual al principio y al final, y hay 20 entradas cambiadas.

Queda cerrado el P2 de la tercera pasada, con captura propia. También quedan cerrados los P3 del godoc desplazado y de «on every OS». No queda ningún P1 ni P2. Hay tres P3 nuevos, todos menores.

Notación. S = `/private/tmp/claude-501/-Users-sebastianmorenosaavedra-Desktop-korvun-nosync/9058be48-6f33-4a92-b847-a9d29f3100f5/scratchpad`. Los comandos Go se lanzaron con `S/adv-cures/gt4.sh` (cwd `tree4`, HOME=`S/adv-cures/home4`, toolchain y cachés fijados). Cada mutante se revirtió y se comprobó con `cmp` contra el árbol.

## El P2 de la tercera pasada: cerrado

`TestHarnessBinary_neverTouchesTheRealUserConfigDir` (`cmd/korvun-desktop/e2e-harness/main_binary_test.go:34`) compila el arnés y lo arranca como proceso aparte. Antes de arrancarlo planta un perfil marcador justo donde el hijo escribiría si nada lo aislara. Verifiqué los tres puntos del ataque:

- **Fuerza y observa.** Con los bytes finales:
  - `gt4.sh test -race -count=1 -v ./cmd/korvun-desktop/e2e-harness/` → los cuatro tests en `PASS`, `ok … 8.088s`.
  - MU-ADV-H4 (quitar la llamada a `isolate` de `run`) → rojo en `main_binary_test.go:147` («resolves its config at …/real/Library/Application Support/korvun/korvun.json, outside its temp dir») y en `:151` («the real profile was touched: {"channels": …»).
  - MU-ADV-H6 (llamar a `isolate` después de `DefaultConfigPath`) → rojo en `:151`.
  - En ningún caso apareció un directorio `korvun` en el HOME de prueba.
- **Precedencia de variables en Windows.** Por lectura de Go 1.26.6: `dedupEnv` (`src/os/exec/exec.go:1251-1253` y `:1258-1295`) conserva la última aparición sin distinguir mayúsculas en Windows, así que la lista de variables que añade el test gana. TMP y TEMP gobiernan el `MkdirTemp` del hijo. El marcador se resuelve con el mismo entorno que vería el hijo. El comentario «the last value wins, case-insensitively on Windows» es correcto.
- **CI.** Ni `Makefile:155` ni `quality.yml:205` usan `-short`, así que el test corre en Windows. `GOOS=windows go vet` y `go test -c` → ok. `gofmt -l` → vacío.

## P3 · [TEST][DOC] La CI de Windows verá el arranque del arnés, no el núcleo

El test arranca con `-start=false` (`main_binary_test.go:88`), así que el núcleo nunca abre su libro durante la prueba. El punto 2 del Aviso incluye «el núcleo que arranca abría el libro real», y su condición de retirada (`docs/HANDOFF.md:175-178`, «comprueba que no lo toca») se cumplirá en Windows sin que el núcleo haya llegado a correr.

Por lectura, el núcleo resuelve con el mismo entorno del proceso. Lo ejecuté en darwin con una variante idéntica pero con el núcleo arrancado:
1. La misma prueba con `-start=true`.
2. Tras «korvun is serving», espera de 3 s y parada.
3. Resultado: `real config dir after a STARTED core: [korvun.json]` y `PASS`.

El comportamiento en darwin es correcto; lo que falta es que la CI de Windows lo observe. Conviene que el director lo sepa al retirar el aviso.

## P3 · [DOC] N3: «Si es un libro sin marca, la puerta sella» no cubre un libro sin marca con esquema antiguo

`docs/HANDOFF.md:241-242`. La puerta del operador rechaza un fichero existente con un esquema anterior al actual (`internal/action/sqlite/store.go:1664-1674`, `ErrSchemaBehind`, «an operator act never migrates an existing store»). Según las propias notas, «libro sin marca» incluye el libro de todo perfil anterior a esta versión. Un libro de esos que no haya pasado por un arranque de la v0.16.2 se rechaza sin sellar. Esto es derivación por lectura; no lo ejecuté.

## P3 · [DOC] Menores

- `main.go:504-505` dice «(it never ran there)»: nadie puede saber que el arnés no se ejecutó nunca en Windows; la afirmación va más allá de lo que se puede comprobar.
- El nombre `TestIsolationEnv_pointsEveryUserDirSourceIntoTheTempDir` sigue diciendo «Every», aunque su cabecera ya lo acota a tres sistemas.
- `main.go:258` quedó con una línea suelta de comentario («// the harness's»). Es cosmético.

## Otras comprobaciones

- **`go doc`.** `isolationEnv`, `isolate` y `writeScriptedConfig` tienen cada uno su propio comentario.
- **«on every OS».** Acotado a «macOS, Linux and Windows» en `main.go:256-257`, `:320-321` y `:391-395`, en la cabecera de `TestIsolationEnv` y en `sp6c-onboarding.spec.ts:12-16`.
- **HANDOFF.** El punto 2 del Aviso ya nombra el test binario y dice que la e2e de chrome no arranca nunca el arnés en Windows (comprobado: los cuatro jobs de `frontend.yml` están en `ubuntu-latest`). El arnés salió de «Cerrados».
- **Cambios en `main.go`.** Solo afectan a comentarios y al texto de ayuda de `-fresh`; la e2e no cambia desde la pasada anterior, donde dio 50 pasan y 1 se salta.
- **Posiciones relativas.** No hay ninguna.
- **Inestabilidad.** El puerto se elige con Listen/Close, con una ventana mínima. La espera usa el propio binding, con un límite de 90 s. En Windows la parada es `Kill` más `Wait`, y el directorio temporal lo borra `t.TempDir`. No lo pude ejecutar en Windows.

## Alcance

Leí `adversary-brief-4.md`, `since-verdict-3.patch`, la evidencia nueva de `t11` (hb-green, mutations-hb, touched-final2, cross2, frontend-lint4) y los cinco ficheros finales.

Ejecuté todo lo citado arriba en `tree4`, siempre con HOME de prueba. Las mutaciones de la sesión corrieron a las 19:05 sobre un `main.go` que cambió a las 19:06:58, así que las repetí sobre los bytes finales. No queda ningún arnés vivo. El árbol auditado no se ha tocado.

No pude verificar nada de Windows, más allá de compilarlo en cruzado y de `vet`. No repetí la e2e completa, porque en esta pasada no cambió el código del arnés, ni `make quality`.

# Quinta pasada — VETO LEVANTADO

VETO LEVANTADO

Quinta pasada, muy corta: solo el movimiento que siguió al veredicto de la cuarta. El objeto es `t11/since-verdict-4.patch` (5 ficheros). Coincide con mi propio diff entre `adv-cures/tree4` y una copia nueva del worktree (`adv-cures/tree5`): 43 líneas +/- idénticas. El árbol no se movió: el último mtime es 19:19:51, igual al principio y al final, con 20 entradas.

Los tres P3 de la cuarta pasada quedan cerrados, dos de ellos con captura propia. No hay hallazgos nuevos: ni P1, ni P2, ni P3.

Notación. S = `/private/tmp/claude-501/-Users-sebastianmorenosaavedra-Desktop-korvun-nosync/9058be48-6f33-4a92-b847-a9d29f3100f5/scratchpad`. Los comandos Go se lanzaron con `S/adv-cures/gt5.sh` (cwd `tree5`, HOME=`S/adv-cures/home5`, toolchain y cachés fijados). Cada mutante se revirtió y se comprobó con `cmp` contra el árbol.

## P3 «la CI de Windows verá el arranque, no el núcleo»: cerrado, con prueba del test

`main_binary_test.go:89` ahora usa `-start=true`. Por lectura, el núcleo abre su libro antes de que el arnés sirva: `main.go:433-440` llama a `ctrl.Start` antes de `ListenAndServe` en `:472-476`, y `Start` espera a `started` después de `app.Build` (`internal/shell/controller.go:239-300`).

Test del test, ejecutado. Mutante MU-ADV-H7: fuga solo el libro del núcleo, no el fichero de configuración. La ruta de almacén se resuelve antes de `isolate` y se escribe en la configuración del arnés.
1. Contra el test actual → rojo en `main_binary_test.go:164`: «the real config dir holds [keys korvun.db korvun.json korvun.lock] after the harness ran, want only korvun.json».
2. El mismo mutante contra el test de la cuarta pasada (`-start=false`) → `PASS`. Ese era el hueco, y ya está cerrado.

Con los bytes finales:
- Verde: `gt5.sh test -race -count=1 -v ./cmd/korvun-desktop/e2e-harness/` → cuatro `PASS`, `ok … 5.856s`.
- MU-ADV-H4 (quitar la llamada a `isolate`) → rojo en `:148`, `:152` y `:164`.
- MU-ADV-H6 (`isolate` después de `DefaultConfigPath`) → rojo en `:152`.
- No apareció ningún directorio `korvun` en el HOME de prueba.

## P3 N3 y el libro con esquema antiguo: cerrado

- `docs/HANDOFF.md:242-248` limita el sellado a «un libro sin marca que esta versión ya abrió y migró». La frase es exacta también para un fichero nuevo: `openWithIdentityMode` lo siembra y luego lo migra hasta el esquema actual. Del libro con esquema anterior dice que «se rechaza sin sellar (`ErrSchemaBehind` …), por lectura del código», y eso coincide con `internal/action/sqlite/store.go:1664-1674`.
- Las notas (`docs/releases/v0.16.2.md:279-283`) limitan la frase a «que esta versión ya haya abierto».

## P3 menores: cerrados

- `main.go:506-508` ahora dice «by reading the code, not by running it there».
- `TestIsolationEnv_pointsTheUserDirSourcesIntoTheTempDir` (`main_isolation_test.go:28`, `:51`): no queda ninguna referencia al nombre viejo en todo el árbol (grep sin `node_modules` ni `.git` → 0).
- La línea suelta de `main.go:257-259` quedó recompuesta.

## Otras comprobaciones

- `gofmt -l` → vacío.
- `GOOS=windows` y `GOOS=linux`: `vet` → ok, `test -c` → ok.
- Posiciones relativas: ninguna. «esquema anterior» se refiere a la versión, no a una posición en el texto.
- Una línea de `docs/HANDOFF.md:246` pasa de 80 columnas; es cosmético y el fichero ya tiene otras así.

## Alcance

Leí `adversary-brief-5.md`, `since-verdict-4.patch`, la evidencia nueva de `t11` (hb-green2, mutations-hb2, cross3), las líneas cambiadas en los cinco ficheros y el camino de `ctrl.Start`. El hash de `main.go` en `hb-before2.sha` coincide con el final, así que su mtime posterior viene de la restauración.

Ejecuté todo lo citado arriba en `tree5`, con HOME de prueba. No queda ningún arnés vivo. El árbol auditado no se ha tocado.

No pude verificar nada en Windows real. No repetí la e2e completa: en esta pasada solo cambiaron comentarios de `main.go` y el flag del test binario, y la sesión da 50 pasan y 1 se salta en `t11/e2e-final5.txt`. Tampoco repetí `make quality`.

# Sexta pasada — VETO LEVANTADO

VETO LEVANTADO

Sexta pasada, mínima. El árbol se movió por dos cosas: el reformateo con prettier de dos specs y el fichero nuevo `.claude/adversary/v0162-pr69-cures-verdict.md`. Mi diff entre `adv-cures/tree5` y una copia nueva del worktree (`adv-cures/tree6`) da tres ficheros: esos dos specs, con 20 líneas +/- idénticas a las de `t11/since-verdict-5.patch`, y el registro. Excluí `cmd/korvun-desktop/frontend/coverage/` y `coverage.out`, que son artefactos del gate e ignorados por git (`frontend/.gitignore:13`, `.gitignore:16`).

No hay ningún P1 ni P2. Hay un P3 sobre mi propio texto, que decide el director.

Notación. S = `/private/tmp/claude-501/-Users-sebastianmorenosaavedra-Desktop-korvun-nosync/9058be48-6f33-4a92-b847-a9d29f3100f5/scratchpad`. Todo lo ejecutado corrió en `S/adv-cures/tree6`, con HOME de prueba (`home5` y `home6`).

## El reformateo es solo de formato

1. Comparé los dos specs de antes (`tree5`) y de ahora (`tree6`) quitando todo el espacio en blanco y la coma final antes de un cierre. `sp6b.spec.ts` (266 → 274 líneas) y `sp6c.spec.ts` (142 → 144) salen idénticos: el cambio es de formato.
2. `npx prettier --check e2e/sp6b.spec.ts e2e/sp6c.spec.ts` sobre los bytes nuevos → «All matched files use Prettier code style!», exit 0. Sobre las versiones de `tree5` → «[warn] … sp6b.spec.ts», «[warn] … sp6c.spec.ts», que es exactamente el rojo del gate (`t11/commit-c2.out`).
3. `npx eslint` sobre los dos specs → exit 0.
4. `e2e6.sh specs6 e2e/sp6b.spec.ts e2e/sp6c.spec.ts` → `13 passed (50.7s)`, sin ningún directorio `korvun` en el HOME de prueba.
5. `desktop-frontend-check` es el último requisito de `quality` (`Makefile:266`). Que el gate cayera ahí es coherente con que los pasos anteriores pasaran. Lo leo en la cola del fallo; no lo repetí.

## El registro reproduce fielmente las cinco pasadas

1. Rearmé el fichero a partir de `t10/adversary-verdict-1.md`, `-2.md` y `t11/adversary-verdict-3.md` … `-5.md`, con su introducción y su nota de custodia. Salen 42530 bytes, idéntico al del árbol: `diff` vacío.
2. Contra mis mensajes de entrega comprobé 22 pasajes exactos, con al menos cuatro de cada pasada: la primera frase, títulos, líneas de reproducción con sus salidas y la última frase de cada «Alcance». Aparecen todos. Los títulos «#» y «##» de las cinco pasadas están en mi mismo orden, y los primeros renglones son MANTENIDO, LEVANTADO, MANTENIDO, LEVANTADO y LEVANTADO. No es una comparación byte a byte con mis mensajes, que no tengo en disco.
3. La nota de custodia es correcta: no escribí nada en el árbol.
4. `t11/assemble-verdicts.py` ya está preparado para regenerar el fichero con seis pasadas. Concatena literalmente los veredictos guardados y exige que el último empiece por «VETO LEVANTADO». Ese fichero de seis pasadas todavía no existe, así que mi comprobación cubre el de cinco.

## P3 · [DOC] El registro copia cinco localizadores relativos que escribí yo

En cinco secciones «Alcance» del registro (líneas 147, 238, 308, 368 y 418) aparece «Ejecuté todo lo citado arriba». Eso sitúa otras frases del documento por su posición, algo que CLAUDE.md prohíbe en todo documento («Comments carry no relative positions»). El texto es mío, y la copia literal impide que la sesión lo corrija. Hay precedente: registros ya commiteados, como `v0162-tren-E-diff-verdict.md`, tienen 6 casos del mismo tipo. El director decide si un registro literal y congelado queda fuera de esa ley. Este mensaje no usa ninguno.

Dos casos que no cuentan: la línea 276 describe dónde quedó un bloque de código respecto a líneas numeradas, y la línea 17 cita el comentario antiguo «(set below)».

## Nota sobre lo que se commitea

El índice todavía tiene `sp6b.spec.ts` y `sp6c.spec.ts` sin reformatear: `git diff` muestra 15 inserciones y 5 borrados pendientes en el worktree. Este veredicto cubre los bytes del worktree, y el commit tiene que llevar esos bytes.

## Alcance

Leí `since-verdict-5.patch`, los dos specs, el registro completo, `assemble-verdicts.py`, `commit-c2.out`, `gate-c2.txt`, `frontend-check-gate.txt` y el objetivo `quality` del Makefile.

Ejecuté todo lo que describe este informe, siempre en `tree6` y con HOME de prueba. El árbol auditado no se ha tocado; el último mtime sigue siendo 20:24:39.

No repetí `make quality`, ni la e2e completa (los 50 los da la sesión en `t11/e2e-final6.txt`), ni nada en Windows.

---

*Persistido por el ejecutor el 2026-09-27, copiado de los mensajes de entrega del adversario sin cambios en el texto; solo se quitó la sangría de dos espacios que añade el arnés. El adversario no escribió en el árbol: su definición se lo prohíbe.*
