package app

import (
	"context"
	"errors"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
)

const clipboardLeaseTTL = 10 * time.Second

type ClipboardChange struct {
	Generation uint64
	Kinds      []clipboardsync.Kind
}

type ClipboardPeerStatus struct {
	PeerID       string `json:"peer_id"`
	State        string `json:"state"`
	SendReady    bool   `json:"send_ready"`
	ReceiveReady bool   `json:"receive_ready"`
	Waiting      string `json:"waiting,omitempty"`
	Error        string `json:"error,omitempty"`
}

type clipboardPeerRuntimeStatus struct {
	Waiting string
	Error   string
}

type clipboardSendJob struct {
	peer  *PeerSession
	lease clipboardsync.Lease
	event clipboardsync.Event
}

type clipboardPeerSender struct {
	mu      sync.Mutex
	wake    chan struct{}
	pending *clipboardSendJob
	current *clipboardSendJob
	cancel  context.CancelFunc
}

func (s *clipboardPeerSender) enqueue(job *clipboardSendJob) {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.pending = job
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func clipboardMessageGenerationValid(peer *PeerSession, message clipboardsync.Message) bool {
	if peer == nil {
		return false
	}
	switch message.Type {
	case "lease":
		return peer.RemoteAuthorizationGeneration > 0 && message.Generation == peer.RemoteAuthorizationGeneration
	case "event":
		return peer.AuthorizationGeneration > 0 && message.Generation == peer.AuthorizationGeneration
	default:
		return false
	}
}

func (s *Service) ClipboardPeerStatuses() []ClipboardPeerStatus {
	s.directPoolMu.Lock()
	statuses := make([]ClipboardPeerStatus, 0, len(s.directPool))
	seen := make(map[string]bool, len(s.directPool))
	for _, pooled := range s.directPool {
		if pooled.peer == nil {
			continue
		}
		state := "ready"
		if !pooled.peer.ClipboardSync {
			state = "unsupported"
		} else if !pooled.clipboardSendReady && !pooled.clipboardReceiveReady {
			state = "connecting"
		}
		statuses = append(statuses, ClipboardPeerStatus{PeerID: pooled.peer.PeerID, State: state, SendReady: pooled.clipboardSendReady, ReceiveReady: pooled.clipboardReceiveReady})
		seen[pooled.peer.PeerID] = true
	}
	s.directPoolMu.Unlock()
	s.clipboardStatusMu.Lock()
	for index := range statuses {
		runtime := s.clipboardPeerRuntime[statuses[index].PeerID]
		statuses[index].Waiting = runtime.Waiting
		statuses[index].Error = runtime.Error
	}
	for peerID, runtime := range s.clipboardPeerRuntime {
		if !seen[peerID] {
			statuses = append(statuses, ClipboardPeerStatus{PeerID: peerID, State: "connecting", Waiting: runtime.Waiting, Error: runtime.Error})
		}
	}
	s.clipboardStatusMu.Unlock()
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].PeerID < statuses[j].PeerID })
	return statuses
}

func (s *Service) setClipboardPeerRuntime(peerID, waiting, statusError string) {
	if peerID == "" {
		return
	}
	s.clipboardStatusMu.Lock()
	if waiting == "" && statusError == "" {
		delete(s.clipboardPeerRuntime, peerID)
	} else {
		s.clipboardPeerRuntime[peerID] = clipboardPeerRuntimeStatus{Waiting: waiting, Error: statusError}
	}
	s.clipboardStatusMu.Unlock()
}

func (s *Service) clearClipboardPeerRuntime(peerID string) {
	s.setClipboardPeerRuntime(peerID, "", "")
}

func (s *Service) setClipboardPeerWaiting(peerID, waiting string) {
	if peerID == "" {
		return
	}
	s.clipboardStatusMu.Lock()
	status := s.clipboardPeerRuntime[peerID]
	status.Waiting = waiting
	if status.Waiting == "" && status.Error == "" {
		delete(s.clipboardPeerRuntime, peerID)
	} else {
		s.clipboardPeerRuntime[peerID] = status
	}
	s.clipboardStatusMu.Unlock()
}

