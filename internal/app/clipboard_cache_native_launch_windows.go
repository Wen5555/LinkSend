//go:build windows

package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"unicode/utf16"
	"unsafe"

	nc "github.com/Wen5555/LinkSend/internal/clipboardnativecontract"
	"github.com/zeebo/blake3"
	"golang.org/x/sys/windows"
)

// This private primitive has no default caller, Wails binding or installed
// image discovery. Its expected digest must come from a verified build/release
// trust anchor; a helper's own path/hash/JSON is never an authority.
type cacheNativeWindowsLaunch struct {
	r                           *clipboardCacheReceipt
	image                       *os.File
	imageHash                   string
	process                     windows.Handle
	pipe                        *nc.Pipe
	prepare                     nc.Prepare
	hello                       nc.Hello
	partial                     cacheNativeBinding
	verified, committed, closed bool
}

func cacheNativeFinalPath(handle windows.Handle) (string, error) {
	buf := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], uint32(len(buf)), 0)
	if err != nil || n == 0 || n >= uint32(len(buf)) {
		return "", errors.Join(errClipboardCacheOwnership, err)
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// Pin the same actual file while denying future write/delete opens. Ancestor
// path races are harmless only after the returned identity is checked again.
func cacheNativePinFile(file *os.File, write bool) (*os.File, error) {
	path, err := cacheNativeFinalPath(windows.Handle(file.Fd()))
	if err != nil {
		return nil, err
	}
	w, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.GENERIC_READ)
	if write {
		access |= windows.GENERIC_WRITE
	}
	share := uint32(windows.FILE_SHARE_READ)
	if write {
		share |= windows.FILE_SHARE_WRITE
	}
	h, err := windows.CreateFile(w, access, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	pinned := os.NewFile(uintptr(h), "private-native-pinned")
	a, e1 := inboxFileIdentity(file)
	b, e2 := inboxFileIdentity(pinned)
	var info windows.ByHandleFileInformation
	e3 := windows.GetFileInformationByHandle(h, &info)
	if e1 != nil || e2 != nil || e3 != nil || a != b || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		_ = pinned.Close()
		return nil, errClipboardCacheOwnership
	}
	return pinned, nil
}

func cacheNativeDuplicate(handle windows.Handle, access uint32) (windows.Handle, error) {
	var result windows.Handle
	err := windows.DuplicateHandle(windows.CurrentProcess(), handle, windows.CurrentProcess(), &result, access, true, 0)
	return result, err
}

