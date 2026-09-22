package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

// A group revision can advance while a durable revocation is offline. Refresh
// only its concurrency token under the original actor binding. An explicit
// removal may establish a new actor-bound intent through the identity CAS;
// the target stays fixed and there is at most one additional network attempt.
func (s *Service) revokeMembershipWithRefresh(ctx context.Context, c *signaling.Client, pending identity.DeniedPeer, explicit bool) (string, uint64, error) {
	revision, revokeErr := c.RevokeMembership(ctx, pending.ID, pending.TargetIncarnation, pending.SeenRevision, pending.RequestID)
	var remote *signaling.RemoteError
	if protocol.ErrorCode(revokeErr) != protocol.AuthenticationFailed || !errors.As(revokeErr, &remote) || remote.Status != http.StatusForbidden {
		return pending.RequestID, revision, revokeErr
	}
	if !explicit && (pending.ActorGroupID == "" || pending.ActorIncarnation == "") {
		return pending.RequestID, 0, revokeErr
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		return pending.RequestID, 0, revokeErr
	}
	var local, target signaling.Device
	seen := make(map[string]bool, len(devices))
	for _, device := range devices {
		if seen[device.ID] || device.Revoked {
			return pending.RequestID, 0, revokeErr
		}
		seen[device.ID] = true
		if device.ID == s.identity.ID() {
			local = device
		}
		if device.ID == pending.ID {
			target = device
		}
	}
	if local.ID == "" || target.ID == "" || local.GroupID == "" || target.Incarnation != pending.TargetIncarnation || local.MembershipRevision <= pending.SeenRevision {
		return pending.RequestID, 0, revokeErr
	}
	for _, device := range devices {
		if device.GroupID != local.GroupID || device.MembershipRevision != local.MembershipRevision {
			return pending.RequestID, 0, revokeErr
		}
	}
	sameActor := pending.ActorGroupID == local.GroupID && pending.ActorIncarnation == local.Incarnation
	if !sameActor && !explicit {
		return pending.RequestID, 0, revokeErr
	}

	// Pairing or an explicit unblock may have replaced this tombstone while
	// the snapshot was in flight. Never replay a superseded local request.
	s.trustMu.Lock()
	denied, loadErr := identity.LoadDeniedPeers(s.cfg.DataDir)
	current := false
	for _, candidate := range denied {
		if candidate.ID == pending.ID && candidate.RequestID == pending.RequestID && candidate.TargetIncarnation == pending.TargetIncarnation && candidate.ActorGroupID == pending.ActorGroupID && candidate.ActorIncarnation == pending.ActorIncarnation && candidate.SeenRevision == pending.SeenRevision && candidate.PendingSync && !candidate.LocalBlock {
			current = true
			break
		}
	}
	if loadErr == nil && current && !sameActor {
		var renewed identity.DeniedPeer
		renewed, loadErr = identity.RenewMembershipRevokeIntent(s.cfg.DataDir, pending, protocol.RandomID(), local.GroupID, local.Incarnation, local.MembershipRevision)
		if loadErr == nil {
			pending = renewed
		}
	}
	s.trustMu.Unlock()
	if loadErr != nil || !current {
		return pending.RequestID, 0, errors.Join(revokeErr, loadErr)
	}
	revision, err = c.RevokeMembership(ctx, pending.ID, pending.TargetIncarnation, local.MembershipRevision, pending.RequestID)
	return pending.RequestID, revision, err
}
