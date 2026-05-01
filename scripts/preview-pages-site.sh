#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${PORT:-4000}"
SITE_DIR="${SITE_DIR:-_site}"

cd "$ROOT"

if ! command -v jekyll >/dev/null 2>&1 && command -v gem >/dev/null 2>&1; then
  GEM_HOME_BIN="$(gem env home 2>/dev/null)/bin"
  if [ -x "${GEM_HOME_BIN}/jekyll" ]; then
    export PATH="${GEM_HOME_BIN}:${PATH}"
  fi
fi

if ! command -v jekyll >/dev/null 2>&1; then
  cat >&2 <<'EOF'
jekyll was not found on PATH.

If you installed Ruby with Homebrew, add both Ruby and the gem bin directory:

  echo 'export PATH="/opt/homebrew/opt/ruby/bin:/opt/homebrew/lib/ruby/gems/4.0.0/bin:$PATH"' >> ~/.zshrc
  source ~/.zshrc
  gem install bundler jekyll

Then re-run:

  make docs-preview

EOF
  exit 127
fi

rm -rf "$SITE_DIR"

# Local preview serves _site at http://localhost:${PORT}/, so strip the
# production GitHub Pages baseurl (/cli) while building. The production
# workflow still uses docs/_config.yml unchanged.
jekyll build -s docs -d "$SITE_DIR" --baseurl ""

CLI_DOCS_BASE_URL="http://localhost:${PORT}" \
CLI_DOCS_MD_BASE_URL="http://localhost:${PORT}/md" \
  python3 scripts/prepare-pages-site.py --source docs --site "$SITE_DIR" --env-file docs/site.env

cat <<EOF

Preview ready:
  Human docs:    http://localhost:${PORT}/
  Agent docs MD: http://localhost:${PORT}/md/index.md
  Resolved env:  http://localhost:${PORT}/docs.env

Press Ctrl-C to stop.

EOF

python3 -m http.server "$PORT" --directory "$SITE_DIR"
