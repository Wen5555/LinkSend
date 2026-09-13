// control-inspect reads health, authenticates WSS, then reads presence.
// Use an isolated profile containing an authorized test identity; opening WSS
// replaces another connection of that identity. Never run alongside its GUI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func main() {
	profile := flag.String("profile", "", "isolated existing identity directory")
	server := flag.String("server", "", "verified HTTPS service")
	flag.Parse()
	if err := run(*profile, *server); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(profile, server string) error {
	if !filepath.IsAbs(profile) {
		return fmt.Errorf("absolute existing isolated profile required")
	}
	if _, err := os.Stat(filepath.Join(profile, "identity.key")); err != nil {
		return err
	}
	id, err := identity.LoadOrCreate(profile)
	if err != nil {
		return err
	}
	client, err := signaling.New(signaling.Config{ServerURL: server, Identity: id})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	encoder := json.NewEncoder(os.Stdout)
	health, err := client.Health(ctx)
	if err != nil {
		return err
	}
	if err = encoder.Encode(map[string]any{"stage": "health", "capabilities": health}); err != nil {
		return err
	}
	session, err := client.Connect(ctx)
	if err != nil {
		return err
	}
	defer session.Close()
	if err = encoder.Encode(map[string]any{"stage": "authenticated_wss", "self": id.ID()[:12]}); err != nil {
		return err
	}
	devices, err := client.Devices(ctx)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(devices))
	for _, d := range devices {
		rows = append(rows, map[string]any{"id_prefix": d.ID[:12], "online": d.Online, "self": d.ID == id.ID()})
	}
	return encoder.Encode(map[string]any{"stage": "authenticated_presence_snapshot", "devices": rows})
}
