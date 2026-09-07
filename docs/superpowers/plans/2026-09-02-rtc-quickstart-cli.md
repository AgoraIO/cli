# RTC Quickstart CLI Implementation Plan

> **For agentic workers:** Implement this plan task-by-task with a red-green-refactor loop. This task is local-only: do not commit, push, or mutate GitHub state.

**Goal:** Add the `nextjs + video-call` quickstart scenario and video-call project preset described by `rtc-qs-design/03-cli-design.md`, with matching CLI, JSON, MCP, completion, and doctor behavior.

**Architecture:** Replace the one-entry-per-template quickstart model with a catalog keyed by stable ID plus `template + scenario`. A shared selector resolves explicit flags, local binding, repository manifest, and legacy detection; all command surfaces consume that selector and the shared stable feature merger.

**Tech Stack:** Go 1.26.5, Cobra, standard-library JSON/filesystem/process APIs, existing fake BFF and local Git integration fixtures.

---

### Task 1: Project presets and stable feature calculation

**Files:**
- Modify: `internal/cli/projects.go`
- Modify: `internal/cli/commands.go`
- Test: `internal/cli/features_test.go`
- Test: `internal/cli/integration_project_test.go`

- [ ] Add a failing test for `project create --template video-call --dry-run --json` returning `template: video-call` and `enabledFeatures: [rtc]`.
- [ ] Add a failing test proving an unknown preset returns `PROJECT_TEMPLATE_UNKNOWN` before any fake BFF create request.
- [ ] Implement the project preset catalog and one stable feature merger that expands dependencies, deduplicates, and orders by `featureCatalog`.
- [ ] Route dry-run and real project creation through the same preset parser.
- [ ] Run focused project tests and keep old no-template behavior unchanged.

### Task 2: Scenario catalog, manifest, and local binding

**Files:**
- Modify: `internal/cli/quickstart.go`
- Modify: `internal/cli/local_project.go`
- Test: `internal/cli/quickstart_test.go`
- Test: `internal/cli/project_env_layout_test.go`

- [ ] Add a failing selector test for `nextjs + video-call`, default `nextjs + voice-agent`, and unsupported combinations.
- [ ] Add failing manifest tests for valid schema v1, malformed JSON, missing fields, unsupported schema, and selection conflicts.
- [ ] Add `scenario` to local bindings while preserving legacy bindings without it.
- [ ] Implement the `nextjs-video-call` catalog entry, manifest parser, selector precedence, mismatch errors, and scenario-specific repo override key.
- [ ] Keep manifest identity aligned with the CLI parameter model: schema v1 requires explicit `template` and `scenario`; existing default-scenario quickstarts remain valid without a manifest.
- [ ] Run focused quickstart and binding tests.

### Task 3: Quickstart and init command paths

**Files:**
- Modify: `internal/cli/quickstart.go`
- Modify: `internal/cli/init.go`
- Modify: `internal/cli/render.go`
- Test: `internal/cli/integration_quickstart_test.go`
- Test: `internal/cli/integration_init_test.go`

- [ ] Add a failing integration test for list/create/env-write JSON fields and RTC env layout.
- [ ] Add failing clone tests proving a missing or mismatched video-call manifest fails before env/binding writes, removes the cloned target, and does not affect default quickstarts without manifests.
- [ ] Implement `--scenario` on `quickstart create`, `quickstart env write`, and `init`.
- [ ] Validate required manifests immediately after clone and before stripping Git metadata or writing credentials; preserve the structured manifest error while reporting clone cleanup.
- [ ] Include `template`, `scenario`, and `requiredFeatures` in result payloads and bindings.
- [ ] Validate an existing project's required features before clone; return `QUICKSTART_REQUIRED_FEATURE_MISSING` with a remediation command.
- [ ] Use scenario requirements as new-project defaults, merged with explicit `--feature` values.
- [ ] Run focused quickstart and init tests, including legacy Next.js/Python/Go cases.

### Task 4: Doctor, MCP, completion, and introspection

**Files:**
- Modify: `internal/cli/doctor.go`
- Modify: `internal/cli/mcp.go`
- Modify: `internal/cli/completion.go`
- Modify: `internal/cli/introspect.go`
- Modify: `internal/cli/skills.go`
- Test: `internal/cli/mcp_test.go`
- Test: `internal/cli/integration_help_test.go`
- Test: `internal/cli/integration_project_test.go`
- Test: `internal/cli/quickstart_test.go`

- [ ] Add failing tests for MCP scenario schemas/dispatch and completion values.
- [ ] Add failing deep-doctor tests for binding/manifest/catalog/env consistency.
- [ ] Route MCP tools through the same command selectors and expose scenario in schemas/results.
- [ ] Complete project preset and scenario values from their catalogs; filter scenario completion by template when available.
- [ ] Update `create-nextjs-video-app` to run `init --template nextjs --scenario video-call --new-project --json`, use `pnpm install && pnpm dev`, and recommend `project doctor --feature rtc --deep --json`.
- [ ] Add a catalog test that locks the built-in RTC skill to the video-call scenario and the Quickstart's package-manager/runtime commands.
- [ ] Ensure introspection exposes every new flag and enum source.
- [ ] Run focused doctor, MCP, completion, and introspection tests.

### Task 5: Documentation and complete local verification

**Files:**
- Modify: `docs/commands.md`
- Modify: `docs/automation.md`
- Modify: `docs/llms.txt`

- [ ] Regenerate command documentation from the live Cobra tree.
- [ ] Document stable JSON/MCP fields, project presets, scenarios, errors, manifest, and repo override.
- [ ] Run `gofmt` and `go test ./...`.
- [ ] Build `./agora` and inspect `--help --all`, `introspect --json`, and MCP tool schemas.
- [ ] Clone `https://github.com/littleDogWang/agora-rtc-nextjs-quickstart` into a temporary local fixture and verify its manifest/env layout.
- [ ] Run all new commands locally using isolated config and fake/local endpoints where remote control-plane state would otherwise be required.
- [ ] Report observed command results separately from runtime/media behavior, which remains outside CLI scope.
