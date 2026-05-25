param(
  [string]$OutDir = "dist\windows-amd64",
  [string]$InstallerScript = "installer\tailscale-dev.iss",
  [string]$ElectronDir = "client\electron",
  [string]$DependencyRoot = "D:\workspace-qoder\deps",
  [switch]$SkipElectron,
  [switch]$SkipInstaller
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

$installerOutDirAbs = Join-Path $repoRoot "dist\installer"
New-Item -ItemType Directory -Force -Path $installerOutDirAbs | Out-Null
Get-ChildItem -LiteralPath $installerOutDirAbs -Filter "tailscale-*-windows-amd64-setup*.exe" -ErrorAction SilentlyContinue |
  Remove-Item -Force
Get-ChildItem -LiteralPath $installerOutDirAbs -Filter "scaletail-*-windows-amd64-setup*.exe" -ErrorAction SilentlyContinue |
  Remove-Item -Force

$outDirAbs = Join-Path $repoRoot $OutDir
New-Item -ItemType Directory -Force -Path $outDirAbs | Out-Null
  Remove-Item -LiteralPath `
  (Join-Path $outDirAbs "ScaleTail.exe"), `
  (Join-Path $outDirAbs "Tailscale.exe"), `
  (Join-Path $outDirAbs "tailscale-cli.exe"), `
  (Join-Path $outDirAbs "tailscale.exe"), `
  (Join-Path $outDirAbs "tailscale-systray.exe") `
  -Force -ErrorAction SilentlyContinue

$oldCgo = $env:CGO_ENABLED
$env:CGO_ENABLED = "0"
try {
  Write-Host "Building tailscaled.exe"
  go build -trimpath -o (Join-Path $outDirAbs "tailscaled.exe") ./cmd/tailscaled
  Write-Host "Building tailscale-localapi.exe"
  go build -trimpath -o (Join-Path $outDirAbs "tailscale-localapi.exe") ./cmd/tailscale-localapi
} finally {
  $env:CGO_ENABLED = $oldCgo
}

& (Join-Path $PSScriptRoot "ensure-wintun.ps1") -OutputDir $OutDir

if (-not $SkipElectron) {
  $electronAbs = Join-Path $repoRoot $ElectronDir
  if (-not (Test-Path -LiteralPath $electronAbs)) {
    throw "Electron client directory not found: $electronAbs"
  }

  $depRootAbs = $DependencyRoot
  New-Item -ItemType Directory -Force -Path $depRootAbs | Out-Null
  New-Item -ItemType Directory -Force -Path (Join-Path $depRootAbs "npm-cache") | Out-Null
  New-Item -ItemType Directory -Force -Path (Join-Path $depRootAbs "electron-cache") | Out-Null
  New-Item -ItemType Directory -Force -Path (Join-Path $depRootAbs "electron-builder-cache") | Out-Null

  $oldNpmCache = $env:npm_config_cache
  $oldElectronCache = $env:ELECTRON_CACHE
  $oldElectronBuilderCache = $env:ELECTRON_BUILDER_CACHE
  $oldCSC = $env:CSC_IDENTITY_AUTO_DISCOVERY
  try {
    $env:npm_config_cache = Join-Path $depRootAbs "npm-cache"
    $env:ELECTRON_CACHE = Join-Path $depRootAbs "electron-cache"
    $env:ELECTRON_BUILDER_CACHE = Join-Path $depRootAbs "electron-builder-cache"
    $env:CSC_IDENTITY_AUTO_DISCOVERY = "false"

    Push-Location $electronAbs
    try {
      if (Test-Path -LiteralPath "package-lock.json") {
        Write-Host "Installing Electron dependencies with npm ci"
        npm ci
      } else {
        Write-Host "Installing Electron dependencies with npm install"
        npm install
      }
      Write-Host "Building Electron GUI"
      npm run package:win
    } finally {
      Pop-Location
    }
  } finally {
    $env:npm_config_cache = $oldNpmCache
    $env:ELECTRON_CACHE = $oldElectronCache
    $env:ELECTRON_BUILDER_CACHE = $oldElectronBuilderCache
    $env:CSC_IDENTITY_AUTO_DISCOVERY = $oldCSC
  }

  $electronOut = Join-Path $repoRoot "dist\electron\win-unpacked"
  if (-not (Test-Path -LiteralPath $electronOut)) {
    throw "Electron output not found: $electronOut"
  }
  Remove-Item -LiteralPath (Join-Path $electronOut "Tailscale.exe") -Force -ErrorAction SilentlyContinue
  $appIcon = Join-Path $electronAbs "resources\app.ico"
  $rceditCandidates = @(
    (Join-Path $electronAbs "node_modules\electron-winstaller\vendor\rcedit.exe"),
    (Join-Path $electronAbs "node_modules\rcedit\bin\rcedit-x64.exe"),
    (Join-Path $electronAbs "node_modules\rcedit\bin\rcedit.exe")
  )
  $rcedit = $rceditCandidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
  if ((Test-Path -LiteralPath $appIcon) -and $rcedit) {
    Write-Host "Embedding Electron executable icon"
    & $rcedit (Join-Path $electronOut "ScaleTail.exe") "--set-icon" $appIcon
  } elseif (Test-Path -LiteralPath $appIcon) {
    Write-Warning "rcedit.exe not found; shortcut, tray, window, and installer icons will still use app.ico."
  }
  Copy-Item -LiteralPath (Join-Path $outDirAbs "tailscaled.exe") -Destination (Join-Path $electronOut "tailscaled.exe") -Force
  Copy-Item -LiteralPath (Join-Path $outDirAbs "tailscale-localapi.exe") -Destination (Join-Path $electronOut "tailscale-localapi.exe") -Force
  Copy-Item -LiteralPath (Join-Path $outDirAbs "wintun.dll") -Destination (Join-Path $electronOut "wintun.dll") -Force
}

if ($SkipInstaller) {
  Write-Host "SkipInstaller set; installer build skipped."
  exit 0
}

$iscc = $env:ISCC
if (-not $iscc) {
  $candidates = @(
    "D:\Inno Setup 6\ISCC.exe",
    "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
    "${env:ProgramFiles}\Inno Setup 6\ISCC.exe"
  )
  $iscc = $candidates | Where-Object { $_ -and (Test-Path -LiteralPath $_) } | Select-Object -First 1
}

if (-not $iscc) {
  throw "ISCC.exe not found. Install Inno Setup 6 or set the ISCC environment variable to ISCC.exe."
}

Write-Host "Building installer with $iscc"
& $iscc $InstallerScript
