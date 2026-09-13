package app

import (
	"errors"
	"testing"

	"github.com/Wen5555/LinkSend/internal/identity"
)

func TestClipboardGrantsDefaultOffCASAndRevocation(t *testing.T) {
	dir := t.TempDir()
	service, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	peer, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.TrustPairedPeer(dir, identity.TrustedPeer{ID: peer.ID(), Name: "peer", PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	grants, err := service.ClipboardGrants(t.Context(), peer.ID())
	if err != nil || len(grants) != 0 {
		t.Fatalf("new peer inherited clipboard permission: %+v %v", grants, err)
	}
	send, err := service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: true})
	if err != nil || !send.Enabled || send.Revision != 1 {
		t.Fatalf("enable send: %+v %v", send, err)
	}
	if _, err = service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: false}); !errors.Is(err, ErrMetadataConflict) {
		t.Fatal("stale clipboard grant update accepted:", err)
	}
	grants, err = service.ClipboardGrants(t.Context(), peer.ID())
	if err != nil || len(grants) != 1 || grants[0].Direction != "send" || grants[0].Kind != "text" {
		t.Fatalf("direction/type permissions were conflated: %+v %v", grants, err)
	}
	if err = service.BlockPeer(peer.ID()); err != nil {
		t.Fatal(err)
	}
	grants, err = service.ClipboardGrants(t.Context(), peer.ID())
	if err != nil || len(grants) != 0 {
		t.Fatalf("revocation retained clipboard grant: %+v %v", grants, err)
	}
	if _, err = service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: true}); err == nil {
		t.Fatal("blocked peer regained clipboard permission")
	}
}

func TestClipboardGrantValidation(t *testing.T) {
	service, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	for _, patch := range []ClipboardGrantPatch{
		{}, {PeerID: "peer", Direction: "both", Kind: "text"}, {PeerID: "peer", Direction: "send", Kind: "file"},
	} {
		if _, err := service.SetClipboardGrant(t.Context(), patch); err == nil {
			t.Fatalf("invalid clipboard grant accepted: %+v", patch)
		}
	}
}
