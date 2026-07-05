# Installing Korvun

Korvun ships as a single self-contained binary (no runtime dependencies, no Go
toolchain needed to run it). Each release on GitHub carries a binary for every
supported OS/arch, a `checksums.txt`, an SBOM, and a cosign signature.

> The repository is **public**, so every release is downloadable by anyone (with
> or without the `gh` CLI). Each release is **verifiable end to end**: the
> `checksums.txt` covers every artifact, it is signed with keyless cosign
> (Sigstore OIDC), and its build provenance is attested — the steps below.

## Supported targets

`linux`, `darwin` (macOS), `windows` — each on `amd64` and `arm64` (a Raspberry
Pi 4/5 64-bit is `linux/arm64`). All built `CGO_ENABLED=0`, so they are static and
portable.

---

## macOS — full walkthrough (validated on Intel hardware)

A complete, copy-paste path from download to `korvun --version` on macOS, using the
real `v0.1.0` release. This is the **install-a-release** path (no Go toolchain, no
building from source). The generic per-step reference for all platforms follows in
[§1–§5](#1-download); Linux and Windows walkthroughs come later.

> **Validated end to end on real hardware** (iMac, Intel `x86_64`, macOS 13):
> `uname -m` → `gh release download` (amd64) → `shasum -c` (`OK`) → *(cosign
> optional, see §4)* → `tar -xzf` + `chmod +x` → *(Gatekeeper step not needed for
> the terminal-download path, see §6)* → `./korvun --version` →
> `korvun v0.1.0 (1676b5371ca7)`.

> The commands below target **`v0.1.0`**. For a newer release, replace the version
> and asset names accordingly.

> **Copy-paste tip:** when you copy a command, do **not** include the shell prompt
> (anything up to and including the `$`). Pasting the prompt makes the shell try to
> run it and fail with `command not found`. Copy only what comes after the `$`.

### 1. Pick your architecture — do this FIRST

Your Mac's CPU decides which binary you download. Getting this wrong downloads a
binary that will not run, so it is step one and there is no skipping it:

```bash
uname -m
```

| `uname -m` prints | Your Mac | Use the asset | Arch token |
|-------------------|----------|---------------|------------|
| `x86_64` | **Intel** | `korvun_0.1.0_darwin_amd64.tar.gz` | **`amd64`** |
| `arm64` | **Apple Silicon** (M1/M2/M3/M4) | `korvun_0.1.0_darwin_arm64.tar.gz` | **`arm64`** |

Throughout the rest of this walkthrough, **substitute your arch token** wherever you
see `<ARCH>`: use `amd64` on Intel, `arm64` on Apple Silicon. (The validated run
above was an Intel Mac, i.e. `amd64`.)

### 2. Download the binary + verification material

Work in a scratch directory so the downloads stay together:

```bash
mkdir -p ~/korvun-install && cd ~/korvun-install
```

**With the GitHub CLI (`gh`)** — replace `<ARCH>` with `amd64` (Intel) or `arm64`
(Apple Silicon):

```bash
gh release download v0.1.0 --repo Sebastian197/korvun \
  --pattern 'korvun_0.1.0_darwin_<ARCH>.tar.gz' \
  --pattern 'checksums.txt' \
  --pattern 'checksums.txt.sig' \
  --pattern 'checksums.txt.pem'
```

**Or without `gh`, with `curl`** (from the real release,
<https://github.com/Sebastian197/korvun/releases/tag/v0.1.0>) — again substituting
`<ARCH>`:

```bash
BASE=https://github.com/Sebastian197/korvun/releases/download/v0.1.0
curl -fLO "$BASE/korvun_0.1.0_darwin_<ARCH>.tar.gz"
curl -fLO "$BASE/checksums.txt"
curl -fLO "$BASE/checksums.txt.sig"
curl -fLO "$BASE/checksums.txt.pem"
```

### 3. Verify the checksum — sufficient for most users

This is the verification that matters for integrity, it needs no extra tools, and it
is the one you should always run. macOS ships `shasum` (there is no `sha256sum`
unless you installed coreutils):

```bash
shasum -a 256 -c checksums.txt --ignore-missing
# -> korvun_0.1.0_darwin_amd64.tar.gz: OK
```

`OK` on your archive line means the download is intact and matches the published
release. **Do not proceed if it prints `FAILED`.** For most users this checksum
check is enough; the cosign step in §4 is an optional extra.

### 4. (Optional, advanced) Verify the signature with cosign

This step proves not just integrity but **origin** — that `checksums.txt` was signed
by Korvun's release workflow. It is **optional** and aimed at users who already have,
or want, [cosign](https://docs.sigstore.dev/). If you just want to install and run,
the checksum in §3 is sufficient; skip to §5.

> **Heads-up (from the real validation run):** cosign is **not** preinstalled, and
> `brew install cosign` is heavy — it pulls a large `homebrew-core` clone (~1.3 GB).
> On the macOS 13 (Tier 3) test machine the `brew install` **failed twice** with a
> `.gitconfig` permissions error (`Operation not permitted`) and cosign never
> installed. So the cosign path below is **not yet validated on our hardware** — the
> command is written from the verified certificate identity and the cosign docs, but
> treat it as unproven until someone completes it on a working cosign install.

Install cosign if you want this layer (and if brew cooperates):

```bash
brew install cosign
```

Verify — the identity below is the **exact** signer of `v0.1.0` (Korvun's release
workflow, read from the real certificate):

```bash
cosign verify-blob checksums.txt \
  --signature checksums.txt.sig \
  --certificate checksums.txt.pem \
  --certificate-identity 'https://github.com/Sebastian197/korvun/.github/workflows/release.yml@refs/tags/v0.1.0' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
# -> Verified OK
```

`Verified OK` means `checksums.txt` — and therefore every artifact it covers — was
built and signed by the trusted GitHub Actions release workflow.

> For a future release, either bump the `@refs/tags/vX.Y.Z` in the identity to the
> new tag, or match any tag with a regexp:
> `--certificate-identity-regexp 'https://github.com/Sebastian197/korvun/\.github/workflows/release\.yml@.*'`.

> **If cosign errors on the certificate** (rather than printing `Verified OK`): the
> `checksums.txt.pem` asset is base64-wrapped PEM, so some cosign versions may need
> it decoded first. Try:
> `base64 -d checksums.txt.pem > checksums.pem`, then re-run with
> `--certificate checksums.pem`. (Whether this is necessary is **unconfirmed** — the
> test machine never got cosign installed, so the base64 behavior was not observed.)

### 5. Extract

Replace `<ARCH>` as in §1 (`amd64` on Intel, `arm64` on Apple Silicon):

```bash
tar -xzf korvun_0.1.0_darwin_<ARCH>.tar.gz
chmod +x korvun
```

The archive contains the `korvun` binary plus `LICENSE` and `README.md`.

### 6. Gatekeeper — only if you downloaded via a browser

If you downloaded with `gh` or `curl` in the terminal (as in §2), macOS does **not**
quarantine the binary and it runs directly — **this was confirmed on the validation
run: `./korvun --version` ran with no Gatekeeper prompt.** In that case, **skip this
step.**

You only need this if you downloaded the archive with a **web browser** (Safari,
Chrome, …), which tags it with `com.apple.quarantine`; macOS then refuses to run the
unsigned binary with *"cannot be opened because the developer cannot be verified."*
Clear the attribute (once):

```bash
xattr -d com.apple.quarantine ./korvun
```

Alternatively, the first time only, reveal it in Finder, **right-click → Open**, and
confirm — after which macOS remembers your choice.

### 7. Confirm it runs

```bash
./korvun --version
# -> korvun v0.1.0 (1676b5371ca7)
```

That exact output is what the validation run produced. Optionally put it on your
`PATH`:

```bash
sudo install -m755 ./korvun /usr/local/bin/korvun
korvun --version
```

### 8. Next: zero to a message answered

Installation is done. To wire a config, export the bot token by environment
variable, start Korvun, and get a reply back from your Telegram bot, follow
[`../QUICKSTART.md`](../QUICKSTART.md) (minimal config + `export` the token by name +
run + message the bot).

---

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

### Verify the signature (recommended)

Every release signs `checksums.txt` with keyless [cosign](https://docs.sigstore.dev/)
(Sigstore OIDC) — one signature transitively vouches for every artifact, and there
is no signing key to distribute or trust. Download `checksums.txt.sig` and
`checksums.txt.pem` alongside `checksums.txt`, then verify the signature was
produced by Korvun's release workflow:

```bash
cosign verify-blob checksums.txt \
  --signature checksums.txt.sig \
  --certificate checksums.txt.pem \
  --certificate-identity-regexp 'https://github.com/Sebastian197/korvun/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

A `Verified OK` means the checksum file — and therefore every artifact it lists —
was built and signed by the trusted GitHub Actions release workflow.

> The `checksums.txt.pem` asset is base64-encoded. `cosign verify-blob` decodes it
> transparently (the command above just works); you only need to `base64 -d` it
> first if you want to inspect the certificate directly with `openssl x509`.

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

See [`korvun.service`](./korvun.service) for a **hardened** systemd unit (dedicated
`korvun` system user, `StateDirectory` for the SQLite database, `ProtectSystem=strict`,
`NoNewPrivileges`, an empty capability set, and `SystemCallFilter=@system-service`)
and step-by-step setup. Point the config's `storage.path` at
`/var/lib/korvun/korvun.db` so the database lives in the state directory systemd
creates and owns. Audit the sandbox with `systemd-analyze security korvun`.
