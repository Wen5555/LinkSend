package app

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

const clipboardCacheKeyPrefix = "clipboard-cache/v1/"

var errClipboardCacheOwnership = errors.New("CLIPBOARD_CACHE_OWNERSHIP_UNPROVEN")

// This is private lifecycle metadata in the existing profile database. Paths
// are relative manifest names; no absolute path or native delete authority is
// reconstructed from a row. Every filesystem operation still needs pinned roots.
type clipboardCacheRecord struct {
	Version    int                           `json:"version"`
	Purpose    string                        `json:"purpose"`
	Owner      string                        `json:"owner"`
	Run        string                        `json:"run"`
	ID         string                        `json:"id"`
	CacheRoot  string                        `json:"cache_root"`
	Directory  string                        `json:"directory"`
	Peer       string                        `json:"peer"`
	Created    int64                         `json:"created"`
	Revision   uint64                        `json:"revision"`
	State      string                        `json:"state"`
	Manifest   *transfer.Manifest            `json:"manifest,omitempty"`
	Digest     string                        `json:"manifest_digest,omitempty"`
	Stage      string                        `json:"stage_identity,omitempty"`
	PlanDigest string                        `json:"plan_digest,omitempty"`
	Parts      map[string]string             `json:"parts,omitempty"`
	Signature  []byte                        `json:"signature"`
	Sealed     bool                          `json:"sealed,omitempty"`
	Cleanup    map[string]clipboardCacheNode `json:"cleanup,omitempty"`
	Native     *cacheNativeAuthorization     `json:"native,omitempty"`
}

type clipboardCacheReceipt struct {
	service        *Service
	destination    *clipboardCacheDestination
	record         clipboardCacheRecord
	raw            string
	nativeStarted  bool // Current worker proof only; never inferred after restart.
	finalInventory map[string]clipboardCacheNode
	release        func()
}

func clipboardRootIdentity(root *os.Root) (string, error) {
	f, err := root.Open(".")
	if err != nil {
		return "", err
	}
	defer f.Close()
	return inboxFileIdentity(f)
}

func cacheRecordMessage(record clipboardCacheRecord) ([]byte, error) {
	record.Signature = nil
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	return append([]byte("LinkSend clipboard cache metadata v1\x00"), data...), nil
}

func (s *Service) encodeCacheRecord(record clipboardCacheRecord) (string, error) {
	if err := validateClipboardCacheInventory(record); err != nil {
		return "", err
	}
	if err := s.validateCacheNativeAuthorization(record); err != nil {
		return "", err
	}
	message, err := cacheRecordMessage(record)
	if err != nil {
		return "", err
	}
	record.Signature = s.identity.Sign(message)
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	if len(data) > 2*transfer.MaxMetadata {
		return "", errClipboardCacheOwnership
	}
	return string(data), nil
}

