# v0.16.2 · el almacén que se activa desde la pantalla, y el acto que cierra tras un cutover real — plan de fallos

**Decisión del director (2026-09-24, cierre del §0 del handoff):**

> Un perfil sin almacén NO puede mutarse por la API — se rechaza con nombre
> (`no_ledger`) — SALVO una mutación: activar el almacén (`storage.path`), que es
> la que hace posibles los actos y deja el PRIMER recibo del libro que ella misma
> crea. La pantalla «¿Qué pasa hoy?» lo enseña en su primera fila con el botón
> «Activar almacén». Los tres tests de `internal/shell` se reescriben a esa
> verdad: sin almacén el builder rehúsa con `no_ledger`; activar el almacén es la
> única puerta abierta y deja acto. Tres moldes y mutación roja para el bootstrap.

**Encargo del copiloto:** plan corto, con las filas «perfil sin almacén intenta
cada mutación», «bootstrap del almacén con fallo a mitad» y «bootstrap sobre ruta
no escribible»; lectura adversaria interna de diez minutos; al rojo.

---

## 0 · Un hecho capturado ANTES de diseñar, que cambia el diseño

La pieza del acto (papel `2026-09-24-v0162-el-acto-del-operador-pretest.md`) se
probó contra un recargador FALSO que responde estados terminales en la misma
petición. Contra el supervisor REAL el acto no cierra nunca con su desenlace.
Capturado en `evidence/v0.16.2/probe-real-cutover.txt` (perfil con `storage`,
`shell.Controller` real, `POST /api/config`, sondeo hasta `succeeded`):

```
POST /api/config -> 202 {"action_id":"act_7fb8…","handle":"reload-1","receipt_id":""}
post-cutover poll 0..2: {"state":"succeeded"}          <- sin action_id ni receipt_id
ACTION act_7fb8… state=OUTCOME_UNKNOWN (config act? true)
RECEIPT rcpt_8e97… action=act_7fb8… outcome=OUTCOME_UNKNOWN
```

**Por qué, leído en el fuente:**

1. `Supervisor.Run` apaga el app VIEJO (`s.shutdownApp(app)`) antes de construir
   el nuevo, y en una vuelta atrás reconstruye OTRO app desde la config vieja.
   El `configActRecorder` que ató `handle → acto` vive en el app viejo, así que
   el app que sirve `GET /api/reload/{handle}` cuando el estado es terminal
   nunca es el que selló el acto. Solo un fallo de PREFLIGHT (estado `failed`
   con el app viejo aún sirviendo) llega a cerrar.
2. `Build` del app nuevo abre el mismo fichero y corre
   `actions.RecoverPreviousLife`, que cierra TODA fila `AUTHORIZED` como
   `OUTCOME_UNKNOWN` con marcador de recuperación y su recibo — antes de que el
   supervisor sepa el desenlace. Un cutover no es una vida anterior del
   proceso, pero la recuperación no puede distinguirlo.

**Consecuencia:** en producción (escritorio y `korvun serve`) el libro dice
«desenlace desconocido» de cada cambio de perfil que se aplicó. La frase de las
notas de release «un cutover que rueda atrás cierra su acto como fallido» es
falsa contra el supervisor real. Entra en este cierre porque el bootstrap del
almacén NO puede dejar «el primer recibo del libro» si el acto nunca cierra con
su desenlace: es dependencia, no ampliación de alcance.

---

## 1 · Diseño, en las líneas que caben

