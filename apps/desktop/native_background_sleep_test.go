package main

import (
	"runtime"
	"sync"
	"testing"
)

func TestNativeSleepRequestLifecycle(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("native sleep requests target Windows/macOS")
	}
	i := newNativeSleepInhibitor()
	t.Cleanup(func() { _ = i.Close() })
	if i.Active() {
		t.Fatal("idle app acquired a sleep request")
	}
	for j := 0; j < 2; j++ {
		if err := i.Acquire(); err != nil {
			t.Fatal(err)
		}
	}
	if !i.Active() {
		t.Fatal("active transfer has no sleep request")
	}
	for j := 0; j < 2; j++ {
		if err := i.Release(); err != nil {
			t.Fatal(err)
		}
	}
	if i.Active() {
		t.Fatal("pause did not release the request")
	}
	if err := i.Acquire(); err != nil {
		t.Fatal(err)
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	if i.Active() || i.Acquire() == nil {
		t.Fatal("closed inhibitor was reactivated")
	}
}

func TestNativeSleepRequestConcurrentTransitions(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("native sleep requests target Windows/macOS")
	}
	i := newNativeSleepInhibitor()
	t.Cleanup(func() { _ = i.Close() })
	var workers sync.WaitGroup
	for j := 0; j < 8; j++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := i.Acquire(); err != nil {
				t.Error(err)
			}
			if err := i.Release(); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	if err := i.Close(); err != nil || i.Active() {
		t.Fatalf("request survived final close: %v", err)
	}
}
