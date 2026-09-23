---
sidebar_position: 2
---

# CLI de operador: intenciones y grants

Desde v0.12.0 el binario `korvun` lleva las herramientas de autoridad del
operador: **contratos de intención** (qué autorizas, con límites) y
**grants de autoridad** (quién puede actuar bajo una intención, con
menos). Trabajan contra la misma base de datos local que usa el servidor,
con acceso breve y seguro sobre WAL — puedes usarlas mientras Korvun
sirve.

Cada mutación deja un recibo identificado en el registro de acciones: tú
como principal, evidencia loopback en proceso y una regla de auditoría
finita — `operator` para tus actos, `attenuation_violated` para una
delegación que el muro rechazó. Los rechazos también se registran: el
rastro dice por qué.

## Intenciones

Un contrato de intención declara un resultado autorizado y sus límites:
un propósito en palabras, un conjunto de operaciones, un conjunto grueso
de recursos, un presupuesto de acciones opcional y una ventana de validez
opcional.

```sh
korvun intent create --config korvun.json \
  --purpose "test week" \
  --operations calc,time \
  --max-actions 10 \
  --expires 2026-09-06T20:00:00Z
```

Imprime el id nuevo (`int_…`) en `DRAFT`. Flags: `--purpose` y
`--operations` (separadas por comas) son obligatorias; `--resources` vale
`*` por defecto; `--max-actions 0` significa ilimitado; `--valid-from`
por defecto es ahora; sin `--expires` no hay caducidad.

```sh
korvun intent activate --config korvun.json int_…
korvun intent revoke   --config korvun.json int_…
korvun intent list     --config korvun.json
korvun intent show     --config korvun.json int_…
```

El ciclo de vida falla cerrado y se camina desde el estado ALMACENADO:
`DRAFT → ACTIVE → EXPIRED | REVOKED`. Lo terminal es terminal — reactivar
una intención revocada falla honestamente, y el intento fallido deja su
recibo. `show` incluye el digest del contrato: un hash determinista de los
TÉRMINOS (el estado queda fuera — revocar cierra la vida del contrato, no
reescribe su identidad).

## Grants

Un grant da a un principal autoridad acotada bajo una intención.

```sh
korvun grant issue --config korvun.json \
  --intent int_… \
  --subject principal_brain_default \
  --operations calc \
  --max-actions 5 \
  --depth 1
```

La intención debe estar EN VIGOR en el instante de emisión: una intención
`DRAFT` deniega con `intent_inactive`, una ventana caducada con
`intent_expired` — el reloj gana a un estado rancio, y el rechazo queda
registrado.

```sh
korvun grant delegate --config korvun.json \
  --parent grant_… \
  --subject principal_ch_telegram \
  --operations calc
```

Delegar traspasa autoridad — y **la autoridad solo puede menguar**. El
hijo hereda la intención, la caducidad y el presupuesto del padre salvo
que los estreches con flags, lo emite el subject del padre con profundidad
`padre − 1`, y debe ser subconjunto de su padre en TODAS las dimensiones:
operaciones, recursos, presupuesto, caducidad, ventana de validez,
profundidad. Un hijo que amplía se deniega nombrando la dimensión ampliada
y jamás toca el disco. El mismo muro gobierna al propio kernel — la CLI
del operador no tiene aquí ningún poder especial.

```sh
korvun grant revoke --config korvun.json grant_…
```

Un grant revocado ya no delega nada (`authority_revoked`).

### Techos de efecto (v0.13.0)

Un grant puede llevar un **techo de efecto**: la clase de consecuencia
más alta que su autoridad puede alcanzar, sobre la escalera `pure <
read_external < write_reversible < write_compensatable <
write_irreversible < critical`.

```sh
korvun grant issue --config korvun.json \
  --intent int_… \
  --subject principal_brain_default \
  --operations calc \
  --effect-ceiling read_external
```

Delegar también tiene que menguar aquí: el hijo hereda el techo del
padre salvo que lo estreches, y un hijo que alcance POR ENCIMA se
deniega nombrando `effect_ceiling` — la décima dimensión de la
atenuación, juzgada por el mismo validador en todas partes. Bajo un
grant con techo (autoridad acotada), las acciones `write_irreversible`
y `critical` exigen además aprobación humana. Con `approvals.enabled`
puesto, esa exigencia APARCA la acción como petición pendiente que
decides en la bandeja de más abajo; sin él —y si el aparcamiento falla—
la llamada sigue muriendo con el no honesto `approval_unavailable`.
Los grants sin techo (la autoridad permanente de la raíz y los derivados
de config) se comportan exactamente como antes.

## El verificador (v0.14.0)

Desde v0.14.0 cada desenlace terminal deja un recibo firmado en una
cadena hash de solo-añadir, y la CLI lleva al juez.