func (s *Service) setClipboardPeerError(peerID, statusError string) {
	s.setClipboardPeerRuntime(peerID, "", statusError)
}

type ClipboardAdapter struct {
	Generation func() uint64
	Read       func(context.Context, clipboardsync.Kind, uint64) ([]byte, uint64, error)
	Validate   func(clipboardsync.Kind, []byte) error
	Write      func(clipboardsync.Kind, []byte, uint64, time.Time) (uint64, error)
}

func (s *Service) ConfigureClipboard(adapter ClipboardAdapter) {
	s.clipboardAdapterMu.Lock()
	s.clipboardAdapter = adapter
	s.clipboardAdapterMu.Unlock()
	generation := uint64(0)
	if adapter.Generation != nil {
		generation = adapter.Generation()
	}
	s.clipboardSync.Reset(false, generation)
}

func (s *Service) invalidateClipboardState() {
	s.clipboardAdapterMu.RLock()
	adapter := s.clipboardAdapter
	s.clipboardAdapterMu.RUnlock()
	generation := uint64(0)
	if adapter.Generation != nil {
		generation = adapter.Generation()
	}
	s.ResetClipboard(s.clipboardPaused.Load(), generation)
}

func (s *Service) clipboardKinds(peerID, direction string, generation uint64) []clipboardsync.Kind {
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	return s.clipboardKindsLocked(peerID, direction, generation)
}

func (s *Service) clipboardKindsLocked(peerID, direction string, generation uint64) []clipboardsync.Kind {
	rows, err := s.store.db.Query(`SELECT kind FROM clipboard_grants WHERE peer_id=? AND direction=? AND enabled=1 AND authorization_generation=? ORDER BY kind`, peerID, direction, generation)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var kinds []clipboardsync.Kind
	for rows.Next() {
		var kind string
		if rows.Scan(&kind) == nil {
			kinds = append(kinds, clipboardsync.Kind(kind))
		}
	}
	return kinds
}

func (s *Service) clipboardGrant(peerID, direction string, kind clipboardsync.Kind, generation uint64) (ClipboardGrant, bool) {
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	return s.clipboardGrantLocked(peerID, direction, kind, generation)
}

func (s *Service) clipboardGrantLocked(peerID, direction string, kind clipboardsync.Kind, generation uint64) (ClipboardGrant, bool) {
	var grant ClipboardGrant
	err := s.store.db.QueryRow(`SELECT peer_id,direction,kind,enabled,revision,updated_at,authorization_generation FROM clipboard_grants WHERE peer_id=? AND direction=? AND kind=? AND enabled=1 AND authorization_generation=?`, peerID, direction, kind, generation).Scan(&grant.PeerID, &grant.Direction, &grant.Kind, &grant.Enabled, &grant.Revision, &grant.UpdatedAt, &grant.AuthorizationGeneration)
	return grant, err == nil && grant.Enabled
}

func (s *Service) clipboardGrants(peerID, direction string, generation uint64) []clipboardsync.Grant {
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	return s.clipboardGrantsLocked(peerID, direction, generation)
}

func (s *Service) clipboardGrantsLocked(peerID, direction string, generation uint64) []clipboardsync.Grant {
	rows, err := s.store.db.Query(`SELECT kind,revision FROM clipboard_grants WHERE peer_id=? AND direction=? AND enabled=1 AND authorization_generation=? ORDER BY kind`, peerID, direction, generation)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var grants []clipboardsync.Grant
	for rows.Next() {
		var kind string
		var revision uint64
		if rows.Scan(&kind, &revision) == nil {
			grants = append(grants, clipboardsync.Grant{Kind: clipboardsync.Kind(kind), Revision: revision})
		}
	}
	return grants
}

