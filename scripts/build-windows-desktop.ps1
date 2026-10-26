[CmdletBinding()]
param(
  [ValidateSet('amd64','arm64')][string]$Arch = 'amd64',
  [string]$BinDirectory = 'bin',
  [string]$AppName = 'LinkSend',
  [ValidateSet('true','false')][string]$Dev = 'false',
  [string]$ExtraTags = '',
  [ValidateSet('true','false')][string]$Obfuscated = 'false',
  [string]$GarbleArgs = ''
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:OS -ne 'Windows_NT') { throw 'The private clipboard helper requires the native Windows build path.' }
$desktopRoot = (Get-Location).Path
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
if ($desktopRoot -ne (Join-Path $repoRoot 'apps/desktop')) { throw 'Run this builder from apps/desktop.' }
if ($AppName -notmatch '^[A-Za-z0-9_-]+$') { throw 'Invalid executable name.' }
$binRoot = [IO.Path]::GetFullPath((Join-Path $desktopRoot $BinDirectory))
New-Item -ItemType Directory -Force -Path $binRoot | Out-Null
$manifestPath = Join-Path $binRoot 'clipboard-helper-build.json'
$includePath = Join-Path $binRoot 'clipboard-helper-build.nsh'
# Stale metadata must never make a failed build look packageable.
foreach ($path in @($manifestPath, $includePath)) { if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force } }

