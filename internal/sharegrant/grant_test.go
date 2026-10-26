package sharegrant

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

var testNow = time.Unix(1792368000, 0)
var sender = Key{1}
var receiver = Key{2}
var stranger = Key{3}
var manifest = Digest{4}
var plan = Digest{5}

const grantID = "11111111111111111111111111111111"
const claimID = "22222222222222222222222222222222"
const approvalID = "33333333333333333333333333333333"
const taskID = "44444444444444444444444444444444"
const otherID = "55555555555555555555555555555555"

func offered(t *testing.T) (Record, string) {
	t.Helper()
	code, digest, err := GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(grantID, sender, manifest, digest, testNow)
	if err != nil {
		t.Fatal(err)
	}
	return r, code
}
func bound(t *testing.T) Record {
	t.Helper()
	r, code := offered(t)
	r, err := r.Claim(receiver, code, claimID, testNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Approve(sender, claimID, approvalID, taskID, r.Generation, testNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.Bind(receiver, taskID, manifest, plan, r.Generation, testNow.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func unchanged(t *testing.T, before, after Record, err error) {
	t.Helper()
	if err == nil || before != after {
		t.Fatalf("failed action changed authorization: err=%v", err)
	}
}

func TestCodeIsCompleteAndOnlyDigestIsStored(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 128; i++ {
		r, code := offered(t)
		if len(code) != 32 || seen[code] {
			t.Fatal("code length/collision")
		}
		seen[code] = true
		var grouped []string
		for j := 0; j < 32; j += 4 {
			grouped = append(grouped, strings.ToLower(code[j:j+4]))
		}
		digest, err := CodeDigest(strings.Join(grouped, "-"))
		if err != nil || digest != r.CodeDigest {
			t.Fatal("display round trip")
		}
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), code) {
			t.Fatal("plaintext credential persisted")
		}
		read, err := Decode(raw)
		if err != nil || read != r {
			t.Fatalf("record round trip: %v", err)
		}
		for _, bad := range []string{code[:31], code + "A", code[:3] + "-" + code[3:], code[:4] + "0" + code[5:], "Ａ" + code[1:], "\n" + code, strings.Repeat("-", 39)} {
			if _, err := CodeDigest(bad); err == nil {
				t.Fatalf("accepted malformed code %q", bad)
			}
		}
	}
}

func TestTwoExplicitConfirmationsBeforeOriginalTaskAuthorization(t *testing.T) {
	r, code := offered(t)
	if err := r.Authorize(sender, taskID, manifest, plan, 1, testNow); err == nil {
		t.Fatal("offered authorized bytes")
	}
	r, err := r.Claim(receiver, code, claimID, testNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Authorize(receiver, taskID, manifest, plan, 1, testNow.Add(time.Second)); err == nil {
		t.Fatal("claim authorized bytes")
	}
	for _, actor := range []Key{receiver, stranger} {
		a, e := r.Approve(actor, claimID, approvalID, taskID, r.Generation, testNow.Add(2*time.Second))
		unchanged(t, r, a, e)
	}
	r, err = r.Approve(sender, claimID, approvalID, taskID, 1, testNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Authorize(receiver, taskID, manifest, plan, 1, testNow.Add(2*time.Second)); err == nil {
		t.Fatal("sender approval bypassed receiver save confirmation")
	}
	for _, mutate := range []func() (Record, error){
		func() (Record, error) { return r.Bind(stranger, taskID, manifest, plan, 1, testNow.Add(3*time.Second)) },
		func() (Record, error) {
			return r.Bind(receiver, otherID, manifest, plan, 1, testNow.Add(3*time.Second))
		},
		func() (Record, error) {
			return r.Bind(receiver, taskID, Digest{99}, plan, 1, testNow.Add(3*time.Second))
		},
		func() (Record, error) { return r.Bind(receiver, taskID, manifest, plan, 2, testNow.Add(3*time.Second)) },
	} {
		a, e := mutate()
		unchanged(t, r, a, e)
	}
	r, err = r.Bind(receiver, taskID, manifest, plan, 1, testNow.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range []Key{sender, receiver} {
		if err = r.Authorize(actor, taskID, manifest, plan, 1, testNow.Add(4*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []error{
		r.Authorize(stranger, taskID, manifest, plan, 1, testNow.Add(4*time.Second)),
		r.Authorize(receiver, otherID, manifest, plan, 1, testNow.Add(4*time.Second)),
		r.Authorize(receiver, taskID, Digest{99}, plan, 1, testNow.Add(4*time.Second)),
		r.Authorize(receiver, taskID, manifest, Digest{99}, 1, testNow.Add(4*time.Second)),
		r.Authorize(receiver, taskID, manifest, plan, 2, testNow.Add(4*time.Second)),
	} {
		if !errors.Is(e, ErrRejected) {
			t.Fatalf("different task/actor/content authorized: %v", e)
		}
	}
}

func TestClaimCannotBeHeldOrSwappedByLateApproval(t *testing.T) {
	r, code := offered(t)
	a, err := r.Claim(receiver, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", claimID, testNow)
	unchanged(t, r, a, err)
	r, err = r.Claim(receiver, code, claimID, testNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	a, err = r.Claim(stranger, code, otherID, testNow.Add(2*time.Second))
	unchanged(t, r, a, err)
	a, err = r.Claim(receiver, code, claimID, testNow.Add(time.Minute))
	if err != nil || a != r {
		t.Fatal("retry renewed pending claim")
	}
	a, err = r.Approve(sender, claimID, approvalID, taskID, 1, time.Unix(r.ClaimUntil, 0))
	unchanged(t, r, a, err)
	next, err := r.Claim(stranger, code, otherID, time.Unix(r.ClaimUntil, 0))
	if err != nil {
		t.Fatal(err)
	}
	if next.Receiver != stranger || next.Generation != 2 {
		t.Fatal("expired occupancy not released")
	}
	a, err = next.Approve(sender, claimID, approvalID, taskID, 1, testNow.Add(3*time.Minute))
	unchanged(t, next, a, err)
	denied, err := next.Deny(sender, otherID, 2, testNow.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if denied.State != Offered || denied.Receiver != (Key{}) || denied.ClaimID != "" || denied.Generation != 3 {
		t.Fatal("deny retained claimant")
	}
	a, err = denied.Deny(sender, otherID, 2, testNow.Add(4*time.Minute))
	if err != nil || a != denied {
		t.Fatal("deny retry not idempotent")
	}
	if err = denied.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFiniteResumeAndCompletionOnlyAcknowledgement(t *testing.T) {
	r := bound(t)
	if err := r.Authorize(receiver, taskID, manifest, plan, r.Generation, testNow.Add(OfferTTL+time.Minute)); err != nil {
		t.Fatal("offer expiry incorrectly ended bound recovery")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ResumeUntil != r.ResumeUntil || restored != r {
		t.Fatal("serialization changed bound authorization")
	}
	retry, err := restored.Bind(receiver, taskID, manifest, plan, r.Generation, testNow.Add(time.Hour))
	if err != nil || retry != r {
		t.Fatal("bind retry renewed resume window")
	}
	if err = r.Authorize(receiver, taskID, manifest, plan, r.Generation, time.Unix(r.ResumeUntil, 0)); !errors.Is(err, ErrExpired) {
		t.Fatal("expired resume accepted")
	}
	if err = r.Authorize(sender, taskID, manifest, plan, r.Generation, testNow.Add(-time.Second)); !errors.Is(err, ErrExpired) {
		t.Fatal("clock rollback authorized")
	}
	done, err := r.Complete(receiver, taskID, manifest, plan, r.Generation, testNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = done.Authorize(sender, taskID, manifest, plan, r.Generation, testNow.Add(time.Hour)); err == nil {
		t.Fatal("completed grant reopened file access")
	}
	again, err := done.Complete(sender, taskID, manifest, plan, r.Generation, testNow.Add(48*time.Hour))
	if err != nil || again != done {
		t.Fatal("lost completion confirmation not idempotent")
	}
	a, err := done.Complete(stranger, taskID, manifest, plan, r.Generation, testNow.Add(48*time.Hour))
	unchanged(t, done, a, err)
	a, err = done.Revoke(sender, testNow.Add(2*time.Hour))
	unchanged(t, done, a, err)
}

func TestRevokeWinsAgainstLateFileCallbacks(t *testing.T) {
	r := bound(t)
	revoked, err := r.Revoke(sender, testNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Generation != r.Generation+1 || revoked.State != Revoked {
		t.Fatal("revoke did not retire generation")
	}
	a, err := revoked.Bind(receiver, taskID, manifest, plan, r.Generation, testNow.Add(time.Hour))
	unchanged(t, revoked, a, err)
	a, err = revoked.Complete(sender, taskID, manifest, plan, r.Generation, testNow.Add(time.Hour))
	unchanged(t, revoked, a, err)
	if err = revoked.Authorize(receiver, taskID, manifest, plan, r.Generation, testNow.Add(time.Hour)); err == nil {
		t.Fatal("revoked transfer allowed")
	}
	a, err = revoked.Revoke(sender, testNow.Add(2*time.Hour))
	if err != nil || a != revoked {
		t.Fatal("revoke retry not idempotent")
	}
}

func TestExpiryAndStrictStoredRecordValidation(t *testing.T) {
	r, code := offered(t)
	expiry := time.Unix(r.ExpiresAt, 0)
	a, err := r.Claim(receiver, code, claimID, expiry)
	unchanged(t, r, a, err)
	r, err = r.Reconcile(expiry)
	if err != nil || r.State != Expired {
		t.Fatalf("expire: %v", err)
	}
	if err = r.Validate(); err != nil {
		t.Fatal(err)
	}
	for i, mutate := range []func(*Record){
		func(r *Record) { r.Version = 99 }, func(r *Record) { r.ID = "../../escape" },
		func(r *Record) { r.Sender = Key{} }, func(r *Record) { r.Manifest = Digest{} },
		func(r *Record) { r.State = "future" }, func(r *Record) { r.Revision = 0 },
		func(r *Record) { r.ExpiresAt++ }, func(r *Record) { r.State = Pending },
		func(r *Record) { r.Receiver = sender; r.ClaimID = claimID },
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			invalid := r
			mutate(&invalid)
			raw, _ := json.Marshal(invalid)
			if _, e := Decode(raw); e == nil {
				t.Fatal("invalid record accepted")
			}
		})
	}
	raw, _ := json.Marshal(r)
	for _, bad := range [][]byte{append(append([]byte{}, raw...), raw...), []byte(strings.Replace(string(raw), `"version":1`, `"future":true,"version":1`, 1)), []byte(strings.Repeat("x", MaxRecordBytes+1))} {
		if _, e := Decode(bad); e == nil {
			t.Fatal("malformed storage accepted")
		}
	}
}