func containsClipboardKind(kinds []clipboardsync.Kind, kind clipboardsync.Kind) bool {
	for _, candidate := range kinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

func (s *Service) ClipboardChanged(ctx context.Context, change ClipboardChange) error {
	if s.clipboardPaused.Load() || change.Generation == 0 {
		return clipboardsync.ErrStale
	}
	observation, err := s.clipboardSync.ObserveLocalEvent(change.Generation)
	if err != nil {
		return err
	}
	s.cancelClipboardSends("", "")
	if len(change.Kinds) == 0 {
		return nil
	}
	s.clipboardAdapterMu.RLock()
	adapter := s.clipboardAdapter
	s.clipboardAdapterMu.RUnlock()
	if adapter.Read == nil {
		return errors.New("CLIPBOARD_ADAPTER_UNAVAILABLE")
	}
	s.directPoolMu.Lock()
	peers := make([]*PeerSession, 0, len(s.directPool))
	for _, pooled := range s.directPool {
		if pooled.peer != nil && pooled.peer.Data != nil && pooled.peer.Data.Conn != nil && pooled.peer.Data.Conn.Context().Err() == nil {
			peers = append(peers, pooled.peer)
		}
	}
	s.directPoolMu.Unlock()
	for _, kind := range change.Kinds {
		targets := make([]struct {
			peer     *PeerSession
			lease    clipboardsync.Lease
			revision uint64
		}, 0, len(peers))
		for _, peer := range peers {
			if !peer.ClipboardSync || s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration) != nil {
				continue
			}
			grant, allowed := s.clipboardGrant(peer.PeerID, "send", kind, peer.AuthorizationGeneration)
			lease, leased := s.clipboardSync.Outbound(peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration, kind)
			if allowed && !leased {
				s.setClipboardPeerWaiting(peer.PeerID, "waiting_lease")
			}
			if leased && allowed {
				targets = append(targets, struct {
					peer     *PeerSession
					lease    clipboardsync.Lease
					revision uint64
				}{peer, lease, grant.Revision})
			}
		}
		if len(targets) == 0 {
			continue
		}
		payload, current, err := adapter.Read(ctx, kind, change.Generation)
		if err != nil || current != change.Generation {
			statusError := "read_failed"
			if current != change.Generation {
				statusError = "clipboard_changed"
			}
			for _, target := range targets {
				s.setClipboardPeerError(target.peer.PeerID, statusError)
			}
			continue
		}
		base, err := s.clipboardSync.PrepareObserved(observation, kind, payload)
		if err != nil {
			return err
		}
		for _, target := range targets {
			event, err := s.clipboardSync.BindScoped(base, target.lease.ID, target.revision)
			if err != nil {
				continue
			}
			s.startClipboardSend(target.peer, target.lease, event)
		}
		break
	}
	return nil
}

type clipboardGuardedWriter struct {
	ctx      context.Context
	dst      io.Writer
	state    *clipboardsync.State
	event    clipboardsync.Event
	deadline time.Time
}

func (w clipboardGuardedWriter) Write(payload []byte) (int, error) {
	total := 0
	for len(payload) > 0 {
		if w.ctx.Err() != nil || !time.Now().Before(w.deadline) || !w.state.LocalCurrent(w.event) {
			return total, clipboardsync.ErrStale
		}
		n := 64 << 10
		if len(payload) < n {
			n = len(payload)
		}
		written, err := w.dst.Write(payload[:n])
		total += written
		payload = payload[written:]
		if err != nil {
			return total, err
		}
		if written == 0 {
			return total, io.ErrShortWrite
		}
		if len(payload) > 0 {
			timer := time.NewTimer(4 * time.Millisecond)
			select {
			case <-w.ctx.Done():
				timer.Stop()
				return total, w.ctx.Err()
			case <-timer.C:
			}
		}
	}
	return total, nil
}

func (s *Service) startClipboardSend(peer *PeerSession, lease clipboardsync.Lease, event clipboardsync.Event) {
	if peer == nil || peer.Data == nil || peer.Data.Conn == nil {
		return
	}
	key := peer.PeerID
	s.operationMu.Lock()
	if s.isClosing() {
		s.operationMu.Unlock()
		return
	}
	s.clipboardDispatchMu.Lock()
	sender := s.clipboardSenders[key]
	if sender == nil {
		sender = &clipboardPeerSender{wake: make(chan struct{}, 1)}
		s.clipboardSenders[key] = sender
		s.clipboardWorkers.Add(1)
		go s.runClipboardSender(key, sender)
	}
	s.clipboardDispatchMu.Unlock()
	s.operationMu.Unlock()
	job := &clipboardSendJob{peer: peer, lease: lease, event: event}
	sender.enqueue(job)
	s.setClipboardPeerWaiting(peer.PeerID, "queued")
}

