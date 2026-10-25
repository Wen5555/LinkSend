package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

const cacheNativeReceiptOnly = "receipt-only-v1"
const maxCacheNativeMessage = 4 << 10

var errCacheNativeConsumed = errors.New("CLIPBOARD_CACHE_NATIVE_RECEIPT_ALREADY_CONSUMED")

// There is deliberately NO production implementation of these interfaces in
// this increment. A future platform launcher must own stable image/process
// handles and verify the challenge; an IPC hash or boolean is not a proof.
// Tests implement them only in _test.go. None of the current path publishers
// call the private authorization entry points.
type cacheNativeVerifiedLaunch interface {
	verifyCacheNativeLaunch(context.Context, cacheNativeBinding) (cacheNativeLaunchIdentity, error)
}

type cacheNativeVerifiedBoot interface {
	cacheNativeBootID() (string, error)
}

type cacheNativeLaunchIdentity struct {
	BootID                  string
	HelperImageSHA256       string
	ChildEphemeralPublicKey ed25519.PublicKey
}

type cacheNativeBinding struct {
	Owner                   string `json:"owner"`
	OwnerRun                string `json:"owner_run"`
	BootID                  string `json:"boot_id"`
	CacheID                 string `json:"cache_id"`
	ProfileRootID           string `json:"profile_root_id"`
	CacheRootID             string `json:"cache_root_id"`
	ObjectID                string `json:"object_id"`
	StageID                 string `json:"stage_id"`
	ManifestDigest          string `json:"manifest_digest"`
	PlanDigest              string `json:"plan_digest"`
	InventoryDigest         string `json:"inventory_digest"`
	PublicationID           string `json:"publication_id"`
	PublicationGeneration   uint64 `json:"publication_generation"`
	Nonce                   string `json:"nonce"`
	NativeContract          string `json:"native_contract"`
	HelperImageSHA256       string `json:"helper_image_sha256"`
	ChildEphemeralPublicKey []byte `json:"child_ephemeral_public_key"`
}

type cacheNativeDelegation struct {
	Version   int                `json:"version"`
	Purpose   string             `json:"purpose"`
	Binding   cacheNativeBinding `json:"binding"`
	Signature []byte             `json:"signature"`
}

type cacheNativeCloseReceipt struct {
	Version              int                `json:"version"`
	Purpose              string             `json:"purpose"`
	DelegationDigest     string             `json:"delegation_digest"`
	Binding              cacheNativeBinding `json:"binding"`
	Sequence             uint64             `json:"sequence"`
	AdmissionClosed      bool               `json:"admission_closed"`
	SourcesClosed        bool               `json:"sources_closed"`
	NativeCloseWitnessID string             `json:"native_close_witness_id"`
	Signature            []byte             `json:"signature"`
}

type cacheNativeAuthorization struct {
	Version               int                   `json:"version"`
	Phase                 string                `json:"phase"`
	Delegation            cacheNativeDelegation `json:"delegation"`
	ConsumedReceiptDigest string                `json:"consumed_receipt_digest,omitempty"`
	ReceiptRaw            []byte                `json:"receipt,omitempty"`
	QuiescentObservedAt   int64                 `json:"quiescent_observed_at,omitempty"`
}

func cacheNativeMessage(domain string, value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil || len(body) > maxCacheNativeMessage {
		return nil, errClipboardCacheOwnership
	}
	return append([]byte(domain+"\x00"), body...), nil
}

func cacheNativeDelegationMessage(value cacheNativeDelegation) ([]byte, error) {
	value.Signature = nil
	return cacheNativeMessage("LinkSend clipboard cache native delegation v1", value)
}

func cacheNativeReceiptMessage(value cacheNativeCloseReceipt) ([]byte, error) {
	value.Signature = nil
	return cacheNativeMessage("LinkSend clipboard cache native quiescent v1", value)
}

