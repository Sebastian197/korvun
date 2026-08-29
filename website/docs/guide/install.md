# Install

Korvun ships as a **single self-contained binary** — no runtime dependencies,
nothing else to install. Every [release](https://github.com/Sebastian197/korvun/releases)
carries binaries for `linux`, `darwin` (macOS), and `windows`, each on `amd64`
and `arm64` (a 64-bit Raspberry Pi 4/5 is `linux/arm64`), plus a
`checksums.txt`, an SBOM, and a keyless cosign signature.

Prefer a window to a terminal? Jump to [Korvun Desktop](#korvun-desktop-the-native-app).

## 1. Pick your architecture — first, always

The wrong architecture downloads a binary that will not run:

```sh
uname -m
```

| It prints | Arch token |
|---|---|
| `x86_64` | `amd64` |
| `arm64` / `aarch64` | `arm64` |

On Windows (PowerShell): `$env:PROCESSOR_ARCHITECTURE` → `AMD64` or `ARM64`.

## 2. Download and verify

From the [latest release](https://github.com/Sebastian197/korvun/releases/latest),
download the archive for your OS/arch plus `checksums.txt` — substituting the
release version and your arch token in the names, e.g.
`korvun_<VERSION>_linux_<ARCH>.tar.gz` (Windows archives are `.zip`).

Always verify before running:

```sh
# Linux
sha256sum -c checksums.txt --ignore-missing
# macOS (ships shasum, not sha256sum)
shasum -a 256 -c checksums.txt --ignore-missing
# -> korvun_<VERSION>_<os>_<ARCH>.tar.gz: OK
```

`OK` means the download is intact. **Do not proceed on `FAILED`.**

> **Optional, for the extra layer:** every release signs `checksums.txt` with
> keyless [cosign](https://docs.sigstore.dev/) — one signature vouches for
> every artifact, no key to distribute. Download `checksums.txt.sig` and
> `checksums.txt.pem` and run:
>
> ```sh
> cosign verify-blob checksums.txt \
>   --signature checksums.txt.sig \
>   --certificate checksums.txt.pem \
>   --certificate-identity-regexp 'https://github.com/Sebastian197/korvun/.*' \
>   --certificate-oidc-issuer https://token.actions.githubusercontent.com
> # -> Verified OK
> ```

## 3. Extract and run

```sh
tar -xzf korvun_<VERSION>_<os>_<ARCH>.tar.gz
chmod +x korvun
./korvun --version
# -> korvun v<VERSION> (<short-revision>)
sudo install -m755 ./korvun /usr/local/bin/korvun   # optional: put it on PATH
```

On Windows, unzip and run `.\korvun.exe --version` in PowerShell.

> **macOS note:** downloaded in the terminal (`curl`, `gh`), the binary runs
> directly. Downloaded with a browser, macOS quarantines it — clear it once
> with `xattr -d com.apple.quarantine ./korvun`, or right-click → Open.
> **Windows note:** the binary is not code-signed, so SmartScreen may show
> *"Windows protected your PC"* — click **More info → Run anyway**.

## Korvun Desktop (the native app)

The same release also carries **Korvun Desktop** — the full gateway behind a
native window: first-run onboarding, secrets stored in your OS keychain
(managed from the **SECRETOS** card in Ajustes, write-only by design), and
the visual builder embedded. The onboarding's model step speaks both local
Ollama and any **OpenAI-compatible server** (LM Studio, llama.cpp,
OpenRouter…), and it verifies the model you chose actually exists on that
server before the step can close. Same core, same version; the headless
binary stays the way to run Korvun on a server.

| OS | Artifact |
|---|---|
| macOS (universal, Intel + Apple Silicon) | `korvun-desktop_<VERSION>_darwin_universal.dmg` |
| Windows x64 | `korvun-desktop_<VERSION>_windows_amd64-installer.exe` |
| Linux x64 | `korvun-desktop_<VERSION>_linux_amd64.tar.gz` |

The desktop family has its own signed manifest, `checksums-desktop.txt`,
verified exactly like the headless one above.

> **First launch:** these builds are not Apple-notarized and carry no Windows
> certificate (a deliberate, cost-free decision — integrity is covered by the
> cosign signatures). The OS asks once:
>
> - **macOS** — drag Korvun to Applications; on the Gatekeeper block, open
>   **System Settings → Privacy & Security**, click **Open Anyway**, confirm
>   with **Open**. macOS remembers permanently.
> - **Windows** — on SmartScreen, **More info → Run anyway**. The installer
>   fetches the WebView2 runtime if missing.
> - **Linux** — untar and run `./korvun-desktop` (needs WebKitGTK,
>   `libwebkit2gtk-4.1` on current distros); install the bundled `.desktop`
>   launcher for a menu entry.

## Secrets: environment variables, by name

Korvun never takes a secret on the command line or in the config file. The
config names the **environment variable** that holds each secret
(`token_env`, `api_key_env`); Korvun reads the value at boot. A missing
secret is a loud, named boot error — Korvun will not start silently deaf.

```sh
export TELEGRAM_BOT_TOKEN=<your-bot-token>   # the value "token_env" points to
korvun serve --config korvun.local.json
```

## Run as a service (Linux / Raspberry Pi)

The repository ships a **hardened systemd unit** —
[`docs/packaging/korvun.service`](https://github.com/Sebastian197/korvun/blob/master/docs/packaging/korvun.service)
— with a dedicated `korvun` user, a `StateDirectory` for the SQLite database,
and a strict sandbox. Point the config's `storage.path` at
`/var/lib/korvun/korvun.db` and audit with `systemd-analyze security korvun`.

## Updating

Korvun does not update itself and never phones home. Updating is downloading
the newer release the same way (checksum included), replacing the binary, and
restarting. Your config file and your data are separate files — an update
never touches them.

## Next

Get a bot answering from a local model → [Quickstart](/guide/quickstart)