func (s *Service) decodeCacheRecord(id, raw string) (clipboardCacheRecord, error) {
	var record clipboardCacheRecord
	if len(raw) > 2*transfer.MaxMetadata {
		return record, errClipboardCacheOwnership
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return record, errClipboardCacheOwnership
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return record, errClipboardCacheOwnership
	}
	// Existing writers already emit this canonical typed representation. This
	// also rejects duplicate/case-aliased fields instead of silently accepting
	// encoding/json's last value. Noncanonical evidence is kept, never repaired.
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(canonical, []byte(raw)) {
		return record, errClipboardCacheOwnership
	}
	if record.Version != 1 || record.Purpose != "clipboard-files" || record.Owner != s.identity.ID() || record.ID != id ||
		!validClipboardFileID(id, 16) || !validClipboardFileID(record.Run, 16) || record.CacheRoot == "" || record.Directory == "" || !validClipboardFileID(record.Peer, 32) || record.Created <= 0 || record.Revision == 0 {
		return record, errClipboardCacheOwnership
	}
	switch record.State {
	case "receiving", "verified", "publishing", "published", "protected", "abandoned", "reclaimable", "deleting":
	default:
		return record, errClipboardCacheOwnership
	}
	if record.Manifest != nil && (record.Manifest.TransferID != id || record.Manifest.Validate() != nil || record.Digest != record.Manifest.Digest()) {
		return record, errClipboardCacheOwnership
	}
	if record.Manifest == nil && (record.Digest != "" || record.Stage != "" || record.PlanDigest != "" || len(record.Parts) != 0) {
		return record, errClipboardCacheOwnership
	}
	if record.State == "verified" || record.State == "publishing" || record.State == "published" || record.State == "protected" {
		if record.Manifest == nil || record.Stage == "" || record.PlanDigest == "" {
			return record, errClipboardCacheOwnership
		}
	}
	if len(record.Parts) > transfer.MaxEntries {
		return record, errClipboardCacheOwnership
	}
	if err := validateClipboardCacheInventory(record); err != nil {
		return record, err
	}
	if err := s.validateCacheNativeAuthorization(record); err != nil {
		return record, err
	}
	for name, identity := range record.Parts {
		if record.Manifest == nil || identity == "" {
			return record, errClipboardCacheOwnership
		}
		var part uint64
		if len(name) < 6 || name[len(name)-5:] != ".part" {
			return record, errClipboardCacheOwnership
		}
		part, err := strconv.ParseUint(name[:len(name)-5], 10, 32)
		if err != nil || part >= uint64(len(record.Manifest.Files)) || record.Manifest.Files[part].Type != "file" || name != fmt.Sprintf("%d.part", part) {
			return record, errClipboardCacheOwnership
		}
	}
	message, err := cacheRecordMessage(record)
	if err != nil || !ed25519.Verify(s.identity.PublicKey(), message, record.Signature) {
		return record, errClipboardCacheOwnership
	}
	return record, nil
}

func (s *Service) registerClipboardCache(d *clipboardCacheDestination, peer string) (*clipboardCacheReceipt, error) {
	if !validClipboardFileID(peer, 32) {
		return nil, errClipboardCacheOwnership
	}
	if s.store == nil || s.storeErr != nil {
		return nil, errors.New("CLIPBOARD_CACHE_METADATA_UNAVAILABLE")
	}
	release, err := s.clipboardCache.acquire(d.id)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			release()
		}
	}()
	cache, err := clipboardRootIdentity(d.cache)
	if err != nil {
		return nil, err
	}
	directory, err := clipboardRootIdentity(d.object)
	if err != nil {
		return nil, err
	}
	r := &clipboardCacheReceipt{service: s, destination: d, release: release, record: clipboardCacheRecord{Version: 1, Purpose: "clipboard-files", Owner: s.identity.ID(), Run: s.epoch,
		ID: d.id, CacheRoot: cache, Directory: directory, Peer: peer, Created: time.Now().Unix(), Revision: 1, State: "receiving"}}
	raw, err := s.encodeCacheRecord(r.record)
	if err != nil {
		return nil, err
	}
	result, err := s.store.db.Exec(`INSERT INTO metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`, clipboardCacheKeyPrefix+d.id, raw)
	if err != nil {
		return nil, err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	if err := r.readback(raw); err != nil {
		return nil, err
	}
	r.raw = raw
	keep = true
	return r, nil
}

func (r *clipboardCacheReceipt) readback(expected string) error {
	var raw string
	if err := r.service.store.db.QueryRow(`SELECT value FROM metadata WHERE key=?`, clipboardCacheKeyPrefix+r.record.ID).Scan(&raw); err != nil {
		return err
	}
	if raw != expected {
		return errClipboardCacheOwnership
	}
	_, err := r.service.decodeCacheRecord(r.record.ID, raw)
	return err
}

func (r *clipboardCacheReceipt) save(next clipboardCacheRecord) error {
	if r.record.Run != r.service.epoch || next.Run != r.record.Run || r.record.Revision == math.MaxUint64 || next.Revision == 0 || next.Revision != r.record.Revision+1 {
		return errClipboardCacheOwnership
	}
	if !unchangedCacheNativeAuthorization(r.raw, next.Native) {
		return errClipboardCacheOwnership
	}
	raw, err := r.service.encodeCacheRecord(next)
	if err != nil {
		return err
	}
	result, err := r.service.store.db.Exec(`UPDATE metadata SET value=? WHERE key=? AND value=?`, raw, clipboardCacheKeyPrefix+r.record.ID, r.raw)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(errClipboardCacheOwnership, err)
	}
	if err := r.readback(raw); err != nil {
		return err
	}
	r.record, r.raw = next, raw
	return nil
}

