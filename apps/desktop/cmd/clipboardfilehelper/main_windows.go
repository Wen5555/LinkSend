//go:build windows && cgo

// clipboardfilehelper has one private receipt-only entry point. It does not
// publish a system clipboard object or accept an arbitrary signing request.
package main

/*
#cgo CXXFLAGS: -std=c++17 -Wall -Wextra -Wpedantic -Werror
#cgo LDFLAGS: -static -lole32 -luuid -luser32 -lstdc++ -lwinpthread
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	nc "github.com/Wen5555/LinkSend/internal/clipboardnativecontract"
	"github.com/zeebo/blake3"
	"golang.org/x/sys/windows"
)

func sourceIdentity(h windows.Handle) (nc.Node, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return nc.Node{}, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nc.Node{}, nc.ErrContract
	}
	n := nc.Node{Identity: fmt.Sprintf("windows:%x:%x:%x:%x:%x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow, info.CreationTime.HighDateTime, info.CreationTime.LowDateTime), Directory: info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0}
	if !n.Directory {
		n.Size = int64(uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow))
		n.Modified = info.LastWriteTime.Nanoseconds()
	}
	return n, nil
}

func token() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func run() error {
	if len(os.Args) != 3 || os.Args[1] != "--private-receipt-only" {
		return nc.ErrContract
	}
	rawHandle, err := strconv.ParseUint(os.Args[2], 10, 64)
	if err != nil || rawHandle == 0 {
		return nc.ErrContract
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pipe := &nc.Pipe{Handle: windows.Handle(rawHandle), Context: ctx}
	defer pipe.Close()
	if err = windows.SetHandleInformation(pipe.Handle, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return err
	}
	var p nc.Prepare
	if err = nc.Read(pipe, &p); err != nil {
		return err
	}
	var inv nc.Inventory
	challenge, err := hex.DecodeString(p.Challenge)
	if err != nil || len(challenge) != 32 || p.Version != 2 || len(p.OwnerPublic) != ed25519.PublicKeySize || len(p.Sources) == 0 || len(p.Sources) > nc.MaxSources || p.Manifest.Validate() != nil || nc.Decode(p.Inventory, &inv) != nil || inv.Version != 1 || inv.CacheID != p.Manifest.TransferID || inv.Manifest != p.Manifest.Digest() || len(p.Sources) != len(inv.Nodes)+1 {
		return nc.ErrContract
	}
	deadlineObservedAt := time.Now()
	remaining := time.Unix(0, p.DeadlineUnixNano).Sub(deadlineObservedAt)
	if remaining <= 0 || remaining > 30*time.Second {
		return nc.ErrContract
	}
	// Convert the transmitted absolute deadline once to a local monotonic
	// deadline. No phase, process exit, or later main renews this budget.
	ctx, deadlineCancel := context.WithDeadline(ctx, deadlineObservedAt.Add(remaining))
	defer deadlineCancel()
	pipe.Context = ctx
	used := map[uint64]bool{rawHandle: true}
	for _, h := range []uint64{p.LauncherHandle, p.SpoolHandle} {
		if h == 0 || used[h] {
			return nc.ErrContract
		}
		used[h] = true
		if err = windows.SetHandleInformation(windows.Handle(h), windows.HANDLE_FLAG_INHERIT, 0); err != nil {
			return err
		}
	}
	launcher := windows.Handle(p.LauncherHandle)
	defer windows.CloseHandle(launcher)
	if pid, e := windows.GetProcessId(launcher); e != nil || pid == 0 || pid == windows.GetCurrentProcessId() {
		return nc.ErrContract
	}
	if state, e := windows.WaitForSingleObject(launcher, 0); e != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return nc.ErrContract
	}
	spool := os.NewFile(uintptr(p.SpoolHandle), "private-native-receipt")
	defer spool.Close()
	spoolNode, e := sourceIdentity(windows.Handle(p.SpoolHandle))
	if e != nil || spoolNode.Directory || spoolNode.Size != 0 {
		return nc.ErrContract
	}
	entries := (*C.BrokerEntry)(C.calloc(C.size_t(len(p.Sources)), C.size_t(C.sizeof_BrokerEntry)))
	if entries == nil {
		return nc.ErrContract
	}
	defer C.free(unsafe.Pointer(entries))
	view := unsafe.Slice(entries, len(p.Sources))
	var names []unsafe.Pointer
	defer func() {
		for _, name := range names {
			C.free(name)
		}
	}()
	var nativeHandles []windows.Handle
	transferred := false
	defer func() {
		if !transferred {
			for _, h := range nativeHandles {
				windows.CloseHandle(h)
			}
		}
	}()
	manifest := map[string]int{}
	for i, entry := range p.Manifest.Files {
		manifest[entry.Path] = i
	}
	seen := map[string]bool{}
	var total int64
	for i, source := range p.Sources {
		if source.Handle == 0 || used[source.Handle] || seen[source.Name] {
			return nc.ErrContract
		}
		used[source.Handle] = true
		seen[source.Name] = true
		expected, exists := inv.Nodes[source.Name]
		if source.Name == "." {
			expected = nc.Node{Identity: p.ObjectID, Directory: true}
			exists = true
		}
		if !exists || source.Node != expected {
			return nc.ErrContract
		}
		h := windows.Handle(source.Handle)
		if err = windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
			return err
		}
		file := os.NewFile(uintptr(h), "private-native-source")
		actual, e := sourceIdentity(h)
		if e != nil || actual != source.Node {
			file.Close()
			return nc.ErrContract
		}
		if !actual.Directory {
			total += actual.Size
			if actual.Size < 0 || total > nc.MaxSourceBytes {
				file.Close()
				return nc.ErrContract
			}
			sha, b3 := sha256.New(), blake3.New()
			n, e := io.Copy(io.MultiWriter(sha, b3), io.LimitReader(file, actual.Size+1))
			if e != nil || n != actual.Size || source.SHA256 != hex.EncodeToString(sha.Sum(nil)) {
				file.Close()
				return nc.ErrContract
			}
			if index, ok := manifest[source.Name]; ok && p.Manifest.Files[index].Hash != hex.EncodeToString(b3.Sum(nil)) {
				file.Close()
				return nc.ErrContract
			}
			if _, e = file.Seek(0, io.SeekStart); e != nil {
				file.Close()
				return e
			}
		}
		var nativeHandle windows.Handle
		if err = windows.DuplicateHandle(windows.CurrentProcess(), h, windows.CurrentProcess(), &nativeHandle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
			file.Close()
			return err
		}
		nativeHandles = append(nativeHandles, nativeHandle)
		if err = file.Close(); err != nil {
			return err
		}
		name := strings.ReplaceAll(source.Name, "/", "\\")
		u := utf16.Encode([]rune(name))
		u = append(u, 0)
		cname := C.malloc(C.size_t(len(u) * 2))
		if cname == nil {
			return nc.ErrContract
		}
		names = append(names, cname)
		copyUTF := unsafe.Slice((*uint16)(cname), len(u))
		copy(copyUTF, u)
		view[i].handle = C.uintptr_t(nativeHandle)
		view[i].name = (*C.wchar_t)(cname)
		view[i].size = C.uint64_t(actual.Size)
		if actual.Directory {
			view[i].directory = 1
		}
		if index, ok := manifest[source.Name]; ok {
			entry := p.Manifest.Files[index]
			if (entry.Type == "directory") != actual.Directory || len(u) > 260 {
				return nc.ErrContract
			}
			view[i].published = 1
			delete(manifest, source.Name)
		}
	}
	if len(manifest) != 0 {
		return nc.ErrContract
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	defer clear(private)
	boot, err := nc.WindowsBootID()
	if err != nil {
		return err
	}
	launchRaw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	hello := nc.Hello{Version: 2, Challenge: p.Challenge, BootID: boot, ChildPublic: public, InventoryDigest: nc.InventoryDigest(p.Inventory), LaunchDigest: nc.SHA(launchRaw)}
	message, err := nc.HelloMessage(hello)
	if err != nil {
		return err
	}
	hello.Signature = ed25519.Sign(private, message)
	if err = nc.Write(pipe, hello); err != nil {
		return err
	}
	// The sole next command is a complete, profile-signed delegation. Neither
	// counters, raw paths, refs=0, a trace, nor a Sign(payload) command exists.
	var delegation nc.Delegation
	if err = nc.Read(pipe, &delegation); err != nil {
		return err
	}
	draw, err := json.Marshal(delegation)
	if err != nil {
		return err
	}
	if _, err = nc.VerifyDelegation(draw, p, hello); err != nil {
		return err
	}
	if current, e := nc.WindowsBootID(); e != nil || current != boot {
		return nc.ErrContract
	}
	digest, err := hex.DecodeString(nc.SHA(draw))
	if err != nil {
		return err
	}
	contextBytes := C.CBytes(digest)
	defer C.free(contextBytes)
	session := C.broker_create(entries, C.size_t(len(view)), (*C.uchar)(contextBytes), 32)
	// After this call all offered handles belong to the native process owner,
	// including a partially rejected construction. No source is deleted.
	transferred = true
	if session == nil {
		return nc.ErrContract
	}
	defer C.broker_destroy(session)
	var result C.BrokerResult
	deadline, _ := ctx.Deadline()
	milliseconds := time.Until(deadline).Milliseconds()
	if milliseconds <= 0 || milliseconds > 30000 {
		return nc.ErrContract
	}
	finished := make(chan C.int, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		waitExit := C.int(0)
		if p.WaitForLauncherExit {
			waitExit = 1
		}
		finished <- C.broker_run(session, C.uintptr_t(p.LauncherHandle), C.uint32_t(milliseconds), waitExit, &result)
	}()
	// Never destroy a session while its C++ call is active. At the original
	// hard deadline this private process exits without signing, and the OS
	// closes its own handles. No receipt grants deletion authority.
	join := func() C.int {
		select {
		case status := <-finished:
			return status
		case <-ctx.Done():
			os.Exit(1)
			return 0
		}
	}
	if p.WaitForLauncherExit {
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for C.broker_consumer_held(session) != 1 {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				os.Exit(1)
			case <-finished:
				return nc.ErrContract
			}
		}
		held := nc.Held{Version: 1, DelegationDigest: nc.SHA(draw), LaunchDigest: hello.LaunchDigest, DeadlineUnixNano: p.DeadlineUnixNano}
		message, e := nc.HeldMessage(held)
		if e != nil {
			C.broker_abort(session)
			_ = join()
			return e
		}
		held.Signature = ed25519.Sign(private, message)
		if e = nc.Write(pipe, held); e != nil {
			C.broker_abort(session)
			_ = join()
			return e
		}
	}
	status := join()
	if status != 1 {
		return nc.ErrContract
	}
	witness := C.broker_take(session, (*C.uchar)(contextBytes), 32)
	if witness == nil {
		return nc.ErrContract
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	wrong := C.CBytes(bytes.Repeat([]byte{0x55}, 32))
	defer C.free(wrong)
	if C.broker_consume(session, witness, (*C.uchar)(wrong), 32) != 0 || C.broker_consume(session, witness, (*C.uchar)(contextBytes), 32) != 1 || C.broker_consume(session, witness, (*C.uchar)(contextBytes), 32) != 0 {
		return nc.ErrContract
	}
	witnessID, err := token()
	if err != nil {
		return err
	}
	receipt := nc.Receipt{Version: 1, Purpose: "cache-native-quiescent", DelegationDigest: nc.SHA(draw), Binding: delegation.Binding, Sequence: 1, AdmissionClosed: true, SourcesClosed: true, NativeCloseWitnessID: witnessID}
	message, err = nc.ReceiptMessage(receipt)
	if err != nil {
		return err
	}
	receipt.Signature = ed25519.Sign(private, message)
	clear(private)
	body, err := json.Marshal(receipt)
	if err != nil || len(body) > 4096 {
		return nc.ErrContract
	}
	if n, e := spool.WriteAt(body, 0); e != nil || n != len(body) {
		return errors.Join(nc.ErrContract, e)
	}
	if err = spool.Sync(); err != nil {
		return err
	}
	if err = spool.Close(); err != nil {
		return err
	}
	if p.WaitForLauncherExit {
		return nil
	}
	return nc.Write(pipe, nc.Result{Version: 1, Receipt: body, ProtectedChecks: uint32(result.protected_checks), SourcesClosed: uint32(result.sources_closed)})
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}
