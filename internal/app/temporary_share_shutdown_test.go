package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func TestTemporaryShareShutdownCancelsSynchronousPreparation(t *testing.T) {
	f := newTemporaryShareFixture(t)
	entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.a.temporaryShares.beforePrepare = func(ctx context.Context) {
		close(entered)
		select {
		case <-ctx.Done():
			close(cancelled)
		case <-release:
		}
	}
	caller, cancelCaller := context.WithCancel(context.Background())
	t.Cleanup(cancelCaller)
	created := make(chan error, 1)
	go func() { _, err := f.a.CreateTemporaryShare(caller, []string{f.source}, nil); created <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("create did not enter synchronous preparation")
	}
	shutdown := make(chan error, 1)
	go func() { shutdown <- f.a.ShutdownContext(context.Background()) }()
	select {
	case <-f.a.workCtx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("profile shutdown did not publish cancellation")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Error("profile shutdown did not cancel the actual preparation context")
		unblock()
		cancelCaller()
	}
	if caller.Err() != nil && !t.Failed() {
		t.Fatal("profile shutdown cancelled the caller's separate runtime context")
	}
	select {
	case err := <-created:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("synchronous create lost cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("synchronous create did not join after profile cancellation")
	}
	select {
	case err := <-shutdown:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown retained the synchronous create worker")
	}
	if f.requests.Load() != 0 {
		t.Fatal("cancelled preparation sent an HTTP operation")
	}
}

type temporaryBlockedHTTP struct {
	entered, cancelled, release chan struct{}
	once                        sync.Once
}

func (b *temporaryBlockedHTTP) RoundTrip(r *http.Request) (*http.Response, error) {
	b.once.Do(func() { close(b.entered) })
	select {
	case <-r.Context().Done():
		close(b.cancelled)
		return nil, r.Context().Err()
	case <-b.release:
		return nil, io.ErrUnexpectedEOF
	}
}

func TestTemporaryShareShutdownCancelsSynchronousHTTP(t *testing.T) {
	for _, operation := range []string{"create", "claim", "refresh", "retry_intent"} {
		t.Run(operation, func(t *testing.T) {
			f := newTemporaryShareFixture(t)
			var id string
			if operation == "refresh" || operation == "retry_intent" {
				created, err := f.a.CreateTemporaryShare(t.Context(), []string{f.source}, nil)
				if err != nil {
					t.Fatal(err)
				}
				id = created.Share.ID
				if operation == "retry_intent" {
					e, _ := f.a.temporaryEntry(id)
					e.mu.Lock()
					r := cloneTemporaryRecord(e.record)
					r.Intent = temporaryIntent(signaling.ShareCreate, signaling.ShareRequest{ID: r.ID, CodeDigest: r.CodeDigest, Manifest: r.Manifest.Digest()})
					if err = f.a.saveTemporaryRecord(e, r); err != nil {
						t.Fatal(err)
					}
					e.mu.Unlock()
				}
			}
			blocked := &temporaryBlockedHTTP{entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
			var once sync.Once
			unblock := func() { once.Do(func() { close(blocked.release) }) }
			t.Cleanup(unblock)
			client, err := signaling.New(signaling.Config{ServerURL: f.origin, Identity: f.a.identity, HTTPClient: &http.Client{Transport: blocked}})
			if err != nil {
				t.Fatal(err)
			}
			f.a.signal = client
			caller, cancelCaller := context.WithCancel(context.Background())
			t.Cleanup(cancelCaller)
			finished := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "create":
					_, err = f.a.CreateTemporaryShare(caller, []string{f.source}, nil)
				case "claim":
					code, _, _ := sharegrant.GenerateCode()
					_, err = f.a.ClaimTemporaryShare(caller, code)
				case "refresh":
					_, err = f.a.RefreshTemporaryShare(caller, id)
				case "retry_intent":
					_, err = f.a.RetryTemporaryShareAction(caller, id)
				}
				finished <- err
			}()
			select {
			case <-blocked.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("operation did not reach blocked HTTP boundary")
			}
			shutdown := make(chan error, 1)
			go func() { shutdown <- f.a.ShutdownContext(context.Background()) }()
			select {
			case <-f.a.workCtx.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("profile shutdown did not publish cancellation")
			}
			select {
			case <-blocked.cancelled:
			case <-time.After(time.Second):
				t.Error("profile shutdown left synchronous HTTP alive")
				unblock()
				cancelCaller()
			}
			if caller.Err() != nil && !t.Failed() {
				t.Fatal("profile shutdown cancelled the independent runtime caller context")
			}
			select {
			case err := <-finished:
				if !errors.Is(err, context.Canceled) {
					t.Fatal("HTTP operation did not preserve cancellation", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("HTTP operation did not join")
			}
			select {
			case err := <-shutdown:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("profile owner did not join synchronous HTTP")
			}
		})
	}
}
