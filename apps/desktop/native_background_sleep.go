package main

import (
	"errors"
	"sync"
)

// nativeSleepInhibitor is a boolean ownership lease, not a reference counter.
// Repeated active-task snapshots may Acquire; only actual active transfers
// should hold it. Release on pause, terminal state, shutdown and before sleep.
type nativeSleepInhibitor struct {
	mu      sync.Mutex
	closed  bool
	release func() error
}

func newNativeSleepInhibitor() *nativeSleepInhibitor { return &nativeSleepInhibitor{} }

func (i *nativeSleepInhibitor) Acquire() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return errors.New("SLEEP_INHIBITOR_CLOSED")
	}
	if i.release != nil {
		return nil
	}
	release, err := acquireNativeSleepRequest()
	if err != nil {
		return err
	}
	i.release = release
	return nil
}

func (i *nativeSleepInhibitor) releaseLocked() error {
	if i.release == nil {
		return nil
	}
	if err := i.release(); err != nil {
		return err // Retain the handle so a later Close/Release can retry.
	}
	i.release = nil
	return nil
}

func (i *nativeSleepInhibitor) Release() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.releaseLocked()
}

func (i *nativeSleepInhibitor) Active() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.release != nil
}

func (i *nativeSleepInhibitor) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.closed = true
	return i.releaseLocked()
}
