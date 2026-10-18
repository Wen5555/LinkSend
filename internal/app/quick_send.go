package app

import (
	"database/sql"
	"errors"
	"strconv"
)

const QuickSendDraftID = "quick"

// A saved revision is one deliberate quick-send intent. Derive its request ID
// in Go so closing the panel or restarting cannot duplicate a lost response.
func (s *Service) EnqueueQuickSend(revision uint64, waitForPeer bool) (QueueItem, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return QueueItem{}, err
	}
	defer done()
	if revision == 0 || !validMetadataRevision(revision) {
		return QueueItem{}, ErrMetadataInvalid
	}
	requestID := "quick-send-v1-" + strconv.FormatUint(revision, 10)
	if existing, err := scanQueue(s.store.db.QueryRow("SELECT "+queueColumns+" FROM send_queue WHERE request_id=?", requestID)); err == nil {
		if existing.DraftID != QuickSendDraftID || existing.DraftRevision != revision || existing.WaitForPeer != waitForPeer {
			return QueueItem{}, ErrMetadataConflict
		}
		return existing.QueueItem, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return QueueItem{}, err
	}
	draft, err := s.store.Draft(QuickSendDraftID)
	if err != nil {
		return QueueItem{}, err
	}
	if draft.Revision != revision || len(draft.PeerIDs) != 0 || draft.SourceSelection != nil {
		return QueueItem{}, ErrMetadataConflict
	}
	return s.Enqueue(EnqueueRequest{RequestID: requestID, PeerID: draft.PeerID, Paths: draft.Paths, DraftID: draft.ID, DraftRevision: draft.Revision, WaitForPeer: waitForPeer})
}