| # | Pieza | Qué hace |
|---|---|---|
| D1 | `supervisor.WithStateObserver(func(Handle, State))` | el supervisor avisa de cada transición de estado, desde `setStatus` y `RequestReload`. Costura nueva, sin cambiar la cutover |
| D2 | `app.ConfigActRegistry` (uno por proceso; en el escritorio uno por ciclo de `Start`) | memoria compartida de `handle → (acto, ruta del libro)`, `settled`, `receipts`. Se inyecta a cada `Build` con `app.WithConfigActRegistry`. Sin opción, `Build` crea uno privado (los moldes actuales no cambian) |
| D3 | el recorder por app delega en el registro | `BindReload`, `SettleAct`, `SettleReload` guardan estado en el registro; `Finish` va por el store del app que sirve |
| D4 | `registry.ObserveReload(h, st)` (el observador de D1) | en un estado terminal cierra el acto SIN esperar a que nadie sondee. Elige por VITALIDAD, no por igualdad de ruta: el recorder se ATA al registro en `Build` (store abierto) y se DESATA en `App.Shutdown` antes de cerrar el store, así que en `StateRolledBack` —que el supervisor emite con el app viejo ya apagado y antes de construir el de vuelta— no hay recorder atado y el cierre va por apertura transitoria `actionsqlite.OpenOperator` + `EnsureSigningKey` (la clave ya existe) + sellador, y se cierra la apertura. «Una vez» con ESPERA: el primer cerrador deja un `done` por acto; un segundo cerrador concurrente (el handler que lee un estado ya terminal mientras el observador está a mitad de `Finish`) espera a `done`, acotado por su contexto, y responde el MISMO recibo — nunca un acto sin recibo |
| D5 | `Store.RecoverPreviousLife(ctx, keep ...string)` | `keep` nombra los actos que ESTE proceso todavía gobierna: SELLADOS por `BeginConfigAct` en ese libro y no cerrados — no «atados», porque `BindReload` corre DESPUÉS de que `RequestReload` entregó la petición y la ventana se cerraría solo por el drenaje del admin server. Se saltan, con el motivo en el godoc: un cutover no es una vida anterior. Sin `keep` (arranque de proceso, registro vacío) se recupera todo, como hoy. Alcance honesto: protege los actos de config de ESTE proceso; un acto de autoridad de la CLI en vuelo junto al servidor sigue recuperándose como hoy (preexistente, FICHADO), y un segundo `korvun serve` sobre el mismo fichero con registro vacío se los lleva todos |
| D6 | `ledgerlessRecorder` para el app sin almacén | `BeginConfigAct` devuelve `controlapi.ErrNoLedger` → las puertas responden `no_ledger` (el `rec == nil` de los moldes sigue significando lo mismo). `SettleReload`/`SettleAct` cierran por D4. `CreateLedger` es el bootstrap |
| D7 | `ActRecorder.CreateLedger(ctx, params) (ConfigAct, path, error)` | resuelve la ruta por defecto (`storagePath`, la misma del arranque). **Solo CREA**: si en esa ruta ya hay un fichero, rehúsa por nombre (`ledger_exists`, con la ruta en el detalle) — la ruta por defecto es el libro del ESCRITORIO en esa máquina, y adoptarlo desde un perfil de `serve` sería sellar el acto en el libro de otro perfil. Única excepción: un fichero que ESTE proceso creó en un bootstrap anterior que rodó atrás (el registro lo recuerda), que se readopta para que el botón pueda reintentarse. Luego `OpenOperator` (crea el fichero; NUNCA migra un almacén existente), la misma preparación que el arranque (`ensureRootIntent`, `ensureSigningKey`, `wireIdentitySigners`, `RegisterIdentity`, sellador), sella el acto fundacional `config.enable-storage` con `{"door","path"}`, cierra la apertura y devuelve acto + ruta. Taxonomía de fallo: no se pudo abrir/crear el fichero → `ledger_not_created`; el fichero se creó y un paso de preparación o el sello falló → `act_not_recorded` con el paso nombrado en el detalle (el fichero queda, declarado). En un recorder con libro: rehúsa por nombre |
| D8 | la puerta `POST /api/whats-happening/enable-storage` | sin `confirm` (no abre ejecución real: crea el libro que la registra). Rehúsa `refused` (422) si el perfil ya tiene `storage`; `CreateLedger` → `ledger_exists` (409) / `ledger_not_created` (503) / `act_not_recorded` (503) por nombre; `next.Storage = {Path: <ruta resuelta>}`; `Validate`; `wouldSelfLock`; `RequestReload`; ata; desenlace. `Detail` nombra la ruta. El `message` del `no_ledger` de `POST /api/config` nombra la salida (el botón, o `storage.path` a mano) porque el builder lo pinta tal cual |
| D9 | la fila `store` | `HasButton: true, Door: "enable-storage"`; aritmética de cabecera 16 / **6** / **10** / **5**. Frases que quedan falsas y se corrigen POR FICHERO: `docs/releases/v0.16.2.md` («Cinco llevan botón» → seis; «un cutover que rueda atrás cierra su acto como fallido» — cierto solo desde esta pieza, se reescribe con el hallazgo; «Nada aquí prueba un cutover real del supervisor» — desde esta pieza SÍ, se reescribe); la cabecera de `internal/controlapi/act.go` («otherwise by the status door the screen polls» y «the process can die with the act OPEN» describen el cierre por sondeo y el «abierto» que D4/D5 sustituyen); el papel del acto A6/T6 («no se montan») |
| D10 | la pantalla | `BUTTON_ES['enable-storage'] = 'Activar almacén'`; `OUTCOME_ES['ledger_not_created']`; `press` no manda `brain` cuando la fila no lo tiene |
| D11 | cableado | `shell.Controller.Start` crea el registro del ciclo y lo pasa a `appOptions` y a `supervisor.WithStateObserver`; `cli/serve.go` uno por proceso |

