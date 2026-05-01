#!/usr/bin/env sh
# Agora CLI installer for macOS, Linux, and Windows POSIX shells.
#
# Quick start:
#   curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh
#
# Pin a version:
#   curl -fsSL .../install.sh | sh -s -- --version 0.1.4
#
# User-writable install:
#   curl -fsSL .../install.sh | INSTALL_DIR="$HOME/.local/bin" sh
#
# Discover all options:
#   sh install.sh --help

set -eu
LC_ALL=C
LANG=C
export LC_ALL LANG

if (set -o pipefail) >/dev/null 2>&1; then
  set -o pipefail
fi

INSTALLER_VERSION="2026.04.27"
INSTALL_RECEIPT_FILE="agora.install.json"

# ---- Defaults --------------------------------------------------------------
GITHUB_REPO="${GITHUB_REPO:-AgoraIO/cli}"
INSTALL_DIR_EXPLICIT=0
if [ -n "${INSTALL_DIR+x}" ] && [ -n "${INSTALL_DIR-}" ]; then
  INSTALL_DIR_EXPLICIT=1
fi
INSTALL_DIR="${INSTALL_DIR-}"
VERSION="${VERSION:-}"
SUDO="${SUDO:-sudo}"
GITHUB_API_URL="${GITHUB_API_URL:-https://api.github.com}"
RELEASES_DOWNLOAD_BASE_URL="${RELEASES_DOWNLOAD_BASE_URL:-https://github.com/${GITHUB_REPO}/releases/download}"
RELEASES_PAGE_URL="${RELEASES_PAGE_URL:-https://github.com/${GITHUB_REPO}/releases}"
DOCS_URL="${DOCS_URL:-https://github.com/${GITHUB_REPO}#readme}"
ISSUES_URL="${ISSUES_URL:-https://github.com/${GITHUB_REPO}/issues}"
AUTH_TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
NO_COLOR_ENV="${NO_COLOR:-}"

# ---- Mode flags ------------------------------------------------------------
DRY_RUN=0
FORCE=0
LIST_VERSIONS=0
PRERELEASE=0
QUIET=0
VERBOSE=0
NO_COLOR_FLAG=0
UNINSTALL=0
# Shell-integration opt-outs. Default behavior matches modern installers
# (bun, fnm, deno, uv, volta): auto-wire PATH and shell completion for
# the detected $SHELL. Granular opt-outs let users decouple each piece;
# --skip-shell is the umbrella that disables both.
NO_PATH=0
NO_COMPLETION=0
SKIP_SHELL=0

# ---- Exit codes ------------------------------------------------------------
EXIT_OK=0
EXIT_GENERIC=1
EXIT_USAGE=2
EXIT_MISSING_PREREQ=3
EXIT_UNSUPPORTED=4
EXIT_NETWORK=5
EXIT_CHECKSUM=6
EXIT_INSTALL=7
EXIT_VERIFY=8

# ---- Mutable state set during run ------------------------------------------
TMP=""
USE_SUDO=0
TEMP_DESTINATION=""
MANAGED_INSTALL=""
MANAGED_PATH=""
MANAGED_UPGRADE_CMD=""
OS=""
ARCH=""
ARCHIVE_EXT=""
BINARY_NAME=""
DOWNLOAD_TOOL=""

# Color codes (initialized in init_colors).
BOLD=""
DIM=""
RED=""
YELLOW=""
GREEN=""
BLUE=""
RESET=""

# ---- Color and logging -----------------------------------------------------
init_colors() {
  if [ -n "$NO_COLOR_ENV" ]; then
    return 0
  fi
  if ! [ -t 1 ]; then
    return 0
  fi
  BOLD=$(printf '\033[1m')
  DIM=$(printf '\033[2m')
  RED=$(printf '\033[31m')
  YELLOW=$(printf '\033[33m')
  GREEN=$(printf '\033[32m')
  BLUE=$(printf '\033[34m')
  RESET=$(printf '\033[0m')
}

reset_colors() {
  BOLD=""
  DIM=""
  RED=""
  YELLOW=""
  GREEN=""
  BLUE=""
  RESET=""
}

say() {
  if [ "$QUIET" = "1" ]; then
    return 0
  fi
  printf '%s\n' "$*"
}

say_step() {
  if [ "$QUIET" = "1" ]; then
    return 0
  fi
  printf '%s==>%s %s\n' "$BLUE" "$RESET" "$*"
}

say_ok() {
  if [ "$QUIET" = "1" ]; then
    return 0
  fi
  printf '    %s%s%s\n' "$DIM" "$*" "$RESET"
}

warn() {
  printf '%sWarning:%s %s\n' "$YELLOW" "$RESET" "$*" >&2
}

err() {
  printf '%sError:%s %s\n' "$RED" "$RESET" "$*" >&2
}

verbose() {
  if [ "$VERBOSE" != "1" ]; then
    return 0
  fi
  printf '%s[debug]%s %s\n' "$DIM" "$RESET" "$*" >&2
}

die() {
  err "$1"
  exit "${2:-$EXIT_GENERIC}"
}

# ---- Cleanup / trap --------------------------------------------------------
cleanup() {
  if [ -n "$TMP" ] && [ -d "$TMP" ]; then
    rm -rf "$TMP" 2>/dev/null || true
  fi
  if [ -n "$TEMP_DESTINATION" ] && [ -e "$TEMP_DESTINATION" ]; then
    if [ "$USE_SUDO" = "1" ]; then
      run_elevated rm -f "$TEMP_DESTINATION" 2>/dev/null || true
    else
      rm -f "$TEMP_DESTINATION" 2>/dev/null || true
    fi
  fi
}

