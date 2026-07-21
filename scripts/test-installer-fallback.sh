#!/usr/bin/env sh
# Unit tests for install.sh source-selection / fallback logic.
# Extracts download_with_fallback (and set_fetch_profile) from install.sh and
# runs them with a stubbed single-attempt fetch so no network is touched.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
INSTALLER="$ROOT/install.sh"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP" 2>/dev/null || true' EXIT HUP INT TERM
ASSERTIONS=0

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

extract() {
  awk '
    /^set_fetch_profile\(\) \{/,/^\}/ { print }
    /^download_with_fallback\(\) \{/,/^\}/ { print }
  ' "$INSTALLER"
}

# run <source> <gh_status> <s3_status> -> prints "ATTEMPTS:<list>" and "RESULT:<rc>"
run() {
  source_mode=$1
  gh_status=$2
  s3_status=$3
  out_file="$TMP/out.txt"
  : >"$out_file"
  (
    set +e
    AGORA_INSTALL_SOURCE=$source_mode
    CURL_PROTO_OPTS="" ; CURL_RETRY_OPTS="" ; CURL_FASTFAIL_OPTS=""
    WGET_RETRY_OPTS="" ; WGET_FASTFAIL_OPTS=""
    curl_common_opts="" ; wget_common_opts=""
    RELEASES_PAGE_URL="https://example/releases"
    EXIT_NETWORK=20
    ATTEMPTS=""
    GH_STATUS=$gh_status
    S3_STATUS=$s3_status
    verbose() { :; }
    say() { :; }
    warn() { :; }
    err() { :; }
    die() { printf 'ATTEMPTS:%s\n' "$ATTEMPTS"; printf 'RESULT:die\n'; exit 1; }
    # Stub the single-attempt primitive used by download_with_fallback.
    _fetch_once() {
      __url=$1
      case "$__url" in
        *dl.agora.io*|*mirror*|*s3*) ATTEMPTS="$ATTEMPTS s3"; return "$S3_STATUS" ;;
        *) ATTEMPTS="$ATTEMPTS github"; return "$GH_STATUS" ;;
      esac
    }
    eval "$(extract)"
    download_with_fallback \
      "https://api.github.com/x" \
      "https://dl.agora.io/cli/latest.json" \
      "$TMP/dl.out" api
    rc=$?
    printf 'ATTEMPTS:%s\n' "$ATTEMPTS"
    printf 'RESULT:%s\n' "$rc"
  ) >"$out_file" 2>/dev/null || true
  cat "$out_file"
}

assert_eq() {
  got=$1; want=$2; msg=$3
  if [ "$got" != "$want" ]; then
    fail "$msg (got '$got', want '$want')"
  fi
  ASSERTIONS=$((ASSERTIONS + 1))
}

# auto + GitHub succeeds: only GitHub attempted.
out=$(run auto 0 0)
assert_eq "$(printf '%s' "$out" | grep '^ATTEMPTS:')" "ATTEMPTS: github" "auto/gh-ok attempts"
assert_eq "$(printf '%s' "$out" | grep '^RESULT:')" "RESULT:0" "auto/gh-ok result"

# auto + GitHub fails + S3 succeeds: GitHub then S3, success.
out=$(run auto 1 0)
assert_eq "$(printf '%s' "$out" | grep '^ATTEMPTS:')" "ATTEMPTS: github s3" "auto/gh-fail attempts"
assert_eq "$(printf '%s' "$out" | grep '^RESULT:')" "RESULT:0" "auto/gh-fail result"

# auto + both fail: GitHub then S3, then die.
out=$(run auto 1 1)
assert_eq "$(printf '%s' "$out" | grep '^ATTEMPTS:')" "ATTEMPTS: github s3" "auto/both-fail attempts"
assert_eq "$(printf '%s' "$out" | grep '^RESULT:')" "RESULT:die" "auto/both-fail result"

# github-only: never touches S3 even when GitHub fails.
out=$(run github 1 0)
assert_eq "$(printf '%s' "$out" | grep '^ATTEMPTS:')" "ATTEMPTS: github" "github-only attempts"
assert_eq "$(printf '%s' "$out" | grep '^RESULT:')" "RESULT:die" "github-only result"

# s3-only: skips GitHub entirely.
out=$(run s3 0 0)
assert_eq "$(printf '%s' "$out" | grep '^ATTEMPTS:')" "ATTEMPTS: s3" "s3-only attempts"
assert_eq "$(printf '%s' "$out" | grep '^RESULT:')" "RESULT:0" "s3-only result"

printf 'ok - %d assertions passed\n' "$ASSERTIONS"
