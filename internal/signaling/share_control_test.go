package signaling_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func shareClientIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newShareClient(t *testing.T, h *httptest.Server, id *identity.Identity, httpClient *http.Client) *signaling.Client {
	t.Helper()
	if httpClient == nil {
		httpClient = h.Client()
	}
	c, err := signaling.New(signaling.Config{ServerURL: h.URL, Identity: id, HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func experimentalShareServer(t *testing.T, database string) *server.Server {
	t.Helper()
	s, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: database, AllowInsecureLoopback: true, ExperimentalShareControl: true})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func shareControlCall(t *testing.T, c *signaling.Client, op signaling.ShareOperation, request signaling.ShareRequest) signaling.ShareStatus {
	t.Helper()
	s, err := c.ShareControl(t.Context(), op, request)
	if err != nil {
		t.Fatalf("%s failed: %v", op, err)
	}
	return s
}

func createClientShare(t *testing.T, c *signaling.Client) (signaling.ShareRequest, signaling.ShareStatus, string) {
	t.Helper()
	code, digest, err := sharegrant.GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	r := signaling.ShareRequest{ID: protocol.RandomID(), CodeDigest: hex.EncodeToString(digest[:]), Manifest: strings.Repeat("a", 64)}
	return r, shareControlCall(t, c, signaling.ShareCreate, r), code
}

func TestShareControlClientRealSignedLifecycleWithoutMembership(t *testing.T) {
	s := experimentalShareServer(t, filepath.Join(t.TempDir(), "control.sqlite"))
	defer s.Close()
	h := httptest.NewTLSServer(s.Handler())
	defer h.Close()
	sender, receiver, outsider := shareClientIdentity(t), shareClientIdentity(t), shareClientIdentity(t)
	sc, rc, oc := newShareClient(t, h, sender, nil), newShareClient(t, h, receiver, nil), newShareClient(t, h, outsider, nil)
	info, err := sc.ShareControlInfo(t.Context())
	if err != nil || info.Version != 1 || len(info.Epoch) != 32 || !info.FileTransferReady {
		t.Fatalf("control info=%+v err=%v", info, err)
	}
	creation, offered, code := createClientShare(t, sc)
	claimRequest := signaling.ShareRequest{Code: code, ClaimID: protocol.RandomID(), ID: offered.ID, Manifest: offered.Manifest}
	pending := shareControlCall(t, rc, signaling.ShareClaim, claimRequest)
	if pending.State != sharegrant.Pending || pending.ReceiverKey != base64.RawStdEncoding.EncodeToString(receiver.PublicKey()) {
		t.Fatal("claim response did not bind the real receiver")
	}
	approvalRequest := signaling.ShareRequest{ID: pending.ID, ClaimID: pending.ClaimID, ApprovalID: protocol.RandomID(), TaskID: protocol.RandomID(), Generation: pending.Generation, Manifest: pending.Manifest, ReceiverKey: pending.ReceiverKey}
	approved := shareControlCall(t, sc, signaling.ShareApprove, approvalRequest)
	if approved.State != sharegrant.Approved {
		t.Fatal("approval failed")
	}
	if got := shareControlCall(t, rc, signaling.ShareClaim, claimRequest); got.State != sharegrant.Approved {
		t.Fatal("legal claim retry after approval rejected")
	}
	binding := signaling.ShareRequest{ID: approved.ID, TaskID: approved.TaskID, Manifest: approved.Manifest, Plan: strings.Repeat("b", 64), Generation: approved.Generation}
	bound := shareControlCall(t, rc, signaling.ShareBind, binding)
	if bound.State != sharegrant.Bound {
		t.Fatal("save-plan binding failed")
	}
	if got := shareControlCall(t, sc, signaling.ShareAuthorize, binding); got.ResumeUntil != bound.ResumeUntil {
		t.Fatal("authorization changed original deadline")
	}
	completed := shareControlCall(t, rc, signaling.ShareComplete, binding)
	if completed.State != sharegrant.Completed {
		t.Fatal("completion failed")
	}
	for _, call := range []struct {
		client *signaling.Client
		op     signaling.ShareOperation
		req    signaling.ShareRequest
	}{{sc, signaling.ShareComplete, binding}, {rc, signaling.ShareBind, binding}, {rc, signaling.ShareClaim, claimRequest}, {sc, signaling.ShareApprove, approvalRequest}, {sc, signaling.ShareCreate, creation}} {
		if got := shareControlCall(t, call.client, call.op, call.req); got.State != sharegrant.Completed || got.ResumeUntil != bound.ResumeUntil || got.ExpiresAt != offered.ExpiresAt {
			t.Fatalf("%s retry changed terminal authorization", call.op)
		}
	}
	if _, err := sc.ShareControl(t.Context(), signaling.ShareAuthorize, binding); protocol.ErrorCode(err) != protocol.ShareTerminal {
		t.Fatalf("completed task reopened: %v", err)
	}
	if _, err := oc.ShareControl(t.Context(), signaling.ShareRead, signaling.ShareRequest{ID: offered.ID}); protocol.ErrorCode(err) != protocol.ShareRejected {
		t.Fatalf("unrelated actor read a grant: %v", err)
	}
	for _, c := range []*signaling.Client{sc, rc, oc} {
		if _, err := c.Devices(t.Context()); err == nil {
			t.Fatal("temporary actor gained group enumeration")
		}
		ws, err := c.Connect(t.Context())
		if ws != nil {
			ws.Close()
			t.Fatal("temporary actor gained ordinary signaling")
		}
		if err == nil {
			t.Fatal("unregistered websocket authentication succeeded")
		}
	}
}

func TestShareControlClientDenyRevokeAndConcurrentFreshProofs(t *testing.T) {
	s := experimentalShareServer(t, filepath.Join(t.TempDir(), "control.sqlite"))
	defer s.Close()
	h := httptest.NewTLSServer(s.Handler())
	defer h.Close()
	sc := newShareClient(t, h, shareClientIdentity(t), nil)
	rc := newShareClient(t, h, shareClientIdentity(t), nil)
	_, offer, code := createClientShare(t, sc)
	claim := shareControlCall(t, rc, signaling.ShareClaim, signaling.ShareRequest{Code: code, ClaimID: protocol.RandomID()})
	denial := signaling.ShareRequest{ID: claim.ID, ClaimID: claim.ClaimID, Generation: claim.Generation}
	for range 2 {
		if got := shareControlCall(t, sc, signaling.ShareDeny, denial); got.State != sharegrant.Offered || got.Generation != claim.Generation+1 {
			t.Fatal("denial did not retire the exact pending claim")
		}
	}
	var wg sync.WaitGroup
	errorsOut := make(chan error, 6)
	for range 6 {
		wg.Go(func() {
			_, err := sc.ShareControl(t.Context(), signaling.ShareRead, signaling.ShareRequest{ID: offer.ID, Manifest: offer.Manifest, Generation: claim.Generation + 1})
			errorsOut <- err
		})
	}
	wg.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil {
			t.Fatalf("concurrent request reused proof or state: %v", err)
		}
	}
	for range 2 {
		if got := shareControlCall(t, sc, signaling.ShareRevoke, signaling.ShareRequest{ID: offer.ID, Manifest: offer.Manifest}); got.State != sharegrant.Revoked {
			t.Fatal("revoke retry reopened an offer")
		}
	}
}

func controlHealth(w http.ResponseWriter, epoch string, control any) {
	if control == nil {
		control = signaling.ShareControlInfo{Version: 1, Epoch: epoch, FileTransferReady: true}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "capabilities": readyShareCapabilities(), "share_control": control})
}

