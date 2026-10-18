package identity

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const recoveryTestID = "ffffffffffffffffffffffffffffffff"
const recoveryTestEndpoint = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

func recoverySnapshot(local TrustedPeer, peers []TrustedPeer, group, incarnation string, revision uint64) []TrustedPeer {
	result := append([]TrustedPeer{local}, peers...)
	for index := range result {
		p := &result[index]
		p.GroupID, p.LocalIncarnation, p.PeerIncarnation, p.MembershipRevision = group, incarnation, p.ID[:32], revision
		if p.ID == local.ID {
			p.PeerIncarnation = incarnation
		}
	}
	return result
}

func TestRecoveryFirstLegacyWriteBacksUpAndCannotReopenMissingPolicy(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "new_profile", true: "legacy_profile"}[legacy], func(t *testing.T) {
			dir := t.TempDir()
			original := []byte(`{"peers":[]}`)
			if legacy {
				original = writeLegacyPolicy(t, dir, policyTestPeer(t, "legacy"))
			}
			if _, err := PrepareMembershipRecovery(dir, recoveryTestID, "initialize", recoveryTestEndpoint); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{trustMigrationBackup, trustRecoveryBackup} {
				backup, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || !bytes.Equal(backup, original) {
					t.Fatal("recovery lost exact original policy backup", name, err)
				}
			}
			path := filepath.Join(dir, "trust.json")
			if err := os.WriteFile(path+".previous", original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadTrust(dir); err == nil {
				t.Fatal("missing schema3 policy silently reopened a baseline or previous policy")
			}
		})
	}
}

func TestRecoveryRejectsInvalidIntentAndCapacityBeforePolicyMutation(t *testing.T) {
	for _, scenario := range []string{"bad_id", "bad_mode", "bad_endpoint", "retirement_capacity", "backup_is_directory"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			local := policyTestPeer(t, "local")
			if err := ApplyMembershipSnapshot(dir, local.ID, recoverySnapshot(local, nil, "active", strings.Repeat("a", 32), 1)); err != nil {
				t.Fatal(err)
			}
			id, mode, endpoint := recoveryTestID, "initialize", recoveryTestEndpoint
			switch scenario {
			case "bad_id":
				id = "invalid"
			case "bad_mode":
				mode = "automatic"
			case "bad_endpoint":
				endpoint = "not-a-fingerprint"
			case "retirement_capacity":
				f, err := loadTrustFile(dir)
				if err != nil {
					t.Fatal(err)
				}
				f.SchemaVersion = 3
				for index := range maxRetiredMemberships {
					f.RetiredMemberships = append(f.RetiredMemberships, MembershipSnapshot{GroupID: "retired-" + strconv.Itoa(index), LocalIncarnation: strings.Repeat("b", 32), Revision: 1})
				}
				if err = saveTrust(dir, f); err != nil {
					t.Fatal(err)
				}
				if err = ApplyMembershipSnapshot(dir, local.ID, recoverySnapshot(local, nil, "active", strings.Repeat("a", 32), 2)); err != nil {
					t.Fatal("current membership refresh failed at retirement capacity", err)
				}
			case "backup_is_directory":
				if err := os.Mkdir(filepath.Join(dir, trustRecoveryBackup), 0700); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(dir, "trust.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = PrepareMembershipRecovery(dir, id, mode, endpoint); err == nil {
				t.Fatal("invalid recovery preparation succeeded")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed preparation changed live policy", err)
			}
		})
	}
}

