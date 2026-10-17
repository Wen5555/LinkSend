package identity

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMembershipTransitionFencesTasksAndPreservesIndependentLAN(t *testing.T) {
	for _, mode := range []string{"other_group_without_peer", "other_group_with_peer", "new_local_incarnation", "ordinary_refresh"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			local, peer, unrelated := policyTestPeer(t, "local"), policyTestPeer(t, "peer"), policyTestPeer(t, "independent LAN")
			localInc, peerInc := strings.Repeat("a", 32), strings.Repeat("b", 32)
			makeGrant := func(p TrustedPeer, group, incarnation string, revision uint64) TrustedPeer {
				p.LastLANAddress, p.LANAddressHistory, p.LANRequestID = "", nil, ""
				p.GroupID, p.LocalIncarnation, p.MembershipRevision = group, incarnation, revision
				p.PeerIncarnation = peerInc
				if p.ID == local.ID {
					p.PeerIncarnation = incarnation
				}
				return p
			}
			if err := ApplyMembershipSnapshot(dir, local.ID, []TrustedPeer{makeGrant(local, "old", localInc, 1), makeGrant(peer, "old", localInc, 1)}); err != nil {
				t.Fatal(err)
			}
			generation, err := AuthorizationGeneration(dir, peer.ID)
			if err != nil {
				t.Fatal(err)
			}
			peer.LastLANAddress, peer.LANAddressHistory, peer.LANRequestID = "192.168.10.5", []string{"192.168.10.4", "192.168.10.5"}, strings.Repeat("c", 32)
			if err = CommitLANPeer(dir, peer, generation); err != nil {
				t.Fatal(err)
			}
			if err = CommitLANPeer(dir, unrelated, 1); err != nil {
				t.Fatal(err)
			}
			f, err := loadTrustFile(dir)
			if err != nil {
				t.Fatal(err)
			}
			for index := range f.Peers {
				f.Peers[index].AutoAccept = true
				f.Peers[index].GrantedAt = time.Now().Add(-time.Hour).UTC()
			}
			if err = saveTrust(dir, f); err != nil {
				t.Fatal(err)
			}
			oldTaskStarted := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
			group, incarnation := "new", strings.Repeat("d", 32)
			if mode == "new_local_incarnation" || mode == "ordinary_refresh" {
				group = "old"
			}
			if mode == "ordinary_refresh" {
				incarnation = localInc
			}
			next := []TrustedPeer{makeGrant(local, group, incarnation, 2)}
			if mode != "other_group_without_peer" {
				next = append(next, makeGrant(peer, group, incarnation, 2))
			}
			if err = ApplyMembershipSnapshot(dir, local.ID, next); err != nil {
				t.Fatal(err)
			}
			peers, err := LoadTrust(dir) // Reload the persisted policy, not an in-memory result.
			if err != nil || len(peers) != 2 {
				t.Fatalf("peers=%+v err=%v", peers, err)
			}
			for _, actual := range peers {
				if actual.ID == unrelated.ID {
					if actual.GrantKind != "lan" || actual.GrantGeneration != 1 || !actual.AutoAccept {
						t.Fatalf("unrelated LAN grant changed: %+v", actual)
					}
					continue
				}
				if actual.LastLANAddress != peer.LastLANAddress || !reflect.DeepEqual(actual.LANAddressHistory, peer.LANAddressHistory) || actual.LANRequestID != peer.LANRequestID {
					t.Fatalf("valid independent LAN metadata lost: %+v", actual)
				}
				if mode == "ordinary_refresh" {
					if actual.GrantGeneration != generation || !actual.AutoAccept {
						t.Fatalf("refresh replaced a current grant: %+v", actual)
					}
				} else if actual.GrantGeneration <= generation || actual.AutoAccept || CheckTaskAuthorization(dir, peer.ID, oldTaskStarted) == nil {
					t.Fatalf("old task/consent survived membership transition: %+v", actual)
				}
				if mode == "other_group_without_peer" && (actual.GrantKind != "lan" || actual.GroupID != "" || actual.LocalIncarnation != "" || actual.PeerIncarnation != "" || actual.MembershipRevision != 0) {
					t.Fatalf("retained LAN grant kept old membership: %+v", actual)
				}
			}
			if mode != "ordinary_refresh" {
				late := []TrustedPeer{makeGrant(local, "old", localInc, 9999), makeGrant(peer, "old", localInc, 9999)}
				if err = ApplyMembershipSnapshot(dir, local.ID, late); err == nil {
					t.Fatal("superseded ordinary membership context revived through a higher old snapshot")
				}
			}
		})
	}
}

func TestMembershipTransitionGenerationExhaustionIsAtomic(t *testing.T) {
	for _, includesPeer := range []bool{false, true} {
		t.Run(map[bool]string{false: "retained_LAN", true: "new_membership"}[includesPeer], func(t *testing.T) {
			dir := t.TempDir()
			local, peer := policyTestPeer(t, "local"), policyTestPeer(t, "peer")
			peer.GroupID, peer.LocalIncarnation, peer.PeerIncarnation, peer.MembershipRevision = "old", strings.Repeat("a", 32), strings.Repeat("b", 32), 1
			if err := TrustMembershipPeer(dir, peer); err != nil {
				t.Fatal(err)
			}
			if !includesPeer {
				if err := CommitLANPeer(dir, peer, 1); err != nil {
					t.Fatal(err)
				}
			}
			f, err := loadTrustFile(dir)
			if err != nil {
				t.Fatal(err)
			}
			f.Peers[0].GrantGeneration = ^uint64(0)
			if err = saveTrust(dir, f); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "trust.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			local.GroupID, local.LocalIncarnation, local.PeerIncarnation, local.MembershipRevision = "new", strings.Repeat("c", 32), strings.Repeat("c", 32), 1
			next := []TrustedPeer{local}
			if includesPeer {
				// Same group, new local incarnation reaches applyMembershipPeer
				// without the separate cross-group removal pass.
				local.GroupID = "old"
				next[0] = local
				peer.LocalIncarnation = local.LocalIncarnation
				next = append(next, peer)
			}
			if err = ApplyMembershipSnapshot(dir, local.ID, next); err == nil {
				t.Fatal("exhausted authorization generation wrapped around")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected snapshot changed persisted trust policy", err)
			}
		})
	}
}