func (s *Service) runClipboardSender(key string, sender *clipboardPeerSender) {
	defer s.clipboardWorkers.Done()
	defer func() {
		s.clipboardDispatchMu.Lock()
		if s.clipboardSenders[key] == sender {
			delete(s.clipboardSenders, key)
		}
		s.clipboardDispatchMu.Unlock()
	}()
	serviceCtx := s.workCtx
	if serviceCtx == nil {
		serviceCtx = context.Background()
	}
	for {
		select {
		case <-serviceCtx.Done():
			return
		case <-sender.wake:
		}
		sender.mu.Lock()
		job := sender.pending
		sender.pending = nil
		if job == nil {
			sender.mu.Unlock()
			continue
		}
		connectionCtx := job.peer.Data.Conn.Context()
		jobCtx, cancel := context.WithDeadline(connectionCtx, job.lease.ExpiresAt())
		stopServiceCancel := context.AfterFunc(serviceCtx, cancel)
		sender.current, sender.cancel = job, cancel
		sender.mu.Unlock()
		err := s.sendClipboardJob(jobCtx, job)
		stopServiceCancel()
		cancel()
		sender.mu.Lock()
		current := sender.current == job
		pending := sender.pending != nil
		if current {
			sender.current, sender.cancel = nil, nil
		}
		sender.mu.Unlock()
		if !current {
			continue
		}
		if pending {
			s.setClipboardPeerWaiting(job.peer.PeerID, "queued")
		} else if err == nil {
			s.clearClipboardPeerRuntime(job.peer.PeerID)
		} else if s.clipboardPaused.Load() || !s.clipboardSync.LocalCurrent(job.event) {
			s.clearClipboardPeerRuntime(job.peer.PeerID)
		} else if errors.Is(err, context.DeadlineExceeded) || !time.Now().Before(job.lease.ExpiresAt()) {
			s.setClipboardPeerError(job.peer.PeerID, "send_timeout")
		} else if connectionCtx.Err() != nil {
			s.setClipboardPeerError(job.peer.PeerID, "connection_lost")
		} else {
			s.setClipboardPeerError(job.peer.PeerID, "send_failed")
		}
	}
}

func (s *Service) sendClipboardJob(ctx context.Context, job *clipboardSendJob) error {
	s.setClipboardPeerWaiting(job.peer.PeerID, "waiting_slot")
	select {
	case s.clipboardSendSlots <- struct{}{}:
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	defer func() { <-s.clipboardSendSlots }()
	s.setClipboardPeerWaiting(job.peer.PeerID, "opening_stream")
	stream, err := job.peer.Data.Conn.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}
	deadline := job.lease.ExpiresAt()
	_ = stream.SetWriteDeadline(deadline)
	done := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			stream.CancelWrite(0)
		case <-done:
		}
	}()
	s.setClipboardPeerWaiting(job.peer.PeerID, "sending")
	message := clipboardsync.Message{Type: "event", LeaseID: job.event.LeaseID, SessionID: job.peer.SessionID, Generation: job.peer.RemoteAuthorizationGeneration, OriginID: job.event.OriginID, Boot: job.event.Boot, OriginSeq: job.event.OriginSeq, Lamport: job.event.Lamport, Kind: job.event.Kind, Digest: job.event.Digest, OSGeneration: job.event.OSGeneration, SenderGrantRevision: job.event.SenderGrantRevision, ReceiverGrantRevision: job.event.ReceiverGrantRevision, PayloadBytes: uint32(len(job.event.Payload))}
	writer := clipboardGuardedWriter{ctx: ctx, dst: stream, state: s.clipboardSync, event: job.event, deadline: deadline}
	err = clipboardsync.Write(writer, message, job.event.Payload)
	close(done)
	<-watcherDone
	if err != nil {
		stream.CancelWrite(0)
		return err
	}
	return stream.Close()
}

