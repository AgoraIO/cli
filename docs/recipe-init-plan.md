# Recipe-backed project initialization

## Goal

Extend Agora CLI so users and agents can discover Agora recipes and initialize a
local project from a recipe repository while preserving the existing quickstart,
project-resolution, JSON-output, and MCP contracts.

The recommended design treats recipes as a second initialization source rather
than converting remote recipes into `quickstartTemplate` values. A built-in
quickstart carries CLI-owned environment layouts, credential keys, install
commands, run commands, and region-specific URLs. The recipes API currently
provides catalog and clone metadata, so it cannot safely provide all of the same
behavior without additional initialization metadata.

## Proposed command surface

```bash
agora recipes list
agora recipes list --type ai
agora recipes list --type rtc
agora recipes show python-quickstart

agora init my-app --recipe python-quickstart
agora init my-app --recipe python-quickstart --project my-project --json
agora init my-app --recipe python-quickstart --new-project --json
```

Command rules:

- `--recipe` and `--template` are mutually exclusive.
- Non-interactive `init` requires one of `--recipe` or `--template`.
- Existing `--template` behavior and response fields remain unchanged.
- When neither source is provided in an interactive session, retain the current
  built-in quickstart picker. Do not make the established flow depend on a live
  recipes API request.
- `recipes list` and `recipes show` are read-only discovery commands.
- Do not add a separate `recipes clone` command because that would overlap with
  `init` and the existing command layering.

If the first release is strictly limited to initialization, `recipes list` and
`recipes show` can be deferred. Direct slug resolution only needs the detail
endpoint.

## Recipe platform metadata

Keep `platforms` in both recipe summary and detail responses, but define it as a
normalized, machine-readable array rather than a set of free-form tags.

```json
{
  "platforms": ["nextjs", "python"]
}
```

Suggested initial values include:

- `nextjs`
- `python`
- `go`
- `react`
- `node`
- `ios`
- `android`

An array is preferable to a singular `platform` because a recipe may contain a
Next.js client and a Python or Go service. The CLI may use `platforms` for:

- display and client-side filtering
- choosing descriptive setup guidance
- recording `projectType` when exactly one unambiguous value is present

The CLI must not use `platforms` alone to guess environment file paths,
credential variable names, or shell commands. Repositories using the same
language or framework can have different layouts.

If the API should support fully automatic environment configuration, add an
optional detail-only `cli` object:

```json
{
  "platforms": ["nextjs", "python"],
  "cli": {
    "projectType": "nextjs",
    "env": {
      "examplePath": "server/.env.example",
      "targetPath": "server/.env.local",
      "appIdKey": "AGORA_APP_ID",
      "appCertificateKey": "AGORA_APP_CERTIFICATE"
    },
    "installCommand": "pnpm install",
    "runCommand": "pnpm dev"
  }
}
```

This object should remain optional. A recipe without `cli.env` is cloneable but
requires manual configuration unless its checked-out filesystem matches a
built-in quickstart layout.

## Internal design

Introduce a small source abstraction shared by quickstarts and recipes:

```go
type scaffoldSpec struct {
	Kind          string // "quickstart" or "recipe"
	ID            string
	Title         string
	RepoURL       string
	DocsURL       string
	RawDocsURL    string
	PrimaryPrompt string
	Template      *quickstartTemplate
}
```

Convert a built-in `quickstartTemplate` or fetched recipe detail into this type.
A shared scaffold operation should own:

1. Target-directory validation.
2. Clone progress events.
3. The shallow `git clone` operation.
4. Removal of upstream `.git` metadata.
5. Optional environment seeding.
6. Repo-local project binding.
7. Common result construction.

Preserve the existing Git invocation. It runs without a shell, disables
credential helpers, and puts `--` before the repository URL and target path.

Recipe resolution and validation should happen before creating a remote Agora
project. A missing recipe, invalid response, unavailable API, or occupied target
directory should not leave behind an unnecessary control-plane project.

## Environment behavior

For recipe initialization:

1. Clone the repository from `mainRepoUrl`.
2. Inspect the checked-out filesystem for one of the existing built-in
   quickstart layouts.
3. If a known layout matches, reuse the existing `seedQuickstartEnv` behavior.
4. Otherwise, write the repo-local project binding without creating an env file.
5. Return an explicit manual-configuration state and expose the recipe document
   URLs and `primaryPrompt` for the next step.

Example result when no supported env layout is available:

```json
{
  "sourceType": "recipe",
  "sourceId": "python-quickstart",
  "recipe": "python-quickstart",
  "envPath": "",
  "envStatus": "manual",
  "status": "needs-configuration"
}
```

Do not automatically execute `primaryPrompt`, install commands, run commands, or
content fetched from `recipeRawUrl`. These are untrusted remote values and must
remain output or guidance unless a user explicitly requests the corresponding
action.

