package identity

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func policyTestPeer(t *testing.T, name string) TrustedPeer {
	t.Helper()
	i, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	return TrustedPeer{ID: i.ID(), Name: name, PublicKey: i.PublicKey()}
}

func TestProvisionalLANSurvivesReloadAndRejectsNewGeneration(t *testing.T) {
	dir := t.TempDir()
	peer := policyTestPeer(t, "peer")
	grant := ProvisionalLANGrant{RequestID: strings.Repeat("a", 32), Nonce: strings.Repeat("b", 32), Peer: peer, Generation: 1, State: "accepted", ExpiresAt: time.Now().Add(time.Minute).UTC()}
	if err := BeginProvisionalLAN(dir, grant); err != nil {
		t.Fatal(err)
	}
	if status, err := LANPairStatus(dir, peer.ID, grant.RequestID, grant.Nonce); err != nil || status != "ready" {
		t.Fatalf("persisted provisional status=%q err=%v", status, err)
	}
	if resumed, ok, err := PendingProvisionalLAN(dir, peer.ID); err != nil || !ok || resumed.RequestID != grant.RequestID || resumed.Nonce != grant.Nonce {
		t.Fatalf("restart recovery lost provisional: %+v ok=%v err=%v", resumed, ok, err)
	}
	if err := RevokePeer(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	if err := AllowPeer(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	if err := CommitProvisionalLAN(dir, grant.RequestID, grant.Nonce); err == nil {
		t.Fatal("old provisional grant committed after authorization generation changed")
	}
}

func TestProvisionalLANAllowsSignedFrameClockSkew(t *testing.T) {
	peer := policyTestPeer(t, "clock-skew")
	grant := ProvisionalLANGrant{RequestID: strings.Repeat("c", 32), Nonce: strings.Repeat("d", 32), Peer: peer, Generation: 1, State: "accepted", ExpiresAt: time.Now().Add(74 * time.Second).UTC()}
	if err := BeginProvisionalLAN(t.TempDir(), grant); err != nil {
		t.Fatalf("valid signed-frame skew was rejected: %v", err)
	}
	grant.RequestID = strings.Repeat("e", 32)
	grant.ExpiresAt = time.Now().Add(76 * time.Second).UTC()
	if err := BeginProvisionalLAN(t.TempDir(), grant); err == nil {
		t.Fatal("provisional exceeded signed TTL plus clock-skew bound")
	}
}

func writeLegacyPolicy(t *testing.T, dir string, peers ...TrustedPeer) []byte {
	t.Helper()
	data, err := json.Marshal(TrustFile{Peers: peers})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "trust.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLegacyTrustPolicyMigratesWithoutLosingPreferences(t *testing.T) {
	dir := t.TempDir()
	a := policyTestPeer(t, "legacy A")
	b := policyTestPeer(t, "legacy B")
	a.AutoAccept = true
	a.LastLANAddress = "192.168.10.5"
	b.AutoAccept = true
	original := writeLegacyPolicy(t, dir, a, b)
	peers, err := LoadTrust(dir)
	if err != nil || len(peers) != 2 || !peers[0].AutoAccept {
		t.Fatalf("legacy read: peers=%+v err=%v", peers, err)
	}
	if _, err = os.Stat(filepath.Join(dir, trustMigrationBackup)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only load unexpectedly migrated policy: %v", err)
	}
	if err = SetAutoAccept(dir, b.ID, false); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(filepath.Join(dir, trustMigrationBackup))
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatalf("migration did not preserve exact legacy backup: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "trust.json"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := parseTrustFile(data)
	if err != nil || f.SchemaVersion != trustSchemaVersion || len(f.Peers) != 2 || !f.Peers[0].AutoAccept || f.Peers[0].LastLANAddress != a.LastLANAddress || f.Peers[1].AutoAccept {
		t.Fatalf("migrated policy lost unrelated preferences: %+v %v", f, err)
	}
}

func TestRevocationSurvivesReloadAndCannotBeUndoneByEnrollment(t *testing.T) {
	dir := t.TempDir()
	peer := policyTestPeer(t, "workstation")
	peer.AutoAccept = true // Enrollment must never smuggle in local consent.
	if err := TrustPairedPeer(dir, peer); err != nil {
		t.Fatal(err)
	}
	peers, err := LoadTrust(dir)
	if err != nil || len(peers) != 1 || peers[0].AutoAccept {
		t.Fatalf("enrollment granted consent: %+v %v", peers, err)
	}
	if err = SetAutoAccept(dir, peer.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = RevokePeer(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	peers, err = LoadTrust(dir)
	if err != nil || len(peers) != 0 {
		t.Fatalf("revoked pin/consent still visible: %+v %v", peers, err)
	}
	denied, err := LoadDeniedPeers(dir)
	if err != nil || len(denied) != 1 || denied[0].ID != peer.ID || denied[0].Name != peer.Name || denied[0].DeniedAt.IsZero() {
		t.Fatalf("denial not persistent: %+v %v", denied, err)
	}
	for name, action := range map[string]func() error{
		"authorization": func() error { return CheckPeerAllowed(dir, peer.ID) },
		"automatic pin": func() error { return TrustPairedPeer(dir, peer) },
		"manual pin":    func() error { return TrustPeer(dir, peer, peer.ID) },
		"auto accept":   func() error { return SetAutoAccept(dir, peer.ID, true) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := action(); !errors.Is(err, ErrPeerDenied) || !errors.Is(err, ErrAuthentication) {
				t.Fatalf("denial was bypassed or misclassified: %v", err)
			}
		})
	}
	before, _ := os.ReadFile(filepath.Join(dir, "trust.json"))
	if err = RevokePeer(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "trust.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("repeated revocation changed its durable decision")
	}
	if err = AllowPeer(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	if err = CheckPeerAllowed(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	peers, err = LoadTrust(dir)
	if err != nil || len(peers) != 0 {
		t.Fatalf("unblocking restored a stale pin: %+v %v", peers, err)
	}
	if err = SetAutoAccept(dir, peer.ID, true); err == nil {
		t.Fatal("unblocked device inherited consent without new trust")
	}
	if err = TrustPairedPeer(dir, peer); err != nil {
		t.Fatal(err)
	}
	peers, err = LoadTrust(dir)
	if err != nil || len(peers) != 1 || peers[0].AutoAccept {
		t.Fatalf("new trust inherited previous auto accept: %+v %v", peers, err)
	}
}

func TestSameNameDifferentKeyDoesNotInheritPolicy(t *testing.T) {
	dir := t.TempDir()
	trusted := policyTestPeer(t, "My Mac")
	other := policyTestPeer(t, "My Mac")
	if err := TrustPairedPeer(dir, trusted); err != nil {
		t.Fatal(err)
	}
	if err := SetAutoAccept(dir, trusted.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := TrustPairedPeer(dir, other); err != nil {
		t.Fatal(err)
	}
	peers, err := LoadTrust(dir)
	if err != nil || len(peers) != 2 || peers[1].AutoAccept {
		t.Fatalf("same name inherited authorization: %+v %v", peers, err)
	}
	if err = RevokePeer(dir, trusted.ID); err != nil {
		t.Fatal(err)
	}
	if err = CheckPeerAllowed(dir, other.ID); err != nil {
		t.Fatalf("denial crossed identity boundary: %v", err)
	}
	other.ID = trusted.ID
	if err = TrustPairedPeer(dir, other); err == nil {
		t.Fatal("different key accepted with revoked identity")
	}
}

func TestRevocationOfUnpinnedDiscoveredPeerPersists(t *testing.T) {
	dir := t.TempDir()
	peer := policyTestPeer(t, "nearby")
	if err := RevokePeer(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	if err := TrustPairedPeer(dir, peer); !errors.Is(err, ErrPeerDenied) {
		t.Fatalf("first enrollment bypassed pre-existing denial: %v", err)
	}
}

func TestTrustMigrationWriteFailurePreservesPreviousPolicy(t *testing.T) {
	dir := t.TempDir()
	peer := policyTestPeer(t, "existing")
	peer.AutoAccept = true
	original := writeLegacyPolicy(t, dir, peer)
	// A directory at the exact backup target produces a deterministic write
	// failure on Windows and Unix, including when the test runs as root.
	if err := os.Mkdir(filepath.Join(dir, trustMigrationBackup), 0700); err != nil {
		t.Fatal(err)
	}
	if err := RevokePeer(dir, peer.ID); err == nil {
		t.Fatal("revocation reported success after backup write failure")
	}
	after, err := os.ReadFile(filepath.Join(dir, "trust.json"))
	if err != nil || !bytes.Equal(original, after) {
		t.Fatalf("failed migration damaged user policy: %v", err)
	}
	peers, err := LoadTrust(dir)
	if err != nil || len(peers) != 1 || !peers[0].AutoAccept {
		t.Fatalf("failed migration changed effective policy: %+v %v", peers, err)
	}
}

func TestRevocationCannotBeRolledBackByAutomaticBackupRecovery(t *testing.T) {
	for _, damage := range []string{"corrupt", "missing", "future schema"} {
		t.Run(damage, func(t *testing.T) {
			dir := t.TempDir()
			peer := policyTestPeer(t, "revoked")
			legacy := writeLegacyPolicy(t, dir, peer)
			if err := RevokePeer(dir, peer.ID); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "trust.json")
			if err := os.WriteFile(path+".previous", legacy, 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch damage {
			case "corrupt":
				err = os.WriteFile(path, []byte("truncated"), 0600)
			case "missing":
				err = os.Remove(path)
			case "future schema":
				err = os.WriteFile(path, []byte(`{"schema_version":3,"peers":[]}`), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = CheckPeerAllowed(dir, peer.ID); err == nil {
				t.Fatal("damaged policy reopened authorization")
			}
			if err = TrustPairedPeer(dir, peer); err == nil {
				t.Fatal("damaged policy allowed automatic re-enrollment")
			}
			peers, err := LoadTrust(dir)
			if err == nil || len(peers) != 0 {
				t.Fatalf("backup silently resurrected old pin: %+v %v", peers, err)
			}
		})
	}
}

func TestConcurrentRevocationAndEnrollmentDoesNotLoseDenial(t *testing.T) {
	dir := t.TempDir()
	revoked := policyTestPeer(t, "revoked")
	if err := TrustPairedPeer(dir, revoked); err != nil {
		t.Fatal(err)
	}
	const count = 8
	others := make([]TrustedPeer, count)
	for i := range others {
		others[i] = policyTestPeer(t, "other")
	}
	start := make(chan struct{})
	errorsFound := make(chan error, count+1)
	var wg sync.WaitGroup
	wg.Add(count + 1)
	go func() {
		defer wg.Done()
		<-start
		errorsFound <- RevokePeer(dir, revoked.ID)
	}()
	for _, peer := range others {
		go func() {
			defer wg.Done()
			<-start
			errorsFound <- TrustPairedPeer(dir, peer)
		}()
	}
	close(start)
	wg.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckPeerAllowed(dir, revoked.ID); !errors.Is(err, ErrPeerDenied) {
		t.Fatalf("concurrent enrollment lost denial: %v", err)
	}
	peers, err := LoadTrust(dir)
	if err != nil || len(peers) != count {
		t.Fatalf("concurrent write lost unrelated pins: got=%d err=%v", len(peers), err)
	}
}

func TestMembershipRejoinAdvancesGrantWithoutRestoringConsent(t *testing.T) {
	dir := t.TempDir()
	peer := policyTestPeer(t, "peer")
	peer.GroupID = "group"
	peer.PeerIncarnation = strings.Repeat("a", 32)
	peer.LocalIncarnation = strings.Repeat("b", 32)
	peer.MembershipRevision = 2
	if err := TrustMembershipPeer(dir, peer); err != nil {
		t.Fatal(err)
	}
	oldTaskStarted := time.Now().UTC().Format(time.RFC3339Nano)
	if err := SetAutoAccept(dir, peer.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := RevokeMembershipPeer(dir, peer.ID, peer.PeerIncarnation, "request-1", 2); err != nil {
		t.Fatal(err)
	}
	if err := TrustMembershipPeer(dir, peer); !errors.Is(err, ErrPeerDenied) {
		t.Fatalf("old membership snapshot reopened authorization: %v", err)
	}
	peer.PeerIncarnation = strings.Repeat("c", 32)
	peer.MembershipRevision = 3
	if err := TrustMembershipPeer(dir, peer); err != nil {
		t.Fatal("verified new incarnation did not establish a fresh grant:", err)
	}
	peers, err := LoadTrust(dir)
	if err != nil || len(peers) != 1 || peers[0].AutoAccept || peers[0].GrantGeneration < 2 {
		t.Fatalf("fresh relationship restored old consent or generation: %+v %v", peers, err)
	}
	if err = CheckTaskAuthorization(dir, peer.ID, oldTaskStarted); err == nil {
		t.Fatal("task created under revoked grant was reusable")
	}
}

func TestLocalBlockCannotBeClearedByMembershipSnapshot(t *testing.T) {
	dir := t.TempDir()
	peer := policyTestPeer(t, "peer")
	if err := RevokePeer(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	peer.GroupID = "group"
	peer.PeerIncarnation = strings.Repeat("c", 32)
	peer.LocalIncarnation = strings.Repeat("d", 32)
	peer.MembershipRevision = 99
	if err := TrustMembershipPeer(dir, peer); !errors.Is(err, ErrPeerDenied) {
		t.Fatalf("membership cleared explicit local block: %v", err)
	}
}

func TestSchemaOneDeniedPeerRemainsExplicitLocalBlockDuringSync(t *testing.T) {
	dir := t.TempDir()
	peer := policyTestPeer(t, "peer")
	legacy := TrustFile{SchemaVersion: 1, Peers: []TrustedPeer{}, DeniedPeers: []DeniedPeer{{ID: peer.ID, Name: peer.Name, DeniedAt: time.Now().UTC()}}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "trust.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	peer.GroupID = "group"
	peer.PeerIncarnation = strings.Repeat("e", 32)
	peer.LocalIncarnation = strings.Repeat("f", 32)
	peer.MembershipRevision = 2
	if err = TrustMembershipPeer(dir, peer); !errors.Is(err, ErrPeerDenied) {
		t.Fatalf("schema1 local block was cleared: %v", err)
	}
}

func TestCompleteMembershipSnapshotRevokesMissingPeerAndRejectsRollback(t *testing.T) {
	dir := t.TempDir()
	local := policyTestPeer(t, "local")
	b := policyTestPeer(t, "b")
	c := policyTestPeer(t, "c")
	localInc := strings.Repeat("1", 32)
	makePeer := func(peer TrustedPeer, incarnation string, revision uint64) TrustedPeer {
		peer.GroupID = "group"
		peer.PeerIncarnation = incarnation
		peer.LocalIncarnation = localInc
		peer.MembershipRevision = revision
		return peer
	}
	initial := []TrustedPeer{makePeer(local, localInc, 1), makePeer(b, strings.Repeat("2", 32), 1), makePeer(c, strings.Repeat("3", 32), 1)}
	if err := ApplyMembershipSnapshot(dir, local.ID, initial); err != nil {
		t.Fatal(err)
	}
	removed := []TrustedPeer{makePeer(local, localInc, 2), makePeer(c, strings.Repeat("3", 32), 2)}
	if err := ApplyMembershipSnapshot(dir, local.ID, removed); err != nil {
		t.Fatal(err)
	}
	if _, err := AuthorizationGeneration(dir, b.ID); !errors.Is(err, ErrPeerDenied) {
		t.Fatalf("missing peer remained authorized: %v", err)
	}
	if err := ApplyMembershipSnapshot(dir, local.ID, initial); err == nil {
		t.Fatal("older complete snapshot rolled membership back")
	}
	rejoined := append(removed, makePeer(b, strings.Repeat("4", 32), 3))
	for index := range rejoined {
		rejoined[index].MembershipRevision = 3
	}
	if err := ApplyMembershipSnapshot(dir, local.ID, rejoined); err != nil {
		t.Fatal(err)
	}
	if generation, err := AuthorizationGeneration(dir, b.ID); err != nil || generation < 2 {
		t.Fatalf("verified rejoin did not create new generation: %d %v", generation, err)
	}
}

func TestLateLANCommitCannotAttachToNewMembershipGeneration(t *testing.T) {
	dir := t.TempDir()
	peer := policyTestPeer(t, "peer")
	peer.GroupID = "group"
	peer.PeerIncarnation = strings.Repeat("a", 32)
	peer.LocalIncarnation = strings.Repeat("b", 32)
	peer.MembershipRevision = 1
	if err := TrustMembershipPeer(dir, peer); err != nil {
		t.Fatal(err)
	}
	oldGeneration, _ := LANPairGeneration(dir, peer.ID)
	if err := RevokeMembershipPeer(dir, peer.ID, peer.PeerIncarnation, "remove", 1); err != nil {
		t.Fatal(err)
	}
	peer.PeerIncarnation = strings.Repeat("c", 32)
	peer.MembershipRevision = 2
	if err := TrustMembershipPeer(dir, peer); err != nil {
		t.Fatal(err)
	}
	if err := CommitLANPeer(dir, TrustedPeer{ID: peer.ID, Name: peer.Name, PublicKey: peer.PublicKey}, oldGeneration); err == nil {
		t.Fatal("late LAN commit reused old generation")
	}
	peers, err := LoadTrust(dir)
	if err != nil || len(peers) != 1 || peers[0].GroupID != "group" || peers[0].GrantKind != "group" {
		t.Fatalf("late LAN commit damaged group grant: %+v %v", peers, err)
	}
}
