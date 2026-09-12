// Package transfer implements one file protocol for every authenticated direct path.
package transfer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zeebo/blake3"
	"golang.org/x/text/unicode/norm"
)

const (
	DefaultChunkSize = 4 << 20
	MaxMetadata      = 8 << 20
	MaxEntries       = 10000
	MaxChunks        = 100000
)

var (
	ErrPath      = errors.New("DANGEROUS_PATH")
	ErrChanged   = errors.New("SOURCE_CHANGED")
	ErrIntegrity = errors.New("CHECKSUM_FAILED")
	ErrRejected  = errors.New("RECEIVE_REJECTED")
	ErrConflict  = errors.New("FILE_CONFLICT")
)

type FileEntry struct {
	ID       uint32   `json:"id"`
	Path     string   `json:"path"`
	Type     string   `json:"type"`
	Size     int64    `json:"size"`
	Modified int64    `json:"modified"`
	Hash     string   `json:"hash,omitempty"`
	Chunks   []string `json:"chunks,omitempty"`
}

type Manifest struct {
	Version    int         `json:"version"`
	TransferID string      `json:"transfer_id"`
	ChunkSize  int         `json:"chunk_size"`
	Files      []FileEntry `json:"files"`
}

func Sum(data []byte) string { sum := blake3.Sum256(data); return hex.EncodeToString(sum[:]) }

func validHex(s string, n int) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == n && strings.ToLower(s) == s
}

// ValidatePath applies the same portable policy on Windows, macOS and Linux.
// Root confinement is additionally enforced by os.Root at the actual IO boundary.
func ValidatePath(p string) error {
	if p == "" || len(p) > 1024 || !utf8.ValidString(p) || !norm.NFC.IsNormalString(p) || strings.ContainsAny(p, "\\:<>\"|?*\x00") || path.IsAbs(p) || path.Clean(p) != p {
		return ErrPath
	}
	parts := strings.Split(p, "/")
	if len(parts) > 32 {
		return ErrPath
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || len(part) > 240 || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.HasPrefix(strings.ToLower(part), ".linksend") {
			return ErrPath
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return ErrPath
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
			return ErrPath
		}
		if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
			return ErrPath
		}
		if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
			if strings.ContainsAny(base, "¹²³") {
				return ErrPath
			}
		}
	}
	return nil
}

