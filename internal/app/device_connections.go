package app

import (
	"context"
	"sync"

	"github.com/Wen5555/LinkSend/internal/identity"
	quic "github.com/quic-go/quic-go"
)

// deviceConnectionRegistry observes authenticated connections without owning
// them. It also includes direct callers and legacy peers outside directPool.
// Its methods acquire no task, trust or pool locks and perform no I/O.
type deviceConnectionRegistry struct {
	mu   sync.Mutex
	live map[*quic.Conn]deviceConnection
}

type deviceConnection struct {
	peerID     string
	generation uint64
}

// remember is called only after pinned QUIC authentication and the final
// authorization-generation check. A signaling session ID is not such proof.
func (r *deviceConnectionRegistry) remember(peer *PeerSession) {
	conn := peer.Data.Conn
	r.mu.Lock()
	defer r.mu.Unlock()
	if conn.Context().Err() != nil {
		return
	}
	if r.live == nil {
		r.live = make(map[*quic.Conn]deviceConnection)
	}
	if _, exists := r.live[conn]; exists {
		return
	}
	r.live[conn] = deviceConnection{peerID: peer.PeerID, generation: peer.AuthorizationGeneration}
	// No waiting goroutine or network activity is added. The close callback
	// touches only this in-memory record, including after service shutdown.
	context.AfterFunc(conn.Context(), func() {
		r.mu.Lock()
		delete(r.live, conn)
		r.mu.Unlock()
	})
}

func (r *deviceConnectionRegistry) connected(trusted map[string]identity.TrustedPeer) map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	connected := make(map[string]bool, len(r.live))
	for conn, record := range r.live {
		// Observe closure synchronously; its asynchronous cleanup may still wait.
		if conn.Context().Err() != nil {
			delete(r.live, conn)
			continue
		}
		peer, ok := trusted[record.peerID]
		// Match identity.AuthorizationGeneration's legacy generation fallback.
		if ok && record.generation == max(uint64(1), peer.GrantGeneration) {
			connected[record.peerID] = true
		}
	}
	return connected
}