func (s *Service) cancelClipboardSends(peerID string, kind clipboardsync.Kind) {
	s.clipboardDispatchMu.Lock()
	senders := make([]*clipboardPeerSender, 0, len(s.clipboardSenders))
	for _, sender := range s.clipboardSenders {
		senders = append(senders, sender)
	}
	s.clipboardDispatchMu.Unlock()
	for _, sender := range senders {
		sender.mu.Lock()
		matches := func(job *clipboardSendJob) bool {
			return job != nil && (peerID == "" || job.peer.PeerID == peerID) && (kind == "" || job.event.Kind == kind)
		}
		if matches(sender.pending) {
			sender.pending = nil
		}
		if matches(sender.current) && sender.cancel != nil {
			sender.cancel()
		}
		sender.mu.Unlock()
	}
}

func (s *Service) clearClipboardWaiting() {
	s.clipboardStatusMu.Lock()
	for peerID, status := range s.clipboardPeerRuntime {
		status.Waiting = ""
		if status.Error == "" {
			delete(s.clipboardPeerRuntime, peerID)
		} else {
			s.clipboardPeerRuntime[peerID] = status
		}
	}
	s.clipboardStatusMu.Unlock()
}

func (s *Service) serveClipboardPeer(peer *PeerSession) {
	ctx := peer.Data.Conn.Context()
	peer.mu.Lock()
	if peer.clipboardLeaseWake == nil {
		peer.clipboardLeaseWake = make(chan struct{}, 1)
	}
	leaseWake := peer.clipboardLeaseWake
	peer.mu.Unlock()
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		s.sendClipboardLease(ctx, peer)
		s.setClipboardReady(peer, "send", s.clipboardSync.HasOutbound(peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration))
		ticker := time.NewTicker(7 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sendClipboardLease(ctx, peer)
				s.setClipboardReady(peer, "send", s.clipboardSync.HasOutbound(peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration))
			case <-leaseWake:
				s.sendClipboardLease(ctx, peer)
				s.setClipboardReady(peer, "send", s.clipboardSync.HasOutbound(peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration))
			}
		}
	}()
	defer func() { <-renewDone; s.clipboardSync.DropSession(peer.PeerID, peer.SessionID) }()
	for {
		stream, err := peer.Data.Conn.AcceptUniStream(ctx)
		if err != nil {
			return
		}
		_ = stream.SetReadDeadline(time.Now().Add(clipboardLeaseTTL))
		message, err := clipboardsync.ReadHeader(stream)
		if err != nil {
			stream.CancelRead(0)
			continue
		}
		if message.SessionID != peer.SessionID || !clipboardMessageGenerationValid(peer, message) || s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration) != nil {
			stream.CancelRead(0)
			continue
		}
		if s.clipboardPaused.Load() {
			stream.CancelRead(0)
			continue
		}
		switch message.Type {
		case "lease":
			payload, payloadErr := clipboardsync.ReadPayload(stream, message)
			stream.CancelRead(0)
			if payloadErr != nil || len(payload) != 0 {
				continue
			}
			var grants []clipboardsync.Grant
			for _, remoteGrant := range message.Grants {
				if _, ok := s.clipboardGrant(peer.PeerID, "send", remoteGrant.Kind, peer.AuthorizationGeneration); ok {
					grants = append(grants, clipboardsync.Grant{Kind: remoteGrant.Kind, Revision: remoteGrant.Revision})
				}
			}
			s.clipboardAdapterMu.RLock()
			adapter := s.clipboardAdapter
			s.clipboardAdapterMu.RUnlock()
			if adapter.Generation == nil {
				continue
			}
			if err := s.clipboardSync.InstallScoped(message.LeaseID, peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration, grants, time.Duration(message.TTLMillis)*time.Millisecond, adapter.Generation()); err == nil {
				s.setClipboardReady(peer, "send", true)
			}
		case "event":
			if message.OriginID != peer.PeerID {
				stream.CancelRead(0)
				continue
			}
			if !containsClipboardKind(s.clipboardKinds(peer.PeerID, "receive", peer.AuthorizationGeneration), message.Kind) {
				stream.CancelRead(0)
				continue
			}
			event := clipboardsync.Event{LeaseID: message.LeaseID, OriginID: message.OriginID, Boot: message.Boot, OriginSeq: message.OriginSeq, Lamport: message.Lamport, Kind: message.Kind, Digest: message.Digest, OSGeneration: message.OSGeneration, SenderGrantRevision: message.SenderGrantRevision, ReceiverGrantRevision: message.ReceiverGrantRevision}
			candidate, err := s.clipboardSync.BeginHeader(peer.PeerID, peer.SessionID, peer.AuthorizationGeneration, event)
			if err != nil {
				stream.CancelRead(0)
				continue
			}
			eventCtx, cancelEvent := context.WithDeadline(ctx, candidate.Deadline)
			s.setClipboardPeerWaiting(peer.PeerID, "waiting_receive_slot")
			select {
			case s.clipboardReceiveSlots <- struct{}{}:
			case <-eventCtx.Done():
				stream.CancelRead(0)
				cancelEvent()
				if ctx.Err() != nil {
					return
				}
				s.setClipboardPeerError(peer.PeerID, "receive_timeout")
				continue
			}
			func() {
				defer cancelEvent()
				defer func() { <-s.clipboardReceiveSlots }()
				s.setClipboardPeerWaiting(peer.PeerID, "receiving")
				_ = stream.SetReadDeadline(candidate.Deadline)
				payload, readErr := clipboardsync.ReadPayload(stream, message)
				stream.CancelRead(0)
				if readErr != nil {
					s.setClipboardPeerError(peer.PeerID, "receive_failed")
					return
				}
				candidate, readErr = s.clipboardSync.AttachPayload(candidate, payload)
				if readErr != nil {
					s.setClipboardPeerError(peer.PeerID, "receive_invalid")
					return
				}
				s.clipboardAdapterMu.RLock()
				adapter := s.clipboardAdapter
				s.clipboardAdapterMu.RUnlock()
				if adapter.Write == nil {
					s.setClipboardPeerError(peer.PeerID, "receive_failed")
					return
				}
				if commitErr := s.commitClipboardCandidate(peer, candidate, adapter); commitErr != nil {
					if !s.clipboardPaused.Load() {
						s.setClipboardPeerError(peer.PeerID, "write_failed")
					}
					return
				}
				s.clearClipboardPeerRuntime(peer.PeerID)
			}()
		}
	}
}

