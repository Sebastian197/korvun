# Korvun como Execution Trust Layer

Blueprint maestro de producto, arquitectura y evolución

Estado: diseño aprobado para revisión

Fecha: 2026-08-15

## 1. Propósito del documento

Este documento define qué debe construir Korvun para convertirse en una infraestructura universal de confianza, autoridad y transacciones para agentes de IA.

No es todavía un plan ligado a archivos o funciones concretas. Es la referencia de producto y arquitectura que deberá dividirse después en especificaciones implementables. Cada etapa tendrá su propio diseño técnico, plan de código, pruebas y revisión.

La propuesta parte del estado real descrito en `KORVUN-CURRENT-REALITY-MAP-2026-08-15.md`: Korvun ya tiene un router de IA, dos tipos de Brain, governed tools, shadow, jaulas, escudo de red, auditoría operativa y un único punto de ejecución en `runTool`. La evolución debe aprovechar esas piezas sin romperlas.

## 2. La meta

Korvun debe convertirse en la capa que se sitúa entre cualquier agente y cualquier sistema real para responder, antes de cada acción:

> ¿Quién solicita esta acción, qué intención humana la justifica, qué autoridad la permite, qué efecto puede producir, necesita aprobación y qué prueba quedará después?

El producto objetivo acepta solicitudes procedentes de Brains internos, MCP, A2A, API, CLI u otros adaptadores. Todas se convierten al mismo modelo de acción, pasan por la misma política, usan el mismo sistema de ejecución y generan receipts compatibles.

La promesa central será:

> Ninguna acción con efecto real puede evitar el núcleo de confianza. Cada intento produce una decisión explicable y un registro durable.

## 3. El problema de software que resuelve

Los agentes actuales suelen ejecutar tools y APIs como llamadas independientes. Un permiso técnico permite usar una capacidad, pero no demuestra que una acción concreta corresponda con la intención actual del usuario.

Ejemplos del problema:

- Un agente autorizado para consultar una base de datos intenta modificarla.
- Un agente de compras supera el presupuesto expresado por el usuario.
- Un subagente recibe más autoridad que el agente que lo creó.
- Una llamada se repite tras un timeout y duplica un efecto externo.
- Una aprobación humana autoriza un plan, pero el agente ejecuta parámetros distintos.
- Una empresa puede ver que una tool se utilizó, pero no demostrar quién autorizó la acción.
- Una operación irreversible se trata igual que una lectura.

Korvun debe resolver la distancia entre tener acceso técnico y estar autorizado para realizar una acción específica.

## 4. Qué producto será Korvun

Korvun será una plataforma compuesta por cinco superficies coherentes:

1. Korvun Core: identidad, intención, autoridad, política, efectos, approvals, transacciones y receipts.
2. Korvun Gateway: adaptadores para agentes internos, API, MCP, A2A y futuros protocolos.
3. Korvun Executor: conectores gobernados y Credential Broker para operar sistemas externos.
4. Korvun Console: supervisión, previews, approvals, transacciones, investigación y prueba.
5. Korvun SDK: contratos para crear conectores, políticas y adaptadores sin introducir caminos alternativos de ejecución.

Korvun no será otro framework de agentes. Tampoco será solamente un sistema IAM, un proxy de tools o una herramienta de logs. Será el runtime que vincula intención, autoridad, acción, efecto y prueba.

## 5. Qué no intentará resolver al principio

La primera evolución no incluirá:

- Un servicio distribuido separado del proceso Go actual.
- Shell arbitrario o ejecución de código proporcionado por el modelo.
- Compatibilidad inmediata con todos los proveedores y protocolos.
- Rollback universal de acciones que el sistema externo no puede revertir.
- Multitenancy empresarial antes de estabilizar el modelo de acción.
- Un lenguaje de políticas generalista capaz de expresar cualquier condición imaginable.
- Uso de un LLM como autoridad final para aprobar acciones.
- Custodia de credenciales sin un modelo de amenaza y una implementación específica.

Estas exclusiones evitan que Korvun intente resolver infraestructura distribuida antes de validar su núcleo semántico.

## 6. Punto de partida real

### 6.1 Estado Git de referencia

| Elemento | Estado de referencia |
|---|---|
| `origin/master` | `81bfad8`, 2026-08-08 |
| `master` local y HEAD | `db45a7b`, 2026-08-15 |
| Divergencia | 48 commits por delante, 0 por detrás |
| Tipo de divergencia | Fast-forward puro |
| Staged y unstaged tracked | Ninguno |
| Governed tools y native tool calling | Local, no publicado |
| Panel de gobernanza del Builder | No implementado |

La primera obligación operativa es proteger, revisar y publicar de forma controlada el lote local. El futuro núcleo depende de capacidades que todavía no existen en el repositorio público.

### 6.2 Activos que deben reutilizarse

- `runTool` como cuello único entre el modelo y `Tool.Execute`.
- `SelectTools` como política pura y determinista.
- Doble puerta de anuncio y ejecución.
- Modos `allow`, `shadow` y `deny`.
- Jaula de `read_file` y resolución de symlinks.
- Allow-lists y escudo de red posterior a DNS.
- Carriles textual y nativo convergentes.
- Guardias de boot y configuración estricta.
- Preflight, hot reload, cutover y rollback del supervisor.
- SQLite v2 y separación actual de sesiones y turnos.
- Consola de Operador, Builder y desktop in-process.
- Bus, métricas y SSE sin contenido.
- Regla que impide modelos cloud en Brains privados.

### 6.3 Carencias que debe cubrir el nuevo producto

- Identidad autenticada del principal.
- Intent Contract.
- Autoridad limitada y delegable.
- Action Envelope de primera clase.
- Autorización por acción, argumentos y recurso.
- Presupuestos y expiración vinculados a la intención.
- Clasificación semántica de efectos.
- Approvals vinculados al digest exacto de una acción o plan.
- Idempotencia de efectos externos.
- Prepare, commit, abort y compensation.
- Ledger durable de acciones.
- Receipts verificables y firmados.
- Versionado de política por decisión.
- Gateway API, MCP y A2A.
- Aislamiento de tenants y políticas remotas en fases posteriores.

