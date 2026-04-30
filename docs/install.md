# Install Agora CLI

This page lists the supported installation paths for Agora CLI and the direct installers for macOS, Linux, and Windows.

## Direct Installers

### macOS, Linux, and Windows POSIX shells

Install the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh -s -- --add-to-path
agora --help
```

Install a pinned version:

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh -s -- --version 0.1.10 --add-to-path
agora --help
```

Install to a user-writable directory and let the installer add it to your shell rc:

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh \
  | INSTALL_DIR="$HOME/.local/bin" sh -s -- --add-to-path
agora --help
```

Run a dry run before installing:

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh -s -- --dry-run
```

The shell installer supports macOS, Linux, and Windows POSIX shells such as Git Bash, MSYS2, and Cygwin. On macOS and Linux, the default install directory is `/usr/local/bin`; when that directory requires elevation and `sudo` is unavailable in the current shell, the installer falls back to a user-writable directory such as `$HOME/.local/bin`. On Windows POSIX shells, the default install directory is `$HOME/bin` and the installed binary is `agora.exe`.

The shell installer is idempotent. Re-running with the same `--version` will detect the existing install at the target install directory and exit successfully without re-downloading. Pass `--force` to reinstall.

### Windows (PowerShell)

Install the latest release:

```powershell
irm https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1 | iex
agora --help
```

Install a pinned version and add the default install directory to your user PATH:

```powershell
$env:VERSION = "0.1.10"
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1))) -AddToPath
agora --help
```

The Windows installer installs `agora.exe` into `%LOCALAPPDATA%\Programs\Agora\bin` by default.

If your PowerShell execution policy blocks inline scripts, download `install.ps1` first and run it with `powershell -ExecutionPolicy Bypass -File .\install.ps1`.

## Unix Installer Flags

```text
--version VERSION       Install a specific version (with or without leading 'v').
--dir INSTALL_DIR       Install directory (default: /usr/local/bin on macOS/Linux,
                        $HOME/bin on Windows POSIX shells).
--prerelease            Resolve latest including GitHub prereleases.
--list-versions         Print recent published versions and exit.
--force                 Reinstall even if the requested version is present, or
                        proceed past an existing managed install warning.
--add-to-path           Append INSTALL_DIR to your shell rc file (bash, zsh,
                        fish, or .profile).
--dry-run               Show what would happen without writing any files.
--uninstall             Remove the installer-managed binary and receipt.
--no-color              Disable colored output.
-q, --quiet             Suppress non-error output.
-v, --verbose           Verbose debug output.
--installer-version     Print this installer's revision and exit.
-h, --help              Show full help.
```

If another managed `agora` install is detected, the installer refuses by default to avoid creating two installs that shadow each other on PATH. Pass `--force` to install alongside it.

## Uninstall

Direct installer installs can be removed with:

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh -s -- --uninstall
```

On Windows PowerShell:

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1))) -Uninstall
```

Uninstall removes the binary and `agora.install.json` receipt from the install directory. It preserves config, session, context, and logs.

## Supported Environment Variables

Both direct installers support these core overrides:

- `GITHUB_REPO`: install from a fork or alternate repository.
- `VERSION`: install a specific version. Both `0.1.10` and `v0.1.10` are accepted.
- `INSTALL_DIR`: install to a custom directory.
- `GITHUB_TOKEN` or `GH_TOKEN`: optional GitHub token to avoid API rate limits when resolving the latest release.

Shell installer only:

- `NO_COLOR`: any non-empty value disables colored output.
- `SUDO`: command for privileged installs (default `sudo`; set to `doas`, `sudo -n`, or empty to disable elevation).
- `DOCS_URL`: alternate docs URL printed in the next-steps footer.
- `ISSUES_URL`: alternate issues URL printed in error messages.

Advanced or test overrides supported by both direct installers:

- `GITHUB_API_URL`: alternate API base URL.
- `RELEASES_DOWNLOAD_BASE_URL`: alternate release download base URL.
- `RELEASES_PAGE_URL`: alternate releases page URL used in error messages.

## Exit Codes

Both `install.sh` and `install.ps1` use the same stable exit-code contract for scripted callers:

