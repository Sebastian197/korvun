# v0.16.2 · «¿Qué pasa hoy?» — plan de fallos antes del primer rojo

Fecha: 2026-09-23. Rama `v0162-mockups`.
Maqueta aprobada por el director: `design-drafts/2026-09-23-v0162-que-pasa-hoy.html`.
**`design-drafts/` está en `.gitignore`**, así que esa ruta NO es alcanzable desde
este árbol: el fichero vive solo en el checkout principal. Se dice aquí en vez de
dejar que lo descubra quien audite.

**SEGUNDA VERSIÓN, tras un veto con `3 P1, 5 P2, 7 P3`.** Los tres P1 se
verificaron uno a uno contra el árbol y los tres eran ciertos. El veredicto está
en `.claude/adversary/v0162-que-pasa-hoy-pretest-verdict.md`. Lo que sigue no es
el plan anterior con parches: **el diseño cambió de forma**, porque el anterior
peleaba contra la arquitectura en vez de usarla.

**Esta pieza nace de la séptima ley** (CLAUDE.md) y crea la pantalla que esa ley
exige de todos los trenes siguientes.

---

## 0 · Lo que el primer veto cambió, y por qué

| Lo que el plan v1 decía | Lo que el árbol dice | Consecuencia |
|---|---|---|
| «La pantalla consume el veredicto del núcleo, no lo recalcula» | `brainCanPark` **cortocircuita** en la condición 3 y devuelve un `bool`; `ApprovalGate` lleva cuatro números y **ningún motivo**. Dos estados distintos de la maqueta —el rojo sin techo y el ámbar de la sombra— producen el MISMO `BrainsCanPark = 0` | El núcleo tiene que **publicar el motivo**. Sin eso, o la pantalla recalcula (prohibido) o no puede pintar la maqueta aprobada |
| «acto → escritura → recarga → vuelta atrás» | El supervisor persiste **SOLO tras un `Start` confirmado** (ADR-0027 §c). En un cutover fallido **nunca** escribe el fichero | El plan **invertía** la invariante e **inventaba** `ErrRollbackFailed`, que hoy no puede ocurrir. Se retira entera |
| «si la recarga falla → vuelta atrás» | `POST /api/config` devuelve **`202 Accepted` + handle**. No hay retorno síncrono de fallo | La pantalla **sondea el handle**. Nace un estado de UI que la maqueta no tenía: *aplicando…* |
| «A1 es el molde de integración, sin mutación propia» | La doctrina: «sin mutación roja capturada, el test no existe», y «ninguna instrucción futura puede suspenderla» | A1 **deja de listarse como molde**: es una PASADA MANUAL, con su etiqueta. Su hermano sintético sí es molde y sí tiene mutación |

---

## 1 · Las garantías, literales, y su cable

| # | Garantía | Cable HOY |
|---|---|---|
| G1 | El motivo que la pantalla pinta lo **emite el núcleo**, en el mismo sitio que decide si un cerebro puede aparcar. La pantalla no reimplementa las cinco condiciones | `brainCanPark` (`internal/app/approvals_adapter.go`) — que hoy devuelve `bool` y **hay que ampliar** |
| G2 | Cuando algo bloquea, la pantalla nombra **cuál** condición falla, y si fallan varias las nombra **todas**, no la primera | no existe: el cable corta en la 3 |
| G3a | Cada acción escribe **por la misma superficie de mutación** que el builder, con la misma validación y el mismo guardián de autobloqueo, y **nunca** llega al supervisor un cambio que no las pase | `internal/controlapi/mutation.go`: `cfg.Validate()` y `wouldSelfLock` |
| G3b | **Toda** mutación del perfil por la Control API —los cuatro botones **y** `POST /api/config` del builder— queda como **acto del operador en el libro, con recibo sellado**. Si el acto no se sella, el cambio **no se intenta**; y el acto dice **lo que se intentó y el resultado**, nunca lo que se deseaba | `RecordAttemptAuthenticated` → `apply` → `Finish`, el mismo patrón de tres pasos que `recordAuthorityAct` usa en la CLI. La Control API recibe el recorder que hoy tiene el adaptador de aprobaciones |
| G4 | **El fichero del perfil nunca queda por delante del proceso.** Si el cutover falla, el disco sigue con la config que arranca | **YA GARANTIZADO** por `reasonReload` (`internal/supervisor/supervisor.go`): «persist is NEVER called on a failed cutover». G4 se **hereda**, no se aporta |
| G5 | «Levantar la sombra» exige confirmación antes de POSTear nada | no existe |
| G6 | Con el núcleo sin responder, la pantalla **no afirma** veredicto — y distingue los CUATRO estados que el store ya distingue | `CoreState = 'running' \| 'stopped' \| 'unreachable' \| 'unknown'` (`status/store.ts`) |
| G7 | El indicador dice la hora de la última respuesta y cuánto tardó, y al perder la respuesta **conserva** esa hora | `lastOkAt` **nunca se borra** en el store; es `HealthzBadge` el que deja de enseñarlo. El cronómetro no existe |