## 7. Principios de producto y arquitectura

### 7.1 La acción es el objeto central

Korvun no autorizará solamente una tool. Autorizará una acción concreta sobre un recurso concreto, con parámetros, actor, intención, autoridad, límites y expiración.

### 7.2 La intención precede a la autoridad

Un permiso no basta. Toda autoridad significativa debe estar vinculada a un Intent Contract que describa el resultado autorizado por el humano o sistema responsable.

### 7.3 La autoridad solo puede reducirse

Una delegación puede conservar o estrechar alcance, presupuesto, recursos y duración. Nunca puede ampliarlos.

### 7.4 Los efectos se entienden antes de ejecutarse

Korvun debe distinguir lecturas, escrituras reversibles, acciones compensables, operaciones irreversibles y acciones críticas. La política y la experiencia de aprobación dependen de esta clasificación.

### 7.5 Denegación por defecto

Una acción con efecto se deniega cuando falta identidad, intención, autoridad, política, clasificación de efecto o una aprobación requerida.

### 7.6 Los agentes no reciben secretos permanentes

El agente solicita una operación. Korvun ejecuta mediante un conector o entrega una credencial efímera y estrechamente limitada cuando el sistema externo lo permita.

### 7.7 Los estados son explícitos

Una acción o transacción nunca depende de un estado implícito dentro de un prompt. Cada transición se persiste y valida.

### 7.8 La prueba forma parte de la ejecución

El receipt no es un log opcional añadido al final. Es una salida obligatoria del protocolo de ejecución.

### 7.9 Neutralidad de protocolo

MCP, A2A, API, CLI y AgentBrain son adaptadores. No deben introducir semánticas de autorización distintas.

### 7.10 Honestidad sobre la reversibilidad

Korvun no afirmará que puede deshacer lo irreversible. Diferenciará rollback, abort, compensation y escalado humano.

## 8. Arquitectura objetivo

```text
Brain interno | API | MCP | A2A | CLI
                  |
                  v
        Protocol and Agent Adapters
                  |
                  v
          Principal Resolver
                  |
                  v
            Intent Service
                  |
                  v
         Action Canonicalizer
                  |
                  v
           Authority Engine
                  |
                  v
             Policy Engine
                  |
                  v
              Effect Engine
                  |
          +-------+--------+
          |                |
          v                v
     Preview/Shadow   Direct execution
          |
          v
    Approval Workflow
          |
          v
    Transaction Coordinator
          |
          v
   Executor and Credential Broker
          |
          v
      External systems
          |
          v
       Receipt Ledger
          |
          v
 Console | Proof API | Audit export
```

Todo el núcleo se construirá inicialmente dentro del proceso Go actual. Sus interfaces deberán permitir una separación futura, pero la primera versión no pagará el coste operativo de una arquitectura distribuida.

## 9. Componentes del núcleo

### 9.1 Protocol and Agent Adapters

Traducen solicitudes externas o internas al mismo formato. Verifican autenticación de transporte, límites de tamaño, replay básico y procedencia. No toman la decisión final de autorización.

Adaptadores previstos:

- AgentBrain actual mediante `runTool`.
- API de acciones versionada.
- MCP para interceptar y gobernar `tools/call`.
- A2A para tareas y delegaciones.
- CLI administrativa para desarrollo y recuperación.

### 9.2 Principal Resolver

Convierte evidencia autenticada en una identidad interna. Nunca confía en un `Sender.ID` o nombre proporcionado por el agente sin verificar su procedencia.

Debe distinguir:

- Humano.
- Agente.
- Workload o servicio.
- Operador.
- Sistema externo.

También conserva la relación entre actor directo, humano responsable, tenant y cadena de delegación.

### 9.3 Intent Service

Crea, valida, versiona, expira y revoca Intent Contracts. Una intención expresa el resultado autorizado y sus límites, no una secuencia rígida de tool calls.

Debe admitir:

- Alcance funcional.
- Recursos permitidos.
- Datos permitidos y prohibidos.
- Presupuestos cuantitativos.
- Ventana temporal.
- Clases de efecto permitidas.
- Reglas de aprobación.
- Capacidad de delegación.

### 9.4 Action Canonicalizer

Convierte una solicitud de tool, MCP, A2A o API en un `ActionEnvelope` estable. Valida el schema, canonicaliza parámetros, identifica recursos y calcula un digest antes de pedir autorización.

La canonicalización debe ser determinista. La misma acción lógica debe producir el mismo digest bajo la misma versión de schema.

### 9.5 Authority Engine

Resuelve la autoridad efectiva del principal para esa intención y acción. Combina grants, delegaciones, revocaciones, expiraciones y consumo de presupuestos.

No ejecuta tools ni consulta modelos. Su resultado es una autoridad efectiva explicable.

### 9.6 Policy Engine

Evalúa la acción completa. Recibe principal, intención, autoridad, recurso, parámetros canonicalizados, efecto, canal, modelo, tenant, hora y versión de política.

Produce una decisión estable:

- `allow`
- `deny`
- `shadow`
- `require_approval`
- `require_prepare`

La decisión incluye razones estructuradas y la identidad exacta de la política evaluada.

### 9.7 Effect Engine

Clasifica el impacto de cada operación mediante descriptores declarados por el conector y restricciones verificadas durante su registro.

No debe inferir efectos críticos a partir de texto libre del modelo.

### 9.8 Preview and Approval Workflow

Genera una vista comprensible de la acción o plan. Cuando la política exige aprobación, crea una solicitud vinculada al digest exacto. Cualquier modificación invalida la aprobación anterior.

### 9.9 Transaction Coordinator

Gestiona tareas con varias acciones, dependencias y efectos. Decide qué puede prepararse, qué se ejecuta directamente, qué requiere aprobación y qué compensaciones están disponibles.

### 9.10 Executor Registry

Es el único componente autorizado para invocar conectores. Sustituirá gradualmente el llamador directo de `Tool.Execute` sin crear un segundo camino de ejecución.

### 9.11 Credential Broker

