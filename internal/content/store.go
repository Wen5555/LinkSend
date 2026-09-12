// Package content owns immutable, bounded local snapshots. File bodies stay in
// Go and on disk; Snapshot is the only type intended for frontend metadata.
package content

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/zeebo/blake3"
)

const (
	MaxTextBytes      = 64 << 10
	MaxImageBytes     = 32 << 20
	MaxImagePixels    = 40_000_000
	MaxImageDimension = 32768
	maxMetadataBytes  = 256 << 10
	maxReferences     = 1024
	maxObjects        = 10000
)

type Kind string

const (
	Text  Kind = "text"
	URL   Kind = "url"
	Image Kind = "image"
)

var (
	ErrInvalidText  = errors.New("INVALID_CONTENT_TEXT")
	ErrInvalidURL   = errors.New("INVALID_CONTENT_URL")
	ErrLimit        = errors.New("CONTENT_LIMIT_EXCEEDED")
	ErrInvalidImage = errors.New("INVALID_CONTENT_IMAGE")
	ErrOwnership    = errors.New("CONTENT_OWNERSHIP_MISMATCH")
	ErrChanged      = errors.New("CONTENT_SNAPSHOT_CHANGED")
	ErrInUse        = errors.New("CONTENT_STORE_IN_USE")
	ErrClosed       = errors.New("CONTENT_STORE_CLOSED")
	ErrReference    = errors.New("INVALID_CONTENT_REFERENCE")
)

// Snapshot deliberately excludes local paths, text previews and encoded bytes.
type Snapshot struct {
	ID        string `json:"id"`
	Kind      Kind   `json:"kind"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
	MediaType string `json:"media_type"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	CreatedAt string `json:"created_at"`
}

type objectRecord struct {
	Version      int      `json:"version"`
	Owner        string   `json:"store_owner"`
	BodyIdentity string   `json:"body_identity"`
	Snapshot     Snapshot `json:"snapshot"`
	References   []string `json:"references"`
}

// Store has one OS-locked writer. Retain before recording a draft/task reference;
// Release only after that reference is durably removed. A crash can then leak an
// extra reference, but cannot let cleanup delete a queued or resumable body.
type Store struct {
	mu        sync.Mutex
	root      *os.Root
	lock      *os.File
	directory string
	owner     string
}

func OpenStore(profileDirectory string) (_ *Store, err error) {
	directory, err := filepath.Abs(filepath.Join(profileDirectory, "content-snapshots"))
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrOwnership
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	s := &Store{root: root, directory: directory}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()
	if info, e := root.Lstat(".lock"); e == nil && !info.Mode().IsRegular() {
		return nil, ErrOwnership
	} else if e != nil && !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	}
	s.lock, err = root.OpenFile(".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockStoreFile(s.lock); err != nil {
		return nil, err
	}
	f, err := root.OpenFile(".owner", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, fs.ErrExist) {
		info, err := root.Lstat(".owner")
		if err != nil || !info.Mode().IsRegular() {
			return nil, ErrOwnership
		}
		f, err = root.Open(".owner")
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(f, 64))
		_ = f.Close()
		if readErr != nil {
			return nil, readErr
		}
		s.owner = string(data)
		if !validID(s.owner) {
			return nil, ErrOwnership
		}
	} else if err != nil {
		return nil, err
	} else {
		s.owner, err = randomID()
		if err == nil {
			_, err = io.WriteString(f, s.owner)
		}
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if s.lock != nil {
		err = s.lock.Close()
		s.lock = nil
	}
	if s.root != nil {
		err = errors.Join(err, s.root.Close())
		s.root = nil
	}
	return err
}

// ValidateURL never launches anything. Received unsupported schemes can be
// presented as Text; only explicit http/https actions should call this helper.
func ValidateURL(value string) error {
	if value == "" || len(value) > MaxTextBytes || !utf8.ValidString(value) {
		return ErrInvalidURL
	}
	if strings.TrimSpace(value) != value || strings.ContainsFunc(value, unicode.IsControl) {
		return ErrInvalidURL
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return ErrInvalidURL
	}
	return nil
}