func (r *clipboardCacheReceipt) prepareCacheNativeWindows(ctx context.Context, imagePath, expectedHash string) (launch *cacheNativeWindowsLaunch, err error) {
	if r == nil || r.service == nil || r.destination == nil || r.destination.object == nil || r.record.Manifest == nil || r.record.State != "verified" || r.record.Native != nil || r.record.Run != r.service.epoch || len(r.finalInventory) == 0 ||
		!filepath.IsAbs(imagePath) || !validClipboardFileID(expectedHash, 32) {
		return nil, errClipboardCacheOwnership
	}
	done, workErr := r.service.beginProfileWork()
	if workErr != nil {
		return nil, workErr
	}
	defer done()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(r.service.workCtx, cancel)
	defer stop()
	launch = &cacheNativeWindowsLaunch{r: r, imageHash: expectedHash}
	defer func() {
		if err != nil {
			_ = launch.close()
		}
	}()
	file, e := os.Open(imagePath)
	if e != nil {
		return launch, e
	}
	launch.image, err = cacheNativePinFile(file, false)
	_ = file.Close()
	if err != nil {
		return launch, err
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, io.LimitReader(launch.image, 64<<20)); err != nil {
		return launch, err
	}
	info, e := launch.image.Stat()
	if e != nil || info.Size() <= 0 || info.Size() > 64<<20 || hex.EncodeToString(hash.Sum(nil)) != expectedHash {
		return launch, errClipboardCacheOwnership
	}
	if err = r.destination.visible(); err != nil {
		return launch, err
	}
	sealed := r.record
	sealed.Sealed = true
	sealed.Cleanup = r.finalInventory
	if err = checkClipboardCacheTree(ctx, r.destination.object, sealed.Cleanup, false); err != nil {
		return launch, err
	}
	invRaw, err := json.Marshal(nc.Inventory{Version: 1, CacheID: sealed.ID, Manifest: sealed.Digest, Plan: sealed.PlanDigest, Stage: sealed.Stage, Parts: sealed.Parts, Nodes: cacheNativeContractNodes(sealed.Cleanup)})
	if err != nil {
		return launch, err
	}
	digest, e := cacheNativeInventoryDigest(sealed)
	if e != nil || digest != nc.InventoryDigest(invRaw) {
		return launch, errClipboardCacheOwnership
	}
	challenge, e := randomCacheNativeHex(32)
	if e != nil {
		return launch, e
	}
	launch.prepare = nc.Prepare{Version: 1, Challenge: challenge, OwnerPublic: r.service.identity.PublicKey(), ObjectID: sealed.Directory, Inventory: invRaw, Manifest: *sealed.Manifest}
	var inherited []windows.Handle
	defer func() {
		for _, h := range inherited {
			_ = windows.CloseHandle(h)
		}
	}()
	var total int64
	names := make([]string, 0, len(sealed.Cleanup)+1)
	names = append(names, ".")
	for name := range sealed.Cleanup {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > nc.MaxSources {
		return launch, errClipboardCacheOwnership
	}
	manifestFiles := map[string]string{}
	for _, entry := range sealed.Manifest.Files {
		if len(utf16.Encode([]rune(entry.Path))) >= 260 {
			return launch, errClipboardCacheOwnership
		}
		if entry.Type == "file" {
			manifestFiles[entry.Path] = entry.Hash
		}
	}
	for _, name := range names {
		node := sealed.Cleanup[name]
		if name == "." {
			node = clipboardCacheNode{Identity: sealed.Directory, Directory: true}
		}
		var source *os.File
		if name == "." {
			source, err = r.destination.object.Open(".")
		} else {
			parent, openErr := openCacheParent(r.destination.object, filepath.ToSlash(filepath.Dir(name)), sealed.Cleanup)
			if openErr != nil {
				return launch, openErr
			}
			source, err = parent.Open(filepath.Base(name))
			_ = parent.Close()
		}
		if err != nil {
			return launch, err
		}
		if !node.Directory {
			pinned, pinErr := cacheNativePinFile(source, false)
			_ = source.Close()
			if pinErr != nil {
				return launch, pinErr
			}
			source = pinned
		}
		actual, nodeErr := cacheNode(source)
		if nodeErr != nil || actual != node {
			_ = source.Close()
			return launch, errClipboardCacheOwnership
		}
		entry := nc.Source{Name: name, Node: nc.Node{Identity: node.Identity, Directory: node.Directory, Size: node.Size, Modified: node.Modified}}
		access := uint32(windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE)
		if !node.Directory {
			total += node.Size
			if total > nc.MaxSourceBytes {
				_ = source.Close()
				return launch, errClipboardCacheOwnership
			}
			sha, b3 := sha256.New(), blake3.New()
			count, readErr := io.Copy(io.MultiWriter(sha, b3), io.LimitReader(source, node.Size+1))
			if readErr != nil || count != node.Size {
				_ = source.Close()
				return launch, errClipboardCacheOwnership
			}
			entry.SHA256 = hex.EncodeToString(sha.Sum(nil))
			if expected, ok := manifestFiles[name]; ok && expected != hex.EncodeToString(b3.Sum(nil)) {
				_ = source.Close()
				return launch, errClipboardCacheOwnership
			}
			if _, seekErr := source.Seek(0, io.SeekStart); seekErr != nil {
				_ = source.Close()
				return launch, seekErr
			}
			access |= windows.FILE_READ_DATA
		}
		h, dupErr := cacheNativeDuplicate(windows.Handle(source.Fd()), access)
		closeErr := source.Close()
		if dupErr != nil || closeErr != nil {
			if dupErr == nil {
				_ = windows.CloseHandle(h)
			}
			return launch, errors.Join(dupErr, closeErr)
		}
		inherited = append(inherited, h)
		entry.Handle = uint64(h)
		launch.prepare.Sources = append(launch.prepare.Sources, entry)
	}
	spool, e := r.service.openCacheNativeSpool(sealed.ID, true)
	if e != nil {
		return launch, e
	}
	pinnedSpool, e := cacheNativePinFile(spool, true)
	_ = spool.Close()
	if e != nil {
		return launch, e
	}
	spoolHandle, e := cacheNativeDuplicate(windows.Handle(pinnedSpool.Fd()), windows.FILE_WRITE_DATA|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE)
	closeErr := pinnedSpool.Close()
	if e != nil || closeErr != nil {
		if e == nil {
			_ = windows.CloseHandle(spoolHandle)
		}
		return launch, errors.Join(e, closeErr)
	}
	inherited = append(inherited, spoolHandle)
	launch.prepare.SpoolHandle = uint64(spoolHandle)
	lifetime, e := cacheNativeDuplicate(windows.CurrentProcess(), windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if e != nil {
		return launch, e
	}
	inherited = append(inherited, lifetime)
	launch.prepare.LauncherHandle = uint64(lifetime)
	pipe, childEndpoint, e := nc.NewPipePair(ctx)
	if e != nil {
		return launch, e
	}
	launch.pipe = pipe
	inherited = append(inherited, childEndpoint)
	attributes, e := windows.NewProcThreadAttributeList(1)
	if e != nil {
		return launch, e
	}
	defer attributes.Delete()
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&inherited[0]), uintptr(len(inherited))*unsafe.Sizeof(inherited[0])); err != nil {
		return launch, err
	}
	exact, e := cacheNativeFinalPath(windows.Handle(launch.image.Fd()))
	if e != nil {
		return launch, e
	}
	app, e := windows.UTF16PtrFromString(exact)
	if e != nil {
		return launch, e
	}
	command, e := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{exact, "--private-receipt-only", strconv.FormatUint(uint64(childEndpoint), 10)}))
	if e != nil {
		return launch, e
	}
	startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))}, ProcThreadAttributeList: attributes.List()}
	var process windows.ProcessInformation
	if err = windows.CreateProcess(app, command, nil, nil, true, windows.CREATE_NO_WINDOW|windows.CREATE_SUSPENDED|windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, &process); err != nil {
		return launch, err
	}
	launch.process = process.Process
	defer windows.CloseHandle(process.Thread)
	// No child code runs until its process image has been checked against the
	// original, still-pinned image. Never reopen a process using its PID.
	imageName := make([]uint16, 32768)
	size := uint32(len(imageName))
	if err = windows.QueryFullProcessImageName(launch.process, 0, &imageName[0], &size); err != nil {
		return launch, err
	}
	actualImage, e := os.Open(windows.UTF16ToString(imageName[:size]))
	if e != nil {
		return launch, e
	}
	a, e1 := inboxFileIdentity(launch.image)
	b, e2 := inboxFileIdentity(actualImage)
	_ = actualImage.Close()
	if e1 != nil || e2 != nil || a != b {
		return launch, errClipboardCacheOwnership
	}
	if _, err = windows.ResumeThread(process.Thread); err != nil {
		return launch, err
	}
	// Child inherited copies now own these resources; close the parent's
	// copies before expecting its source-close witness.
	for _, h := range inherited {
		if err = windows.CloseHandle(h); err != nil {
			return launch, err
		}
	}
	inherited = nil
	if err = nc.Write(pipe, launch.prepare); err != nil {
		return launch, err
	}
	if err = nc.Read(pipe, &launch.hello); err != nil {
		return launch, err
	}
	hello := launch.hello
	message, e := nc.HelloMessage(hello)
	boot, e2 := cacheNativeWindowsBoot{}.cacheNativeBootID()
	if e != nil || e2 != nil || hello.Version != 1 || hello.Challenge != challenge || hello.BootID != boot || hello.InventoryDigest != digest || len(hello.ChildPublic) != ed25519.PublicKeySize || !ed25519.Verify(hello.ChildPublic, message, hello.Signature) {
		return launch, errClipboardCacheOwnership
	}
	if state, e := windows.WaitForSingleObject(launch.process, 0); e != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return launch, errClipboardCacheOwnership
	}
	return launch, nil
}

