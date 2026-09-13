# v0.15.0 — Aprobaciones en la app: diseño de experiencia (UX)

> **Estado:** APROBADA (v36) — VETO LEVANTADO en la pasada 35 sobre la clase
> GARANTÍA/COMPORTAMIENTO, tras TREINTA Y CINCO pasadas adversariales
> (2026-09-08/09). Las cinco adjudicaciones de §17 A-E, resueltas.
> **§17 F: las VEINTICINCO filas, AUTORIZADAS EN BLOQUE por el director
> (2026-09-09).** No queda ningún `[NEEDS CLARIFICATION]`. Condición vigente:
> **cada molde tocado se re-ejecuta con su mutación**, y el canto declara las
> veinticinco con su porqué en una tabla. **Corte: NO se parte** — un solo
> tren con el papel entero (adjudicación del director, §17 H). La garantía
> CENTRAL del tren es **FR-UI-62**, por encima de la geometría y del armado, y
> su escenario a dos conexiones reales (AS-83) es criterio de aceptación
> innegociable.
> **Ley (CLAUDE.md — UX-DESIGN-FIRST + MANOS-DE-CHANO):** no abre RED sin el
> sí del director mirando. Maqueta: `design-drafts/approvals-screen-ux.html`,
> ventana **1100×760** (la que abre el binario), tokens reales.
> **Papel rector:** `docs/superpowers/specs/2026-09-08-approvals-in-the-app-pre-review.md`.
> **Petición de diseño:** `docs/superpowers/specs/2026-09-08-approvals-screen-design-request.md`.
> **Base:** `c6ccd24`. `internal/controlapi/approvals.go` no existe todavía y
> el paquete no compila: todo lo que este documento dice del RED es lectura de
> su contrato, no observación.

## 0. La decisión del director, literal

2026-09-08: **propuesta 1 (read-through gate) con la geometría y el escape de
la propuesta 3**, y las siete decisiones del informe confirmadas.

| Pregunta | Decisión |
|---|---|
| 1 · dónde va el digest | La petición es un DOCUMENTO: digest primer bloque con su frase, parámetros literales, clase de efecto destacada, decisión al FINAL. El digest queda FIJADO en la barra superior mientras se decide. |
| 2 · qué se interpone | **Armado por tecleo**: reteclear los SEIS últimos caracteres del digest con confirmación visible, graduado por clase de efecto (§3 y §17 A). |
| 3 · aprobar vs rechazar | **Asimetría por coste del error**: Rechazar grande, cercano, siempre a un clic; Aprobar pequeño, lejano, deshabilitado hasta armarse. Tamaño y distancia, no color. Esc rechaza mientras la petición esté abierta y sin decidir. |

## 1. Objetivo

Después de esta pieza, una acción aparcada se lee y se decide dentro de la
ventana. Fuera: editar lo aparcado, lotes, avisos, móvil, web, y el reanudado
de una aprobación consumida (§10).

## 2. Dónde vive

- **FR-UI-1** — Entrada nueva en la barra lateral, **«Aprobaciones»**, entre
  «Actividad» y «Ajustes» (`App.tsx`, `NAV`), **con su icono SVG propio** en
  `components/icons.tsx`: `App.test.tsx` exige que «every nav item carries its
  real design icon (svg), not a placeholder dot». Rompe dos guardas vigentes que
  este tren actualiza: `e2e/chrome.spec.ts` («six sections in design order») y
  `App.test.tsx`.
- **FR-UI-2** — Sin contador ni punto: `data-unread` no se reutiliza.
- **FR-UI-3** — La entrada **sigue visible** con las aprobaciones apagadas.
- **FR-UI-4** — Esta vista **no escribe** en el `StatusChip` global, que sigue
  diciendo lo suyo («En marcha» / «Detenido» / «Sin configurar», más el puerto
  y el nombre del fichero de config — `components/StatusChip.tsx`).

## 3. La graduación por clase de efecto

Escalera y ley fail-closed: `internal/action/effect.go`.

**Verdad de hoy, que manda sobre la tabla.** `effectGateRule`
(`internal/brain/effects.go`) aparca **solo** si se cumplen las dos:

1. el cerebro tiene techo (`ceiling != ""`) y la clase **no lo supera**
   (`class.Rank() > ceiling.Rank()` deniega antes, con `effect_ceiling`);
2. la clase es `write_irreversible` o `critical`.

De ahí: para aparcar una irreversible el techo tiene que rankear ≥
`write_irreversible`; para aparcar una crítica, el techo tiene que ser
`critical`. Con un techo `pure`, `read_external`, `write_reversible` o
`write_compensatable` **nada aparca nunca**: se deniega.

| `effect_class` | Cartel | Frase | Aprobar |
|---|---|---|---|
| `write_irreversible` | **IRREVERSIBLE** | «escribe sin deshacer y sin compensación conocida» | tecleo |
| `critical` | **CRÍTICO** | «mueve dinero, credenciales o equivalente» | tecleo |
| cualquier otra de la escalera | **ANOMALÍA · &lt;clase&gt;** | ver FR-UI-5 | tecleo |
| valor fuera de la escalera | **CLASE DESCONOCIDA** | «fuera de la escalera: se trata por encima de crítico» | tecleo |
| el preview no se puede leer | **SIN CLASE LEGIBLE** | «no se ha podido leer la previa de esta petición; ábrela para ver qué dice el almacén» | no se ofrece decisión en la lista |

- **FR-UI-5 (anomalía)** — Una fila PENDING cuya clase no sea
  `write_irreversible` ni `critical` no la puede generar la producción de hoy.
  La pantalla la marca con esta frase exacta — «Esta petición no debería
  existir: el gate solo aparca irreversible y crítico. Trátala como
  sospechosa.» — y **exige tecleo**. Cuando el gate se ensanche, la graduación
  del director (un clic para reversible y compensable) entra retirando esta
  regla, no antes. **Adjudicado (§17 A):** sí, y con esas palabras — al
  estado más sospechoso no se le da la puerta más barata.
- **FR-UI-6** — La puerta la fija la CLASE por rango, nunca un texto.
- **FR-UI-7** — Bajo el cartel, la pantalla imprime `reversibility`
  **verbatim**. Ese campo nunca nace vacío: `NewBoundApprovalRequest`
  (`internal/action/bound.go`) le da una de TRES formas — `"unclassified
  consequence"` sin descriptor y sin clase; `"<clase>"` a secas sin descriptor
  y con clase; y `"<clase> — reversible"`, `"<clase> — compensatable, not
  reversible"` o `"<clase> — irreversible, no documented undo"` con
  descriptor, según sus banderas (`internal/action/bound.go`). Vacío ⇒ «el registro no
  declara reversibilidad», que es **defensa contra corrupción**, no un estado
  ordinario, y no cambia la puerta.
- **FR-UI-8** — `risk_summary` y `reversibility` nacen de **la misma cadena**
  (`bound.go`: `RiskSummary: reversibility`). La pantalla la imprime **una
  vez**, en el detalle. Alcance honesto: son iguales AL NACER y ningún
  cinturón las vuelve a comparar (`ValidatePreviewBinding` no mira
  `RiskSummary`), así que una corrupción de una sola columna no se detecta;
  por eso se imprime la del preview, que sí entra en el digest del preview.
- **FR-UI-9** — La geometría de los dos botones **no cambia** con la clase.

## 4. Pantalla a pantalla

Textos literales, en español. Maqueta: `design-drafts/approvals-screen-ux.html`.

### P1 · Lista de pendientes

- **FR-UI-10** — Fila: operación, cartel de clase, **origen** (el canal:
  `preview.Resources` es `[env.Source.Channel]` — «telegram», «console» — y
  **no** una URL; `internal/action/bound.go`), cabeza y cola del digest en la
  forma `8…8`, y la caducidad. Ningún dato de la fila procede de los
  parámetros del modelo.
- **FR-UI-11** — La lista **no se refresca sola**: se pide al entrar y con
  **[Actualizar]**.
- **FR-UI-12 (colisión de cola)** — Si dos filas **de la página** comparten los
  seis últimos caracteres, ambas imprimen el digest completo y la lista escribe
  «Dos peticiones muestran la misma cola de digest; aquí van enteros». Alcanza
  solo a la página que la lista trae (`ApprovalsPageLimit`): dos colisiones
  separadas por el corte no se ven, y quien abre un detalle sin pasar por la
  lista tampoco. Se declara así, sin venderlo como garantía global. **Y la
  página tampoco es una instantánea:** `ListApprovals` no trae la previa, así
  que la lista la lee fila a fila (1+N lecturas). Los carteles de clase y esta
  detección se computan sobre una página que puede mezclar instantáneas;
  FR-API-22 cierra el DETALLE, no la lista, y esto se dice en vez de
  suponerlo.
- **FR-UI-13** — `expires_at` vacío ⇒ «no caduca», sin cuenta atrás.
  **Defensa contra corrupción**: en producción `ExpiresAt` siempre se fija
  (`actx.Now.Add(actx.TTL)`, TTL mínimo validado), así que este camino solo lo
  alcanza una fila anómala.

### P2 · Detalle — el documento

Barra superior fija: `← Pendientes` · `sha256:a3f91c7d…5e1b6072 · fijado
mientras decides` · caducidad.

Cuerpo, en este orden, nada plegado, **un solo contenedor de scroll** (el de
la vista, `.main { overflow: auto }`):

1. **EL DIGEST — EXACTAMENTE ESTO SE EJECUTARÁ**: los 64 hex en **ocho grupos
   de ocho, en dos filas** — la única agrupación del documento, idéntica en
   spec, maqueta y `aria-label`, sin elisiones en ningún estado. Debajo:
   «Cambia un solo carácter de lo que se ve abajo y este digest es otro
   digest» — y se ve la forma canónica, que es la que el digest sella
   (FR-UI-68). Y debajo, la confirmación de FR-UI-62: **«✓ el almacén
   devolvió parámetros que re-derivan este digest»** — el documento no se
   pinta sin ella.
2. **OPERACIÓN** y el canal de origen.
3. **CLASE DE EFECTO** (§3), con `reversibility` verbatim.
4. **PARÁMETROS — LITERALES** (§6.2).
5. **ORIGEN**: propósito (`purpose`) y principal. **No** se pintan `grant_id`,
   `grant_depth` ni `cost_line`: en una petición aparcada son siempre `"-"`,
   `0` y `"unbudgeted"` — `identify` (`internal/brain/identity.go`) solo
   rellena `AuthorityRefs` cuando la regla es `granted`, y una aprobación nace
   con `require_approval`. Enseñar tres constantes como si informaran es peor
   que no enseñarlas.
6. **LA LEY QUE LO EXIGIÓ**: `law_digest` y `required_rule`. **No** se pinta
   `law_version`: es el FORMATO del pin, la constante `policyPinFormat = 3`
   (`internal/app/policy.go`, «sameness of law is sameness of digest»), y
   enseñar una constante como si informara es el defecto que ya retiró
   `grant_id`.
7. **CADUCIDAD**: instante absoluto UTC **y** la cuenta atrás.
8. «↓ la decisión está al final de la petición».
9. **FIN DE LA PETICIÓN** — el bloque de decisión (P3).

- **FR-UI-62 (el documento se verifica antes de enseñarse) — LA GARANTÍA
  CENTRAL DEL TREN**, por encima de la geometría y del armado (director,
  2026-09-09: «una pantalla read-through que no re-deriva lo que pinta no es
  read-through»); AS-83, a dos conexiones reales, es criterio de aceptación
  innegociable. — El detalle **re-deriva** `action.Digest(operation, parameters)`
  sobre los bytes **que el almacén devuelve** y los compara con el `action_digest`
  almacenado. Si no coinciden, no se pinta el documento: se rehúsa con nombre
  propio (`params_digest_mismatch`, FR-API-19) y no se ofrece ninguna
  decisión. **El cinturón corre sobre la FILA, ANTES de elegir
  `parameters_state`**: `params_digest_mismatch` precede a `empty` y a
  `too_large`. **No** precede a `unavailable`: sin bytes leídos no hay digest
  que re-derivar, así que un fallo de lectura de la fila es `unavailable` y
  punto. Si corriera solo cuando hay bytes que
  devolver quedaría fuera justo de los estados que produce la corrupción, y un
  atacante que VACÍE `canonical_params` cosecharía «esta acción se aparcó sin
  parámetros» — un hecho de nacimiento afirmado sobre una fila mutada.
  **Por qué existe:** hoy nadie hace esa comparación en el camino de LECTURA.
  `ValidatePreviewBinding` compara `preview.ArgsDigest` (un digest
  *almacenado*) contra `action_digest`, y `ApprovalParams` devuelve
  `canonical_params` **crudos, sin cinturón**
  (`internal/action/sqlite/approvals.go`). La única re-derivación real vive en
  `ExecuteApprovedAction`, o sea **después** de consumir la aprobación y
  vaciar la fila. Sin FR-UI-62, una segunda conexión que mute
  `canonical_params` hace que la pantalla enseñe los bytes del atacante bajo
  el digest legítimo, y la frase «Cambia un solo carácter de cualquier
  parámetro de abajo y este digest es otro digest» —el letrero que sostiene
  toda la propuesta 1— sería falsa en la superficie que la imprime.
  El propio almacén dice de su cinturón de lectura que existe «so the human
  never reads a lie»: esta es la columna que faltaba.
- **FR-UI-14** — El detalle se pide **una vez** al abrirlo y no se refresca
  solo. Refrescar es **[Volver a leer]**, que borra el armado.
- **FR-UI-15 (digest con forma)** — Antes de imprimirlo, recortarlo o
  compararlo, la pantalla valida `sha256:` + 64 hex minúsculas — la forma que produce
  `action.Digest` (`internal/action/action.go`), que es de donde sale el
  `ActionDigest` que la pantalla enseña; el validador no existe en el árbol y
  lo añade esta pantalla. Si no lo es: estado
  **«digest ilegible»**, sin Aprobar, con el valor crudo a la vista, y fuera de
  FR-UI-12.
- **FR-UI-16 (parámetros)** — Cuatro clases, cuatro desenlaces, nunca un bloque
  vacío ambiguo:
  | `parameters_state` | En pantalla | ¿Aprobar? |
  |---|---|---|
  | `present` | se pintan | sí |
  | `empty` (nacida sin argumentos, **probado por la re-derivación** `Digest(op,"") == action_digest`, FR-API-21) | «esta acción se aparcó sin parámetros, y así **no se puede ejecutar**: el claim rechaza la fila vacía» | **no** |
  | `unavailable` | «los parámetros no se pudieron leer en este instante; es transitorio y no dice nada sobre la evidencia» + [Reintentar] | no |
  | `too_large` | «esta pantalla no puede enseñarte esta petición entera, así que no te ofrece el sí» + el tamaño + la puerta de la CLI | no |
  La CLI colapsa hoy fila ausente, purgado y error de driver en una sola frase
  (`internal/cli/approvals.go`); esa mezcla **no se importa** (FR-API-8).

### P3 · El momento — el bloque de decisión

```
FIN DE LA PETICIÓN

Aprobar ejecuta tool/webhook_call exactamente como lo sella el digest de
arriba. No hay deshacer y no hay compensación conocida.

Para armar Aprobar, reteclea los seis últimos caracteres del digest.

  …7ca8d9f3 5e  [1][b][6][0][7][2]  ✓ coincide

Motivo del rechazo (opcional) [__________________________]

[   Rechazar   ]                                    [Aprobar y ejecutar]
 Siempre a un clic. Le dice que no al agente;        armado · a3f91c7d…5e1b6072
 no se ejecuta nada.

Esc rechaza mientras esta petición esté abierta y sin decidir.
```

- **FR-UI-17 (un solo envío)** — Al primer clic Aprobar se deshabilita y **solo
  la respuesta de SU petición pinta**. Dos clics no producen dos POST; una
  respuesta tardía no sobreescribe el desenlace vigente.

### P4 · Después del sí — los desenlaces de la ejecución, y su reparto honesto

La aprobación ejecuta de forma síncrona por la cadena de G3.

| Desenlace | Texto | ¿Ocurrió el efecto? |
|---|---|---|
| **Ejecutando** | «Ejecutando la acción aprobada. No cierres la ventana.» Sin cancelar. | en curso |
| **Ejecutada** | digest, resultado verbatim, y el identificador del recibo TERMINAL de la acción — que tampoco viaja hoy (`FinishWithResult` devuelve solo `error`, `internal/action/sqlite/ledger.go`) y exige la misma re-lectura: cambio 25 de §17 F | sí |
| `not_started_params_held` | «La decisión quedó registrada y **esta** ejecución **no llegó a salir**: `<error>`. La petición conserva sus parámetros **ahora**, y el libro solo cierra las aprobadas que ya no los conservan — así que ninguna pasada automática la ha cerrado. **Sobre otros ejecutores esta pantalla no se pronuncia** — el almacén es de varios procesos y ninguna lectura de aquí puede prometer que nadie más la ejecute. El camino es `korvun approvals execute` una vez restaurada la causa — y esta rama solo la reciben causas que ADMITEN reparación; si la causa fuese la evidencia, el desenlace sería otro y lo diría. Cuando la causa es la ley, devolver el perfil a como estaba devuelve el mismo digest y el comando vuelve a abrir. Si otro ejecutor se le adelantó, ese comando te lo dirá al fallarle el claim.» | esta ejecución, **no** |
| `not_started_params_gone` (la fila SIGUE, sus params están vacíos **y el digest de la cadena vacía re-deriva**: nació vacía) | «La decisión quedó registrada y esta ejecución **no llegó a salir**: `<error>`. Esta petición ya no conserva sus parámetros, así que `korvun approvals execute` tampoco la retomaría: **si la acción sigue en el libro, el próximo arranque la cerrará como desenlace desconocido**, con su recibo.» **Sin nombrar ningún comando.** | esta ejecución, **no** |
| `params_unaccounted` (params vacíos que **no** re-derivan el digest vacío, o **fila de aprobación ausente**) | «Esta ejecución no arrancó, y **esta pantalla no puede afirmar que la petición no se haya ejecutado**: al volver a leerla, sus parámetros ya no estaban donde estaban, o la fila de aprobación no estaba. Pudo llevárselos otra ejecución, o una mano sobre el almacén. Compruébalo antes de repetir nada.» | **no se sabe** |
| `decided_evidence_corrupt` (un cinturón o un parseo de evidencia rehúsa DESPUÉS de un decide commiteado, en cualquiera de sus sitios: pin de la ley que no casa con el ALMACENADO, previa que miente, historia que no cuadra, previa impaseable, columna de tiempo impaseable, fila de `actions` ausente) | «La decisión **quedó registrada y sellada**, con su recibo, y **esta** ejecución no llegó a salir. Lo que ya no verifica es la evidencia guardada: `<nombre del cinturón, o el error de parseo, crudo>` — y cuando ese nombre sea `approval_invalidated`, el literal añade que **aquí no significa que la ley se haya movido**: desde esta ventana no puede, así que lo que no casa es el pin guardado. **Esto es permanente**, y no se arregla devolviendo el perfil: la fila borrada no vuelve y la previa no depende de la configuración. **Sobre lo que haya hecho otro ejecutor esta pantalla no se pronuncia**, y sobre el estado en que queda la petición tampoco: este desenlace se nombra por el cinturón que rehusó, **sin volver a leer la fila**. Mira el libro antes de repetir nada.» **Sin nombrar ningún comando.** | **no se sabe** |
| `params_unreadable` (un fallo de DRIVER, en la lectura inicial o en la re-lectura) | «Esta ejecución no arrancó, y **el almacén no se pudo leer**: `<error>`. Por eso esta pantalla no puede afirmar que la petición no se haya ejecutado. **No dice que nadie se los llevara** — dice que no lo sabe. Reintenta la lectura antes de repetir nada.» | **no se sabe** |
| `not_decided` (corte 1 con `approval.Status == PENDING`) | «El almacén dice que esta petición **sigue esperando decisión**, así que esta ejecución no ha hecho nada. Si acabas de decidirla desde aquí, alguien ha reescrito su estado por debajo: **mira el libro antes de repetir nada**.» | **no**, y esta ejecución no ha hecho nada |
| `already_closed`, corte 1 (`approval.Status` es CUALQUIER valor distinto de `APPROVED`: los terminales del vocabulario **y** uno fuera de él, que solo escribe una mano externa) | «Esta petición no está esperando ejecución: la aprobación figura como `<status>`, impreso **crudo**. **Esta** ejecución no ha hecho nada.» **El estado de la ACCIÓN no aparece: este corte no lo lee.** | **no se sabe** |
| `already_closed`, corte 2 (`rec.State != StateApproved`) **sin** marcador de recuperación | «Entre la decisión y esta ejecución, la acción dejó de estar aprobada: figura como `<estado>`. **Esta** ejecución no ha hecho nada. **El almacén no guarda quién la cerró**, así que compruébalo antes de repetir nada.» | **no se sabe** |
| `already_closed`, corte 2 **con** `recovery_marker = outcome_unknown` | la frase del desconocimiento se sustituye por lo que el marcador SÍ sostiene: «una vida anterior de este proceso la cerró al arrancar **porque la petición ya no conservaba sus parámetros**». Y **solo entonces** se juzga el efecto, con la misma prueba de la escalera: si la aprobación sigue y `action.Digest(op, "")` **no** re-deriva, «nació con parámetros y ya no los tenía: **pudo llevárselos una ejecución, o una mano sobre el almacén**, así que el efecto pudo haber salido» — la misma disyunción que `params_unaccounted`, porque la evidencia es la misma y FR-API-21 ya declara que no nombra actor; si **sí** re-deriva, «nació sin parámetros, así que ninguna ejecución pudo arrancar» y la frase del efecto **no se escribe**. | **no se sabe**, salvo la rama nacida vacía, donde la pantalla dice que nada pudo salir |
| `params_belt_failed` | el literal de E6-bis | **no** |
| `unknown_outcome` | «La decisión salió de esta ventana. **No sabemos si el efecto llegó a ocurrir**: `<error>`. Compruébalo antes de repetir nada.» | **no se sabe** |

- **FR-UI-71 (QUIÉN produce cada desenlace — la acotación de la ronda 29)** —
  El papel mezclaba dos llamantes de `ExecuteApprovedAction` y esta pantalla
  solo conduce uno. Desde el **endpoint**, el `POST /approve` llama al
  ejecutor **inmediatamente después de su propio decide commiteado**, así que
  los dos cortes previos al claim solo se alcanzan si un escritor EXTERNO
  muta la fila en esa ventana — el modelo de atacante de FR-UI-62, no un error
  de dedo. Desde **`korvun approvals execute`** se alcanzan sin atacante,
  porque ese verbo no comprueba el estado antes de llamar
  (`internal/cli/approvals.go`), pero **esta pantalla no tiene ese verbo**
  (§1 lo deja fuera). Consecuencias, y hay que escribirlas porque cambian qué
  se puede probar:
  - `not_decided` sigue siendo el nombre correcto —fail-closed, y el árbol
    escribe la frase: «approval %s is PENDING — only APPROVED requests
    execute»—, pero su alcanzabilidad **desde esta pantalla exige el escritor
    externo**. El papel decía «sin atacante» y era de la otra superficie;
  - la subrama de `already_closed` **con `recovery_marker` NO es alcanzable
    desde esta pantalla**: tras el crash la aprobación queda `APPROVED` (la
    recuperación cierra la ACCIÓN, no la aprobación), así que un nuevo
    `POST /approve` muere en el decide con `approval_already_decided`
    (`ApprovalConsumableAt`, `internal/action/approval.go`) y la ventana pinta
    E9. Esa subrama es del **camino CLI**, y AS-110 y AS-112 se re-etiquetan
    en consecuencia: son moldes sobre el centinela tipado de `internal/app`,
    no sobre lo que esta ventana pinta. Se conservan porque el nombre viaja
    por el mismo mapa, y se declara que **no prueban una garantía de esta
    superficie**;
  - **`decided_law_moved` es del camino CLI y esta pantalla no lo pinta**
    (ronda 33). Desde el endpoint la ley no puede haberse movido —FR-API-24
    exige una sola resolución y el objeto es el mismo que pasó en el decide—,
    así que un rehúse de `ValidateApprovalBinding` aquí solo puede venir del
    pin ALMACENADO mutado por una mano externa: permanente, y sale por
    `decided_evidence_corrupt`. Desde `korvun approvals execute`, que resuelve
    de nuevo, sí es la ley y sí es reparable. **Y ni siquiera allí basta
    «devolver el perfil»**: la ley es el digest de la caja efectiva, y esa
    vista incluye contenido del BINARIO —`policyPinFormat` y el snapshot de
    los descriptores de efecto (`internal/app/policy.go`)—, así que un binario
    distinto mueve la ley sin que nadie toque el perfil. El literal de ese
    nombre, en su tren, dice «vuelve a abrir si la ley vuelve a ser la misma»,
    no «devuelve el perfil».
