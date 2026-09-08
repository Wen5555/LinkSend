package app

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/server"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func TestDirectServiceSendAndReceive(t *testing.T) {
	root := t.TempDir()
	token := "direct-service-test-token-012345678901234567890"
	srv, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "control.db"), BootstrapToken: token, AllowInsecureLoopback: true, AllowLoopbackCandidates: true})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := httptest.NewServer(srv.Handler())
	defer h.Close()
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	a, err := New(Config{DataDir: filepath.Join(root, "a"), ServerURL: h.URL, AllowInsecureLoopback: true, Identity: aID})
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(Config{DataDir: filepath.Join(root, "b"), ServerURL: h.URL, AllowInsecureLoopback: true, Identity: bID})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err = a.Bootstrap(ctx, token, "a"); err != nil {
		t.Fatal(err)
	}
	inv, err := a.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Join(ctx, inv.Token, "b"); err != nil {
		t.Fatal(err)
	}
	if err = a.Trust(ctx, bID.ID(), bID.ID()); err != nil {
		t.Fatal(err)
	}
	if err = b.Trust(ctx, aID.ID(), aID.ID()); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.txt")
	data := []byte("authenticated application service transfer")
	if err = os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "dest")
	acceptErr := make(chan error, 1)
	resultCh := make(chan DirectTransferResult, 1)
	go func() {
		result, receiveErr := b.ReceiveOnceDetailed(ctx, aID.ID(), dest, DirectConfig{AllowLoopback: true}, func(transfer.Manifest) bool { return true }, nil)
		if receiveErr != nil {
			acceptErr <- receiveErr
			return
		}
		resultCh <- result
		acceptErr <- nil
	}()
	sent, err := a.SendFilesDetailed(ctx, bID.ID(), []string{source}, DirectConfig{AllowLoopback: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-acceptErr; err != nil {
		t.Fatal(err)
	}
	received := <-resultCh
	if sent.Transfer.Digest != received.Transfer.Digest || received.Transfer.Bytes != int64(len(data)) {
		t.Fatalf("transfer mismatch: sent=%+v received=%+v", sent, received)
	}
	for _, evidence := range []DirectEvidence{sent.Evidence, received.Evidence} {
		if evidence.PeerID == "" || evidence.BaseSocket == "" || evidence.LocalCandidate == "" || evidence.RemoteCandidate == "" || evidence.TransportProtocol != "quic" || evidence.Relay || evidence.TLSVersion != 0x0304 || evidence.ALPN != identity.ALPN {
			t.Fatalf("incomplete direct evidence: %+v", evidence)
		}
	}
}