| Code | Meaning                                                                                                                        |
| ---- | ------------------------------------------------------------------------------------------------------------------------------ |
| 0    | success (or already at target version on idempotent re-run)                                                                    |
| 1    | generic / unknown error                                                                                                        |
| 2    | invalid arguments                                                                                                              |
| 3    | missing prerequisite (`curl`/`wget`, `tar`/`unzip`, `sha256sum`, ... — Unix only)                                              |
| 4    | unsupported platform / architecture                                                                                            |
| 5    | network or download failure                                                                                                    |
| 6    | checksum verification failed                                                                                                   |
| 7    | install or permission failure (non-writable dir, sudo, or refused to overwrite a managed install without `--force` / `-Force`) |
| 8    | post-install verification failed                                                                                               |

### Idempotent re-runs

Both installers short-circuit with exit `0` when the target install path already contains the target version. Pass `--force` (Unix) or `-Force` (PowerShell) to reinstall anyway.

### Managed-install detection

Both installers refuse to overwrite an `agora` binary that came from a package manager and exit `7` with a recommended upgrade command. Override with `--force` / `-Force`.

| Manager                | Detected by                                                  | Recommended upgrade         |
| ---------------------- | ------------------------------------------------------------ | --------------------------- |
| Homebrew (Unix)        | binary path under `brew --prefix`                            | `brew upgrade agora`        |
| npm (Unix and Windows) | binary path under `npm prefix -g`                            | `npm update -g agoraio-cli` |
| Scoop (Windows)        | `$env:SCOOP` or path contains `\scoop\shims\`                | `scoop update agora`        |
| Chocolatey (Windows)   | `$env:ChocolateyInstall` or path contains `\chocolatey\bin\` | `choco upgrade agora`       |
| winget (Windows)       | path contains `\WinGet\Packages\`                            | `winget upgrade Agora.Cli`  |

### Install receipt and upgrades

Direct installer runs (`install.sh` and `install.ps1`) write `agora.install.json` next to the installed binary after the binary has been downloaded, checksum-verified, installed, and smoke-tested. Direct self-updates refresh the same receipt after replacing the binary. The receipt records the install method, install path, version, timestamp, and installer source so `agora upgrade` can choose the right update path without relying on shell environment variables.

`agora upgrade` uses this order:

1. Read and validate the adjacent `agora.install.json` receipt.
2. Fall back to the resolved binary path for package-manager installs (`node_modules`, Homebrew `Cellar`, Scoop, Chocolatey, or winget paths).
3. Fall back to the direct installer path when the binary is named `agora` / `agora.exe` and no package-manager path is detected.
4. Report `unknown` for development/test binaries where the install method cannot be verified.

Direct-installer installs self-update in place. Package-manager installs print the package-manager command and exit successfully so the package manager remains the owner of the installed files.

## Build From Source

Requirements:

- Go toolchain from `go.mod`
- `git`

```bash
go build -o agora .
./agora --help
```

## Package Channels

| Channel                                  | Status                                              | Command                                                                                              |
| ---------------------------------------- | --------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Shell installer                          | Available                                           | `curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh \| sh -s -- --add-to-path` |
| Windows PowerShell                       | Available                                           | `irm https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1 \| iex`                          |
| Linux `.deb` / `.rpm` / `.apk` artifacts | Available on GitHub releases                        | Download the package for your distro from the release page.                                          |
| apt repository                           | Available when `apt-repo.yml` publishes the release | Use the signed repository documented by the release.                                                 |
| Docker / GHCR                            | Available when release images publish               | `docker run --rm ghcr.io/agoraio/agora-cli:latest --help`                                            |
| npm wrapper                              | Available                                           | `npm install -g agoraio-cli`                                                                         |
| Homebrew / Scoop                         | Coming soon                                         | Use the direct installer until package-manager taps are published.                                   |

### npm wrapper

The `agoraio-cli` npm package is a thin Node.js shim that resolves the right native binary for your platform via `optionalDependencies`. The platform binary itself is the same artifact published to GitHub Releases. Each release is published with [npm provenance](https://docs.npmjs.com/generating-provenance-statements) so consumers can verify the package was built from this repository's release workflow.

```bash
# Install globally
npm install -g agoraio-cli
agora --help

# Or run without a global install
npx agoraio-cli --help

# Pin a specific version
npm install -g agoraio-cli@0.1.10

