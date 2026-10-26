package clipboardnativecontract

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestShellLifetimeUnknownCommitCannotBeKilledOrExpire(t *testing.T) {
	now := time.Now()
	deadline := now.Add(time.Second)
	l := NewShellLifetime(deadline)
	digest := strings.Repeat("a", 64)
	if err := l.CommitMayStart(digest, now); err == nil {
		t.Fatal("commit without durable authorization")
	}
	if err := l.Authorize(digest); err != nil {
		t.Fatal(err)
	}
	if err := l.CommitMayStart(strings.Repeat("b", 64), now); err == nil {
		t.Fatal("wrong authorization committed")
	}
	if !l.MayTerminate() {
		t.Fatal("definitely unexported preparation cannot be stopped")
	}
	if err := l.CommitMayStart(digest, now); err != nil {
		t.Fatal(err)
	}
	// Deliberately provide no ack: this is the lost-response/failed-write
	// window. A later main cancellation or admission expiry cannot kill it.
	if l.MayTerminate() {
		t.Fatal("unknown committed export can be terminated")
	}
	if _, expires := l.NativeDeadline(); expires {
		t.Fatal("Shell source got a post-export hard lifetime")
	}
	if err := l.CommitMayStart(digest, deadline.Add(time.Hour)); err == nil {
		t.Fatal("old publication reactivated")
	}
	if l.MayTerminate() {
		t.Fatal("rejected retry cleared sticky export protection")
	}
	if err := l.AcknowledgeGeneration(19); err != nil {
		t.Fatal(err)
	}
	if err := l.AcknowledgeGeneration(20); err == nil {
		t.Fatal("watcher/later copy replaced original publication ack")
	}
}

func TestPublicationAdmissionExpiryNeverCreatesExposure(t *testing.T) {
	deadline := time.Now()
	l := NewShellLifetime(deadline)
	digest := strings.Repeat("a", 64)
	if err := l.Authorize(digest); err != nil {
		t.Fatal(err)
	}
	if err := l.CommitMayStart(digest, deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expired admission", err)
	}
	if !l.MayTerminate() {
		t.Fatal("rejected pre-export admission became an active producer")
	}
	probe := NewControlledLifetime(deadline)
	if err := probe.Authorize(digest); err != nil {
		t.Fatal(err)
	}
	if err := probe.CommitMayStart(digest, deadline.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if got, expires := probe.NativeDeadline(); !expires || !got.Equal(deadline) {
		t.Fatal("controlled probe lost its original fixed deadline")
	}
}