# ---- Help ------------------------------------------------------------------
usage() {
  cat <<EOF
${BOLD}Agora CLI installer${RESET} ${DIM}(rev ${INSTALLER_VERSION})${RESET}

Install the Agora CLI on macOS, Linux, or Windows (Git Bash / MSYS2 / Cygwin).

${BOLD}Usage:${RESET}
  sh install.sh [options]

${BOLD}Options:${RESET}
  --version VERSION       Install a specific version (with or without leading 'v').
  --dir INSTALL_DIR       Install directory (default: /usr/local/bin on macOS/Linux,
                          \$HOME/bin on Windows POSIX shells).
  --prerelease            Resolve latest including prereleases.
  --list-versions         Print recent published versions and exit.
  --force                 Reinstall even if the requested version is already present,
                          or proceed past a Homebrew/npm-managed install warning.

${BOLD}Shell integration${RESET} ${DIM}(auto-on; pass an opt-out flag to disable)${RESET}
  --no-path               Don't append the install directory to your shell rc file.
                          The script will still print manual PATH instructions if
                          'agora' is not already resolvable on PATH.
  --no-completion         Don't install shell completion. The script will still
                          print the manual 'agora completion <shell>' command.
  --skip-shell            Umbrella for --no-path --no-completion. Install the
                          binary only and skip every shell modification.

${BOLD}Other:${RESET}
  --dry-run               Show what would happen without making changes.
  --uninstall             Remove the installer-managed binary and receipt.
  --no-color              Disable colored output.
  -q, --quiet             Suppress non-error output.
  -v, --verbose           Verbose debug output (installer-internal; unrelated to
                          'agora --debug').
  --installer-version     Print this installer's revision and exit.
  -h, --help              Show this help.

${BOLD}Environment:${RESET}
  GITHUB_REPO                 GitHub repository (default: ${GITHUB_REPO}).
  INSTALL_DIR                 Install directory (default: platform-specific).
  VERSION                     Version to install when --version is omitted.
  GITHUB_TOKEN / GH_TOKEN     Optional token to avoid GitHub API rate limits.
  SUDO                        Command for privileged installs (default: ${SUDO}).
  NO_COLOR                    Disable colored output (any non-empty value).
  GITHUB_API_URL              Override GitHub API base URL.
  RELEASES_DOWNLOAD_BASE_URL  Override release download base URL.
  RELEASES_PAGE_URL           Override release page URL (used in messages).
  DOCS_URL                    Override docs URL (used in next-steps footer).
  ISSUES_URL                  Override issues URL (used in error messages).

${BOLD}Exit codes:${RESET}
  ${EXIT_OK}  success
  ${EXIT_GENERIC}  generic / unknown error
  ${EXIT_USAGE}  invalid arguments
  ${EXIT_MISSING_PREREQ}  missing prerequisite (curl/wget, tar/unzip, sha256, ...)
  ${EXIT_UNSUPPORTED}  unsupported platform / architecture
  ${EXIT_NETWORK}  network or download failure
  ${EXIT_CHECKSUM}  checksum verification failed
  ${EXIT_INSTALL}  install / permission failure
  ${EXIT_VERIFY}  post-install verification failed

Docs:   ${DOCS_URL}
Issues: ${ISSUES_URL}
EOF
}

# ---- Argument parsing ------------------------------------------------------
parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --version)
        if [ $# -lt 2 ]; then
          die "Missing value for --version" "$EXIT_USAGE"
        fi
        VERSION=$2
        shift 2
        ;;
      --version=*)
        VERSION=${1#--version=}
        shift
        ;;
      --dir)
        if [ $# -lt 2 ]; then
          die "Missing value for --dir" "$EXIT_USAGE"
        fi
        INSTALL_DIR=$2
        INSTALL_DIR_EXPLICIT=1
        shift 2
        ;;
      --dir=*)
        INSTALL_DIR=${1#--dir=}
        INSTALL_DIR_EXPLICIT=1
        shift
        ;;
      --prerelease)
        PRERELEASE=1
        shift
        ;;
      --list-versions)
        LIST_VERSIONS=1
        shift
        ;;
      --force)
        FORCE=1
        shift
        ;;
      --no-path)
        NO_PATH=1
        shift
        ;;
      --no-completion)
        NO_COMPLETION=1
        shift
        ;;
      --skip-shell)
        SKIP_SHELL=1
        shift
        ;;
      --dry-run)
        DRY_RUN=1
        shift
        ;;
      --uninstall)
        UNINSTALL=1
        shift
        ;;
      --no-color)
        NO_COLOR_FLAG=1
        shift
        ;;
      -q|--quiet)
        QUIET=1
        shift
        ;;
      -v|--verbose)
        VERBOSE=1
        shift
        ;;
      --installer-version)
        printf '%s\n' "$INSTALLER_VERSION"
        exit "$EXIT_OK"
        ;;
      -h|--help)
        usage
        exit "$EXIT_OK"
        ;;
      *)
        err "Unknown option: $1"
        say "Run with --help for usage."
        exit "$EXIT_USAGE"
        ;;
    esac
  done
}

# ---- Uninstall --------------------------------------------------------------
uninstall() {
  ensure_install_dir_default
  binary_path="${INSTALL_DIR}/${BINARY_NAME}"
  receipt_path="${INSTALL_DIR}/${INSTALL_RECEIPT_FILE}"
  say_step "Uninstalling Agora CLI from ${INSTALL_DIR}"
  if [ "$DRY_RUN" = "1" ]; then
    say "[dry-run] Would remove ${binary_path}"
    say "[dry-run] Would remove ${receipt_path}"
    return 0
  fi
  if [ -e "$binary_path" ]; then
    rm -f "$binary_path" 2>/dev/null || run_elevated rm -f "$binary_path"
    say_ok "Removed ${binary_path}"
  else
    say "No agora binary found at ${binary_path}."
  fi
  if [ -e "$receipt_path" ]; then
    rm -f "$receipt_path" 2>/dev/null || run_elevated rm -f "$receipt_path"
    say_ok "Removed ${receipt_path}"
  fi
  say "Config, session, context, and logs are preserved under the Agora CLI config directory."
}

# ---- Helpers ---------------------------------------------------------------
need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "Missing required command: $1" "$EXIT_MISSING_PREREQ"
  fi
}