func cacheNativeSHA(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func cacheNativeDelegationDigest(value cacheNativeDelegation) (string, error) {
	body, err := json.Marshal(value)
	if err != nil || len(body) > maxCacheNativeMessage {
		return "", errClipboardCacheOwnership
	}
	return cacheNativeSHA(body), nil
}

func cacheNativeInventoryDigest(record clipboardCacheRecord) (string, error) {
	if !record.Sealed || len(record.Cleanup) == 0 || validateClipboardCacheInventory(record) != nil {
		return "", errClipboardCacheOwnership
	}
	// encoding/json sorts string map keys. These are the original relative
	// provenance names, never new paths supplied by a receipt or helper.
	body, err := json.Marshal(struct {
		Version  int                           `json:"version"`
		CacheID  string                        `json:"cache_id"`
		Manifest string                        `json:"manifest"`
		Plan     string                        `json:"plan"`
		Stage    string                        `json:"stage"`
		Parts    map[string]string             `json:"parts"`
		Nodes    map[string]clipboardCacheNode `json:"nodes"`
	}{1, record.ID, record.Digest, record.PlanDigest, record.Stage, record.Parts, record.Cleanup})
	if err != nil {
		return "", errClipboardCacheOwnership
	}
	return cacheNativeSHA(append([]byte("LinkSend clipboard cache inventory v1\x00"), body...)), nil
}

func validCacheNativeBoot(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func (s *Service) validateCacheNativeAuthorization(record clipboardCacheRecord) error {
	n := record.Native
	if n == nil {
		return nil
	}
	// Even an authentic close claim is receipt-only. No native record may be
	// fed to the existing unpublished reclaimable/deleting state machine.
	if n.Version != 1 || (record.State != "publishing" && record.State != "published" && record.State != "protected") {
		return errClipboardCacheOwnership
	}
	d := n.Delegation
	b := d.Binding
	inventory, err := cacheNativeInventoryDigest(record)
	if err != nil || d.Version != 1 || d.Purpose != "cache-native-delegation" || b.NativeContract != cacheNativeReceiptOnly ||
		b.Owner != record.Owner || b.Owner != s.identity.ID() || b.OwnerRun != record.Run || b.CacheID != record.ID ||
		b.CacheRootID != record.CacheRoot || b.ObjectID != record.Directory || b.StageID != record.Stage || b.ProfileRootID == "" ||
		b.ManifestDigest != record.Digest || !validClipboardFileID(b.ManifestDigest, 32) || b.PlanDigest != record.PlanDigest || !validClipboardFileID(b.PlanDigest, 32) ||
		b.InventoryDigest != inventory || !validCacheNativeBoot(b.BootID) || !validClipboardFileID(b.PublicationID, 16) || b.PublicationGeneration != 1 ||
		!validClipboardFileID(b.Nonce, 32) || !validClipboardFileID(b.HelperImageSHA256, 32) || len(b.ChildEphemeralPublicKey) != ed25519.PublicKeySize {
		return errClipboardCacheOwnership
	}
	message, err := cacheNativeDelegationMessage(d)
	if err != nil || !ed25519.Verify(s.identity.PublicKey(), message, d.Signature) {
		return errClipboardCacheOwnership
	}
	switch n.Phase {
	case "delegated":
		if n.ConsumedReceiptDigest != "" || len(n.ReceiptRaw) != 0 || n.QuiescentObservedAt != 0 {
			return errClipboardCacheOwnership
		}
	case "quiescent":
		if n.QuiescentObservedAt <= 0 || n.ConsumedReceiptDigest != cacheNativeSHA(n.ReceiptRaw) {
			return errClipboardCacheOwnership
		}
		if _, err := verifyCacheNativeCloseReceipt(d, n.ReceiptRaw); err != nil {
			return err
		}
	default:
		return errClipboardCacheOwnership
	}
	return nil
}

func verifyCacheNativeCloseReceipt(delegation cacheNativeDelegation, raw []byte) (cacheNativeCloseReceipt, error) {
	var receipt cacheNativeCloseReceipt
	if len(raw) == 0 || len(raw) > maxCacheNativeMessage {
		return receipt, errClipboardCacheOwnership
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil || decoder.Decode(new(any)) != io.EOF {
		return receipt, errClipboardCacheOwnership
	}
	canonical, err := json.Marshal(receipt)
	expectedBinding, _ := json.Marshal(delegation.Binding)
	actualBinding, _ := json.Marshal(receipt.Binding)
	digest, digestErr := cacheNativeDelegationDigest(delegation)
	if err != nil || !bytes.Equal(canonical, raw) || digestErr != nil || receipt.Version != 1 || receipt.Purpose != "cache-native-quiescent" ||
		receipt.DelegationDigest != digest || !bytes.Equal(actualBinding, expectedBinding) || receipt.Sequence != 1 || !receipt.AdmissionClosed || !receipt.SourcesClosed || !validClipboardFileID(receipt.NativeCloseWitnessID, 16) {
		return receipt, errClipboardCacheOwnership
	}
	message, err := cacheNativeReceiptMessage(receipt)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(delegation.Binding.ChildEphemeralPublicKey), message, receipt.Signature) {
		return receipt, errClipboardCacheOwnership
	}
	return receipt, nil
}

func randomCacheNativeHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// sealNativeDelegation is unreachable from current production publishers. Its
// opaque launcher proof has no production implementation yet. The current run
// may only seal its receiver's actual, fully closed storage inventory.
func (r *clipboardCacheReceipt) sealNativeDelegation(ctx context.Context, launch cacheNativeVerifiedLaunch) (cacheNativeDelegation, error) {
	var empty cacheNativeDelegation
	if launch == nil || r.service == nil || r.destination == nil || r.destination.profile == nil || r.destination.object == nil {
		return empty, errClipboardCacheOwnership
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	s := r.service
	done, err := s.beginProfileWork()
	if err != nil {
		return empty, err
	}
	defer done()
	c := &s.clipboardCache
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active[r.record.ID] || r.nativeStarted || r.record.Run != s.epoch || r.record.State != "verified" || r.record.Native != nil || len(r.finalInventory) == 0 {
		return empty, errClipboardCacheOwnership
	}
	var current string
	if err := s.store.db.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=? THEN value ELSE '' END FROM metadata WHERE key=?`, 2*transfer.MaxMetadata, clipboardCacheKeyPrefix+r.record.ID).Scan(&current); err != nil {
		return empty, err
	}
	base, err := s.decodeCacheRecord(r.record.ID, current)
	if err != nil || current != r.raw || base.Run != s.epoch || base.State != "verified" || base.Native != nil {
		return empty, errors.Join(errClipboardCacheOwnership, err)
	}
	if err := r.destination.visible(); err != nil {
		return empty, err
	}
	profile, err := clipboardRootIdentity(r.destination.profile)
	if err != nil {
		return empty, err
	}
	cache, err := clipboardRootIdentity(r.destination.cache)
	if err != nil || cache != base.CacheRoot {
		return empty, errors.Join(errClipboardCacheOwnership, err)
	}
	object, err := clipboardRootIdentity(r.destination.object)
	if err != nil || object != base.Directory {
		return empty, errors.Join(errClipboardCacheOwnership, err)
	}
	next := base
	next.Sealed = true
	next.Cleanup = make(map[string]clipboardCacheNode, len(r.finalInventory))
	for name, node := range r.finalInventory {
		next.Cleanup[name] = node
	}
	if err := checkClipboardCacheTree(ctx, r.destination.object, next.Cleanup, false); err != nil {
		return empty, err
	}
	inventory, err := cacheNativeInventoryDigest(next)
	if err != nil {
		return empty, err
	}
	publication, err := randomCacheNativeHex(16)
	if err != nil {
		return empty, err
	}
	nonce, err := randomCacheNativeHex(32)
	if err != nil {
		return empty, err
	}
	binding := cacheNativeBinding{Owner: base.Owner, OwnerRun: base.Run, CacheID: base.ID, ProfileRootID: profile, CacheRootID: base.CacheRoot, ObjectID: base.Directory, StageID: base.Stage,
		ManifestDigest: base.Digest, PlanDigest: base.PlanDigest, InventoryDigest: inventory, PublicationID: publication, PublicationGeneration: 1, Nonce: nonce, NativeContract: cacheNativeReceiptOnly}
	identity, err := launch.verifyCacheNativeLaunch(ctx, binding)
	if err != nil || !validCacheNativeBoot(identity.BootID) || !validClipboardFileID(identity.HelperImageSHA256, 32) || len(identity.ChildEphemeralPublicKey) != ed25519.PublicKeySize {
		return empty, errors.Join(errClipboardCacheOwnership, err)
	}
	binding.BootID, binding.HelperImageSHA256 = identity.BootID, identity.HelperImageSHA256
	binding.ChildEphemeralPublicKey = bytes.Clone(identity.ChildEphemeralPublicKey)
	delegation := cacheNativeDelegation{Version: 1, Purpose: "cache-native-delegation", Binding: binding}
	message, err := cacheNativeDelegationMessage(delegation)
	if err != nil {
		return empty, err
	}
	delegation.Signature = s.identity.Sign(message)
	next.Native = &cacheNativeAuthorization{Version: 1, Phase: "delegated", Delegation: delegation}
	if next.Revision == math.MaxUint64 {
		return empty, errClipboardCacheOwnership
	}
	next.State, next.Revision = "publishing", next.Revision+1
	encoded, err := s.encodeCacheRecord(next)
	if err != nil {
		return empty, err
	}
	if err := s.writeCacheNativeCAS(ctx, next.ID, current, encoded); err != nil {
		return empty, err
	}
	r.record, r.raw = next, encoded
	return delegation, nil
}

// A new main may consume the original publication's old OwnerRun. It must not
// mint a new delegation, change that run, or reactivate a legacy/native write.
// The close booleans are signed claims, NOT proof of native lifetime semantics.
func (s *Service) consumeCacheNativeReceipt(ctx context.Context, id, expectedRaw string, body []byte, boot cacheNativeVerifiedBoot) (bool, error) {
	if boot == nil || !validClipboardFileID(id, 16) || len(expectedRaw) == 0 || len(expectedRaw) > 2*transfer.MaxMetadata || len(body) == 0 || len(body) > maxCacheNativeMessage {
		return false, errClipboardCacheOwnership
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	currentBoot, err := boot.cacheNativeBootID()
	if err != nil || !validCacheNativeBoot(currentBoot) {
		return false, errors.Join(errClipboardCacheOwnership, err)
	}
	done, err := s.beginProfileWork()
	if err != nil {
		return false, err
	}
	defer done()
	c := &s.clipboardCache
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active[id] {
		return false, errClipboardCacheOwnership
	}
	var raw string
	if err := s.store.db.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=? THEN value ELSE '' END FROM metadata WHERE key=?`, 2*transfer.MaxMetadata, clipboardCacheKeyPrefix+id).Scan(&raw); err != nil {
		return false, err
	}
	record, err := s.decodeCacheRecord(id, raw)
	if err != nil || raw != expectedRaw || record.Native == nil {
		return false, errors.Join(errClipboardCacheOwnership, err)
	}
	if _, err := verifyCacheNativeCloseReceipt(record.Native.Delegation, body); err != nil {
		return false, err
	}
	if record.Native.Phase == "quiescent" {
		if record.Native.ConsumedReceiptDigest == cacheNativeSHA(body) {
			return false, errCacheNativeConsumed
		}
		return false, errClipboardCacheOwnership
	}
	if record.Native.Phase != "delegated" || record.Revision == math.MaxUint64 {
		return false, errClipboardCacheOwnership
	}
	destination, err := openRecordedClipboardCache(s.cfg.DataDir, record)
	if err != nil {
		return false, err
	}
	defer destination.Close()
	if destination.object == nil || destination.profile == nil {
		return false, errClipboardCacheOwnership
	}
	profile, err := clipboardRootIdentity(destination.profile)
	if err != nil || profile != record.Native.Delegation.Binding.ProfileRootID {
		return false, errors.Join(errClipboardCacheOwnership, err)
	}
	if err := checkClipboardCacheTree(ctx, destination.object, record.Cleanup, false); err != nil {
		return false, err
	}
	// A boot change alone provides no authority. A receipt still has to match
	// its original signed BootID, inventory and exact publication. There is no
	// platform boot implementation or deletion capability in this increment.
	next := record
	native := *record.Native
	native.Phase, native.ConsumedReceiptDigest = "quiescent", cacheNativeSHA(body)
	native.ReceiptRaw, native.QuiescentObservedAt = bytes.Clone(body), time.Now().Unix()
	next.Native, next.Revision = &native, record.Revision+1
	encoded, err := s.encodeCacheRecord(next)
	if err != nil {
		return false, err
	}
	if err := s.writeCacheNativeCAS(ctx, id, raw, encoded); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) writeCacheNativeCAS(ctx context.Context, id, previous, next string) error {
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE metadata SET value=? WHERE key=? AND value=?`, next, clipboardCacheKeyPrefix+id, previous)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return errors.Join(errClipboardCacheOwnership, err)
	}
	var actual string
	if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=? THEN value ELSE '' END FROM metadata WHERE key=?`, 2*transfer.MaxMetadata, clipboardCacheKeyPrefix+id).Scan(&actual); err != nil {
		return err
	}
	if actual != next {
		return errClipboardCacheOwnership
	}
	if _, err := s.decodeCacheRecord(id, actual); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := s.store.db.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=? THEN value ELSE '' END FROM metadata WHERE key=?`, 2*transfer.MaxMetadata, clipboardCacheKeyPrefix+id).Scan(&actual); err != nil {
		return err
	}
	if actual != next {
		return errClipboardCacheOwnership
	}
	_, err = s.decodeCacheRecord(id, actual)
	return err
}

func unchangedCacheNativeAuthorization(raw string, next *cacheNativeAuthorization) bool {
	var previous struct {
		Native *cacheNativeAuthorization `json:"native"`
	}
	if json.Unmarshal([]byte(raw), &previous) != nil {
		return false
	}
	a, aErr := json.Marshal(previous.Native)
	b, bErr := json.Marshal(next)
	return aErr == nil && bErr == nil && bytes.Equal(a, b)
}