Custodia referencias a secretos y obtiene credenciales de alcance reducido. Los secretos no aparecen en prompts, envelopes, receipts ni respuestas de tool.

### 9.12 Receipt Ledger

Persiste acciones, decisiones, approvals, intentos, resultados y compensaciones. El ledger es distinto del transcript conversacional y del bus operativo.

### 9.13 Console and Builder

Permiten administrar políticas, actores, autoridad, conectores y efectos; revisar previews; aprobar acciones; observar transacciones; buscar receipts y verificar pruebas.

## 10. Modelo de dominio

### 10.1 Principal

Representa al actor autenticado.

Campos mínimos:

```text
principal_id
principal_type
tenant_id
display_name
identity_provider
identity_subject
authentication_strength
created_at
disabled_at
```

`display_name` nunca se usa como identificador de autorización.

### 10.2 Identity Evidence

Registra cómo se autenticó el principal para una solicitud:

```text
evidence_id
provider
subject
credential_type
issued_at
expires_at
claims_digest
transport_binding
```

El material secreto no se persiste en este objeto.

### 10.3 Intent Contract

Representa la intención autorizada:

```text
intent_id
schema_version
tenant_id
owner_principal_id
purpose
allowed_operations
allowed_resources
prohibited_resources
data_constraints
effect_constraints
budgets
approval_rules
delegation_rules
valid_from
expires_at
status
version
contract_digest
signature
```

Estados:

```text
DRAFT -> ACTIVE -> EXPIRED
              \-> REVOKED
```

### 10.4 Authority Grant

Concede autoridad limitada dentro de una intención:

```text
grant_id
intent_id
issuer_principal_id
subject_principal_id
parent_grant_id
operations
resource_scope
data_scope
effect_ceiling
budgets
valid_from
expires_at
delegation_depth_remaining
status
grant_digest
signature
```

Una delegación válida debe ser un subconjunto del grant padre en todas sus dimensiones.

### 10.5 Action Envelope

Es el objeto central de Korvun:

```json
{
  "schema_version": 1,
  "action_id": "act_...",
  "correlation_id": "corr_...",
  "transaction_id": "tx_...",
  "intent_id": "int_...",
  "tenant_id": "tenant_...",
  "principal": {
    "principal_id": "agent_...",
    "evidence_id": "evidence_...",
    "responsible_human_id": "human_..."
  },
  "source": {
    "kind": "agent_brain",
    "protocol": "internal",
    "channel": "console"
  },
  "operation": {
    "namespace": "webhook",
    "name": "call",
    "version": 1
  },
  "resource": {
    "type": "endpoint",
    "id": "endpoint_...",
    "scope": "orders"
  },
  "parameters_digest": "sha256:...",
  "protected_parameters_ref": "params_...",
  "effect": {
    "class": "write_compensatable",
    "data_egress": true,
    "financial": false
  },
  "authority_refs": ["grant_..."],
  "idempotency_key": "idem_...",
  "requested_at": "...",
  "expires_at": "..."
}
```

El envelope no transporta secretos. Los parámetros sensibles se guardan en un almacén protegido y se referencian por ID.

### 10.6 Effect Descriptor

Clasifica la semántica de una operación:

```text
effect_class
reads_external_state
writes_external_state
data_egress
financial
credential_use
reversible
compensatable
criticality
prepare_supported
status_query_supported
idempotency_supported
```

Clases iniciales:

| Clase | Ejemplo | Tratamiento normal |
|---|---|---|
| `pure` | Cálculo local | Ejecutar dentro de límites |
| `read_external` | Consultar una factura | Autorizar y auditar |
| `write_reversible` | Crear un borrador | Ejecutar con undo documentado |
| `write_compensatable` | Crear un pedido cancelable | Registrar compensación |
| `write_irreversible` | Enviar un correo definitivo | Preview y posible aprobación |
| `critical` | Mover dinero o cambiar credenciales | Autoridad reforzada y aprobación |

La sensibilidad de datos será una dimensión separada. Una lectura puede ser más sensible que una escritura.

### 10.7 Policy Decision

```text
decision_id
action_id
outcome
reason_codes
policy_bundle_id
policy_version
policy_digest
authority_snapshot_digest
required_approval_policy
required_execution_mode
decided_at
expires_at
```

### 10.8 Approval

```text
approval_request_id
action_or_plan_digest
requested_from
reason
risk_summary
expires_at
status
decision_principal_id
decision
decision_at
comment
signature
```

Una aprobación no puede reutilizarse para una acción con digest distinto.

### 10.9 Transaction

```text
transaction_id
intent_id
principal_id
plan_digest
state
action_ids
dependency_graph
prepared_at
committed_at
aborted_at
compensation_state
version
```

### 10.10 Execution Receipt

```text
receipt_id
action_id
transaction_id
intent_digest
principal_id
authority_digest
decision_digest
approval_digest
action_digest
effect_descriptor_digest
executor_id
target_system
attempt
outcome
result_digest
external_reference
started_at
finished_at
previous_receipt_hash
receipt_hash
signing_key_id
signature
```

## 11. Flujo de una acción

1. El adaptador autentica la procedencia y crea evidencia de identidad.
2. El Principal Resolver obtiene el actor interno y el humano responsable.
3. El Intent Service valida que la intención esté activa.
4. El canonicalizador valida y normaliza operación, recurso y parámetros.
5. Korvun calcula el digest de la acción.
6. El Authority Engine resuelve grants y delegaciones vigentes.
7. El Effect Engine obtiene la clasificación registrada del conector.
8. El Policy Engine decide con una versión fijada.
9. Korvun persiste la solicitud y la decisión.
10. Si la política exige preview o aprobación, la acción queda pendiente.
11. La aprobación se vincula al digest exacto.
12. El Transaction Coordinator prepara o ejecuta según el conector.
13. El Credential Broker entrega la capacidad mínima al Executor.
14. El Executor llama al sistema externo.
15. Korvun consulta el estado cuando el resultado es ambiguo.
16. El ledger registra resultado, referencia externa y compensación disponible.
17. Se genera y firma el receipt.
18. El adaptador devuelve una respuesta sanitizada al agente.

