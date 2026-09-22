package identity

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMembershipRevocationActorBindingAndExplicitIntentCAS(t *testing.T) {
	dir := t.TempDir()
	peer := policyTestPeer(t, "peer")
	peer.GroupID, peer.LocalIncarnation, peer.PeerIncarnation, peer.MembershipRevision = "group", strings.Repeat("a", 32), strings.Repeat("b", 32), 1
	if err := TrustMembershipPeer(dir, peer); err != nil {
		t.Fatal(err)
	}
	if err := RevokeMembershipPeer(dir, peer.ID, peer.PeerIncarnation, "old-request", 1); err != nil {
		t.Fatal(err)
	}
	denied, err := LoadDeniedPeers(dir)
	if err != nil || len(denied) != 1 || denied[0].ActorGroupID != peer.GroupID || denied[0].ActorIncarnation != peer.LocalIncarnation {
		t.Fatalf("revocation did not retain the original actor: %+v %v", denied, err)
	}
	original := denied[0]
	changed := original
	changed.ActorIncarnation = strings.Repeat("c", 32)
	if _, err = RenewMembershipRevokeIntent(dir, changed, "new-request", peer.GroupID, changed.ActorIncarnation, 2); !errors.Is(err, ErrPeerDenied) {
		t.Fatal("changed actor binding passed the pending intent CAS", err)
	}
	next, err := RenewMembershipRevokeIntent(dir, original, "new-request", peer.GroupID, changed.ActorIncarnation, 2)
	if err != nil || next.RequestID == original.RequestID || next.TargetIncarnation != original.TargetIncarnation || next.AuthorizationGeneration != original.AuthorizationGeneration || next.ActorIncarnation != changed.ActorIncarnation {
		t.Fatalf("explicit intent changed the wrong authorization fields: %+v %v", next, err)
	}
	if err = MarkMembershipRevokeSynced(dir, peer.ID, original.RequestID, 3); !errors.Is(err, ErrPeerDenied) {
		t.Fatal("late completion replaced the new pending intent", err)
	}
	if _, err = RenewMembershipRevokeIntent(dir, original, "another-request", peer.GroupID, changed.ActorIncarnation, 3); !errors.Is(err, ErrPeerDenied) {
		t.Fatal("stale explicit renewal replaced the new intent", err)
	}
	if err = MarkMembershipRevokeSynced(dir, peer.ID, next.RequestID, 3); err != nil {
		t.Fatal(err)
	}
	if _, err = RenewMembershipRevokeIntent(dir, next, "another-request", peer.GroupID, changed.ActorIncarnation, 4); !errors.Is(err, ErrPeerDenied) {
		t.Fatal("completed intent was silently reopened", err)
	}
}

func TestRevocationActorBindingLegacyAndMalformedRecords(t *testing.T) {
	peer := policyTestPeer(t, "peer")
	for _, tc := range []struct {
		name, group, actor string
		valid              bool
	}{
		{name: "legacy_without_binding", valid: true},
		{name: "bound", group: "group", actor: strings.Repeat("a", 32), valid: true},
		{name: "group_only", group: "group"},
		{name: "actor_only", actor: strings.Repeat("a", 32)},
		{name: "malformed_actor", group: "group", actor: "short"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := TrustFile{SchemaVersion: 2, Peers: []TrustedPeer{}, DeniedPeers: []DeniedPeer{{ID: peer.ID, DeniedAt: time.Now().UTC(), AuthorizationGeneration: 2, ActorGroupID: tc.group, ActorIncarnation: tc.actor}}}
			data, err := json.Marshal(f)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = parseTrustFile(data); (err == nil) != tc.valid {
				t.Fatalf("actor binding validity=%v: %v", tc.valid, err)
			}
		})
	}
}