func cacheNativeContractNodes(nodes map[string]clipboardCacheNode) map[string]nc.Node {
	out := make(map[string]nc.Node, len(nodes))
	for name, n := range nodes {
		out[name] = nc.Node{Identity: n.Identity, Directory: n.Directory, Size: n.Size, Modified: n.Modified}
	}
	return out
}

func (p *cacheNativeWindowsLaunch) verifyCacheNativeLaunch(ctx context.Context, b cacheNativeBinding) (cacheNativeLaunchIdentity, error) {
	if p == nil || p.closed || p.committed || p.verified || ctx.Err() != nil {
		return cacheNativeLaunchIdentity{}, errClipboardCacheOwnership
	}
	r := p.r.record
	if b.Owner != r.Owner || b.OwnerRun != r.Run || b.CacheID != r.ID || b.CacheRootID != r.CacheRoot || b.ObjectID != r.Directory || b.StageID != r.Stage || b.ManifestDigest != r.Digest || b.PlanDigest != r.PlanDigest || b.InventoryDigest != p.hello.InventoryDigest {
		return cacheNativeLaunchIdentity{}, errClipboardCacheOwnership
	}
	if state, e := windows.WaitForSingleObject(p.process, 0); e != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return cacheNativeLaunchIdentity{}, errClipboardCacheOwnership
	}
	p.partial = b
	p.verified = true
	return cacheNativeLaunchIdentity{BootID: p.hello.BootID, HelperImageSHA256: p.imageHash, ChildEphemeralPublicKey: bytes.Clone(p.hello.ChildPublic)}, nil
}

