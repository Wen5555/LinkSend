package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

type temporaryShareFixture struct {
	a, b     *Service
	h        *httptest.Server
	source   string
	origin   string
	requests *atomic.Int32
}

func newTemporaryShareFixture(t *testing.T) temporaryShareFixture {
	t.Helper()
	control, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(t.TempDir(), "server.sqlite"), AllowInsecureLoopback: true, ExperimentalShareControl: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	requests := new(atomic.Int32)
	handler := control.Handler()
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); handler.ServeHTTP(w, r) }))
	t.Cleanup(h.Close)
	makeService := func() *Service {
		s, err := New(Config{DataDir: t.TempDir(), ServerURL: h.URL, AllowInsecureLoopback: true})
		if err != nil {
			t.Fatal(err)
		}
		client, err := signaling.New(signaling.Config{ServerURL: h.URL, Identity: s.identity, HTTPClient: h.Client()})
		if err != nil {
			t.Fatal(err)
		}
		s.signal = client
		t.Cleanup(s.Shutdown)
		return s
	}
	source := filepath.Join(t.TempDir(), "临时文件.txt")
	if err = os.WriteFile(source, []byte("one-time authenticated content"), 0600); err != nil {
		t.Fatal(err)
	}
	return temporaryShareFixture{a: makeService(), b: makeService(), h: h, source: source, origin: h.URL, requests: requests}
}

func createApprovedTemporaryShare(t *testing.T, f temporaryShareFixture) (TemporaryShareCreation, TemporaryShareSnapshot) {
	t.Helper()
	created, err := f.a.CreateTemporaryShare(t.Context(), []string{f.source}, nil)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := f.b.ClaimTemporaryShare(t.Context(), created.Code)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := f.a.RefreshTemporaryShare(t.Context(), created.Share.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.ApproveTemporaryShare(t.Context(), pending.ID, pending.ClaimID, pending.ReceiverPublicKey, pending.Generation); err != nil {
		t.Fatal(err)
	}
	approved, err := f.b.RefreshTemporaryShare(t.Context(), claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	return created, approved
}

func onlyTemporary(t *testing.T, s *Service) TemporaryShareSnapshot {
	t.Helper()
	all, err := s.TemporaryShares()
	if err != nil || len(all) != 1 {
		t.Fatalf("unexpected share snapshots count=%d err=%v", len(all), err)
	}
	return all[0]
}

func TestTemporaryShareExplicitConfirmRunsOriginalFileEngineWithoutPermanentTrust(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, claimed := createApprovedTemporaryShare(t, f)
	destination := t.TempDir()
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 8 * time.Second}
	rtask, err := f.b.StartTemporaryShare(t.Context(), claimed.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	stask, err := f.a.StartTemporaryShare(t.Context(), created.Share.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			a, _ := f.a.Task(stask.ID)
			b, _ := f.b.Task(rtask.ID)
			t.Logf("sender task=%+v receiver task=%+v", a, b)
			t.Logf("sender share=%+v receiver share=%+v", onlyTemporary(t, f.a), onlyTemporary(t, f.b))
		}
	})
	waitFor(t, 8*time.Second, func() bool { return onlyTemporary(t, f.b).State == "awaiting_save" }, "encrypted manifest preview did not await explicit save")
	if entries, err := os.ReadDir(destination); err != nil || len(entries) != 0 {
		t.Fatal("preview created receiver storage before explicit bind", err)
	}
	if onlyTemporary(t, f.b).GrantState != sharegrant.Approved {
		t.Fatal("preview implicitly bound the share")
	}
	if _, err = f.b.ConfirmTemporaryShareSave(t.Context(), claimed.ID, destination, transfer.PlanRequest{ConflictPolicy: transfer.ConflictKeepBoth}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool {
		a, _ := f.a.Task(stask.ID)
		b, _ := f.b.Task(rtask.ID)
		return a.State == "completed" && b.State == "completed"
	}, "one-time share did not finish its single original file task")
	a, _ := f.a.Task(stask.ID)
	b, _ := f.b.Task(rtask.ID)
	if a.TLSVersion != 772 || b.TLSVersion != 772 || a.ALPN != "linksend/1" || a.Relay || b.Relay || a.TransferID != created.Share.Manifest.TransferID || b.TransferID != a.TransferID || a.TemporaryShareID != created.Share.ID || b.TemporaryShareID != claimed.ID {
		t.Fatal("share did not retain exact task and authenticated transport")
	}
	want, err := os.ReadFile(f.source)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination, filepath.Base(f.source)))
	if err != nil || sha256.Sum256(want) != sha256.Sum256(got) {
		t.Fatal("received file did not match source", err)
	}
	for _, s := range []*Service{f.a, f.b} {
		peers, err := identity.LoadTrust(s.cfg.DataDir)
		if err != nil || len(peers) != 0 {
			t.Fatal("temporary share mutated permanent trust", err)
		}
		c, _ := s.client()
		if _, err = c.Devices(t.Context()); err == nil {
			t.Fatal("temporary actor gained group membership")
		}
		if len(s.directPool) != 0 {
			t.Fatal("temporary share entered the ordinary pool")
		}
	}
	t.Logf("same-IP complete two-phase requests=%d (unchanged burst=60)", f.requests.Load())
	if f.requests.Load() > 60 {
		t.Fatal("one ordinary share exceeded the unchanged request burst")
	}
}

