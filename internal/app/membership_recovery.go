package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

type MembershipRecoveryStatus struct {
	State   string `json:"state"`
	ID      string `json:"id,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Message string `json:"message,omitempty"`
}

func (s *Service) membershipEndpointFingerprint() string {
	digest := sha256.Sum256([]byte(strings.TrimRight(strings.TrimSpace(s.cfg.ServerURL), "/")))
	return hex.EncodeToString(digest[:])
}

func (s *Service) requireNoMembershipRecovery() error {
	intent, _, err := identity.LoadMembershipRecovery(s.cfg.DataDir)
	if err == nil && intent != nil {
		err = identity.ErrMembershipRecoveryPending
	}
	if err != nil {
		return protocol.Wrap(protocol.AuthenticationFailed, "explicit membership recovery must be resolved first", err)
	}
	return nil
}

// Pin misses may refresh group membership; established LAN/session peers stay
// on their local path. Serialize the complete read/apply operation with recovery
// so an unknown legacy context cannot arrive after the retirement transaction.
func (s *Service) refreshMembershipDevices(ctx context.Context) ([]signaling.Device, error) {
	s.membershipMu.Lock()
	defer s.membershipMu.Unlock()
	if err := s.requireNoMembershipRecovery(); err != nil {
		return nil, err
	}
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	devices, err := c.Devices(ctx)
	if err == nil {
		err = s.syncPairedDevices(devices)
	}
	return devices, err
}

// An existing membership profile cannot silently enroll into an empty service
// through the ordinary init button. A failed read remains a failure; it
// neither declares a reset nor creates a recovery intent.
func (s *Service) checkMembershipRegistrationContext(ctx context.Context, c *signaling.Client) error {
	known, err := identity.HasMembershipHistory(s.cfg.DataDir)
	if err != nil || !known {
		return err
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		var remote *signaling.RemoteError
		if errors.As(err, &remote) && remote.Status == http.StatusUnauthorized && protocol.ErrorCode(err) == protocol.AuthenticationFailed {
			return protocol.Wrap(protocol.AuthenticationFailed, "existing profile requires explicit membership recovery before re-enrollment", err)
		}
		return err
	}
	for _, device := range devices {
		if device.ID == s.identity.ID() && bytes.Equal(device.PublicKey, s.identity.PublicKey()) && !device.Revoked {
			return nil
		}
	}
	return protocol.Fail(protocol.AuthenticationFailed, "current membership is not proven; use explicit recovery")
}

func (s *Service) MembershipRecoveryStatus() (MembershipRecoveryStatus, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return MembershipRecoveryStatus{}, err
	}
	defer done()
	intent, last, err := identity.LoadMembershipRecovery(s.cfg.DataDir)
	if err != nil {
		return MembershipRecoveryStatus{}, err
	}
	if intent != nil {
		message := "旧设备组任务已隔离，独立局域网关系仍可使用；可继续恢复或取消准备。"
		if intent.Phase != "prepared" {
			message = "服务端操作可能已发生；继续时会先核对实际结果。暂停不会恢复旧授权。"
		}
		if intent.EndpointFingerprint != s.membershipEndpointFingerprint() {
			message = "恢复请求绑定了原服务地址，请恢复该地址后继续核对。"
		}
		return MembershipRecoveryStatus{State: intent.Phase, ID: intent.ID, Mode: intent.Mode, Message: message}, nil
	}
	if last != nil {
		return MembershipRecoveryStatus{State: "completed", ID: last.ID, Message: "已重新连接设备组；旧任务不会自动发送。"}, nil
	}
	return MembershipRecoveryStatus{State: "idle"}, nil
}

// Preparing is an explicit user action, never a response to an inferred reset.
func (s *Service) PrepareMembershipRecovery(mode string, confirmed bool) (MembershipRecoveryStatus, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return MembershipRecoveryStatus{}, err
	}
	defer done()
	if !confirmed || mode != "initialize" && mode != "join" {
		return MembershipRecoveryStatus{}, errors.New("INVALID_ARGUMENT: explicit recovery confirmation and mode required")
	}
	s.membershipMu.Lock()
	defer s.membershipMu.Unlock()
	if _, err = s.client(); err != nil {
		return MembershipRecoveryStatus{}, err
	}
	var intent identity.MembershipRecovery
	err = s.mutateMembershipTrust(func() error {
		var prepareErr error
		intent, prepareErr = identity.PrepareMembershipRecovery(s.cfg.DataDir, protocol.RandomID(), mode, s.membershipEndpointFingerprint())
		return prepareErr
	})
	s.notifyChange()
	if err != nil {
		return MembershipRecoveryStatus{}, err
	}
	return MembershipRecoveryStatus{State: intent.Phase, ID: intent.ID, Mode: intent.Mode, Message: "旧设备组任务已隔离。选择继续以连接新组，或取消准备。"}, nil
}

func (s *Service) CancelMembershipRecovery(requestID string) (MembershipRecoveryStatus, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return MembershipRecoveryStatus{}, err
	}
	defer done()
	s.membershipMu.Lock()
	defer s.membershipMu.Unlock()
	s.trustMu.Lock()
	err = identity.CancelMembershipRecovery(s.cfg.DataDir, requestID)
	s.trustMu.Unlock()
	s.notifyChange()
	if err != nil {
		return MembershipRecoveryStatus{}, err
	}
	return s.MembershipRecoveryStatus()
}

func (s *Service) ContinueMembershipRecovery(ctx context.Context, requestID, code, name string) (DeviceInfo, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return DeviceInfo{}, err
	}
	defer done()
	s.membershipMu.Lock()
	defer s.membershipMu.Unlock()
	intent, last, err := identity.LoadMembershipRecovery(s.cfg.DataDir)
	if err != nil {
		return DeviceInfo{}, err
	}
	fingerprint := s.membershipEndpointFingerprint()
	idempotentCompletion := intent == nil && last != nil && last.ID == requestID && last.EndpointFingerprint == fingerprint
	if !idempotentCompletion && (intent == nil || intent.ID != requestID || intent.EndpointFingerprint != fingerprint) {
		return DeviceInfo{}, protocol.Fail(protocol.AuthenticationFailed, "recovery intent or service address changed")
	}
	c, err := s.client()
	if err != nil {
		return DeviceInfo{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// Always reconcile first. This also handles a successful earlier request
	// whose response was lost or whose process stopped before local commit.
	devices, err := c.Devices(ctx)
	if err != nil {
		var remote *signaling.RemoteError
		if idempotentCompletion || !errors.As(err, &remote) || remote.Status != http.StatusUnauthorized || protocol.ErrorCode(err) != protocol.AuthenticationFailed {
			return DeviceInfo{}, err
		}
		if intent.Mode == "join" && strings.TrimSpace(code) == "" {
			return DeviceInfo{}, errors.New("INVALID_ARGUMENT: a fresh pairing code is required")
		}
		s.trustMu.Lock()
		err = identity.SetMembershipRecoveryStarted(s.cfg.DataDir, requestID, fingerprint)
		s.trustMu.Unlock()
		if err != nil {
			return DeviceInfo{}, err
		}
		s.notifyChange()
		if intent.Mode == "initialize" {
			_, err = c.Initialize(ctx, s.name(name))
		} else {
			_, err = c.Join(ctx, code, s.name(name))
		}
		if err != nil {
			return DeviceInfo{}, err
		}
		devices, err = c.Devices(ctx)
		if err != nil {
			return DeviceInfo{}, err
		}
	}
	var local signaling.Device
	for _, device := range devices {
		if device.ID == s.identity.ID() && bytes.Equal(device.PublicKey, s.identity.PublicKey()) && !device.Revoked {
			local = device
		}
	}
	if local.ID == "" {
		return DeviceInfo{}, protocol.Fail(protocol.AuthenticationFailed, "recovery snapshot missing this identity")
	}
	if idempotentCompletion {
		if local.GroupID != last.GroupID || local.Incarnation != last.LocalIncarnation {
			return DeviceInfo{}, protocol.Fail(protocol.AuthenticationFailed, "completed recovery belongs to another membership")
		}
		return s.device(local), nil
	}
	snapshot := make([]identity.TrustedPeer, 0, len(devices))
	for _, device := range devices {
		if device.Revoked {
			return DeviceInfo{}, protocol.Fail(protocol.AuthenticationFailed, "recovery snapshot contains revoked member")
		}
		snapshot = append(snapshot, identity.TrustedPeer{ID: device.ID, Name: device.Name, PublicKey: device.PublicKey, GroupID: device.GroupID, PeerIncarnation: device.Incarnation, LocalIncarnation: local.Incarnation, MembershipRevision: device.MembershipRevision})
	}
	err = s.mutateMembershipTrust(func() error {
		return identity.CompleteMembershipRecovery(s.cfg.DataDir, requestID, fingerprint, s.identity.ID(), snapshot)
	})
	s.notifyChange()
	if err != nil {
		return DeviceInfo{}, err
	}
	return s.device(local), nil
}
