package identity

import "time"

// ForgetPeer removes a local pairing, not the remote group membership. The
// private tombstone stops a background snapshot restoring the same grant, while
// allowing a new explicit LAN consent or pairing-code enrollment.
func ForgetPeer(dir, peerID string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	if !validPeerID(peerID) {
		return authenticationError("invalid device identity")
	}
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	for _, prior := range f.DeniedPeers {
		if prior.ID == peerID {
			return nil
		} // deletion never unblocks a blacklist
	}
	for index, peer := range f.Peers {
		if peer.ID != peerID {
			continue
		}
		if peer.GrantGeneration == ^uint64(0) {
			return authenticationError("peer grant generation exhausted")
		}
		d := DeniedPeer{ID: peerID, Name: peer.Name, DeniedAt: time.Now().UTC(), Forgotten: true,
			AuthorizationGeneration: max(uint64(1), peer.GrantGeneration+1), TargetIncarnation: peer.PeerIncarnation,
			ActorGroupID: peer.GroupID, ActorIncarnation: peer.LocalIncarnation, SeenRevision: peer.MembershipRevision}
		f.Peers = append(f.Peers[:index], f.Peers[index+1:]...)
		for i := len(f.ProvisionalLAN) - 1; i >= 0; i-- {
			if f.ProvisionalLAN[i].Peer.ID == peerID {
				f.ProvisionalLAN = append(f.ProvisionalLAN[:i], f.ProvisionalLAN[i+1:]...)
			}
		}
		retainAuthorizationEpoch(&f, peerID, d.AuthorizationGeneration)
		f.DeniedPeers = append(f.DeniedPeers, d)
		return saveTrust(dir, f)
	}
	return nil
}

// Only a successful explicit pairing-code join may clear forgotten peers in
// that authenticated result. Local blacklists and pending revocations survive.
func RestoreForgottenPeers(dir string, peerIDs []string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	selected := make(map[string]bool, len(peerIDs))
	for _, id := range peerIDs {
		selected[id] = true
	}
	changed := false
	for i := len(f.DeniedPeers) - 1; i >= 0; i-- {
		d := f.DeniedPeers[i]
		if selected[d.ID] && d.Forgotten && !d.LocalBlock && !d.PendingSync {
			retainAuthorizationEpoch(&f, d.ID, d.AuthorizationGeneration)
			f.DeniedPeers = append(f.DeniedPeers[:i], f.DeniedPeers[i+1:]...)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return saveTrust(dir, f)
}