```sh
korvun receipt verify --config korvun.json rcpt_…
```

Un recibo (o todos los de un id `act_…`), re-juzgado offline contra el
fichero del store. Lo que juzga la escalera, en el orden en que lo juzga: el
roundtrip canónico, el hash recomputado, la clave de firma encontrada
en el registro, la firma Ed25519 contra esa clave, la ventana de
validez de la clave, el eslabón de cadena con su predecesor, después
—cuando el recibo sella un digest de aprobación— la aprobación y su
lápida, y en último lugar la coherencia con la fila de la acción. `receipt
verify` imprime TODOS los fallos que encuentra, uno por línea, en ese
orden; `korvun ledger check`, más abajo, imprime solo el PRIMER fallo
del primer recibo que falla. Cada fallo que juzga la escalera lleva su
nombre (`hash_mismatch`, `signature_invalid`, `custody_mismatch`, …) en
vez de un «invalid» genérico; un recibo cuyos bytes almacenados no
parsean se rechaza con el error de lectura y sin nombre de escalera.

```sh
korvun ledger check --config korvun.json
```

La cadena entera de una partición, estructura primero: un recibo
borrado de DENTRO de la cadena se denuncia por su hueco
(`chain_seq_gap` con la posición que falta), una posición clonada como
`chain_seq_duplicate`, y después cada eslabón por los mismos checks —
el PRIMER eslabón roto detiene el veredicto con su id de recibo y su
motivo. Lo que prueba es que el perfil que nombra tu configuración es
consistente consigo mismo: un corte de cola, una cadena re-firmada con
una clave que el atacante registró en ese mismo perfil y una
configuración apuntada a otra tienda sobreviven las tres, y cada una
exige un anclaje externo que Korvun aún no entrega.

```sh
korvun receipt rotate-key --config korvun.json
```

Rotación atómica retira-y-activa de la clave de firma del perfil. El
acto de rotación deja SU PROPIO recibo sellado con la clave NUEVA; las
claves retiradas se conservan para siempre, así cada era de la cadena
verifica con la clave de su era. La verificación es de solo lectura; el
alcance honesto está documentado: el libro es tamper-evident, jamás
«immutable». El alcance no es un límite de acceso sino una propiedad:
los comandos prueban que el perfil es consistente consigo mismo. Las
cuatro cosas que NO prueban: que el perfil sea el que escribió tu
historia, que la cadena esté completa, que sus claves sean las
autoritativas y que una fila cuya columna de búsqueda cambió de clase
de almacenamiento esté presente en vez de ausente. Las notas de
seguridad del proyecto las llevan enteras.

## El buzón de aprobaciones (v0.15.0)

Con `approvals.enabled` activado, una acción cuya clase de efecto
exige un sí humano ya no muere con el honesto `approval_unavailable`:
se APARCA como solicitud pendiente con su preview sellado.

**Desde la v0.15.0 se decide desde DOS sitios, y es la misma
decisión.** La pantalla de Aprobaciones de Korvun Desktop lista lo
aparcado, enseña el documento entero y toma el sí detrás de una puerta
que hay que teclear; esta página documenta la CLI, que hace lo mismo
desde un terminal y es la única entrada en un servidor sin pantalla.
Las dos tocan el mismo almacén, los mismos cinturones y el mismo claim
que consume una vez los parámetros guardados: decida quien decida
primero, a la otra se lo dicen por su nombre. Ese consumo único tiene
dos límites conocidos. En la v0.15.0, un trigger que restaura los
parámetros dentro de la propia transacción del claim lo rompe; está
curado en la v0.15.1, la release actual. Y si otra conexión vuelve a
escribir los parámetros después de que el claim confirme, mientras la
acción siga APPROVED —una primera ejecución aún en curso, una cuyo
cierre falló, o un claim seguido de una caída antes del cierre—, una
segunda ejecución vuelve a disparar el efecto; una restauración después
de que la acción cerrara no obtiene una segunda. Ese segundo límite es
un problema conocido fichado para la v0.15.2.

El aparcamiento necesita un cerebro ACOTADO: pon `agent.effect_ceiling`
en el brain (por ejemplo `"write_reversible"`) — el cable que faltaba
aterrizó con esta etapa: ausente significa sin techo, exactamente como
antes, y entonces nada se aparca.

```sh
korvun approvals list --config korvun.json
```

Cada solicitud con su estado y su caducidad — las consultas van por la
puerta de solo lectura: sin migración de esquema, sin recovery, y una
conexión sellada que rechaza cada ESCRITURA a nivel de SQLite. No es
una promesa de que el fichero en disco quede intacto — la propia apertura crea los
sidecars de un store WAL, y reescribe la cabecera de journal de un
store dejado en otro modo.

