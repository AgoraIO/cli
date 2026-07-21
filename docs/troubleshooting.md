---
title: Troubleshooting
---

# Troubleshooting

Common issues and their fixes when running Agora CLI. For broader install
guidance see [Install](install.html). For programmatic error inspection,
prefer `agora project doctor --json` and `agora auth status --json`.

## Diagnostics first

Before opening an issue, capture these:

```bash
agora --version
agora project doctor --json
agora auth status --json
```

The output above is what the [bug report
template](https://github.com/AgoraIO/cli/issues/new?template=bug_report.yml)
asks for and is the fastest path to a fix.

## Login or browser issues

Symptom: the OAuth browser window does not open, or you are running over
SSH / in a container.

```bash
agora login --no-browser
```

This prints the login URL so you can open it on another machine and paste
the callback. You can also disable auto-open globally:

```bash
agora config update --browser-auto-open=false
```

## "command not found: agora"

The installer printed the install directory but it is not on `PATH`.

```bash
# macOS / Linux
echo "$PATH"
sh install.sh                      # re-run installer (PATH wiring is auto-on by default)

# Windows PowerShell
$env:Path -split ';'
.\install.ps1                      # re-run installer (PATH wiring is auto-on by default)
```

## Multiple `agora` binaries on PATH

The installer detects when another `agora` shadows the freshly installed
binary and warns. You can also check directly:

```bash
which -a agora     # macOS / Linux
where.exe agora    # Windows PowerShell
```

Reorder `PATH` so the installer's directory comes first, or remove the
older binary.

If the older binary came from a global npm install and you want to switch
to the standalone installer, either migrate in one installer run:

```bash
curl -fsSL @@CLI_INSTALL_SH_URL@@ | sh -s -- --replace-npm
```

Or uninstall npm first and then run the standalone installer:

```bash
npm uninstall -g agoraio-cli
curl -fsSL @@CLI_INSTALL_SH_URL@@ | sh
```

Use `--force` only when you intentionally want two installs and understand
that the first `agora` on `PATH` wins.

## Installer can't reach GitHub (blocked region or rate limit)

The installer downloads from GitHub by default and automatically falls back to
the Agora mirror at `dl.agora.io` when GitHub is unreachable or rate-limited;
downloads stay SHA-256 verified either way. Where GitHub is **fully** blocked,
the GitHub-hosted script URL is unreachable too, so fetch the script from the
mirror and skip GitHub entirely:

```sh
curl -fsSL https://dl.agora.io/cli/install.sh | AGORA_INSTALL_SOURCE=s3 sh
```

PowerShell:

```powershell
$env:AGORA_INSTALL_SOURCE = 's3'; irm https://dl.agora.io/cli/install.ps1 | iex
```

`AGORA_INSTALL_SOURCE` accepts `auto` (default), `github`, or `s3`. Note that
`--prerelease` and version listing require GitHub; pin an explicit `--version`
to install a specific release from the mirror. See
[Install](install.html#mirror-fallback-for-restricted-networks) for details.

## `agora init` or `agora quickstart create` fails on `git clone`

The CLI shells out to `git clone` for quickstarts. Most failures map to a stable error code:

| Error code | Meaning | Fix |
|------------|---------|-----|
| `QUICKSTART_GIT_MISSING` | `git` is not on `PATH`. | Install git ([git-scm.com/downloads](https://git-scm.com/downloads)) and retry. |
| `QUICKSTART_REF_INVALID` | `--ref` is empty, dash-prefixed, or contains whitespace. | Pass a valid branch/tag/commit. |
| `QUICKSTART_REPO_OVERRIDE_INVALID` | The `AGORA_QUICKSTART_<TEMPLATE>_REPO_URL` env override is malformed. | Set it to an `https://`, `ssh://`, `git://`, `file://`, `git@host:path`, or absolute local path URL — or unset it. |

If the clone reaches git but still fails, verify connectivity:

```bash
git --version
git ls-remote https://github.com/AgoraIO-Conversational-AI/agent-quickstart-nextjs.git
```

Check proxies and corporate firewall rules if `ls-remote` hangs or fails.

The CLI invokes git with `-c credential.helper=` so credential helpers (including macOS keychain) are not consulted for these public clones — agent and CI subprocesses no longer fail with "Device not configured."

Workshops and internal forks can override the clone URL per template, e.g.:

```bash
AGORA_QUICKSTART_NEXTJS_REPO_URL=https://github.com/my-org/fork agora init demo --template nextjs
```

A `clone:override` progress event is emitted whenever this is in use, so JSON automation runs show which fork was cloned.

## "project does not have an app certificate"

`quickstart env write`, `init`, and `project env --with-secrets` need a
project with an App Certificate. Either pick another project or enable
the certificate in [Agora Console](https://console.agora.io).

```bash
agora project list --json
agora project use <project-name>
agora project doctor --json
```

## `--yes` or `AGORA_NO_INPUT=1` is not skipping the OAuth browser

This is intentional. `--yes` accepts the default for confirmation
prompts; it does not start a brand-new interactive OAuth flow in JSON,
CI, or non-TTY contexts. Authenticate once on the host first:

```bash
agora login
```

Then re-run your automation. CI runners should authenticate as part of
their bootstrap, not as part of every command.

## CI: "command requires authentication" without prompting

CI auto-detection is intentional: in CI, the CLI never spawns an OAuth
browser flow even with `--yes`. Pre-authenticate the runner:

```bash
agora login
```

Or set `AGORA_HOME=$(mktemp -d)` per job for an isolated session and
provision credentials via your secret store before invoking the CLI.

## CI accidentally self-upgraded the binary

`agora upgrade` performs an in-place update for installer-managed installs.
In CI and agent automation, prefer non-mutating checks:

```bash
agora upgrade --check --json
agora --upgrade-check --json
```

Installer-managed self-update is blocked in CI by default (v0.2.1+). Blocked
runs return `status: "manual"` with `ciBlocked: true` in JSON. Set
`AGORA_ALLOW_UPGRADE_IN_CI=1` only when a job intentionally needs to mutate
the installed binary.

For package-manager installs, use the package manager's own upgrade command.

## Upgrade from v0.1.7–v0.2.0 fails

Release archives were renamed from `agora-cli-go_v*` to `agora-cli_v*`
starting in v0.2.1. Binaries installed from v0.1.7 through v0.2.0 embed
upgrade logic that only knows the old prefix, so `agora upgrade` may fail
when downloading v0.2.1+.

Re-run the installer once (it always fetches the latest script and archive
names):

```bash
curl -fsSL @@CLI_INSTALL_SH_URL@@ | sh
```

npm and other package-manager installs are unaffected.

## `agora init --yes` fails with QUICKSTART_TEMPLATE_REQUIRED

In JSON, CI, or non-TTY runs, `agora init` requires an explicit template.
Pass `--template` (list options with `agora quickstart list`):

```bash
agora init my-demo --template nextjs --new-project --yes --json
```

## Output looks wrong in scripts (color codes, table widths)

The CLI auto-detects CI and disables color and progress bars there. In
local TTYs you can override:

```bash
agora <command> --no-color
NO_COLOR=1 agora <command>
agora <command> --json
```

For wrappers that parse output, always pass `--json`. Pretty output is
not a stable contract.

## "did you mean" suggestions

If you mistype a subcommand the CLI prints the closest matches:

```text
$ agora projct doctor
Error: unknown command "projct" for "agora"

Did you mean this?
        project
```

## Debug logging

Use `--debug` (equivalent to `AGORA_DEBUG=1`) to mirror structured log
records to stderr. JSON envelopes and exit codes are unchanged.

> v0.2.0 removed the legacy `--verbose` / `-v` alias and the
> `AGORA_VERBOSE` environment variable. If you still have a 0.1.x
> config file with a `verbose` key, it is silently migrated to
> `debug` on first load — no action required. Update any scripts
> that set `AGORA_VERBOSE=1` to set `AGORA_DEBUG=1` instead.

```bash
agora --debug project list
AGORA_DEBUG=1 agora init my-demo --template nextjs --json
```

The same lines are written to a rotating log file. Print the path with:

```bash
agora config path        # parent directory
```

The log file is `agora-cli.log` next to the config file.

## Telemetry / Sentry

Telemetry is opt-out. Disable with any of:

```bash
agora telemetry disable
agora config update --telemetry-enabled=false
DO_NOT_TRACK=1 agora <command>
```

See [Telemetry](telemetry.html) for the field schema.

## Boolean config flags are not turning off

Boolean flags in `agora config update` need explicit `=false` syntax. For example:

```bash
agora config update --telemetry-enabled=false
agora config update --browser-auto-open=false
agora config update --debug=false
```

Passing only `--telemetry-enabled` sets the value to true; omitting the flag leaves the existing config unchanged.

## Still stuck?

- Open a [GitHub Discussion](https://github.com/AgoraIO/cli/discussions)
  for "how do I" questions.
- Open a [bug report](https://github.com/AgoraIO/cli/issues/new?template=bug_report.yml)
  for a reproducible defect.
- Email **security@agora.io** for a suspected security vulnerability
  (see [SECURITY.md](https://github.com/AgoraIO/cli/blob/main/SECURITY.md)).