function Get-SourceSnapshot {
  $commit = (& git -C $repoRoot rev-parse HEAD).Trim()
  if ($LASTEXITCODE -ne 0 -or $commit -notmatch '^[0-9a-f]{40}$') { throw 'Cannot resolve source commit.' }
  $names = @(& git -C $repoRoot ls-files --cached --others --exclude-standard)
  if ($LASTEXITCODE -ne 0) { throw 'Cannot enumerate build sources.' }
  $sources = @($names | Where-Object {
    $_ -match '^(go\.(mod|sum|work|work\.sum)$|internal/|cmd/|apps/desktop/|scripts/build-windows-desktop\.ps1$)' -and
    $_ -notmatch '^apps/desktop/(bin/|frontend/node_modules/|build/(ios|linux)/|\.task/)' -and
    $_ -notmatch '^apps/desktop/build/windows/nsis/MicrosoftEdgeWebview2Setup\.exe$'
  } | Sort-Object -Unique)
  $hash = [Security.Cryptography.SHA256]::Create()
  try {
    foreach ($name in $sources) {
      $path = Join-Path $repoRoot $name
      if (!(Test-Path -LiteralPath $path -PathType Leaf)) { throw "Build source disappeared: $name" }
      $line = [Text.Encoding]::UTF8.GetBytes($name + "`0" + (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() + "`n")
      [void]$hash.TransformBlock($line, 0, $line.Length, $line, 0)
    }
    [void]$hash.TransformFinalBlock([byte[]]::new(0), 0, 0)
    $digest = [Convert]::ToHexString($hash.Hash).ToLowerInvariant()
  } finally { $hash.Dispose() }
  $status = @(& git -C $repoRoot status --porcelain --untracked-files=normal)
  if ($LASTEXITCODE -ne 0) { throw 'Cannot read source status.' }
  # This transaction's Wails resource is a generated build input and is already
  # hashed above. All other real Git status entries remain dirty, including
  # preserved untracked user directories; Git excludes are never changed.
  $generatedResource = '?? apps/desktop/wails_windows_' + $Arch + '.syso'
  $dirty = @($status | Where-Object { $_ -cne $generatedResource })
  if ($dirty.Count -gt 0) {
    Write-Host ("Build source is dirty ({0} Git entries; showing at most 16):" -f $dirty.Count)
    $dirty | Select-Object -First 16 | ForEach-Object { Write-Host ('  ' + $_) }
  }
  return [pscustomobject]@{ commit=$commit; digest=$digest; clean=($dirty.Count -eq 0) }
}

function Assert-PE([string]$Path, [string]$ExpectedArch) {
  $stream = [IO.File]::OpenRead($Path)
  $reader = [IO.BinaryReader]::new($stream)
  try {
    if ($reader.ReadUInt16() -ne 0x5a4d) { throw 'Helper is not a PE image.' }
    $stream.Position = 0x3c
    $offset = $reader.ReadUInt32()
    if ($offset -gt $stream.Length - 26) { throw 'Invalid helper PE header.' }
    $stream.Position = $offset
    if ($reader.ReadUInt32() -ne 0x4550) { throw 'Invalid helper PE signature.' }
    $machine = $reader.ReadUInt16()
    $expected = if ($ExpectedArch -eq 'amd64') { 0x8664 } else { 0xaa64 }
    if ($machine -ne $expected) { throw 'Helper PE platform mismatch.' }
    $stream.Position = $offset + 24
    if ($reader.ReadUInt16() -ne 0x20b) { throw 'Helper must be a PE32+ executable.' }
  } finally { $reader.Dispose(); $stream.Dispose() }
}

$savedCGO = $env:CGO_ENABLED
$savedOS = $env:GOOS
$savedArch = $env:GOARCH
$temporaryHelper = Join-Path $binRoot ('clipboard-helper-' + [Guid]::NewGuid().ToString('N') + '.exe')
try {
  $source = Get-SourceSnapshot
  $env:GOOS = 'windows'; $env:GOARCH = $Arch; $env:CGO_ENABLED = '1'
  & go build -trimpath -buildvcs=false -o $temporaryHelper ./cmd/clipboardfilehelper
  if ($LASTEXITCODE -ne 0) { throw "Clipboard helper build failed ($LASTEXITCODE). A matching C/C++ compiler is required." }
  Assert-PE $temporaryHelper $Arch
  $helperBuildInfo = @(& go version -m $temporaryHelper)
  if ($LASTEXITCODE -ne 0 -or !($helperBuildInfo -match '\bpath\s+github.com/Wen5555/LinkSend/apps/desktop/cmd/clipboardfilehelper$') -or !($helperBuildInfo -match '\bCGO_ENABLED=1$') -or !($helperBuildInfo -match '\bGOOS=windows$') -or !($helperBuildInfo -match ('\bGOARCH=' + $Arch + '$'))) { throw 'Actual helper Go build metadata mismatch.' }
  $helperHash = (Get-FileHash -LiteralPath $temporaryHelper -Algorithm SHA256).Hash.ToLowerInvariant()
  $helperRelative = "native-clipboard/$helperHash/LinkSendClipboardFileHelper.exe"
  $helperPath = Join-Path $binRoot $helperRelative
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $helperPath) | Out-Null
  if (Test-Path -LiteralPath $helperPath) {
    if ((Get-FileHash -LiteralPath $helperPath -Algorithm SHA256).Hash.ToLowerInvariant() -ne $helperHash) { throw 'Existing content-addressed helper changed.' }
  } else { Copy-Item -LiteralPath $temporaryHelper -Destination $helperPath }
  $anchor = [ordered]@{version=1; target_os='windows'; target_arch=$Arch; helper_sha256=$helperHash; source_commit=$source.commit; source_digest=$source.digest; source_clean=$source.clean}
  $anchorJSON = $anchor | ConvertTo-Json -Compress
  $encoded = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($anchorJSON)).TrimEnd('=').Replace('+','-').Replace('/','_')
  $ldflags = '-X main.clipboardHelperBuildAnchor=' + $encoded
  $flags = @('-buildvcs=false')
  $tags = @()
  if ($Dev -eq 'true') { $flags += '-gcflags=all=-l' } else { $flags += '-trimpath'; $tags += 'production'; $ldflags = '-w -s -H windowsgui ' + $ldflags }
  if ($Obfuscated -eq 'true') { $tags += 'wails_obfuscated' }
  if ($ExtraTags) { $tags += $ExtraTags }
  if ($tags.Count) { $flags += @('-tags', ($tags -join ',')) }
  $flags += @('-ldflags', $ldflags, '-o', (Join-Path $binRoot "$AppName.exe"))
  $env:CGO_ENABLED = '0'
  if ($Obfuscated -eq 'true') {
    # Match the existing task's whitespace-delimited garble option semantics;
    # arguments are data and never evaluated by a second shell.
    $garbleOptions = @($GarbleArgs -split '\s+' | Where-Object { $_ })
    & garble @garbleOptions build @flags
  } else { & go build @flags }
  if ($LASTEXITCODE -ne 0) { throw "Desktop build failed ($LASTEXITCODE)." }
  $desktopBuildInfo = @(& go version -m (Join-Path $binRoot "$AppName.exe"))
  if ($LASTEXITCODE -ne 0 -or !($desktopBuildInfo -match '\bCGO_ENABLED=0$') -or !($desktopBuildInfo -match '\bGOOS=windows$') -or !($desktopBuildInfo -match ('\bGOARCH=' + $Arch + '$'))) { throw 'Actual desktop Go build metadata mismatch.' }
  $after = Get-SourceSnapshot
  if ($source.commit -ne $after.commit -or $source.digest -ne $after.digest -or $source.clean -ne $after.clean) { throw 'Build sources changed during the helper/desktop transaction; no package metadata was emitted.' }
  $manifest = [ordered]@{version=1; target_os='windows'; target_arch=$Arch; helper_sha256=$helperHash; helper_relative_path=$helperRelative; source_commit=$source.commit; source_digest=$source.digest; source_clean=$source.clean; desktop_sha256=(Get-FileHash -LiteralPath (Join-Path $binRoot "$AppName.exe") -Algorithm SHA256).Hash.ToLowerInvariant(); anchor=$encoded}
  $manifest | ConvertTo-Json | Set-Content -LiteralPath $manifestPath -Encoding utf8
  # These constants are consumed only during NSIS compilation, never by the
  # running app. No recursive wildcard can collect an old helper candidate.
  @(
    ('!define LINKSEND_CLIPBOARD_HELPER_SHA256 "' + $helperHash + '"')
    ('!define LINKSEND_CLIPBOARD_HELPER_SOURCE "' + $helperPath.Replace('$','$$') + '"')
  ) | Set-Content -LiteralPath $includePath -Encoding utf8
  Write-Output "Windows same-candidate helper/desktop built: $helperHash ($Arch), source clean=$($source.clean)"
} finally {
  $env:CGO_ENABLED=$savedCGO; $env:GOOS=$savedOS; $env:GOARCH=$savedArch
  if (Test-Path -LiteralPath $temporaryHelper) { Remove-Item -LiteralPath $temporaryHelper -Force }
}