**Lo que el diseño NO promete:** el acto y el cutover siguen sin ser atómicos
entre sí (papel del acto, §0). Lo nuevo es que el proceso VIVO cierra el acto
con el desenlace en cuanto lo sabe, y que si el proceso MUERE en la ventana, el
siguiente arranque lo recupera como `OUTCOME_UNKNOWN` con recibo — un terminal
honesto, mejor que el «abierto para siempre» que el papel anterior declaraba.

---

## 2 · Garantías, literales

| # | Garantía |
|---|---|
| B1 | Un perfil sin almacén rehúsa las CINCO mutaciones existentes con `no_ledger`, con 0 llamadas al recargador y el perfil intacto — también en un app REAL sin almacén (no solo con `rec == nil`) |
| B2 | `enable-storage` es la única puerta abierta sin almacén: CREA el libro (nunca adopta uno ajeno: un fichero ya presente rehúsa `ledger_exists`), sella el acto fundacional ANTES de pedir el cambio, y tras un cutover REAL el acto cierra `SUCCEEDED` con recibo; el perfil en disco gana `storage.path`; a partir de ahí `POST /api/config` responde 202 |
| B3 | Tras un cutover REAL, el acto de cualquier puerta cierra con el desenlace real — `SUCCEEDED` en éxito, `FAILED` en vuelta atrás — aunque nadie sondee la puerta de estado, y la recuperación de arranque de ESTE proceso no se lo lleva |
| B4 | El cierre ocurre UNA vez aunque el observador y el sondeo compitan: cero notas de segundo cierre, y el cerrador que llega segundo ESPERA y responde el mismo recibo — nunca un acto sin recibo |
| B5 | Un bootstrap que no puede crear el fichero responde `ledger_not_created`, 0 recargas, perfil intacto, sin fichero |
| B6 | Un bootstrap cuyo cutover rueda atrás responde `not_applied`, el acto fundacional cierra `FAILED` (por apertura transitoria), el perfil en disco sigue sin `storage`, y `POST /api/config` sigue en `no_ledger` |
| B7 | Un bootstrap sobre un perfil que ya tiene almacén rehúsa `refused` con 0 actos y 0 recargas; y uno cuya ruta por defecto ya tiene un fichero que este proceso no creó rehúsa `ledger_exists` con 0 actos, 0 recargas y el fichero intacto |
| B8 | Un acto `AUTHORIZED` que NADIE gobierna (registro vacío) SÍ se recupera como `OUTCOME_UNKNOWN` — `keep` no abre un agujero en la recuperación. Nivel honesto: «otro proceso murió» se simula in-process con un registro vacío sobre el mismo fichero, no con un proceso OS aparte |

### Lo que estas garantías NO dicen

- B2 no promete que el recibo del bootstrap esté firmado con una clave que ya
  existiera: la clave nace con el libro (`ensureSigningKey`), y el arranque
  siguiente la reutiliza. Firmado con la clave del perfil, sí; con una clave
  «anterior», no hay tal.
- B6 deja el FICHERO del libro en disco con un acto `FAILED`. Se declara, no se
  borra: borrar un fichero que quizá ya existía es pérdida de datos, y un libro
  con un intento fallido dentro es exactamente lo que el libro es.
- Ninguna garantía cubre un perfil cuyo `storage.path` apunte a un almacén de
  esquema antiguo: `OpenOperator` rehúsa por nombre («an operator act never
  migrates an existing store») y esa refusal es `ledger_not_created`.
- Un crash del PROCESO entre el cutover y el cierre sigue sin molde de
  crash-restart real (fichado en el papel del acto). Lo que sí se prueba (B8)
  es qué hace el arranque siguiente con lo que ese crash dejaría.
- **El escritorio no produce hoy el estado «sin almacén»**: `EnsureDefaultConfig`
  llama `EnsureChatBlocks`, que inyecta `storage: {}` en todo perfil de
  escritorio (`internal/shell/upgrade.go`). La fila con botón la alcanza un
  perfil de `korvun serve` escrito a mano y gobernado por un cliente admin, o
  un perfil de escritorio al que se le quitó el bloque a mano. Se aplica la
  opción conservadora (no tocar `EnsureChatBlocks`) y la pregunta va al
  informe: si el director quiere que el primer arranque nazca SIN almacén para
  que la pantalla lo explique, es una decisión suya.
- La ruta por defecto es la de `storage: {}` en todo el árbol
  (`<UserConfigDir>/korvun/korvun.db`), no «junto al perfil». Un perfil de
  `serve` en otra carpeta recibe su libro ahí; la alternativa (junto al
  fichero del perfil) exige que la app conozca la ruta del perfil, que hoy solo
  conoce el supervisor. Declarado para el director.

---

## 3 · Plan de fallos

