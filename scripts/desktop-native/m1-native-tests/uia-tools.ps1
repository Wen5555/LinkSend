$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
if (-not ('LinkSendNativeUIA' -as [type])) {
Add-Type @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;
public static class LinkSendNativeUIA {
 public delegate bool EnumProc(IntPtr hwnd, IntPtr parameter);
 [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc callback, IntPtr parameter);
 [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr parent, EnumProc callback, IntPtr parameter);
 [DllImport("user32.dll")] public static extern int GetDlgCtrlID(IntPtr hwnd);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr hwnd, uint message, IntPtr wparam, string text);
 [DllImport("user32.dll", CharSet=CharSet.Unicode, EntryPoint="SendMessageW")] public static extern IntPtr ReadText(IntPtr hwnd, uint message, IntPtr wparam, StringBuilder text);
 [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetWindowText(IntPtr hwnd, StringBuilder name, int size);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr hwnd, StringBuilder name, int size);
 [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hwnd, int command);
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hwnd);
 [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr hwnd);
 [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr hwnd, uint message, IntPtr wparam, IntPtr lparam);
 [DllImport("kernel32.dll", SetLastError=true)] public static extern IntPtr OpenProcess(uint access, bool inherit, int pid);
 [DllImport("kernel32.dll", SetLastError=true)] public static extern bool GetExitCodeProcess(IntPtr process, out uint code);
 [DllImport("kernel32.dll")] public static extern bool CloseHandle(IntPtr handle);
 public static IntPtr[] Windows(uint pid) {
  var found = new List<IntPtr>();
  EnumWindows((hwnd,p)=>{uint owner;GetWindowThreadProcessId(hwnd,out owner);if(owner==pid) found.Add(hwnd);return true;},IntPtr.Zero);
  return found.ToArray();
 }
 public static string Title(IntPtr hwnd) {var text=new StringBuilder(2048);GetWindowText(hwnd,text,2048);return text.ToString();}
 public static string Class(IntPtr hwnd) {var text=new StringBuilder(256);GetClassName(hwnd,text,256);return text.ToString();}
 public static string ControlText(IntPtr hwnd) {var text=new StringBuilder(32768);ReadText(hwnd,0x000D,(IntPtr)32768,text);return text.ToString();}
 public static IntPtr Child(IntPtr parent, int controlID, string className) {
  IntPtr result=IntPtr.Zero;
  EnumChildWindows(parent,(hwnd,p)=>{if(GetDlgCtrlID(hwnd)==controlID && Class(hwnd)==className){result=hwnd;return false;}return true;},IntPtr.Zero);
  return result;
 }
}
'@
}

function Wait-NativeCondition {
    param([scriptblock]$Condition, [string]$Description, [int]$Seconds = 12)
    $limit = [DateTime]::UtcNow.AddSeconds($Seconds)
    do {
        $result = & $Condition
        if ($result) { return $result }
        Start-Sleep -Milliseconds 150
    } while ([DateTime]::UtcNow -lt $limit)
    throw ('UIA_TIMEOUT: ' + $Description)
}

function Find-OwnedWindow {
    param([int]$OwnerProcessId, [switch]$Dialog)
    foreach ($handle in [LinkSendNativeUIA]::Windows($OwnerProcessId)) {
        $name = [LinkSendNativeUIA]::Title($handle)
        $class = [LinkSendNativeUIA]::Class($handle)
        if (($Dialog -and $class -eq '#32770') -or (!$Dialog -and $name -eq 'LinkSend')) { return $handle }
    }
    return $null
}

function Read-NativeTree {
    param([System.Windows.Automation.AutomationElement]$Root)
    $items = $Root.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
    $result = [Collections.Generic.List[object]]::new()
    for ($index = 0; $index -lt [Math]::Min($items.Count, 700); $index++) {
        try {
            $item = $items.Item($index)
            $value = $item.Current
            $result.Add([pscustomobject][ordered]@{ name=$value.Name; automation_id=$value.AutomationId; control_type=$value.ControlType.ProgrammaticName; class=$value.ClassName; enabled=$value.IsEnabled; offscreen=$value.IsOffscreen; process_id=$value.ProcessId; rectangle=$value.BoundingRectangle.ToString(); patterns=@($item.GetSupportedPatterns() | ForEach-Object ProgrammaticName) })
        } catch [System.Windows.Automation.ElementNotAvailableException] { }
    }
    return $result.ToArray()
}

