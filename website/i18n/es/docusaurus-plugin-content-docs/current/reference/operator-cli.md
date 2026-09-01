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
y `critical` exigen además aprobación humana — que, hasta que llegue el
workflow de aprobación, muere con el no honesto `approval_unavailable`.
Los grants sin techo (la autoridad permanente de la raíz y los derivados
de config) se comportan exactamente como antes.

## El verificador (v0.14.0)

Desde v0.14.0 cada desenlace terminal deja un recibo firmado en una
cadena hash de solo-añadir, y la CLI lleva al juez.

```sh
korvun receipt verify --config korvun.json rcpt_…
```

Un recibo (o todos los de un id `act_…`), re-juzgado offline contra el fichero del store con siete
checks nombrados: roundtrip canónico, recomputación del hash, firma
Ed25519 contra la clave pública REGISTRADA, la ventana de validez de la
clave, el eslabón de cadena con su predecesor y la coherencia con la
fila de la acción. Cada fallo lleva su nombre (`hash_mismatch`,
`signature_invalid`, `custody_mismatch`, …) — jamás un «invalid»
genérico.

```sh
korvun ledger check --config korvun.json
```

La cadena entera, estructura primero: un recibo borrado se denuncia por
su hueco (`chain_seq_gap` con la posición que falta), una posición
clonada como `chain_seq_duplicate`, y después cada eslabón por los
mismos siete checks — el PRIMER eslabón roto detiene el veredicto con
su id de recibo y su motivo.

```sh
korvun receipt rotate-key --config korvun.json
```

Rotación atómica retira-y-activa de la clave de firma del perfil. El
acto de rotación deja SU PROPIO recibo sellado con la clave NUEVA; las
claves retiradas se conservan para siempre, así cada era de la cadena
verifica con la clave de su era. La verificación es de solo lectura; el
alcance honesto está documentado: el libro es tamper-evident, jamás
«immutable» — el operador controla almacenamiento y claves.

## El buzón de aprobaciones (v0.15.0)

Con `approvals.enabled` activado, una acción cuya clase de efecto
exige un sí humano ya no muere con el honesto `approval_unavailable`:
se APARCA como solicitud pendiente con su preview sellado, y la CLI
es donde decides.

El aparcamiento necesita un cerebro ACOTADO: pon `agent.effect_ceiling`
en el brain (por ejemplo `"write_reversible"`) — el cable que faltaba
aterrizó con esta etapa: ausente significa sin techo, exactamente como
antes, y entonces nada se aparca.

```sh
korvun approvals list --config korvun.json
```

Cada solicitud con su estado y su caducidad — las consultas van por la
puerta de solo lectura (sin migración, sin recovery, nada se escribe).

```sh
korvun approvals show --config korvun.json apr_…
```

EL DIGEST que apruebas, primero y bien visible; después el preview
completo — propósito, actor y posición en la delegación, operación,
recursos, qué datos salen, coste, clase de efecto y reversibilidad,
la ley pineada — y los parámetros CRUDOS (solo loopback: no existen
en ningún otro sitio).

```sh
korvun approvals approve --config korvun.json apr_…
korvun approvals reject --config korvun.json --comment "why" apr_…
```

Ambos son actos de operador registrados con su recibo firmado.
Aprobar ejecuta EL objeto guardado — recuperado íntegro, re-verificado
contra el digest aprobado, reclamado atómicamente para que dos
aprobaciones en carrera no disparen el efecto dos veces — y reporta el
desenlace real; el recibo de una acción aprobada sella su referencia
de aprobación (canónico v2), y `receipt verify` gana el check
`approval_mismatch`. El rechazo, la cancelación o la caducidad
cierran la acción aparcada con recibo y no queda ningún camino de
ejecución. Las solicitudes caducan por su TTL (por defecto 1h,
`approvals.ttl`), juzgado al toque de la decisión.

## Leer el rastro

Los recibos viven en el registro de acciones junto a todas las demás
acciones registradas. Cada fila lleva sus columnas de identidad —
principal, intención, autoridad — y su evidencia por intento (proveedor,
clase de credencial, subject). Nunca se almacena material secreto: las
CLASES de credencial son un enum finito por construcción.
