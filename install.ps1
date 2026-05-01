#!/usr/bin/env pwsh
#
# Agora CLI installer for Windows PowerShell.
# Mirrors the feature set of install.sh:
#   - SHA-256 checksum verification of downloaded archive
#   - NO_COLOR / -NoColor honored for stderr/stdout messages
#   - Idempotent re-runs: short-circuit when target version is already installed (use -Force to override)
#   - Managed-install detection (Scoop / Chocolatey / winget / npm) with -Force bypass
#   - Documented exit-code contract (matches install.sh; see docs/install.md)
#
# Quick start:
#   irm https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1 | iex
#
# Pin a version:
#   $env:VERSION = '0.2.0'; & ([scriptblock]::Create((irm .../install.ps1)))
#
[CmdletBinding()]
param(
    [string]$Version = $env:VERSION,
    [string]$InstallDir = $(if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\Agora\bin' }),
    [string]$GitHubRepo = $(if ($env:GITHUB_REPO) { $env:GITHUB_REPO } else { 'AgoraIO/cli' }),
    [switch]$Force,
    [switch]$NoColor,
    [switch]$Uninstall,
    # Shell-integration opt-outs. Default behavior matches modern
    # installers (bun, fnm, deno, uv): auto-wire user PATH and
    # PowerShell completion. Granular switches let callers decouple
    # each piece; -SkipShell is the umbrella that disables both.
    [switch]$NoPath,
    [switch]$NoCompletion,
    [switch]$SkipShell
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ---- Exit codes (mirror install.sh) ---------------------------------------
$EXIT_OK             = 0
$EXIT_GENERIC        = 1
$EXIT_USAGE          = 2
$EXIT_MISSING_PREREQ = 3
$EXIT_UNSUPPORTED    = 4
$EXIT_NETWORK        = 5
$EXIT_CHECKSUM       = 6
$EXIT_INSTALL        = 7
$EXIT_VERIFY         = 8
$InstallReceiptFileName = 'agora.install.json'

$GitHubApiUrl            = if ($env:GITHUB_API_URL) { $env:GITHUB_API_URL } else { 'https://api.github.com' }
$ReleasesDownloadBaseUrl = if ($env:RELEASES_DOWNLOAD_BASE_URL) { $env:RELEASES_DOWNLOAD_BASE_URL } else { "https://github.com/$GitHubRepo/releases/download" }
$ReleasesPageUrl         = if ($env:RELEASES_PAGE_URL) { $env:RELEASES_PAGE_URL } else { "https://github.com/$GitHubRepo/releases" }
$DocsUrl                 = if ($env:DOCS_URL) { $env:DOCS_URL } else { "https://github.com/$GitHubRepo#readme" }
$AuthToken               = if ($env:GITHUB_TOKEN) { $env:GITHUB_TOKEN } elseif ($env:GH_TOKEN) { $env:GH_TOKEN } else { $null }

# Color is suppressed when NO_COLOR env is set, -NoColor switch is passed,
# or the host does not support ANSI / is not a TTY.
$Script:UseColor = $true
if ($env:NO_COLOR) { $Script:UseColor = $false }
if ($NoColor)      { $Script:UseColor = $false }
if (-not $Host.UI.RawUI -or [Console]::IsOutputRedirected) { $Script:UseColor = $false }

function Write-Color {
    param(
        [string]$Message,
        [ConsoleColor]$Color = [ConsoleColor]::White
    )
    if ($Script:UseColor) {
        Write-Host $Message -ForegroundColor $Color
    } else {
        Write-Host $Message
    }
}

function Write-Info {
    param([string]$Message)
    Write-Host $Message
}

function Write-Warn {
    param([string]$Message)
    Write-Color "warn: $Message" -Color Yellow
}

function Fail {
    param(
        [string]$Message,
        [int]$ExitCode = $EXIT_GENERIC
    )
    Write-Color "error: $Message" -Color Red
    exit $ExitCode
}

function Normalize-Version {
    param([string]$Value)
    if ([string]::IsNullOrWhiteSpace($Value)) {
        return $null
    }
    if ($Value.StartsWith('v')) {
        return $Value.Substring(1)
    }
    return $Value
}

function Get-AuthHeaders {
    $headers = @{
        Accept = 'application/vnd.github+json'
    }
    if ($AuthToken) {
        $headers.Authorization = "Bearer $AuthToken"
    }
    return $headers
}

function Resolve-Architecture {
    switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()) {
        'x64'   { return 'amd64' }
        'arm64' { return 'arm64' }
        default {
            Fail "Unsupported architecture: $([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture)." -ExitCode $EXIT_UNSUPPORTED
        }
    }
}

function Resolve-Version {
    if ($script:Version) {
        $script:Version = Normalize-Version $script:Version
        return
    }

    $latestUrl = "$($GitHubApiUrl.TrimEnd('/'))/repos/$GitHubRepo/releases/latest"

    try {
        $release = Invoke-RestMethod -Uri $latestUrl -Headers (Get-AuthHeaders)
    } catch {
        Fail "Could not resolve the latest release. Set VERSION explicitly or provide GITHUB_TOKEN / GH_TOKEN if you are hitting rate limits. Release page: $ReleasesPageUrl" -ExitCode $EXIT_NETWORK
    }

    $script:Version = Normalize-Version $release.tag_name
    if (-not $script:Version) {
        Fail 'Could not parse the latest release version.' -ExitCode $EXIT_NETWORK
    }
}

function Download-File {
    param(
        [Parameter(Mandatory = $true)][string]$Url,
        [Parameter(Mandatory = $true)][string]$Destination
    )
    try {
        Invoke-WebRequest -Uri $Url -OutFile $Destination -Headers (Get-AuthHeaders)
    } catch {
        Fail "Failed to download $Url`nRelease page: $ReleasesPageUrl`nCheck your network or proxy settings, or try again with VERSION pinned." -ExitCode $EXIT_NETWORK
    }
}

function Get-ExpectedChecksum {
    param(
        [Parameter(Mandatory = $true)][string]$ChecksumsPath,
        [Parameter(Mandatory = $true)][string]$FileName
    )
    foreach ($line in Get-Content -Path $ChecksumsPath) {
        if ($line -match '^\s*([0-9A-Fa-f]+)\s+[*]?(.+?)\s*$') {
            if ($matches[2] -eq $FileName) {
                return $matches[1].ToLowerInvariant()
            }
        }
    }
    return $null
}

function Ensure-InstallDirectory {
    try {
        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    } catch {
        Fail "Could not create or write to $InstallDir. Use a writable -InstallDir or rerun from an elevated PowerShell session." -ExitCode $EXIT_INSTALL
    }
}

# Show-ManualPathBlock prints the copy-pasteable manual PATH-setup
# block. Callers (Add-InstallDirToUserPath on failure, Show-PathInstructions
# on explicit opt-out) emit a single warn line, then call this helper
# so wording, indentation, and the example command stay identical
# across both paths. Mirrors install.sh's print_manual_path_block.
function Show-ManualPathBlock {
    param([string]$BinaryPath)
    Write-Host "  agora is installed at $BinaryPath and is ready to run."
    Write-Host "  To add it to your user PATH, run one of:"
    Write-Host ""
    Write-Host "    setx PATH `"$InstallDir;%PATH%`""
    Write-Host "    [Environment]::SetEnvironmentVariable('Path', `"$InstallDir;`" + [Environment]::GetEnvironmentVariable('Path','User'), 'User')"
    Write-Host ""
    Write-Host "  Then open a new terminal so the change takes effect."
    Write-Host "  For other options (custom INSTALL_DIR, containers), see $DocsUrl"
}