func (s *Store) CreateText(ctx context.Context, kind Kind, value, ownerRef string) (Snapshot, error) {
	if len(value) > MaxTextBytes {
		return Snapshot{}, ErrLimit
	}
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || (kind != Text && kind != URL) {
		return Snapshot{}, ErrInvalidText
	}
	if kind == URL {
		if err := ValidateURL(value); err != nil {
			return Snapshot{}, err
		}
	}
	return s.create(ctx, Snapshot{Kind: kind, MediaType: "text/plain; charset=utf-8"}, ownerRef, MaxTextBytes, func(w io.Writer) error { _, err := io.WriteString(w, value); return err })
}

// DecodeImage checks encoded size and DecodeConfig dimensions before allocating
// pixels. Only PNG and JPEG are accepted; native TIFF is converted by ImageIO.
func DecodeImage(ctx context.Context, input io.Reader) (image.Image, error) {
	data, err := io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, r: input}, MaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxImageBytes {
		return nil, ErrLimit
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") {
		return nil, ErrInvalidImage
	}
	if err := ValidateDimensions(config.Width, config.Height); err != nil {
		return nil, err
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return decoded, nil
}

func ValidateDimensions(width, height int) error {
	if width <= 0 || height <= 0 || width > MaxImageDimension || height > MaxImageDimension || width > MaxImagePixels/height {
		return ErrLimit
	}
	return nil
}

func (s *Store) CreateImage(ctx context.Context, input io.Reader, ownerRef string) (Snapshot, error) {
	img, err := DecodeImage(ctx, input)
	if err != nil {
		return Snapshot{}, err
	}
	return s.CreateImageFromImage(ctx, img, ownerRef)
}

// CreateImageFromImage is for Go/native adapters, never a JavaScript binding.
// The caller must keep the image unchanged until this synchronous call returns.
func (s *Store) CreateImageFromImage(ctx context.Context, img image.Image, ownerRef string) (Snapshot, error) {
	if img == nil {
		return Snapshot{}, ErrInvalidImage
	}
	bounds := img.Bounds()
	if err := ValidateDimensions(bounds.Dx(), bounds.Dy()); err != nil {
		return Snapshot{}, err
	}
	return s.create(ctx, Snapshot{Kind: Image, MediaType: "image/png", Width: bounds.Dx(), Height: bounds.Dy()}, ownerRef, MaxImageBytes, func(w io.Writer) error { return png.Encode(w, img) })
}

func (s *Store) create(ctx context.Context, snapshot Snapshot, ownerRef string, limit int64, write func(io.Writer) error) (_ Snapshot, err error) {
	if !validReference(ownerRef) {
		return Snapshot{}, ErrReference
	}
	if err = ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return Snapshot{}, ErrClosed
	}
	id, err := randomID()
	if err != nil {
		return Snapshot{}, err
	}
	if err = s.root.Mkdir(id, 0700); err != nil {
		return Snapshot{}, err
	}
	obj, err := s.root.OpenRoot(id)
	if err != nil {
		_ = s.root.Remove(id)
		return Snapshot{}, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = obj.Remove(bodyName(snapshot.Kind))
			_ = obj.Remove("metadata.json")
			_ = obj.Remove("metadata.tmp")
		}
		_ = obj.Close()
		if !keep {
			_ = s.root.Remove(id)
		}
	}()
	f, err := obj.OpenFile(bodyName(snapshot.Kind), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Snapshot{}, err
	}
	hash := blake3.New()
	w := &boundedWriter{ctx: ctx, w: io.MultiWriter(f, hash), remaining: limit}
	err = write(w)
	if err == nil {
		err = f.Sync()
	}
	var identity string
	if err == nil {
		identity, err = bodyFileIdentity(f)
	}
	closeErr := f.Close()
	if err != nil {
		return Snapshot{}, err
	}
	if closeErr != nil {
		return Snapshot{}, closeErr
	}
	if err = ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	snapshot.ID, snapshot.Size = id, limit-w.remaining
	snapshot.Digest = hex.EncodeToString(hash.Sum(nil))
	snapshot.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	record := objectRecord{Version: 1, Owner: s.owner, BodyIdentity: identity, Snapshot: snapshot, References: []string{ownerRef}}
	if err = saveRecord(obj, record); err != nil {
		return Snapshot{}, err
	}
	keep = true
	return snapshot, nil
}

func bodyName(kind Kind) string {
	switch kind {
	case Text:
		return "text.txt"
	case URL:
		return "link.txt"
	case Image:
		return "image.png"
	default:
		return ""
	}
}

