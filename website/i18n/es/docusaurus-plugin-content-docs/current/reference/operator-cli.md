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

## Leer el rastro

Los recibos viven en el registro de acciones junto a todas las demás
acciones registradas. Cada fila lleva sus columnas de identidad —
principal, intención, autoridad — y su evidencia por intento (proveedor,
clase de credencial, subject). Nunca se almacena material secreto: las
CLASES de credencial son un enum finito por construcción.
