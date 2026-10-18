#Requires -Version 5.1
$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

function Write-Step {
    param([string]$Message)
    Write-Host "==> " -ForegroundColor Blue -NoNewline
    Write-Host $Message -ForegroundColor White
}

function Write-Ok {
    param([string]$Message)
    Write-Host "[ok] " -ForegroundColor Green -NoNewline
    Write-Host $Message
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[!] " -ForegroundColor Yellow -NoNewline
    Write-Host $Message
}

function Write-Info {
    param([string]$Message)
    Write-Host "[info] " -ForegroundColor Cyan -NoNewline
    Write-Host $Message
}

function Write-Fail {
    param([string]$Message)
    Write-Host "[error] " -ForegroundColor Red -NoNewline
    Write-Host $Message
    exit 1
}

function Get-NtlVersion {
    $versionFile = Join-Path $ScriptDir "internal\app\version.json"

    if (-not (Test-Path $versionFile)) {
        Write-Fail "version.json not found at $versionFile"
    }

    $content = Get-Content $versionFile -Raw

    if ($content -match '"version"\s*:\s*"([^"]*)"') {
        return $Matches[1]
    }

    Write-Fail "Could not parse version from version.json"
}

$NtlVersion = Get-NtlVersion
$GoMin = "1.25.5"
$GoVer = "1.25.14"

function Show-Banner {
    Write-Host ""
    Write-Host "  Lunex lang  v$NtlVersion"
    Write-Host "  Created by David Dev"
    Write-Host "  GitHub: https://github.com/Megamexlevi2"
    Write-Host ""
}

function Get-HostArch {
    $arch = $env:PROCESSOR_ARCHITECTURE

    switch ($arch) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { return "amd64" }
    }
}

function Test-VersionGte {
    param(
        [string]$Have,
        [string]$Want
    )

    $haveParts = $Have.Split(".") | ForEach-Object { [int]$_ }
    $wantParts = $Want.Split(".") | ForEach-Object { [int]$_ }

    for ($i = 0; $i -lt [Math]::Max($haveParts.Count, $wantParts.Count); $i++) {
        $h = if ($i -lt $haveParts.Count) { $haveParts[$i] } else { 0 }
        $w = if ($i -lt $wantParts.Count) { $wantParts[$i] } else { 0 }

        if ($h -gt $w) { return $true }
        if ($h -lt $w) { return $false }
    }

    return $true
}

function Get-GoVersionInstalled {
    try {
        $out = & go version 2>$null

        if ($out -match 'go(\d+\.\d+(\.\d+)?)') {
            return $Matches[1]
        }
    } catch {}

    return $null
}

function Find-PackageManager {
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        return "winget"
    }

    if (Get-Command choco -ErrorAction SilentlyContinue) {
        return "choco"
    }

    if (Get-Command scoop -ErrorAction SilentlyContinue) {
        return "scoop"
    }

    return ""
}

function Get-PackageManagerInstallCmd {
    param([string]$Pm)

    switch ($Pm) {
        "winget" { return "winget install --id GoLang.Go -e --accept-source-agreements --accept-package-agreements" }
        "choco"  { return "choco install golang -y" }
        "scoop"  { return "scoop install go" }
        default  { return "" }
    }
}

function Confirm-Prompt {
    param([string]$Question)

    if (-not [Environment]::UserInteractive) {
        return $false
    }

    $answer = Read-Host "$Question [y/N]"
    return $answer -match '^(y|yes)$'
}

