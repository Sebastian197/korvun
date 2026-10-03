VETO LEVANTADO

Las cuatro pasadas del adversario sobre la cura declarada de AS07 (orden del director del 2026-09-29), en orden y copiadas literalmente. La última es el veredicto que vale; las anteriores quedan como historia de lo que encontró y de cómo se curó.

Custodia: el adversario no escribió en el árbol. Entregó cada veredicto como mensaje final de su pasada, y cada uno se copió byte a byte desde su propia transcripción. Hora (UTC) y sha256 de cada mensaje:
- Primera pasada: 2026-09-29T18:49:20.483Z, `582095f0632cd89334f422c72c0a174fed0f8fedfc51edcbcea99fb940776da2`.
- Segunda pasada: 2026-09-29T19:37:23.976Z, `7d62fbd8f4e4779e05539d655f3f432d6c923b1d98f261c91a1e0c8c5abe01b4`.
- Tercera pasada: 2026-09-29T20:05:44.866Z, `947fdb49560e594734ab43e3ef320db5dbad71324cd4281be88c570552f29e7b`.
- Cuarta pasada: 2026-09-29T20:19:25.198Z, `4b48c179de4bfb4cce9e9acad114f596ef97107a1e9d27403bc9e22724771ad6`.

# Primera pasada — VETO MANTENIDO

VETO MANTENIDO