func (s *Service) commitClipboardCandidate(peer *PeerSession, candidate clipboardsync.Candidate, adapter ClipboardAdapter) error {
	if peer == nil || adapter.Write == nil {
		return errors.New("CLIPBOARD_ADAPTER_UNAVAILABLE")
	}
	if adapter.Validate != nil {
		if err := adapter.Validate(candidate.Kind, candidate.Payload); err != nil {
			return err
		}
	}
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	return s.clipboardSync.Commit(candidate, func(expected uint64, kind clipboardsync.Kind, body []byte) (uint64, error) {
		if s.clipboardPaused.Load() || s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration) != nil {
			return 0, clipboardsync.ErrStale
		}
		grant, enabled := s.clipboardGrantLocked(peer.PeerID, "receive", kind, peer.AuthorizationGeneration)
		if generation, err := s.clipboardPeerGeneration(peer.PeerID); err != nil || generation != peer.AuthorizationGeneration || !enabled || grant.Revision != candidate.ReceiverGrantRevision || s.clipboardPaused.Load() || !time.Now().Before(candidate.Deadline) {
			return 0, clipboardsync.ErrStale
		}
		return adapter.Write(kind, body, expected, candidate.Deadline)
	})
}

func (s *Service) sendClipboardLease(parent context.Context, peer *PeerSession) {
	// The policy snapshot, lease issue and stream write form one serialization
	// boundary with SetClipboardGrant. A stale snapshot therefore completes
	// before the new policy commits; the single renewal worker emits the new
	// policy afterwards instead of letting an old lease overtake it.
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	if s.clipboardPaused.Load() || !peer.ClipboardSync || peer.RemoteAuthorizationGeneration == 0 {
		s.setClipboardReady(peer, "receive", false)
		return
	}
	grants := s.clipboardGrantsLocked(peer.PeerID, "receive", peer.AuthorizationGeneration)
	if len(grants) == 0 {
		s.setClipboardReady(peer, "receive", false)
		return
	}
	lease, err := s.clipboardSync.IssueScoped(peer.PeerID, peer.SessionID, peer.AuthorizationGeneration, grants, clipboardLeaseTTL)
	if err != nil {
		s.setClipboardReady(peer, "receive", false)
		return
	}
	deadline := time.Now().Add(2 * time.Second)
	if parentDeadline, ok := parent.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	stream, err := peer.Data.Conn.OpenUniStreamSync(ctx)
	if err != nil {
		s.setClipboardReady(peer, "receive", false)
		return
	}
	if err = stream.SetWriteDeadline(deadline); err != nil {
		stream.CancelWrite(0)
		s.setClipboardReady(peer, "receive", false)
		return
	}
	if hook := s.clipboardLeaseBeforeWrite; hook != nil {
		hook()
	}
	message := clipboardsync.Message{Type: "lease", LeaseID: lease.ID, SessionID: peer.SessionID, Generation: peer.AuthorizationGeneration, Grants: grants, TTLMillis: uint32(clipboardLeaseTTL / time.Millisecond)}
	done, watcherDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			stream.CancelWrite(0)
		case <-done:
		}
	}()
	err = clipboardsync.Write(stream, message, nil)
	if err == nil {
		err = stream.Close()
	}
	close(done)
	<-watcherDone
	if err != nil {
		stream.CancelWrite(0)
		s.setClipboardReady(peer, "receive", false)
		return
	}
	s.setClipboardReady(peer, "receive", true)
}

