package app

import (
	"context"
	"strings"
	"testing"
)

func TestLANConfigurationDefersRestartWhileTaskIsActive(t *testing.T) {
	manager := newTaskManager()
	_, cancel := context.WithCancel(context.Background())
	if _, err := manager.create(TaskSnapshot{Direction: "receive"}, cancel); err != nil {
		t.Fatal(err)
	}
	runtime := &lanRuntime{directory: "old", cfg: DirectConfig{BindAddress: "192.0.2.1:0"}}
	s := &Service{tasks: manager, lan: runtime}
	s.startLANDiscovery("new", DirectConfig{BindAddress: "192.0.2.2:0"})
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	if !runtime.restartPending || runtime.directory != "new" || runtime.cfg.BindAddress != "192.0.2.2:0" {
		t.Fatalf("deferred LAN configuration=%+v", runtime)
	}
}

func TestNetworkChangeInvalidatesFutureSelectionWithoutCancellingActiveTask(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	record, err := s.tasks.create(TaskSnapshot{Direction: "send", PeerID: strings.Repeat("a", 64)}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	s.network = cachedNetworkSelection{key: "stale"}
	if err = s.NetworkChanged("wake"); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil || record.snapshot().State == "cancel_requested" || s.network.key != "" {
		t.Fatalf("network refresh cancelled healthy task or retained cache: task=%+v cache=%+v", record.snapshot(), s.network)
	}
}