normalize_version() {
  VERSION=$(printf '%s' "$VERSION" | sed 's/^v//')
}

platform_default_install_dir() {
  case "$OS" in
    windows) printf '%s\n' "$HOME/bin" ;;
    *) printf '%s\n' "/usr/local/bin" ;;
  esac
}

platform_fallback_install_dir() {
  case "$OS" in
    windows) printf '%s\n' "$HOME/bin" ;;
    *)
      if [ -d "$HOME/.local" ] || [ ! -e "$HOME/.local" ]; then
        printf '%s\n' "$HOME/.local/bin"
      else
        printf '%s\n' "$HOME/bin"
      fi
      ;;
  esac
}

ensure_install_dir_default() {
  if [ -z "$INSTALL_DIR" ]; then
    INSTALL_DIR=$(platform_default_install_dir)
  fi
}

path_starts_with() {
  case "$1" in
    "$2") return 0 ;;
    "$2"/*) return 0 ;;
    *) return 1 ;;
  esac
}

run_with_timeout() {
  if command -v timeout >/dev/null 2>&1; then
    timeout 3 "$@"
    return $?
  fi
  if command -v gtimeout >/dev/null 2>&1; then
    gtimeout 3 "$@"
    return $?
  fi
  "$@"
}

run_elevated() {
  # SUDO is intentionally word-split to honor SUDO="sudo -n", SUDO="doas", etc.
  # shellcheck disable=SC2086
  $SUDO "$@"
}

detect_downloader() {
  if command -v curl >/dev/null 2>&1; then
    DOWNLOAD_TOOL="curl"
    return 0
  fi
  if command -v wget >/dev/null 2>&1; then
    DOWNLOAD_TOOL="wget"
    return 0
  fi
  die "Missing required command: curl or wget" "$EXIT_MISSING_PREREQ"
}

# ---- Download --------------------------------------------------------------
# TLS hardening defaults. Tests can override INSTALLER_CURL_PROTO_OPTS to allow
# non-HTTPS fixtures (e.g. a local HTTP server). Not intended for end users.
CURL_PROTO_OPTS="${INSTALLER_CURL_PROTO_OPTS:---proto =https --tlsv1.2}"
CURL_RETRY_OPTS="--retry 3 --retry-delay 2 --retry-connrefused --connect-timeout 10 --max-time 300"
curl_common_opts="$CURL_PROTO_OPTS $CURL_RETRY_OPTS -fL"

download_quiet() {
  url=$1
  output=$2
  mode=${3:-download}

  if [ "$DOWNLOAD_TOOL" = "wget" ]; then
    if [ "$mode" = "api" ] && [ -n "$AUTH_TOKEN" ]; then
      wget -q -O "$output" \
        --header='Accept: application/vnd.github+json' \
        --header="Authorization: Bearer $AUTH_TOKEN" \
        "$url"
      return $?
    fi
    if [ "$mode" = "api" ]; then
      wget -q -O "$output" \
        --header='Accept: application/vnd.github+json' \
        "$url"
      return $?
    fi
    wget -q -O "$output" "$url"
    return $?
  fi

  if [ "$mode" = "api" ] && [ -n "$AUTH_TOKEN" ]; then
    # shellcheck disable=SC2086
    curl $curl_common_opts -sS \
      -H 'Accept: application/vnd.github+json' \
      -H "Authorization: Bearer $AUTH_TOKEN" \
      "$url" -o "$output"
    return $?
  fi
  if [ "$mode" = "api" ]; then
    # shellcheck disable=SC2086
    curl $curl_common_opts -sS \
      -H 'Accept: application/vnd.github+json' \
      "$url" -o "$output"
    return $?
  fi
  # shellcheck disable=SC2086
  curl $curl_common_opts -sS "$url" -o "$output"
}

download_archive() {
  url=$1
  output=$2

  if [ "$DOWNLOAD_TOOL" = "wget" ]; then
    if [ -t 1 ] && [ "$QUIET" = "0" ]; then
      wget -O "$output" "$url"
      return $?
    fi
    wget -q -O "$output" "$url"
    return $?
  fi

  if [ -t 1 ] && [ "$QUIET" = "0" ]; then
    # shellcheck disable=SC2086
    curl $curl_common_opts --progress-bar "$url" -o "$output"
    return $?
  fi
  # shellcheck disable=SC2086
  curl $curl_common_opts -sS "$url" -o "$output"
}

download_or_fail() {
  url=$1
  output=$2
  mode=${3:-download}

  verbose "GET $url"
  status=0
  if [ "$mode" = "archive" ]; then
    download_archive "$url" "$output" || status=$?
  else
    download_quiet "$url" "$output" "$mode" || status=$?
  fi
  if [ "$status" = "0" ]; then
    return 0
  fi

  err "Download failed (${DOWNLOAD_TOOL} exit $status): $url"
  warn "Release page: ${RELEASES_PAGE_URL}"
  if [ "$mode" = "api" ]; then
    die "Could not reach the GitHub API. Set --version explicitly, or provide GITHUB_TOKEN / GH_TOKEN if you are hitting rate limits." "$EXIT_NETWORK"
  fi
  die "Network or proxy issue. Re-run with --verbose for details, or pin --version." "$EXIT_NETWORK"
}

# ---- Hashes ----------------------------------------------------------------
sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
    return 0
  fi
  die "Missing required command: sha256sum or shasum" "$EXIT_MISSING_PREREQ"
}

# ---- Filesystem / sudo -----------------------------------------------------
nearest_existing_dir() {
  target=$1
  while [ ! -d "$target" ]; do
    parent=$(dirname "$target")
    if [ "$parent" = "$target" ]; then
      break
    fi
    target=$parent
  done
  printf '%s\n' "$target"
}

user_can_write_install_dir() {
  probe=$(nearest_existing_dir "$INSTALL_DIR")
  [ -w "$probe" ]
}

fallback_to_user_install_dir() {
  if [ "$INSTALL_DIR_EXPLICIT" = "1" ]; then
    return 1
  fi

  default_dir=$(platform_default_install_dir)
  if [ "$INSTALL_DIR" != "$default_dir" ]; then
    return 1
  fi

  fallback_dir=$(platform_fallback_install_dir)
  if [ "$fallback_dir" = "$INSTALL_DIR" ]; then
    return 1
  fi

  warn "Install directory ${INSTALL_DIR} is not writable; falling back to ${fallback_dir}."
  INSTALL_DIR=$fallback_dir
  return 0
}

ensure_install_dir_writable() {
  USE_SUDO=0
  if user_can_write_install_dir; then
    return 0
  fi

  if [ "$OS" = "windows" ]; then
    die "Install directory ${INSTALL_DIR} is not writable. Set INSTALL_DIR to a writable path." "$EXIT_INSTALL"
  fi

  if [ -z "$SUDO" ]; then
    if fallback_to_user_install_dir && user_can_write_install_dir; then
      say "Using user-writable install directory: ${INSTALL_DIR}"
      return 0
    fi
    die "Install directory ${INSTALL_DIR} is not writable. Set INSTALL_DIR to a writable path or set SUDO." "$EXIT_INSTALL"
  fi

  # First word of SUDO must exist on PATH.
  # shellcheck disable=SC2086
  set -- $SUDO
  if [ $# -eq 0 ]; then
    if fallback_to_user_install_dir && user_can_write_install_dir; then
      say "Using user-writable install directory: ${INSTALL_DIR}"
      return 0
    fi
    die "SUDO is empty. Set INSTALL_DIR to a writable path or configure SUDO." "$EXIT_INSTALL"
  fi
  if ! command -v "$1" >/dev/null 2>&1; then
    if fallback_to_user_install_dir && user_can_write_install_dir; then
      say "Using user-writable install directory: ${INSTALL_DIR}"
      return 0
    fi
    die "${1} not found on PATH. Set INSTALL_DIR to a writable path or set SUDO to a different elevation tool." "$EXIT_INSTALL"
  fi

  # When stdin is not a TTY (curl|sh) and we cannot get cached sudo, abort BEFORE
  # downloading so the user is not surprised by a sudo prompt mid-install.
  if ! [ -t 0 ]; then
    if ! run_elevated -n true >/dev/null 2>&1; then
      if fallback_to_user_install_dir && user_can_write_install_dir; then
        say "Using user-writable install directory: ${INSTALL_DIR}"
        return 0
      fi
      err "${INSTALL_DIR} requires elevated permissions, but this shell has no TTY for a sudo prompt."
      say "  Re-run interactively, or use a writable INSTALL_DIR. For example:"
      say "    ${GREEN}curl -fsSL .../install.sh | INSTALL_DIR=\"\$HOME/.local/bin\" sh${RESET}"
      exit "$EXIT_INSTALL"
    fi
  fi

  USE_SUDO=1
}

install_binary() {
  source_bin=$1
  temp_dest=$2
  final_dest=$3

  if [ "$USE_SUDO" = "1" ]; then
    run_elevated mkdir -p "$INSTALL_DIR"
    if command -v install >/dev/null 2>&1; then
      run_elevated install -m 755 "$source_bin" "$temp_dest"
    else
      run_elevated cp "$source_bin" "$temp_dest"
      run_elevated chmod 755 "$temp_dest"
    fi
    run_elevated mv -f "$temp_dest" "$final_dest"
    return
  fi

  mkdir -p "$INSTALL_DIR"
  if command -v install >/dev/null 2>&1; then
    install -m 755 "$source_bin" "$temp_dest"
  else
    cp "$source_bin" "$temp_dest"
    chmod 755 "$temp_dest"
  fi
  mv -f "$temp_dest" "$final_dest"
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

write_install_receipt() {
  final_dest=$1
  receipt_path="${INSTALL_DIR}/${INSTALL_RECEIPT_FILE}"
  receipt_tmp="${TMP}/${INSTALL_RECEIPT_FILE}"
  installed_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

  {
    printf '{\n'
    printf '  "schemaVersion": 1,\n'
    printf '  "tool": "agora",\n'
    printf '  "installMethod": "installer",\n'
    printf '  "installPath": "%s",\n' "$(json_escape "$final_dest")"
    printf '  "version": "%s",\n' "$(json_escape "$VERSION")"
    printf '  "installedAt": "%s",\n' "$(json_escape "$installed_at")"
    printf '  "source": "install.sh"\n'
    printf '}\n'
  } >"$receipt_tmp"

  if [ "$USE_SUDO" = "1" ]; then
    run_elevated cp "$receipt_tmp" "$receipt_path"
    run_elevated chmod 644 "$receipt_path" || true
    return
  fi

  cp "$receipt_tmp" "$receipt_path"
  chmod 644 "$receipt_path" || true
}

extract_archive() {
  archive_path=$1
  if [ "$OS" = "windows" ]; then
    unzip -oq "$archive_path" "$BINARY_NAME" -d "$TMP" || return 1
    return 0
  fi
  tar -xzf "$archive_path" -C "$TMP" "$BINARY_NAME"
}

# ---- Existing install detection --------------------------------------------
extract_installed_version() {
  binary_path=$1
  out=$(run_with_timeout "$binary_path" --version 2>/dev/null || true)
  printf '%s' "$out"
}

show_existing_install() {
  current_path=""
  if ! current_path=$(command -v agora 2>/dev/null); then
    return 0
  fi

  current_version=$(extract_installed_version "$current_path")
  if [ -n "$current_version" ]; then
    say "Existing install: ${current_version} ${DIM}(${current_path})${RESET}"
  else
    say "Existing install detected at ${current_path}"
  fi
}

detect_managed_install() {
  MANAGED_INSTALL=""
  MANAGED_PATH=""
  MANAGED_UPGRADE_CMD=""

  current_path=""
  if ! current_path=$(command -v agora 2>/dev/null); then
    return 0
  fi

  if command -v brew >/dev/null 2>&1; then
    brew_prefix=$(brew --prefix 2>/dev/null || true)
    if [ -n "$brew_prefix" ] && path_starts_with "$current_path" "$brew_prefix"; then
      MANAGED_INSTALL="Homebrew"
      MANAGED_PATH="$current_path"
      MANAGED_UPGRADE_CMD="brew upgrade agora"
      return 0
    fi
  fi

  if command -v npm >/dev/null 2>&1; then
    npm_prefix=$(npm prefix -g 2>/dev/null || true)
    if [ -n "$npm_prefix" ] && path_starts_with "$current_path" "$npm_prefix"; then
      MANAGED_INSTALL="npm"
      MANAGED_PATH="$current_path"
      MANAGED_UPGRADE_CMD="npm update -g agoraio-cli"
      return 0
    fi
  fi
}

guard_managed_install() {
  detect_managed_install
  if [ -z "$MANAGED_INSTALL" ]; then
    return 0
  fi

  if [ "$FORCE" = "1" ]; then
    warn "Detected ${MANAGED_INSTALL}-managed install at ${MANAGED_PATH}. Continuing because --force is set."
    return 0
  fi

  err "Detected ${MANAGED_INSTALL}-managed install at ${MANAGED_PATH}."
  say "  Recommended: ${BOLD}${MANAGED_UPGRADE_CMD}${RESET}"
  say "  Or re-run with ${BOLD}--force${RESET} to install alongside (may shadow the ${MANAGED_INSTALL} install on PATH)."
  exit "$EXIT_INSTALL"
}

verify_installed_binary() {
  binary_path=$1
  if run_with_timeout "$binary_path" --version >/dev/null 2>&1; then
    return 0
  fi
  run_with_timeout "$binary_path" --help >/dev/null 2>&1
}

already_at_target_version() {
  binary_path=$1
  target=$2

  out=$(extract_installed_version "$binary_path")
  if [ -z "$out" ]; then
    return 1
  fi
  case "$out" in
    *"$target"*) return 0 ;;
    *) return 1 ;;
  esac
}

# ---- Version resolution ----------------------------------------------------
first_tag_name_from_json() {
  json_path=$1
  grep -m 1 '"tag_name"' "$json_path" 2>/dev/null | cut -d '"' -f 4 | sed -n '1p' || true
}

resolve_version() {
  if [ "$PRERELEASE" = "1" ]; then
    url="${GITHUB_API_URL%/}/repos/${GITHUB_REPO}/releases?per_page=10"
  else
    url="${GITHUB_API_URL%/}/repos/${GITHUB_REPO}/releases/latest"
  fi
  json="${TMP}/latest.json"
  download_or_fail "$url" "$json" api

  VERSION=$(first_tag_name_from_json "$json")
  VERSION=${VERSION#v}

  if [ -z "$VERSION" ]; then
    die "Could not parse the latest release version. Set --version explicitly." "$EXIT_NETWORK"
  fi
}

list_versions_and_exit() {
  url="${GITHUB_API_URL%/}/repos/${GITHUB_REPO}/releases?per_page=20"
  json="${TMP}/versions.json"
  download_or_fail "$url" "$json" api

  say "Recent ${GITHUB_REPO} releases:"
  grep '"tag_name"' "$json" 2>/dev/null | cut -d '"' -f 4 | sed 's/^/  /'
  exit "$EXIT_OK"
}

# ---- Platform --------------------------------------------------------------
detect_platform() {
  OS=$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m 2>/dev/null)

  case "$OS" in
    darwin|linux) ;;
    msys*|mingw*|cygwin*)
      OS="windows"
      ;;
    *)
      die "Unsupported OS: ${OS}. Try Homebrew, npm, Scoop, or a release package." "$EXIT_UNSUPPORTED"
      ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
      die "Unsupported architecture: ${ARCH}. Supported architectures: amd64 and arm64." "$EXIT_UNSUPPORTED"
      ;;
  esac

  case "$OS" in
    windows)
      ARCHIVE_EXT="zip"
      BINARY_NAME="agora.exe"
      ;;
    *)
      ARCHIVE_EXT="tar.gz"
      BINARY_NAME="agora"
      ;;
  esac

  ensure_install_dir_default
}

# ---- PATH guidance ---------------------------------------------------------
bash_writable_rc() {
  for candidate in \
    "${XDG_CONFIG_HOME:+$XDG_CONFIG_HOME/bash/bashrc}" \
    "$HOME/.bashrc" \
    "$HOME/.bash_profile" \
    "$HOME/.profile"; do
    if [ -z "$candidate" ]; then
      continue
    fi
    if [ -w "$candidate" ] || { [ ! -e "$candidate" ] && [ -w "$(dirname "$candidate")" ]; }; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done

  # Fallback target for manual hints when no candidate can be written.
  printf '%s\n' "$HOME/.bashrc"
  return 0
}

shell_rc_for_path() {
  shell_name=""
  if [ -n "${SHELL:-}" ]; then
    shell_name=$(basename "$SHELL" 2>/dev/null || true)
  fi
  case "$shell_name" in
    bash) bash_writable_rc ;;
    zsh)  printf '%s\n' "$HOME/.zshrc" ;;
    fish) printf '%s\n' "$HOME/.config/fish/config.fish" ;;
    *)    printf '%s\n' "$HOME/.profile" ;;
  esac
}

shell_path_line() {
  shell_name=""
  if [ -n "${SHELL:-}" ]; then
    shell_name=$(basename "$SHELL" 2>/dev/null || true)
  fi
  case "$shell_name" in
    fish) printf 'fish_add_path "%s"\n' "$INSTALL_DIR" ;;
    *)    printf 'export PATH="%s:$PATH"\n' "$INSTALL_DIR" ;;
  esac
}

shell_refresh_command() {
  shell_name=""
  if [ -n "${SHELL:-}" ]; then
    shell_name=$(basename "$SHELL" 2>/dev/null || true)
  fi
  case "$shell_name" in
    fish) printf 'source %s\n' "$(shell_rc_for_path)" ;;
    *)    printf 'exec %s\n' "${SHELL:-/bin/sh}" ;;
  esac
}

# print_path_instructions is the manual-fallback hint shown when the
# user opted out of auto-PATH (--no-path / --skip-shell) and the binary
# is not yet resolvable on PATH. It reuses print_manual_path_block so
# the wording and exact command match what the auto-failed path emits.
print_path_instructions() {
  rcfile=$(shell_rc_for_path)
  line=$(shell_path_line)
  warn "agora is not on your PATH yet."
  print_manual_path_block "$rcfile" "$line"
  say "  ${DIM}(Tip: re-run the installer without --no-path / --skip-shell to do this automatically.)${RESET}"
}

# print_manual_path_block prints a complete, copy-pasteable manual
# PATH-setup block. Used as the body of every PATH failure message so
# the wording, indentation, and example command stay identical across
# the mkdir-failed, write-failed, and explicit-opt-out paths.
print_manual_path_block() {
  rcfile=$1
  line=$2
  say "  agora is installed at ${BOLD}${DESTINATION}${RESET} and is ready to run."
  say "  To add it to your PATH, append this line to a shell rc file you can write to:"
  say ""
  say "    ${GREEN}${line}${RESET}"
  say ""
  say "  Then open a new shell, or ${DIM}source${RESET} the file you edited."
  say "  ${DIM}For other options (custom INSTALL_DIR, containers), see ${DOCS_URL}${RESET}"
}

# add_to_path appends INSTALL_DIR to the user's shell rc file. Best-effort:
# returns 0 on success (added or already present), 1 on any write failure.
# On failure it emits a plain-language branch message followed by a
# copy-pasteable manual block. The caller does NOT need to print any
# additional fallback hints. This mirrors the softer manual-fallback
# style used by bun and uv.
add_to_path() {
  rcfile=$(shell_rc_for_path)
  line=$(shell_path_line)

  if [ -f "$rcfile" ] && grep -qF "$INSTALL_DIR" "$rcfile" 2>/dev/null; then
    say "${INSTALL_DIR} is already referenced in ${rcfile}."
    return 0
  fi

  if [ "$DRY_RUN" = "1" ]; then
    say "[dry-run] Would append to ${rcfile}:"
    say "  ${line}"
    return 0
  fi

  rcdir=$(dirname "$rcfile")
  if ! mkdir -p "$rcdir" 2>/dev/null; then
    say "${rcfile} is not writable, so the installer can't add agora to your PATH automatically."
    print_manual_path_block "$rcfile" "$line"
    return 1
  fi
  # Wrap the redirection in a brace group so the shell's own
  # "Permission denied" message on the redirection (emitted before
  # the command runs, so a bare `>> file 2>/dev/null` does not catch
  # it) is suppressed. We surface only the friendlier warn below.
  if ! { printf '\n# Added by Agora CLI installer\n%s\n' "$line" >> "$rcfile"; } 2>/dev/null; then
    say "${rcfile} is not writable, so the installer can't add agora to your PATH automatically."
    print_manual_path_block "$rcfile" "$line"
    return 1
  fi
  say "Added ${INSTALL_DIR} to PATH in ${rcfile}."
  say "To use agora in this shell now, run:"
  say "    ${GREEN}$(shell_refresh_command)${RESET}"
  say "${DIM}(Or open a new terminal - the change takes effect either way.)${RESET}"
  return 0
}

# detect_user_shell returns "bash", "zsh", "fish", or "" for the user's
# login shell. Reads $SHELL because the installer often runs in a
# subshell that does not match the user's interactive shell.
detect_user_shell() {
  case "${SHELL:-}" in
    */bash) printf 'bash' ;;
    */zsh)  printf 'zsh' ;;
    */fish) printf 'fish' ;;
    *)      printf '' ;;
  esac
}

