//go:build windows

package nativeclipboard

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

const wmClipboardUpdate = 0x031D

var (
	clipboardWatchID atomic.Uint64
	clipboardWatches sync.Map
	comctl32         = windows.NewLazySystemDLL("comctl32.dll")
	setSubclass      = comctl32.NewProc("SetWindowSubclass")
	removeSubclass   = comctl32.NewProc("RemoveWindowSubclass")
	defSubclass      = comctl32.NewProc("DefSubclassProc")
	addListener      = user32.NewProc("AddClipboardFormatListener")
	removeListener   = user32.NewProc("RemoveClipboardFormatListener")
	getSequence      = user32.NewProc("GetClipboardSequenceNumber")
	watchCallback    = windows.NewCallback(clipboardSubclassProc)
)

type windowsClipboardWatch struct {
	id      uintptr
	hwnd    uintptr
	notify  func(Change)
	changes chan Change
	cancel  context.CancelFunc
	done    chan struct{}
	urlW    uintptr
	urlA    uintptr
	once    sync.Once
}

func clipboardSubclassProc(hwnd uintptr, message uint32, wParam, lParam, subclassID, _ uintptr) uintptr {
	if message == wmClipboardUpdate {
		if value, ok := clipboardWatches.Load(subclassID); ok {
			watch := value.(*windowsClipboardWatch)
			change := watch.snapshot()
			select {
			case watch.changes <- change:
			default:
			}
		}
	}
	result, _, _ := defSubclass.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (w *windowsClipboardWatch) snapshot() Change {
	sequence, _, _ := getSequence.Call()
	available := func(format uintptr) bool {
		ok, _, _ := clipboardFormatAvailable.Call(format)
		return ok != 0
	}
	return Change{
		Sequence: uint64(uint32(sequence)),
		Text:     available(13) || available(1), // CF_UNICODETEXT / CF_TEXT
		Link:     (w.urlW != 0 && available(w.urlW)) || (w.urlA != 0 && available(w.urlA)),
		Image:    available(2) || available(8) || available(17), // CF_BITMAP / CF_DIB / CF_DIBV5
	}
}

func watchChanges(ctx context.Context, nativeWindow uintptr, notify func(Change)) (func(), error) {
	if nativeWindow == 0 {
		return nil, errors.New("CLIPBOARD_NATIVE_WINDOW_REQUIRED")
	}
	watchCtx, cancel := context.WithCancel(ctx)
	id := uintptr(clipboardWatchID.Add(1))
	watch := &windowsClipboardWatch{id: id, hwnd: nativeWindow, notify: notify, changes: make(chan Change, 1), cancel: cancel, done: make(chan struct{})}
	register := func(name string) uintptr {
		value, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return 0
		}
		format, _, _ := registerClipboardFormat.Call(uintptr(unsafe.Pointer(value)))
		return format
	}
	watch.urlW, watch.urlA = register("UniformResourceLocatorW"), register("UniformResourceLocator")
	clipboardWatches.Store(id, watch)
	if ok, _, callErr := setSubclass.Call(nativeWindow, watchCallback, id, 0); ok == 0 {
		clipboardWatches.Delete(id)
		cancel()
		return nil, errors.Join(ErrUnavailable, callErr)
	}
	if ok, _, callErr := addListener.Call(nativeWindow); ok == 0 {
		removeSubclass.Call(nativeWindow, watchCallback, id)
		clipboardWatches.Delete(id)
		cancel()
		return nil, errors.Join(ErrUnavailable, callErr)
	}
	go func() {
		defer close(watch.done)
		for {
			select {
			case <-watchCtx.Done():
				return
			case change := <-watch.changes:
				watch.notify(change)
			}
		}
	}()
	return func() {
		watch.once.Do(func() {
			removeListener.Call(nativeWindow)
			removeSubclass.Call(nativeWindow, watchCallback, id)
			clipboardWatches.Delete(id)
			cancel()
			<-watch.done
		})
	}, nil
}