```sh
korvun approvals show --config korvun.json apr_…
```

EL DIGEST que apruebas, primero y bien visible; después el preview
completo — propósito, actor y posición en la delegación, operación,
recursos, qué datos salen, coste, clase de efecto y reversibilidad,
la ley pineada — y los parámetros CRUDOS. Solo se guardan en el almacén
local, y la API de aprobaciones también los sirve, en la dirección del
servidor de administración (`observability.addr`, loopback por defecto).

```sh
korvun approvals approve --config korvun.json apr_…
korvun approvals reject --config korvun.json --comment "why" apr_…
```

Ambos son actos de operador registrados con su recibo firmado.
Aprobar ejecuta EL objeto guardado — recuperado íntegro, re-verificado
contra el digest aprobado, reclamado atómicamente para que dos
aprobaciones en carrera no obtengan las dos los parámetros (no frente a
una restauración confirmada mientras la acción siga APPROVED, el
problema conocido fichado para la v0.15.2) — y reporta el
desenlace real; el recibo de una acción aprobada sella su referencia
de aprobación (canónico v2), y `receipt verify` gana el check
`approval_mismatch`. El rechazo, la cancelación o la caducidad
cierran la acción aparcada con recibo y no queda ningún camino de
ejecución. Las solicitudes caducan por su TTL (por defecto 1h,
`approvals.ttl`), juzgado al toque de la decisión.

## Autoridad estricta (v0.16.0)

Apagada salvo que el perfil la pida. Con ella encendida, un inicio con efecto
necesita un principal autenticado y verificado, una intención firmada activa y
una cadena de autoridad firmada y activa completa, todo juzgado dentro de la
transacción que confirma el inicio. Encenderla son cinco actos del operador, en
este orden, y cada uno deja su recibo firmado:

```bash
korvun intent create-v2   --config korvun.json --file intent.json
korvun intent activate-v2 --config korvun.json int_pedidos 1
korvun authority admin-issue --config korvun.json --file grant.json \
                             --reason "why this authority exists"
korvun intent bind        --config korvun.json --actor principal_brain_ops \
                          --channel console --grant grant_pedidos_root int_pedidos 1
korvun authority activate --config korvun.json --profile profile_ops \
                          --reason "why this profile goes strict"
```

El último comando imprime un `activation_digest`. Va a la configuración, y el
perfil es estricto desde el siguiente arranque:

```json
"authority": { "mode": "strict", "activation_digest": "sha256:…" }
```

`authority` tiene además `issue`, `delegate`, `revoke` e `import-v1`, cada uno
con su forma `admin-` (`admin-issue`, `admin-delegate`, `admin-revoke`). LAS DOS
formas de `revoke` exigen `--reason`, y también todas las administrativas; lo
que añaden las administrativas es el operador humano registrado aparte del
emisor del grant. El `issue` ordinario exige que el actor sea el dueño de la
propia intención, y desde el CLI el actor es siempre `principal_local_operator`
— así que sobre una intención de otro dueño, el `issue` ordinario rechaza con
«action/sqlite: authority issuer mismatch» y la puerta es `admin-issue`.

**Lo que la receta de arriba no dice, y hace falta.** En un perfil que viene de
una release anterior, el primer comando se rechaza hasta que el servidor haya
arrancado una vez, para levantar el esquema del almacén. `grant.json` debe
llevar el `intent_digest` de la versión exacta de la intención, y ningún verbo
del CLI lo imprime: hoy se lee del almacén a mano. Y la forma de `intent.json` y
`grant.json` todavía no está documentada, mientras los dos analizadores rechazan
campos desconocidos o repetidos. Las tres cosas quedan fichadas.

**Qué debe decir una intención en modo estricto.** `read_file`, `http_fetch` y
`webhook_call` son allí de MUNDO CERRADO: solo arrancan bajo términos que
enumeren los recursos, las etiquetas de datos y los destinos que pueden tocar.
Una intención sin `allowed_resources` no concede ninguno, y el inicio se rechaza
por su nombre: «action: resource out of authority scope», o «action: authority
use unresolved» cuando los argumentos no se pueden resolver — ese texto literal,
no un código corto. Una ruta de `read_file` que no sea absoluta queda sin
resolver, porque esta capa no conoce la raíz de la jaula a la que la herramienta
la uniría. Y el alcance es de la INTENCIÓN, no de la operación: en cuanto una
intención enumera cualquier recurso, toda operación SIN analizador registrado se
rechaza también — `memory_note` incluido —, así que una intención con alcance
para `read_file` cierra en silencio las demás.

