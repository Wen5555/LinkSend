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

	"github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

const usage = `LinkSend CLI

Usage:
  linksend [global flags] <command> [command flags]

Commands:
  version        print the LinkSend product version
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
  status         print persisted task snapshots (optionally one --task)
  resume         explicitly confirm and resume one recoverable task in this process
  cancel         not implemented across processes; use the owning desktop process

Global flags:
  --server URL              signaling server URL
  --data-dir DIR            isolated profile directory
  --name NAME               local display name
  --allow-insecure-loopback allow http/ws on literal loopback only

Task commands use one profile at a time. Do not run resume against a data
directory that is currently open in another LinkSend desktop/CLI process.
`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Print(usage)
		return
	}
	if os.Args[1] == "--version" || os.Args[1] == "-version" || os.Args[1] == "version" {
		fmt.Printf("LinkSend %s\n", protocol.ProductVersion)
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
	defer svc.Shutdown()
	if err = run(ctx, svc, command, args[1:]); err != nil {
		fatal(err)
	}
}

func run(ctx context.Context, svc *app.Service, command string, args []string) error {
	if command != "send" && command != "receive" && command != "resume" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	switch command {
	case "version":
		fmt.Printf("LinkSend %s\n", protocol.ProductVersion)
		return nil
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
		interfaces := fs.String("interface-priority", "", "comma-separated interface names in preferred order")
		excluded := fs.String("exclude-interface", "", "comma-separated interface names to exclude")
		stun := fs.String("stun", "", "comma-separated stun: URLs")
		allowLoopback := fs.Bool("allow-loopback", false, "allow loopback candidates for a local fixture")
		evidence := fs.Bool("evidence", false, "include the selected direct path evidence in JSON output")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *peer == "" || len(fs.Args()) == 0 {
			return errors.New("send requires --peer and at least one file or directory")
		}
		result, err := svc.SendFilesDetailed(ctx, *peer, fs.Args(), directConfig(*bind, *stun, *interfaces, *excluded, *allowLoopback, 0), func(p transfer.Progress) {
			fmt.Fprintf(os.Stderr, "progress state=%s verified=%d total=%d bps=%.1f\n", p.State, p.Verified, p.Total, p.BytesPerSecond)
		})
		if err != nil {
			err = app.ClassifyError(err)
			if *evidence {
				_ = printJSON(map[string]any{"transfer": result.Transfer, "evidence": result.Evidence, "error": err.Error()})
			}
			return err
		}
		if *evidence {
			return printJSON(result)
		}
		return printJSON(result.Transfer)
	case "receive":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		peer := fs.String("peer", "", "expected trusted peer device ID (optional)")
		directory := fs.String("dir", "", "selected receive root directory")
		bind := fs.String("bind", "", "concrete local IP:port for ICE")
		interfaces := fs.String("interface-priority", "", "comma-separated interface names in preferred order")
		excluded := fs.String("exclude-interface", "", "comma-separated interface names to exclude")
		stun := fs.String("stun", "", "comma-separated stun: URLs")
		allowLoopback := fs.Bool("allow-loopback", false, "allow loopback candidates for a local fixture")
		evidence := fs.Bool("evidence", false, "include the selected direct path evidence in JSON output")
		autoAccept := fs.Bool("auto-accept", false, "accept only with explicit unattended test opt-in")
		waitTimeout := fs.Duration("wait-timeout", 0, "receiver request wait timeout (for example 30m)")
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
		result, err := svc.ReceiveOnceDetailed(ctx, *peer, *directory, directConfig(*bind, *stun, *interfaces, *excluded, *allowLoopback, *waitTimeout), accept, func(p transfer.Progress) {
			fmt.Fprintf(os.Stderr, "progress state=%s verified=%d total=%d bps=%.1f\n", p.State, p.Verified, p.Total, p.BytesPerSecond)
		})
		if err != nil {
			err = app.ClassifyError(err)
			if *evidence {
				_ = printJSON(map[string]any{"transfer": result.Transfer, "evidence": result.Evidence, "error": err.Error()})
			}
			return err
		}
		if *evidence {
			return printJSON(result)
		}
		return printJSON(result.Transfer)
	case "accept":
		return app.ErrNotImplemented
	case "reject":
		return app.ErrNotImplemented
	case "status":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		taskID := fs.String("task", "", "logical task ID (omit to list all tasks)")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *taskID == "" {
			return printJSON(svc.Tasks())
		}
		task, ok := svc.Task(*taskID)
		if !ok {
			return errors.New("TASK_NOT_FOUND")
		}
		return printJSON(task)
	case "resume":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		taskID := fs.String("task", "", "recoverable logical task ID")
		bind := fs.String("bind", "", "concrete local IP:port for ICE")
		interfaces := fs.String("interface-priority", "", "comma-separated interface names in preferred order")
		excluded := fs.String("exclude-interface", "", "comma-separated interface names to exclude")
		stun := fs.String("stun", "", "comma-separated stun: URLs")
		allowLoopback := fs.Bool("allow-loopback", false, "allow loopback candidates for a local fixture")
		waitTimeout := fs.Duration("wait-timeout", 0, "receiver request wait timeout")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *taskID == "" {
			return errors.New("resume requires --task")
		}
		started, err := svc.ResumeTask(*taskID, directConfig(*bind, *stun, *interfaces, *excluded, *allowLoopback, *waitTimeout))
		if err != nil {
			return err
		}
		return waitResumedTask(ctx, svc, started)
	case "cancel":
		return app.ErrNotImplemented
	default:
		return fmt.Errorf("unknown command %q; use --help", command)
	}
}