## Repo-local metadata

Extend `.agora/project.json` additively:

```go
type localProjectBinding struct {
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	Region      string `json:"region"`
	ProjectType string `json:"projectType,omitempty"`
	Template    string `json:"template,omitempty"`
	Recipe      string `json:"recipe,omitempty"`
	EnvPath     string `json:"envPath,omitempty"`
}
```

Do not store a recipe slug in `Template`. Existing quickstart env resolution
expects that field to identify a compiled quickstart template. When a cloned
recipe matches a known quickstart layout, both `Recipe` and `Template` may be
stored so recipe provenance and env-layout behavior are preserved.

## Recipes API client

Add `internal/cli/recipes.go` containing:

- DTOs matching the public API summary, detail, and list payloads
- `listRecipes(recipeType)`
- `getRecipe(slug)`
- `recipes list` and `recipes show` command builders
- response validation and deterministic sorting
- conversion from recipe detail to `scaffoldSpec`

Client requirements:

- Use a fixed production base URL once the hostname is confirmed.
- Support `AGORA_RECIPES_BASE_URL` for staging and integration tests.
- Use the shared `App.httpClient` and Agora CLI `User-Agent`.
- Escape recipe slugs as URL path segments.
- Bound response reads to prevent an unexpectedly large payload from consuming
  unbounded memory.
- Validate `schemaVersion` where it is present.
- Require non-empty `slug`, `title`, and `mainRepoUrl` on detail responses.
- Validate `mainRepoUrl` before invoking Git.
- Sort list responses case-insensitively by title and then by slug. The server
  remains responsible only for filtering by catalog type.

Suggested structured error codes:

- `RECIPE_TYPE_INVALID`
- `RECIPE_NOT_FOUND`
- `RECIPE_API_FAILED`
- `RECIPE_RESPONSE_INVALID`
- `RECIPE_SCHEMA_UNSUPPORTED`
- `RECIPE_REPO_URL_INVALID`
- `INIT_SOURCE_CONFLICT`
- `INIT_SOURCE_REQUIRED`

Add every new literal code to `docs/error-codes.md` so the repository's error
code audit continues to pass.

## JSON output contract

Add source fields to `init` results:

```json
{
  "sourceType": "quickstart",
  "sourceId": "nextjs"
}
```

For recipe-backed initialization, also include:

- `recipe`
- `recipeUrl`
- `recipeRawUrl`
- `primaryPrompt`
- `envStatus`

Existing quickstart results keep their current `template` field and all existing
values. For recipe results, document `template` as optional and populate it only
when a known quickstart env layout was detected. New automation should branch on
`sourceType` and `sourceId` rather than inferring the source from `template`.

Use `status: "ready"` only when the CLI completed all supported credential
configuration. Use `status: "needs-configuration"` when clone and binding
succeeded but recipe-specific setup remains.

## Other integration points

Update the following surfaces:

- Add `--recipe` to the Cobra `init` command.
- Add a mutually exclusive `recipe` input to the `agora.init` MCP tool.
- Optionally expose `agora.recipes.list` and `agora.recipes.show` MCP tools.
- Add pretty renderers for recipe list, detail, and recipe-backed init results.
- Add `recipeTypes` (`all`, `ai`, and `rtc`) to introspection enums.
- Document `AGORA_RECIPES_BASE_URL` in `agora env-help`.
- Update the README command model.
- Update `docs/automation.md`, `docs/llms.txt`, and generated command docs.
- Update `docs/error-codes.md` and `CHANGELOG.md`.

Do not perform a live recipes API request during shell completion. Network-bound
completion would make every Tab press depend on API availability and latency. A
future release can add recipe slug completion after introducing a short-lived
on-disk recipe catalog cache.

## Test coverage

Add unit and integration coverage for:

- all, AI, and RTC list endpoint selection
- normalized platform decoding
- deterministic client-side sorting
- detail fetch with a safely escaped slug
- unknown recipe handling
- malformed JSON and missing required fields
- unsupported schema versions
- invalid repository URLs
- `--template` and `--recipe` conflicts
- non-interactive invocation without either source
- recipe resolution failure before remote project creation
- successful clone and removal of upstream `.git`
- `.agora/project.json` recipe provenance
- automatic env seeding for a recognized built-in layout
- `envStatus: "manual"` for an unknown layout
- stable JSON fields
- MCP recipe initialization and input conflicts

Verification commands:

```bash
go test ./...
make lint
./agora --help --all --json
./agora introspect --json
```

## Required API decision

Confirm the production hostname for the recipes API. The current specification
defines the `/api/v1` base path but not the host. The CLI should not infer or
hard-code a hostname until that deployment contract is settled.
