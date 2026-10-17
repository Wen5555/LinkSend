package identity

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrMembershipRecoveryPending = fmt.Errorf("%w: explicit membership recovery pending", ErrAuthentication)

const maxRetiredMemberships = 128
const trustRecoveryBackup = "trust.json.pre-schema-3"

// The invitation is deliberately absent. A resumed join uses a fresh code or
// an authenticated Devices response proving an earlier request completed.
type MembershipRecovery struct {
	ID                  string    `json:"id"`
	Mode                string    `json:"mode"`
	EndpointFingerprint string    `json:"endpoint_fingerprint"`
	Phase               string    `json:"phase"`
	CreatedAt           time.Time `json:"created_at"`
}

type MembershipRecoveryReceipt struct {
	ID                  string    `json:"id"`
	EndpointFingerprint string    `json:"endpoint_fingerprint"`
	GroupID             string    `json:"group_id"`
	LocalIncarnation    string    `json:"local_incarnation"`
	CompletedAt         time.Time `json:"completed_at"`
}

func hasMembershipGrant(peer TrustedPeer) bool {
	return peer.GrantKind == "group" || peer.GrantKind == "group+lan" || peer.GrantKind == ""
}

func membershipActorBinding(peer TrustedPeer) (string, string) {
	if peer.GroupID == "" {
		return "", ""
	}
	if len(peer.LocalIncarnation) != 32 {
		return peer.GroupID, ""
	}
	return peer.GroupID, peer.LocalIncarnation
}

func validRecoveryHex(value string, size int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == size && hex.EncodeToString(decoded) == value
}

func validateMembershipRecoveryPolicy(f TrustFile) error {
	if f.SchemaVersion < 3 && (f.MembershipRecovery != nil || len(f.RetiredMemberships) != 0 || f.LastRecovery != nil) {
		return errUnsupportedTrustSchema
	}
	if recovery := f.MembershipRecovery; recovery != nil {
		if !validRecoveryHex(recovery.ID, 16) || !validRecoveryHex(recovery.EndpointFingerprint, 32) || recovery.Mode != "initialize" && recovery.Mode != "join" || recovery.Phase != "prepared" && recovery.Phase != "started" && recovery.Phase != "paused" || recovery.CreatedAt.IsZero() {
			return errors.New("invalid membership recovery intent")
		}
	}
	if len(f.RetiredMemberships) > maxRetiredMemberships {
		return errors.New("too many retired membership contexts")
	}
	seen := make(map[string]bool, len(f.RetiredMemberships))
	for _, retired := range f.RetiredMemberships {
		key := retired.GroupID + "/" + retired.LocalIncarnation
		if retired.GroupID == "" || len(retired.GroupID) > 128 || len(retired.LocalIncarnation) != 32 || retired.Revision == 0 || seen[key] {
			return errors.New("invalid retired membership context")
		}
		seen[key] = true
	}
	if last := f.LastRecovery; last != nil {
		if !validRecoveryHex(last.ID, 16) || !validRecoveryHex(last.EndpointFingerprint, 32) || last.GroupID == "" || len(last.GroupID) > 128 || len(last.LocalIncarnation) != 32 || last.CompletedAt.IsZero() {
			return errors.New("invalid membership recovery receipt")
		}
	}
	for _, denied := range f.DeniedPeers {
		if denied.ContextRetired && (f.SchemaVersion < 3 || denied.LocalBlock || denied.PendingSync) {
			return errors.New("invalid retired revocation context")
		}
	}
	return nil
}

func LoadMembershipRecovery(dir string) (*MembershipRecovery, *MembershipRecoveryReceipt, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	return f.MembershipRecovery, f.LastRecovery, err
}

func HasMembershipHistory(dir string) (bool, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return false, err
	}
	if len(f.MembershipSnapshots) != 0 || len(f.RetiredMemberships) != 0 || f.LastRecovery != nil {
		return true, nil
	}
	for _, peer := range f.Peers {
		if hasMembershipGrant(peer) {
			return true, nil
		}
	}
	for _, denied := range f.DeniedPeers {
		if denied.PendingSync || denied.ActorGroupID != "" || denied.TargetIncarnation != "" {
			return true, nil
		}
	}
	return false, nil
}

