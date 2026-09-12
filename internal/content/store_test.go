package content

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	profile := t.TempDir()
	store, err := OpenStore(profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, profile
}

func TestSnapshotTextReferenceRestartAndOwnedCleanup(t *testing.T) {
	store, profile := openTestStore(t)
	text := "中文\n第二行\n🙂"
	snapshot, err := store.CreateText(t.Context(), Text, text, "draft:one")
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.OwnedPath(t.Context(), snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != text {
		t.Fatalf("bad snapshot: %s %v", data, err)
	}
	if err := store.Retain(snapshot.ID, "task:paused"); err != nil {
		t.Fatal(err)
	}
	if err := store.Retain(snapshot.ID, "task:paused"); err != nil {
		t.Fatal(err)
	}
	if err := store.Release(snapshot.ID, "draft:one"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(profile)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if removed, err := store.Cleanup(t.Context()); err != nil || len(removed) != 0 {
		t.Fatalf("removed resumable content: %v %v", removed, err)
	}
	if restored, err := store.Metadata(snapshot.ID); err != nil || restored != snapshot {
		t.Fatalf("metadata changed: %+v %v", restored, err)
	}
	if err := store.Release(snapshot.ID, "task:paused"); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.Cleanup(t.Context()); err != nil || len(removed) != 1 || removed[0] != snapshot.ID {
		t.Fatalf("cleanup failed: %v %v", removed, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("body survived cleanup: %v", err)
	}
}

func TestSnapshotTextAndURLLimits(t *testing.T) {
	store, _ := openTestStore(t)
	for _, text := range []string{"", string([]byte{0xff}), "nul\x00byte"} {
		if _, err := store.CreateText(t.Context(), Text, text, "draft:a"); !errors.Is(err, ErrInvalidText) {
			t.Fatalf("invalid text accepted: %q %v", text, err)
		}
	}
	if _, err := store.CreateText(t.Context(), Text, strings.Repeat("x", MaxTextBytes), "draft:a"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateText(t.Context(), Text, strings.Repeat("x", MaxTextBytes+1), "draft:a"); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	for _, value := range []string{"javascript:alert(1)", "file:///C:/secret", "data:text/html,test", "https://", "https://user:pass@example.com/", " https://example.com", "https://example.com\n"} {
		if err := ValidateURL(value); !errors.Is(err, ErrInvalidURL) {
			t.Fatalf("unsafe URL accepted: %q %v", value, err)
		}
	}
	if _, err := store.CreateText(t.Context(), URL, "https://example.com/path?q=%E4%B8%AD%E6%96%87", "draft:url"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateText(t.Context(), Text, "body", ""); !errors.Is(err, ErrReference) {
		t.Fatal(err)
	}
}

func TestSnapshotImageImmutableAndMetadataOnly(t *testing.T) {
	store, _ := openTestStore(t)
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 201, G: 42, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.CreateImage(t.Context(), bytes.NewReader(encoded.Bytes()), "draft:image")
	if err != nil {
		t.Fatal(err)
	}
	img.SetNRGBA(0, 0, color.NRGBA{B: 255, A: 255})
	path, err := store.OwnedPath(t.Context(), snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := decoded.At(0, 0).RGBA()
	if r>>8 != 201 || g>>8 != 42 || b != 0 {
		t.Fatal("queued image followed mutable source")
	}
	if snapshot.Width != 3 || snapshot.Height != 2 || snapshot.MediaType != "image/png" {
		t.Fatal(snapshot)
	}
	metadata, _ := json.Marshal(snapshot)
	for _, forbidden := range []string{"path", "body", "base64", "preview", "data:"} {
		if bytes.Contains(metadata, []byte(forbidden)) {
			t.Fatalf("body leaked in DTO: %s", metadata)
		}
	}
}

func TestSnapshotImageBombRejectedBeforeDecode(t *testing.T) {
	store, _ := openTestStore(t)
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := pngData.Bytes()
	binary.BigEndian.PutUint32(data[16:20], 100000)
	binary.BigEndian.PutUint32(data[20:24], 100000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if _, err := store.CreateImage(t.Context(), bytes.NewReader(data), "draft:image"); !errors.Is(err, ErrLimit) {
		t.Fatalf("pixel bomb decoded: %v", err)
	}
	if _, err := store.CreateImage(t.Context(), io.LimitReader(zeroReader{}, MaxImageBytes+1), "draft:image"); !errors.Is(err, ErrLimit) {
		t.Fatalf("encoded size unbounded: %v", err)
	}
	if _, err := store.CreateImage(t.Context(), strings.NewReader("not an image"), "draft:image"); !errors.Is(err, ErrInvalidImage) {
		t.Fatal(err)
	}
	for _, dims := range [][2]int{{0, 1}, {-1, 1}, {40000001, 1}, {20000000, 3}} {
		if ValidateDimensions(dims[0], dims[1]) == nil {
			t.Fatal(dims)
		}
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestSnapshotCleanupPreservesChangedAndForeignFiles(t *testing.T) {
	for _, scenario := range []string{"body_changed", "foreign_identical_body", "extra_user_file", "foreign_owner", "symlink_body"} {
		t.Run(scenario, func(t *testing.T) {
			store, _ := openTestStore(t)
			snapshot, err := store.CreateText(t.Context(), Text, "owned", "draft:a")
			if err != nil {
				t.Fatal(err)
			}
			body, err := store.OwnedPath(t.Context(), snapshot.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Release(snapshot.ID, "draft:a"); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "body_changed":
				err = os.WriteFile(body, []byte("other"), 0600)
			case "foreign_identical_body":
				// Keep the original inode alive so no filesystem can recycle it.
				if e := os.Rename(body, filepath.Join(t.TempDir(), "original.txt")); e != nil {
					t.Fatal(e)
				}
				err = os.WriteFile(body, []byte("owned"), 0600)
			case "extra_user_file":
				err = os.WriteFile(filepath.Join(filepath.Dir(body), "user.txt"), []byte("preserve"), 0600)
			case "foreign_owner":
				meta := filepath.Join(filepath.Dir(body), "metadata.json")
				data, e := os.ReadFile(meta)
				if e != nil {
					t.Fatal(e)
				}
				data = bytes.Replace(data, []byte(store.owner), []byte(strings.Repeat("0", 32)), 1)
				err = os.WriteFile(meta, data, 0600)
			case "symlink_body":
				foreign := filepath.Join(t.TempDir(), "foreign.txt")
				if e := os.WriteFile(foreign, []byte("owned"), 0600); e != nil {
					t.Fatal(e)
				}
				if e := os.Remove(body); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(foreign, body); e != nil {
					t.Skipf("host cannot create symlinks: %v", e)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if removed, err := store.Cleanup(t.Context()); err == nil || len(removed) != 0 {
				t.Fatalf("unsafe cleanup: %v %v", removed, err)
			}
			if _, err := os.Lstat(body); err != nil {
				t.Fatal("modified/foreign file removed", err)
			}
		})
	}
}

func TestSnapshotReferencePersistenceFailureAndWriterLock(t *testing.T) {
	store, profile := openTestStore(t)
	if duplicate, err := OpenStore(profile); !errors.Is(err, ErrInUse) {
		if duplicate != nil {
			duplicate.Close()
		}
		t.Fatalf("second writer allowed: %v", err)
	}
	snapshot, err := store.CreateText(t.Context(), Text, "keep", "draft:a")
	if err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(profile, "content-snapshots", snapshot.ID, "metadata.tmp")
	if err := os.Mkdir(blocker, 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.Release(snapshot.ID, "draft:a"); err == nil {
		t.Fatal("release acknowledged unpersisted reference")
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.Cleanup(t.Context()); err != nil || len(removed) != 0 {
		t.Fatalf("retained body lost after persistence failure: %v %v", removed, err)
	}
	if _, err := store.OwnedPath(t.Context(), snapshot.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotConcurrentRefsCancellationAndTraversal(t *testing.T) {
	store, _ := openTestStore(t)
	snapshot, err := store.CreateText(t.Context(), Text, "keep", "draft:a")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if err := store.Retain(snapshot.ID, "task:same"); err != nil {
					t.Error(err)
				}
				if _, err := store.Metadata(snapshot.ID); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	for _, id := range []string{"../outside", strings.Repeat("f", 31), strings.Repeat("F", 32)} {
		if _, err := store.Metadata(id); !errors.Is(err, ErrOwnership) {
			t.Fatal(id, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.CreateText(ctx, Text, "cancelled", "draft:b"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := store.Cleanup(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSnapshotRecoversOwnedInterruptedReferenceUpdate(t *testing.T) {
	store, profile := openTestStore(t)
	snapshot, err := store.CreateText(t.Context(), Text, "keep", "draft:original")
	if err != nil {
		t.Fatal(err)
	}
	obj, record, err := store.load(snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = obj.Close()
	record.References = append(record.References, "task:uncommitted")
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(profile, "content-snapshots", snapshot.ID, "metadata.tmp")
	if err = os.WriteFile(temporary, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(profile)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Retain(snapshot.ID, "task:actual"); err != nil {
		t.Fatal("interrupted checkpoint blocked resume", err)
	}
	obj, record, err = store.load(snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Close()
	if len(record.References) != 2 || record.References[0] != "draft:original" || record.References[1] != "task:actual" {
		t.Fatal("uncommitted reference was adopted", record.References)
	}
	if _, err = os.Stat(temporary); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned stale checkpoint survived", err)
	}
}

func TestSnapshotEncoderLimitDoesNotLeaveOwnedObject(t *testing.T) {
	store, profile := openTestStore(t)
	_, err := store.create(t.Context(), Snapshot{Kind: Text, MediaType: "text/plain; charset=utf-8"}, "draft:a", 2, func(w io.Writer) error { _, err := w.Write([]byte("too large")); return err })
	if !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(profile, "content-snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if validID(entry.Name()) {
			t.Fatal("failed encoder leaked partial snapshot", entry.Name())
		}
	}
}

func TestSnapshotMaximumImageMemory(t *testing.T) {
	if os.Getenv("LINKSEND_CONTENT_MEMORY_TEST") != "1" {
		t.Skip("opt-in 40 MP / 16-bit decode memory measurement")
	}
	makeEncoded := func() []byte {
		img := image.NewNRGBA64(image.Rect(0, 0, 8000, 5000))
		// Non-8-bit channel values force the worst standard PNG pixel storage.
		img.SetNRGBA64(0, 0, color.NRGBA64{R: 12345, A: 65535})
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			t.Fatal(err)
		}
		return encoded.Bytes()
	}
	encoded := makeEncoded()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	decoded, err := DecodeImage(t.Context(), bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if decoded.Bounds().Dx()*decoded.Bounds().Dy() != MaxImagePixels {
		t.Fatal(decoded.Bounds())
	}
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("40 MP 16-bit PNG: encoded=%d bytes decode_total_alloc=%d bytes heap_delta=%d bytes", len(encoded), allocated, int64(after.HeapAlloc)-int64(before.HeapAlloc))
	if allocated > 400<<20 {
		t.Fatalf("decoder exceeds 400 MiB allocation budget: %d", allocated)
	}
	runtime.KeepAlive(decoded)
}

func TestSnapshotImageEncoderReal32MiBLimit(t *testing.T) {
	if os.Getenv("LINKSEND_CONTENT_MEMORY_TEST") != "1" {
		t.Skip("opt-in incompressible 9 MP PNG output limit test")
	}
	store, profile := openTestStore(t)
	img := image.NewNRGBA(image.Rect(0, 0, 3000, 3000))
	rng := rand.New(rand.NewPCG(123, 456))
	for i := 0; i < len(img.Pix); i += 8 {
		binary.LittleEndian.PutUint64(img.Pix[i:], rng.Uint64())
	}
	if _, err := store.CreateImageFromImage(t.Context(), img, "draft:too-large"); !errors.Is(err, ErrLimit) {
		t.Fatalf("32 MiB encoder limit failed: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(profile, "content-snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if validID(entry.Name()) {
			t.Fatal("failed encoder leaked body", entry.Name())
		}
	}
	t.Log("incompressible 9 MP PNG refused at 32 MiB; temporary body removed")
}
