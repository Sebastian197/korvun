# v0.16.2 · el acto del operador por la Control API — plan de fallos

**Encargo del director, 2026-09-24, revocando la reducción de G3:**

> G3 NO SE RETIRA NI SE FICHA. Antes del tag v0.16.2, en este mismo tren: toda
> mutación del perfil por la Control API —los cuatro botones nuevos Y
> `POST /api/config` del builder— registra un acto del operador en el libro, con
> recibo sellado, atómico con el cambio (si el acto no se sella, el cambio no se
> aplica; y viceversa). La Control API recibe el recorder/store que hoy tiene el
> adaptador de aprobaciones. Tres moldes por puerta (acto registrado; sin acto no
> hay cambio; recibo verificable con `receipt verify`), mutaciones rojas, plan de
> fallos con la fila «reload falla después de sellar el acto» (el acto dice lo que
> se intentó y el resultado, no lo que se deseaba).

---

## 0 · Lo que «atómico» puede significar aquí, dicho antes de prometerlo

La palabra del mandato es «atómico con el cambio». Una transacción SQLite y un
cutover de proceso **no pueden** ser atómicos entre sí: el `Start` que confirma
el cambio corre sobre otro objeto de app, no participa de la transacción del
libro, y no hay segunda fase que los ate. Prometer lo contrario sería una frase
más fuerte que el código.

La propia orden del director resuelve la tensión, y es la lectura que se
implementa: **«el acto dice lo que se intentó y el resultado, no lo que se
deseaba»**. En concreto:

| Momento | Qué pasa | Qué garantiza |
|---|---|---|
| 1 | El acto se **sella** con lo que se va a intentar | Si falla, se devuelve error y **el supervisor no se llama jamás** |
| 2 | El cutover corre | — |
| 3 | El acto se **cierra** con el desenlace real (`succeeded` / `failed`) por el PROCESO en cuanto el supervisor lo sabe — no por el sondeo de la pantalla (CORREGIDO 2026-09-24 tras la captura `evidence/v0.16.2/probe-real-cutover.txt`: el app que sella el acto muere antes del estado terminal) | El libro dice el resultado, no el deseo |

La mitad «sin acto no hay cambio» es una garantía **por imposibilidad**: un
supervisor que no se llamó no pudo cambiar nada por ningún camino, y el molde la
observa con un contador sobre la costura, no comparando un fichero después.

La mitad inversa —«si el cambio no se aplica, no hay acto»— es **falsa por
construcción y no se promete**: el acto de un cambio que se intentó y falló DEBE
existir, porque un intento que no deja rastro es exactamente lo que el libro
existe para impedir. Lo que el acto dice en ese caso es `failed`.

**La ventana declarada:** entre 2 y 3 el PROCESO puede morir con el acto abierto
(`authorized`). El arranque siguiente lo recupera como `OUTCOME_UNKNOWN` con
recibo (CORREGIDO 2026-09-24: la redacción anterior decía «abierto para
siempre»; la recuperación de arranque cierra todo `AUTHORIZED` huérfano, y solo
respeta los actos que el proceso VIVO todavía gobierna).

---

## 1 · Las garantías, literales

| # | Garantía |
|---|---|
| A1 | **Las cinco puertas** registran acto: `enable-approvals`, `set-ceiling`, `lift-shadow`, `allow-host` y `POST /api/config` |
| A2 | Si el sellado del acto **falla**, el supervisor **no se llama** y la puerta responde un error nombrado |
| A3 | El acto cierra en `succeeded` cuando el cutover se aplicó y en `failed` cuando no |
| A4 | El recibo del acto **verifica** con `korvun receipt verify` |
| A5 | El acto nombra **qué** se intentó: verbo por puerta, y los parámetros del acto sellan el cambio pedido |
| A6 | Sin almacén de acciones (`storage` ausente), las puertas de mutación **se montan y rehúsan por nombre** (`no_ledger`): no hay cambio sin libro donde apuntarlo. CORREGIDO 2026-09-24: la primera redacción decía «no se montan», y el código las monta —para que la pantalla explique el estado— y las rehúsa. Y desde el cierre del §0 hay UNA excepción, `enable-storage`, que funda el libro y sella en él su propio acto antes de pedir el cambio (papel `2026-09-24-v0162-el-almacen-y-el-cierre-del-acto-pretest.md`) |

