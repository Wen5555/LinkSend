package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/content"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		return fmt.Errorf("fixture required")
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	var f struct {
		Server   string   `json:"server_url"`
		Profile  string   `json:"profile_b"`
		Excluded []string `json:"excluded_non_loopback_interfaces"`
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		return err
	}
	root := filepath.Dir(os.Args[1])
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	svc, err := app.New(app.Config{DataDir: f.Profile, ServerURL: f.Server, AllowInsecureLoopback: true, Name: "M5 native peer"})
	if err != nil {
		return err
	}
	defer svc.Shutdown()
	devices, err := svc.Devices(ctx)
	if err != nil {
		return err
	}
	target := ""
	for _, device := range devices {
		if device.ID != svc.Identity().ID {
			target = device.ID
		}
	}
	if target == "" {
		return fmt.Errorf("target missing")
	}
	svc.EnableNativeContentActions()
	if err = svc.SetAlwaysAccept(target, true); err != nil {
		return err
	}
	cfg := app.DirectConfig{BindAddress: "127.0.0.1:0", ExcludedInterfaces: f.Excluded, STUNURLs: []string{"stun:127.0.0.1:9"}, AllowLoopback: true, CheckTimeout: 10 * time.Second, WaitTimeout: time.Minute}
	receiveDir := filepath.Join(root, "m5-peer-received")
	if err = os.MkdirAll(receiveDir, 0700); err != nil {
		return err
	}
	if err = svc.StartInbox(receiveDir, cfg); err != nil {
		return err
	}
	if err = svc.StartQueue(cfg); err != nil {
		return err
	}
	defer func() {
		encoded, _ := json.MarshalIndent(svc.Tasks(), "", "  ")
		_ = os.WriteFile(filepath.Join(root, "m5-peer-tasks.json"), encoded, 0600)
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	if len(os.Args) == 3 {
		if os.Args[2] != "receive-only" {
			return fmt.Errorf("unknown mode")
		}
		for {
			if _, err = os.Stat(filepath.Join(root, "m5-peer-finish")); err == nil {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "m5-peer-send-start")); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	for _, kind := range []content.Kind{content.Text, content.URL, content.Image} {
		var draft app.ContentDraft
		if kind == content.Image {
			draft, err = svc.CreateClipboardImage(ctx, "native-peer-image", func(context.Context) (image.Image, error) {
				img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
				for y := 0; y < 2; y++ {
					for x := 0; x < 3; x++ {
						img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 80), G: uint8(y * 120), B: 90, A: 255})
					}
				}
				return img, nil
			})
		} else {
			value := "M5 原生文字验收\n精确快照，不自动复制。"
			if kind == content.URL {
				value = f.Server + "/healthz"
			}
			draft, err = svc.CreateContentText(ctx, app.ContentTextRequest{RequestID: "native-peer-" + string(kind), Kind: kind, Text: value})
		}
		if err != nil {
			return err
		}
		item, err := svc.EnqueueContent(ctx, app.EnqueueContentRequest{RequestID: "native-queue-" + string(kind), DraftID: draft.ID, DraftRevision: draft.Revision, PeerID: target, WaitForPeer: true})
		if err != nil {
			return err
		}
		for {
			workspace, err := svc.Workspace()
			if err != nil {
				return err
			}
			complete := false
			for _, q := range workspace.Queue {
				if q.ID == item.ID {
					if q.State == "completed" {
						complete = true
					}
					if q.State == "needs_attention" {
						return fmt.Errorf("content send: %s", q.LastError)
					}
				}
			}
			if complete {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
	}
	return nil
}