- **FR-UI-18 (los diez desenlaces de la ejecución, y su cable)** — Tras un
  `approve`, la decisión **ya está comprometida** pase lo que pase después
  (`DecideApprovalUnderLaw` commitea antes de que la ejecución empiece), así
  que la pantalla nunca vuelve a ofrecer una decisión. Lo que cambia es qué se
  sabe del efecto, y eso lo dicen **diez nombres** sobre los puntos de
  retorno de `ExecuteApprovedAction` más los del armado del ejecutor
  (`BuildApprovalExecutorFromCage` y `ResolveApprovalLaw`, que viven en el
  llamante), agrupados así (`internal/app/approvals.go`):

  | Nombre | Cuándo | ¿Ocurrió el efecto? |
  |---|---|---|
  | `not_started_params_held` | fallo DENTRO del claim que no es cinturón ni parseo de evidencia — la clase busy y el `RowsAffected` — **y la re-lectura encuentra la fila con sus params** (FR-API-25), y la causa NO es un cinturón, ni un parseo de evidencia, ni un fallo de driver: la clase busy y el `RowsAffected`. **El fallo de driver tiene un solo nombre** —`params_unreadable`, por el eje 1— y salió de aquí en la ronda 33: tenerlo en dos sitios dejaba un molde rojo por construcción. **Los cinturones internos del claim salieron de aquí en la ronda 32**: son las mismas funciones que corre `GetApproval` y el eje 1 los nombra corran donde corran. Y «`store.Get` ilegible» **tampoco es una sola cosa**: sus fallos de `time.Parse` y de `json.Unmarshal` son corrupción permanente y van a `decided_evidence_corrupt`; solo un error de driver se queda aquí. Desde `korvun approvals execute`, además: jaula y ley re-juzgada. **Lo que NO cae aquí desde la ronda 29**: los rehúses de `verifyApprovalStory` y `ValidatePreviewBinding` (fila de decisión borrada, previa mutada), que son PERMANENTES y salen por `evidence_corrupt` | **esta ejecución no salió**; sobre otras, la pantalla no se pronuncia — y su literal ya no promete que nadie más la ejecute |
  | `not_started_params_gone` | lo mismo, pero la fila SIGUE con sus params vacíos **y el digest vacío re-deriva**: nació vacía | **esta ejecución no salió** |
  | `params_unaccounted` | params vacíos que **no** re-derivan el digest vacío (FR-API-21), o **fila de aprobación ausente**. La re-lectura fallida **no**: tiene su propio nombre desde la ronda 33 | **no se sabe** |
  | `not_decided` | corte 1 con `PENDING`. **Desde esta pantalla exige un escritor externo** (FR-UI-71): el POST llega al corte con su propio decide ya commiteado. Sin atacante solo lo alcanza `korvun approvals execute`, que no comprueba el estado antes de llamar (`internal/cli/approvals.go`) — otra superficie. Nombrarla «ya cerrada» sería auto-contradictorio en cualquiera de las dos | **no**, y esta ejecución no ha hecho nada |
  | `decided_evidence_corrupt` | un cinturón o un parseo de evidencia rehúsa DESPUÉS de un decide commiteado, **en cualquiera de sus sitios** (eje 1 de FR-API-25). Literal PROPIO: el de E8 es de lectura y ahí afirmaría «no se ha leído nada como bueno» sobre una historia que el decide leyó como buena y commiteó | **no se sabe**: nombrado por centinela, sin releer, y un ejecutor ajeno pudo ganar el claim |
    | `decided_evidence_corrupt` (por `store.Get`) | la fila de `actions` borrada entre `GetApproval` y `store.Get` (`internal/app/approvals.go`). Está FUERA de `GetApproval`, así que necesita su propia rama en el eje 1 — sin ella caía en `not_started_params_held` y ofrecía `execute`, que es justo lo que AS-113 declara prohibido para esa causa | **no se sabe** |
  | `params_unreadable` | la re-lectura de FR-API-25 falla por driver o fila ilegible. Nunca comparte literal con el robo: un fallo transitorio no recibe el texto de una sospecha permanente | **no se sabe** |
  | `already_closed` | los dos cortes PREVIOS al claim (`internal/app/approvals.go`). El corte 1 cubre **todo** `approval.Status` distinto de `APPROVED` —los terminales del vocabulario y cualquier valor fuera de él—, porque el árbol compara contra `APPROVED` y no contra una lista. **El literal imprime SOLO lo que su corte leyó**: el corte 1 retorna antes del `store.Get`, así que no tiene el estado de la acción y no lo inventa. Y no atribuye el cierre a nadie **salvo cuando el almacén lo dice**: `store.Get` trae `recovery_marker` en la misma lectura (`internal/action/sqlite/store.go`). **Lo que ese marcador significa se lee en la CONSULTA, no en su godoc** (ronda 28): el predicado de la pasada 1 selecciona `state = 'APPROVED' AND NOT EXISTS (… canonical_params != '')` — es decir, **params ausentes**, no «reclamada». Una aprobación NACIDA VACÍA lo cumple sin que ningún claim haya existido jamás, y `ClaimApprovalParams` garantiza que no existió porque rechaza la fila vacía antes de tocar nada. Así que el literal dice lo que el predicado sostiene y deja el juicio del efecto a la misma prueba del digest vacío que usa la escalera | **no se sabe** |
  | `params_belt_failed` | claim GANADO, fila vaciada, y lo guardado no re-deriva el digest — **la terna o los params**, porque el digest cubre las dos y ningún cinturón compara `op_version` (FR-API-19). Es E6-bis | **no** |
  | *(el claim rehúsa por sí mismo)* | un fallo de driver **dentro** de `ClaimApprovalParams`. La ley, la previa y la historia **ya no caen aquí** (ronda 32): las nombra el eje 1 —`decided_law_moved` la primera, `decided_evidence_corrupt` las otras dos— porque son los mismos cinturones que `GetApproval` corre y darles dos nombres según el sitio dejaba un molde rojo por construcción. **El nombre lo decide una RE-LECTURA de la fila, nunca nuestro rollback** (FR-API-25): `held` si la fila conserva sus params; `gone` si la fila sigue, están vacíos y el digest vacío re-deriva; `params_unaccounted` en todo lo demás. **No** se reutilizan `invalidated`, `evidence_corrupt` ni `unavailable`: sus literales dirían «rechazarla sí funciona» sobre una decisión ya comprometida, «es transitorio» sobre algo permanente, y ninguno nombraría `korvun approvals execute` | **no** |
  | `unknown_outcome` | `exec.Run` corrió y falló la herramienta | **no se sabe** |
| `close_failed` | «La decisión salió de esta ventana. **No sabemos si el efecto llegó a ocurrir**, y además **esta ejecución no pudo cerrar el registro**: `<error>`. Compruébalo antes de repetir nada.» | **no se sabe** |

  **Hasta dónde se afina.** Dentro de los dos `not_started_*` quedan solo
  la clase busy y el `RowsAffected` — más la jaula, que solo llega desde
  `korvun approvals execute`. **Ni la ley ni «`store.Get` ilegible» viven ya
  aquí** (rondas 32-33): la primera va a `decided_evidence_corrupt` desde el
  endpoint, y del segundo sus parseos van ahí y sus fallos de driver a
  `params_unreadable`. Las irreparables salieron en la ronda 30 a
  `decided_evidence_corrupt` por sus centinelas tipados (§17 F fila 21). La
  justificación vieja —«el código no las distingue sin leer el texto inglés»—
  era FALSA y se retira: los cinturones se distinguen por centinela, no por
  texto y no por sitio.
  El error de la herramienta y el del cierre **sí** se separan, porque salen
  por dos líneas distintas: son `unknown_outcome` y `close_failed`. Lo que no
  se afina es dentro de cada uno, y para separarlos haría falta que `ExecuteApprovedAction` dejara de
  descartar `execErr`. Se pecan por exceso de prudencia las dos veces.
- **FR-UI-59 (el fallo del cierre)** — Tiene **nombre propio**, `close_failed`,
  porque el punto de retorno sí es distinguible en el llamante: el error del
  cierre y el de la herramienta salen por dos líneas distintas de
  `ExecuteApprovedAction` (`internal/app/approvals.go`). Su literal es el del
  desenlace desconocido más una frase sobre el INTENTO, no sobre el libro:
  «esta ejecución no pudo cerrar el registro». **No** se escribe «el registro
  no se cerró» a secas: si otra vida cerró la fila antes —la recuperación deja
  la acción en `OUTCOME_UNKNOWN` con su recibo, y entonces `FinishWithResult`
  falla por transición inválida y sale por esta misma línea— el registro **sí**
  está cerrado y la frase categórica sería falsa. Tampoco se escribe «el efecto
  ocurrió»: saber si la herramienta triunfó exigiría que
  `ExecuteApprovedAction` dejara de descartar `execErr` en esta rama, y no lo
  hace. Y cuando fallan las dos —la herramienta y el cierre— este nombre se
  emite igual y el error de la herramienta se pierde: las dos filas de la
  tabla no son disjuntas, y se dice aquí en vez de fingirlo.
- **FR-UI-19 (dónde se nombra `execute`)** — Solo en `not_started_params_held`, y
  como el camino que abre desde ESTA ventana — nunca como el único que puede
  tocar la petición: otro proceso puede reclamarla sin que nadie restaure nada. El literal de `not_started_params_gone` acota su
  promesa: el cierre del próximo arranque **solo llega si la fila de `actions`
  sigue ahí** — la pasada de recuperación selecciona `FROM actions`
  (`internal/action/sqlite/store.go`), así que una acción borrada por el
  CASCADE no se cierra ni deja recibo, y el endpoint no puede distinguir las
  dos subramas: las dos llegan como el mismo `ErrNotFound`. En
  `not_started_params_gone` no se nombra: `execute` moriría antes de ejecutar nada: con la fila
  de aprobación ausente, en su propio `GetApproval`; con la columna vacía, en
  el claim con «already claimed or closed»
  (`internal/action/sqlite/approvals.go`), y además esa fila **sí** entra en
  el predicado de la recuperación, que la cerrará como desenlace desconocido
  en el próximo arranque. Prometer el comando ahí sería mandar al operador a
  una puerta cerrada y negar un cierre que sí llega. La acción queda `APPROVED` con sus params intactos, y esa fila
  **sobrevive a todos los arranques**: la pasada de recuperación solo cierra
  las aprobadas que ya no conservan parámetros (columna vacía **o** fila de
  aprobación ausente: el predicado es un `NOT EXISTS … AND canonical_params != ''`)
  (`internal/action/sqlite/store.go`, cuyo godoc lo dice literal: «An APPROVED
  action whose params are still held awaits its deferred execution and
  **survives**»), y el prune solo borra terminales. Así que **ningún
  BARRENDERO la cierra**: o se restaura la causa y `korvun approvals execute`
  la retoma, o sigue aprobada y sin ejecutar hasta que alguien la ejecute.
  Lo que esta frase NO dice —y las rondas 25 y 26 obligaron a acotar— es que
  nadie más pueda ejecutarla: el almacén es de varios procesos y un claim
  ajeno puede ganarla en cualquier momento, sin restaurar nada. La promesa es
  sobre las pasadas automáticas, no sobre el mundo. **Y el pin no es el
  enemigo que parecía:** la ley es el digest de la caja efectiva y dos cargas
  de la misma configuración dan la MISMA ley (`internal/app/policy.go`: «two
  loads of the same effective config are the SAME law»), así que devolver el
  perfil a como estaba devuelve el digest y **revive** el comando. Lo que sí
  cambia es por dónde sale el fallo: si la caja se estropea **antes** de
  decidir, el decide rehúsa con `invalidated` (E7) y el ejecutor no llega a
  armarse; `not_started_params_held` por jaula solo aparece si la caja se
  rompe entre el decide y el armado — una ventana que **el endpoint no tiene**,
  porque FR-API-24 le exige una sola resolución y la caja ya está en memoria
  antes del decide. Desde el endpoint, `not_started_params_held` lo producen
  las causas que **no** son cinturón, parseo de evidencia ni fallo de driver: la
  clase busy y el `RowsAffected`. Los fallos de PARSEO de `store.Get` son
  corrupción permanente y salen por `decided_evidence_corrupt`; sus fallos de
  DRIVER salen por `params_unreadable`. La previa corrompida entre el decide y
  la re-lectura ya **no** cae aquí desde la ronda 30: va a
  `decided_evidence_corrupt`, porque devolver el perfil no la repara. La causa «jaula» solo es alcanzable
  desde `korvun approvals execute`, que resuelve de nuevo. La causa «ley» tampoco llega desde el
  endpoint como `invalidated`: los cinturones anteriores al claim la cazan
  primero, y desde la ronda 33 su desenlace es `decided_evidence_corrupt` (AS-89): desde esta pantalla la ley no puede haberse movido, así que un rehúse del pin solo lo produce el valor ALMACENADO mutado por una mano externa (FR-UI-71). **Y con las cuatro mutaciones coordinadas SÍ dispara el `ValidateApprovalBinding` de `internal/app`**: juzga la copia releída en esa misma llamada, no una copia rancia; es su tercer sitio, y el eje 1 lo nombra igual. La pantalla lo dice con esas palabras y
  no promete un cierre que no llega. En los otros **nueve**
  desenlaces —`not_started_params_gone`, `params_unaccounted`,
  `params_unreadable`, `decided_evidence_corrupt`, `already_closed`,
  `not_decided`, `params_belt_failed`, `unknown_outcome` y `close_failed`— **no se nombra ningún comando**: tras el
  claim ganado o tras `exec.Run`, `execute` muere en «already claimed or
  closed» o en «already executed or closed»; y en `not_decided` el comando
  existe pero es el verbo equivocado — lo que falta es decidir, no ejecutar.

### P5 · Después del no

«Rechazada. La acción aparcada se cierra con su recibo sellado.» + el
identificador del recibo del ACTO DE DECISIÓN —no el terminal de la acción—,
que **no lo devuelve `DecideApprovalUnderLaw`** (su firma es `(string, error)`,
`internal/action/sqlite/approvals.go`) y por tanto exige una RE-LECTURA de
`decision_receipt_id`, que `scanApproval` sí trae: es el cambio 25 de §17 F.
No hay deshacer.

- **FR-UI-20 (lo que NO se promete)** — La pantalla **no dice que al agente se
  le avise**, ni en el no ni en la caducidad: su única observación llegó al
  aparcar (`pendingApprovalObservation`, `internal/brain/effects.go`) y le
  ordena literalmente «do NOT retry it». Por eso la pantalla tampoco escribe
  «tendrá que volver a pedirla»: si el operador quiere la acción, la pide él en
  la conversación.
- **FR-UI-21 (el no perdido)** — Si la respuesta del rechazo se pierde:
  **desenlace desconocido simétrico** — «El rechazo salió de esta ventana y no
  hemos recibido su desenlace.» Nunca «Rechazada».

### E1 · Núcleo parado

`503` con cuerpo exacto `{"error":"core stopped"}` (`internal/shell/proxy.go`).

```
El núcleo está parado
No se puede registrar ninguna decisión ni ejecutar ninguna acción con el
gateway detenido. Lo que hubiera aparcado sigue intacto: sus digests y sus
relojes de caducidad los guarda el almacén, no esta ventana.
[Arrancar el núcleo]  [Ir a Inicio]
```

- **FR-UI-22** — No dice cuántas hay aparcadas: no lo sabe.
- **FR-UI-63 (un 503 «core stopped» tiene TRES causas)** — El godoc del proxy
  las nombra: «stopped, mid-cutover, or observability disabled»
  (`internal/shell/proxy.go`). La vista **cruza** con `Status().Running` antes
  de afirmar nada: con `Running=false` escribe el literal de E1; con
  `Running=true` escribe otro — «Esta ventana no alcanza la puerta de
  aprobaciones del núcleo. El proceso está en marcha; puede ser una recarga en
  curso o la observabilidad apagada en el perfil.» + [Reintentar] y [Abrir la
  carpeta de configuración], **sin** ofrecer [Arrancar el núcleo]. Sin este
  cruce, la vista diría «parado» mientras el chip de al lado dice «En marcha»,
  y E3/E4 mandan al operador a editar el perfil, que es justo lo que provoca
  el mid-cutover. **Tercera rama:** si `Status()` falla o no hay bindings
  (arnés, navegador), la vista no afirma ninguna de las dos — escribe «Esta
  ventana no alcanza la puerta de aprobaciones, y tampoco ha podido preguntar
  al núcleo en qué estado está» + [Reintentar]. El cruce vive en la costura
  Wails, así que su nivel de evidencia honesto es **navegador contra el
  arnés**, no jsdom.

### E2 · Núcleo no responde — ramificado por método

`503 {"error":"core unreachable"}` lo emite el `ErrorHandler` del proxy ante un
fallo de RoundTrip, que **incluye** el caso en que la petición llegó, se
decidió y se ejecutó.

- **FR-UI-23** — En un **GET**: «El núcleo no responde. El proceso figura en
  marcha pero no contesta. Ninguna decisión ha salido de esta ventana.» +
  [Reintentar].
- **FR-UI-24** — En un **POST**: **desenlace desconocido**. Jamás «ninguna
  decisión ha salido».

### E3 · Aprobaciones apagadas (G7)

`409 {"error":"disabled"}`.

```
APROBACIONES APAGADAS EN ESTE PERFIL
Con las aprobaciones apagadas ya no se retiene nada nuevo
Una acción irreversible se ejecuta ahora en el momento en que el agente la
llama. Esto no es una bandeja vacía: es un hueco en la garantía.
Y lo que se aparcó ANTES de apagar el interruptor sigue vivo en el almacén:
esta pantalla no puede enseñártelo con las aprobaciones apagadas, y solo se
decide desde la CLI hasta que el barrendero lo cierre.
Se enciende en el perfil: approvals.enabled
[Abrir la carpeta de configuración]
```

- **FR-UI-25** — Prohibido pintar cero filas. Prohibida aquí la cadena de V1
  «No hay nada aparcado». **Y prohibida toda afirmación de vacío**: el
  interruptor solo bifurca el RECORDER (`internal/app/approvals.go`), así que
  las peticiones aparcadas ANTES de apagarlo siguen PENDING, siguen
  decidiéndose desde la CLI (`DecideApprovalUnderLaw` no lo consulta) y el
  barrendero sigue corriendo sobre ellas. Decir «no se está reteniendo nada»
  sería falso sobre un almacén que retiene acciones irreversibles vivas.
- **FR-UI-26** — El botón llama `OpenConfigFolder`
  (`internal/shell/secrets_names.go`) y nombra el fichero. **No** manda a
  Ajustes: `views/Settings.tsx` no expone `approvals.enabled`. Si la llamada devuelve error o no hay bindings
  (arnés, navegador), la pantalla lo dice — «No se ha podido abrir la carpeta»
  + la ruta en texto seleccionable. Alcance honesto: `defaultFolderOpener`
  devuelve `nil` en cuanto `cmd.Start()` tiene éxito
  (`internal/shell/secrets_names.go`), así que un gestor que muere después es
  un no-op que esta pantalla no puede detectar; por eso la ruta se imprime
  siempre, no solo en el error.
- **FR-UI-27** — No cuenta acciones ejecutadas sin revisar: sin fuente.

### E4 · Encendidas y sin poder aparcar

Aparcar exige DOS llaves y la segunda tiene grado: `approvals.enabled` **y** un
cerebro cuyo techo rankee ≥ `write_irreversible` (§3). El perfil de fábrica
(`internal/shell/firstrun_template.json`) no trae ninguna.

```
APROBACIONES ENCENDIDAS · NINGÚN CEREBRO PUEDE APARCAR
Nada puede llegar a esta lista
Las aprobaciones están encendidas, pero ningún cerebro reúne las condiciones
para aparcar. Falta al menos una de estas cinco: el almacén de acciones
abierto, un cerebro agente, un techo de efecto en write_irreversible o
critical, una herramienta de esa clase en su jaula, y esa herramienta
permitida por la gobernanza. Con un techo por debajo la acción se deniega;
sin techo se ejecuta al instante. Esto tampoco es una bandeja vacía.
[Abrir la carpeta de configuración]
```

- **FR-UI-28** — Se decide con el bloque `gate` de la lista (FR-API-6), nunca
  adivinando desde una lista vacía.

### E5 · Caducada

- **FR-UI-29 (el reloj de la ventana no juzga)** — Cuando el reloj local pasa
  `expires_at`, la pantalla **retira Aprobar y borra el armado** y **mantiene
  Rechazar**, con: «El reloj de esta ventana dice que esta petición ya ha
  caducado. Quien lo juzga es el servidor, en el toque de la decisión.» No
  afirma que caducó.
- **FR-UI-30 (caducidad de verdad)** — Solo un `409 {"error":"expired"}` pinta:
  ```
  Esta petición caducó y ya no se puede decidir
  Caducó a las 2026-09-08T14:31:00Z. La acción aparcada se cierra con su
  recibo; no se ha ejecutado nada.
  DIGEST — YA NO ACCIONABLE  (los ocho grupos, enteros)
  [Volver a pendientes]
  ```
  Aquí sí desaparecen los dos botones.
- **FR-UI-31 (la caducidad barrida)** — Existe un barrendero:
  `SweepExpiredApprovals` corre en el arranque (`internal/app/app.go`) y en la
  cadencia de prune (`internal/action/sqlite/store.go`). Una vez barrida, la
  fila queda `EXPIRED` y `ApprovalConsumableAt` devuelve
  `approval_already_decided`, **no** `expired`. Sin cura, la pantalla diría «ya
  estaba decidida — la primera decisión es la que vale» de algo que cerró el
  reloj y que nadie decidió. El endpoint mapea por ESTADO de la fila:
  `EXPIRED` ⇒ `expired` (FR-API-9).

### E6 · `digest_mismatch` (G2) — el digest que traes no es el de esta petición

`409 {"error":"digest_mismatch"}`, comprobado **antes** de decidir y de
reclamar: el digest del cuerpo contra el `action_digest` almacenado, que es
inmutable: los cuatro UPDATE de producción sobre `approvals`
(`internal/action/sqlite/approvals.go`) tocan estado/decisión y vacían
`canonical_params`, y ninguno escribe `action_digest`. (El godoc del fichero
dice «There are NO other update or delete paths», que es él mismo más ancho
que su cable — la retención borra filas por CASCADE —; lo que sostiene esto
son los UPDATE, no la frase.) Nada se ha consumido, así que volver a leer sirve.

```
La petición cambió entre que la leíste y que la aprobaste
No se ha ejecutado nada y nada se ha consumido.
Aprobaste  sha256:a3f91c7d…5e1b6072
El servidor comparó ese digest con el de la petición guardada y no coinciden,
así que paró antes de decidir. Eso es exactamente lo que tiene que pasar.
Lo que hay que hacer: leerla otra vez, entera, desde el principio.
[Volver a leer la petición]  [Volver a pendientes]
```

- **FR-UI-32** — No enseña el digest nuevo ni ofrece «aprobar de todas formas».
- **FR-UI-33** — [Volver a leer] descarta el documento, borra el armado y el
  scroll vuelve arriba.
- **FR-UI-60 (la comprobación va PRIMERO)** — Si el digest se comprobase
  después del decide, un cliente rancio consumiría la aprobación antes de que
  nadie mirase (FR-API-14).

### E6-bis · El cinturón de los parámetros — la petición ya está consumida

Distinto animal, mismo apellido. Si los `canonical_params` guardados ya no
re-derivan el digest, quien lo descubre es el cinturón de
`ExecuteApprovedAction` — **después** de que la decisión se comprometió y de
que `ClaimApprovalParams` vació la fila. Volver a leer aquí no sirve: no queda
nada que leer.