function Install-GoBinary {
    $installed = Get-GoVersionInstalled

    if ($installed -and (Test-VersionGte -Have $installed -Want $GoMin)) {
        Write-Ok "Go $installed is already installed"
        return
    }

    Write-Warn "Go $GoMin+ was not found on this system"
    Write-Info "Detected environment: Windows"

    $pm = Find-PackageManager
    $wasInstalled = $false

    if ($pm -ne "") {
        $installCmd = Get-PackageManagerInstallCmd -Pm $pm
        Write-Info "Found package manager: $pm"

        if (Confirm-Prompt "Install Go via '$pm' now? ($installCmd)") {
            Write-Step "Installing Go via $pm"

            try {
                Invoke-Expression $installCmd
            } catch {
                Write-Fail "Go installation via $pm failed"
            }

            $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
            $installed = Get-GoVersionInstalled

            if ($installed -and (Test-VersionGte -Have $installed -Want $GoMin)) {
                Write-Ok "Go $installed installed"
                return
            }

            Write-Warn "Package manager installed a Go version older than $GoMin or go is not on PATH"
        }
    }

    if (Get-Command curl -ErrorAction SilentlyContinue) {
        if (Confirm-Prompt "Download and install Go $GoMin+ directly from go.dev via curl?") {
            Write-Step "Installing Go via curl"

            $arch = Get-HostArch
            $name = "go$GoVer.windows-$arch"
            $url = "https://go.dev/dl/$name.zip"

            $tmp = Join-Path $env:TEMP ([System.Guid]::NewGuid().ToString())
            New-Item -ItemType Directory -Path $tmp | Out-Null

            $zipPath = Join-Path $tmp "go.zip"
            & curl -fsSL $url -o $zipPath

            if ($LASTEXITCODE -ne 0) {
                Write-Fail "Download failed from $url"
            }

            Expand-Archive -Path $zipPath -DestinationPath $tmp -Force

            $installRoot = Join-Path $env:LOCALAPPDATA "go"

            if (Test-Path $installRoot) {
                Remove-Item -Recurse -Force $installRoot
            }

            Move-Item (Join-Path $tmp "go") $installRoot

            $goBin = Join-Path $installRoot "bin"
            $env:Path = "$goBin;$env:Path"

            [System.Environment]::SetEnvironmentVariable("Path", "$goBin;" + [System.Environment]::GetEnvironmentVariable("Path", "User"), "User")

            Remove-Item -Recurse -Force $tmp

            Write-Ok "Go $GoVer installed"
            return
        }
    }

    Write-Fail "Go $GoMin+ is required to build Lunex. Installation was declined or no installer is available."
}

