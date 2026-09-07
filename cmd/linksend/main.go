package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"example.com/linksend/internal/app"
	"example.com/linksend/internal/transfer"
)

const usage = `LinkSend CLI

Usage:
  linksend [global flags] <command> [command flags]

Commands:
  identity       show the local device identity
  health         check signaling capabilities
  bootstrap      create the first administrator device
  invite         create a one-time pairing invitation
  join           join a device group with an invitation
  devices        list current group devices and trust state
  trust          pin a device after out-of-band fingerprint verification
  revoke         revoke a group device
  diagnostics    print a redacted diagnostic report
  send           send files or folders over an authenticated direct session
  receive        wait for one incoming transfer and ask for consent
  accept         not implemented; receiver consent control is not exposed yet
  reject         not implemented; receiver consent control is not exposed yet
  status         not implemented; task status is not exposed yet
  resume         not implemented; transfer resume is not exposed yet
  cancel         not implemented; transfer cancellation is not exposed yet

Global flags:
  --server URL              signaling server URL
  --data-dir DIR            isolated profile directory
  --name NAME               local display name
  --allow-insecure-loopback allow http/ws on literal loopback only
`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Print(usage)
		return
	}
	global := flag.NewFlagSet("linksend", flag.ContinueOnError)
	global.SetOutput(os.Stderr)
	serverURL := global.String("server", "", "signaling server URL")
	dataDir := global.String("data-dir", defaultDataDir(), "isolated profile directory")
	name := global.String("name", "", "local display name")
	allowLoopback := global.Bool("allow-insecure-loopback", false, "allow loopback HTTP/WSS development mode")
	if err := global.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	args := global.Args()
	if len(args) == 0 {
		fmt.Print(usage)
		return
	}
	command := args[0]
	ctx, cancel := signalContext()
	defer cancel()
	svc, err := app.New(app.Config{DataDir: app.DataPath(*dataDir), ServerURL: *serverURL, Name: *name, AllowInsecureLoopback: *allowLoopback})
	if err != nil {
		fatal(err)
	}
	if err = run(ctx, svc, command, args[1:]); err != nil {
		fatal(err)
	}
}

func run(ctx context.Context, svc *app.Service, command string, args []string) error {
	if command != "send" && command != "receive" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	switch command {
	case "identity":
		return printJSON(svc.Identity())
	case "health":
		caps, err := svc.Health(ctx)
		if err != nil {
			return err
		}
		return printJSON(caps)
	case "bootstrap":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		token := fs.String("token", os.Getenv("LINKSEND_BOOTSTRAP_TOKEN"), "high-entropy bootstrap token")
		deviceName := fs.String("name", "", "administrator display name")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if len(*token) < 32 {
			return errors.New("bootstrap token must contain at least 32 characters")
		}
		d, err := svc.Bootstrap(ctx, *token, *deviceName)
		if err != nil {
			return err
		}
		return printJSON(d)
	case "invite":
		i, err := svc.CreateInvitation(ctx)
		if err != nil {
			return err
		}
		return printJSON(i)
	case "join":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		token := fs.String("token", "", "single-use invitation token")
		deviceName := fs.String("name", "", "device display name")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *token == "" {
			return errors.New("join requires --token")
		}
		d, err := svc.Join(ctx, *token, *deviceName)
		if err != nil {
			return err
		}
		return printJSON(d)
	case "devices":
		devices, err := svc.Devices(ctx)
		if err != nil {
			return err
		}
		return printJSON(devices)
	case "trust":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		id := fs.String("id", "", "device ID")
		fingerprint := fs.String("fingerprint", "", "full fingerprint confirmed out of band")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *id == "" || *fingerprint == "" {
			return errors.New("trust requires --id and --fingerprint")
		}
		return svc.Trust(ctx, *id, *fingerprint)
	case "revoke":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		id := fs.String("id", "", "device ID")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *id == "" {
			return errors.New("revoke requires --id")
		}
		return svc.Revoke(ctx, *id)
	case "diagnostics":
		return printJSON(svc.Diagnostics(ctx))
	case "send":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		peer := fs.String("peer", "", "trusted peer device ID")
		bind := fs.String("bind", "", "concrete local IP:port for ICE")
		stun := fs.String("stun", "", "comma-separated stun: URLs")
		allowLoopback := fs.Bool("allow-loopback", false, "allow loopback candidates for a local fixture")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *peer == "" || len(fs.Args()) == 0 {
			return errors.New("send requires --peer and at least one file or directory")
		}
		result, err := svc.SendFiles(ctx, *peer, fs.Args(), directConfig(*bind, *stun, *allowLoopback), func(p transfer.Progress) {
			fmt.Fprintf(os.Stderr, "progress state=%s verified=%d total=%d bps=%.1f\n", p.State, p.Verified, p.Total, p.BytesPerSecond)
		})
		if err != nil {
			return err
		}
		return printJSON(result)
	case "receive":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		peer := fs.String("peer", "", "expected trusted peer device ID (optional)")
		directory := fs.String("dir", "", "selected receive root directory")
		bind := fs.String("bind", "", "concrete local IP:port for ICE")
		stun := fs.String("stun", "", "comma-separated stun: URLs")
		allowLoopback := fs.Bool("allow-loopback", false, "allow loopback candidates for a local fixture")
		autoAccept := fs.Bool("auto-accept", false, "accept only with explicit unattended test opt-in")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *directory == "" {
			return errors.New("receive requires --dir")
		}
		accept := func(m transfer.Manifest) bool {
			if *autoAccept {
				return true
			}
			_ = printJSON(m)
			fmt.Fprint(os.Stderr, "Accept this transfer? [y/N] ")
			line, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
			return readErr == nil && strings.EqualFold(strings.TrimSpace(line), "y")
		}
		result, err := svc.ReceiveOnce(ctx, *peer, *directory, directConfig(*bind, *stun, *allowLoopback), accept, func(p transfer.Progress) {
			fmt.Fprintf(os.Stderr, "progress state=%s verified=%d total=%d bps=%.1f\n", p.State, p.Verified, p.Total, p.BytesPerSecond)
		})
		if err != nil {
			return err
		}
		return printJSON(result)
	case "accept":
		return app.ErrNotImplemented
	case "reject":
		return app.ErrNotImplemented
	case "status":
		return app.ErrNotImplemented
	case "resume":
		return app.ErrNotImplemented
	case "cancel":
		return app.ErrNotImplemented
	default:
		return fmt.Errorf("unknown command %q; use --help", command)
	}
}

func directConfig(bind, stun string, allowLoopback bool) app.DirectConfig {
	var urls []string
	for _, raw := range strings.Split(stun, ",") {
		if raw = strings.TrimSpace(raw); raw != "" {
			urls = append(urls, raw)
		}
	}
	return app.DirectConfig{BindAddress: bind, STUNURLs: urls, AllowLoopback: allowLoopback, CheckTimeout: 20 * time.Second}
}

func defaultDataDir() string {
	if root, err := os.UserConfigDir(); err == nil {
		return filepath.Join(root, "LinkSend")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "LinkSend")
	}
	return filepath.Join(os.TempDir(), "linksend")
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "linksend:", strings.TrimSpace(err.Error()))
	os.Exit(1)
}