```
Lo guardado no reproduce el digest que aprobaste — y **puede ser la operación, no los parámetros**: el digest cubre la TERNA (namespace, nombre y VERSIÓN) además de los argumentos (`internal/action/action.go`), y ningún cinturón compara hoy `op_version`
No se ha ejecutado nada. Pero esta petición ya está consumida: la decisión se
registró y los parámetros se vaciaron al reclamarlos, así que no se puede
volver a intentar ni volver a leer.
La acción queda aprobada y sin ejecutar; en el próximo arranque el libro la
cerrará como desenlace desconocido, con su recibo.
[Volver a pendientes]
```

- **FR-UI-61** — No ofrece [Volver a leer] ni ninguna decisión, y **no**
  reutiliza el texto de E6: el de E6 dice «léela otra vez», que aquí sería
  falso.

### E6-ter · el documento no cuadra consigo mismo — DOS causas, dos nombres

La cara visible de FR-UI-62. Llega de un **GET** del detalle y nunca lleva
decisión.

El estado tiene **dos** causas y no comparten frase, porque no dicen lo mismo:
`params_digest_mismatch` (lo guardado **no** re-deriva el digest — y puede ser la operación y no los bytes: el digest cubre la TERNA, `internal/action/action.go`, y ningún cinturón compara hoy `op_version`) y
`params_not_canonical` (sí lo re-derivan, pero **no son su forma canónica**, y
en producción la columna nace canónica: es una mano externa). Los dos son
permanentes, los dos rehúsan sin decisión.

```
[params_digest_mismatch] Los parámetros guardados no reproducen el digest de
esta petición
Esta ventana no te enseña un documento que no cuadra consigo mismo. No se ha
ejecutado nada y no se ofrece ninguna decisión.
digest de la petición  sha256:a3f91c7d…5e1b6072
Esto no es transitorio. Guarda el identificador y mira el libro.
[Volver a pendientes]

[params_not_canonical] Los parámetros guardados no están en la forma que este
digest sella
Re-derivan el digest, pero alguien los ha reescrito en otra forma. En
producción esta columna nace ya canónica, así que esto es una mano externa.
No se ha ejecutado nada y no se ofrece ninguna decisión.
[Volver a pendientes]
```

- **FR-UI-66** — Este nombre **jamás** se degrada a `unavailable` ni a
  «respuesta que esta pantalla no reconoce», y no ofrece Aprobar ni Rechazar.

### E9-bis · `brain_gone` en un POST — el cerebro ya no está

`ResolveApprovalLaw` falla antes de decidir (`internal/app/approvals.go`), así
que **no se escribe nada** y la fila sigue `PENDING`.

```
El cerebro que pidió esta acción ya no está en el perfil
No se ha decidido nada. Aprobar necesita la ley de ese cerebro y esa ley ya no
existe; rechazar no la necesita, así que sigue disponible.
[Rechazar esta petición]  [Volver a pendientes]
```

- **FR-UI-67** — Junto con la forma de LECTURA de E7, es uno de los **dos**
  estados de fallo que conservan Rechazar. E8 **no** es uno de ellos: el
  rechazo también corre los dos cinturones dentro de su transacción.

### E7 · La ley se movió — `invalidated` (permanente)

`ValidateApprovalBinding` (`internal/action/approval.go`) devuelve
`approval_invalidated` cuando el pin de la ley ya no coincide, y el store lo
refuerza en el toque. Mueven el digest de la caja efectiva las claves que `canonicalView` mete en la vista
(`sensitivity`, `tools`, `governance`, `attrs`, `effect_ceiling`, `read_file`,
`http_fetch`, `webhook_call`, `memory`) más el snapshot de efectos que añade
`policyDigestFromCage` (`internal/app/policy.go`)
(`internal/app/effectivecage.go`). `approvals.enabled`, `max_iterations` o el
`system_prompt` **no** entran: encender las aprobaciones desde E3 no invalida
nada; es E4, que manda tocar techos y jaulas, quien sí puede hacerlo.

```
La ley bajo la que se aparcó esta petición ya no existe
No se ha ejecutado nada, y esto NO es transitorio: mientras el perfil siga
como está, esta petición no se puede aprobar nunca.
Se aparcó bajo la ley sha256:aab9b0d7; ahora rige sha256:31c0f7ae
Rechazarla sí funciona: retirar autoridad es seguro bajo cualquier ley.
[Rechazar esta petición]  [Volver a pendientes]
```

- **FR-UI-64 (E7 y E8 solo conservan Rechazar cuando llegan de una LECTURA)** —
  Los mismos nombres (`invalidated`, `evidence_corrupt`) nacen en dos sitios
  distintos: al **leer** el detalle, con la aprobación intacta y `PENDING`; y
  en las re-comprobaciones de `ExecuteApprovedAction`, que corren **después**
  de que el decide se comprometió (`internal/app/approvals.go`), con la
  aprobación ya `APPROVED`. En el segundo caso ofrecer Rechazar sería ofrecer
  un botón que devuelve `already_decided` garantizado
  (`ApprovalConsumableAt`). **Precedencia escrita, por NOMBRE**, porque «si el
  decide llegó a comprometerse» no es observable desde el endpoint: la rama
  que caduca y la que ya estaba decidida vuelven las dos como `(rule, nil)`, y
  la primera **sí escribe** — `expired` cierra la aprobación, rechaza la
  acción, sella su recibo y vacía los params en la misma transacción
  (`internal/action/sqlite/approvals.go`). La tabla:

  | Nombre en un POST | ¿Escribió? | Qué pinta la pantalla |
  |---|---|---|
  | `digest_mismatch` | no | E6, con su Rechazar |
  | `already_decided` | no | su literal de E9 |
  | `invalidated` **antes** del one-shot | no | E7, con su Rechazar |
| `evidence_corrupt` | no | E8, **sin** Rechazar: el toque de decisión re-verifica la previa y la historia **para todo verbo humano**, rechazo incluido (`internal/action/sqlite/approvals.go`), así que ese botón moriría con el mismo nombre |
  | `brain_gone` | no | su literal, en modo solo-rechazo |
  | `expired` del toque | **sí**: cierra, rechaza, sella y vacía en la misma transacción | E5, **sin** Rechazar |
| `expired` de una fila ya barrida | no: el barrendero lo hizo antes, y el toque solo lee el estado | E5, **sin** Rechazar |
  | los **diez** de FR-UI-18 | sí, el decide está comprometido | su literal, sin ninguna decisión |

  Los literales de E7 y E8 **con Rechazar** son los de la LECTURA y los de
  esta tabla; un `invalidated` posterior al one-shot se pinta con el nombre de FR-UI-18 que corresponda a su punto de retorno.
- **FR-UI-34** — En su forma de LECTURA (FR-UI-64), es uno de los dos estados
  de fallo que **siguen ofreciendo Rechazar** (el otro es E9-bis; E8 no, porque
  el rechazo corre los mismos cinturones),
  porque el rechazo no consulta la ley (`internal/cli/approvals.go`: el pin
  solo se resuelve en la rama `approve`). Aprobar no aparece.
- **FR-UI-35** — Jamás bajo `unavailable`: llamar «transitorio» a una negativa
  permanente es la mentira que este estado existe para evitar.

### E8 · La evidencia no cuadra — `evidence_corrupt` (permanente)

`GetApproval` corre un cinturón sobre la historia guardada y rehúsa por nombre:
`preview_digest_mismatch`, `preview_policy_mismatch`, `preview_args_mismatch`,
`preview_rule_mismatch` (`ValidatePreviewBinding`, `internal/action/approval.go`)
y `preview_effect_mismatch`, `preview_operation_mismatch`,
`preview_principal_mismatch`, `decision_outcome_mismatch`,
`decision_policy_mismatch` (`verifyApprovalStory`,
`internal/action/sqlite/approvals.go`) — nueve, y la pantalla imprime el que
venga sin presumir la lista.
`ListApprovals` **no** corre ese cinturón, así que la fila sale en la lista y
el detalle rehúsa.

```
La evidencia de esta petición no cuadra consigo misma
El almacén guarda esta petición, pero su historia no verifica: <nombre del
cinturón>. No se ha leído nada como bueno y no se ofrece ninguna decisión.
Esto no es transitorio. Guarda el identificador y mira el libro.
[Volver a pendientes]
```

- **FR-UI-36** — Ni Aprobar ni Rechazar. El nombre del cinturón se imprime
  crudo. Jamás bajo `unavailable`.

### E9 · Los demás desenlaces con nombre

Traducción **por nombre**, nunca por el texto inglés del cuerpo.

| `error` | HTTP | Texto | Arreglo |
|---|---|---|---|
| `already_decided` | 409 | «Esta petición **ya no está esperando decisión**, y **esta ventana no la ha decidido**. Puede haberla decidido otro operador, o el almacén guardar para ella un estado que esta ventana no puede juzgar; desde aquí no se distingue. Mira el libro.» | [Volver a pendientes] |
| `not_found` | 404 | «No hay ninguna petición con ese identificador.» | [Volver a pendientes] |
| `unavailable` | 503 | «El almacén no se pudo leer en este instante. Es transitorio y no dice nada sobre la evidencia.» | [Reintentar] |
| `forbidden` | 401 | «La ventana no ha podido autenticarse contra el núcleo. No ha salido ninguna decisión.» | [Ir a Inicio] |
| **superficie no montada** | 404 con cuerpo que no es de la API | «Esta ventana no encuentra la puerta de aprobaciones en el núcleo. Suele ser un perfil sin bloque `admin`: sin él no se genera credencial y la superficie de mutación no se monta.» | [Abrir la carpeta de configuración] |
| nombre desconocido | cualquiera | «Respuesta que esta pantalla no reconoce.» + código, `error` y `message` **crudos** | en un GET, [Reintentar]; en un **POST**, desenlace desconocido y **sin** Reintentar |
| **sin respuesta o ilegible** | — | «El núcleo no ha contestado nada legible.» + lo que llegó, crudo (o «cuerpo vacío») | [Reintentar] · en un POST, desenlace desconocido |

- **FR-UI-65** — Un nombre desconocido en un **POST** hereda la regla del
  POST: desenlace desconocido, sin [Reintentar] — que por §8 repite el último
  GET y nunca un POST. La taxonomía es demostrablemente incompleta (el
  sellador solo aporta siete clases propias: `receipt_mutated_at_birth`,
  `receipt_hash_invalid_at_birth`, `receipt_unsigned`,
  `signing_key_unregistered`, `key_registry_unavailable`,
  `signing_key_retired`, `signature_invalid_at_birth`,
  `internal/action/sqlite/ledger.go`), así que esta fila **se alcanza de
  verdad** desde un `/approve` o un `/reject`.
- **FR-UI-70 (el origen vacío — hueco DECLARADO, no alcanzable probado)** —
  `Resources: []string{strings.TrimSpace(env.Source.Channel)}`
  (`internal/action/bound.go`) puede producir un elemento vacío. El papel
  define defensa para `expires_at` vacío (FR-UI-13), `reversibility` vacío
  (FR-UI-7) y digest malformado (FR-UI-15), y **no** para el origen. No se ha
  encontrado un camino de producción que deje el canal vacío —`envelope.Validate`
  rechaza `Channel == ""` (`internal/envelope/validate.go`)—, así que se
  declara como hueco de la taxonomía y **no** se le pide molde: la pantalla
  imprime el valor crudo, sea el que sea, por FR-UI-45.
- **FR-UI-72 — RETIRADA en la ronda 33.** Daba a `already_decided` un segundo
  literal elegido por un campo `stored_status`, y ese campo **no tiene cable**:
  `DecideApprovalUnderLaw` devuelve un escalar en sus **TRES** ramas de `already_decided` —vía `ApprovalConsumableAt`, el toque de caducidad que PIERDE la carrera del cierre, y el one-shot perdido (`internal/action/sqlite/approvals.go`)—
  (`internal/action/sqlite/approvals.go`), así que el endpoint no puede
  distinguirlas ni leer el estado guardado sin una re-lectura que el propio
  literal declaraba no hacer. Un solo literal, y redactado para ser cierto en
  las dos ramas: dice que la petición ya no espera decisión y que esta ventana
  no la ha decidido, sin afirmar cuál de las dos causas fue.
- **FR-UI-37** — Las tres últimas filas son la ley: nada se degrada a lista
  vacía ni a un «algo ha ido mal» genérico.
- **FR-UI-73 (E8 sin nombre de cinturón)** — Una previa IMPASEABLE en el
  camino de LECTURA sale por `ParseCanonicalPreview`, que no es un cinturón y
  no tiene nombre que imprimir. El hueco `<nombre del cinturón>` de E8 lleva
  entonces el **error de parseo crudo**, con la misma regla que el literal de
  `decided_evidence_corrupt`. Sin esto, E8 imprimiría un hueco vacío o un
  nombre inventado.
- **FR-UI-38** — `unavailable` jamás se redacta como «no existe», y jamás
  recibe una negativa permanente (E7, E8).

### V1 · Vacío legítimo · C1 · Cargando

- **Vacío** (200, cero filas, núcleo en marcha, `gate` armado): **«No hay nada
  aparcado.»** + «Las aprobaciones están encendidas y N de M cerebros pueden
  aparcar acciones irreversibles: si uno de ellos lo intenta, aparecerá aquí.»
  Único estado con cero filas.
- **Cargando**: «Consultando el almacén…», sin esqueletos.

## 5. El armado por tecleo

- **FR-UI-39** — Un solo `<input maxlength=6>` dibujado como seis casillas.
- **FR-UI-40** — Alfabeto `[0-9a-f]`; mayúsculas a minúscula; el resto se
  ignora sin pintar error.
- **FR-UI-41 (pegar no arma)** — `paste`, `drop` y autorrelleno se rechazan con
  «Pegar no arma: teclea los seis caracteres».
- **FR-UI-42** — «faltan N» / «✓ coincide» / «no coincide». Sin bloqueos ni
  contador de intentos.
- **FR-UI-43** — El armado muere con: navegar fuera, [Volver a leer],
  caducidad presentacional, y cualquier respuesta que cambie el documento.
  **Sobrevive** a perder el foco de la ventana.
- **FR-UI-44** — El cuerpo del POST lleva **exactamente una clave**, `digest`,
  con el digest entero que trajo el detalle. Los seis caracteres tecleados no
  viajan como dato propio (son, por construcción, la cola de ese digest).

## 6. Los bytes que vienen de un modelo

### 6.1 Qué prueba el tecleo y qué no

Seis caracteres hex son ~16,7 millones de combinaciones: no son prueba
criptográfica y esto no las vende como tal. Prueban que quien aprueba ha mirado
ESTE digest, y cambian en cada petición. La identidad de lo aprobado la
garantiza el servidor rehaciendo el digest del objeto guardado (G2, y el
cinturón que ya existe en `ExecuteApprovedAction`), no el teclado.

### 6.2 El renderizador de texto no confiable

- **FR-UI-68 (se imprime lo que se digiere)** — El digest no hashea bytes:
  hashea `CanonicalParams(raw)`. Y **lo que se aparca es la cadena que el
  modelo escribió, verbatim y sin validar**: hay **dos productores** y conviene
  nombrar los dos. En el carril de TEXTO, `protocol.go` captura la cadena
  entre los paréntesis del `TOOL:` («captured VERBATIM for the tool to parse»)
  y `agent.go` la pasa tal cual a `RequestApproval`. En el carril NATIVO,
  `agent_native.go` re-serializa el mapa del proveedor con `json.Marshal`, y
  si la herramienta es un `ParamTool` reconstruye la cadena con su
  `ArgsFromCall` — que para `webhook_call` siempre da «URL, espacio, cuerpo».
  Es decir: el carril nativo produce de forma determinista la forma identidad;
  el de texto puede producir cualquiera de las dos. De ahí que convivan **dos
  formas** en la columna:
  - **un valor JSON suelto** —`{"a":1,"a":2}`— que `CanonicalParams`
    **re-serializa**: ordena claves, borra espacios, resuelve duplicados
    last-wins y escapa `<`, `>` y `&` a seis bytes;
  - **cualquier otra cosa** —incluida la forma que `webhook_call` espera,
    «URL, espacio, cuerpo JSON»— sobre la que `CanonicalParams` es la
    identidad.

  Para la primera forma, **bytes distintos pueden dar el mismo digest**, así
  que imprimir los bytes crudos bajo el ✓ sería una puerta abierta. Dos reglas
  la cierran:
  1. **La pantalla imprime `CanonicalParams(bytes)`**: lo que se lee es lo que
     se digiere. Una clave duplicada deja de verse porque no es lo sellado.
  2. **Si los bytes guardados no son ya su forma canónica, es manipulación**:
     en producción la columna nace de `CanonicalParams`
     (`internal/action/sqlite/approvals.go`), así que
     `bytes != CanonicalParams(bytes)` solo lo produce una mano externa ⇒
     E6-ter, sin decisión.

  **Historial de este párrafo, para que no se repita:** la versión anterior
  afirmó que los parámetros nunca son un valor JSON suelto «porque
  `webhook_call` espera URL + cuerpo». Era falso, y el error de método fue
  comprobar la forma del dato contra su CONSUMIDOR
  (`internal/tool/webhookcall.go`, que solo valida en `Execute`, ya después de
  la aprobación) en vez de contra su PRODUCTOR (`protocol.go` + `agent.go`).
  Lo que sí se retira y no vuelve: la frase de que «el tool decodea con las
  mismas reglas» — `webhook_call` parte por el primer espacio y valida el
  cuerpo con `json.Valid`, así que un objeto JSON suelto ni siquiera pasa su
  validación; el daño de esta puerta es sobre lo que el operador **lee**, y
  sobre la herramienta con argumentos JSON que aparque el día de mañana.
- **FR-UI-45** — Se aplica a **TODO campo de origen no controlado**, no a un
  bloque: `parameters`, `operation`, `resources`, `purpose`, `reversibility`,
  `tool_cage`, `required_rule`. Pinta como TEXTO, sin autoenlazar, sin `<a>`,
  sin cargar imágenes. Los parámetros se imprimen en su **forma canónica**
  (FR-UI-68); el resto de campos, verbatim.
- **FR-UI-46** — Controles, invisibles (`U+200B`, `U+FEFF`) y marcas bidi
  (`U+202A`–`U+202E`, `U+2066`–`U+2069`) se pintan como escapes visibles
  (`<U+202E>`). Sin esto, un `U+202E` deja leer «hooks.acme.io» donde el digest
  sella otra cosa: el documento diría una cosa y el digest sellaría otra, que
  es **una** de las formas de romper una pantalla
  read-through sin tocar el servidor; la otra es la equivalencia canónica, y
  la cierra FR-UI-68.
- **FR-UI-47** — El bloque de parámetros no tiene scroll propio.
  `maxApprovalParamsBytes` es **64 KiB**
  (`internal/action/sqlite/approvals.go`), y ahí está el filo: **la cota mide
  el string CRUDO y lo que se guarda es el canónico**. Para un valor JSON
  suelto —forma que el modelo puede aparcar (FR-UI-68)— la canonicalización
  **escapa** `<`, `>` y `&` a seis bytes, así que la columna puede pasar de
  64 KiB con un crudo que no llegaba. `too_large` es por tanto **alcanzable
  desde una petición legítima**, y lleva molde de servidor que lo fuerza
  midiendo **la cadena guardada**, no la de entrada.

## 7. La geometría — medida donde el producto se abre

El binario abre **1100×760** y no declara ancho mínimo
(`cmd/korvun-desktop/main.go`). La pista de la barra lateral mide 232 px y su borde de 1 px vive **dentro** de
la pista (`box-sizing: border-box`, del preflight de Tailwind importado en
`src/styles/index.css`); `.main` lleva `padding: 24px 28px 26px`
(`src/App.css`). Ancho útil **calculado** a 1100: 1100 − 232 − 56 = **812 px**.
Es cálculo sobre el CSS, **no medida**: el número que vale lo publica AS-53
desde el navegador, y la barra de scroll de `.main` puede restar ancho según
plataforma. El 1440×900 de `cmd/korvun-desktop/frontend/playwright.config.ts` es,
por su propio comentario, el contrato de captura de la review, no el de la
ventana.

| | Rechazar | Aprobar |
|---|---|---|
| tamaño | 260 × 64 px | 168 × 40 px |
| posición | borde izquierdo, primero en DOM, lectura y tabulación | borde derecho, último |
| estado inicial | siempre habilitado | deshabilitado hasta armarse |
| color | neutro | armado: tono de peligro |

- **FR-UI-48** — `área(Aprobar) ≤ 0,50 × área(Rechazar)` (real: 6 720 / 16 640 =
  **0,4038**); separación horizontal `≥ 320 px` a anchos `≥ 1100` (el cálculo da
  812 − 260 − 168 = **384 px**, sin medir); por debajo de 1100 se apilan con Rechazar
  **primero** y `≥ 120 px` de separación vertical.
- **FR-UI-49** — El rojo marca el efecto irreversible, no el rechazo.
- **FR-UI-50** — **Ningún diálogo modal en esta pantalla, nunca.** Es lo que
  deja a Esc un solo significado.
- **FR-UI-51** — Ninguno de los dos usa el gradiente de identidad.

## 8. Ciclo completo de cada elemento (plantilla UX)

| Elemento | Se abre | Se cierra | Deshacer | Cancelar a mitad |
|---|---|---|---|---|
| Entrada «Aprobaciones» del NAV | siempre visible (FR-UI-3) | cambiando de vista | no aplica | no aplica |
| Fila de la lista | clic o `↵` sobre la fila enfocada | ← Pendientes, `⌫` | no aplica | no aplica |
| **[Actualizar]** | siempre visible en la lista | vuelve solo | no aplica | la respuesta tardía no pinta si ya se navegó |
| `← Pendientes` | siempre en el detalle | navega a la lista | no aplica | descarta el armado |
| Campo de armado | con la clase que exige tecleo | al retirarse Aprobar o al salir | `⌫`; FR-UI-43 | Esc (rechaza) |
| Motivo del rechazo | siempre visible, opcional | con la pantalla | no | queda vacío |
| **Rechazar** | un clic o Esc | manda la decisión | **no hay** | no hay: es instantáneo |
| **Aprobar** | al armarse | manda la decisión y ejecuta | **no hay** | no hay durante la ejecución |
| **[Volver a leer]** | en E6 y en el detalle | repite el GET | no aplica | borra el armado y sube el scroll |
| **[Reintentar]** | en `unavailable`, E2-GET, y en los ilegibles | repite la ÚLTIMA petición GET; nunca reintenta un POST | no aplica | — |
| **[Volver a pendientes]** | en todo estado terminal | navega | no aplica | — |
| **[Arrancar el núcleo]** | en E1 | llama `Start()`; su error se pinta como el hero de Inicio | no aplica | — |
| **[Ir a Inicio]** | en E1 y en `forbidden` | cambia de vista | no aplica | — |
| **[Abrir la carpeta de configuración]** | en E3, E4 y en «superficie no montada» | `OpenConfigFolder`; su fallo se pinta (FR-UI-26) | no aplica | — |
| **[Rechazar esta petición]** (E7) | solo en `invalidated` | manda el rechazo | **no hay** | — |

**Esc.** Rechaza **mientras la petición esté abierta y sin decidir**, incluso
mientras se teclea.

- **FR-UI-52** — La línea permanente es exactamente «Esc rechaza mientras esta
  petición esté abierta y sin decidir.», idéntica en todos los casos.
- **FR-UI-53** — Esc inerte en: la lista, la ejecución en curso, y todo estado
  de E5–E9, **E9-bis incluida**: allí Rechazar existe como botón, pero Esc
  sigue inerte, y por eso su literal no lleva la línea de Esc.

## 9. Accesibilidad

- **FR-UI-54** — El digest se lee en ocho grupos de ocho con `aria-label`, la
  misma agrupación que se pinta.
- **FR-UI-55** — Los seis huecos son un campo con etiqueta y
  `aria-describedby`; la coincidencia se anuncia por `aria-live="polite"`.
- **FR-UI-56** — Cada estado es `role="status"` (condición) o `role="alert"`
  (suceso).
- **FR-UI-57** — Tabulación: documento → motivo → **Rechazar** → armado →
  **Aprobar**. Aprobar es el último foco alcanzable. Se verifica **en
  navegador** (jsdom no recorre el tabulador).
