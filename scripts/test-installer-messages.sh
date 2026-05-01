#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
INSTALLER="$ROOT/install.sh"
TMPROOT=$(mktemp -d)
ASSERTIONS=0

cleanup() {
  rm -rf "$TMPROOT" 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_contains() {
  file=$1
  needle=$2
  if ! grep -qF "$needle" "$file"; then
    printf '--- output ---\n' >&2
    sed 's/^/  /' "$file" >&2
    fail "missing \"$needle\""
  fi
  ASSERTIONS=$((ASSERTIONS + 1))
}

assert_not_contains() {
  file=$1
  needle=$2
  if grep -qF "$needle" "$file"; then
    printf '--- output ---\n' >&2
    sed 's/^/  /' "$file" >&2
    fail "unexpected \"$needle\""
  fi
  ASSERTIONS=$((ASSERTIONS + 1))
}

extract_helpers() {
  awk '
    /^bash_writable_rc\(\) \{/,/^\}/ { print }
    /^shell_rc_for_path\(\) \{/,/^\}/ { print }
    /^shell_path_line\(\) \{/,/^\}/ { print }
    /^shell_refresh_command\(\) \{/,/^\}/ { print }
    /^print_manual_path_block\(\) \{/,/^\}/ { print }
    /^add_to_path\(\) \{/,/^\}/ { print }
  ' "$INSTALLER"
}

run_case() {
  name=$1
  shell_path=$2
  body=$3
  case_dir="$TMPROOT/$name"
  helper_file="$case_dir/helpers.sh"
  output_file="$case_dir/output.txt"
  home_dir="$case_dir/home"
  install_dir="$home_dir/.local/bin"

  mkdir -p "$case_dir" "$home_dir"
  extract_helpers > "$helper_file"

  (
    set -eu
    QUIET=0
    BOLD=""
    RESET=""
    DIM=""
    GREEN=""
    DOCS_URL="https://github.com/AgoraIO/cli#readme"
    HOME=$home_dir
    SHELL=$shell_path
    INSTALL_DIR=$install_dir
    BINARY_NAME=agora
    DESTINATION="$INSTALL_DIR/$BINARY_NAME"
    DRY_RUN=0
    XDG_CONFIG_HOME=""
    export HOME SHELL INSTALL_DIR BINARY_NAME DESTINATION DRY_RUN XDG_CONFIG_HOME DOCS_URL
    say() { printf '%s\n' "$*"; }
    warn() { printf 'Warning: %s\n' "$*" >&2; }
    . "$helper_file"
    eval "$body"
  ) >"$output_file" 2>&1

  printf '%s\n' "$output_file"
}

success_output=$(run_case "success-zsh" "/bin/zsh" '
  : > "$HOME/.zshrc"
  mkdir -p "$INSTALL_DIR"
  add_to_path
')
assert_contains "$success_output" "Added $TMPROOT/success-zsh/home/.local/bin to PATH in $TMPROOT/success-zsh/home/.zshrc."
assert_contains "$success_output" "To use agora in this shell now, run:"
assert_contains "$success_output" "exec /bin/zsh"
assert_contains "$success_output" "(Or open a new terminal - the change takes effect either way.)"

failure_output=$(run_case "unwritable-zsh" "/bin/zsh" '
  : > "$HOME/.zshrc"
  chmod 444 "$HOME/.zshrc"
  mkdir -p "$INSTALL_DIR"
  if add_to_path; then
    echo "expected add_to_path to fail"
    exit 1
  fi
')
assert_not_contains "$failure_output" "Warning:"
assert_not_contains "$failure_output" "warn:"
assert_contains "$failure_output" "$TMPROOT/unwritable-zsh/home/.zshrc is not writable, so the installer can't add agora to your PATH automatically."
assert_contains "$failure_output" "agora is installed at $TMPROOT/unwritable-zsh/home/.local/bin/agora and is ready to run."
assert_contains "$failure_output" "export PATH=\"$TMPROOT/unwritable-zsh/home/.local/bin:\$PATH\""
assert_contains "$failure_output" "For other options (custom INSTALL_DIR, containers), see https://github.com/AgoraIO/cli#readme"

bash_walk_output=$(run_case "bash-walk" "/bin/bash" '
  : > "$HOME/.bashrc"
  chmod 444 "$HOME/.bashrc"
  : > "$HOME/.bash_profile"
  mkdir -p "$INSTALL_DIR"
  add_to_path
  grep -qF "$INSTALL_DIR" "$HOME/.bash_profile"
')
assert_contains "$bash_walk_output" "Added $TMPROOT/bash-walk/home/.local/bin to PATH in $TMPROOT/bash-walk/home/.bash_profile."

printf 'OK: %s installer message assertions passed\n' "$ASSERTIONS"
