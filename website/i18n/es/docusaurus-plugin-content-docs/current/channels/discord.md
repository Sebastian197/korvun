# Discord

Korvun recibe los mensajes de Discord por el **Gateway** (un WebSocket, con
reconexión y resume automáticos) y responde por REST. La puesta en marcha
son unos clics en el Developer Portal — más **un interruptor manual que es
fácil pasar por alto**: el intent de Message Content.

## Paso 1 — Crea la aplicación y el bot

1. Abre el [Discord Developer Portal](https://discord.com/developers/applications).
2. **New Application** → ponle nombre → **Create**.
3. Abre la pestaña **Bot** en la barra lateral.

## Paso 2 — Copia el token del bot

En la pestaña **Bot**, bajo **Token**, pulsa **Reset Token** para revelarlo
y cópialo.

> **El token es un secreto.** Va en una variable de entorno — nunca en el
> fichero de configuración, un commit o un chat. Si se filtra, vuelve aquí
> y **Reset Token**.

## Paso 3 — Activa el intent de Message Content ⚠️

**El único paso no obvio. Sin él, Korvun conecta pero recibe cada mensaje
con el texto vacío — el bot parece "sordo".**

En la misma pestaña **Bot**, baja hasta **Privileged Gateway Intents**:

- **MESSAGE CONTENT INTENT** → interruptor **ON** → **Save Changes**.

Es autoservicio mientras tu bot tenga menos de 10.000 usuarios — y cada
usuario de Korvun corre su propio bot, así que estás muy por debajo por
construcción. Los intents de Presence y Server Members **no** hacen falta.

## Paso 4 — Invita el bot a tu servidor

1. Pestaña **OAuth2** → **URL Generator**.
2. En **Scopes**, marca **`bot`**.
3. En **Bot Permissions**: **View Channels** y **Send Messages**
   (obligatorios); **Read Message History** (recomendado).
4. Abre la **Generated URL** en tu navegador, elige tu servidor y
   **Authorize** — hasta el final.

El bot aparece ya (desconectado) en la lista de miembros del servidor.

## Paso 5 — Configura, exporta, arranca

`mode` es siempre `"gateway"`; `token_env` es el **nombre** de la variable:

```json
{
  "channels": [
    { "type": "discord", "mode": "gateway", "token_env": "DISCORD_BOT_TOKEN" }
  ],
  "routes": [
    { "channel": "discord", "brain": "assistant" }
  ]
}
```

```sh
export DISCORD_BOT_TOKEN=<your-bot-token>
korvun config check --preflight korvun.local.json
korvun serve --config korvun.local.json
```

Envía un mensaje en un canal que el bot pueda ver (o un MD). Korvun lo
enruta por tu cerebro y responde en el mismo sitio. Este round-trip está
verificado en hardware de punta a punta.

## Si algo no cuadra

- **Conectado pero nunca responde / respuestas en blanco** → el intent de
  Message Content sigue apagado. Repite el paso 3, **Save Changes**,
  reinicia `korvun serve`.
- **`config check --preflight` falla nombrando la variable** → el token no
  está exportado en la shell que corre Korvun. (El `config check` normal no
  lee secretos, así que no puede detectarlo.)
- **Autentica pero publicar falla con 403 Missing Access** → la invitación
  OAuth2 nunca se completó. Repite el paso 4 hasta el final y confirma que
  el bot está en la lista de miembros.
- **No ve un canal / no puede publicar** → reinvita con los permisos del
  paso 4 y comprueba que el canal permite el rol del bot.

## Bueno saberlo

- Texto de entrada y salida, canales de servidor y MDs. Adjuntos, hilos,
  slash commands, reacciones y ediciones quedan fuera de la superficie v1.
- Las respuestas de los modelos no pueden mencionar nunca
  `@everyone`/`@here`/roles — Korvun bloquea las menciones por defecto.

## Siguiente

- [Telegram](/channels/telegram) · [Webhook](/channels/webhook)