func readyShareCapabilities() protocol.Capabilities {
	caps := protocol.Supported()
	caps.ShareGrantsV1 = true
	return caps
}

func TestShareCapabilityGateRejectsControlOnlyOrOldServerBeforeSecretOrWS(t *testing.T) {
	ready, _ := json.Marshal(readyShareCapabilities())
	old, _ := json.Marshal(protocol.Supported())
	for name, tc := range map[string]struct {
		caps, control string
	}{
		"ordinary service":      {string(old), "null"},
		"old control prototype": {string(old), `{"version":1,"epoch":"` + strings.Repeat("4", 32) + `","file_transfer_ready":false}`},
		"missing capability":    {string(old), `{"version":1,"epoch":"` + strings.Repeat("4", 32) + `","file_transfer_ready":true}`},
		"not ready":             {string(ready), `{"version":1,"epoch":"` + strings.Repeat("4", 32) + `","file_transfer_ready":false}`},
		"aliased capability":    {strings.Replace(string(ready), "share_grants_v1", "Share_Grants_V1", 1), `{"version":1,"epoch":"` + strings.Repeat("4", 32) + `","file_transfer_ready":true}`},
	} {
		t.Run(name, func(t *testing.T) {
			var actions atomic.Int32
			h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/healthz" {
					actions.Add(1)
				}
				_, _ = io.WriteString(w, `{"status":"ok","capabilities":`+tc.caps+`,"share_control":`+tc.control+`}`)
			}))
			defer h.Close()
			local, peer := shareClientIdentity(t), shareClientIdentity(t)
			c := newShareClient(t, h, local, nil)
			if _, err := c.ShareControl(t.Context(), signaling.ShareClaim, signaling.ShareRequest{Code: strings.Repeat("A", 32), ClaimID: protocol.RandomID()}); protocol.ErrorCode(err) != protocol.ShareUnsupported {
				t.Fatalf("old/control-only service accepted a secret: %v", err)
			}
			pinned := boundClientStatus(peer, local)
			scope := sharegrant.SignalScope{GrantID: pinned.ID, ClaimID: pinned.ClaimID, TaskID: pinned.TaskID, GrantGeneration: pinned.Generation, Manifest: pinned.Manifest, Plan: pinned.Plan, Phase: sharegrant.SignalTransfer}
			if session, err := c.ConnectShare(t.Context(), scope, pinned, signaling.ShareSignalOptions{}); protocol.ErrorCode(err) != protocol.ShareUnsupported || session != nil {
				if session != nil {
					session.Close()
				}
				t.Fatalf("old/control-only service opened a scoped connection: %v", err)
			}
			if actions.Load() != 0 {
				t.Fatal("capability failure sent a mutation/status or opened a websocket")
			}
		})
	}
}