**Qué ve la persona.** Una petición aparcada bajo un perfil estricto lleva el
bloque `AUTORIDAD` en el documento de aprobación: quién pidió, bajo qué
contrato, por qué cadena de principales, y el presupuesto que quedaba CUANDO SE
APARCÓ — leído de una instantánea firmada y verificado contra esa firma en cada
lectura, no un contador en vivo.

**`--grant`, y qué pasa sin él.** El `--grant` de arriba es lo que ata el grant
firmado al enlace, y es la bandera que decide por qué autoridad resuelve el
perfil. Su valor es el `grant_id` que hay dentro del fichero que emitiste. El
enlace se rechaza, antes de escribir nada, salvo que se cumpla todo esto:

- el grant está ACTIVO, y también todos los que tiene por encima en su cadena;
- su sujeto es el `--actor` que nombras;
- **todos los grants de esa cadena llevan el `--channel` que nombras**;
- la intención está activa y coincide con el digest contra el que se emitió el grant.

El canal está en esa lista porque el inicio también lo comprueba: un enlace
escrito sobre un canal que el grant no lleva se rechazaría en cada arranque, y
no hay motivo para dejarte escribirlo.

Ata sin `--grant` y el enlace no lleva grant, así que un perfil estricto
resuelve su autoridad por la cláusula de configuración derivada de la lista de
herramientas del cerebro mientras el grant que emitiste queda ACTIVO y sin usar.
Nada arranca fuera del alcance de la intención en ninguno de los dos caminos —la
cláusula verifica los mismos términos—, pero la delegación, los grants hijos
atenuados y los presupuestos compartidos con el ancestro solo entran en el
camino de una ejecución por `--grant`.

**Atar otra vez CON `--grant` reemplaza el enlace DEL MISMO SELECTOR en vez de
fallar:** el anterior se conserva como REVOCADO —es el registro de qué autorizó
la acción de ayer— y el nuevo se escribe en la revisión siguiente.

El selector es `--actor` + `--channel` + `--conversation`, y esa tercera parte
importa más de lo que parece. `--conversation` es opcional; omitida, escribe el
enlace de CUALQUIER-CONVERSACIÓN, y un inicio cae en ese solo cuando la
conversación en la que corre no tiene enlace propio — una conversación nombrada
gana siempre.

Así que las dos direcciones son más estrechas de lo que parecen, y ninguna
reemplaza a la otra:

- Atar CON `--conversation` reemplaza solo el enlace de esa conversación. Las
  demás, y el de cualquier-conversación, quedan intactos.
- Atar SIN ella reemplaza solo el enlace de cualquier-conversación. **Cada
  conversación con enlace propio sigue resolviendo por el suyo**, lo que
  significa que el grant viejo las sigue autorizando.

**No hay un solo comando que reemplace todos los enlaces de un actor y un
canal.** Si estás rotando un grant porque se comprometió o hay que atenuarlo,
vuelve a atar cada conversación que tenga enlace propio, y también el de
cualquier-conversación. Listarlos no es posible desde el CLI hoy; los dos huecos
están fichados.

La línea `revoked binding` te dice que el selector que nombraste tenía titular, y
su ausencia te dice que ese selector estaba libre — NO te dice si otros
selectores siguen con el grant viejo.

El comando nombra el reemplazo, y nombra también el enlace anterior cuando lo
había:

```
revoked binding bind_act_5f1ce33dab70ba5918800de9ad4bf067
binding bind_act_3fd7470b7d2e62d0857d8c36daa46b79 -> int_pedidos version 1 ACTIVE under grant grant_pedidos_root
```

Un primer enlace sobre un selector libre imprime solo la segunda línea.

**Atar otra vez SIN `--grant` NO lo reemplaza.** Ese camino es una inserción
llana y el selector ya tiene una fila ACTIVA, así que se detiene en la regla de
unicidad de la base de datos y la imprime en crudo:

```
korvun intent bind: constraint failed: UNIQUE constraint failed: index 'execution_bindings_active_selector' (2067)
```

No se escribe ningún enlace y no se pierde nada —el acto rechazado queda en el
registro como FALLIDO, que es para lo que ese registro existe—, pero hoy no hay
camino de vuelta a la cláusula de configuración desde la línea de comandos. Dar
al camino sin grant el mismo reemplazo en sitio queda fichado.

Un `--grant` con valor vacío es un error de uso, no un enlace sin grant.

**Si el grant atado se revoca después**, todo inicio bajo ese enlace se rechaza
hasta que vuelvas a atar **con otro `--grant`**. El enlace no se repara solo, y
nada más lo repara.

## Leer el rastro

Los recibos viven en el registro de acciones junto a todas las demás
acciones registradas. Cada fila lleva sus columnas de identidad —
principal, intención, autoridad — y su evidencia por intento (proveedor,
clase de credencial, subject). Nunca se almacena material secreto: las
CLASES de credencial son un enum finito por construcción.
