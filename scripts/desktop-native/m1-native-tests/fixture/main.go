package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
)

func main() {
	rootFlag := flag.String("root", "", "isolated fixture directory")
	flag.Parse()
	if err := run(*rootFlag); err != nil {
		fmt.Fprintln(os.Stderr, "FIXTURE_FAILED:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	if !filepath.IsAbs(root) {
		return fmt.Errorf("absolute fixture root required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	bootstrap := protocol.RandomID() + protocol.RandomID()
	srv, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "control.sqlite"), BootstrapToken: bootstrap, AllowInsecureLoopback: true, AllowLoopbackCandidates: true})
	if err != nil {
		return err
	}
	defer srv.Close()
	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()
	profileA, profileB := filepath.Join(root, "profile-a"), filepath.Join(root, "profile-b")
	a, err := app.New(app.Config{DataDir: profileA, ServerURL: httpServer.URL, AllowInsecureLoopback: true, Name: "M1 本机验收"})
	if err != nil {
		return err
	}
	defer a.Shutdown()
	b, err := app.New(app.Config{DataDir: profileB, ServerURL: httpServer.URL, AllowInsecureLoopback: true, Name: "M1 原生对端"})
	if err != nil {
		return err
	}
	defer b.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err = a.Bootstrap(ctx, bootstrap, "M1 本机验收"); err != nil {
		return err
	}
	invitation, err := a.CreateInvitation(ctx)
	if err != nil {
		return err
	}
	if _, err = b.Join(ctx, invitation.Token, "M1 原生对端"); err != nil {
		return err
	}
	if _, err = a.Devices(ctx); err != nil {
		return err
	}
	peerID := b.Identity().ID
	a.Shutdown()
	b.Shutdown() // The GUI must be the sole writer when it opens profile A.
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	excluded := []string{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback == 0 {
			excluded = append(excluded, iface.Name)
		}
	}
	source := filepath.Join(root, "中文 验收 文件.txt")
	if err = os.WriteFile(source, []byte("LinkSend M1 native UI acceptance fixture\n"), 0600); err != nil {
		return err
	}
	receive := filepath.Join(root, "接收 目录")
	peerReceive := filepath.Join(root, "对端 专属目录")
	for _, directory := range []string{receive, peerReceive} {
		if err = os.MkdirAll(directory, 0700); err != nil {
			return err
		}
	}
	preferences := map[string]any{"format_version": 1, "server_url": httpServer.URL, "bind_address": "127.0.0.1:0", "interface_priority": []string{}, "excluded_interfaces": excluded, "stun_urls": []string{}, "receive_directory": receive, "device_name": "M1 本机验收"}
	encoded, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(profileA, "desktop-preferences.json"), encoded, 0600); err != nil {
		return err
	}
	info := map[string]any{"server_url": httpServer.URL, "profile_a": profileA, "profile_b": profileB, "peer_id": peerID, "peer_name": "M1 原生对端", "source": source, "receive_directory": receive, "peer_receive_directory": peerReceive, "profile_locks_released": true, "excluded_non_loopback_interfaces": excluded}
	encoded, err = json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "fixture.json"), encoded, 0600); err != nil {
		return err
	}
	fmt.Println("FIXTURE_READY: real loopback pairing complete; GUI profile owner released")
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		if _, err = os.Stat(filepath.Join(root, "stop-fixture")); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}