func (r *clipboardCacheReceipt) bindManifest(m transfer.Manifest) error {
	if r.record.State != "receiving" || m.TransferID != r.record.ID || m.Validate() != nil {
		return errClipboardCacheOwnership
	}
	if r.record.Manifest != nil && r.record.Digest != m.Digest() {
		return errClipboardCacheOwnership
	}
	next := r.record
	next.Manifest, next.Digest, next.Revision = &m, m.Digest(), next.Revision+1
	return r.save(next)
}

func (r *clipboardCacheReceipt) bindStage(storage transfer.ReceiveStorage) error {
	plan := storage.Plan
	m := r.record.Manifest
	if m == nil || r.record.State != "receiving" || plan.Validate(*m) != nil || storage.Manifest.Digest() != r.record.Digest || storage.Peer != r.record.Peer || storage.Root == nil || storage.Stage == nil {
		return errClipboardCacheOwnership
	}
	rootID, err := clipboardRootIdentity(storage.Root)
	if err != nil || rootID != r.record.Directory {
		return errors.Join(errClipboardCacheOwnership, err)
	}
	stage := storage.Stage
	state, err := readInboxStageState(stage, "state.json")
	if err != nil {
		return err
	}
	if state.Peer != r.record.Peer || state.Digest != r.record.Digest || state.Manifest.Digest() != r.record.Digest || state.Plan == nil || state.PlanDigest != plan.Digest() {
		return errClipboardCacheOwnership
	}
	stageID, err := clipboardRootIdentity(stage)
	if err != nil {
		return err
	}
	parts, err := stagePartIdentities(stage, *m)
	if err != nil {
		return err
	}
	if len(parts) != len(storage.Files) {
		return errClipboardCacheOwnership
	}
	for id, file := range storage.Files {
		identity, err := inboxFileIdentity(file)
		if err != nil || parts[fmt.Sprintf("%d.part", id)] != identity {
			return errors.Join(errClipboardCacheOwnership, err)
		}
	}
	next := r.record
	next.Stage, next.Parts, next.PlanDigest, next.Revision = stageID, parts, plan.Digest(), next.Revision+1
	return r.save(next)
}

func (r *clipboardCacheReceipt) advance(state string) error {
	allowed := false
	switch r.record.State {
	case "receiving":
		allowed = state == "verified" || state == "abandoned"
	case "verified":
		allowed = state == "publishing" || state == "abandoned"
	case "publishing":
		allowed = (r.nativeStarted && (state == "published" || state == "protected")) || (!r.nativeStarted && state == "abandoned")
	}
	if !allowed {
		return errClipboardCacheOwnership
	}
	if state == "verified" && (r.record.Manifest == nil || r.record.Stage == "" || r.record.PlanDigest == "") {
		return errClipboardCacheOwnership
	}
	next := r.record
	next.State, next.Revision = state, next.Revision+1
	return r.save(next)
}

func (r *clipboardCacheReceipt) beginNative() error {
	if r.record.Run != r.service.epoch || r.record.State != "publishing" || r.nativeStarted || r.record.Native != nil {
		return errClipboardCacheOwnership
	}
	r.nativeStarted = true
	return nil
}

// Forget only a proven, already removed unpublished object. Never erase a
// publishing intent just because native completion could not be confirmed.
func (r *clipboardCacheReceipt) forgetRemoved() error {
	if r.record.Run != r.service.epoch || r.nativeStarted || r.record.Native != nil || (r.record.State != "receiving" && r.record.State != "verified" && r.record.State != "abandoned" && r.record.State != "publishing" && r.record.State != "reclaimable" && r.record.State != "deleting") {
		return errClipboardCacheOwnership
	}
	if _, err := r.destination.cache.Lstat(r.record.ID); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(errClipboardCacheOwnership, err)
	}
	result, err := r.service.store.db.Exec(`DELETE FROM metadata WHERE key=? AND value=?`, clipboardCacheKeyPrefix+r.record.ID, r.raw)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(errClipboardCacheOwnership, err)
	}
	return nil
}

func cacheMetadataError(err error) error {
	return fmt.Errorf("CLIPBOARD_CACHE_METADATA_UNAVAILABLE: %w", err)
}
