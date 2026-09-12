[CmdletBinding()]
param(
    [Parameter(Mandatory)][long]$RunId,
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-f]{40}$')][string]$ExpectedCommit,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$ArtifactNamePattern = '*',
    [ValidateRange(1, 60)][int]$DownloadTimeoutMinutes = 15
)
$ErrorActionPreference = 'Stop'
$repository = 'Wen5555/LinkSend'
$runJson = & gh api "repos/$repository/actions/runs/$RunId"
if ($LASTEXITCODE -ne 0) { throw 'Cannot read workflow metadata.' }
$run = $runJson | ConvertFrom-Json
if ($run.head_sha -ne $ExpectedCommit -or $run.status -ne 'completed' -or $run.conclusion -ne 'success') {
    throw 'Workflow must have completed successfully for the exact expected source commit.'
}
$artifactJson = & gh api "repos/$repository/actions/runs/$RunId/artifacts?per_page=100"
if ($LASTEXITCODE -ne 0) { throw 'Cannot read artifact metadata.' }
$artifacts = @((($artifactJson | ConvertFrom-Json).artifacts) | Where-Object { $_.name -like $ArtifactNamePattern })
if ($artifacts.Count -eq 0) { throw 'No matching artifacts.' }
$outputRoot = [IO.Path]::GetFullPath($OutputDirectory)
[IO.Directory]::CreateDirectory($outputRoot) | Out-Null
$token = & gh auth token
if ($LASTEXITCODE -ne 0) { throw 'GitHub authentication is unavailable.' }
$handler = [Net.Http.HttpClientHandler]::new()
$handler.AllowAutoRedirect = $false
$apiClient = [Net.Http.HttpClient]::new($handler)
$apiClient.Timeout = [TimeSpan]::FromSeconds(30)
$apiClient.DefaultRequestHeaders.UserAgent.ParseAdd('LinkSend-milestone-verifier')
$apiClient.DefaultRequestHeaders.Authorization = [Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $token.Trim())
# Separate client deliberately has no bearer header. .NET uses the configured
# Windows HTTP proxy, including for the signed Azure artifact location.
$downloadClient = [Net.Http.HttpClient]::new()
$downloadClient.Timeout = [TimeSpan]::FromMinutes($DownloadTimeoutMinutes)
$verified = [Collections.Generic.List[object]]::new()
try {
    foreach ($artifact in $artifacts) {
        if ($artifact.expired -or $artifact.name -notmatch '^[A-Za-z0-9._-]+$' -or
            $artifact.digest -notmatch '^sha256:([0-9a-f]{64})$') { throw 'Artifact metadata is not verifiable.' }
        $expectedHash = $Matches[1]
        $zipPath = Join-Path $outputRoot ($artifact.name + '.zip')
        if (-not (Test-Path -LiteralPath $zipPath)) {
            $reply = $apiClient.GetAsync("https://api.github.com/repos/$repository/actions/artifacts/$($artifact.id)/zip").GetAwaiter().GetResult()
            try {
                if ([int]$reply.StatusCode -ne 302 -or $reply.Headers.Location.Scheme -ne 'https') { throw 'Artifact redirect was not the expected HTTPS response.' }
                $signedLocation = $reply.Headers.Location
            } finally { $reply.Dispose() }
            $partialFiles = @(Get-ChildItem -LiteralPath $outputRoot -File | Where-Object { $_.Name -like ($artifact.name + '.zip.*.partial') } | Sort-Object Length -Descending)
            $temporary = if ($partialFiles.Count) { $partialFiles[0].FullName } else { $zipPath + '.' + [Guid]::NewGuid().ToString('N') + '.partial' }
            [long]$offset = if (Test-Path -LiteralPath $temporary) { (Get-Item -LiteralPath $temporary).Length } else { 0 }
            if ($offset -gt $artifact.size_in_bytes) { throw 'Partial artifact is larger than expected; preserved for inspection.' }
            Write-Output ('Downloading ' + $artifact.name)
            $deadline = [Threading.CancellationTokenSource]::new([TimeSpan]::FromMinutes($DownloadTimeoutMinutes))
            try {
              if ($offset -lt $artifact.size_in_bytes) {
                $request = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::Get, $signedLocation)
                if ($offset -gt 0) { $request.Headers.Range = [Net.Http.Headers.RangeHeaderValue]::new($offset, $null); Write-Output ('Resuming at byte ' + $offset) }
                $response = $downloadClient.SendAsync($request, [Net.Http.HttpCompletionOption]::ResponseHeadersRead, $deadline.Token).GetAwaiter().GetResult()
                try {
                    if (-not $response.IsSuccessStatusCode) { throw ('Artifact HTTP status ' + [int]$response.StatusCode) }
                    if ($offset -gt 0 -and ([int]$response.StatusCode -ne 206 -or $response.Content.Headers.ContentRange.From -ne $offset -or $response.Content.Headers.ContentRange.Length -ne $artifact.size_in_bytes)) { throw 'Server did not confirm the exact requested artifact range; partial file preserved.' }
                    $stream = $response.Content.ReadAsStreamAsync($deadline.Token).GetAwaiter().GetResult()
                    $mode = if ($offset -gt 0) { [IO.FileMode]::Append } else { [IO.FileMode]::CreateNew }
                    $file = [IO.FileStream]::new($temporary, $mode, [IO.FileAccess]::Write, [IO.FileShare]::None)
                    try { [void]$stream.CopyToAsync($file, $deadline.Token).GetAwaiter().GetResult(); $file.Flush($true) }
                    finally { $file.Dispose(); $stream.Dispose() }
                } finally { $response.Dispose(); $request.Dispose() }
              }
            } finally { $deadline.Dispose(); $signedLocation = $null }
            if ((Get-FileHash -LiteralPath $temporary -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expectedHash) { throw 'Downloaded artifact SHA256 mismatch; partial file retained for inspection.' }
            Move-Item -LiteralPath $temporary -Destination $zipPath
        }
        if ((Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expectedHash) { throw 'Existing artifact SHA256 mismatch; not overwritten.' }
        $expanded = Join-Path $outputRoot $artifact.name
        if (-not (Test-Path -LiteralPath $expanded)) {
            # ZipFile rejects entry paths escaping the extraction directory.
            [IO.Compression.ZipFile]::ExtractToDirectory($zipPath, $expanded)
        }
        $verified.Add([ordered]@{ id=$artifact.id; name=$artifact.name; sha256=$expectedHash; bytes=(Get-Item -LiteralPath $zipPath).Length; expanded=$expanded })
        Write-Output ('Verified ' + $artifact.name)
    }
    [ordered]@{ run_id=$RunId; source_commit=$ExpectedCommit; artifacts=$verified } | ConvertTo-Json -Depth 6 |
        Set-Content -LiteralPath (Join-Path $outputRoot 'artifact-verification.json') -Encoding utf8
} finally {
    $token = $null
    $apiClient.Dispose()
    $downloadClient.Dispose()
}
