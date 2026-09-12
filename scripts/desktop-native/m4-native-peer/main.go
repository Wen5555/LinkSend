package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/transfer"
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
	if len(os.Args) != 3 {
		return fmt.Errorf("fixture path and scenario required")
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
	scenario := os.Args[2]
	if scenario != "subset" && scenario != "allskip" {
		return fmt.Errorf("bad scenario")
	}
	source := filepath.Join(root, "m4-source-"+scenario)
	if err = os.MkdirAll(source, 0700); err != nil {
		return err
	}
	paths := []string{}
	for _, name := range []string{"A 中文 冲突.txt", "B 跳过.txt", "C 正常.txt"} {
		p := filepath.Join(source, name)
		if err = os.WriteFile(p, []byte("native M4 "+scenario+" "+name+"\n"), 0600); err != nil {
			return err
		}
		paths = append(paths, p)
	}
	empty := filepath.Join(source, "D 空目录")
	if err = os.MkdirAll(empty, 0700); err != nil {
		return err
	}
	paths = append(paths, empty)
	svc, err := app.New(app.Config{DataDir: f.Profile, ServerURL: f.Server, AllowInsecureLoopback: true, Name: "M4 native peer"})
	if err != nil {
		return err
	}
	defer svc.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	devices, err := svc.Devices(ctx)
	if err != nil {
		return err
	}
	target := ""
	for _, d := range devices {
		if d.ID != svc.Identity().ID {
			target = d.ID
		}
	}
	if target == "" {
		return fmt.Errorf("missing target")
	}
	prepared, err := transfer.Prepare(ctx, paths, transfer.DefaultChunkSize)
	if err != nil {
		return err
	}
	defer prepared.Close()
	cfg := app.DirectConfig{BindAddress: "127.0.0.1:0", ExcludedInterfaces: f.Excluded, STUNURLs: []string{"stun:127.0.0.1:9"}, AllowLoopback: true, CheckTimeout: 10 * time.Second, WaitTimeout: time.Minute}
	result, sendErr := svc.SendPreparedWithHooksDetailed(ctx, target, prepared, cfg, transfer.SendHooks{})
	data, err := json.MarshalIndent(struct {
		Result app.DirectTransferResult `json:"result"`
		Error  string                   `json:"error"`
	}{result, fmt.Sprint(sendErr)}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "m4-peer-"+scenario+"-result.json"), data, 0600); err != nil {
		return err
	}
	return sendErr
}
