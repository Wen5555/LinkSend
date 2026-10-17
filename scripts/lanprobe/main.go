// lanprobe measures production LAN discovery and pinned TLS control only.
// It neither grants trust/consent nor transfers file or clipboard content.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Wen5555/LinkSend/internal/discovery"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
)

var sourceCommit = "unrecorded"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	name := flag.String("name", "", "explicit fixture name")
	peerFile := flag.String("peer-file", "", "local file containing only the expected peer identity")
	readyFile := flag.String("ready-file", "", "exclusive local ready receipt path")
	duration := flag.Duration("duration", 20*time.Second, "bounded run duration (max 2 minutes)")
	refresh := flag.Bool("refresh", false, "request one refresh after initial discovery")
	flag.Parse()
	if !strings.HasPrefix(*name, "AE-") || *peerFile == "" || *readyFile == "" || *duration <= 0 || *duration > 2*time.Minute || sourceCommit == "unrecorded" {
		return errors.New("require AE- fixture name, peer/ready files, bounded duration and build sourceCommit")
	}
	id, err := identity.Generate()
	if err != nil {
		return err
	}
	m, err := discovery.Start(discovery.Config{Identity: id, Name: *name})
	if err != nil {
		return err
	}
	defer m.Close()
	readyAt := time.Now()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *duration)
	defer cancel()
	var outputMu sync.Mutex
	emit := func(v any) {
		outputMu.Lock()
		defer outputMu.Unlock()
		_ = json.NewEncoder(os.Stdout).Encode(v)
	}
	ready := map[string]any{"event": "ready", "identity": id.ID(), "source_commit": sourceCommit, "at": readyAt.UTC(), "providers": m.ChannelErrors(), "control_port": m.ControlPort()}
	data, err := json.Marshal(ready)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*readyFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	if err = errors.Join(writeErr, f.Close()); err != nil {
		return err
	}
	emit(ready)
	var workers sync.WaitGroup
	defer workers.Wait()
	defer cancel()
	workers.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case incoming, ok := <-m.Incoming():
				if !ok {
					return
				}
				readCtx, end := context.WithTimeout(ctx, 2*time.Second)
				wire, readErr := incoming.Session.Read(readCtx)
				end()
				_ = incoming.Session.Close()
				if readErr == nil && wire.Type == "heartbeat" {
					emit(map[string]any{"event": "incoming_pinned_control", "peer_id": incoming.Peer.ID})
				}
			}
		}
	})
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	firstSeen := map[string]time.Time{}
	var peerID string
	var seen, controlAttempted, authenticated, refreshed bool
	var refreshAt time.Time
	for {
		select {
		case <-ctx.Done():
			emit(map[string]any{"event": "result", "source_commit": sourceCommit, "candidate_seen": seen, "authenticated_control": authenticated, "refresh_observed": refreshed, "providers": m.ChannelErrors(), "elapsed_ms": time.Since(readyAt).Milliseconds(), "file_transfer_tested": false})
			if !seen || !authenticated || *refresh && !refreshed {
				return errors.New("LAN probe incomplete; inspect bounded events")
			}
			return nil
		case <-ticker.C:
			peers := m.Peers()
			for _, peer := range peers {
				if _, exists := firstSeen[peer.ID]; !exists {
					firstSeen[peer.ID] = time.Now()
				}
			}
			if peerID == "" {
				body, readErr := os.ReadFile(*peerFile)
				if errors.Is(readErr, os.ErrNotExist) {
					continue
				}
				if readErr != nil {
					return readErr
				}
				peerID = strings.TrimSpace(string(body))
				if decoded, decodeErr := hex.DecodeString(peerID); decodeErr != nil || len(decoded) != 32 || peerID == id.ID() {
					return errors.New("expected peer identity must be a different 64-digit hex identity")
				}
			}
			peer, found := m.Device(peerID)
			if !found {
				continue
			}
			if !seen {
				seen = true
				if _, exists := firstSeen[peerID]; !exists {
					firstSeen[peerID] = time.Now()
				}
				emit(map[string]any{"event": "automatic_discovery", "peer_id": peerID, "ready_to_seen_ms": firstSeen[peerID].Sub(readyAt).Milliseconds(), "routes": peer.Routes, "candidate_only": true})
			}
			if !controlAttempted {
				controlAttempted = true
				dialCtx, end := context.WithTimeout(ctx, 3*time.Second)
				started := time.Now()
				session, _, route, dialErr := m.Dial(dialCtx, peerID)
				if dialErr == nil {
					dialErr = session.SendHeartbeat(dialCtx)
					_ = session.Close()
				}
				end()
				if dialErr != nil {
					failure := map[string]any{"event": "control_failed", "elapsed_ms": time.Since(started).Milliseconds(), "error_code": protocol.ErrorCode(dialErr)}
					var networkError *net.OpError
					if errors.As(dialErr, &networkError) {
						failure["operation"] = networkError.Op
					}
					var errno syscall.Errno
					if errors.As(dialErr, &errno) {
						failure["os_error"] = errno.Error()
					}
					emit(failure)
				} else {
					authenticated = true
					emit(map[string]any{"event": "pinned_control", "elapsed_ms": time.Since(started).Milliseconds(), "route": route})
				}
				if *refresh {
					refreshAt = time.Now()
					if err = m.Refresh(); err != nil {
						return err
					}
				}
			}
			if *refresh && !refreshed {
				for _, route := range peer.Routes {
					if route.LastSeen.After(refreshAt) {
						refreshed = true
						emit(map[string]any{"event": "fresh_announcement_after_refresh", "elapsed_ms": route.LastSeen.Sub(refreshAt).Milliseconds()})
						break
					}
				}
			}
		}
	}
}
