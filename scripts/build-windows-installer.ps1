param(
  [string]$OutDir = "dist\windows-amd64",
  [string]$InstallerScript = "installer\scaletail.iss",
  [string]$ElectronDir = "client\electron",
  [string]$DependencyRoot = "D:\workspace-qoder\deps",
  [string]$UpdateSigningKey = $env:SCALETAIL_UPDATE_SIGNING_KEY,
  [switch]$SkipElectron,
  [switch]$SkipInstaller
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot
$electronPackage = Get-Content -Raw -Encoding UTF8 (Join-Path $repoRoot "$ElectronDir\package.json") | ConvertFrom-Json
$appVersion = [string]$electronPackage.version
if (-not $appVersion) {
  throw "Electron package version is empty."
}
$goKeySource = Get-Content -Raw -Encoding UTF8 (Join-Path $repoRoot "clientupdate\scaletailota\manifest.go")
$electronKeySource = Get-Content -Raw -Encoding UTF8 (Join-Path $repoRoot "$ElectronDir\src\main\client_update.ts")
$goPublicKey = [regex]::Match($goKeySource, 'PublicKeyBase64\s*=\s*"([^"]+)"').Groups[1].Value
$electronPublicKey = [regex]::Match($electronKeySource, 'otaPublicKeyBase64\s*=\s*"([^"]+)"').Groups[1].Value
if (-not $goPublicKey -or $goPublicKey -ne $electronPublicKey) {
  throw "The OTA public keys embedded in scaletaild and Electron do not match."
}
if (-not $UpdateSigningKey) {
  $defaultSigningKey = Join-Path $DependencyRoot "scaletail-ota\ed25519-private.key"
  if (Test-Path -LiteralPath $defaultSigningKey) {
    $UpdateSigningKey = $defaultSigningKey
  }
}

$installerOutDirAbs = Join-Path $repoRoot "dist\installer"
New-Item -ItemType Directory -Force -Path $installerOutDirAbs | Out-Null
Get-ChildItem -LiteralPath $installerOutDirAbs -Filter "ScaleTail-*-windows-amd64-setup*.exe" -ErrorAction SilentlyContinue |
  Remove-Item -Force
Get-ChildItem -LiteralPath $installerOutDirAbs -Filter "scaletail-*-windows-amd64-setup*.exe" -ErrorAction SilentlyContinue |
  Remove-Item -Force
Get-ChildItem -LiteralPath $installerOutDirAbs -Filter "ScaleTail-*.ota.json" -ErrorAction SilentlyContinue |
  Remove-Item -Force

$outDirAbs = Join-Path $repoRoot $OutDir
New-Item -ItemType Directory -Force -Path $outDirAbs | Out-Null
Remove-Item -LiteralPath `
  (Join-Path $outDirAbs "ScaleTail.exe"), `
  (Join-Path $outDirAbs "ScaleTailUI.exe"), `
  (Join-Path $outDirAbs "scaletail.exe"), `
  (Join-Path $outDirAbs "scaletaild.exe"), `
  (Join-Path $outDirAbs "scaletail-localapi.exe"), `
  (Join-Path $outDirAbs "ScaleTailUpdateHelper.exe") `
  -Force -ErrorAction SilentlyContinue

$oldCgo = $env:CGO_ENABLED
$env:CGO_ENABLED = "0"
try {
  Write-Host "Building scaletail.exe"
  go build -trimpath -o (Join-Path $outDirAbs "scaletail.exe") ./cmd/scaletail
  Write-Host "Building scaletaild.exe"
  go build -trimpath -o (Join-Path $outDirAbs "scaletaild.exe") ./cmd/scaletaild
  Write-Host "Building scaletail-localapi.exe"
  go build -trimpath -o (Join-Path $outDirAbs "scaletail-localapi.exe") ./cmd/scaletail-localapi
  Write-Host "Building ScaleTailUpdateHelper.exe"
  go build -trimpath -ldflags "-H=windowsgui" -o (Join-Path $outDirAbs "ScaleTailUpdateHelper.exe") ./cmd/scaletail-update-helper
} finally {
  $env:CGO_ENABLED = $oldCgo
}

& (Join-Path $PSScriptRoot "ensure-wintun.ps1") -OutputDir $OutDir

$electronAbs = Join-Path $repoRoot $ElectronDir
$electronOut = Join-Path $repoRoot "dist\electron\win-unpacked"
if (-not $SkipElectron) {
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

  Move-Item -LiteralPath (Join-Path $electronOut "ScaleTail.exe") -Destination (Join-Path $electronOut "ScaleTailUI.exe") -Force
  $appIcon = Join-Path $electronAbs "resources\app.ico"
  $rceditCandidates = @(
    (Join-Path $electronAbs "node_modules\electron-winstaller\vendor\rcedit.exe"),
    (Join-Path $electronAbs "node_modules\rcedit\bin\rcedit-x64.exe"),
    (Join-Path $electronAbs "node_modules\rcedit\bin\rcedit.exe")
  )
  $rcedit = $rceditCandidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
  if ((Test-Path -LiteralPath $appIcon) -and $rcedit) {
    Write-Host "Embedding Electron executable icon"
    & $rcedit (Join-Path $electronOut "ScaleTailUI.exe") "--set-icon" $appIcon
  } elseif (Test-Path -LiteralPath $appIcon) {
    Write-Warning "rcedit.exe not found; shortcut, tray, window, and installer icons will still use app.ico."
  }
}

if (-not (Test-Path -LiteralPath $electronOut)) {
  throw "Electron output not found: $electronOut"
}
Copy-Item -LiteralPath (Join-Path $outDirAbs "scaletail.exe") -Destination (Join-Path $electronOut "scaletail.exe") -Force
Copy-Item -LiteralPath (Join-Path $outDirAbs "scaletaild.exe") -Destination (Join-Path $electronOut "scaletaild.exe") -Force
Copy-Item -LiteralPath (Join-Path $outDirAbs "scaletail-localapi.exe") -Destination (Join-Path $electronOut "scaletail-localapi.exe") -Force
Copy-Item -LiteralPath (Join-Path $outDirAbs "ScaleTailUpdateHelper.exe") -Destination (Join-Path $electronOut "ScaleTailUpdateHelper.exe") -Force
Copy-Item -LiteralPath (Join-Path $outDirAbs "wintun.dll") -Destination (Join-Path $electronOut "wintun.dll") -Force

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
& $iscc "/DAppVersion=$appVersion" $InstallerScript

$installer = Get-ChildItem -LiteralPath $installerOutDirAbs -Filter "ScaleTail-$appVersion-windows-amd64-setup*.exe" |
  Sort-Object LastWriteTime -Descending |
  Select-Object -First 1
if (-not $installer) {
  throw "Installer output for version $appVersion was not found."
}
$installerHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $installer.FullName).Hash
$checksumPath = Join-Path $installerOutDirAbs "SHA256SUMS.txt"
$checksumLine = "$installerHash  $($installer.Name)`n"
[System.IO.File]::WriteAllText($checksumPath, $checksumLine, [System.Text.UTF8Encoding]::new($false))
Write-Host "Checksum list: $checksumPath"

if ($UpdateSigningKey) {
  if (-not (Test-Path -LiteralPath $UpdateSigningKey)) {
    throw "OTA signing key not found: $UpdateSigningKey"
  }
  $metadataPath = Join-Path $installerOutDirAbs "ScaleTail-$appVersion-windows-amd64.ota.json"
  Write-Host "Signing OTA metadata"
  go run ./cmd/scaletail-update-sign `
    -private-key $UpdateSigningKey `
    -file $installer.FullName `
    -version $appVersion `
    -platform windows-amd64 `
    -json-out $metadataPath
  Write-Host "OTA metadata: $metadataPath"
} else {
  Write-Warning "No OTA signing key configured. The installer is usable manually but cannot be installed silently through ScaleTail OTA."
}