| # | Fallo | Desenlace exigido | Molde | Nivel de evidencia |
|---|---|---|---|---|
| F1 | Perfil sin almacén intenta cada mutación: `enable-approvals`, `set-ceiling`, `lift-shadow`, `allow-host`, `POST /api/config` | `no_ledger` (503), recargador 0 llamadas, perfil intacto | existentes (`TestAct_noLedgerMeansNoChange`, `TestConfigAct_noLedgerMeansNoChange`) + NUEVO `TestMount_aProfileWithNoStoreRefusesEveryDoorButOne` (app real, servidor admin real en loopback, `ledgerlessRecorder`) | in-process, HTTP real en loopback |
| F2 | Bootstrap feliz sobre el supervisor real | 200 `applying`→`succeeded`; perfil en disco con `storage.path`; libro con el acto `config.enable-storage` en `SUCCEEDED` y recibo; después `POST /api/config` 202 | `TestReload_reprovisionsKeychainSecret` REESCRITO (antes: plantilla sin storage, POST directo 202; después: POST → 503 `no_ledger`, `enable-storage` → aplicado, POST → 202, canal conecta) | cutover REAL del supervisor, in-process |
| F3 | Bootstrap con fallo a mitad: el cutover rueda atrás (el test sostiene el lock del perfil en la carpeta destino, así que el `Build` del app nuevo rehúsa `ErrProfileLocked`) | `not_applied`; acto fundacional `FAILED` con recibo (cierre transitorio: no hay app atado); perfil sin `storage`; `POST /api/config` sigue 503 `no_ledger`; el fichero queda (declarado); un SEGUNDO intento en el mismo proceso readopta ese fichero (lo creó él) y, soltado el lock, se aplica — la reparación | NUEVO `TestBootstrap_aRolledBackCutoverClosesTheFoundingActAsFailed` (shell, con `sandboxUserDir`) | cutover REAL, in-process |
| F4 | Bootstrap sobre ruta no escribible (la carpeta de usuario de Korvun sin permiso de escritura) | `ledger_not_created`; recargador 0; perfil intacto; ningún fichero | NUEVO `TestBootstrap_anUnwritableProfileDirCreatesNoLedger` (shell, con `sandboxUserDir`; se salta como root) | cutover real disponible y NO pedido, in-process |
| F4b | Bootstrap cuando la ruta por defecto YA tiene un fichero que este proceso no creó | `ledger_exists` (409) nombrando la ruta; recargador 0; 0 actos; el fichero ajeno intacto byte a byte | NUEVO `TestBootstrap_aLedgerAlreadyThereIsNeverAdopted` (shell, con `sandboxUserDir`) | in-process |
| F5 | El libro se crea y se prepara, pero el SELLO del acto fundacional falla (store mentiroso que falla SOLO en `RecordAttemptAuthenticated`, inyectado por la costura de envoltura del recorder sin libro) | `act_not_recorded` nombrando el paso; recargador 0; perfil intacto; el fichero queda (declarado) | NUEVO `TestCreateLedger_aSealThatFailsAsksForNoChange` (app, store real envuelto) | in-process, store real |
| F6 | Bootstrap sobre perfil que ya tiene almacén | `refused` (422); 0 actos; 0 recargas | NUEVO `TestBootstrap_aProfileWithAStoreRefusesASecondOne` (controlapi, doble) | in-process, httptest |
| F7 | Cutover REAL que se aplica (cualquier puerta) | acto `SUCCEEDED` con recibo; `GET /api/reload/{h}` en el app nuevo responde `action_id` y `receipt_id` | `TestReload_pristinePersistAndAddrRotation` ELEVADO (antes: sin storage, solo perfil en disco; después: con storage + el acto y el recibo) y `TestProxy_reloadCutover_pollNeverSeesPhantomFailure` (con storage; las cinco rondas dejan cinco actos `SUCCEEDED`) | cutover REAL |
| F8 | Cutover REAL que rueda atrás (el cambio añade un canal cuya fábrica falla SOLO en el segundo build) | acto `FAILED` con recibo; `not_applied` | NUEVO `TestReload_aRolledBackCutoverClosesTheActAsFailed` (shell) | cutover REAL |
| F9 | Nadie sondea la puerta de estado | tras `succeeded`, el libro ya dice `SUCCEEDED` (observador) | NUEVO `TestReload_theActClosesEvenIfNobodyPolls` (shell) | cutover REAL |
| F10 | Observador y sondeo compiten: el segundo cerrador entra mientras el primero está DENTRO de `Finish` (barrera real: store envuelto cuyo `Finish` bloquea hasta que el segundo ha entrado) | un solo `Finish`; cero notas; el segundo responde el MISMO recibo, no vacío | `TestConfigActRecorder_closesTheActExactlyOnce` (existente, secuencial: no lo ve) + NUEVO `TestRegistry_aConcurrentSettlerWaitsForTheReceipt` (barrera) | in-process, store real envuelto |
| F10b | Vuelta atrás con el MISMO `storage.path` en la config vieja y la nueva (F8): el app viejo está apagado cuando el supervisor emite `rolled-back` | el cierre NO va por el recorder del app muerto (store cerrado); va por apertura transitoria; acto `FAILED` con recibo | F8 lo fuerza; mutación: atar por igualdad de ruta sin desatar en `Shutdown` → nota «database is closed» y acto abierto | cutover REAL |
| F11 | Recuperación con `keep` | el acto nombrado sobrevive `AUTHORIZED`; otro `AUTHORIZED` no nombrado cierra `OUTCOME_UNKNOWN` (control); sin `keep` cierran los dos | NUEVO `TestRecoverPreviousLife_keepsOnlyWhatTheLivingProcessOwns` (sqlite) | in-process, store real |
| F12 | Registro vacío (proceso nuevo) sobre un acto `AUTHORIZED` heredado | `OUTCOME_UNKNOWN` con recibo — B8 | mismo molde que F11, rama sin `keep` | in-process, store real |