func (m Manifest) Validate() error {
	if m.Version != 1 || !validHex(m.TransferID, 16) || m.ChunkSize < 64<<10 || m.ChunkSize > 8<<20 || len(m.Files) == 0 || len(m.Files) > MaxEntries {
		return errors.New("INVALID_MANIFEST")
	}
	seen := map[string]string{}
	chunks := 0
	for i, e := range m.Files {
		if e.ID != uint32(i) || ValidatePath(e.Path) != nil || e.Size < 0 || e.Size > 1<<50 {
			return ErrPath
		}
		key := strings.ToLower(e.Path)
		if _, ok := seen[key]; ok {
			return ErrConflict
		}
		seen[key] = e.Type
		switch e.Type {
		case "directory":
			if e.Size != 0 || e.Hash != "" || len(e.Chunks) != 0 {
				return errors.New("INVALID_MANIFEST")
			}
		case "file":
			if !validHex(e.Hash, 32) || int64(len(e.Chunks)) != (e.Size+int64(m.ChunkSize)-1)/int64(m.ChunkSize) {
				return errors.New("INVALID_MANIFEST")
			}
			for _, h := range e.Chunks {
				if !validHex(h, 32) {
					return errors.New("INVALID_MANIFEST")
				}
			}
		default:
			return ErrPath
		}
		chunks += len(e.Chunks)
		if chunks > MaxChunks {
			return errors.New("RESOURCE_LIMIT")
		}
	}
	for _, e := range m.Files {
		for parent := path.Dir(strings.ToLower(e.Path)); parent != "."; parent = path.Dir(parent) {
			if kind, exists := seen[parent]; !exists || kind != "directory" {
				return ErrPath
			}
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(b) > MaxMetadata {
		return errors.New("RESOURCE_LIMIT")
	}
	return nil
}

func (m Manifest) Digest() string { b, _ := json.Marshal(m); return Sum(b) }
func (m Manifest) TotalBytes() int64 {
	var n int64
	for _, e := range m.Files {
		n += e.Size
	}
	return n
}

type source struct {
	root     *os.Root
	relative string
	info     fs.FileInfo
}
type Prepared struct {
	Manifest Manifest
	sources  map[uint32]source
	roots    []*os.Root
}

func (p *Prepared) Close() error {
	var result error
	for _, root := range p.roots {
		result = errors.Join(result, root.Close())
	}
	p.roots = nil
	return result
}

// Prepare precomputes both chunk and content hashes; it never serializes absolute source paths.
func Prepare(ctx context.Context, paths []string, chunkSize int) (_ *Prepared, err error) {
	return prepare(ctx, paths, chunkSize, "")
}

// PrepareForResume rebuilds a manifest for an existing logical transfer. The
// caller must compare the resulting digest with its persisted recovery record
// before opening a new network session.
func PrepareForResume(ctx context.Context, paths []string, chunkSize int, transferID string) (_ *Prepared, err error) {
	if !validHex(transferID, 16) {
		return nil, errors.New("INVALID_TRANSFER_ID")
	}
	return prepare(ctx, paths, chunkSize, transferID)
}

func prepare(ctx context.Context, paths []string, chunkSize int, transferID string) (_ *Prepared, err error) {
	if chunkSize == 0 {
		chunkSize = DefaultChunkSize
	}
	if chunkSize < 64<<10 || chunkSize > 8<<20 {
		return nil, errors.New("INVALID_CHUNK_SIZE")
	}
	if transferID == "" {
		id := make([]byte, 16)
		if _, err = rand.Read(id); err != nil {
			return nil, err
		}
		transferID = hex.EncodeToString(id)
	}
	p := &Prepared{Manifest: Manifest{Version: 1, TransferID: transferID, ChunkSize: chunkSize}, sources: make(map[uint32]source)}
	defer func() {
		if err != nil {
			_ = p.Close()
		}
	}()
	type found struct {
		entry FileEntry
		src   source
	}
	var entries []found
	for _, input := range paths {
		abs, e := filepath.Abs(input)
		if e != nil {
			return nil, e
		}
		info, e := os.Lstat(abs)
		if e != nil {
			return nil, e
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
			return nil, ErrPath
		}
		root, e := os.OpenRoot(filepath.Dir(abs))
		if e != nil {
			return nil, e
		}
		p.roots = append(p.roots, root)
		base := filepath.Base(abs)
		e = fs.WalkDir(root.FS(), filepath.ToSlash(base), func(rel string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if e := ctx.Err(); e != nil {
				return e
			}
			if ValidatePath(rel) != nil {
				return ErrPath
			}
			if d.Type()&os.ModeSymlink != 0 {
				return ErrPath
			}
			stat, e := d.Info()
			if e != nil {
				return e
			}
			entry := FileEntry{Path: rel, Type: "directory", Modified: stat.ModTime().UnixNano()}
			if !stat.IsDir() {
				if !stat.Mode().IsRegular() {
					return ErrPath
				}
				entry.Type = "file"
				entry.Size = stat.Size()
				f, e := root.Open(rel)
				if e != nil {
					return e
				}
				entry.Hash, entry.Chunks, e = hashFile(ctx, f, chunkSize)
				after, statErr := f.Stat()
				closeErr := f.Close()
				if e != nil {
					return e
				}
				if statErr != nil {
					return statErr
				}
				if closeErr != nil {
					return closeErr
				}
				if !os.SameFile(stat, after) || stat.Size() != after.Size() || !stat.ModTime().Equal(after.ModTime()) {
					return ErrChanged
				}
			}
			entries = append(entries, found{entry, source{root, rel, stat}})
			if len(entries) > MaxEntries {
				return errors.New("RESOURCE_LIMIT")
			}
			return nil
		})
		if e != nil {
			return nil, e
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].entry.Path < entries[j].entry.Path })
	for i, item := range entries {
		item.entry.ID = uint32(i)
		p.Manifest.Files = append(p.Manifest.Files, item.entry)
		if item.entry.Type == "file" {
			p.sources[uint32(i)] = item.src
		}
	}
	if err = p.Manifest.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func hashFile(ctx context.Context, r io.Reader, size int) (string, []string, error) {
	h := blake3.New()
	buf := make([]byte, size)
	var chunks []string
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
			chunks = append(chunks, Sum(buf[:n]))
			if len(chunks) > MaxChunks {
				return "", nil, errors.New("RESOURCE_LIMIT")
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return "", nil, err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), chunks, nil
}

func (p *Prepared) ReadChunk(ctx context.Context, file uint32, index int, buf []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int(file) >= len(p.Manifest.Files) {
		return nil, errors.New("INVALID_CHUNK")
	}
	e := p.Manifest.Files[file]
	if index < 0 || index >= len(e.Chunks) {
		return nil, errors.New("INVALID_CHUNK")
	}
	s, ok := p.sources[file]
	if !ok {
		return nil, ErrChanged
	}
	f, err := s.root.Open(s.relative)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(s.info, info) || info.Size() != e.Size || info.ModTime().UnixNano() != e.Modified {
		return nil, ErrChanged
	}
	n := min(int64(p.Manifest.ChunkSize), e.Size-int64(index)*int64(p.Manifest.ChunkSize))
	if int64(len(buf)) < n {
		return nil, io.ErrShortBuffer
	}
	if _, err = f.ReadAt(buf[:n], int64(index)*int64(p.Manifest.ChunkSize)); err != nil {
		return nil, err
	}
	if Sum(buf[:n]) != e.Chunks[index] {
		return nil, ErrChanged
	}
	return buf[:n], nil
}

func (p *Prepared) Revalidate(ctx context.Context) error {
	return p.revalidate(ctx, nil)
}

func (p *Prepared) RevalidateSelected(ctx context.Context, ids []uint32) error {
	selected := make(map[uint32]bool, len(ids))
	for _, id := range ids {
		if uint64(id) >= uint64(len(p.Manifest.Files)) {
			return ErrPlanMismatch
		}
		selected[id] = true
	}
	return p.revalidate(ctx, selected)
}

func (p *Prepared) revalidate(ctx context.Context, selected map[uint32]bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	buf := make([]byte, p.Manifest.ChunkSize)
	for _, e := range p.Manifest.Files {
		if selected != nil && !selected[e.ID] {
			continue
		}
		if e.Type != "file" {
			continue
		}
		s := p.sources[e.ID]
		st, err := s.root.Stat(s.relative)
		if err != nil {
			return err
		}
		if st.Size() != e.Size || st.ModTime().UnixNano() != e.Modified {
			return ErrChanged
		}
		for i := range e.Chunks {
			if _, err := p.ReadChunk(ctx, e.ID, i, buf); err != nil {
				return fmt.Errorf("%w: source file %d", err, e.ID)
			}
		}
	}
	return nil
}