- **FR-UI-58** — AA en los dos temas, con el gate de axe del e2e.

## 10. Lo que NO hace

| No hace | Destino |
|---|---|
| Lotes | Descartado. |
| Editar lo aparcado | Descartado por construcción. |
| Avisar (badge, notificación, sonido) | Descartado en esta ola. |
| Reanudar una ejecución fallida | **Imposible**: queda `StateFailed` y `execute` la rechaza. |
| Histórico de decididas | Otra ola. |
| Contar ejecutadas sin revisar | Sin fuente. |
| Móvil y web | Fuera. |

## 11. Lo que la pantalla pide a los cuatro endpoints

El RED (`internal/controlapi/approvals_test.go`) fija hoy: las cuatro rutas, el
401 por código, `digest` y `comment` como claves de petición, `error` y
`message` como claves de error, `ApprovalsPageLimit`, y seis nombres+textos.
**No fija ninguna clave JSON de fila/detalle**: decodifica con los mismos tipos
Go, así que cualquier etiqueta pasaría.

- **FR-API-1** — `ApprovalRow`: `id`, `action_id`, `operation`, `effect_class`,
  `expires_at`, **`digest`**, **`origin`** (el canal). El digest vive **una
  vez**, en la fila; `ApprovalDetail` lo hereda por embebido.
  **De dónde salen los tres primeros:** `ListApprovals` devuelve
  `[]action.Approval`, que **no** lleva operación, clase ni canal
  (`internal/action/sqlite/approvals.go`), así que la lista los toma de la
  vista canónica del preview guardado de cada fila (la operación de la LISTA
  es presentacional; la terna que se digiere sale de `actions`, FR-UI-62). La lista **no corre el
  cinturón de verificación** (`ListApprovals` tampoco lo corre): solo lee. Una
  fila cuyo preview no se pueda LEER no desaparece — sale con su
  identificador, su digest y el cartel **«SIN CLASE LEGIBLE»**, sin clase que
  presumir y sin ofrecer nada; al abrirla, el detalle sí corre el cinturón y
  contesta E8. Una fila cuyo preview se lee pero no verifica sale con su clase
  normal **marcada como sin verificar** (la lista no corre el cinturón, así
  que su cartel de clase es lo que dice la previa, no lo que el almacén
  certifica) y es el detalle quien la rehúsa: eso es lo que dice E8 y no se
  contradice con esto. La puerta de lectura que trae la previa **sin**
  cinturón no existe hoy y la añade este tren (§17 F).
- **FR-API-2** — `ApprovalDetail` añade `purpose`, `principal_id`,
  `reversibility`, `tool_cage`, `required_rule`, `law_digest`, `parameters`,
  `parameters_state`. **Sin `law_version`** (constante, §4 P2 punto 6). **No** lleva `grant_id`, `grant_depth` ni
  `cost_line` (§4 P2 punto 5), ni `risk_summary` (FR-UI-8).
- **FR-API-3** — `expires_at` RFC3339 UTC; cadena vacía solo si `ExpiresAt` es
  cero.
- **FR-API-4** — `digest` es el `ActionDigest`, el mismo que imprime
  `approvals show` en su primera línea (G1). No el `PreviewDigest`.
- **FR-API-5** — La aprobación devuelve el desenlace real (resultado y recibo).
  **Cambia la firma de `Approve` en el RED** (§17 C, autorizado).
- **FR-API-6 (bloque `gate`)** — La lista responde
  `{"gate":{"approvals_enabled":bool,"brains_total":N,"brains_can_park":M},"rows":[…]}`.
  `brains_can_park` cuenta los cerebros que cumplen las CINCO condiciones que
  el gate exige de verdad, derivadas de la MISMA resolución que cablea las
  identidades (`internal/app/app.go`), nunca releyendo el JSON del perfil.
  Son **cinco** condiciones, y el predicado es **por herramienta**, no por cerebro suelto: existe al menos
  una herramienta en su jaula cuya clase declarada sea aparcable
  (`write_irreversible` o `critical`) **y** cuyo rango **no supere** el techo
  — un techo `write_irreversible` con una única herramienta `critical` se
  deniega por `effect_ceiling` y no aparca nada. Sobre eso, las condiciones:
  (1) el almacén de acciones está abierto (`b.actions != nil`, que a su vez
  exige el bloque `storage`); (2) es un cerebro AGENTE (`cage.HasAgent`);
  (3) su techo resuelto está en la escalera y rankea ≥ `write_irreversible`;
  (4) su jaula lista al menos una herramienta cuya clase declarada sea
  `write_irreversible` o `critical` (`internal/tool/effects.go`; hoy solo
  `webhook_call`); (5) esa herramienta está **permitida por la gobernanza**
  del cerebro — el gate de capacidad corre ANTES del de efecto
  (`internal/brain/agent.go`), así que un `mode: "deny"`, un `shadow` o un
  grant restringido por canal la matan sin llegar nunca a `effectGateRule`.
  Sin la cuarta y la quinta, un techo `critical` sobre un cerebro sin
  herramientas aparcables —o con la única aparcable denegada— contaría y no
  podría aparcar nada.
  **Alcance honesto:** la quinta condición se evalúa **por canal**
  (`policy.SelectTools` recibe `ToolQuery{Channel: …}` en cada mensaje,
  `internal/brain/agent.go`), así que no existe un booleano por cerebro
  independiente del canal: `brains_can_park` se computa sobre los canales
  configurados y es una **cota superior**, no una promesa. Un cerebro sin
  bloque de gobernanza (`decisions == nil`) es ungoverned y cumple la quinta.
  V1 y E4 imprimen ese número con ese alcance. **Cambia la forma de la respuesta de la
  lista y la costura `ListPending`** (§17 B y C, autorizado): es requisito,
  no adorno — sin él «nada puede aparcar» y «no hay nada pendiente» son la
  misma respuesta.
- **FR-API-17 (los dos digests de `invalidated`)** — El literal de E7 imprime
  el digest bajo el que se aparcó **y** el vigente. El pineado viaja en
  `law_digest`; el vigente viaja en un **campo propio** del cuerpo de error
  (`current_law_digest`), nunca dentro del `message`: el `message` es texto de
  operador con contrato byte a byte en el RED, y extraer un valor estructurado
  de una frase sería el «por texto» que E9 prohíbe. Su molde y su mutación
  entran en FR-TEST-4.
- **FR-API-19 (`params_digest_mismatch`)** — El detalle re-deriva el digest de
  los parámetros que devuelve y rehúsa con ese nombre si no cuadra
  (FR-UI-62). Es un desenlace **permanente** y sospechoso, jamás
  `unavailable`. **Alcance del nombre:** cubre «la terna o los params», no
  solo los params — `op_version` entra en el digest y ningún cinturón la
  compara hoy (`verifyApprovalStory` compara `ns/name` sin versión,
  `internal/action/sqlite/approvals.go`), así que su corrupción también cae
  aquí. Corre **antes** de clasificar `parameters_state`, así que
  precede a `empty` y a `too_large`. **Y la ausencia no se confunde con lo
  transitorio:** si la fila de `approvals` está pero la de `actions` no —**ausente**, que es lo que un `LEFT JOIN`
  distingue—, el desenlace es **`evidence_corrupt`**, permanente, jamás
  `not_found` ni `unavailable`. Solo un fallo de lectura que no distinga nada
  (un error de driver sobre la consulta entera) es `unavailable`. Escribirlo
  al revés sería repetir el JOIN que falla abierto que R15 ya cazó. **La operación sale de la fila `actions`**, no del
  preview: `action.Digest` hashea la TERNA
  `(op_namespace, op_name, op_version)` (`internal/action/action.go`) y el
  preview canónico solo guarda `"ns/name"`, sin versión
  (`internal/action/bound.go`), así que con el preview la re-derivación es
  imposible. Exige una puerta de lectura que traiga la terna y los params **de
  la misma fila y en la misma consulta**: **toca `internal/action/sqlite`**
  (§17 F).
- **FR-API-24 (una sola resolución de la ley)** — El endpoint resuelve la ley
  **una vez** por decisión y alimenta con ese mismo objeto el pin del decide y
  el ejecutor, como ya hace la CLI (`internal/cli/approvals.go`, R6-X3). De
  El endpoint usa **`BuildApprovalExecutorFromCage`**, que recibe la caja ya
  resuelta; **no** `BuildApprovalExecutor`, que vuelve a resolverla
  (`internal/app/approvals.go`) y abriría la puerta contraria: ejecutar bajo
  una jaula nueva con el pin viejo, que `ValidateApprovalBinding` dejaría
  pasar. De la resolución única se sigue que editar el perfil a mitad de una
  decisión **no** puede mover el pin ya capturado — que es lo que sostiene el acotado de la segunda mitad
  de AS-89, y ahora se apoya en el cable de este tren y no en el de otro.
- **FR-API-25 (el nombre se decide por RE-LECTURA, no por inferencia)** — Tras
  cualquier fallo del claim o del camino previo, el endpoint **vuelve a leer**
  `canonical_params` y elige entre `not_started_params_held` y
  `not_started_params_gone` según lo que la fila tenga **ahora**. Inferirlo de
  «nuestra transacción hizo rollback» sería falso en al menos dos ramas
  verificadas:
  - **el conflicto de instantánea WAL**: `ClaimApprovalParams` lee y luego
    escribe, así que si otra conexión commitea el vaciado entre ambas, el
    UPDATE no da `RowsAffected()==0` sino la clase busy —«database is locked
    (517)», que este repositorio ya capturó y nombra «a reader's transaction
    losing its write upgrade to the other connection»
    (`internal/action/sqlite/atomic_r5s5_test.go`, en el ayudante `isBusy`
    de ese fichero de test; el godoc de `isBusyClass` en
    `internal/action/sqlite/store.go` dice otra cosa —«another live
    connection legitimately holds the row»— y es la que manda en la regla 1)—. Ese error **no dice** en qué quedó la
    fila: puede venir del vaciado de un competidor o de un commit ajeno a esta
    petición, así que no se convierte en nombre — lo decide la re-lectura;
  - **`approval.Status` fuera de `APPROVED`**: `REJECTED`, `CANCELLED` y
    `EXPIRED` vacían `canonical_params` en la misma transacción que cierran
    (`internal/action/sqlite/approvals.go`). Esa rama **no entra en la
    escalera**: corta antes, tiene su propia evidencia leída y su nombre es
    `already_closed`. Mandarla a `gone` —como hacía este papel hasta la ronda
    26— afirmaba un nacimiento vacío que la purga desmiente y prometía un
    cierre que la recuperación no hace.
  **Dos ejes, y en este orden.** La ronda 30 obligó a separarlos, porque la
  escalera anterior mezclaba «qué error salió» con «qué dice la fila ahora» en
  una sola lista de primer-match, y el peldaño 1 se comía a los demás.

  **EJE 1 — qué rehusó, por CENTINELA y en TODO SITIO.** No por sitio: los
  rehúses de `GetApproval` salen por **un solo** `return` de
  `ExecuteApprovedAction` (`internal/app/approvals.go`), así que el sitio no
  distingue nada. Y —cura de la ronda 32— **no basta con mirar `GetApproval`**:
  `ClaimApprovalParams` corre LOS MISMOS cuatro cinturones
  (`ValidateApprovalBinding`, `ParseCanonicalPreview`, `ValidatePreviewBinding`,
  `verifyApprovalStory`, `internal/action/sqlite/approvals.go`), y
  `ValidateApprovalBinding` corre además en el propio `internal/app`. El eje 1
  clasifica **por lo que rehusó, corra donde corra**:

  | Rehúsa | Desenlace | Por qué |
  |---|---|---|
  | `ValidateApprovalBinding` (el pin de la ley) **desde el endpoint** | **`decided_evidence_corrupt`** | Corregido en la ronda 33, y es la clase (g): desde esta pantalla la ley **no puede haberse movido**. FR-API-24 exige UNA sola resolución, así que el `law` que juzga el ejecutor es el MISMO que acaba de pasar dentro del decide commiteado. Lo único que puede diferir es el pin ALMACENADO — y ningún UPDATE de producción lo escribe (los cuatro de `internal/action/sqlite/approvals.go` no lo tocan), así que un mismatch aquí solo lo produce una mano externa. Es corrupción permanente, y llamarla reparable mandaba al operador a devolver un perfil que nadie tocó |
| `ValidateApprovalBinding` **desde `korvun approvals execute`** | **`decided_law_moved`** | Ahí sí es reparable, porque ese verbo resuelve la ley de nuevo. **No lo pinta esta pantalla** (FR-UI-71): vive en el mapa del camino CLI |
  | `ValidatePreviewBinding`, `verifyApprovalStory`, en cualquiera de sus dos sitios | **`decided_evidence_corrupt`** | permanente: lo mutado es la fila, y ningún perfil la repara |
  | `ParseCanonicalPreview` (previa IMPASEABLE; corre con `DisallowUnknownFields()`, `internal/action/preview.go`, así que un campo de más basta) | **`decided_evidence_corrupt`** | no es un cinturón sino un parseo, así que el hueco del literal lleva el error de parseo CRUDO, no un nombre de cinturón |
  | `scanApproval` fallando en `time.Parse` sobre `requested_at`, `expires_at` o `decision_at` | **`decided_evidence_corrupt`** | corrupción permanente de evidencia, no fallo de driver |
  | `store.Get` fallando en `time.Parse` sobre `actions.requested_at` o en el `json.Unmarshal` de `authority_refs`, o en el `time.Parse` de `finished_at` (`internal/action/sqlite/store.go`) | **`decided_evidence_corrupt`** | misma clase y misma instrumentación que la fila anterior, en la otra tabla. Hasta la ronda 32 este papel la llamaba «`store.Get` ilegible» y la declaraba REPARABLE: era falso |
  | fila de `actions` ausente — por el cinturón **o** por el `store.Get` posterior, que está fuera de `GetApproval` | **`decided_evidence_corrupt`** | la recuperación selecciona `FROM actions`: no queda nada que cerrar |
  | fila de aprobación ausente (`ErrApprovalNotFound`) | **`params_unaccounted`** | |
  | fallo de driver, y solo él | **`params_unreadable`** | |

  **El residual «y solo él» exige separar `sql.ErrNoRows` del error de driver
  en TRES sitios**, no en dos: las dos consultas de `verifyApprovalStory`
  (`actions` y `action_decisions`) y el `SELECT canonical_preview` de
  `GetApproval`, que hoy envuelve la ausencia igual que un fallo de driver
  (`internal/action/sqlite/approvals.go`). Sin los tres, una fila DESTRUIDA
  tras una decisión sellada recibiría «reintenta la lectura», que es la
  simetría exacta de lo que FR-UI-38 prohíbe. Es §17 F fila 21.

  **Y lo que el eje 1 nombra, NO lo lee.** Un nombre dado por centinela se
  emite sin volver a mirar la fila, así que su literal **no puede afirmar
  estado**: no puede decir «queda con sus parámetros» ni «ninguna pasada la
  cerrará». Lo segundo sería además falso — si la columna quedó vacía, la
  pasada 1 de recuperación la cierra en el arranque siguiente
  (`state = 'APPROVED' AND NOT EXISTS (… canonical_params != '')`,
  `internal/action/sqlite/store.go`)—, y lo primero es una lectura que no se
  hizo. Por eso el literal de `decided_evidence_corrupt` afirma solo lo que su
  cinturón prueba y su columna de efecto es **«no se sabe»**: un ejecutor
  ajeno pudo ganar el claim y estar dentro de `exec.Run` mientras nosotros
  rehusamos.

  **EJE 2 — la escalera de la RE-LECTURA**, que solo se recorre cuando el eje
  1 no ha nombrado ya el desenlace — es decir, cuando el fallo NO fue de un
  cinturón ni de un parseo de evidencia: la clase busy, el `RowsAffected`, y
  la pregunta de qué tiene la fila ahora:
  1. **la fila está y conserva params** ⇒ `not_started_params_held`. Da igual
     con qué error saliera el claim, la clase busy incluida: el árbol define
     `isBusyClass` como «another live connection **legitimately holds** the
     row» (`internal/action/sqlite/store.go`), que no afirma un commit, y el
     conflicto de instantánea (517) es de BASE, no de fila;
  2. **la fila está, sus params están vacíos y `action.Digest(op, "")` SÍ
     re-deriva** ⇒ nació vacía ⇒ `not_started_params_gone`;
  3. **la fila está, sus params están vacíos y `action.Digest(op, "")` NO
     re-deriva** ⇒ `params_unaccounted`;
  4. **la re-lectura falla** ⇒ `params_unreadable`.

  **Lo que la clase busy SÍ cambia** es el diagnóstico técnico que acompaña
  al literal (`<error>`), nunca el nombre del desenlace.
- **FR-API-22 (el detalle lee en UNA instantánea)** — Hoy `GetApproval` hace
  cuatro lecturas sueltas sobre `s.db` sin transaccion (la aprobacion, la
  previa, `actions` y `action_decisions`)
  (`internal/action/sqlite/approvals.go`), y el cinturon de FR-API-19 seria
  la quinta. Entre dos de ellas cabe un rechazo legítimo de la CLI, que vacía
  `canonical_params` en su misma transacción: el cinturón vería la columna
  vacía, no re-derivaría, y la pantalla pintaría E6-ter —«esto no es
  transitorio, mira el libro»— sobre una decisión **correcta**. Por eso la
  puerta de lectura del detalle corre **el estado, la terna, los params y el
  cinturón dentro de UNA transacción**; y si aun así el estado leído no es
  `PENDING`, gana la precedencia de FR-API-18 (`already_decided` / `expired`)
  sobre el nombre del cinturon. **Lo que esta regla promete es CONSISTENCIA,
  no frescura:** SQLite en WAL da a la transacción una instantánea tomada en
  su primera lectura, así que un commit ajeno que aterrice a mitad no se ve —
  y eso es justo lo que se quiere. La lectura puede ser rancia; lo que no
  puede es estar **rota**, que es lo que pintaría E6-ter sobre un rechazo
  legítimo. **Alcance:** la transaccion cubre TODO lo que
  se sirve —la fila de aprobacion, la previa y sus dos cinturones incluidos—,
  no solo el trozo del digest; si no, el documento se pintaria desde dos
  instantaneas, que es lo que esta regla cierra. Y **ninguna lectura suelta de
  `s.db` anida dentro de esa transaccion**: el pool es de UNA conexion
  (`db.SetMaxOpenConns(1)`), asi que una lectura anidada no compite, se cuelga
  hasta el deadline. El JOIN a `actions` es **LEFT**: uno interior convertiria
  la fila huerfana en «no existe», el fail-open que R15 ya cazo.
  **Toca `internal/action/sqlite`** (§17 F).
- **FR-API-18 (la precedencia del detalle se escribe)** — Sin
  él, una aprobación decidida desde la CLI entre la lista y el detalle se
  pinta como documento vivo: la purga de params la disfrazaría de `purged` y
  el bloque de decisión seguiría ahí. **Precedencia única**, por estado de la
  fila: `EXPIRED` ⇒ `expired`; `APPROVED`/`REJECTED`/`CANCELLED` ⇒
  `already_decided`; `PENDING` ⇒ el documento; **cualquier otro valor** —la columna no tiene `CHECK`— ⇒ `already_decided`, que es la respuesta fail-closed que el dominio ya escribe para todo status desconocido. Así FR-API-9 y esta regla no
  chocan. **El campo `status` NO entra en el DTO**: bajo esa precedencia solo
  podría valer `PENDING`, y un campo de un único valor posible es el defecto
  por el que este documento ya retiró `law_version` y `grant_id`.
- **FR-API-7** — El detalle rehúsa con nombre propio lo que hoy no lo tiene:
  `invalidated` (E7) y `evidence_corrupt` (E8, el nombre del cinturón en
  `message`). Ninguno bajo `unavailable`.
- **FR-API-8** — `parameters_state` ∈
  `present|empty|unavailable|too_large`, decidido por el ESTADO de la fila y
  no por `params == ""`. **`purged` no está**: hoy los params solo se vacían en la misma
  transacción que saca la fila de `PENDING`, o en el claim, cuyos únicos
  llamantes actúan sobre una fila ya `APPROVED` — la comprobación de estado
  vive en el llamante, no dentro de `ClaimApprovalParams` (`internal/action/sqlite/approvals.go`), y con la precedencia de
  FR-API-18 esas filas responden `expired` o `already_decided` antes. Hoy `ApprovalParams` empata tres cosas en un
  `ErrNotFound` (fila ausente, params purgados, fila nacida vacía) y
  `ClaimApprovalParams` rechaza igual la fila vacía como «already claimed or
  closed» (`internal/action/sqlite/approvals.go`): por eso `empty` **no ofrece
  el sí** — aprobarla está garantizado a morir en el claim.
- **FR-API-14 (el digest se comprueba PRIMERO)** — El `approve` compara el
  digest recibido contra el `action_digest` almacenado **antes** de decidir y
  de reclamar. Ese es el `digest_mismatch` de E6. El cinturón de
  `ExecuteApprovedAction` sobre los params re-derivados es OTRO desenlace
  (E6-bis) y llega ya consumido: nombre propio y texto propio, jamás el de E6.
- **FR-API-15 (centinelas tipados en el almacén)** — E7 y E8 no se pueden
  implementar por texto. `ValidateApprovalBinding` y el cinturón de
  `GetApproval` devuelven hoy `fmt.Errorf` planos, y una fila `actions`
  ausente o ilegible sale envuelta sobre `sql.ErrNoRows`
  (`internal/action/sqlite/approvals.go`), así que corrupción y ausencia son
  el mismo error para el llamante. El tren añade centinelas
  (`ErrApprovalInvalidated`, `ErrApprovalEvidenceCorrupt`) y el endpoint mapea
  por `errors.Is`. Un mapeo por `strings.Contains` sería un hallazgo, no una
  implementación. **Toca `internal/action/sqlite`** (§17 F).
- **FR-API-20 (el cerebro que ya no está)** — Resolver la ley al leer
  (FR-API-16) añade una clase de fallo que la taxonomía §5 del papel no cubre:
  `ResolveApprovalLaw` devuelve «brain %q is not in the current config»
  (`internal/app/approvals.go`). No es `invalidated` (la ley no se movió), no
  es `unavailable` (no es transitorio), no es `evidence_corrupt`. Nombre
  propio: `brain_gone`. Y como rechazar **no** consulta la ley
  (`internal/cli/approvals.go` resuelve el pin solo en la rama approve), el
  detalle degrada a **modo solo-rechazo**. Para que eso sea posible,
  `brain_gone` **no viaja como error**: viaja como **campo del 200**. Un cuerpo
  de error no lleva documento, y E9 manda todo nombre de error a un estado
  terminal con [Volver a pendientes] — dejaría la petición incerrable, que es
  justo lo que esta regla evita. **En un POST sí es un nombre de error**
  (`ResolveApprovalLaw` falla antes de decidir, así que no se escribe nada):
  la pantalla lo pinta con su literal y **conserva Rechazar**, que no consulta
  la ley. Entra por tanto en la tabla de desenlaces con nombre.
- **FR-API-16 (la ley se juzga también al leer)** — Hoy `GetApproval` no
  compara la ley vigente: `invalidated` solo puede nacer en el toque. Para que
  E7 aparezca al abrir el detalle, el endpoint resuelve la ley y compara al
  leer, con el mismo `ValidateApprovalBinding` que usa el toque.
- **FR-API-9** — Una fila `EXPIRED` responde `expired`, no `already_decided`
  (FR-UI-31).