**Tres moldes por cura, nombrados:** cada fila lleva su FALLO (la rama
peligrosa forzada), su CONTROL (la hermana que sigue verde y prueba que el
molde discrimina — F11 la lleva dentro; F2/F3 son la pareja; F7/F8 son la
pareja; F1/F2 son la pareja) y su REPARACIÓN (F2 tras F1: activar el almacén
convierte el `no_ledger` en 202; F3 no repara sola — el perfil queda como
estaba y la pantalla vuelve a ofrecer el botón).

**Mutaciones probatorias previstas** (se ejecutan y se capturan en
`evidence/v0.16.2/mutations.txt`, M57 en adelante): quitar el `keep` de la
recuperación (F7/F9 enrojecen con `OUTCOME_UNKNOWN`); no llamar al observador
desde `setStatus` (F9 enrojece); cerrar el acto del bootstrap como aplicado
pase lo que pase (F3 enrojece); sellar el acto DESPUÉS de `RequestReload` en el
bootstrap (F5 enrojece: el recargador ya fue llamado); que `CreateLedger` no
rehúse sobre un recorder con libro (F6 enrojece); que `ledgerlessRecorder`
devuelva un error genérico en vez de `ErrNoLedger` (F1 enrojece con
`act_not_recorded`); que el registro no recuerde la ruta del libro del acto
(F3 enrojece: nada puede cerrarlo); que `press` mande `brain: "—"` (molde TS
enrojece sobre el cuerpo); **control de F11**: con `keep` no vacío saltar TODOS
los `AUTHORIZED` (la pata de control enrojece); **F10**: que el segundo
cerrador no espere (responde sin recibo → enrojece); **F10b**: no desatar en
`Shutdown` (enrojece con la nota del store cerrado); **F4b**: adoptar un fichero
existente (enrojece: el acto aparece en el libro ajeno); **`keep` desde
`BindReload`** en vez de desde `BeginConfigAct` (no hay molde que lo fuerce sin
tocar el drenaje del supervisor — DECLARADO como mutación sin rojo alcanzable,
la razón es de construcción, no de prueba).

---

## 4 · Tests aprobados que cambian (antes → después), declarados

| Test | Antes | Después | Por qué |
|---|---|---|---|
| `TestReload_pristinePersistAndAddrRotation` | perfil sin `storage`; espera 202 | perfil con `storage` en el tempdir; espera 202 **y** el acto `SUCCEEDED` con recibo en la puerta de estado del app nuevo | decisión del director + F7 |
| `TestProxy_reloadCutover_pollNeverSeesPhantomFailure` | sin `storage` | con `storage`; al final, cinco actos `SUCCEEDED` | decisión del director + F7 |
| `TestReload_reprovisionsKeychainSecret` | plantilla sin `storage`, POST directo 202 | POST → 503 `no_ledger`; `enable-storage` → aplicado; POST → 202; el canal conecta | decisión del director + F2 |
| `TestContract_theHeaderArithmeticIsTrue` | 16 / 5 / 11 / 4 | 16 / 6 / 10 / 5 | la fila `store` gana botón (D9). El molde está diseñado para enrojecer aquí y corregirse con los números impresos |
| `TestMount_noAdminTokenMeansNoScreenDoors` | cuatro rutas a mano | cinco | puerta nueva |
| `fakeActs`, `extActs`, `realRecorder` | sin `CreateLedger`; recorder sin registro | ganan `CreateLedger` (rehúsa por defecto); `newConfigActRecorder` recibe el registro y la ruta | interfaz ampliada (D7), estado compartido (D3). Ningún aserto se relaja |
| `CONTRACT` (fixture TS) | fila `store` sin botón | con `has_button` y `door` | D9 |

---

## 5 · Lo no verificable, declarado