func TestRecoveryKeepsLegacyKnownGroupWithoutInventingAnActorBinding(t *testing.T) {
	dir := t.TempDir()
	local, peer := policyTestPeer(t, "local"), policyTestPeer(t, "legacy partial membership")
	peer.GrantKind, peer.GroupID, peer.PeerIncarnation, peer.MembershipRevision, peer.GrantGeneration = "group", "old", strings.Repeat("a", 32), 50, 1
	original, err := json.Marshal(TrustFile{SchemaVersion: 2, Peers: []TrustedPeer{peer}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "trust.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = PrepareMembershipRecovery(dir, recoveryTestID, "initialize", recoveryTestEndpoint); err != nil {
		t.Fatal(err)
	}
	if err = CompleteMembershipRecovery(dir, recoveryTestID, recoveryTestEndpoint, local.ID, recoverySnapshot(local, nil, "new", strings.Repeat("d", 32), 1)); err != nil {
		t.Fatal(err)
	}
	denied, err := LoadDeniedPeers(dir)
	if err != nil || len(denied) != 1 || denied[0].ActorGroupID != "old" || denied[0].ActorIncarnation != "" || denied[0].PendingSync || !denied[0].ContextRetired {
		t.Fatalf("legacy metadata lost or an active actor was invented: %+v %v", denied, err)
	}
	backup, err := os.ReadFile(filepath.Join(dir, trustRecoveryBackup))
	if err != nil || !bytes.Equal(original, backup) {
		t.Fatal("legacy original backup changed", err)
	}
}

func TestInvalidCompleteSnapshotCannotRetireOrReplaceCurrentContext(t *testing.T) {
	for _, damage := range []string{"duplicate_peer", "missing_local", "wrong_local_incarnation", "wrong_peer_key", "missing_peer_incarnation"} {
		t.Run(damage, func(t *testing.T) {
			dir := t.TempDir()
			local, peer := policyTestPeer(t, "local"), policyTestPeer(t, "peer")
			if err := ApplyMembershipSnapshot(dir, local.ID, recoverySnapshot(local, []TrustedPeer{peer}, "old", strings.Repeat("a", 32), 1)); err != nil {
				t.Fatal(err)
			}
			candidate := recoverySnapshot(local, []TrustedPeer{peer}, "new", strings.Repeat("b", 32), 1)
			switch damage {
			case "duplicate_peer":
				candidate = append(candidate, candidate[1])
			case "missing_local":
				candidate = candidate[1:]
			case "wrong_local_incarnation":
				candidate[0].PeerIncarnation = strings.Repeat("c", 32)
			case "wrong_peer_key":
				candidate[1].PublicKey = local.PublicKey
			case "missing_peer_incarnation":
				candidate[1].PeerIncarnation = ""
			}
			path := filepath.Join(dir, "trust.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = ApplyMembershipSnapshot(dir, local.ID, candidate); err == nil {
				t.Fatal("invalid complete snapshot changed authority")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("invalid snapshot partially retired current context", err)
			}
		})
	}
}

func TestExplicitRecoveryRetiresSnapshotsAndOutboxPreservingLANAndLocalBlock(t *testing.T) {
	dir := t.TempDir()
	local, shared, rejoined, removed, pending, lan, blocked := policyTestPeer(t, "local"), policyTestPeer(t, "shared LAN"), policyTestPeer(t, "rejoined"), policyTestPeer(t, "removed"), policyTestPeer(t, "offline revoke"), policyTestPeer(t, "independent LAN"), policyTestPeer(t, "local block")
	old := recoverySnapshot(local, []TrustedPeer{shared, rejoined, removed, pending}, "old", strings.Repeat("a", 32), 50)
	if err := ApplyMembershipSnapshot(dir, local.ID, old); err != nil {
		t.Fatal(err)
	}
	shared.LastLANAddress, shared.LANAddressHistory, shared.LANRequestID = "192.168.10.5", []string{"192.168.10.4", "192.168.10.5"}, strings.Repeat("c", 32)
	if err := CommitLANPeer(dir, shared, 1); err != nil {
		t.Fatal(err)
	}
	if err := CommitLANPeer(dir, lan, 1); err != nil {
		t.Fatal(err)
	}
	if err := SetAutoAccept(dir, lan.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := SetAutoAccept(dir, shared.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := RevokePeer(dir, blocked.ID); err != nil {
		t.Fatal(err)
	}
	if err := RevokeMembershipPeer(dir, pending.ID, pending.ID[:32], "offline-revoke", 50); err != nil {
		t.Fatal(err)
	}
	denied, err := LoadDeniedPeers(dir)
	if err != nil {
		t.Fatal(err)
	}
	oldRevoke := denied[1]
	if err = CheckMembershipRevocationCurrent(dir, oldRevoke); err != nil {
		t.Fatal(err)
	}
	intent, err := PrepareMembershipRecovery(dir, recoveryTestID, "initialize", recoveryTestEndpoint)
	if err != nil || intent.Phase != "prepared" {
		t.Fatalf("prepare=%+v err=%v", intent, err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "trust.json"))
	if _, err = PrepareMembershipRecovery(dir, recoveryTestID, "initialize", recoveryTestEndpoint); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "trust.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("idempotent prepare changed authorization generations")
	}
	if generation, e := AuthorizationGeneration(dir, shared.ID); e != nil || generation <= 1 {
		t.Fatal("retained independent LAN is unavailable or reused old generation", generation, e)
	}
	if generation, e := AuthorizationGeneration(dir, lan.ID); e != nil || generation != 1 {
		t.Fatal("unrelated LAN stopped during recovery", generation, e)
	}
	if _, e := AuthorizationGeneration(dir, removed.ID); !errors.Is(e, ErrMembershipRecoveryPending) {
		t.Fatal("group-only grant survived pending recovery", e)
	}
	freshLANGeneration, err := LANPairGeneration(dir, rejoined.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = CommitLANPeer(dir, old[2], freshLANGeneration); err != nil {
		t.Fatal("new explicit LAN consent failed while recovering membership", err)
	}
	if generation, e := AuthorizationGeneration(dir, rejoined.ID); e != nil || generation != freshLANGeneration {
		t.Fatal("new independent LAN grant remains blocked by old group", generation, e)
	}
	if err = ApplyMembershipSnapshot(dir, local.ID, old); !errors.Is(err, ErrMembershipRecoveryPending) {
		t.Fatal("in-flight snapshot bypassed recovery fence", err)
	}
	if err = TrustMembershipPeer(dir, old[1]); !errors.Is(err, ErrMembershipRecoveryPending) {
		t.Fatal("single-peer enrollment bypassed recovery fence", err)
	}
	if err = CheckMembershipRevocationCurrent(dir, oldRevoke); !errors.Is(err, ErrMembershipRecoveryPending) {
		t.Fatal("old outbox could send during recovery", err)
	}
	if err = SetMembershipRecoveryStarted(dir, recoveryTestID, recoveryTestEndpoint); err != nil {
		t.Fatal(err)
	}
	if err = CompleteMembershipRecovery(dir, recoveryTestID, recoveryTestEndpoint, local.ID, old); err == nil {
		t.Fatal("unchanged old membership was declared recovered")
	}
	next := recoverySnapshot(local, []TrustedPeer{shared, rejoined, pending, blocked}, "new", strings.Repeat("d", 32), 1)
	if err = CompleteMembershipRecovery(dir, recoveryTestID, recoveryTestEndpoint, local.ID, next); err != nil {
		t.Fatal(err)
	}
	f, err := loadTrustFile(dir)
	if err != nil || f.SchemaVersion != 3 || f.MembershipRecovery != nil || f.LastRecovery == nil || f.LastRecovery.ID != recoveryTestID || len(f.RetiredMemberships) != 1 {
		t.Fatalf("persisted recovery state=%+v err=%v", f, err)
	}
	for _, peer := range f.Peers {
		if peer.ID == lan.ID {
			if peer.GrantKind != "lan" || peer.GrantGeneration != 1 || !peer.AutoAccept {
				t.Fatalf("unrelated LAN changed: %+v", peer)
			}
			continue
		}
		if peer.GroupID != "new" || peer.GrantGeneration <= 1 || peer.AutoAccept || peer.GrantedAt.IsZero() {
			t.Fatalf("new member inherited old work/consent: %+v", peer)
		}
		if peer.ID == shared.ID && (peer.LastLANAddress != shared.LastLANAddress || len(peer.LANAddressHistory) != 2 || peer.LANRequestID != shared.LANRequestID || peer.GrantKind != "group+lan") {
			t.Fatalf("independent LAN evidence lost: %+v", peer)
		}
	}
	if err = CheckPeerAllowed(dir, blocked.ID); !errors.Is(err, ErrPeerDenied) {
		t.Fatal("new group cleared explicit local block", err)
	}
	if err = CheckMembershipRevocationCurrent(dir, oldRevoke); err == nil {
		t.Fatal("retired outbox could send to new membership")
	}
	for index := range old {
		old[index].MembershipRevision = 9999
	}
	if err = ApplyMembershipSnapshot(dir, local.ID, old); err == nil {
		t.Fatal("higher-revision retired snapshot restored old group")
	}
	if err = TrustMembershipPeer(dir, old[1]); err == nil {
		t.Fatal("single-peer retired context bypassed snapshot retirement")
	}
	if err = ApplyMembershipSnapshot(dir, local.ID, next); err != nil {
		t.Fatal("current new context refresh failed", err)
	}
	denied, err = LoadDeniedPeers(dir)
	if err != nil || len(denied) != 2 {
		t.Fatalf("local block / retired removed member lost: %+v %v", denied, err)
	}
	for _, denial := range denied {
		if denial.ID == removed.ID && (!denial.ContextRetired || denial.PendingSync || denial.ActorGroupID != "old") {
			t.Fatalf("retired removal metadata changed: %+v", denial)
		}
	}
}

func TestRecoveryCancellationAndIntentBindingSurviveReload(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared", true: "network_started"}[started], func(t *testing.T) {
			dir := t.TempDir()
			local, peer := policyTestPeer(t, "local"), policyTestPeer(t, "old")
			old := recoverySnapshot(local, []TrustedPeer{peer}, "old", strings.Repeat("a", 32), 1)
			if err := ApplyMembershipSnapshot(dir, local.ID, old); err != nil {
				t.Fatal(err)
			}
			if _, err := PrepareMembershipRecovery(dir, recoveryTestID, "join", recoveryTestEndpoint); err != nil {
				t.Fatal(err)
			}
			if _, err := PrepareMembershipRecovery(dir, strings.Repeat("1", 32), "join", recoveryTestEndpoint); !errors.Is(err, ErrMembershipRecoveryPending) {
				t.Fatal("different request replaced pending intent", err)
			}
			if err := SetMembershipRecoveryStarted(dir, recoveryTestID, strings.Repeat("1", 64)); err == nil {
				t.Fatal("intent silently retargeted another endpoint")
			}
			if started {
				if err := SetMembershipRecoveryStarted(dir, recoveryTestID, recoveryTestEndpoint); err != nil {
					t.Fatal(err)
				}
			}
			if err := CancelMembershipRecovery(dir, recoveryTestID); err != nil {
				t.Fatal(err)
			}
			intent, _, err := LoadMembershipRecovery(dir)
			if err != nil || started && (intent == nil || intent.Phase != "paused") || !started && intent != nil {
				t.Fatalf("cancel state=%+v err=%v", intent, err)
			}
			generation, err := AuthorizationGeneration(dir, peer.ID)
			if started && !errors.Is(err, ErrMembershipRecoveryPending) || !started && (err != nil || generation <= 1) {
				t.Fatal("cancel rolled back the durable fence or guessed a remote outcome", generation, err)
			}
		})
	}
}
