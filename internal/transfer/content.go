package transfer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/Wen5555/LinkSend/internal/content"
)

const CapabilityContent = "content_v1"

var (
	ErrContentUnsupported = errors.New("CONTENT_UNSUPPORTED")
	ErrContentInvalid     = errors.New("INVALID_CONTENT_DESCRIPTOR")
	ErrContentMismatch    = errors.New("CONTENT_MISMATCH")
)

// ContentDescriptor contains metadata only. Digest is the lowercase BLAKE3-256
// of the file body, matching content.Snapshot.Digest and FileEntry.Hash.
// It is deliberately separate from Manifest, whose legacy JSON stays unchanged.
// URL is a sender-provided type hint, never permission to open it. Received
// values must pass content.ValidateURL again at the explicit open action;
// unsupported schemes are displayed as inert text.
type ContentDescriptor struct {
	Version   int          `json:"version"`
	EntryID   uint32       `json:"entry_id"`
	Kind      content.Kind `json:"kind"`
	MediaType string       `json:"media_type"`
	Size      int64        `json:"size"`
	Digest    string       `json:"digest"`
	Width     int          `json:"width,omitempty"`
	Height    int          `json:"height,omitempty"`
}

// ContentAcceptance reports the validated interpretation after capability and
// explicit fallback negotiation. The descriptor is a caller-owned copy.
type ContentAcceptance struct {
	Content       *ContentDescriptor `json:"content,omitempty"`
	ContentDigest string             `json:"content_digest,omitempty"`
	FileFallback  bool               `json:"file_fallback,omitempty"`
}

type SendOptions struct {
	Hooks   SendHooks
	Content *ContentDescriptor
	// The caller must obtain the user's explicit file-fallback choice. Absence
	// of content_v1 acceptance otherwise stops before the first body frame.
	AllowFileFallback bool
}

func NewContentDescriptor(m Manifest, snapshot content.Snapshot) (ContentDescriptor, error) {
	d := ContentDescriptor{Version: 1, Kind: snapshot.Kind, MediaType: snapshot.MediaType, Size: snapshot.Size, Digest: snapshot.Digest, Width: snapshot.Width, Height: snapshot.Height}
	return d, d.Validate(m)
}

func (d ContentDescriptor) Validate(m Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if d.Version != 1 || len(m.Files) != 1 || d.EntryID != 0 || m.Files[0].Type != "file" || !validHex(d.Digest, 32) || d.Size <= 0 {
		return ErrContentInvalid
	}
	if d.Size != m.Files[0].Size || d.Digest != m.Files[0].Hash {
		return ErrContentMismatch
	}
	switch d.Kind {
	case content.Text, content.URL:
		if d.MediaType != "text/plain; charset=utf-8" || d.Size > content.MaxTextBytes || d.Width != 0 || d.Height != 0 {
			return ErrContentInvalid
		}
	case content.Image:
		if d.MediaType != "image/png" || d.Size > content.MaxImageBytes || content.ValidateDimensions(d.Width, d.Height) != nil {
			return ErrContentInvalid
		}
	default:
		return ErrContentUnsupported
	}
	return nil
}

// BindingDigest includes interpretation and the original manifest in a
// domain-separated BLAKE3 digest. Selection has its own M4 terminal binding.
func (d ContentDescriptor) BindingDigest(m Manifest) string {
	b, _ := json.Marshal(struct {
		Domain         string            `json:"domain"`
		ManifestDigest string            `json:"manifest_digest"`
		Descriptor     ContentDescriptor `json:"descriptor"`
	}{"LinkSend/content_v1", m.Digest(), d})
	return Sum(b)
}

func cloneContent(d *ContentDescriptor) *ContentDescriptor {
	if d == nil {
		return nil
	}
	copy := *d
	return &copy
}

func hasCapability(capabilities []string, wanted string) bool {
	for _, c := range capabilities {
		if c == wanted {
			return true
		}
	}
	return false
}

