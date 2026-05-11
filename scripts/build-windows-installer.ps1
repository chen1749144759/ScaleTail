param(
  [string]$OutDir = "dist\windows-amd64",
  [string]$InstallerScript = "installer\tailscale-dev.iss",
  [switch]$SkipInstaller
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

$outDirAbs = Join-Path $repoRoot $OutDir
New-Item -ItemType Directory -Force -Path $outDirAbs | Out-Null

$oldCgo = $env:CGO_ENABLED
$env:CGO_ENABLED = "0"
try {
  Write-Host "Building tailscale.exe"
  go build -trimpath -o (Join-Path $outDirAbs "tailscale.exe") ./cmd/tailscale

  Write-Host "Building tailscaled.exe"
  go build -trimpath -o (Join-Path $outDirAbs "tailscaled.exe") ./cmd/tailscaled

  Write-Host "Building tailscale-systray.exe"
  go build -trimpath -ldflags "-H=windowsgui" -o (Join-Path $outDirAbs "tailscale-systray.exe") ./cmd/systray
} finally {
  $env:CGO_ENABLED = $oldCgo
}

& (Join-Path $PSScriptRoot "ensure-wintun.ps1") -OutputDir $OutDir

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