function Find-NativeControl {
    param([System.Windows.Automation.AutomationElement]$Root, [string]$NamePattern, [string]$AutomationId, [string]$ControlType)
    $items = $Root.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
    for ($index = 0; $index -lt $items.Count; $index++) {
        $item = $items.Item($index)
        try {
            $value = $item.Current
            if ($NamePattern -and $value.Name -notmatch $NamePattern) { continue }
            if ($AutomationId -and $value.AutomationId -ne $AutomationId) { continue }
            if ($ControlType -and $value.ControlType.ProgrammaticName -ne ('ControlType.' + $ControlType)) { continue }
            if (!$value.IsEnabled) { continue }
            return $item
        } catch [System.Windows.Automation.ElementNotAvailableException] { }
    }
    return $null
}

function Invoke-NativeControl {
    param([System.Windows.Automation.AutomationElement]$Element)
    if (!$Element) { throw 'UIA_CONTROL_MISSING' }
    $pattern = $null
    if ($Element.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern, [ref]$pattern)) {
        ([System.Windows.Automation.InvokePattern]$pattern).Invoke()
        return
    }
    if ($Element.TryGetCurrentPattern([System.Windows.Automation.ExpandCollapsePattern]::Pattern, [ref]$pattern)) {
        $expand = [System.Windows.Automation.ExpandCollapsePattern]$pattern
        if ($expand.Current.ExpandCollapseState -eq [System.Windows.Automation.ExpandCollapseState]::Collapsed) { $expand.Expand() } else { $expand.Collapse() }
        return
    }
    if ($Element.TryGetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern, [ref]$pattern)) {
        ([System.Windows.Automation.TogglePattern]$pattern).Toggle()
        return
    }
    throw 'UIA_CONTROL_NOT_ACTIONABLE'
}

function Set-NativeValue {
    param([System.Windows.Automation.AutomationElement]$Element, [string]$Value)
    if (!$Element) { throw 'UIA_VALUE_CONTROL_MISSING' }
    $pattern = $Element.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern)
    ([System.Windows.Automation.ValuePattern]$pattern).SetValue($Value)
}

function Get-NativeText {
    param([System.Windows.Automation.AutomationElement]$Root)
    return ((Read-NativeTree -Root $Root | ForEach-Object name) -join "`n")
}

function Set-OwnedDialogPath {
    param([IntPtr]$Dialog, [int]$OwnerProcessId, [string]$Path, [ValidateSet(1001,1148,1152)][int]$EditControlID=1148)
    [uint32]$owner = 0
    [void][LinkSendNativeUIA]::GetWindowThreadProcessId($Dialog, [ref]$owner)
    if ($owner -ne $OwnerProcessId -or [LinkSendNativeUIA]::Class($Dialog) -ne '#32770') { throw 'DIALOG_OWNER_MISMATCH' }
    $edit = Wait-NativeCondition -Description 'native filename edit ready' -Seconds 8 -Condition {
        $candidate = [LinkSendNativeUIA]::Child($Dialog, $EditControlID, 'Edit')
        if ($candidate -ne [IntPtr]::Zero) { return $candidate }
    }
    # Common Item Dialog creates its HWND before asynchronously restoring the
    # last folder. Wait for that initialization before setting the real Edit.
    Start-Sleep -Milliseconds 700
    for ($attempt=0; $attempt -lt 5; $attempt++) {
        [void][LinkSendNativeUIA]::SendMessage($edit, 0x000C, [IntPtr]::Zero, $Path)
        Start-Sleep -Milliseconds 200
        if ([LinkSendNativeUIA]::ControlText($edit) -eq $Path) { return }
    }
    throw 'NATIVE_FILENAME_VALUE_NOT_SET'
}

function Invoke-OwnedDialogButton {
    param([IntPtr]$Dialog, [int]$OwnerProcessId, [ValidateSet(1,2)][int]$ControlID)
    [uint32]$owner = 0
    [void][LinkSendNativeUIA]::GetWindowThreadProcessId($Dialog, [ref]$owner)
    if ($owner -ne $OwnerProcessId -or [LinkSendNativeUIA]::Class($Dialog) -ne '#32770') { throw 'DIALOG_OWNER_MISMATCH' }
    $button = [LinkSendNativeUIA]::Child($Dialog, $ControlID, 'Button')
    if ($button -eq [IntPtr]::Zero) { throw 'NATIVE_DIALOG_BUTTON_MISSING' }
    if (![LinkSendNativeUIA]::PostMessage($button, 0x00F5, [IntPtr]::Zero, [IntPtr]::Zero)) { throw 'NATIVE_DIALOG_BUTTON_ACTION_FAILED' }
}