// renewClipboardLeases wakes the existing per-session renewal owner after a
// receive-policy change. The capacity-one wake channel coalesces rapid edits.
func (s *Service) renewClipboardLeases(peerID string) {
	s.directPoolMu.Lock()
	if s.isClosing() {
		s.directPoolMu.Unlock()
		return
	}
	peers := make([]*PeerSession, 0, 1)
	for _, pooled := range s.directPool {
		peer := pooled.peer
		if peer == nil || peer.PeerID != peerID || peer.Data == nil || peer.Data.Conn.Context().Err() != nil {
			continue
		}
		peers = append(peers, peer)
	}
	s.directPoolMu.Unlock()
	for _, peer := range peers {
		peer.requestClipboardLeaseRenewal()
	}
}

func (s *Service) setClipboardReady(peer *PeerSession, direction string, ready bool) {
	if ready && s.clipboardPaused.Load() {
		ready = false
	}
	s.directPoolMu.Lock()
	defer s.directPoolMu.Unlock()
	for key, pooled := range s.directPool {
		if pooled.peer != peer {
			continue
		}
		if direction == "send" {
			pooled.clipboardSendReady = ready
		} else {
			pooled.clipboardReceiveReady = ready
		}
		if ready {
			if pooled.timer != nil {
				pooled.timer.Stop()
				pooled.timer = nil
			}
		} else if !pooled.clipboardSendReady && !pooled.clipboardReceiveReady {
			s.schedulePoolIdleLocked(key, pooled)
		}
	}
}

func (s *Service) ResetClipboard(paused bool, generation uint64) {
	s.clipboardPaused.Store(paused)
	s.cancelClipboardSends("", "")
	s.clearClipboardWaiting()
	s.clipboardSync.Reset(paused, generation)
	s.directPoolMu.Lock()
	for key, pooled := range s.directPool {
		pooled.clipboardSendReady = false
		pooled.clipboardReceiveReady = false
		s.schedulePoolIdleLocked(key, pooled)
	}
	s.directPoolMu.Unlock()
}