- **FR-API-21 (el `ErrNotFound` del claim, y lo único que se puede probar)** —
  `ClaimApprovalParams` devuelve hoy el mismo error para la fila ausente, la
  fila con `canonical_params` vacío y el claim perdido. Y la fila vacía tiene
  **tres causas indistinguibles por el estado**: nació sin argumentos, un
  competidor ya commiteó su claim y la vació, o un escritor externo la vació —
  el ataque que la propia FR-UI-62 posita. Por eso el literal de
  `params_unaccounted` **no nombra un actor**. Lo único demostrable es la
  primera: una fila nacida vacía cumple `action.Digest(op, "") ==
  action_digest` (`internal/action/action.go`). Por eso el almacén parte así,
  y **fail-closed**:
  - fila ausente ⇒ `not_found`;
  - `params == ""` **y** el digest re-deriva ⇒ nació vacía ⇒ `empty`;
  - `params == ""` **y** el digest NO re-deriva ⇒ alguien se los llevó, y el
    cable no dice quién ⇒ `params_unaccounted`, que dice «no se sabe»;
  - `RowsAffected() == 0` ⇒ centinela INTERNO del claim, **nunca un desenlace de wire**: el nombre que viaja lo decide siempre la re-lectura de FR-API-25, y con la fila intacta es `not_started_params_held` (AS-104). **Inalcanzable POR LA VÍA DE LA
    INSTANTÁNEA**, y solo por ésa: el vaciado ajeno tendría que ser visible
    dentro de nuestra propia instantánea, y ese caso sale antes por
    `params == ""`; fuera de ella el driver da la clase busy, no un cero.
    **Tiene una SEGUNDA puerta**, y el acotado de la ronda 24 no la
    mencionaba: el árbol descarta el error de `RowsAffected()`
    (`if n, _ := res.RowsAffected(); n == 0`,
    `internal/action/sqlite/approvals.go`), así que un fallo del driver ahí
    entra en la rama con `n == 0` **sin concurrencia ninguna** y sale como
    el `ErrNotFound` indistinguible. Esa puerta SÍ es forzable y SÍ lleva
    molde: es el cambio 17 de §17 F — dejar de descartar ese error — y su
    escenario es AS-104. Sin él, un fallo de driver se lee como «ya
    reclamada». Y hay una **tercera**, del mismo instrumento que §13-bis ya
    usa: un `BEFORE UPDATE ON approvals` con `RAISE(IGNORE)` hace que la fila
    se salte y `changes()` valga 0 **sin concurrencia y sin error de driver**.
    No cambia el desenlace exigido —`params_unaccounted`—; sí la enumeración,
    y es la vía barata de forzar la rama en un molde.
  El `empty` del claim se pliega a **`not_started_params_gone`**: la fila
  sigue, sus params nacieron vacíos, nada salió y la decisión quedó
  registrada. El `not_found` del claim **NO se pliega ahí** (corregido en la
  ronda 25): `ClaimApprovalParams` devuelve el mismo `ErrNotFound` para la
  fila con params vacíos y para la **fila ausente**
  (`internal/action/sqlite/approvals.go`), y una aprobación que desaparece
  DESPUÉS de una decisión sellada es destrucción de evidencia, no un
  nacimiento vacío: su columna nunca se leyó, así que no se puede afirmar
  que nadie la ejecutara, y el literal de `gone` le prometería un cierre en
  el próximo arranque que puede no llegar — si la fila de `actions` cayó por
  el `ON DELETE CASCADE` (`internal/action/sqlite/store.go`), la pasada de
  recuperación selecciona `FROM actions` y no queda nada que cerrar. Va a
  **`params_unaccounted`**. Para separarlas hace falta distinguir la fila
  ausente de la columna vacía, que es el cambio 16 de §17 F.
  Ninguno de los dos puede heredar el literal de E9 («No hay ninguna
  petición con ese identificador») — que negaría una decisión ya sellada con
  su recibo.
  **Toca `internal/action/sqlite`** (§17 F).
- **FR-API-10 (los desenlaces viajan CON NOMBRE)** — La tabla de
  desenlaces con nombre gana las **diez** filas de FR-UI-18:
  `not_started_params_held`, `not_started_params_gone`, `params_unaccounted`,
  `params_unreadable`, `decided_evidence_corrupt`, `already_closed`,
  `not_decided`, `params_belt_failed`, `unknown_outcome` y `close_failed`. Los dos primeros son
  dos nombres y no uno con dos literales, porque la pantalla no puede adivinar en qué rama está: la
  diferencia la sabe el almacén (¿la fila conserva `canonical_params`?) y solo
  viaja si tiene nombre propio. Sin ellas, caerían todas en «nombre desconocido» de E9, que es justo la cadena que sus
  AS exigen ausente. Del lado de `internal/app`, cada nombre nace de un
  centinela tipado en `ExecuteApprovedAction` — no del texto inglés — y
  `params_unaccounted` solo se emite sobre las causas que FR-API-25 enumera, y `already_closed` solo sobre los estados leídos en sus dos cortes previos al claim — el de la APROBACIÓN en el corte 1, el de la ACCIÓN en el corte 2 —, nunca inferido.
  **Toca `internal/app`** (§17 D, autorizado) **y el rojo** (§17 F).
- **FR-API-23 (el registro)** — Los nombres viven en constantes de un
  `type OutcomeName string` y en un registro `ApprovalOutcomeNames` escrito a
  partir de ellas, en `internal/controlapi`. El tipo **no** impide un literal
  suelto (Go asigna una constante *untyped* a un tipo definido), así que el
  registro es una ayuda, no una barrera: la barrera la decide el tren de
  endpoints (FR-TEST-6). **Toca el rojo** (§17 F).
- **FR-API-11** — El 401 responde con el nombre `forbidden` en el cuerpo, como
  la taxonomía §5 del papel.
- **FR-API-12** — El RED gana asserts sobre las **claves JSON crudas**.
- **FR-API-13** — `TestApprovals_RejectCommentIsBounded` acepta `413` **o**
  `400`: un nombre por ataque; se cura a uno.

## 11-bis. Qué fija esta spec y qué fija el tren de endpoints

Veintiuna pasadas adversariales han enseñado una cosa sobre este documento: cada
vez que intenta **cerrar la enumeración** de los desenlaces del servidor,
inventa una distinción que el almacén no puede hacer todavía, y la pasada
siguiente la derriba con razón. La causa no es descuido: es que la superficie
de error del endpoint **no existe aún**, y su ambigüedad real
(`ClaimApprovalParams` devolviendo un `ErrNotFound` para tres cosas,
`ExecuteApprovedAction` con diez puntos de retorno y ningún tipo) solo se
resuelve escribiendo ese código con sus moldes contra almacén real.

Así que la frontera se declara, en vez de fingir que no existe:

**Esta spec fija, y es innegociable:**
1. **FR-UI-62** y su AS-83 a dos conexiones reales — la garantía central.
2. La **ley de renderizado**: por NOMBRE, jamás por el texto inglés del
   cuerpo (E9).
3. El **fail-closed de superficie**: un nombre que la pantalla no conozca, un
   `fetch` que rechace o un cuerpo ilegible se imprimen crudos y **nunca** se
   degradan a lista vacía ni a un genérico.
4. La **regla de no ofrecer lo que el endpoint SABE que el almacén
   rechazaría**: ningún estado pinta un botón de decisión cuando la respuesta
   que el endpoint tiene en la mano dice que la fila ya no es `PENDING`. Una
   instantánea coherente pero rancia **no es conocimiento**: ahí el botón se
   pinta, y si el operador lo pulsa, el POST recibe un desenlace nombrado y seguro. Los más probables son
   `already_decided` y `expired` (si el barrendero cerró la fila entre
   medias, FR-API-9), pero la lista **no es cerrada**: `decideApprovalWithLaw`
   también puede rehusar por sus cinturones sobre una fila rancia
   (`ParseCanonicalPreview`, `ValidatePreviewBinding`, `verifyApprovalStory`)
   o por la ley (`internal/action/sqlite/approvals.go`), y todos ésos son
   nombrados y seguros igual — un desenlace nombrado y seguro, que es justo para lo que existe.
   Prometer más sería prometer frescura, que FR-API-22 declara imposible.
5. La **ley del tono aplicada al efecto**: ninguna pantalla afirma que un
   efecto ocurrió o no ocurrió más fuerte de lo que su cable sostiene.
6. Para **cada nombre que esta spec lista**: su literal exacto, su AS y su
   mutación. **Esto se verifica, no se promete** (FR-TEST-6).

**Lo fija el tren de endpoints, con código y moldes contra almacén real:**
- el **conjunto cerrado** de nombres que el endpoint emite;
- qué centinela tipado nace en `internal/app` e `internal/action/sqlite` para
  cada uno;
- cuáles de las ambigüedades de hoy se pueden partir y cuáles se declaran no
  partibles.

**Contrato de crecimiento, CON DIENTE:** todo nombre que el endpoint acabe
emitiendo y que esta spec no liste entra **en el mismo commit** con su
literal, su AS y su mutación — y eso lo comprueba un molde, no la disciplina
de nadie (**FR-TEST-6**). Un nombre sin literal enrojece el gate.

**Y lo que NO se difiere, porque son decisiones de diseño y no
descubrimientos del código:** la atomicidad de la lectura del detalle
(FR-API-22), la regla de ausencia frente a corrupción en la puerta de lectura
(FR-API-19), y la precedencia por nombre de FR-UI-64. Diferir esas tres sería
la evasión que esta sección existe para no ser.

## 12. Escenarios de aceptación

- **AS-1** El primer bloque del documento es el digest con su frase, y ambos
  botones están después de los parámetros en el DOM.
- **AS-2** (navegador) Los botones están por debajo del bloque de parámetros en
  la página, a 1100×760.
- **AS-3** Sin teclear: Aprobar `disabled`, Rechazar habilitado.
- **AS-4** Seis correctos: «✓ coincide» y Aprobar habilitado.
- **AS-5** `paste`: campo vacío, «Pegar no arma…», Aprobar `disabled`.
- **AS-6** `drop`: idéntico a AS-5.
- **AS-7** Clase `write_compensatable`: banda ANOMALÍA con su frase exacta y
  campo de armado presente.
- **AS-8** Clase `"garabato"`: «CLASE DESCONOCIDA» y armado presente.
- **AS-9** Esc con motivo «no en la ceremonia»: un POST a `/reject` con ese
  comentario y **cero** a `/approve`.
- **AS-10** Esc en la lista: cero llamadas.
- **AS-11** Esc durante «Ejecutando»: cero llamadas.
- **AS-12** `409 digest_mismatch`: literal de E6 y ningún control que apruebe.
- **AS-13** `409 digest_mismatch`: el digest nuevo **no aparece** en el DOM.
- **AS-14** [Volver a leer] tras E6: se repite el GET, el armado queda vacío.
- **AS-15** `503 core stopped`: literal de E1, cero filas, «No hay nada
  aparcado» **ausente**.
- **AS-16** `503 core unreachable` en GET: literal de E2-GET; «No hay nada
  aparcado» y el literal de E1 **ausentes**.
- **AS-17** `503 core unreachable` en POST: desenlace desconocido; «Ninguna
  decisión ha salido de esta ventana» **ausente**.
- **AS-18** `409 disabled`: literal de E3; «No hay nada aparcado» y «Nada puede
  llegar a esta lista» **ausentes**.
- **AS-19** `gate.approvals_enabled=true`, `brains_can_park=0`, cero filas:
  literal de E4; «No hay nada aparcado» **ausente**.
- **AS-20** `brains_can_park=2`, `brains_total=3`, cero filas: literal de V1 con
  «2 de 3».
- **AS-21** Reloj adelantado sobre un PENDING vigente: Aprobar retirado, armado
  borrado, **Rechazar presente y habilitado**, «Esta petición caducó»
  **ausente**.
- **AS-22** `409 expired`: literal de E5 con el instante UTC; ambos botones
  ausentes.
- **AS-23** `409 invalidated`: literal de E7, **Rechazar presente**, Aprobar
  ausente, y la cadena «Es transitorio» **ausente**.
- **AS-24** `409 evidence_corrupt` con `message:"preview_effect_mismatch"`:
  literal de E8, el nombre del cinturón impreso, ningún botón de decisión.
- **AS-25** Dos clics en Aprobar: **un solo** POST; pinta el desenlace de esa
  petición, no `already_decided`.
- **AS-26** Respuesta de `/reject` perdida: desenlace desconocido simétrico;
  «Rechazada» **ausente**.
- **AS-27** Ejecución fallida (herramienta): el literal de `unknown_outcome`;
  `korvun approvals execute` **ausente**.
- **AS-28** `close_failed`: su literal, con «esta ejecución no pudo cerrar el
  registro» **presente**; las cadenas «la ejecución falló» y «el registro no
  se cerró» (a secas, sin sujeto) **ausentes**.
- **AS-29** Desenlace desconocido tras aprobar: la cadena `korvun approvals
  execute` **ausente** (ahí el comando no abre: la acción quedó `StateFailed`,
  o los params ya se reclamaron), y «no se ejecutó» **ausente**.
- **AS-77** `not_started_params_held`: `korvun approvals execute` **presente**
  con su condicional; la frase del cierre en el próximo arranque **ausente**.
- **AS-95** `not_started_params_gone`: la frase del cierre en el próximo
  arranque **presente y condicionada** («si la acción sigue en el libro»);
  `korvun approvals execute` **ausente**.
- **AS-30** `{"error":"pepino","message":"algo raro"}`: imprime «pepino» y
  «algo raro»; no pinta lista vacía.
- **AS-31** `fetch` rechazado en la lista: literal de la última fila de E9;
  cero filas y «No hay nada aparcado» **ausente**.
- **AS-32** Cuerpo no-JSON con 200: mismo estado que AS-31.
- **AS-33** 404 con cuerpo ajeno a la API: literal de «superficie no montada».
- **AS-34** `parameters_state` **nunca** vale `purged` en el detalle: con la
  precedencia de FR-API-18, toda fila con params vaciados ya no es `PENDING` y
  responde `expired` o `already_decided` antes de llegar aquí. Un `purged`
  recibido se trata como respuesta ilegible (E9, última fila).
- **AS-35** `parameters_state:"unavailable"`: su literal, Aprobar ausente,
  [Reintentar] presente.

> **ACOTADO el 2026-09-13** — regla del director «nombre sin productor, fuera». Verificado por `grep` sobre el árbol entero: **ningún camino de producción emite este nombre**, así que su literal era interfaz muerta y su molde certificaba la nada. El nombre sale del registro y de la pantalla; este AS queda sin objeto hasta que exista un emisor.

- **AS-36** `parameters_state:"too_large"`: su literal, Aprobar ausente, la
  CLI nombrada. Lleva molde de servidor: se fuerza aparcando un valor JSON
  suelto cuyo crudo cabe en 64 KiB y cuyo canónico no.
- **AS-37** `parameters_state:"present"` exige cuerpo NO vacío: una respuesta
  con `present` y `parameters:""` se trata como respuesta ilegible del núcleo
  (E9, última fila) y **no** ofrece Aprobar. La fila nacida sin argumentos es
  `empty` (AS-64), nunca `present`.
- **AS-38** Parámetros con `U+202E`: el DOM contiene `<U+202E>` y **no** el
  carácter crudo.
- **AS-39** `purpose` con `U+202E`: idéntico en el bloque ORIGEN.
- **AS-40** `operation` con `U+202E`: idéntico en la fila de la lista.
- **AS-41** Parámetros con una URL: **cero** elementos `<a>` en el documento.
- **AS-42** Detalle abierto 30 s (reloj falso): **una sola** petición del
  detalle.
- **AS-43** Dos filas de la página con la misma cola: ambas imprimen el digest
  entero y aparece el aviso de FR-UI-12.
- **AS-44** `digest:""` y `digest:"nosoyundigest"`: «digest ilegible», sin
  Aprobar, y fuera de la detección de colisión.
- **AS-45** `expires_at:""`: «no caduca», sin cuenta atrás.
- **AS-46** `reversibility:""`: «el registro no declara reversibilidad», y el
  armado sigue presente en una irreversible.
- **AS-47** `unavailable`: su literal; «no existe» **ausente**.
- **AS-48** Aprobaciones apagadas: la entrada «Aprobaciones» **sigue** en el
  `NAV`.
- **AS-49** Aprobaciones apagadas: el `StatusChip` global **no cambia** de
  etiqueta.
- **AS-50** El cuerpo de cualquier POST a `/approve` tiene **exactamente** la
  clave `digest`, con el digest servido en el detalle.
- **AS-51** (navegador) La tecla Tab desde el motivo alcanza Rechazar, luego el
  armado, luego Aprobar, y Aprobar es el último.
- **AS-52** (navegador, 1100×760) `área(Aprobar) ≤ 0,50 × área(Rechazar)`.
- **AS-53** (navegador, 1100×760) separación horizontal `≥ 320 px`, y el
  molde **imprime el `boundingBox()` medido** en la salida del test: ese es el
  número publicado, no el cálculo de §7.
- **AS-54** (navegador, 1100×760) Rechazar precede a Aprobar en el DOM y en el
  eje X.
- **AS-55** (navegador, 900×700) apilados, Rechazar primero, `≥ 120 px`.
- **AS-56** La misma petición en clase irreversible y en clase crítica produce
  botones con **idéntica** caja (FR-UI-9).
- **AS-57** La vista no contiene ningún elemento con `role="dialog"` en ningún
  estado (FR-UI-50).
- **AS-58** Ninguno de los dos botones lleva la clase del gradiente de
  identidad (FR-UI-51).
- **AS-59** E1 no imprime ningún número de peticiones (FR-UI-22).
- **AS-60** La línea de Esc es la misma cadena exacta en irreversible, crítica
  y anomalía (FR-UI-52).
- **AS-61** `OpenConfigFolder` que rechaza: «No se ha podido abrir la carpeta»
  + la ruta (FR-UI-26).
- **AS-62** La entrada del NAV no lleva `data-unread` en ningún estado
  (FR-UI-2).
- **AS-63** AA en tema claro y oscuro (axe, en navegador).
- **AS-64** `parameters_state:"empty"`: su literal y Aprobar **ausente**.
- **AS-65 — RETIRADO.** Exigía distinguir «pre-entrega» dentro del tool, que
  es justo lo que FR-UI-18 declara imposible sin leer el texto inglés del
  error. Su lugar lo ocupan AS-66, AS-77 y AS-95.
- **AS-66** `unknown_outcome`: la cadena «no llegó a salir» **ausente**, sea
  cual sea el texto del error que lo acompañe.
- **AS-87 — RETIRADO.** Exigía el literal de un nombre que ya no existe en el
  cable: sus dos ramas son AS-77 y AS-95.
- **AS-96** (dos conexiones reales) Fila de `approvals` presente y fila de
  `actions` borrada por un escritor externo con `PRAGMA foreign_keys=OFF`: el
  detalle responde **`evidence_corrupt`**, y las cadenas de `not_found` y de
  `unavailable` **ausentes**.
- **AS-93** `forbidden` (401): su literal de E9, [Ir a Inicio] presente, y la
  cadena «No sabemos si el efecto llegó a ocurrir» **ausente**.
- **AS-94** `brain_gone` en un POST: literal de E9-bis, **Rechazar presente y
  habilitado**, Aprobar ausente.
- **AS-92** `params_digest_mismatch` en el GET del detalle: literal de E6-ter,
  sin ningún control de decisión; las cadenas «Es transitorio» y «Respuesta
  que esta pantalla no reconoce» **ausentes**.
- **AS-88** `params_belt_failed`: literal de E6-bis, sin [Volver a leer] y sin
  decisión; la cadena «Respuesta que esta pantalla no reconoce» **ausente**.

> **ACOTADO el 2026-09-13** — regla del director «nombre sin productor, fuera». Verificado por `grep` sobre el árbol entero: **ningún camino de producción emite este nombre**, así que su literal era interfaz muerta y su molde certificaba la nada. El nombre sale del registro y de la pantalla; este AS queda sin objeto hasta que exista un emisor.

- **AS-89** Un POST que muere **antes** del commit con `invalidated` (la ley
  juzgada en el decide, fila intacta y `PENDING`): se pinta E7 **con** su
  Rechazar, y la cadena «La decisión quedó registrada» **ausente**. **Segunda mitad, con el desenlace que el árbol
  produce de verdad:** mutar `approvals.policy_digest` desde una segunda
  conexión entre el commit del decide y el claim **no** da `invalidated` desde
  el endpoint. Antes disparan dos cinturones: `GetApproval` corre
  `ValidatePreviewBinding`, y la previa y la aprobación **nacen con el mismo
  pin** (`internal/action/bound.go`), así que mutar uno solo lo caza ahí como
  `preview_policy_mismatch` — y su desenlace, desde la ronda 30, es
  **`decided_evidence_corrupt`**, no `not_started_params_held`: es el mismo
  cinturón, el mismo retorno y la misma clase que AS-115, y exigirle dos
  nombres dejaba uno de los dos moldes rojo por construcción. Y si mutara también el pin de dentro de la previa,
  el `ValidateApprovalBinding` de `internal/app/approvals.go` **sí** dispararía
  —la copia que juzga es la releída en esa misma llamada, no una rancia— y su
  desenlace es el mismo `decided_evidence_corrupt` por el eje 1; si además se
  coordinara ese pin, quien rehusaría sería el de dentro del
  claim, y para llegar hasta ahí harían falta cuatro mutaciones coordinadas
  (`approvals.policy_digest`, el pin dentro de `canonical_preview`,
  `approvals.preview_digest` y `action_decisions.policy_digest`). En todas las
  temporizaciones la re-lectura encuentra los params intactos — y aun así el
  desenlace es **`decided_evidence_corrupt`**, porque desde la ronda 30 el eje
  1 de FR-API-25 nombra ANTES que la escalera: quien rehusó fue un cinturón de
  evidencia, y devolver el perfil no lo repara. **Mutación:** dejar que el
  nombre lo decida la re-lectura (el orden anterior) ⇒ saldría
  `not_started_params_held` y su literal ofrecería `korvun approvals execute`,
  que ahí muere para siempre en su propio `GetApproval`
  (`internal/cli/approvals.go`). **Y la mutación del eje 1 es por centinela,
  no por sitio:** los seis rehúses de `GetApproval` salen por un solo `return`
  de `ExecuteApprovedAction` (`internal/app/approvals.go`), así que
  neutralizar «el sitio» no distingue nada — lo que se neutraliza es el mapeo
  `errors.Is` del centinela de evidencia.
  Nivel: **dos conexiones reales**. (Una versión anterior de este AS exigía
  `invalidated` apoyándose en el orden interno del claim — que es cierto, pero
  inalcanzable desde el endpoint porque los cinturones de arriba llegan
  primero.)
- **AS-90** Nombre desconocido en un POST: desenlace desconocido y
  [Reintentar] **ausente**; en un GET, [Reintentar] presente.
- **AS-101** (dos conexiones reales, con punto de sincronización: el canal se
  cierra tras el `Commit()` confirmado del competidor) Un competidor gana el
  claim y vacía la fila; el endpoint reclama después: la fila sigue, sus
  params están vacíos y `action.Digest(op, "")` **no** re-deriva ⇒
  **`params_unaccounted`** con su literal, y la cadena «no llegó a salir»
  **ausente**. **Mutación:** decidir el nombre por la clase del error del
  driver en vez de por la fila ⇒ el robo saldría como
  `not_started_params_gone` con su promesa de cierre.
- **AS-102** (dos conexiones reales) El claim falla con la clase busy —el
  `SQLITE_BUSY` (5) que llega al agotarse el `busy_timeout` del DSN, no el 517
  de conflicto de instantánea— porque otra conexión sostiene el write lock por
  una petición DISTINTA, con la fila intacta ⇒
  **`not_started_params_held`**, con `korvun approvals execute` **presente**.
  **Dónde vive el punto de sincronización** (declarado, como exige AS-100):
  en el almacén, en una puerta de test que abre `BEGIN IMMEDIATE` sobre OTRA
  aprobación desde una segunda conexión real, señala por canal que el lock
  está tomado, y lo suelta al recibir la respuesta del endpoint. **Mutación:**
  hacer que la clase busy nombre por sí sola ⇒ saldría `params_unaccounted`
  sobre una fila que conserva sus params, negando el único camino que
  funciona (AS-91).
