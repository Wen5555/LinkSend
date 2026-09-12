package transfer

import (
	"context"
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
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const CapabilityReceivePlan = "receive_plan_v1"
const maxKeepBothNames = 100

var (
	ErrPlanUnsupported = errors.New("RECEIVE_PLAN_UNSUPPORTED")
	ErrPlanMismatch    = errors.New("RECEIVE_PLAN_MISMATCH")
	ErrPlanInvalid     = errors.New("INVALID_RECEIVE_PLAN")
	ErrPlanPersistence = errors.New("RECEIVE_PLAN_PERSIST_FAILED")
	ErrResumeMismatch  = errors.New("RESUME_IDENTITY_MISMATCH")
)

type ConflictPolicy string

const (
	ConflictError    ConflictPolicy = "error"
	ConflictKeepBoth ConflictPolicy = "keep_both"
	ConflictSkip     ConflictPolicy = "skip"
)

// Nil selects the complete offer; a non-nil empty list explicitly skips all.
// Selecting a directory recursively selects its descendants. Required ancestor
// directories are then included, without selecting their other descendants.
type PlanRequest struct {
	SelectedIDs    []uint32                  `json:"selected_ids"`
	ConflictPolicy ConflictPolicy            `json:"conflict_policy"`
	Policies       map[uint32]ConflictPolicy `json:"policies,omitempty"`
}

type PlannedEntry struct {
	FileID uint32         `json:"file_id"`
	Path   string         `json:"path"`
	Policy ConflictPolicy `json:"policy"`
}

// Directory belongs to this machine and never enters a checkpoint or a wire
// frame. OriginalDigest always identifies the immutable offered manifest.
type ReceivePlan struct {
	Version        int            `json:"version"`
	OriginalDigest string         `json:"original_digest"`
	Entries        []PlannedEntry `json:"entries"`
	RequestedIDs   []uint32       `json:"requested_ids,omitempty"`
	Directory      string         `json:"-"`
}

type Selection struct {
	Version int      `json:"version"`
	IDs     []uint32 `json:"ids"`
	Digest  string   `json:"digest"`
}

type Offer struct {
	Manifest     Manifest     `json:"manifest"`
	Capabilities []string     `json:"capabilities"`
	ResumePlan   *ReceivePlan `json:"resume_plan,omitempty"`
}

type ReceiveOptions struct {
	Directory   string
	Peer        string
	Accept      func(Manifest) bool
	Plan        func(context.Context, Offer) (ReceivePlan, error)
	PlanChanged func(ReceivePlan) error
	Progress    func(Progress)
}

type PlanSummary struct {
	OriginalTotal   int64 `json:"original_total"`
	SelectedTotal   int64 `json:"selected_total"`
	SelectedFiles   int   `json:"selected_files"`
	SelectedEntries int   `json:"selected_entries"`
	SkippedFiles    int   `json:"skipped_files"`
	SkippedEntries  int   `json:"skipped_entries"`
	SkippedBytes    int64 `json:"skipped_bytes"`
	RemainingBytes  int64 `json:"remaining_bytes"`
	CommitBytes     int64 `json:"commit_bytes"`
}

func cloneManifest(m Manifest) Manifest {
	m.Files = append([]FileEntry(nil), m.Files...)
	for i := range m.Files {
		m.Files[i].Chunks = append([]string(nil), m.Files[i].Chunks...)
	}
	return m
}

func clonePlan(p ReceivePlan) ReceivePlan {
	p.Entries = append(make([]PlannedEntry, 0, len(p.Entries)), p.Entries...)
	p.RequestedIDs = append([]uint32(nil), p.RequestedIDs...)
	return p
}

func FullReceivePlan(m Manifest) ReceivePlan {
	p := ReceivePlan{Version: 1, OriginalDigest: m.Digest(), Entries: make([]PlannedEntry, 0, len(m.Files))}
	for _, e := range m.Files {
		p.Entries = append(p.Entries, PlannedEntry{FileID: e.ID, Path: e.Path, Policy: ConflictError})
		p.RequestedIDs = append(p.RequestedIDs, e.ID)
	}
	return p
}

func (p ReceivePlan) Digest() string {
	b, _ := json.Marshal(p)
	return Sum(b)
}

func (p ReceivePlan) Selection() Selection {
	ids := make([]uint32, 0, len(p.Entries))
	for _, e := range p.Entries {
		ids = append(ids, e.FileID)
	}
	return Selection{Version: 1, IDs: ids, Digest: selectionDigest(p.OriginalDigest, ids)}
}

func selectionDigest(original string, ids []uint32) string {
	b, _ := json.Marshal(struct {
		Version  int      `json:"version"`
		Original string   `json:"original_digest"`
		IDs      []uint32 `json:"ids"`
	}{1, original, ids})
	return Sum(b)
}

func (s Selection) Validate(m Manifest) error {
	if s.Version != 1 || s.IDs == nil || len(s.IDs) > len(m.Files) || s.Digest != selectionDigest(m.Digest(), s.IDs) {
		return ErrPlanMismatch
	}
	selected := make(map[string]bool, len(s.IDs))
	for i, id := range s.IDs {
		if uint64(id) >= uint64(len(m.Files)) || (i > 0 && id <= s.IDs[i-1]) {
			return ErrPlanMismatch
		}
		selected[m.Files[id].Path] = true
	}
	for _, id := range s.IDs {
		for parent := path.Dir(m.Files[id].Path); parent != "."; parent = path.Dir(parent) {
			if !selected[parent] {
				return ErrPlanMismatch
			}
		}
	}
	return nil
}

func (p ReceivePlan) Validate(m Manifest) error {
	if p.Version != 1 || p.OriginalDigest != m.Digest() || p.Entries == nil || len(p.Entries) > len(m.Files) {
		return ErrPlanInvalid
	}
	if err := p.Selection().Validate(m); err != nil {
		return err
	}
	if p.RequestedIDs != nil {
		request := Selection{Version: 1, IDs: p.RequestedIDs, Digest: selectionDigest(p.OriginalDigest, p.RequestedIDs)}
		if err := request.Validate(m); err != nil {
			return err
		}
		allowed := make(map[uint32]bool, len(p.RequestedIDs))
		for _, id := range p.RequestedIDs {
			allowed[id] = true
		}
		for _, e := range p.Entries {
			if !allowed[e.FileID] {
				return ErrPlanInvalid
			}
		}
	}
	targets := make(map[string]string, len(p.Entries))
	for _, e := range p.Entries {
		if ValidatePath(e.Path) != nil || !validConflictPolicy(e.Policy) {
			return ErrPlanInvalid
		}
		key := portablePathKey(e.Path)
		if _, exists := targets[key]; exists {
			return ErrConflict
		}
		targets[key] = m.Files[e.FileID].Type
	}
	for _, e := range p.Entries {
		for parent := path.Dir(portablePathKey(e.Path)); parent != "."; parent = path.Dir(parent) {
			if targets[parent] != "directory" {
				return ErrPlanInvalid
			}
		}
	}
	if b, err := json.Marshal(p); err != nil || len(b) > MaxMetadata {
		return ErrPlanInvalid
	}
	return nil
}

func validConflictPolicy(p ConflictPolicy) bool {
	return p == ConflictError || p == ConflictKeepBoth || p == ConflictSkip
}

func (p ReceivePlan) Summary(m Manifest) PlanSummary {
	s := PlanSummary{OriginalTotal: m.TotalBytes(), SelectedEntries: len(p.Entries), SkippedEntries: len(m.Files) - len(p.Entries)}
	selected := make(map[uint32]bool, len(p.Entries))
	for _, e := range p.Entries {
		selected[e.FileID] = true
	}
	for _, e := range m.Files {
		if e.Type != "file" {
			continue
		}
		if selected[e.ID] {
			s.SelectedFiles++
			s.SelectedTotal += e.Size
		} else {
			s.SkippedFiles++
			s.SkippedBytes += e.Size
		}
	}
	s.RemainingBytes = s.SelectedTotal
	// Commit is an atomic hard link into the destination filesystem. It does
	// not copy file bytes; filesystem directory/inode overhead remains unknown.
	s.CommitBytes = 0
	return s
}

func BuildReceivePlan(ctx context.Context, directory string, m Manifest, request PlanRequest) (ReceivePlan, error) {
	if err := m.Validate(); err != nil {
		return ReceivePlan{}, err
	}
	policy := request.ConflictPolicy
	if policy == "" {
		policy = ConflictError
	}
	if !validConflictPolicy(policy) || len(request.SelectedIDs) > MaxEntries || len(request.Policies) > MaxEntries {
		return ReceivePlan{}, ErrPlanInvalid
	}
	selected := make(map[uint32]bool, len(m.Files))
	byPath := make(map[string]uint32, len(m.Files))
	for _, e := range m.Files {
		byPath[e.Path] = e.ID
	}
	if request.SelectedIDs == nil {
		for _, e := range m.Files {
			selected[e.ID] = true
		}
	} else {
		for _, id := range request.SelectedIDs {
			if uint64(id) >= uint64(len(m.Files)) {
				return ReceivePlan{}, ErrPlanInvalid
			}
			selected[id] = true
			if m.Files[id].Type == "directory" {
				prefix := m.Files[id].Path + "/"
				for _, e := range m.Files {
					if strings.HasPrefix(e.Path, prefix) {
						selected[e.ID] = true
					}
				}
			}
		}
	}
	for id := range selected {
		for parent := path.Dir(m.Files[id].Path); parent != "."; parent = path.Dir(parent) {
			selected[byPath[parent]] = true
		}
	}
	for id, override := range request.Policies {
		if uint64(id) >= uint64(len(m.Files)) || !validConflictPolicy(override) {
			return ReceivePlan{}, ErrPlanInvalid
		}
	}
	stored, err := LoadReceivePlan(ctx, directory, "", m)
	if err != nil {
		return ReceivePlan{}, err
	}
	if stored != nil {
		matches := func(ids []uint32) bool {
			if len(ids) != len(selected) {
				return false
			}
			for _, id := range ids {
				if !selected[id] {
					return false
				}
			}
			return true
		}
		if !matches(stored.Selection().IDs) && (stored.RequestedIDs == nil || !matches(stored.RequestedIDs)) {
			return ReceivePlan{}, ErrPlanMismatch
		}
		return clonePlan(*stored), nil
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return ReceivePlan{}, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return ReceivePlan{}, err
	}
	defer root.Close()
	inventory := newPortableInventory(ctx, root)
	plan := ReceivePlan{Version: 1, OriginalDigest: m.Digest(), Entries: make([]PlannedEntry, 0, len(selected)), Directory: directory}
	for id := range selected {
		plan.RequestedIDs = append(plan.RequestedIDs, id)
	}
	sort.Slice(plan.RequestedIDs, func(i, j int) bool { return plan.RequestedIDs[i] < plan.RequestedIDs[j] })
	order := append([]FileEntry(nil), m.Files...)
	sort.Slice(order, func(i, j int) bool {
		di, dj := strings.Count(order[i].Path, "/"), strings.Count(order[j].Path, "/")
		if di != dj {
			return di < dj
		}
		return order[i].Path < order[j].Path
	})
	mapped := make(map[string]string, len(selected))
	skippedDirectories := make([]string, 0)
	used := make(map[string]bool, len(selected))
	for _, e := range order {
		if !selected[e.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return ReceivePlan{}, err
		}
		skip := false
		for _, prefix := range skippedDirectories {
			if strings.HasPrefix(e.Path, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		target := e.Path
		if parent := path.Dir(e.Path); parent != "." {
			target = path.Join(mapped[parent], path.Base(e.Path))
		}
		entryPolicy := policy
		if override, ok := request.Policies[e.ID]; ok {
			entryPolicy = override
		}
		existing, err := inventory.lookup(target)
		if err != nil {
			return ReceivePlan{}, err
		}
		conflict := existing != nil
		if existing != nil && e.Type == "directory" && existing.IsDir() && existing.Mode()&os.ModeSymlink == 0 && existing.Name() == path.Base(target) && entryPolicy != ConflictKeepBoth {
			conflict = false // an existing regular directory is a structural parent
		}
		if conflict || used[portablePathKey(target)] {
			switch entryPolicy {
			case ConflictSkip:
				if e.Type == "directory" {
					skippedDirectories = append(skippedDirectories, e.Path+"/")
				}
				continue
			case ConflictKeepBoth:
				target, err = inventory.available(target, used)
				if err != nil {
					return ReceivePlan{}, err
				}
			default:
				return ReceivePlan{}, ErrConflict
			}
		}
		mapped[e.Path] = target
		used[portablePathKey(target)] = true
		plan.Entries = append(plan.Entries, PlannedEntry{FileID: e.ID, Path: target, Policy: entryPolicy})
	}
	sort.Slice(plan.Entries, func(i, j int) bool { return plan.Entries[i].FileID < plan.Entries[j].FileID })
	if err = plan.Validate(m); err != nil {
		return ReceivePlan{}, err
	}
	return plan, nil
}

// LoadReceivePlan reads bounded local metadata only. A nil result means this
// transfer has no checkpoint here. An empty peer is for preview only; opening
// the receiver always performs the authenticated peer comparison again.
func LoadReceivePlan(ctx context.Context, directory, peer string, m Manifest) (*ReceivePlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := ".linksend-" + m.TransferID
	st, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, ErrPath
	}
	stage, err := root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	defer stage.Close()
	f, err := stage.Open("state.json")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxResumeStateBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResumeStateBytes {
		return nil, ErrPlanInvalid
	}
	var state ResumeState
	if err = json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Digest != m.Digest() || state.Manifest.Digest() != m.Digest() || (peer != "" && state.Peer != peer) {
		return nil, ErrResumeMismatch
	}
	plan := FullReceivePlan(m)
	if state.Plan != nil {
		if state.PlanDigest != state.Plan.Digest() {
			return nil, ErrPlanMismatch
		}
		plan = clonePlan(*state.Plan)
	}
	plan.Directory = directory
	if err = plan.Validate(m); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return &plan, nil
}

// The portable namespace also forbids case-only collisions on a case-sensitive
// host. Read directory entries in bounded batches rather than unbounded ReadDir.
type portableInventory struct {
	ctx         context.Context
	root        *os.Root
	directories map[string]map[string]string
	entries     int
}

func portablePathKey(p string) string { return cases.Fold().String(norm.NFC.String(p)) }
func newPortableInventory(ctx context.Context, root *os.Root) *portableInventory {
	return &portableInventory{ctx: ctx, root: root, directories: make(map[string]map[string]string)}
}

func (v *portableInventory) refresh(parent string) { delete(v.directories, parent); v.entries = 0 }

func (v *portableInventory) lookup(target string) (fs.FileInfo, error) {
	if err := v.ctx.Err(); err != nil {
		return nil, err
	}
	if st, err := v.root.Lstat(target); err == nil {
		return st, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	parent := path.Dir(target)
	if names, ok := v.directories[parent]; ok {
		if actual, found := names[portablePathKey(path.Base(target))]; found {
			return v.root.Lstat(path.Join(parent, actual))
		}
		return nil, nil
	}
	f, err := v.root.Open(parent)
	if errors.Is(err, fs.ErrNotExist) {
		v.directories[parent] = make(map[string]string)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names := make(map[string]string)
	for {
		if err := v.ctx.Err(); err != nil {
			return nil, err
		}
		entries, readErr := f.ReadDir(128)
		for _, e := range entries {
			v.entries++
			if v.entries > MaxEntries*4 {
				return nil, errors.New("DIRECTORY_SCAN_LIMIT")
			}
			names[portablePathKey(e.Name())] = e.Name()
		}
		if errors.Is(readErr, io.EOF) {
			v.directories[parent] = names
			if actual, ok := names[portablePathKey(path.Base(target))]; ok {
				return v.root.Lstat(path.Join(parent, actual))
			}
			return nil, nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}

func (v *portableInventory) available(base string, used map[string]bool) (string, error) {
	parent, name := path.Dir(base), path.Base(base)
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; n <= maxKeepBothNames; n++ {
		suffix := fmt.Sprintf(" (%d)", n)
		limit := 240 - len(ext) - len(suffix)
		if parent != "." {
			limit = min(limit, 1024-len(parent)-1-len(ext)-len(suffix))
		}
		if limit < 1 {
			return "", ErrConflict
		}
		short := stem
		for len(short) > limit {
			_, size := utf8.DecodeLastRuneInString(short)
			short = short[:len(short)-size]
		}
		candidate := path.Join(parent, short+suffix+ext)
		if used[portablePathKey(candidate)] {
			continue
		}
		if ValidatePath(candidate) != nil {
			return "", ErrPath
		}
		st, err := v.lookup(candidate)
		if err != nil {
			return "", err
		}
		if st == nil {
			return candidate, nil
		}
	}
	return "", ErrConflict
}
