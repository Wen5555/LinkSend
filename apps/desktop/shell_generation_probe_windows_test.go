//go:build windows

package main

import (
	"encoding/json"
	"os"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// This helper is called only by the existing exact native-code clipboard test.
// It is a one-shot platform probe, not a production publisher or a CAS proof.
// No callback COM implementation, path, file, Shell format or source is used.
type shellGenerationProbeFormat struct {
	Format uint16
	Device uintptr
	Aspect uint32
	Index  int32
	Medium uint32
}
type shellGenerationProbeMedium struct {
	Medium  uint32
	Data    uintptr
	Release uintptr
}
type shellGenerationProbeCacheTable struct {
	ole.IUnknownVtbl
	Cache, Uncache, EnumCache, InitCache, SetData uintptr
}
type shellGenerationProbeDataTable struct {
	ole.IUnknownVtbl
	GetData, GetDataHere, QueryGetData uintptr
}
type shellGenerationProbeContender struct {
	Attempts, Acquisitions uint64
	BeforeOpened           bool
	AfterOpened            bool
	WindowFailed           bool
}
type shellGenerationProbeResult struct {
	Version                int    `json:"version"`
	MutationAttempts       int    `json:"mutation_attempts"`
	OleHRESULT             uint32 `json:"ole_hresult"`
	SequenceBefore         uint32 `json:"sequence_before"`
	SequenceAfter          uint32 `json:"sequence_after"`
	OwnLockerBefore        bool   `json:"own_locker_before"`
	OwnLockerAfter         bool   `json:"own_locker_after"`
	ContenderAttempts      uint64 `json:"contender_attempts"`
	ContenderAcquisitions  uint64 `json:"contender_acquisitions"`
	ContenderBeforeOpened  bool   `json:"contender_before_opened"`
	ContenderAfterOpened   bool   `json:"contender_after_opened"`
	OuterCloseSucceeded    bool   `json:"outer_close_succeeded"`
	CompatibleOnThisRunner bool   `json:"compatible_on_this_runner"`
}

func shellGenerationProbeWindow(user32 *windows.LazyDLL) uintptr {
	class, _ := windows.UTF16PtrFromString("STATIC")
	title, _ := windows.UTF16PtrFromString("LinkSend disposable OLE lock probe")
	owner, _, _ := user32.NewProc("CreateWindowExW").Call(0,
		uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, 0, 0, 0, 0)
	runtime.KeepAlive(class)
	runtime.KeepAlive(title)
	return owner
}

func shellGenerationProbeHR(t *testing.T, phase string, hr uintptr) {
	t.Helper()
	if int32(hr) < 0 {
		t.Fatalf("OLE lock probe setup failed phase=%s hresult=0x%08x; no publish attempted", phase, uint32(hr))
	}
}

func runHostedShellGenerationProbe(t *testing.T) {
	t.Helper()
	if os.Getenv("LINKSEND_SHELL_GENERATION_PROBE") != "1" {
		return
	}
	// Independently repeat the workflow guard before even creating the native
	// object/window. An opt-in flag alone never authorizes a local clipboard.
	for _, item := range []struct{ key, value string }{
		{"GITHUB_ACTIONS", "true"},
		{"LINKSEND_RUNNER_ENVIRONMENT", "github-hosted"},
		{"RUNNER_ENVIRONMENT", "github-hosted"},
		{"LINKSEND_RUNNER_OS", "Windows"},
		{"RUNNER_OS", "Windows"},
		{"LINKSEND_TEMP_SHARE_NATIVE_CLIPBOARD", "1"},
	} {
		if os.Getenv(item.key) != item.value {
			t.Fatal("OLE lock probe refused: verified disposable hosted Windows context required")
		}
	}
	// The caller already holds this test's STA OS-thread lock. OleInitialize,
	// all COM methods, and the sole OleSetClipboard call stay on that thread.
	ole32 := windows.NewLazySystemDLL("ole32.dll")
	user32 := windows.NewLazySystemDLL("user32.dll")
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	hr, _, _ := ole32.NewProc("OleInitialize").Call(0)
	shellGenerationProbeHR(t, "OleInitialize", hr)
	defer ole32.NewProc("OleUninitialize").Call()

	// Documented OLE implementation: CreateDataCache accepts CLSID_NULL and
	// supports IOleCache and IDataObject. Configure one known synthetic format
	// before opening the clipboard. No private OLE HWND or window-name lookup.
	var zero windows.GUID
	var cache *ole.IUnknown
	cacheIID := ole.NewGUID("{0000011E-0000-0000-C000-000000000046}")
	hr, _, _ = ole32.NewProc("CreateDataCache").Call(0,
		uintptr(unsafe.Pointer(&zero)), uintptr(unsafe.Pointer(cacheIID)),
		uintptr(unsafe.Pointer(&cache)))
	runtime.KeepAlive(cacheIID)
	shellGenerationProbeHR(t, "CreateDataCache", hr)
	if cache == nil {
		t.Fatal("OLE lock probe setup returned no data cache; no publish attempted")
	}
	defer cache.Release()
	table := (*shellGenerationProbeCacheTable)(unsafe.Pointer(cache.RawVTable))
	format := shellGenerationProbeFormat{Format: 13, Aspect: 1, Index: -1, Medium: 1}
	var connection uint32
	hr, _, _ = syscall.SyscallN(table.Cache, uintptr(unsafe.Pointer(cache)),
		uintptr(unsafe.Pointer(&format)), 1, uintptr(unsafe.Pointer(&connection)))
	shellGenerationProbeHR(t, "IOleCache.Cache", hr)
	if connection == 0 {
		t.Fatal("OLE lock probe format was not cached; no publish attempted")
	}
	text, _ := windows.UTF16FromString("LinkSend disposable OLE lock fixture")
	memory, _, _ := kernel32.NewProc("GlobalAlloc").Call(0x42, uintptr(len(text)*2))
	if memory == 0 {
		t.Fatal("OLE lock probe synthetic allocation failed; no publish attempted")
	}
	defer kernel32.NewProc("GlobalFree").Call(memory)
	address, _, _ := kernel32.NewProc("GlobalLock").Call(memory)
	if address == 0 {
		t.Fatal("OLE lock probe synthetic allocation could not be locked; no publish attempted")
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(address)), len(text)), text)
	kernel32.NewProc("GlobalUnlock").Call(memory)
	medium := shellGenerationProbeMedium{Medium: 1, Data: memory}
	// FALSE: OLE must make its own copy; this fixture always frees its HGLOBAL.
	hr, _, _ = syscall.SyscallN(table.SetData, uintptr(unsafe.Pointer(cache)),
		uintptr(unsafe.Pointer(&format)), uintptr(unsafe.Pointer(&medium)), 0)
	shellGenerationProbeHR(t, "IOleCache.SetData", hr)
	var data *ole.IUnknown
	if err := cache.PutQueryInterface(ole.NewGUID("{0000010E-0000-0000-C000-000000000046}"), &data); err != nil || data == nil {
		t.Fatal("OLE lock probe has no IDataObject; no publish attempted")
	}
	defer data.Release()
	dataTable := (*shellGenerationProbeDataTable)(unsafe.Pointer(data.RawVTable))
	hr, _, _ = syscall.SyscallN(dataTable.QueryGetData, uintptr(unsafe.Pointer(data)), uintptr(unsafe.Pointer(&format)))
	shellGenerationProbeHR(t, "IDataObject.QueryGetData", hr)

	owner := shellGenerationProbeWindow(user32)
	if owner == 0 {
		t.Fatal("OLE lock probe owner window unavailable; no publish attempted")
	}
	defer user32.NewProc("DestroyWindow").Call(owner)
	opened, _, _ := user32.NewProc("OpenClipboard").Call(owner)
	if opened == 0 {
		t.Fatal("OLE lock probe outer OpenClipboard failed; no publish attempted")
	}
	ownLock := true
	defer func() {
		if ownLock {
			user32.NewProc("CloseClipboard").Call()
		}
	}()
	lockerBefore, _, _ := user32.NewProc("GetOpenClipboardWindow").Call()
	kernel32.NewProc("SetLastError").Call(0)
	firstFormat, _, enumErr := user32.NewProc("EnumClipboardFormats").Call(0)
	if lockerBefore != owner || firstFormat != 0 || enumErr != syscall.Errno(0) {
		t.Fatal("OLE lock probe requires an unchanged empty disposable clipboard; no publish attempted")
	}
	expected, _, _ := user32.NewProc("GetClipboardSequenceNumber").Call()
	if expected == 0 {
		t.Fatal("OLE lock probe generation unavailable; no publish attempted")
	}

	ready := make(chan bool, 1)
	stop := make(chan struct{})
	done := make(chan shellGenerationProbeContender, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		var result shellGenerationProbeContender
		window := shellGenerationProbeWindow(user32)
		if window == 0 {
			result.WindowFailed = true
			ready <- false
			done <- result
			return
		}
		defer user32.NewProc("DestroyWindow").Call(window)
		tryOpen := func() bool {
			result.Attempts++
			ok, _, _ := user32.NewProc("OpenClipboard").Call(window)
			if ok != 0 {
				result.Acquisitions++
				user32.NewProc("CloseClipboard").Call()
				return true
			}
			return false
		}
		result.BeforeOpened = tryOpen()
		ready <- true
		for {
			select {
			case <-stop:
				result.AfterOpened = tryOpen()
				done <- result
				return
			default:
				_ = tryOpen()
				runtime.Gosched()
			}
		}
	}()
	stopped := false
	defer func() {
		if !stopped {
			close(stop)
			<-done
		}
	}()
	if !<-ready {
		t.Fatal("OLE lock probe contender window unavailable; no publish attempted")
	}
	// Recheck under the already-held outer lock. Never drop it or retry.
	current, _, _ := user32.NewProc("GetClipboardSequenceNumber").Call()
	if current != expected {
		t.Fatal("OLE lock probe clipboard changed before mutation; no publish attempted")
	}
	hr, _, _ = ole32.NewProc("OleSetClipboard").Call(uintptr(unsafe.Pointer(data)))
	runtime.KeepAlive(data)
	actual, _, _ := user32.NewProc("GetClipboardSequenceNumber").Call()
	lockerAfter, _, _ := user32.NewProc("GetOpenClipboardWindow").Call()
	close(stop)
	contender := <-done
	stopped = true
	result := shellGenerationProbeResult{
		Version: 1, MutationAttempts: 1, OleHRESULT: uint32(hr),
		SequenceBefore: uint32(expected), SequenceAfter: uint32(actual),
		OwnLockerBefore: lockerBefore == owner, OwnLockerAfter: lockerAfter == owner,
		ContenderAttempts: contender.Attempts, ContenderAcquisitions: contender.Acquisitions,
		ContenderBeforeOpened: contender.BeforeOpened, ContenderAfterOpened: contender.AfterOpened,
	}
	if lockerAfter == owner {
		closed, _, _ := user32.NewProc("CloseClipboard").Call()
		result.OuterCloseSucceeded = closed != 0
		ownLock = false // Record a failed close; no second close/retry follows.
	} else {
		// The OLE call lost/changed the lock. Do not close an unknown lock,
		// retry without it, clear, restore, flush, or make a second publication.
		ownLock = false
	}
	result.CompatibleOnThisRunner = int32(hr) >= 0 && result.OwnLockerAfter &&
		result.OuterCloseSucceeded && contender.Acquisitions == 0
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal("OLE lock probe result encoding failed")
	}
	t.Logf("R2_SHELL_GENERATION_PROBE %s", raw)
	// A negative HRESULT is valid probe evidence and keeps production blocked.
	// A success only describes this runner; this sampling cannot establish a
	// global compare-exchange contract or exclude a transient unlocked window.
	// Leave any synthetic publication to normal COM/process cleanup. There is
	// no user snapshot restoration or post-publication rollback in this probe.
}
