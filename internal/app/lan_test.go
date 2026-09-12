package app

import (
	"context"
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
