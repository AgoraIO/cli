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

## `agora init` or `agora quickstart create` fails on `git clone`

The CLI shells out to `git clone` for quickstarts. Verify:

```bash
git --version
git ls-remote https://github.com/AgoraIO/agora-quickstart-nextjs.git
```

If `git` is missing, install it (Homebrew, apt, winget, etc.). If
network access fails, check proxies and corporate firewall rules.

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

## Still stuck?

- Open a [GitHub Discussion](https://github.com/AgoraIO/cli/discussions)
  for "how do I" questions.
- Open a [bug report](https://github.com/AgoraIO/cli/issues/new?template=bug_report.yml)
  for a reproducible defect.
- Email **security@agora.io** for a suspected security vulnerability
  (see [SECURITY.md](https://github.com/AgoraIO/cli/blob/main/SECURITY.md)).
