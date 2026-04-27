# Install Agora CLI Go

This page lists supported installation methods.

## Quick Install (npm)

```bash
npm install -g agoraio-cli
agora --help
```

Requires Node.js 18+.

## Homebrew Formula

```bash
brew tap agora/tap
brew install agora
agora --help
```

For the eventual no-tap install path (`brew install agora` via `homebrew/core`), follow [docs/homebrew-core.md](homebrew-core.md).

Upgrade:

```bash
brew update
brew upgrade agora
```

## Homebrew Cask (macOS Optional)

```bash
brew tap agora/tap
brew install --cask agora-cli-go
agora --help
```

## Build From Source

Requirements:
- Go toolchain from `go.mod`
- `git`

```bash
go build -o agora .
./agora --help
```

## Distribution and Tap Automation

Homebrew packaging templates, generator script, and tap automation workflow are documented in [docs/homebrew.md](homebrew.md).
