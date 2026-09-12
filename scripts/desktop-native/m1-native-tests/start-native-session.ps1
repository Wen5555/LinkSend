[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$BinaryPath,
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-f]{64}$')][string]$ExpectedSha256,
    [string]$SourceState = 'UNCOMMITTED_M1_TEST_SNAPSHOT'
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'uia-tools.ps1')
$binary = (Resolve-Path -LiteralPath $BinaryPath).Path
$actualHash = (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualHash -ne $ExpectedSha256) { throw 'BINARY_HASH_MISMATCH' }
$runRoot = Join-Path $PSScriptRoot ('run-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfffZ'))
[void][IO.Directory]::CreateDirectory($runRoot)
$fixtureBinary = Join-Path $PSScriptRoot 'fixture.exe'
$fixtureProcess = Start-Process -FilePath $fixtureBinary -ArgumentList @('-root', ('"' + $runRoot + '"')) -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $runRoot 'fixture-stdout.log') -RedirectStandardError (Join-Path $runRoot 'fixture-stderr.log')
$guiProcess = $null
try {
    $fixtureFile = Join-Path $runRoot 'fixture.json'
    $null = Wait-NativeCondition -Description 'isolated pairing fixture' -Seconds 20 -Condition {
        if ($fixtureProcess.HasExited) { throw 'FIXTURE_EXITED_EARLY' }
        Test-Path -LiteralPath $fixtureFile
    }
    $fixture = Get-Content -LiteralPath $fixtureFile -Raw | ConvertFrom-Json
    if (!$fixture.profile_locks_released) { throw 'GUI_PROFILE_OWNER_NOT_RELEASED' }
    $nativeLog = Join-Path $runRoot 'native-stderr.log'
    $guiProcess = Start-Process -FilePath $binary -PassThru -WindowStyle Hidden -Environment @{
        LINKSEND_DATA_DIR=$fixture.profile_a
        LINKSEND_SERVER_URL=$fixture.server_url
        LINKSEND_BIND='127.0.0.1:0'
        LINKSEND_STUN='stun:127.0.0.1:9'
        LINKSEND_ALLOW_INSECURE_LOOPBACK='true'
    } -RedirectStandardOutput (Join-Path $runRoot 'native-stdout.log') -RedirectStandardError $nativeLog
    $handle = Wait-NativeCondition -Description 'owned Wails HWND and runtime ready' -Seconds 25 -Condition {
        $guiProcess.Refresh()
        if ($guiProcess.HasExited) { throw 'NATIVE_PROCESS_EXITED_EARLY' }
        $hwnd = Find-OwnedWindow -OwnerProcessId $guiProcess.Id
        if ($hwnd -and (Test-Path -LiteralPath $nativeLog) -and ((Get-Content -LiteralPath $nativeLog -Raw) -match 'desktop runtime ready')) { return $hwnd }
    }
    # UIA is allowed to make this freshly owned test window visible. No old
    # LinkSend window or other application is enumerated as an interaction target.
    [void][LinkSendNativeUIA]::ShowWindow($handle, 9)
    $foreground = [LinkSendNativeUIA]::SetForegroundWindow($handle)
    Start-Sleep -Milliseconds 700
    $uiaRoot = [System.Windows.Automation.AutomationElement]::FromHandle($handle)
    Read-NativeTree -Root $uiaRoot | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $runRoot 'initial-uia-tree.json') -Encoding utf8
    $session = [ordered]@{
        run_root=$runRoot; binary=$binary; sha256=$actualHash; source_state=$SourceState
        gui_pid=$guiProcess.Id; gui_started_utc=$guiProcess.StartTime.ToUniversalTime().ToString('o')
        fixture_pid=$fixtureProcess.Id; fixture_started_utc=$fixtureProcess.StartTime.ToUniversalTime().ToString('o')
        hwnd=$handle.ToInt64(); native_runtime_ready=$true; show_window='SW_RESTORE'; foreground_request=$foreground
        fixture=$fixture; created_at=[DateTime]::UtcNow.ToString('o')
    }
    $sessionFile = Join-Path $runRoot 'session.json'
    $session | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $sessionFile -Encoding utf8
    Write-Output ('SESSION_FILE=' + $sessionFile)
    Write-Output ('OWNED_GUI_PID=' + $guiProcess.Id)
    Write-Output ('OWNED_HWND=' + $handle.ToInt64())
    Write-Output ('UIA_ELEMENT_COUNT=' + @(Read-NativeTree -Root $uiaRoot).Count)
} catch {
    if ($guiProcess -and !$guiProcess.HasExited) { $guiProcess.Kill(); $guiProcess.WaitForExit() }
    [IO.File]::WriteAllText((Join-Path $runRoot 'stop-fixture'), '')
    if (!$fixtureProcess.WaitForExit(5000)) { $fixtureProcess.Kill(); $fixtureProcess.WaitForExit() }
    throw
}
