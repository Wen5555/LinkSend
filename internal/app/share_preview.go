package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/Wen5555/LinkSend/internal/transport"
)

const sharePreviewMagic = "LSSP01"
const maxSharePreview = transfer.MaxMetadata + 2048

// This metadata protocol runs only inside the separately pinned preview QUIC
// connection. It never opens a receiver, sends file body, or creates storage.
type sharePreviewFrame struct {
	Version    int                    `json:"version"`
	Type       string                 `json:"type"`
	Scope      sharegrant.SignalScope `json:"scope"`
	Manifest   *transfer.Manifest     `json:"manifest,omitempty"`
	PlanDigest string                 `json:"plan_digest,omitempty"`
	Code       protocol.Code          `json:"code,omitempty"`
}

func sharePreviewError() error {
	return protocol.Fail(protocol.ShareInvalid, "invalid encrypted share preview")
}

func previewDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) || value == strings.Repeat("0", 64) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func writeSharePreviewFrame(writer io.Writer, frame sharePreviewFrame, limit int) error {
	body, err := json.Marshal(frame)
	if err != nil || len(body) > limit || limit > maxSharePreview {
		return sharePreviewError()
	}
	header := make([]byte, len(sharePreviewMagic)+4)
	copy(header, sharePreviewMagic)
	binary.BigEndian.PutUint32(header[len(sharePreviewMagic):], uint32(len(body)))
	for _, part := range [][]byte{header, body} {
		for len(part) > 0 {
			n, writeErr := writer.Write(part)
			if writeErr != nil {
				return writeErr
			}
			if n <= 0 {
				return io.ErrShortWrite
			}
			part = part[n:]
		}
	}
	return nil
}

func readSharePreviewFrame(reader io.Reader, limit int) (sharePreviewFrame, error) {
	var frame sharePreviewFrame
	var header [len(sharePreviewMagic) + 4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return frame, err
	}
	length := binary.BigEndian.Uint32(header[len(sharePreviewMagic):])
	if string(header[:len(sharePreviewMagic)]) != sharePreviewMagic || length == 0 || limit <= 0 || limit > maxSharePreview || length > uint32(limit) {
		return frame, sharePreviewError()
	}
	body := make([]byte, int(length))
	if _, err := io.ReadFull(reader, body); err != nil {
		return frame, err
	}
	// Detect aliases before encoding/json's case-insensitive struct matching.
	unique := json.NewDecoder(bytes.NewReader(body))
	if uniquePreviewValue(unique, 0) != nil {
		return frame, sharePreviewError()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&frame) != nil || decoder.Decode(new(any)) != io.EOF || frame.Version != 1 || frame.Scope.Validate() != nil || frame.Scope.Phase != sharegrant.SignalPreview {
		return frame, sharePreviewError()
	}
	return frame, nil
}