function Build-GoBinary {
    param(
        [string]$Arch,
        [string]$TargetBin = "",
        [switch]$SkipUpToDate
    )

    $goArch = switch ($Arch) {
        "x86_64"  { "amd64" }
        "amd64"   { "amd64" }
        "aarch64" { "arm64" }
        "arm64"   { "arm64" }
        default   { $Arch }
    }

    if ($TargetBin -eq "") {
        $TargetBin = Join-Path $ScriptDir "lunex.exe"
    }

    $targetDir = Split-Path -Parent $TargetBin

    if ($targetDir -ne "" -and -not (Test-Path $targetDir)) {
        New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
    }

    $targetMeta = "$TargetBin.target"
    $targetId = "windows/$goArch"

    if (-not $SkipUpToDate -and (Test-Path $TargetBin) -and (Test-Path $targetMeta)) {
        $existingId = Get-Content $targetMeta -Raw -ErrorAction SilentlyContinue

        if ($existingId -and $existingId.Trim() -eq $targetId) {
            $binTime = (Get-Item $TargetBin).LastWriteTime

            $newer = Get-ChildItem -Path $ScriptDir -Recurse -File -ErrorAction SilentlyContinue |
                Where-Object {
                    ($_.Extension -eq ".go" -or $_.Name -eq "go.mod" -or $_.Name -eq "go.sum" -or $_.Name -eq "version.json") -and
                    $_.FullName -notmatch '\\\.git\\' -and
                    $_.LastWriteTime -gt $binTime
                }

            if (-not $newer) {
                Write-Ok "Go binary is up to date"
                return
            }
        }
    }

    Write-Step "Building Lunex for windows/$goArch"
    Push-Location $ScriptDir

    try {
        if (Test-Path $TargetBin) {
            Remove-Item -Force $TargetBin
        }

        $env:GONOSUMDB = "*"
        $env:GOFLAGS = "-mod=mod"
        $env:GOOS = "windows"
        $env:GOARCH = $goArch
        $env:CGO_ENABLED = "0"

        $logFile = Join-Path $env:TEMP ([System.Guid]::NewGuid().ToString() + ".log")

        & go build -trimpath -tags "netgo,osusergo" -ldflags "-s -w" -o $TargetBin ./cmd/lunex 2>&1 | Out-File -FilePath $logFile -Encoding utf8

        if ($LASTEXITCODE -ne 0) {
            Get-Content $logFile | Where-Object { $_ -notmatch '^go: downloading' } | Write-Host -ForegroundColor Red
            Remove-Item -Force $logFile -ErrorAction SilentlyContinue
            Write-Fail "Build failed"
        }

        Remove-Item -Force $logFile -ErrorAction SilentlyContinue

        if (-not (Test-Path $TargetBin)) {
            Write-Fail "Build completed without producing $TargetBin"
        }

        Set-Content -Path $targetMeta -Value $targetId -NoNewline

        $sizeKb = [Math]::Round((Get-Item $TargetBin).Length / 1KB, 1)
        $sizeDisplay = if ($sizeKb -gt 1024) { "$([Math]::Round($sizeKb / 1024, 1))M" } else { "${sizeKb}K" }

        Write-Ok "$(Split-Path -Leaf $TargetBin) built: $sizeDisplay"
    } finally {
        Pop-Location
        Remove-Item Env:\GONOSUMDB, Env:\GOFLAGS, Env:\GOOS, Env:\GOARCH, Env:\CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

function Build-Release {
    Write-Step "Starting multi-platform release build"

    $outDir = Join-Path $ScriptDir "release"

    if (-not (Test-Path $outDir)) {
        New-Item -ItemType Directory -Path $outDir | Out-Null
    }

    $targets = @("amd64", "arm64")

    foreach ($arch in $targets) {
        Write-Info "Building windows/$arch"

        $releaseName = "lunex-windows-$arch.exe"
        $releasePath = Join-Path $outDir $releaseName

        Build-GoBinary -Arch $arch -TargetBin $releasePath -SkipUpToDate

        $sizeKb = [Math]::Round((Get-Item $releasePath).Length / 1KB, 1)
        $sizeDisplay = if ($sizeKb -gt 1024) { "$([Math]::Round($sizeKb / 1024, 1))M" } else { "${sizeKb}K" }

        Write-Ok "Done: $releasePath ($sizeDisplay)"
    }

    Write-Ok "All targets built successfully in .\release\"
}

function Invoke-Tests {
    param([string]$Bin)

    $runner = Join-Path $ScriptDir "tests\run_all.sh"

    if (-not (Test-Path $runner)) {
        Write-Fail "Test runner not found: $runner"
    }

    Write-Step "Running Lunex integration tests"

    $bash = Get-Command bash -ErrorAction SilentlyContinue

    if (-not $bash) {
        Write-Fail "bash is required to run tests (install Git for Windows or WSL)"
    }

    & bash $runner $Bin

    if ($LASTEXITCODE -ne 0) {
        Write-Fail "Tests failed"
    }

    Write-Ok "All tests passed"
}

function Invoke-Clean {
    Write-Step "Cleaning build artifacts"
    Push-Location $ScriptDir

    try {
        $paths = @("lunex.exe", "lunex.exe.target", "release")

        foreach ($p in $paths) {
            if (Test-Path $p) {
                Remove-Item -Recurse -Force $p
            }
        }

        Write-Ok "Cleaned"
    } finally {
        Pop-Location
    }
}

$cmd = if ($args.Count -ge 1) { $args[0] } else { "build" }
$rest = if ($args.Count -ge 2) { $args[1..($args.Count - 1)] } else { @() }

switch ($cmd) {
    { $_ -eq "build" -or $_ -eq "" } {
        Show-Banner

        $targetArch = if ($rest.Count -ge 1) { $rest[0] } else { Get-HostArch }

        Install-GoBinary
        Build-GoBinary -Arch $targetArch -TargetBin (Join-Path $ScriptDir "lunex.exe")

        Write-Host ""
        Write-Ok "Lunex v$NtlVersion built successfully"
        Write-Info "Run:  .\lunex.exe run <file.lx>"
    }

    { $_ -eq "release" -or $_ -eq "--release" } {
        Show-Banner
        Install-GoBinary
        Build-Release
    }

    "test" {
        Install-GoBinary

        $targetBin = Join-Path $ScriptDir "lunex.exe"

        Build-GoBinary -Arch (Get-HostArch) -TargetBin $targetBin
        Invoke-Tests -Bin $targetBin
    }

    "clean" {
        Invoke-Clean
    }

    default {
        Write-Host "Usage: .\build.ps1 [build|release|--release|test|clean]"
        Write-Host "  build    - build for current architecture (default)"
        Write-Host "  release  - build for amd64 and arm64"
        Write-Host "  test     - build + run tests"
        Write-Host "  clean    - remove build artifacts"
        exit 1
    }
}