// PrepareMembershipRecovery persists the old-work fence before any server
// mutation. Cancellation never rolls its generation advances backwards.
func PrepareMembershipRecovery(dir, requestID, mode, endpointFingerprint string) (MembershipRecovery, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return MembershipRecovery{}, err
	}
	if existing := f.MembershipRecovery; existing != nil {
		if existing.ID == requestID && existing.Mode == mode && existing.EndpointFingerprint == endpointFingerprint {
			return *existing, nil
		}
		return MembershipRecovery{}, ErrMembershipRecoveryPending
	}
	if f.LastRecovery != nil && f.LastRecovery.ID == requestID {
		return MembershipRecovery{}, authenticationError("recovery request already completed")
	}
	contexts, err := recoveryContexts(f)
	if err != nil {
		return MembershipRecovery{}, err
	}
	intent := MembershipRecovery{ID: requestID, Mode: mode, EndpointFingerprint: endpointFingerprint, Phase: "prepared", CreatedAt: time.Now().UTC()}
	check := f
	check.SchemaVersion, check.MembershipRecovery = 3, &intent
	if err = validateMembershipRecoveryPolicy(check); err != nil {
		return MembershipRecovery{}, err
	}
	if err = ensureRecoveryTrustBackup(dir, trustMigrationBackup); err != nil {
		return MembershipRecovery{}, err
	}
	if f.SchemaVersion < 3 {
		if err = ensureRecoveryTrustBackup(dir, trustRecoveryBackup); err != nil {
			return MembershipRecovery{}, err
		}
	}
	// Keep known old context facts even when an independent LAN grant is
	// separated immediately. This also preserves history if preparation is
	// cancelled, including profiles enrolled through an older single-peer API.
	f.MembershipSnapshots = nil
	for _, snapshot := range contexts {
		retired := false
		for _, old := range f.RetiredMemberships {
			retired = retired || old.GroupID == snapshot.GroupID && old.LocalIncarnation == snapshot.LocalIncarnation
		}
		if !retired {
			f.MembershipSnapshots = append(f.MembershipSnapshots, snapshot)
		}
	}
	f.SchemaVersion, f.MembershipRecovery = 3, &intent
	for index := range f.Peers {
		peer := &f.Peers[index]
		if !hasMembershipGrant(*peer) {
			continue
		}
		if peer.GrantGeneration == ^uint64(0) {
			return MembershipRecovery{}, authenticationError("peer grant generation exhausted")
		}
		peer.GrantGeneration = max(uint64(1), peer.GrantGeneration) + 1
		peer.GrantedAt, peer.AutoAccept = intent.CreatedAt, false
		if peer.GrantKind == "group+lan" {
			peer.GroupID, peer.PeerIncarnation, peer.LocalIncarnation, peer.MembershipRevision, peer.GrantKind = "", "", "", 0, "lan"
		}
	}
	if err = saveTrust(dir, f); err != nil {
		return MembershipRecovery{}, err
	}
	return intent, nil
}

// Recovery may be the first policy write of a legacy/new profile. Preserve
// both the normal fail-closed migration marker and the exact pre-schema3
// policy; never let a missing trust.json later reopen an empty denylist.
func ensureRecoveryTrustBackup(dir, name string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("trust recovery backup is not a regular file")
		}
		data, err := readTrustFile(path)
		if err == nil {
			_, err = parseTrustFile(data)
		}
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := readTrustFile(filepath.Join(dir, "trust.json"))
	if errors.Is(err, os.ErrNotExist) {
		data, err = []byte(`{"peers":[]}`), nil
	}
	if err != nil {
		return err
	}
	if _, err = parseTrustFile(data); err != nil {
		return err
	}
	return writeNewFile(path, data)
}

func SetMembershipRecoveryStarted(dir, requestID, endpointFingerprint string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	if f.MembershipRecovery == nil || f.MembershipRecovery.ID != requestID || f.MembershipRecovery.EndpointFingerprint != endpointFingerprint {
		return authenticationError("membership recovery intent changed")
	}
	f.MembershipRecovery.Phase = "started"
	return saveTrust(dir, f)
}