func waitResumedTask(ctx context.Context, svc *app.Service, started app.TaskSnapshot) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	lastRevision := uint64(0)
	for {
		task, ok := svc.Task(started.ID)
		if !ok {
			return errors.New("TASK_NOT_FOUND")
		}
		if task.AttemptID != started.AttemptID {
			return errors.New("TASK_ATTEMPT_REPLACED: a newer attempt owns this task")
		}
		if task.Revision != lastRevision {
			lastRevision = task.Revision
			fmt.Fprintf(os.Stderr, "task=%s attempt=%s state=%s phase=%s sent=%d received=%d retransmitted=%d verified=%d committed=%d\n", task.TaskID, task.AttemptID, task.State, task.Phase, task.SentBytes, task.ReceivedBytes, task.RetransmittedBytes, task.VerifiedBytes, task.CommittedBytes)
		}
		switch task.State {
		case "completed":
			return printJSON(task)
		case "rejected", "cancelled", "failed":
			_ = printJSON(task)
			if task.ErrorCode != "" {
				return protocol.Fail(protocol.Code(task.ErrorCode), task.ErrorMessage)
			}
			return fmt.Errorf("task ended in %s", task.State)
		}
		select {
		case <-ctx.Done():
			_ = svc.CancelTask(started.ID)
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func splitList(value string) []string {
	var result []string
	for _, raw := range strings.Split(value, ",") {
		if raw = strings.TrimSpace(raw); raw != "" {
			result = append(result, raw)
		}
	}
	return result
}

func directConfig(bind, stun, interfaces, excluded string, allowLoopback bool, waitTimeout time.Duration) app.DirectConfig {
	return app.DirectConfig{BindAddress: bind, InterfacePriority: splitList(interfaces), ExcludedInterfaces: splitList(excluded), STUNURLs: splitList(stun), AllowLoopback: allowLoopback, CheckTimeout: 20 * time.Second, WaitTimeout: waitTimeout}
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
	var protocolErr *protocol.Error
	if errors.As(err, &protocolErr) {
		fmt.Fprintf(os.Stderr, "linksend: %s: %s\n", protocolErr.Code, app.UserError(err))
		os.Exit(1)
	}
	if strings.Contains(err.Error(), "signaling HTTP") {
		fmt.Fprintln(os.Stderr, "linksend: 无法完成信令请求，请检查服务地址和网络后重试。")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "linksend:", strings.TrimSpace(err.Error()))
	os.Exit(1)
}
