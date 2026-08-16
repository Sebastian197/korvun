# El builder visual

El **builder** es un panel visual, servido por el propio Korvun, donde
editas tu configuración — canales, cerebros, rutas, políticas y modelos —
desde el navegador en vez de escribir JSON a mano. Desde v0.6.0 es un
**lienzo**: arrastra bloques desde una paleta, conéctalos con cables, edita
cada pieza en su panel y aplícalo todo en vivo. No hace falta ser
desarrollador.

El servidor de administración de Korvun expone dos cosas en el navegador:

- **`/ui`** — una vista en vivo de solo lectura: mira los mensajes fluir en
  tiempo real.
- **`/builder`** — el panel editable del que va esta guía.

Sin token de administración, `/builder` directamente no se sirve — una
instalación recién hecha es de solo lectura y segura por defecto.

## 1. Activa la edición (el token de administración)

La edición está protegida por un **token bearer de administración** — un
secreto que eliges tú. Dos pasos:

**a)** Nombra la variable de entorno del token en tu configuración — el
**nombre**, nunca el secreto:

```json
{ "admin": { "token_env": "KORVUN_ADMIN_TOKEN" } }
```

**b)** Exporta el token antes de arrancar Korvun:

```sh
export KORVUN_ADMIN_TOKEN="a-long-random-secret-you-choose"
korvun serve --config korvun.local.json
```

> **El token de administración es un secreto.** Quien lo tenga puede
> cambiar cómo corre Korvun. Vive solo en el entorno — nunca en el fichero
> de configuración, chats, capturas ni logs. Si se filtra, cambia el valor
> y reinicia.

## 2. Abre el builder

Con Korvun corriendo, abre:

```
http://127.0.0.1:2112/builder
```

`127.0.0.1:2112` es la dirección por defecto del servidor de
administración — loopback, alcanzable solo desde la misma máquina. Pega tu
token cuando te lo pida. El token se mantiene **solo en memoria**, viaja
como cabecera `Authorization: Bearer` — nunca se guarda, nunca es una
cookie. Si recargas la página, lo pegas otra vez.

## 3. Compón en el lienzo

Tu configuración aparece como bloques y cables:

- **Arrastra** canales, cerebros y modelos desde la paleta al lienzo.
- **Cablea** un canal a un cerebro — el único cable manual; el lienzo
  valida qué puede conectarse con qué.
- **Edita** cualquier bloque en su panel de propiedades — la personalidad
  del cerebro incluida.
- **La privacidad se ve**: marca un cerebro `private` y cada modelo cloud
  muestra un **cable gris discontinuo** — excluido antes del despacho,
  dibujado en el lienzo en vez de enterrado en JSON.
- **Borrar** elimina un bloque y todo lo que solo tenía sentido con él
  (sus cables), con confirmación.

## 4. Guardar y recargar — en vivo, con red de seguridad

Nada se aplica hasta que pulsas **Save and reload**. Entonces Korvun aplica
la nueva configuración **sin reiniciar**:

- El formulario se bloquea mientras ocurre el cambio; ves los estados
  reales: **reloading → reload succeeded**.
- Si sale bien, Korvun reescribe tu fichero de configuración en disco para
  que coincida.
- **Si la nueva configuración no puede arrancar, Korvun revierte**: sigue
  corriendo con la configuración anterior, muestra **reload rolled-back**,
  y tu fichero en disco no se sobrescribe con algo roto. Corrige y prueba
  otra vez.

Pulsa **Discard** para tirar los cambios sin aplicar.

## 5. Seguridad — léelo, en serio

- El servidor de administración escucha en **loopback por defecto**, así
  que el builder solo es alcanzable desde la máquina donde corre Korvun.
  Es deliberado: un token bearer sobre HTTP plano solo es seguro cuando
  nunca cruza una red.
- **No expongas el servidor de administración a la red** (`0.0.0.0`) sin
  TLS y control de acceso delante. Si abres el builder desde cualquier
  sitio que no sea loopback o HTTPS, el propio panel te avisa de que un
  token pegado viajaría en claro.

## Siguiente

- Escribir la configuración a mano → [referencia de configuración](/reference/configuration)
- Poner Korvun en marcha primero → [inicio rápido](/guide/quickstart)
