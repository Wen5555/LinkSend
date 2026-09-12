package app

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestQueueTerminalReceiptWaitsForTaskDurability(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "delayed"
		if fail {
			name = "failed"
		}
		t.Run(name, func(t *testing.T) {
			f := newDirectFixtureServices(t)
			source := filepath.Join(t.TempDir(), "source.txt")
			if err := os.WriteFile(source, []byte("durable receipt"), 0600); err != nil {
				t.Fatal(err)
			}
			item, err := f.a.Enqueue(EnqueueRequest{RequestID: "durable-queue", PeerID: f.bID.ID(), Paths: []string{source}, WaitForPeer: true})
			if err != nil {
				t.Fatal(err)
			}
			task, err := f.a.tasks.create(TaskSnapshot{Direction: "send", PeerID: f.bID.ID()}, func() {})
			if err != nil {
				t.Fatal(err)
			}
			id := task.snapshot().ID
			if _, err = f.a.store.db.Exec(`UPDATE send_queue SET state='running',task_id=? WHERE id=?`, id, item.ID); err != nil {
				t.Fatal(err)
			}
			entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			save := task.persist
			task.persist = func(snap TaskSnapshot, recovery taskRecovery) error {
				if snap.State == "completed" {
					close(entered)
					<-release
					if fail {
						return errors.New("injected terminal save failure")
					}
				}
				return save(snap, recovery)
			}
			go func() { task.finish("completed", nil); close(finished) }()
			<-entered
			f.a.dispatchQueue(t.Context())
			queued := waitQueue(t, f.a, item.ID, func(q QueueItem) bool { return q.State == "running" })
			if queued.State != "running" {
				t.Fatal("queue outran its task commit", queued)
			}
			once.Do(func() { close(release) })
			<-finished
			f.a.dispatchQueue(t.Context())
			state := "completed"
			if fail {
				state = "needs_attention"
			}
			queued = waitQueue(t, f.a, item.ID, func(q QueueItem) bool { return q.State == state })
			if fail {
				if queued.LastError != "TASK_PERSISTENCE_FAILED" {
					t.Fatal("missing persistence failure", queued)
				}
			} else {
				persisted, _, err := f.a.loadInboxTask(t.Context(), id)
				if err != nil || persisted.State != "completed" || persisted.Revision != task.snapshot().Revision {
					t.Fatal("receipt lacks matching durable task", persisted, err)
				}
			}
		})
	}
}
