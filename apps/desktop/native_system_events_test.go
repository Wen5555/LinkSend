package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeSystemEventPumpStopsDeliveryAndSubscriptions(t *testing.T) {
	var delivered atomic.Int32
	pump := newNativeSystemEventPump(context.Background(), func(string) { delivered.Add(1) })
	stopped := make(chan struct{}, 1)
	pump.addStop(func() { stopped <- struct{}{} })
	pump.notify("network")
	deadline := time.Now().Add(time.Second)
	for delivered.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if delivered.Load() != 1 {
		t.Fatalf("native event was not delivered: %d", delivered.Load())
	}
	pump.close()
	pump.close()
	select {
	case <-stopped:
	default:
		t.Fatal("native subscription was not stopped")
	}
	pump.notify("wake")
	time.Sleep(10 * time.Millisecond)
	if delivered.Load() != 1 {
		t.Fatalf("event delivered after close: %d", delivered.Load())
	}
}

func TestNativeSystemCriticalEventsBypassBlockedNetworkDelivery(t *testing.T) {
	block := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once
	critical := make(chan string, 8)
	pump := newNativeSystemEventPump(context.Background(), func(string) { once.Do(func() { close(entered) }); <-block }, func(reason string) { critical <- reason })
	defer pump.close()
	pump.notify("network")
	<-entered
	for i := 0; i < 100; i++ {
		pump.notify("network")
	}
	pump.notify("lock")
	pump.notify("sleep")
	pump.notify("wake")
	for _, want := range []string{"lock", "sleep", "wake"} {
		select {
		case got := <-critical:
			if got != want {
				t.Fatalf("critical order=%s want=%s", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("critical event delayed behind network")
		}
	}
	close(block)
}