## 12. Estados de acción

```text
RECEIVED
   |
   v
NORMALIZED
   |
   +-----------------> DENIED
   |
   v
AUTHORIZED
   |
   +-----------------> SHADOWED
   |
   +-----------------> PENDING_APPROVAL
   |                         |
   |                         +----> REJECTED
   |                         |
   |                         v
   |                      APPROVED
   |
   v
PREPARING
   |
   +-----------------> PREPARE_FAILED
   |
   v
PREPARED
   |
   v
COMMITTING
   |
   +-----------------> OUTCOME_UNKNOWN
   |
   +-----------------> FAILED
   |
   v
SUCCEEDED
   |
   +-----------------> COMPENSATING
                             |
                             +----> COMPENSATED
                             +----> COMPENSATION_FAILED
```

Las transiciones se validan en el dominio y se escriben atómicamente con su evento de ledger.

## 13. Política en dos niveles

Korvun conservará la doble puerta actual, pero separará sus funciones.

### 13.1 Puerta de capacidades

Decide qué operaciones puede conocer el modelo. Mantiene la protección actual de anuncio y soporta `shadow` para observar intención del modelo.

No constituye autorización final.

### 13.2 Puerta de acción

Decide la acción canonicalizada justo antes del efecto. Evalúa actor, intención, autoridad, recurso, parámetros, presupuesto, efecto, aprobación y versión de política.

### 13.3 Orden de comprobación

1. Schema y tamaño válidos.
2. Identidad autenticada.
3. Tenant y principal activos.
4. Intención activa y no expirada.
5. Autoridad válida y no revocada.
6. Operación permitida.
7. Recurso dentro de alcance.
8. Parámetros y datos dentro de límites.
9. Presupuesto disponible.
10. Efecto permitido.
11. Requisitos de shadow, prepare o approval.
12. Protección contra replay e idempotencia.
13. Disponibilidad del conector y Credential Broker.

La primera denegación aplicable queda registrada con un código estable. La UI puede explicar varias restricciones, pero el runtime mantiene una precedencia determinista.

## 14. Identidad y delegación

### 14.1 La identidad no procede del prompt

Los nombres incluidos por el modelo o en el texto del usuario son datos no confiables. La identidad debe proceder de un canal autenticado, token, sesión, certificado, workload identity o integración de identidad aprobada.

### 14.2 Cadena de responsabilidad

Korvun conservará:

```text
humano responsable
  -> agente principal
     -> subagente
        -> workload ejecutor
```

Cada salto incluye grant padre, límites heredados, expiración y profundidad restante.

### 14.3 Regla de atenuación

Una delegación es válida únicamente si:

```text
operaciones_hijas     subset operaciones_padre
recursos_hijos        subset recursos_padre
datos_hijos           subset datos_padre
presupuesto_hijo      <= presupuesto_restante_padre
effect_ceiling_hijo   <= effect_ceiling_padre
expiracion_hijo       <= expiracion_padre
profundidad_hija      < profundidad_padre
```

La validación debe ser determinista y susceptible de property testing.

## 15. Preview, shadow y approvals

### 15.1 Shadow

Shadow conserva su significado actual: el agente cree que la operación está disponible, Korvun observa la solicitud, pero no ejecuta el efecto.

El nuevo shadow produce un Action Envelope y una Policy Decision completos. Esto permite medir qué habría ocurrido bajo una política antes de activarla.

### 15.2 Agent diff

El preview debe mostrar:

- Objetivo de la intención.
- Actor y cadena de delegación.
- Recursos que cambiarán.
- Datos que saldrán del sistema.
- Coste o presupuesto consumido.
- Acciones reversibles, compensables e irreversibles.
- Credenciales o sistemas que se usarán.
- Políticas relevantes.
- Diferencia respecto al plan previamente aprobado.

### 15.3 Approval Workflow

La aprobación puede aplicarse a una acción o a un plan inmutable. Debe tener expiración, actor aprobador, autenticación suficiente y firma o prueba de decisión.

Una aprobación pierde validez si cambia:

- Operación.
- Recurso.
- Parámetros protegidos.
- Importe o presupuesto.
- Destinatario.
- Clase de efecto.
- Plan o dependencias.
- Política requerida.

## 16. Transacciones semánticas

### 16.1 Objetivo

Una transacción agrupa acciones relacionadas con una intención y permite evaluar el resultado global antes de externalizar los efectos que puedan prepararse.

### 16.2 Contrato de conectores

Un conector declara las operaciones que soporta:

```text
Describe
Validate
Plan
Execute
Prepare        opcional
Commit         opcional
Abort          opcional
Compensate     opcional
GetStatus      recomendado para efectos externos
```

Korvun nunca invoca una operación opcional que el conector no haya declarado.

### 16.3 Modos de ejecución

- Ejecución directa para operaciones puras o lecturas autorizadas.
- Prepare y commit cuando el sistema externo ofrece staging.
- Ejecución con compensation registrada para efectos compensables.
- Approval antes de ejecutar efectos irreversibles.
- Bloqueo o autoridad reforzada para acciones críticas.

### 16.4 Resultado desconocido

Un timeout no significa que el sistema externo no ejecutó la acción. Cuando exista ambigüedad:

1. La acción pasa a `OUTCOME_UNKNOWN`.
2. Korvun no repite el efecto automáticamente.
3. Consulta el estado mediante referencia externa o idempotency key.
4. Si no puede resolverlo, escala al operador.
5. El receipt refleja la incertidumbre.

### 16.5 Compensation

La compensación es otra acción gobernada. Requiere autoridad, política, idempotencia y receipt propios. No se ejecuta como callback oculto.

## 17. Idempotencia y replay

Cada acción effectful debe tener una idempotency key estable dentro de su intención y recurso.

Korvun mantiene:

- Registro durable de claves.
- Estado de cada intento.
- Referencia externa cuando el proveedor la devuelve.
- Resultado conocido o ambiguo.
- Política de reintento por operación.