func (s *Store) Metadata(id string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	obj, record, err := s.load(id)
	if err != nil {
		return Snapshot{}, err
	}
	defer obj.Close()
	return record.Snapshot, nil
}

// OwnedPath returns a verified local path for Go transfer.Prepare. Callers must
// retain a durable owner reference while any task can use or resume this path.
func (s *Store) OwnedPath(ctx context.Context, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	obj, record, err := s.load(id)
	if err != nil {
		return "", err
	}
	defer obj.Close()
	if len(record.References) == 0 {
		return "", ErrReference
	}
	if err = verifyBody(ctx, obj, record); err != nil {
		return "", err
	}
	return filepath.Join(s.directory, id, bodyName(record.Snapshot.Kind)), nil
}

func (s *Store) Retain(id, ownerRef string) error  { return s.changeReference(id, ownerRef, true) }
func (s *Store) Release(id, ownerRef string) error { return s.changeReference(id, ownerRef, false) }

func (s *Store) changeReference(id, ownerRef string, retain bool) error {
	if !validReference(ownerRef) {
		return ErrReference
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	obj, record, err := s.load(id)
	if err != nil {
		return err
	}
	defer obj.Close()
	for i, ref := range record.References {
		if ref != ownerRef {
			continue
		}
		if retain {
			return nil
		}
		record.References = append(record.References[:i], record.References[i+1:]...)
		return saveRecord(obj, record)
	}
	if !retain {
		return nil
	}
	if len(record.References) >= maxReferences {
		return ErrLimit
	}
	record.References = append(record.References, ownerRef)
	sort.Strings(record.References)
	return saveRecord(obj, record)
}

// Cleanup never recursively removes a directory and never infers ownership from
// a hidden name. Unknown files, changed bodies and corrupt records are preserved.
// Paused, active and resumable tasks must retain their references until discarded.
func (s *Store) Cleanup(ctx context.Context) (removed []string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return nil, ErrClosed
	}
	f, err := s.root.Open(".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	count := 0
	for {
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			if e := ctx.Err(); e != nil {
				return removed, errors.Join(err, e)
			}
			count++
			if count > maxObjects {
				return removed, errors.Join(err, ErrLimit)
			}
			if !validID(entry.Name()) {
				continue
			}
			obj, record, e := s.load(entry.Name())
			if e != nil {
				err = errors.Join(err, e)
				continue
			}
			if len(record.References) != 0 {
				_ = obj.Close()
				continue
			}
			e = discardOwnedRecordTemp(obj, record)
			if e == nil {
				e = verifyObjectNames(obj, record.Snapshot.Kind)
			}
			if e == nil {
				e = verifyBody(ctx, obj, record)
			}
			if e == nil {
				e = obj.Remove(bodyName(record.Snapshot.Kind))
			}
			if e == nil {
				e = obj.Remove("metadata.json")
			}
			_ = obj.Close()
			if e == nil {
				e = s.root.Remove(entry.Name())
			}
			if e != nil {
				err = errors.Join(err, e)
			} else {
				removed = append(removed, entry.Name())
			}
		}
		if errors.Is(readErr, io.EOF) {
			return removed, err
		}
		if readErr != nil {
			return removed, errors.Join(err, readErr)
		}
	}
}

func (s *Store) load(id string) (*os.Root, objectRecord, error) {
	if s.root == nil {
		return nil, objectRecord{}, ErrClosed
	}
	if !validID(id) {
		return nil, objectRecord{}, ErrOwnership
	}
	info, err := s.root.Lstat(id)
	if err != nil {
		return nil, objectRecord{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, objectRecord{}, ErrOwnership
	}
	obj, err := s.root.OpenRoot(id)
	if err != nil {
		return nil, objectRecord{}, err
	}
	fail := func(e error) (*os.Root, objectRecord, error) { _ = obj.Close(); return nil, objectRecord{}, e }
	info, err = obj.Lstat("metadata.json")
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() {
		return fail(ErrOwnership)
	}
	f, err := obj.Open("metadata.json")
	if err != nil {
		return fail(err)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxMetadataBytes+1))
	_ = f.Close()
	if err != nil {
		return fail(err)
	}
	if len(data) > maxMetadataBytes {
		return fail(ErrLimit)
	}
	var record objectRecord
	if json.Unmarshal(data, &record) != nil {
		return fail(ErrOwnership)
	}
	if record.Version != 1 || record.Owner != s.owner || record.BodyIdentity == "" || len(record.BodyIdentity) > 128 || record.Snapshot.ID != id || !validSnapshot(record.Snapshot) || len(record.References) > maxReferences {
		return fail(ErrOwnership)
	}
	seen := make(map[string]bool)
	for _, ref := range record.References {
		if !validReference(ref) || seen[ref] {
			return fail(ErrReference)
		}
		seen[ref] = true
	}
	return obj, record, nil
}

