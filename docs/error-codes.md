# Agora CLI Error Codes

Structured JSON failures include `error.code` when the CLI can classify the recovery path. Agents and scripts should branch on `error.code` first, then `error.message`.

This catalog is the source of truth for stable codes. CI runs `make snapshot-error-codes` to assert that every `Code:` literal in `internal/cli/*.go` is documented here (or matches a documented dynamic prefix).

## Static codes

### Authentication

| Code | Exit | Meaning | Recovery |
|------|------|---------|----------|
| `AUTH_UNAUTHENTICATED` | 3 | No usable local session exists. | Run `agora login`. |
| `AUTH_SESSION_EXPIRED` | 3 | The stored session is expired or rejected after refresh. | Run `agora login` again. |
| `AUTH_OAUTH_EXCHANGE_FAILED` | 1 | The OAuth token endpoint rejected the authorization-code or refresh-token exchange. | Retry login; if persistent, file a bug with the HTTP status and request ID. |
| `AUTH_OAUTH_RESPONSE_INVALID` | 1 | The OAuth token endpoint returned a response missing required token fields. | Retry login; if persistent, file a bug. |

### Project resolution

| Code | Exit | Meaning | Recovery |
|------|------|---------|----------|
| `PROJECT_NOT_SELECTED` | 1 | No explicit, repo-local, or global project context is available. | Pass `--project`, work inside a bound quickstart, or run `agora project use <project>`. |
| `PROJECT_NOT_FOUND` | 1 | The requested project ID or exact name was not found. | Run `agora project list` and retry with the project ID. |
| `PROJECT_AMBIGUOUS` | 1 | A project name matched multiple projects. | Retry with the project ID. |
| `PROJECT_NO_CERTIFICATE` | 1 | The selected project has no app certificate for env seeding. | Enable an app certificate in Console or select another project. |
| `PROJECT_ENV_TEMPLATE_UNKNOWN` | 1 | The `--template` value for `project env write` is not supported. | Use `nextjs` or `standard`. |
| `PROJECT_NOT_READY` | 1 | `project doctor` could not surface a more specific issue. | Re-run `project doctor` for details. |

### Quickstart / init

| Code | Exit | Meaning | Recovery |
|------|------|---------|----------|
| `QUICKSTART_TEMPLATE_REQUIRED` | 1 | `init` needs a template in JSON, CI, or non-TTY mode. | Pass `--template` or run `agora quickstart list`. |
| `QUICKSTART_TEMPLATE_UNKNOWN` | 1 | The template ID is not known to this CLI. | Run `agora quickstart list`. |
| `QUICKSTART_TEMPLATE_UNAVAILABLE` | 1 | The template exists but is not currently available. | Choose an available template. |
| `QUICKSTART_TEMPLATE_ENV_UNSUPPORTED` | 1 | The selected template does not define an env target path. | Choose a template with env support or configure the env file manually. |
| `QUICKSTART_TARGET_EXISTS` | 1 | The clone target already exists. | Choose a new directory. |
| `INIT_NAME_REQUIRED` | 1 | `agora init` was run without the required target directory name. | Pass a directory name, for example `agora init my-nextjs-demo --template nextjs`. |
| `INIT_ABORTED` | 1 | The interactive `agora init` reuse prompt was answered "no". | Re-run with `--project <id>`, `--new-project`, or accept the prompt. |
| `PROJECT_NAME_REQUIRED` | 1 | `agora project create` (or the equivalent MCP tool) was called without `name`. | Pass a project name, for example `agora project create my-app`. |

### Self-update (`agora upgrade`)

These codes only appear from the `agora upgrade` self-update path on installer-managed installs. Homebrew, npm, and other managed channels do not emit upgrade error codes — they always return `status: "manual"` with a manager-specific command.

| Code | Exit | Meaning | Recovery |
|------|------|---------|----------|
| `UPGRADE_NETWORK_FAILED` | 1 | Could not query the GitHub API for the latest release. | Check connectivity; provide `GITHUB_TOKEN` if rate-limited. |
| `UPGRADE_DOWNLOAD_FAILED` | 1 | Could not download the release archive or `checksums.txt`. | Check connectivity; retry; pin a specific version in the release page if needed. |
| `UPGRADE_CHECKSUM_FAILED` | 1 | The downloaded archive failed SHA-256 verification. | Do not continue; retry the download; inspect for a transparent proxy rewriting downloads. |
| `UPGRADE_EXTRACT_FAILED` | 1 | The new binary could not be extracted from the archive. | File a bug with the archive contents. |
| `UPGRADE_BINARY_RESOLVE_FAILED` | 1 | The CLI could not locate its own running executable. | Re-run with the absolute path to `agora`. |
| `UPGRADE_INSTALL_FAILED` | 1 | The new binary could not replace the running one (typically a Windows file-in-use error or a non-writable install dir). | Close other agora processes, or run from an elevated shell, then retry. |
| `UPGRADE_UNSUPPORTED_PLATFORM` | 1 | Self-update does not support the current OS/arch combination. | Use the platform installer manually. |