Los conectores deben declarar si el sistema externo soporta idempotencia nativa. Si no la soporta, Korvun puede evitar repeticiones propias, pero no promete exactamente una ejecución frente a fallos fuera de su control.

## 18. Credential Broker

### 18.1 Principio

El agente no recibe claves administrativas, tokens permanentes ni contraseñas de sistemas externos.

### 18.2 Flujo

```text
Action autorizada
      |
      v
Credential Broker
      |
      +-> obtiene secreto por referencia
      +-> solicita credencial efímera si existe soporte
      +-> limita recurso, operación y duración
      |
      v
Executor usa la credencial
      |
      v
secreto eliminado de memoria según lifecycle
```

### 18.3 Requisitos

- Referencias de secretos, nunca valores, en configuración y receipts.
- Rotación y revocación.
- Auditoría de acceso al secreto.
- Separación entre operadores de política y custodios de credenciales.
- Redacción de errores.
- Integración inicial con el keychain existente y proveedores posteriores mediante interfaces.

## 19. Ledger y receipts verificables

### 19.1 Separación de superficies

Korvun mantendrá tres sistemas distintos:

1. Transcript: conversación entre usuario, agente y operador.
2. Observabilidad: métricas, SSE y logs sin contenido sensible.
3. Ledger: registro durable de decisiones y ejecuciones.

El ledger no se enviará automáticamente al transcript ni al feed general.

### 19.2 Propiedades del ledger

- Append-only a través de la API de dominio.
- Escrituras transaccionales.
- Orden estable por acción y transacción.
- Hash encadenado por partición.
- Detección de huecos o alteraciones.
- Backups y restauración verificables.
- Retención configurable por tenant.
- Cifrado de parámetros protegidos.
- Índices para acción, intención, principal, recurso y referencia externa.

### 19.3 Firma de receipts

El receipt se canonicaliza antes de firmarse. Incluye `signing_key_id`, algoritmo, digest y firma. La rotación conserva claves públicas o material de verificación suficiente para receipts históricos.

La cadena hash aporta evidencia de manipulación. No debe describirse comercialmente como inmutabilidad absoluta si el operador controla almacenamiento y claves.

### 19.4 Verificación

Korvun ofrecerá una operación offline o API para verificar:

- Canonicalización.
- Digest de la acción.
- Integridad del receipt.
- Firma.
- Cadena anterior.
- Validez temporal de la clave.
- Coincidencia con decisión, approval y resultado.

## 20. API y protocolos

### 20.1 API de acciones

Será la primera interfaz externa porque permite estabilizar contratos sin depender de semánticas ajenas.

Operaciones mínimas:

```text
CreateIntent
ActivateIntent
RevokeIntent
IssueAuthority
DelegateAuthority
SubmitAction
GetAction
ApproveAction
RejectAction
SubmitTransaction
GetTransaction
GetReceipt
VerifyReceipt
```

### 20.2 MCP

Korvun podrá actuar como gateway o proxy gobernado:

```text
MCP client
   -> Korvun MCP gateway
      -> Action Envelope
         -> Action Kernel
            -> MCP server externo
```

`tools/list` refleja capacidades visibles. `tools/call` nunca llama directamente al servidor externo sin pasar por el Action Kernel.

### 20.3 A2A

Las tareas y delegaciones A2A se traducen a intención, autoridad y acciones. Una task remota no recibe autoridad implícita por haber llegado mediante un peer autenticado.

### 20.4 SDK de conectores

El SDK exige descriptor de operaciones, schemas, efectos, idempotencia, timeouts, secretos, status query y compensaciones. Registrar un conector incompleto para una operación crítica debe fallar durante preflight.

## 21. Consola y Builder

### 21.1 Panel mínimo de governed tools

Antes de publicar la base local se completará el panel pendiente para editar:

- Tool grants.
- Modos `allow`, `shadow` y `deny`.
- Restricciones por canal.
- Atributos sensibles y de red.
- Jaulas y allow-lists.
- Skills y budgets de prompt.

### 21.2 Consola de confianza

La evolución añadirá vistas para:

- Principals y estado de identidad.
- Intent Contracts.
- Authority Grants y delegaciones.
- Acciones en shadow.
- Approvals pendientes.
- Agent diff.
- Transacciones y dependencias.
- Resultados ambiguos.
- Compensaciones.
- Receipts y verificación.
- Versiones de política.
- Acceso del Credential Broker.

### 21.3 Requisitos de seguridad de UI

- El navegador no recibe bearers administrativos permanentes.
- Las mutaciones usan protección contra replay y autorización por operación.
- Los parámetros protegidos aparecen redactados.
- Una aprobación muestra el digest y las diferencias desde la última revisión.
- Acciones críticas requieren autenticación reforzada cuando la política lo indique.
- Las vistas de observabilidad siguen sin contenido por defecto.

## 22. Persistencia y almacenamiento

### 22.1 Primera implementación

El primer Action Kernel usará SQLite para conservar la operación single-process actual. El ledger tendrá migraciones y lifecycle separados de las tablas de conversación.

Grupos de datos:

- Principals e identity evidence.
- Intents y versiones.
- Authority grants y delegaciones.
- Actions y parámetros protegidos.
- Policy decisions.
- Approvals.
- Transactions y dependencias.
- Execution attempts.
- Ledger entries y receipts.
- Signing keys públicas y metadatos de rotación.

### 22.2 Evolución distribuida

La extracción a un servicio o base compartida solo se plantea después de estabilizar contratos, invariantes y recuperación. La interfaz de almacenamiento no debe asumir que SQLite es distribuido ni prometer consenso que no existe.

## 23. Límites de confianza y amenazas

