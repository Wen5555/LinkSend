package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"example.com/linksend/internal/app"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Println("devtool commands: check-core, demo-local, test-nat, bench-transport, desktop-dev, desktop-build")
		return
	}
	var err error
	switch os.Args[1] {
	case "check-core":
		err = checkCore()
	case "demo-local":
		err = demoLocal()
	case "test-nat":
		err = testNAT()
	case "bench-transport":
		err = runGo("test", "-run", "^$", "-bench", "BenchmarkTransport", "-benchtime=1x", "./internal/transport")
	case "desktop-dev":
		err = runWails("dev")
	case "desktop-build":
		err = runWails("build")
	default:
		err = fmt.Errorf("unknown devtool command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "devtool:", err)
		os.Exit(1)
	}
}

func checkCore() error {
	if err := runGo("test", "./..."); err != nil {
		return fmt.Errorf("core tests failed: %w", err)
	}
	if err := runGo("vet", "./..."); err != nil {
		return fmt.Errorf("core vet failed: %w", err)
	}
	return nil
}

func demoLocal() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	report, err := app.RunLocalDemo(ctx)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func testNAT() error {
	if runtime.GOOS != "linux" {
		return errors.New("NAT lab not run: controlled namespace/NAT fixture requires Linux privileges; no success is claimed")
	}
	return errors.New("NAT lab not run: namespace fixture is not enabled in this environment; use tests/natlab after provisioning a private Linux runner")
}

func runGo(args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runWails(args ...string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	desktop := filepath.Join(root, "apps", "desktop")
	if _, err = os.Stat(desktop); err != nil {
		return fmt.Errorf("desktop module unavailable: %w", err)
	}
	wails := "wails"
	if runtime.GOOS == "windows" {
		local := filepath.Join(root, ".tools", "bin", "wails.exe")
		if _, statErr := os.Stat(local); statErr == nil {
			wails = local
		}
	}
	cmd := exec.Command(wails, args...)
	cmd.Dir = desktop
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