### Lo que estas garantías NO dicen

- **A4 no promete que el recibo esté FIRMADO.** El sellador se instala con
  `SetReceiptSealer`; si el perfil no tiene clave de sellado, el recibo existe y
  verifica como no sellado. Lo que se promete es que `receipt verify` lo acepta,
  no que lleve firma.
- **A5 no promete que los parámetros sean legibles por un humano.** Son el
  digest canónico del cambio pedido, que es lo que el libro sella.
- **Ninguna garantía cubre un cambio del perfil hecho a mano en el fichero.** El
  libro registra lo que pasa por la Control API. Un editor de texto no.

---

## 2 · Matriz de ataque

| # | Ataque | Desenlace exigido | Nivel de evidencia |
|---|---|---|---|
| T1 | El store del libro devuelve error al sellar el acto | La puerta responde `act_not_recorded`; **el recargador tiene 0 llamadas** | en proceso, httptest real, store mentiroso |
| T2 | **El reload falla DESPUÉS de sellar el acto** (la fila que pidió el director) | El acto existe y cierra en `failed`; la respuesta dice `not_applied`; el perfil sigue igual | en proceso, store real en disco, recargador que rueda atrás |
| T3 | El reload se aplica | El acto cierra en `succeeded`; hay recibo; `receipt verify` da OK | binario en proceso OS separado para el verify |
| T4 | `Finish` falla después de que el cutover se aplicó | El cambio **se reporta aplicado** (está aplicado) y el fallo de cierre va a un observador, **nunca** al operador como rechazo | en proceso, store con `Finish` mentiroso |
| T5 | Dos puertas a la vez | Cada una tiene su acto, con ids distintos | en proceso, dos peticiones |
| T6 | Perfil **sin `storage`** | Las puertas de mutación están montadas y rehúsan `no_ledger` (503); solo `enable-storage` aplica | app real, servidor admin en loopback (`TestMount_aProfileWithNoStoreRefusesEveryDoorButOne`) |
| T7 | Una puerta nueva añadida sin acto | Un molde estructural enrojece nombrándola | en proceso, caminata sobre el mapa de puertas |
| T8 | El acto de una puerta rechazada (`deny` a `lift-shadow`, techo que bajaría) | **No hay acto**: la puerta rechaza antes de sellar, porque no hubo intento de cambio que registrar | en proceso |

### T4 es la lección de `85013fd`, repetida aquí a propósito

El 2026-09-22 se curó una clase para cuatro escritores: «una escritura durable
nunca reporta el fallo de la cadencia». `recordAuthorityAct` la aprendió en la
CLI —su cierre va a `c.note` y jamás al código de salida— y esta pieza la hereda:
el fallo de `Finish` **no** puede convertirse en el rechazo que el operador lee
sobre un cambio que ya está en marcha.

---

## 3 · Lo no verificable, declarado

- **La ventana entre el cutover y el cierre del acto** no se prueba con un
  crash-restart real en este tren. Se declara, y el estado que deja —acto
  `authorized` que el arranque siguiente cierra como `OUTCOME_UNKNOWN` con
  recibo— es el que un operador leería. Un molde de crash real con sonda del
  punto exacto de interrupción queda **fichado**.
- **El sellado criptográfico del recibo** depende de la clave del perfil, que la
  app instala. Los moldes en proceso corren sobre un store real sin sellador
  instalado por defecto; el `receipt verify` de T3 corre sobre un perfil con la
  ceremonia hecha, que es el único sitio donde la firma existe.
