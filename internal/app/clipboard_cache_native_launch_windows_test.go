//go:build windows

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClipboardCacheWindowsNativeBootStable(t *testing.T) {
	a, err := (cacheNativeWindowsBoot{}).cacheNativeBootID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := (cacheNativeWindowsBoot{}).cacheNativeBootID()
	if err != nil || a != b || !strings.HasPrefix(a, "windows-boot-v1-") || !validCacheNativeBoot(a) {
		t.Fatal("actual kernel boot identifier was unavailable or unstable", err)
	}
}

// This opt-in uses only an explicitly built/pinned helper and fresh self-owned
// filesystem/profile fixtures. It never queries or mutates a system clipboard.
func TestClipboardCacheWindowsNativeControlled(t *testing.T) {
	image, expected := os.Getenv("LINKSEND_NATIVE_HELPER_TEST_EXE"), os.Getenv("LINKSEND_NATIVE_HELPER_TEST_SHA256")
	if image == "" || expected == "" {
		t.Skip("requires an explicitly source-verified private helper image and digest")
	}
	f := newCacheNativeFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if bad, err := f.r.prepareCacheNativeWindows(ctx, image, strings.Repeat("0", 64)); err == nil {
		_ = bad.close()
		t.Fatal("wrong trusted image hash launched a helper")
	}
	launch, err := f.r.prepareCacheNativeWindows(ctx, image, expected)
	if err != nil {
		t.Fatal("prepare real helper", err)
	}
	t.Cleanup(func() { _ = launch.close() })
	d, err := f.r.sealNativeDelegation(ctx, launch)
	if err != nil {
		t.Fatal("durable delegation", err)
	}
	wrong := d
	wrong.Binding.ChildEphemeralPublicKey = bytes.Repeat([]byte{0x33}, 32)
	if _, err = launch.commit(ctx, wrong); err == nil {
		t.Fatal("wrong child key was committed")
	}
	// A valid signed delegation presented after its exact durable row changed
	// must not export COM. Restore only the original self-owned signed fixture.
	if _, err = f.s.store.db.Exec(`UPDATE metadata SET value=? WHERE key=?`, f.r.raw+" ", clipboardCacheKeyPrefix+f.r.record.ID); err != nil {
		t.Fatal(err)
	}
	_, rejected := launch.commit(ctx, d)
	if _, err = f.s.store.db.Exec(`UPDATE metadata SET value=? WHERE key=?`, f.r.raw, clipboardCacheKeyPrefix+f.r.record.ID); err != nil {
		t.Fatal(err)
	}
	if rejected == nil {
		t.Fatal("changed SQLite authorization was committed")
	}
	spoolPath := filepath.Join(f.profile, cacheNativeSpoolDirectory, f.r.record.ID+".json")
	if info, e := os.Stat(spoolPath); e != nil || info.Size() != 0 {
		t.Fatal("rejected commit produced a receipt", e)
	}
	result, err := launch.commit(ctx, d)
	if err != nil {
		t.Fatal("real controlled STA/MTA source-close receipt", err)
	}
	if result.ProtectedChecks != 0x7f || result.SourcesClosed == 0 {
		t.Fatal("missing real retained-consumer/source-close checks")
	}
	if err = launch.close(); err != nil {
		t.Fatal(err)
	}
	if ok, e := f.s.consumeCacheNativeSpool(ctx, f.r.record.ID, cacheNativeWindowsBoot{}); ok || e == nil {
		t.Fatal("active receiver consumed native receipt")
	}
	var forged cacheNativeCloseReceipt
	if err = json.Unmarshal(result.Receipt, &forged); err != nil {
		t.Fatal(err)
	}
	forged.Signature[0] ^= 1
	body, _ := json.Marshal(forged)
	f.closeStorage()
	if ok, e := f.s.consumeCacheNativeReceipt(ctx, f.r.record.ID, f.r.raw, body, cacheNativeWindowsBoot{}); ok || e == nil {
		t.Fatal("forged native receipt was consumed")
	}
	oldRun, oldPublic := f.s.epoch, f.s.identity.PublicKey()
	f.s.Shutdown()
	reopened, err := New(Config{DataDir: f.profile})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Shutdown)
	if reopened.epoch == oldRun || !bytes.Equal(oldPublic, reopened.identity.PublicKey()) {
		t.Fatal("real profile identity was not reloaded into a new main run")
	}
	if ok, e := reopened.consumeCacheNativeSpool(ctx, f.r.record.ID, cacheNativeWindowsBoot{}); !ok || e != nil {
		t.Fatal("new main did not consume original native spool", e)
	}
	consumed := cacheNativeTestRaw(t, reopened, f.r.record.ID)
	if ok, e := reopened.consumeCacheNativeSpool(ctx, f.r.record.ID, cacheNativeWindowsBoot{}); ok || !errors.Is(e, errCacheNativeConsumed) {
		t.Fatal("replayed native spool was consumed", e)
	}
	if cacheNativeTestRaw(t, reopened, f.r.record.ID) != consumed {
		t.Fatal("replay changed the durable first observation")
	}
	if _, e := reopened.collectClipboardCache(ctx, true, time.Now().Add(365*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(filepath.Join(f.d.directory, "owned.txt"))
	if e != nil || !bytes.Equal(data, f.body) {
		t.Fatal("Native receipt-only object was deleted or changed", e)
	}
	t.Logf("actual helper closed %d inherited source handles after seven retained COM/source-close gates; original-run spool consumed exactly once; source preserved", result.SourcesClosed)
}
