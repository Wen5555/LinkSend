package identity

import (
	"errors"
	"strings"
	"testing"
)

func TestForgetAllowsFreshLANConsentButDoesNotRestoreOldMembership(t *testing.T) {
	dir := t.TempDir()
	local, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	remote, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	peer := TrustedPeer{ID: remote.ID(), Name: "peer", PublicKey: remote.PublicKey(), GroupID: "group", PeerIncarnation: strings.Repeat("1", 32), LocalIncarnation: strings.Repeat("2", 32), MembershipRevision: 1}
	self := TrustedPeer{ID: local.ID(), Name: "self", PublicKey: local.PublicKey(), GroupID: "group", PeerIncarnation: peer.LocalIncarnation, LocalIncarnation: peer.LocalIncarnation, MembershipRevision: 1}
	if err = ApplyMembershipSnapshot(dir, local.ID(), []TrustedPeer{self, peer}); err != nil {
		t.Fatal(err)
	}
	before, err := AuthorizationGeneration(dir, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = ForgetPeer(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	if err = CheckPeerAllowed(dir, peer.ID); err != nil {
		t.Fatal("deletion became a blacklist", err)
	}
	if _, err = AuthorizationGeneration(dir, peer.ID); err == nil {
		t.Fatal("deleted peer retained authorization")
	}
	if err = ApplyMembershipSnapshot(dir, local.ID(), []TrustedPeer{self, peer}); err != nil {
		t.Fatal(err)
	}
	peers, _ := LoadTrust(dir)
	for _, p := range peers {
		if p.ID == peer.ID {
			t.Fatal("refresh restored deleted pairing")
		}
	}
	generation, err := LANPairGeneration(dir, peer.ID)
	if err != nil || generation <= before {
		t.Fatal("old authorization epoch reused", generation, err)
	}
	peer.LastLANAddress = "192.168.1.20"
	if err = CommitLANPeer(dir, peer, generation); err != nil {
		t.Fatal("new LAN consent rejected", err)
	}
	if err = RevokePeer(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	if err = ForgetPeer(dir, peer.ID); err != nil {
		t.Fatal(err)
	}
	if err = RestoreForgottenPeers(dir, []string{peer.ID}); err != nil {
		t.Fatal(err)
	}
	if err = CheckPeerAllowed(dir, peer.ID); !errors.Is(err, ErrPeerDenied) {
		t.Fatal("deletion or enrollment undid blacklist", err)
	}
}

func TestForgottenPeerCanBeBlacklistedAndExplicitlyPairedAgain(t *testing.T) {
	dir := t.TempDir()
	peer, _ := Generate()
	if err := TrustPairedPeer(dir, TrustedPeer{ID: peer.ID(), PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	if err := ForgetPeer(dir, peer.ID()); err != nil {
		t.Fatal(err)
	}
	if err := RevokePeer(dir, peer.ID()); err != nil {
		t.Fatal(err)
	}
	denied, err := LoadDeniedPeers(dir)
	if err != nil || len(denied) != 1 || !denied[0].LocalBlock {
		t.Fatal("forget-to-blacklist failed", denied, err)
	}
	if err = AllowPeer(dir, peer.ID()); err != nil {
		t.Fatal(err)
	}
	if err = TrustPairedPeer(dir, TrustedPeer{ID: peer.ID(), PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	if err = ForgetPeer(dir, peer.ID()); err != nil {
		t.Fatal(err)
	}
	if err = RestoreForgottenPeers(dir, []string{peer.ID()}); err != nil {
		t.Fatal(err)
	}
	denied, err = LoadDeniedPeers(dir)
	if err != nil || len(denied) != 0 {
		t.Fatal(denied, err)
	}
}