# completion_target_for_shell prints the user-writable file path the
# completion script should be written to for the given shell. Empty
# output means "no known target", and the caller should print a manual
# hint instead. We deliberately prefer user-owned paths so the
# installer never needs sudo just to wire completion.
completion_target_for_shell() {
  shell_name=$1
  case "$shell_name" in
    bash)
      # bash-completion v2 reads from XDG_DATA_HOME/bash-completion/completions.
      # Falls back to ~/.local/share/... which is the documented default.
      data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
      printf '%s/bash-completion/completions/agora' "$data_home"
      ;;
    zsh)
      # ~/.zsh/completions is appended to fpath in our zsh hint below.
      printf '%s/.zsh/completions/_agora' "$HOME"
      ;;
    fish)
      # fish auto-loads ~/.config/fish/completions/*.fish.
      config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
      printf '%s/fish/completions/agora.fish' "$config_home"
      ;;
    *)
      printf ''
      ;;
  esac
}

# print_completion_instructions tells the user how to enable completion
# manually when they opted out (--no-completion / --skip-shell) or when
# auto-detect failed. Mirrors print_path_instructions for symmetry.
print_completion_instructions() {
  shell_name=$(detect_user_shell)
  case "$shell_name" in
    bash|zsh|fish)
      say "  ${GREEN}agora completion ${shell_name} > $(completion_target_for_shell "$shell_name")${RESET}"
      ;;
    *)
      say "  ${GREEN}agora completion --help${RESET}"
      ;;
  esac
}