| Amenaza | Control requerido |
|---|---|
| Prompt injection | El modelo no decide autoridad; todas las acciones pasan por política |
| Confused deputy | Intención, principal y recurso vinculados a la decisión |
| Escalada por delegación | Atenuación comprobada en todas las dimensiones |
| Replay | Nonce, expiración, idempotency key y ledger durable |
| Efecto duplicado | Estado persistente, status query y reintentos por operación |
| SSRF | Conservar allow-lists y escudo dial-time actual |
| Escape de filesystem | Conservar jaula, symlinks resueltos y comprobación de fichero regular |
| Exfiltración de secretos | Credential Broker, referencias y redacción |
| Cambio de política en vuelo | Pin de versión y digest por decisión |
| Approval reutilizado | Vinculación al digest exacto y expiración |
| Alteración del audit | Ledger encadenado, receipts firmados y verificación |
| Resultado ambiguo | Estado explícito y consulta antes de repetir |
| Admin expuesto | Auth obligatoria, bind seguro y TLS cuando no sea loopback |
| Tenant crossing | Tenant en todas las claves y checks de autorización |

El modelo, el texto de usuario, los argumentos de tools y los peers externos se consideran no confiables. El núcleo de política, el ledger, las claves de firma y el Credential Broker forman la base de confianza y requieren límites operativos separados.

## 24. Compatibilidad que no puede perderse

1. `Orchestrator` sigue funcionando sin tools.
2. AgentBrain conserva los carriles textual y nativo.
3. Ambos carriles usan el mismo Action Kernel.
4. `shadow` nunca ejecuta.
5. Las tools desconocidas fallan cerradas.
6. La configuración conserva decode estricto y schema versionado.
7. Los Brains privados no usan modelos cloud.
8. `read_file`, `http_fetch` y `webhook_call` conservan jaulas y límites.
9. El escudo de red continúa validando la IP resuelta en cada conexión.
10. Los retries del modelo mantienen la misma semántica en ambos carriles.
11. Preflight valida conectores, políticas y secretos sin producir efectos.
12. Hot reload conserva la app anterior hasta confirmar la nueva.
13. El transcript no se contamina con trazas internas de acción.
14. Métricas, SSE y feeds no reciben secretos ni argumentos.
15. Skills siguen siendo documentación y no conceden autoridad.
16. Los configs existentes tienen una ruta de migración explícita.

## 25. Plan de evolución

### Etapa 0: consolidar la base gobernada

Objetivo: convertir el lote local en una base publicada, reproducible y operable.

Construir:

- Revisión final de los 48 commits locales.
- Panel mínimo de governed tools en el Builder.
- Visualización de eventos de tools en Actividad.
- Corrección de rutas administrativas sin auth o enforcement de loopback.
- Uso real de `outbound_token_env` en webhook.
- Documentación de schema actualizada.
- Release etiquetada con governed tools y native tool calling.

Pruebas obligatorias:

- Regresión completa de canales, Brains, sesiones y desktop.
- Tests de precedencia de `SelectTools`.
- Shadow sin llamada a `Execute`.
- Jaulas y escudo frente a symlinks, redirects, DNS rebinding y link-local.
- Reload con rollback.
- Prueba viva opt-in de Ollama.

Criterio de salida: un tercero puede instalar la release y reproducir governed tools sin editar código ni depender del árbol local del autor.

### Etapa 1: Action Kernel y Action Envelope

Objetivo: reificar cada tool call como acción sin cambiar todavía la experiencia exterior.

Construir:

- Paquete de dominio de acciones.
- IDs, schemas, canonicalización y digests.
- Máquina de estados inicial.
- Executor Registry.
- Adaptador de `runTool` al Action Kernel.
- Adaptadores para las seis tools existentes.
- Decisión compatible con grants actuales.
- Persistencia mínima de acciones y decisiones.

Pruebas obligatorias:

- El único camino a `Tool.Execute` pasa por el Executor Registry.
- Carriles textual y nativo producen envelopes equivalentes.
- Canonicalización determinista.
- Reinicio sin pérdida de estados terminales.
- Compatibilidad byte-level donde los protocolos antiguos la exijan.

Criterio de salida: toda tool actual genera un Action Envelope y una decisión antes de ejecutarse.

### Etapa 2: identidad, intención y autoridad

Objetivo: vincular acciones a un actor y a una autorización humana verificable.

Construir:

- Principal Resolver.
- Identity Evidence.
- Intent Service.
- Authority Grants.
- Expiración y revocación.
- Delegación con atenuación.
- Budgets básicos por intención.
- Migración explícita para grants legacy.

Pruebas obligatorias:

- Un `Sender` falsificado no cambia el principal.
- Autoridad expirada o revocada falla cerrada.
- Ninguna delegación amplía operaciones, recursos, presupuesto o tiempo.
- Consumo concurrente de budget no supera el límite.
- Property tests del modelo de subconjuntos.

Criterio de salida: cada acción effectful tiene principal, intent y authority válidos o queda denegada.

### Etapa 3: política por acción y efectos

Objetivo: decidir sobre recurso, parámetros y consecuencia, no solo sobre el nombre de la tool.

Construir:

- Effect Descriptor.
- Registro validado de operaciones de conectores.
- Policy Decision versionada.
- Alcance de recursos.
- Restricciones de datos.
- Límites cuantitativos.
- Pin de política y autoridad por acción.
- Nuevos resultados `require_approval` y `require_prepare`.

Pruebas obligatorias:

- Cambiar argumentos cambia el digest y fuerza una nueva decisión.
- Un reload no altera acciones ya autorizadas.
- Lectura y escritura reciben políticas distintas.
- Operaciones sin Effect Descriptor fallan en preflight o quedan denegadas.
- Las reglas actuales de canal, sensibilidad y localidad siguen vigentes.

Criterio de salida: Korvun puede explicar por qué una acción específica sobre un recurso fue permitida o denegada.

### Etapa 4: ledger durable y receipts

Objetivo: convertir la auditoría operativa en evidencia durable sin exponer contenido sensible.

Construir:

- Ledger append-only de dominio.
- Parámetros protegidos y cifrado.
- Receipts canonicalizados.
- Hash chaining.
- Firma y rotación de claves.
- Verificador offline y API.
- Backup, restore y comprobación de integridad.

Pruebas obligatorias:

- Una modificación de ledger se detecta.
- Receipts históricos siguen verificando tras rotar claves.
- Restaurar un backup conserva cadena y estados.
- Ningún secreto llega a logs, SSE o métricas.
- Todas las denegaciones y ejecuciones producen receipt o error fatal visible.

