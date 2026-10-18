package transfer

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// SourceSelection is a local, versioned file-selection policy, not gitignore.
// Nil means full contents. Patterns use /, are case sensitive on every OS,
// support * and ?, and support ** only as an entire path segment.
type SourceSelection struct {
	Version int      `json:"version"`
	Preset  string   `json:"preset"`
	Exclude []string `json:"exclude,omitempty"`
}

var sourcePreset = []string{".git/", "node_modules/", "dist/", "build/", "target/", "bin/", "obj/", ".venv/", "venv/", "__pycache__/", ".cache/", "coverage/"}

func NormalizeSourceSelection(input *SourceSelection) (*SourceSelection, error) {
	if input == nil {
		return nil, nil
	}
	if input.Version != 1 || input.Preset != "all" && input.Preset != "source" || len(input.Exclude) > 32 {
		return nil, errors.New("INVALID_SOURCE_SELECTION")
	}
	out := &SourceSelection{Version: 1, Preset: input.Preset}
	seen := map[string]bool{}
	for _, raw := range input.Exclude {
		pattern := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
		if pattern == "" || len(pattern) > 128 || !utf8.ValidString(pattern) || !norm.NFC.IsNormalString(pattern) || strings.HasPrefix(pattern, "/") || strings.ContainsAny(pattern, "![]:{}") || strings.ContainsFunc(pattern, unicode.IsControl) {
			return nil, errors.New("INVALID_SOURCE_SELECTION")
		}
		for _, part := range strings.Split(strings.TrimSuffix(pattern, "/"), "/") {
			if part == "" || part == "." || part == ".." || strings.Contains(part, "**") && part != "**" {
				return nil, errors.New("INVALID_SOURCE_SELECTION")
			}
		}
		if !seen[pattern] {
			out.Exclude = append(out.Exclude, pattern)
			seen[pattern] = true
		}
	}
	sort.Strings(out.Exclude)
	if out.Preset == "all" && len(out.Exclude) == 0 {
		return nil, nil
	}
	return out, nil
}

func sourceExcluded(selection *SourceSelection, relative string, directory bool) bool {
	if selection == nil || relative == "" {
		return false
	} // Keep the selected directory as a container.
	patterns := selection.Exclude
	if selection.Preset == "source" {
		patterns = append(append([]string(nil), sourcePreset...), patterns...)
	}
	parts := strings.Split(relative, "/")
	for _, pattern := range patterns {
		directoryOnly := strings.HasSuffix(pattern, "/")
		pattern = strings.TrimSuffix(pattern, "/")
		for count := 1; count <= len(parts); count++ {
			if directoryOnly && count == len(parts) && !directory {
				continue
			}
			if !strings.Contains(pattern, "/") {
				if match, _ := path.Match(pattern, parts[count-1]); match {
					return true
				}
			} else if sourceSegmentsMatch(strings.Split(pattern, "/"), parts[:count]) {
				return true
			}
		}
	}
	return false
}

func sourceSegmentsMatch(pattern, parts []string) bool {
	type key struct{ i, j int }
	seen, matches := map[key]bool{}, map[key]bool{}
	var match func(int, int) bool
	match = func(i, j int) bool {
		k := key{i, j}
		if seen[k] {
			return matches[k]
		}
		seen[k] = true
		result := false
		if i == len(pattern) {
			result = j == len(parts)
		} else if pattern[i] == "**" {
			result = match(i+1, j) || j < len(parts) && match(i, j+1)
		} else if j < len(parts) {
			ok, _ := path.Match(pattern[i], parts[j])
			result = ok && match(i+1, j+1)
		}
		matches[k] = result
		return result
	}
	return match(0, 0)
}

