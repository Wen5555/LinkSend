package sharegrant

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
)

func signalFixture(t *testing.T) (SignalScope, *identity.Identity, *identity.Identity, SignalFrame) {
	t.Helper()
	a, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	b, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	scope := SignalScope{GrantID: grantID, ClaimID: claimID, TaskID: taskID, GrantGeneration: 3, Phase: SignalPreview, Manifest: strings.Repeat("a", 64)}
	envelope, err := protocol.NewEnvelope("connect_request", a.ID(), b.ID(), protocol.RandomID(), 1, map[string]string{"ufrag": "fixture", "password": "fixture-password-only"})
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature = a.Sign(envelope.SigningBytes())
	frame, err := NewSignalFrame(scope, protocol.RandomID(), a.ID(), "signal", &envelope, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	frame.Signature = a.Sign(frame.SigningBytes())
	return scope, a, b, frame
}

func TestSignalFrameBindsOriginalProofAndGrantScope(t *testing.T) {
	scope, a, b, frame := signalFixture(t)
	if err := frame.Verify(a.PublicKey(), scope, frame.Epoch, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSignalFrame(raw)
	if err != nil || decoded.Verify(a.PublicKey(), scope, frame.Epoch, time.Now(), false) != nil {
		t.Fatal("signed frame roundtrip failed")
	}
	for name, change := range map[string]func(*SignalFrame){
		"grant":            func(f *SignalFrame) { f.Scope.GrantID = protocol.RandomID() },
		"claim":            func(f *SignalFrame) { f.Scope.ClaimID = protocol.RandomID() },
		"task":             func(f *SignalFrame) { f.Scope.TaskID = protocol.RandomID() },
		"grant generation": func(f *SignalFrame) { f.Scope.GrantGeneration++ },
		"phase":            func(f *SignalFrame) { f.Scope.Phase, f.Scope.Plan = SignalTransfer, strings.Repeat("b", 64) },
		"manifest":         func(f *SignalFrame) { f.Scope.Manifest = strings.Repeat("c", 64) },
		"epoch":            func(f *SignalFrame) { f.Epoch = protocol.RandomID() },
		"nonce":            func(f *SignalFrame) { f.Nonce = protocol.RandomID() },
		"actor":            func(f *SignalFrame) { f.ActorID = b.ID() },
		"outer signature":  func(f *SignalFrame) { f.Signature = b.Sign(f.SigningBytes()) },
		"inner signature": func(f *SignalFrame) {
			f.Message.Signature = b.Sign(f.Message.SigningBytes())
			f.Signature = a.Sign(f.SigningBytes())
		},
		"ICE generation": func(f *SignalFrame) { f.Message.Generation++; f.Signature = a.Sign(f.SigningBytes()) },
		"ICE session": func(f *SignalFrame) {
			f.Message.SessionID = protocol.RandomID()
			f.Signature = a.Sign(f.SigningBytes())
		},
		"expired": func(f *SignalFrame) {
			f.IssuedAt = time.Now().Add(-2 * time.Minute).Unix()
			f.ExpiresAt = f.IssuedAt + 60
			f.Signature = a.Sign(f.SigningBytes())
		},
	} {
		t.Run(name, func(t *testing.T) {
			copyFrame, _ := DecodeSignalFrame(raw)
			change(&copyFrame)
			if err := copyFrame.Verify(a.PublicKey(), scope, frame.Epoch, time.Now(), false); err == nil {
				t.Fatal("changed authorization or proof admitted")
			}
		})
	}
}

func TestSignalPayloadCannotForwardFileOrClipboardFields(t *testing.T) {
	for _, fixture := range []struct{ kind, body string }{
		{"connect_request", `{"ufrag":"a","password":"b","file_path":"private"}`},
		{"connect_response", `{"ufrag":"a","password":"b","clipboard_sync":true}`},
		{"connect_request", `{"ufrag":"a","password":"b","session_reuse":true}`},
		{"connect_request", `{"ufrag":"a","password":"b","clipboard_files_v1":true}`},
		{"connect_request", `{"ufrag":"a","password":"b","authorization_generation":1}`},
		{"candidate", `{"candidate":"1 1 udp 2130706431 10.0.0.1 3000 typ host","data":"private"}`},
		{"end_of_candidates", `{"file_bytes":"private"}`},
		{"end_of_candidates", `null`},
		{"status", `{"state":"failed","code":"CHECK_TIMEOUT","body":"private"}`},
		{"status", `{"state":"completed","code":"CHECK_TIMEOUT"}`},
		{"status", `{"state":"failed","code":"anything-private"}`},
		{"file", `{"body":"private"}`},
	} {
		if ValidateSignalPayload(protocol.Envelope{Type: fixture.kind, Payload: json.RawMessage(fixture.body)}, false) == nil {
			t.Fatalf("unbounded signaling payload accepted: %s", fixture.kind)
		}
	}
}

func TestSignalJSONRejectsNestedSemanticDuplicateFields(t *testing.T) {
	_, _, _, frame := signalFixture(t)
	raw, _ := json.Marshal(frame)
	for name, changed := range map[string]string{
		"frame":        strings.Replace(string(raw), `"version":1`, `"version":1,"VERSION":1`, 1),
		"Unicode fold": strings.Replace(string(raw), `"scope":`, `"ſcope":{},"scope":`, 1),
		"scope":        strings.Replace(string(raw), `"claim_id":`, `"CLAIM_ID":"x","claim_id":`, 1),
		"envelope":     strings.Replace(string(raw), `"sender":`, `"SENDER":"x","sender":`, 1),
		"payload":      strings.Replace(string(raw), `"ufrag":"fixture"`, `"ufrag":"fixture","u\u0066rag":"fixture"`, 1),
		"trailing":     string(raw) + ` {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSignalFrame([]byte(changed)); err == nil {
				t.Fatal("ambiguous frame decoded")
			}
		})
	}
}

func TestSignalScopeStrictQueryAndStateGates(t *testing.T) {
	r := bound(t)
	scope := SignalScope{GrantID: r.ID, ClaimID: r.ClaimID, TaskID: r.TaskID, GrantGeneration: r.Generation, Phase: SignalTransfer, Manifest: hex.EncodeToString(r.Manifest[:]), Plan: hex.EncodeToString(r.Plan[:])}
	uri := scope.RequestURI()
	rawQuery := strings.SplitN(uri, "?", 2)[1]
	decoded, err := ParseSignalScope(scope.GrantID, rawQuery)
	if err != nil || decoded != scope || scope.Authorize(r, receiver, testNow.Add(5*time.Second)) != nil {
		t.Fatal("bound original scope rejected")
	}
	for _, bad := range []string{rawQuery + "&task_id=" + taskID, rawQuery + "&code=private", rawQuery + "&unknown=x", strings.Replace(rawQuery, "generation=1", "generation=01", 1)} {
		if _, err := ParseSignalScope(scope.GrantID, bad); err == nil {
			t.Fatal("ambiguous or foreign query accepted")
		}
	}
	if scope.Authorize(r, stranger, testNow.Add(5*time.Second)) == nil || scope.Authorize(r, receiver, time.Unix(r.ResumeUntil, 0)) == nil {
		t.Fatal("foreign actor or expired original task admitted")
	}
	preview := scope
	preview.Phase, preview.Plan = SignalPreview, ""
	if preview.Authorize(r, receiver, testNow.Add(5*time.Second)) == nil {
		t.Fatal("bound task reused the retired preview scope")
	}
	approved := r
	approved.State, approved.Plan, approved.ResumeUntil = Approved, Digest{}, 0
	if preview.Authorize(approved, receiver, testNow.Add(5*time.Second)) != nil || scope.Authorize(approved, receiver, testNow.Add(5*time.Second)) == nil {
		t.Fatal("preview/transfer permissions were conflated")
	}
}
