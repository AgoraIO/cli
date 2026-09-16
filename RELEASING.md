# Releasing Agora CLI

Releases are fully automated via GoReleaser. Pushing a `v*` tag is the only manual step.

## Release

```bash
git tag v0.2.5
git push origin v0.2.5
```

The release workflow (`.github/workflows/release.yml`) then:

1. **GoReleaser** builds and publishes everything in parallel:
   - Cross-platform binaries (Linux, macOS, Windows — amd64 + arm64)
   - Archives: `agora-cli_v<version>_<os>_<arch>.{tar.gz,zip}` (v0.2.1+; older releases used `agora-cli-go_v*`)
   - Linux packages: `.deb`, `.rpm`, `.apk`
   - GitHub release with auto-generated changelog and checksums
   - Docker images → GitHub Container Registry (`ghcr.io/{owner}/agora-cli`)

2. **npm publish** job (currently disabled):
   - The npm channel is paused because the published package is stale and the release credentials are not ready; do not advertise npm installation until this job is explicitly re-enabled and a successful release is verified
   - Downloads the release archives, verifies them against `checksums.txt` (SHA-256), and refuses to publish on mismatch
   - Stages the per-platform binary into each unscoped `agoraio-cli-{os}-{arch}` package
   - Stamps the tag version into all package.json files (wrapper + 6 platform packages)
   - Publishes the six per-platform packages with `npm publish` authenticated by `NPM_TOKEN`
   - Publishes the wrapper package (`agoraio-cli`) with `npm publish --provenance`
   - Runs a post-publish smoke test: `npx --yes agoraio-cli@<tag> --version` with retry/backoff to handle registry propagation
   - Authenticates the wrapper via [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/) (OIDC from GitHub Actions)
   - Requires `NPM_TOKEN` for the platform packages and `id-token: write` workflow permission for the wrapper (already set in `release.yml`)

3. **Apt repository** job (triggered by the published release):
   - Downloads `.deb` files from the release
   - Rebuilds the signed apt repo on GitHub Pages
   - Requires `APT_SIGNING_KEY` secret + `APT_SIGNING_KEY_ID` variable

## Release notes

Before tagging, ensure [CHANGELOG.md](CHANGELOG.md) has the version section finalized (empty `[Unreleased]`, dated release heading, updated compare links), including any migration or upgrade notes. GoReleaser publishes auto-generated release notes from commits; paste highlights from the CHANGELOG section into the GitHub release description if you want a curated summary.

## Recipe-backed init dependency (v0.2.9+)

Recipe-backed init depends on the versioned API at
`https://recipes.agora.io/api/v1`. Deploy and smoke-test the recipes release
before tagging a CLI release that advertises `agora init --recipe`:

- `GET /recipes?type=all`, `?type=ai`, and `?type=rtc` return schema version 1
  and only official recipes.
- `GET /recipes/<slug>` returns the documented detail wrapper. Recipes intended
  for CLI initialization include `cli.projectType` and all four `cli.env`
  fields; catalog-only recipes may omit `cli`.
- A missing or non-official slug returns `RECIPE_NOT_FOUND` without exposing
  its metadata.
- Run a CLI smoke test against production using `agora recipes list --json`,
  `agora recipes show <slug> --json`, and an init-compatible recipe in a
  disposable directory before pushing the CLI tag.

Do not tag the CLI first: existing binaries fail safely when the API is
unavailable, but the advertised onboarding path would not be usable.

## Local Verification

Before cutting a tag:

```bash
go test ./...
go build -o agora .
./agora --help
./agora whoami

# Dry-run GoReleaser to catch config errors before the real release:
goreleaser release --snapshot --clean
```

## npm release readiness (currently paused)

The npm publish job is currently disabled. Before re-enabling it, use the workflow's `workflow_dispatch` dry-run against a synthetic version tag to validate npm packaging changes (metadata, scripts, provenance permissions) without minting a real GitHub release:

1. GitHub → Actions → Release → Run workflow → leave `dry_run` set to `true`.
2. Inspect the job logs for what would be published, including provenance request and tarball contents.
3. The smoke-test step is skipped in dry-run mode (nothing was actually published).

## Pre-tag checklist (npm re-enable)

Before tagging a real npm release, confirm:

