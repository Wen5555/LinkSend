package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

type shareDirectFixture struct {
	a, b     *Service
	ca, cb   *signaling.Client
	approved signaling.ShareStatus
	prepared *transfer.Prepared
	scope    sharegrant.SignalScope
	ctx      context.Context
}

func newShareDirectFixture(t *testing.T) shareDirectFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	t.Cleanup(cancel)
	srv, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(t.TempDir(), "server.sqlite"), AllowInsecureLoopback: true, AllowLoopbackCandidates: true, ExperimentalShareControl: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	httpServer := httptest.NewServer(srv.Handler())
	t.Cleanup(httpServer.Close)
	makeService := func() *Service {
		t.Helper()
		s, e := New(Config{DataDir: t.TempDir(), ServerURL: httpServer.URL, AllowInsecureLoopback: true})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(s.Shutdown)
		return s
	}
	a, b := makeService(), makeService()
	ca, _ := a.client()
	cb, _ := b.client()
	source := filepath.Join(t.TempDir(), "中文 目录")
	if err = os.MkdirAll(filepath.Join(source, "空目录"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "源 文件.txt"), []byte("body stays outside signaling and preview"), 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := transfer.Prepare(ctx, []string{source}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close() })
	code, digest, err := sharegrant.GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	offer, err := ca.ShareControl(ctx, signaling.ShareCreate, signaling.ShareRequest{ID: protocol.RandomID(), Manifest: prepared.Manifest.Digest(), CodeDigest: hex.EncodeToString(digest[:])})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := cb.ShareControl(ctx, signaling.ShareClaim, signaling.ShareRequest{Code: code, ClaimID: protocol.RandomID()})
	if err != nil || claim.ID != offer.ID {
		t.Fatal("isolated share claim failed", err)
	}
	approved, err := ca.ShareControl(ctx, signaling.ShareApprove, signaling.ShareRequest{ID: claim.ID, Manifest: claim.Manifest, ClaimID: claim.ClaimID, ReceiverKey: claim.ReceiverKey, TaskID: prepared.Manifest.TransferID, ApprovalID: protocol.RandomID(), Generation: claim.Generation})
	if err != nil {
		t.Fatal(err)
	}
	scope := sharegrant.SignalScope{GrantID: approved.ID, ClaimID: approved.ClaimID, TaskID: approved.TaskID, GrantGeneration: approved.Generation, Phase: sharegrant.SignalPreview, Manifest: approved.Manifest}
	return shareDirectFixture{a: a, b: b, ca: ca, cb: cb, approved: approved, prepared: prepared, scope: scope, ctx: ctx}
}

func (f shareDirectFixture) connect(t *testing.T) (*PeerSession, *PeerSession) {
	t.Helper()
	type result struct {
		peer *PeerSession
		err  error
	}
	responder := make(chan result, 1)
	cfg := DirectConfig{BindAddress: "127.0.0.1:0", AllowLoopback: true, CheckTimeout: 8 * time.Second, WaitTimeout: 8 * time.Second}
	go func() {
		peer, err := f.b.connectSharePeer(f.ctx, f.scope, f.approved, cfg, func() error { return nil })
		responder <- result{peer, err}
	}()
	a, err := f.a.connectSharePeer(f.ctx, f.scope, f.approved, cfg, func() error { return nil })
	b := <-responder
	if a != nil {
		t.Cleanup(func() { _ = a.Close() })
	}
	if b.peer != nil {
		t.Cleanup(func() { _ = b.peer.Close() })
	}
	if err != nil || b.err != nil {
		t.Fatal("temporary authenticated ICE/QUIC failed", err, b.err)
	}
	return a, b.peer
}

func TestShareDirectUsesPinnedQUICWithoutMembershipOrClipboard(t *testing.T) {
	f := newShareDirectFixture(t)
	a, b := f.connect(t)
	for _, peer := range []*PeerSession{a, b} {
		e := peer.Evidence()
		if peer.SessionReuse || peer.ClipboardSync || peer.ClipboardFiles || peer.AuthorizationGeneration != 0 || e.TLSVersion != 772 || e.ALPN != identity.ALPN || e.Relay {
			t.Fatal("temporary session acquired unrelated rights", e)
		}
	}
	for _, svc := range []*Service{f.a, f.b} {
		trust, err := identity.LoadTrust(svc.cfg.DataDir)
		if err != nil || len(trust) != 0 {
			t.Fatal("temporary connection wrote permanent trust", err)
		}
		svc.directPoolMu.Lock()
		n := len(svc.directPool)
		svc.directPoolMu.Unlock()
		if n != 0 {
			t.Fatal("temporary session entered normal connection pool")
		}
	}
	if _, err := f.ca.Devices(f.ctx); protocol.ErrorCode(err) != protocol.AuthenticationFailed {
		t.Fatal("temporary sender became a member", err)
	}
}

