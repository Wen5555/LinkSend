param([Parameter(Mandatory)][string]$SessionPath,[switch]$SkipTransfers)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'm1-native-tests/uia-tools.ps1')
$s=Get-Content -LiteralPath $SessionPath -Raw | ConvertFrom-Json
$gui=Get-Process -Id $s.gui_pid
if($gui.Path -ne $s.binary){throw 'GUI_OWNERSHIP_MISMATCH'}
$root=[System.Windows.Automation.AutomationElement]::FromHandle([IntPtr]$s.hwnd)
function Click([string]$Pattern) { Invoke-NativeControl (Wait-NativeCondition -Description $Pattern -Seconds 20 -Condition {Find-NativeControl -Root $root -NamePattern $Pattern -ControlType Button}) }
function Toggle([string]$Pattern) {
 $item=Wait-NativeCondition -Description $Pattern -Condition {Find-NativeControl -Root $root -NamePattern $Pattern -ControlType CheckBox}
 ([System.Windows.Automation.TogglePattern]$item.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern)).Toggle()
}
$target=Join-Path $s.run_root 'M4 自选 目录'
[void][IO.Directory]::CreateDirectory($target)
$existing=Join-Path $target 'A 中文 冲突.txt'
if(!$SkipTransfers){[IO.File]::WriteAllText($existing,'foreign original must survive')}
$originalHash=(Get-FileHash -LiteralPath $existing).Hash
foreach($scenario in $(if($SkipTransfers){@()}else{@('subset','allskip')})) {
 $peer=Start-Process -FilePath (Join-Path $PSScriptRoot 'm4-native-peer/peer.exe') -ArgumentList @(('"'+(Join-Path $s.run_root 'fixture.json')+'"'),$scenario) -WindowStyle Hidden -PassThru -RedirectStandardError (Join-Path $s.run_root ('m4-'+$scenario+'-stderr.log')) -RedirectStandardOutput (Join-Path $s.run_root ('m4-'+$scenario+'-stdout.log'))
 try {
  $null=Wait-NativeCondition -Description 'real incoming file list' -Seconds 30 -Condition {Find-NativeControl -Root $root -NamePattern 'B 跳过.txt' -ControlType CheckBox}
  if($scenario -eq 'subset') {
   Click '^更换目录$'
   $dialog=Wait-NativeCondition -Description 'native directory picker' -Condition {Find-OwnedWindow -OwnerProcessId $gui.Id -Dialog}
   Set-OwnedDialogPath -Dialog $dialog -OwnerProcessId $gui.Id -Path $target -EditControlID 1152
   Invoke-OwnedDialogButton -Dialog $dialog -OwnerProcessId $gui.Id -ControlID 1
   $null=Wait-NativeCondition -Description 'directory selected' -Condition {-not (Find-OwnedWindow -OwnerProcessId $gui.Id -Dialog)}
   Toggle 'B 跳过.txt'
  } else {Click '^全部跳过$'}
  Click '^预览保存计划$'
  $null=Wait-NativeCondition -Description 'plan persisted and preview loaded' -Condition {Find-NativeControl -Root $root -NamePattern '^确认(接收所选内容|全部跳过)$' -ControlType Button}
  $text=Get-NativeText -Root $root
  $text | Set-Content -LiteralPath (Join-Path $s.run_root ('m4-'+$scenario+'-preview-uia.txt')) -Encoding utf8
  if($scenario -eq 'subset' -and $text -notmatch '拟保存为 A 中文 冲突 \(1\).txt'){throw 'KEEP_BOTH_PREVIEW_NOT_VISIBLE'}
  Click '^确认(接收所选内容|全部跳过)$'
  if(!$peer.WaitForExit(30000)){throw 'PEER_COMPLETION_TIMEOUT'}
  if($peer.ExitCode -ne 0){throw ('REAL_PEER_FAILED_'+$scenario)}
  $null=Wait-NativeCondition -Description 'native terminal after bilateral result' -Seconds 15 -Condition {(Get-NativeText -Root $root) -match $(if($scenario -eq 'subset'){'已完成'}else{'未接收内容|全部跳过'})}
 } finally {if(!$peer.HasExited){$peer.Kill();$peer.WaitForExit()}}
}
if((Get-FileHash -LiteralPath $existing).Hash -ne $originalHash){throw 'FOREIGN_FILE_CHANGED'}
if([IO.File]::ReadAllText($existing) -ne 'foreign original must survive'){throw 'FOREIGN_FILE_CONTENT_CHANGED'}
$saved=Join-Path $target 'A 中文 冲突 (1).txt'
if((Get-FileHash -LiteralPath $saved).Hash -ne (Get-FileHash -LiteralPath (Join-Path $s.run_root 'm4-source-subset/A 中文 冲突.txt')).Hash){throw 'SELECTED_FILE_HASH_MISMATCH'}
if((Get-FileHash -LiteralPath (Join-Path $target 'C 正常.txt')).Hash -ne (Get-FileHash -LiteralPath (Join-Path $s.run_root 'm4-source-subset/C 正常.txt')).Hash){throw 'SECOND_SELECTED_FILE_HASH_MISMATCH'}
if(Test-Path -LiteralPath (Join-Path $target 'B 跳过.txt')){throw 'SKIPPED_FILE_EXISTS'}
if(!(Test-Path -LiteralPath (Join-Path $target 'D 空目录') -PathType Container)){throw 'EMPTY_DIR_MISSING'}
foreach($name in @('A 中文 冲突.txt','B 跳过.txt','C 正常.txt','D 空目录')) {
 if(Test-Path -LiteralPath (Join-Path $s.fixture.receive_directory $name)){throw 'ALLSKIP_CREATED_CONTENT'}
}
Click '^收件箱$'
$search=Wait-NativeCondition -Description 'inbox search' -Condition {Find-NativeControl -Root $root -NamePattern '^文件名$' -ControlType Edit}
Set-NativeValue $search 'C 正常'
Click '^应用筛选$'
$null=Wait-NativeCondition -Description 'real indexed history results' -Condition {(Get-NativeText -Root $root) -match '本页\s+2\s+条'}
Read-NativeTree $root | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $s.run_root 'm6-search-uia.json') -Encoding utf8
$views=$root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,'查看文件'))
if($views.Count -ne 2){throw 'EXPECTED_TWO_HISTORY_RECORDS'}
Invoke-NativeControl $views.Item(1)
Click '^定位文件$'
$shell=New-Object -ComObject Shell.Application
try {
 $opened=Wait-NativeCondition -Description 'real Explorer selected renamed received file' -Seconds 20 -Condition {
  foreach($win in $shell.Windows()) {
   try {
    if($win.Document.Folder.Self.Path -ne $target){continue}
    $chosen=@($win.Document.SelectedItems() | ForEach-Object Path)
    if($chosen -contains $saved){return $win}
   } catch {}
  }
 }
 [ordered]@{folder=$opened.Document.Folder.Self.Path;selected=@($opened.Document.SelectedItems() | ForEach-Object Path)} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $s.run_root 'm6-explorer-reveal.json') -Encoding utf8
 $opened.Quit()
} finally {if($shell){[void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($shell)}}
$checks=$root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ControlTypeProperty,[System.Windows.Automation.ControlType]::CheckBox))
foreach($check in $checks){if($check.Current.Name -like '选择记录 *'){([System.Windows.Automation.TogglePattern]$check.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern)).Toggle()}}
Click '^删除选中记录'
Click '^只删除记录$'
$null=Wait-NativeCondition -Description 'records forgotten' -Condition {(Get-NativeText -Root $root) -match '没有匹配的记录'}
if(!(Test-Path -LiteralPath $saved) -or (Get-FileHash -LiteralPath $existing).Hash -ne $originalHash){throw 'FORGET_DELETED_USER_FILE'}
Click '^清理孤立暂存$'
Click '^确认清理暂存$'
$null=Wait-NativeCondition -Description 'cleanup result' -Condition {(Get-NativeText -Root $root) -match '本次没有清理暂存|已清理'}
if(!(Test-Path -LiteralPath $saved)){throw 'CLEANUP_DELETED_RECEIVED_FILE'}
Read-NativeTree $root | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $s.run_root 'm6-final-uia.json') -Encoding utf8
[ordered]@{sha256=$s.sha256;source=$s.source_state;native_directory='PASS';native_subset='PASS';native_keep_both='PASS';native_empty_directory='PASS';native_allskip='PASS';real_quic='loopback pinned';foreign_sha256=$originalHash;native_search='PASS';native_explorer_reveal='PASS';native_forget_preserves_files='PASS';native_staging_cleanup='PASS';target=$target;saved=$saved} | ConvertTo-Json | Tee-Object -FilePath (Join-Path $s.run_root 'm4-m6-native.json')
