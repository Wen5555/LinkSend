[CmdletBinding()]
param(
  [Parameter(Mandatory)][string]$ArtifactDirectory,
  [Parameter(Mandatory)][ValidatePattern('^[0-9a-f]{40}$')][string]$ExpectedCommit,
  [Parameter(Mandatory)][long]$RunId,
  [Parameter(Mandatory)][string]$Version
)
$ErrorActionPreference='Stop'
$root=(Resolve-Path -LiteralPath $ArtifactDirectory).Path
$records=[Collections.Generic.List[object]]::new()
$metadata=[Collections.Generic.List[object]]::new()
foreach($platform in @('windows-amd64','macos-arm64','macos-amd64')) {
  $dir=Join-Path $root "LinkSend-v$Version-$platform-$ExpectedCommit"
  $infoPath=Join-Path $dir 'BUILD-INFO.txt'
  $info=@{}
  foreach($line in Get-Content -LiteralPath $infoPath) { $parts=$line.Split('=',2); if($parts.Count -eq 2) {$info[$parts[0]]=$parts[1]} }
  foreach($pair in @{product_version=$Version;protocol_version='1';source_commit=$ExpectedCommit;workflow_head_sha=$ExpectedCommit;workflow_run="$RunId";source_state='COMMITTED';source_checkout_clean='true'}.GetEnumerator()) {
    if($info[$pair.Key] -ne $pair.Value) {throw "Build metadata mismatch for $platform / $($pair.Key)"}
  }
  $metadata.Add([ordered]@{platform=$platform;build_info=$info})
  foreach($line in Get-Content -LiteralPath (Join-Path $dir 'SHA256SUMS.txt')) {
    if($line -notmatch '^([0-9a-f]{64})\s+\*?(.+)$') {throw 'Invalid package checksum entry'}
    $expected=$Matches[1]
    $name=[IO.Path]::GetFileName($Matches[2].Replace('/',[IO.Path]::DirectorySeparatorChar))
    $matchingFiles=@(Get-ChildItem -LiteralPath $dir -File -Recurse | Where-Object Name -EQ $name)
    if($matchingFiles.Count -ne 1) {throw 'Package checksum does not resolve to exactly one downloaded asset'}
    $asset=$matchingFiles[0]
    $hash=(Get-FileHash -LiteralPath $asset.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    if($hash -ne $expected) {throw "Package checksum mismatch: $name"}
    $records.Add([ordered]@{name=$name;path=$asset.FullName;sha256=$hash;bytes=$asset.Length;platform=$platform})
    if($platform -eq 'windows-amd64' -and $asset.Extension -eq '.zip') {
      $expanded=Join-Path $root 'verified-windows-payload'
      if(!(Test-Path -LiteralPath $expanded)) { [IO.Compression.ZipFile]::ExtractToDirectory($asset.FullName,$expanded) }
      $zip=[IO.Compression.ZipFile]::OpenRead($asset.FullName)
      try {
        $entries=@($zip.Entries | Where-Object { $_.Name -ne '' })
        $contents=[Collections.Generic.List[object]]::new()
        foreach($entry in $entries) {
          if($entry.FullName -notin @('LinkSend.exe','README-WINDOWS-TEST.txt','BUILD-INFO.txt','SHA256SUMS.txt')) {throw 'Unexpected Windows package entry'}
          $stream=$entry.Open()
          try { $sha=[Security.Cryptography.SHA256]::Create(); try { $digest=[Convert]::ToHexString($sha.ComputeHash($stream)).ToLowerInvariant() } finally {$sha.Dispose()} } finally {$stream.Dispose()}
          $extracted=Join-Path $expanded $entry.FullName
          if((Get-FileHash -LiteralPath $extracted -Algorithm SHA256).Hash.ToLowerInvariant() -ne $digest) {throw 'Previously extracted payload changed'}
          $contents.Add([ordered]@{name=$entry.FullName;sha256=$digest;bytes=$entry.Length})
        }
      } finally {$zip.Dispose()}
      if([IO.File]::ReadAllText((Join-Path $expanded 'BUILD-INFO.txt')) -ne [IO.File]::ReadAllText($infoPath)) {throw 'Embedded Windows build metadata mismatch'}
      $exe=Get-Item -LiteralPath (Join-Path $expanded 'LinkSend.exe')
      if($exe.VersionInfo.ProductVersion -ne $Version) {throw 'Windows executable version mismatch'}
      $metadata.Add([ordered]@{platform='windows-amd64';payload=$contents.ToArray();product_version=$exe.VersionInfo.ProductVersion})
    } elseif($platform -eq 'windows-amd64' -and $asset.Extension -eq '.exe') {
      if($asset.VersionInfo.ProductVersion -ne $Version) {throw 'Windows installer version mismatch'}
    }
  }
}
if($records.Count -ne 4) {throw 'Expected exactly four release assets'}
$report=[ordered]@{commit=$ExpectedCommit;run=$RunId;version=$Version;utc=[DateTime]::UtcNow.ToString('o');assets=$records.ToArray();metadata=$metadata.ToArray()}
$report | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $root 'PACKAGE-VERIFICATION.json') -Encoding utf8
@($records | Sort-Object name | ForEach-Object { $_.sha256+'  '+$_.name }) | Set-Content -LiteralPath (Join-Path $root 'SHA256SUMS.txt') -Encoding utf8
$records | ForEach-Object { [pscustomobject]$_ } | Select-Object name,sha256,bytes | ConvertTo-Json
