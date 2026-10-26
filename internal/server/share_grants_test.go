package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
)

func shareServer(t *testing.T, path string, enabled bool) *Server {
	t.Helper()
	s, err := New(Config{Listen: "127.0.0.1:0", Database: path, AllowInsecureLoopback: true, ExperimentalShareControl: enabled})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func shareIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func signedShare(t *testing.T, s *Server, id *identity.Identity, operation string, value any) *http.Request {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := "/v1/shares/" + operation
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:40101"
	nonce, at := protocol.RandomID(), time.Now().Unix()
	req.Header.Set("X-LinkSend-Membership", "2")
	req.Header.Set("X-LinkSend-Share-Grants", "1")
	req.Header.Set("X-LinkSend-Device", id.ID())
	req.Header.Set("X-LinkSend-Public-Key", base64.RawStdEncoding.EncodeToString(id.PublicKey()))
	req.Header.Set("X-LinkSend-Share-Epoch", s.shareEpoch)
	req.Header.Set("X-LinkSend-Nonce", nonce)
	req.Header.Set("X-LinkSend-Time", strconv.FormatInt(at, 10))
	req.Header.Set("X-LinkSend-Signature", base64.RawStdEncoding.EncodeToString(id.Sign(protocol.AuthBytes(http.MethodPost, path, id.ID(), s.shareEpoch+":"+nonce, at, body))))
	return req
}
func runShare(t *testing.T, s *Server, req *http.Request) (ShareStatus, int, string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	var result ShareStatus
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
	}
	return result, w.Code, w.Body.String()
}
func shareCall(t *testing.T, s *Server, id *identity.Identity, operation string, value ShareRequest) ShareStatus {
	t.Helper()
	got, status, body := runShare(t, s, signedShare(t, s, id, operation, value))
	if status != 200 {
		t.Fatalf("%s status=%d response=%s", operation, status, body)
	}
	return got
}
func createShareHTTP(t *testing.T, s *Server, sender *identity.Identity) (ShareStatus, string) {
	t.Helper()
	code, digest, err := sharegrant.GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	status := shareCall(t, s, sender, "create", ShareRequest{ID: protocol.RandomID(), CodeDigest: hex.EncodeToString(digest[:]), Manifest: strings.Repeat("a", 64)})
	return status, code
}
func bindShareHTTP(t *testing.T, s *Server, sender, receiver *identity.Identity) (ShareStatus, string) {
	t.Helper()
	offer, code := createShareHTTP(t, s, sender)
	claim := shareCall(t, s, receiver, "claim", ShareRequest{Code: code, ClaimID: protocol.RandomID()})
	if claim.ID != offer.ID || claim.State != sharegrant.Pending {
		t.Fatal("claim did not bind original offer")
	}
	approved := shareCall(t, s, sender, "approve", ShareRequest{ID: claim.ID, ClaimID: claim.ClaimID, ApprovalID: protocol.RandomID(), TaskID: protocol.RandomID(), Generation: claim.Generation, Manifest: claim.Manifest, ReceiverKey: base64.RawStdEncoding.EncodeToString(receiver.PublicKey())})
	bound := shareCall(t, s, receiver, "bind", ShareRequest{ID: approved.ID, TaskID: approved.TaskID, Generation: approved.Generation, Manifest: approved.Manifest, Plan: strings.Repeat("b", 64)})
	return bound, code
}