# Add-InstallDirToUserPath appends $InstallDir to the user's persistent
# PATH. Best-effort: returns $true on success (added or already present),
# $false on any write failure. On failure it emits a plain-language
# branch message followed by the copy-pasteable manual block — the
# caller does NOT need to print any additional fallback hints.
function Add-InstallDirToUserPath {
    param([string]$BinaryPath = (Join-Path $InstallDir 'agora.exe'))
    try {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $segments = @()
        if ($userPath) {
            $segments = $userPath.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
        }
        if ($segments -contains $InstallDir) {
            Write-Info "$InstallDir is already on your user PATH."
            return $true
        }
        $newPath = if ($userPath) { "$userPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Info "Added $InstallDir to your user PATH."
        Write-Host "To use agora in this PowerShell session now, run:"
        Write-Host "    `$env:Path += ';$InstallDir'"
        Write-Host "(Or open a new terminal - the change takes effect either way.)"
        return $true
    } catch {
        Write-Host "Could not auto-update your user PATH (likely a permissions / UAC restriction)."
        Show-ManualPathBlock -BinaryPath $BinaryPath
        return $false
    }
}

# Show-PathInstructions is the manual-fallback hint shown when the user
# opted out of auto-PATH (-NoPath / -SkipShell) and the binary is not
# yet resolvable on PATH. Reuses Show-ManualPathBlock so the wording
# matches what the auto-failed path emits.
function Show-PathInstructions {
    param([string]$BinaryPath)
    Write-Warn "agora is not on your PATH yet."
    Show-ManualPathBlock -BinaryPath $BinaryPath
    Write-Host "  (Tip: re-run the installer without -NoPath / -SkipShell to do this automatically.)"
}

# Install-AgoraCompletion writes a small loader to the user's
# PowerShell profile so a fresh PowerShell session has tab-completion.
# Best-effort: failures never abort the install. Idempotent: subsequent
# runs detect the loader and skip. Caller is responsible for honoring
# -NoCompletion / -SkipShell before invoking.
function Install-AgoraCompletion {
    param(
        [string]$BinaryPath
    )
    if (-not (Test-Path -LiteralPath $BinaryPath)) {
        return
    }
    if (-not $PROFILE) {
        Write-Host "No PowerShell profile path detected. Skipping completion install."
        return
    }
    $profileDir = Split-Path -Parent $PROFILE
    try {
        if (-not (Test-Path -LiteralPath $profileDir)) {
            New-Item -ItemType Directory -Path $profileDir -Force | Out-Null
        }
    } catch {
        Write-Warn "Could not create profile directory: $profileDir"
        return
    }
    $marker = '# agora-cli completion (managed by install.ps1)'
    if (Test-Path -LiteralPath $PROFILE) {
        $existing = Get-Content -LiteralPath $PROFILE -Raw -ErrorAction SilentlyContinue
        if ($existing -and $existing.Contains($marker)) {
            Write-Info "Agora completion already wired in $PROFILE."
            return
        }
    }
    $loader = @"

$marker
if (Get-Command agora -ErrorAction SilentlyContinue) {
    agora completion powershell | Out-String | Invoke-Expression
}
"@
    try {
        Add-Content -LiteralPath $PROFILE -Value $loader -ErrorAction Stop
        Write-Info "Wired Agora CLI completion into $PROFILE."
        Write-Host "  Open a new PowerShell window or run: . `$PROFILE"
    } catch {
        Write-Warn "Could not append completion loader to $PROFILE. Run 'agora completion powershell | Out-String | Invoke-Expression' manually."
    }
}

function Verify-Binary {
    param([string]$Path)
    try {
        & $Path --version *> $null
        return
    } catch {
    }
    try {
        & $Path --help *> $null
    } catch {
        Fail "Installed binary did not start correctly: $Path" -ExitCode $EXIT_VERIFY
    }
}

function Write-InstallReceipt {
    param(
        [Parameter(Mandatory = $true)][string]$BinaryPath
    )
    $receiptPath = Join-Path (Split-Path -Parent $BinaryPath) $InstallReceiptFileName
    $receipt = [ordered]@{
        schemaVersion = 1
        tool = 'agora'
        installMethod = 'installer'
        installPath = $BinaryPath
        version = $Version
        installedAt = [DateTimeOffset]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
        source = 'install.ps1'
    }
    $receipt | ConvertTo-Json -Depth 3 | Set-Content -Path $receiptPath -Encoding UTF8
}

function Get-InstalledVersion {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) {
        return $null
    }
    try {
        $output = (& $Path --version 2>$null | Out-String).Trim()
        if ($output) { return $output }
    } catch {
    }
    return $null
}

