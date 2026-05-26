param(
  [string]$Distro = "Ubuntu-24.04",
  [string]$Version = "0.0.1",
  [string]$OutDir = "",
  [string]$DependencyRoot = "D:\DevDeps",
  [switch]$SkipGui
)

$ErrorActionPreference = "Stop"
if (-not $OutDir) {
  $OutDir = "dist\linux-v$Version"
}

function ConvertTo-WslPath([string]$Path) {
  $resolved = if (Test-Path -LiteralPath $Path) {
    (Resolve-Path -LiteralPath $Path).Path
  } else {
    $Path
  }
  if ($resolved -match '^([A-Za-z]):\\(.*)$') {
    $drive = $matches[1].ToLowerInvariant()
    $rest = $matches[2] -replace '\\', '/'
    return "/mnt/$drive/$rest"
  }
  return $resolved
}

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$repoRoot = $repoRoot.Path
Set-Location $repoRoot

$outDirAbs = Join-Path $repoRoot $OutDir
$depRootAbs = $DependencyRoot
New-Item -ItemType Directory -Force -Path $outDirAbs | Out-Null
New-Item -ItemType Directory -Force -Path $depRootAbs | Out-Null

$repoWsl = ConvertTo-WslPath $repoRoot
$outWsl = ConvertTo-WslPath $outDirAbs
$depWsl = ConvertTo-WslPath $depRootAbs

$targets = @("linux/amd64/tgz", "linux/amd64/deb", "linux/amd64/rpm")
if (-not $SkipGui) {
  $targets += @("linux/amd64/gui-deb", "linux/amd64/gui-rpm")
}
$targetArgs = $targets -join " "

$wslBin = Join-Path $depRootAbs "wsl-bin"
New-Item -ItemType Directory -Force -Path $wslBin | Out-Null

$bootstrap = @"
set -euo pipefail
mkdir -p "$depWsl/wsl-bin" "$depWsl/wsl-home" "$depWsl/go/pkg/mod-wsl" "$depWsl/go-build-cache"
cp /bin/true "$depWsl/wsl-bin/yarn"
chmod +x "$depWsl/wsl-bin/yarn"
"@

$build = @"
set -euo pipefail
export HOME="$depWsl/wsl-home"
export GOCACHE="$depWsl/go-build-cache"
export GOMODCACHE="$depWsl/go/pkg/mod-wsl"
export PATH="$depWsl/wsl-bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
cd "$repoWsl"
rm -rf "$outWsl"
mkdir -p "$outWsl"
TS_VERSION_OVERRIDE="$Version" ./tool/go run ./cmd/dist build --out "$outWsl" $targetArgs
cd "$outWsl"
{
  find . -maxdepth 1 -type f \( -name "scaletail_${Version}_*" -o -name "scaletail-gui_${Version}_*" \) -printf "%f\n" | sort | while read -r f; do
    [ -n "`$f" ] || continue
    sha256sum "`$f"
  done
} > SHA256SUMS-linux-amd64.txt
"@

$scriptDir = Join-Path $depRootAbs "wsl-scripts"
New-Item -ItemType Directory -Force -Path $scriptDir | Out-Null
$bootstrapScript = Join-Path $scriptDir "scaletail-linux-bootstrap.sh"
$buildScript = Join-Path $scriptDir "scaletail-linux-build.sh"
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($bootstrapScript, ($bootstrap -replace "`r`n", "`n"), $utf8NoBom)
[System.IO.File]::WriteAllText($buildScript, ($build -replace "`r`n", "`n"), $utf8NoBom)

$bootstrapWsl = ConvertTo-WslPath $bootstrapScript
$buildWsl = ConvertTo-WslPath $buildScript

Write-Host "Preparing WSL dependencies in $depRootAbs"
& wsl -d $Distro -- bash $bootstrapWsl
if ($LASTEXITCODE -ne 0) {
  throw "WSL dependency preparation failed."
}

Write-Host "Building Linux packages in $outDirAbs"
& wsl -d $Distro -- bash $buildWsl
if ($LASTEXITCODE -ne 0) {
  throw "Linux package build failed."
}

Write-Host "Done. Artifacts:"
Get-ChildItem -LiteralPath $outDirAbs | Sort-Object Name | Select-Object Name, Length
