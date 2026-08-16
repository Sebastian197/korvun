# Webhook

El canal webhook convierte a Korvun en un **endpoint HTTP**: cualquier cosa
que sepa hacer POST de JSON — el backend de un formulario, un CRM, un hub
IoT, otro servicio tuyo — envía un mensaje, Korvun lo enruta por un cerebro
y publica la respuesta en la URL que elijas. Sin cuenta de proveedor, sin
SDK — solo HTTP.

A diferencia de Telegram (polling) o Discord (Gateway), este canal
**escucha en un socket tuyo**: solo-loopback por defecto, y autenticado en
cada petición con un secreto compartido que generas tú.

## Paso 1 — Configura el canal

El canal webhook **no lleva `mode`**. `token_env` nombra la variable con el
secreto de entrada; `outbound_url` es adonde se envían las respuestas:

```json
{
  "channels": [
    {
      "type": "webhook",
      "token_env": "KORVUN_WEBHOOK_SECRET",
      "webhook": {
        "bind": "127.0.0.1:8090",
        "path": "/webhook",
        "outbound_url": "http://127.0.0.1:9099/replies"
      }
    }
  ],
  "routes": [
    { "channel": "webhook", "brain": "assistant" }
  ]
}
```

`bind` tiene por defecto `127.0.0.1:8090` y `path` `/webhook` — arriba se
muestran explícitos.

## Paso 2 — Genera el secreto y expórtalo

```sh
export KORVUN_WEBHOOK_SECRET=$(openssl rand -hex 32)
korvun config check --preflight korvun.local.json   # confirms it resolves
```

> **El secreto es un secreto.** Vive en el entorno, nunca en el fichero de
> configuración, un commit o un chat — la configuración solo guarda el
> **nombre** de la variable. Si se filtra, genera uno nuevo y reexporta.

## Paso 3 — Arranca y envía un mensaje

```sh
korvun serve --config korvun.local.json
curl -s http://127.0.0.1:8090/healthz   # liveness, no auth needed -> 200
```

Cada petición de entrada necesita el secreto Bearer y el content type JSON:

```sh
curl -i http://127.0.0.1:8090/webhook \
  -H "Authorization: Bearer $KORVUN_WEBHOOK_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"sender_id": "alice", "text": "hello korvun"}'
```

Un `200` significa que Korvun aceptó el mensaje y lo está enrutando. La
respuesta del cerebro va a tu `outbound_url` — no en esta respuesta HTTP.

## Paso 4 — Mira las respuestas

Un receptor desechable que imprime lo que recibe (solo la librería estándar
de Python):

```sh
python3 -c 'import http.server as h; \
c=type("C",(h.BaseHTTPRequestHandler,),{"do_POST":lambda s:(print(s.rfile.read(int(s.headers["Content-Length"])).decode()),s.send_response(200),s.end_headers())}); \
h.HTTPServer(("127.0.0.1",9099),c).serve_forever()'
```

Apunta `outbound_url` a `http://127.0.0.1:9099/replies` (como en el paso
1), envía un mensaje, y el JSON de la respuesta se imprime en la terminal
del receptor.

## conversation_id y el mapeo de campos

Korvun mapea tus campos JSON a su forma de mensaje; sobrescribe solo lo que
difiera, en `webhook.mapping`:

| Campo de Korvun | Clave JSON por defecto | Significado |
|---|---|---|
| sender id | `sender_id` | **obligatorio** — quién lo envió |
| sender name | `sender_name` | nombre visible, opcional |
| text | `text` | el cuerpo del mensaje |
| media url | `media_url` | URL de adjunto, opcional |
| media type | `media_type` | tipo MIME, opcional |
| conversation id | `conversation_id` | agrupa mensajes en un hilo |

**`conversation_id` es lo que da memoria al cerebro entre mensajes** —
reutiliza el mismo valor y Korvun los trata como una sola conversación. Si
falta, el respaldo es el sender id, así que cada remitente conserva un hilo
estable. La respuesta lleva el mismo `conversation_id` de vuelta.

```json
"webhook": {
  "outbound_url": "http://127.0.0.1:9099/replies",
  "mapping": { "conversation_id": "thread", "text": "message" }
}
```

## Qué significa cada rechazo

| Estado | Significado | Arreglo |
|---|---|---|
| `401` | `Authorization: Bearer` ausente o incorrecto. | Envía el secreto exacto del paso 2. |
| `415` | El Content-Type no es `application/json`. | Añade la cabecera (`; charset=utf-8` vale). |
| `413` | Cuerpo de más de 1 MiB. | Envía un payload menor. |
| `400` | JSON inválido, o sin remitente / sin contenido. | Corrige el JSON; incluye `sender_id` y texto o media. |
| `503` | Buffer de entrada momentáneamente lleno. | **Reintenta luego** — tu petición estaba bien. |

## Exponerlo al exterior (proxy inverso + TLS)

El bind por defecto es loopback, así que nada es alcanzable desde fuera de
la máquina. Si escuchas en una dirección no-loopback, **Korvun avisa al
arrancar**: el secreto Bearer cruzaría la red en claro. Korvun no termina
TLS por sí mismo — pon delante un proxy inverso que termine TLS y deja a
Korvun en loopback:

```nginx
server {
    listen 443 ssl;
    server_name korvun.example.com;

    ssl_certificate     /etc/letsencrypt/live/korvun.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/korvun.example.com/privkey.pem;

    location /webhook {
        proxy_pass http://127.0.0.1:8090/webhook;
    }
}
```

## Siguiente

- [Telegram](/channels/telegram) · [Discord](/channels/discord)
- Cada campo del webhook → [referencia de configuración](/reference/configuration)
