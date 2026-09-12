param([Parameter(Mandatory)][string]$SessionPath)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'uia-tools.ps1')
$session = Get-Content -LiteralPath $SessionPath -Raw | ConvertFrom-Json
$root = [IO.Path]::GetFullPath($session.run_root)
$allowed = [IO.Path]::GetFullPath($PSScriptRoot) + [IO.Path]::DirectorySeparatorChar
if (!$root.StartsWith($allowed, [StringComparison]::OrdinalIgnoreCase)) { throw 'SESSION_OUTSIDE_TEST_SCOPE' }
$gui = Get-Process -Id $session.gui_pid -ErrorAction SilentlyContinue
$guiExit = $null
$forced = $false
if ($gui) {
    if ($gui.Path -ne $session.binary -or $gui.StartTime.ToUniversalTime().Ticks -ne ([DateTimeOffset]$session.gui_started_utc).UtcDateTime.Ticks) { throw 'GUI_PROCESS_ID_REUSED' }
    $nativeProcess = [LinkSendNativeUIA]::OpenProcess(0x00101000, $false, $gui.Id)
    if ($nativeProcess -eq [IntPtr]::Zero) { throw 'OWNED_PROCESS_STATUS_HANDLE_UNAVAILABLE' }
    $window = Find-OwnedWindow -OwnerProcessId $gui.Id
    if ($window) {
        $uiaRoot = [System.Windows.Automation.AutomationElement]::FromHandle($window)
        $settings = Find-NativeControl -Root $uiaRoot -NamePattern '^设置与诊断$' -ControlType Button
        if ($settings) {
            Invoke-NativeControl $settings
            $quit = Wait-NativeCondition -Description 'owned explicit quit action' -Condition { Find-NativeControl -Root $uiaRoot -NamePattern '^退出应用$' -ControlType Button }
            Invoke-NativeControl $quit
        } else { [void][LinkSendNativeUIA]::PostMessage($window, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) }
    }
    if (!$gui.WaitForExit(15000)) { $forced = $true; $gui.Kill(); $gui.WaitForExit() }
    [uint32]$nativeExit = 259
    if (![LinkSendNativeUIA]::GetExitCodeProcess($nativeProcess, [ref]$nativeExit)) { throw 'OWNED_PROCESS_EXIT_STATUS_UNAVAILABLE' }
    [void][LinkSendNativeUIA]::CloseHandle($nativeProcess)
    $guiExit = $nativeExit
}
[IO.File]::WriteAllText((Join-Path $root 'stop-fixture'), '')
$fixture = Get-Process -Id $session.fixture_pid -ErrorAction SilentlyContinue
if ($fixture) {
    if ($fixture.StartTime.ToUniversalTime().Ticks -ne ([DateTimeOffset]$session.fixture_started_utc).UtcDateTime.Ticks -or $fixture.Path -ne (Join-Path $PSScriptRoot 'fixture.exe')) { throw 'FIXTURE_PROCESS_ID_REUSED' }
    if (!$fixture.WaitForExit(10000)) { $fixture.Kill(); $fixture.WaitForExit() }
}
$result = [ordered]@{gui_close='owned explicit quit or WM_CLOSE fallback'; gui_exit_code=$guiExit; forced_gui_termination=$forced; fixture_stopped=$true; stopped_at=[DateTime]::UtcNow.ToString('o')}
$result | ConvertTo-Json | Tee-Object -FilePath (Join-Path $root 'shutdown.json')
if ($forced -or ($null -ne $guiExit -and $guiExit -ne 0)) { exit 1 }
