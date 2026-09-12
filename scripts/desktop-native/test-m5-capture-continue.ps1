param([Parameter(Mandatory)][string]$SessionPath)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'm1-native-tests/uia-tools.ps1')
. (Join-Path $PSScriptRoot 'clipboard-fixture.ps1')
$s=Get-Content -LiteralPath $SessionPath -Raw | ConvertFrom-Json
$h=Find-OwnedWindow -OwnerProcessId $s.gui_pid
[void][LinkSendNativeUIA]::ShowWindow($h,9);[void][LinkSendNativeUIA]::SetForegroundWindow($h)
$r=[System.Windows.Automation.AutomationElement]::FromHandle($h)
$form=[System.Windows.Forms.Form]::new()
$clip=$null
$report=Get-Content -LiteralPath (Join-Path $s.run_root 'm5-native-content-actions.json') -Raw | ConvertFrom-Json
try {
 $clip=[LinkSendClipboardFixture]::new($form.Handle)
 $clip.SetPNG([IO.File]::ReadAllBytes((Join-Path $s.run_root 'M5 原生 另存.png')))
 Invoke-NativeControl (Find-NativeControl -Root $r -NamePattern '^图片$' -ControlType Button)
 $read=Wait-NativeCondition -Description 'image capture action' -Condition {Find-NativeControl -Root $r -NamePattern '^读取图片并保存草稿$' -ControlType Button}
 Invoke-NativeControl $read
 $null=Wait-NativeCondition -Description 'immutable native PNG draft metadata' -Condition {(Get-NativeText $r) -match '3\s*×\s*2'}
 $clip.SetText('Clipboard has changed after the owned immutable PNG was captured.')
 $report.clipboard_image_capture='PASS_3_BY_2_THEN_CLIPBOARD_CHANGED'
 Get-NativeText $r | Set-Content -LiteralPath (Join-Path $s.run_root 'm5-native-captured-png-uia.txt') -Encoding utf8
} finally {
 if($clip){$report.clipboard_restored=$clip.Restore()}
 $form.Dispose()
 $report | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $s.run_root 'm5-native-content-actions.json') -Encoding utf8
}
if(!$report.clipboard_restored){throw 'USER_CLIPBOARD_CHANGED_NOT_OVERWRITTEN'}
$report | ConvertTo-Json
