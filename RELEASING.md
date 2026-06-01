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

2. **npm publish** job (active):
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

Before tagging, ensure [CHANGELOG.md](CHANGELOG.md) has the version section finalized, including any migration or upgrade notes. GoReleaser publishes auto-generated release notes from commits; paste highlights from the CHANGELOG section into the GitHub release description if you want a curated summary.

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

## Manual npm dry-run (no tag required)

The release workflow exposes a `workflow_dispatch` trigger that runs the npm publish job in `--dry-run` mode against a synthetic version tag. Use this to validate npm packaging changes (metadata, scripts, provenance permissions) without minting a real GitHub release:

1. GitHub → Actions → Release → Run workflow → leave `dry_run` set to `true`.
2. Inspect the job logs for what would be published, including provenance request and tarball contents.
3. The smoke-test step is skipped in dry-run mode (nothing was actually published).

## Pre-tag checklist (npm)

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

## Distribution Channels

| Channel                 | How                                                         |
| ----------------------- | ----------------------------------------------------------- |
| Homebrew                | Coming soon; direct installer is current primary macOS path |
| npm (convenience)       | Active; wrapper uses trusted publishing, platform packages use `NPM_TOKEN` |
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
