package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

// These are test-only stand-ins, NOT measured native launch/boot evidence.
// In particular a child-signed SourcesClosed=true claim below does not prove
// COM lifetime. Receipt-only must keep every such object protected anyway.
type testCacheNativeLaunch struct {
	public  ed25519.PublicKey
	private ed25519.PrivateKey
	err     error
}

func (p *testCacheNativeLaunch) verifyCacheNativeLaunch(_ context.Context, _ cacheNativeBinding) (cacheNativeLaunchIdentity, error) {
	if p == nil {
		return cacheNativeLaunchIdentity{}, errClipboardCacheOwnership
	}
	return cacheNativeLaunchIdentity{BootID: "fixture-boot-A", HelperImageSHA256: strings.Repeat("a", 64), ChildEphemeralPublicKey: p.public}, p.err
}

type testCacheNativeBoot string

func (b testCacheNativeBoot) cacheNativeBootID() (string, error) { return string(b), nil }

type cacheNativeFixture struct {
	s       *Service
	d       *clipboardCacheDestination
	r       *clipboardCacheReceipt
	launch  *testCacheNativeLaunch
	closed  bool
	body    []byte
	profile string
}

func newCacheNativeFixture(t *testing.T) *cacheNativeFixture {
	t.Helper()
	s := newCacheLifecycleTestService(t)
	d, err := openClipboardCacheDestination(s.cfg.DataDir, protocol.RandomID())
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.registerClipboardCache(d, s.identity.ID())
	if err != nil {
		t.Fatal(err)
	}
	f := &cacheNativeFixture{s: s, d: d, r: r, body: []byte("owned native authorization fixture"), profile: s.cfg.DataDir}
	t.Cleanup(f.closeStorage)
	source := filepath.Join(t.TempDir(), "owned.txt")
	if err := os.WriteFile(source, f.body, 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := transfer.Prepare(t.Context(), []string{source}, transfer.DefaultChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	m := prepared.Manifest
	m.TransferID = r.record.ID
	_ = prepared.Close()
	if err := r.bindManifest(m); err != nil {
		t.Fatal(err)
	}
	stageName := ".linksend-" + r.record.ID
	if err := d.object.Mkdir(stageName, 0700); err != nil {
		t.Fatal(err)
	}
	stage, err := d.object.OpenRoot(stageName)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	if err := stage.WriteFile("0.part", f.body, 0600); err != nil {
		t.Fatal(err)
	}
	plan := transfer.FullReceivePlan(m)
	state := transfer.ResumeState{Peer: s.identity.ID(), Manifest: m, Digest: m.Digest(), Plan: &plan, PlanDigest: plan.Digest()}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.WriteFile("state.json", stateJSON, 0600); err != nil {
		t.Fatal(err)
	}
	part, err := stage.Open("0.part")
	if err != nil {
		t.Fatal(err)
	}
	defer part.Close()
	checkpoint, err := stage.Open("state.json")
	if err != nil {
		t.Fatal(err)
	}
	defer checkpoint.Close()
	storage := transfer.ReceiveStorage{Root: d.object, Stage: stage, Files: map[uint32]*os.File{0: part}, Checkpoint: checkpoint, Manifest: m, Plan: plan, Peer: s.identity.ID()}
	if err := r.bindStage(storage); err != nil {
		t.Fatal(err)
	}
	if err := d.object.Link(stageName+"/0.part", "owned.txt"); err != nil {
		t.Fatal(err)
	}
	// This fixture owns these real files/handles. Product provenance still
	// comes only from the actual receiver's closing-storage observer.
	if err := r.captureClosingStorage(storage); err != nil {
		t.Fatal(err)
	}
	_ = checkpoint.Close()
	_ = part.Close()
	_ = stage.Close()
	if err := r.advance("verified"); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f.launch = &testCacheNativeLaunch{public: public, private: private}
	return f
}

func (f *cacheNativeFixture) closeStorage() {
	if !f.closed {
		_ = f.d.Close()
		f.r.release()
		f.closed = true
	}
}

func (f *cacheNativeFixture) seal(t *testing.T) cacheNativeDelegation {
	t.Helper()
	d, err := f.r.sealNativeDelegation(t.Context(), f.launch)
	if err != nil {
		t.Fatal("native fixture delegation", err)
	}
	return d
}

func signedTestCacheNativeReceipt(t *testing.T, delegation cacheNativeDelegation, private ed25519.PrivateKey, mutate func(*cacheNativeCloseReceipt)) []byte {
	t.Helper()
	digest, err := cacheNativeDelegationDigest(delegation)
	if err != nil {
		t.Fatal(err)
	}
	r := cacheNativeCloseReceipt{Version: 1, Purpose: "cache-native-quiescent", DelegationDigest: digest, Binding: delegation.Binding, Sequence: 1,
		AdmissionClosed: true, SourcesClosed: true, NativeCloseWitnessID: protocol.RandomID()}
	if mutate != nil {
		mutate(&r)
	}
	message, err := cacheNativeReceiptMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Signature = ed25519.Sign(private, message)
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cacheNativeTestRaw(t *testing.T, s *Service, id string) string {
	t.Helper()
	var raw string
	if err := s.store.db.QueryRow(`SELECT value FROM metadata WHERE key=?`, clipboardCacheKeyPrefix+id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertCacheNativeBody(t *testing.T, f *cacheNativeFixture) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(f.profile, "clipboard-files", f.r.record.ID, "owned.txt"))
	if err != nil || !bytes.Equal(body, f.body) {
		t.Fatal("receipt-only changed or removed the real owned body", err)
	}
}

func TestClipboardCacheNativeNilGoldenCompatibility(t *testing.T) {
	// A literal pre-extension v1 signing vector catches field order, omission,
	// purpose-domain and null-signature drift independently of the new struct.
	r := clipboardCacheRecord{Version: 1, Purpose: "clipboard-files", Owner: "owner", Run: "run", ID: "id", CacheRoot: "cache", Directory: "object", Peer: "peer", Created: 1, Revision: 1, State: "receiving"}
	const golden = "LinkSend clipboard cache metadata v1\x00" + `{"version":1,"purpose":"clipboard-files","owner":"owner","run":"run","id":"id","cache_root":"cache","directory":"object","peer":"peer","created":1,"revision":1,"state":"receiving","signature":null}`
	message, err := cacheRecordMessage(r)
	if err != nil || string(message) != golden {
		t.Fatal("nil Native changed the literal v1 signing vector", err)
	}
	s := newCacheLifecycleTestService(t)
	r.Owner, r.Peer, r.Run, r.ID = s.identity.ID(), s.identity.ID(), s.epoch, protocol.RandomID()
	legacyJSON, err := json.Marshal(struct {
		Version   int    `json:"version"`
		Purpose   string `json:"purpose"`
		Owner     string `json:"owner"`
		Run       string `json:"run"`
		ID        string `json:"id"`
		CacheRoot string `json:"cache_root"`
		Directory string `json:"directory"`
		Peer      string `json:"peer"`
		Created   int64  `json:"created"`
		Revision  uint64 `json:"revision"`
		State     string `json:"state"`
		Signature []byte `json:"signature"`
	}{r.Version, r.Purpose, r.Owner, r.Run, r.ID, r.CacheRoot, r.Directory, r.Peer, r.Created, r.Revision, r.State, nil})
	if err != nil {
		t.Fatal(err)
	}
	legacySignature := s.identity.Sign(append([]byte("LinkSend clipboard cache metadata v1\x00"), legacyJSON...))
	legacySigned := bytes.Replace(legacyJSON, []byte(`"signature":null`), append([]byte(`"signature":`), mustJSONNative(t, legacySignature)...), 1)
	encoded, err := s.encodeCacheRecord(r)
	if err != nil || encoded != string(legacySigned) {
		t.Fatal("real profile signature/serialization changed for a legacy nil record", err)
	}
	if decoded, err := s.decodeCacheRecord(r.ID, encoded); err != nil || decoded.Native != nil || !bytes.Equal(decoded.Signature, legacySignature) {
		t.Fatal("legacy golden signature no longer loads", err)
	}
	for _, malformed := range []string{encoded + `{}`, strings.TrimSuffix(encoded, "}") + `,"version":1}`, strings.TrimSuffix(encoded, "}") + `,"Version":1}`, strings.TrimSuffix(encoded, "}") + `,"future":true}`} {
		if _, err := s.decodeCacheRecord(r.ID, malformed); err == nil {
			t.Fatal("ambiguous/unknown legacy metadata accepted")
		}
	}
}

func TestClipboardCacheNativePopulatedLegacyGoldenCompatibility(t *testing.T) {
	f := newCacheNativeFixture(t)
	r := f.r.record
	r.State, r.Sealed, r.Cleanup = "published", true, f.r.finalInventory
	// Frozen pre-Native v1 layout, including every old optional field. A
	// receiving-only vector would miss ordering/omission of real provenance.
	legacy := struct {
		Version    int                           `json:"version"`
		Purpose    string                        `json:"purpose"`
		Owner      string                        `json:"owner"`
		Run        string                        `json:"run"`
		ID         string                        `json:"id"`
		CacheRoot  string                        `json:"cache_root"`
		Directory  string                        `json:"directory"`
		Peer       string                        `json:"peer"`
		Created    int64                         `json:"created"`
		Revision   uint64                        `json:"revision"`
		State      string                        `json:"state"`
		Manifest   *transfer.Manifest            `json:"manifest,omitempty"`
		Digest     string                        `json:"manifest_digest,omitempty"`
		Stage      string                        `json:"stage_identity,omitempty"`
		PlanDigest string                        `json:"plan_digest,omitempty"`
		Parts      map[string]string             `json:"parts,omitempty"`
		Signature  []byte                        `json:"signature"`
		Sealed     bool                          `json:"sealed,omitempty"`
		Cleanup    map[string]clipboardCacheNode `json:"cleanup,omitempty"`
	}{r.Version, r.Purpose, r.Owner, r.Run, r.ID, r.CacheRoot, r.Directory, r.Peer, r.Created, r.Revision, r.State,
		r.Manifest, r.Digest, r.Stage, r.PlanDigest, r.Parts, nil, r.Sealed, r.Cleanup}
	beforeSignature := mustJSONNative(t, legacy)
	legacy.Signature = f.s.identity.Sign(append([]byte("LinkSend clipboard cache metadata v1\x00"), beforeSignature...))
	oldRaw := mustJSONNative(t, legacy)
	newRaw, err := f.s.encodeCacheRecord(r)
	if err != nil || newRaw != string(oldRaw) {
		t.Fatal("populated legacy bytes/signature changed with Native=nil", err)
	}
	loaded, err := f.s.decodeCacheRecord(r.ID, string(oldRaw))
	if err != nil || loaded.Native != nil || !bytes.Equal(loaded.Signature, legacy.Signature) || len(loaded.Cleanup) == 0 {
		t.Fatal("populated legacy golden did not retain provenance and signature", err)
	}
	assertCacheNativeBody(t, f)
}

func mustJSONNative(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestClipboardCacheNativeRequiresLaunchAndActualInventory(t *testing.T) {
	f := newCacheNativeFixture(t)
	original := f.r.raw
	for _, launch := range []cacheNativeVerifiedLaunch{nil, (*testCacheNativeLaunch)(nil), &testCacheNativeLaunch{}, &testCacheNativeLaunch{public: f.launch.public, err: errClipboardCacheOwnership}} {
		if _, err := f.r.sealNativeDelegation(t.Context(), launch); err == nil {
			t.Fatal("missing/unverified launcher was accepted")
		}
	}
	inventory := f.r.finalInventory
	f.r.finalInventory = nil
	if _, err := f.r.sealNativeDelegation(t.Context(), f.launch); err == nil {
		t.Fatal("missing closing-storage inventory was accepted")
	}
	f.r.finalInventory = inventory
	if cacheNativeTestRaw(t, f.s, f.r.record.ID) != original {
		t.Fatal("rejected launch modified metadata")
	}
	delegation := f.seal(t)
	loaded := loadTestCacheRecord(t, f.s, f.r.record.ID)
	if loaded.State != "publishing" || !loaded.Sealed || loaded.Native == nil || loaded.Native.Phase != "delegated" || loaded.Native.Delegation.Binding.Nonce != delegation.Binding.Nonce {
		t.Fatal("actual inventory and delegation were not durably sealed before dispatch")
	}
	if _, err := f.r.sealNativeDelegation(t.Context(), f.launch); err == nil {
		t.Fatal("publication obtained a second delegation")
	}
	if err := f.r.beginNative(); err == nil {
		t.Fatal("legacy path publisher acquired a native delegation")
	}
	next := f.r.record
	native := *next.Native
	native.Phase = "quiescent"
	next.Native, next.Revision = &native, next.Revision+1
	if err := f.r.save(next); err == nil {
		t.Fatal("ordinary current-run save bypassed native receipt consumption")
	}
	assertCacheNativeBody(t, f)
}

func TestClipboardCacheNativeRejectsBindingAndSignatureChanges(t *testing.T) {
	f := newCacheNativeFixture(t)
	d := f.seal(t)
	f.closeStorage()
	raw := cacheNativeTestRaw(t, f.s, f.r.record.ID)
	changes := map[string]func(*cacheNativeCloseReceipt){
		"owner":        func(r *cacheNativeCloseReceipt) { r.Binding.Owner = strings.Repeat("b", 64) },
		"run":          func(r *cacheNativeCloseReceipt) { r.Binding.OwnerRun = protocol.RandomID() },
		"boot":         func(r *cacheNativeCloseReceipt) { r.Binding.BootID = "different-boot" },
		"cache":        func(r *cacheNativeCloseReceipt) { r.Binding.CacheID = protocol.RandomID() },
		"profile_root": func(r *cacheNativeCloseReceipt) { r.Binding.ProfileRootID = "different-profile" },
		"cache_root":   func(r *cacheNativeCloseReceipt) { r.Binding.CacheRootID = "different-cache" },
		"object":       func(r *cacheNativeCloseReceipt) { r.Binding.ObjectID = "different-object" },
		"stage":        func(r *cacheNativeCloseReceipt) { r.Binding.StageID = "different-stage" },
		"manifest":     func(r *cacheNativeCloseReceipt) { r.Binding.ManifestDigest = strings.Repeat("b", 64) },
		"plan":         func(r *cacheNativeCloseReceipt) { r.Binding.PlanDigest = strings.Repeat("b", 64) },
		"inventory":    func(r *cacheNativeCloseReceipt) { r.Binding.InventoryDigest = strings.Repeat("b", 64) },
		"publication":  func(r *cacheNativeCloseReceipt) { r.Binding.PublicationID = protocol.RandomID() },
		"generation":   func(r *cacheNativeCloseReceipt) { r.Binding.PublicationGeneration++ },
		"nonce":        func(r *cacheNativeCloseReceipt) { r.Binding.Nonce = strings.Repeat("b", 64) },
		"contract":     func(r *cacheNativeCloseReceipt) { r.Binding.NativeContract = "claimed-delete-authority" },
		"helper_image": func(r *cacheNativeCloseReceipt) { r.Binding.HelperImageSHA256 = strings.Repeat("b", 64) },
		"child_key": func(r *cacheNativeCloseReceipt) {
			r.Binding.ChildEphemeralPublicKey = bytes.Repeat([]byte{1}, ed25519.PublicKeySize)
		},
		"delegation_digest":    func(r *cacheNativeCloseReceipt) { r.DelegationDigest = strings.Repeat("b", 64) },
		"sequence":             func(r *cacheNativeCloseReceipt) { r.Sequence++ },
		"admission_still_open": func(r *cacheNativeCloseReceipt) { r.AdmissionClosed = false },
		"source_still_open":    func(r *cacheNativeCloseReceipt) { r.SourcesClosed = false },
		"witness_missing":      func(r *cacheNativeCloseReceipt) { r.NativeCloseWitnessID = "" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			body := signedTestCacheNativeReceipt(t, d, f.launch.private, change)
			if ok, err := f.s.consumeCacheNativeReceipt(t.Context(), f.r.record.ID, raw, body, testCacheNativeBoot("fixture-boot-A")); ok || err == nil {
				t.Fatal("changed binding/close claim accepted")
			}
			if cacheNativeTestRaw(t, f.s, f.r.record.ID) != raw {
				t.Fatal("rejected binding changed the signed row")
			}
			assertCacheNativeBody(t, f)
		})
	}
	_, wrongKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	valid := signedTestCacheNativeReceipt(t, d, f.launch.private, nil)
	malformed := [][]byte{signedTestCacheNativeReceipt(t, d, wrongKey, nil), append(bytes.Clone(valid), []byte(`{}`)...), bytes.Repeat([]byte{'x'}, maxCacheNativeMessage+1)}
	for _, suffix := range []string{`,"version":1}`, `,"Version":1}`, `,"unknown":true}`} {
		malformed = append(malformed, append(bytes.Clone(valid[:len(valid)-1]), []byte(suffix)...))
	}
	malformed = append(malformed, append([]byte(" "), valid...))
	for _, body := range malformed {
		if ok, err := f.s.consumeCacheNativeReceipt(t.Context(), f.r.record.ID, raw, body, testCacheNativeBoot("fixture-boot-A")); ok || err == nil {
			t.Fatal("wrong signature/ambiguous receipt accepted")
		}
	}
	for _, boot := range []cacheNativeVerifiedBoot{nil, testCacheNativeBoot("")} {
		if ok, err := f.s.consumeCacheNativeReceipt(t.Context(), f.r.record.ID, raw, valid, boot); ok || err == nil {
			t.Fatal("unknown current boot provider accepted")
		}
	}
	assertCacheNativeBody(t, f)
}

func TestClipboardCacheNativeOldMainReceiptSingleCASAfterRealKeyReload(t *testing.T) {
	f := newCacheNativeFixture(t)
	d := f.seal(t)
	body := signedTestCacheNativeReceipt(t, d, f.launch.private, nil)
	oldRun, oldPublic, raw := f.s.epoch, f.s.identity.PublicKey(), f.r.raw
	if ok, err := f.s.consumeCacheNativeReceipt(t.Context(), f.r.record.ID, raw, body, testCacheNativeBoot("fixture-boot-A")); ok || err == nil {
		t.Fatal("active original worker was consumed")
	}
	f.closeStorage()
	f.s.Shutdown()
	reopened, err := New(Config{DataDir: f.profile})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Shutdown)
	if reopened.epoch == oldRun || !bytes.Equal(reopened.identity.PublicKey(), oldPublic) {
		t.Fatal("fixture did not reload the same real profile key into a new main run")
	}
	stale := &clipboardCacheReceipt{service: reopened, record: f.r.record, raw: raw}
	if err := stale.beginNative(); err == nil {
		t.Fatal("old delegation reactivated a native publisher")
	}
	next := stale.record
	next.Revision++
	if err := stale.save(next); err == nil {
		t.Fatal("ordinary receipt save lost its current-run restriction")
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			ok, err := reopened.consumeCacheNativeReceipt(t.Context(), f.r.record.ID, raw, body, testCacheNativeBoot("fixture-boot-B"))
			if ok && err == nil {
				successes.Add(1)
			} else if ok || err == nil {
				t.Error("CAS failure reported an ambiguous outcome")
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("original publication receipt did not have exactly one CAS consumer")
	}
	consumedRaw := cacheNativeTestRaw(t, reopened, f.r.record.ID)
	consumed := loadTestCacheRecord(t, reopened, f.r.record.ID)
	if consumed.Run != oldRun || consumed.Native.Phase != "quiescent" || consumed.Revision != f.r.record.Revision+1 || consumed.Native.QuiescentObservedAt <= 0 || consumed.Native.ConsumedReceiptDigest != cacheNativeSHA(body) {
		t.Fatal("old-run receipt did not persist its exact one-time consumption")
	}
	if ok, err := reopened.consumeCacheNativeReceipt(t.Context(), f.r.record.ID, consumedRaw, body, testCacheNativeBoot("fixture-boot-B")); ok || !errors.Is(err, errCacheNativeConsumed) {
		t.Fatal("replay was not rejected as already consumed", err)
	}
	if cacheNativeTestRaw(t, reopened, f.r.record.ID) != consumedRaw {
		t.Fatal("replay reset the first observation time or signature")
	}
	var schema int
	if err := reopened.store.db.QueryRow(`PRAGMA user_version`).Scan(&schema); err != nil || schema != 12 {
		t.Fatal("native receipt added a schema migration", err)
	}
	// Valid child signature plus self-reported close, old run and changed boot
	// are STILL not deletion authority. This is not a native semantic proof.
	for _, pressure := range []bool{false, true} {
		if removed, err := reopened.collectClipboardCache(t.Context(), pressure, time.Now().Add(100*365*24*time.Hour)); err != nil || removed != 0 {
			t.Fatal("receipt-only gained automatic deletion authority", err)
		}
	}
	if err := reopened.collectClipboardCacheObject(t.Context(), consumed, consumedRaw); !errors.Is(err, errClipboardCacheOwnership) {
		t.Fatal("direct collector accepted a receipt-only object", err)
	}
	assertCacheNativeBody(t, f)
}

func TestClipboardCacheNativeReceiptOnlyCannotBecomeDeleteIntent(t *testing.T) {
	f := newCacheNativeFixture(t)
	d := f.seal(t)
	f.closeStorage()
	body := signedTestCacheNativeReceipt(t, d, f.launch.private, nil)
	if ok, err := f.s.consumeCacheNativeReceipt(t.Context(), f.r.record.ID, f.r.raw, body, testCacheNativeBoot("fixture-boot-A")); !ok || err != nil {
		t.Fatal(err)
	}
	base := loadTestCacheRecord(t, f.s, f.r.record.ID)
	for _, state := range []string{"publishing", "published", "protected", "reclaimable", "deleting"} {
		t.Run(state, func(t *testing.T) {
			r := base
			r.State, r.Revision, r.Created = state, base.Revision+1, 1
			// Model a signed, even malicious future state without relying on
			// the new encoder to approve a forbidden deletion transition.
			message, err := cacheRecordMessage(r)
			if err != nil {
				t.Fatal(err)
			}
			r.Signature = f.s.identity.Sign(message)
			raw := string(mustJSONNative(t, r))
			if _, err := f.s.store.db.Exec(`UPDATE metadata SET value=? WHERE key=?`, raw, clipboardCacheKeyPrefix+r.ID); err != nil {
				t.Fatal(err)
			}
			if removed, err := f.s.collectClipboardCache(t.Context(), true, time.Now().Add(100*365*24*time.Hour)); err != nil || removed != 0 {
				t.Fatal("old/pressured native row reached the delete loop", err)
			}
			if err := f.s.collectClipboardCacheObject(t.Context(), r, raw); err == nil {
				t.Fatal("receipt-only reached per-object deletion")
			}
			assertCacheNativeBody(t, f)
		})
	}
}

func TestClipboardCacheNativeCollectorRejectsStrippedCallerRecord(t *testing.T) {
	f := newCacheNativeFixture(t)
	f.seal(t)
	f.closeStorage()
	raw := f.r.raw
	caller := loadTestCacheRecord(t, f.s, f.r.record.ID)
	caller.Native, caller.State = nil, "reclaimable"
	// The durable row is still Native. A stale/incomplete caller-side struct
	// must never be able to discard that protection at the per-object gate.
	if err := f.s.collectClipboardCacheObject(t.Context(), caller, raw); err == nil {
		t.Error("collector trusted caller fields instead of the authenticated row")
	}
	if cacheNativeTestRaw(t, f.s, f.r.record.ID) != raw {
		t.Error("collector replaced native evidence with an unpublished intent")
	}
	assertCacheNativeBody(t, f)
}

func TestClipboardCacheNativeCASAndReadbackFailuresKeepOriginal(t *testing.T) {
	for _, mode := range []string{"stale_raw", "abort", "readback_changed"} {
		t.Run(mode, func(t *testing.T) {
			f := newCacheNativeFixture(t)
			d := f.seal(t)
			f.closeStorage()
			raw := f.r.raw
			body := signedTestCacheNativeReceipt(t, d, f.launch.private, nil)
			expected := raw
			switch mode {
			case "stale_raw":
				expected += " "
			case "abort":
				_, err := f.s.store.db.Exec(`CREATE TRIGGER native_abort BEFORE UPDATE ON metadata WHEN instr(NEW.value,'"phase":"quiescent"')>0 BEGIN SELECT RAISE(ABORT,'fixture write failure'); END`)
				if err != nil {
					t.Fatal(err)
				}
			case "readback_changed":
				_, err := f.s.store.db.Exec(`CREATE TRIGGER native_change AFTER UPDATE ON metadata WHEN instr(NEW.value,'"phase":"quiescent"')>0 BEGIN UPDATE metadata SET value='broken fixture row' WHERE key=NEW.key; END`)
				if err != nil {
					t.Fatal(err)
				}
			}
			if ok, err := f.s.consumeCacheNativeReceipt(t.Context(), f.r.record.ID, expected, body, testCacheNativeBoot("fixture-boot-A")); ok || err == nil {
				t.Fatal("failed CAS/readback reported consumption")
			}
			if cacheNativeTestRaw(t, f.s, f.r.record.ID) != raw {
				t.Fatal("failed transaction did not preserve the prior full row")
			}
			assertCacheNativeBody(t, f)
		})
	}
}

func TestClipboardCacheNativeRechecksActualRootsAndInventory(t *testing.T) {
	for _, mode := range []string{"unknown_entry", "replaced_entry", "missing_entry", "object_identity", "cache_root_identity", "profile_root_identity"} {
		t.Run(mode, func(t *testing.T) {
			f := newCacheNativeFixture(t)
			d := f.seal(t)
			f.closeStorage()
			raw := f.r.raw
			body := signedTestCacheNativeReceipt(t, d, f.launch.private, nil)
			object := filepath.Join(f.profile, "clipboard-files", f.r.record.ID)
			preservedOwned := filepath.Join(object, "owned.txt")
			switch mode {
			case "unknown_entry":
				if err := os.WriteFile(filepath.Join(object, "user-owned.txt"), []byte("preserve unknown body"), 0600); err != nil {
					t.Fatal(err)
				}
			case "replaced_entry":
				if err := os.Rename(filepath.Join(object, "owned.txt"), filepath.Join(object, "original.txt")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(object, "owned.txt"), []byte("user replacement"), 0600); err != nil {
					t.Fatal(err)
				}
				preservedOwned = filepath.Join(object, "original.txt")
			case "missing_entry":
				preservedOwned = filepath.Join(t.TempDir(), "preserved.txt")
				if err := os.Rename(filepath.Join(object, "owned.txt"), preservedOwned); err != nil {
					t.Fatal(err)
				}
			case "object_identity":
				if err := os.Rename(object, object+"-preserved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(object, 0700); err != nil {
					t.Fatal(err)
				}
				preservedOwned = filepath.Join(object+"-preserved", "owned.txt")
			case "cache_root_identity":
				cache := filepath.Join(f.profile, "clipboard-files")
				if err := os.Rename(cache, cache+"-preserved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(cache, 0700); err != nil {
					t.Fatal(err)
				}
				preservedOwned = filepath.Join(cache+"-preserved", f.r.record.ID, "owned.txt")
			case "profile_root_identity":
				r := loadTestCacheRecord(t, f.s, f.r.record.ID)
				n := *r.Native
				n.Delegation.Binding.ProfileRootID = "another-profile-root"
				message, err := cacheNativeDelegationMessage(n.Delegation)
				if err != nil {
					t.Fatal(err)
				}
				n.Delegation.Signature = f.s.identity.Sign(message)
				r.Native = &n
				encoded, err := f.s.encodeCacheRecord(r)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.store.db.Exec(`UPDATE metadata SET value=? WHERE key=?`, encoded, clipboardCacheKeyPrefix+r.ID); err != nil {
					t.Fatal(err)
				}
				raw = encoded
				body = signedTestCacheNativeReceipt(t, n.Delegation, f.launch.private, nil)
			}
			if ok, err := f.s.consumeCacheNativeReceipt(t.Context(), f.r.record.ID, raw, body, testCacheNativeBoot("fixture-boot-A")); ok || err == nil {
				t.Fatal("changed real root/tree identity was accepted")
			}
			if cacheNativeTestRaw(t, f.s, f.r.record.ID) != raw {
				t.Fatal("identity failure rewrote evidence")
			}
			// Check actual retained bytes, including the original tree under a
			// renamed root. Merely finding some file would not prove protection.
			preserved, err := os.ReadFile(preservedOwned)
			if err != nil || !bytes.Equal(preserved, f.body) {
				t.Fatal("original owned bytes not preserved", err)
			}
			if mode == "unknown_entry" || mode == "replaced_entry" {
				name, want := "user-owned.txt", "preserve unknown body"
				if mode == "replaced_entry" {
					name, want = "owned.txt", "user replacement"
				}
				body, err := os.ReadFile(filepath.Join(object, name))
				if err != nil || string(body) != want {
					t.Fatal("user bytes not preserved", err)
				}
			}
		})
	}
}
