param(
  [Parameter(Position = 0)]
  [ValidateSet("list", "show", "check", "export", "build")]
  [string]$Action = "list",

  [string[]]$Module = @(),

  [string]$OutDir = "dist\modules",

  [switch]$Clean
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$ModuleRoot = Join-Path $RepoRoot "modules"
$script:RegexCache = @{}

function Normalize-PathForModule([string]$Path) {
  return ($Path -replace "\\", "/").TrimStart("/")
}

function Convert-GlobToRegex([string]$Pattern) {
  $pattern = Normalize-PathForModule $Pattern
  if ($script:RegexCache.ContainsKey($pattern)) {
    return $script:RegexCache[$pattern]
  }

  $regex = [regex]::Escape($pattern)
  $regex = $regex -replace "\\\*\\\*/", "(?:.*/)?"
  $regex = $regex -replace "\\\*\\\*", ".*"
  $regex = $regex -replace "\\\*", "[^/]*"
  $regex = $regex -replace "\\\?", "[^/]"
  $regex = "^$regex$"
  $options = [System.Text.RegularExpressions.RegexOptions]::Compiled -bor [System.Text.RegularExpressions.RegexOptions]::CultureInvariant
  $compiled = New-Object System.Text.RegularExpressions.Regex -ArgumentList $regex, $options
  $script:RegexCache[$pattern] = $compiled
  return $compiled
}

function Test-ModuleGlob([string]$Path, [string]$Pattern) {
  $rel = Normalize-PathForModule $Path
  $regex = Convert-GlobToRegex $Pattern
  return $regex.IsMatch($rel)
}

function Test-AnyModuleGlob([string]$Path, $Patterns) {
  foreach ($pattern in @($Patterns)) {
    if (Test-ModuleGlob $Path $pattern) {
      return $true
    }
  }
  return $false
}

function Get-TrackedFiles {
  $files = & git -C $RepoRoot ls-files --cached --others --exclude-standard
  if ($LASTEXITCODE -ne 0) {
    throw "git ls-files failed"
  }
  return @($files | ForEach-Object { Normalize-PathForModule $_ })
}

function Get-ModuleDefinitions {
  $defs = @{}
  foreach ($file in Get-ChildItem -LiteralPath $ModuleRoot -Directory | ForEach-Object { Join-Path $_.FullName "module.json" }) {
    if (-not (Test-Path -LiteralPath $file)) {
      continue
    }
    $def = Get-Content -LiteralPath $file -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not $def.id) {
      throw "module file has no id: $file"
    }
    $defs[$def.id] = $def
  }
  return $defs
}

function Resolve-ModuleClosure($Definitions, [string[]]$ModuleIds) {
  if ($ModuleIds.Count -eq 0) {
    return @()
  }

  $ordered = New-Object System.Collections.Generic.List[string]
  $seen = @{}

  function Visit([string]$id) {
    if (-not $Definitions.ContainsKey($id)) {
      throw "unknown module: $id"
    }
    if ($seen.ContainsKey($id)) {
      return
    }
    $seen[$id] = $true
    foreach ($dep in @($Definitions[$id].dependsOn)) {
      Visit $dep
    }
    $ordered.Add($id)
  }

  foreach ($id in $ModuleIds) {
    Visit $id
  }
  return @($ordered)
}

function Get-FilesForModule($Definition, [string[]]$AllFiles) {
  $selected = New-Object System.Collections.Generic.List[string]
  foreach ($file in $AllFiles) {
    if (-not (Test-AnyModuleGlob $file $Definition.include)) {
      continue
    }
    if (Test-AnyModuleGlob $file $Definition.exclude) {
      continue
    }
    $selected.Add($file)
  }
  return @($selected)
}

function Get-FilesForModuleClosure($Definitions, [string[]]$ModuleIds, [string[]]$AllFiles) {
  $files = [ordered]@{}
  foreach ($id in $ModuleIds) {
    foreach ($file in Get-FilesForModule $Definitions[$id] $AllFiles) {
      $files[$file] = $true
    }
  }
  return @($files.Keys)
}