function Test-AlreadyAtTargetVersion {
    param(
        [string]$Path,
        [string]$TargetVersion
    )
    $output = Get-InstalledVersion -Path $Path
    if (-not $output) { return $false }
    return $output -match [regex]::Escape($TargetVersion)
}

# Detect any agora install on PATH that came from a managed package manager
# (Scoop, Chocolatey, winget, npm). Returns a hashtable describing the manager
# or $null if no managed install is on PATH.
function Detect-ManagedInstall {
    $cmd = Get-Command agora -ErrorAction SilentlyContinue
    if (-not $cmd) { return $null }
    $source = $cmd.Source
    if (-not $source) { return $null }

    # Scoop installs to $env:USERPROFILE\scoop\shims by default.
    if ($env:SCOOP -and $source.StartsWith($env:SCOOP)) {
        return @{ Manager = 'Scoop'; Path = $source; Upgrade = 'scoop update agora' }
    }
    if ($source -match '\\scoop\\shims\\') {
        return @{ Manager = 'Scoop'; Path = $source; Upgrade = 'scoop update agora' }
    }

    # Chocolatey installs to $env:ChocolateyInstall\bin.
    if ($env:ChocolateyInstall -and $source.StartsWith($env:ChocolateyInstall)) {
        return @{ Manager = 'Chocolatey'; Path = $source; Upgrade = 'choco upgrade agora' }
    }
    if ($source -match '\\chocolatey\\bin\\') {
        return @{ Manager = 'Chocolatey'; Path = $source; Upgrade = 'choco upgrade agora' }
    }

    # winget installs typically land under $env:LOCALAPPDATA\Microsoft\WinGet\Packages.
    if ($source -match '\\WinGet\\Packages\\') {
        return @{ Manager = 'winget'; Path = $source; Upgrade = 'winget upgrade Agora.Cli' }
    }

    # npm-global installs land under (npm prefix -g)\agora.cmd or .ps1.
    $npm = Get-Command npm -ErrorAction SilentlyContinue
    if ($npm) {
        try {
            $npmPrefix = (& npm prefix -g 2>$null | Out-String).Trim()
            if ($npmPrefix -and $source.StartsWith($npmPrefix)) {
                return @{ Manager = 'npm'; Path = $source; Upgrade = 'npm update -g agoraio-cli' }
            }
        } catch {
        }
    }

    return $null
}