### Lo que estas garantías NO dicen

- **G1 no promete que el veredicto sea correcto**: promete que la pantalla y el
  núcleo dicen lo mismo. Si el núcleo se equivoca, la pantalla se equivoca igual.
- **G2 no promete una condición**: promete **todas** las que fallen. El plan v1
  decía «LA condición», y con dos herramientas que fallan condiciones distintas
  eso no es función de nada.
- **G4 no es mérito de esta pieza.** Es del supervisor, y el canto lo dirá así.
- **La frase del veredicto es POR INSTALACIÓN y el cable es POR HERRAMIENTA Y POR
  CANAL.** El godoc del cable lo declara: «judged PER CHANNEL … no
  channel-independent boolean exists». La pantalla dice «lo más permisivo que
  puede pasarle a una acción irreversible aquí», y esa es la frase, no «lo que
  pasa siempre».
- **G7 corrige una frase falsa del plan v1**, que decía que el estado de fallo
  «tira la hora». No la tira el store: la tira el rótulo. La ficha del HANDOFF ya
  lo tenía bien medido y el plan la degradó al copiarla.
- **G3b NO promete atomicidad entre una transacción SQLite y un cutover de
  proceso**, porque eso no existe. Un `Start` que se confirma en otro objeto de
  app no participa de la transacción del libro y no hay dos fases que lo aten.
  Lo que G3b promete es exactamente lo que el director escribió: **el acto dice
  lo que se INTENTÓ y el RESULTADO**. En concreto, y con este orden:
  1. el acto se sella ANTES de pedir el cutover — si el sellado falla, el
     supervisor no se llama nunca, y eso sí es una garantía por imposibilidad
     («sin acto no hay cambio»);
  2. el cutover corre;
  3. el acto se CIERRA con el desenlace real, `succeeded` o `failed`.

  Entre 2 y 3 hay una ventana: el proceso puede morir con el acto abierto. Un
  acto abierto es honesto —dice «se intentó», no «se logró»— y es lo que un
  operador lee después de un corte. Lo que NO puede ocurrir, y tiene molde, es un
  cambio aplicado sin acto.
- **La primera lectura de G3 lo declaró irrealizable y lo redujo; el director lo
  revocó el 2026-09-24 y la pieza entra en este mismo tren.** Lo que aquella
  lectura tenía de cierto, y sigue en pie, es que `POST /api/config` no
  registraba acto desde la Etapa 14: por eso G3b cubre también esa puerta y no
  solo los cuatro botones nuevos.

---

## 2 · Vecinos y hermanos de clase

| Vecino | Qué comparte | Qué no romper |
|---|---|---|
| `brainCanPark` / `brainsThatCanPark` (`internal/app/approvals_adapter.go`) | las cinco condiciones | Se AMPLÍA con un motivo por condición, **sin cambiar el valor de `BrainsCanPark`**, o la bandeja cambia de semántica |
| `Approvals.tsx`, rama `gate.brains_can_park === 0` | hoy pinta «APROBACIONES ENCENDIDAS · NINGÚN CEREBRO PUEDE APARCAR» | Ese literal está fijado por AS-19 en `Approvals.test.tsx`. Cualquier cambio de semántica lo enrojece |
| `internal/supervisor` | **el dueño real del ciclo recarga/persistencia** — ausente del plan v1 | Su invariante es G4. Escribir el fichero por fuera la rompe |
| `POST /api/config` (`internal/controlapi/mutation.go`) | `202` + handle, `DisallowUnknownFields`, límite de 1 MiB, `wouldSelfLock` | La pantalla sondea; no supone retorno síncrono |
| `writeRawConfigAtomic` (`internal/shell/upgrade.go`) | el OTRO camino de escritura, que preserva claves crudas | Un perfil migrado pasa por ahí. `DisallowUnknownFields` en la puerta de la pantalla puede rechazar lo que aquel aceptó |
| `status/store.ts` + `HealthzBadge.tsx` | sondeo de 2 s, `lastOkAt`, y el guardián `if (polling) return` | `lastOkAt` no se borra; el rediseño lo EXPONE |
| `WriteConfigAtomic` (`internal/supervisor`) | la escritura atómica | **Destruye un perfil que sea enlace simbólico** — capturado. Deuda ANTERIOR a esta pieza, y **tren mínimo propio inmediatamente después de esta release** por decisión del director del 2026-09-24: es pérdida de datos. Esta pieza no la hereda porque ya no escribe el fichero |

