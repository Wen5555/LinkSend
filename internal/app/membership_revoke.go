package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

// Prove the live actor before the first DELETE, including after ordinary code
// re-pairing. A new explicit removal may establish a new actor-bound intent
// through the identity CAS. The target stays fixed; at most two DELETEs use
// one authenticated snapshot, with no automatic retargeting or unbounded loop.
func (s *Service) revokeMembershipWithRefresh(ctx context.Context, c *signaling.Client, pending identity.DeniedPeer, explicit bool) (string, uint64, error) {
	s.membershipMu.Lock()
	defer s.membershipMu.Unlock()
	return s.revokeMembershipWithRefreshLocked(ctx, c, pending, explicit)
}

func (s *Service) revokeMembershipWithRefreshLocked(ctx context.Context, c *signaling.Client, pending identity.DeniedPeer, explicit bool) (string, uint64, error) {
	if err := identity.CheckMembershipRevocationCurrent(s.cfg.DataDir, pending); err != nil {
		return pending.RequestID, 0, protocol.Wrap(protocol.AuthenticationFailed, "revocation context is no longer current", err)
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		return pending.RequestID, 0, err
	}
	invalid := protocol.Fail(protocol.AuthenticationFailed, "current revocation actor or original target is not proven")
	var local, target signaling.Device
	seen := make(map[string]bool, len(devices))
	for _, device := range devices {
		if seen[device.ID] || device.Revoked {
			return pending.RequestID, 0, invalid
		}
		seen[device.ID] = true
		if device.ID == s.identity.ID() {
			local = device
		}
		if device.ID == pending.ID {
			target = device
		}
	}
	if local.ID == "" || local.GroupID == "" || target.ID != "" && target.Incarnation != pending.TargetIncarnation {
		return pending.RequestID, 0, invalid
	}
	for _, device := range devices {
		if device.GroupID != local.GroupID || device.MembershipRevision != local.MembershipRevision {
			return pending.RequestID, 0, invalid
		}
	}
	sameActor := pending.ActorGroupID == local.GroupID && pending.ActorIncarnation == local.Incarnation
	if !sameActor && (!explicit || target.ID == "" || local.MembershipRevision <= pending.SeenRevision) {
		return pending.RequestID, 0, invalid
	}

	// Pairing or an explicit unblock may have replaced this tombstone while
	// the snapshot was in flight. Never replay a superseded local request.
	if !sameActor {
		var renewed identity.DeniedPeer
		s.trustMu.Lock()
		renewed, err = identity.RenewMembershipRevokeIntent(s.cfg.DataDir, pending, protocol.RandomID(), local.GroupID, local.Incarnation, local.MembershipRevision)
		s.trustMu.Unlock()
		if err != nil {
			return pending.RequestID, 0, err
		}
		pending = renewed
	}
	if err = identity.CheckMembershipRevocationCurrent(s.cfg.DataDir, pending); err != nil {
		return pending.RequestID, 0, protocol.Wrap(protocol.AuthenticationFailed, "revocation context changed before dispatch", err)
	}
	revision, err := c.RevokeMembership(ctx, pending.ID, pending.TargetIncarnation, pending.SeenRevision, pending.RequestID)
	var remote *signaling.RemoteError
	if !sameActor || target.ID == "" || local.MembershipRevision <= pending.SeenRevision || protocol.ErrorCode(err) != protocol.AuthenticationFailed || !errors.As(err, &remote) || remote.Status != http.StatusForbidden {
		return pending.RequestID, revision, err
	}
	if err = identity.CheckMembershipRevocationCurrent(s.cfg.DataDir, pending); err != nil {
		return pending.RequestID, 0, protocol.Wrap(protocol.AuthenticationFailed, "revocation context changed before retry", err)
	}
	revision, err = c.RevokeMembership(ctx, pending.ID, pending.TargetIncarnation, local.MembershipRevision, pending.RequestID)
	return pending.RequestID, revision, err
}