func uniquePreviewValue(decoder *json.Decoder, depth int) error {
	if depth > 12 {
		return sharePreviewError()
	}
	token, err := decoder.Token()
	if err != nil {
		return sharePreviewError()
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	switch delimiter {
	case '{':
		var seen []string
		for decoder.More() {
			token, err = decoder.Token()
			key, ok := token.(string)
			if err != nil || !ok || len(key) > 128 || len(seen) >= 32 {
				return sharePreviewError()
			}
			for _, previous := range seen {
				if strings.EqualFold(previous, key) {
					return sharePreviewError()
				}
			}
			seen = append(seen, key)
			if uniquePreviewValue(decoder, depth+1) != nil {
				return sharePreviewError()
			}
		}
	case '[':
		for decoder.More() {
			if uniquePreviewValue(decoder, depth+1) != nil {
				return sharePreviewError()
			}
		}
	default:
		return sharePreviewError()
	}
	end, err := decoder.Token()
	if err != nil || delimiter == '{' && end != json.Delim('}') || delimiter == '[' && end != json.Delim(']') {
		return sharePreviewError()
	}
	return nil
}

func sendSharePreview(ctx context.Context, peer *PeerSession, scope sharegrant.SignalScope, manifest transfer.Manifest) (string, error) {
	if peer == nil || peer.Data == nil || scope.Validate() != nil || scope.Phase != sharegrant.SignalPreview || manifest.Validate() != nil || manifest.Digest() != scope.Manifest || manifest.TransferID != scope.TaskID {
		return "", sharePreviewError()
	}
	stream, err := peer.Data.Conn.OpenStreamSync(ctx)
	if err != nil {
		return "", err
	}
	wrapped := transport.WrapStream(stream)
	stop := context.AfterFunc(ctx, wrapped.Abort)
	defer stop()
	completed := false
	defer func() {
		if !completed {
			wrapped.Abort()
		}
	}()
	if err = writeSharePreviewFrame(wrapped, sharePreviewFrame{Version: 1, Type: "manifest", Scope: scope, Manifest: &manifest}, maxSharePreview); err != nil {
		return "", err
	}
	response, err := readSharePreviewFrame(wrapped, 4096)
	if err != nil {
		return "", err
	}
	if response.Scope != scope || response.Manifest != nil {
		return "", sharePreviewError()
	}
	if response.Type == "rejected" && response.Code == protocol.ReceiveRejected && response.PlanDigest == "" {
		return "", protocol.Fail(protocol.ReceiveRejected, "temporary share save was not confirmed")
	}
	if response.Type != "accepted" || response.Code != "" || !previewDigest(response.PlanDigest) {
		return "", sharePreviewError()
	}
	if err = writeSharePreviewFrame(wrapped, sharePreviewFrame{Version: 1, Type: "confirmed", Scope: scope, PlanDigest: response.PlanDigest}, 4096); err != nil {
		return "", err
	}
	if err = wrapped.FlushTerminal(ctx); err != nil {
		return "", err
	}
	completed = true
	return response.PlanDigest, nil
}

func receiveSharePreview(ctx context.Context, peer *PeerSession, scope sharegrant.SignalScope, confirm func(context.Context, transfer.Manifest) (transfer.ReceivePlan, error)) (transfer.Manifest, transfer.ReceivePlan, error) {
	var manifest transfer.Manifest
	var plan transfer.ReceivePlan
	if peer == nil || peer.Data == nil || scope.Validate() != nil || scope.Phase != sharegrant.SignalPreview || confirm == nil {
		return manifest, plan, sharePreviewError()
	}
	stream, err := peer.Data.Conn.AcceptStream(ctx)
	if err != nil {
		return manifest, plan, err
	}
	wrapped := transport.WrapStream(stream)
	stop := context.AfterFunc(ctx, wrapped.Abort)
	defer stop()
	completed := false
	defer func() {
		if !completed {
			wrapped.Abort()
		}
	}()
	offer, err := readSharePreviewFrame(wrapped, maxSharePreview)
	if err != nil {
		return manifest, plan, err
	}
	if offer.Type != "manifest" || offer.Scope != scope || offer.Manifest == nil || offer.PlanDigest != "" || offer.Code != "" {
		return manifest, plan, sharePreviewError()
	}
	manifest = *offer.Manifest
	if manifest.Validate() != nil || manifest.Digest() != scope.Manifest || manifest.TransferID != scope.TaskID {
		return transfer.Manifest{}, plan, sharePreviewError()
	}
	plan, err = confirm(ctx, manifest)
	if err != nil {
		_ = writeSharePreviewFrame(wrapped, sharePreviewFrame{Version: 1, Type: "rejected", Scope: scope, Code: protocol.ReceiveRejected}, 4096)
		_ = wrapped.FlushTerminal(ctx)
		return manifest, transfer.ReceivePlan{}, err
	}
	if plan.Validate(manifest) != nil || plan.Selection().Digest != transfer.FullReceivePlan(manifest).Selection().Digest {
		return manifest, transfer.ReceivePlan{}, protocol.Fail(protocol.ReceivePlanMismatch, "temporary share requires its complete original manifest")
	}
	digest := plan.Digest()
	if err = writeSharePreviewFrame(wrapped, sharePreviewFrame{Version: 1, Type: "accepted", Scope: scope, PlanDigest: digest}, 4096); err != nil {
		return manifest, plan, err
	}
	confirmed, err := readSharePreviewFrame(wrapped, 4096)
	if err != nil {
		return manifest, plan, err
	}
	if confirmed.Type != "confirmed" || confirmed.Scope != scope || confirmed.Manifest != nil || confirmed.PlanDigest != digest || confirmed.Code != "" {
		return manifest, plan, sharePreviewError()
	}
	err = wrapped.FlushTerminal(ctx)
	completed = err == nil
	return manifest, plan, err
}