// A request which may have reached the server cannot be undone by guessing
// its outcome. Keep it paused for an explicit authenticated reconciliation.
func CancelMembershipRecovery(dir, requestID string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	if f.MembershipRecovery == nil {
		return nil
	}
	if f.MembershipRecovery.ID != requestID {
		return authenticationError("membership recovery intent changed")
	}
	if f.MembershipRecovery.Phase == "prepared" {
		f.MembershipRecovery = nil
	} else {
		f.MembershipRecovery.Phase = "paused"
	}
	return saveTrust(dir, f)
}

// CheckMembershipRevocationCurrent must run under the application's membership
// operation lock before the first DELETE and each retry. Recovery takes the
// same lock, so an already selected old outbox item cannot cross the transition.
func CheckMembershipRevocationCurrent(dir string, pending DeniedPeer) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	if f.MembershipRecovery != nil {
		return ErrMembershipRecoveryPending
	}
	for _, current := range f.DeniedPeers {
		if current.ID == pending.ID && current.RequestID == pending.RequestID && current.TargetIncarnation == pending.TargetIncarnation && current.ActorGroupID == pending.ActorGroupID && current.ActorIncarnation == pending.ActorIncarnation && current.AuthorizationGeneration == pending.AuthorizationGeneration && current.PendingSync && !current.LocalBlock && !current.ContextRetired {
			return nil
		}
	}
	return authenticationError("revocation intent is no longer current")
}

func CompleteMembershipRecovery(dir, requestID, endpointFingerprint, localID string, peers []TrustedPeer) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	intent := f.MembershipRecovery
	if intent == nil || intent.ID != requestID || intent.EndpointFingerprint != endpointFingerprint {
		return authenticationError("explicit membership recovery intent required")
	}
	var local TrustedPeer
	seen := make(map[string]bool, len(peers))
	for _, peer := range peers {
		if len(peer.PublicKey) != 32 || DeviceID(peer.PublicKey) != peer.ID || seen[peer.ID] {
			return authenticationError("invalid recovery snapshot identity")
		}
		seen[peer.ID] = true
		if peer.ID == localID {
			local = peer
		}
	}
	if local.ID == "" || local.GroupID == "" || len(local.LocalIncarnation) != 32 || local.PeerIncarnation != local.LocalIncarnation || local.MembershipRevision == 0 {
		return authenticationError("recovery snapshot missing local membership")
	}
	// Retire every context known before this explicit operation. Pending ordinary
	// sync is blocked, so this set cannot silently grow from a background fetch.
	contexts, err := recoveryContexts(f)
	if err != nil {
		return err
	}
	for _, snapshot := range contexts {
		if snapshot.GroupID == local.GroupID && snapshot.LocalIncarnation == local.LocalIncarnation {
			return authenticationError("server still reports the original membership")
		}
	}
	f.RetiredMemberships = contexts
	f.MembershipSnapshots = nil
	for index := len(f.Peers) - 1; index >= 0; index-- {
		peer := f.Peers[index]
		if !hasMembershipGrant(peer) {
			continue
		}
		if peer.GrantKind == "group+lan" {
			f.Peers[index].GroupID, f.Peers[index].PeerIncarnation, f.Peers[index].LocalIncarnation, f.Peers[index].MembershipRevision, f.Peers[index].GrantKind = "", "", "", 0, "lan"
			continue
		}
		retainAuthorizationEpoch(&f, peer.ID, peer.GrantGeneration)
		f.Peers = append(f.Peers[:index], f.Peers[index+1:]...)
		actorGroup, actorInc := membershipActorBinding(peer)
		f.DeniedPeers = append(f.DeniedPeers, DeniedPeer{ID: peer.ID, Name: peer.Name, DeniedAt: time.Now().UTC(), AuthorizationGeneration: peer.GrantGeneration, TargetIncarnation: peer.PeerIncarnation, ActorGroupID: actorGroup, ActorIncarnation: actorInc, SeenRevision: peer.MembershipRevision, ContextRetired: true})
	}
	for index := range f.DeniedPeers {
		denied := &f.DeniedPeers[index]
		if !denied.LocalBlock {
			denied.PendingSync, denied.ContextRetired = false, true
		}
	}
	f.MembershipRecovery = nil
	if err = applyMembershipSnapshotFile(&f, localID, peers); err != nil {
		return err
	}
	f.LastRecovery = &MembershipRecoveryReceipt{ID: requestID, EndpointFingerprint: endpointFingerprint, GroupID: local.GroupID, LocalIncarnation: local.LocalIncarnation, CompletedAt: time.Now().UTC()}
	return saveTrust(dir, f)
}

