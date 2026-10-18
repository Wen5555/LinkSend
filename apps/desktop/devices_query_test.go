package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/server"
)

func TestDevicesReadDoesNotEnterSystemShareStorage(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent outbox", true: "unavailable outbox"}[blocked], func(t *testing.T) {
			root := t.TempDir()
			srv, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "control.db"), AllowInsecureLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			defer srv.Close()
			h := httptest.NewServer(srv.Handler())
			defer h.Close()
			profile := filepath.Join(root, "profile")
			core, err := coreapp.New(coreapp.Config{DataDir: profile, ServerURL: h.URL, AllowInsecureLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Shutdown()
			peer, err := coreapp.New(coreapp.Config{DataDir: filepath.Join(root, "peer"), ServerURL: h.URL, AllowInsecureLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Shutdown()
			if _, err = core.InitializeMembership(t.Context(), "desktop"); err != nil {
				t.Fatal(err)
			}
			invite, err := core.CreateInvitation(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = peer.Join(t.Context(), invite.Token, "paired peer"); err != nil {
				t.Fatal(err)
			}
			outbox := filepath.Join(profile, "native-share-v1")
			if blocked {
				if err = os.WriteFile(outbox, []byte("preserve unavailable outbox"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			a := &App{ctx: t.Context(), core: core, dataDir: profile, entries: &desktopEntries{status: DesktopEntryStatus{Error: "existing entry failure"}}}
			devices, err := a.Devices()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, device := range devices {
				if device.ID == peer.Identity().ID && device.Trusted {
					found = true
				}
			}
			if !found {
				t.Fatal("paired directory did not return its trusted peer")
			}
			if a.entries.status.Error != "existing entry failure" {
				t.Fatalf("device read touched unrelated entry storage/status: %q", a.entries.status.Error)
			}
			if !blocked {
				if _, err = os.Stat(outbox); !os.IsNotExist(err) {
					t.Fatalf("device read opened/created the system-share namespace: %v", err)
				}
			}
		})
	}
}
