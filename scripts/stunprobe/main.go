// stunprobe checks production Pion gathering and a STUN-only server. It does
// not establish a peer connection, grant trust, or transfer file content.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/pion/ice/v4"
	"github.com/pion/stun/v4"
)

var sourceCommit = "unrecorded"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	bind := flag.String("bind", "", "concrete local IPv4 address (no port)")
	server := flag.String("stun", "", "one stun: UDP URL")
	timeout := flag.Duration("timeout", 10*time.Second, "overall timeout, 1–30 seconds")
	requireSrflx := flag.Bool("require-srflx", true, "fail unless production Pion gathers srflx")
	flag.Parse()
	ip, err := netip.ParseAddr(*bind)
	if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || *timeout < time.Second || *timeout > 30*time.Second || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(sourceCommit) || flag.NArg() != 0 {
		return errors.New("require concrete IPv4, bounded timeout and build-time sourceCommit")
	}
	uri, err := stun.ParseURI(*server)
	if err != nil || uri.Scheme != stun.SchemeTypeSTUN || uri.Proto != stun.ProtoTypeUDP {
		return errors.New("require stun: over UDP; relay=false")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	emit := func(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }
	e, err := connectivity.New(connectivity.Config{BindAddress: net.JoinHostPort(ip.String(), "0"), STUNURLs: []string{*server}, Generation: 1})
	if err != nil {
		return err
	}
	defer e.Close()
	started := time.Now()
	if err = e.Gather(); err != nil {
		return err
	}
	srflx := 0
	gathered := false
	for !gathered {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case candidate, ok := <-e.Candidates():
			if !ok {
				gathered = true
				break
			}
			c, parseErr := ice.UnmarshalCandidate(candidate.Value)
			if parseErr != nil {
				return parseErr
			}
			if c.Type() == ice.CandidateTypeRelay {
				return errors.New("unexpected relay candidate")
			}
			if c.Type() == ice.CandidateTypeServerReflexive {
				srflx++
			}
			if err = emit(map[string]any{"event": "candidate", "type": c.Type().String(), "address": c.Address(), "port": c.Port(), "related": c.RelatedAddress()}); err != nil {
				return err
			}
		}
	}
	if err = emit(map[string]any{"event": "gathered", "source_commit": sourceCommit, "base_socket": e.BaseAddress(), "elapsed_ms": time.Since(started).Milliseconds(), "srflx_count": srflx, "stats": e.Stats(), "relay": false, "file_transfer_tested": false}); err != nil {
		return err
	}
	// This is a separate control socket; ICE/QUIC continue to share e's socket.
	address := net.JoinHostPort(uri.Host, strconv.Itoa(uri.Port))
	negative, probeErr := checkSTUNOnly(ctx, ip.String(), address)
	if err = emit(negative); err != nil {
		return err
	}
	if *requireSrflx && srflx == 0 {
		probeErr = errors.Join(probeErr, errors.New("production Pion did not gather srflx"))
	}
	return probeErr
}

func checkSTUNOnly(ctx context.Context, bind, server string) (map[string]any, error) {
	result := map[string]any{"event": "stun_only_check", "binding_success": false, "allocation_observed": false}
	dialer := net.Dialer{LocalAddr: &net.UDPAddr{IP: net.ParseIP(bind)}}
	conn, err := dialer.DialContext(ctx, "udp4", server)
	if err != nil {
		return result, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	binding, err := stun.Build(stun.TransactionID, stun.BindingRequest, stun.Fingerprint)
	if err != nil {
		return result, err
	}
	response, err := exchange(ctx, conn, binding)
	if err != nil {
		return result, fmt.Errorf("binding: %w", err)
	}
	if response.Type != stun.BindingSuccess {
		return result, errors.New("binding was not successful")
	}
	var mapped stun.XORMappedAddress
	if err = mapped.GetFrom(response); err != nil {
		return result, err
	}
	result["binding_success"] = true
	result["mapped_address"] = mapped.String()
	allocate, err := stun.Build(stun.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassRequest), stun.RawAttribute{Type: stun.AttrRequestedTransport, Value: []byte{17, 0, 0, 0}}, stun.Fingerprint)
	if err != nil {
		return result, err
	}
	response, err = exchange(ctx, conn, allocate)
	if err != nil {
		var networkError net.Error
		if ctx.Err() == nil && errors.As(err, &networkError) && networkError.Timeout() {
			result["allocate_result"] = "no_response_within_2s"
			return result, nil
		}
		return result, fmt.Errorf("allocate negative probe: %w", err)
	}
	if response.Type == stun.NewType(stun.MethodAllocate, stun.ClassSuccessResponse) {
		result["allocation_observed"] = true
		return result, errors.New("server accepted TURN Allocate; expected STUN-only")
	}
	if response.Type != stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse) {
		return result, errors.New("unexpected response to TURN Allocate")
	}
	var code stun.ErrorCodeAttribute
	if err = code.GetFrom(response); err != nil {
		return result, err
	}
	result["allocate_result"] = "rejected"
	result["allocate_error_code"] = code.Code
	return result, nil
}

func exchange(ctx context.Context, conn net.Conn, request *stun.Message) (*stun.Message, error) {
	deadline := time.Now().Add(2 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	if _, err := conn.Write(request.Raw); err != nil {
		return nil, err
	}
	buffer := make([]byte, 2048)
	for range 32 {
		n, err := conn.Read(buffer)
		if err != nil {
			return nil, err
		}
		response := &stun.Message{Raw: append([]byte(nil), buffer[:n]...)}
		if err = response.Decode(); err != nil {
			return nil, err
		}
		if response.TransactionID != request.TransactionID {
			continue
		}
		if err = stun.Fingerprint.Check(response); err != nil {
			return nil, err
		}
		return response, nil
	}
	return nil, errors.New("too many unrelated STUN responses")
}