func TestShareControlClientRejectsOldOrMalformedHealthBeforeSendingCode(t *testing.T) {
	for name, control := range map[string]any{
		"missing":               json.RawMessage("null"),
		"version":               signaling.ShareControlInfo{Version: 2, Epoch: protocol.RandomID()},
		"missing epoch":         map[string]any{"version": 1, "file_transfer_ready": false},
		"missing readiness":     map[string]any{"version": 1, "epoch": protocol.RandomID()},
		"unknown control field": map[string]any{"version": 1, "epoch": protocol.RandomID(), "file_transfer_ready": false, "unexpected": true},
		"case-folded epoch":     json.RawMessage(`{"version":1,"epoch":"` + strings.Repeat("4", 32) + `","Epoch":"` + strings.Repeat("4", 32) + `","file_transfer_ready":false}`),
		"long s readiness":      json.RawMessage(`{"version":1,"epoch":"` + strings.Repeat("4", 32) + `","file_transfer_ready":false,"file_tranſfer_ready":false}`),
	} {
		t.Run(name, func(t *testing.T) {
			var posts atomic.Int32
			h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
				}
				controlHealth(w, "", control)
			}))
			defer h.Close()
			c := newShareClient(t, h, shareClientIdentity(t), nil)
			_, err := c.ShareControl(t.Context(), signaling.ShareClaim, signaling.ShareRequest{Code: strings.Repeat("A", 32), ClaimID: protocol.RandomID()})
			if err == nil || posts.Load() != 0 {
				t.Fatalf("invalid health sent credential or was accepted: posts=%d err=%v", posts.Load(), err)
			}
		})
	}
}