// NormalizeSourcePaths removes duplicate and nested selections consistently
// for preview and prepare. A selected symlink is always rejected.
func NormalizeSourcePaths(inputs []string) ([]string, error) {
	if len(inputs) == 0 || len(inputs) > 4096 {
		return nil, ErrPath
	}
	type input struct {
		path, key string
		directory bool
	}
	items := make([]input, 0, len(inputs))
	for _, raw := range inputs {
		absolute, err := filepath.Abs(raw)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(absolute)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
			return nil, ErrPath
		}
		key := absolute
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		items = append(items, input{absolute, key, info.IsDir()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].key < items[j].key })
	var selected []input
	for _, candidate := range items {
		covered := false
		for _, prior := range selected {
			if candidate.key == prior.key || prior.directory && strings.HasPrefix(candidate.key, strings.TrimRight(prior.key, string(filepath.Separator))+string(filepath.Separator)) {
				covered = true
				break
			}
		}
		if !covered {
			selected = append(selected, candidate)
		}
	}
	paths := make([]string, len(selected))
	for i, item := range selected {
		paths[i] = item.path
	}
	return paths, nil
}

type sourceVisitor func(*os.Root, string, fs.FileInfo, bool) error

// os.Root and WalkDir preserve confinement and never follow symbolic links.
// Preview may count excluded descendants; prepare prunes them before any hash.
func walkSelectedSources(ctx context.Context, inputs []string, selection *SourceSelection, countExcluded bool, visit sourceVisitor) (_ []*os.Root, err error) {
	selection, err = NormalizeSourceSelection(selection)
	if err != nil {
		return nil, err
	}
	inputs, err = NormalizeSourcePaths(inputs)
	if err != nil {
		return nil, err
	}
	var roots []*os.Root
	defer func() {
		if err != nil {
			for _, root := range roots {
				_ = root.Close()
			}
		}
	}()
	visited := 0
	for _, absolute := range inputs {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		root, openErr := os.OpenRoot(filepath.Dir(absolute))
		if openErr != nil {
			return nil, openErr
		}
		roots = append(roots, root)
		base := filepath.ToSlash(filepath.Base(absolute))
		err = fs.WalkDir(root.FS(), base, func(relative string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			visited++
			if visited > 100000 {
				return errors.New("RESOURCE_LIMIT: source scan")
			}
			name := strings.TrimPrefix(relative, base+"/")
			if relative == base && entry.IsDir() {
				name = ""
			}
			excluded := sourceExcluded(selection, name, entry.IsDir())
			if excluded && !countExcluded {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !excluded && (ValidatePath(relative) != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular()) {
				return ErrPath
			}
			return visit(root, relative, info, excluded)
		})
		if err != nil {
			return nil, err
		}
	}
	return roots, nil
}

type SourcePreviewEntry struct {
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Excluded  bool   `json:"excluded"`
	Bytes     int64  `json:"bytes"`
}

type SourcePreview struct {
	Files               int                  `json:"files"`
	Directories         int                  `json:"directories"`
	Bytes               int64                `json:"bytes"`
	ExcludedFiles       int                  `json:"excluded_files"`
	ExcludedDirectories int                  `json:"excluded_directories"`
	ExcludedBytes       int64                `json:"excluded_bytes"`
	Entries             []SourcePreviewEntry `json:"entries"`
	EntriesTruncated    bool                 `json:"entries_truncated"`
}

func PreviewSources(ctx context.Context, inputs []string, selection *SourceSelection) (SourcePreview, error) {
	result := SourcePreview{Entries: []SourcePreviewEntry{}}
	roots, err := walkSelectedSources(ctx, inputs, selection, true, func(_ *os.Root, relative string, info fs.FileInfo, excluded bool) error {
		size := int64(0)
		if info.Mode().IsRegular() {
			size = info.Size()
		}
		if size < 0 || size > math.MaxInt64-result.Bytes-result.ExcludedBytes {
			return errors.New("RESOURCE_LIMIT: source size")
		}
		if excluded {
			if info.IsDir() {
				result.ExcludedDirectories++
			} else {
				result.ExcludedFiles++
				result.ExcludedBytes += size
			}
		} else if info.IsDir() {
			result.Directories++
		} else {
			result.Files++
			result.Bytes += size
		}
		if result.Files+result.Directories > MaxEntries {
			return errors.New("RESOURCE_LIMIT: included entries")
		}
		if len(result.Entries) < 200 {
			result.Entries = append(result.Entries, SourcePreviewEntry{relative, info.IsDir(), excluded, size})
		} else {
			result.EntriesTruncated = true
		}
		return nil
	})
	for _, root := range roots {
		_ = root.Close()
	}
	return result, err
}