# install_completion writes the cobra-generated completion script to a
# user-writable location for the detected shell. Best-effort: failures
# never abort the install, and we always print the manual fallback
# command the user can copy-paste if anything goes wrong. Honors
# NO_COMPLETION and SKIP_SHELL.
install_completion() {
  if [ "$NO_COMPLETION" = "1" ] || [ "$SKIP_SHELL" = "1" ]; then
    return 0
  fi

  shell_name=$(detect_user_shell)
  case "$shell_name" in
    bash|zsh|fish) ;;
    "")
      say "Could not detect your shell from \$SHELL. To enable completion later, run 'agora completion --help'."
      return 0
      ;;
    *)
      say "Shell completion auto-install not supported for ${shell_name}. Run 'agora completion --help' to wire it manually."
      return 0
      ;;
  esac

  target=$(completion_target_for_shell "$shell_name")
  if [ -z "$target" ]; then
    say "No known completion install path for ${shell_name}. Run 'agora completion --help' for manual instructions."
    return 0
  fi

  if [ "$DRY_RUN" = "1" ]; then
    say "[dry-run] Would install ${shell_name} completion to ${target}."
    return 0
  fi

  if ! command -v agora >/dev/null 2>&1 && [ ! -x "$DESTINATION" ]; then
    warn "Cannot install completion: agora binary not found yet."
    return 0
  fi

  agora_bin="$DESTINATION"
  if [ ! -x "$agora_bin" ]; then
    agora_bin=$(command -v agora 2>/dev/null || printf '')
  fi
  if [ -z "$agora_bin" ]; then
    return 0
  fi

  if ! mkdir -p "$(dirname "$target")" 2>/dev/null; then
    warn "Could not create completion directory: $(dirname "$target")"
    return 0
  fi

  if ! "$agora_bin" completion "$shell_name" > "$target" 2>/dev/null; then
    warn "Could not generate ${shell_name} completion. You can enable it manually with 'agora completion ${shell_name}'."
    rm -f "$target" 2>/dev/null || true
    return 0
  fi

  say_ok "Shell completion installed: ${target}"

  # Print the one-time activation hint per shell. This is deliberately
  # idempotent — we never modify rc files for completion (only PATH).
  case "$shell_name" in
    bash)
      say "  Activate now (or open a new shell):  source \"${target}\""
      say "  Note: requires bash-completion v2. Most distros install it by default; on macOS use 'brew install bash-completion@2'."
      ;;
    zsh)
      say "  Add this to ~/.zshrc if not already present:"
      say "    fpath=(\"\${HOME}/.zsh/completions\" \$fpath); autoload -Uz compinit && compinit"
      ;;
    fish)
      say "  Completion is auto-loaded on next fish session."
      ;;
  esac
}