func (p *cacheNativeWindowsLaunch) commit(ctx context.Context, d cacheNativeDelegation) (nc.Result, error) {
	var result nc.Result
	if p == nil || p.closed || p.committed || !p.verified || ctx.Err() != nil {
		return result, errClipboardCacheOwnership
	}
	done, workErr := p.r.service.beginProfileWork()
	if workErr != nil {
		return result, workErr
	}
	defer done()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(p.r.service.workCtx, cancel)
	defer stop()
	raw, err := json.Marshal(d)
	if err != nil {
		return result, err
	}
	parsed, err := nc.VerifyDelegation(raw, p.prepare, p.hello)
	if err != nil {
		return result, err
	}
	expected := p.partial
	expected.BootID = p.hello.BootID
	expected.HelperImageSHA256 = p.imageHash
	expected.ChildEphemeralPublicKey = p.hello.ChildPublic
	a, _ := json.Marshal(expected)
	b, _ := json.Marshal(parsed.Binding)
	if !bytes.Equal(a, b) {
		return result, errClipboardCacheOwnership
	}
	// Re-read the actual durable authorization. A caller's delegation alone
	// cannot cause COM exposure, including after a failed/uncertain CAS.
	var durable string
	if err = p.r.service.store.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key=?`, clipboardCacheKeyPrefix+p.r.record.ID).Scan(&durable); err != nil {
		return result, err
	}
	record, e := p.r.service.decodeCacheRecord(p.r.record.ID, durable)
	if e != nil || durable != p.r.raw || record.Native == nil {
		return result, errClipboardCacheOwnership
	}
	actual, _ := json.Marshal(record.Native.Delegation)
	if !bytes.Equal(actual, raw) {
		return result, errClipboardCacheOwnership
	}
	if err = p.r.destination.visible(); err != nil {
		return result, err
	}
	if err = checkClipboardCacheTree(ctx, p.r.destination.object, record.Cleanup, false); err != nil {
		return result, err
	}
	p.pipe.Context = ctx
	p.committed = true
	if err = nc.Write(p.pipe, d); err != nil {
		return result, err
	}
	if err = nc.Read(p.pipe, &result); err != nil {
		return result, err
	}
	if result.Version != 1 || result.ProtectedChecks != 0x7f || result.SourcesClosed != uint32(len(p.prepare.Sources)) {
		return result, errClipboardCacheOwnership
	}
	if _, err = verifyCacheNativeCloseReceipt(d, result.Receipt); err != nil {
		return result, err
	}
	for {
		state, e := windows.WaitForSingleObject(p.process, 10)
		if e != nil {
			return result, e
		}
		if state == windows.WAIT_OBJECT_0 {
			break
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
	}
	var exit uint32
	if err = windows.GetExitCodeProcess(p.process, &exit); err != nil || exit != 0 {
		return result, errors.Join(errClipboardCacheOwnership, err)
	}
	return result, nil
}

func (p *cacheNativeWindowsLaunch) close() error {
	if p == nil || p.closed {
		return nil
	}
	p.closed = true
	var err error
	if p.pipe != nil {
		err = errors.Join(err, p.pipe.Close())
	}
	if p.process != 0 {
		state, e := windows.WaitForSingleObject(p.process, 0)
		if e == nil && state == uint32(windows.WAIT_TIMEOUT) {
			_ = windows.TerminateProcess(p.process, 1)
			_, _ = windows.WaitForSingleObject(p.process, 3000)
		}
		err = errors.Join(err, e, windows.CloseHandle(p.process))
	}
	if p.image != nil {
		err = errors.Join(err, p.image.Close())
	}
	return err
}

// Keep errors/path/keys out of externally visible diagnostics. This prototype
// has no production call site; a future publisher needs its own permission CAS.
var _ cacheNativeVerifiedLaunch = (*cacheNativeWindowsLaunch)(nil)