func TestTemporaryShareLedgerNeverStoresCodeAndRejectsTamperingOrOriginSwitch(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, err := f.a.CreateTemporaryShare(t.Context(), []string{f.source}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err = f.a.store.db.QueryRow(`SELECT value FROM metadata WHERE key=?`, temporarySharePrefix+created.Share.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, created.Code) {
		t.Fatal("share plaintext entered private persistence")
	}
	if _, err = f.a.decodeTemporaryRecord(created.Share.ID, strings.Replace(raw, `"local_state":"ready"`, `"local_state":"stopped"`, 1)); err == nil {
		t.Fatal("tampered authorization row was accepted")
	}
	other, err := signaling.New(signaling.Config{ServerURL: "https://different.invalid", Identity: f.a.identity})
	if err != nil {
		t.Fatal(err)
	}
	f.a.mu.Lock()
	f.a.signal = other
	f.a.mu.Unlock()
	if _, err = f.a.RefreshTemporaryShare(t.Context(), created.Share.ID); protocol.ErrorCode(err) != protocol.ShareRejected {
		t.Fatal("server origin changed an existing share", err)
	}
}

type temporaryDropTransport struct {
	base  http.RoundTripper
	path  string
	drops atomic.Int32
}

func (d *temporaryDropTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := d.base.RoundTrip(r)
	if err == nil && r.URL.Path == d.path && d.drops.CompareAndSwap(0, 1) {
		_ = resp.Body.Close()
		return nil, io.ErrUnexpectedEOF
	}
	return resp, err
}

func TestTemporaryShareLostCreateAndClaimResponseKeepsStableIntent(t *testing.T) {
	f := newTemporaryShareFixture(t)
	transport := &temporaryDropTransport{base: f.h.Client().Transport, path: "/v1/shares/create"}
	client, _ := signaling.New(signaling.Config{ServerURL: f.origin, Identity: f.a.identity, HTTPClient: &http.Client{Transport: transport, Timeout: 5 * time.Second}})
	f.a.signal = client
	created, err := f.a.CreateTemporaryShare(t.Context(), []string{f.source}, nil)
	if err == nil || created.Code == "" || created.Share.PendingAction != signaling.ShareCreate {
		t.Fatal("response loss discarded the original create intent")
	}
	first, err := f.a.RetryTemporaryShareAction(t.Context(), created.Share.ID)
	if err != nil || first.GrantID != created.Share.ID {
		t.Fatal("create retry did not resolve the same grant", err)
	}
	transport2 := &temporaryDropTransport{base: f.h.Client().Transport, path: "/v1/shares/claim"}
	client, _ = signaling.New(signaling.Config{ServerURL: f.origin, Identity: f.b.identity, HTTPClient: &http.Client{Transport: transport2, Timeout: 5 * time.Second}})
	f.b.signal = client
	claimed, err := f.b.ClaimTemporaryShare(t.Context(), created.Code)
	if err == nil || claimed.PendingAction != signaling.ShareClaim {
		t.Fatal("claim response loss discarded stable local identity")
	}
	retried, err := f.b.RetryTemporaryShareClaim(t.Context(), claimed.ID, created.Code)
	if err != nil || retried.ID != claimed.ID || retried.GrantID != created.Share.ID {
		t.Fatal("claim retry changed local/grant identity", err)
	}
	r := temporaryRecordCopy(f.b.temporaryShares.entries[claimed.ID])
	digest, _ := sharegrant.CodeDigest(created.Code)
	if r.CodeDigest != hex.EncodeToString(digest[:]) || r.Status.ClaimID != r.ClaimID {
		t.Fatal("claim retry changed its credential verifier or claim ID")
	}
	jsonValue, _ := json.Marshal(r)
	if strings.Contains(string(jsonValue), created.Code) {
		t.Fatal("claim plaintext persisted")
	}
}

func TestTemporaryShareReceiverCancelNeverBecomesSenderRevoke(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, claimed := createApprovedTemporaryShare(t, f)
	if _, err := f.b.RevokeTemporaryShare(t.Context(), claimed.ID); protocol.ErrorCode(err) != protocol.ShareRejected {
		t.Fatal("receiver invoked sender revoke", err)
	}
	if _, err := f.b.CancelTemporaryShare(t.Context(), claimed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.StartTemporaryShare(t.Context(), claimed.ID, DirectConfig{}); err == nil {
		t.Fatal("locally cancelled receiver restarted")
	}
	sender, err := f.a.RefreshTemporaryShare(t.Context(), created.Share.ID)
	if err != nil || sender.GrantState != sharegrant.Approved {
		t.Fatal("local receiver cancel impersonated remote revoke", err)
	}
}
