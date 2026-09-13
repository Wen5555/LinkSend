package clipboardsync

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestLeaseAndEventWireRoundTrip(t *testing.T) {
	lease := Message{Type: "lease", LeaseID: "lease", SessionID: "session", Generation: 3, Kinds: []Kind{Text, Image}, TTLMillis: 10_000}
	var wire bytes.Buffer
	if err := Write(&wire, lease, nil); err != nil {
		t.Fatal(err)
	}
	got, payload, err := Read(&wire)
	if err != nil || got.LeaseID != lease.LeaseID || len(payload) != 0 {
		t.Fatal(got, payload, err)
	}
	event := Event{LeaseID: "lease", OriginID: "peer", Boot: "boot", OriginSeq: 2, Lamport: 4, Kind: Text, OSGeneration: 9, SenderGrantRevision: 1, ReceiverGrantRevision: 1, Payload: []byte("hello")}
	event.Digest = eventDigest(event)
	message := Message{Type: "event", LeaseID: event.LeaseID, SessionID: "session", Generation: 3, OriginID: event.OriginID, Boot: event.Boot, OriginSeq: event.OriginSeq, Lamport: event.Lamport, Kind: event.Kind, Digest: event.Digest, OSGeneration: event.OSGeneration, SenderGrantRevision: event.SenderGrantRevision, ReceiverGrantRevision: event.ReceiverGrantRevision, PayloadBytes: uint32(len(event.Payload))}
	wire.Reset()
	if err = Write(&wire, message, event.Payload); err != nil {
		t.Fatal(err)
	}
	got, payload, err = Read(&wire)
	if err != nil || got.Digest != event.Digest || !bytes.Equal(payload, event.Payload) {
		t.Fatal(got, payload, err)
	}
}

func TestWireRejectsOversizeTruncatedAndReboundEvent(t *testing.T) {
	var prefix bytes.Buffer
	prefix.Write(wireMagic[:])
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], MaxHeaderBytes+1)
	prefix.Write(size[:])
	if _, _, err := Read(&prefix); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	event := Event{LeaseID: "old", OriginID: "peer", Boot: "boot", OriginSeq: 1, Lamport: 1, Kind: Text, OSGeneration: 2, SenderGrantRevision: 1, ReceiverGrantRevision: 1, Payload: []byte("body")}
	event.Digest = eventDigest(event)
	message := Message{Type: "event", LeaseID: "new", SessionID: "session", Generation: 1, OriginID: event.OriginID, Boot: event.Boot, OriginSeq: event.OriginSeq, Lamport: event.Lamport, Kind: event.Kind, Digest: event.Digest, OSGeneration: event.OSGeneration, SenderGrantRevision: event.SenderGrantRevision, ReceiverGrantRevision: event.ReceiverGrantRevision, PayloadBytes: uint32(len(event.Payload))}
	if err := Write(&bytes.Buffer{}, message, event.Payload); !errors.Is(err, ErrStale) {
		t.Fatal("rebound event accepted", err)
	}
	message.LeaseID = "old"
	var wire bytes.Buffer
	if err := Write(&wire, message, event.Payload); err != nil {
		t.Fatal(err)
	}
	data := wire.Bytes()
	if _, _, err := Read(bytes.NewReader(data[:len(data)-1])); err == nil {
		t.Fatal("truncated payload accepted")
	}
}

func TestWireRejectsAmbiguousGrantsAndRevisionTampering(t *testing.T) {
	lease := Message{Type: "lease", LeaseID: "lease", SessionID: "session", Generation: 1, Kinds: []Kind{Text}, Grants: []Grant{{Kind: Text, Revision: 1}}, TTLMillis: 1000}
	if err := Write(&bytes.Buffer{}, lease, nil); !errors.Is(err, ErrStale) {
		t.Fatal("mixed legacy and revision grants accepted", err)
	}
	lease.Kinds = nil
	lease.Grants = []Grant{{Kind: Text, Revision: 1}, {Kind: Text, Revision: 2}}
	if err := Write(&bytes.Buffer{}, lease, nil); !errors.Is(err, ErrStale) {
		t.Fatal("duplicate grant accepted", err)
	}
	event := Event{LeaseID: "lease", OriginID: "peer", Boot: "boot", OriginSeq: 1, Lamport: 1, Kind: Text, SenderGrantRevision: 2, ReceiverGrantRevision: 3, Payload: []byte("body")}
	event.Digest = eventDigest(event)
	message := Message{Type: "event", LeaseID: event.LeaseID, SessionID: "session", Generation: 1, OriginID: event.OriginID, Boot: event.Boot, OriginSeq: event.OriginSeq, Lamport: event.Lamport, Kind: event.Kind, Digest: event.Digest, SenderGrantRevision: event.SenderGrantRevision, ReceiverGrantRevision: event.ReceiverGrantRevision + 1, PayloadBytes: uint32(len(event.Payload))}
	if err := Write(&bytes.Buffer{}, message, event.Payload); !errors.Is(err, ErrStale) {
		t.Fatal("grant revision tampering accepted", err)
	}
}
