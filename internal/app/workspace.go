package app

import (
	"context"
	"errors"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
)

type changeHub struct {
	mu   sync.Mutex
	seq  atomic.Uint64
	subs map[chan uint64]struct{}
}

// WorkspaceSnapshot contains local presentation metadata only. Private source
// recovery maps, checkpoint paths and file/image bytes never cross this DTO.
type WorkspaceSnapshot struct {
	Epoch                string         `json:"epoch"`
	Revision             uint64         `json:"revision"`
	Draft                SendDraft      `json:"draft"`
	Queue                []QueueItem    `json:"queue"`
	Tasks                []TaskSnapshot `json:"tasks"`
	QueuePaused          bool           `json:"queue_paused"`
	PersistenceAvailable bool           `json:"persistence_available"`
}

type WorkspaceChange struct {
	Epoch    string `json:"epoch"`
	Revision uint64 `json:"revision"`
}

func (s *Service) WorkspaceChange() WorkspaceChange {
	return WorkspaceChange{Epoch: s.epoch, Revision: s.changes.seq.Load()}
}

func (s *Service) notifyChange() {
	s.changes.mu.Lock()
	defer s.changes.mu.Unlock()
	revision := s.changes.seq.Add(1)
	for ch := range s.changes.subs {
		select {
		case ch <- revision:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- revision:
			default:
			}
		}
	}
	if s.queue.wake != nil {
		select {
		case s.queue.wake <- struct{}{}:
		default:
		}
	}
}

// SubscribeChanges coalesces invalidations. Consumers fetch an authoritative
// snapshot after loss/resume instead of attempting to replay task transitions.
func (s *Service) SubscribeChanges() (<-chan uint64, func()) {
	ch := make(chan uint64, 1)
	s.changes.mu.Lock()
	if s.changes.subs == nil {
		s.changes.subs = make(map[chan uint64]struct{})
	}
	s.changes.subs[ch] = struct{}{}
	s.changes.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() { s.changes.mu.Lock(); delete(s.changes.subs, ch); close(ch); s.changes.mu.Unlock() })
	}
}

func (s *Service) beginWorkspaceWork() (func(), error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return nil, err
	}
	if s.store == nil || s.storeErr != nil {
		done()
		return nil, errors.New("WORKSPACE_STORE_UNAVAILABLE")
	}
	return done, nil
}

func (s *Service) beginProfileWork() (func(), error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.isClosing() {
		return nil, errors.New("APP_CLOSING")
	}
	s.tasks.workers.Add(1)
	return s.tasks.workers.Done, nil
}

func (s *Service) Workspace() (WorkspaceSnapshot, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	defer done()
	revision := s.changes.seq.Load()
	draft, err := s.store.Draft("main")
	if errors.Is(err, ErrMetadataNotFound) {
		draft = SendDraft{ID: "main", Paths: []string{}}
		err = nil
	}
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	queue, err := s.queueItems()
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	s.queue.mu.Lock()
	paused := s.queue.paused
	persistent := !s.queue.persistFailed && s.tasks.historyAvailable()
	s.queue.mu.Unlock()
	return WorkspaceSnapshot{Epoch: s.epoch, Revision: revision, Draft: draft, Queue: queue, Tasks: s.Tasks(), QueuePaused: paused, PersistenceAvailable: persistent}, nil
}

func (s *Service) SaveDraft(draft SendDraft, workingDir string) (SendDraft, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return SendDraft{}, err
	}
	defer done()
	result, err := s.store.SaveDraft(draft, workingDir)
	if err == nil {
		s.notifyChange()
	}
	return result, err
}

func (s *Service) SaveDeviceProfile(profile DeviceProfile) (DeviceProfile, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return DeviceProfile{}, err
	}
	defer done()
	if profile.PeerID == s.identity.ID() {
		return DeviceProfile{}, errors.New("INVALID_ARGUMENT: peer profile required")
	}
	// Labels, pins and directories cannot grant trust or consent.
	result, err := s.store.SaveDeviceProfile(profile)
	if err == nil {
		s.notifyChange()
	}
	return result, err
}