---

## 3 · Fronteras transaccionales y de persistencia

**El orden se INVIERTE respecto al plan v1, y ahora es el del árbol.**

| Paso | Dónde | Qué error propio nace ahí |
|---|---|---|
| 1 · la pantalla pregunta | `GET /api/whats-happening` | lectura |
| 2 · el operador pulsa (y confirma, si es la sombra) | — | — |
| 3 · el acto al libro | UNA transacción, patrón `recordAuthorityAct` | `ErrOperatorActWriteFailed` |
| 4 · `POST /api/config` | bearer; el supervisor **construye, arranca y SOLO ENTONCES persiste** | `409 reload_in_progress`, `409 config_would_self_lock`, `400` de validación, `413`, `500` |
| 5 · la pantalla **sondea** `GET /api/reload/{handle}` | estado de UI *aplicando…* | — |
| 6 · el handle resuelve | `StateSucceeded`, `StateRolledBack`, `StateFailed`, `StatePersistFailed` | cada uno con su frase |

**No hay paso de vuelta atrás, y eso es una mejora, no un recorte**: el supervisor
nunca escribe el fichero en un cutover fallido, así que no hay nada que revertir.
`ErrRollbackFailed` desaparece del plan.

**El acto va antes del POST** y eso se mantiene: un intento que falla es un hecho
que el libro conserva. Un libro que solo guarda lo que salió bien no sirve para
auditar a nadie.

**`StatePersistFailed` es la clase grave que el plan v1 no tenía**: la app nueva
SÍ está sirviendo y el disco no pudo actualizarse. El proceso va por delante del
fichero — la inversa exacta de lo que v1 temía, y la única que el árbol admite.

---

## 4 · Taxonomía de fallos — completa contra el camino real

| Clase | Sitio | Qué dice la pantalla |
|---|---|---|
| `ErrUnauthorized` | `bearerAuth` | no se pudo autorizar |
| `409 reload_in_progress` | `supervisor.ErrReloadInProgress` | **«ya se está aplicando otro cambio»** — ver A15, dos ventanas |
| `409 config_would_self_lock` | `wouldSelfLock` | **«este cambio te dejaría sin puerta»** — ver A16 |
| `400` validación / campo desconocido / datos sobrantes | `cfg.Validate`, `DisallowUnknownFields` | el perfil no se pudo guardar, con su razón |
| `413` cuerpo grande | `mutation.go` | ídem |
| `500 reload could not be started` | `mutation.go` | ídem |
| `StateRolledBack` / `StateFailed` | `supervisor` | **«no se aplicó; tu perfil sigue como estaba»** |
| `StatePersistFailed` | `supervisor` | **«se aplicó pero no se pudo guardar: al reiniciar volverá atrás»** |
| `ErrOperatorActWriteFailed` | el libro | el acto no se pudo registrar; no se intenta el cambio |
| `503 no current config` | `mutation.go` | no hay config viva que modificar |
| handle descartado en apagado | `supervisor` | el sondeo termina sin desenlace; se dice así |

**Los cuatro estados del núcleo, que NO se colapsan en uno**: `running`,
`stopped`, `unreachable`, `unknown` — y `unknown` incluye **el primer render**,
donde `lastOkAt === null`. Como esta pantalla es ahora **Inicio**, el primer
render es el caso más frecuente de todos, y «nunca respondió» no puede pintarse
igual que «respondió y se perdió».