func TestShareControlClientRejectsAmbiguousHealth(t *testing.T) {
	caps, _ := json.Marshal(readyShareCapabilities())
	control, _ := json.Marshal(signaling.ShareControlInfo{Version: 1, Epoch: protocol.RandomID(), FileTransferReady: true})
	base := `"status":"ok","capabilities":` + string(caps) + `,"share_control":` + string(control)
	for _, duplicate := range []string{
		`"status":"ok"`, `"Status":"ok"`, `"ſtatus":"ok"`,
		`"capabilities":` + string(caps), `"Capabilities":` + string(caps),
		`"share_control":` + string(control), `"Share_Control":` + string(control), `"ſhare_control":` + string(control),
	} {
		t.Run(strings.SplitN(duplicate, ":", 2)[0], func(t *testing.T) {
			var posts atomic.Int32
			h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
				}
				_, _ = io.WriteString(w, `{`+base+`,`+duplicate+`}`)
			}))
			defer h.Close()
			c := newShareClient(t, h, shareClientIdentity(t), nil)
			if _, err := c.ShareControl(t.Context(), signaling.ShareClaim, signaling.ShareRequest{Code: strings.Repeat("A", 32), ClaimID: protocol.RandomID()}); protocol.ErrorCode(err) != protocol.InvalidMessage || posts.Load() != 0 {
				t.Fatalf("ambiguous health sent a credential: posts=%d err=%v", posts.Load(), err)
			}
		})
	}
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{`+base+`,"future_health_metadata":{"bounded":true}}`)
	}))
	defer h.Close()
	if _, err := newShareClient(t, h, shareClientIdentity(t), nil).ShareControlInfo(t.Context()); err != nil {
		t.Fatalf("ordinary unknown health metadata broke compatibility: %v", err)
	}
}

func TestShareControlClientRejectsAmbiguousCapabilities(t *testing.T) {
	caps, _ := json.Marshal(readyShareCapabilities())
	control, _ := json.Marshal(signaling.ShareControlInfo{Version: 1, Epoch: protocol.RandomID(), FileTransferReady: true})
	for name, duplicate := range map[string]string{
		"relay case":      `"Relay":false`,
		"protocol long s": `"protocol_verſion":1`,
		"membership case": `"Membership_Version":2`,
	} {
		t.Run(name, func(t *testing.T) {
			var posts atomic.Int32
			h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
				}
				_, _ = io.WriteString(w, `{"status":"ok","capabilities":`+string(caps[:len(caps)-1])+`,`+duplicate+`},"share_control":`+string(control)+`}`)
			}))
			defer h.Close()
			c := newShareClient(t, h, shareClientIdentity(t), nil)
			if _, err := c.ShareControl(t.Context(), signaling.ShareClaim, signaling.ShareRequest{Code: strings.Repeat("A", 32), ClaimID: protocol.RandomID()}); protocol.ErrorCode(err) != protocol.InvalidMessage || posts.Load() != 0 {
				t.Fatalf("ambiguous capabilities sent a credential: posts=%d err=%v", posts.Load(), err)
			}
		})
	}
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"ok","capabilities":`+string(caps[:len(caps)-1])+`,"future_capability":{"bounded":true}},"share_control":`+string(control)+`}`)
	}))
	defer h.Close()
	if _, err := newShareClient(t, h, shareClientIdentity(t), nil).ShareControlInfo(t.Context()); err != nil {
		t.Fatalf("ordinary unknown capability metadata broke compatibility: %v", err)
	}
}

func boundClientStatus(sender, receiver *identity.Identity) signaling.ShareStatus {
	now := time.Now().Unix()
	return signaling.ShareStatus{ID: strings.Repeat("1", 32), State: sharegrant.Bound, Generation: 2, Revision: 4, SenderKey: base64.RawStdEncoding.EncodeToString(sender.PublicKey()), ReceiverKey: base64.RawStdEncoding.EncodeToString(receiver.PublicKey()), Manifest: strings.Repeat("a", 64), Plan: strings.Repeat("b", 64), ClaimID: strings.Repeat("2", 32), TaskID: strings.Repeat("3", 32), ExpiresAt: now + 600, ClaimUntil: now + 120, ResumeUntil: now + 86400}
}