Criterio de salida: un tercero puede verificar qué se pidió, qué política decidió y qué resultado se registró.

### Etapa 5: preview, agent diff y approvals

Objetivo: permitir control humano preciso antes de efectos sensibles.

Construir:

- Generador de preview estructurado.
- Agent diff.
- Approval Workflow persistente.
- Expiración, rechazo y cancelación.
- Vinculación de approvals a digests.
- Inbox de approvals en la Consola.
- Autenticación reforzada configurable.

Pruebas obligatorias:

- Cambiar un parámetro invalida la aprobación.
- Dos operadores concurrentes no aprueban dos veces.
- Approval expirado no ejecuta.
- Shadow y approval no producen efectos antes de commit.
- El preview muestra datos salientes, coste y reversibilidad.

Criterio de salida: una acción irreversible no puede ejecutarse hasta que el humano apruebe exactamente la versión mostrada.

### Etapa 6: transacciones, idempotencia y compensation

Objetivo: gobernar tareas de varias acciones y recuperarse de fallos parciales.

Construir:

- Transaction Coordinator.
- Grafo de dependencias.
- Prepare, commit y abort.
- Registro de compensaciones.
- Idempotency store durable.
- Estado `OUTCOME_UNKNOWN` y reconciliación.
- Recuperación tras crash.

Pruebas obligatorias:

- Crash en cada transición posible.
- Reintento sin duplicar efectos cuando el proveedor soporta idempotencia.
- Un timeout ambiguo no repite automáticamente.
- Compensation requiere autorización propia.
- Fallo parcial produce estado coherente y visible.
- Pruebas de concurrencia sobre presupuesto y recursos compartidos.

Criterio de salida: Korvun puede ejecutar una tarea multiacción, recuperar su estado y distinguir commit, abort, compensation e incertidumbre.

### Etapa 7: Credential Broker

Objetivo: impedir que los agentes tengan acceso directo a credenciales potentes.

Construir:

- Referencias de secretos versionadas.
- Integración con keychain actual.
- Interface de proveedores de secretos.
- Credenciales efímeras cuando el destino las soporte.
- Policies de uso de credenciales.
- Rotación, revocación y audit de acceso.

Pruebas obligatorias:

- Secret scanning de prompts, envelopes, logs y receipts.
- Revocación durante una operación.
- Rotación sin invalidar receipts históricos.
- Fallo del broker sin fallback inseguro.
- Limpieza de credenciales al terminar lifecycle.

Criterio de salida: un agente puede completar una operación autorizada sin conocer la credencial permanente utilizada.

### Etapa 8: Consola y Builder completos

Objetivo: hacer operable el sistema sin editar JSON o consultar la base manualmente.

Construir:

- Gestión de principals, intents y grants.
- Editor de políticas y efectos.
- Árbol de delegación.
- Preview y approval inbox.
- Timeline de transacciones.
- Investigación de resultados ambiguos.
- Búsqueda y verificación de receipts.
- Herramientas de revocación y emergencia.

Pruebas obligatorias:

- E2E con navegador real para cada mutación crítica.
- Control de acceso por rol.
- Redacción visual de parámetros protegidos.
- Consistencia entre UI, API y ledger.
- Accesibilidad y flujos de error.

Criterio de salida: un operador formado puede configurar, aprobar, investigar y demostrar una ejecución desde la Consola.

### Etapa 9: Gateway API, MCP y A2A

Objetivo: convertir el núcleo validado en infraestructura vendor-neutral.

Construir:

- API pública versionada.
- Autenticación de workloads.
- MCP gateway.
- Adaptador A2A.
- SDK de conectores.
- Conformance suite.
- Rate limits y cuotas.

Pruebas obligatorias:

- La misma acción por API, MCP y AgentBrain produce la misma decisión.
- Ningún protocolo evita approvals o receipts.
- Fuzzing de decoders y schemas.
- Compatibilidad con implementaciones reales de MCP y A2A seleccionadas.
- Protección contra replay y cross-tenant access.

Criterio de salida: un agente externo puede usar Korvun sin conocer su implementación interna y recibe semántica uniforme de autorización.

### Etapa 10: multitenancy y operación empresarial

Objetivo: operar Korvun para varias organizaciones y cargas críticas.

Construir:

- Tenant isolation completo.
- Integración OIDC, mTLS o workload identity.
- Separación de roles administrativos.
- Almacenamiento compartido y estrategia de alta disponibilidad.
- Policy distribution y cache segura.
- Retención y exportación de evidencia.
- SLOs, alertas y capacity planning.
- Proceso de actualización y migración sin pérdida de ledger.

Pruebas obligatorias:

- Aislamiento entre tenants en todas las queries y caches.
- Failover y recuperación desde backup.
- Rotación de claves y credenciales en producción.
- Pruebas de carga y degradación.
- Revisión externa de seguridad.
- Pruebas de upgrade y downgrade admitidos.

Criterio de salida: Korvun puede operar como infraestructura profesional con responsabilidades, disponibilidad y evidencia documentadas.

## 26. Estrategia de pruebas profesional

### 26.1 Unitarias

- Canonicalización.
- Máquinas de estados.
- Atenuación de autoridad.
- Evaluación de políticas.
- Clasificación de efectos.
- Digests y firmas.

### 26.2 Property testing

- Una delegación nunca amplía autoridad.
- Una acción modificada nunca conserva el mismo digest salvo equivalencia canonicalizada.
- Una máquina de estados nunca acepta transiciones inválidas.
- Un budget nunca queda por debajo de cero.
- Un receipt válido deja de verificar si se altera un campo cubierto.

### 26.3 Integración

- Action Kernel con SQLite.
- Executor con tools actuales.
- Credential Broker con keychain.
- Policy Engine con reload.
- Approval con Consola.
- Connectors con sistemas simulados y reales opt-in.

### 26.4 Crash y recuperación

Se inyectarán fallos antes y después de cada escritura durable y llamada externa. La prueba debe verificar estado, idempotencia, receipt y comportamiento de recuperación.

### 26.5 Seguridad

