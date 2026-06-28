# Installing Korvun

Korvun ships as a single self-contained binary (no runtime dependencies, no Go
toolchain needed to run it). Each release on GitHub carries a binary for every
supported OS/arch, a `checksums.txt`, and an SBOM.

> **Note (Stage 15).** While the repository is private, releases are downloadable
> only by the owner and authenticated collaborators (`gh release download`). The
> public download story lands in Stage 16 when the repo goes public.

## Supported targets

`linux`, `darwin` (macOS), `windows` — each on `amd64` and `arm64` (a Raspberry
Pi 4/5 64-bit is `linux/arm64`). All built `CGO_ENABLED=0`, so they are static and
portable.

## 1. Download

Pick the archive for your OS/arch from the release page (or with the `gh` CLI):

```bash
# Example: Linux arm64 (Raspberry Pi 64-bit). Replace VERSION + target as needed.
gh release download VERSION --pattern 'korvun_*_linux_arm64.tar.gz'
gh release download VERSION --pattern 'checksums.txt'
```

Unix archives are `.tar.gz`; Windows archives are `.zip`. Each archive contains the
`korvun` binary plus `LICENSE` and `README.md`.

## 2. Verify the checksum

Always verify the download against `checksums.txt` before running it:

```bash
# Linux / macOS
sha256sum -c checksums.txt --ignore-missing
# macOS without coreutils:
shasum -a 256 -c checksums.txt --ignore-missing
```

```powershell
# Windows (PowerShell): compare the printed hash against the matching line in checksums.txt
Get-FileHash .\korvun_VERSION_windows_amd64.zip -Algorithm SHA256
```

## 3. Extract and place the binary

```bash
tar -xzf korvun_*_linux_arm64.tar.gz
chmod +x korvun                # macOS/Linux: ensure it is executable
sudo mv korvun /usr/local/bin/ # optional: put it on PATH
```

On Windows, unzip the archive and place `korvun.exe` somewhere on your `PATH`.

Confirm it runs:

```bash
korvun --version
# -> korvun vX.Y.Z (<short-revision>)
```

## 4. Configure

Korvun reads one JSON config file, selected with `-config`:

```bash
korvun -config /etc/korvun/korvun.json
```

Start from a profile (see `configs/`):

- **`configs/edge.json`** — Raspberry Pi / small box: one local Ollama model,
  durable memory on, `sensitivity: private` (dispatch stays local-only).
- **`configs/cloud.json`** — server / VM: a wider fan-out across local Ollama + a
  cloud Groq model, durable memory on, observability on loopback.

Copy one, adjust models/policy, and point `-config` at it.

### Secrets are environment variables, by NAME

Korvun never takes a secret on the command line or in the config file. The config
names the **environment variable** that holds each secret (`token_env`,
`api_key_env`); Korvun reads the value from the environment at boot. Export them
before starting — never inline them:

```bash
export TELEGRAM_BOT_TOKEN=...   # the value the config's "token_env" points to
export GROQ_API_KEY=...         # only if a Groq model is configured ("api_key_env")
korvun -config configs/cloud.json
```

A missing secret is a loud, named boot error — Korvun will not start silently
deaf.

## 5. Run as a service (Linux / Raspberry Pi)

See [`korvun.service`](./korvun.service) for a basic systemd unit and how to use
it.