- **AS-103** (dos conexiones reales — **oráculo por imposibilidad**, no un
  escenario de desenlace) **La lectura no es evidencia de exclusividad.** Una
  segunda conexión abre `BEGIN IMMEDIATE`, ejecuta el mismo
  `UPDATE approvals SET canonical_params = '' WHERE approval_id = ?` que el
  claim y **no commitea**; el canal señala que el lock está tomado. Desde la
  primera conexión, la re-lectura de esa aprobación devuelve **los params
  INTACTOS**. Eso es lo que el molde exige, literalmente: bytes iguales a los
  aparcados. **Mutación: NINGUNA, y se declara así.** Lo que este molde asegura es una
  propiedad de SQLite —el aislamiento de instantánea de WAL—, no una rama de
  Korvun, así que no existe código nuestro que neutralizar: su competidor es
  SQL crudo desde la segunda conexión, y mutar `ClaimApprovalParams` no
  cambiaría una línea de lo que el molde ejecuta (además de colgarse contra
  `SetMaxOpenConns(1)`). Por eso **no es un test de garantía** y no reclama la
  regla 3: es la EVIDENCIA que justifica el acotado del literal. El test de
  garantía es AS-103-bis, y ése sí lleva su mutación. Es el hecho del árbol que prohíbe cualquier
  promesa de exclusividad, y por eso vive aquí y no en el renderizador.
  **Nivel honesto:** dos conexiones reales dentro de un proceso; **no** se
  vende como dos procesos OS ni como prueba de que el efecto salga, porque
  `ClaimApprovalParams` no ofrece ningún asidero de pausa entre su `UPDATE` y
  su `Commit()` y forzar el disparo exigiría inyección de fallo en producción,
  que este tren no autoriza.
- **AS-103-bis** (jsdom) **El literal que AS-103 obliga.** Con el desenlace
  `not_started_params_held`, el texto pintado **no** contiene «y así se
  queda» ni ninguna promesa de exclusividad, y **sí** contiene «sobre otros
  ejecutores esta pantalla no se pronuncia». **Mutación:** devolver el
  literal viejo ⇒ rojo. Es mutación de renderizador y se declara como tal:
  AS-103 prueba el hecho del almacén, AS-103-bis prueba que la pantalla no
  lo contradice.
- **AS-104** (en proceso, con un `BEFORE UPDATE ON approvals` que
  `RAISE(IGNORE)`) El UPDATE del claim se salta sin error de driver, así que
  `changes()` vale 0 **sin concurrencia ninguna**, y —esto es lo que la ronda
  28 corrigió— **`canonical_params` sigue con sus bytes**: un `RAISE(IGNORE)`
  no toca ningún `SELECT`. Por eso el desenlace exigido es
  **`not_started_params_held`**, NO `params_unaccounted`: la fila conserva sus
  params y el literal que dijera «ya no estaban donde estaban» sería
  literalmente falso sobre una fila donde están. Es el molde que fija el
  principio entero de FR-API-25 — **manda la fila, no el error del claim** —
  sobre el único camino que produce el `ErrNotFound` de «ya reclamada» con
  los params intactos. **Mutación:** nombrar por la clase del error del claim
  ⇒ saldría `params_unaccounted` sobre una fila intacta y le negaría al
  operador el comando que funciona. (El cambio 17 sigue en pie por su propio
  motivo: un error descartado es un fallo de driver leído como «ya
  reclamada». Su molde es AS-104-bis, con un driver envuelto, y su
  desenlace exigido es **también `not_started_params_held`**: el
  `defer tx.Rollback()` deshace el UPDATE, así que la fila conserva sus bytes
  pase lo que pase con `RowsAffected()`. Es uno de los DOS moldes de la lista que
  necesitan una costura de test en producción — el otro es AS-100, cuya
  barrera se declaró al reescribirla el 2026-09-12; AS-119 evita la suya
  llamando al almacén directamente, con su nivel declarado.)
- **AS-105** (dos conexiones reales) **La fila de aprobación desaparece tras
  una decisión sellada**, con params NORMALES. La segunda conexión ejecuta
  `DELETE FROM approvals WHERE approval_id = ?`; la puerta nueva no encuentra
  la fila. Desenlace exigido: **`params_unaccounted`**, **no**
  `not_started_params_gone`. La cadena «el próximo arranque la cerrará»
  **ausente**. **Mutación:** plegar la fila ausente a `gone`.
- **AS-107** (dos conexiones reales) **La misma desaparición, pero sobre una
  aprobación NACIDA VACÍA** — el caso que AS-105 no cubre y que la ronda 26
  demostró que pasaba idéntico con la implementación rota. Aquí
  `action.Digest(op, "")` **SÍ** re-deriva, así que una puerta que no
  distinga fila ausente de columna vacía daría `not_started_params_gone` y su
  promesa de cierre. El desenlace exigido sigue siendo **`params_unaccounted`**.
  **Mutación:** devolver el mismo `ErrNotFound` para la fila ausente y la
  columna vacía —el estado de hoy de `ApprovalParams`
  (`internal/action/sqlite/approvals.go`)— ⇒ rojo. Es la mutación que fija el
  cambio 18 de §17 F.
- **AS-106** (dos conexiones reales) **La acción sale de `StateApproved`
  antes del claim.** Con los params intactos, la segunda conexión lleva la
  acción a `SUCCEEDED`; el endpoint corta en la comprobación previa
  (`internal/app/approvals.go`). Desenlace exigido: **`already_closed`** con
  el estado de la ACCIÓN en el literal —el de la aprobación **no**, que ahí
  vale siempre `APPROVED` y sería un campo de un único valor posible— y **sin
  nombrar a nadie**.
  **Mutación:** devolver el literal que atribuía el cierre a «otra ejecución
  o el barrendero» ⇒ rojo, porque con los params intactos ninguna otra
  ejecución pudo terminar (el claim purga en su transacción) y el barrendero
  de caducidad solo selecciona `PENDING`.
- **AS-108** (en proceso, sobre el centinela tipado de `internal/app` — **no** sobre la salida del binario, que imprime el error crudo) **Rechazar y luego ejecutar.**
  `korvun approvals reject` cierra y purga; `korvun approvals execute` sobre
  la misma aprobación corta en `approval.Status != APPROVED`. Desenlace
  exigido: **`already_closed`**, **no** `not_started_params_gone`. La cadena
  «el próximo arranque la cerrará como desenlace desconocido» **ausente** — la
  acción quedó `REJECTED`, terminal, y la recuperación solo selecciona
  `state = 'APPROVED'`. **Mutación:** mandar esta rama a `gone` (como hacía
  el papel hasta la ronda 26) ⇒ rojo por la cadena del cierre. **Y el estado
  de la ACCIÓN no aparece en el literal**: este corte no lo lee.
- **AS-109** (en proceso, sobre el centinela tipado de `internal/app`) **Ejecutar sin haber decidido.**
  Sobre una aprobación `PENDING`, `korvun approvals execute` —que no comprueba
  el estado antes de llamar (`internal/cli/approvals.go`)— corta en
  `approval.Status != APPROVED`. Desenlace exigido: **`not_decided`**, con
  «sigue esperando decisión»; la cadena «dejó de estar esperando»
  **ausente**. **Mutación:** mandar `PENDING` a `already_closed` ⇒ rojo, porque
  el literal se contradiría en una frase.
- **AS-110** (crash-restart real, sobre el centinela de `internal/app` — camino CLI, **no** una garantía de esta pantalla: FR-UI-71) **El marcador que el almacén sí guarda.**
  El claim gana y commitea; el proceso muere durante `exec.Run`; al reabrir,
  la pasada de recuperación cierra la acción a `OUTCOME_UNKNOWN` con
  `recovery_marker = "outcome_unknown"` (`internal/action/sqlite/store.go`).
  Entonces `korvun approvals execute` sobre esa misma aprobación corta en
  `rec.State != StateApproved`. Desenlace exigido: **`already_closed`** con la
  frase del marcador —que el crash llegó DESPUÉS del claim y el efecto pudo
  haber salido— y la cadena «el almacén no guarda quién la cerró»
  **AUSENTE**. **Mutación:** ignorar `recovery_marker` y pintar siempre el
  desconocimiento ⇒ rojo, porque la pantalla ocultaría la única pista de que
  un efecto irreversible pudo ejecutarse. **Nivel: crash-restart real.**
- **AS-111** (en proceso) **La lectura que falla no acusa a nadie.** Con la
  re-lectura devolviendo un error de driver, el desenlace es
  **`params_unreadable`** y la cadena «pudo llevárselos otra ejecución»
  **ausente**. **Mutación:** colapsarlo en `params_unaccounted` ⇒ rojo: un
  fallo transitorio recibiría el texto de una sospecha permanente.
- **AS-112** (crash-restart real, camino CLI, con la misma acotación de FR-UI-71) **El marcador sobre una aprobación NACIDA
  VACÍA** — es el hallazgo P1-1 de la ronda 28 y el falsificador que AS-110 no
  tenía. El modelo emite una llamada sin argumentos (`(.*)` del carril de
  texto admite vacío, `internal/brain/protocol.go`), la petición aparca con
  `canonical_params = ''`, se aprueba, y el proceso muere. Al reabrir, la
  pasada 1 la cierra a `OUTCOME_UNKNOWN` con el marcador — **sin que ningún
  claim haya existido**, porque `ClaimApprovalParams` rechaza la fila vacía
  antes de tocar nada. El literal exigido dice «ya no conservaba sus
  parámetros» y, como `action.Digest(op, "")` **sí** re-deriva, añade que
  nació sin parámetros y que ninguna ejecución pudo arrancar. La cadena «el
  efecto pudo haber salido» **AUSENTE**. **Mutación:** leer el marcador por su
  godoc («the crash hit AFTER the params claim») en vez de por su predicado ⇒
  rojo, porque la pantalla avisaría de un efecto irreversible que el cable
  prueba imposible. **Nivel: crash-restart real.**
- **AS-113** (dos conexiones reales, **con `PRAGMA foreign_keys=OFF` en la
  conexión atacante**, declarado como en AS-96: el DSN de producción trae
  `_pragma=foreign_keys(on)` y el FK es `ON DELETE CASCADE`
  (`internal/action/sqlite/store.go`), así que sin ese pragma el `DELETE FROM
  actions` se llevaría también la fila de aprobación y el desenlace sería
  `params_unaccounted`, no el que este molde vigila) **La fila de `actions`
  desaparece después del sí.** Con la aprobación presente y su fila de
  `actions` borrada, el
  desenlace es **`evidence_corrupt`**, permanente. El desenlace es `decided_evidence_corrupt` con su literal PROPIO —el de E8 es de LECTURA y ahí sería falso: diría «no se ha leído nada como bueno» sobre una historia que el decide leyó como buena y commiteó—. La cadena `korvun approvals execute` **ausente**, la palabra «restaura» **ausente**, y la frase de la decisión sellada **presente**. **Mutación:** dejar que caiga en `held` ⇒ ofrecería un comando
  que ahí muere para siempre y prometería que ninguna pasada la ha cerrado,
  cuando la recuperación selecciona `FROM actions` y no puede cerrarla.
- **AS-115** (dos conexiones reales) **La evidencia borrada DESPUÉS del sí.**
  Con el decide ya commiteado, la segunda conexión ejecuta
  `DELETE FROM action_decisions WHERE action_id = ?` (una sentencia, sin
  `PRAGMA`: borrar el hijo de una FK siempre se permite). `GetApproval` rehúsa
  en `verifyApprovalStory` **antes** del claim, y la re-lectura encuentra la
  fila con sus params. Desenlace exigido: **`evidence_corrupt`**, permanente,
  **no** `not_started_params_held`. La cadena `korvun approvals execute`
  **ausente** y la palabra «restaura» **ausente**: la fila borrada no vuelve y
  `ValidatePreviewBinding` no depende del perfil, así que ese consejo mandaría
  al operador a una reparación imposible. **Mutación:** dejarlo caer en `held`
  por el peldaño 1 ⇒ rojo. **Variante en el mismo molde:** mutar
  `canonical_preview`, que rehúsa por `ValidatePreviewBinding` y exige el
  mismo desenlace.
- **AS-119** (llamada en proceso al almacén) **El cinturón que rehúsa DENTRO del
  claim.** Tras el decide commiteado, el molde llama **directamente a `ClaimApprovalParams`** sobre una fila con
  `preview_digest` mutado — nivel honesto: **llamada en proceso al almacén**,
  no el endpoint de punta a punta. Se hace así porque entre `store.Get` y el
  claim no hay costura ninguna en `internal/app/approvals.go` y
  `SetMaxOpenConns(1)` impide colgar la lectura desde dentro; montar el ataque
  antes haría que `GetApproval` rehusara primero por el MISMO cinturón y el
  molde pasaría idéntico sin la rama vigilada. `ClaimApprovalParams` rehúsa en
  su propio `ValidatePreviewBinding` sin tocar la fila. Desenlace exigido: **`decided_evidence_corrupt`**, **no**
  `not_started_params_held` — cuyo literal dice «si la causa fuese la
  evidencia, el desenlace sería otro» y ofrece un comando que muere para
  siempre en su propio `GetApproval`. **Mutación:** clasificar los cinturones
  por SITIO en vez de por centinela ⇒ el mismo cinturón daría dos nombres
  según dónde corriera, y uno de los dos moldes nacería rojo.
- **AS-120** (dos conexiones reales) **El `store.Get` que falla por parseo.**
  `UPDATE actions SET requested_at = 'ayer'` tras el decide: `GetApproval`
  **pasa** —`verifyApprovalStory` no lee esa columna— y el fallo aparece en
  `store.Get` (`internal/action/sqlite/store.go`). Desenlace exigido:
  **`decided_evidence_corrupt`**; la cadena `korvun approvals execute`
  **ausente** y la palabra «restaura» **ausente**. **Mutación:** tratar todo
  fallo de `store.Get` como reparable ⇒ una corrupción permanente saldría con
  comando y promesa de reparación.
- **AS-121** (en proceso, con sonda de conteo) **Una sola resolución de la ley
  por decisión.** *Añadida el 2026-09-12 por adjudicación del director: era la
  única costura del tren que no estaba declarada en esta sección.* El endpoint
  resuelve la ley **una vez** y alimenta con ese mismo objeto el pin del decide
  y el ejecutor, como manda FR-API-24. El molde monta una **sonda de conteo**
  sobre la resolución —un contador que el paquete expone a su test, incrementado
  en el punto único de resolución— y exige **exactamente 1** para un `approve`
  completo. Aserto único, sin either/or: no «una o ninguna», **una**.
  **Mutación:** llamar a `BuildApprovalExecutor` en vez de a
  `BuildApprovalExecutorFromCage` ⇒ la sonda cuenta **2** y el molde enrojece.
  Sin la sonda esa mutación NO enrojece, porque con el perfil intacto las dos
  llamadas resuelven la misma jaula y el desenlace visible es idéntico — por eso
  la sonda es el oráculo y no el resultado. Nivel: **en proceso, con sonda de
  conteo**.
- **AS-116** (dos conexiones reales) **La previa que no se puede ni leer.**
  Tras el decide commiteado, la segunda conexión hace
  `UPDATE approvals SET canonical_preview = canonical_preview || '}'` (o le
  añade un campo desconocido, que basta porque `ParseCanonicalPreview` corre
  con `DisallowUnknownFields()`). `GetApproval` sale por el parseo, **no** por
  un cinturón. Desenlace exigido: **`decided_evidence_corrupt`**, con el error
  de parseo crudo en el hueco del literal; **no** `not_started_params_held`
  (cuyo texto dice «si la causa fuese la evidencia, el desenlace sería otro»)
  y **no** `unavailable` en el GET (que diría «es transitorio» sobre un blob
  roto). **Mutación:** dejar el parseo sin centinela ⇒ cae en la escalera, la
  re-lectura encuentra los params y sale `held` ofreciendo un comando que
  muere para siempre en su propio `GetApproval`.
- **AS-117** (dos conexiones reales) **La columna de tiempo corrupta.**
  `UPDATE approvals SET requested_at = 'ayer'`, **aterrizando DESPUÉS del commit
  del decide** (si llega antes, el propio decide muere en `approvalTx` →
  `scanApproval` y el desenlace es otro, sin nombre en esta taxonomía) ⇒
  `scanApproval` falla en `time.Parse`. Desenlace exigido: **`decided_evidence_corrupt`**, y la cadena
  «Reintenta la lectura» **ausente**: es corrupción permanente, no un fallo de
  driver. **Mutación:** mandarla al residual de `params_unreadable` ⇒ un
  estado permanente se vendería como transitorio.
- **AS-118** (dos conexiones reales + reinicio) **El literal que no puede
  afirmar estado.** Tras el decide, la segunda conexión vacía
  `canonical_params` **y muta `canonical_preview`** — **no** borra
  `action_decisions`, y la ronda 32 explicó por qué: `receiptForFinish` es un
  JOIN con esa tabla (`internal/action/sqlite/ledger.go`), así que sin esa fila
  la pasada de recuperación falla, `RecoverPreviousLife` aborta y **el arranque
  muere** (pin vigente en `internal/action/sqlite/errors_test.go`: «a failing
  recovery pass must fail loud»). Con la fila de decisión intacta, el endpoint
  rehúsa por `ValidatePreviewBinding` y pinta `decided_evidence_corrupt`, y el
  arranque siguiente sí corre. El molde exige
  que su literal **no** contenga «con sus parámetros» ni «ninguna pasada
  automática la cerrará» — y lo prueba reiniciando: la pasada 1 de
  recuperación **sí** la cierra a `OUTCOME_UNKNOWN`. **Mutación:** devolver
  esas dos frases al literal ⇒ rojo, porque el arranque siguiente las
  desmiente. **Nivel: dos conexiones reales + crash-restart.**
- **AS-114** (dos conexiones reales) **El estado que nadie escribió.** La
  segunda conexión pone `status = 'BANANA'` (no hay `CHECK` en la columna).
  En el GET del detalle, la precedencia de FR-API-18 rehúsa con
  **`already_decided`** en vez de servir el documento. **No** `evidence_corrupt`:
  ningún cinturón lee `status` —ni `ValidatePreviewBinding` ni
  `verifyApprovalStory` (`internal/action/approval.go`,
  `internal/action/sqlite/approvals.go`)—, así que la historia verifica sin
  problema y no habría nombre de cinturón que imprimir. El nombre correcto lo
  escribe el propio dominio: `RuleApprovalAlreadyDecided` es «the fail-closed
  answer for **any unknown status**» (`internal/action/approval.go`); en el POST, el corte 1 da **`already_closed`** **con el valor crudo
  impreso**. La ronda 34 corrigió aquí un error mío de superficie: FR-UI-72 se
  retiró porque el DECIDE no lee el estado guardado en su rama perdedora, y yo
  trasladé esa retirada al EJECUTOR, que es otra función y sí lo lee — el corte
  1 lo lee y lo formatea en su propio error
  (`internal/app/approvals.go`). El campo tiene cable, y ocultarlo le negaba al
  operador el único dato que ese corte sí leyó, sobre un estado que solo puede
  haber escrito una mano externa — **y eso exige que el `UPDATE` aterrice DESPUÉS del commit del
  decide**: si llega antes, `decideApprovalWithLaw` corta en
  `ApprovalConsumableAt` (`internal/action/sqlite/approvals.go`) y devuelve
  `already_decided` sin llamar nunca al ejecutor. El punto de sincronización
  es el canal que se cierra tras ese commit confirmado. **Mutación:** dejar la rama por defecto sirviendo el
  documento ⇒ rojo, y es fail-open donde `internal/action/approval.go` dice
  fail-closed («the fail-closed answer for any unknown status»).
- **AS-100** (dos conexiones reales, con barrera DENTRO de la puerta de
  producción) **La instantánea del detalle.** *Reescrita el 2026-09-12 por
  adjudicación del director: la redacción anterior admitía dos lecturas y bajo
  una de ellas el molde no llamaba a la puerta de producción, así que la
  mutación no lo enrojecía. Su regla, literal: «una lectura que hace al molde
  inmune a su propia mutación no es una garantía, es una tautología».*

  **El molde llama a la puerta REAL del detalle** —la de la cura 20, la que
  abre la transacción única— y no ejecuta SQL propio dentro de ella. La puerta
  lleva una **barrera declarada**: un punto de sincronización interno del
  paquete, nulo en producción, que el molde arma para detenerla **después de su
  primera lectura**. Con la puerta detenida ahí, una segunda conexión real
  ejecuta el rechazo legítimo de la CLI —que vacía `canonical_params` en su
  misma transacción— y **confirma su `Commit`**; solo entonces el molde suelta
  la barrera y la puerta sigue leyendo dentro de la MISMA transacción.

  Aserto único, sin either/or: las lecturas posteriores devuelven **los valores
  de la instantánea** —`PENDING` y los params intactos—, así que el documento
  se sirve entero y las cadenas de E6-ter y de `params_not_canonical` están
  **ausentes**.

  **Mutación de almacén:** sacar las lecturas del detalle de su transacción
  única ⇒ la segunda lectura ve la columna ya vaciada junto al digest leído
  antes, el cinturón no re-deriva y sale E6-ter sobre un rechazo legítimo.
  **Con esta redacción la mutación SÍ enrojece**, porque el molde recorre la
  puerta mutada; con la anterior no, y ése era el hallazgo.

  **La barrera es costura de producción y se declara como tal**, junto a la de
  AS-104-bis. El «único molde que necesita una costura de test en producción»
  de AS-104 pasa a ser **dos**, y se acota ahí.

  Nivel: **dos conexiones reales con barrera dentro de la puerta de
  producción**.
- **AS-99** `Status()` que falla o sin bindings: literal de la tercera rama de
  FR-UI-63; las cadenas «El núcleo está parado» y «El proceso está en marcha»
  **ausentes**.
- **AS-98** (dos conexiones reales) **Sobre una fila cuyos params sean un
  valor JSON suelto** — la única forma en que la canonicalización no es la
  identidad, y la que hace probatoria a esta mutación —, con el detalle ya
  servido, una segunda conexión escribe en `canonical_params` unos bytes **no
  canónicos que canonizan al mismo valor**: el desenlace es
  `params_not_canonical`, no `params_digest_mismatch`, y su literal es el suyo (el mismo objeto con espacios de
  relleno, o con las claves en otro orden): el nuevo GET
  **no** pinta el documento — sale el estado de E6-ter — y no se ofrece
  decisión. **Mutación:** quitar del almacén la comparación
  `bytes == CanonicalParams(bytes)` ⇒ el documento se pinta. (La mutación
  anterior —imprimir crudo en vez de canónico— no valía: con la comparación en
  pie, el desenlace no cambiaba.)

> **ACOTADO el 2026-09-13** — regla del director «nombre sin productor, fuera». Verificado por `grep` sobre el árbol entero: **ningún camino de producción emite este nombre**, así que su literal era interfaz muerta y su molde certificaba la nada. El nombre sale del registro y de la pantalla; este AS queda sin objeto hasta que exista un emisor.

- **AS-97** `params_unaccounted`: su literal; la cadena «Otra ejecución se llevó
  esta petición» **ausente** — el cable no puede probar un tercer ejecutor.
- **AS-91** `not_decided`, `already_closed`, `params_unreadable`, `decided_evidence_corrupt`, `params_unaccounted`, `params_belt_failed`, `unknown_outcome`,
  `close_failed` y `not_started_params_gone`: la cadena
  `korvun approvals execute` **ausente** en los NUEVE.
- **AS-67 — RETIRADO.** Exigía pintar «el registro no se cerró», la frase que
  FR-UI-59 demuestra falsa en una rama alcanzable. Su lugar lo ocupa AS-28.
- **AS-68** E6-bis (cinturón de params): su literal, **sin** [Volver a leer], y
  la cadena «léela otra vez, entera» **ausente**.
- **AS-69** Esc en E5, E6, E6-bis, E7, E8 y E9: **cero** llamadas en los seis.
- **AS-70** El armado sobrevive a un `blur` de la ventana y muere al navegar a
  la lista.
- **AS-71** (navegador) [Volver a leer] deja el scroll del contenedor en 0.
- **AS-72** Parámetros con `U+200B` y `U+FEFF`: el DOM contiene sus escapes y
  no los caracteres crudos.
- **AS-73** (navegador) Autorrelleno del navegador sobre el campo de armado:
  el campo sigue vacío y Aprobar `disabled`.
- **AS-74** Fila cuyo preview no se puede LEER: sale con «SIN CLASE LEGIBLE»,
  su identificador y su digest; **no** desaparece ni presume clase.
- **AS-75** La entrada «Aprobaciones» del `NAV` contiene un `svg`.
- **AS-76** El detalle **no** imprime `law_version` en ningún estado.
- **AS-78** Detalle `200` con el campo `brain_gone: true`: el documento se
  enseña entero, **Rechazar presente y habilitado**, Aprobar **ausente**, y
  ningún estado terminal de E9 pintado.
