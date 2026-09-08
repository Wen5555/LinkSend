package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/app"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Println("devtool commands: check-core, demo-local, test-nat, bench-transport, network-info, desktop-dev, desktop-build")
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
	case "network-info":
		err = networkInfo()
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

type interfaceInfo struct {
	Name      string   `json:"name"`
	Index     int      `json:"index"`
	Up        bool     `json:"up"`
	Loopback  bool     `json:"loopback"`
	MTU       int      `json:"mtu"`
	Addresses []string `json:"addresses"`
}

func networkInfo() error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	result := make([]interfaceInfo, 0, len(interfaces))
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return err
		}
		values := make([]string, 0, len(addrs))
		for _, addr := range addrs {
			values = append(values, addr.String())
		}
		result = append(result, interfaceInfo{Name: iface.Name, Index: iface.Index, Up: iface.Flags&net.FlagUp != 0, Loopback: iface.Flags&net.FlagLoopback != 0, MTU: iface.MTU, Addresses: values})
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func checkCore() error {
	if err := runGo("test", "./..."); err != nil {
		return fmt.Errorf("core tests failed: %w", err)
	}
	if err := runGo("vet", "./..."); err != nil {
		return fmt.Errorf("core vet failed: %w", err)
	}
	env, err := raceEnvironment()
	if err != nil {
		return err
	}
	if err = runCommand("go", []string{"test", "-race", "./..."}, env); err != nil {
		return fmt.Errorf("core race tests failed: %w", err)
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
	return runCommand("go", args, nil)
}

func runCommand(name string, args []string, env []string) error {
	cmd := exec.Command(name, args...)
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func raceEnvironment() ([]string, error) {
	env := os.Environ()
	if runtime.GOOS != "windows" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.New("race tests require a usable C toolchain; cannot determine user home")
	}
	bin := filepath.Join(home, "scoop", "apps", "mingw", "current", "bin")
	gcc := filepath.Join(bin, "gcc.exe")
	gxx := filepath.Join(bin, "g++.exe")
	if _, err = os.Stat(gcc); err != nil {
		return nil, fmt.Errorf("race tests require modern MinGW; install it or set CC/CXX explicitly (expected %s)", gcc)
	}
	if _, err = os.Stat(gxx); err != nil {
		return nil, fmt.Errorf("race tests require modern MinGW C++; expected %s", gxx)
	}
	env = setEnv(env, "CC", gcc)
	env = setEnv(env, "CXX", gxx)
	pathValue := lookupEnv(env, "Path")
	if pathValue == "" {
		pathValue = lookupEnv(env, "PATH")
	}
	env = setEnv(env, "Path", bin+string(os.PathListSeparator)+pathValue)
	return env, nil
}

func lookupEnv(env []string, name string) string {
	prefix := name + "="
	for _, item := range env {
		if len(item) >= len(prefix) && strings.EqualFold(item[:len(prefix)-1], name) && item[len(prefix)-1] == '=' {
			return item[len(prefix):]
		}
	}
	return ""
}

func setEnv(env []string, name, value string) []string {
	prefix := name + "="
	for i, item := range env {
		if len(item) >= len(prefix) && strings.EqualFold(item[:len(prefix)-1], name) && item[len(prefix)-1] == '=' {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
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