func TestShareControlClientRejectsInvalidStatusAndResponseBinding(t *testing.T) {
	sender, receiver, stranger := shareClientIdentity(t), shareClientIdentity(t), shareClientIdentity(t)
	base := boundClientStatus(sender, receiver)
	req := signaling.ShareRequest{ID: base.ID, Manifest: base.Manifest, Plan: base.Plan, ClaimID: base.ClaimID, TaskID: base.TaskID, Generation: base.Generation, ReceiverKey: base.ReceiverKey}
	for name, change := range map[string]func(*signaling.ShareStatus){
		"grant": func(s *signaling.ShareStatus) { s.ID = strings.Repeat("9", 32) },
		"sender": func(s *signaling.ShareStatus) {
			s.SenderKey = base64.RawStdEncoding.EncodeToString(stranger.PublicKey())
		},
		"receiver": func(s *signaling.ShareStatus) {
			s.ReceiverKey = base64.RawStdEncoding.EncodeToString(stranger.PublicKey())
		},
		"manifest":               func(s *signaling.ShareStatus) { s.Manifest = strings.Repeat("c", 64) },
		"plan":                   func(s *signaling.ShareStatus) { s.Plan = strings.Repeat("c", 64) },
		"task":                   func(s *signaling.ShareStatus) { s.TaskID = strings.Repeat("9", 32) },
		"claim":                  func(s *signaling.ShareStatus) { s.ClaimID = strings.Repeat("9", 32) },
		"generation":             func(s *signaling.ShareStatus) { s.Generation++ },
		"unknown state":          func(s *signaling.ShareStatus) { s.State = "ready" },
		"zero revision":          func(s *signaling.ShareStatus) { s.Revision = 0 },
		"counter overflow":       func(s *signaling.ShareStatus) { s.Generation = 1 << 63 },
		"long actor":             func(s *signaling.ShareStatus) { s.SenderKey = strings.Repeat("A", 8192) },
		"wrong lifetime":         func(s *signaling.ShareStatus) { s.ResumeUntil = s.ExpiresAt + 86401 },
		"missing binding":        func(s *signaling.ShareStatus) { s.Plan = ""; s.ResumeUntil = 0 },
		"unknown fields":         nil,
		"duplicate fields":       nil,
		"case-folded generation": nil,
		"long s state":           nil,
		"long s sender key":      nil,
		"multiple documents":     nil,
		"oversized body":         nil,
		"null body":              nil,
	} {
		t.Run(name, func(t *testing.T) {
			got := base
			if change != nil {
				change(&got)
			}
			raw, _ := json.Marshal(got)
			switch name {
			case "unknown fields":
				raw = append(raw[:len(raw)-1], []byte(`,"unexpected":true}`)...)
			case "duplicate fields":
				raw = append(raw[:len(raw)-1], []byte(`,"generation":2}`)...)
			case "case-folded generation":
				raw = append(raw[:len(raw)-1], []byte(`,"Generation":2}`)...)
			case "long s state":
				raw = append(raw[:len(raw)-1], []byte(`,"ſtate":"bound"}`)...)
			case "long s sender key":
				raw = append(raw[:len(raw)-1], []byte(`,"ſender_public_key":"`+base.SenderKey+`"}`)...)
			case "multiple documents":
				raw = append(raw, []byte(` {}`)...)
			case "oversized body":
				raw = []byte(`{"private":"` + strings.Repeat("x", protocol.MaxPayloadBytes) + `"}`)
			case "null body":
				raw = []byte("null")
			}
			h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/healthz" {
					controlHealth(w, strings.Repeat("4", 32), nil)
					return
				}
				_, _ = w.Write(raw)
			}))
			defer h.Close()
			c := newShareClient(t, h, sender, nil)
			status, err := c.ShareControl(t.Context(), signaling.ShareRead, req)
			if protocol.ErrorCode(err) != protocol.InvalidMessage || status.ID != "" {
				t.Fatalf("bad status accepted or returned: result=%+v err=%v", status, err)
			}
		})
	}
}

func TestShareControlClientSanitizesFailuresAndDoesNotRetryOrRedirect(t *testing.T) {
	const secret = "PRIVATE_SHARE_CODE_AND_SERVER_DETAIL"
	for name, tc := range map[string]struct {
		status int
		body   string
		code   protocol.Code
	}{
		"known":           {403, `{"code":"SHARE_REJECTED","detail":"` + secret + `"}`, protocol.ShareRejected},
		"unknown code":    {400, `{"code":"` + secret + `","detail":"private"}`, protocol.InvalidMessage},
		"gateway":         {504, `<html>` + secret + `</html>`, protocol.SignalingTimeout},
		"oversized error": {503, `{"code":"SHARE_REJECTED","detail":"` + strings.Repeat(secret, 2000) + `"}`, protocol.SignalingUnreachable},
		"redirect":        {307, `{}`, protocol.InvalidMessage},
	} {
		t.Run(name, func(t *testing.T) {
			var posts, redirected atomic.Int32
			target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
			defer target.Close()
			h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/healthz" {
					controlHealth(w, protocol.RandomID(), nil)
					return
				}
				posts.Add(1)
				w.Header().Set("Location", target.URL)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer h.Close()
			c := newShareClient(t, h, shareClientIdentity(t), nil)
			_, err := c.ShareControl(t.Context(), signaling.ShareClaim, signaling.ShareRequest{Code: strings.Repeat("A", 32), ClaimID: protocol.RandomID()})
			if protocol.ErrorCode(err) != tc.code || strings.Contains(err.Error(), secret) || posts.Load() != 1 || redirected.Load() != 0 {
				t.Fatalf("failure escaped bounds: code=%s posts=%d redirect=%d err=%v", protocol.ErrorCode(err), posts.Load(), redirected.Load(), err)
			}
		})
	}
}