- El crash-restart real en la ventana entre cutover y cierre (fichado en el
  papel del acto; F12 prueba qué hace el arranque siguiente con esa herencia).
- La app EMPAQUETADA: la pasada manual del director es su casilla.
- `korvun serve` cableado con el registro se prueba por compilación y por el
  molde de shell; no hay molde que arranque el binario `serve` y haga un cutover
  en un proceso OS aparte.

---

## 6 · Las cinco preguntas, antes de la lectura

1. **¿Alguna frase promete más que el código?** No: el §0 del papel del acto se
   corrige (A6/T6 decían «no se montan» y el código monta y rehúsa; se reescribe
   a la verdad y a la excepción del bootstrap); «única puerta abierta» se prueba
   en F1 sobre las cinco puertas existentes y en F2 sobre la sexta.
2. **¿Algún comentario cita un test/función/fichero?** Los nombres de este plan
   se crean en el rojo con estos nombres exactos; la revisión final los verifica
   con grep.
3. **¿Algún molde entra por una función privada?** F5 y F10/F11 entran por el
   recorder y el store, que SON la unidad bajo prueba (nivel declarado); las
   puertas se prueban por HTTP real en F1–F4, F6–F9.
4. **¿Curé una puerta de una clase con varias?** Las hermanas del `no_ledger`
   son las cinco puertas (F1 las recorre); las hermanas del cierre tras cutover
   son éxito / vuelta atrás / persist-failed / preflight-failed — F7, F8 cubren
   dos; `persist-failed` cierra como aplicado por `actOutcome` (molde existente
   `TestConfigAct_theStatusDoor…` con doble) y `preflight-failed` cierra por el
   app viejo (hoy ya funciona; sin molde real de supervisor — DECLARADO).
5. **¿Alguna cura sin molde y sin mutación roja?** Ninguna prevista; la lista de
   mutaciones está en §3 y se captura en `mutations.txt`.

---

## 7 · Delta tras la lectura adversaria interna (9 min, VETO MANTENIDO: 3 P2)

Los tres P2 de producto se pliegan arriba, en su párrafo, y se listan aquí para
que la siguiente lectura ataque SOLO este delta:

| # | Hallazgo | Dónde se plegó |
|---|---|---|
| 1 P2 | el segundo cerrador concurrente recibía el acto SIN recibo (`settled` bajo mutex, `Finish` fuera, `receipts` después) | D4 («una vez» CON ESPERA), B4, F10 con barrera real |
| 2 P2 | «el recorder del app que sirve» elegía un app MUERTO en `rolled-back` (store cerrado, lock suelto, emitido antes de construir el de vuelta) | D4 (atar en `Build`, desatar en `Shutdown`, vitalidad y no igualdad de ruta), F10b |
| 3 P2 | la ruta por defecto puede ser el libro del ESCRITORIO; «crea» y «primer recibo» eran más anchas que `OpenOperator`, que adopta un store existente | D7 (solo crea; `ledger_exists`; readopción solo de lo que este proceso creó), D8, B2, B7, F4b |
| 4 P3 | el escritorio nunca entrega a `Start` un perfil sin `storage` | §2 «Lo que NO dicen», y pregunta al director en el informe |
| 5 P3 | `keep` solo cubre los actos de config de este proceso | D5 acotado, B3 acotado, fichado el acto de la CLI en vuelo |
| 6 P3 | `keep` debe nacer en `BeginConfigAct`, no en `BindReload` | D5 |
| 7 P3 [INSTRUMENT] | F5 sin costura ni taxonomía | D7 taxonomía, F5 con store envuelto |
| 8 P3 [INSTRUMENT] | `sandboxUserDir` no nombrado | F2–F4b lo nombran |
| 9 P3 [INSTRUMENT] | faltaba la mutación de la pata de control de F11 | §3 mutaciones |
| 10 P3 | frases falsas sin fichero | D9 las nombra por fichero |

Lo que la lectura NO revisó y sigue abierto para la pasada sobre el diff: la
idempotencia de `RegisterIdentity`/`wireIdentitySigners` en la readopción, los
moldes TS, y ninguna mutación ejecutada (la lectura fue solo lectura).

---

## 8 · Segundo delta — la pasada interna sobre el diff completo (≈7 min, VETO LEVANTADO)

Sin P1/P2 de producto. Un P2 de instrumento y ocho P3, adjudicados uno a uno y
TODOS curados, cada cura con su molde y su mutación roja (mutations.txt,
M83–M90):