func TestShareHTTPAdvertisesFileSupportOnlyAfterExplicitActivation(t *testing.T) {
	s := shareServer(t, filepath.Join(t.TempDir(), "control.sqlite"), false)
	sender := shareIdentity(t)
	_, status, body := runShare(t, s, signedShare(t, s, sender, "create", ShareRequest{}))
	if status != 426 || !strings.Contains(body, "SHARE_UNSUPPORTED") {
		t.Fatalf("disabled share status=%d %s", status, body)
	}
	w := httptest.NewRecorder()
	healthRequest := httptest.NewRequest("GET", "/healthz", nil)
	healthRequest.RemoteAddr = "127.0.0.1:40101"
	s.Handler().ServeHTTP(w, healthRequest)
	if w.Code != 200 {
		t.Fatalf("ordinary health status=%d", w.Code)
	}
	if strings.Contains(w.Body.String(), "share_control") || strings.Contains(w.Body.String(), "share_grants_v1") {
		t.Fatal("ordinary service advertised temporary sharing")
	}
	enabled := shareServer(t, filepath.Join(t.TempDir(), "enabled.sqlite"), true)
	w = httptest.NewRecorder()
	healthRequest = httptest.NewRequest("GET", "/healthz", nil)
	healthRequest.RemoteAddr = "127.0.0.1:40101"
	enabled.Handler().ServeHTTP(w, healthRequest)
	if w.Code != 200 {
		t.Fatalf("experimental health status=%d", w.Code)
	}
	var health struct {
		Control struct {
			Version int    `json:"version"`
			Epoch   string `json:"epoch"`
			Ready   bool   `json:"file_transfer_ready"`
		} `json:"share_control"`
		Capabilities protocol.Capabilities `json:"capabilities"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health.Control.Version != 1 || health.Control.Epoch != enabled.shareEpoch || !health.Control.Ready || !health.Capabilities.ShareGrantsV1 || health.Capabilities.Relay {
		t.Fatal("explicit share service did not advertise its supported data path")
	}
}

func TestShareHTTPSignaturesBindKeyBodyPathEpochAndRejectReplay(t *testing.T) {
	s := shareServer(t, filepath.Join(t.TempDir(), "control.sqlite"), true)
	sender, other := shareIdentity(t), shareIdentity(t)
	offer, code := createShareHTTP(t, s, sender)
	for name, mutate := range map[string]func(*http.Request){
		"wrong public key": func(r *http.Request) {
			r.Header.Set("X-LinkSend-Public-Key", base64.RawStdEncoding.EncodeToString(other.PublicKey()))
		},
		"wrong signature": func(r *http.Request) {
			r.Header.Set("X-LinkSend-Signature", base64.RawStdEncoding.EncodeToString(make([]byte, 64)))
		},
		"wrong epoch": func(r *http.Request) { r.Header.Set("X-LinkSend-Share-Epoch", protocol.RandomID()) },
		"future proof": func(r *http.Request) {
			resignShareTime(t, r, sender, time.Now().Add(time.Hour))
		},
		"expired signed proof": func(r *http.Request) { resignShareTime(t, r, sender, time.Now().Add(-2*time.Minute)) },
		"different path":       func(r *http.Request) { r.URL.Path = "/v1/shares/revoke" },
		"different body":       func(r *http.Request) { r.Body = http.NoBody },
	} {
		t.Run(name, func(t *testing.T) {
			req := signedShare(t, s, sender, "status", ShareRequest{ID: offer.ID})
			mutate(req)
			_, status, _ := runShare(t, s, req)
			if status != 401 {
				t.Fatalf("forged proof status=%d", status)
			}
		})
	}
	request := ShareRequest{ID: offer.ID}
	req := signedShare(t, s, sender, "status", request)
	clone := req.Clone(context.Background())
	body, _ := json.Marshal(request)
	clone.Body = http.NoBody
	_, status, _ := runShare(t, s, req)
	if status != 200 {
		t.Fatal("valid proof rejected")
	}
	clone.Body = io.NopCloser(bytes.NewReader(body))
	_, status, response := runShare(t, s, clone)
	if status != 409 || !strings.Contains(response, "REPLAY") {
		t.Fatalf("replayed proof status=%d %s", status, response)
	}
	got := shareCall(t, s, sender, "status", request)
	if got.Revision != 1 || got.State != sharegrant.Offered {
		t.Fatal("forged request changed grant")
	}
	if strings.Contains(response, code) || strings.Contains(response, offer.Manifest) {
		t.Fatal("failure exposed share metadata")
	}
}

func resignShareTime(t *testing.T, r *http.Request, id *identity.Identity, at time.Time) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.Header.Set("X-LinkSend-Time", strconv.FormatInt(at.Unix(), 10))
	r.Header.Set("X-LinkSend-Signature", base64.RawStdEncoding.EncodeToString(id.Sign(protocol.AuthBytes(r.Method, r.URL.RequestURI(), id.ID(), r.Header.Get("X-LinkSend-Share-Epoch")+":"+r.Header.Get("X-LinkSend-Nonce"), at.Unix(), body))))
}

func TestShareHTTPTwoConfirmationsResumeAndCompletionRemainScoped(t *testing.T) {
	s := shareServer(t, filepath.Join(t.TempDir(), "control.sqlite"), true)
	sender, receiver, stranger := shareIdentity(t), shareIdentity(t), shareIdentity(t)
	bound, code := bindShareHTTP(t, s, sender, receiver)
	request := ShareRequest{ID: bound.ID, TaskID: bound.TaskID, Manifest: bound.Manifest, Plan: bound.Plan, Generation: bound.Generation}
	for _, actor := range []*identity.Identity{sender, receiver} {
		got := shareCall(t, s, actor, "authorize", request)
		if got.ResumeUntil != bound.ResumeUntil {
			t.Fatal("authorization renewed restore window")
		}
	}
	_, status, _ := runShare(t, s, signedShare(t, s, stranger, "status", ShareRequest{ID: bound.ID}))
	if status != 403 {
		t.Fatal("stranger read grant")
	}
	_, status, _ = runShare(t, s, signedShare(t, s, stranger, "authorize", request))
	if status != 403 {
		t.Fatal("wrong public key accessed original task")
	}
	changed := request
	changed.Manifest = strings.Repeat("c", 64)
	_, status, _ = runShare(t, s, signedShare(t, s, receiver, "authorize", changed))
	if status != 403 {
		t.Fatal("changed source authorized")
	}
	changed = request
	changed.TaskID = protocol.RandomID()
	_, status, _ = runShare(t, s, signedShare(t, s, receiver, "authorize", changed))
	if status != 403 {
		t.Fatal("different task authorized")
	}
	done := shareCall(t, s, receiver, "complete", request)
	if done.State != sharegrant.Completed {
		t.Fatal("completion not durable")
	}
	_, status, _ = runShare(t, s, signedShare(t, s, sender, "authorize", request))
	if status != 409 {
		t.Fatal("completed grant authorized new file operation")
	}
	again := shareCall(t, s, sender, "complete", request)
	if again.Revision != done.Revision || again.ResumeUntil != done.ResumeUntil {
		t.Fatal("completion acknowledgement reopened task")
	}
	_, status, response := runShare(t, s, signedShare(t, s, stranger, "claim", ShareRequest{Code: code, ClaimID: protocol.RandomID()}))
	if status != 403 || strings.Contains(response, bound.ReceiverKey) || strings.Contains(response, bound.SenderKey) {
		t.Fatal("code reused or claimant details exposed")
	}
}

func TestShareHTTPUnpairedAndCrossGroupActorsDoNotGainMembership(t *testing.T) {
	s := shareServer(t, filepath.Join(t.TempDir(), "control.sqlite"), true)
	ctx := context.Background()
	sender, receiver, unpaired := shareIdentity(t), shareIdentity(t), shareIdentity(t)
	a, err := s.store.Initialize(ctx, "group a", sender.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.store.Initialize(ctx, "group b", receiver.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	if a.GroupID == b.GroupID {
		t.Fatal("fixture groups are equal")
	}
	bound, _ := bindShareHTTP(t, s, sender, receiver)
	for _, device := range []struct{ ID, Group string }{{a.ID, a.GroupID}, {b.ID, b.GroupID}} {
		got, e := s.store.Device(ctx, device.ID)
		if e != nil || got.GroupID != device.Group {
			t.Fatal("share changed permanent group")
		}
	}
	second, code := createShareHTTP(t, s, sender)
	claim := shareCall(t, s, unpaired, "claim", ShareRequest{Code: code, ClaimID: protocol.RandomID()})
	if claim.ID != second.ID {
		t.Fatal("unpaired claimant rejected")
	}
	if _, err = s.store.Device(ctx, unpaired.ID()); err == nil {
		t.Fatal("claim manufactured permanent identity")
	}
	for _, path := range []string{"/v1/devices", "/v1/pairing/invitations"} {
		method := "GET"
		if path != "/v1/devices" {
			method = "POST"
		}
		body := []byte("{}")
		if method == "GET" {
			body = nil
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:40101"
		nonce := protocol.RandomID()
		at := time.Now().Unix()
		req.Header.Set("X-LinkSend-Membership", "2")
		req.Header.Set("X-LinkSend-Device", unpaired.ID())
		req.Header.Set("X-LinkSend-Nonce", nonce)
		req.Header.Set("X-LinkSend-Time", strconv.FormatInt(at, 10))
		req.Header.Set("X-LinkSend-Signature", base64.RawStdEncoding.EncodeToString(unpaired.Sign(protocol.AuthBytes(method, path, unpaired.ID(), nonce, at, body))))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("temporary claimant accessed ordinary member route %s status=%d", path, w.Code)
		}
	}
	_, status, _ := runShare(t, s, signedShare(t, s, unpaired, "status", ShareRequest{ID: bound.ID}))
	if status != 403 {
		t.Fatal("temporary actor read another grant")
	}
}

func TestShareHTTPRestartRejectsOldProofAndAllowsFreshOriginalResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.sqlite")
	s := shareServer(t, path, true)
	sender, receiver := shareIdentity(t), shareIdentity(t)
	bound, _ := bindShareHTTP(t, s, sender, receiver)
	request := ShareRequest{ID: bound.ID, TaskID: bound.TaskID, Manifest: bound.Manifest, Plan: bound.Plan, Generation: bound.Generation}
	oldProof := signedShare(t, s, receiver, "authorize", request)
	changedEpochProof := oldProof.Clone(context.Background())
	raw, _ := json.Marshal(request)
	changedEpochProof.Body = io.NopCloser(bytes.NewReader(raw))
	oldEpoch := s.shareEpoch
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = shareServer(t, path, true)
	if s.shareEpoch == oldEpoch {
		t.Fatal("boot challenge reused")
	}
	_, status, _ := runShare(t, s, oldProof)
	if status != 401 {
		t.Fatal("pre-restart proof accepted")
	}
	changedEpochProof.Header.Set("X-LinkSend-Share-Epoch", s.shareEpoch)
	_, status, _ = runShare(t, s, changedEpochProof)
	if status != 401 {
		t.Fatal("replacing public epoch bypassed signature")
	}
	got := shareCall(t, s, receiver, "authorize", request)
	if got.ResumeUntil != bound.ResumeUntil || got.Revision != bound.Revision {
		t.Fatal("restart extended or replaced authorization")
	}
	shareCall(t, s, sender, "revoke", ShareRequest{ID: bound.ID})
	_, status, _ = runShare(t, s, signedShare(t, s, receiver, "authorize", request))
	if status != 409 {
		t.Fatal("revoked task resumed")
	}
}

func TestShareHTTPClaimLimitsIdentityAndIgnoresForwardedAddress(t *testing.T) {
	t.Run("identity", func(t *testing.T) {
		s := shareServer(t, filepath.Join(t.TempDir(), "control.sqlite"), true)
		actor := shareIdentity(t)
		for i := 0; i < 11; i++ {
			_, status, _ := runShare(t, s, signedShare(t, s, actor, "claim", ShareRequest{Code: strings.Repeat("A", 32), ClaimID: protocol.RandomID()}))
			if i < 10 && status != 403 {
				t.Fatalf("guess %d status=%d", i, status)
			}
			if i == 10 && status != 429 {
				t.Fatal("identity guess quota bypass")
			}
		}
	})
	t.Run("ip", func(t *testing.T) {
		s := shareServer(t, filepath.Join(t.TempDir(), "control.sqlite"), true)
		for i := 0; i < 21; i++ {
			req := signedShare(t, s, shareIdentity(t), "claim", ShareRequest{Code: strings.Repeat("A", 32), ClaimID: protocol.RandomID()})
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.20.30.%d", i))
			_, status, _ := runShare(t, s, req)
			if i < 20 && status != 403 {
				t.Fatalf("guess %d status=%d", i, status)
			}
			if i == 20 && status != 429 {
				t.Fatal("forwarded address bypassed actual IP quota")
			}
		}
	})
}

func TestShareHTTPBoundsAndSafeProjection(t *testing.T) {
	s := shareServer(t, filepath.Join(t.TempDir(), "control.sqlite"), true)
	sender := shareIdentity(t)
	offer, code := createShareHTTP(t, s, sender)
	for _, value := range []any{map[string]any{"id": offer.ID, "file_path": "D:/private/FORBIDDEN_SOURCE"}, ShareRequest{ID: strings.Repeat("x", 33)}, map[string]any{"code": strings.Repeat("x", protocol.MaxPayloadBytes+1)}} {
		_, status, response := runShare(t, s, signedShare(t, s, sender, "status", value))
		if status != 400 || strings.Contains(response, "FORBIDDEN_SOURCE") {
			t.Fatal("unbounded/unknown request accepted or echoed")
		}
	}
	_, status, response := runShare(t, s, signedShare(t, s, sender, "status", ShareRequest{ID: offer.ID}))
	if status != 200 {
		t.Fatal("safe status failed")
	}
	for _, forbidden := range []string{code, "code_digest", "denied_claim_id", "group_id", "incarnation", "private", "file_path"} {
		if strings.Contains(response, forbidden) {
			t.Fatalf("unsafe status contains %q", forbidden)
		}
	}
}