- Fuzzing de inputs.
- Prompt injection orientada a efectos.
- Replay.
- Escalada de delegación.
- Cross-tenant access.
- SSRF y filesystem escape.
- Exfiltración de secretos.
- Manipulación de approvals y receipts.

### 26.6 Compatibilidad

Cada etapa ejecutará la suite de canales, Brains, modelos, sesiones, desktop, governed tools y reload. Una capacidad nueva no justifica degradar el gateway actual.

### 26.7 Conformance

El SDK de conectores y los gateways de protocolo tendrán suites que cualquier integración debe superar antes de declararse compatible.

## 27. Operación profesional

Antes de una release general, Korvun debe tener:

- Modelo de amenazas versionado.
- Runbooks de incidentes y recuperación.
- Backups probados mediante restauración.
- Rotación de claves y secretos.
- Métricas de decisiones, approvals, latencia, resultados ambiguos y compensaciones.
- Alertas de integridad del ledger.
- Límites de recursos y backpressure.
- Política de retención y borrado compatible con obligaciones de datos.
- SBOM, artefactos firmados y verificación de releases.
- Migraciones verificadas con copias reales representativas.
- Documentación para operadores, desarrolladores de conectores y auditores.
- Objetivos de servicio basados en pruebas de carga del despliegue soportado.

Korvun no debe anunciar alta disponibilidad, exactamente una ejecución, inmutabilidad absoluta o rollback universal sin demostrar las condiciones concretas bajo las que se cumplen.

## 28. Definición de MVP, beta y producto empresarial

### MVP de la Trust Layer

Incluye:

- Action Envelope.
- Principal autenticado.
- Intent Contract.
- Authority Grant.
- Política por acción.
- Effect Descriptor.
- Ledger durable.
- Receipt verificable.
- Shadow compatible.
- Un flujo de approval.
- Adaptación de las seis tools existentes.

Demostración mínima:

Un usuario crea una intención limitada. Un agente solicita una acción de lectura permitida y una escritura fuera de alcance. Korvun ejecuta la primera, deniega la segunda y genera receipts verificables para ambas. Una tercera acción irreversible queda pendiente hasta aprobación.

### Beta

Añade:

- Transacciones multiacción.
- Idempotencia y reconciliación.
- Compensation.
- Credential Broker.
- Consola completa.
- API pública.
- MCP gateway.
- Receipts firmados con rotación de claves.
- Recuperación probada tras crash.

### Producto empresarial

Añade:

- Multitenancy.
- Workload identity externa.
- Aislamiento y administración por roles.
- Almacenamiento y operación de alta disponibilidad.
- A2A.
- SDK y conformance suite.
- Integraciones de secretos empresariales.
- Exportación de evidencia y políticas de retención.
- SLOs y soporte operativo definidos.

## 29. Riesgos principales

| Riesgo | Respuesta de diseño |
|---|---|
| Intent Contract demasiado complejo | Empezar con schema limitado y versionado |
| Política difícil de explicar | Reason codes estables y decisión determinista |
| Falsa promesa de rollback | Clases de efecto y compensation explícita |
| Ledger filtra datos | Referencias protegidas, cifrado y redacción |
| Firma da falsa sensación de seguridad | Documentar modelo de claves y límite del operador |
| Protocolos desvían el núcleo | Adaptadores sin capacidad de ejecutar directamente |
| Migración rompe AgentBrain | Adaptador compatible y suite de regresión |
| Approvals se convierten en fatiga | Política por riesgo, batching seguro y previews claros |
| Exactamente una ejecución imposible | Declarar garantías por conector y resolver ambigüedad |
| Scope crece demasiado | Una especificación y release verificable por etapa |
| Credenciales siguen accesibles al agente | Executor exclusivo y revisión del proceso completo |
| UI se queda detrás del runtime | Criterios de salida que incluyen operación real |

## 30. Decisiones que deben permanecer abiertas hasta cada especificación

Estas decisiones requieren inspección del repositorio y diseño detallado antes de implementar, pero no cambian la arquitectura aprobada:

- Nombre definitivo de los packages Go.
- Schema físico exacto de las tablas.
- Algoritmo y formato de firma inicial.
- Proveedor de cifrado para parámetros protegidos.
- Lenguaje o formato de políticas.
- Protocolo exacto de workload identity para la beta.
- Estrategia de almacenamiento distribuido empresarial.
- Integraciones MCP y A2A que formarán la conformance suite inicial.

Cada decisión deberá resolverse en la especificación de su etapa, con alternativas, trade-offs, modelo de amenaza y compatibilidad.

## 31. Secuencia de especificaciones posteriores

Este blueprint debe descomponerse en los siguientes documentos antes de escribir código nuevo:

1. Consolidación y release de governed tools.
2. Action Kernel y migración de `runTool`.
3. Identidad, Intent Contract y Authority Grant.
4. Policy Engine por acción y Effect Registry.
5. Ledger y receipts verificables.
6. Preview y Approval Workflow.
7. Transaction Coordinator e idempotencia.
8. Credential Broker.
9. Consola de confianza.
10. API, MCP y A2A.
11. Multitenancy y operación distribuida.

Cada especificación produce un plan de implementación independiente. No se agruparán varias etapas en un único cambio masivo.

## 32. Criterio final de éxito

Korvun habrá alcanzado la meta cuando pueda demostrar este ciclo completo:

```text
Un humano expresa una intención limitada
        |
        v
Korvun crea un contrato verificable
        |
        v
Un agente recibe autoridad atenuada
        |
        v
Solicita acciones mediante cualquier protocolo soportado
        |
        v
Korvun canonicaliza, clasifica y decide cada acción
        |
        v
Simula o pide aprobación cuando corresponde
        |
        v
Ejecuta con credenciales que el agente no controla
        |
        v
Coordina commit, abort o compensation según el efecto
        |
        v
Genera una prueba durable y verificable
```

El producto no se define por cuántas tools soporta. Se define por la capacidad de demostrar que cada efecto fue solicitado por un actor identificable, correspondía con una intención vigente, estaba dentro de una autoridad limitada, fue ejecutado bajo una política conocida y dejó evidencia verificable.

