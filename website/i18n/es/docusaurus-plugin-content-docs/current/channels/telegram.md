# Telegram

El canal más rápido para probar Korvun: crea un bot con BotFather, exporta
el token, añade tres líneas de configuración. Korvun recibe las
actualizaciones por **polling**, así que no necesita URL pública, ni TLS,
ni nada expuesto — funciona desde un portátil detrás de NAT.

## Paso 1 — Crea el bot con @BotFather

1. Abre [@BotFather](https://t.me/BotFather) en Telegram.
2. Envía `/newbot`, elige un nombre visible y un usuario único.
3. BotFather responde con el **token del bot** — cópialo.

> **El token es un secreto.** Quien lo tenga controla tu bot. Va en una
> variable de entorno — nunca en el fichero de configuración, un commit o
> un chat. Si se filtra, revócalo en @BotFather: `/mybots` → tu bot →
> **API Token** → **Revoke current token**.

## Paso 2 — Configura el canal

Añade un canal `telegram` y una ruta a tu configuración. `token_env` es el
**nombre** de la variable de entorno — el valor nunca aparece aquí:

```json
{
  "channels": [
    { "type": "telegram", "mode": "polling", "token_env": "TELEGRAM_TOKEN" }
  ],
  "routes": [
    { "channel": "telegram", "brain": "assistant" }
  ]
}
```

`mode` es siempre `"polling"`, y `routes[].channel` es el **nombre de
tipo** del canal (`telegram`). El cerebro es el que tengas configurado — el
[inicio rápido](/es/guide/quickstart) trae un fichero mínimo completo.

## Paso 3 — Exporta el token y arranca

```sh
export TELEGRAM_TOKEN=<your-bot-token>
korvun config check --preflight korvun.local.json   # confirms the var resolves
korvun serve --config korvun.local.json
```

Abre tu bot en Telegram y mándale un mensaje — la respuesta vuelve al chat,
contestada por los modelos que corra el cerebro enrutado.

## Bueno saberlo

- **Un WARN de `DeleteWebhook` al arrancar es esperado e inofensivo** —
  Korvun limpia de forma proactiva cualquier webhook residual antes del
  polling; en un bot que nunca tuvo uno, la llamada de seguridad puede
  avisar sin afectar a nada.
- Texto de entrada y de salida es la superficie v1.

## Siguiente

- [Discord](/es/channels/discord) · [Webhook](/es/channels/webhook)
- Cada campo de canal → [referencia de configuración](/es/reference/configuration)
