# Instalación

Korvun se distribuye como **un único binario autocontenido** — sin
dependencias en ejecución, sin nada más que instalar. Cada
[release](https://github.com/Sebastian197/korvun/releases) trae binarios
para `linux`, `darwin` (macOS) y `windows`, cada uno en `amd64` y `arm64`
(una Raspberry Pi 4/5 de 64 bits es `linux/arm64`), más un
`checksums.txt`, un SBOM y una firma cosign sin clave.

¿Prefieres una ventana a una terminal? Salta a
[Korvun Desktop](#korvun-desktop-la-app-nativa).

## 1. Identifica tu arquitectura — primero, siempre

Con la arquitectura equivocada descargas un binario que no arranca:

```sh
uname -m
```

| Imprime | Token de arquitectura |
|---|---|
| `x86_64` | `amd64` |
| `arm64` / `aarch64` | `arm64` |

En Windows (PowerShell): `$env:PROCESSOR_ARCHITECTURE` → `AMD64` o `ARM64`.

## 2. Descarga y verifica

Desde la [última release](https://github.com/Sebastian197/korvun/releases/latest),
descarga el archivo de tu SO/arquitectura más `checksums.txt` —
sustituyendo la versión y tu token de arquitectura en los nombres, p. ej.
`korvun_<VERSION>_linux_<ARCH>.tar.gz` (los archivos de Windows son
`.zip`).

Verifica siempre antes de ejecutar:

```sh
# Linux
sha256sum -c checksums.txt --ignore-missing
# macOS (ships shasum, not sha256sum)
shasum -a 256 -c checksums.txt --ignore-missing
# -> korvun_<VERSION>_<os>_<ARCH>.tar.gz: OK
```

`OK` significa que la descarga está íntegra. **No sigas si imprime
`FAILED`.**

> **Opcional, para la capa extra:** cada release firma `checksums.txt` con
> [cosign](https://docs.sigstore.dev/) sin clave — una firma responde por
> todos los artefactos, sin claves que repartir. Descarga
> `checksums.txt.sig` y `checksums.txt.pem` y ejecuta:
>
> ```sh
> cosign verify-blob checksums.txt \
>   --signature checksums.txt.sig \
>   --certificate checksums.txt.pem \
>   --certificate-identity-regexp 'https://github.com/Sebastian197/korvun/.*' \
>   --certificate-oidc-issuer https://token.actions.githubusercontent.com
> # -> Verified OK
> ```

## 3. Extrae y ejecuta

```sh
tar -xzf korvun_<VERSION>_<os>_<ARCH>.tar.gz
chmod +x korvun
./korvun --version
# -> korvun v<VERSION> (<short-revision>)
sudo install -m755 ./korvun /usr/local/bin/korvun   # optional: put it on PATH
```

En Windows, descomprime y ejecuta `.\korvun.exe --version` en PowerShell.

> **Nota macOS:** descargado en la terminal (`curl`, `gh`), el binario
> corre directamente. Descargado con un navegador, macOS lo pone en
> cuarentena — límpiala una vez con
> `xattr -d com.apple.quarantine ./korvun`, o clic derecho → Abrir.
> **Nota Windows:** el binario no está firmado con certificado, así que
> SmartScreen puede mostrar *"Windows protegió su PC"* — pulsa
> **Más información → Ejecutar de todas formas**.

## Korvun Desktop (la app nativa)

La misma release trae también **Korvun Desktop** — la pasarela completa
tras una ventana nativa: onboarding en el primer arranque, secretos
guardados en el llavero del sistema, y el builder visual embebido. Mismo
núcleo, misma versión; el binario headless sigue siendo la forma de correr
Korvun en un servidor.

| SO | Artefacto |
|---|---|
| macOS (universal, Intel + Apple Silicon) | `korvun-desktop_<VERSION>_darwin_universal.dmg` |
| Windows x64 | `korvun-desktop_<VERSION>_windows_amd64-installer.exe` |
| Linux x64 | `korvun-desktop_<VERSION>_linux_amd64.tar.gz` |

La familia desktop tiene su propio manifiesto firmado,
`checksums-desktop.txt`, que se verifica exactamente igual que el headless
de arriba.

> **Primer arranque:** estas builds no están notarizadas por Apple ni
> llevan certificado de Windows (una decisión deliberada y sin coste — la
> integridad la cubren las firmas cosign). El sistema pregunta una vez:
>
> - **macOS** — arrastra Korvun a Aplicaciones; ante el bloqueo de
>   Gatekeeper, abre **Ajustes del Sistema → Privacidad y seguridad**,
>   pulsa **Abrir de todos modos** y confirma con **Abrir**. macOS lo
>   recuerda para siempre.
> - **Windows** — en SmartScreen, **Más información → Ejecutar de todas
>   formas**. El instalador descarga el runtime WebView2 si falta.
> - **Linux** — descomprime y ejecuta `./korvun-desktop` (necesita
>   WebKitGTK, `libwebkit2gtk-4.1` en distros actuales); instala el
>   lanzador `.desktop` incluido para tener entrada de menú.

## Secretos: variables de entorno, por nombre

Korvun nunca acepta un secreto por línea de comandos ni en el fichero de
configuración. La configuración nombra la **variable de entorno** que
guarda cada secreto (`token_env`, `api_key_env`); Korvun lee el valor al
arrancar. Un secreto ausente es un error de arranque claro y con nombre —
Korvun no arranca sordo en silencio.

```sh
export TELEGRAM_BOT_TOKEN=<your-bot-token>   # the value "token_env" points to
korvun serve --config korvun.local.json
```

## Como servicio (Linux / Raspberry Pi)

El repositorio incluye una **unidad systemd endurecida** —
[`docs/packaging/korvun.service`](https://github.com/Sebastian197/korvun/blob/master/docs/packaging/korvun.service)
— con un usuario `korvun` dedicado, un `StateDirectory` para la base de
datos SQLite y un sandbox estricto. Apunta el `storage.path` de la
configuración a `/var/lib/korvun/korvun.db` y audita con
`systemd-analyze security korvun`.

## Actualizar

Korvun no se actualiza solo y nunca llama a casa. Actualizar es descargar
la release nueva de la misma forma (checksum incluido), reemplazar el
binario y volver a arrancar. Tu configuración y tus datos son ficheros
aparte — una actualización nunca los toca.

## Siguiente

Un bot respondiendo con un modelo local → [Inicio rápido](/es/guide/quickstart)
