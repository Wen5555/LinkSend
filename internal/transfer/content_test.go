package transfer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/content"
	"github.com/Wen5555/LinkSend/internal/transport"
)

func contentFixture(t *testing.T, kind content.Kind) (*Prepared, ContentDescriptor, []byte) {
	t.Helper()
	store, err := content.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var snapshot content.Snapshot
	switch kind {
	case content.Text:
		snapshot, err = store.CreateText(t.Context(), kind, "中文正文\n<script>never execute</script>", "test:content")
	case content.URL:
		snapshot, err = store.CreateText(t.Context(), kind, "https://example.test/path?q=%E4%B8%AD", "test:content")
	case content.Image:
		img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
		img.Set(0, 0, color.NRGBA{R: 31, G: 62, B: 93, A: 255})
		snapshot, err = store.CreateImageFromImage(t.Context(), img, "test:content")
	default:
		t.Fatal("bad fixture kind")
	}
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.OwnedPath(t.Context(), snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Prepare(t.Context(), []string{path}, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	d, err := NewContentDescriptor(p.Manifest, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if d.Digest != Sum(body) || d.Digest != p.Manifest.Files[0].Hash {
		t.Fatal("snapshot and wire BLAKE3 differ")
	}
	return p, d, body
}

type contentOutcome struct {
	result Result
	err    error
}

func TestContentSnapshotRealQUICAndAllSkipped(t *testing.T) {
	for _, kind := range []content.Kind{content.Text, content.URL, content.Image} {
		for _, skipped := range []bool{false, true} {
			t.Run(string(kind)+map[bool]string{false: "/selected", true: "/all_skipped"}[skipped], func(t *testing.T) {
				p, descriptor, body := contentFixture(t, kind)
				original, _ := json.Marshal(p.Manifest)
				client, server, peer := receivePlanQUICPair(t)
				dest := t.TempDir()
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				done := make(chan contentOutcome, 1)
				go func() {
					stream, err := server.AcceptStream(ctx)
					if err != nil {
						done <- contentOutcome{err: err}
						return
					}
					result, err := ReceiveWithOptions(ctx, transport.WrapStream(stream), ReceiveOptions{Directory: dest, Peer: peer, AcceptNativeContent: true, Plan: func(ctx context.Context, offer Offer) (ReceivePlan, error) {
						if !reflect.DeepEqual(offer.Content, &descriptor) || offer.ContentDigest != descriptor.BindingDigest(p.Manifest) {
							return ReceivePlan{}, ErrContentMismatch
						}
						// A UI callback receives a copy and cannot mutate the wire binding.
						offer.Content.Kind = "mutated callback"
						var selected []uint32
						if skipped {
							selected = []uint32{}
						}
						return BuildReceivePlan(ctx, dest, offer.Manifest, PlanRequest{SelectedIDs: selected})
					}})
					done <- contentOutcome{result, err}
				}()
				stream, err := client.OpenStreamSync(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var sendProgress Progress
				result, err := SendWithOptions(ctx, transport.WrapStream(stream), p, SendOptions{Content: &descriptor, Hooks: SendHooks{Progress: func(progress Progress) { sendProgress = progress }}})
				received := <-done
				if err != nil || received.err != nil {
					t.Fatalf("send=%v receive=%v", err, received.err)
				}
				if !reflect.DeepEqual(result.Content, &descriptor) || !reflect.DeepEqual(received.result.Content, &descriptor) || result.ContentDigest != descriptor.BindingDigest(p.Manifest) || result.ContentDigest != received.result.ContentDigest || result.FileFallback || received.result.FileFallback {
					t.Fatalf("content terminal mismatch: %+v %+v", result, received.result)
				}
				if result.SelectionDigest != received.result.SelectionDigest || result.SelectionDigest == "" {
					t.Fatal("lost M4 selection binding")
				}
				want := int64(len(body))
				if skipped {
					want = 0
					if result.State != "NoContent" || received.result.State != "NoContent" {
						t.Fatal("all skipped claimed completion")
					}
				}
				if result.Bytes != want || received.result.Bytes != want || sendProgress.Sent != want {
					t.Fatal("body accounting mismatch")
				}
				actual, readErr := os.ReadFile(filepath.Join(dest, p.Manifest.Files[0].Path))
				if skipped {
					if !os.IsNotExist(readErr) {
						t.Fatal("skipped content was committed")
					}
				} else if readErr != nil || !bytes.Equal(actual, body) {
					t.Fatal("actual content differs")
				}
				after, _ := json.Marshal(p.Manifest)
				if !bytes.Equal(original, after) {
					t.Fatal("content changed legacy manifest JSON")
				}
			})
		}
	}
}

func TestContentDescriptorLimitsAndDigestBinding(t *testing.T) {
	p, d, _ := contentFixture(t, content.Text)
	original := d.BindingDigest(p.Manifest)
	for _, change := range []func(*ContentDescriptor){
		func(d *ContentDescriptor) { d.Version = 2 }, func(d *ContentDescriptor) { d.EntryID = 1 },
		func(d *ContentDescriptor) { d.Size++ }, func(d *ContentDescriptor) { d.Digest = strings.Repeat("0", 64) },
		func(d *ContentDescriptor) { d.MediaType = "text/html" }, func(d *ContentDescriptor) { d.Kind = "executable" },
		func(d *ContentDescriptor) { d.Width = 1 }, func(d *ContentDescriptor) { d.Height = 1 },
	} {
		bad := d
		change(&bad)
		if bad.Validate(p.Manifest) == nil || bad.BindingDigest(p.Manifest) == original {
			t.Fatal("invalid interpretation accepted or not bound")
		}
	}
	url := d
	url.Kind = content.URL
	if url.Validate(p.Manifest) != nil || url.BindingDigest(p.Manifest) == original {
		t.Fatal("kind is not independently bound")
	}
	large := cloneManifest(p.Manifest)
	large.Files[0].Size = content.MaxTextBytes + 1
	large.Files[0].Chunks = append(large.Files[0].Chunks, large.Files[0].Chunks[0])
	oversized := d
	oversized.Size = large.Files[0].Size
	if !errors.Is(oversized.Validate(large), ErrContentInvalid) {
		t.Fatal("text bound missing")
	}
	ip, id, _ := contentFixture(t, content.Image)
	for _, dimensions := range [][2]int{{0, 1}, {1, 0}, {content.MaxImageDimension + 1, 1}, {10000, 10000}} {
		bad := id
		bad.Width, bad.Height = dimensions[0], dimensions[1]
		if !errors.Is(bad.Validate(ip.Manifest), ErrContentInvalid) {
			t.Fatal("image dimension bound missing")
		}
	}
	many, _ := planFixture(t)
	if d.Validate(many.Manifest) == nil {
		t.Fatal("multi-file typed content allowed")
	}
	encoded, _ := json.Marshal(d)
	if bytes.Contains(encoded, []byte("<script>")) || bytes.Contains(encoded, []byte("content-snapshots")) {
		t.Fatal("body/path leaked into descriptor")
	}
}

// Frozen pre-content shape; the peer has no knowledge of content metadata or
// capabilities. Default denial must happen even if it has accepted the file.
func frozenContentReceiver(rw io.ReadWriteCloser, expectBody bool) (int64, error) {
	defer rw.Close()
	offer, err := legacyRead(rw)
	if err != nil {
		return 0, err
	}
	if offer.Manifest == nil || offer.Digest != offer.Manifest.Digest() {
		return 0, ErrIntegrity
	}
	if err = legacyWrite(rw, legacyControl{Op: "accept", Digest: offer.Digest}); err != nil {
		return 0, err
	}
	var verified int64
	for _, e := range offer.Manifest.Files {
		for i := range e.Chunks {
			err = legacyWrite(rw, legacyControl{Op: "chunk", File: e.ID, Index: i})
			if err != nil {
				if !expectBody {
					return 0, nil
				}
				return verified, err
			}
			data, err := readFrame(rw, offer.Manifest.ChunkSize)
			if err != nil {
				if !expectBody {
					return 0, nil
				}
				return verified, err
			}
			if !expectBody {
				return int64(len(data)), errors.New("unsupported receiver received a body frame")
			}
			if Sum(data) != e.Chunks[i] {
				return verified, ErrIntegrity
			}
			verified += int64(len(data))
			if err = legacyWrite(rw, legacyControl{Op: "ack", File: e.ID, Index: i, Verified: verified}); err != nil {
				return verified, err
			}
		}
	}
	if err = legacyWrite(rw, legacyControl{Op: "ready"}); err != nil {
		return verified, err
	}
	finish, err := legacyRead(rw)
	if err != nil || finish.Op != "finish" || finish.Digest != offer.Digest {
		return verified, errors.Join(err, ErrIntegrity)
	}
	if err = legacyWrite(rw, legacyControl{Op: "completed", Digest: offer.Digest, Verified: verified}); err != nil {
		return verified, err
	}
	confirmed, err := legacyRead(rw)
	if err != nil || confirmed.Op != "confirmed" || confirmed.Digest != offer.Digest {
		return verified, errors.Join(err, ErrIntegrity)
	}
	return verified, nil
}

func TestContentFrozenLegacyReceiverExplicitFallbackOnly(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "default_stop", true: "explicit_file"}[allow], func(t *testing.T) {
			p, d, body := contentFixture(t, content.Text)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			a, b := net.Pipe()
			done := make(chan contentOutcome, 1)
			go func() { n, err := frozenContentReceiver(b, allow); done <- contentOutcome{Result{Bytes: n}, err} }()
			result, err := SendWithOptions(ctx, a, p, SendOptions{Content: &d, AllowFileFallback: allow})
			received := <-done
			if received.err != nil {
				t.Fatal(received.err)
			}
			if !allow {
				if !errors.Is(err, ErrContentUnsupported) || received.result.Bytes != 0 {
					t.Fatalf("default not blocked before body: %v %+v", err, received.result)
				}
				return
			}
			if err != nil || !result.FileFallback || result.Content != nil || result.ContentDigest != "" || result.Bytes != int64(len(body)) || result.Bytes != received.result.Bytes {
				t.Fatalf("untruthful fallback: %+v %v", result, err)
			}
		})
	}
}

func TestContentAcceptanceRequiresContentAwareDecision(t *testing.T) {
	for _, mode := range []string{"no_plan", "legacy_plan", "reject", "fallback", "fallback_plan"} {
		t.Run(mode, func(t *testing.T) {
			p, d, _ := contentFixture(t, content.URL)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			a, b := net.Pipe()
			done := make(chan contentOutcome, 1)
			dest := t.TempDir()
			options := ReceiveOptions{Directory: dest, Peer: "peer", Accept: func(Manifest) bool { return true }}
			if mode == "legacy_plan" || mode == "fallback_plan" {
				options.Plan = func(_ context.Context, o Offer) (ReceivePlan, error) {
					if o.Content != nil {
						return ReceivePlan{}, ErrContentMismatch
					}
					return FullReceivePlan(o.Manifest), nil
				}
			}
			if mode == "reject" {
				options.AcceptNativeContent = true
				options.Plan = func(context.Context, Offer) (ReceivePlan, error) { return ReceivePlan{}, ErrRejected }
			}
			go func() { r, e := ReceiveWithOptions(ctx, b, options); done <- contentOutcome{r, e} }()
			var sent int64
			r, e := SendWithOptions(ctx, a, p, SendOptions{Content: &d, AllowFileFallback: strings.HasPrefix(mode, "fallback"), Hooks: SendHooks{Progress: func(p Progress) { sent = p.Sent }}})
			other := <-done
			if strings.HasPrefix(mode, "fallback") {
				if e != nil || other.err != nil || !r.FileFallback || !other.result.FileFallback || r.Content != nil || other.result.Content != nil {
					t.Fatalf("fallback: %+v %v %+v", r, e, other)
				}
				return
			}
			want := ErrContentUnsupported
			if mode == "reject" {
				want = ErrRejected
			}
			if !errors.Is(e, want) || !errors.Is(other.err, want) || sent != 0 {
				t.Fatalf("rejection: %v %v sent=%d", e, other.err, sent)
			}
		})
	}
}

type contentDigestMutator struct {
	io.ReadWriteCloser
	op string
}

func (m *contentDigestMutator) Write(p []byte) (int, error) {
	// Production writeControl writes the length and JSON separately. Replacing
	// exactly 64 digest characters retains the length prefix and other fields.
	if bytes.HasPrefix(p, []byte(`{"op":"`+m.op+`"`)) {
		var c map[string]json.RawMessage
		if json.Unmarshal(p, &c) == nil && c["content_digest"] != nil {
			old := append([]byte(`"content_digest":`), c["content_digest"]...)
			replacement := []byte(`"content_digest":"` + strings.Repeat("0", 64) + `"`)
			p = bytes.Replace(p, old, replacement, 1)
		}
	}
	return m.ReadWriteCloser.Write(p)
}

func TestContentEveryTerminalDigestIsChecked(t *testing.T) {
	for _, op := range []string{opAccept, opFinish, opCompleted, opConfirmed, opConfirmedAck} {
		t.Run(op, func(t *testing.T) {
			p, d, _ := contentFixture(t, content.Text)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			a, b := net.Pipe()
			var send, receive io.ReadWriteCloser = a, b
			if op == opFinish || op == opConfirmed {
				send = &contentDigestMutator{a, op}
			} else {
				receive = &contentDigestMutator{b, op}
			}
			done := make(chan contentOutcome, 1)
			dest := t.TempDir()
			go func() {
				r, e := ReceiveWithOptions(ctx, receive, ReceiveOptions{Directory: dest, Peer: "peer", AcceptNativeContent: true, Plan: func(_ context.Context, o Offer) (ReceivePlan, error) { return FullReceivePlan(o.Manifest), nil }})
				done <- contentOutcome{r, e}
			}()
			r, e := SendWithOptions(ctx, send, p, SendOptions{Content: &d})
			other := <-done
			if op == opFinish || op == opConfirmed {
				if !errors.Is(other.err, ErrContentMismatch) {
					t.Fatalf("receiver missed binding: %v", other.err)
				}
			} else if !errors.Is(e, ErrContentMismatch) {
				t.Fatalf("sender missed binding: %v", e)
			}
			if e == nil && other.err == nil {
				t.Fatalf("mismatched terminal completed: %+v %+v", r, other)
			}
			if op == opAccept || op == opFinish {
				if _, err := os.Stat(filepath.Join(dest, p.Manifest.Files[0].Path)); !os.IsNotExist(err) {
					t.Fatal("committed before matching acceptance/finish")
				}
			}
		})
	}
}

func TestContentCheckpointBindingAndLegacyProtection(t *testing.T) {
	p, d, _ := contentFixture(t, content.URL)
	dest := t.TempDir()
	plan := FullReceivePlan(p.Manifest)
	r, err := openReceiverContent(t.Context(), dest, "peer", p.Manifest, &plan, nil, &d)
	if err != nil {
		t.Fatal(err)
	}
	fillPlannedReceiver(t, r, p)
	if err = r.Mark("Paused"); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	path := filepath.Join(dest, ".linksend-"+p.Manifest.TransferID, "state.json")
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state ResumeState
	if json.Unmarshal(saved, &state) != nil || !strings.HasPrefix(state.State, "ContentV1/") || state.ContentDigest != d.BindingDigest(p.Manifest) {
		t.Fatal("checkpoint content binding missing")
	}
	if r, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan); !errors.Is(err, ErrContentMismatch) {
		if r != nil {
			_ = r.Close()
		}
		t.Fatal("plain file resume discarded content type")
	}
	changed := d
	changed.Kind = content.Text
	if r, err := openReceiverContent(t.Context(), dest, "peer", p.Manifest, &plan, nil, &changed); !errors.Is(err, ErrContentMismatch) {
		if r != nil {
			_ = r.Close()
		}
		t.Fatal("same bytes changed interpretation on resume")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(saved, after) {
		t.Fatal("failed content resume mutated checkpoint")
	}
	r, err = openReceiverContent(t.Context(), dest, "peer", p.Manifest, &plan, nil, &d)
	if err != nil {
		t.Fatal(err)
	}
	if r.VerifiedBytes() != p.Manifest.TotalBytes() {
		t.Fatal("matching content did not recover verified chunks")
	}
	_ = r.Close()
}

func TestContentInvalidOfferFailsBeforeDecision(t *testing.T) {
	for _, mode := range []string{"unknown_kind", "wrong_digest", "without_capability"} {
		t.Run(mode, func(t *testing.T) {
			p, d, _ := contentFixture(t, content.Text)
			c := control{Op: opOffer, Manifest: &p.Manifest, Digest: p.Manifest.Digest(), Content: &d, ContentDigest: d.BindingDigest(p.Manifest), Capabilities: []string{CapabilityContent}}
			switch mode {
			case "unknown_kind":
				c.Content.Kind = "exec"
			case "wrong_digest":
				c.ContentDigest = strings.Repeat("0", 64)
			case "without_capability":
				c.Capabilities = nil
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			a, b := net.Pipe()
			done := make(chan error, 1)
			go func() {
				_, e := ReceiveWithOptions(ctx, b, ReceiveOptions{Directory: t.TempDir(), Peer: "peer", AcceptNativeContent: true, Plan: func(context.Context, Offer) (ReceivePlan, error) {
					return ReceivePlan{}, errors.New("decision must not run")
				}})
				done <- e
			}()
			encoded, _ := json.Marshal(c)
			if err := writeFrame(a, encoded, MaxMetadata); err != nil {
				t.Fatal(err)
			}
			_, err := readControl(a)
			_ = a.Close()
			other := <-done
			want := ErrContentInvalid
			if mode == "unknown_kind" {
				want = ErrContentUnsupported
			}
			if mode == "wrong_digest" {
				want = ErrContentMismatch
			}
			if !errors.Is(err, want) || !errors.Is(other, want) {
				t.Fatalf("invalid offer: %v %v", err, other)
			}
		})
	}
}

// This sender intentionally bypasses SendWithOptions' local content validation,
// so receiver checks are exercised against correctly hashed malicious bodies.
func rawContentSender(ctx context.Context, rw io.ReadWriteCloser, p *Prepared, d ContentDescriptor) error {
	defer rw.Close()
	binding := d.BindingDigest(p.Manifest)
	if err := writeControl(rw, control{Op: opOffer, Manifest: &p.Manifest, Digest: p.Manifest.Digest(), Capabilities: []string{CapabilityContent, CapabilityReceivePlan}, Content: &d, ContentDigest: binding}); err != nil {
		return err
	}
	accepted, err := readControl(rw)
	if err != nil {
		return err
	}
	if accepted.Op != opAccept || accepted.ContentDigest != binding || accepted.Selection == nil {
		return ErrContentMismatch
	}
	buffer := make([]byte, p.Manifest.ChunkSize)
	for {
		request, err := readControl(rw)
		if err != nil {
			return err
		}
		if request.Op == opReady {
			break
		}
		if request.Op != opChunk {
			return errors.New("unexpected request")
		}
		data, err := p.ReadChunk(ctx, request.File, request.Index, buffer)
		if err != nil {
			return err
		}
		if err = writeFrame(rw, data, p.Manifest.ChunkSize); err != nil {
			return err
		}
		if _, err = readControl(rw); err != nil {
			return err
		}
	}
	if err = writeControl(rw, control{Op: opFinish, Digest: p.Manifest.Digest(), SelectionDigest: accepted.Selection.Digest, ContentDigest: binding}); err != nil {
		return err
	}
	completed, err := readControl(rw)
	if err != nil {
		return err
	}
	if completed.Op != opCompleted || completed.ContentDigest != binding {
		return ErrContentMismatch
	}
	if err = writeControl(rw, control{Op: opConfirmed, Digest: p.Manifest.Digest(), SelectionDigest: accepted.Selection.Digest, ContentDigest: binding}); err != nil {
		return err
	}
	_, err = readControl(rw)
	return err
}

func TestContentReceiverChecksHashedBodiesBeforeCommit(t *testing.T) {
	_, imageDescriptor, imageBody := contentFixture(t, content.Image)
	for _, kind := range []string{"invalid_utf8", "nul_text", "png_dimensions", "truncated_png", "unsupported_url_is_inert"} {
		t.Run(kind, func(t *testing.T) {
			body := []byte{0xff, 0xfe}
			descriptor := ContentDescriptor{Version: 1, Kind: content.Text, MediaType: "text/plain; charset=utf-8"}
			want := ErrContentInvalid
			switch kind {
			case "nul_text":
				body = []byte("hello\x00world")
			case "png_dimensions":
				body = imageBody
				descriptor = imageDescriptor
				descriptor.Width++
				want = ErrContentMismatch
			case "truncated_png":
				body = imageBody[:len(imageBody)-10]
				descriptor = imageDescriptor
			case "unsupported_url_is_inert":
				body = []byte("javascript:alert('not executed')")
				descriptor.Kind = content.URL
				want = nil
			}
			file := filepath.Join(t.TempDir(), "incoming.data")
			if err := os.WriteFile(file, body, 0600); err != nil {
				t.Fatal(err)
			}
			p, err := Prepare(t.Context(), []string{file}, 64<<10)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			descriptor.Size = int64(len(body))
			descriptor.Digest = p.Manifest.Files[0].Hash
			if err = descriptor.Validate(p.Manifest); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			a, b := net.Pipe()
			done := make(chan contentOutcome, 1)
			dest := t.TempDir()
			go func() {
				result, e := ReceiveWithOptions(ctx, b, ReceiveOptions{Directory: dest, Peer: "peer", AcceptNativeContent: true, Plan: func(_ context.Context, o Offer) (ReceivePlan, error) { return FullReceivePlan(o.Manifest), nil }})
				done <- contentOutcome{result, e}
			}()
			err = rawContentSender(ctx, a, p, descriptor)
			other := <-done
			if !errors.Is(err, want) || !errors.Is(other.err, want) {
				t.Fatalf("body validation: %v %v want %v", err, other.err, want)
			}
			actual, readErr := os.ReadFile(filepath.Join(dest, "incoming.data"))
			if want != nil {
				if !os.IsNotExist(readErr) {
					t.Fatal("invalid typed body committed")
				}
			} else if readErr != nil || !bytes.Equal(actual, body) || content.ValidateURL(string(actual)) == nil || other.result.Content == nil {
				t.Fatal("unsupported URL not preserved as inert text")
			}
		})
	}
}

func TestContentValidCheckpointResumesWireAndCannotUpgradeLegacy(t *testing.T) {
	p, d, body := contentFixture(t, content.Text)
	dest := t.TempDir()
	plan := FullReceivePlan(p.Manifest)
	r, err := openReceiverContent(t.Context(), dest, "peer", p.Manifest, &plan, nil, &d)
	if err != nil {
		t.Fatal(err)
	}
	fillPlannedReceiver(t, r, p)
	if err = r.Mark("Paused"); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	a, b := net.Pipe()
	done := make(chan contentOutcome, 1)
	go func() {
		result, e := ReceiveWithOptions(ctx, b, ReceiveOptions{Directory: dest, Peer: "peer", AcceptNativeContent: true, Plan: func(_ context.Context, o Offer) (ReceivePlan, error) {
			if o.ResumePlan == nil {
				return ReceivePlan{}, ErrResumeMismatch
			}
			return *o.ResumePlan, nil
		}})
		done <- contentOutcome{result, e}
	}()
	var sent int64
	result, err := SendWithOptions(ctx, a, p, SendOptions{Content: &d, Hooks: SendHooks{Progress: func(p Progress) { sent = p.Sent }}})
	other := <-done
	if err != nil || other.err != nil || sent != 0 || result.Bytes != int64(len(body)) || other.result.ContentDigest != result.ContentDigest {
		t.Fatalf("resume: %+v %v %+v sent=%d", result, err, other, sent)
	}
	actual, err := os.ReadFile(filepath.Join(dest, p.Manifest.Files[0].Path))
	if err != nil || !bytes.Equal(actual, body) {
		t.Fatal("resumed content differs")
	}
	legacyDest := t.TempDir()
	r, err = OpenReceiver(t.Context(), legacyDest, "peer", p.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	if r, err := openReceiverContent(t.Context(), legacyDest, "peer", p.Manifest, &plan, nil, &d); !errors.Is(err, ErrContentMismatch) {
		if r != nil {
			_ = r.Close()
		}
		t.Fatal("legacy checkpoint silently gained content type")
	}
}

func TestContentSourceChangeCannotOfferStaleSnapshot(t *testing.T) {
	p, d, _ := contentFixture(t, content.Text)
	src := p.sources[0]
	if err := src.root.WriteFile(src.relative, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	a, b := net.Pipe()
	defer b.Close()
	done := make(chan error, 1)
	go func() { _, err := SendWithOptions(ctx, a, p, SendOptions{Content: &d}); done <- err }()
	if frame, err := readFrame(b, MaxMetadata); err == nil || len(frame) != 0 {
		t.Fatal("stale source emitted an offer")
	}
	if err := <-done; !errors.Is(err, ErrContentMismatch) {
		t.Fatal(err)
	}
}