func (s *Service) DeviceProfiles() ([]DeviceProfile, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return nil, err
	}
	defer done()
	return s.store.DeviceProfiles()
}

func (s *Service) decorateDevices(devices []DeviceInfo) []DeviceInfo {
	if s.store == nil {
		return devices
	}
	profiles, err := s.store.DeviceProfiles()
	if err != nil {
		return devices
	}
	byID := make(map[string]DeviceProfile, len(profiles))
	for _, p := range profiles {
		byID[p.PeerID] = p
	}
	for i := range devices {
		if p, ok := byID[devices[i].ID]; ok {
			devices[i].Profile = p
		}
	}
	sort.SliceStable(devices, func(i, j int) bool {
		a, b := devices[i].Profile, devices[j].Profile
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		if a.Pinned && a.Position != b.Position {
			return a.Position < b.Position
		}
		if a.LastUsedAt != b.LastUsedAt {
			return a.LastUsedAt > b.LastUsedAt
		}
		return devices[i].ID < devices[j].ID
	})
	return devices
}

func (s *Service) receiveDirectory(peerID, fallback string) (string, error) {
	if s.storeErr != nil {
		return "", errors.New("WORKSPACE_STORE_UNAVAILABLE")
	}
	if s.store == nil {
		return fallback, nil
	}
	profile, err := s.store.DeviceProfile(peerID)
	if errors.Is(err, ErrMetadataNotFound) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	if profile.ReceiveDirectory != "" {
		info, err := os.Stat(profile.ReceiveDirectory)
		if err != nil || !info.IsDir() {
			return "", errors.New("RECEIVE_DIRECTORY_UNAVAILABLE")
		}
		return profile.ReceiveDirectory, nil
	}
	return fallback, nil
}

// MergeDraftPaths is shared by OS activation and native file drop. Re-reading
// on CAS conflict preserves edits that happened while the activation arrived.
func (s *Service) MergeDraftPaths(paths []string, workingDir string) (SendDraft, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return SendDraft{}, err
	}
	defer done()
	for range 4 {
		draft, err := s.store.Draft("main")
		if errors.Is(err, ErrMetadataNotFound) {
			draft = SendDraft{ID: "main", Paths: []string{}}
			err = nil
		}
		if err != nil {
			return SendDraft{}, err
		}
		draft.Paths = append(draft.Paths, paths...)
		next, err := s.store.SaveDraft(draft, workingDir)
		if errors.Is(err, ErrMetadataConflict) {
			continue
		}
		if err == nil {
			s.notifyChange()
		}
		return next, err
	}
	return SendDraft{}, ErrMetadataConflict
}

func (s *Service) taskChanged(task TaskSnapshot) {
	if task.State == "completed" && task.PeerID != "" && s.store != nil {
		_ = s.store.TouchDevice(task.PeerID, time.Now().UTC())
	}
	s.notifyChange()
}

func (s *Service) initializeWorkspace() {
	s.epoch = protocol.RandomID()
	s.queue.wake = make(chan struct{}, 1)
	s.workCtx, s.workCancel = context.WithCancel(context.Background())
	s.store, s.storeErr = openDesktopStore(s.tasks.historyPath)
	if s.storeErr == nil {
		_, s.storeErr = s.store.db.Exec(`UPDATE send_queue SET state='needs_attention',last_error='RESTART_CONFIRMATION_REQUIRED',revision=revision+1,updated_at=? WHERE state IN ('queued','waiting_peer','running')`, time.Now().UTC().Format(time.RFC3339Nano))
		var paused string
		if err := s.store.db.QueryRow(`SELECT value FROM metadata WHERE key='queue_paused'`).Scan(&paused); err == nil {
			s.queue.paused = paused == "1"
		}
	}
	s.tasks.onChange = s.taskChanged
	// Loaded rows receive the same event owner without replaying old completion.
	for _, task := range s.tasks.tasks {
		task.changed = s.taskChanged
	}
}
