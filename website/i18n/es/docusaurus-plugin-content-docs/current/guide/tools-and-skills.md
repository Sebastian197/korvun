# Herramientas y skills gobernadas

Los cerebros agente de Korvun pueden usar herramientas — y el **motor de
políticas es el gatekeeper**: tú decides qué herramienta puede usar cada
cerebro, en qué canal, en qué modo, y cada uso, ensayo y denegación se
audita exactamente igual que las decisiones de enrutado. Las skills son
ficheros markdown que enseñan a un cerebro CUÁNDO usar sus herramientas;
nunca conceden nada.

Las decisiones de diseño viven en el repositorio:
[ADR-0041](https://github.com/Sebastian197/korvun/blob/master/docs/adr/0041-governed-tools-shadow-shield-skills.md)
(gobernanza, modo shadow, el escudo de red, herramientas enjauladas,
skills). Campos de configuración: la
[referencia de configuración](/reference/configuration).

## El catálogo

| Herramienta | Qué hace | Alcance |
|------|--------------|-------|
| `time` | Hora UTC actual. | ninguno (pura) |
| `echo` | Devuelve sus argumentos. | ninguno (pura) |
| `calc` | Aritmética acotada. | ninguno (pura) |
| `read_file` | Lee un fichero de texto. | SOLO bajo tu `root` configurado (los escapes por symlink mueren en la comprobación de ruta resuelta); con tope de tamaño. **Sensible**: solo modelos locales. |
| `http_fetch` | HTTP GET. | SOLO tus `allow_hosts`; respuesta con tope; redirecciones solo a hosts listados, con tope de saltos. |
| `webhook_call` | HTTP POST de un payload JSON — tu fábrica de herramientas no-code (flujos n8n, domótica, cualquier webhook). | SOLO tus `allow_hosts`; respuesta con tope; timeout duro; SIN redirecciones. |
| `memory_note` | Almacena una nota breve que el cerebro recordará en su ámbito. | SOLO el almacén de conversaciones, dentro del ámbito declarado del cerebro. Mira [memoria gobernada](/guide/memory). |

La ejecución de shell no existe y no se resolverá — por decisión, no por
omisión.

Una herramienta enjaulada no puede existir sin su jaula: listar
`read_file` sin su bloque de configuración es un error de arranque, nunca
un valor por defecto — y listar `memory_note` sin su bloque `memory`, o
sin un permiso de gobernanza que la cubra, rechaza el arranque de la misma
manera.

## El gatekeeper

Los permisos son por cerebro, tri-estado:

- **`allow`** — la herramienta se anuncia al modelo y se ejecuta.
- **`shadow`** — la herramienta se ANUNCIA pero NUNCA se ejecuta: la
  intención del modelo queda registrada (auditada como `tool_shadowed`) y
  el modelo recibe una observación de simulación honesta. **Usa shadow
  para observar el juicio real de un cerebro antes de confiar en él**:
  concede `shadow`, observa el feed de Actividad o `/tools`, y cuando te
  guste lo que ves, aplica en caliente el permiso a `allow` — sin
  reiniciar.
- **`deny`** — ni se anuncia ni puede ejecutarse.

Las restricciones siempre ganan al modo concedido: una restricción de
canal, la regla de sensibilidad (una herramienta `sensitive` nunca llega a
un cerebro con modelo cloud) y las jaulas se aplican incluso con `allow`.
Un cerebro SIN bloque `governance` está sin gobernar: toda herramienta
listada, permitida en todos los canales.

## El escudo de red (cerebros privados)

Un cerebro declarado `"sensitivity": "private"` recibe el escudo en sus
herramientas de red: `http_fetch` y `webhook_call` solo pueden alcanzar
direcciones PRIVADAS (loopback, RFC1918, IPv6 ULA/link-local) — Y aun así
solo hosts de la lista de permitidos. La comprobación corre sobre la IP
RESUELTA en el momento de conectar, así que el DNS rebinding y las
redirecciones hacia la internet pública mueren en el socket sin enviar
nada. El escudo restringe, nunca amplía: un host público en tu lista de
permitidos sigue denegado.

## La auditoría — y `/tools`

Cada uso, ensayo y denegación aterriza en tres superficies: logs
estructurados, métricas Prometheus y el feed de Actividad — solo
metadatos: cerebro, herramienta, canal, resultado, regla, latencia; NUNCA
los argumentos de la herramienta.

Desde el canal console del chat de escritorio, envía **`/tools`** para
obtener el informe del gatekeeper del cerebro de esa conversación: sus
permisos efectivos (con restricciones de canal, sensibilidad y el escudo)
y la actividad reciente de herramientas. Es una respuesta de sistema — sin
modelo por medio.

## Skills — enseñar CUÁNDO

Una skill es un directorio con un `SKILL.md` (el formato abierto
AgentSkills):

```markdown
---
name: home-assistant
description: Teaches when to call the home automation webhook. Use when the user asks to turn things on or off.
---

# Home automation

When the user asks to switch a device, call webhook_call with the
n8n flow URL and a JSON body like {"device": "...", "state": "on"}.
Confirm to the user what was switched.
```

Reglas (validadas al cargar):

- El nombre del directorio DEBE ser igual al `name` del frontmatter (1–64
  caracteres, minúsculas/dígitos/guiones simples).
- `description` es obligatoria (1–1024 caracteres): di qué Y cuándo.
- `allowed-tools` se registra pero NUNCA concede nada — las skills son
  documentación; la decisión del gatekeeper es final.
- `SKILL.md` tiene un tope de 64 KiB. Una skill malformada se omite con un
  aviso al arrancar; nunca detiene Korvun.

Apunta el cerebro al directorio con `skills_dir`. Los nombres y
descripciones de las skills siempre se suman al prompt de sistema del
agente; los cuerpos se incluyen bajo un presupuesto total
(`skills_body_budget`, por defecto 8192 runas).

## Un ejemplo completo

```json
{
  "name": "casa",
  "sensitivity": "private",
  "policy": {"kind": "priority"},
  "models": [{"provider": "ollama", "model_id": "llama3.2", "locality": "local"}],
  "agent": {
    "tools": ["time", "calc", "read_file", "webhook_call"],
    "governance": [
      {"tool": "time", "mode": "allow"},
      {"tool": "calc", "mode": "allow"},
      {"tool": "read_file", "mode": "allow", "channels": ["console"]},
      {"tool": "webhook_call", "mode": "shadow"}
    ],
    "read_file": {"root": "/home/chano/korvun-notes"},
    "webhook_call": {"allow_hosts": ["192.168.1.20:5678"]},
    "skills_dir": "/home/chano/korvun-skills"
  }
}
```

Este cerebro: lee notas solo de una carpeta y solo desde el canal console;
ENSAYA el webhook de n8n en shadow (observa `/tools` y luego promociona a
`allow` con una aplicación en caliente); y — al ser privado — nunca podría
alcanzar una dirección pública a través de una herramienta de red aunque
la lista de permitidos lo dijera.
