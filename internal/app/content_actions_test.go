package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wen5555/LinkSend/internal/content"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

// This fixture creates and verifies real local transfer output. Native OS
// callbacks are observed in Go; it is not a claim of physical menu execution.
func contentActionFixture(t *testing.T, kind content.Kind, body []byte) (*Service, *taskRecord, string) {
	t.Helper()
	s := contentService(t)
	file := filepath.Join(t.TempDir(), "received-content.txt")
	if kind == content.Image {
		file = filepath.Join(t.TempDir(), "received-image.png")
	}
	if err := os.WriteFile(file, body, 0600); err != nil {
		t.Fatal(err)
	}
	task, p := inboxIndexedFixture(t, s, []string{file}, "receive")
	dest := t.TempDir()
	plan, err := transfer.BuildReceivePlan(t.Context(), dest, p.Manifest, transfer.PlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	task.updateRecordAttempt("", func(snap *TaskSnapshot, recovery *taskRecovery) {
		snap.TargetDirectory = dest
		recovery.TargetDirectory = dest
	})
	r, err := transfer.OpenReceiverWithPlan(t.Context(), dest, task.snap.PeerID, p.Manifest, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err = s.RegisterInboxReceivePlan(task.snap.ID, p.Manifest, plan); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, p.Manifest.ChunkSize)
	for i := range p.Manifest.Files[0].Chunks {
		chunk, err := p.ReadChunk(t.Context(), 0, i, buffer)
		if err != nil {
			t.Fatal(err)
		}
		if err = r.WriteChunk(t.Context(), 0, i, chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err = r.Finish(t.Context()); err != nil {
		t.Fatal(err)
	}
	d := transfer.ContentDescriptor{Version: 1, Kind: kind, MediaType: "text/plain; charset=utf-8", Size: int64(len(body)), Digest: transfer.Sum(body)}
	if kind == content.Image {
		d.MediaType = "image/png"
		d.Width = 2
		d.Height = 3
	}
	if err = d.Validate(p.Manifest); err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(d)
	if _, err = s.store.db.Exec(`INSERT INTO content_tasks(task_id,direction,descriptor,binding,mode)VALUES(?,'receive',?,?,'native')`, task.snap.ID, metadata, d.BindingDigest(p.Manifest)); err != nil {
		t.Fatal(err)
	}
	task.update(func(snap *TaskSnapshot) { snap.BilateralConfirmed = true })
	task.finish("completed", nil)
	return s, task, filepath.Join(dest, p.Manifest.Files[0].Path)
}

func TestContentActionsRequireCompletedBoundTaskAndExplicitNativeEnable(t *testing.T) {
	body := []byte("private copied 中文\n<script>inert</script>")
	s, task, path := contentActionFixture(t, content.Text, body)
	calls := 0
	copyFn := func(value string) error {
		calls++
		if value != string(body) {
			return errors.New("wrong bytes")
		}
		return nil
	}
	if _, err := s.CopyReceivedText(t.Context(), task.snap.ID, copyFn); err == nil || calls != 0 {
		t.Fatal("native actions enabled implicitly")
	}
	s.EnableNativeContentActions()
	info, err := s.ContentTask(t.Context(), task.snap.ID)
	if err != nil || !info.Available || !info.CanCopy || !info.CanPreview || info.CanOpen || info.CanSave {
		t.Fatal("wrong action metadata", info, err)
	}
	result, err := s.CopyReceivedText(t.Context(), task.snap.ID, copyFn)
	if err != nil || calls != 1 || result.State != "completed" {
		t.Fatal("copy did not use verified Go bytes", err)
	}
	encoded, _ := json.Marshal([]any{info, result})
	if bytes.Contains(encoded, body) || bytes.Contains(encoded, []byte(path)) {
		t.Fatal("content action DTO leaked body/path")
	}
	unconfirmed := task.snapshot()
	unconfirmed.BilateralConfirmed = false
	unconfirmed.Revision++
	if err = upsertTask(s.store.db, unconfirmed, task.recoverySnapshot()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CopyReceivedText(t.Context(), task.snap.ID, copyFn); !errors.Is(err, ErrInboxFileUnavailable) || calls != 1 {
		t.Fatal("unconfirmed receive authorised action")
	}
	unconfirmed.BilateralConfirmed = true
	unconfirmed.Revision++
	if err = upsertTask(s.store.db, unconfirmed, task.recoverySnapshot()); err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), body...)
	changed[0] = 'X'
	if err = os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CopyReceivedText(t.Context(), task.snap.ID, copyFn); !errors.Is(err, ErrInboxFileChanged) || calls != 1 {
		t.Fatal("same-size changed content reached clipboard")
	}
	if info, err = s.ContentTask(t.Context(), task.snap.ID); err != nil || info.Available {
		t.Fatal("changed content still available", err)
	}
}

func TestContentURLOpenAlwaysChecksSchemeAtAction(t *testing.T) {
	for _, value := range []string{"https://example.test/path", "javascript:alert('inert')", "file:///etc/passwd", "https://user:secret@example.test"} {
		t.Run(value, func(t *testing.T) {
			s, task, _ := contentActionFixture(t, content.URL, []byte(value))
			s.EnableNativeContentActions()
			opened := ""
			result, err := s.OpenReceivedURL(t.Context(), task.snap.ID, func(url string) error { opened = url; return nil })
			valid := content.ValidateURL(value) == nil
			if valid {
				if err != nil || opened != value || result.State != "submitted" {
					t.Fatal("explicit supported open failed", err)
				}
			} else {
				if err == nil || opened != "" {
					t.Fatal("unsupported URL reached OS")
				}
				var copied string
				if _, err = s.CopyReceivedText(t.Context(), task.snap.ID, func(text string) error { copied = text; return nil }); err != nil || copied != value {
					t.Fatal("unsafe scheme cannot be copied as inert text", err)
				}
			}
		})
	}
}

func TestContentPreviewBoundedAndPNGSaveNeverOverwrites(t *testing.T) {
	s, task, _ := contentActionFixture(t, content.Text, []byte(strings.Repeat("中", 6000)))
	s.EnableNativeContentActions()
	preview := ""
	if result, err := s.PreviewReceivedText(t.Context(), task.snap.ID, func(value string) error { preview = value; return nil }); err != nil || result.State != "submitted" || len([]rune(preview)) > 4140 || !strings.Contains(preview, "4096") {
		t.Fatal("unbounded native preview", err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	images, imageTask, source := contentActionFixture(t, content.Image, encoded.Bytes())
	images.EnableNativeContentActions()
	target := filepath.Join(t.TempDir(), "saved.png")
	if result, err := images.SaveReceivedImage(t.Context(), imageTask.snap.ID, target); err != nil || result.State != "completed" {
		t.Fatal("PNG save failed", err)
	}
	if actual, err := os.ReadFile(target); err != nil || !bytes.Equal(actual, encoded.Bytes()) {
		t.Fatal("PNG saved bytes differ")
	}
	if err := os.WriteFile(target, []byte("existing user file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := images.SaveReceivedImage(t.Context(), imageTask.snap.ID, target); !errors.Is(err, os.ErrExist) {
		t.Fatal("save overwrote existing file", err)
	}
	if actual, _ := os.ReadFile(target); string(actual) != "existing user file" {
		t.Fatal("existing file changed")
	}
	if result, err := images.CleanupContentSnapshots(t.Context()); err != nil || result.Removed != 0 {
		t.Fatal("received files treated as owned snapshots", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("received output removed")
	}
}