function Resolve-OutputPath([string]$Path) {
  if ([System.IO.Path]::IsPathRooted($Path)) {
    return [System.IO.Path]::GetFullPath($Path)
  }
  return [System.IO.Path]::GetFullPath((Join-Path $RepoRoot $Path))
}

function Assert-SafeCleanTarget([string]$Path) {
  $full = [System.IO.Path]::GetFullPath($Path)
  $repo = [System.IO.Path]::GetFullPath($RepoRoot)
  if ($full -eq $repo -or $full.StartsWith((Join-Path $repo ".git"), [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "refusing to clean unsafe output path: $full"
  }
}

$definitions = Get-ModuleDefinitions
$allFiles = Get-TrackedFiles

switch ($Action) {
  "list" {
    foreach ($id in ($definitions.Keys | Sort-Object)) {
      $def = $definitions[$id]
      $deps = @($def.dependsOn) -join ","
      if (-not $deps) { $deps = "-" }
      "{0,-8} {1,-20} deps: {2}" -f $def.id, $def.repoName, $deps
    }
  }

  "show" {
    if ($Module.Count -eq 0) {
      throw "show requires -Module"
    }
    foreach ($id in $Module) {
      if (-not $definitions.ContainsKey($id)) {
        throw "unknown module: $id"
      }
      $definitions[$id] | ConvertTo-Json -Depth 10
    }
  }

  "check" {
    $ids = if ($Module.Count -eq 0) { @($definitions.Keys | Sort-Object) } else { Resolve-ModuleClosure $definitions $Module }
    $failed = $false
    foreach ($id in $ids) {
      $def = $definitions[$id]
      $files = Get-FilesForModule $def $allFiles
      "{0}: {1} files" -f $id, $files.Count

      foreach ($pattern in @($def.include)) {
        $matches = @($allFiles | Where-Object { Test-ModuleGlob $_ $pattern })
        if ($matches.Count -eq 0) {
          Write-Warning "module '$id' include pattern matched no files: $pattern"
          $failed = $true
        }
      }
    }
    if ($failed) {
      exit 1
    }
  }

  "export" {
    if ($Module.Count -eq 0) {
      throw "export requires -Module"
    }
    $ids = Resolve-ModuleClosure $definitions $Module
    $files = Get-FilesForModuleClosure $definitions $ids $allFiles
    $target = Resolve-OutputPath $OutDir

    if ($Clean) {
      Assert-SafeCleanTarget $target
      if (Test-Path -LiteralPath $target) {
        Remove-Item -LiteralPath $target -Recurse -Force
      }
    }
    New-Item -ItemType Directory -Force -Path $target | Out-Null

    foreach ($rel in $files) {
      $src = Join-Path $RepoRoot ($rel -replace "/", [System.IO.Path]::DirectorySeparatorChar)
      $dst = Join-Path $target ($rel -replace "/", [System.IO.Path]::DirectorySeparatorChar)
      New-Item -ItemType Directory -Force -Path (Split-Path $dst) | Out-Null
      Copy-Item -LiteralPath $src -Destination $dst -Force
    }

    $summary = @(
      "ScaleTail module export",
      "modules: $($ids -join ', ')",
      "files: $($files.Count)",
      "source: $RepoRoot",
      "generated: $(Get-Date -Format o)"
    )
    Set-Content -LiteralPath (Join-Path $target "MODULE_EXPORT.txt") -Value $summary -Encoding UTF8
    "exported {0} files to {1}" -f $files.Count, $target
  }

  "build" {
    if ($Module.Count -eq 0) {
      throw "build requires -Module"
    }
    foreach ($id in $Module) {
      if (-not $definitions.ContainsKey($id)) {
        throw "unknown module: $id"
      }
      $commands = @($definitions[$id].build)
      if ($commands.Count -eq 0) {
        Write-Warning "module '$id' has no build commands"
        continue
      }
      foreach ($cmd in $commands) {
        Write-Host ">> $cmd"
        cmd /c $cmd
        if ($LASTEXITCODE -ne 0) {
          exit $LASTEXITCODE
        }
      }
    }
  }
}