### Doctor — workspace and local-binding

These codes appear inside `data.checks[].issues[].code` and (for blocking issues) propagate to the top-level `error.code` when JSON-mode `project doctor` exits non-zero.

| Code | Exit | Meaning | Recovery |
|------|------|---------|----------|
| `WORKSPACE_SCAN_FAILED` | 1 | `project doctor --deep` could not enumerate the repo workspace. | Inspect the directory permissions and retry. |
| `LOCAL_PROJECT_BINDING_INVALID` | 1 | `.agora/project.json` exists but is missing `projectId`. | Re-bind the repo: `agora project use <project>` or `agora init` from the repo root. |
| `LOCAL_PROJECT_BINDING_MISMATCH` | 1 | `.agora/project.json` points at a project that does not match the selected project. | Use `--project` to select the bound project, or rebind. |
| `WORKSPACE_TEMPLATE_UNKNOWN` | 1 | The CLI could not detect the quickstart template for this repo. | Pass `--template` to the failing command. |
| `WORKSPACE_ENV_PATH_UNKNOWN` | 1 | The CLI could not determine the quickstart env target path. | Pass `--template` and re-run; if persistent, file an issue. |
| `WORKSPACE_ENV_FILE_MISSING` | 1 | A quickstart env file expected by the bound template is missing. | Run the command from `suggestedCommand` (typically `agora quickstart env write`). |
| `WORKSPACE_ENV_READ_FAILED` | 1 | The CLI could not read the quickstart env file. | Run the command from `suggestedCommand` (`agora quickstart env write . --project <id> --overwrite`); if it still fails, inspect file permissions and contents. |
| `WORKSPACE_ENV_PROJECT_MISMATCH` | 1 | The quickstart env file points at a different App ID than the selected project. | Run the command from `suggestedCommand` to overwrite the env file. |
| `WORKSPACE_ENV_METADATA_MISSING` | 1 | The quickstart env file is missing Agora-managed project metadata comments. | Run the command from `suggestedCommand` to refresh metadata. |
| `WORKSPACE_ENV_APP_ID_MISSING` | 1 | A quickstart env file is missing the required app ID key. | Run the command from `suggestedCommand`. |
| `WORKSPACE_ENV_APP_ID_MISMATCH` | 1 | A quickstart env file points at a different app ID. | Run the command from `suggestedCommand`. |
| `APP_CREDENTIALS_MISSING` | 1 | The selected project has no app ID / app certificate yet. | Run the command from `suggestedCommand` (`agora project show --project <id>`) to re-fetch credentials; if still missing, enable the app certificate in Console (`agora open --target console`). |
| `TOKEN_CAPABILITY_DISABLED` | (warning) | The project has token issuance disabled. | Enable token issuance in Console. |

## Dynamic code families

Some doctor codes are generated from the feature name at runtime. Agents should match by prefix.

| Pattern | Example | Meaning | Recovery |
|---------|---------|---------|----------|
| `FEATURE_<NAME>_PROVISIONING` | `FEATURE_RTC_PROVISIONING` | The named feature is being provisioned (warning). | Wait and re-run `project doctor`. |
| `FEATURE_<NAME>_DISABLED` | `FEATURE_CONVOAI_DISABLED` | The named feature is disabled for this project. | Run the command from `suggestedCommand` or enable the feature in Console. |

`<NAME>` is uppercased and matches the feature ID set returned by `agora introspect --json` under `data.enums.features` (currently `RTC`, `RTM`, `CONVOAI`).

## Upstream API errors

Failures returned by the Agora API preserve the upstream `code`, `httpStatus`, and `requestId` in the envelope when present. Agents should treat any `error.code` not in this catalog and not matching a dynamic prefix as a passthrough from the API and log/escalate it rather than handling it.

Common API error codes are not enumerated here because they may evolve independently of the CLI; consult the API documentation when handling them programmatically.
