//go:build windows

package clipboardnativecontract

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Pipe is a single-owner private inherited endpoint. Every operation is
// overlapped and cancellation is joined before its pinned buffers are released.
type Pipe struct {
	Handle  windows.Handle
	Context context.Context
}

func (p *Pipe) Read(b []byte) (int, error)  { return p.io(b, false) }
func (p *Pipe) Write(b []byte) (int, error) { return p.io(b, true) }
func (p *Pipe) Close() error                { return windows.CloseHandle(p.Handle) }
func (p *Pipe) io(b []byte, write bool) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if err := p.Context.Err(); err != nil {
		return 0, err
	}
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(event)
	ov := windows.Overlapped{HEvent: event}
	var pin runtime.Pinner
	pin.Pin(&b[0])
	pin.Pin(&ov)
	defer pin.Unpin()
	var done uint32
	if write {
		err = windows.WriteFile(p.Handle, b, &done, &ov)
	} else {
		err = windows.ReadFile(p.Handle, b, &done, &ov)
	}
	if err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
		return 0, err
	}
	if errors.Is(err, windows.ERROR_IO_PENDING) {
		for {
			state, waitErr := windows.WaitForSingleObject(event, 10)
			if waitErr != nil || state == windows.WAIT_FAILED {
				_ = windows.CancelIoEx(p.Handle, &ov)
				_ = windows.GetOverlappedResult(p.Handle, &ov, &done, true)
				return 0, errors.Join(ErrContract, waitErr)
			}
			if state == windows.WAIT_OBJECT_0 {
				break
			}
			if cancelErr := p.Context.Err(); cancelErr != nil {
				_ = windows.CancelIoEx(p.Handle, &ov)
				_ = windows.GetOverlappedResult(p.Handle, &ov, &done, true)
				return 0, cancelErr
			}
		}
		err = windows.GetOverlappedResult(p.Handle, &ov, &done, false)
	}
	return int(done), err
}

// NewPipePair creates both ends in the owner, before launching the helper.
// The random, local-only, first-instance name is never sent to the helper or
// opened by a third process. Only the already-connected client is inheritable.
func NewPipePair(ctx context.Context) (*Pipe, windows.Handle, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, 0, err
	}
	name, err := windows.UTF16PtrFromString(`\\.\pipe\LinkSend-private-native-` + hex.EncodeToString(token[:]))
	if err != nil {
		return nil, 0, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, 0, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return nil, 0, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	server, err := windows.CreateNamedPipe(name, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED|windows.FILE_FLAG_FIRST_PIPE_INSTANCE, windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS, 1, MaxFrame+4, MaxFrame+4, 0, &sa)
	if err != nil {
		return nil, 0, err
	}
	ok := false
	defer func() {
		if !ok {
			windows.CloseHandle(server)
		}
	}()
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, 0, err
	}
	defer windows.CloseHandle(event)
	ov := windows.Overlapped{HEvent: event}
	var pin runtime.Pinner
	pin.Pin(&ov)
	defer pin.Unpin()
	err = windows.ConnectNamedPipe(server, &ov)
	if !errors.Is(err, windows.ERROR_IO_PENDING) {
		return nil, 0, errors.Join(ErrContract, err)
	}
	client, openErr := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if openErr != nil {
		_ = windows.CancelIoEx(server, &ov)
		var n uint32
		_ = windows.GetOverlappedResult(server, &ov, &n, true)
		return nil, 0, openErr
	}
	var n uint32
	if err = windows.GetOverlappedResult(server, &ov, &n, true); err != nil {
		windows.CloseHandle(client)
		return nil, 0, err
	}
	if err = windows.SetHandleInformation(client, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
		windows.CloseHandle(client)
		return nil, 0, err
	}
	ok = true
	return &Pipe{server, ctx}, client, nil
}