func TestShareControlClientCancellationDoesNotRepeatMutation(t *testing.T) {
	for _, stage := range []string{"health", "post"} {
		t.Run(stage, func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			var posts atomic.Int32
			h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/healthz" && stage != "health" {
					controlHealth(w, protocol.RandomID(), nil)
					return
				}
				if r.Method == http.MethodPost {
					posts.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
				}
				close(started)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer h.Close()
			defer close(release)
			c := newShareClient(t, h, shareClientIdentity(t), nil)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := c.ShareControl(ctx, signaling.ShareClaim, signaling.ShareRequest{Code: strings.Repeat("A", 32), ClaimID: protocol.RandomID()})
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not reach controlled boundary")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation cause lost: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancellation did not return")
			}
			if posts.Load() > 1 || stage == "health" && posts.Load() != 0 {
				t.Fatal("cancellation retried or sent mutation before health")
			}
		})
	}
}

type shareCaptureTransport struct {
	base http.RoundTripper
	mu   sync.Mutex
	req  *http.Request
	body []byte
}

func (t *shareCaptureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		t.mu.Lock()
		t.req, t.body = r.Clone(context.Background()), append([]byte(nil), body...)
		t.mu.Unlock()
	}
	return t.base.RoundTrip(r)
}

func TestShareControlClientRestartRejectsOldEpochAndFetchesFreshChallenge(t *testing.T) {
	database := filepath.Join(t.TempDir(), "control.sqlite")
	first := experimentalShareServer(t, database)
	var current atomic.Pointer[server.Server]
	current.Store(first)
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { current.Load().Handler().ServeHTTP(w, r) }))
	defer h.Close()
	base := h.Client()
	capture := &shareCaptureTransport{base: base.Transport}
	c := newShareClient(t, h, shareClientIdentity(t), &http.Client{Transport: capture, Timeout: 5 * time.Second})
	_, offered, _ := createClientShare(t, c)
	capture.mu.Lock()
	oldRequest := capture.req.Clone(t.Context())
	oldRequest.Body = io.NopCloser(bytes.NewReader(capture.body))
	oldEpoch := oldRequest.Header.Get("X-LinkSend-Share-Epoch")
	capture.mu.Unlock()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second := experimentalShareServer(t, database)
	defer second.Close()
	current.Store(second)
	response, err := base.Do(oldRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("old signed proof survived server restart")
	}
	got := shareControlCall(t, c, signaling.ShareRead, signaling.ShareRequest{ID: offered.ID, Manifest: offered.Manifest})
	capture.mu.Lock()
	newEpoch := capture.req.Header.Get("X-LinkSend-Share-Epoch")
	capture.mu.Unlock()
	if newEpoch == oldEpoch || got.ExpiresAt != offered.ExpiresAt || got.State != sharegrant.Offered || got.Revision != offered.Revision {
		t.Fatal("restart reused challenge or extended/mutated durable grant")
	}
}

func TestShareControlClientRejectsInvalidRequestsWithoutNetwork(t *testing.T) {
	var requests atomic.Int32
	h := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer h.Close()
	c := newShareClient(t, h, shareClientIdentity(t), nil)
	for _, call := range []struct {
		op signaling.ShareOperation
		r  signaling.ShareRequest
	}{{"../../devices", signaling.ShareRequest{ID: protocol.RandomID()}}, {signaling.ShareClaim, signaling.ShareRequest{Code: "invalid-private-code", ClaimID: protocol.RandomID()}}, {signaling.ShareRead, signaling.ShareRequest{ID: protocol.RandomID(), Code: strings.Repeat("A", 32)}}, {signaling.ShareApprove, signaling.ShareRequest{ID: protocol.RandomID()}}, {signaling.ShareBind, signaling.ShareRequest{ID: protocol.RandomID(), Generation: 1 << 63}}} {
		if _, err := c.ShareControl(t.Context(), call.op, call.r); protocol.ErrorCode(err) != protocol.ShareInvalid || strings.Contains(err.Error(), call.r.Code) && call.r.Code != "" {
			t.Fatalf("invalid request leaked or was sent: %v", err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("invalid request reached network")
	}
}