- **AS-79** `503 core stopped` con `Status().Running = true`: se pinta el
  literal de FR-UI-63, **sin** [Arrancar el núcleo]; la cadena «El núcleo está
  parado» **ausente**.
- **AS-80** Fila `EXPIRED` en el detalle ⇒ `expired`; fila `REJECTED` ⇒
  `already_decided`. Las dos ramas, sus dos literales.
- **AS-81** La lista **no** dispara una segunda petición en 30 s (reloj falso).
- **AS-82** Una respuesta tardía de la lista no pinta si ya se navegó al
  detalle.

## 12-bis. CROSS-SCENARIOS (ley de la verificación cruzada, §4)

Escenarios de primera clase, con DOS actores reales. Ninguno es in-process.

- **AS-83 (la garantía central, FR-UI-62)** — Con la petición aparcada y el
  detalle **ya servido**, una SEGUNDA conexión al mismo fichero ejecuta
  `UPDATE approvals SET canonical_params = '{"url":"http://otro/hook"}' WHERE
  approval_id = ?`. Un nuevo GET del detalle **rehúsa** con
  `params_digest_mismatch` y **no** pinta el documento. Nivel de evidencia:
  **dos conexiones reales al mismo SQLite**. Mutación: quitar la
  re-derivación ⇒ el GET devuelve 200 con los bytes del atacante bajo el
  digest legítimo.
- **AS-84 (CLI × app, A1 del papel)** — La app tiene el detalle abierto; la
  CLI aprueba esa misma petición; la app aprueba después. La app recibe
  `already_decided` y **cero** ejecuciones adicionales. Nivel: **binario de la
  CLI en otro proceso + el servidor**.
- **AS-85 (retención × detalle)** — Pasos exactos, porque el mecanismo importa:
  (1) la app abre el detalle; (2) la **CLI rechaza** esa petición, con lo que
  la acción pasa a `REJECTED`, un estado terminal; (3) corre el prune por
  encima del cap y la borra por CASCADE — su `DELETE` filtra
  `state IN ('DENIED','SHADOWED','SUCCEEDED','FAILED','REJECTED','OUTCOME_UNKNOWN')`
  (`internal/action/sqlite/store.go`), así que **nunca toca una fila viva**;
  (4) la decisión posterior de la app
  responde `not_found` con su literal, nunca `unavailable`. Nivel: **binario
  de la CLI en otro proceso + almacén real**.
- **AS-86 — RETIRADO.** Decía forzar `unavailable` con la conexión sellada de
  la CLI ocupando el almacén. No puede: `SetMaxOpenConns(1)` acota el pool de
  **cada proceso** (`internal/action/sqlite/store.go`), y el DSN abre en WAL
  con `busy_timeout(5000)`, así que un lector externo toma su snapshot y no se
  bloquea. Agotar el pool in-process tampoco sirve: eso **espera** en el pool
  y acaba en `context deadline exceeded`, no en un error del almacén. Este
  documento **no aporta un mecanismo que fuerce `unavailable`**, así que no
  finge tenerlo: la pantalla lo pinta cuando llegue, y su alcanzabilidad queda
  fichada en §15 riesgo 4. Un AS que solo «permite» un desenlace viola el
  punto 3 de la doctrina.

## 12-ter. El registro de nombres y sus anclas (FR-TEST-6)

> **ACOTADO el 2026-09-13** — regla del director «nombre sin productor, fuera». `params_not_canonical` y `params_belt_failed` SALEN del registro: verificado por `grep` sobre el árbol entero, ningún camino de producción podía emitirlos. El registro cerrado es de **veinte** nombres hasta que alguien los emita. `parameters_state: "unavailable"` sale por lo mismo, de FR-UI-16.


Cada fila lleva su marcador estable. El molde de FR-TEST-6 cruza
`ApprovalOutcomeNames` contra esta tabla y falla si falta alguna.

| Nombre | Donde vive su literal |
|---|---|
| <!-- outcome:already_decided --> `already_decided` | E9 |
| <!-- outcome:not_found --> `not_found` | E9 |
| <!-- outcome:unavailable --> `unavailable` | E9 |
| <!-- outcome:forbidden --> `forbidden` | E9 |
| <!-- outcome:disabled --> `disabled` | E3 |
| <!-- outcome:expired --> `expired` | E5 |
| <!-- outcome:digest_mismatch --> `digest_mismatch` | E6 |
| <!-- outcome:params_digest_mismatch --> `params_digest_mismatch` | E6-ter |
| <!-- outcome:params_not_canonical --> `params_not_canonical` | E6-ter |
| <!-- outcome:invalidated --> `invalidated` | E7 |
| <!-- outcome:evidence_corrupt --> `evidence_corrupt` | E8 |
| <!-- outcome:brain_gone --> `brain_gone` | E9-bis |
| <!-- outcome:not_started_params_held --> `not_started_params_held` | P4 |
| <!-- outcome:not_started_params_gone --> `not_started_params_gone` | P4 |
| <!-- outcome:params_unaccounted --> `params_unaccounted` | P4 |
| <!-- outcome:already_closed --> `already_closed` | P4 |
| <!-- outcome:not_decided --> `not_decided` | P4 |
| <!-- outcome:decided_evidence_corrupt --> `decided_evidence_corrupt` | P4 |
| <!-- outcome:params_unreadable --> `params_unreadable` | P4 |
| <!-- outcome:params_belt_failed --> `params_belt_failed` | E6-bis |
| <!-- outcome:unknown_outcome --> `unknown_outcome` | P4 |
| <!-- outcome:close_failed --> `close_failed` | P4 |

## 13. Pruebas, evidencia y mutacion — una mutación por rama vigilada

| AS | Evidencia | Mutación (debe enrojecer) |
|---|---|---|
| AS-1 | jsdom (orden DOM) | mover el digest detrás de los botones |
| AS-2 | navegador | subir los botones sobre los parámetros |
| AS-3 | jsdom | habilitar Aprobar sin armado |
| AS-4 | jsdom | no habilitar con los seis correctos |
| AS-5 | jsdom | dejar pasar el `paste` |
| AS-6 | jsdom | dejar pasar el `drop` |
| AS-7 | jsdom | dar el clic único a compensable |
| AS-8 | jsdom | dar el clic único a la clase desconocida |
| AS-9 | jsdom | mapear Esc a «cerrar» |
| AS-10 | jsdom | dejar Esc activo en la lista |
| AS-11 | jsdom | dejar Esc activo durante la ejecución |
| AS-12 | jsdom | pintar E5 en `digest_mismatch` |
| AS-13 | jsdom | imprimir el digest nuevo en E6 |
| AS-14 | jsdom | conservar el armado tras [Volver a leer] |
| AS-15 | jsdom | pintar V1 sobre `core stopped` |
| AS-16 | jsdom | usar el texto de E1 para `core unreachable` |
| AS-17 | jsdom | usar el texto de E2-GET en el POST |
| AS-18 | jsdom | pintar V1 sobre `disabled` |
| AS-19 | jsdom | pintar V1 con `brains_can_park=0` |
| AS-20 | jsdom | escribir V1 sin los números del `gate` |
| AS-21 | jsdom + reloj falso | retirar también Rechazar al pasar el reloj |
| AS-22 | jsdom | tratar `expired` como `already_decided` |
| AS-23 | jsdom | mapear `invalidated` a `unavailable` |
| AS-24 | jsdom | mapear `evidence_corrupt` a `unavailable` |
| AS-25 | jsdom + contador | quitar el `disabled` del primer clic |
| AS-26 | jsdom | pintar «Rechazada» sobre el fetch rechazado |
| AS-27 | jsdom | ofrecer `execute` en el fallo de herramienta |
| AS-28 | jsdom | pintar «la ejecución falló» sobre el fallo de cierre |
| AS-29 | jsdom | ofrecer `approvals execute` en el desconocido |
| AS-77 | jsdom | pintar el literal de `not_started_params_gone` |
| AS-95 | jsdom | pintar el literal de `not_started_params_held` |
| AS-30 | jsdom | degradar el nombre desconocido a lista vacía |
| AS-31 | jsdom | tragar el rechazo del fetch y pintar cero filas |
| AS-32 | jsdom | tratar el cuerpo ilegible como lista vacía |
| AS-33 | jsdom | tratar el 404 ajeno como `not_found` |
| AS-34 | jsdom | aceptar `purged` como estado normal del detalle |
| AS-35 | jsdom | mapear `unavailable` a `purged` |
| AS-36 | jsdom | ofrecer Aprobar por encima de la cota |
| AS-37 | jsdom | aceptar `present` con cuerpo vacío y ofrecer Aprobar |
| AS-38 | jsdom | quitar el escapado bidi de los parámetros |
| AS-39 | jsdom | aplicar el escapado solo a los parámetros |
| AS-40 | jsdom | no escapar la fila de la lista |
| AS-41 | jsdom | activar el autoenlazado |
| AS-42 | jsdom + reloj falso | añadir un `setInterval` de refresco |
| AS-43 | jsdom | quitar la detección de colisión |
| AS-44 | jsdom | recortar un digest sin validar su forma |
| AS-45 | jsdom | pintar cuenta atrás sobre `expires_at` vacío |
| AS-46 | jsdom | dejar que `reversibility` decida la puerta |
| AS-47 | jsdom | redactar `unavailable` como «no existe» |
| AS-48 | jsdom | esconder la entrada del NAV con `disabled` |
| AS-49 | jsdom | escribir en el chip desde esta vista |
| AS-50 | jsdom (assert de estado final sobre el cuerpo) | añadir una clave con los seis caracteres |
| AS-51 | navegador | poner Aprobar antes que Rechazar en el orden de foco |
| AS-52 | navegador · `boundingBox()` | agrandar Aprobar al tamaño de Rechazar |
| AS-53 | navegador · `boundingBox()` | juntar los dos botones |
| AS-54 | navegador · `boundingBox()` | intercambiar sus posiciones |
| AS-55 | navegador · `boundingBox()` | apilar con Aprobar primero |
| AS-56 | jsdom | cambiar el tamaño de Aprobar según la clase |
| AS-57 | jsdom | envolver un estado en `role="dialog"` |
| AS-58 | jsdom | poner el gradiente en Aprobar |
| AS-59 | jsdom | imprimir el número de pendientes en E1 |
| AS-60 | jsdom | acortar la línea de Esc en un caso |
| AS-61 | jsdom | tragar el error de `OpenConfigFolder` |
| AS-62 | jsdom | añadir `data-unread` a la entrada |
| AS-63 | navegador (axe) | bajar el contraste del cartel de clase |
| AS-64 | jsdom | ofrecer Aprobar con `empty` |

| AS-92 | jsdom | mapear `params_digest_mismatch` a `unavailable` |
| AS-93 | jsdom | mandar el 401 al desenlace desconocido |
| AS-96 | **dos conexiones reales** | servir la fila huerfana como `not_found` o `unavailable` |
| AS-94 | jsdom | esconder Rechazar en `brain_gone` |
| AS-88 | jsdom | mandar el cinturón post-claim a «nombre desconocido» |
| AS-89 (1ª mitad) | jsdom | usar el mismo literal para el `invalidated` de antes y el de después del commit |
| AS-89 (2ª mitad) | **dos conexiones reales** | dejar que el nombre lo decida la re-lectura (el orden anterior a la ronda 30) ⇒ saldría `not_started_params_held` en vez de `decided_evidence_corrupt` |
| AS-90 | jsdom | ofrecer Reintentar tras un POST desconocido |
| AS-97 | jsdom | afirmar un tercer ejecutor en el literal |
| AS-101 | **dos conexiones reales** | decidir el nombre por la clase del error del driver en vez de por la fila |
| AS-102 | **dos conexiones reales** | dejar que la clase busy nombre por sí sola ⇒ `params_unaccounted` con los params intactos |
| AS-103 | **dos conexiones reales** (oráculo por imposibilidad) | **sin mutación por construcción** — no es test de garantía; ver AS-103 |
| AS-103-bis | jsdom | devolver el literal con «y así se queda» |
| AS-104 | en proceso | volver a descartar el error de `RowsAffected()` |
| AS-105 | **dos conexiones reales** | plegar la fila ausente a `gone` |
| AS-107 | **dos conexiones reales** | devolver el mismo `ErrNotFound` para fila ausente y columna vacía |
| AS-106 | **dos conexiones reales** | devolver el literal que atribuía el cierre a un actor |
| AS-108 | en proceso CLI | mandar `approval.Status` terminal a `gone`, o imprimir un estado de acción que el corte no leyó |
| AS-109 | en proceso CLI | mandar `PENDING` a `already_closed` |
| AS-110 | **crash-restart real** | ignorar `recovery_marker` y pintar siempre el desconocimiento |
| AS-111 | en proceso | colapsar el fallo de lectura en `params_unaccounted` |
| AS-112 | **crash-restart real** | leer el marcador por su godoc en vez de por su predicado |
| AS-113 | **dos conexiones reales** | dejar la fila de `actions` ausente cayendo en `held` |
| AS-114 | **dos conexiones reales** | servir el documento con un `status` desconocido |
| AS-115 | **dos conexiones reales** | neutralizar el mapeo `errors.Is` del centinela de evidencia ⇒ cae en `held` |
| AS-116 | **dos conexiones reales** | dejar el parseo de la previa sin centinela |
| AS-117 | **dos conexiones reales** | mandar la columna corrupta al residual de `params_unreadable` |
| AS-118 | **dos conexiones reales + crash-restart** | devolver al literal «con sus parámetros» y «ninguna pasada la cerrará» |
| AS-119 | llamada en proceso al almacén | mandar los cinturones del claim a `held` |
| AS-120 | **dos conexiones reales** | llamar «reparable» a un `store.Get` que falla por parseo |
| AS-100 | **dos conexiones reales** | sacar las lecturas del detalle de su transacción única |
| AS-99 | navegador contra el arnés | afirmar una de las dos ramas cuando `Status()` falla |
| AS-98 | **dos conexiones reales** | quitar la comparación de canonicidad en la puerta de lectura |
| AS-91 | jsdom | imprimir `execute` en `params_unaccounted`, `params_belt_failed` o `unknown_outcome` |
| AS-66 | jsdom | leer el texto del error para decidir la rama |

| AS-68 | jsdom | reutilizar el texto de E6 en E6-bis |
| AS-69 | jsdom | dejar Esc activo en uno solo de los seis |
| AS-70 | jsdom | borrar el armado en el `blur` |
| AS-71 | navegador | conservar el scroll tras [Volver a leer] |
| AS-72 | jsdom | escapar solo las marcas bidi y no los invisibles |
| AS-73 | navegador | aceptar el `input` de autorrelleno como tecleo |
| AS-74 | jsdom | omitir la fila cuyo preview no verifica |
| AS-75 | jsdom | dejar un punto en vez del icono |
| AS-76 | jsdom | imprimir `law_version` en el detalle |
| AS-78 | jsdom | esconder Rechazar en `brain_gone` |
| AS-79 | navegador contra el arnés | usar el literal de E1 con el núcleo en marcha |
| AS-80 | jsdom | mapear `EXPIRED` a `already_decided` |
| AS-81 | jsdom + reloj falso | añadir un `setInterval` a la lista |
| AS-82 | jsdom | dejar que la respuesta tardía pinte |
| AS-83 | **dos conexiones reales al mismo SQLite** | quitar la re-derivación del detalle |
| AS-84 | **binario CLI en otro proceso + servidor** | aceptar la segunda decisión |
| AS-85 | almacén real | mapear la fila borrada a `unavailable` |


- **FR-TEST-1 (cómo aparca el arnés)** — El modelo falso del arnés
  (`cmd/korvun-desktop/e2e-harness/main.go`, `newFakeModel`) devuelve siempre
  un mensaje de asistente plano: **nunca una llamada a herramienta**, así que
  hoy el arnés no puede aparcar por el camino real. Dos opciones, y la spec
  elige la segunda declarando su nivel:
  1. enseñar al modelo falso a emitir una tool call y dar al perfil del arnés
     `approvals.enabled` + `agent.effect_ceiling` + la jaula de `webhook_call`
     (aparca por el pipeline real);
  2. **`POST /__test/park`** en la superficie de test, que crea la petición
     por el almacén real. Nivel de evidencia honesto: «fila creada por la
     superficie de test contra el almacén real», **no** «aparcada por el
     pipeline». Los AS de navegador miden geometría y foco, no el gate.
- **FR-TEST-2 (aislamiento)** — El arnés es UNO, compartido, `workers: 1`
  (`playwright.config.ts`). El e2e de aprobaciones **estrena arnés y puerto
  propios** (la tercera instancia, junto a `HARNESS_ADDR` y `FRESH_ADDR`); no
  se apoya en el estado de arranque compartido.
- **FR-TEST-5 (cada molde de servidor declara SU mutación)** — Las mutaciones de servidor viven en §13-bis, una por molde de FR-TEST-4, no en un «con su mutación»
  genérico. Cada una neutraliza la rama del ENDPOINT o del ALMACÉN que
  vigila, nunca la del renderizador.
- **FR-TEST-6 (el gate del contrato de crecimiento)** — El requisito es de
  esta spec; **el mecanismo lo cierra el tren de endpoints**, y se entrega con
  el obstáculo ya localizado para que nadie lo redescubra:
  - **El requisito:** ningún nombre llega a la pantalla sin literal. Se
    comprueba cruzando el conjunto de nombres que el endpoint emite contra las
    anclas de §12-ter, en las **dos** direcciones (un ancla huérfana también
    enrojece), y el molde **falla duro** si el fichero de anclas no se lee —
    nunca se salta.
  - **El obstáculo, ya medido:** `internal/controlapi` **ya tiene** un emisor
    de `{"error": …}` tipado `string` —`writeError` en `mutation.go`— con **41
    llamadas, 39 de ellas con literal** en `mutation.go` y `console.go`. Y su
    forma es `{"error": <mensaje humano>}`, **no** el `{"error": <nombre>,
    "message": <texto>}` que el rojo exige, así que la superficie de
    aprobaciones no puede reutilizarlo: la barrera se decide sobre un emisor
    propio. Por eso NO sirve ni cerrar por tipo (una constante *untyped* entra en un tipo
    definido de base `string`: comprobado con un `go build` mínimo y presente
    en el árbol, `internal/brain/agent_ceiling_test.go`) ni un barrido de AST
    que prohíba literales en general: enrojecería los 39 sitios legítimos
    preexistentes.
  - **Lo que el tren de endpoints tiene que decidir y probar:** o los handlers
    de aprobaciones usan un emisor propio y el barrido de AST se acota **a los
    handlers que registra `RegisterApprovals`** (con `writeError` en la
    allow-list para el resto del paquete), o se elige otro mecanismo. El
    precedente de barrido por sitio con receptor exacto y allow-list vive en
    `internal/app/effectivecage_r4f5_test.go`.
  - **Su mutación, sea cual sea el mecanismo:** emitir desde un handler de
    aprobaciones un nombre que no esté en §12-ter debe enrojecer. Se ejecuta
    con `purged` —un nombre que YA aparece en la prosa, y que AS-34 prohíbe—
    para demostrar que el molde cruza anclas y no busca palabras.
  Mientras ese molde no exista, **la protección que sí es de esta pantalla
  sigue en pie y es la que importa para el operador**: un nombre desconocido
  se imprime crudo y nunca se degrada (FR-UI-37, AS-30).
- **FR-TEST-3** — Ninguna aserción acepta dos clases. Hoy hay **dos** en el
  rojo: `TestApprovals_RejectCommentIsBounded` acepta `413` **o** `400`
  (FR-API-13), y `TestApprovals_DisabledIsNotAnEmptyList` acepta **cualquier
  no-200** en la ruta de la LISTA. El nombre `disabled` sí lo fija ese mismo
  molde, y el 409 con su nombre y su texto los fija
  `TestApprovals_NamedOutcomes`: lo que no fija nada es el **código en la
  lista**, del que depende E3. Y hay un tercer molde flojo:
  `TestApprovals_ListIsBounded` pasa con cero filas. Los tres se curan
  (§17 F, filas 6, 11 y 12).
- **FR-TEST-4 (las curas de servidor se prueban contra el almacén real)** —
  Los AS de §12 alimentan a la PANTALLA con respuestas fabricadas: pasarían
  idénticos aunque el endpoint no produjera nunca `expired`, `invalidated`,
  `evidence_corrupt` ni distinguiera el fallo de cierre. Cada FR-API que
  afirma algo del servidor lleva **su propio molde en `internal/controlapi` o
  `internal/app` contra un almacén SQLite real**, con su mutación:
  fila barrida a `EXPIRED` ⇒ `expired` (FR-API-9); pin de ley movido ⇒
  `invalidated` al leer y al tocar (FR-API-16); historia mutada ⇒
  `evidence_corrupt` (FR-API-15); `FinishWithResult` forzado a fallar tras un
  `exec.Run` con éxito ⇒ **`close_failed`**, no `unknown_outcome` (FR-API-10); fila nacida vacía
  ⇒ `empty` (FR-API-8); digest rancio ⇒ `digest_mismatch` **sin** consumir la
  aprobación (FR-API-14); perfil con un cerebro que cumple cuatro condiciones
  y falla la quinta ⇒ `brains_can_park` = 0 (FR-API-6); fila cuyo preview no
  se puede leer ⇒ sale en la lista sin clase (FR-API-1); fila decidida entre
  la lista y el detalle ⇒ `already_decided` (FR-API-18); params mutados por
  otra conexión ⇒ `params_digest_mismatch` **antes** de pintar (FR-API-19, y
  es AS-83); `current_law_digest` presente en el cuerpo de `invalidated`
  (FR-API-17); cerebro ausente del perfil ⇒ `brain_gone` (FR-API-20); el claim que pierde el upgrade de escritura ⇒
  competidor que vacía la fila ⇒ `params_unaccounted`, no `gone`; busy ajeno con la fila intacta ⇒ `held`, no `params_unaccounted`; claim ajeno sin commitear ⇒ la re-lectura ve los params intactos, y por eso ningún literal promete exclusividad (FR-API-25, y son AS-101, AS-103 y AS-102); **una sola
  resolución de la ley por decisión** (FR-API-24), con su mutación: resolver
  dos veces —o llamar a `BuildApprovalExecutor` en vez de a su variante
  `FromCage`— debe enrojecer; params
  mutados **entre el commit del decide y el claim** ⇒
  `params_belt_failed`, con la aprobación consumida y la fila vaciada
  (FR-API-10, AS-88); fallo
  del sellador al cerrar un rechazo ⇒ la transacción rueda atrás y la
  aprobación sigue `PENDING` (`internal/action/sqlite/ledger.go`); bytes no canónicos que canonizan al mismo
  valor ⇒ el detalle rehúsa (FR-UI-68, y es AS-98); valor JSON suelto cuyo crudo cabe en 64 KiB y cuyo
  canónico no ⇒ `too_large` (FR-UI-47); rechazo de la CLI confirmado entre dos lecturas
  del detalle ⇒ la instantánea sirve el documento coherente y nunca E6-ter
  (FR-API-22, y es AS-100); params
  vaciados por un competidor ⇒ `params_unaccounted` (FR-API-21).
  **Estos veintiún moldes**, no «todos»: el resto de las FR-API se prueban por la
  pantalla.

## 13-bis. Las mutaciones de servidor (FR-TEST-5)

