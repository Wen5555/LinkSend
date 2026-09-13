package main

import (
	"context"
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
