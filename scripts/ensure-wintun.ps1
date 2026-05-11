param(
  [string]$Version = "0.14.1",
  [string]$Sha256 = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51",
  [ValidateSet("amd64", "x86", "arm64", "arm")]
  [string]$Architecture = "amd64",
  [string]$OutputDir = "dist\windows-amd64",
  [string]$CacheDir = "dist\cache\wintun"
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$outDirAbs = Join-Path $repoRoot $OutputDir
$cacheDirAbs = Join-Path $repoRoot $CacheDir
$zipPath = Join-Path $cacheDirAbs "wintun-$Version.zip"
$extractDir = Join-Path $cacheDirAbs "wintun-$Version"
$url = "https://www.wintun.net/builds/wintun-$Version.zip"

New-Item -ItemType Directory -Force -Path $outDirAbs | Out-Null
New-Item -ItemType Directory -Force -Path $cacheDirAbs | Out-Null

if (-not (Test-Path -LiteralPath $zipPath)) {
  Write-Host "Downloading Wintun $Version from $url"
  Invoke-WebRequest -Uri $url -OutFile $zipPath
}

$actualHash = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualHash -ne $Sha256.ToLowerInvariant()) {
  Remove-Item -LiteralPath $zipPath -Force
  throw "Wintun ZIP SHA256 mismatch. Expected $Sha256, got $actualHash. The cached ZIP was deleted."
}

if (Test-Path -LiteralPath $extractDir) {
  Remove-Item -LiteralPath $extractDir -Recurse -Force
}
Expand-Archive -LiteralPath $zipPath -DestinationPath $extractDir -Force

$dll = Get-ChildItem -Path $extractDir -Recurse -File -Filter "wintun.dll" |
  Where-Object { $_.FullName -match "\\bin\\$Architecture\\wintun\.dll$" } |
  Select-Object -First 1

if (-not $dll) {
  throw "Could not find bin\$Architecture\wintun.dll in $zipPath"
}

$target = Join-Path $outDirAbs "wintun.dll"
Copy-Item -LiteralPath $dll.FullName -Destination $target -Force
Write-Host "Wintun copied to $target"