| Molde de FR-TEST-4 | Mutación que debe enrojecerlo |
|---|---|
| params mutados y leídos en el detalle ⇒ `params_digest_mismatch` **antes** de clasificar | devolver `unavailable` o `empty` en vez del nombre |
| `policy_digest` mutado tras el commit ⇒ `decided_evidence_corrupt` (el eje 1 nombra antes que la escalera) | dejar que el nombre lo decida la re-lectura |
| competidor que gana el claim y vacía la fila ⇒ `params_unaccounted` | decidir el nombre por la clase del error del driver en vez de por la fila |
| claim ajeno sin commitear ⇒ la re-lectura ve los params INTACTOS | **sin mutación por construcción**: la propiedad es de SQLite, no de una rama nuestra |
| fila borrada tras la decisión, params normales ⇒ `params_unaccounted` | plegar la fila ausente a `gone` |
| fila borrada sobre una aprobación NACIDA VACÍA ⇒ `params_unaccounted` | devolver el mismo `ErrNotFound` para fila ausente y columna vacía |
| error de `RowsAffected()` ⇒ `not_started_params_held` (el `defer tx.Rollback()` deja los params intactos) | descartarlo con `n, _ :=`, o nombrarlo `params_unaccounted` sobre una fila que los conserva |
| cortes previos al claim con estados terminales ⇒ `already_closed` | mandarlos a `gone`, o imprimir en el corte 1 un estado de acción que no se leyó |
| aprobación `PENDING` ⇒ `not_decided` | mandarla a `already_closed` |
| acción cerrada con `recovery_marker` ⇒ el literal lo dice | ignorar el marcador |
| re-lectura fallida ⇒ `params_unreadable` | colapsarlo en `params_unaccounted` |
| aprobación nacida vacía cerrada por la recuperación ⇒ sin frase de efecto | leer el marcador por su godoc |
| fila de `actions` ausente tras el sí ⇒ `evidence_corrupt` | dejarla caer en `held` |
| `status` desconocido ⇒ `already_decided` (GET) / `already_closed` (POST) | servir el documento |
| fila de decisión borrada o previa mutada tras el sí ⇒ `decided_evidence_corrupt` | neutralizar el mapeo del centinela ⇒ cae en `held` y ofrece `execute` |
| previa impaseable ⇒ `decided_evidence_corrupt` | dejar el parseo sin centinela |
| `requested_at` corrupto ⇒ `decided_evidence_corrupt` | mandarlo al residual de driver |
| el literal nombrado por centinela no afirma estado | devolverle «con sus parámetros» |
| claim saltado por trigger con params intactos ⇒ `held` | nombrar por la clase del error del claim |
| busy por un commit ajeno con la fila intacta ⇒ `not_started_params_held` | dejar que la clase busy nombre por sí sola |
| rechazo de la CLI **confirmado entre la primera y la segunda lectura** del detalle ⇒ la instantánea sirve el documento coherente, y **nunca** E6-ter | sacar las lecturas de la transacción única |
| bytes guardados no canónicos ⇒ E6-ter sin decisión (FR-UI-68, AS-98) | quitar la comparación `bytes == CanonicalParams(bytes)` |
| valor JSON suelto cuyo canónico pasa de 64 KiB ⇒ `too_large` | medir la cota sobre el string crudo en vez de sobre el guardado |

| fila barrida a `EXPIRED` ⇒ `expired` | mapear el estado `EXPIRED` a `already_decided` |
| pin de ley movido ⇒ `invalidated` al leer y al tocar | quitar la comparación de ley del camino de lectura |
| historia mutada ⇒ `evidence_corrupt` | envolver el fallo del cinturón sobre `sql.ErrNoRows` |
| `FinishWithResult` forzado a fallar (trigger `BEFORE UPDATE ON actions` que `RAISE` sobre `state='SUCCEEDED'`, el molde que el árbol ya usa) ⇒ **`close_failed`** | emitir `unknown_outcome` para el fallo del cierre |
| fila nacida vacía ⇒ `empty` | devolver `params_unaccounted` para toda fila vacía (la rama vigilada es la re-derivación, no el vacío) |
| digest rancio ⇒ `digest_mismatch` sin consumir | comprobar el digest después de decidir |
| `brains_can_park` = 0 por fallar la quinta condición | contar el cerebro sin mirar la gobernanza |
| previa ilegible ⇒ fila sin clase en la lista | omitir la fila |
| decidida entre lista y detalle ⇒ `already_decided` | servir el documento de una fila no `PENDING` |
| `current_law_digest` en el cuerpo de `invalidated` | omitir el campo |
| cerebro ausente ⇒ `brain_gone` en el 200 | devolverlo como error |
| params mutados entre commit y claim ⇒ `params_belt_failed` | devolver `unknown_outcome` |

| fallo del sellador al cerrar un rechazo ⇒ tx revertida, fila `PENDING` | commitear la decisión sin recibo |
| una sola resolución de la ley por decisión ⇒ el ejecutor se arma con la caja ya resuelta | llamar a `BuildApprovalExecutor`, que vuelve a resolver |
| params vaciados por un competidor ⇒ `params_unaccounted` | tratar todo `params == ""` como `empty`, sin comprobar el digest |

## 14. Decisiones del director, confirmadas 2026-09-08

Sin refresco automático · pegar no arma · la geometría no se mueve con la
clase · clase fuera de la escalera exige tecleo · la entrada del menú sigue
visible con aprobaciones apagadas · motivo del rechazo opcional encima de los
botones · parámetros por encima del tope no ofrecen el sí y nombran la CLI
(la cifra baja de 128 KiB a **64 KiB**, el tope real del almacén — §17 E).

Y como garantías del tren: el renderizador de texto no confiable
(FR-UI-45/46), `digest_mismatch` sin digest nuevo (FR-UI-32), el 503 partido en
tres estados con tres textos.

## 15. Riesgos abiertos

1. **Esc.** Un no es un no registrado. Mitigado con un solo significado en toda
   la superficie (FR-UI-50, FR-UI-53); la pasada manual lo ataca a propósito.
2. **La ejecución síncrona.** Un webhook lento deja la ventana en «Ejecutando»
   tanto como aguante la jaula.
3. **Los parámetros crudos.** Derecho de loopback de ADR-0024; lo que añade
   esta pantalla es que esos bytes vienen de un modelo: FR-UI-45/46.
4. **`unavailable` sigue SIN FORZARSE**, y este tren no lo resuelve: ni
   cross-proceso (WAL + `busy_timeout`, pools por proceso) ni agotando el pool
   in-process (eso espera y expira por contexto). AS-86 se retiró por eso. La
   pantalla tiene su estado y su literal; la alcanzabilidad es la misma que
   R15 retiró y que A11 del papel deja abierta.
5. **Estados que hoy solo alcanza la corrupción** — FR-UI-5, FR-UI-7 y
   FR-UI-13 — se declaran como defensas, no como casos ordinarios. **FR-UI-47
   no está aquí**: `too_large` lo alcanza un valor JSON suelto con muchos
   `<`, `>` o `&`, porque la cota mide el crudo y el escapado infla el
   canónico.

## 15-quater. Tres `RowsAffected()` más con el error descartado, fichados

§17 F fila 17 cura el `n, _ := res.RowsAffected()` del claim. El árbol tiene
al menos tres más de la misma forma: en el one-shot del DECIDE, en
`transitionTx` (`internal/action/sqlite/approvals.go`) y en `closeCrashOrphan`
(`internal/action/sqlite/store.go`). Dos tocan esta pantalla. El del DECIDE: un error ahí saldría como `already_decided` sobre una fila que sigue
`PENDING` —la transacción rueda atrás—, y la primera cláusula del literal
(«ya no está esperando decisión») sería falsa. Y el de `closeCrashOrphan`, que
vive en la pasada 1 de recuperación — el cierre que el literal de
`not_started_params_gone` promete: su llamante descarta también el bool
(`internal/action/sqlite/store.go`), así que un no-cierre silencioso es
indistinguible de una carrera legítimamente perdida.

**No se cura en este tren y no se le pide molde**: con el driver actual
(`modernc.org/sqlite`) no consta que ese error nazca, y este papel ya aceptó
la clase como forzable solo con driver envuelto (AS-104-bis). Queda fichado
con su evidencia, junto a la asimetría de curar uno de los cuatro.

## 15-ter. Un efecto colateral del ataque de AS-115, fichado

Borrar `action_decisions` tras una decisión sellada no solo corrompe la
evidencia: deja el almacén **sin arrancar**. La pasada 1 de recuperación
selecciona la acción, hace su UPDATE y llama a `receiptForFinish`, que es un
JOIN con `action_decisions` (`internal/action/sqlite/ledger.go`); sin esa fila
devuelve `sql.ErrNoRows`, el error no-busy aborta `RecoverPreviousLife` y el
arranque muere con «app: recover previous life» (`internal/app/app.go`). El
pin que lo fija es deliberado —«a failing recovery pass must fail loud
(boot-fatal)», `internal/action/sqlite/errors_test.go`— así que **es
comportamiento querido**, no un defecto. Se ficha porque cambia lo que un
molde puede usar como oráculo: AS-118 no puede apoyarse en un arranque que no
llega a ocurrir, y por eso ataca la previa en vez de la fila de decisión.

## 15-bis. Un hallazgo SOBRE EL ÁRBOL, fichado y no curado aquí

`recoveryMarkerOutcomeUnknown` lleva este godoc
(`internal/action/sqlite/store.go`):

> the crash hit AFTER the params claim — the external effect may have fired

Su predicado no dice eso: selecciona `state = 'APPROVED' AND NOT EXISTS (…
canonical_params != '')`, o sea **params ausentes**, que una aprobación
nacida vacía cumple sin que ningún claim exista. El godoc es más ancho que su
cable, que es exactamente lo que la ley del Tono prohíbe en cualquier punto
del árbol.

Y no está solo en su fichero. Dos más de la misma clase, fichados con él:

- `RecoverPreviousLife` promete «an APPROVED action whose params were
  **CLAIMED** was mid-execution» — mismo predicado, misma anchura de más;
- el godoc de `Record.RecoveryMarker` dice «"" normally and
  "crash_recovered"» y **omite `outcome_unknown`**, que es el valor del que
  depende toda esta rama: más ESTRECHO que su cable.

Este tren **no los cura**: no tiene código autorizado en ese fichero y el
alcance manda. Queda fichado con su evidencia para quien toque
`internal/action/sqlite/store.go`, y la spec ya no se apoya en esa frase —
lee el predicado.

## 16. Cambios respecto a la v1 que el director vio

| Antes (v1) | Ahora | Por qué |
|---|---|---|
| Reversible y compensable con un clic | ANOMALÍA con tecleo | esas clases no pueden aparcar hoy |
| V1 prometía «aparecerá aquí» | E4 + `gate` con números | dos llaves, y la segunda con grado |
| El reloj local afirmaba la caducidad y retiraba los dos botones | retira solo Aprobar | un reloj adelantado dejaba sin poder rechazar |
| Geometría a 1440×900 | a 1100×760 | es la ventana que abre el binario |
| `execute` ofrecido en el fallo de ejecución, y luego en el desconocido | solo en el fallo del ejecutor | es el único sitio donde el comando abre: params intactos y acción no terminal |
| Sin estados para ley movida ni evidencia corrupta | E7 y E8 | eran negativas permanentes cayendo en «es transitorio» |
| 128 KiB | 64 KiB | es el tope del almacén |

## 17. Adjudicaciones del director (2026-09-08 / 09)

| | Pregunta | Decisión |
|---|---|---|
| **A** | La graduación | **SÍ**: toda clase que no sea `write_irreversible` ni `critical` exige tecleo. No contradice la decisión de §0, la completa: una fila de otra clase solo puede venir de corrupción, y al estado más sospechoso no se le da la puerta más barata. La pantalla lo dice con esas palabras (FR-UI-5). |
| **B** | El bloque `gate` | **SÍ**, ampliando el alcance del tren de endpoints: sin él, «nada puede aparcar» y «no hay nada pendiente» son la misma respuesta, que es exactamente el hueco que G7 obliga a enseñar. Es un requisito del bloque (FR-API-6). |
| **C** | Los cuatro cambios sobre el rojo aprobado | **SÍ** a los cuatro, incluida la lista pasando de array a objeto. El molde que decodifica un array (`TestApprovals_ListIsBounded`) se actualiza **en el mismo commit** y **su mutación probatoria se re-ejecuta** (FR-API-1, 5, 6, 12). |
| **D** | Error tipado en `ExecuteApprovedAction` | **SÍ**: un efecto irreversible que ocurrió y se reporta como fallo es la peor mentira que puede decir esta pantalla. El desenlace desconocido es un estado propio, con su nombre y su texto — dice que el efecto **pudo ocurrir** y que **el registro no se cerró** (FR-UI-18, FR-API-10). |
| **E** | La cota de parámetros | **64 KiB**, el número del árbol (`maxApprovalParamsBytes`, `internal/action/sqlite/approvals.go`). |

**Regla permanente que sale de esto (director, 2026-09-08):** cuando se pida
confirmar un número, se trae **el del código**, no el de la conversación.

### F · Los cambios sobre el rojo — LAS VEINTICINCO AUTORIZADAS EN BLOQUE (director, 2026-09-09)

La adjudicación C autorizó cuatro sin el dato completo; contados contra el
fichero RED eran **ocho** cuando los autorizó; tras la trigésima quinta son
**veinticinco**, y el director las autorizó **todas en bloque** el 2026-09-09,
con esta razón suya: «*las filas 9 a 25 salen de treinta y cinco pasadas que
verificaron el árbol; negarlas dejaría el rojo desalineado con lo que hay que
construir*». Condición vigente, la de siempre: **cada molde tocado se
re-ejecuta con su mutación**, y el canto declara las veinticinco con su porqué
en una tabla. Las filas 19 a 21 no son nuevas: son cambios que otras FR ya exigían y
que esta tabla no listaba, así que el nod fila-por-fila no los cubría. El
director autorizó los ocho: «autoricé cuatro sin
el dato; el dato manda». Con ellos, los **cuatro paquetes** que el tren toca —
`internal/controlapi`, `internal/app` (el centinela de la ejecución) e
`internal/action/sqlite` (los centinelas tipados, la desambiguación del claim
y la puerta de lectura que trae operación y parámetros juntos, la que
FR-UI-62 necesita) y `cmd/korvun-desktop` (la vista, su icono y el arnés) — quedan
autorizados con la condición de siempre: **cada molde tocado se re-ejecuta con
su mutación**.

| # | Cambio | Origen |
|---|---|---|
| 1 | la firma de `Approve` | FR-API-5 |
| 2 | `digest` a la fila | FR-API-1 |
| 3 | la lista pasa de array a objeto (`TestApprovals_ListIsBounded` decodifica un array) | FR-API-6 |
| 4 | la costura `ListPending` transporta el `gate` | FR-API-6 |
| 5 | el 401 gana aserción de CUERPO (hoy solo se mira el código) | FR-API-11 |
| 6 | `TestApprovals_RejectCommentIsBounded` deja de aceptar `413` **o** `400` | FR-API-13 |
| 7 | la tabla de desenlaces con nombre pasa de **seis a once** filas (`invalidated`, `evidence_corrupt`, `params_digest_mismatch`, `params_not_canonical`, `brain_gone`; `empty` **no** entra: es un valor de `parameters_state` en un 200) | FR-API-7, 15, 19, 20 |
| 8 | `LawVersion` sale del DTO, y `TestApprovals_DetailCarriesTheDigest` lo asigna hoy (`LawVersion: "v3"`): deja de compilar si no se toca | FR-API-2 |
| 9 | el detalle gana el campo `brain_gone` en el DTO del 200 (`status` **no**: FR-API-18 lo retiró) | FR-API-20 |
| 10 | el cuerpo de error gana `current_law_digest` | FR-API-17 |
| 11 | `TestApprovals_DisabledIsNotAnEmptyList` fija el **409 en la ruta de la lista** (hoy acepta cualquier no-200; el nombre y el texto sí los fija `TestApprovals_NamedOutcomes`) | FR-TEST-3 |
| 12 | `TestApprovals_ListIsBounded` pasa a `len(rows) == ApprovalsPageLimit`: hoy solo comprueba `>` y **pasaría con cero filas** | FR-TEST-3 |
| 13 | la tabla de desenlaces gana además `not_started_params_held`, `not_started_params_gone`, `params_unaccounted`, `already_closed`, `params_belt_failed`, `unknown_outcome` y `close_failed` (con las filas 5 y 7, de seis a **diecinueve** filas: las seis del rojo, `forbidden`, y las **doce** que añaden las curas; de ellas **diez** son desenlaces de la ejecución) | FR-API-10 |
| 14 | `taken_by_another` se renombra a **`params_unaccounted`**: el nombre viejo afirmaba un actor que ninguna de sus tres ramas demuestra | FR-API-25, ronda 25 |
| 15 | nace **`already_closed`** para los DOS cortes previos al claim (`approval.Status != APPROVED` y `rec.State != StateApproved`), con un literal que imprime SOLO lo que su corte leyó y no nombra actor | FR-UI-18, rondas 25-26 |
| 16 | `ClaimApprovalParams` **y `ApprovalParams`** distinguen la **fila ausente** de la columna vacía en vez de devolver el mismo `ErrNotFound` para las dos | FR-API-21, rondas 25-26 |
| 17 | `ClaimApprovalParams` deja de descartar el error de `RowsAffected()` (`n, _ :=`) y lo propaga | FR-API-21, ronda 25 |
| 18 | nace en el almacén la puerta de lectura que **une `approvals` y `actions`** y devuelve centinelas tipados para las cuatro ramas de la escalera; sin ella el discriminador no tiene la terna de la operación ni puede separar fila ausente de columna vacía | FR-API-25, ronda 26 |
| 19 | la puerta de LISTA que trae la previa **sin cinturón** (FR-API-1 la declara «no existe hoy y la añade este tren» y apunta aquí) | FR-API-1 |
| 20 | la puerta del DETALLE en **una** transacción, con estado, terna, params y cinturones (FR-API-22 apunta aquí) | FR-API-22 |
| 21 | centinelas tipados en el almacén **con granularidad POR CINTURÓN** —`ErrApprovalInvalidated`, `ErrApprovalEvidenceCorrupt`, `ErrApprovalNotFound`— y el colapso de `sql.ErrNoRows` con el error de driver separado en los **TRES** sitios: las dos consultas de `verifyApprovalStory` (`actions` y `action_decisions`) y el `SELECT canonical_preview` de `GetApproval`. Sin esa granularidad el eje 1 de FR-API-25 no existe: los rehúses salen por un solo `return` | FR-API-15, FR-API-25 |
| 22 | nacen `not_decided` y `params_unreadable`; el literal de `already_closed` imprime solo lo que su corte leyó y dice el `recovery_marker` cuando lo hay | FR-API-25, ronda 27 |
| 23 | nace `decided_evidence_corrupt` con literal propio, y FR-API-18 gana su rama por defecto (`status` fuera del juego ⇒ `already_decided`) | FR-API-25, FR-API-18, ronda 30 |
| 24 | `ValidateApprovalBinding` rehusando **desde el endpoint** se mapea a `decided_evidence_corrupt`: ahí la ley no puede haberse movido, así que un mismatch solo lo produce el pin ALMACENADO mutado | FR-API-25, ronda 33 |
| 25 | los identificadores de recibo que P4 y P5 imprimen viajan en el DTO: ni `DecideApprovalUnderLaw` ni `FinishWithResult` los devuelven, así que el endpoint los re-lee (`decision_receipt_id` de `scanApproval`, y el terminal de la acción) | P4, P5, ronda 35 |

#### F-bis · Los moldes YA EXISTENTES que el tren toca, y su re-mutación

La condición del director es «cada molde TOCADO se re-ejecuta con su
mutación». Estos son los del fichero RED que las veinticinco filas tocan, con
la fila que los toca y lo que su mutación tiene que enrojecer después del
cambio. Ninguno se edita sin volver a correr su mutación y capturar el rojo.

| Molde existente | Filas que lo tocan | Su mutación, después del cambio |
|---|---|---|
| `TestApprovals_DetailCarriesTheDigest` | 2, 8, 9 | quitar `digest` de la fila, o devolver `LawVersion` retirado |
| `TestApprovals_ListIsBounded` | 3, 12 | devolver un array en vez del objeto, o servir `ApprovalsPageLimit ± 1` |
| `TestApprovals_EveryRouteRequiresBearer` | 5 | devolver 401 con cuerpo vacío |
| `TestApprovals_RejectCommentIsBounded` | 6 | devolver 400 donde el contrato exige 413 |
| `TestApprovals_NamedOutcomes` | 7, 13, 14, 15, 22, 23, 24 | emitir un nombre fuera de `ApprovalOutcomeNames`, y emitir el nombre viejo de cada renombre |
| `TestApprovals_DisabledIsNotAnEmptyList` | 11 | contestar 200 vacío, o el 409 en una ruta que no es la de la lista |
| `TestApprovals_ApproveCarriesTheDigestTheOperatorSaw` | 1 | aceptar un `approve` sin digest, o con uno rancio |

Los moldes que el tren NO toca —`ApproveWithoutADigestIsRefused`,
`RejectCarriesTheComment`— se re-ejecutan igualmente en el gate, pero no
reclaman mutación nueva: nada de su rama vigilada cambia.

### G · La regla de parada del papel (director, 2026-09-09)

Las pasadas adversariales sobre este documento **siguen mientras encuentren P1
de GARANTÍA o de COMPORTAMIENTO** — algo que el árbol hace y el papel niega, o
al revés. En cuanto una pasada devuelva solo letreros, redacción, aritmética o
citas, **se para**, eso se ficha, y el tren baja al rojo. No se cuentan
pasadas: se cuenta la CLASE del hallazgo. La aplica quien conduce el tren, sin
consultar. **Y está subordinada a la ley UX:** parar las pasadas del papel no
abre el RED por sí solo — queda la pasada manual del director sobre la v36 de
la maqueta, y, por `UX-TEMPLATE.md`, bloquea la apertura hasta
que el director las despache. La regla cierra el bucle adversarial; el sí del
director sigue siendo la puerta.

### H · El corte — NO se parte (director, 2026-09-09)

Recomendé siete veces cerrar el papel de la pantalla y mudar la taxonomía del
endpoint y del camino CLI a su propio tren. **El director decide que NO**, y
la razón es suya y es de ingeniería, no de prisa:

> El veto está levantado sobre todo, la frontera del §11-bis existe pero
> partir ahora crea dos trenes que se pisan en los mismos tres paquetes, y el
> criterio por llamante ya está escrito. Un solo tren.

Queda constancia de las dos cosas: de mi recomendación, que era real y estaba
argumentada con la curva, y de por qué no se sigue. §11-bis y FR-UI-71 **no se
retiran**: siguen siendo la frontera y el criterio, y valen para el tren
siguiente si alguna vez hace falta partir. Lo que cambia es que hoy no se
parte.

### I · Los tres fichajes del §15 — aceptados, con condición (director, 2026-09-09)

Aceptados **como fichaje**, con la condición de siempre: **ninguno deja viva
una frase falsa PUBLICADA por este papel**. Verificado uno a uno antes de
cerrar:

| Fichaje | ¿Deja viva una frase falsa nuestra? | Evidencia |
|---|---|---|
| §15-bis (tres godocs de `store.go`) | **No.** La spec dejó de apoyarse en ellos en la ronda 28: lee el PREDICADO, no el godoc, y lo dice con esas palabras | eje 1 de FR-API-25 y la fila del `recovery_marker` en P4 |
| §15-ter (borrar `action_decisions` mata el arranque) | **No.** Es comportamiento querido y pinado; la spec lo usa para NO apoyarse en un oráculo que no ocurre, y AS-118 ataca la previa en su lugar | AS-118 y §15-ter |
| §15-quater (tres `RowsAffected()` con el error descartado) | **No.** No se les promete cura ni molde, y la razón está escrita: la clase solo es forzable con driver envuelto | §15-quater |

**Los godocs anchos de `store.go` NO se curan aquí** — este tren no tiene ese
fichero autorizado. Se curan en el tren que lo abra, con esta fecha objetivo:

> **Objetivo: el primer tren posterior a v0.15.0 que abra código en
> `internal/action/sqlite/store.go`, y como tope duro la release v0.16.0.**
> Si v0.16.0 se etiquetara sin haberlos curado, es deuda de tono declarada y
> el canto de esa release la nombra.

Son tres, y los tres en `internal/action/sqlite/store.go`:

1. `recoveryMarkerOutcomeUnknown` — «the crash hit AFTER the params claim»,
   más ancho que su predicado, que solo dice «params ausentes»;
2. `RecoverPreviousLife` — «an APPROVED action whose params were CLAIMED was
   mid-execution», la misma anchura;
3. `Record.RecoveryMarker` — «"" normally and "crash_recovered"», más
   ESTRECHO: omite `outcome_unknown`, que es el valor del que depende toda esa
   rama.

## 18. Criterio de aceptación = la pasada del director

- Sobre las maquetas, antes de RED: ____
- Sobre la build empaquetada (1100×760), antes del tag: ____