function Guard-ManagedInstall {
    $managed = Detect-ManagedInstall
    if (-not $managed) { return }

    if ($Force) {
        Write-Warn "Detected $($managed.Manager)-managed install at $($managed.Path). Continuing because -Force is set."
        return
    }

    Write-Color "error: Detected $($managed.Manager)-managed install at $($managed.Path)." -Color Red
    Write-Host "  Recommended: $($managed.Upgrade)"
    Write-Host "  Or re-run with -Force to install alongside (may shadow the $($managed.Manager) install on PATH)."
    exit $EXIT_INSTALL
}

function Show-ExistingInstall {
    $command = Get-Command agora -ErrorAction SilentlyContinue
    if (-not $command) { return }

    $versionOutput = ''
    try {
        $versionOutput = (& $command.Source --version 2>$null | Out-String).Trim()
    } catch {
    }

    if ($versionOutput) {
        Write-Info "Existing install: $versionOutput ($($command.Source))"
    } else {
        Write-Info "Existing install detected at $($command.Source)"
    }
}

function Uninstall-Agora {
    $destinationBinary = Join-Path $InstallDir 'agora.exe'
    $receiptPath = Join-Path $InstallDir $InstallReceiptFileName

    Write-Info "Uninstalling Agora CLI from $InstallDir"
    if (Test-Path -LiteralPath $destinationBinary) {
        Remove-Item -LiteralPath $destinationBinary -Force
        Write-Info "Removed $destinationBinary"
    } else {
        Write-Info "No agora binary found at $destinationBinary"
    }
    if (Test-Path -LiteralPath $receiptPath) {
        Remove-Item -LiteralPath $receiptPath -Force
        Write-Info "Removed $receiptPath"
    }
    Write-Info "Config, session, context, and logs are preserved under the Agora CLI config directory."
}

# ---- Main ------------------------------------------------------------------
$destinationBinary = Join-Path $InstallDir 'agora.exe'
if ($Uninstall) {
    Uninstall-Agora
    exit $EXIT_OK
}

$Version = Normalize-Version $Version
$arch = Resolve-Architecture
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("agora-install-" + [System.Guid]::NewGuid().ToString('N'))