# ---- Banner / footer -------------------------------------------------------
print_banner() {
  if [ "$QUIET" = "1" ]; then
    return 0
  fi
  printf '%sAgora CLI installer%s %s(rev %s)%s\n' "$BOLD" "$RESET" "$DIM" "$INSTALLER_VERSION" "$RESET"
}

print_next_steps() {
  if [ "$QUIET" = "1" ]; then
    return 0
  fi
  cat <<EOF

${BOLD}Next steps:${RESET}
  1. ${GREEN}agora login${RESET}             authenticate with Agora
  2. ${GREEN}agora init <name>${RESET}        scaffold your first project
  3. ${GREEN}agora --help${RESET}             explore all commands

${DIM}Docs:   ${DOCS_URL}${RESET}
${DIM}Issues: ${ISSUES_URL}${RESET}
EOF
}

# ---- Main ------------------------------------------------------------------
main() {
  init_colors
  parse_args "$@"
  if [ "$NO_COLOR_FLAG" = "1" ]; then
    reset_colors
  fi

  detect_platform
  detect_downloader
  need_cmd awk
  need_cmd cut
  need_cmd grep
  need_cmd sed
  need_cmd uname
  if [ "$OS" = "windows" ]; then
    need_cmd unzip
  else
    need_cmd tar
  fi

  TMP=$(mktemp -d)
  trap cleanup EXIT HUP INT TERM

  if [ "$LIST_VERSIONS" = "1" ]; then
    list_versions_and_exit
  fi

  if [ "$UNINSTALL" = "1" ]; then
    uninstall
    exit "$EXIT_OK"
  fi

  print_banner

  normalize_version
  if [ -z "$VERSION" ]; then
    say_step "Resolving latest version..."
    resolve_version
    say_ok "Latest is v${VERSION}"
  fi
  if [ -z "$VERSION" ]; then
    die "VERSION cannot be empty." "$EXIT_USAGE"
  fi

  FILENAME="agora-cli-go_v${VERSION}_${OS}_${ARCH}.${ARCHIVE_EXT}"
  ARCHIVE_URL="${RELEASES_DOWNLOAD_BASE_URL%/}/v${VERSION}/${FILENAME}"
  CHECKSUMS_URL="${RELEASES_DOWNLOAD_BASE_URL%/}/v${VERSION}/checksums.txt"
  ARCHIVE_PATH="${TMP}/${FILENAME}"
  CHECKSUMS_PATH="${TMP}/checksums.txt"
  EXTRACTED_BINARY="${TMP}/${BINARY_NAME}"

  show_existing_install

  ensure_install_dir_writable
  DESTINATION="${INSTALL_DIR}/${BINARY_NAME}"
  TEMP_DESTINATION="${INSTALL_DIR}/.${BINARY_NAME}.tmp.$$"

  # Idempotence: if the requested version is already at the target path, exit
  # early. This runs before the managed-install guard so re-running over a
  # matching install never errors on unrelated Homebrew/npm installs.
  if [ "$FORCE" != "1" ] && [ -x "$DESTINATION" ]; then
    if already_at_target_version "$DESTINATION" "$VERSION"; then
      say "agora ${VERSION} is already installed at ${DESTINATION}. Use --force to reinstall."
      exit "$EXIT_OK"
    fi
  fi

  guard_managed_install

  if [ "$DRY_RUN" = "1" ]; then
    say_step "Dry run - no changes will be made."
    say "  archive:   ${ARCHIVE_URL}"
    say "  checksums: ${CHECKSUMS_URL}"
    say "  install:   ${DESTINATION}"
    sudo_status="no"
    if [ "$USE_SUDO" = "1" ]; then
      sudo_status="yes"
    fi
    say "  sudo:      ${sudo_status}"
    say "  shell setup:"
    if [ "$SKIP_SHELL" = "1" ]; then
      say "    PATH:       skip (--skip-shell)"
      say "    completion: skip (--skip-shell)"
    else
      if [ "$NO_PATH" = "1" ]; then
        say "    PATH:       skip (--no-path)"
      else
        say "    PATH:       auto-add to your shell rc file if needed"
        add_to_path
      fi
      if [ "$NO_COMPLETION" = "1" ]; then
        say "    completion: skip (--no-completion)"
      else
        say "    completion: auto-install for the detected shell"
        install_completion
      fi
    fi
    exit "$EXIT_OK"
  fi

  say_step "Installing agora ${VERSION} (${OS}/${ARCH}) -> ${DESTINATION}"

  say_step "Downloading archive..."
  download_or_fail "$ARCHIVE_URL" "$ARCHIVE_PATH" archive

  say_step "Verifying checksum..."
  download_or_fail "$CHECKSUMS_URL" "$CHECKSUMS_PATH"

  EXPECTED_SHA=$(
    awk -v file="$FILENAME" '
      NF >= 2 {
        name = $2
        sub(/^\*/, "", name)
        if (name == file) {
          print $1
          exit
        }
      }
    ' "$CHECKSUMS_PATH"
  )
  if [ -z "$EXPECTED_SHA" ]; then
    err "Could not find checksum for ${FILENAME} in checksums.txt."
    say "First lines of checksums.txt:"
    head -n 5 "$CHECKSUMS_PATH" 2>/dev/null | sed 's/^/  /' || true
    say "Source: ${CHECKSUMS_URL}"
    exit "$EXIT_CHECKSUM"
  fi

  ACTUAL_SHA=$(sha256_file "$ARCHIVE_PATH")
  if [ "$EXPECTED_SHA" != "$ACTUAL_SHA" ]; then
    err "Checksum verification failed for ${FILENAME}."
    say "  expected: ${EXPECTED_SHA}"
    say "  actual:   ${ACTUAL_SHA}"
    say "If this is unexpected, please report at ${ISSUES_URL}."
    exit "$EXIT_CHECKSUM"
  fi
  say_ok "sha256 ${ACTUAL_SHA}"

  say_step "Extracting..."
  if ! extract_archive "$ARCHIVE_PATH"; then
    die "Could not extract ${BINARY_NAME} from ${FILENAME}." "$EXIT_INSTALL"
  fi
  if [ ! -f "$EXTRACTED_BINARY" ]; then
    die "Expected binary not found after extraction." "$EXIT_INSTALL"
  fi

  say_step "Installing to ${DESTINATION}..."
  install_binary "$EXTRACTED_BINARY" "$TEMP_DESTINATION" "$DESTINATION"
  TEMP_DESTINATION=""

  if ! verify_installed_binary "$DESTINATION"; then
    die "Installed binary did not start correctly." "$EXIT_VERIFY"
  fi
  write_install_receipt "$DESTINATION"
  say_ok "agora ${VERSION} installed."

  # ---- Shell setup (auto-by-default) -------------------------------------
  # PATH and completion are wired automatically by default. --no-path,
  # --no-completion, and --skip-shell let users opt out granularly.
  # Order matters: we resolve PATH first, then completion, so the
  # completion step can use the binary we just installed. Both PATH and
  # completion are best-effort — a write failure never aborts the
  # install; we always fall back to printing exact manual instructions.
  path_status="ok"          # ok | added | auto_failed | skipped | shadowed | manual
  completion_status="ok"    # ok | installed | skipped
  resolved=""

  if [ "$SKIP_SHELL" = "1" ] || [ "$NO_PATH" = "1" ]; then
    if resolved=$(command -v agora 2>/dev/null); then
      if [ "$resolved" = "$DESTINATION" ]; then
        path_status="ok"
      else
        path_status="shadowed"
      fi
    else
      path_status="manual"
    fi
  else
    # Auto path-wiring: only modify rc when DESTINATION isn't already
    # resolvable on PATH. INSTALL_DIR=/usr/local/bin (the macOS/Linux
    # default) is normally already on PATH, so this is a no-op for the
    # common case and only writes to ~/.zshrc / ~/.bashrc / fish config
    # when needed (custom INSTALL_DIR like ~/.local/bin).
    if resolved=$(command -v agora 2>/dev/null); then
      if [ "$resolved" = "$DESTINATION" ]; then
        path_status="ok"
      else
        # Another binary shadows ours; don't silently rewrite the user's
        # rc file — that wouldn't fix the shadowing anyway.
        path_status="shadowed"
      fi
    else
      if add_to_path; then
        path_status="added"
      else
        # Auto-add failed (rc unwritable, permission denied, etc.).
        # Fall back to printing exact manual instructions so the user
        # always knows the next step.
        path_status="auto_failed"
      fi
    fi
  fi

  if [ "$SKIP_SHELL" = "1" ] || [ "$NO_COMPLETION" = "1" ]; then
    completion_status="skipped"
  else
    install_completion
    completion_status="installed"
  fi

  # ---- Shell-setup summary footer ---------------------------------------
  case "$path_status" in
    ok)
      say "Resolved on PATH: ${DESTINATION}"
      ;;
    added)
      : # already explained inside add_to_path
      ;;
    auto_failed)
      : # add_to_path already printed a complete failure block — do
        # not duplicate it here. The single warn + manual block style
        # mirrors rustup / uv / Stripe CLI conventions.
      ;;
    shadowed)
      warn "Another agora is earlier on PATH: ${resolved}"
      warn "Reorder PATH so ${INSTALL_DIR} comes first, or remove the other binary."
      ;;
    manual)
      print_path_instructions
      ;;
  esac

  if [ "$completion_status" = "skipped" ]; then
    say "Shell completion skipped. Enable later with:"
    print_completion_instructions
  fi

  print_next_steps
}

main "$@"
