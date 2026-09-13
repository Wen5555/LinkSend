package clipboardsync

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

const MaxHeaderBytes = 8 << 10

var wireMagic = [8]byte{'L', 'S', 'C', 'B', '0', '1', 0, 0}

type Message struct {
	Type         string `json:"type"`
	LeaseID      string `json:"lease_id"`
	SessionID    string `json:"session_id"`
	Generation   uint64 `json:"generation"`
	Kinds        []Kind `json:"kinds,omitempty"`
	TTLMillis    uint32 `json:"ttl_ms,omitempty"`
	OriginID     string `json:"origin_id,omitempty"`
	Boot         string `json:"boot,omitempty"`
	OriginSeq    uint64 `json:"origin_sequence,omitempty"`
	Lamport      uint64 `json:"lamport,omitempty"`
	Kind         Kind   `json:"kind,omitempty"`
	Digest       string `json:"digest,omitempty"`
	OSGeneration uint64 `json:"os_generation,omitempty"`
	PayloadBytes uint32 `json:"payload_bytes,omitempty"`
}

func Write(w io.Writer, message Message, payload []byte) error {
	if err := validateMessage(message, payload); err != nil {
		return err
	}
	header, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(header) > MaxHeaderBytes {
		return ErrLimit
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(header)))
	for _, part := range [][]byte{wireMagic[:], size[:], header, payload} {
		if _, err = writeAll(w, part); err != nil {
			return err
		}
	}
	return nil
}

func Read(r io.Reader) (Message, []byte, error) {
	message, err := ReadHeader(r)
	if err != nil {
		return Message{}, nil, err
	}
	payload, err := ReadPayload(r, message)
	return message, payload, err
}

func ReadHeader(r io.Reader) (Message, error) {
	var magic [8]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return Message{}, err
	}
	if magic != wireMagic {
		return Message{}, ErrStale
	}
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return Message{}, err
	}
	n := binary.BigEndian.Uint32(size[:])
	if n == 0 || n > MaxHeaderBytes {
		return Message{}, ErrLimit
	}
	header := make([]byte, n)
	if _, err := io.ReadFull(r, header); err != nil {
		return Message{}, err
	}
	var message Message
	if err := json.Unmarshal(header, &message); err != nil {
		return Message{}, err
	}
	if err := validateHeader(message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func ReadPayload(r io.Reader, message Message) ([]byte, error) {
	limit := payloadLimit(message.Kind)
	if uint64(message.PayloadBytes) > uint64(limit) {
		return nil, ErrLimit
	}
	payload := make([]byte, message.PayloadBytes)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	if err := validateMessage(message, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func validateHeader(m Message) error {
	if m.LeaseID == "" || m.SessionID == "" || m.Generation == 0 {
		return ErrStale
	}
	if m.Type == "lease" {
		if m.PayloadBytes != 0 || m.TTLMillis == 0 || m.TTLMillis > 10_000 || len(m.Kinds) == 0 || len(m.Kinds) > 3 {
			return ErrStale
		}
		return nil
	}
	if m.Type == "event" {
		if m.OriginID == "" || m.Boot == "" || m.OriginSeq == 0 || m.Lamport == 0 || !validKind(m.Kind) || len(m.Digest) != 64 {
			return ErrStale
		}
		return nil
	}
	return errors.New("CLIPBOARD_MESSAGE_UNSUPPORTED")
}

func validateMessage(m Message, payload []byte) error {
	if m.LeaseID == "" || m.SessionID == "" || m.Generation == 0 {
		return ErrStale
	}
	switch m.Type {
	case "lease":
		if len(payload) != 0 || m.TTLMillis == 0 || m.TTLMillis > 10000 || len(m.Kinds) == 0 || len(m.Kinds) > 3 {
			return ErrStale
		}
		for _, k := range m.Kinds {
			if !validKind(k) {
				return ErrStale
			}
		}
	case "event":
		if m.OriginID == "" || m.Boot == "" || m.OriginSeq == 0 || m.Lamport == 0 || !validKind(m.Kind) || int(m.PayloadBytes) != len(payload) {
			return ErrStale
		}
		if err := validatePayload(m.Kind, payload); err != nil {
			return err
		}
		event := Event{LeaseID: m.LeaseID, OriginID: m.OriginID, Boot: m.Boot, OriginSeq: m.OriginSeq, Lamport: m.Lamport, Kind: m.Kind, Digest: m.Digest, OSGeneration: m.OSGeneration, Payload: payload}
		if m.Digest != eventDigest(event) {
			return ErrStale
		}
	default:
		return errors.New("CLIPBOARD_MESSAGE_UNSUPPORTED")
	}
	return nil
}
func payloadLimit(kind Kind) int {
	if kind == Image {
		return MaxImageBytes
	}
	return MaxTextBytes
}
func writeAll(w io.Writer, p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		n, err := w.Write(p)
		total += n
		p = p[n:]
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}
