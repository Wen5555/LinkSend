# Test-only clipboard transaction for a user-cleared disposable clipboard.
# Never starts with nonempty user contents. Fixture data is cleared only while
# its owner and exact known payload still match. User contents are never
# written to evidence files or printed.
Add-Type -AssemblyName System.Windows.Forms
if (-not ('LinkSendClipboardFixture' -as [type])) {
Add-Type @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
public sealed class LinkSendClipboardFixture {
 [DllImport("user32.dll")] static extern bool OpenClipboard(IntPtr owner);
 [DllImport("user32.dll")] static extern bool CloseClipboard();
 [DllImport("user32.dll")] static extern uint EnumClipboardFormats(uint previous);
 [DllImport("user32.dll")] static extern IntPtr GetClipboardData(uint format);
 [DllImport("user32.dll")] static extern IntPtr SetClipboardData(uint format,IntPtr value);
 [DllImport("user32.dll")] static extern bool EmptyClipboard();
 [DllImport("user32.dll")] static extern uint GetClipboardSequenceNumber();
 [DllImport("user32.dll")] static extern IntPtr GetClipboardOwner();
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern uint RegisterClipboardFormat(string format);
 [DllImport("kernel32.dll")] static extern UIntPtr GlobalSize(IntPtr value);
 [DllImport("kernel32.dll")] static extern IntPtr GlobalAlloc(uint flags,UIntPtr size);
 [DllImport("kernel32.dll")] static extern IntPtr GlobalLock(IntPtr value);
 [DllImport("kernel32.dll")] static extern bool GlobalUnlock(IntPtr value);
 [DllImport("kernel32.dll")] static extern IntPtr GlobalFree(IntPtr value);
 readonly IntPtr owner;
 readonly List<KeyValuePair<uint,byte[]>> original=new List<KeyValuePair<uint,byte[]>>();
 uint expected;
 uint fixtureFormat;
 byte[] fixtureBytes;
 IntPtr fixtureOwner;
 bool changed;
 public int FormatCount {get{return original.Count;}}
 public LinkSendClipboardFixture(IntPtr window) {
  owner=window;
  if(!OpenClipboard(owner))throw new Exception("CLIPBOARD_BACKUP_BUSY");
  try {
   if(EnumClipboardFormats(0)!=0)throw new Exception("CLIPBOARD_TEST_REQUIRES_EMPTY_DISPOSABLE_CLIPBOARD");
   uint format=0;ulong total=0;
   while((format=EnumClipboardFormats(format))!=0) {
    if(original.Count>=64 || format==2 || format==3 || format==9 || format==14)throw new Exception("CLIPBOARD_BACKUP_UNSUPPORTED_FORMAT");
    IntPtr value=GetClipboardData(format);ulong size=GlobalSize(value).ToUInt64();total+=size;
    if(value==IntPtr.Zero||size==0||size>67108864||total>67108864)throw new Exception("CLIPBOARD_BACKUP_UNAVAILABLE");
    IntPtr data=GlobalLock(value);if(data==IntPtr.Zero)throw new Exception("CLIPBOARD_BACKUP_UNAVAILABLE");
    try {byte[] bytes=new byte[(int)size];Marshal.Copy(data,bytes,0,bytes.Length);original.Add(new KeyValuePair<uint,byte[]>(format,bytes));}
    finally {GlobalUnlock(value);}
   }
   expected=GetClipboardSequenceNumber();
  } finally {CloseClipboard();}
 }
 void Put(uint format,byte[] bytes) {
  IntPtr memory=GlobalAlloc(0x42,(UIntPtr)bytes.Length);if(memory==IntPtr.Zero)throw new Exception("CLIPBOARD_ALLOC_FAILED");
  bool transferred=false;
  try {
   IntPtr address=GlobalLock(memory);if(address==IntPtr.Zero)throw new Exception("CLIPBOARD_ALLOC_FAILED");
   try {Marshal.Copy(bytes,0,address,bytes.Length);} finally {GlobalUnlock(memory);}
   if(SetClipboardData(format,memory)==IntPtr.Zero)throw new Exception("CLIPBOARD_SET_FAILED");
   transferred=true;
  } finally {if(!transferred)GlobalFree(memory);}
 }
 public void SetPNG(byte[] bytes){Replace(RegisterClipboardFormat("PNG"),bytes);}
 public void SetText(string value){Replace(13,System.Text.Encoding.Unicode.GetBytes(value+"\0"));}
 void Replace(uint format,byte[] bytes) {
  if(!OpenClipboard(owner))throw new Exception("CLIPBOARD_FIXTURE_BUSY");
  try {
   if(!MatchesExpected())throw new Exception("CLIPBOARD_CHANGED_BY_USER");
   if(!EmptyClipboard())throw new Exception("CLIPBOARD_EMPTY_FAILED");
   changed=true;
   Put(format,bytes); fixtureFormat=format; fixtureBytes=(byte[])bytes.Clone(); fixtureOwner=GetClipboardOwner(); expected=GetClipboardSequenceNumber();
  } finally {CloseClipboard();}
 }
 public void ConfirmAppCopy(string knownText) {
  if(!OpenClipboard(owner))throw new Exception("CLIPBOARD_VERIFY_BUSY");
  try {
   IntPtr value=GetClipboardData(13),ptr=GlobalLock(value);
   if(ptr==IntPtr.Zero)throw new Exception("COPY_RESULT_UNAVAILABLE");
   try {if(Marshal.PtrToStringUni(ptr)!=knownText)throw new Exception("COPY_RESULT_MISMATCH_OR_USER_CHANGE");}
   finally {GlobalUnlock(value);}
   changed=true;expected=GetClipboardSequenceNumber();fixtureFormat=13;fixtureBytes=System.Text.Encoding.Unicode.GetBytes(knownText+"\0");fixtureOwner=GetClipboardOwner();
  } finally {CloseClipboard();}
 }
 bool MatchesExpected() {
 if(fixtureBytes==null)return GetClipboardSequenceNumber()==expected;
 if(GetClipboardOwner()!=fixtureOwner)return false;
 IntPtr value=GetClipboardData(fixtureFormat);
 if(value==IntPtr.Zero||GlobalSize(value).ToUInt64()<(ulong)fixtureBytes.Length)return false;
 IntPtr address=GlobalLock(value);if(address==IntPtr.Zero)return false;
 try {byte[] current=new byte[fixtureBytes.Length];Marshal.Copy(address,current,0,current.Length);for(int i=0;i<current.Length;i++)if(current[i]!=fixtureBytes[i])return false;return true;}
 finally {GlobalUnlock(value);}
}
public bool Restore() {
  if(!changed)return true;
  if(!OpenClipboard(owner))throw new Exception("CLIPBOARD_RESTORE_BUSY");
  try {
   if(!MatchesExpected())return false;
   if(!EmptyClipboard())throw new Exception("CLIPBOARD_RESTORE_FAILED");
   foreach(var entry in original)Put(entry.Key,entry.Value);
   changed=false;return true;
  } finally {CloseClipboard();}
 }
}
'@
}