func validateContentBody(ctx context.Context, r io.Reader, d ContentDescriptor, outgoing bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(r, d.Size+1))
	if err != nil {
		return err
	}
	if int64(len(data)) != d.Size || Sum(data) != d.Digest {
		return ErrContentMismatch
	}
	switch d.Kind {
	case content.Text, content.URL:
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return ErrContentInvalid
		}
		if outgoing && d.Kind == content.URL && content.ValidateURL(string(data)) != nil {
			return ErrContentInvalid
		}
	case content.Image:
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width != d.Width || config.Height != d.Height {
			return ErrContentMismatch
		}
		// A plausible PNG header alone must not allow a truncated/corrupt body.
		if _, err = png.Decode(bytes.NewReader(data)); err != nil {
			return ErrContentInvalid
		}
	default:
		return ErrContentUnsupported
	}
	return ctx.Err()
}

func (p *Prepared) validateContent(ctx context.Context, d *ContentDescriptor) error {
	if d == nil {
		return nil
	}
	if err := d.Validate(p.Manifest); err != nil {
		return err
	}
	src, ok := p.sources[d.EntryID]
	if !ok {
		return ErrChanged
	}
	f, err := src.root.Open(src.relative)
	if err != nil {
		return err
	}
	defer f.Close()
	return validateContentBody(ctx, f, *d, true)
}

func (r *Receiver) validateContent(ctx context.Context, d *ContentDescriptor) error {
	if d == nil {
		return nil
	}
	if _, selected := r.selected[d.EntryID]; !selected {
		return nil
	}
	if record, committed := r.State.Committed[d.EntryID]; committed {
		f, err := r.root.Open(record.Path)
		if err != nil {
			return err
		}
		defer f.Close()
		return validateContentBody(ctx, f, *d, false)
	}
	f := r.files[d.EntryID]
	if f == nil {
		return ErrContentMismatch
	}
	return validateContentBody(ctx, io.NewSectionReader(f, 0, d.Size+1), *d, false)
}

func validateContentControl(c control) error {
	if c.AllowFileFallback && (c.Op != opOffer || c.Content == nil) {
		return ErrContentInvalid
	}
	if c.Content != nil {
		if c.Op != opOffer || c.Manifest == nil || !hasCapability(c.Capabilities, CapabilityContent) {
			return ErrContentInvalid
		}
		if err := c.Content.Validate(*c.Manifest); err != nil {
			return err
		}
		if c.ContentDigest != c.Content.BindingDigest(*c.Manifest) {
			return ErrContentMismatch
		}
	} else if c.Op == opOffer && c.ContentDigest != "" {
		return ErrContentInvalid
	}
	if c.ContentDigest != "" {
		if !validHex(c.ContentDigest, 32) {
			return ErrContentInvalid
		}
		switch c.Op {
		case opOffer, opAccept, opAccepted, opFinish, opCompleted, opConfirmed, opConfirmedAck:
		default:
			return ErrContentInvalid
		}
	}
	if c.Op == opAccept && ((c.ContentDigest != "") != hasCapability(c.Capabilities, CapabilityContent)) {
		return ErrContentInvalid
	}
	return nil
}

func contentResult(result Result, d *ContentDescriptor, digest string, fallback bool) Result {
	result.Content = cloneContent(d)
	result.ContentDigest = digest
	result.FileFallback = fallback
	return result
}

// ContentV1/ is a critical checkpoint marker: older readers must reject the
// state instead of dropping an unknown optional content binding on resume.
func validateContentCheckpoint(saved *ResumeState, expected string) error {
	marked := strings.HasPrefix(saved.State, "ContentV1/")
	if marked != (saved.ContentDigest != "") || saved.ContentDigest != expected {
		return ErrContentMismatch
	}
	if marked {
		saved.State = strings.TrimPrefix(saved.State, "ContentV1/")
	}
	return nil
}