func TestSharePreviewBindsConfirmedPlanBeforeAnyFileStorage(t *testing.T) {
	f := newShareDirectFixture(t)
	a, b := f.connect(t)
	destination := t.TempDir()
	received := make(chan error, 1)
	var planDigest string
	go func() {
		_, plan, err := receiveSharePreview(f.ctx, b, f.scope, func(ctx context.Context, m transfer.Manifest) (transfer.ReceivePlan, error) {
			entries, readErr := os.ReadDir(destination)
			if readErr != nil || len(entries) != 0 {
				return transfer.ReceivePlan{}, errors.New("storage existed before explicit save confirmation")
			}
			plan, buildErr := transfer.BuildReceivePlan(ctx, destination, m, transfer.PlanRequest{ConflictPolicy: transfer.ConflictKeepBoth})
			if buildErr != nil {
				return plan, buildErr
			}
			_, bindErr := f.cb.ShareControl(ctx, signaling.ShareBind, signaling.ShareRequest{ID: f.scope.GrantID, TaskID: f.scope.TaskID, Generation: f.scope.GrantGeneration, Manifest: f.scope.Manifest, Plan: plan.Digest()})
			return plan, bindErr
		})
		if err == nil {
			planDigest = plan.Digest()
		}
		received <- err
	}()
	got, err := sendSharePreview(f.ctx, a, f.scope, f.prepared.Manifest)
	receiveErr := <-received
	if err != nil || receiveErr != nil || got != planDigest || !previewDigest(got) {
		t.Fatal("encrypted metadata confirmation failed", err, receiveErr)
	}
	entries, err := os.ReadDir(destination)
	if err != nil || len(entries) != 0 {
		t.Fatal("preview opened file receiver storage", err)
	}
}

func TestShareDirectRequiresPositiveOwnerAuthorization(t *testing.T) {
	f := newShareDirectFixture(t)
	denied := errors.New("local owner cancelled")
	if peer, err := f.a.connectSharePeer(f.ctx, f.scope, f.approved, DirectConfig{}, func() error { return denied }); !errors.Is(err, denied) || peer != nil {
		t.Fatal("cancelled owner entered a session", err)
	}
	if _, err := f.a.connectSharePeer(f.ctx, f.scope, f.approved, DirectConfig{}, nil); protocol.ErrorCode(err) != protocol.ShareRejected {
		t.Fatal("missing authorization owner accepted", err)
	}
}

func TestSharePreviewFramingRejectsAmbiguityAndUnboundedMetadata(t *testing.T) {
	scope := sharegrant.SignalScope{GrantID: strings.Repeat("1", 32), ClaimID: strings.Repeat("2", 32), TaskID: strings.Repeat("3", 32), GrantGeneration: 1, Phase: sharegrant.SignalPreview, Manifest: strings.Repeat("a", 64)}
	valid, _ := json.Marshal(sharePreviewFrame{Version: 1, Type: "accepted", Scope: scope, PlanDigest: strings.Repeat("b", 64)})
	for name, raw := range map[string][]byte{
		"duplicate version": append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"Version":1}`)...),
		"unknown body":      append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"file_body":"private"}`)...),
		"extra document":    append(append([]byte(nil), valid...), []byte(` {}`)...),
		"null":              []byte(`null`),
	} {
		t.Run(name, func(t *testing.T) {
			var wire bytes.Buffer
			wire.WriteString(sharePreviewMagic)
			_ = binary.Write(&wire, binary.BigEndian, uint32(len(raw)))
			wire.Write(raw)
			if _, err := readSharePreviewFrame(&wire, 4096); err == nil {
				t.Fatal("ambiguous preview admitted")
			}
		})
	}
	var wire bytes.Buffer
	wire.WriteString(sharePreviewMagic)
	_ = binary.Write(&wire, binary.BigEndian, uint32(maxSharePreview+1))
	if _, err := readSharePreviewFrame(&wire, maxSharePreview); err == nil {
		t.Fatal("oversized declaration allocated a preview body")
	}
}