| # | Hallazgo | Cura | Molde / mutación |
|---|---|---|---|
| 1 P2 [INSTRUMENT] | el molde F8 de shell declaraba roja una mutación que la evidencia captura verde (M66), y llevaba un either/or sobre el recibo de la puerta de estado | cabecera reescrita a la verdad (prueba el desenlace; el rojo de esa mutación vive en `app`, M66-bis); aserto estricto: la puerta de estado del app de vuelta responde EL recibo | M89 (la puerta omite el recibo → rojo) |
| 2 P3 | `keep` comparaba la GRAFÍA de la ruta: un `storage.path` re-escrito por el operador dejaba al acto en vuelo fuera de `keep` | `ledgerKey` = ruta absoluta y limpia (la misma resolución que `open`) para `homes`, `created` y el recorder | `TestRegistry_keepMatchesTheLedgerFileNotItsSpelling`; M84 verde a la primera (el molde unía con `filepath.Join`, que limpia: tautológico), M84-bis roja con grafías reales |
| 3 P3 | `settle` confiaba en `via` por ruta aunque Shutdown ya lo hubiera desatado; el godoc prometía vitalidad | `detach` marca el recorder `dead`; un `via` muerto nunca cierra | `TestRegistry_aDetachedRecorderIsNeverUsedToClose`; M85 |
| 4 P3 | el 409 «otro cambio en vuelo» respondía el acto SELLADO sin recibo; el del builder no llevaba ids | ambas puertas responden el acto CERRADO con su recibo | `TestAct_anotherChangeInFlightAnswersTheClosedAct`, `TestConfigAct_aRefusedReloadAnswersTheClosedAct`; M87, M88 |
| 5 P3 | `closedHandles`/`receipts` sin poda | `closedRetention` = 512, se olvida el más antiguo; `homes` de actos incerrables se reintenta en cada observador/sondeo (declarado) | `TestRegistry_forgetsTheOldestClosedActs`; M86 |
| 6 P3 [INSTRUMENT] | la mutación «quitar el filtro keep» estaba anunciada y no capturada | capturada | M83 |
| 7 P3 [INSTRUMENT] | `settledAct` sin llamador de producción | retirado; el molde usa `settleHandle`, el camino de la puerta de estado | — |
| 8 P3 | la pantalla sondeaba UNA vez tras el POST: contra un cutover real se quedaba en «Aplicando…» | `pollUntilTerminal`: cada 250 ms hasta 30 s, tolera un sondeo fallido (503 del proxy entre puertos) | molde TS con puerta de estado SECUENCIADA (pending, 503, cutover-in-progress, succeeded); M90 |
| 9 P3 | notas: «Seis llevan botón» y «Los cinco botones» en el mismo párrafo | reescrito: seis FILAS con botón, cinco PUERTAS, y por qué | `TestContract_theHeaderArithmeticIsTrue` sigue verde |

Lo que la pasada declaró no revisado y sigue abierto para la oficial: los tres
tests reescritos de shell, el papel del acto completo, `probe-real-cutover.txt`
y `red.txt`, los diffs de `approvals_adapter.go`/`identity.go`/`approvals.go`,
`HealthzBadge`, y M80–M82.

---

## 9 · Tercer delta — la pasada OFICIAL (10 min de 30, VETO MANTENIDO por un P2 de letrero)

Ningún P1. Un [PRODUCT] P2 público y nueve P3 (dos de instrumento). Todos
curados en la misma vuelta, con molde y mutación donde hay cable; lo que no cabe
en una línea se declara:

| # | Hallazgo | Cura | Molde / mutación |
|---|---|---|---|
| 1 P2 | las notas, la fila de la pantalla y un texto de resultado decían «en la carpeta del perfil»; el libro se crea en la carpeta de usuario de Korvun | las cuatro frases reescritas a la ruta real (`storage: {}`), en notas, fila `store`, `OUTCOME_ES` y fixture | `TestContract_theHeaderArithmeticIsTrue` y los moldes TS siguen verdes |
| 2 P3 | el godoc de `RegisterMutation` llamaba «lectura» a una ruta que puede cerrar el acto sin bearer | godoc a la verdad: la ruta responde el cierre y lo ejecuta si un sondeo llega antes que el observador, siempre con el desenlace del SUPERVISOR; ningún dato del cliente llega al libro. Gatear la ruta o hacerla solo-lectura cambiaría ADR-0028 §2 y un test aprobado (`TestConfigAct_theStatusDoorClosesTheActWhenItLearnsTheOutcome`): decisión del copiloto, opción conservadora aplicada | — |
| 3 P3 | act.go decía que la puerta de estado «no cierra»; mutation.go y su molde dicen que sí | act.go reescrito: responde el cierre y lo ejecuta si llega primero | — |
| 4 P3 | supervisor.go: «the desktop polls once», falso desde `pollUntilTerminal` | reescrito («the screen's poll is bounded») | — |
| 5 P3 [INSTRUMENT] | los moldes de vuelta atrás de shell pasaban aunque el observador ignorara los rollbacks (el sondeo del app de vuelta cerraba); B3/B6 «sin sondeo» solo probado en éxito | NUEVO `TestReload_aRolledBackCutoverClosesEvenIfNobodyPolls` (shell, cutover real, sin un solo GET /api/reload); notas acotadas y luego vueltas a la afirmación, ya probada | M91 (observador ignora rollbacks → rojo en shell y en app) |
| 6 P3 | tras reiniciar el proceso, el libro que este perfil fundó en un intento que rodó atrás responde `ledger_exists` llamándolo «ajeno» | el mensaje ya no lo llama ajeno: «solo adopta un libro que ella misma haya creado en esta sesión». Un marcador durable de «fundado por este perfil» es rediseño: DECLARADO para el director | molde TS de `ledger_exists` sigue verde |
| 7 P3 | TOCTOU: `Stat` → `OpenOperator` adoptaba un fichero aparecido en la ventana | creación EXCLUSIVA (`O_CREATE|O_EXCL`) y `Open` sobre el fichero vacío recién reclamado; la readopción (fundado aquí) sigue por `OpenOperator` | M93 (sin O_EXCL, el fichero ajeno se abre → `TestCreateLedger_aLedgerAlreadyThereIsNeverAdopted` y el de shell enrojecen) |
| 8 P3 | con `UserConfigDir` fallido, `storagePath` cae a una ruta RELATIVA al cwd | `CreateLedger` rehúsa `ledger_not_created` si la ruta no es absoluta | NUEVO `TestCreateLedger_refusesARelativeDefaultPath`; M92 |
| 9 P3 [INSTRUMENT] | F1 sobre app real no contaba llamadas al recargador | `profileReloader` cuenta: 0 tras las refusals, 1 tras `enable-storage` | molde del montaje ampliado |
| 10 P3 [INSTRUMENT] | F2/F3 aceptan `applying` o el terminal en la respuesta del POST | declarado en el molde como carrera de TIEMPO sobre un estado intermedio (el supervisor real puede haber terminado antes de responder), con los hechos terminales exactos | — |

Verificado y descartado por la pasada (no repetir): misma clave de firma en cierre
transitorio y arranque; `Preflight` no ata recorder; carrera bind→observador
cerrada; ningún secreto en el libro (digest); bearer en las cinco puertas y en
la lectura; `RegisterIdentity` idempotente en la readopción (F3 verde).

---

## 10 · Cuarto delta — la segunda vuelta oficial, acotada al delta (7 min de 15, VETO LEVANTADO)

Ningún P1/P2 de producto. Dos P2 de instrumento y cinco P3, todos plegados:

| # | Hallazgo | Cura | Molde / mutación |
|---|---|---|---|
| 1 P2 [INSTRUMENT] | `TestCreateLedger_refusesARelativeDefaultPath` estaba ROJO en el árbol: M92 fundó su libro en `internal/app/korvun/` (cwd del paquete) y el corredor restaura el fuente, no el disco; y `keys/receipt-signing.key` no está gitignored | residuo borrado; el molde rehúsa medir sobre un resto anterior y limpia su carpeta relativa al terminar; sin `Skip` (también fija `AppData` para Windows), la premisa fallida es un FAULT | — |
| 2 P2 [INSTRUMENT] | la captura de M93 era un `[build failed]` contado como rojo | M93-bis con una mutación que compila: rojo real en los dos moldes de `app` y en el de shell; recuento corregido y nota en la evidencia | M93-bis |
| 3 P3 | la readopción reconocía el fichero por su RUTA: un fichero borrado y sustituido en la misma ruta se readoptaba | `created` guarda `os.FileInfo` y `createdHere` compara con `os.SameFile` | `TestCreateLedger_aReplacedFileIsNeverReadopted`; M94 |
| 4 P3 | «nothing exists that did not before» era falso si `Open` fallaba tras el `O_EXCL`: quedaba un fichero de 0 bytes recordado como propio | la misma llamada borra el husk que reclamó y lo olvida; costura `openFresh` para forzarlo | `TestCreateLedger_anOpenThatFailsLeavesNothing`; M95 |
| 5 P3 [INSTRUMENT] | el molde de vuelta atrás sin sondeo aceptaba un cierre hecho por la recuperación del app de vuelta | exige `RecoveryMarker == ""`: cerrado por el observador | molde ampliado |
| 6 P3 [INSTRUMENT] | el molde de ruta relativa podía callarse (`Skip`) | sin `Skip` | — |
| 7 P3 | la frase TS de `ledger_not_created` nombraba la carpeta cuando la causa puede ser que no se resolvió | frase genérica; el `Detail` de Go lleva la causa exacta | — |

Con esto el tren no admite más vueltas oficiales por la regla del director
(dos como máximo); lo que queda es la pieza nueva del marcador durable, que
llega con sus propias pasadas.