- [ ] The wrapper package has a **Trusted Publisher** configured on [npmjs.com](https://www.npmjs.com) (Package → Settings → Trusted Publisher → GitHub Actions):
  - Repository: `AgoraIO/cli`
  - Workflow filename: `release.yml`
- [ ] The six `agoraio-cli-{os}-{arch}` platform packages are publishable by the npm automation token stored in the `NPM_TOKEN` GitHub secret.
- [ ] `agoraio-cli` and `agoraio-cli-*` package names on npmjs.com are owned by the Agora npm org / publisher and not squatted.
- [ ] The workflow has `id-token: write` permission (already set in `release.yml`); wrapper trusted publishing and provenance require it.
- [ ] A `workflow_dispatch` dry-run on the current `main` succeeds end-to-end (validates packaging and tarball contents).
- [ ] First publish should be a release-candidate tag (e.g. `v0.1.x-rc.1`) so an unexpected failure does not affect a "latest" tag in the registry.

## Required Secrets and Variables

| Name                 | Type     | Required for                    |
| -------------------- | -------- | ------------------------------- |
| `APT_SIGNING_KEY`    | secret   | Signed apt repo on GitHub Pages |
| `APT_SIGNING_KEY_ID` | variable | Signed apt repo on GitHub Pages |
| `NPM_TOKEN`          | secret   | Publishing npm platform packages |

Homebrew and Scoop are not part of the current GoReleaser config. Add `brews:` / `scoops:` blocks before documenting them as automated channels.

## S3 mirror (dl.agora.io)

On every tag push, the `mirror-to-s3` job in `release.yml` copies the release to
the CloudFront-fronted S3 mirror so installers work where GitHub is blocked:

- Versioned artifacts → `s3://dl-agora-io/cli/releases/v<version>/`
  (archives, `checksums.txt`, `checksums.txt.sigstore.json`; cached immutable).
- `install.sh` / `install.ps1` → `s3://dl-agora-io/cli/` (short cache).
- `latest.json` → `s3://dl-agora-io/cli/latest.json` (stable releases only).
- CloudFront (`E2U1WWAZBG33XY`) invalidation for the mutable paths.

### Required GitHub secrets

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`

### Minimal IAM policy for those keys

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "s3:PutObject", "Resource": "arn:aws:s3:::dl-agora-io/cli/*" },
    { "Effect": "Allow", "Action": "cloudfront:CreateInvalidation", "Resource": "arn:aws:cloudfront::*:distribution/E2U1WWAZBG33XY" }
  ]
}
```

## Distribution Channels

| Channel                 | How                                                         |
| ----------------------- | ----------------------------------------------------------- |
| Homebrew                | Coming soon; direct installer is current primary macOS path |
| npm (convenience)       | Paused; do not use until the published package and release credentials are current |
| apt/deb (Debian/Ubuntu) | apt-repo.yml → GitHub Pages                                 |
| rpm (RHEL/Fedora)       | Release artifact (.rpm via GoReleaser)                      |
| apk (Alpine/Docker)     | Release artifact (.apk via GoReleaser)                      |
| Scoop (Windows)         | Coming soon                                                 |
| Docker (GHCR)           | GoReleaser dockers block                                    |
| Shell install script    | `install.sh` downloads from GitHub Releases                 |
| Winget (Windows)        | Manual: submit PR to microsoft/winget-pkgs                  |

## Rollback (npm)

If a published version is bad:

- Use `npm deprecate agoraio-cli@<bad-version> "<reason and recommended version>"` to warn anyone who installs it.
- Cut a fixed patch release as soon as possible.
- **Do not** `npm unpublish` (irreversible reputational damage and registry policy restricts unpublishing after 72 hours anyway).

## One-Time Setup Checklist

- [ ] Enable GitHub Pages on this repo (Settings → Pages → Source: GitHub Actions)
- [ ] Generate GPG key for apt signing; set `APT_SIGNING_KEY` and `APT_SIGNING_KEY_ID`
- [ ] Configure npm **Trusted Publisher** for `agoraio-cli` (repo: `AgoraIO/cli`, workflow: `release.yml`)
- [ ] Configure an npm automation token with publish access to the six `agoraio-cli-*` platform packages and store it as `NPM_TOKEN`
- [ ] Run a `workflow_dispatch` dry-run of the release workflow to validate npm packaging
- [ ] Add Homebrew and Scoop GoReleaser blocks before announcing those channels
- [ ] Submit first Winget manifest PR to `microsoft/winget-pkgs` after the first release
