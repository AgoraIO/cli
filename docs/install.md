# Install Agora CLI Go

This page lists the supported installation paths for Agora CLI and the direct installers for macOS, Linux, and Windows.

## Direct Installers

### macOS, Linux, and Windows POSIX shells

Install the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO-Community/cli/main/install.sh | sh -s -- --add-to-path
agora --help
```

Install a pinned version:

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO-Community/cli/main/install.sh | sh -s -- --version 0.1.4
agora --help
```

Install to a user-writable directory and let the installer add it to your shell rc:

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO-Community/cli/main/install.sh \
  | INSTALL_DIR="$HOME/.local/bin" sh -s -- --add-to-path
agora --help
```

Run a dry run before installing:

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO-Community/cli/main/install.sh | sh -s -- --dry-run
```

The shell installer supports macOS, Linux, and Windows POSIX shells such as Git Bash, MSYS2, and Cygwin. On macOS and Linux, the default install directory is `/usr/local/bin`; when that directory requires elevation and `sudo` is unavailable in the current shell, the installer falls back to a user-writable directory such as `$HOME/.local/bin`. On Windows POSIX shells, the default install directory is `$HOME/bin` and the installed binary is `agora.exe`.

The shell installer is idempotent. Re-running with the same `--version` will detect the existing install at the target install directory and exit successfully without re-downloading. Pass `--force` to reinstall.

### Windows (PowerShell)

Install the latest release:

```powershell
irm https://raw.githubusercontent.com/AgoraIO-Community/cli/main/install.ps1 | iex
agora --help
```

Install a pinned version and add the default install directory to your user PATH:

```powershell
$env:VERSION = "0.1.4"
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/AgoraIO-Community/cli/main/install.ps1))) -AddToPath
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
--no-color              Disable colored output.
-q, --quiet             Suppress non-error output.
-v, --verbose           Verbose debug output.
--installer-version     Print this installer's revision and exit.
-h, --help              Show full help.
```

If another managed `agora` install is detected, the installer refuses by default to avoid creating two installs that shadow each other on PATH. Pass `--force` to install alongside it.

## Supported Environment Variables

Both direct installers support these core overrides:

- `GITHUB_REPO`: install from a fork or alternate repository.
- `VERSION`: install a specific version. Both `0.1.4` and `v0.1.4` are accepted.
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

## Exit Codes (Unix Installer)

The Unix installer uses a stable exit-code contract for scripted callers:

| Code | Meaning                                                               |
| ---- | --------------------------------------------------------------------- |
| 0    | success                                                               |
| 1    | generic / unknown error                                               |
| 2    | invalid arguments                                                     |
| 3    | missing prerequisite (`curl`/`wget`, `tar`/`unzip`, `sha256sum`, ...) |
| 4    | unsupported platform / architecture                                   |
| 5    | network or download failure                                           |
| 6    | checksum verification failed                                          |
| 7    | install or permission failure (non-writable dir, sudo)                |
| 8    | post-install verification failed                                      |

## Build From Source

Requirements:

- Go toolchain from `go.mod`
- `git`

```bash
go build -o agora .
./agora --help
```

## Troubleshooting

### GitHub API rate limits

If latest-version resolution fails, retry with a pinned version or provide `GITHUB_TOKEN` / `GH_TOKEN`:

```bash
GITHUB_TOKEN=your-token-here VERSION=0.1.4 sh install.sh
```

```powershell
$env:GITHUB_TOKEN = "your-token-here"
$env:VERSION = "0.1.4"
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/AgoraIO-Community/cli/main/install.ps1)))
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