func recoveryContexts(f TrustFile) ([]MembershipSnapshot, error) {
	var contexts []MembershipSnapshot
	add := func(snapshot MembershipSnapshot) error {
		if snapshot.GroupID == "" || len(snapshot.LocalIncarnation) != 32 || snapshot.Revision == 0 {
			return nil // Legacy unbound grants are fenced separately by peer generation.
		}
		for index := range contexts {
			if contexts[index].GroupID == snapshot.GroupID && contexts[index].LocalIncarnation == snapshot.LocalIncarnation {
				contexts[index].Revision = max(contexts[index].Revision, snapshot.Revision)
				return nil
			}
		}
		if len(contexts) == maxRetiredMemberships || len(snapshot.GroupID) > 128 {
			return errors.New("retired membership capacity reached")
		}
		contexts = append(contexts, snapshot)
		return nil
	}
	for _, snapshot := range append(append([]MembershipSnapshot(nil), f.RetiredMemberships...), f.MembershipSnapshots...) {
		if err := add(snapshot); err != nil {
			return nil, err
		}
	}
	for _, peer := range f.Peers {
		if hasMembershipGrant(peer) {
			if err := add(MembershipSnapshot{GroupID: peer.GroupID, LocalIncarnation: peer.LocalIncarnation, Revision: peer.MembershipRevision}); err != nil {
				return nil, err
			}
		}
	}
	for _, denied := range f.DeniedPeers {
		if err := add(MembershipSnapshot{GroupID: denied.ActorGroupID, LocalIncarnation: denied.ActorIncarnation, Revision: denied.SeenRevision}); err != nil {
			return nil, err
		}
	}
	return contexts, nil
}

// A successful ordinary code re-pair/switch also supersedes the previous
// local membership context. Preserve pending local removals for a new explicit
// removal intent, while permanently rejecting in-flight old snapshots.
func retireSupersededMembershipContexts(f *TrustFile, groupID, localIncarnation string) error {
	seen := make(map[string]bool, len(f.RetiredMemberships))
	for _, old := range f.RetiredMemberships {
		seen[old.GroupID+"/"+old.LocalIncarnation] = true
	}
	add := func(old MembershipSnapshot) error {
		key := old.GroupID + "/" + old.LocalIncarnation
		if old.GroupID == "" || len(old.LocalIncarnation) != 32 || old.Revision == 0 || old.GroupID == groupID && old.LocalIncarnation == localIncarnation || seen[key] {
			return nil
		}
		if len(f.RetiredMemberships) == maxRetiredMemberships || len(old.GroupID) > 128 {
			return errors.New("retired membership capacity reached")
		}
		f.RetiredMemberships = append(f.RetiredMemberships, old)
		f.SchemaVersion = 3
		seen[key] = true
		return nil
	}
	for _, old := range f.MembershipSnapshots {
		if err := add(old); err != nil {
			return err
		}
	}
	for _, peer := range f.Peers {
		if hasMembershipGrant(peer) {
			if err := add(MembershipSnapshot{GroupID: peer.GroupID, LocalIncarnation: peer.LocalIncarnation, Revision: peer.MembershipRevision}); err != nil {
				return err
			}
		}
	}
	for _, denied := range f.DeniedPeers {
		if err := add(MembershipSnapshot{GroupID: denied.ActorGroupID, LocalIncarnation: denied.ActorIncarnation, Revision: denied.SeenRevision}); err != nil {
			return err
		}
	}
	return nil
}