func (s *Service) EnsureClipboardSessions(ctx context.Context, cfg DirectConfig) error {
	if s.clipboardPaused.Load() || s.isClosing() {
		return nil
	}
	if !s.clipboardEnsureMu.TryLock() {
		return nil
	}
	defer s.clipboardEnsureMu.Unlock()
	connectRoot, cancelConnectRoot := context.WithCancel(ctx)
	stopWork := context.AfterFunc(s.workCtx, cancelConnectRoot)
	defer func() {
		stopWork()
		cancelConnectRoot()
	}()

	s.signalHandoffMu.Lock()
	if s.isClosing() || s.workCtx.Err() != nil || s.clipboardPaused.Load() {
		s.signalHandoffMu.Unlock()
		return nil
	}
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		s.signalHandoffMu.Unlock()
		return workErr
	}
	defer done()
	s.clipboardGrantMu.Lock()
	rows, err := s.store.db.Query(`SELECT DISTINCT peer_id,authorization_generation FROM clipboard_grants WHERE enabled=1`)
	if err != nil {
		s.clipboardGrantMu.Unlock()
		s.signalHandoffMu.Unlock()
		return err
	}
	type target struct {
		id         string
		generation uint64
	}
	var targets []target
	for rows.Next() {
		var item target
		if rows.Scan(&item.id, &item.generation) == nil {
			if current, currentErr := s.clipboardPeerGeneration(item.id); currentErr == nil && current == item.generation {
				targets = append(targets, item)
			}
		}
	}
	rowsErr := rows.Close()
	s.clipboardGrantMu.Unlock()
	if rowsErr != nil {
		s.signalHandoffMu.Unlock()
		return rowsErr
	}
	if len(targets) == 0 {
		s.signalHandoffMu.Unlock()
		return nil
	}
	requiresSignalHandoff := false
	for _, item := range targets {
		key := directPoolKey(item.id, item.generation)
		s.directPoolMu.Lock()
		pooled := s.directPool[key]
		healthy := pooled != nil && pooled.peer != nil && pooled.peer.Data != nil && pooled.peer.Data.Conn.Context().Err() == nil
		s.directPoolMu.Unlock()
		if !healthy {
			requiresSignalHandoff = true
			break
		}
	}
	if !requiresSignalHandoff {
		for _, item := range targets {
			s.setClipboardPeerWaiting(item.id, "")
		}
		s.signalHandoffMu.Unlock()
		return nil
	}
	if s.tasks.hasActive() {
		s.signalHandoffMu.Unlock()
		return nil
	}
	// The signaling server owns one live WSS connection per identity. Park the
	// persistent inbox before every new clipboard dial, then reuse the spare
	// slot. The gate spans one cancellable dial only; it is released before the
	// next peer so shutdown or a file task can take ownership promptly.
	if err := s.stopInbox(false); err != nil {
		s.signalHandoffMu.Unlock()
		return err
	}
	var result error
	for index, item := range targets {
		if s.isClosing() || s.workCtx.Err() != nil || connectRoot.Err() != nil || s.tasks.hasActive() {
			break
		}
		key := directPoolKey(item.id, item.generation)
		s.directPoolMu.Lock()
		pooled := s.directPool[key]
		healthy := pooled != nil && pooled.peer != nil && pooled.peer.Data != nil && pooled.peer.Data.Conn.Context().Err() == nil
		s.directPoolMu.Unlock()
		if healthy {
			s.setClipboardPeerWaiting(item.id, "")
		} else {
			s.setClipboardPeerWaiting(item.id, "connecting")
			connectCtx, cancel := context.WithTimeout(connectRoot, 10*time.Second)
			peerCfg := cfg
			peerCfg.expectedAuthorizationGeneration = item.generation
			peer, connectErr := s.acquirePeerSession(connectCtx, item.id, peerCfg)
			cancel()
			if connectErr != nil {
				s.setClipboardPeerError(item.id, "connection_failed")
				result = errors.Join(result, connectErr)
			} else {
				releaseErr := s.releasePeerSession(peer, nil)
				if releaseErr != nil {
					s.setClipboardPeerError(item.id, "connection_failed")
				} else {
					s.setClipboardPeerWaiting(item.id, "waiting_lease")
				}
				result = errors.Join(result, releaseErr)
			}
		}
		if index+1 >= len(targets) {
			continue
		}
		s.signalHandoffMu.Unlock()
		s.signalHandoffMu.Lock()
		if s.isClosing() || s.workCtx.Err() != nil || connectRoot.Err() != nil || s.clipboardPaused.Load() || s.tasks.hasActive() {
			break
		}
		if err := s.stopInbox(false); err != nil {
			result = errors.Join(result, err)
			break
		}
	}
	s.signalHandoffMu.Unlock()
	s.ensureInbox()
	return result
}