# Update to the latest published version
npm update -g agoraio-cli
```

Supported platforms: `darwin-arm64`, `darwin-x64`, `linux-arm64`, `linux-x64`, `win32-x64`, `win32-arm64`. Node 18 or newer is required. If you see "platform package not installed," run `npm install -g agoraio-cli` again.

## Shell Completion

Generate completion scripts with Cobra's built-in command:

```bash
agora completion bash
agora completion zsh
agora completion fish
agora completion powershell
```

For one-off shell sessions, source the generated script according to your shell's completion setup.

## Troubleshooting

### GitHub API rate limits

If latest-version resolution fails, retry with a pinned version or provide `GITHUB_TOKEN` / `GH_TOKEN`:

```bash
GITHUB_TOKEN=your-token-here VERSION=0.1.10 sh install.sh
```

```powershell
$env:GITHUB_TOKEN = "your-token-here"
$env:VERSION = "0.1.10"
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1)))
```

### Permission errors

- On macOS and Linux, prefer `INSTALL_DIR="$HOME/.local/bin"` if you do not want `sudo`. The installer refuses to prompt for `sudo` when `stdin` is not a TTY (the typical `curl | sh` case) and falls back to a user-writable default when possible.
- On Windows, choose a writable `-InstallDir` or run PowerShell elevated if you are installing into a system directory.

### "Detected managed install"

The shell installer refuses to install over an existing managed `agora` to avoid creating two installs that shadow each other on PATH. Either:

- Keep using the existing install, or
- Re-run the installer with `--force` to install alongside it.

### PATH issues

If `agora` installs successfully but is not found:

- macOS, Linux, and Windows POSIX shells: re-run with `--add-to-path` to update your shell rc automatically, or add `INSTALL_DIR` to your shell profile manually, for example `export PATH="$HOME/.local/bin:$PATH"`.
- Windows: rerun `install.ps1 -AddToPath` or add `%LOCALAPPDATA%\Programs\Agora\bin` to your user PATH manually, then open a new terminal.

### Checksum failures

The installers verify release artifacts against the published `checksums.txt`. If checksum verification fails, the installer prints the expected and actual SHA256 and exits with code `6`. Do not continue with the install. Retry the download, confirm the requested version exists on the GitHub release, and check whether a proxy or cache is rewriting downloads.

### Proxies and restricted networks

The installers rely on your platform's normal HTTP proxy settings. If downloads fail behind a corporate proxy, retry with the appropriate proxy environment configured and prefer a pinned `VERSION`. The Unix installer enables `curl --retry 3 --retry-connrefused` with sane connect and total timeouts by default.

## Security

The shell installer:

- Restricts `curl` to `--proto =https --tlsv1.2`, refusing plain HTTP and legacy TLS when `curl` is used.
- Verifies every artifact against the published `checksums.txt` before installing.
- Installs atomically: the binary is written to a temp path inside `INSTALL_DIR` and renamed only after extraction and checksum verification succeed. Interrupted runs leave no partial binary behind.

For CI, automation, and reproducible environments, pin `VERSION` explicitly instead of relying on the latest release lookup.

### Verify release artifacts (Cosign + SBOM)

Every release is signed with [Cosign](https://docs.sigstore.dev/cosign/overview/) using GitHub Actions OIDC (keyless mode) and ships an [SPDX 2.3](https://spdx.dev/) SBOM per archive and per Linux package. To verify the `checksums.txt` file before trusting any artifact:

```bash
TAG=v0.1.10
ASSET_BASE="https://github.com/AgoraIO/cli/releases/download/${TAG}"
curl -fsSLO "${ASSET_BASE}/checksums.txt"
curl -fsSLO "${ASSET_BASE}/checksums.txt.sig"
curl -fsSLO "${ASSET_BASE}/checksums.txt.pem"

cosign verify-blob \
  --certificate-identity-regexp '^https://github.com/AgoraIO/cli/' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  checksums.txt
```

Once `checksums.txt` is verified, the existing SHA-256 entries inside it transitively cover every release archive.

The published Docker images are also signed:

```bash
cosign verify "ghcr.io/agoraio/agora-cli:${TAG#v}" \
  --certificate-identity-regexp '^https://github.com/AgoraIO/cli/' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
```

To audit dependencies, download the `*.spdx.json` SBOM that ships next to each archive (e.g. `agora-cli-go_v0.1.10_linux_amd64.tar.gz.spdx.json`) and feed it to a scanner such as [Grype](https://github.com/anchore/grype):

```bash
grype sbom:agora-cli-go_v0.1.10_linux_amd64.tar.gz.spdx.json
```