**Lo que NO tiene clase, declarado**: un `korvun.json` editado a mano entre la
lectura y el POST. La pantalla decide sobre lo que leyó; el supervisor escribe lo
que el POST trae. Ver A8, que cambia de forma respecto a v1.

---

## 5 · Matriz de ataque

| # | Ataque | Desenlace EXACTO exigido | Nivel de evidencia |
|---|---|---|---|
| A2 | Sin aprobaciones | «Se ejecuta al instante», rojo, acción **Encender aprobaciones**, y ninguna otra condición marcada | In-process |
| A3 | Sin techo | ídem, y la condición marcada es **exactamente** la del techo | In-process |
| A4 | Herramienta en sombra | «Se observa sin ejecutar», ámbar, y el botón **pide confirmación antes de POSTear** | In-process |
| A5 | **La confirmación se rechaza** | **Cero escrituras, por IMPOSIBILIDAD**: el directorio del perfil en `0o500`, de modo que cualquier intento falle ruidosamente — no un sha256 comparado a posteriori. Más cero actos nuevos | In-process con imposibilidad |
| A6 | **El cutover falla** | `StateRolledBack`, el fichero **intacto por el supervisor** —no por nosotros—, y la pantalla dice que no se aplicó. Jamás un verde | Fichero real + sha256 |
| A7 | **`StatePersistFailed`** | La app sirve con lo nuevo y el disco tiene lo viejo. La pantalla lo dice **con esas dos mitades**, no como éxito ni como fallo | In-process con seam |
| A8 | **Campo desconocido en un perfil migrado** | Un perfil que `writeRawConfigAtomic` aceptó puede llevar claves que `DisallowUnknownFields` rechaza. La pantalla **no pierde la clave**: rehúsa con su razón | SQLite/fichero real |
| A9 | Sin bearer | `ErrUnauthorized`, cero escrituras | In-process |
| A11 | Núcleo sin responder | La pantalla **no afirma** veredicto, y distingue `stopped` de `unreachable` de `unknown` | In-process |
| A12 | El indicador pierde la respuesta | El rótulo conserva **la hora**. Mutación en el RÓTULO, no en el store | In-process, reloj forzado |
| A13 | El cronómetro | El ms corresponde al retardo forzado | In-process, reloj forzado |
| A13-bis | **La petición que nunca resuelve** | `if (polling) return` congela los ticks. El rótulo **no puede seguir diciendo «en vivo»** con un ms rancio: pasa a un estado propio | In-process, fetch que no resuelve |
| A14 | **La pantalla sigue al núcleo** | Mutar el motivo en `brainCanPark` cambia la pantalla. Si no cambia, la pantalla lo reimplementó | In-process |
| A15 | **Dos ventanas** | La segunda recibe `409 reload_in_progress` y dice **«ya se está aplicando otro cambio»** — jamás «no se aplicó», que sería falso | Dos clientes reales |
| A16 | **Perfil que se auto-bloquea** | `admin.token_env` sin resolver hace que NINGÚN botón pueda funcionar. La pantalla lo dice antes de ofrecer el botón, no después de fallar | In-process |
| A17 | **Dos condiciones fallan a la vez** | Dos herramientas, una que falla la 4 y otra la 5: la pantalla marca **las dos**, no la primera | In-process |
| A18 | **Primer render** | `lastOkAt === null` y `core === 'unknown'`: ni veredicto ni hora inventada | In-process |

### Retirada de la matriz, y por qué

- **A1** (perfil migrado real) deja de ser una fila de moldes: es una **PASADA
  MANUAL** con su etiqueta, en §7. Un molde sin mutación no es un molde.
- **A10** se funde en A9.

---

## 6 · Moldes y sus mutaciones

**TRES por cura**, como el director pidió, y esta vez contado: cada ataque de §5
tiene su molde, y las tres curas que el tren entrega —el motivo publicado, la
acción por la Control API, el indicador— tienen tres moldes cada una.