func validSnapshot(s Snapshot) bool {
	digest, err := hex.DecodeString(s.Digest)
	if err != nil || len(digest) != 32 || s.Size < 0 || bodyName(s.Kind) == "" {
		return false
	}
	if _, err = time.Parse(time.RFC3339Nano, s.CreatedAt); err != nil {
		return false
	}
	if s.Kind == Image {
		return s.Size > 0 && s.Size <= MaxImageBytes && s.MediaType == "image/png" && ValidateDimensions(s.Width, s.Height) == nil
	}
	return s.Size > 0 && s.Size <= MaxTextBytes && s.MediaType == "text/plain; charset=utf-8" && s.Width == 0 && s.Height == 0
}

func verifyBody(ctx context.Context, obj *os.Root, record objectRecord) error {
	snapshot := record.Snapshot
	info, err := obj.Lstat(bodyName(snapshot.Kind))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != snapshot.Size {
		return ErrChanged
	}
	f, err := obj.Open(bodyName(snapshot.Kind))
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) {
		return ErrChanged
	}
	identity, err := bodyFileIdentity(f)
	if err != nil {
		return err
	}
	if identity != record.BodyIdentity {
		return ErrOwnership
	}
	hash := blake3.New()
	n, err := io.Copy(hash, &contextReader{ctx: ctx, r: io.LimitReader(f, snapshot.Size+1)})
	if err != nil {
		return err
	}
	if n != snapshot.Size || hex.EncodeToString(hash.Sum(nil)) != snapshot.Digest {
		return ErrChanged
	}
	return nil
}

func verifyObjectNames(obj *os.Root, kind Kind) error {
	f, err := obj.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.ReadDir(3)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) != 2 {
		return ErrOwnership
	}
	for _, entry := range entries {
		if entry.Name() != "metadata.json" && entry.Name() != bodyName(kind) {
			return ErrOwnership
		}
	}
	return nil
}

func saveRecord(obj *os.Root, record objectRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(data) > maxMetadataBytes {
		return ErrLimit
	}
	if err = discardOwnedRecordTemp(obj, record); err != nil {
		return err
	}
	f, err := obj.OpenFile("metadata.tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer obj.Remove("metadata.tmp")
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return obj.Rename("metadata.tmp", "metadata.json")
}

// A crash before rename leaves an uncommitted reference update. Only remove a
// temporary record that proves it belongs to this exact owned snapshot. The
// current metadata.json remains authoritative; a failed Retain never authorized
// the caller to persist a task reference, and a failed Release keeps the old ref.
func discardOwnedRecordTemp(obj *os.Root, record objectRecord) error {
	info, err := obj.Lstat("metadata.tmp")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxMetadataBytes {
		return ErrOwnership
	}
	f, err := obj.Open("metadata.tmp")
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxMetadataBytes+1))
	_ = f.Close()
	if err != nil {
		return err
	}
	var pending objectRecord
	if len(data) > maxMetadataBytes || json.Unmarshal(data, &pending) != nil || pending.Version != record.Version || pending.Owner != record.Owner || pending.Snapshot != record.Snapshot || pending.BodyIdentity != record.BodyIdentity {
		return ErrOwnership
	}
	return obj.Remove("metadata.tmp")
}

func validID(value string) bool {
	data, err := hex.DecodeString(value)
	return err == nil && len(data) == 16 && value == strings.ToLower(value)
}
func validReference(value string) bool {
	return value != "" && len(value) <= 128 && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}
func randomID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

type boundedWriter struct {
	ctx       context.Context
	w         io.Writer
	remaining int64
}

func (w *boundedWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(data)) > w.remaining {
		return 0, ErrLimit
	}
	n, err := w.w.Write(data)
	w.remaining -= int64(n)
	return n, err
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(data)
}
