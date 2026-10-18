package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func TestMembershipTransitionRetainsLANButCancelsOldTaskAndQueue(t *testing.T) {
	svc, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Shutdown()
	peer, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	initial := []signaling.Device{
		{ID: svc.identity.ID(), PublicKey: svc.identity.PublicKey(), GroupID: "old", Incarnation: strings.Repeat("a", 32), MembershipRevision: 1},
		{ID: peer.ID(), PublicKey: peer.PublicKey(), GroupID: "old", Incarnation: strings.Repeat("b", 32), MembershipRevision: 1},
	}
	if err = svc.syncPairedDevices(initial); err != nil {
		t.Fatal(err)
	}
	generation, err := identity.AuthorizationGeneration(svc.cfg.DataDir, peer.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.CommitLANPeer(svc.cfg.DataDir, identity.TrustedPeer{ID: peer.ID(), PublicKey: peer.PublicKey(), LastLANAddress: "192.168.10.5"}, generation); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "pending.txt")
	if err = os.WriteFile(source, []byte("must not resume under retained LAN"), 0600); err != nil {
		t.Fatal(err)
	}
	item, err := svc.Enqueue(EnqueueRequest{RequestID: "old-membership", PeerID: peer.ID(), Paths: []string{source}, WaitForPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.setQueueState(item.ID, "needs_attention", "RESTART_CONFIRMATION_REQUIRED"); err != nil {
		t.Fatal(err)
	}
	cancelled := make(chan struct{})
	var once sync.Once
	task, err := svc.tasks.create(TaskSnapshot{Direction: "send", PeerID: peer.ID(), AuthorizationGeneration: generation}, func() { once.Do(func() { close(cancelled) }) })
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.syncPairedDevices([]signaling.Device{{ID: svc.identity.ID(), PublicKey: svc.identity.PublicKey(), GroupID: "new", Incarnation: strings.Repeat("c", 32), MembershipRevision: 1}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("old group's active task survived through its independent LAN grant")
	}
	if err = svc.checkTaskGrant(task.snapshot()); protocol.ErrorCode(err) != protocol.AuthenticationFailed {
		t.Fatalf("old active task can use retained LAN: %v", err)
	}
	items, err := svc.queueItems()
	if err != nil || len(items) != 1 {
		t.Fatalf("queue history lost: %+v %v", items, err)
	}
	if err = svc.ConfirmQueue(item.ID, items[0].Revision); protocol.ErrorCode(err) != protocol.AuthenticationFailed {
		t.Fatalf("old queue can rebind to retained LAN: %v", err)
	}
	if err = svc.checkPeerAllowed(peer.ID()); err != nil {
		t.Fatalf("valid independent LAN relation was removed: %v", err)
	}
	if next, e := svc.Enqueue(EnqueueRequest{RequestID: "fresh-LAN-intent", PeerID: peer.ID(), Paths: []string{source}, WaitForPeer: true}); e != nil || next.AuthorizationGeneration <= generation {
		t.Fatalf("fresh explicit LAN enqueue rejected: %+v %v", next, e)
	}
	svc.Shutdown()
	reopened, err := New(Config{DataDir: svc.cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	items, err = reopened.queueItems()
	if err != nil || len(items) != 2 {
		t.Fatalf("queue records did not survive restart: %+v %v", items, err)
	}
	for _, saved := range items {
		if saved.ID == item.ID {
			if err = reopened.ConfirmQueue(saved.ID, saved.Revision); protocol.ErrorCode(err) != protocol.AuthenticationFailed {
				t.Fatalf("restart rebound old queue to retained LAN: %v", err)
			}
		}
	}
}