Objeto: `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `as07-declared-cure`, HEAD `abb51d4975bec01dc38fd957df9d51d5de5f50fe` (= origin/master). El delta está sin commit y tiene tres ficheros: `docs/HANDOFF.md` (M), `internal/action/sqlite/authority_as07_test.go` (M) y `internal/action/sqlite/authority_as07_driver_test.go` (nuevo). Comparé el patch guardado (`scratchpad/t14/as07-delta.patch`) con `git diff HEAD` más el fichero nuevo, sin contar las líneas `index`, y coinciden. Hashes sha256 al empezar y al terminar, idénticos: test `0f2a7370…`, driver `a620cb9a…`, `authority_v2.go` `9cae4ae7…`, `store.go` `37efa0d0…`, HANDOFF `8839d6dd…`. `git status --short` sale igual antes y después. No he escrito nada en el árbol. Las mutaciones corrieron solo en `scratchpad/adv-as07/{mut,mastermut}`, aplicadas por `adv-as07/mutate.py`: cada reemplazo tiene que aparecer exactamente una vez y el fichero se restaura por sha256 después de cada ejecución.

Comandos (todos con `-race -count=1 -v`, ejecutados en la copia salvo que diga otra cosa):
- CMD3 = `go test -race -count=1 -run 'TestAuthority_ConcurrentStartsShareAncestorBudget|TestAuthority_AS07_' -v ./internal/action/sqlite/`
- CMDmain = `go test -race -count=1 -run '^TestAuthority_ConcurrentStartsShareAncestorBudget$' -v ./internal/action/sqlite/`
- CMDm1 / CMDm2 = lo mismo con `^TestAuthority_AS07_aRealBusyOnOneStartIsRetriedByTheTest$` / `^TestAuthority_AS07_busyPastTheCapOnTheLastStartIsNamedBusy$`

Línea base en el árbol auditado, con CMD3: `PASS (4.70s)`, `busy retries … 1`, `PASS (9.77s)`, `PASS (4.72s)`, `ok … 20.900s`; `git status` sin cambios.

No hay hallazgos [PRODUCT]: la puerta no se toca.

---

## F1 · P2 · [DOC] · El comentario de AS07 da como desenlace de una mutación un «committed=48» que la ejecución desmiente

Afirmación. `authority_as07_test.go:28-31` dice: «the exhaustion check removed on every account → committed=48 and the shared account spent past its max_total → reddens». Al ejecutarlo salen 13 committed y 35 `budget evidence corrupt`. El rojo no viene del recuento de 48 ni del aserto del libro: lo da la rama «other» de `as07Check`, porque una segunda guarda de la tienda para la carrera en 13. El comentario se escribió antes de ejecutar la mutación: el fichero está congelado desde las 19:09 (`freeze-as07.txt`, mismo hash que ahora) y MU-P2 se ejecutó después (`mutations-as07.jsonl`, 19:10-19:11). Después siguió así, aunque la captura del propio autor lo contradecía («start 0: action/sqlite: budget evidence corrupt», y MU-P2b «spent=13 of max_total=12»). Es una predicción presentada como captura, dentro de un fichero congelado.

Reproducción:
1. Copia de `authority_v2.go`: en `:2391` `if v <= 0 {` → `if v <= 0 && v > 0 {`, y en `:2407` `if spent >= maximum {` → `if spent >= maximum && spent < maximum {` (la MU-P2 del autor).
2. Sonda de solo lectura en el test principal, justo después de `results := x.runAll(nil, nil)`: cuenta los desenlaces por clase y llama a `x.sharedAccount(t)`.
3. CMDmain, captura C1 (las líneas del test salen desplazadas por la sonda):
   `PROBE outcomes: map[committed:13 other | action/sqlite: budget evidence corrupt:35]`
   `PROBE ledger shared account: spent=13 max_total=12`
   `authority_as07_test.go:52: start 0: action/sqlite: budget evidence corrupt` → `--- FAIL`
4. Añadiendo además las dos guardas de `remainingBeforeAccountTx` (`:2262` y `:2276`, `value < 0` → `value < 0 && value > 0`), captura C2:
   `PROBE outcomes: map[committed:13 other | action/sqlite: authorization snapshot corrupt:35]`, `spent=13 max_total=12`. La tercera guarda está en `snapshot.Validate()` (`authority_v2.go:2011-2012`).

Por qué P2: es una frase falsa dentro del árbol sobre la evidencia de una mutación probatoria, y la ley del tono no admite embarcarla. Además oculta que lo que detiene el sobregasto es `remainingBeforeAccountTx` (`authority_v2.go:2261-2263`, llamada desde `resolveAuthorityTx` `:2150-2154`) y no el recuento. No es P1 porque el código no hace nada malo. Aviso de proceso: curarlo obliga a tocar un fichero congelado.

## F2 · P2 · [TEST] · Ningún test fija que solo se reintenta `ErrLedgerBusy`: si se amplía el reintento, los tres tests siguen en verde

Afirmación. `as07Drive` promete «Any other error ends the start at once, as itself» (godoc en `authority_as07_driver_test.go:108-109`; el código está en `:122-127`). El pretest lo declaró como ataque: «Solo se reintenta ErrLedgerBusy. Cualquier otro error, también ErrAuthorityStoreBusy por un contexto vencido, termina el arranque como su propia clase» (`pretest.md:13`). Ningún molde lo fuerza. Hoy el código lo cumple, pero las dos mutaciones más naturales sobreviven a toda la suite.

Reproducción:
1. A1: en `driver:124-126`, el `default:` pasa de `r.outcome = as07Other` + `return r` a `r.busy++` (se reintenta cualquier error). CMD3 → los tres `PASS`, `ok … 21.247s`.
2. A2: en `driver:122`, `errors.Is(r.err, ErrLedgerBusy)` → `errors.Is(r.err, ErrAuthorityStoreBusy)`. CMD3 → los tres `PASS`, `ok … 21.285s`.
3. A2 reintentaría respuestas de la puerta que no llevan `ErrLedgerBusy`: el bucle propio de 30 s cuando se rinde (`authority_v2.go:675-678`, con el error crudo envuelto en `:706`), el fin de un contexto (`:705`) y un COMMIT fallido (`:1983-1985`). Lo comprobé con una sonda sintética del conductor, no con la puerta real: un fichero temporal en la copia que llama a `as07Drive` con tres errores.
   - Sobre el código sin mutar (A0, verde): `store busy from a deadline: outcome=other attempts=1 calls=1`, `store busy from a commit: outcome=other attempts=1 calls=1`, `budget evidence corrupt: outcome=other attempts=1 calls=1`.
   - Bajo A2 (A2p, rojo): `store busy from a deadline: outcome=busy attempts=20 calls=20` y `store busy from a commit: outcome=busy attempts=20 calls=20`.
4. A1 junto con la MU-P2 de producción, CMDmain (A1xP2): `busy retries across the 48 starts: 700` y `authority_as07_test.go:39: start 1 still ledger_busy after 20 attempts: action/sqlite: budget evidence corrupt`. Sin A1, el mismo defecto sale con su nombre: «start 0: action/sqlite: budget evidence corrupt» (C1).

Por qué P2: la revisión previa aprobada declaró este ataque, no tiene molde, y su mutación sobrevive a toda la suite (puntos 1 y 4 de la doctrina). Que el reintento sea estrecho es lo único que hace cierto «la puerta no cambia y el test solo reintenta busy». Si se ensancha, el test absorbe errores de otras clases o los rebautiza: corrupción disfrazada de busy, que es una clase de fallo que este repo ya ha tenido. No es P1 porque hoy el conductor clasifica bien (A0).

## F3 · P3 · [TEST] · El molde 1 no comprueba el reintento que le da nombre

Afirmación. `TestAuthority_AS07_aRealBusyOnOneStartIsRetriedByTheTest` fuerza un busy real y comprueba el primer error (`authority_as07_test.go:90-92`), pero no mira `driven`: ni su desenlace ni sus intentos. El arranque 26 corre solo, antes que los demás y con el presupuesto intacto, y los otros 47 dan 12/35. Por eso la cura ingenua (sin reintento y con el busy contado como agotado) vuelve a sumar 12/36, y el molde 1 no la detecta. Solo la detecta el molde 2, que usa un busy sintético.

Reproducción:
1. G1: en `driver:122-123`, después de `r.busy++`, añadir `r.outcome = as07Exhausted` y `return r`.
2. CMD3: principal `PASS`; molde 1 `PASS (9.94s)`; molde 2 `FAIL` con «the last start = exhausted after 1 attempts (1 calls), want busy after exactly 20».
3. Sin mutación, una sonda de tiempos en CMDm1 (F1-timing) confirma que el busy es real y viene de la puerta: «PROBE attempt 1 of start 26 took 5.067526912s: action/sqlite: authority store busy: action/sqlite: ledger_busy: … database is locked (5) (SQLITE_BUSY)» y «attempt 2 … took 83.372385ms: <nil>». El desenlace de `driven` es determinista (committed): fijarlo no afirma ningún número de reintentos.
4. Predicción, sin ejecutar: la liberación descarta el error del ROLLBACK (`:74-75`). Si fallara, el cerrojo seguiría hasta el Cleanup y el fallo se achacaría a la puerta. El riesgo que anota el pretest (20 × 5 s = 100 s «por arranque») no tiene en cuenta que cada store serializa sus arranques en una sola conexión (`store.go:1635`). Con un cerrojo sostenido serían 24 × 20 × 5 s ≈ 40 min por test, en un paquete que ya tarda 2066 s en local (`t7/quality.txt`) y 2587 s en CI Windows, con `-timeout 60m`. Esa cuenta es mía y no la he ejecutado.

## F4 · P3 · [TEST] · El veredicto del molde 2 se comprueba por texto, y ese texto ya lo trae el centinela

Afirmación. `authority_as07_test.go:137` acepta cualquier error cuyo texto contenga «ledger_busy». Si el caso `as07Busy` de `as07Check` (`driver:171-172`) se funde con `default`, el veredicto sigue nombrando busy gracias al mensaje de `ErrLedgerBusy`, y la clase propia del comprobador queda sin fijar (clases g e i). El comentario «wrapped the way the door wraps it» (`:113-115`) es cierto para la cadena (`authority_v2.go:706`, `ledger_identity.go:395`), pero no para el texto: el error sintético repite la frase del centinela y la puerta la dice una sola vez.

Reproducción:
1. B0, sin mutación: una sonda que registra el veredicto, con CMDm2 → «start 47 still ledger_busy after 20 attempts: action/sqlite: authority store busy: action/sqlite: ledger_busy: another writer held the ledger past the busy timeout: another writer held the ledger past the busy timeout: database is locked (5) (SQLITE_BUSY)».
2. B1: quitar `case as07Busy: return fmt.Errorf("start %d still ledger_busy …")` (`driver:171-172`). CMD3 → los tres `PASS`; el veredicto que registra la sonda queda en «start 47: action/sqlite: authority store busy: action/sqlite: ledger_busy: …».

## F5 · P3 · [TEST] · El oráculo del doble cobro que declara el pretest no existe en el test

Afirmación. `pretest.md:14` dice que un reintento que cobrase dos veces «lo delata el aserto del libro, spent == committed». El test afirma otra cosa, `spent == 12` (`authority_as07_test.go:42`). Con 48 arranques contra un presupuesto de 12, el libro satura en 12 tanto si hay doble cobro como si no. Quien lo detecta es el recuento. El pretest no está en el árbol, así que el defecto está en la evidencia declarada; la propiedad sí queda guardada.

Reproducción:
1. D1: en la copia de `authority_v2.go`, justo después del `tx.Commit()` correcto (`:1983-1985`), devolver la primera vez `ErrAuthorityStoreBusy` envolviendo `ErrLedgerBusy`. Es un commit real que se anuncia como busy: el conductor lo reintenta y ese arranque cobra dos veces.
2. CMDmain → `FAIL` con «committed=11 exhausted=37, want 12 and 36». Lo que ve la sonda en el libro: `spent=12 max_total=12`.
3. D2: D1 y además `t.Fatal(err)` de `:38-40` cambiado a `t.Log`. CMDmain → `PASS`: el aserto del libro no ve el doble cobro.

## F6 · P3 · [TEST] · «The busy retries are logged», pero en el gate esa línea nunca aparece en verde

Afirmación. El recuento sale por `t.Logf` (`:37`, `:94`; el comentario está en `:25-26`), y `go test` solo imprime el log de un test que pasa cuando se ejecuta con `-v`. `make quality` (`Makefile:155`, `:178`) y la CI (`quality.yml:205`, `:230`) se ejecutan sin `-v`, así que un arranque rescatado por el reintento en CI no deja rastro. Se cumple la letra del contrato («se registra en el log del test»), pero en la práctica el recuento solo se ve con `-v` o cuando el test falla.

Reproducción:
1. `go test -race -count=1 -run 'TestAuthority_ConcurrentStartsShareAncestorBudget|TestAuthority_AS07_aRealBusy' ./internal/action/sqlite/` en la copia sin mutar (I1). La salida entera es `ok  github.com/Sebastian197/korvun/internal/action/sqlite 16.747s`, aunque el molde 1 hizo un reintento (con `-v` imprime «busy retries across the 48 starts: 1»).

## F7 · P3 · [TEST] · La congelación por hash solo cubre la mitad del test

Afirmación. `freeze-as07.txt` contiene únicamente `authority_as07_test.go`. El veredicto (`as07Check`), la lectura del libro (`sharedAccount`), `runAll` y el fixture están en `authority_as07_driver_test.go`, que se editó durante el verde (ahí se añadió el reintento) y del que no se guardaron los bytes del rojo. Su único hash (`a620cb9a…`, en `as07-before-mut.sha`) es posterior al verde. Para esas funciones, «test congelado por hash tras el rojo» no se puede verificar. Lo único que el rojo deja ver es que el andamio ya tenía `as07Check` con la rama busy y `busyRetries` (textos de `as07-red.txt`).

## F8 · P3 · [DOC] · «retries … at most as07MaxAttempts times»

`authority_as07_test.go:19-21` habla de reintentos, pero el tope es de 20 intentos en total, es decir, 19 reintentos. `as07Drive` lo dice bien (`driver:106-108`, «at most maxAttempts times in all»), igual que el HANDOFF («un tope de 20 intentos»), y el molde 2 fija exactamente 20 intentos (`:132`). La frase no es falsa, porque «como mucho 20» incluye 19, pero no describe lo mismo que el código.

---

## Lo que resiste (con captura)

- La puerta no cambia. `git diff HEAD --stat` muestra solo el HANDOFF y el test; el único fichero nuevo es de test.
- Volví a ejecutar las mutaciones declaradas y todas salen rojas como dicen:
  - MU-D1: el molde 1 cae con «start 26 still ledger_busy after 1 attempts: … database is locked (5) (SQLITE_BUSY)» y el molde 2 con «busy after 1 attempts».
  - MU-D2: «exhausted after 20 attempts».
  - MU-D3: «busy after 21 attempts».
  - MU-D4: «the verdict = <nil>».
  - MU-P1: «committed=24 exhausted=24».
- El molde (3) del director, en su forma literal y más estrecha: saltar solo el débito del ancestro común (índice 2 de `resolved.accounts`, `authority_v2.go:1909`), con CMDmain → «committed=24 exhausted=24» (H1). La intención y la raíz tienen presupuesto 24 (`authority_phase3_test_helpers_test.go:62,79`), así que lo único que limita a 12 es el ancestro común.
- Lectura del libro. La sonda sobre el árbol sin mutar (C0) da 12 committed, 36 exhausted, `spent=12 max_total=12`, y un `max_total` de 12 solo puede ser el ancestro común. La clave `'*'` es la del contador total (`authority_v2.go:2381,2396`). Un `Scan` fallido termina en `t.Fatalf` y nunca se convierte en cero (`driver:197-202`). El CHECK del esquema (`store.go:607`, `spent>=0`) no acota `max_total − spent`; eso lo calcula el test.
- La cura no deja a AS07 ciego ante la pérdida de la propiedad de escritura. Sin el UPDATE de `authority_write_lock` (`authority_v2.go:664-666`) y sin `_txlock=immediate` (`store.go:96`), CMDmain con `-count=5` sale en rojo 5 de 5, con «start N still ledger_busy after 20 attempts» y entre 422 y 448 busy por pasada (W2x5). La versión de master también sale en rojo (W2).
- Tope sin límite (E1: `for r.attempts < maxAttempts` → `for`): el molde 2 solo cae por el timeout del paquete (`FAIL … 60.691s` con `-timeout 60s`) y no nombra busy. El autor no declaró esta mutación.
- Carreras: 24 ejecuciones con `-race` y ningún «DATA RACE». Cada goroutine escribe solo su propio índice, `calls` se lee después de `wg.Wait()` y las banderas del molde 1 viven en la goroutine del test.
- `go vet ./internal/action/sqlite/` en el árbol, para darwin, windows y linux: exit 0 en los tres.
- Hechos del HANDOFF y de los comentarios. La caída de CI (run 36520026213, 11.84s, «start 26: … (SQLITE_BUSY)», committed=12 exhausted=35) coincide con `scratchpad/t13/run-36520026213-log-failed.txt:361-365`. La local (20.67s, arranque 0, committed=12 exhausted=35) coincide con `scratchpad/t7/quality.txt:52-54`. Ningún comentario nuevo localiza por posición (grep).

## Preguntas obligatorias

- Test principal.
  - Garantía literal: 48 arranques sobre dos conexiones reales terminan en exactamente 12 committed y 36 exhausted, y la cuenta del ancestro nunca queda por debajo de cero.
  - Dónde está: `driver:136-181`, `:194-204`; `test:38-45`.
  - Se pone rojo si falta el débito del ancestro (H1 y MU-P1, ejecutadas) o si faltan las guardas de agotamiento (MU-P2, ejecutada; el rojo viene de «other» y el comentario lo cuenta mal, F1).
  - Nivel de evidencia: honesto (dos stores sobre un fichero real). El solape de las dos transacciones está permitido, no forzado; eso viene de antes de este delta.
- Molde 1.
  - Garantía literal: un busy real de la puerta en el arranque 26 no tumba el test y el test lo reintenta.
  - Dónde está: `test:59-101` y `driver:122-123`.
  - Se pone rojo con MU-D1. Falta una mutación que lo ponga rojo por sí solo: G1 lo deja verde (F3).
  - Nivel: honesto; medí 5.07 s de espera antes del busy.
- Molde 2.
  - Garantía literal: un busy más allá del tope se nombra busy después de exactamente 20 intentos, y los otros 47 dan 12/35.
  - Dónde está: `test:116-156`, `driver:110-131` y `:163-181`.
  - Se pone rojo con MU-D2, MU-D3, MU-D4 y G1. B1 sobrevive (F4).
  - Nivel: honesto (busy sintético; los otros 47 son reales).
- Conductor.
  - Garantía literal: solo se reintenta `ErrLedgerBusy`.
  - Dónde está: `driver:105-131`.
  - Ningún test se pone rojo si es falsa. Faltan A1 y A2 (F2).
- HANDOFF, punto 17 (`docs/HANDOFF.md:362-373`): los hechos coinciden con las capturas.

## Las nueve clases

- (a) No.
- (b) El libro se compara con 12, no con el recuento (F5).
- (c) El ROLLBACK de la liberación descarta su error (F3; predicción).
- (d) A1, A2, B1, G1 frente al molde 1, y D2.
- (e) «committed=48» (F1), el texto del error «wrapped» (F4) y «logged» (F6).
- (f) No.
- (g) El veredicto del molde 2 se comprueba por texto (F4).
- (h) «committed=48» y la cuenta de riesgo del pretest (F1, F3).
- (i) `:137` acepta tanto el busy del comprobador como un «other» cuyo texto dice busy (F4).

## Alcance

Leído:
- Evidencia del autor: el brief; el patch, comparado con el árbol; `pretest.md`, `as07-red.txt`, `as07-green.txt`, `as07-stability.txt`, `freeze-as07.txt`, `mutations-as07.jsonl` y `mutations-as07.json`, `as07-before-mut.sha`, `cross.txt`, `handoff.diff`, `rerun.txt` y `rerun-watch.txt`; `authority_as07_test.go.master`, cuyo blob coincide con el `fed7360` de HEAD (calculado); `t13/log-trimmed.txt` y `run-36520026213-log-failed.txt:361-365`; `t7/quality.txt:40-60`.
- En el árbol: los dos ficheros de test completos; `authority_v2.go` 1-120, 640-900, 1500-1567, 1860-2180, 2242-2291 y 2360-2480; `ledger_class.go` completo; `ledger_identity.go` 30-79, 180-270 y 360-420; `store.go` 80-110, 585-625, 1395-1560 y 1599-1658; `authority_phase3_test_helpers_test.go` completo; `ledger_identity_test.go` 25-54, 285-300 y 905-925; `docs/HANDOFF.md` 280-389; `Makefile` 150-179; las líneas `go test` de `quality.yml`; `go.mod` (go 1.26.6, variable de bucle por iteración).

Ejecutado:
- La línea base en el árbol y `go vet` para los tres sistemas.
- En la copia: A0, A1, A2, A2p, A1xP2, B0, B1, C0, C1, C2, D1, D2, E1, F1-timing, G1, H1, I1, R-MU-D1 a R-MU-D4, R-MU-P1, W2 (versión curada y versión de master) y W2x5.
- Para copiar y comparar hashes usé `rsync`, `patch`, `python3`, `shasum` y `cmp`; solo leen el árbol y escriben en mi scratch.

No verificado:
- golangci-lint y gofmt (fuera de la lista permitida) y `make quality` (prohibido).
- La CI real: sus hechos vienen de capturas del autor; no ejecuté `gh`.
- El resultado del rerun de 36520026213: `rerun-watch.txt` termina con el paso Test de Windows todavía en curso.
- El comportamiento de los moldes en Windows (solo `go vet` con GOOS=windows).
- Los bytes del driver en el estado rojo (F7).
- Las dos predicciones de F3 (el fallo del ROLLBACK y los ≈40 min).

# Segunda pasada — VETO LEVANTADO

VETO LEVANTADO

Segunda pasada, acotada a las curas de F1–F8 y a lo que cambió desde la primera.

Objeto: `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `as07-declared-cure`, HEAD `abb51d4975bec01dc38fd957df9d51d5de5f50fe`. El delta sigue sin commit.

Integridad de lo auditado, comprobado al empezar y al terminar:
- Hashes sha256: test `86070026…`, driver `e570a9fc…`, HANDOFF `57e899fd…`, `authority_v2.go` `9cae4ae7…`, `store.go` `37efa0d0…`.
- El diff de producción contra HEAD está vacío.
- Mi diff propio de `base-pass1` al árbol coincide con `t14/as07-since-verdict1.patch`, incluidas las cabeceras de hunk.
- `t14/as07-delta-2.patch` coincide con `git diff HEAD` más el fichero nuevo.
- `git status` es el mismo antes y después.

No he escrito nada en el árbol. Refresqué la copia: `adv-as07/base` y `adv-as07/mut` tienen ahora los bytes actuales, y los de la primera pasada quedan en `base-pass1`. Las mutaciones se aplican por reemplazo exacto y se restauran por sha256 (`adv-as07/mutate.py`).

Comandos:
- CMD4: `go test -race -count=1 -run 'TestAuthority_ConcurrentStartsShareAncestorBudget|TestAuthority_AS07_' -v ./internal/action/sqlite/`
- CMDmain: la misma orden con `-run '^TestAuthority_ConcurrentStartsShareAncestorBudget$'`
- CMDm1: la misma orden con `-run '^TestAuthority_AS07_aRealBusyOnOneStartIsRetriedByTheTest$'`

Línea base en el árbol con CMD4: `PASS (4.71s)`, `busy retries … 1`, `PASS (9.91s)`, `PASS (4.69s)`, `PASS (0.00s)`, `ok … 21.024s`. `go vet` para darwin, windows y linux: OK.

No quedan P1 ni P2. Los cuatro hallazgos nuevos son P3 y ninguno es [PRODUCT].

## Las curas, una por una

- **F1 · CURADO.** El párrafo de mutaciones (`authority_as07_test.go:29-37`) casa con las ejecuciones:
  - R2-MU-P1b (solo se salta el ancestro compartido, índice 2): `:45: committed=24 exhausted=24, want 12 and 36`.
  - R-MU-P1 de la primera pasada (solo se debita la hoja): 24.
  - R2-MU-P2: `:45: start 0: action/sqlite: budget evidence corrupt`.
  - C1 de la primera pasada: 13 committed y 35 `budget evidence corrupt`. C2 confirma que la guarda que corta es `remainingBeforeAccountTx`, porque al quitarla el error pasa a `authorization snapshot corrupt`.
  - R2-MU-P2b: `:55: … spent=13 of max_total=12 with 13 starts committed`.
  - R2-MU-P3b: `:55: … spent=12 of max_total=12 with 11 starts committed`.
- **F2 · CURADO.** El molde 3 (`:190-219`) existe y cumple:
  - R2-MU-A1 (cualquier error se reintenta): rojo en cuatro filas, `:215: the store busy from a deadline: busy after 20 attempts (20 busy, 20 calls), want other after 1 (0 busy)` y las de interrupted, corrupt budget y plain error.
  - R2-MU-A2 (reintento según `ErrAuthorityStoreBusy`): rojo solo en las filas de deadline e interrupted.
  - La fila del deadline (`:203`) reproduce la cadena real de la puerta cuando el contexto ya ha vencido al entrar (N-DL (i) más abajo).
  - Queda un residuo, que va en N1 y N2.
- **F3 · CURADO.**
  - `:111-113`: R2-MU-G1 pone en rojo el molde 1 en su propio aserto: `:112: start 26 after its retry = exhausted (1 attempts): … want committed`.
  - El error del ROLLBACK ya se comprueba (`:87-89`). N-REL (`ROLLBACK` → `ROLLBACKX`) da primero `:88: release the raw BEGIN IMMEDIATE: SQL logic error: near "ROLLBACKX": syntax error (1)` y luego `:112: start 26 after its retry = busy (20 attempts) …` (102.71 s). El fallo del arnés se nombra antes que el de la puerta. Además queda capturado lo que en la primera pasada era predicción: el cerrojo sigue vivo hasta el Cleanup.
  - El busy sigue siendo real (N-F1-timing-2): intento 1 del arranque 26 en `5.102858134s`, respuesta de la puerta `ledger_busy`; intento 2 en `82.142145ms`, `<nil>`.
- **F4 · CURADO.**
  - La clase está en `driver:49-52` y el caso busy la envuelve con `%w` (`driver:176-177`). El molde 2 la comprueba con `errors.Is` (`:158-161`).
  - R2-MU-B1: `:160: the verdict = start 47: action/sqlite: authority store busy: … (SQLITE_BUSY), want its busy class (errAS07StillBusy) …`.
  - El texto sintético (`:141-142`) ya es idéntico al de la puerta: el veredicto de R2-MU-B1 y el primer error real de N-F1-timing-2 dicen lo mismo, byte a byte.
  - Queda un residuo, en N3.
- **F5 · CURADO.** Está en `:47-57`, y R2-MU-P3b se pone en rojo en `:55`. Observación sin gravedad: con el recuento activo, el doble cobro cae en `:45` (`committed=11 exhausted=37`) tanto con la cláusula nueva como sin ella (R2-MU-P3-…-active y …-noclause). `spent == committed` va detrás de `as07Check`, que ya exige committed == 12, así que en el flujo que se embarca equivale a `spent == 12`. Solo aporta detección si se neutraliza el recuento, y el comentario ya lo dice. Por la misma razón no puede introducir un falso rojo: en la puerta, un commit implica `nil` (`authority_v2.go:1983-1991`, `Probe` nil).
- **F6 · CURADO** (`:26-27`).
- **F7 · CURADO.**
  - `freeze-as07-final.txt` coincide con los hashes actuales.
  - Volví a ejecutar `red-driver-proof.py` sobre el transcript (`adv-as07/red-proof-rerun.txt`) y da `byte-identical: True`.
  - Añadí algo que el script no mira: entre el heredoc del driver (línea 22861) y la captura roja (línea 22878) solo hay una orden más, la línea 22873. Esa orden reescribe el test (el heredoc da `0f2a7370…`) y ejecuta `gofmt -l` y `go vet`; ninguno de los dos escribe el driver.
  - Del driver rojo a la primera pasada solo cambia `as07Drive`. `as07Check`, `sharedAccount`, `runAll` y el fixture están intactos desde el rojo.
- **F8 · CURADO** (`:20-21`).

¿Se relajó algún aserto? No, lo comprobé en cada punto:
- La cláusula del libro se amplía.
- El molde 1 gana dos comprobaciones.
- En el molde 2, `errors.Is(err, errAS07StillBusy)` implica lo que antes pedía por texto. Es no-nil y contiene «ledger_busy», porque `%w` incluye el mensaje del centinela. Con `err == nil`, el `||` corta antes de llamar a `err.Error()`, y MU-D4 da `the verdict = <nil>`.
- El molde 3 es nuevo.

## Hallazgos nuevos

### N1 · P3 · [DOC] · El paréntesis del molde 3 sobre el deadline solo vale si el contexto ya venció al entrar

Afirmación: `authority_as07_test.go:180-183` pone «a deadline» como ejemplo de «its busy class without ledger_busy». Eso es así solo si el contexto ya está vencido al entrar en la puerta. Un deadline o una cancelación que llega mientras BEGIN IMMEDIATE espera vuelve como `ledger_busy` tras los 5 s completos, sin rastro del contexto en la cadena, y el conductor lo reintenta.

Reproducción:
1. Sonda en la copia, fichero temporal `adv2_probe_test.go` con `TestADV2_theDoorsDeadlineChains`, sobre el fixture de AS07. Tres casos:
   - (i) `StartAuthorization` con un contexto cuyo deadline pasó hace 1 s.
   - (ii) Una conexión cruda sostiene `BEGIN IMMEDIATE`, como en el molde 1, y `StartAuthorization` recibe `context.WithTimeout(…, time.Second)`.
   - (iii) El mismo cerrojo, con el contexto cancelado a 1 s.
   En cada caso compruebo `errors.Is` contra `ErrAuthorityStoreBusy`, `ErrLedgerBusy`, `DeadlineExceeded` y `Canceled`, y paso la respuesta por `as07Drive`.
2. `go test -race -count=1 -run '^TestADV2_' -v ./internal/action/sqlite/`:
   - (i) `action/sqlite: authority store busy: context deadline exceeded | ErrAuthorityStoreBusy=yes ErrLedgerBusy=no DeadlineExceeded=yes Canceled=no | driver: other after 1`
   - (ii) `after 5.091s: action/sqlite: authority store busy: action/sqlite: ledger_busy: another writer held the ledger past the busy timeout: database is locked (5) (SQLITE_BUSY) | ErrAuthorityStoreBusy=yes ErrLedgerBusy=yes DeadlineExceeded=no Canceled=no | driver: busy after 20`
   - (iii) `after 5.107s: … ledger_busy … | … Canceled=no | driver: busy after 20`

Sitios donde se forma la cadena:
- `beginWrite` (`ledger_identity.go:199-205`): BeginTx y luego `mapGuardError`. Los códigos 5 y 6 salen como `%w: %v` de `ErrLedgerBusy` (`:394-395`).
- `beginAuthorityWrite` (`authority_v2.go:659-662`).
- `mapAuthorityStoreError` (`:700-709`).

Por qué P3: leído como lista de ejemplos, el paréntesis no es falso. La fila `:203` casa exactamente con (i). Pero dice más de lo que hace la puerta en el caso realista, que es el deadline que vence durante la espera. En AS07 no tiene efecto, porque usa `context.Background()`. Además corrige mi F2 de la primera pasada: allí afirmé, solo leyendo, que «un contexto que acaba (`:705`)» llega sin `ErrLedgerBusy`. Eso solo es cierto si ya ha acabado al entrar.

### N2 · P3 · [TEST] · El molde 3 fija solo una de las dos cadenas busy de la puerta

Reproducción:
1. N-A3: en `driver:127`, `case errors.Is(r.err, ErrLedgerBusy):` → `case errors.Is(r.err, ErrLedgerBusy) && errors.Is(r.err, ErrAuthorityStoreBusy):`.
2. CMD4: los cuatro tests en `PASS` (`ok … 21.294s`).

Las dos cadenas:
- El busy de BEGIN llega como `ErrAuthorityStoreBusy{ErrLedgerBusy}`. Es la que usan las filas, y el primer error real del molde 1 lo confirma.
- El busy de una sentencia dentro de la transacción llega sin envoltorio, como `ErrLedgerBusy{raw}`, vía `txExec` (`ledger_identity.go:268-271` → `:394-395`), y `StartAuthorization` lo devuelve tal cual (por ejemplo `authority_v2.go:1949-1951`). W2 de la primera pasada capturó esa forma: `start 6 still ledger_busy after 20 attempts: action/sqlite: ledger_busy: … (SQLITE_BUSY)`, sin «authority store busy».

`:183` promete «ErrLedgerBusy is attempted again», pero eso solo queda fijado para la cadena doble.

Por qué P3: estrechar así no esconde defectos, porque un busy sin envoltorio acabaría en «other» y en rojo. Además, en la puerta sin mutar esa cadena es casi inalcanzable dentro de una transacción IMMEDIATE: solo la he visto quitando la propiedad de escritura.

### N3 · P3 · [TEST] · Nada impide que el comprobador ponga la clase busy a un veredicto que no lo es

Reproducción:
1. N-B2: el `default` de `driver:178-179` pasa a `return fmt.Errorf("start %d: %w (%d): %v", i, errAS07StillBusy, r.attempts, r.err)`.
2. CMD4: los cuatro tests en `PASS` (`ok … 21.231s`).
3. N-B2 junto con MU-P2 (sobregasto en producción), con CMDmain: `authority_as07_test.go:45: start 0: still ledger_busy after the test's attempts (1): action/sqlite: budget evidence corrupt`.

El godoc (`driver:49-51`) dice que es la clase de un arranque que seguía recibiendo `ErrLedgerBusy`. La cura de F4 creó la clase y fija solo que aparece en el veredicto busy (molde 2), no que falte en los demás.

Por qué P3: el test sigue en rojo y lo único falso es la etiqueta. Aun así, una corrupción anunciada con las palabras del flake conocido invita justo al relanzamiento con el que empezó este tren.

### N4 · P3 · [DOC] · «the adjacent mould» (`authority_as07_test.go:110`) se lee como una posición

Se refiere al molde 2, cuyo título es «the adjacent attack» (`:124`), el término del director. La ley pide nombrar la cosa: el molde 2, o `TestAuthority_AS07_busyPastTheCapOnTheLastStartIsNamedBusy`. No encontré ninguna otra referencia posicional nueva: grep de above, below, next, previous, adjacent, following y earlier. «above all» y «the store's next guard (remainingBeforeAccountTx)» nombran, no sitúan.

## Fuera del delta (para fichar; no bloquea este tren)

La puerta no acorta la espera de BEGIN IMMEDIATE cuando vence el deadline o llega la cancelación del llamante. Espera el `busy_timeout` completo (5.091 s y 5.107 s frente a 1 s) y responde `ledger_busy`, sin el error del contexto en la cadena (N-DL (ii) y (iii)). No lo introduce este delta y no aparece en el HANDOFF (grep de deadline y cancel). Mi hipótesis del mecanismo, sin verificar: el manejador de busy de SQLite no atiende `sqlite3_interrupt`.

## Lo que esta pasada vio y la anterior no

- La cadena real de la puerta ante un deadline o una cancelación durante la espera (N1), con corrección de mi propia F2 de la primera pasada.
- La segunda cadena busy, sin envoltorio, frente al molde 3 (N2).
- La superficie nueva que abre la cura de F4 en la rama `default` del comprobador (N3).
- Que `spent == committed` va implícito en el flujo real (observación, en F5).
- «the adjacent mould» (N4).
- La comprobación de las órdenes intermedias en la prueba del rojo (F7).

Observación: por los tiempos (el test principal tarda 0.32–0.50 s en `mutations-as07-final.jsonl`, frente a unos 4.7 s con `-race`), las mutaciones finales del autor parecen ejecutadas sin `-race`. Es una inferencia. Yo las repetí con `-race` (A1, A2, G1, B1, P2, P2b, P1b, P3b con su global sin sincronizar y una variante atómica) y todas se ponen en rojo igual. Ninguna de mis 18 ejecuciones de esta pasada dio `DATA RACE`.

## Preguntas obligatorias

- **Test principal.** Cable: `:41-57`. Se pone en rojo con P1, P1b y P2. El aserto del libro se pone en rojo aislado con P2b y P3b. No falta ninguna mutación. Nivel de evidencia honesto.
- **Molde 1.** Cable: `:72-122`. Se pone en rojo con D1 y G1 en su propio aserto (`:112`), y la liberación queda observada (N-REL). Nivel honesto: 5.10 s medidos.
- **Molde 2.** Cable: `:138-178`. Se pone en rojo con D2, D3, D4 y B1. Nivel honesto.
- **Molde 3.** Cable: `:190-219` y `driver:115-136`. Se pone en rojo con A1, A2, D1 y G1. Faltan A3 (N2) y la cadena de deadline durante la espera (N1). La etiqueta «unit, in process» es honesta.
- **HANDOFF, punto 17** (`docs/HANDOFF.md:362-374`). Cada dato casa con su captura:
  - la caída local: `t7/quality.txt:52-54` (20.67 s, arranque 0);
  - la primera de CI: `t13/run-36520026213-log-failed.txt:361-365` (11.84 s, arranque 26);
  - el relanzamiento: `t14/rerun-windows.log:362-364` (14.22 s, arranque 1, committed=12 exhausted=35; el runner arrancó 2026-09-29T18:01:18Z).
  Las otras menciones de un FAIL de AS07 en el scratchpad (t8, t9, t12) son la misma caída de 20.67 s.

## Las nueve clases

- (a) no;
- (b) no;
- (c) no: el ROLLBACK ya se comprueba;
- (d) N2, N3 y la cláusula `spent == committed`, redundante en el flujo real;
- (e) N1;
- (f) no;
- (g) no: el veredicto ya va por clase;
- (h) no: toda cifra del comentario tiene su captura;
- (i) no.

## Alcance

Leído:
- De la evidencia: `adversary-brief-2.md`, `as07-since-verdict1.patch`, `as07-delta-2.patch`, los tres ficheros actuales completos, `freeze-as07{,-2,-3,-final}.txt`, `as07-before-mut{2,3,-final}.sha`, `mutations-as07-3.json` (las 13 definiciones), `mutations-as07-final.jsonl` con su resumen, `as07-stability-final.txt`, `cross-final.txt`, `as07-green2.txt`, `red-driver-proof.py` y su salida, `authority_as07_driver_test.go.red`, `rerun-windows.log` (cabecera y 355-370), `rerun-job-id.txt` y `rerun-watch.txt` (el ID del job).
- Del código: `ledger_judge_seams.go:278-291` (`stageFault`), `ledger_identity.go:197-271` y `384-396`, y los tramos de `authority_v2.go` de la primera pasada.
- Del transcript de la sesión (solo lectura): las líneas 22855-22878.

Ejecutado:
- Línea base y `go vet` en el árbol.
- En la copia: N-A3, N-B2, N-B2xP2, N-DL, N-F1-timing-2, N-REL, R2-MU-A1, R2-MU-A2, R2-MU-G1, R2-MU-B1, R2-MU-P2, R2-MU-P2b, R2-MU-P1b, R2-MU-P3b (en cuatro variantes), R2-MU-P3-active y …-noclause.
- La prueba del rojo, vuelta a ejecutar.

No verificado:
- gofmt, golangci-lint y `make quality`.
- La CI real: sus datos salen de capturas del autor; no ejecuté `gh`.
- Una sentencia interrumpida de verdad en la puerta: la fila «interrupted» (`:204`) se apoya solo en la lectura de `mapAuthorityStoreError` (`authority_v2.go:705`), porque mi cancelación durante la espera dio busy, no interrupted.
- El mecanismo de N1 (hipótesis).
- El informe al director donde el autor dice corregir el pretest, que no está en el árbol.

# Tercera pasada — VETO LEVANTADO

VETO LEVANTADO

Tercera pasada, acotada a las curas de N1–N4 y a lo que cambió desde la segunda.

Objeto: `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `as07-declared-cure`, HEAD `abb51d4975bec01dc38fd957df9d51d5de5f50fe`. El delta sigue sin commit.

Integridad de lo auditado, comprobado al empezar y al terminar:
- Hashes sha256: test `95587319…`, driver `789d9c54…`, HANDOFF `5bdd512e…`, `authority_v2.go` `9cae4ae7…`, `store.go` `37efa0d0…`.
- Coinciden con `t14/freeze-as07-final2.txt` y `t14/final2-hashes.txt`.
- El diff de producción contra HEAD está vacío.
- Mi diff propio de `base-pass2` al árbol es idéntico byte a byte a `t14/as07-since-verdict2.patch`.
- `t14/as07-delta-3.patch` coincide con `git diff HEAD` más el fichero nuevo.
- `git status` es el mismo antes y después.

No he escrito nada en el árbol. Refresqué la copia: `adv-as07/base` y `adv-as07/mut` tienen ahora los bytes actuales, y los de la segunda pasada quedan en `base-pass2` y `mut-pass2`. Cada mutación se restaura por sha256.

Línea base en el árbol, `go test -race -count=1 -run 'TestAuthority_ConcurrentStartsShareAncestorBudget|TestAuthority_AS07_' -v ./internal/action/sqlite/`: cinco `PASS` (4.83s, 9.93s, 4.63s, 0.00s, 0.00s), `ok … 21.050s`. `go vet` para darwin, windows y linux: OK. En las 15 ejecuciones de esta pasada, todas con `-race`, no apareció ningún `DATA RACE`.

No quedan P1 ni P2. Los dos hallazgos son P3 [DOC], y ninguno es [PRODUCT].

## Las curas

- **N1 · CURADO.**
  - El comentario del molde 3 (`authority_as07_test.go:182-189`) ya no dice «a deadline»: dice «a context already over when the start begins».
  - La fila correspondiente (`:212`) reproduce exactamente la cadena que capturé en la segunda pasada, N-DL (i): `action/sqlite: authority store busy: context deadline exceeded`.
  - El árbol no describe mis capturas (ii) y (iii). Siguen siendo un asunto fuera del delta, para el director.
- **N2 · CURADO.**
  - La fila sin envoltorio (`:200`, `:210`) tiene la forma de `txExec` → `mapGuardError`, es decir `%w: %v` sobre `ErrLedgerBusy` (`ledger_identity.go:268-271`, `:394-395`). Su texto coincide con el que capturé en W2 de la primera pasada.
  - R3-MU-A3 (reintentar solo si están las dos clases) da rojo en esa fila y solo en ella: `:225: a busy statement, bare, then committed: other after 1 attempts (0 busy, 1 calls), want committed after 2 (1 busy)`.
- **N3 · CURADO.** El molde 4 (`:241-278`) cubre las cuatro ramas de `as07Check` y también el orden:
  - R3-MU-B2 (la clase busy en todo arranque fallido) da rojo en la fila «other»: `start 5: still ledger_busy after the test's attempts (1): action/sqlite: budget evidence corrupt, want … busy class=false`.
  - R3-MU-B1 (el veredicto busy metido en «other») da rojo en la fila busy.
  - R3-MU-C1 (sin comparación de cuentas) da rojo en la fila de cuentas: `the verdict = <nil>, want failing=true`.
  - Mutación nueva mía, N-O1 (comparar las cuentas antes de nombrar los arranques que fallan): rojo en el molde 4 (`a start still busy: the verdict = committed=11 exhausted=36 …` y lo mismo en la fila «other») y en el molde 2 (`:162: the verdict = committed=12 exhausted=35 …`).
  - El rojo de `t14/as07-red-n.txt` es real. Además lo comprobé en el transcript: después de la captura roja (línea 23694, 19:40:10Z) no hay ninguna llamada a herramienta hasta la 23706. Esa orden hace la congelación (`freeze-as07-n.txt`) como su primera acción, después copia el driver rojo y luego lo edita.
  - El driver actual es exactamente el `.red-n` (`e570a9…`, que es el de la segunda pasada) más las dos ediciones del verde de esa línea. Lo reconstruí y da `789d9c5477c7…`, igual al del árbol.
- **N4 · CURADO.** La línea `:110-112` dice ahora «this mould too, not only mould 2». No encontré ninguna referencia posicional nueva (grep de above, below, adjacent, next, previous, following y here). «the adjacent attack» (`:126`) es el nombre del molde 2 y «here» (`:128`) se refiere al propio escenario.

Lo demás que cambió, comprobado:
- **Mutaciones nuevas del molde 1**, que declara el párrafo `:66-70`. Todas las ejecuté con `-race`:
  - R3-MU-G1: `:114: start 26 after its retry = exhausted (1 attempts): … want committed`.
  - R3-MU-G3: `:114: … = other (1 attempts) …`.
  - R3-MU-G2: `:119: committed=13 exhausted=35, want 12 and 36`.
  - R3-MU-L1: `:108: start 26's first attempt behind the held lock = <nil> …`.
  - R3-MU-R1: `:90: release the raw BEGIN IMMEDIATE: SQL logic error: cannot rollback - no transaction is active (1)`.
  - R3-MU-R1c, el control: `PASS`.
- **MU-P3b del autor:** ahora usa `atomic.Int32` con su import (lo leí en `mutations-as07-final2.json`).
- **HANDOFF** (`:370-374`): «y el comprobador, que solo da la clase busy a un arranque que sigue en busy». Es cierto, y el molde 4 lo fija con MU-B2.

¿Se relajó algún aserto? No:
- Las filas del molde 3 cambian de nombre pero conservan los mismos asertos, y se añaden dos.
- El molde 1 solo cambia en comentarios.
- En el molde 2 y en el test principal, el paso de `%v` a `%w` no toca ningún texto. N-TXT da el mismo veredicto, byte a byte, con los bytes de la segunda pasada y con los de ahora: `"start 47: still ledger_busy after the test's attempts (20): action/sqlite: authority store busy: action/sqlite: ledger_busy: another writer held the ledger past the busy timeout: database is locked (5) (SQLITE_BUSY)"`. El `«exhausted=»` que busca `:161` ve los mismos bytes.

¿Puede el veredicto busy con varios `%w` ocultar algo? La sonda N-MW, con cada caso en un hueco de «exhausted», muestra lo que lleva cada veredicto:
- busy: `stillBusy=true ledgerBusy=true storeBusy=true`;
- «other» por un contexto ya vencido: `"start 20: action/sqlite: authority store busy: context deadline exceeded" | stillBusy=false ledgerBusy=false storeBusy=true deadline=true`;
- presupuesto corrupto: solo `corrupt=true`.

Ningún veredicto que no sea busy lleva `errAS07StillBusy`, y MU-B2 lo vigila. Los moldes 2 y 4 comprueban por clase. No encontré nada oculto.

## Hallazgos

### N5 · P3 · [DOC] · El molde 3 declara tres filas rojas para MU-A2, y la ejecución da cuatro

Afirmación: `authority_as07_test.go:191-194` dice «the retry keyed on ErrAuthorityStoreBusy instead of ErrLedgerBusy → reddens on the three rows of that class without ledger_busy». Con esa mutación también cae la fila sin envoltorio, porque ese busy no lleva `ErrAuthorityStoreBusy` y deja de reintentarse.

Reproducción:
1. En `driver:127`, `case errors.Is(r.err, ErrLedgerBusy):` → `case errors.Is(r.err, ErrAuthorityStoreBusy):`.
2. `go test -race -count=1 -run 'TestAuthority_AS07_theDriverRetriesOnlyLedgerBusy|TestAuthority_AS07_theCheckerNamesEachClass' -v ./internal/action/sqlite/` → cuatro filas en `:225`:
   - `a busy statement, bare, then committed: other after 1 attempts (0 busy, 1 calls), want committed after 2 (1 busy)`
   - `the store busy from a context already over: busy after 20 attempts (20 busy, 20 calls) …`
   - `the store busy from a raw busy its own retry window gave up on: busy after 20 attempts …`
   - `the store busy from an interrupted statement: busy after 20 attempts …`

La captura del autor (`mutations-as07-final2-summary.txt`, MU-A2) muestra esas mismas cuatro filas.

Por qué P3: la frase no es falsa, porque esas tres filas sí caen. Pero el desenlace declarado de una mutación probatoria no coincide con el capturado: le falta una fila. Es la vara de F1, en pequeño.

### N6 · P3 · [DOC] · El borrador del commit pone un solo nivel de evidencia a los cuatro moldes y vuelve a decir «logged» sin alcance

Afirmación:
- `t14/commit-c-msg.txt:30`, «Evidence level: real connections in process.», va justo detrás de la lista de cuatro moldes (`:21-28`). Pero los moldes 3 y 4 son unitarios, en proceso y sin conexión (`authority_as07_test.go:196`, `:240`), y el último arranque del molde 2 es sintético (`:137-139`).
- `:18-19`, «The busy retries are logged and never asserted.», no lleva el alcance que el árbol ya da (`:26-27`, «go test shows them with -v, or when the test fails»).

No está en el árbol, pero entra en la historia con el commit.

Reproducción: leer `commit-c-msg.txt:18-19` y `:21-30` frente a `authority_as07_test.go:26-27`, `:137-139`, `:196` y `:240`.

Por qué P3: las etiquetas del árbol son honestas; el borrador las generaliza en un artefacto público (punto 6 de la doctrina). No encontré atribuciones en el borrador (lo leí entero con `cat -n`).

## Observaciones sin gravedad

- **Un resultado nunca conducido** (valor cero) produce ahora `"start 20: %!w(<nil>)"` (N-MW). Antes salía `"start 20: <nil>"`. Sigue fallando, así que es cerrado, y solo lo alcanzaría un defecto del arnés. Es cosmético.
- **La fila «a raw busy its own retry window gave up on».** Por lectura, la forma casa con `authority_v2.go:664-679` y `:700-709`. Sin embargo, todo opener que escribe usa `buildWriterDSN`, con `_txlock=immediate` (`store.go:96`, `:1628`); los que usan `buildFileDSN` (`:1578`, `:2359`) están sellados con `query_only`. Así que el UPDATE corre con el cerrojo de escritura ya tomado, y no encontré manera de que ese sitio llegue hoy a busy. Es razonamiento, no ejecución. La fila fija cómo trata el conductor esa forma, que es para lo que está, y el comentario la acota con «as the rows build it».

## Lo que esta pasada vio y las anteriores no

- N5: el desenlace de MU-A2 declarado en el texto nuevo del molde 3 se queda corto.
- N6: el borrador del commit, que es un artefacto nuevo.
- El orden de `as07Check` (nombrar antes de contar) no estaba declarado como mutación. N-O1 lo prueba y los moldes 2 y 4 lo cazan.
- La captura de que `%v` → `%w` no cambia ningún texto (N-TXT, con los dos juegos de bytes).
- Lo que lleva cada veredicto tras los `%w` (N-MW), incluido el `%!w(<nil>)` del resultado nunca conducido.
- La comprobación en el transcript de la congelación tras el rojo del molde 4, y la reconstrucción del driver final a partir del `.red-n`.
- Que el sitio del «30 s loop gave up» es inalcanzable con el DSN de escritura (solo razonamiento).

## Alcance

Leído:
- `adversary-brief-3.md`, `as07-since-verdict2.patch`, `as07-delta-3.patch`.
- Los dos ficheros de test completos.
- `freeze-as07-n.txt`, `freeze-as07-final2.txt`, `final2-hashes.txt`, `as07-before-mut-final2.sha`, `as07-red-n.txt`, `as07-green-n.txt`, `as07-stability-final2.txt`, `cross-final2.txt`.
- `mutations-as07-final2.json` (las 21 definiciones), el jsonl con su resumen, `commit-c-msg.txt`, `do-commit-c.sh`, el principio de `build-verdict-record.py`, `authority_as07_driver_test.go.red-n`.
- Del código: `store.go:1560-1598` y `:2320-2372`, `ledger_identity.go:268-309`, `identity_phase1_test.go:27-57`.
- Del transcript de la sesión (solo lectura): las líneas 23690-23712.

Ejecutado:
- La línea base y `go vet` en el árbol.
- En la copia: R3-MU-A2, A3, B1, B2, C1, G1, G2, G3, L1, R1 y R1c; N-O1; N-MW; N-TXT con los bytes de ahora y con los de la segunda pasada.
- La reconstrucción del driver final.

No verificado:
- gofmt, golangci-lint y `make quality`.
- Las 21 ejecuciones del autor: solo repetí las once de arriba.
- Que la fila «interrupted» y la del «loop gave up» ocurran en la puerta real: solo por lectura.
- El informe al director sobre mis capturas (ii) y (iii), que no está en el árbol.

# Cuarta pasada — VETO LEVANTADO

VETO LEVANTADO

Cuarta pasada, mínima: las curas de N5 y N6 de mi tercera pasada.

Objeto: `/Users/sebastianmorenosaavedra/Desktop/korvun-tag0161.nosync`, rama `as07-declared-cure`, HEAD `abb51d4975bec01dc38fd957df9d51d5de5f50fe`. El delta sigue sin commit.

Integridad de lo auditado, comprobado al empezar y al terminar:
- Hashes sha256: test `ebf937aa…`, driver `789d9c54…`, HANDOFF `5bdd512e…`, `authority_v2.go` `9cae4ae7…`, `store.go` `37efa0d0…`. Test y driver coinciden con `t14/freeze-as07-final3.txt`.
- El diff de producción contra HEAD está vacío.
- `t14/as07-delta-4.patch` coincide con `git diff HEAD` más el fichero nuevo.
- `git status` es el mismo antes y después.

No he escrito nada en el árbol. Las copias de la tercera pasada quedan en `adv-as07/base-pass3` y `mut-pass3`; `base` y `mut` tienen ahora los bytes actuales.

El delta es solo ese párrafo. Comparé mi copia de la tercera pasada con el árbol entero (sin node_modules, .git, website ni coverage.out) y el único fichero distinto es `internal/action/sqlite/authority_as07_test.go`. Dentro de él solo cambian tres líneas del párrafo de mutaciones del molde 3 (`:192-194`, dentro del párrafo `:191-194`). El diff es idéntico a `t14/as07-since-verdict3.patch`.

Línea base en el árbol, `go test -race -count=1 -run 'TestAuthority_ConcurrentStartsShareAncestorBudget|TestAuthority_AS07_' -v ./internal/action/sqlite/`: cinco `PASS` (4.70s, 10.23s, 4.65s, 0.00s, 0.00s), `ok … 21.239s`. `go vet` para darwin, windows y linux: OK.

## Las curas

- **N5 · CURADO.** `authority_as07_test.go:191-194` dice ahora: «reddens on the bare row and on the three rows of that class without ledger_busy; the retry requiring both classes → reddens on the bare row alone». Lo repetí sobre los bytes finales con `go test -race -count=1 -run 'TestAuthority_AS07_theDriverRetriesOnlyLedgerBusy|TestAuthority_AS07_theCheckerNamesEachClass' -v ./internal/action/sqlite/`:
  - R4-MU-A2 (en `driver:127`, `errors.Is(r.err, ErrLedgerBusy)` → `errors.Is(r.err, ErrAuthorityStoreBusy)`) da rojo en cuatro filas de `:225`: `a busy statement, bare, then committed: other after 1 attempts (0 busy, 1 calls) …`, `the store busy from a context already over: busy after 20 attempts …`, `the store busy from a raw busy its own retry window gave up on: busy after 20 attempts …` y `the store busy from an interrupted statement: busy after 20 attempts …`.
  - R4-MU-A3 (exigir las dos clases) da rojo solo en `a busy statement, bare, then committed: other after 1 attempts (0 busy, 1 calls), want committed after 2 (1 busy)`.
  - El comentario casa exactamente con ambas capturas y con las del autor (`t14/mutations-as07-final3-summary.txt`, MU-A2 y MU-A3).
  - La serie final del autor, 21 mutaciones sobre los bytes finales, se ejecutó con `-race`. Lo confirman los tiempos del test principal en `mutations-as07-final3.jsonl`: de 3.04 a 4.81 s en las MU-P; sin `-race` rondaba 0.3–0.5 s. `as07-stability-final3.txt` da 15 de 15 en `PASS` con `-race -count=3`.
- **N6 · CURADO.** Comprobado contra el borrador `t14/commit-c-msg.txt`:
  - `:30-32` da ya un nivel por molde: conexiones reales en proceso para el test y los moldes 1 y 2 («mould 2's last start answers through a lying attempt»), y unitario en proceso para los moldes 3 y 4. Casa con las etiquetas del árbol (`authority_as07_test.go:39`, `:72-73`, `:137-139`, `:196`, `:240`).
  - `:18-19` acota el registro: «logged, shown by go test with -v or when the test fails, and never asserted». Casa con `:26-27`.
  - Leí el borrador entero y no contiene atribuciones.

## Lo que se declara sin cambiar

- **El `%!w(<nil>)` del resultado nunca conducido:** de acuerdo. Es cosmético y falla igual; solo lo alcanzaría un defecto del arnés.
- **La fila «a raw busy its own retry window gave up on»:** de acuerdo en mantenerla. Fija cómo trata el conductor esa forma, y «as the rows build it» la acota. Que el sitio sea hoy inalcanzable es razonamiento mío, no una ejecución, y así lo dice el informe.
- **El registro literal de mis pasadas.** Recalculé el sha256 de mis tres mensajes de entrega desde mi propia transcripción (`agent-a5e5a46a82db1106b.jsonl`): `582095f0…`, `7d62fbd8…`, `947fdb49…`. Son iguales a los de `t14/dry-record.md`, y cada cuerpo aparece allí byte a byte.
  - Los dos localizadores relativos son míos: «(N-DL (i) más abajo)» (`dry-record.md:210`, de mi segunda pasada) y «las once de arriba» (`dry-record.md:472`, de mi tercera).
  - Estoy de acuerdo en no editarlos. Editar rompería la custodia por sha256. Además, dentro de un registro congelado no pueden pudrirse: cada uno apunta dentro de su propio mensaje, que no cambia.
  - Aun así, esas dos frases no cumplen la letra de la ley. Es un defecto de forma mío, declarado y sin gravedad contra el delta. Este veredicto no usa ninguno.

## Hallazgos

Ninguno.

## Lo que esta pasada vio y las anteriores no

- La verificación byte a byte del registro literal contra mi propia transcripción.
- La confirmación, por los tiempos del test principal, de que la serie final del autor corrió con `-race`.
- Las cuatro filas rojas de MU-A2 y la única de MU-A3, capturadas de nuevo sobre los bytes finales.
- La comprobación, sobre la copia entera, de que el delta de esta vuelta es solo ese párrafo.

## Alcance

Leído: `adversary-brief-4.md`, `as07-since-verdict3.patch`, `as07-delta-4.patch`, `freeze-as07-final3.txt`, `as07-before-mut-final3.sha`, `mutations-as07-final3-summary.txt` y su jsonl (tiempos), el principio de `mutations-as07-final3.progress`, `as07-stability-final3.txt`, `commit-c-msg.txt` entero, la cabecera y los localizadores de `dry-record.md`, y `authority_as07_test.go:180-197`.

Ejecutado: la línea base y `go vet` en el árbol; R4-MU-A2 y R4-MU-A3 en la copia; el recálculo de los sha256 del registro desde mi transcripción.

No verificado:
- gofmt, golangci-lint y `make quality`.
- Las otras 19 mutaciones del autor sobre los bytes finales. El código no cambia desde mi tercera pasada, donde repetí once de ellas; solo cambió ese comentario.
- La CI real.