| Cura | Molde | Mutación probatoria | Rojo que debe salir |
|---|---|---|---|
| **1 · el motivo lo publica el núcleo** | `TestGate_reportsEveryFailedCondition` (A2, A3) | cortocircuitar en la primera condición fallida | una condición fallida desaparece del motivo |
| | `TestGate_reportsBothWhenTwoFail` (A17) | ídem | la segunda no se nombra |
| | `TestScreen_followsTheCoreNotItsOwnCopy` (A14) | reimplementar las condiciones en la pantalla | la pantalla no se mueve con el núcleo |
| **2 · la acción, por la Control API** | `TestAction_declinedConfirmationWritesNothing` (A5) | POSTear antes de confirmar | el directorio en `0o500` hace fallar el intento **ruidosamente** |
| | `TestAction_rolledBackSaysItDidNotApply` (A6) | pintar verde sobre `StateRolledBack` | la pantalla afirma un cambio que no ocurrió |
| | `TestAction_persistFailedSaysBothHalves` (A7) | tratar `StatePersistFailed` como éxito | se oculta que el disco quedó atrás |
| **3 · el indicador** | `TestHealthz_badgeKeepsTheLastHour` (A12) | devolver el rótulo a `sin respuesta` **sin** la hora | la hora desaparece cuando más hace falta |
| | `TestHealthz_showsTheMeasuredRoundTrip` (A13) | mostrar una constante | el ms no sigue al retardo |
| | `TestHealthz_frozenPollIsNotLive` (A13-bis) | dejar «en vivo» con el sondeo congelado | el rótulo miente sobre estar vivo |

Y los moldes de las clases que no son cura sino defensa: `TestAction_refusesWithoutBearer`
(A9), `TestAction_secondWindowSaysSomeoneElseIsApplying` (A15),
`TestScreen_selfLockingProfileSaysSoFirst` (A16),
`TestScreen_unknownCoreAffirmsNothing` (A11, A18),
`TestAction_unknownFieldIsNotLost` (A8) — cada uno con su mutación, en la tabla
extendida del canto.

---

## 7 · La pasada MANUAL, que no es un molde

La séptima ley exige probar sobre un perfil REAL migrado de dos versiones atrás.
Eso **no es un test**: es una pasada manual sobre la app empaquetada, del mismo
género que la sexta ley, y se etiqueta así en vez de colarse en §6.

**Qué es**: copia real del perfil del director (schema 12 → 15), recorriendo los
tres estados **con los botones**, sin editar ficheros.
**Qué NO es**: evidencia en proceso OS aparte de una garantía. Es binario +
humano, que es otra cosa y vale por otras razones.
**Su hermano sintético** —perfil fabricado en schema 12 y migrado en un test— sí
es molde, sí corre en CI y sí tiene mutación: saltarse la migración y comprobar
que la pantalla entonces no puede pintar los tres estados.

---

## 8 · Leído como lo leería el adversario: qué me tumbaría AHORA

**1 · «Ampliar `brainCanPark` cambia la bandeja.»** Es el riesgo real. El motivo
se añade **junto a** `BrainsCanPark`, sin tocar su valor, precisamente para que
AS-19 y su literal sigan verdes. Si al implementarlo resultara que no se puede
sin moverlo, el tren PARA y se replantea, en vez de mover un test aprobado.

**2 · «El símlink sigue roto.»** Sí, y no lo hereda esta pieza porque ya no
escribe el fichero. **Y no se queda fichado**: el director lo sacó de la cola el
2026-09-24 y le dio **tren mínimo propio, inmediatamente después de esta
release**, por ser pérdida de datos. Escribirlo aquí como si esta pieza lo curara
sería mentir; dejarlo en una ficha a largo plazo, también.

**3 · «El acto antes del POST registra intentos que no cambian nada.»**
Deliberado, dicho, y no se toca.

**4 · «A5 con el directorio en 0o500 prueba que NO SE PUEDE escribir, no que no
se INTENTÓ.»** Cierto, y por eso lleva las dos mitades: la imposibilidad y el
conteo de actos. Juntas cubren al escritor que intenta y al que no.

**5 · Lo que sigo sin poder probar**: que la recarga en caliente sea equivalente
a un reinicio. Se prueba que la config montada es la del POST; que ningún
componente quede con la vieja es más ancho y no se afirma.

---

## 9 · Estado

`PLANEADO`, segunda versión, tras un veto con `3 P1, 5 P2, 7 P3`. Ninguna línea
de código hasta que una lectura adversaria levante el veto.

**Y la maqueta necesita tres estados que no tiene**, consecuencia directa de este
rediseño: *aplicando…* mientras el handle se sondea, *ya se está aplicando otro
cambio* para la segunda ventana, y *este perfil te dejaría sin puerta* para el
auto-bloqueo. Van al director antes del rojo, por la sexta ley.