try {
    Resolve-Version
    $fileName = "agora-cli-go_v$Version" + "_windows_${arch}.zip"
    $archivePath = Join-Path $tempRoot $fileName
    $checksumsPath = Join-Path $tempRoot 'checksums.txt'
    $extractDir = Join-Path $tempRoot 'extract'
    $sourceBinary = Join-Path $extractDir 'agora.exe'
    $tempDestinationBinary = Join-Path $InstallDir ('.agora.tmp.' + [System.Guid]::NewGuid().ToString('N') + '.exe')
    $archiveUrl = "$($ReleasesDownloadBaseUrl.TrimEnd('/'))/v$Version/$fileName"
    $checksumsUrl = "$($ReleasesDownloadBaseUrl.TrimEnd('/'))/v$Version/checksums.txt"

    Show-ExistingInstall

    # Idempotent short-circuit: if the destination already has the target version,
    # do nothing unless -Force is set. Mirrors install.sh's already_at_target_version.
    if (-not $Force) {
        if (Test-AlreadyAtTargetVersion -Path $destinationBinary -TargetVersion $Version) {
            Write-Info "agora $Version already installed at $destinationBinary. Use -Force to reinstall."
            exit $EXIT_OK
        }
    }

    # Refuse to overwrite a managed install (Scoop / Chocolatey / winget / npm)
    # unless -Force is set. Mirrors install.sh's guard_managed_install.
    Guard-ManagedInstall

    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    New-Item -ItemType Directory -Force -Path $extractDir | Out-Null

    Write-Info "Installing agora $Version (windows/$arch) -> $destinationBinary"

    Download-File -Url $archiveUrl -Destination $archivePath
    Download-File -Url $checksumsUrl -Destination $checksumsPath

    $expectedChecksum = Get-ExpectedChecksum -ChecksumsPath $checksumsPath -FileName $fileName
    if (-not $expectedChecksum) {
        Fail "Could not find checksum for $fileName in checksums.txt." -ExitCode $EXIT_CHECKSUM
    }

    $actualChecksum = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualChecksum -ne $expectedChecksum) {
        Fail "Checksum verification failed for $fileName. expected=$expectedChecksum actual=$actualChecksum" -ExitCode $EXIT_CHECKSUM
    }

    Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force
    if (-not (Test-Path -LiteralPath $sourceBinary)) {
        Fail "Expected binary not found after extraction: $sourceBinary" -ExitCode $EXIT_INSTALL
    }

    Ensure-InstallDirectory
    Copy-Item -LiteralPath $sourceBinary -Destination $tempDestinationBinary -Force
    Move-Item -LiteralPath $tempDestinationBinary -Destination $destinationBinary -Force

    Verify-Binary -Path $destinationBinary
    Write-InstallReceipt -BinaryPath $destinationBinary
    Write-Info "Installed agora to $destinationBinary"

    # ---- Shell setup (auto-by-default) -----------------------------------
    # PATH and completion are wired automatically by default. -NoPath,
    # -NoCompletion, and -SkipShell are granular opt-outs (the umbrella
    # -SkipShell implies both). Order matters: PATH first so completion
    # can use the binary we just installed.
    $resolved = Get-Command agora -ErrorAction SilentlyContinue

    if ($SkipShell -or $NoPath) {
        if (-not $resolved) {
            Show-PathInstructions -BinaryPath $destinationBinary
        } elseif ($resolved.Source -ne $destinationBinary) {
            Write-Warn "Another agora is earlier on PATH: $($resolved.Source)"
            Write-Warn "Reorder PATH so $InstallDir comes first, or remove the other binary."
        } else {
            Write-Info "Resolved on PATH: $($resolved.Source)"
        }
    } else {
        if (-not $resolved) {
            if (Add-InstallDirToUserPath -BinaryPath $destinationBinary) {
            }
            # On failure Add-InstallDirToUserPath already emitted a
            # complete warn + manual block (rustup / uv / Stripe CLI
            # convention). Do not double-print a fallback hint here.
        } elseif ($resolved.Source -ne $destinationBinary) {
            Write-Warn "Another agora is earlier on PATH: $($resolved.Source)"
            Write-Warn "Reorder PATH so $InstallDir comes first, or remove the other binary."
        } else {
            Write-Info "Resolved on PATH: $($resolved.Source)"
        }
    }

    if ($SkipShell -or $NoCompletion) {
        Write-Info "Shell completion skipped. Enable later with:"
        Write-Host "  agora completion powershell | Out-String | Invoke-Expression"
        Write-Host "  (or add the line above to your PowerShell `$PROFILE)"
    } else {
        Install-AgoraCompletion -BinaryPath $destinationBinary
    }

    Write-Color 'Done. Run: agora --help' -Color Green
    exit $EXIT_OK
} finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
