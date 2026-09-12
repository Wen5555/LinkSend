param([Parameter(Mandatory)][string]$SessionPath,[switch]$SkipImage)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'm1-native-tests/uia-tools.ps1')
. (Join-Path $PSScriptRoot 'clipboard-fixture.ps1')
$s=Get-Content -LiteralPath $SessionPath -Raw | ConvertFrom-Json
$gui=Get-Process -Id $s.gui_pid
if($gui.Path -ne $s.binary){throw 'GUI_OWNER_CHANGED'}
$hwnd=Find-OwnedWindow -OwnerProcessId $gui.Id
[void][LinkSendNativeUIA]::ShowWindow($hwnd,9);[void][LinkSendNativeUIA]::SetForegroundWindow($hwnd)
$root=[System.Windows.Automation.AutomationElement]::FromHandle($hwnd)
function Click([string]$Pattern) {Invoke-NativeControl (Wait-NativeCondition -Description $Pattern -Seconds 15 -Condition {Find-NativeControl -Root $root -NamePattern $Pattern -ControlType Button})}
function Preview([string]$Expected) {
 Click '^预览文字$'
 $dialog=Wait-NativeCondition -Description 'real native text preview' -Condition {Find-OwnedWindow -OwnerProcessId $gui.Id -Dialog}
 $tree=[System.Windows.Automation.AutomationElement]::FromHandle($dialog)
 $null=Wait-NativeCondition -Description 'exact received text in native dialog' -Condition {(Get-NativeText $tree).Contains($Expected)}
 Invoke-OwnedDialogButton -Dialog $dialog -OwnerProcessId $gui.Id -ControlID 2
 $null=Wait-NativeCondition -Description 'native preview closed' -Condition {-not (Find-OwnedWindow -OwnerProcessId $gui.Id -Dialog)}
}
$form=[System.Windows.Forms.Form]::new()
$clipboard=$null
$report=[ordered]@{source=$s.source_state;sha256=$s.sha256;received_image_save='NOT_RUN';url_preview='NOT_RUN';url_native_dispatch='NOT_RUN';text_preview='NOT_RUN';text_native_copy='NOT_RUN';clipboard_image_capture='NOT_RUN';clipboard_restored=$false}
try {
 $clipboard=[LinkSendClipboardFixture]::new($form.Handle)
 $report.original_clipboard_formats=$clipboard.FormatCount
 Click '^传输$'
 $saved=Join-Path $s.run_root 'M5 原生 另存.png'
 if(!$SkipImage){
 Click '^查看内容操作$'
 Click '^图片另存为…$'
 $dialog=Wait-NativeCondition -Description 'native image save dialog' -Condition {Find-OwnedWindow -OwnerProcessId $gui.Id -Dialog}
 Set-OwnedDialogPath -Dialog $dialog -OwnerProcessId $gui.Id -Path $saved -EditControlID 1001
 Invoke-OwnedDialogButton -Dialog $dialog -OwnerProcessId $gui.Id -ControlID 1
 $null=Wait-NativeCondition -Description 'actual saved PNG' -Condition {Test-Path -LiteralPath $saved}
 Click '^收起内容操作$'
 }
 if((Get-FileHash -LiteralPath $saved).Hash -ne (Get-FileHash -LiteralPath (Join-Path $s.fixture.receive_directory 'image.png')).Hash){throw 'NATIVE_IMAGE_SAVE_HASH_MISMATCH'}
 $report.received_image_save='PASS'
 # The three received records have image, URL, text order in the task list.
 $all=$root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,'查看内容操作'))
 if($all.Count -ne 3){throw 'EXPECTED_THREE_RECEIVED_CONTENT_TASKS'}
 Invoke-NativeControl $all.Item(1)
 Preview ($s.fixture.server_url+'/healthz')
 $report.url_preview='PASS'
 Click '^打开链接…$';Click '^确认打开$'
 $null=Wait-NativeCondition -Description 'native URL dispatch receipt' -Condition {(Get-NativeText $root) -match '已请求在浏览器中打开链接'}
 $report.url_native_dispatch='PASS_SUBMITTED'
 [void][LinkSendNativeUIA]::SetForegroundWindow($hwnd)
 Click '^收起内容操作$'
 $all=$root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,'查看内容操作'))
 Invoke-NativeControl $all.Item(2)
 $expected="M5 原生文字验收`n精确快照，不自动复制。"
 Preview $expected
 $report.text_preview='PASS'
 Click '^复制文字$'
 $null=Wait-NativeCondition -Description 'native clipboard copy receipt' -Condition {(Get-NativeText $root) -match '文字已复制到剪贴板'}
 $clipboard.ConfirmAppCopy($expected)
 $report.text_native_copy='PASS'
 $clipboard.SetPNG([IO.File]::ReadAllBytes($saved))
 Click '^图片$'
 Click '^读取图片并保存草稿$'
 $null=Wait-NativeCondition -Description 'real native image snapshot metadata' -Condition {(Get-NativeText $root) -match '3\s*×\s*2'}
 $clipboard.SetText('Clipboard changed after LinkSend captured the immutable PNG.')
 $report.clipboard_image_capture='PASS_3_BY_2_THEN_CLIPBOARD_CHANGED'
 Get-NativeText $root | Set-Content -LiteralPath (Join-Path $s.run_root 'm5-native-content-actions-uia.txt') -Encoding utf8
} finally {
 if($clipboard){$report.clipboard_restored=$clipboard.Restore()}
 $form.Dispose()
 $report | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $s.run_root 'm5-native-content-actions.json') -Encoding utf8
}
if(!$report.clipboard_restored){throw 'USER_CLIPBOARD_CHANGED_DURING_TEST_NOT_OVERWRITTEN'}
$report | ConvertTo-Json
